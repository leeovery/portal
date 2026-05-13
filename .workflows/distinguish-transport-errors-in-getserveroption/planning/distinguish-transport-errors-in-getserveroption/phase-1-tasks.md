---
phase: 1
phase_name: Surface transport errors through GetServerOption discriminator
total: 7
---

## distinguish-transport-errors-in-getserveroption-1-1 | approved

### Task 1-1: Introduce CommandError type in internal/tmux

**Problem**: The `Commander` interface signature `Run(args ...string) (string, error)` erases tmux's stderr distinction. Without a typed error carrying stderr, callers cannot route on stderr content without type-asserting on `*exec.ExitError`, which couples them to `os/exec` and breaks the mock surface. This is the root primitive missing from the package — every downstream task (RealCommander wiring, GetServerOption discrimination, test reshapes) depends on it.

**Solution**: Introduce an exported, struct-literal-constructable `CommandError` type in `internal/tmux` carrying `Stderr string` and `Err error`, implementing `Error()` per the spec's formatting rules and `Unwrap()` returning `Err`. No production behaviour change in this task — the type is defined but no caller wraps with it yet.

**Outcome**: `internal/tmux` exposes a `CommandError` struct that test mocks can construct as a bare literal (`&tmux.CommandError{Stderr: "...", Err: errors.New("...")}`), supports `errors.As` extraction, and renders consistently under the three documented `Error()` cases (trimmed stderr appended, stderr empty, nil Err defensive fallback).

**Do**:
- In `internal/tmux/tmux.go` (or a new sibling file `internal/tmux/command_error.go` if the implementer prefers — same package), add the exported type:
  - `type CommandError struct { Stderr string; Err error }`
  - `func (e *CommandError) Error() string` — implements the three formatting cases:
    1. `Stderr` after `strings.TrimSpace` non-empty AND `Err != nil`: return `e.Err.Error() + ": " + strings.TrimSpace(e.Stderr)`.
    2. `Stderr` empty/whitespace-only AND `Err != nil`: return `e.Err.Error()`.
    3. `Err == nil`: return `strings.TrimSpace(e.Stderr)`, or `"<no error>"` if that is also empty.
  - `func (e *CommandError) Unwrap() error { return e.Err }`
- Add a docstring on the type matching the spec's "Type" section (CommandError wraps an error returned by Commander.Run / Commander.RunRaw and carries the captured stderr from the underlying process; Stderr empty when the failure was not an *exec.ExitError).
- Do not add a `NewCommandError` factory — plain struct literal only (per spec "Placement and exported-ness").
- No callers are modified in this task.

**Acceptance Criteria**:
- [ ] `tmux.CommandError` is exported from `internal/tmux` with public fields `Stderr string` and `Err error`.
- [ ] `(*CommandError).Error()` returns `"<wrapped>: <trimmed stderr>"` when both `Err` and trimmed `Stderr` are non-empty.
- [ ] `(*CommandError).Error()` returns the bare `Err.Error()` when `Stderr` trims to empty.
- [ ] `(*CommandError).Error()` returns trimmed `Stderr` (or `"<no error>"`) when `Err` is nil.
- [ ] `(*CommandError).Unwrap()` returns the embedded `Err` so `errors.Is` / `errors.As` chains work.
- [ ] `&tmux.CommandError{Stderr: "x", Err: errors.New("y")}` compiles from outside the package.
- [ ] `go build ./...` and `go test ./...` continue to pass — no existing test broken (type is additive).

**Tests** (in `internal/tmux/tmux_test.go`, same-package; no `t.Parallel()` per CLAUDE.md):
- `"it formats with colon-space separator when stderr and err are both present"` — assert `(&CommandError{Stderr: "invalid option: @foo", Err: errors.New("exit status 1")}).Error() == "exit status 1: invalid option: @foo"`.
- `"it trims surrounding whitespace from stderr in the rendered string"` — assert `Stderr: "  invalid option: @foo\n  "` renders as `"exit status 1: invalid option: @foo"`.
- `"it falls back to the bare err message when stderr is empty"` — assert `Stderr: ""` renders as `"exit status 1"`.
- `"it falls back to the bare err message when stderr is whitespace only"` — assert `Stderr: "\n  \t"` renders as `"exit status 1"`.
- `"it returns trimmed stderr when err is nil"` — assert `&CommandError{Stderr: "  boom  ", Err: nil}.Error() == "boom"`.
- `"it returns the sentinel <no error> string when both fields are empty"` — assert `&CommandError{}.Error() == "<no error>"`.
- `"it unwraps to the embedded err"` — assert `errors.Is(&CommandError{Err: sentinelErr}, sentinelErr)` where `sentinelErr := errors.New("x")`.
- `"it is recoverable via errors.As"` — wrap a `*CommandError` in `fmt.Errorf("ctx: %w", ...)` and assert `errors.As` recovers a non-nil `*CommandError` with the same `Stderr`.

**Edge Cases**:
- Whitespace-only `Stderr` (`"\n"`, `"   "`, `"\t\n"`) must be treated as empty by `Error()` rendering.
- `Err == nil` is defensive — callers should never construct this in practice, but `Error()` must not panic.
- Both fields empty — must return the literal `"<no error>"` rather than the empty string (so logs are never blank).

**Context**:
> Spec "Type" section: "Error() formatting:
> - When Stderr (after strings.TrimSpace) is non-empty: return e.Err.Error() + ': ' + strings.TrimSpace(e.Stderr) — colon-space separator, trimmed stderr.
> - When Stderr is empty or whitespace-only: return e.Err.Error().
> - When e.Err == nil (defensive — should not happen in practice): return strings.TrimSpace(e.Stderr), or '<no error>' if that is also empty."
>
> "The rendered format is not part of the public contract; tests assert behavioural properties (e.g., that errors.As extracts a *CommandError, that Stderr contains the expected substring), not the exact string. Stderr itself is stored verbatim — only the Error() rendering trims for readability."
>
> "Placement and exported-ness: package-level exported type in internal/tmux. Exported so test code outside the package (and any future package-level helpers) can construct *CommandError literals as mock returns. The constructor is a plain struct literal — no NewCommandError factory."
>
> Implementation Ordering unit (1) — "introduce the type in internal/tmux. No production behaviour change."

**Spec Reference**: `.workflows/distinguish-transport-errors-in-getserveroption/specification/distinguish-transport-errors-in-getserveroption/specification.md` — "Design: CommandError at the Commander Layer" → "Type"; "Implementation Ordering" unit (1).

---

