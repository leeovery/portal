# Multiple `portal state daemon` instances run concurrently and pin the tmux server

Reproduced today against a long-running tmux server (uptime ~10 days, 20 sessions per `tmux ls`, mostly one window/one pane each). The user-visible complaint that opened the investigation was severe sluggishness across the entire tmux server: TUI redraws, prefix keystrokes, and `tmux ls` itself were all multi-second slow. The reporter is running many Claude sessions concurrently inside a single tmux server. Several sessions have accumulated large scrollbacks measured via `list-panes -F '#{history_bytes}'` (top: fabric-Cja82m 82 MB, fabric-lk26UG 58 MB, codeintel-54Jd4X 56 MB, evvi 50 MB, knowledge-wiki-V4aOHa 23–30 MB across two panes, several more in the 5–25 MB range). Note: `history_bytes` is the cell-grid representation; rendered `capture-pane` text is roughly 5–15× smaller — total `capture-pane` output across all 24 panes measured at ~28 MB.

At investigation time, **seven `portal state daemon` processes were running simultaneously**, all parented to the tmux server (PID 94966):

```
25482 (14:20 elapsed)   35467 (12:00)   46188 (09:40)
72062 (06:01)           79560 (04:15)   82148 (03:58)
82962 (24:38)
```

Three of those (72062, 79560, 82148) were observed appearing *during* the investigation, after `SIGSTOP` had been sent to four of the older ones. The four that were SIGSTOP'd did not stay stopped — within seconds they were back in `S` state in `ps` rather than `T`. Pausing all four originals did not reduce tmux CPU load nor prevent the additional three from spawning. The mechanism that resumed the stopped daemons and that spawned the new ones was not directly observed.

While the seven daemons were running, `ps` snapshots taken once per second showed 4–7 concurrent `tmux capture-pane -e -p -S -` processes at every snapshot, each one a child of one of the daemons. Sampling the tmux server (PID 94966) for 5 seconds showed 100% of CPU time spent in `cmd_capture_pane_exec` → `grid_string_cells` → `grid_string_cells_add_code`. The sample showed the server pegged at 75–98% CPU continuously and the system load average was 5–10 throughout.

Side observations made during the same window, not directly tied to the load:

- Three `sh -c 'portal state hydrate …; exec $SHELL'` wrappers from a previous bootstrap (~20 hours old) were still alive (paneKeys: agentic-workflows-XXrJ3J__1.1, leeovery-Gi5NLG__1.1, leeovery-feqhpg__1.1). Each parents an interactive `/bin/zsh`. Hydrate has already exited and the wrapper is parked waiting on its child interactive shell. The trailing `; exec $SHELL` in the wrapper is therefore unreachable in practice. Not load-causing — minor cruft, mentioned because the same paneKeys recur in `2026-05-08--killed-sessions-resurrect-on-restart.md`.

Impact while reproduced: every tmux operation across every session was sluggish; load average stayed elevated throughout.

## Daemon load model (read from code, not inferred)

`cmd/state_daemon.go:54-103`:

- 1s `time.NewTicker`; `tick()` runs synchronously inside the for/select loop. Go's ticker drops missed ticks if the consumer is slow, so during a long sweep the next tick fires immediately after the previous returns — **back-to-back sweeps when work is queued, not 1-second-aligned**.
- `tick()` fast path (lines 87-91): if `!dirty && !gap` (where `dirty = fileExists(save.requested)` and `gap = time.Since(LastSaveAt) >= MaxGap` with `MaxGap` defaulting to 30s), return immediately — one stat call, idle no-op.
- Otherwise `captureAndCommit` runs: sequential per-pane `CaptureAndHashPane` (line 135) → `WriteScrollbackIfChanged` (line 140). The hash dedup avoids the *file write* on unchanged scrollback, but `capture-pane` is always invoked to get the data to hash. The expensive serialise step happens regardless of whether the write is then a no-op.
- After a successful sweep: `LastSaveAt = now`, `save.requested` removed.

Hooks that touch `save.requested` (verified live with `tmux show-hooks -g`):

- `session-closed`, `session-created`, `session-renamed`, `window-linked`, `window-unlinked` → `portal state notify`
- `client-attached`, `client-session-changed` → `portal state signal-hydrate` (does not set save.requested directly)

So the dirty flag is set on **session/window topology changes only**, not on keystrokes, pane focus, output, or every tmux event. In a quiet system the flag is rarely set and the daemon falls back to the `MaxGap` 30s floor.

This implies two distinct load regimes per-daemon, both observed today:

| Condition | Daemon behaviour | tmux load |
|---|---|---|
| Idle (no topology events) | One sweep every 30s | Mostly idle; 1.5–4s spike per sweep |
| Topology churn (bootstrap, TUI session create, splits) | Hooks fire → dirty stays set → back-to-back sweeps | Continuous, sustained CPU |

With N daemons running concurrently, both regimes scale roughly N× — and during the seven-daemon observation, bootstrap and live-TUI activity were happening, putting the system in the topology-churn regime.

## Spawn mechanism — how N daemons accumulate

