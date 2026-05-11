# Investigation: Multiple State Daemons Running Concurrently

## Symptoms

### Problem Description

**Expected behavior:**
Exactly one `portal state daemon` process runs per tmux server lifetime. Bootstrap-driven kill+respawn cycles (e.g. version mismatch upgrades) produce a clean handover: the old daemon exits before the new one is allowed to start writing to shared state. Per-tick CPU cost stays bounded so the daemon does not pin the tmux server.

**Actual behavior:**
Up to **seven** `portal state daemon` processes were observed simultaneously, all parented to the tmux server (PID 94966). Only one was inside the `_portal-saver` session; the other six were past-lifecycle orphans whose owning session had already been killed but whose `cancel()` had not yet been observed by their in-flight `tick()`. While 2+ daemons ran, the tmux server stayed pegged at 75–98% CPU; `sample` showed all time in `cmd_capture_pane_exec` → `grid_string_cells` → `grid_string_cells_add_code`.

### Manifestation

- Severe sluggishness across the entire tmux server: TUI redraws, prefix keystrokes, and `tmux ls` itself were multi-second slow.
- Load average sustained at 5–10 during the observation window.
- `ps` snapshots once per second showed 4–7 concurrent `tmux capture-pane -e -p -S -` child processes (one per running daemon mid-sweep).
- With zero daemons running, tmux server CPU dropped to 0–22% and capture-pane processes dropped to zero.

### Reproduction Steps

**Trigger mechanism still to be confirmed by code analysis.** The inbox report attributed the kill-respawn cycle to a mixed `release/dev` `portal` binary alternation, but the user has clarified they do **not** have two binaries on PATH. So the version-mismatch trigger (if that is even the trigger) fires for a different reason — possibly the `dev/dev` or empty-stored cases in `portalSaverVersionMismatch`, or via a path unrelated to version mismatch entirely.

What is reproducible regardless of trigger:

1. Whenever `EnsurePortalSaverVersion` decides to recycle the saver, it calls `KillSession(_portal-saver)` then immediately `BootstrapPortalSaver` — no aliveness barrier.
2. If the killed daemon is mid-`tick()` (1.5–4 s on the observed scrollback profile), the old daemon stays alive while the new one is already running.
3. Over enough invocations, daemons accumulate.

**Reproducibility:** Single observation so far (2026-05-09). Conditions for accumulation are structural (the bootstrap race), so any repeat of the trigger reproduces.

### Environment

- **Affected environments:** Any tmux server lifetime where the version-mismatch path is taken repeatedly. Empirically observed on macOS (`darwin`), but the root cause is platform-independent.
- **User conditions:** Mixed `release/dev` binaries on PATH; long-running tmux server; many panes with substantial scrollback.

### Impact

- **Severity:** High. The whole tmux server (all sessions, not just portal-managed ones) becomes unusable for sustained periods.
- **Scope:** Any user running portal long enough to accumulate orphan daemons; worst with mixed release/dev binaries.
- **Business impact:** N/A (developer tool); user-experience degradation severe.

### References

- Inbox file (archived): `.workflows/.inbox/.archived/bugs/2026-05-09--multiple-state-daemons-running-concurrently.md`
- Related (separate work units, may share machinery but not assumed identical):
  - `.workflows/killed-sessions-resurrect-on-restart/` (active, in implementation)
  - `.workflows/daemon-merge-reintroduces-dead-sessions/` (completed)

---

## Analysis

### Initial Hypotheses

1. **Bootstrap does not wait for the killed daemon to exit before spawning a replacement.** Read from `portal_saver.go:106-114` — `KillSession` then immediate `BootstrapPortalSaver` with no aliveness barrier.
2. **No singleton lock.** `state_daemon.go:226` writes `daemon.pid` informationally only; `BootstrapAliveCheck` (`portal_saver.go:37`) signal-0-probes only the *current* `daemon.pid` and cannot see prior orphans whose PID has been overwritten.
3. **Some path is firing the saver-recycle cycle frequently enough that orphans accumulate.** The inbox file blamed mixed release/dev binaries on PATH but the user has none — so we need to identify what actually triggers the kill-respawn cycle. Candidates: empty stored version on disk, `dev`/empty current version embedded in the binary, parallel `portal` invocations stomping each other, or non-version-mismatch code paths that also call `KillSession(_portal-saver)`.

