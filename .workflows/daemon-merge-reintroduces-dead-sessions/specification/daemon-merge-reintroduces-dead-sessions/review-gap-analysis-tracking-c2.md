---
status: complete
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
Files Touched lists three functions but the design only edits `mergeSkippedPanes`. Whether helpers should grow defensive checks was unclear.

**Resolution**: Approved
**Notes**: Approved via auto mode. Added clarification: helpers untouched; the filter is the single point of enforcement at the merge entry. Files Touched mentions them only because they live in the same edited file.

---

### 2. Test file location for stale-marker cleanup unit tests is unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Testing Requirements / Stale-marker cleanup — unit, Files Touched

**Details**:
Cycle 1 pinned integration test files but unit-test locations remained vague.

**Resolution**: Approved
**Notes**: Approved via auto mode. Pinned to a co-located `_test.go` alongside the new step's implementation in `cmd/bootstrap/`. Files Touched updated.

---

### 3. PaneKey normalisation has no targeted test

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Testing Requirements / Stale-marker cleanup — unit

**Details**:
Cycle 1 made `state.SanitizePaneKey` conversion load-bearing but no targeted test guards it.

**Resolution**: Approved
**Notes**: Approved via auto mode. Added a "PaneKey normalisation correctness" sub-test requirement: fixture must mix tmux's `session:window.pane` form with canonical `session__window.pane` form across both sides, plus a complementary negative test.

---

### 4. Synthetic repro test does not specify required `prev` state setup

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Acceptance Criteria #1, Code Context / Reproduction Steps

**Details**:
The bug requires `prev != nil` and `prev.Sessions` containing the session. A naive test that runs kill-then-tick without prior `prev` population would false-green on buggy code.

**Resolution**: Approved
**Notes**: Approved via auto mode. Acceptance criterion 1 amended with explicit precondition (seed `prev.Sessions` directly OR allow one tick before kill) and explicit risk note. Reproduction Steps section similarly updated with a precondition step.

---
