# Implementation Review: Distinguish Transport Errors in GetServerOption

**Plan**: distinguish-transport-errors-in-getserveroption
**QA Verdict**: Approve

## Summary

All five plan tasks (1-1 through 1-5) are fully implemented, tested, and contract-faithful to the specification. The `CommandError` type, `RealCommander` wrapping (via shared `WrapCommandError` helper), `GetServerOption` discriminator + `optionAbsentStderrPatterns` slice, daemon err-branch tests, and the four docstring tightenings all land coherently. Test coverage is behavioural (`errors.Is`/`errors.As`/`strings.Contains`) per spec guidance, with no `t.Parallel()` in the cmd package. No blocking issues found across any task. A handful of non-blocking ideas and one quickfix surfaced — none affect correctness or merge-readiness.

## QA Verification

### Specification Compliance

Implementation aligns with the specification. All three "must land together" units (CommandError type, RealCommander wiring, GetServerOption discriminator) ship together. Pattern slice is unexported, case-sensitive, and matches the three documented absence phrasings. Discriminator fallthrough behaves correctly for non-`*CommandError` errors and empty-stderr `*CommandError` (propagates as non-absence). `TryGetServerOption` body unchanged; its dead branch is now live. Four docstrings now accurately describe the post-fix contract. Documented-gap comment at `cmd/state_daemon_run_test.go:557-565` removed and replaced with both the previously-blocked flush test and the missing tick err-branch test. No daemon consumer logic was modified. No `Commander` interface signature change. No new exported symbols beyond `CommandError`.

### Plan Completion

- [x] Phase 1 acceptance criteria met
- [x] All tasks completed (1-1, 1-2, 1-3, 1-4, 1-5)
- [x] No scope creep — `WrapCommandError` helper extraction is in-bounds (reused by `internal/tmuxtest/socketCommander` for production/test parity, called out in the task's spec authorisation to factor a helper)

### Code Quality

No issues found. Single-responsibility helpers (`runCommand`, `WrapCommandError`); `errors.As` over type assertion; full godoc on all exported symbols; no `t.Parallel()` per CLAUDE.md; idiomatic Go throughout. Cyclomatic complexity is low across the changed surface.

### Test Quality

Tests adequately verify requirements. Coverage is behavioural (`errors.Is`/`errors.As`/`strings.Contains`); no brittle assertions against `.Error()` strings except in the `CommandError` formatting tests where exact strings are part of the spec. Discriminator-set tests iterate `optionAbsentStderrPatterns` directly so future pattern additions auto-extend coverage. Real-subprocess wrap tests use `sh` (with skip-on-missing) and a deterministic non-existent binary — no real `tmux` invocation needed for discriminator tests. Daemon tests use existing `Deps` injection seam with no new mock infrastructure. The optional warn-log subtests are correctly omitted per spec ("observability detail, not a correctness invariant").

### Required Changes

None.

## Recommendations

### Quick-fixes

1. **CommandError godoc leakage** (`internal/tmux/command_error.go`): the type docstring says "Stderr is empty when the failure was not an `*exec.ExitError` (e.g., executable not found)." This is accurate for the production wrap path but slightly leaks the wrapping convention into the type's own contract. Could be tightened to "Stderr is empty when no stderr was captured" so the type's contract stays independent of how it's produced.
2. **Slice comment hardcodes test filename** (`internal/tmux/tmux.go:21`): the `optionAbsentStderrPatterns` comment names `option_discriminator_internal_test.go`. If the file is renamed, the comment goes stale. Rephrase to "the same-package internal test file" without the specific filename.

### Ideas

3. **`WrapCommandError` placement**: lives in `command_error.go` but the filename suggests "type definition only". Future readers expecting a leaf type file will find a production helper too. Either rename the file or move the helper to `tmux.go` alongside `runCommand`.
4. **Commander interface godoc** (`internal/tmux/tmux.go:36-44`): `Run`/`RunRaw` godocs document the error-wrapping behaviour, but the `Commander` interface itself doesn't. A one-line note on the interface that "production implementations return non-nil errors as `*CommandError`" would aid future readers.
5. **`slice_contents_pinned` subtest**: goes slightly beyond the spec's minimum positive-per-entry + one-negative coverage. Defensible as anti-drift insurance; a strict reviewer could classify as over-testing. Net assessment: keep — silent membership drift would be a contract-faithful regression.
6. **Daemon transport-error setup duplication**: `TestDefaultShutdownFlush_SkipsOnTransportError` and `TestTick_SkipsOnTransportError` share fault-injection scaffolding (dir, env, `fc`, `makeDeps`). `transportErrCommandError()` already factors the `*CommandError` literal. If a third transport-error consumer test is added later, factoring deps construction into a helper (e.g., `daemonDepsWithTransportErr(t)`) would reduce more duplication. Not required now.
7. **`TestDefaultShutdownFlush_SkipsOnTransportError` shared state across subtests**: two subtests share the same `deps` and `fc`. Currently safe (subtests only read `fc.calls` and stat `sessions.json`). A future change adding state mutation in one subtest could leak into the other; split setup per-subtest if that risk materialises.
8. **Task 1-3 / 1-5 docstring overlap**: Task 1-3's implementation already supplied the full `GetServerOption` contract docstring (Task 1-5's "Site 2" deliverable). Reviewers auditing Task 1-5 in isolation may double-count or attempt to rewrite what already exists. Worth noting in working notes / commit history.
9. **`GetServerOption` docstring pattern enumeration** (`internal/tmux/tmux.go:331-339`): docstring names `optionAbsentStderrPatterns` but does not enumerate the three substrings inline. Spec template for Site 2 suggested enumerating them. Current form is acceptable — the slice is a reviewable single source of truth — but inline enumeration would let readers extract the full contract without leaving the docstring.
