---
status: complete
created: 2026-05-19
cycle: 1
phase: Gap Analysis
topic: saver-kill-respawn-loop-leaks-daemons
---

# Review Tracking: saver-kill-respawn-loop-leaks-daemons - Gap Analysis

**Resolution summary:** All 9 findings auto-applied under auto-mode (carried from input-review phase). Concrete content was formulated for each gap and added to the specification. Highlights:
- F1: Dev-build rows added to Change 1 decision matrix; dev short-circuits the alive-check.
- F2: Documented that `stateDir` is already a parameter to `EnsurePortalSaverVersion`; no signature change.
- F3: Defensive write uses `currentVersion`; the daemon's actual binary version is not probed.
- F4: Three explicit `ctx.Done()` observation points listed (entry, post-enumeration, between iterations).
- F5: Breadcrumb logs under `state.ComponentDaemon` with stable `daemon.version write:` prefix as grep anchor.
- F6: Criterion #4 scoped to survived path; kill-respawn path explicitly allows brief absence.
- F7: Integration test #3 reframed as fault-injection with concrete observation mechanisms.
- F8: Change 2 signature claim made definitive — local to `cmd/state_daemon.go`, no propagation to `internal/state/capture.go`.
- F9: Existing predicate test is reframed (rename + redoc), not deleted; assertions preserved.

## Findings

### 1. Dev-build branch missing from Change 1 decision matrix

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Change 1 — decision matrix and "What stays unchanged" bullet on dev-build handling

**Details**:
The decision matrix (alive/version-file/match → action) does not include a row for dev-build cases, but the "What stays unchanged" section says: "Dev-build handling (`stored == "dev"` or `currentVersion == "dev"`) — preserve current 'always recycle on dev' behaviour for development workflows." This leaves an implementer needing to reconcile two rules:

- Matrix row "Yes / Present, reads cleanly / Match" says **No kill**.
- Dev rule says **always recycle on dev**.

If a user runs a dev binary against a saver started by another dev binary (both "dev"), does that satisfy "Match" (no kill) or "dev-preserved" (kill)? The unit test table pins `dev/0.5.0 → true` and `0.5.0/dev → true` (mismatch), but doesn't cover `dev/dev`. The interaction order between the alive-check gate, the new matrix, and the preserved dev rule is not explicit.

A planning agent will have to decide where the dev branch sits in the new control flow — that's a design call the spec should make.

**Proposed Addition**:

---

### 2. `stateDir` source for `BootstrapAliveCheck` in `EnsurePortalSaverVersion`

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Change 1 — "Required behaviour" paragraph referencing `BootstrapAliveCheck(stateDir)`

**Details**:
Change 1 says: "Rework the kill decision in `EnsurePortalSaverVersion` to consult `BootstrapAliveCheck(stateDir)` before the version-mismatch branch." The current `EnsurePortalSaverVersion` signature is not shown, and the spec does not specify whether `stateDir` is already available in scope, must be threaded as a new parameter, or resolved internally. This signature decision propagates to every caller of `EnsurePortalSaverVersion` — a planner needs to know whether the surface change is local to the function body or fans out across the bootstrap layer.

**Proposed Addition**:

---

### 3. Version string written by defensive `WriteVersionFile` on the survived path

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Change 1 — "Defensive complement" paragraph and Acceptance Criterion #4

**Details**:
The defensive write says: "write `daemon.version` from the bootstrap side before proceeding." Acceptance Criterion #4 says the file "contains the current binary version" post-bootstrap. The spec doesn't explicitly state which version string the bootstrap-side write uses — implicitly the current portal binary version, but on the "alive daemon + absent file" branch, the alive daemon may technically be running an older binary (if the user upgraded, started portal once with a now-stale daemon still surviving, etc.).

The "Match" semantic in the matrix assumes versions agree when both are readable. If the file is absent, we can't confirm they agree, but we're writing the current binary's version anyway. A planner needs the explicit rule: "write `currentVersion` from `cmd.version`" or equivalent, plus confirmation that no version-comparison happens against the running daemon on this branch.

**Proposed Addition**:

---

### 4. Cancellation observed before first per-pane iteration

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Change 2 — "Cancellation semantics" bullets

**Details**:
Change 2 states cancellation is observed "between per-pane iterations." The capture loop also performs pane enumeration (a `tmux list-panes -a` style call) and likely other setup before the per-pane loop starts. The spec doesn't say whether `ctx.Done()` is checked:

- Before pane enumeration begins
- After enumeration but before the first iteration
- Only between iterations

