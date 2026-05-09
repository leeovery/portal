# Investigation: Killed Sessions Resurrect On Restart

## Symptoms

### Problem Description

**Expected behavior:**
- When a tmux session is killed — either by the user pressing the tmux prefix + `Alt-Q` (`kill-session`) from inside the session, or by selecting the session in the Portal TUI list and pressing `K` then confirming — the session should disappear from the Portal list AND stay gone across the next `portal open`.
- Per-pane on-resume hooks registered with `portal hooks set --on-resume "<cmd>"` should fire end-to-end on the next bootstrap, in the restored pane, with the configured command running before the user's interactive shell.

**Actual behavior:**
- **Symptom A** — Killed sessions disappear from the Portal list as expected, but on the very next `portal open` they reappear. Only some sessions are affected, consistently the same ones (on the reporter's machine: `leeovery-Gi5NLG`, `leeovery-feqhpg`). Other sessions kill cleanly.
- **Symptom B** — On-resume hooks never fire end-to-end for the affected panes. The reporter has not yet observed the registered command run on any restored pane.

The reporter and a prior investigator agreed both symptoms appear to share a single upstream trigger inside the hydrate path.

### Manifestation

Reported against Brew-installed `portal 0.3.1`. HEAD spot-checks of the relevant code paths suggest unchanged from the released binary, so HEAD is expected to reproduce — to be confirmed.

Observed at the time of the report:

- Three `@portal-skeleton-<paneKey>` tmux server-options were still set despite no restoration being in progress:
  - `agentic-workflows-XXrJ3J__1.1`
  - `leeovery-Gi5NLG__1.1`
  - `leeovery-feqhpg__1.1`
- Symptom A reproducers (`leeovery-Gi5NLG`, `leeovery-feqhpg`) are in this set.
- `~/.config/portal/state/portal.log` contains, dated to the most recent bootstrap, repeated entries of two distinct shapes for those same paneKeys:
  - `WARN | hydrate | write fifo …/hydrate-<paneKey>.fifo: open …: no such file or directory` — emitted by the `signal-hydrate` server-side handler (the global `client-attached` hook), failing because the per-pane FIFO does not yet exist when the signal lands.
  - `WARN | hydrate | timeout waiting for signal on --hook-key=<sess>:<w>.<p> --fifo=…` — emitted by the per-pane hydrate helper after waiting 3 s for a signal on its FIFO that never arrives.
- The reporter's `~/.config/portal/state/sessions.json` was being rewritten by the daemon (recent mtime), and the saved index continued to list the affected sessions even after they were killed.

### Reproduction Steps

1. Boot Portal so that bootstrap step 5 reconstructs the affected sessions.
2. Observe `portal.log` for `WARN | hydrate | write fifo …` and `WARN | hydrate | timeout waiting for signal on …` entries against specific paneKeys.
3. Inspect server options: `tmux show-options -s | grep '@portal-skeleton-'` — affected paneKeys still have markers set.
4. Kill one of the affected sessions (TUI `K`, or `tmux kill-session` from inside it). Confirm it disappears from the Portal list.
5. Run `portal open` again. Affected session reappears.
6. Inspect any registered on-resume hook for an affected pane — it has not run; the pane is just a bare interactive shell.

**Reproducibility:** Always, for the affected panes on the reporter's machine. Not all panes are affected — the conditioning factor is whatever causes the hydrate-signal race to land "signal first, FIFO second" for a specific pane.

### Environment

- **Affected environments:** Reporter's local macOS install (Brew `portal 0.3.1`).
- **Platform:** tmux + Portal CLI (Go).
- **User conditions:** Long-running tmux sessions reconstructed by bootstrap step 5 (Restore). Affected paneKeys are the same across reboots once they enter the stuck state.

### Impact

- **Severity:** Medium. Symptom A is low-grade but persistent — users cannot reliably clear sessions and a confirmed kill that is silently undone is a credibility issue. Symptom B effectively makes the on-resume-hooks feature non-functional in any environment where any pane times out hydrate.
- **Scope:** All users whose hydrate signals race ahead of the helper FIFO open. Probably common, not rare — same paneKeys reproduce across every bootstrap on the reporter's machine.
- **Business impact:** N/A (single-user CLI tool). User-trust impact: confirmed kills being undone, and a documented feature (on-resume hooks) silently not running.

### Supporting Observations (may or may not be related)

- `signal-hydrate` "write fifo … no such file or directory" entries appear **twice at the same timestamp** for the same paneKey on some bootstraps. Suggests the `client-attached` hook is firing more than once in quick succession — possibly from the alt-screen toggle in TUI bootstrap, or from how the `signal-hydrate` global hook is registered. Whether this matters for the bug at hand is unclear.
- `portal.log` also contains transient `WARN | daemon | capture pane <paneKey>: failed to capture pane "<paneKey>": exit status 1` entries against live, non-stuck sessions. Sporadic; same panes capture cleanly on later ticks. Possibly a race with `respawn-pane` during the hydrate window. Not user-visible but worth investigating alongside this work.
- The volume of warnings on every bootstrap is high enough to drown out future genuine warnings.

### Addendum 2026-05-09 — orphan `sh -c` wrappers post-timeout

When investigating a separate slowness issue, three `sh -c 'portal state hydrate …; exec $SHELL'` wrappers from the previous day's bootstrap were still alive (~20 hours old) for the same three paneKeys this bug names. On inspection, the inner `portal state hydrate` had long exited (presumably via the timeout path → `execShellAndExit` → exec'd the user's shell into the pane). The wrapper `sh` is parked waiting on the now-interactive shell child, which won't exit while the user has the pane open. The trailing `; exec $SHELL` in the wrapper is therefore dead code in practice — the helper has already exec'd `$SHELL` itself, and the wrapper's own `; exec $SHELL` after that is unreachable.

Probably minor (not load-causing), but every timed-out hydrate leaves a `sh` process parented to the tmux server until the pane closes, which in long-running sessions effectively means forever. Worth considering as part of the timeout-path redesign here, since the wrapper construction is the same code site that owns the bypass-hooks decision (Symptom B).

### References

- Original inbox file (now archived): `.workflows/.inbox/.archived/bugs/2026-05-08--killed-sessions-resurrect-on-restart.md`
- Related bug (already fixed): `.workflows/completed/daemon-merge-reintroduces-dead-sessions/` — the daemon merge defect that turns "stale `@portal-skeleton-*` marker" into "killed session reappears in `sessions.json`".
- Originating feature scope: `.workflows/completed/built-in-session-resurrection/` — source of truth for the timeout-path design choices that this bug interrogates.
- Likely-relevant code paths (pointers, NOT a fix proposal):
  - `cmd/state_signal_hydrate.go` — the `client-attached` handler that writes to the per-pane FIFO. Race partner.
  - `cmd/state_hydrate.go` — `runHydrate`, `handleHydrateTimeout`, `execShellAndExit`, `execShellOrHookAndExit`. Race partner; also where the timeout-path's "no hook firing" and "marker stays set" decisions live.
  - `internal/state/markers.go` — `@portal-skeleton-*` server-option lifecycle (`SetSkeletonMarker`, `UnsetSkeletonMarker`).
  - `cmd/state_hydrate.go` step 8 — the only success-path marker unset; non-fatal on failure.
  - `cmd/bootstrap/bootstrap.go` — step 2 (`RegisterPortalHooks`) for how `client-attached` is registered, step 5 (`Restore`) for the skeleton-build that primes the FIFO, step 7 (`SweepOrphanFIFOs`) for orphan-FIFO cleanup.
  - `~/.config/portal/state/portal.log` and `~/.config/portal/state/sessions.json` — useful artefacts to inspect on a reproducing machine.

---

## Analysis

### Initial Hypotheses

**Working hypothesis (to be validated):** The upstream trigger is a race between two bootstrap-step-5 actors:

- `signal-hydrate` (the global `client-attached` handler) writes a single byte to the per-pane FIFO at `…/hydrate-<paneKey>.fifo`.
- The hydrate helper (`portal state hydrate`, launched as the pane's initial process via `respawn-pane -k` during skeleton reconstruction) `mkfifo`s the FIFO and then `O_RDONLY`-blocks reading from it.

When the signal lands before the helper has the FIFO open, the writer hits ENOENT and the helper times out 3 s later.

Two design decisions in `handleHydrateTimeout` then compound the timeout into the user-visible symptoms:

1. **`handleHydrateTimeout` deliberately leaves the `@portal-skeleton-<paneKey>` server-option set** (commented as "marker stays set so the next attach re-signals"). The "next attach re-signals" promise is itself questionable — by the time the timeout fires, the helper has exec'd a bare shell, so the FIFO has no reader anymore; a subsequent `client-attached` signal would just hit ENOENT again. Meanwhile the persistent marker drives Symptom A (via the daemon-merge re-injection path, which is itself an already-fixed adjacent bug).
2. **`handleHydrateTimeout` deliberately routes to `execShellAndExit` (bare shell), bypassing the hook-firing exec.** This drives Symptom B.

### Code Trace

**Bootstrap step 5 (cold-start only — `internal/restore/restore.go:116-119`):**

```go
if _, alive := liveSet[sess.Name]; alive {
    // Silent skip per spec — the steady-state common case.
    return
}
```

Restore SKIPS already-live sessions. The skeleton path (and therefore the entire hydrate machinery) ONLY runs on the first `portal` invocation after a tmux server cold-start. Subsequent invocations of `portal open` see all sessions live and skip restore for them. **This bug is therefore a cold-start phenomenon** — the user sees it on first attach after a reboot or after the user kills the tmux server, not on every `portal open`.

**Skeleton arm phase (`internal/restore/session.go:194-227`, `armPanes`):**

For each saved pane in each non-live saved session:
1. `state.CreateFIFO(fifo)` — `mkfifo` at `<state>/hydrate-<paneKey>.fifo`. Creates the path.
2. `r.Client.RespawnPane(liveTarget, hydrateCmd)` — runs `respawn-pane -k`, killing the default shell created by `new-session`/`split-window` and replacing it with the wrapper from `buildHydrateCommand` (`internal/restore/session.go:419-426`):
   ```
   sh -c 'portal state hydrate --fifo X --file Y --hook-key Z; exec $SHELL'
   ```
3. After all panes in a session, `ApplySkeletonMarkers` (`internal/restore/session.go:351-359`) sets `@portal-skeleton-<liveKey>` for every live pane.

**Helper start (`cmd/state_hydrate.go:98-184`, `runHydrate`):**

Inside the wrapper sh, `portal state hydrate` runs and immediately calls `cfg.OpenFIFO(cfg.FIFO, hydrateTimeout)` (`runHydrate` step 1) — `os.OpenFile(path, O_RDONLY, 0)` blocks until a writer arrives or the 3-second timeout fires.

**Signal path (`cmd/state_signal_hydrate.go:58-84`, `runSignalHydrate`):**

Triggered by tmux's `client-attached` and `client-session-changed` global hooks (registered in `internal/tmux/hooks_register.go:32-35` against both events). Receives the session name via `#{session_name}`, enumerates panes IN THAT SESSION ONLY, looks up each pane's marker, and writes a single byte to its FIFO via `O_WRONLY|O_NONBLOCK`.

**Hook firing (`cmd/state_hydrate.go:221-239`, `execShellOrHookAndExit`):**

Reached only on the **signal-arrived** path (step 9 of `runHydrate`) and the **file-missing** path (`handleHydrateFileMissing`, `state_hydrate.go:279-300` — also unsets the marker before falling through). Looks up the structural hook key in `hooks.json` and exec's `sh -c '<HOOK>; exec $SHELL'` if found, or bare `$SHELL` if not.

**Timeout path (`cmd/state_hydrate.go:248-266`, `handleHydrateTimeout`):**

Fires when `openFIFOWithTimeout` returns `ErrHydrateTimeout` after 3 s with no signal. The handler:

1. Writes the reset preamble to stdout.
2. `os.Remove(cfg.FIFO)` — unlinks the FIFO from the filesystem.
3. Logs `WARN | hydrate | timeout waiting for signal on …`.
4. **Deliberately does NOT** unset `@portal-skeleton-<paneKey>` (comment at line 262: "marker stays set so the next attach re-signals").
5. Deliberately does NOT sleep 100 ms.

`runHydrate` then falls through to `execShellAndExit` (line 109) — bare shell, **no hook firing**.

### Root Cause

The bug has **one upstream trigger** producing **three downstream symptoms**, plus a fourth defect in the wrapper construction.

**Upstream trigger — selective signaling:**

`signal-hydrate` is fired by `client-attached` / `client-session-changed`, which are **per-session** hooks scoped via `#{session_name}` to the session the client is attaching to or switching to. Bootstrap step 5 creates skeletons for **every** non-live saved session, but `signal-hydrate` only writes to the FIFOs of the session being attached (the user's `portal open <X>` target). On a cold-start bootstrap with N saved sessions, only the panes of session X get signaled. The remaining N−1 sessions' helpers wait 3 seconds, never receive the signal, and time out.

This is not a race with the helper's `O_RDONLY` open — it's a missing trigger. Even if the helper opens the FIFO immediately, no signal will ever arrive for non-attached sessions during this server lifetime.

**Symptom A — killed sessions resurrect on next `portal open`:**

The timed-out helpers run `handleHydrateTimeout`, which deliberately leaves `@portal-skeleton-<paneKey>` set. When the user kills one of those panes' sessions, the daemon's next `captureAndCommit` tick runs:
- Pre-companion-fix: `mergeSkippedPanes` (`internal/state/capture.go`) saw the marker in `skipSet` and re-injected the dead session from `prev` into the freshly-captured index → killed session reappears in `sessions.json` and on next bootstrap.
- Post-companion-fix (currently on `main`): the merge filter requires session/window/pane to exist live in `idx.Sessions` before merging from prev. With the session killed, the merge correctly drops it.

So Symptom A's user-visible behaviour is **already fixed on `main`** by the companion bug's merge filter. The marker still leaks (this bug owns the marker-production path), but the merge no longer turns the leak into a resurrection. **This must be empirically reconfirmed against HEAD before scope-shaping the fix.**

**Symptom B — on-resume hooks never fire:**

`handleHydrateTimeout` returns nil without invoking the hook-firing exec; control returns to `runHydrate` at line 109 which falls through to `execShellAndExit` (bare shell). The pane gets a shell with no hook command run.

By design (`built-in-session-resurrection` spec, "Helper Behavior on Startup", step 3e: "exec $SHELL (bare shell; no hook firing on this path)"), the timeout path bypasses hooks. The spec's reasoning was that timeout signals "something went wrong with the signal flow" and bare shell is safer than potentially mis-firing the hook.

But because the upstream selective-signaling trigger turns the timeout path into the **steady-state path for every non-attached session on every cold-start bootstrap**, the on-resume-hooks feature is effectively non-functional for any user with multiple saved sessions. The "we don't know if the pane is in a healthy state" rationale doesn't hold when timeout has become a deterministic outcome of normal cold-start behaviour, not an exception.

**Symptom C — scrollback save silently skipped for stuck-marker panes:**

`captureAndCommit` (`cmd/state_daemon.go:131-133`) skips capturing scrollback for any pane whose paneKey is in the marker set. So while a marker is stuck (pane is live but marker survives the timeout), the daemon never saves that pane's scrollback. Across the server lifetime, the saved scrollback for affected panes goes stale.

The companion-fix step 7 (`CleanStaleMarkers`) does NOT close this gap because its predicate is "marker without a live pane" — but timeout-stuck markers are ON live panes (a wrapper sh + bare shell is still a live tmux pane). The cleanup correctly leaves them alone, and the markers therefore survive to suppress scrollback save indefinitely.

This is the "secondary harm" the companion spec flagged but cannot fully resolve from outside the marker-production layer.

**Defect D — orphan `sh -c` wrapper after timeout:**

`buildHydrateCommand` produces `sh -c 'portal state hydrate ...; exec $SHELL'`. The wrapper sh forks `portal state hydrate`, which on the timeout path calls `defaultExecShell` (`cmd/state_hydrate.go:322-325`) → `syscall.Exec($SHELL, [$SHELL], env)`. The portal process image becomes the user's shell; the wrapper sh remains the parent, blocked in `wait()` for that PID's exit. When the user eventually exits the shell, the wrapper proceeds to `; exec $SHELL` and replaces itself with **another** shell. Two consequences:

1. **Orphan `sh` parent**, observed at ~20 hours uptime in the addendum. Every timed-out hydrate leaves an `sh` process parented to the tmux server until the pane closes. Same on the success path — the helper exec's $SHELL inside `execShellAndExit` / `execShellOrHookAndExit`, so the wrapper sh is also parked across normal hydration.
2. **Pane does not close on `exit`** — the wrapper's trailing `; exec $SHELL` re-spawns a fresh shell when the user types `exit`. The user has to type `exit` twice to close the pane, contradicting the comment in `buildHydrateCommand` ("exiting the shell ends the pane"). The trailing `exec $SHELL` was apparently intended as a defensive fallback for the case where the inner `portal hydrate` exits without exec'ing — a scenario that does not happen in practice because both exit paths always exec.

### Contributing Factors

- **No bootstrap-driven signaling.** The hydrate-trigger contract assumes tmux's `client-attached` / `client-session-changed` will eventually deliver a signal to every skeleton pane. That contract holds only for the session the user attaches to; it has no answer for non-attached sessions.
- **Spec design choice: timeout path is a bypass, not a recovery.** The original `built-in-session-resurrection` spec treats timeout as an exceptional condition where degrading to bare shell is preferable to firing hooks blind. Under selective-signaling, timeout is the steady state for non-attached sessions.
- **Marker production is fire-and-forget.** `setSkeletonMarker` is non-fatal-if-missing and is set unconditionally for every live pane in the skeleton phase. There is no inverse "if hydration never completes, who cleans this?" lifecycle invariant — the timeout handler explicitly opts out of cleanup, and the bootstrap-time step 7 cleanup only handles the dead-pane case.
- **`exec $SHELL` fallback in the wrapper is dead code on the success path and harmful on the timeout path.** It exists for crash-resilience but is unreachable in practice (both exit paths exec) AND breaks the natural pane-close-on-exit semantics.
- **Both `client-attached` and `client-session-changed` register signal-hydrate.** When user attaches to a session, both events can fire near-simultaneously (the original `client-attached` plus an internal session-change event). Each invocation does a full marker enumeration and FIFO write attempt. For panes whose helpers have already timed out (FIFO removed, marker still set), both invocations log `ENOENT` warnings at the same timestamp. This explains the duplicate-write observation in the inbox.

### Why It Wasn't Caught

- **Integration tests cover only the happy path.** The `built-in-session-resurrection` integration tests verify the signal-arrived flow end-to-end (skeleton + signal + dump + hook + shell). They don't model the case where multiple sessions are skeletoned and only one is attached.
- **Manual reproduction requires multiple saved sessions.** A user with one saved session always attaches to it on first `portal open`, so the timeout path is unreachable. The bug only surfaces when N≥2 saved sessions exist and the user attaches to one — the steady-state shape of any actual user.
- **Timeout handler's "next attach re-signals" comment encoded the wrong invariant as true.** The comment at line 262 of `state_hydrate.go` reads as a deliberate design choice and was not flagged in review. The actual mechanism (FIFO unlinked at line 256, helper exec'd shell, no reader) makes "next attach re-signals" a no-op that just re-fires `ENOENT`.
- **Symptom B's user-visible signal is silent.** The on-resume-hooks feature failing produces no error — just a bare shell where the user expected `claude --resume`. The reporter inferred the failure from the absence of expected behaviour, not from any user-visible error.
- **Symptom C is even more silent.** Stuck markers suppressing scrollback save produces no error, no warning, no diagnostic — the user only notices on the next reboot when scrollback is empty. By then the connection to "marker leaked from a previous timeout" is invisible.

### Blast Radius

**Directly affected:**
- `cmd/state_hydrate.go` — `handleHydrateTimeout` (deliberate bypass of marker-unset and hook-firing), `runHydrate` step 1 (timeout fall-through to `execShellAndExit`), `defaultExecShell` (`syscall.Exec` semantics that the wrapper depends on).
- `cmd/state_signal_hydrate.go` — `runSignalHydrate` (per-session enumeration; no global "signal everyone" path).
- `internal/restore/session.go` — `armPanes` (skeleton arm sequence; FIFO + respawn-pane + marker), `buildHydrateCommand` (wrapper construction with the harmful `; exec $SHELL` trailer).
- `cmd/bootstrap/stale_marker_cleanup.go` — `CleanStaleMarkers` only handles dead panes; does not address timeout-stuck markers on live panes.

**Potentially affected (downstream readers of stuck markers):**
- `cmd/state_daemon.go:131-133` — daemon's capture loop skips scrollback save for marked panes. While a marker is stuck, the affected pane's scrollback is silently not saved across the server lifetime.
- `internal/state/capture.go` — `mergeSkippedPanes`. With the companion fix in place this is defensively guarded; without the fix it was the path that turned stuck markers into resurrection.

**Not affected:**
- The single-saved-session user (no race ever happens; one session = one attach = one signal). All visible bugs vanish.
- Hot-path `portal open <existing-session>` after the cold-start bootstrap completes (Restore skips live sessions; no skeleton, no helpers, no race).

---

## Fix Direction

_To be discussed in findings review after presenting the analysis to the user._

---

## Notes

- **Empirical reconfirmation needed against HEAD.** The companion-bug fix is on `main` but the inbox was written before that fix landed. Specifically: re-verify whether killing one of the affected sessions on a current-HEAD build still reproduces Symptom A (resurrect on next `portal open`), or whether the merge filter has already neutralised the user-visible symptom even with the marker still leaking.
- **Cold-start scope.** Restore skips live sessions, so the skeleton + hydrate machinery only runs on the first `portal` invocation after the tmux server cold-starts. The bug's cardinality is therefore "once per server lifetime, affecting all-saved-sessions-minus-one" rather than "once per `portal open`".
- **Wrapper redesign and timeout-path redesign are the same code site.** Both observable defects (no hooks on timeout, orphan sh after timeout) live in the construction at `internal/restore/session.go:419` together with the consumption at `cmd/state_hydrate.go:248-266` (timeout) and `cmd/state_hydrate.go:191-194` (`execShellAndExit` final terminator). Treating these as one work product is likely cheaper than treating them as two.
- **Open question for fix direction:** does the fix push hydration into bootstrap (eagerly signal every skeleton pane right after step 5, before the user attaches), redesign the timeout path (fire hooks on timeout; unset marker on timeout; possibly drop the wrapper), or both? The findings review will sketch options and trade-offs before scope-shaping.