(1) and (2) are the structural defects — they make accumulation possible. (3) determines frequency. The fix focuses on (1)+(2); (3) is diagnostic context.

To validate against current code.

### Code Trace

**Trigger path** — every `portal` command goes through bootstrap step 4 (`EnsureSaver`):

1. `cmd/bootstrap/bootstrap.go:260-266` — Step 4 calls `o.Saver.EnsureSaver()`. Best-effort; failure becomes `SaverDownWarning`, success continues silently.
2. `cmd/bootstrap_production.go:58-60` — `saverAdapter.EnsureSaver()` delegates to `tmux.EnsurePortalSaverVersion(client, stateDir, version)`, passing the binary's ldflags-injected version.
3. `internal/tmux/portal_saver.go:106-114` — `EnsurePortalSaverVersion`:
   - Reads stored version: `state.ReadVersionFile(stateDir)`.
   - `portalSaverVersionMismatch(stored, currentVersion, readErr)` (lines 120-131) returns true if **any** of: read error, `currentVersion ∈ {"", "dev"}`, `stored ∈ {"", "dev"}`, or `stored != currentVersion`.
   - If mismatch AND `HasSession(_portal-saver)` → tolerant `KillSession(_portal-saver)`.
   - Unconditionally calls `BootstrapPortalSaver` next.
4. `internal/tmux/portal_saver.go:63-83` — `BootstrapPortalSaver`:
   - `HasSession(PortalSaverName)` — observes the kill just took effect → false.
   - Falls through to `createPortalSaverWithRetry`.
5. `internal/tmux/portal_saver.go:138-158` — `createPortalSaverWithRetry`:
   - `c.NewDetachedSessionNoCwd(PortalSaverName, "portal state daemon")` creates a fresh detached tmux session whose initial process is `portal state daemon`. **New daemon process A2 forks-execs as soon as tmux returns from new-session.**
   - On error, retries up to 3 attempts with `HasSession` race-resolution.

**Race window inside the old daemon (A1):**

6. `cmd/state_daemon.go:54-63` — `defaultDaemonRun` is a `for { select { ticker.C / ctx.Done() } }` loop. `tick()` runs synchronously inside the same select arm; `ctx.Done()` is only reachable **between** ticks, never during one. So if SIGHUP arrives at A1 mid-sweep, A1 keeps running for the remainder of the current `tick()` (measured 1.5–4 s on the observed scrollback profile).
7. `cmd/state_daemon.go:265-270` — `signal.Notify(sigCh, SIGHUP, SIGTERM)` then `go func() { <-sigCh; cancel() }()`. The goroutine flips ctx, but ctx.Done() is gated by tick completion (point 6).
8. `cmd/state_daemon.go:115-158` — `captureAndCommit` runs sequentially: marker list → `CaptureStructure` → per-pane `CaptureAndHashPane` (line 135) + `WriteScrollbackIfChanged` (line 140) → `state.Commit`. `CaptureAndHashPane` always invokes `capture-pane` to compute the hash; the dedup at `WriteScrollbackIfChanged` only saves the file write.

**PID file overwrite (no singleton enforcement):**

9. `cmd/state_daemon.go:226` — `state.WritePIDFile(dir, os.Getpid())` is called by A2 early in startup, **before** the tick loop begins.
10. `internal/state/daemon_state.go:33-36` — `WritePIDFile` is `fileutil.AtomicWrite` (temp + rename). No locking; last writer wins.
11. `internal/state/daemon_state.go:82-88` — `DaemonAlive` reads `daemon.pid` then `IsProcessAlive` (signal-0 probe). After A2 overwrites, `DaemonAlive` sees A2's PID and reports alive — **A1's PID is no longer recorded anywhere** and is invisible to `BootstrapAliveCheck`. A1 is now an unreachable orphan.