`portal_saver.go:32` documents the design intent:

> *tmux owns the daemon's lifecycle: when this session is killed (or the server dies), the kernel delivers SIGHUP to the daemon for graceful shutdown.*

And the daemon catches SIGHUP at `state_daemon.go:265-270`:

```go
sigCh := make(chan os.Signal, 1)
signal.Notify(sigCh, syscall.SIGHUP, syscall.SIGTERM)
go func() { <-sigCh; cancel() }()
```

Two structural reasons this fails in practice and lets daemons stack:

**(a) Bootstrap does not wait for the killed daemon to actually exit.** `EnsurePortalSaverVersion` (`portal_saver.go:106-114`) calls `KillSession` and then immediately calls `BootstrapPortalSaver`, which creates a new session+daemon as soon as `has-session` reports false. The killed daemon's `cancel()` only flips the context — `defaultDaemonRun` (`state_daemon.go:54-63`) does not check `ctx.Done()` until the current `tick()` returns. If a sweep is in flight when SIGHUP arrives, the dying daemon lingers for the remainder of that sweep (1.5–4s on this scrollback profile). During that window, the new daemon is already running. If another `portal` command fires inside that window, a third daemon spawns, and so on.

**(b) There is no flock / advisory lock.** `state_daemon.go:226` writes `daemon.pid` on startup but it is informational — last writer wins. `BootstrapAliveCheck` (`portal_saver.go:37`, default `state.DaemonAlive`) only signal-0-probes the PID currently in `daemon.pid`; it cannot see prior orphans whose PID has been overwritten. Nothing prevents two daemons from coexisting.

**Why the trigger fires often.** `portalSaverVersionMismatch` (`portal_saver.go:120-131`) returns true for any of: read error from version file, `currentVersion` is `""` or `"dev"`, stored is `""` or `"dev"`, or `stored != currentVersion`. The reporter has both `/opt/homebrew/bin/portal` (release v0.3.1) and `/Users/leeovery/Code/portal/portal` (dev build) on PATH, so any alternation between them — including hooks, scripts, or shell completions running one or the other — triggers a kill-and-respawn cycle.

**Walkthrough — how 7 daemons accumulate over time:**

```
T=0    │ portal cmd #1 (brew, v0.3.1)
       │ spawn daemon A1 in _portal-saver
       │ A1 alive
───────│───────────────────────────────────────────
T=10s  │ portal cmd #2 (dev build)
       │ version mismatch (dev ≠ 0.3.1)
       │ KillSession(_portal-saver)  → SIGHUP sent to A1
       │   A1: mid-sweep, doesn't reach ctx.Done() yet
       │       └────── A1 keeps running 3-4s ──────┐
       │                                            │
       │ BootstrapPortalSaver immediately:          │
       │   has-session = false (just killed)        │
       │   new-session → A2 spawned                 │
       │                                            │
       │   A1 alive (winding down) ─────────────────┘
       │   A2 alive
───────│───────────────────────────────────────────
T=12s  │ portal cmd #3 (brew again, e.g. shell hook)
       │ version mismatch (0.3.1 ≠ dev)
       │ KillSession → SIGHUP to A2
       │   A2: mid-sweep, doesn't see ctx.Done() yet
       │ BootstrapPortalSaver → spawn A3
       │
       │ Now alive: A1 (winding down), A2 (winding down), A3 (fresh)
       │
       │ Each daemon's sweep takes 3-4s; a third command in the same
       │ window stacks another. With brew/dev alternation across
       │ many invocations over a 10-day server uptime, orphans
       │ accumulate.
```

