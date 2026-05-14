# Review Report: Task 1-1 — Introduce CommandError type in internal/tmux

STATUS: Complete
FINDINGS_COUNT: 0 blocking issues
SUMMARY: Task 1-1's CommandError type is fully implemented per spec with all eight planned test cases present, exported correctly (verified via external-package test file), and adherent to project conventions.

## Acceptance Criteria
- `tmux.CommandError` exported with public fields `Stderr string` and `Err error`
- `(*CommandError).Error()` returns `"<wrapped>: <trimmed stderr>"` when both present
- `(*CommandError).Error()` returns bare `Err.Error()` when Stderr trims to empty
- `(*CommandError).Error()` returns trimmed Stderr (or `"<no error>"`) when Err is nil
- `(*CommandError).Unwrap()` returns embedded Err
- External-package consumer can construct as bare literal (no factory)
- `go build`/`go test` continue to pass

## Status: Complete

## Spec Context
CommandError is the root primitive of the spec's "Design: CommandError at the Commander Layer". The Commander interface's `(string, error)` signature erased tmux's stderr distinction; callers could only discriminate by type-asserting on `*exec.ExitError`, coupling to `os/exec`. The typed error restores diagnostic shape in a value the interface signature already carries. Spec requires: exported, struct-literal constructable (no factory), three-case `Error()` formatting, `Unwrap()` returning Err. Verbatim Stderr storage; trim only at rendering.

## Implementation
- Status: Implemented
- Locations:
  - `internal/tmux/command_error.go:17-20` — type definition
  - `internal/tmux/command_error.go:31-43` — `Error()`
  - `internal/tmux/command_error.go:47-49` — `Unwrap()`
- Notes:
  - Sibling-file placement (`command_error.go`) explicitly permitted by plan
  - Docstring matches spec's "Type" section; calls out no-factory contract
  - `Error()` branch order is clean: nil-Err first, then trimmed-empty fallback, then full rendering
  - `Unwrap()` returns `e.Err` verbatim; safe when Err is nil
  - Stderr stored verbatim; only `Error()` applies `strings.TrimSpace` per spec
  - File also exposes `WrapCommandError` (lines 73-83) — Task 1-2's deliverable, co-located reasonably

## Tests
- Status: Adequate
- Coverage at `internal/tmux/tmux_test.go`:
  - `TestCommandError_Error` (lines 2725-2784) — table-driven, seven subtests covering all three formatting cases plus whitespace edges
  - `TestCommandError_Unwrap` (lines 2786-2795) — asserts Unwrap() + errors.Is traversal
  - `TestCommandError_UnwrapNil` (lines 2797-2802) — defensive: no panic on nil Err
  - `TestCommandError_ErrorsAsThroughFmtWrap` (lines 2804-2818) — errors.As through `fmt.Errorf("%w", ...)`
  - `TestCommandError_StructLiteralConstruction` (lines 2820-2825) — compile-time assertion of bare-literal constructability
- Notes:
  - All eight test cases enumerated in plan's Tests section present, mapped 1:1
  - Tests live in `package tmux_test` (external) — stronger than plan's "same-package" suggestion; implicitly asserts exportedness of type + both fields
  - No `t.Parallel()` (Grep confirmed zero occurrences). Compliant with CLAUDE.md
  - `TestCommandError_Error` asserts exact strings; appropriate for a formatting test where spec defines the strings
  - Not over-tested: each subtest exercises a distinct branch. Not under-tested: every case in plan covered, plus an extra "nil err with whitespace-only stderr" case closing a defensive gap

## Code Quality
- Project conventions: Followed — exported types/methods have full godoc, no `t.Parallel()`, sibling-file placement matches `internal/tmux` organisation
- SOLID: Good — single responsibility, pointer receivers consistent with errors-package idioms, open for future discriminators on the same channel
- Complexity: Low — `Error()` is a four-branch decision tree; `Unwrap()` is one-liner
- Modern idioms: Yes — `Unwrap()` (Go 1.13+), pointer receivers, `strings.TrimSpace`
- Readability: Good — docstring lists three cases in numbered form matching spec; field order matches spec example
- Issues: None

## Blocking Issues
- None

## Non-Blocking Notes
- [idea] `WrapCommandError` lives in `command_error.go` (lines 73-83) — Task 1-2's deliverable. The co-location is fine architecturally, but the filename suggests "type definition only". Future maintainers reading only this file expecting a leaf type file will find a production-side helper too. Worth considering whether `WrapCommandError` belongs in `tmux.go` alongside `runCommand`, or whether the file should be renamed. Not in scope for this task; flagging for holistic review.
- [quickfix] CommandError godoc says "Stderr is empty when the failure was not an `*exec.ExitError` (e.g., executable not found)" — accurate for the production wrap path but slightly leaks the wrapping convention into the type's own docs. Could be tightened to "Stderr is empty when no stderr was captured" to keep the type's contract independent of how it's produced.

## Relevant Files
- `internal/tmux/command_error.go`
- `internal/tmux/tmux_test.go` (lines 2725-2825)