**Scrollback-cost backing data** — confirms `internal/tmux/tmux.go:625` `capture-pane -e -p -S -` (unbounded scrollback) is the cost driver. Inbox measurements showed a 24-pane sweep at 3.9 s cold / 1.5 s warm; same sweep with `-S -100` measured 293 ms (13× faster, 130× less data).

### Trigger frequency (resolved)

The inbox file blamed mixed release/dev binaries on PATH. The user has confirmed they don't. The actual trigger is almost certainly:

- The user's binary is built without `-X github.com/leeovery/portal/cmd.version=<release>` ldflags, so `version` is `""` or `"dev"`.
- `portalSaverVersionMismatch` returns true on the `currentVersion == "" || currentVersion == "dev"` branch (`portal_saver.go:124`).
- **Every `portal` command therefore triggers a kill-respawn** of `_portal-saver`. No mixed-binary alternation needed.

The user reportedly runs many `portal` commands per minute (Claude-driven tmux automation). Combine that frequency with sweep durations > tick interval, and 7+ orphan daemons accumulating over hours is unsurprising. The bootstrap race (no sync wait on the killed daemon) is the structural defect; the dev-build version-mismatch path is the high-frequency producer that makes the race exploitable.

### Root Cause

The bug is the conjunction of two structural defects in the saver-bootstrap and daemon-startup pair:

1. **No singleton enforcement.** `state.WritePIDFile` and `BootstrapAliveCheck` cooperate as an informational pidfile, not as a singleton lock. Once two daemons coexist (for any reason), nothing in the system prevents both from running concurrent capture loops over the same state directory.
2. **Bootstrap does not synchronise with the killed daemon's exit.** `EnsurePortalSaverVersion` issues `KillSession`, then `BootstrapPortalSaver` immediately observes "no session" and creates a fresh one. The new daemon (A2) starts and writes its PID before the old daemon (A1) — currently mid-sweep — observes the cancelled context. A1 finishes its sweep with its PID overwritten and becomes invisible to any future `BootstrapAliveCheck`.

These two defects compose: (1) means N daemons can coexist; (2) is the recurring mechanism that pushes N from 1 to 2+ on every saver recycle.

**Why this happens:** The original design (per the `portalSaverCommand` comment at `internal/tmux/portal_saver.go:28-33`) was: *"tmux owns the daemon's lifecycle: when this session is killed (or the server dies), the kernel delivers SIGHUP to the daemon for graceful shutdown."* True in principle, but graceful ≠ instant — the daemon shuts down at the end of its current sweep, not on signal receipt. The lifecycle promise is honoured eventually, but the bootstrap path treats it as synchronous.

### Contributing Factors

- **Dev-build version-mismatch path** fires on every `portal` invocation when the binary has `version == "" | "dev"` — the high-frequency producer that exploits the bootstrap race.
- **Unbounded scrollback capture** (`tmux.go:625` `capture-pane -S -`) makes each sweep 1.5–4 s at the user's scrollback profile. The Go ticker drops missed fires, so when a sweep overruns the 1 s tick interval the next tick fires immediately on completion — daemons in this regime never reach `ctx.Done()` between sweeps, extending the orphan-eligibility window indefinitely.
- **Long-running tmux server with high scrollback** (24 panes, ~28 MB rendered text, top `history_bytes` 82 MB) — the conditions under which sweep overrun becomes the dominant regime.
- **Topology-churn from bootstrap itself.** Bootstrap step 1 (`EnsureServer`) and step 4 (`EnsureSaver`) can fire `session-created`/`session-closed` hooks (via the recycle) that write `save.requested`, keeping the daemon's dirty flag set and pushing it into the back-to-back-sweep regime exactly when the user is most actively running `portal` commands.

