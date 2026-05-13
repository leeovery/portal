---
status: in-progress
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
The intro reads "Four sites currently document or anticipate the distinguishability contract." The numbered list then contains five entries (1-5). Item 5 is the `cmd/state_daemon_run_test.go:557-565` documented-gap comment block, which is not a contract/docstring site like items 1-4 — it is a test-file comment that is removed and replaced by an actual test. A planner reading this would not know whether "four" is wrong, whether item 5 belongs under a different sub-heading (e.g., "Test housekeeping"), or whether one of items 1-4 should be merged. Minor but creates uncertainty when slicing tasks (does item 5 belong to a docs task or a tests task?).

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 2. `CommandError.Error()` output format is informally specified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: `## Design: CommandError at the Commander Layer` → `### Type`

**Details**:
The `Error()` method body is given as a code comment: `/* "<Err>: <Stderr>" when Stderr non-empty, else <Err>.Error() */`. Several things are under-specified:

- Separator: is it colon-space (`": "`), colon (`":"`), or something else? The comment shows `": "` but it is inside a non-executable comment.
- Trailing whitespace handling on `Stderr` (e.g., `"ambiguous option: "` has a trailing space per the absence-pattern discussion). Should `Error()` `TrimSpace` before joining?
- Is the rendered format part of the public contract (and therefore something tests should assert verbatim) or an internal detail subject to change?

An implementer must pick a format; if a future reviewer disagrees the test churn is wasted.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 3. Option-absent pattern slice export status and identifier are unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: `## Design: Discrimination in GetServerOption` → `### Option-absent pattern family`

**Details**:
The spec says: "The pattern set is exported as a small, package-level slice in `internal/tmux`". The rationale given (reviewable + testable in isolation, future additions in one place) does not require export — package-private would satisfy it. The Testing section's "discriminator-set unit tests" can access an unexported slice because they live in the same package (`tmux_test.go` declares `package tmux` for white-box) — but the spec does not say which test package style is used.

Additionally, no identifier is proposed (e.g., `optionAbsentStderrPatterns`, `OptionAbsentPatterns`). A planner cannot write a task description without naming it.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 4. `RealCommander.RunRaw` wrapping mechanism not fully described

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: `## Design: CommandError at the Commander Layer` → `### Wiring at RealCommander`

**Details**:
The wiring discussion only references `cmd.Output()` populating `(*exec.ExitError).Stderr` "when `cmd.Stderr == nil`". The spec asserts both `Run` and `RunRaw` wrap their errors, but does not confirm both methods invoke the process via `cmd.Output()` (vs. e.g., `cmd.CombinedOutput()` or manual `cmd.Stdout`/`cmd.Stderr` plumbing in `RunRaw`). If `RunRaw` uses a different exec path, the "auto-populated Stderr" invariant may not hold and the wiring task is materially different. The spec assumes both methods share the same shape — but does not state this is verified.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 5. Behavior when `errors.As` cannot extract a `*CommandError`

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: `## Design: Discrimination in GetServerOption` → `### Behaviour`

**Details**:
The behavior table lists three outcomes for `GetServerOption`. The third — "any other failure" — implicitly covers the case where `errors.As(err, &cmdErr)` returns `false` (i.e., a non-nil error that is not a `*CommandError` and does not unwrap to one). This could occur if:

- A future caller wraps the error before it reaches `GetServerOption`.
- A test mock returns a bare `errors.New(...)` — which the Testing section explicitly says is allowed for tests that "don't care about stderr".

The spec implies the discriminator falls through to "return the original error" in this case, but does not state it. The Testing-section claim "discriminators will see an empty `Stderr` and behave conservatively" is slightly misleading — a bare error has no `Stderr` field at all (the extraction fails), distinct from a `*CommandError{Stderr: ""}`. The end behavior is the same (treat as non-absence), but the mechanism should be documented for the planner.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 6. Whitespace normalisation of `Stderr` before pattern matching is implicit

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: `## Design: Discrimination in GetServerOption` → `### Option-absent pattern family`

**Details**:
The spec states "case-sensitive substring" matching with "no normalisation (lowercasing, regex) required" — and notes that `ambiguous option: ` from tmux contains a trailing space. Substring match against the pattern `"ambiguous option:"` works whether the stderr has a trailing space or newline. However the spec does not state whether the captured `Stderr` is the raw bytes from `(*exec.ExitError).Stderr` or trimmed. This matters for:

- `CommandError.Error()` rendering (see finding 2).
- The wrapped error's payload when propagated to consumers (warn-log readability).

Implicit choice: keep raw. Should be made explicit.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 7. Sweep of existing `GetServerOption` / `TryGetServerOption` call sites in tests is not scoped

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: `## Testing` → `### internal/tmux/tmux_test.go`; `## Scope` → `### In scope`

