# Investigation: Saver Kill-Respawn Loop Leaks Daemons

## Symptoms

### Problem Description

**Expected behavior:**
Portal bootstrap should be fast (<1s steady-state when tmux server already running) and leave a single live `_portal-saver` session with one healthy `portal state daemon` process after each invocation. The daemon should respond to SIGHUP within the kill-barrier window when a legitimate version-upgrade kill occurs, and `daemon.version` should persist across runs so version comparison skips the kill path when binaries match.

**Actual behavior:**
Every bootstrap invocation that reaches step 4 (EnsureSaver) fires an unnecessary kill-respawn cycle on `_portal-saver`, adding ~520ms per startup and leaking an orphan `portal state daemon` process. User-visible portal startup hovers at 3–5s. By the end of bootstrap, no live saver session remains, so saves are silently paused between portal invocations and the cycle repeats on the next run.

### Manifestation

The chain of three WARN lines appears in `~/.config/portal/state/portal.log` on every bootstrap:

```
WARN | bootstrap | prior daemon (pid=X) did not exit within 5s
WARN | daemon    | another daemon holds the lock; exiting
WARN | bootstrap | step 4 (EnsureSaver) failed: bootstrap _portal-saver:
                  set destroy-unattached: failed to set session option
                  destroy-unattached on _portal-saver: exit status 1:
                  no such session: _portal-saver
```

Process leak: `ps -o pid,ppid,user,stat,start,command` on the affected machine showed three `portal state daemon` processes alive simultaneously (PIDs 34503 from Saturday, 59610, 57161 from today), all parented to the same tmux server PID 94966. `lsof daemon.lock` confirmed only the most-recent daemon holds the lock; the older two are stranded.

Performance: a `tmux` PATH shim traced 26 tmux subprocess calls in a single `portal hooks list` invocation totalling ~3.2s wall time. Of that, the saver kill+respawn block (`has-session ×2 + new-session + set-option`) accounted for ~520ms.

### Reproduction Steps

1. Have an existing tmux server running with the `_portal-saver` session present and `daemon.version` missing (or any stored version mismatch with the invoking binary).
2. Run any portal command that triggers full bootstrap (e.g. `portal hooks list`, `portal open`, `portal x`).
3. Observe `portal.log` gain the three WARN lines, observe a new orphan `portal state daemon` process via `ps`, observe `_portal-saver` session absent immediately after bootstrap completes.

**Reproducibility:** Always, on this machine — confirmed across multiple runs (~2.0–3.2s wall time, ~520ms attributable to the kill-respawn block).

### Environment