## distinguish-transport-errors-in-getserveroption-1-2 | approved

### Task 1-2: Wire RealCommander.Run and RunRaw to wrap errors as *CommandError

**Problem**: `RealCommander.Run` (`internal/tmux/tmux.go:39-46`) and `RealCommander.RunRaw` (`internal/tmux/tmux.go:51-58`) currently return raw errors from `cmd.Output()`. Without wrapping at this seam, stderr never leaves the `*exec.ExitError` and the downstream discriminator in `GetServerOption` has nothing to inspect.

**Solution**: After `cmd.Output()` returns a non-nil error, wrap it in `*tmux.CommandError`. If the error is `*exec.ExitError`, populate `Stderr` from `(*exec.ExitError).Stderr`; otherwise leave `Stderr` empty. Preserve the original error via `Unwrap()`. Same wiring applied identically to both `Run` and `RunRaw`.

**Outcome**: Every non-nil error returned from `RealCommander.Run` / `RealCommander.RunRaw` is a `*tmux.CommandError`. `errors.As(err, &cmdErr)` succeeds; `errors.Is` against the underlying error continues to work via `Unwrap()`. Note: at this point in the implementation order, `GetServerOption` still maps every error to `ErrOptionNotFound` (load-bearing per spec Implementation Ordering unit 2), so external behaviour is unchanged until Task 1-3 lands.

**Do**:
- In `internal/tmux/tmux.go:39-46`, after `out, err := cmd.Output()`:
  - If `err != nil`, build a `*CommandError`:
    - Use `var exitErr *exec.ExitError; if errors.As(err, &exitErr) { stderr = string(exitErr.Stderr) }` else `stderr = ""`.
    - Return `"", &CommandError{Stderr: stderr, Err: err}`.
- Apply the identical wrapping in `RealCommander.RunRaw` (`internal/tmux/tmux.go:51-58`) — same structure, no behavioural divergence between the two methods on the error path.
- Do **not** assign `cmd.Stderr` — `(*exec.ExitError).Stderr` is auto-populated by `cmd.Output()` only when `cmd.Stderr == nil`. Preserving this invariant is part of the task; add a brief inline comment on each wrap referencing the invariant so future maintainers do not silently regress it.
- Do not modify the success path — successful returns still trim (Run) / verbatim (RunRaw).
- Do not modify the `Commander` interface signature.

**Acceptance Criteria**:
- [ ] On `*exec.ExitError` failures, `RealCommander.Run` returns a `*CommandError` with `Stderr == string(exitErr.Stderr)` (verbatim, no trim) and `Err` carrying the original `*exec.ExitError`.
- [ ] On non-`*exec.ExitError` failures (e.g., `exec.LookPath` failure where the binary is missing), `RealCommander.Run` returns a `*CommandError` with `Stderr == ""` and `Err` carrying the original error.
- [ ] `RealCommander.RunRaw` exhibits identical error-wrapping behaviour to `Run`.
- [ ] `errors.Is(err, originalUnderlyingErr)` continues to work for callers of `Run` / `RunRaw` via the `Unwrap()` chain.
- [ ] `cmd.Stderr` is left as `nil` in both methods (the auto-populate invariant for `(*exec.ExitError).Stderr` is preserved); an inline comment documents this.
- [ ] Happy-path returns are unchanged — `Run` trims, `RunRaw` returns verbatim, both with `nil` error.
- [ ] `go test ./...` passes — existing tests continue to pass because at this point `GetServerOption` still maps any error to `ErrOptionNotFound`.

**Tests**: covered behaviourally by Task 1-5 (`TestRealCommander_RunWrapsExitError`, `TestRealCommander_RunWrapsNonExitError`). This task's implementation is verified by Task 1-5's tests; no additional tests authored here.

**Edge Cases**:
- Non-`ExitError` underlying type (e.g., `*exec.Error` from `exec.LookPath` when `tmux` binary is missing) — wrap with empty `Stderr`; do not attempt to extract stderr from it.
- `cmd.Stderr` assignment invariant — if a future change ever assigns `cmd.Stderr` (to tee, capture, etc.), `(*exec.ExitError).Stderr` becomes empty silently. The inline comment must call this out; spec calls this a "load-bearing invariant of the current RealCommander implementation."
- Exit error with empty stderr (process exited non-zero but emitted nothing on stderr) — `Stderr == ""` is acceptable; downstream discriminator treats empty stderr as non-match = non-absence = propagate.

**Context**:
> Spec "Wiring at RealCommander":
> "If the error is *exec.ExitError, populate Stderr from (*exec.ExitError).Stderr. This field is auto-populated by cmd.Output() only when cmd.Stderr == nil — a precondition of the current RealCommander implementation. Future changes that assign cmd.Stderr (e.g., to tee stderr elsewhere) would silently break the wrapping; the wiring is responsible for preserving this invariant or capturing stderr explicitly via cmd.StderrPipe() if cmd.Stderr is repurposed."
>
> "If the error is any other type (e.g., exec.Command(...) failed to find the binary), wrap with Stderr: ''. An empty Stderr means 'no signal' — discriminators that examine Stderr will see no pattern match and treat the error as non-absence, which is the correct conservative behaviour."
>
> "In both cases the original error is preserved via Unwrap() so existing errors.Is / errors.As checks against sentinel errors continue to work."
>
> Implementation Ordering: unit (2) "RealCommander wiring — Run and RunRaw start returning *CommandError. Existing errors.Is(err, ErrOptionNotFound) checks at TryGetServerOption consumers continue to work only because GetServerOption still maps every error to ErrOptionNotFound — until step 3 lands, this is the load-bearing fact. Do not split (2) and (3) across PRs."

**Spec Reference**: `.workflows/distinguish-transport-errors-in-getserveroption/specification/distinguish-transport-errors-in-getserveroption/specification.md` — "Design: CommandError at the Commander Layer" → "Wiring at RealCommander"; "Implementation Ordering" unit (2).

---

## distinguish-transport-errors-in-getserveroption-1-3 | approved

### Task 1-3: Add optionAbsentStderrPatterns slice and rewrite GetServerOption to discriminate via errors.As

**Problem**: `Client.GetServerOption` at `internal/tmux/tmux.go:304-310` collapses every error from `c.cmd.Run("show-option", "-sv", name)` into `ErrOptionNotFound`. After Task 1-2 lands, the wrapped `*CommandError` carries the stderr needed to discriminate "option absent" from "transport failure" — but `GetServerOption` still discards it. Until this task lands, `TryGetServerOption`'s dead branch remains dead and daemon consumers continue to receive `("", false, nil)` for transport errors (the contract violation described in the spec's Problem section).

