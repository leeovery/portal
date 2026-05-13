---
status: complete
created: 2026-05-13
cycle: 1
phase: Gap Analysis
topic: distinguish-transport-errors-in-getserveroption
---

# Review Tracking: distinguish-transport-errors-in-getserveroption - Gap Analysis

## Findings

### 1. Documentation Updates header says "Four sites" but lists five

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: `## Documentation Updates` (intro line + items 1-5)

**Details**:
Header said "Four sites" but list contained five entries. Item 5 (the test-file comment block) is not a docstring site and belongs to a different task than the docstring tightening.

**Proposed Addition**:
Renamed section to "Documentation & Test-Comment Updates"; intro now states four docstring sites + one test-comment site, with explicit task-routing guidance (docs task vs test-reshape task).

**Resolution**: Approved
**Notes**:

---

### 2. `CommandError.Error()` output format is informally specified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: `## Design: CommandError at the Commander Layer` → `### Type`

**Details**:
The `Error()` body was given as an in-comment placeholder. Separator, whitespace handling, and public-contract status were under-specified.

**Proposed Addition**:
Added explicit formatting rules: colon-space separator, `strings.TrimSpace(Stderr)` for the rendered output, `Stderr` itself stored verbatim, format not part of the public contract (behavioural assertions only).

**Resolution**: Approved
**Notes**:

---

### 3. Option-absent pattern slice export status and identifier are unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: `## Design: Discrimination in GetServerOption` → `### Option-absent pattern family`

**Details**:
Export status of the pattern slice and its identifier were not pinned.

**Proposed Addition**:
Pinned identifier (`optionAbsentStderrPatterns`), export status (unexported — package is `internal/tmux`), test-package style (white-box, same-package tests), and iteration form (`strings.Contains` loop, no regex).

**Resolution**: Approved
**Notes**: Also addresses Finding 11 (slice iteration form).

---

### 4. `RealCommander.RunRaw` wrapping mechanism not fully described

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: `## Design: CommandError at the Commander Layer` → `### Wiring at RealCommander`

**Details**:
Spec asserted both Run and RunRaw wrap errors but did not confirm both methods invoke via `cmd.Output()` with `cmd.Stderr == nil`.

**Proposed Addition**:
Confirmed via source inspection: both methods invoke identically via `exec.Command + cmd.Output()` with `cmd.Stderr` nil, differing only in stdout post-processing. Pinned RunRaw line range (`tmux.go:51-58`).

**Resolution**: Approved
**Notes**:

---

### 5. Behavior when `errors.As` cannot extract a `*CommandError`

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: `## Design: Discrimination in GetServerOption` → `### Behaviour`

**Details**:
Fallthrough behaviour when `errors.As` returns false was implicit.

**Proposed Addition**:
Added explicit "Fallthrough when `errors.As` returns false" paragraph describing the propagate-original semantics and noting `errors.Is(ErrOptionNotFound)` correctly returns false.

**Resolution**: Approved
**Notes**:

---

### 6. Whitespace normalisation of `Stderr` before pattern matching is implicit

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: `## Design: Discrimination in GetServerOption` → `### Option-absent pattern family`

**Details**:
Whether `Stderr` is stored raw or trimmed was implicit.

**Proposed Addition**:
Added explicit "`Stderr` storage" paragraph: verbatim storage, `strings.Contains` for matching, only `Error()` trims for rendering.

**Resolution**: Approved
**Notes**: Folded into the same edit as Finding 5.

---

### 7. Sweep of existing call sites in tests is not scoped

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: `## Testing` → `### internal/tmux/tmux_test.go`; `## Scope` → `### In scope`

**Details**:
Spec called out one existing test to reshape but did not enumerate sweep findings.

**Proposed Addition**:
Added a "Pre-implementation sweep" subsection to Testing enumerating the three audited surfaces (`tmux_test.go`, `markers_test.go:206`, `state_daemon_run_test.go`) with the verdict for each.

**Resolution**: Approved
**Notes**:

---

### 8. `tick()` test conditional acceptance ("if not already covered")

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: `## Testing` → `### cmd/state_daemon_run_test.go`

**Details**:
"if not already covered" left coverage determination to the planner.

**Proposed Addition**:
Replaced conditional with explicit two-path guidance (replace existing if covered, add new if not) anchored to the sweep.

**Resolution**: Approved
**Notes**:

---

### 9. Assertion mechanism for "flush returns nil without committing state"

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: `## Testing` → `### cmd/state_daemon_run_test.go`

**Details**:
"Without committing state" and "warn log is emitted" assertion mechanisms were unspecified.

**Proposed Addition**:
Expanded the bullet into four sub-points: fault injection seam, return-value assertion, zero-commit assertion via existing daemon mock pattern, warn-log capture (with fallback if log-capture seam doesn't exist).

**Resolution**: Approved
**Notes**:

---

### 10. `TestRealCommander_RunWrapsExitError` lacks concrete test invocation

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: `## Testing` → `### internal/tmux — Commander layer`

**Details**:
Test command shapes were illustrative rather than concrete.

**Proposed Addition**:
Pinned concrete invocation: `sh -c 'echo "synthetic stderr marker" 1>&2; exit 1'` for the ExitError test, `__portal_test_nonexistent_binary__` for the non-ExitError test. Also noted that `RealCommander` is hard-coded to `tmux` today, so the test may need a small refactor (small `runner` helper accepting the binary name).

**Resolution**: Approved
**Notes**:

---

### 11. Pattern set is described as a slice, but discriminator semantics may want set/ordered

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: `## Design: Discrimination in GetServerOption` → `### Option-absent pattern family`

**Details**:
Iteration form was not pinned.

**Proposed Addition**:
Addressed in Finding 3's edit — `for _, pat := range optionAbsentStderrPatterns { if strings.Contains(...) { return ErrOptionNotFound } }`.

**Resolution**: Approved
**Notes**: Resolved by the same edit as Finding 3.

---

### 12. Existing-test claim "vindicates the test rather than the test driving the fix" lacks change record

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: `## Testing` → `### internal/state/markers_test.go`

**Details**:
Did not explain why the test passes today despite the bug (it bypasses `TryGetServerOption` via a custom `RestoringChecker` mock).

**Proposed Addition**:
Rewrote the bullet to explain the mock topology: `checkerMock` implements `RestoringChecker` directly and surfaces the error to `IsRestoringSet` without going through `TryGetServerOption`. Test passes today and continues to pass; after the fix, the end-to-end production path is finally capable of delivering the same contract.

**Resolution**: Approved
**Notes**:

---

### 13. No explicit task slicing guidance or commit boundaries

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Whole spec (planning-readiness)

**Details**:
No guidance on what must land atomically vs what can split.

**Proposed Addition**:
Added a new "Implementation Ordering" section (placed before "Alternatives Considered") with five numbered units, mandatory ordering, the load-bearing fact about (1)+(2)+(3) being inseparable, and a single-PR recommendation.

**Resolution**: Approved
**Notes**:

---