- **Affected environments:** User's local Mac (Darwin 25.3.0, arm64). Brew-installed portal 0.5.0 from `leeovery/tools` tap.
- **Binary:** `/opt/homebrew/Cellar/portal/0.5.0/bin/portal`, `portal version` reports `0.5.0`.
- **State dir:** `~/.config/portal/state/` (XDG path).
- **tmux:** server PID 94966, ~5 days uptime, ~23 panes across ~21 sessions.
- **User conditions:** mid-active development session with many long-lived tmux sessions and ~27MB of accumulated per-pane scrollback (which is relevant to symptom #2 — long capture-loop iterations).

### Impact

- **Severity:** Medium. Functionally portal still works, but:
  - Every startup is 2–3s slower than it should be.
  - Saves are silently paused between portal invocations, defeating the purpose of the resurrection-daemon.
  - Orphan daemon processes accumulate over days; an old one from 5 days ago is still consuming memory and holding stale state.
- **Scope:** Any user whose `daemon.version` is missing or whose daemon's capture loop iteration exceeds the 5s kill-barrier window — likely all users with non-trivial scrollback volume.
- **Business impact:** Trust in portal's reliability — the resurrection guarantee is silently broken.

### References

- Inbox capture: `.workflows/.inbox/.archived/bugs/2026-05-18--saver-kill-respawn-loop-leaks-daemons.md`
- Relevant source: `internal/tmux/portal_saver.go`, `cmd/state_daemon.go`, `internal/state/daemon_state.go`, `internal/state/commit.go`

---

## Analysis

### Initial Hypotheses

Three suspected causes from the inbox-stage investigation, all in the `_portal-saver` lifecycle, treated as candidates to verify:

1. **Version-mismatch false positive.** `portalSaverVersionMismatch` (`internal/tmux/portal_saver.go`) returns `true` on any non-nil `readErr` from `daemon.version`, including the "file absent" case. If `daemon.version` is missing for any reason, every bootstrap fires the kill barrier in `EnsurePortalSaverVersion` even when the binaries match.
2. **SIGHUP-unresponsive capture loop.** `cmd/state_daemon.go:306` registers SIGHUP/SIGTERM via `signal.Notify`, but the capture loop appears to block on per-pane work without polling the signal channel mid-iteration. `killSaverAndWaitForDaemon`'s 5s deadline elapses while the daemon is still mid-capture, leaving an orphan.
3. **Lock-contention cascade.** The newly-spawned daemon can't acquire `daemon.lock` (orphan still holds it), exits, its pane process exit destroys the just-created `_portal-saver` session, and the immediately-following `SetSessionOption(_portal-saver, destroy-unattached, off)` fails with "no such session" — the visible `step 4 (EnsureSaver) failed` warning.

**Open sub-question to investigate alongside #1**: why does `daemon.version` keep disappearing? Was present as `0.5.0` at session start, gone by end. Whole state dir got wiped during the investigation. **User confirmed (2026-05-18)**: the disappearance was unprompted — no `portal clean`, no manual `rm`, nothing user-initiated touched the state dir. The deleter is therefore somewhere in portal's own runtime path. Candidates to investigate: an atomic-write race in `state.WriteVersionFile`, an over-eager cleanup pass in the daemon's tick loop, the bootstrap's CleanStale step (#10), or shutdown-flush behaviour in `defaultShutdownFlush`.

### Prior Work — Cross-Reference

The 2026-05-11 work unit **`multiple-state-daemons-running-concurrently`** (completed) is the direct predecessor in the same code surface. Its spec documented:

- **Defect 1 — No singleton enforcement.** `WritePIDFile` was an informational pidfile, not a lock. **Fixed** by introducing `daemon.lock` (flock-based, exposed via `state.AcquireDaemonLock`).
- **Defect 2 — Bootstrap doesn't synchronise with killed daemon's exit.** No barrier between `KillSession(_portal-saver)` and the immediately-following `BootstrapPortalSaver` create. **Fixed** by introducing `killSaverAndWaitForDaemon` with a 5s timeout, sized to "sit above the daemon's cold-sweep ceiling (3.9s on the affected user's scrollback profile) with margin."
- **Why daemons survive kill signal.** `defaultDaemonRun` is `for { select { ticker.C / ctx.Done() } }` — `tick()` runs synchronously inside the select arm, so `ctx.Done()` is only reachable **between ticks, never during one**. SIGHUP `cancel()` is observed only after the in-flight `tick()` completes.
- **Sweep cost drives the orphan-eligibility window.** `captureAndCommit` runs sequentially: marker list → `CaptureStructure` → per-pane `CaptureAndHashPane` (unconditional `capture-pane -e -p -S -` for the hash) → `WriteScrollbackIfChanged` → `state.Commit`. Dedup avoids file write only, not the expensive tmux call.
- **Recycle-induced sweep pressure.** Kill-respawn itself emits `session-closed` and `session-created` hooks, both of which fire `save.requested` and force the surviving daemon's sweep into a back-to-back regime — widening the cancel-to-exit window precisely on the recycle path the barrier was meant to defend.

**Implication for the current bug**: the prior fix shipped the `daemon.lock` and `killSaverAndWaitForDaemon` machinery, but the 5s timeout is now empirically inadequate for this user's scrollback profile (~27MB across 23 panes; earlier evidence shows >5s observed in the wild). On top of that, the current bug compounds with a **second issue not addressed by the prior fix**: `portalSaverVersionMismatch`'s false-positive treatment of "absent version file" as "version mismatch", which triggers the kill barrier on every bootstrap even when no version upgrade has occurred.

### Code Trace

*To be populated during Step 5 (Code Analysis).*

### Root Cause

*To be populated during Step 6 (Root Cause Synthesis).*

### Contributing Factors

*To be populated.*

### Why It Wasn't Caught

*To be populated.*

### Blast Radius

*To be populated.*

---

## Fix Direction

*To be populated during Step 8 (Findings Review & Fix Discussion).*

---

## Notes

- Likely single fix scope: `internal/tmux/portal_saver.go` (mismatch classification) and `cmd/state_daemon.go` (signal-responsive capture loop). Fixes are complementary and should ship together.
- Pre-existing related closed bugfixes worth cross-referencing: `multiple-state-daemons-running-concurrently`, `daemon-merge-reintroduces-dead-sessions`, `killed-sessions-resurrect-on-restart`. The new orphan-leak symptom may overlap with the first of these — verify whether it's a regression or a distinct root cause.
