---
topic: distinguish-transport-errors-in-getserveroption
cycle: 1
total_proposed: 3
---
# Analysis Tasks: distinguish-transport-errors-in-getserveroption (Cycle 1)

## Task 1: Extract CommandError wrap recipe into shared helper
status: approved
severity: high
sources: duplication, architecture

**Problem**: The five-line `*CommandError` wrap-from-exec-output recipe is implemented twice — once in production `runCommand` at `internal/tmux/tmux.go:82-89` and once in test `socketCommander.wrapErr` at `internal/tmuxtest/socket.go:128-143`. Both sites carry near-identical docstrings asserting the shape must match, and `socket.go`'s comment explicitly says it "mirrors" production. The spec's own "Invariant" note in `runCommand` warns that future changes (e.g. switching to `cmd.StderrPipe()`, or adding a third failure-mode branch) will silently diverge between production and the integration-test commander, breaking integration tests' fidelity to production discrimination semantics. There is no shared linkage forcing the two sites to move in lockstep.

**Solution**: Extract the wrap algorithm into an exported helper in `internal/tmux` (e.g. `func WrapCommandError(err error) error`) colocated with `CommandError` in `internal/tmux/command_error.go`. The helper returns nil when err is nil and otherwise returns a `*CommandError` populated from `*exec.ExitError` via `errors.As`. Both `runCommand` and `tmuxtest.socketCommander.Run`/`RunRaw` call it. The `cmd.Stderr == nil` precondition is documented once on the helper.

**Outcome**: Single source of truth for the production-shape `*CommandError` wrap. Production and the integration-test commander move together by construction. `tmuxtest/socket.go`'s "mirrors production" comment collapses to a one-line "uses tmux.WrapCommandError for production parity." No behavioural change — pure refactor.

**Do**:
1. In `/Users/leeovery/Code/portal/internal/tmux/command_error.go`, add an exported helper `func WrapCommandError(err error) error` that returns nil when err is nil; otherwise inspects `err` via `errors.As(err, &exitErr)` to populate `Stderr` from `(*exec.ExitError).Stderr`, then returns `&CommandError{Stderr: stderr, Err: err}`. Document the `cmd.Stderr` nil precondition on the helper's godoc.
2. In `/Users/leeovery/Code/portal/internal/tmux/tmux.go:82-89`, replace the inline wrap in `runCommand` with a call to `WrapCommandError`. Keep a brief invariant comment that links to the helper's docstring.
3. In `/Users/leeovery/Code/portal/internal/tmuxtest/socket.go:128-143`, replace `wrapErr`'s body with a call to `tmux.WrapCommandError`. Collapse the mirrored docstring to a single-line note referencing the production helper.
4. Run `go test ./...` to confirm no behavioural drift.

**Acceptance Criteria**:
- `internal/tmux/command_error.go` exports `WrapCommandError(err error) error`.
- `runCommand` in `internal/tmux/tmux.go` no longer contains the inline `errors.As` + `&CommandError{...}` construction — it delegates to `WrapCommandError`.
- `internal/tmuxtest/socket.go` no longer contains the inline wrap recipe — it delegates to `tmux.WrapCommandError`.
- The "cmd.Stderr must remain nil" precondition is documented exactly once (on `WrapCommandError`).
- `go test ./...` passes with no behavioural change to existing CommandError shape, GetServerOption discriminator, or TryGetServerOption semantics.

**Tests**:
- Add a focused unit test for `WrapCommandError` covering: nil input → nil output; `*exec.ExitError` with populated Stderr → `*CommandError` carrying that stderr and wrapping the exec error; non-exec error → `*CommandError` with empty Stderr wrapping the original error.
- Confirm existing `realcommander_test.go`, `option_discriminator_internal_test.go`, and `tmux_test.go` continue to pass — they implicitly assert the wrap shape via `errors.As(&cmdErr)` traversal.
- Confirm any tmuxtest-driven integration tests still produce the same `*CommandError` shape.

## Task 2: Make daemonFakeCommander exercise the production discriminator path
status: approved
severity: low
sources: architecture

**Problem**: At `cmd/state_daemon_run_test.go:95`, the `daemonFakeCommander`'s "option not in map" branch returns the bare sentinel `("", tmux.ErrOptionNotFound)` directly. Production `RealCommander` never surfaces a bare sentinel through the Commander layer — it always returns a `*CommandError` whose Stderr matches an absence pattern, and the discriminator inside `GetServerOption` is what maps it to `ErrOptionNotFound`. The fake bypasses the discriminator entirely. This works today only because `GetServerOption`'s non-`*CommandError` fallthrough happens to propagate the sentinel intact and `TryGetServerOption` uses `errors.Is`. A future test asserting "the error from a missing-option path unwraps to `*CommandError` with absence-pattern stderr" would pass against production but fail against the fake. The fake-vs-real divergence is precisely the class of issue this work unit was created to eliminate at the production layer.