**Why orphan daemons end up unparented to `_portal-saver`.** After `KillSession` removes the session, the lingering daemon is no longer associated with any tmux pane (the pane is gone), but it remains parented to the tmux server PID and still has a controlling pty (the now-dangling pane's). This matches the investigation-start observation exactly: 7 daemons all parented to tmux server PID 94966, but only 1 actually inside `_portal-saver`. The other 6 were past lingerers whose SIGHUP arrived during sweeps they never reached `ctx.Done()` for.

**Fix surface for daemon multiplication:**

1. **Add a flock-based singleton.** `flock(state_dir/daemon.lock)` at daemon startup; release on exit. Bootstrap acquires-or-fails-fast: if another daemon holds the lock, don't spawn.
2. **Make `KillSession` synchronous.** After `c.KillSession`, poll `BootstrapAliveCheck` against the prior `daemon.pid` until it reports dead, bounded timeout. Don't proceed to `new-session` while the old daemon is still alive.
3. **Optionally tighten the version-mismatch trigger** so `dev/dev` or empty/empty cases don't always re-spawn.

(1) alone closes the "two daemons coexist" window structurally. (2) makes upgrade transitions clean. (3) reduces frequency of (1) being relied on.

## SIGTERM and aftermath

The reporter `kill -TERM`'d all seven daemons. Subsequent observations:

- Within ~9 minutes, two new `portal state daemon` processes existed (PIDs 22734, 48924), parented to the tmux server. The `_portal-saver` tmux session was no longer present in `tmux ls`. How and where the two new daemons were spawned was not directly observed.
- With those two daemons running, the tmux server **stayed pegged at 75–97% CPU**. A fresh 5-second sample showed the same call graph as the seven-daemon sample.
- Some minutes later, both of those daemons had also exited. With zero daemons running, tmux server CPU dropped to 0–22% across a 3-second `top` sample, and `ps aux | grep capture-pane` showed zero capture-pane processes for 5 consecutive 1-second samples. Why those two exited was not directly observed.
- Triggering a fresh `portal list` while at the zero-daemon idle state immediately spawned one daemon and recreated `_portal-saver`. tmux CPU went from 0% to 70% within 1 second of `portal list` returning, sustained 78–97% for the next 30 seconds — the bootstrap-driven topology-churn regime described above.

The "two daemons → still pegged" data point indicates a real load contribution per daemon. **However, the load is not constant per-daemon; it depends on whether the system is in a topology-churn regime.** A later 30-sample monitor of the same single daemon during a quiet period showed 0% CPU on every sample with capture-pane count fluctuating 0–1 — fully consistent with the gap-floor regime, not pegged.

## Capture cost validation (zero-daemon clean baseline)

After the daemons exited and tmux returned to idle, individual `tmux capture-pane -e -p -S -t <pane>` calls were timed against the live system. Per-pane wall time scaled with rendered output size. Selected results (one capture per pane, sequential, no daemon running):

| Pane | history_bytes | wall time | output bytes |
|---|---|---|---|
| fabric-Cja82m:1.1 | 82 MB | 388 ms | 5.1 MB |
| fabric-lk26UG:1.1 | 58 MB | 308 ms | 4.8 MB |
| codeintel-54Jd4X:1.1 | 56 MB | 705 ms | 3.7 MB |
| evvi:1.1 | 50 MB | 859 ms | 2.9 MB |
| knowledge-wiki-V4aOHa:1.1 | 23 MB | 280 ms | 2.6 MB |

A full sweep across **all 24 panes** (one daemon-tick equivalent) was timed back-to-back:

- **Cold sweep:** 3,866 ms total. tmux server CPU peaked at 87.5%.
- **Warm sweep (immediate re-run):** 1,524 ms total. tmux CPU peaked at 48.4%.

Both exceed the 1,000 ms tick interval. In the topology-churn regime where the dirty flag is repeatedly re-set, the daemon's sweeps run back-to-back: a cold sweep finishes at T+4s, the next tick fires at T+4s+ε, and the cycle repeats — explaining sustained pegging even at N=1 during that regime.

Same 24-pane sweep with `-S -100` (last 100 lines per pane only): **293 ms total, 213 KB total output** — 13× faster, 130× less data, well inside the tick budget. This isolates the cost driver to unbounded scrollback in `-S -` (`internal/tmux/tmux.go:625`), not the per-call overhead of `capture-pane` itself.

## Implications for the fix

1. **Singleton lock is necessary but not sufficient.** Even at N=1, in the topology-churn regime each sweep takes 1.5–4s and runs back-to-back, blowing the 1s tick budget at this scrollback profile.
2. **Bounding scrollback (`-S -<N>`) is the high-leverage lever.** A cap in the low thousands of lines would bring sweep duration well inside the tick budget.
3. **Consider tightening when the dirty flag is set.** Today only topology hooks set it (verified above), so this isn't the worst offender, but the bootstrap window itself is a topology-event burst that drives the system into the back-to-back regime exactly when the user is most actively using portal.
4. **Hash dedup before capture, not after, would help further.** Today `capture-pane` is always invoked to compute the hash; the saving in `WriteScrollbackIfChanged` only avoids the file write. A cheaper change-detection signal (e.g., `display-message -p '#{cursor_x},#{cursor_y},#{history_size}'` per pane) before the full capture would let the dedup prevent the expensive call.

## Related bugs

Companion bugs that may share machinery (logged separately, not assumed to be the same defect):

- `.workflows/.inbox/bugs/2026-05-08--killed-sessions-resurrect-on-restart.md`
- `.workflows/daemon-merge-reintroduces-dead-sessions/` (currently in implementation)

## Earlier-version corrections

This file was rewritten after corrections. Earlier drafts contained statements that were not directly verified:

- "All seven daemons appear in the same `_portal-saver` tmux session" — only one was directly verified to be in that session.
- "~140 capture-pane invocations per second" — was a calculation from `daemons × panes × tick rate`, not a measurement; the actual measurement was 4–7 *concurrent* (in-flight at any moment).
- "Total scrollback ~450 MB" — was based on `history_bytes`, which is cell-grid size, not rendered text. Actual rendered text totals ~28 MB across all panes.
- "Hooks aren't registered after the daemon kill" — wrong; they were registered the whole time. The earlier check used a too-narrow grep against `show-options`. Re-verified with `tmux show-hooks -g`.
- "Dirty flag is set on every tmux event" — wrong; only on the seven topology-change hooks listed above.
- "1s tick + 3-4s sweep = always pegged" — incomplete; only holds in the topology-churn regime. Idle-system behaviour is gap-floor-driven and quiet.