**Details**:
The Testing section calls out *one* existing test to reshape ("the `TestGetServerOption` 'option does not exist' case"). It does not state whether other existing tests in `tmux_test.go`, or in any consumer package, currently rely on the old behavior (bare error from mock → `ErrOptionNotFound`). If any other test mocks `Commander.Run` returning a bare error and expects `ErrOptionNotFound` to surface from `GetServerOption` or `TryGetServerOption`, that test breaks under the new contract.

The Acceptance Criteria item 5 ("All existing tests pass without behavioural change in the happy path") implies a sweep was done, but the spec does not enumerate the sweep's findings nor state the sweep is part of the implementation task. A planner cannot scope the "test reshape" task without knowing the surface area.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 8. `tick()` test conditional acceptance ("if not already covered")

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: `## Testing` → `### cmd/state_daemon_run_test.go`

**Details**:
Bullet reads: "**Add a parallel test for `tick()`'s err-handling branch** ... **if not already covered**". The planner is left to determine coverage. This is a small but real planning gap — the task either exists or does not. Acceptance Criterion 6 ("New tests assert each transport-error and pattern-discriminator scenario") does not clarify which tests count as "new" vs. pre-existing.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 9. Assertion mechanism for "flush returns nil without committing state"

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: `## Testing` → `### cmd/state_daemon_run_test.go`

**Details**:
The replacement test for `defaultShutdownFlush`'s err-branch must "assert the flush returns nil without committing state, and that the warn log is emitted." Three under-specifications for the planner:

- "Without committing state": what observable proves no commit? (E.g., a mocked commit-side seam asserts zero calls? A filesystem absence check? An injected stateStore mock?)
- "Warn log is emitted": how is the log asserted? The state daemon's logger is the `state` package's structured logger — does the test capture it via a test sink, or assert via a different seam?
- Whether the existing `state_daemon_run_test.go` already exposes these seams (Deps struct? log sink?) is not stated.

The planner can probably reverse-engineer this from neighboring tests, but the spec should pin the seam.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 10. `TestRealCommander_RunWrapsExitError` lacks concrete test invocation

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: `## Testing` → `### internal/tmux — Commander layer`

**Details**:
The new test uses "a real `os/exec` failure (e.g., invoke a command that returns a known stderr and exit 1)". Cross-platform: on macOS and Linux, `sh -c 'echo "x" 1>&2; exit 1'` works; on Windows it does not — but the spec confines platforms to Darwin/Linux (Risk section), so this is probably fine. Worth pinning the command + expected stderr so the test is reproducible across the planner's environment.

Similarly, "invoke a missing binary" for the non-`ExitError` test — any deterministic non-existent binary name (e.g., `__portal_test_nonexistent_binary__`) should be pinned.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 11. Pattern set is described as a slice, but discriminator semantics may want set/ordered

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: `## Design: Discrimination in GetServerOption` → `### Option-absent pattern family`

**Details**:
The spec calls the pattern set a "slice". Substring matching iterates and short-circuits on first match — order matters only for performance. Three patterns is trivial; not a real concern. But the planner may ask: is the slice iteration the intended discriminator (with `strings.Contains` per element) vs. a single compiled alternation? The spec implies the former by emphasising "no regex required" but does not state the iteration form. Minor.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 12. Existing-test claim "vindicates the test rather than the test driving the fix" lacks change record

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: `## Testing` → `### internal/state/markers_test.go`

**Details**:
The spec states `TestIsRestoringSet :: tmux exploded` (existing, line 206) "continues to pass" and "No code change required". For the planner this is fine, but acceptance criterion 5 ("all existing tests pass") implies this is verified — the spec should state whether the test currently passes or currently fails (perhaps it has been guarded with a known-skip or is silently asserting against the broken contract). If it currently passes by coincidence (because the mock returns something other than a bare error), the wording is accurate. If it currently fails or is skipped, the planner needs to know. Worth a one-line clarification.

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---

### 13. No explicit task slicing guidance or commit boundaries

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Whole spec (planning-readiness)

**Details**:
The spec contains five logical work units: (a) `CommandError` type, (b) `RealCommander` wiring, (c) `GetServerOption` discriminator + pattern slice, (d) docstring tightening at 4 sites, (e) test reshapes/additions including the documented-gap removal. The spec does not state whether these should land as one PR / one commit / multiple commits, nor in what order. Some ordering is load-bearing: (a) before (b) before (c) before (e). Docstrings (d) can land alongside (c). The planner can infer this, but an explicit recommendation prevents accidental partial-landing (e.g., wiring without discriminator leaves callers receiving raw `*CommandError` errors and silently breaks existing `errors.Is(err, ErrOptionNotFound)` checks at the `TryGetServerOption` consumer until (c) lands).

**Proposed Addition**:

**Resolution**: Pending
**Notes**:

---
