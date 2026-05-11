---
status: complete
created: 2026-05-11
cycle: 1
phase: Gap Analysis
topic: multiple-state-daemons-running-concurrently
---

# Review Tracking: multiple-state-daemons-running-concurrently - Gap Analysis

## Findings

### 1. Lock fd lifetime — variable retention not specified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Critical
**Affects**: Fix Part 1 — Behaviour

**Details**:
The spec states "Lock is held for the lifetime of the process and released by the kernel on exit." For this guarantee to hold, the fd must be retained in a long-lived variable — if the helper returns and the fd goes out of scope without `runtime.KeepAlive` or similar, Go's runtime is free to finalize/close it, which would release the lock while the daemon is still running.

**Proposed Addition**:
New bullet in Fix Part 1 Behaviour:
> **Fd retention is load-bearing.** The lock fd MUST be held in a variable that lives for the lifetime of the daemon process — for example, a package-level `var` in the daemon command, or a field on a struct kept alive by the run loop. The fd MUST NOT be allowed to go out of scope while the daemon is running; Go's finalizer mechanism could otherwise close the fd, releasing the lock while the daemon is still active and silently re-introducing the very race the lock is meant to close. If a future refactor wraps the fd in a value with a finalizer, the finalizer must not close the fd.

**Resolution**: Approved
**Notes**: Added as fourth bullet under Fix Part 1 → Behaviour.

---

### 2. Lock file creation — directory existence and open-error handling unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Important
**Affects**: Fix Part 1 — Behaviour

**Details**:
The spec covers `flock` success and EWOULDBLOCK but not `open(2)` failure modes (EACCES, ENOSPC, ENOENT, EMFILE, ENFILE), nor the file mode, nor whether the helper creates `<stateDir>` itself.

**Proposed Addition**:
New "Lock-file create / open semantics" sub-section after the CLOEXEC bullet:
> - The lock file is opened with mode `0600` (matching the file mode of other portal state files).
> - The lock helper does **not** create `<stateDir>` itself. State-directory existence is a pre-existing responsibility of the caller.
> - `open(2)` failures **other than** `EWOULDBLOCK` (which comes from `flock`, not `open`) — e.g. `EACCES`, `ENOSPC`, `ENOENT`, `EMFILE`, `ENFILE` — are treated as fatal: the daemon logs an ERROR-level line describing the failure and exits **non-zero**. Distinct from the contention path (lock held → WARN + exit 0).

**Resolution**: Approved
**Notes**: Added as new "Lock-file create / open semantics" sub-section.

---

### 3. Pidfile cleanup on graceful daemon shutdown — unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Important
**Affects**: Fix Part 1 — Pidfile compatibility; Acceptance Criteria — Pidfile remains coherent

**Details**:
"Always reflects the single daemon that won the lock" is strictly only true while the daemon runs. Need to either weaken the claim or specify explicit cleanup on graceful exit.

**Proposed Addition**:
Weaken acceptance criterion to "while that daemon is running" and note that `BootstrapAliveCheck` already tolerates stale-after-exit content via signal-0 probe. No pidfile cleanup required on graceful exit.

**Resolution**: Approved
**Notes**: Weakened "always reflects" claim; clarified no cleanup required on exit.

---

### 4. Loser-daemon tmux session lifecycle — what tmux does with the empty session

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Important
**Affects**: Risk and Rollout — Upgrade behaviour; Fix Part 1 — Contention path

**Details**:
When daemon exits status 0, default tmux behaviour closes the window/session. Recovery semantics differ between "session closed" (HasSession returns false) and "session with dead pane" (stale-pidfile branch). Spec needs explicit treatment.

**Proposed Addition**:
New "Loser-daemon session aftermath" sub-section in Fix Part 1:
> When the loser exits status 0 as the initial process of `_portal-saver`, default tmux behaviour (no `remain-on-exit`) closes the window — and since the session has only that one window, the **session itself closes**. The next bootstrap therefore typically observes `HasSession(_portal-saver) == false` and falls through to `createPortalSaverWithRetry` (no kill barrier invoked — there is no prior session to kill). If `remain-on-exit` is in effect for any reason, the next bootstrap observes the session with a dead pane, hits the stale-pidfile recovery branch, runs the barrier (which returns immediately because the prior PID is already dead), and recreates. Both convergence paths are recoverable.

**Resolution**: Approved
**Notes**: Added new sub-section covering both default and remain-on-exit paths.

---

### 5. Concurrent bootstrap invocations — barrier has no mutual exclusion

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Important
**Affects**: Fix Part 2 — Behaviour; Acceptance Criteria — Clean handover

**Details**:
Two `portal` commands started in close succession both enter their own `EnsurePortalSaverVersion`, both call KillSession, both poll, both start a new daemon. The lock catches it but produces a WARN. The acceptance criterion "No WARN on common case" needs clarification on whether concurrent-bootstrap is "common case."

