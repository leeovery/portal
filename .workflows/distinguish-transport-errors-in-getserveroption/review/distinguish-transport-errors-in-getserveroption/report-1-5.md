# Review Report: Task 1-5 — Tighten the four contract-violation docstrings

STATUS: Complete
FINDINGS_COUNT: 0 blocking issues
SUMMARY: All four docstrings (TryGetServerOption, GetServerOption, RestoringChecker, IsRestoringSet) accurately describe the post-fix contract with explicit ErrOptionNotFound and *CommandError / errors.As references; no function bodies modified; no misleading phrasing remains.

## Acceptance Criteria
- TryGetServerOption docstring names ErrOptionNotFound explicitly and describes three-case (value, found, err) contract.
- GetServerOption docstring describes discriminator behaviour — ErrOptionNotFound only on stderr pattern match, wrapped *CommandError propagated otherwise, errors.Is compatibility preserved.
- RestoringChecker interface docstring references tmux.ErrOptionNotFound as discriminator sentinel (or unchanged if sufficient).
- IsRestoringSet docstring coherent with new contract (existing wording already accurate; tightening optional).
- No function signature, body, or symbol name modified.
- go build ./... and go test ./... continue to pass — doc-only.
- grep for previous mis-leading phrasing produces no false positives.

## Status: Complete

## Spec Context
Four production-code docstring sites documented or anticipated the distinguishability contract the buggy GetServerOption could not deliver. After Tasks 1-1 through 1-3 made GetServerOption contract-faithful, Task 1-5 brings docstrings into coherence — naming ErrOptionNotFound as absence sentinel and noting other failures surface as wrapped *CommandError errors recoverable via errors.As. Spec Implementation Ordering unit (4) marks the task as doc-only, "No behaviour change."

## Implementation
- Status: Implemented
- Location:
  - `internal/tmux/tmux.go:331-339` (GetServerOption docstring)
  - `internal/tmux/tmux.go:356-369` (TryGetServerOption docstring)
  - `internal/state/markers.go:30-40` (RestoringChecker)
  - `internal/state/markers.go:136-144` (IsRestoringSet)
- Notes per site:
  - Site 1 (TryGetServerOption): Names ErrOptionNotFound explicitly on the "Option absent" arm, lays out three-case (value, found, err) contract, and explicitly mentions *CommandError recovery via errors.As(err, &cmdErr). Mentions representative failure modes ("socket connect, lost server, exec lookup, etc.").
  - Site 2 (GetServerOption): Previously vestigial one-liner replaced with six-line contract description naming the errors.As → *CommandError extraction, the optionAbsentStderrPatterns slice match, and the propagate-on-mismatch behaviour. References optionAbsentStderrPatterns by name (not enumerated inline).
  - Site 3 (RestoringChecker): Existing wording amended to point at errors.Is(err, tmux.ErrOptionNotFound) as discriminator sentinel; adds clarification that TryGetServerOption does discrimination internally so internal/state does not need to import internal/tmux.
  - Site 4 (IsRestoringSet): Existing wording preserved and extended with sentence stating the propagated error wraps a *tmux.CommandError recoverable via errors.As for diagnosis. Matches the spec's optional tightening.
- No function signatures, bodies, or symbol names were modified — confirmed by reading function bodies (GetServerOption tmux.go:340-354, TryGetServerOption tmux.go:370-379, IsRestoringSet markers.go:145-154 all intact).
- Grep sweep for misleading phrasing ("always ErrOptionNotFound", "maps every error", "conflates", "distinguishability") across *.go files returned no matches.

## Tests
- Status: Adequate (none authored — doc-only)
- Coverage: Plan and spec explicitly state no new tests required; behavioural tests from Tasks 1-1 through 1-4 transitively verify the contract the docstrings describe.
- Notes: No under-testing or over-testing concern.

## Code Quality
- Project conventions: Followed. Idiomatic Go docstrings, leading "// FunctionName" form, canonical CamelCase symbol references, sentence punctuation, blank-line separators around bullet lists.
- SOLID principles: N/A — doc-only.
- Complexity: N/A — doc-only.
- Modern idioms: Docstrings reference errors.As / errors.Is correctly (Go 1.13+ wrapping primitives), avoid coupling to os/exec types.
- Readability: Good. TryGetServerOption's bulleted three-case form is the clearest possible expression of the contract. RestoringChecker's added explanation of why the interface lives in internal/state is a thoughtful addition.
- Issues: None substantive.

## Blocking Issues
- None.

## Non-Blocking Notes
- [idea] GetServerOption docstring (tmux.go:331-339) names optionAbsentStderrPatterns but does not enumerate the three substrings (`invalid option:`, `unknown option:`, `ambiguous option:`) inline. Spec template for Site 2 suggested enumerating them. Current form is acceptable — the slice is a reviewable single source of truth — but inline enumeration would let readers extract the full contract without leaving the docstring. Defer to author judgement.
