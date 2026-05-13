---
status: in-progress
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
AC1 reads: *"`GetServerOption(...)` returns `("", ErrOptionNotFound)` if and only if the underlying tmux call's stderr contains a substring from the option-absent pattern family."*

The Behaviour section (added/clarified in cycle 1, finding 5) explicitly states that when `errors.As(err, &cmdErr)` returns false (e.g., a mock returns a bare `errors.New("invalid option: @foo")`), the error propagates unchanged and is **not** mapped to `ErrOptionNotFound` — even though its stderr-like message contains a pattern-family substring.

Strictly read, AC1 is therefore inconsistent with the design: a bare error whose `.Error()` string contains `"invalid option:"` does not satisfy "the underlying tmux call's stderr contains a substring" because there is no captured stderr (no `*CommandError`). But a test author reading AC1 in isolation might write a test that returns a bare `errors.New("invalid option: @foo")` and assert `ErrOptionNotFound` — a test that would fail under the implemented contract.

This is a wording precision issue, not a design flaw, but it affects test-authoring readiness.

**Current**:
> 1. `GetServerOption("@some-marker")` returns `("", ErrOptionNotFound)` if and only if the underlying tmux call's stderr contains a substring from the option-absent pattern family (`invalid option:`, `unknown option:`, `ambiguous option:`).

**Proposed Addition**:
Tighten AC1 to anchor on the `*CommandError` carrier explicitly, e.g.:

> 1. `GetServerOption("@some-marker")` returns `("", ErrOptionNotFound)` if and only if the returned error unwraps (via `errors.As`) to a `*CommandError` whose `Stderr` contains a substring from the option-absent pattern family (`invalid option:`, `unknown option:`, `ambiguous option:`). Errors that do not carry a `*CommandError` propagate as non-`ErrOptionNotFound` errors regardless of their `.Error()` text.

**Resolution**: Pending
**Notes**:

---

### 2. Pre-implementation sweep covers tests but not production-code callers of `GetServerOption` / `TryGetServerOption`

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: `## Testing` → `### Pre-implementation sweep`; `## Scope` → `### In scope`

**Details**:
The sweep subsection (added in cycle 1, finding 7) enumerates the **test-code** audit surface. The Design intro asserts: *"every pre-existing caller of `GetServerOption` was an existence check that happily mapped failure to absence."* This assertion is load-bearing for the "no regression risk" claim in `## Risk & Rollout`, but the spec does not explicitly direct the implementer to re-confirm it at implementation time.

If a production caller anywhere in the codebase uses `errors.Is(err, tmux.ErrOptionNotFound)` semantically as "any failure" (relying on the old conflation), that caller silently flips from "treat any error as absence" to "treat only genuine absence as absence" — which is the *intended* fix, but the spec's Risk section claims "no new failure mode is introduced." A production caller that today relies on the conflation to silently no-op past transport errors would change behaviour, contradicting the Risk claim.

The investigation likely surveyed this, but the spec does not record the audited production surface, so the implementer cannot validate the Risk claim themselves before pressing merge.

**Proposed Addition**:
Either:

- (a) Add a brief enumeration to "Pre-implementation sweep" listing the production-code callers of `GetServerOption` and `TryGetServerOption` that were audited during investigation, with the verdict (existence check vs. behaviour-dependent on conflation), mirroring the test-code sweep enumeration; or
- (b) Add a one-line implementer instruction to re-grep `errors.Is(.*ErrOptionNotFound)` across the production tree and confirm each callsite is an existence check before implementing.

**Resolution**: Pending
**Notes**:

---

### 3. Discriminator-set unit tests location is not pinned

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: `## Testing` → `### internal/tmux/tmux_test.go` (final bullet)

**Details**:
The bullet *"Add discriminator-set unit tests: each entry in the option-absent pattern slice is exercised against a synthetic stderr containing it..."* is placed inside the `tmux_test.go` subsection, which implies that file. But the test target is the unexported `optionAbsentStderrPatterns` slice plus the `GetServerOption` discriminator behaviour — both are in `internal/tmux`. Cycle 1, finding 3, pinned same-package (white-box) tests. This is therefore implicit but not stated at the test-bullet level — a careful implementer would re-derive it from the cycle 1 edit, but the bullet stands on its own as a test instruction.

Minor; flagged for completeness given the cycle 2 brief asks about loose threads from cycle 1 edits.

**Proposed Addition**:
Append to the discriminator-set bullet: *"Tests live in `internal/tmux/tmux_test.go` (same package, white-box) so the unexported `optionAbsentStderrPatterns` slice is directly addressable."*

**Resolution**: Pending
**Notes**: Minor wording tightening only.

---