The unit-test table includes "Cancel context before first per-pane iteration → early return, no commits" which implies a check exists before iteration begins. But the prose only commits to "between iterations." Worst-case exit latency is described as "one pane's `capture-pane` wall time" — but if cancellation isn't honoured before enumeration, an unbounded `list-panes` call on a server with N sessions adds to that bound. A planner needs the explicit set of cancellation observation points.

**Proposed Addition**:

---

### 5. Logger component / level conventions for Change 3 breadcrumb

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Change 3 — "Required behaviour"

**Details**:
Change 3 specifies "a single DEBUG-level log entry inside `state.WriteVersionFile`" capturing version, caller pid, destination path. The `internal/state/logger.go` package (per CLAUDE.md) uses component tags (e.g., `ComponentBootstrap`, `ComponentDaemon`). The spec doesn't specify which component the breadcrumb logs under, nor the exact field naming/structure of the log line. For a single line of code this is minor, but the breadcrumb's whole purpose is grep-ability after a Defect 3 recurrence — log format needs to be searchable.

**Proposed Addition**:

---

### 6. Acceptance Criterion #4 scope — does it apply on the mismatch-and-kill path too?

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Acceptance Criteria #4 ("`daemon.version` is repaired defensively")

**Details**:
Criterion #4 says: "If a bootstrap encounters 'alive daemon + absent `daemon.version`', the file exists after bootstrap completes and contains the current binary version." This is unambiguous for the survived path. But the spec doesn't state what happens to `daemon.version` on the kill-respawn path (matrix row "Yes / Present / Mismatch") — implicitly the fresh daemon writes its own `daemon.version` after acquiring the lock, but Criterion #4 doesn't explicitly cover that path. If the post-kill respawn flow leaves `daemon.version` missing for a window (lock acquired, version write not yet performed), an integration test asserting "file present immediately after bootstrap" could flake. A planner needs to know whether Criterion #4's post-condition is universal or only applies to the survived-daemon branch.

**Proposed Addition**:

---

### 7. Integration test #3 — observation mechanism for cascade chain

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Testing Requirements → Integration tests → #3 "Lock-loser daemon's pane exit destroys _portal-saver"

**Details**:
The test asserts a four-step chain: "lock-loser exits → pane process terminates → `_portal-saver` session is destroyed → `SetSessionOption` fails with 'no such session'." The spec doesn't specify how the chain is observed:

- Polling `tmux has-session` with a timeout?
- Direct probe of `SetSessionOption` return value?
- Reading specific WARN lines from `portal.log`?

The test is described as confirming "the cascade is what we believe before the fix lands" — i.e., it asserts the pre-fix bug behaviour against the unfixed system OR captures the cascade once and pins it as a regression guard. Which framing? If the latter, the test must continue to pass post-fix, which means it has to exercise a path that is no longer reachable on a healthy bootstrap (and would need a fault-injection harness to trigger the lock-contention scenario synthetically). A planner needs the observation mechanism and the test's role in the post-fix world specified.

**Proposed Addition**:

---

### 8. Signature propagation for Change 2 — definite or conditional?

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Change 2 — "Target" line and "Risk & Rollout → Fix complexity"

**Details**:
Change 2's target says: "Signature updates **may propagate** into `internal/state/capture.go` if the per-pane callers live there." Risk & Rollout repeats: "signature updates that **may propagate** into `internal/state/capture.go` per-pane callers." This is investigation-language ("may") in what should be an implementation-ready specification. A planner cannot break this into tasks without knowing whether the change crosses package boundaries. The investigation phase should have established the actual call graph; the spec should state definitively: "ctx threads through `captureAndCommit` and into `internal/state/capture.go`'s `CapturePane`/`<exact name>`" or "stays local to `cmd/state_daemon.go`."

**Proposed Addition**:

---

### 9. "Replace" vs "augment" semantics for existing unit test

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Testing Requirements → Unit tests → `portalSaverVersionMismatch` table tests

**Details**:
The spec says the existing test "pinning false-positive behaviour must be **replaced** (it codifies the bug as contract)." The new table includes the case `("" / 0.5.0 / ErrVersionFileAbsent) → true (predicate alone — alive-check happens in caller)`. This means the predicate's behaviour for the absent case is **unchanged at the predicate layer** — only the caller's interpretation changes. So the existing test asserting absent → mismatch is technically still correct as a predicate-level test; what's wrong is the implicit promise that the predicate's verdict drives the kill decision alone.

If the predicate's return value is unchanged for the absent case, what exactly is being "replaced"? Possibly the test's name/description (which currently implies absent-implies-kill at the caller level) needs revising, but the assertion stays. A planner reading "replace" may delete the test outright, losing predicate-level coverage. The intent — rename/reframe the test, or genuinely change its assertions — should be explicit.

**Proposed Addition**:

---
