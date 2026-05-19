# Specification: Saver Kill-Respawn Loop Leaks Daemons

## Specification

### Problem Statement

Every portal bootstrap that runs step 4 (`EnsureSaver`) fires an unnecessary kill-respawn cycle on the `_portal-saver` session, leaking orphan `portal state daemon` processes and leaving no live saver session after bootstrap completes. The user-visible consequences are:

- **~520ms wasted per portal invocation** on a kill-respawn block that should not run when the daemon is healthy and the binary version matches.
- **Accumulating orphan daemons** — observed three concurrent `portal state daemon` processes parented to the same tmux server (oldest 5 days old, each ~40MB RSS). Only the most recent holds `daemon.lock`; the rest are stranded.
- **Silently-paused saves between portal invocations** — bootstrap ends with `_portal-saver` destroyed, so no daemon runs to capture state until the next bootstrap recreates it. The resurrection guarantee is silently broken.
- **Noisy diagnostic log** — three WARN lines emitted on every bootstrap (`prior daemon … did not exit within 5s` → `another daemon holds the lock; exiting` → `step 4 (EnsureSaver) failed: … no such session: _portal-saver`).

**Reproducibility:** Always, on any environment where (a) `daemon.version` is missing OR (b) the daemon's per-tick wall time exceeds the 5s kill-barrier window. The latter applies to any user with non-trivial scrollback volume (~23 panes × ~1.2MB rendered text was sufficient on the affected machine).

**Scope of impact:** Performance regression and silent reliability regression — portal still works functionally, but the resurrection daemon's continuity guarantee is violated between invocations and startup latency is degraded.

### Root Cause

The bug is the conjunction of two independent defects in the saver-bootstrap and daemon-startup pair, plus a third open question whose user-visible effect is neutralised by fixing the first two.

#### Defect 1 — Version-mismatch false positive when `daemon.version` is absent

`portalSaverVersionMismatch` (`internal/tmux/portal_saver.go`) collapses three distinct conditions into a single "mismatch" result: (a) genuine version disagreement, (b) dev-build workflows (stored or current is `dev`/empty), and (c) "version file absent". Case (c) is the false positive — file absence does not imply version mismatch; it merely means we cannot confirm the version, while the daemon may still be perfectly healthy.

`EnsurePortalSaverVersion` (`internal/tmux/portal_saver.go:249`) consults the mismatch predicate without first checking daemon aliveness. So any condition that removes `daemon.version` while leaving the daemon alive triggers an unnecessary kill on every subsequent bootstrap.

#### Defect 2 — Daemon SIGHUP-unresponsive within the 5s kill-barrier window

`defaultDaemonRun` (`cmd/state_daemon.go`) runs `tick()` synchronously inside the ticker's `select` arm:

```go
for {
    select {
    case <-ticker.C:
        tick(deps)               // synchronous; no ctx awareness inside
    case <-ctx.Done():
        return daemonShutdownFunc(deps)
    }
}
```

`ctx.Done()` is structurally unreachable during a tick. The expensive work inside `tick → captureAndCommit` iterates every live pane and invokes `tmux capture-pane -e -p -S -` (unbounded scrollback) per pane for the hash check. On the affected user's profile (~23 panes × ~1.2MB rendered text), measured wall time exceeds the 5s `killBarrierTimeout` sized by the prior fix.

When the barrier gives up early, the new daemon spawns, immediately collides with the still-held `daemon.lock`, exits cleanly **without writing `daemon.pid` or `daemon.version`**, destroys the just-created `_portal-saver` pane process, and triggers the `SetSessionOption(_portal-saver, destroy-unattached, off)` "no such session" cascade.

#### Defect 3 — `daemon.version` disappearance (open, instrumentation only)

Code-trace exhaustively enumerated every production file-removal path; **no production code path removes `daemon.version` individually**. The disappearance therefore originates from outside portal's production code (manual `--purge`, dev-build escape, or external process). Fixing Defect 1 makes the disappearance non-load-bearing for the user-visible symptom — Defect 3 becomes a follow-up question, not a blocker.

#### Why It Wasn't Caught

- The prior fix (`multiple-state-daemons-running-concurrently`, 2026-05-11) sized `killBarrierTimeout` at 5s against a measured 3.9s cold sweep with margin. The user's profile grew past that bound within months; no telemetry exposed the relationship.
- `portalSaverVersionMismatch`'s existing unit test pins the false-positive "absent → mismatch" behaviour as correct, codifying the bug as contract.
- No alive-daemon-with-missing-version-file integration test exists. The closest test verifies kill-respawn under explicit version mismatch, not under absent version.
- The orphan-leak symptom is invisible without `ps | grep portal`; the WARN cascade only lands in `portal.log`.

### Change 1 — Alive-check first in `EnsurePortalSaverVersion`

**Target:** `internal/tmux/portal_saver.go` — `EnsurePortalSaverVersion`, `portalSaverVersionMismatch`.

**Required behaviour:**

Rework the kill decision in `EnsurePortalSaverVersion` to consult `BootstrapAliveCheck(stateDir)` **before** the version-mismatch branch. The new decision matrix:

| Daemon alive? | Version file state | Versions match? | Action |
|---|---|---|---|
| Yes | Absent | (unknowable) | **No kill.** Write daemon.version defensively from bootstrap, then proceed to BootstrapPortalSaver. |
| Yes | Present, reads cleanly | Match | **No kill.** Proceed to BootstrapPortalSaver. |
| Yes | Present, reads cleanly | Mismatch (real upgrade) | **Kill.** Run `killSaverAndWaitForDaemon`, then BootstrapPortalSaver. |
| Yes | Read error (non-absent I/O failure) | (unknowable) | **Kill.** Conservative — treat unknown I/O failure as needing recycle. |
| No | (any) | (any) | **No kill needed.** No daemon to recycle. Proceed to BootstrapPortalSaver. |

`portalSaverVersionMismatch` keeps its current external shape but is **no longer the lone gate**. The alive-check classifies the situation first; the mismatch predicate is consulted only on the alive-with-readable-version branch.

**Defensive complement:** when bootstrap observes "alive daemon + absent `daemon.version`" on the survived path, write `daemon.version` from the bootstrap side before proceeding. This closes the lock-loser lifecycle hole (lock-loser daemons return cleanly before writing `daemon.version`, leaving the file observably absent until the next bootstrap repairs it).

**What stays unchanged:**

- The `daemon.lock` flock primitive and `killSaverAndWaitForDaemon` machinery from the prior bugfix (`multiple-state-daemons-running-concurrently`).
- `BootstrapPortalSaver` itself — only the gate in front of `killSaverAndWaitForDaemon` changes.
- The no-daemon path (alive-check returns false) — already correct, no behavioural change.
- Dev-build handling (`stored == "dev"` or `currentVersion == "dev"`) — preserve current "always recycle on dev" behaviour for development workflows.

**Rejected alternative:** distinguishing `ErrVersionFileAbsent` inside `portalSaverVersionMismatch` only (smaller change). Rejected because it narrows the symptom (file absent → no kill) but misses the broader invariant: a healthy daemon should never be killed for a missing version marker regardless of *why* the file is missing. The alive-check ordering captures the broader invariant.

---

## Working Notes

