---
status: in-progress
created: 2026-05-08
cycle: 2
phase: Gap Analysis
topic: daemon-merge-reintroduces-dead-sessions
---

# Review Tracking: daemon-merge-reintroduces-dead-sessions - Gap Analysis

## Findings

### 1. `mergePane` / `findOrAppendSession` editing scope is implicit

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Component A / Data Flow / Signature Approach, Files Touched

**Details**:
Files Touched lists three functions in `internal/state/capture.go` — `mergeSkippedPanes`, `mergePane`, `findOrAppendSession`. The Fix Component A discussion describes the filter as a pre-iteration guard built locally inside `mergeSkippedPanes` from `idx.Sessions`, leaving the helpers' public surface unchanged. It is therefore unclear whether `mergePane` and `findOrAppendSession` need editing at all, or whether they appear in Files Touched only because they may be incidentally touched / read during the change. A planner needs to know whether the filter is purely a guard at the merge entry point (helpers untouched) or whether the helpers themselves should grow defensive checks (e.g. `findOrAppendSession` rejecting unknown sessions). Picking one prevents an implementer from layering belt-and-braces guards in helpers that the design assumes are unchanged.

**Proposed Addition**:
Clarify in Fix Component A's "Data Flow / Signature Approach" that the filter lives entirely inside `mergeSkippedPanes` (pre-iteration) and that `mergePane` / `findOrAppendSession` are unchanged in behaviour — they appear in Files Touched only because the merge logic in the same file is being edited. Or, conversely, if the design intent is for helpers to gain their own defensive checks, state the contract for each.

**Resolution**: Pending
**Notes**:

---

### 2. Test file location for stale-marker cleanup unit tests is unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Testing Requirements / Stale-marker cleanup — unit, Files Touched

**Details**:
Cycle 1 finding #12 pinned the bootstrap **integration** test files (`cmd/bootstrap/bootstrap_test.go`, `internal/bootstrapadapter/adapters_test.go`). The Testing Requirements section also lists **unit tests** for stale-marker cleanup ("Given a marker whose paneKey doesn't correspond to a live pane, the cleanup unsets it" / "Given a live marker … the cleanup leaves it alone"). No file location is specified for these unit tests — they could go alongside the new bootstrap step's implementation file, or in a new file co-located with the adapter, and Files Touched does not list a corresponding `_test.go` file for them. A planner cannot break this into a task without knowing where the tests land.

**Proposed Addition**:
Pin the unit-test location to whichever file co-locates with the new step's implementation (likely `cmd/bootstrap/<new-step>_test.go`). Add the file to Files Touched.

**Resolution**: Pending
**Notes**:

---

### 3. PaneKey normalisation has no targeted test

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Testing Requirements / Stale-marker cleanup — unit

**Details**:
Cycle 1 finding #5/#6 made paneKey normalisation a load-bearing implementation step: `ListAllPanes` returns `session:window.pane` form (colon separator), `ListSkeletonMarkers` returns canonical `session__window.pane` form (double-underscore separator), and the cleanup step **must** call `state.SanitizePaneKey(session, window, pane)` before the set-difference or "the diff is meaningless." The Testing Requirements section does not specify a targeted test that would catch a regression where this conversion is dropped, wrong, or applied to the wrong side. A pure "live marker → leave alone" / "stale marker → unset" pair could pass even with broken normalisation if the test fixture happens to use matching forms on both sides. Without an explicit normalisation-correctness assertion, the most error-prone implementation step is uncovered.

**Proposed Addition**:
Add a unit test (or sub-case) that uses a marker keyed in canonical paneKey form (`session__0.1`) AND a live pane returned in tmux format (`session:0.1`), asserting the two are recognised as the same paneKey by the cleanup logic. A complementary negative test where two paneKeys differ only by separator should not collide.

**Resolution**: Pending
**Notes**:

---

### 4. Synthetic repro test does not specify required `prev` state setup

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Acceptance Criteria #1, Code Context / Reproduction Steps

**Details**:
The bug requires `prev != nil` and `prev.Sessions` containing the session — `mergeSkippedPanes` is gated on `prev != nil` (see Code Context line 100) and only resurrects sessions that are present in `prev.Sessions`. Acceptance criterion 1 reads "set marker, kill session, wait one daemon tick", and the Reproduction Steps section similarly does not state that the daemon must have already captured-and-committed the session on a prior tick before the kill (so the session lives in `prev`). For an interactive in-the-wild repro this is implicit (the daemon naturally captures the session before any kill happens), but for the synthetic test that backs acceptance criterion 1, an implementer needs to know to either (a) seed `prev` directly, or (b) let the daemon tick once before the kill to populate `prev`. Without this guidance, a naive test that sets the marker and kills the session before the daemon ever sees it will appear to pass even on the buggy code, producing a false-green regression test.

**Proposed Addition**:
Add a clarifying sentence to acceptance criterion 1 (or the Reproduction Steps section) that the test must establish `prev.Sessions` containing the target session before kill — either by seeding `prev` directly in the test harness, or by allowing one daemon tick to capture-and-commit before the kill. State the risk explicitly: a test that runs kill-then-tick without prior `prev` population will pass on buggy code.

**Resolution**: Pending
**Notes**:

---
