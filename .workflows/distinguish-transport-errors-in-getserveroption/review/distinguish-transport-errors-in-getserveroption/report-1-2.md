# Review Report: Task 1-2 — Wire RealCommander.Run and RunRaw to wrap errors as *CommandError

STATUS: Complete
FINDINGS_COUNT: 0 blocking issues
SUMMARY: Task 1-2 is fully implemented and adequately tested; RealCommander.Run/RunRaw wrap non-nil errors via a shared WrapCommandError helper, preserving the cmd.Stderr==nil invariant with documented godoc, and behavioural tests exercise both error-paths through runCommand for sh-exit and missing-binary cases.

## Acceptance Criteria
- On *exec.ExitError failures, RealCommander.Run returns *CommandError with Stderr == string(exitErr.Stderr) verbatim and Err carrying original *exec.ExitError.
- On non-*exec.ExitError failures, RealCommander.Run returns *CommandError with Stderr == "" and Err carrying original error.
- RealCommander.RunRaw exhibits identical error-wrapping behaviour.
- errors.Is(err, originalUnderlyingErr) continues to work via Unwrap().
- cmd.Stderr is left nil with inline comment documenting the auto-populate invariant.
- Happy-path returns unchanged (Run trims, RunRaw verbatim, both nil err).
- go test ./... passes.

## Status: Complete

## Spec Context
Per spec "Wiring at RealCommander", both methods invoke exec identically via `exec.Command("tmux", args...)` + `cmd.Output()` with cmd.Stderr nil; differ only in stdout post-processing. Non-nil errors must wrap as *CommandError populated from (*exec.ExitError).Stderr when an ExitError, else Stderr empty. Spec calls cmd.Stderr-left-nil a "load-bearing invariant." Spec explicitly authorises factoring out a small runner helper (implementer chose a `runCommand` helper plus a shared `WrapCommandError` helper).

## Implementation
- Status: Implemented
- Location:
  - `internal/tmux/tmux.go:55-65` — `Run`/`RunRaw` thin wrappers calling `runCommand("tmux", trim, args...)`.
  - `internal/tmux/tmux.go:76-87` — `runCommand` helper; `cmd.Stderr` left nil with explicit inline comment referencing the `WrapCommandError` precondition; non-nil errors return `"", WrapCommandError(err)`.
  - `internal/tmux/command_error.go:73-83` — `WrapCommandError` helper. nil-in → nil-out; `errors.As(err, &exitErr)` populates `Stderr` verbatim from `exitErr.Stderr`; else `Stderr=""`. Original err preserved on `*CommandError.Err` so `Unwrap()` chains work.
  - `internal/tmux/command_error.go:62-67` — precondition godoc explicitly calls out the `cmd.Stderr==nil` invariant and the silent-failure mode if a future caller reassigns it.
- Notes:
  - Implementation goes beyond the minimum: `WrapCommandError` is extracted as a shared helper used by both `runCommand` AND `internal/tmuxtest/socketCommander` (`internal/tmuxtest/socket.go:131,141`) so integration tests against a real tmux socket route errors through the identical wrap shape. Right call for production/test parity.
  - `Run` godoc (tmux.go:52-54) and `RunRaw` godoc (tmux.go:59-62) both document the error-wrapping behaviour, more thorough than the task's "brief inline comment" requirement.
  - `runCommand` signature accepts the binary name — clean, lower-cost factoring than a test-only constructor (one of the two acceptable shapes offered by the task).

## Tests
- Status: Adequate
- Coverage:
  - `internal/tmux/realcommander_test.go:18-65` — `TestRealCommander_RunWrapsExitError` with `run` and `runs_raw_variant` subtests. Drives `sh -c 'echo "synthetic stderr marker" 1>&2; exit 1'` through `runCommand` directly. Asserts `errors.As` recovers `*CommandError`, `Stderr` contains marker, and `cmdErr.Err` unwraps to `*exec.ExitError`. Skips when `sh` missing per spec.
  - `internal/tmux/realcommander_test.go:75-118` — `TestRealCommander_RunWrapsNonExitError` with `run` and `runs_raw_variant` subtests. Invokes `__portal_test_nonexistent_binary__`. Asserts `*CommandError` recovered, `Stderr == ""`, and `cmdErr.Err` does NOT unwrap to `*exec.ExitError` — behavioural assertion using `errors.As` exactly as the spec edge-case calls for.
  - `internal/tmux/command_error_test.go` — complementary `TestWrapCommandError` covering the shared helper's three branches (nil input, exec.ExitError, non-exec error). Pins the wrap recipe used by both `runCommand` and `tmuxtest.socketCommander`.
- Notes:
  - Not over-tested: each subtest exercises a distinct behavioural property; no redundant assertions.
  - Not under-tested: Run/RunRaw parity verified via subtests on both paths; the cmd.Stderr-nil invariant is implicitly verified because the test only passes when Stderr is auto-populated.
  - Behavioural assertions only (errors.As, strings.Contains; no `.Error()`-string comparisons) — matches the spec's "rendered format is not part of the public contract" guidance.
  - No `t.Parallel()` — consistent with CLAUDE.md.

## Code Quality
- Project conventions: Followed — package-level docstrings, no t.Parallel, standard Go conventions.
- SOLID: Good. `runCommand` handles exec, `WrapCommandError` handles wrap — single responsibility. DRY without over-abstraction. Run/RunRaw differ only by the trim flag.
- Complexity: Low. `runCommand` 8 lines; `WrapCommandError` 11 lines. Linear control flow, no nesting.
- Modern idioms: Yes. `errors.As` for type extraction; struct-literal `*CommandError`.
- Readability: Good. Inline comment in `runCommand` references the `WrapCommandError` godoc rather than duplicating rationale.
- Issues: None.

## Blocking Issues
- None.

## Non-Blocking Notes
- [idea] The `WrapCommandError` extraction (used by both `runCommand` and `tmuxtest.socketCommander`) was not in the task's explicit "Do" list but is a strict improvement over an inline wrap in `runCommand` alone. The godoc on `WrapCommandError` already captures the rationale; no action required.
- [idea] `Run`/`RunRaw` godocs mention error wrapping; the `Commander` interface godoc (`tmux.go:36-44`) does not. A one-line note on the interface that "production implementations return non-nil errors as `*CommandError`" would aid future readers — out of scope for this task.
