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

**Bootstrap step 4 entry — `EnsurePortalSaverVersion` (`internal/tmux/portal_saver.go:249-259`):**

```go
stored, readErr := state.ReadVersionFile(stateDir)
if portalSaverVersionMismatch(stored, currentVersion, readErr) && c.HasSession(PortalSaverName) {
    _ = killSaverAndWaitForDaemonFn(c, stateDir)
}
return BootstrapPortalSaver(c, stateDir)
```

The mismatch predicate (`portalSaverVersionMismatch`, lines 265-276) returns `true` on **any** `readErr != nil`, including `ErrVersionFileAbsent`:

```go
func portalSaverVersionMismatch(stored, currentVersion string, readErr error) bool {
    if readErr != nil {                                     // ← absent file counts as mismatch
        return true
    }
    if currentVersion == "" || currentVersion == "dev" { return true }
    if stored == "" || stored == "dev" { return true }
    return stored != currentVersion
}
```

The function comment (line 232-241) makes this intentional: *"Read error from state.ReadVersionFile (including ErrVersionFileAbsent — first-ever bootstrap or user-initiated state-dir cleanup)"*. The design assumed daemon.version is reliably present when a healthy daemon runs, so its absence implies the daemon is gone and a recycle is safe. Three properties break that assumption.

**Kill barrier — `killSaverAndWaitForDaemon` (`internal/tmux/portal_saver.go:150-186`):**

```go
priorPID, readErr := killBarrierReadPID(stateDir)
if readErr != nil { _ = c.KillSession(PortalSaverName); return nil }
if !killBarrierIsAlive(priorPID) { _ = c.KillSession(PortalSaverName); return nil }
_ = c.KillSession(PortalSaverName)
// poll IsAlive every 50ms, give up after killBarrierTimeout (5s)
for range ticker.C {
    if !killBarrierIsAlive(priorPID) { return nil }
    if !time.Now().Before(deadline) {
        killBarrierLogger.Warn(state.ComponentBootstrap,
            "prior daemon (pid=%d) did not exit within %v", priorPID, killBarrierTimeout)
        return nil
    }
}
```

The barrier kills `_portal-saver` (which delivers SIGHUP to the pane process), then polls. If the daemon doesn't exit in 5s, it proceeds anyway and the orphan continues running.

**Daemon SIGHUP handler — `cmd/state_daemon.go:303-307`:**

```go
sigCh := make(chan os.Signal, 1)
signal.Notify(sigCh, syscall.SIGHUP, syscall.SIGTERM)
go func() { <-sigCh; cancel() }()
```

SIGHUP cancels the daemon's context. But the run loop (line 70-81) is:

```go
for {
    select {
    case <-ticker.C:
        tick(deps)               // ← runs synchronously; no ctx awareness inside
    case <-ctx.Done():
        return daemonShutdownFunc(deps)
    }
}
```

`tick()` runs synchronously inside the `ticker.C` arm. `ctx.Done()` is only reachable **between** ticks, never during one. The expensive work inside `tick → captureAndCommit` (line 132-159) iterates every live pane and calls `state.CaptureAndHashPane` (which invokes `tmux capture-pane -e -p -S -` — unbounded scrollback) for each. On this user's profile (23 panes × ~1.2MB average rendered text), measured wall time exceeds 5s for the cold sweep — wider than `killBarrierTimeout`.

**Lock-contention cascade — `cmd/state_daemon.go:247-271`:**

```go
lockFile, err := acquireDaemonLock(dir)
if err != nil {
    if errors.Is(err, state.ErrDaemonLockHeld) {
        logger.Warn(state.ComponentDaemon, "another daemon holds the lock; exiting")
        return nil                              // ← early exit; pidfile/version NOT written
    }
    // ... non-EWOULDBLOCK fatal
}
daemonLockFile = lockFile
state.WritePIDFile(dir, os.Getpid())
state.WriteVersionFile(dir, version)
```

A newly-spawned daemon that loses the lock race returns `nil` (clean exit) **before writing daemon.pid or daemon.version**. Its `RunE` returns, the cobra command exits, the pane process terminates. Tmux's `_portal-saver` session is destroyed because its initial pane process has exited (`destroy-unattached=off` doesn't save a session whose only pane has exited normally — that's a different lifecycle axis).