### Why It Wasn't Caught

- **No singleton invariant test.** The comment at `portal_saver.go:31-32` documents the desired property but no integration test asserts it. A test that runs two `EnsurePortalSaverVersion` calls back-to-back in a real-tmux fixture and asserts a single live daemon would catch this in CI.
- **Unit-level seam tests** (`BootstrapAliveCheck` is a `var` for test override) verify the alive-check **for a given pidfile** but cannot model "what happens when the pidfile is overwritten while the prior daemon still runs."
- **Sweep-duration unrealistic in CI.** The bug is latent at N=1 with sub-second sweeps; it only manifests when sweep overrun + saver-recycle frequency combine. No load test exists at realistic scrollback scale.
- **Dev-build trigger is silent.** A `dev` daemon happily writes daemon.version="dev", then `dev != dev` is technically not a mismatch — but the early-return `currentVersion == "dev" → true` short-circuits before the equality check, so every dev-build run trips the mismatch path unconditionally without any logged signal that this is happening.

### Blast Radius

**Directly affected:**
- All tmux operations on the affected tmux server (not just portal-managed sessions): TUI redraws, prefix keystrokes, `tmux ls` itself.
- `_portal-saver` session lifecycle and the resurrection capture loop.
- Shared state files written by the daemon: `sessions.json` (atomic, but two daemons can race the `_, _ = cancel()` between read and commit, producing flip-flop sessions across consecutive ticks), per-pane scrollback `.bin` files (`fileutil.AtomicWrite` is per-call atomic, but two daemons writing the same pane key can interleave content versions across ticks).
- `daemon.pid` and `daemon.version` markers — incoherent under multiplication; `BootstrapAliveCheck` becomes meaningless once N > 1.
- `save.requested` flag — both daemons race to remove it on successful sweep; remove on the loser's side is a benign no-op via `errors.Is(err, fs.ErrNotExist)`.

**Potentially affected:**
- FIFO sweep paths: two daemons could both call into `state` cleanup helpers concurrently. The `FIFOSweeper` runs only in bootstrap (single-shot per process), so daemon-side FIFO interaction is read-only — likely safe but worth confirming during fix design.
- Any future seam expecting daemon-singleton semantics (e.g. a centralised hook queue) would silently break.

---

## Fix Direction

To be filled in during Step 6 (Root Cause Synthesis) and Step 8 (Findings Review).

Initial fix surface candidates (from inbox file, to validate):

1. **Singleton flock** at daemon startup; bootstrap acquires-or-fails-fast.
2. **Synchronous KillSession** — poll `BootstrapAliveCheck` against the prior `daemon.pid` until dead, bounded timeout.
3. **Bound `capture-pane -S -<N>`** to cap per-tick cost (measured 13× speedup, 130× less data with `-S -100`).
4. **Tighten `portalSaverVersionMismatch`** so dev/dev and empty/empty cases don't always re-spawn.
5. **Cheaper change-detection** before `capture-pane` (e.g. `display-message -p '#{cursor_x},#{cursor_y},#{history_size}'`) so the hash dedup avoids the expensive call, not just the file write.

(1)+(2) close the structural multiplication. (3) is the high-leverage performance lever even at N=1. (4) reduces frequency. (5) helps further.

---

## Notes

- The bug report explicitly self-corrected several earlier claims (history_bytes ≠ rendered text; daemons not all in `_portal-saver`; hooks were registered the whole time; dirty flag only set on topology events). Trust the corrected version.
- Companion side-observation: 3 stale `sh -c 'portal state hydrate …; exec $SHELL'` wrappers from a ~20-hour-old bootstrap. PaneKeys recur with `killed-sessions-resurrect-on-restart`. Trailing `; exec $SHELL` is unreachable because hydrate has exited and the wrapper is parked on the child shell. Not load-causing — log as separate observation, not part of this fix.