**Solution**: Introduce an unexported package-level `optionAbsentStderrPatterns []string` slice in `internal/tmux` containing the three known absence phrasings. Rewrite `GetServerOption` to extract `*CommandError` via `errors.As`, iterate the slice with `strings.Contains` against `cmdErr.Stderr`, and return `ErrOptionNotFound` only on match. Everything else propagates the original error unchanged.

**Outcome**: `GetServerOption` is contract-faithful: returns `("", ErrOptionNotFound)` only for genuine absence (stderr pattern match); returns `("", err)` with the wrapped `*CommandError` recoverable via `errors.As` for every other failure. `TryGetServerOption`'s `if err != nil { return "", false, err }` branch becomes live. Daemon consumers (`tick()`, `defaultShutdownFlush()`) start receiving non-nil errors on transport failures and execute their existing warn-log + early-return paths. No consumer code is modified.

**Do**:
- In `internal/tmux/tmux.go` (near `ErrOptionNotFound` at line 12-13, or in the same file as `GetServerOption`), add:
  ```go
  // optionAbsentStderrPatterns lists the stderr substrings tmux uses to signal
  // that a server option does not exist. Substring match is case-sensitive.
  var optionAbsentStderrPatterns = []string{
      "invalid option:",
      "unknown option:",
      "ambiguous option:",
  }
  ```
- Rewrite `GetServerOption` (`internal/tmux/tmux.go:304-310`) to:
  ```go
  func (c *Client) GetServerOption(name string) (string, error) {
      output, err := c.cmd.Run("show-option", "-sv", name)
      if err == nil {
          return strings.TrimSpace(output), nil
      }
      var cmdErr *CommandError
      if errors.As(err, &cmdErr) {
          for _, pat := range optionAbsentStderrPatterns {
              if strings.Contains(cmdErr.Stderr, pat) {
                  return "", ErrOptionNotFound
              }
          }
      }
      return "", err
  }
  ```
- Do not modify `TryGetServerOption` (`internal/tmux/tmux.go:317-326`) — its body is correct; its dead branch becomes live by virtue of this change.
- Do not modify any daemon consumer code (`cmd/state_daemon.go:95-99` `tick()`, `cmd/state_daemon.go:187-201` `defaultShutdownFlush()`).
- The pattern slice is unexported — same-package tests address it directly. Do not add to the package's exported API.
- Pattern iteration is a plain `for _, pat := range` short-circuit on first match. No compiled regex, no alternation, no lowercasing.

**Acceptance Criteria**:
- [ ] `optionAbsentStderrPatterns` exists as an unexported `[]string` in `internal/tmux` containing exactly `"invalid option:"`, `"unknown option:"`, `"ambiguous option:"` (case-sensitive, with trailing colon).
- [ ] `GetServerOption` returns `(strings.TrimSpace(output), nil)` on success — unchanged behaviour.
- [ ] `GetServerOption` returns `("", ErrOptionNotFound)` when (and only when) the underlying error unwraps via `errors.As` to a `*CommandError` whose `Stderr` contains one of the patterns.
- [ ] `GetServerOption` returns `("", err)` propagating the original wrapped error for: (a) `*CommandError` with empty `Stderr`, (b) `*CommandError` with unmatched stderr, (c) any non-`*CommandError` error (e.g., a test mock returning `errors.New(...)` directly).
- [ ] `errors.As(err, &cmdErr)` succeeds on the propagated error for cases (a) and (b); `errors.As` returns false for case (c) and the caller correctly receives `false` because no `*CommandError` exists in the chain.
- [ ] `TryGetServerOption("@some-marker")` returns `("", false, non-nil-err)` when the underlying error is non-absent — the dead branch is now live.
- [ ] Daemon consumers' existing `if err != nil { log.Warn(...); return }` branches fire for transport errors (verified via Task 1-6 tests).
- [ ] No new exported symbols added by this task beyond what Task 1-1 introduced.
- [ ] `go test ./...` passes (combined with Tasks 1-1 / 1-2 / 1-4 / 1-5 / 1-6 / 1-7 in a single landing).

**Tests** (covered by Task 1-4 — reshaped `TestGetServerOption` plus new transport / non-exit / try-wrapper / discriminator-set tests). This task's implementation surface is verified by Task 1-4's tests; no additional tests authored here.