**The immediately-following `SetSessionOption`** (`internal/tmux/portal_saver.go:221`):

```go
if err := c.SetSessionOption(PortalSaverName, "destroy-unattached", "off"); err != nil {
    return fmt.Errorf("bootstrap _portal-saver: set destroy-unattached: %w", err)
}
```

Runs against `_portal-saver`, which has just been destroyed by the lock-loser's pane exit. tmux returns `exit status 1: no such session: _portal-saver`. Surfaced as the visible `step 4 (EnsureSaver) failed` warning.

### Root Cause

The bug is the **conjunction of two defects** in the saver-bootstrap and daemon-startup pair, plus a third open question whose mechanism is unknown but whose user-visible effect is neutralised by fixing the other two.

**Defect 1 — Version-mismatch false positive when `daemon.version` is absent.**

`portalSaverVersionMismatch` collapses three distinct conditions into a single "mismatch" result: (a) genuine version disagreement (release upgrade), (b) dev-build workflows (current or stored is `dev`/empty), and (c) "file absent". Case (c) is the false positive: file absence does not imply version mismatch; it merely means "we cannot confirm the version, but the daemon may still be perfectly healthy". `EnsurePortalSaverVersion` makes no alive-check before the kill decision — so any condition that nukes `daemon.version` while leaving the daemon alive triggers an unnecessary kill on every subsequent bootstrap.

**Defect 2 — Daemon SIGHUP unresponsive within the 5s kill-barrier window for users with non-trivial scrollback.**

The synchronous `tick()` call inside the ticker's `select` arm means `ctx.Done()` is structurally unreachable during a tick. The prior-bug fix (`multiple-state-daemons-running-concurrently`, 2026-05-11) sized `killBarrierTimeout` at 5s based on "3.9s cold sweep + margin"; the user's profile has since grown past that bound. The kill barrier's polling loop is correct, but the **daemon side of the contract** — "exit promptly on SIGHUP" — is violated for any user whose per-tick wall time exceeds the timeout.

When the barrier gives up early, the new daemon spawns, immediately collides with the still-held lock, exits cleanly, destroys the just-created `_portal-saver` pane process, and triggers the `SetSessionOption` "no such session" cascade.

**Defect 3 (open sub-question) — Why does `daemon.version` keep disappearing?**

Code analysis enumerated every production file-removal path that could touch state files:

| Path | Removes | Production reachability |
|---|---|---|
| `cmd/state_cleanup.go:155` `os.RemoveAll(dir)` | entire state dir | Only via explicit `portal state cleanup --purge` — user-confirmed not invoked |
| `cmd/state_daemon.go:117, 241` | `save.requested` only | Daemon-internal; doesn't touch daemon.version |
| `cmd/state_hydrate.go:128, 268` | per-pane FIFO files | hydrate helper; doesn't touch daemon.version |
| `internal/state/logger.go:182` | `portal.log.old` only | log rotation path |
| `internal/state/commit.go:128` | scrollback bin files | dedup cleanup; scrollback-only |
| `internal/state/fifo_sweep.go:47` | per-pane FIFO files | bootstrap sweep; doesn't touch daemon.version |

**No production code path removes `daemon.version` individually.** The disappearance therefore originates either (a) from an external process the user is not aware of, (b) from a dev-build / test path that escaped its sandbox, or (c) from a `--purge` invocation that was forgotten. The investigation cannot pin this without reproducing the disappearance in instrumented conditions.

**Critically, Defect 3 does not need to be fixed for the user-visible symptom to disappear.** Fixing Defect 1 (treating `ErrVersionFileAbsent` + healthy alive-check as "no kill needed") makes the kill decision resilient to daemon.version's transient absence regardless of its cause. The defect can be relegated to a documentation note and a follow-up investigation if it recurs.

### Contributing Factors

