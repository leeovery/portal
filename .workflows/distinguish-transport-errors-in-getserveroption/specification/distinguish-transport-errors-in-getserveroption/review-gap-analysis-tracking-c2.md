---
status: complete
created: 2026-05-13
cycle: 2
phase: Gap Analysis
topic: distinguish-transport-errors-in-getserveroption
---

# Review Tracking: distinguish-transport-errors-in-getserveroption - Gap Analysis

## Findings

### 1. Acceptance Criterion 1 "if and only if" is imprecise vs the documented fallthrough semantics

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: `## Acceptance Criteria` (item 1); cross-reference `## Design: Discrimination in GetServerOption` → `### Behaviour` (Fallthrough paragraph)

**Details**:
AC1 used "underlying tmux call's stderr contains a substring" wording that could be mis-read by a test author. A bare `errors.New("invalid option: @foo")` (no `*CommandError`) would not satisfy the implemented contract but the wording was ambiguous.

**Proposed Addition**:
Tightened AC1 to anchor on the `*CommandError` carrier explicitly via `errors.As`. Added explicit clarification that errors without a `*CommandError` propagate regardless of `.Error()` text. AC2 also tightened to specify "whose Commander invocation produces one."

**Resolution**: Approved
**Notes**:

---

### 2. Pre-implementation sweep covers tests but not production-code callers of `GetServerOption` / `TryGetServerOption`

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: `## Testing` → `### Pre-implementation sweep`; `## Scope` → `### In scope`

**Details**:
The sweep enumerated only test-code surfaces; production-code callers were implicit.

**Proposed Addition**:
Restructured "Pre-implementation sweep" into two subsections (production-code sweep + test-code sweep) with a concrete grep command and audited verdicts for each call site. Production sweep records:
- `internal/tmux/tmux.go` — defines the types
- `internal/state/markers.go:140` — only production caller of TryGetServerOption; no `errors.Is(...ErrOptionNotFound)` callsite exists in production code outside `tmux.go` itself.

**Resolution**: Approved
**Notes**: Sweep verified during application — grep confirmed the audited surface.

---

### 3. Discriminator-set unit tests location is not pinned

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: `## Testing` → `### internal/tmux/tmux_test.go` (final bullet)

**Details**:
Bullet location was within the `tmux_test.go` subsection but did not state same-package white-box test style at the bullet level.

**Proposed Addition**:
Appended "in `internal/tmux/tmux_test.go` (same-package, white-box — so the unexported `optionAbsentStderrPatterns` slice is directly addressable)" to the bullet.

**Resolution**: Approved
**Notes**:

---
