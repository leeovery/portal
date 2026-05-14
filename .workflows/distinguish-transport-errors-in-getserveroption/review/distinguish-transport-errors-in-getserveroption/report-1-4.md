# Review Report: Task 1-4 — Replace documented-gap comment with defaultShutdownFlush err-branch test and add tick() err-branch test

STATUS: Complete
FINDINGS_COUNT: 0 blocking issues
SUMMARY: Task 1-4 fully implemented — documented-gap comment block removed, both err-branch tests (defaultShutdownFlush + tick) added using existing seams with no t.Parallel; assertions correctly verify nil-return and zero-commit via the production discriminator path.

## Acceptance Criteria
- [x] Comment block at `cmd/state_daemon_run_test.go:557-565` is removed (no trace of "cannot be tested through the public Client surface" remains in the file).
- [x] New test (`TestDefaultShutdownFlush_SkipsOnTransportError`) injects `*tmux.CommandError{Stderr: "lost server", ...}` via the daemon test seam, drives `defaultShutdownFlush`, asserts the function returns `nil`.
- [x] Same test asserts via existing capture/commit mock-tracking pattern that zero commit calls occurred.
- [x] New test (`TestTick_SkipsOnTransportError`) injects the same `*tmux.CommandError` shape into `tick()` and asserts that no capture / no commit calls are performed.
- [x] No new test seams introduced — existing daemon `Deps`-style injection and capture/commit mock surfaces are reused.
- [x] No new test uses `t.Parallel()`.
- [x] `go test ./cmd/...` passes; pre-existing daemon tests continue to pass.

## Status: Complete

## Spec Context
Spec's Problem section names `cmd/state_daemon_run_test.go:557-565` as the "fourth site" documenting the bug as a known gap. Spec §"Testing → cmd/state_daemon_run_test.go" mandates removing the comment block, adding the previously-blocked `defaultShutdownFlush` err-branch test, and adding `tick()` err-branch coverage. Fault-injection shape: `TryGetServerOption("@portal-restoring")` returns `("", false, &tmux.CommandError{Stderr: "lost server", Err: errors.New("exit status 1")})`. Acceptance: return-value `nil` + zero-commit are sufficient; log-capture optional.

## Implementation
- Status: Implemented
- Location:
  - `cmd/state_daemon_run_test.go:201-210` — `transportErrCommandError()` shared fault-injection helper (DRY across the two tests).
  - `cmd/state_daemon_run_test.go:583-618` — `TestDefaultShutdownFlush_SkipsOnTransportError` with `returns_nil` + `zero_commits` subtests.
  - `cmd/state_daemon_run_test.go:620-661` — `TestTick_SkipsOnTransportError` with `no_capture` + `no_commit` subtests.
  - Documented-gap comment block previously at 557-565 is fully removed; grep for "cannot be tested through the public" returns no matches.
- Notes:
  - Injection flows correctly through the production discriminator: `daemonFakeCommander.Run` for `show-option` returns `("", optionErr)` where `optionErr` is the `*CommandError` with `Stderr "lost server"`; `GetServerOption`'s `errors.As` + pattern-iteration sees no absent-pattern match, propagates the original wrapped error; `TryGetServerOption` hits its now-live `if err != nil` branch; `IsRestoringSet` propagates; `tick` / `defaultShutdownFlush` exercise their warn-log + early-return branches.
  - `"lost server"` does not contain `"invalid option:"`, `"unknown option:"`, or `"ambiguous option:"` — discriminator correctly propagates rather than collapsing to `ErrOptionNotFound`. Verified against `internal/tmux/tmux.go:340-354`.

## Tests
- Status: Adequate
- Coverage:
  - `returns_nil` subtest asserts `defaultShutdownFlush` returns `nil` under the injected `*CommandError`.
  - `zero_commits` subtest asserts (a) no `list-sessions` call (structural proof `captureAndCommit`'s first tmux call did not fire) and (b) `sessions.json` was not written.
  - `no_capture` subtest asserts (a) no `capture-pane` invocation and (b) no `list-sessions` invocation (independent structural proof).
  - `no_commit` subtest asserts (a) `sessions.json` is not written and (b) `save.requested` survives — extra correctness assertion beyond spec minimum (dirty flag must not be cleared on a skipped tick).
- Notes:
  - Optional `warn_log_fires` subtest omitted — spec explicitly says log-capture is optional. Neighbouring `TestDaemonTick_LogsAndSkipsOnShowOptionsError` (line 458) shows the existing log-capture pattern if needed later.
  - The `save.requested` survives-skip assertion in `no_commit` is a thoughtful invariant check, not over-testing.
  - `daemonFakeCommander`'s `show-option` dispatch was already updated in earlier tasks to return a production-shaped `*CommandError` for missing options — avoids fake/production drift.

## Code Quality
- Project conventions: Followed. CLAUDE.md mandates "Tests must not use `t.Parallel()`" — file-level comment at line 1 reasserts this; neither new test uses it. Existing cmd-package Deps-style injection seam is reused.
- SOLID principles: Good. `transportErrCommandError()` is a tightly-scoped DRY helper; no new abstraction introduced.
- Complexity: Low. Each test is setup + subtest assertion blocks; linear control flow.
- Modern idioms: Yes. Uses `t.Run` subtests with the plan's required names.
- Readability: Good. Docstring on each test names the production line (`cmd/state_daemon.go:95-99` for `tick`) and the spec contract verified.
- Issues: None.

## Blocking Issues
- None.

## Non-Blocking Notes
- [idea] Two new tests share fault-injection setup (dir, env, fc with `optionErr` + `sessionsOut`, `makeDeps`). `transportErrCommandError()` already factors the `*CommandError` literal; if a third transport-error consumer test is added later, factoring deps construction into a helper (e.g., `daemonDepsWithTransportErr(t)`) would reduce more duplication. Not required now.
- [idea] `TestDefaultShutdownFlush_SkipsOnTransportError`'s two subtests share the same `deps` and `fc` — currently safe because subtests only read `fc.calls` and stat `sessions.json` (read-only-shared-state). A future change adding state mutation in one subtest could leak into the other; if that risk materialises, splitting setup per-subtest would be the fix.

## Relevant Files
- `cmd/state_daemon_run_test.go` (implementation site)
- `cmd/state_daemon.go` (production targets at lines 94-120 and 187-202)
- `internal/tmux/tmux.go` (lines 340-354 — discriminator path under test)
- `internal/state/markers.go` (lines 145-154 — IsRestoringSet propagation)