- **`captureAndCommit`'s per-pane cost grows with rendered scrollback size.** `state.CaptureAndHashPane` invokes `tmux capture-pane -e -p -S -` unconditionally for the hash check (dedup avoids the file write only, not the capture call). At 23 panes × ~1.2MB rendered text per pane, the cold sweep wall time exceeds the prior fix's 5s window.
- **`tick()` is structurally non-interruptible.** Even between per-pane iterations there is no `ctx.Done()` poll — the only cancellation observation point is the outer `select` arm.
- **`EnsurePortalSaverVersion` does not consult `BootstrapAliveCheck` before the version-mismatch decision.** A healthy daemon's mere existence is irrelevant to the version-mismatch branch — it asks "are the version strings equal?" without first asking "is there even a daemon to recycle?"
- **Lock-loser daemons don't write `daemon.version` before exiting.** Combined with whatever's nuking the file in Defect 3, this widens the window during which the file is absent on disk.
- **The version-mismatch comment encodes the wrong invariant as intentional.** Line 236-237 of portal_saver.go explicitly says ErrVersionFileAbsent counts as mismatch, "for first-ever bootstrap or user-initiated state-dir cleanup." The comment is a design choice that ages badly once the file proves not to be reliably present.

### Why It Wasn't Caught

- **The prior fix (`multiple-state-daemons-running-concurrently`, 2026-05-11) shipped with a 5s `killBarrierTimeout` sized to the test author's measured worst case (3.9s).** No knob was provided for users with larger scrollback profiles, and the timing isn't measured/exposed in any way that would surface "your sweep is getting close to the bound."
- **`portalSaverVersionMismatch`'s test surface (`internal/tmux/portal_saver_test.go`) asserts the false-positive behaviour as correct.** The unit test for "absent version file" pins the current return-true behaviour — codifying the bug as the contract.
- **No alive-daemon-with-missing-version-file integration test exists.** The healthy-but-missing-marker case isn't modelled. The closest integration test (`portal_saver_integration_test.go`) verifies kill-respawn under explicit version mismatch, not under absent version.
- **The orphan-leak symptom is invisible until you `ps | grep portal`.** The kill-respawn churn is silent in the user's terminal — only portal.log captures the WARN cascade, and the user hadn't been opening portal.log.
- **The scrollback-size-vs-tick-time relationship was characterised in the prior spec but not turned into a regression guard.** A regression test like "fixture pane with N MB of scrollback, daemon must exit within X seconds of SIGHUP" doesn't exist.
- **Defect 3's invisibility is the deepest gap.** If `daemon.version` is silently disappearing for some reason outside portal's code paths, no portal test or check would catch it. The Defect 1 fix makes this irrelevant — but the underlying question "what's deleting it?" deserves a follow-up.

### Blast Radius

**Directly affected:**

- `internal/tmux/portal_saver.go` — `EnsurePortalSaverVersion`, `portalSaverVersionMismatch`. Decision logic for the kill branch.
- `cmd/state_daemon.go` — `defaultDaemonRun`, `tick`, `captureAndCommit`. SIGHUP responsiveness depends on making the per-pane loop ctx-aware.
- `internal/state/capture.go` — `CaptureStructure` and the per-pane callers. If we add per-pane ctx checks, they live here.
- `portal.log` — three WARN lines per bootstrap pollute the diagnostic record, making it harder to spot real warnings.

**Potentially affected:**

- **Save reliability between portal invocations.** Every cycle leaves no live `_portal-saver` session, so no daemon is running to capture state until the next bootstrap. Saves are silently paused.
- **Memory consumption.** Orphan daemons accumulate over uptime; on the affected machine three are alive (the oldest from 5 days ago), each holding ~40MB resident.
- **Startup latency.** ~520ms per bootstrap is consistently wasted on the kill-respawn block; for users who run `portal` frequently this aggregates.

**Not affected:**

- The structural-index merge logic — orthogonal layer, already addressed by the 2026-05-09 daemon-merge fix.
- Per-pane scrollback dump correctness — separate code path; affected only by "are saves running at all" not by "how saves are computed."
- The hydrate cascade — different timing, different markers, different files. Independent.

---

## Fix Direction

*To be populated during Step 8 (Findings Review & Fix Discussion).*

---

## Notes

- Likely single fix scope: `internal/tmux/portal_saver.go` (mismatch classification) and `cmd/state_daemon.go` (signal-responsive capture loop). Fixes are complementary and should ship together.
- Pre-existing related closed bugfixes worth cross-referencing: `multiple-state-daemons-running-concurrently`, `daemon-merge-reintroduces-dead-sessions`, `killed-sessions-resurrect-on-restart`. The new orphan-leak symptom may overlap with the first of these — verify whether it's a regression or a distinct root cause.