**Proposed Addition**:
Clarify acceptance criterion: "common case" means a single bootstrap invocation. Concurrent invocations may produce one WARN line on the loser — accepted behaviour, Part 1 guarantees safety.

**Resolution**: Approved
**Notes**: Re-worded "Clean handover on the common case" acceptance criterion to distinguish single-invocation from concurrent paths.

---

### 6. Integration test — forcing version mismatch is not specified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Minor
**Affects**: Test Strategy — Integration test

**Details**:
Spec says "forced version mismatch" but does not specify the mechanism. Three options have different implications; spec should pick.

**Proposed Addition**:
Specify the mechanism: directly write a different value into `<stateDir>/daemon.version` between calls. No new test seam needed; exercises real `portalSaverVersionMismatch` comparison logic.

**Resolution**: Approved
**Notes**: Integration test case updated to specify the file-write mechanism.

---

### 7. `@portal-restoring` window vs 5 s barrier — interaction unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Minor
**Affects**: Fix Part 2 — Critical-path latency budget; Risk and Rollout

**Details**:
Bootstrap step 3 sets `@portal-restoring`; step 7 clears it. Barrier can extend that window by up to 5 s. Not addressed in spec.

**Proposed Addition**:
New "Interaction with `@portal-restoring` marker" sub-section in Fix Part 2:
> Bootstrap step 3 sets `@portal-restoring`; step 4 (EnsureSaver) is where the kill barrier fires; step 7 clears `@portal-restoring`. On the barrier-timeout path, the marker remains set for up to 5 s longer than in current behaviour. This is **explicitly acceptable**: the marker is designed to bracket the whole bootstrap window, daemon `captureAndCommit` suppression is the intended behaviour while it is set, and a 5 s extension does not affect the correctness of downstream bootstrap steps.

**Resolution**: Approved
**Notes**: Added explicit-acceptability sub-section.

---

### 8. Upgrade behaviour step 3 — claim about orphans being SIGHUP'd is misleading

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Important
**Affects**: Risk and Rollout — Upgrade behaviour

**Details**:
Spec says orphans "will be SIGHUP'd when the prior `_portal-saver` session is killed." Incorrect — orphans are children of the tmux server but no longer attached to any current session. Killing the current `_portal-saver` does not SIGHUP them; they drain naturally as their already-cancelled tick loops finish.

**Proposed Addition**:
Correct steps 3–5 of Upgrade behaviour to reflect the accurate mechanism.

**Resolution**: Approved
**Notes**: Steps 2–5 reworded for accuracy. Convergence story preserved.

---

### 9. WARN log content — load-bearing vs not contradiction

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Important
**Affects**: Fix Part 1; Acceptance Criteria — Observability

**Details**:
Fix Part 1 says content not load-bearing; Acceptance Criteria quotes specific text; Test Strategy doesn't specify whether tests assert content. Tension needs resolution.

**Proposed Addition**:
Explicit clarification in Acceptance → Observability: log content is illustrative, not load-bearing. Tests assert presence and WARN level only.

**Resolution**: Approved
**Notes**: Observability AC reworked to make this explicit.

---

### 10. Lock-acquire ordering assertion — mechanism not specified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Minor
**Affects**: Test Strategy — Unit tests

**Details**:
Test would either need a new `WritePIDFile` seam (added API surface) or observe filesystem state. Spec should pick.

**Proposed Addition**:
Specify: assert via observable filesystem state (pidfile unchanged after failed acquire). Do not add a WritePIDFile seam.

**Resolution**: Approved
**Notes**: Acquire-ordering test case extended with assertion mechanism.

---

### 11. Barrier behaviour on malformed PID file content

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Minor
**Affects**: Fix Part 2 — Behaviour

**Details**:
Behaviour step 6 lists "missing, unreadable, or empty" — but not "malformed (non-numeric)". Test Strategy lists "unreadable/corrupted". Aligning the two removes ambiguity.

**Proposed Addition**:
Extend step 6 to include "malformed — e.g. non-numeric content from a partial write."

**Resolution**: Approved
**Notes**: Step 6 of Behaviour updated.

---

### 12. Lock file path — single-server vs multi-server assumption

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Priority**: Important
**Affects**: Problem Statement — Expected behaviour; Acceptance Criteria — Singleton invariant

**Details**:
Problem Statement says "one daemon per tmux server lifetime"; Acceptance says "per state directory." These differ — `stateDir` is per-user, not per-tmux-server. A user running two tmux servers would share one daemon. Spec needs to reconcile and explicitly accept (or reject) the multi-server scenario.

**Proposed Addition**:
Reconcile by amending Problem Statement: invariant is per state directory (per-user in practice); multi-tmux-server-per-user is an unusual configuration; isolating daemons per tmux server is not in scope.

**Resolution**: Approved
**Notes**: Problem Statement "Expected behaviour" paragraph rewritten to be explicit about per-stateDir scope.

---