**Solution**: Change the dispatch default for `show-option` in `daemonFakeCommander` from returning a bare `tmux.ErrOptionNotFound` to returning a `*tmux.CommandError` with an absence-pattern stderr (e.g. `"unknown option: " + args[2]`). Existing test behaviour is preserved because `GetServerOption`'s discriminator will recognise the absence pattern and map it to `ErrOptionNotFound`, and `TryGetServerOption` will still return `(found=false, err=nil)`.

**Outcome**: The daemon fake exercises the full production discriminator path. Tests written against the fake provably exercise the same error-classification logic that production runs, eliminating a hidden divergence between the fake and `RealCommander`.

**Do**:
1. Locate the `daemonFakeCommander` "option not in map" branch at `/Users/leeovery/Code/portal/cmd/state_daemon_run_test.go:95`.
2. Replace `return "", tmux.ErrOptionNotFound` with `return "", &tmux.CommandError{Stderr: "unknown option: " + args[2], Err: errors.New("exit status 1")}` (ensure `errors` is imported).
3. Verify the option-name index in `args` matches the actual `show-option` argv shape used by `GetServerOption`.
4. Run `go test ./cmd/...` to confirm existing daemon tests still pass — the discriminator now does the absence → sentinel mapping that the fake previously short-circuited.

**Acceptance Criteria**:
- `daemonFakeCommander` no longer returns a bare `tmux.ErrOptionNotFound` for the missing-option branch.
- The returned error is a `*tmux.CommandError` with stderr containing one of the absence patterns recognised by the discriminator (`invalid option:`, `unknown option:`, or `ambiguous option:`).
- Existing daemon tests (`TestTick_*`, `TestDefaultShutdownFlush_*`, etc.) continue to pass unchanged.
- `TryGetServerOption` called through the fake still returns `(found=false, err=nil)` for missing options.

**Tests**:
- All existing tests in `cmd/state_daemon_run_test.go` must continue to pass.
- The new fake path is implicitly covered by every existing daemon test that hits a missing option — those tests now exercise the discriminator end-to-end through the fake.

## Task 3: Extract transport-error fault-injection literal into a same-file helper
status: approved
severity: low
sources: duplication

**Problem**: At `cmd/state_daemon_run_test.go:567-571` and `cmd/state_daemon_run_test.go:605-609`, `TestDefaultShutdownFlush_SkipsOnTransportError` and `TestTick_SkipsOnTransportError` both construct an identical `&tmux.CommandError{Stderr: "lost server", Err: errors.New("exit status 1")}` and seed it on `daemonFakeCommander.optionErr`. The same concept — "transport failure that must not collapse to absence" — is duplicated in two test bodies a few lines apart.

**Solution**: Extract a small same-file helper in `cmd/state_daemon_run_test.go` (e.g. `func transportErrCommandError() *tmux.CommandError`) returning the canonical transport-failure `*tmux.CommandError`. Both transport-error tests call it.

**Outcome**: One source of truth in the test file for the canonical "non-absent transport failure" shape. Future tests inheriting the same fault-injection seed pick it up without re-typing the literal, reducing the chance that one site drifts.

**Do**:
1. In `/Users/leeovery/Code/portal/cmd/state_daemon_run_test.go`, add an unexported helper near the existing fake setup: `func transportErrCommandError() *tmux.CommandError { return &tmux.CommandError{Stderr: "lost server", Err: errors.New("exit status 1")} }`.
2. Replace the inline literal at ~`:567-571` with a call to the helper.
3. Replace the inline literal at ~`:605-609` with a call to the helper.
4. Run `go test ./cmd/...` to confirm behaviour is unchanged.

**Acceptance Criteria**:
- The literal `&tmux.CommandError{Stderr: "lost server", Err: errors.New("exit status 1")}` appears at most once in `cmd/state_daemon_run_test.go` (inside the helper).
- Both `TestDefaultShutdownFlush_SkipsOnTransportError` and `TestTick_SkipsOnTransportError` use the helper.
- Both tests continue to pass.

**Tests**:
- `TestDefaultShutdownFlush_SkipsOnTransportError` and `TestTick_SkipsOnTransportError` must continue to pass unchanged in behaviour.