**Edge Cases**:
- `errors.As` returns false (e.g., mock returns `errors.New("...")` directly) — propagate the original error unchanged. Same outcome as "matched a `*CommandError` with empty `Stderr`" — no pattern match, propagate.
- Empty `Stderr` on a wrapped `*CommandError` — `strings.Contains("", pat)` is false for any non-empty pattern, so empty stderr propagates as non-absence. This is the correct conservative behaviour for non-ExitError wraps.
- `Stderr` stored verbatim (including trailing whitespace or newlines from tmux) — `strings.Contains` is insensitive to trailing whitespace, so verbatim storage is tolerated without normalisation.
- `"ambiguous option: "` (with trailing space) — empirically observed from `show-option -sv ""` on Darwin 25.3.0. Pattern is `"ambiguous option:"` without trailing space, so `strings.Contains` matches the colon and ignores whatever follows.
- Future tmux phrasing not in the slice — propagates as non-absence (correct conservative behaviour; surfaces as a fast unit-test failure in Task 1-4's discriminator-set tests if a new phrasing ships, allowing one-line extension to the slice).

**Context**:
> Spec "Design: Discrimination in GetServerOption" → "Behaviour":
> "Client.GetServerOption(name string) (string, error) extracts the wrapped *CommandError from any non-nil error, inspects its Stderr, and returns:
> - (strings.TrimSpace(output), nil) on success (no error).
> - ('', ErrOptionNotFound) when the error unwraps to a *CommandError whose Stderr matches the option-absent pattern family (see below).
> - ('', err) — the original wrapped error — for every other failure (transport, executable-missing, server crash, unmatched stderr).
> Extraction uses errors.As(err, &cmdErr) so a future error-wrapping change at the Commander layer does not break the discriminator."
>
> "Fallthrough when errors.As returns false: if err is a non-nil error that is not a *CommandError and does not unwrap to one (e.g., a test mock returning errors.New(...) directly, or a future caller that wraps errors before they reach GetServerOption), the discriminator treats the failure as non-absence and returns the original err unchanged."
>
> Spec "Option-absent pattern family":
> "The pattern set is a small, package-level slice in internal/tmux named optionAbsentStderrPatterns (unexported — internal/tmux already gates the surface, so package-private is sufficient and avoids adding to the package's exported API; tests live in the same tmux package and can read the unexported slice directly)."
>
> "Iteration form: a simple for _, pat := range optionAbsentStderrPatterns { if strings.Contains(cmdErr.Stderr, pat) { return ErrOptionNotFound } } — short-circuits on first match. No compiled regex, no alternation. Three patterns; iteration cost is negligible."
>
> "Matching is case-sensitive substring against cmdErr.Stderr. No normalisation (lowercasing, regex) is required."
>
> Implementation Ordering: unit (3) "discriminator becomes contract-faithful. TryGetServerOption's if err != nil branch becomes live. Daemon consumers start receiving transport errors. (1)+(2)+(3) must land together."

**Spec Reference**: `.workflows/distinguish-transport-errors-in-getserveroption/specification/distinguish-transport-errors-in-getserveroption/specification.md` — "Design: Discrimination in GetServerOption"; "Design: TryGetServerOption and Consumer Surface"; "Implementation Ordering" unit (3).

---

## distinguish-transport-errors-in-getserveroption-1-4 | approved

### Task 1-4: Reshape TestGetServerOption and add discriminator/transport/non-exit/try-wrapper tests

**Problem**: The existing `TestGetServerOption` "option does not exist" case in `internal/tmux/tmux_test.go` uses `errors.New("unknown option: @portal-active-%3")` — the error string is decorative, never inspected, because today every error from `cmd.Run` becomes `ErrOptionNotFound` regardless of content. The test therefore does not exercise stderr-pattern matching. Additionally, there are no tests for: transport-error propagation, non-exit-error propagation, `TryGetServerOption`'s previously-dead error branch, or the discriminator pattern set itself. These gaps must be closed atomically with Task 1-3 to lock the new contract behaviourally.

**Solution**: Reshape the existing "option does not exist" test to use a `*CommandError` whose `Stderr` matches the option-absent pattern family. Add five new test functions covering transport errors (parametrised), non-exit-error propagation, `TryGetServerOption` transport propagation, full discriminator-set coverage (each pattern + negative case), and `errors.As` recovery of stderr from the wrapped error.

**Outcome**: `internal/tmux/tmux_test.go` exercises the full contract behaviourally — every option-absent pattern is locked in, transport errors are confirmed to propagate, non-exit errors are confirmed to propagate, and `TryGetServerOption`'s now-live error branch has explicit coverage. Future tmux phrasing changes surface as a fast test failure rather than silent drift.

**Do**:
- All tests in this task live in `internal/tmux/tmux_test.go` (same-package so the unexported `optionAbsentStderrPatterns` slice is directly addressable per spec). No `t.Parallel()` (project policy).
- **Reshape `TestGetServerOption` "option does not exist" case**: locate the subtest in `internal/tmux/tmux_test.go` whose mock previously returned `errors.New("unknown option: @portal-active-%3")`. Replace the bare `errors.New(...)` with `&CommandError{Stderr: "unknown option: @portal-active-%3", Err: errors.New("exit status 1")}`. Keep the assertion `errors.Is(err, ErrOptionNotFound)`.
- **Add `TestGetServerOption_TransportError`** — parametrised (table-driven) over at minimum these stderr shapes:
  - `"error connecting to /tmp/tmux-501//default (No such file or directory)"` (socket/transport)
  - `"lost server"` (server-crash shape)
  For each: mock `c.cmd.Run` to return `"", &CommandError{Stderr: tc.stderr, Err: errors.New("exit status 1")}`. Assert `!errors.Is(err, ErrOptionNotFound)`, assert `errors.As(err, &cmdErr)` succeeds, and assert `cmdErr.Stderr == tc.stderr` (verbatim recovery).
- **Add `TestGetServerOption_NonExitErrorPropagates`** — mock returns `"", &CommandError{Stderr: "", Err: errors.New("exec: \"tmux\": not found")}`. Assert `!errors.Is(err, ErrOptionNotFound)` and assert `errors.As` recovers a `*CommandError` with empty `Stderr`.
- **Add `TestTryGetServerOption_PropagatesTransportError`** — mock returns the same socket-connect `*CommandError` shape. Call `c.TryGetServerOption("@some-marker")`. Assert `val == ""`, `found == false`, `err != nil`, and `errors.As(err, &cmdErr)` succeeds with the expected `Stderr`. This is the test for the previously-unreachable `if err != nil { return "", false, err }` branch.
- **Add `TestGetServerOption_DiscriminatorSet`** — table-driven over every entry in `optionAbsentStderrPatterns`:
  - For each `pat`: build a stderr that contains the pattern as a substring (e.g., `pat + " @foo"`), mock returns `&CommandError{Stderr: stderr, Err: errors.New("exit status 1")}`, assert `errors.Is(err, ErrOptionNotFound)`.
  - One negative subcase: stderr = `"some unrelated error: connection refused"`, assert `!errors.Is(err, ErrOptionNotFound)` and assert the original error propagates.
- All tests use the existing `Commander` mock surface — no new mock framework, no real `os/exec`. Mock returns the canonical `*CommandError` literal shape.

**Acceptance Criteria**:
- [ ] The existing `TestGetServerOption` "option does not exist" subtest is reshaped to construct a `*CommandError` mock return and continues to assert `errors.Is(err, ErrOptionNotFound)`.
- [ ] `TestGetServerOption_TransportError` exists, parametrised over at least the socket-connect and server-crash stderr shapes, and asserts non-`ErrOptionNotFound` propagation plus `errors.As`-recoverable verbatim `Stderr`.
- [ ] `TestGetServerOption_NonExitErrorPropagates` exists and asserts that a wrapped non-`*exec.ExitError` (empty `Stderr`) propagates as non-absence.
- [ ] `TestTryGetServerOption_PropagatesTransportError` exists and exercises the previously-unreachable `if err != nil { return "", false, err }` branch via the public `TryGetServerOption` surface.
- [ ] `TestGetServerOption_DiscriminatorSet` exercises every entry in the unexported `optionAbsentStderrPatterns` slice plus one negative case. If the slice is later extended, the test must surface the addition (e.g., iterate the slice directly rather than hardcoding three subtests).
- [ ] All new tests compile and pass when run with `go test ./internal/tmux/...`.
- [ ] No `t.Parallel()` in any new or reshaped test.
- [ ] All other tests in `internal/tmux/tmux_test.go` (success paths, `ShowAllServerOptions`, etc.) continue to pass unchanged.

**Tests** (this task's tests are the deliverable):
- `"TestGetServerOption/option_does_not_exist"` (reshaped) — `errors.Is(err, ErrOptionNotFound)` holds when mock returns `*CommandError` with `Stderr: "unknown option: @portal-active-%3"`.
- `"TestGetServerOption_TransportError/socket_connect_failure"` — non-absent propagation + `errors.As` recovers verbatim stderr.
- `"TestGetServerOption_TransportError/lost_server"` — same shape, different stderr.
- `"TestGetServerOption_NonExitErrorPropagates"` — empty-stderr wrap propagates as non-absence.
- `"TestTryGetServerOption_PropagatesTransportError"` — `("", false, non-nil)` with `errors.As`-recoverable `*CommandError`.
- `"TestGetServerOption_DiscriminatorSet/invalid_option"` — match.
- `"TestGetServerOption_DiscriminatorSet/unknown_option"` — match.
- `"TestGetServerOption_DiscriminatorSet/ambiguous_option"` — match.
- `"TestGetServerOption_DiscriminatorSet/unrelated_stderr_does_not_match"` — non-match propagates.

**Edge Cases**:
- `errors.As` recovery of stderr from a wrapped error — assert behaviour, not exact `.Error()` string (spec: "rendered format is not part of the public contract").
- Each pattern individually exercised — driving the discriminator-set test off the slice itself (e.g., `for _, pat := range optionAbsentStderrPatterns`) ensures a future slice extension is automatically covered.
- Negative unrelated stderr (e.g., `"some unrelated error: connection refused"` — note: contains a colon but not the absent phrasings) must not falsely match.
- The reshape — the old test passed by accident (every error became `ErrOptionNotFound`); the new test must fail under the old code (before Task 1-3 lands) and pass after.

**Context**:
> Spec "Testing — internal/tmux/tmux_test.go":
> "Reshape existing TestGetServerOption 'option does not exist' case (currently uses errors.New('unknown option: @portal-active-%3')): the existing error string is decorative — it is never inspected because today every error from cmd.Run becomes ErrOptionNotFound regardless of content. Replace the bare errors.New(...) with a *CommandError whose Stderr matches the option-absent pattern family so the test actually exercises stderr-pattern matching. The test continues to assert errors.Is(err, ErrOptionNotFound)."
>
> "Add TestGetServerOption_TransportError: parametrised over a small set of representative non-absent stderr shapes — at minimum the socket-connect failure ('error connecting to /tmp/tmux-501//default (No such file or directory)') and a server-crash shape ('lost server'). Mock returns a *CommandError with each stderr; assert !errors.Is(err, ErrOptionNotFound) and that the returned error unwraps to a *CommandError carrying the original stderr."
>
> "Add TestGetServerOption_NonExitErrorPropagates: mock returns a *CommandError{Stderr: '', Err: errors.New('exec: \"tmux\": not found')}."
>
> "Add TestTryGetServerOption_PropagatesTransportError: covers the previously-unreachable if err != nil { return '', false, err } branch."
>
> "Add discriminator-set unit tests in internal/tmux/tmux_test.go (same-package, white-box — so the unexported optionAbsentStderrPatterns slice is directly addressable): each entry in the slice is exercised against a synthetic stderr containing it, asserting ErrOptionNotFound is returned. A negative case asserts an unrelated stderr does not match."
>
> Test policy: "Per CLAUDE.md: tests in cmd and any package using *Deps injection must not use t.Parallel()."

**Spec Reference**: `.workflows/distinguish-transport-errors-in-getserveroption/specification/distinguish-transport-errors-in-getserveroption/specification.md` — "Testing → internal/tmux/tmux_test.go"; "Pre-implementation sweep → Test-code sweep".

---

## distinguish-transport-errors-in-getserveroption-1-5 | approved

### Task 1-5: Add RealCommander wrap tests for exit and non-exit errors

**Problem**: Task 1-2 wires `RealCommander.Run` / `RealCommander.RunRaw` to wrap errors as `*CommandError`, but the discriminator-layer tests in Task 1-4 use the `Commander` mock and never exercise the real `os/exec` wrap. Without dedicated tests at the `RealCommander` boundary, the auto-populate behaviour of `(*exec.ExitError).Stderr` (load-bearing per spec) is not locked in — a future change that assigns `cmd.Stderr` would silently regress the wrap with no test failure.

**Solution**: Add two boundary tests that exercise `RealCommander` directly: one drives a real subprocess to exit non-zero with synthetic stderr and asserts the wrapped `*CommandError` carries the stderr; the other drives a deterministic non-existent binary and asserts the non-exit wrap produces empty `Stderr` and unwraps to a non-`*exec.ExitError`.

**Outcome**: The `RealCommander` error-wrap surface is locked behaviourally. Future regressions of the `cmd.Stderr == nil` invariant or the non-exit wrap branch surface as fast test failures.

**Do**:
- All tests in `internal/tmux/tmux_test.go` (or a sibling file `internal/tmux/realcommander_test.go` — implementer's choice; same package). No `t.Parallel()`.
- **Add `TestRealCommander_RunWrapsExitError`**:
  - Drive a subprocess that exits non-zero and emits a known stderr marker. The spec recommends `sh -c 'echo "synthetic stderr marker" 1>&2; exit 1'`.
  - `RealCommander.Run` is hard-coded to invoke `tmux` — the implementer must either (a) factor out a small internal `runner` helper that accepts the binary name and call it from both `Run`/`RunRaw` and the test, OR (b) introduce a test-only constructor that targets a configurable binary. The spec explicitly authorises whichever shape is lower-cost — the implementer picks.
  - Assert the returned error is non-nil, `errors.As(err, &cmdErr)` succeeds, and `strings.Contains(cmdErr.Stderr, "synthetic stderr marker")` is true.
  - Skip the test (`t.Skip(...)`) if `sh` is not on `PATH` (`exec.LookPath("sh")` returns error). Spec's platform applicability is Darwin + Linux — `sh` is always available there; the skip exists for defensive portability.
- **Add `TestRealCommander_RunWrapsNonExitError`**:
  - Invoke a deterministic non-existent binary: `__portal_test_nonexistent_binary__`.
  - Assert `errors.As(err, &cmdErr)` succeeds, `cmdErr.Stderr == ""`, and `cmdErr.Unwrap()` returns an error that is **not** `*exec.ExitError` (i.e., a `var exitErr *exec.ExitError; errors.As(cmdErr.Err, &exitErr) == false`). The underlying type is `*exec.Error` from `exec.LookPath` / `cmd.Start`.
- If the test also exercises `RealCommander.RunRaw` (recommended — confirms parity), include a parametrised subtest variant covering both methods with identical assertions.

**Acceptance Criteria**:
- [ ] `TestRealCommander_RunWrapsExitError` exists, drives a real subprocess via `sh -c ...`, and asserts the wrap produces a `*CommandError` with the expected stderr substring.
- [ ] The test skips cleanly on platforms where `sh` is not on `PATH`.
- [ ] `TestRealCommander_RunWrapsNonExitError` exists, drives a non-existent binary, and asserts the wrap produces empty `Stderr` plus a non-`*exec.ExitError` underlying error.
- [ ] If a test-only helper or factored `runner` was added to support driving non-`tmux` binaries, it is unexported and lives in the production file (`internal/tmux/tmux.go`) or a sibling — not in test-only files.
- [ ] Both tests assert behaviourally (`errors.As`, `strings.Contains`, type assertion) — not against the exact `.Error()` string.
- [ ] Both tests run independently of any tmux server (no tmux dependency).
- [ ] `go test ./internal/tmux/...` passes on Darwin and Linux.

**Tests** (this task's tests are the deliverable):
- `"TestRealCommander_RunWrapsExitError"` — `sh -c 'echo ... 1>&2; exit 1'` produces a wrapped `*CommandError` with the expected stderr substring.
- `"TestRealCommander_RunWrapsExitError/runs_raw_variant"` (optional but recommended) — same assertion against `RunRaw`.
- `"TestRealCommander_RunWrapsNonExitError"` — non-existent binary produces `*CommandError{Stderr: "", Err: <non-ExitError>}`.
- `"TestRealCommander_RunWrapsNonExitError/runs_raw_variant"` (optional) — same against `RunRaw`.

**Edge Cases**:
- Platform applicability — `sh` not on `PATH` → `t.Skip`. Darwin + Linux always have it; the skip is defensive.
- Non-`ExitError` underlying type assertion — must use `var exitErr *exec.ExitError; !errors.As(cmdErr.Err, &exitErr)` to confirm the wrap correctly identified the non-exit case. Asserting against `*exec.Error` directly is brittle if Go's exec internals change.
- Cross-platform stderr quoting — the synthetic marker should be a plain ASCII string with no shell metacharacters to avoid platform-dependent quoting.

**Context**:
> Spec "Testing — internal/tmux — Commander layer":
> "TestRealCommander_RunWrapsExitError (new): invoke sh -c 'echo \"synthetic stderr marker\" 1>&2; exit 1' via a temporarily-shimmed exec path or by exposing a small test-only constructor that targets sh instead of tmux (the implementer picks the lower-cost shape; if RealCommander is hard-coded to tmux, factor out a small runner helper that accepts the binary name and have the test target it). Assert the returned error unwraps to *CommandError with Stderr containing 'synthetic stderr marker'. Skipped automatically on platforms where sh is not on PATH; the platform-applicability statement (Darwin + Linux) makes this acceptable."
>
> "TestRealCommander_RunWrapsNonExitError (new): invoke a deterministic non-existent binary name — __portal_test_nonexistent_binary__. Assert the returned error unwraps to *CommandError with Stderr == '' and Unwrap() returning a non-*exec.ExitError error (the underlying *exec.Error from exec.LookPath / cmd.Start)."
>
> Risk & Rollout — Platform applicability: "Darwin and Linux."

**Spec Reference**: `.workflows/distinguish-transport-errors-in-getserveroption/specification/distinguish-transport-errors-in-getserveroption/specification.md` — "Testing → internal/tmux — Commander layer"; "Risk & Rollout → Platform applicability".

---

## distinguish-transport-errors-in-getserveroption-1-6 | approved

### Task 1-6: Replace documented-gap comment with defaultShutdownFlush err-branch test and cover tick() err-branch

**Problem**: `cmd/state_daemon_run_test.go:557-565` contains a comment block documenting that the err-branch of `defaultShutdownFlush()` (`cmd/state_daemon.go:187-201`) cannot be tested through the public `Client` surface because of the `GetServerOption` conflation. After Tasks 1-1 through 1-3 land, the err-branch is finally reachable end-to-end. The comment block must be removed and replaced with the previously-blocked test. Additionally, the spec's test-code sweep calls for `tick()`'s err-handling branch (`cmd/state_daemon.go:95-99`) to receive matching coverage — either by adding a new test or by updating an existing mock that was returning a bare `errors.New(...)` (which the new contract treats as non-absence-propagating).

**Solution**: Remove the lines 557-565 comment block in `cmd/state_daemon_run_test.go`. Add a new test that injects a tmux-client mock whose `TryGetServerOption("@portal-restoring")` returns `("", false, &tmux.CommandError{Stderr: "lost server", Err: errors.New("exit status 1")})`, drives `defaultShutdownFlush`, and asserts `nil` return value + zero commit calls. Audit existing `tick()` tests against the pre-implementation sweep: if any returns a bare `errors.New(...)` to the err-branch, update it to a `*tmux.CommandError`; if `tick()`'s err-branch is uncovered, add a new test of the same fault-injection shape.

**Outcome**: The documented gap is closed by code, not by comment. `defaultShutdownFlush` and `tick()` both have unit tests confirming their existing conservative-on-error branches fire under injected transport errors. The "third site documenting the bug as a known gap" referenced in the spec's Problem section is eliminated.

**Do**:
- **Remove the comment block at `cmd/state_daemon_run_test.go:557-565`** entirely (the block that documents the conflation as a known gap).
- **Add `TestDefaultShutdownFlush_SkipsOnTransportError`** (name per spec equivalent) in `cmd/state_daemon_run_test.go`:
  - Use the existing daemon `Deps`-style seam to inject a tmux-client mock. The mock's `TryGetServerOption("@portal-restoring")` returns `("", false, &tmux.CommandError{Stderr: "lost server", Err: errors.New("exit status 1")})`.
  - Use the same mock-tracking pattern already used by neighbouring tests in `cmd/state_daemon_run_test.go` to verify zero commit calls (capture/commit seam; do **not** introduce a new seam).
  - Drive `defaultShutdownFlush` and assert: return value is `nil`; commit was called zero times.
  - Warn-log assertion is optional — if the existing harness has a log-capture seam, assert the warn fires; otherwise omit (spec: "the warn-log is an observability detail, not a correctness invariant").
- **Audit and update `tick()` err-branch coverage** in `cmd/state_daemon_run_test.go`:
  - Pre-implementation sweep step: locate any existing test that injects a `TryGetServerOption` error into `tick()`. The test-code sweep section of the spec notes "the documented-gap comment at lines 557–565 indicates no existing test reaches the err-branch through the public Client surface" — but this refers specifically to `defaultShutdownFlush`. The implementer must confirm whether `tick()`'s err-branch already has its own coverage through the daemon-side seam (`tickDeps` or equivalent).
  - If `tick()`'s err-branch is **already covered** with a bare `errors.New(...)` mock return — update that mock to return `&tmux.CommandError{Stderr: "lost server", Err: errors.New("exit status 1")}`. Otherwise the test exercises a code path that under the new contract would propagate as non-absence (non-`ErrOptionNotFound`), which is still the correct branch but no longer the realistic shape.
  - If `tick()`'s err-branch is **not covered** — add a new test `TestTick_SkipsOnTransportError` with the same fault-injection shape as the flush test: inject the same `*CommandError`, drive `tick`, assert no capture/commit calls are performed.
- All daemon tests **must not** use `t.Parallel()` per CLAUDE.md (the `cmd` package injects mocks via package-level mutable state like `bootstrapDeps`).
- Use the existing mock-tracking and capture/commit seam already in the file — do not introduce new seams.

**Acceptance Criteria**:
- [ ] The comment block at `cmd/state_daemon_run_test.go:557-565` is removed (no trace of "cannot be tested through the public Client surface" remains in the file).
- [ ] A new test (`TestDefaultShutdownFlush_SkipsOnTransportError` or equivalent name) injects a `*tmux.CommandError{Stderr: "lost server", ...}` via the daemon test seam, drives `defaultShutdownFlush`, and asserts the function returns `nil`.
- [ ] The same test asserts via the existing capture/commit mock-tracking pattern that zero commit calls were performed.
- [ ] `tick()`'s err-branch (`cmd/state_daemon.go:95-99`) has coverage: either an existing test is updated to return a `*tmux.CommandError` instead of a bare `errors.New(...)`, or a new test (`TestTick_SkipsOnTransportError` or equivalent) is added with the same fault-injection shape.
- [ ] No new test seams are introduced — the existing daemon `Deps`-style injection and capture/commit mock surfaces are reused.
- [ ] No new test uses `t.Parallel()`.
- [ ] `go test ./cmd/...` passes; pre-existing daemon tests continue to pass.

**Tests** (this task's tests are the deliverable):
- `"TestDefaultShutdownFlush_SkipsOnTransportError/returns_nil"` — under injected `*CommandError{Stderr: "lost server"}`, flush returns `nil`.
- `"TestDefaultShutdownFlush_SkipsOnTransportError/zero_commits"` — same scenario, capture/commit seam shows zero commit calls.
- `"TestDefaultShutdownFlush_SkipsOnTransportError/warn_log_fires"` (optional, if existing harness has a log-capture seam) — warn log is emitted via the structured logger.
- `"TestTick_SkipsOnTransportError"` — under injected `*CommandError`, `tick` performs no capture/commit and returns without escalating.

**Edge Cases**:
- No `t.Parallel()` — cmd-package mock injection via package-level mutable state cannot be parallelised.
- Zero-commit assertion uses the existing capture/commit mock-tracking seam already wired in `cmd/state_daemon_run_test.go` (look for neighbouring tests around the targeted code paths for the canonical pattern).
- Log-capture is optional — spec explicitly says return-value + zero-commit are sufficient for acceptance.
- The audit for `tick()` coverage: the implementer must `grep` `cmd/state_daemon_run_test.go` for existing tests targeting `tick`'s err-branch before deciding "update vs add" — do not blindly add a duplicate.

**Context**:
> Spec "Testing — cmd/state_daemon_run_test.go":
> "Remove the documented-gap comment block at lines 557-565.
> Add the previously-blocked test for defaultShutdownFlush's if err != nil { return nil } branch:
> - Fault injection: use the existing Deps-style seam in cmd/state_daemon.go to inject a tmux-client mock whose TryGetServerOption('@portal-restoring') returns ('', false, &tmux.CommandError{Stderr: 'lost server', Err: errors.New('exit status 1')}).
> - 'Returns nil': assert the function's return value is nil.
> - 'Without committing state': assert via the daemon's existing capture/commit seam — the same mock-tracking pattern already used by neighbouring tests in cmd/state_daemon_run_test.go to verify zero commit calls. (No new seam is introduced by this fix.)
> - 'Warn log is emitted': capture via the same log-capture pattern used by neighbouring tests (the state package's structured logger writes through a test sink already wired in the daemon test harness). If the existing harness has no log-capture seam, asserting return value + zero-commit is sufficient for acceptance — the warn-log is an observability detail, not a correctness invariant."
>
> "Add a test for tick()'s err-handling branch (cmd/state_daemon.go:95-99). The implementer must first confirm whether existing daemon tests already cover this branch through the test seam (the daemon-side tickDeps or equivalent). If covered, replace the existing mock that returned a bare errors.New(...) with one that returns a non-ErrOptionNotFound error (per the sweep in 'Pre-implementation sweep'). If not covered, add a new test asserting the tick logs warn and returns without performing capture under the same fault-injection shape used for the flush test."
>
> Spec Problem section: "A fourth site (cmd/state_daemon_run_test.go:557-565) documents the bug as a known gap..."
>
> CLAUDE.md: "Tests must not use t.Parallel() — the cmd package injects mocks via package-level mutable state (bootstrapDeps, openDeps, attachDeps, etc.) and cleans up with t.Cleanup()."

**Spec Reference**: `.workflows/distinguish-transport-errors-in-getserveroption/specification/distinguish-transport-errors-in-getserveroption/specification.md` — "Testing → cmd/state_daemon_run_test.go"; "Problem & Goal → Problem" (fourth site); "Documentation & Test-Comment Updates → 5".

---

## distinguish-transport-errors-in-getserveroption-1-7 | approved

### Task 1-7: Tighten the four contract-violation docstrings

**Problem**: Four production docstrings document or anticipate the distinguishability contract that the buggy `GetServerOption` could not deliver. After Tasks 1-1 through 1-3 land, the contract is finally faithful, but the docstrings either pre-date the contract drift (and accurately describe behaviour the code now delivers) or contain wording that could be tightened to reference the new discriminator. Without this tightening, future readers see docstrings that "happen to be accurate" rather than docstrings authored against the implementation — a structural fragility the spec calls out explicitly.

**Solution**: Update the four cited docstrings to coherently describe the post-fix contract. Three already describe what the code now does; lightly tighten them to reference `ErrOptionNotFound` explicitly where useful. One (`GetServerOption` itself) currently has a vestigial one-line docstring — replace it with a contract-faithful version that names the discriminator sentinel and the wrapped error type.

**Outcome**: All four docstrings (`TryGetServerOption`, `GetServerOption`, `RestoringChecker`, `IsRestoringSet`) accurately describe the implemented contract — naming `ErrOptionNotFound` as the absence sentinel and noting that other failures surface as wrapped `*CommandError` errors recoverable via `errors.As`. No behavioural changes; documentation only.

**Do**:
- **Site 1 — `internal/tmux/tmux.go:312-316` — `TryGetServerOption` docstring**: existing wording asserts "distinguishing absence from a real tmux failure (which surfaces as a non-nil error)." This is accurate post-fix. Tighten to reference `ErrOptionNotFound` explicitly and clarify that any other error indicates a transport or environmental failure recoverable via `errors.As(err, &cmdErr)`. The function-level contract should read substantively as: "Returns (value, true, nil) when the option exists; ('', false, nil) when tmux reports the option is absent (per ErrOptionNotFound stderr pattern match); ('', false, non-nil-err) for any other failure (transport, executable missing, server crash, etc.). Callers can recover the wrapped *CommandError via errors.As to inspect tmux's stderr."
- **Site 2 — `internal/tmux/tmux.go` `GetServerOption` docstring (currently 1-2 lines at the top of the function at line 302-303)**: replace with a contract-faithful docstring covering:
  - Returns `ErrOptionNotFound` only when tmux's stderr matches the option-absent pattern family (case-sensitive substrings: `invalid option:`, `unknown option:`, `ambiguous option:`).
  - Returns a wrapped `*CommandError` (accessible via `errors.As`) for any other failure mode whose Commander invocation produces one — transport errors, server crashes, executable-not-found, or any stderr that does not match.
  - Callers using `errors.Is(err, ErrOptionNotFound)` continue to work and now correctly identify genuine absence only.
- **Site 3 — `internal/state/markers.go:34-35` — `RestoringChecker` interface docstring**: existing wording "absence vs. real failure is distinguishable." is accurate post-fix. Lightly amend to point at `tmux.ErrOptionNotFound` as the discriminator sentinel so readers know what to check against (one-line addition; no semantic change). Phrase as something like: "...absence vs. real failure is distinguishable (absence is reported as (false, nil); real failures surface as (false, non-nil-err); callers can use errors.Is(err, tmux.ErrOptionNotFound) to identify genuine absence on the underlying check)."
- **Site 4 — `internal/state/markers.go:136-138` — `IsRestoringSet` docstring**: existing wording "Any underlying tmux error is propagated so a real failure does not silently masquerade as 'not restoring'." is accurate post-fix. No change required unless tightening for clarity. The implementer may add a one-line note that the propagated error wraps a `*tmux.CommandError` recoverable via `errors.As` for diagnostic inspection — but this is optional.
- Do not change any function signatures.
- Do not change any function bodies.
- Do not rename any types or functions.

**Acceptance Criteria**:
- [ ] `TryGetServerOption` docstring (`internal/tmux/tmux.go:312-316`) names `ErrOptionNotFound` explicitly and describes the three-case `(value, found, err)` contract.
- [ ] `GetServerOption` docstring (above `internal/tmux/tmux.go:304`) describes the discriminator behaviour — `ErrOptionNotFound` only on stderr pattern match, wrapped `*CommandError` propagated otherwise, `errors.Is` compatibility preserved.
- [ ] `RestoringChecker` interface docstring (`internal/state/markers.go:34-35`) references `tmux.ErrOptionNotFound` as the discriminator sentinel (or is left unchanged if the implementer judges the existing wording sufficient).
- [ ] `IsRestoringSet` docstring (`internal/state/markers.go:136-138`) is coherent with the new contract (existing wording is already accurate; tightening is optional).
- [ ] No function signature, body, or symbol name is modified.
- [ ] `go build ./...` and `go test ./...` continue to pass — this task is doc-only.
- [ ] A `grep` for the previous mis-leading phrasing produces no false positives (e.g., no remaining docstring claims `GetServerOption` "always" returns `ErrOptionNotFound" on error, or similar).

**Tests**:
- No new tests authored. Verification is by inspection (and CI builds confirming nothing is broken). The behavioural tests authored in Tasks 1-4, 1-5, 1-6 collectively confirm the contract the docstrings describe.

**Edge Cases**: none — this task is doc-only and cannot affect runtime behaviour.

**Context**:
> Spec "Documentation & Test-Comment Updates":
> "Four production-code docstring sites currently document or anticipate the distinguishability contract; after the fix, all four must be coherent with the implementation."
>
> "1. internal/tmux/tmux.go:312-316 — TryGetServerOption docstring. Existing wording asserts: 'distinguishing absence from a real tmux failure (which surfaces as a non-nil error).' This becomes accurate once the dependency is fixed. The docstring may be tightened to reference ErrOptionNotFound explicitly and to clarify that any other error indicates a transport or environmental failure."
>
> "2. internal/tmux/tmux.go — GetServerOption docstring. Add or update a docstring describing the new contract:
> - Returns ErrOptionNotFound only when tmux's stderr matches the option-absent pattern family.
> - Returns a wrapped *CommandError (accessible via errors.As) for any other failure.
> - Callers using errors.Is(err, ErrOptionNotFound) continue to work and now correctly identify genuine absence only."
>
> "3. internal/state/markers.go:34-35 — RestoringChecker interface docstring. Existing wording: 'absence vs. real failure is distinguishable.' Becomes accurate. May be lightly amended to point at tmux.ErrOptionNotFound as the discriminator sentinel, but no semantic change is required."
>
> "4. internal/state/markers.go:136-138 — IsRestoringSet docstring. Existing wording: 'Any underlying tmux error is propagated so a real failure does not silently masquerade as not restoring.' Becomes accurate. No change required unless tightening for clarity."
>
> Implementation Ordering: unit (4) "Docstring tightening at items 1–4 of 'Documentation & Test-Comment Updates' — may land alongside (3) or as a follow-up. No behaviour change."

**Spec Reference**: `.workflows/distinguish-transport-errors-in-getserveroption/specification/distinguish-transport-errors-in-getserveroption/specification.md` — "Documentation & Test-Comment Updates" items 1-4; "Implementation Ordering" unit (4).
