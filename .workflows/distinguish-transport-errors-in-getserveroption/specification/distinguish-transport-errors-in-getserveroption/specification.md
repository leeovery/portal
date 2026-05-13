# Specification: Distinguish Transport Errors in GetServerOption

## Specification

## Problem & Goal

### Problem

`Client.GetServerOption` (`internal/tmux/tmux.go:304-310`) collapses every error from `cmd.Run("show-option", "-sv", name)` into the sentinel `ErrOptionNotFound`. The underlying tmux error and its stderr text are discarded.

The wrapper `Client.TryGetServerOption` (`internal/tmux/tmux.go:317-326`) therefore cannot distinguish "option absent" from "tmux call failed for any other reason." Its `if err != nil { return "", false, err }` branch is unreachable, and its `(value, found, err)` contract reports every transport failure as `("", false, nil)` — i.e. "option absent, no error."

This violates three documented contracts:

1. `internal/tmux/tmux.go:312-316` — `TryGetServerOption` docstring claims it distinguishes absence from real failure.
2. `internal/state/markers.go:34-35` — `RestoringChecker` interface docstring asserts absence is distinguishable from real failure.
3. `internal/state/markers.go:136-138` — `IsRestoringSet` docstring promises real tmux errors are propagated.

A fourth site (`cmd/state_daemon_run_test.go:557-565`) documents the bug as a known gap: a test for the "conservatively skip the final flush on read error" branch in `defaultShutdownFlush` cannot be written through the public `Client` surface today because of the conflation.

The bug is latent — no user-visible incident has been reported. The two production consumers (`cmd/state_daemon.go` `tick()` at L95-99 and `defaultShutdownFlush()` at L187-201) read `@portal-restoring` defensively and already want conservative-on-error behaviour ("skip the tick / skip the flush"). The conflation silently flips them from conservative-on-error to permissive-on-error in the presence of any transient tmux failure during the restoration window.

### Goal

`GetServerOption` and `TryGetServerOption` deliver on their documented contracts:

- **`GetServerOption(name)` → `(value, error)`**: returns `ErrOptionNotFound` only when tmux reports the option is genuinely absent; returns a non-nil, non-sentinel error for any other failure (transport, server crash, executable missing, etc.).
- **`TryGetServerOption(name)` → `(value, found, err)`**:
  - Option present → `(value, true, nil)`
  - Option absent → `("", false, nil)`
  - Any other failure → `("", false, non-nil-err)`
- The two daemon read sites (`tick()`, `defaultShutdownFlush()`) start receiving the errors they were always written to handle. Their consumer code is unchanged.
- The three docstrings cited above accurately describe what the code delivers.

---

## Design: `CommandError` at the Commander Layer

### Type

Introduce a typed error in `internal/tmux` that carries tmux's stderr alongside the underlying error so callers can discriminate failure modes without coupling to `os/exec`:

```go
// CommandError wraps an error returned by Commander.Run / Commander.RunRaw and
// carries the captured stderr from the underlying process. Stderr is empty when
// the failure was not an *exec.ExitError (e.g., executable not found).
type CommandError struct {
    Stderr string
    Err    error
}

func (e *CommandError) Error() string { /* "<Err>: <Stderr>" when Stderr non-empty, else <Err>.Error() */ }
func (e *CommandError) Unwrap() error { return e.Err }
```

**Placement and exported-ness:** package-level exported type in `internal/tmux`. Exported so test code outside the package (and any future package-level helpers) can construct `*CommandError` literals as mock returns. The constructor is a plain struct literal — no `NewCommandError` factory.

### Wiring at `RealCommander`

Both `RealCommander.Run` and `RealCommander.RunRaw` (`internal/tmux/tmux.go:39-46`) wrap their non-nil errors before returning:

- If the error is `*exec.ExitError`, populate `Stderr` from `(*exec.ExitError).Stderr` (already captured automatically by `cmd.Output()`).
- If the error is any other type (e.g., `exec.Command(...)` failed to find the binary), wrap with `Stderr: ""`. An empty `Stderr` means "no signal" — discriminators that examine `Stderr` will see no pattern match and treat the error as non-absence, which is the correct conservative behaviour.

In both cases the original error is preserved via `Unwrap()` so existing `errors.Is` / `errors.As` checks against sentinel errors continue to work.

### Mock surface

The `Commander` interface signature is unchanged: `Run(args ...string) (string, error)` / `RunRaw(args ...string) (string, error)`. Mocks that previously returned a bare `error` continue to compile.

Mocks that need to simulate a specific stderr return a `*CommandError` literal:

```go
mockCmd.RunFn = func(args ...string) (string, error) {
    return "", &tmux.CommandError{Stderr: "invalid option: @foo", Err: errors.New("exit status 1")}
}
```

Tests that don't care about stderr continue to return `errors.New(...)` — discriminators will see an empty `Stderr` and behave conservatively.

---

## Design: Discrimination in `GetServerOption`

### Behaviour

`Client.GetServerOption(name string) (string, error)` extracts the wrapped `*CommandError` from any non-nil error, inspects its `Stderr`, and returns:

- `(strings.TrimSpace(output), nil)` on success (no error).
- `("", ErrOptionNotFound)` when the error unwraps to a `*CommandError` whose `Stderr` matches the option-absent pattern family (see below).
- `("", err)` — the original wrapped error — for every other failure (transport, executable-missing, server crash, unmatched stderr).

Extraction uses `errors.As(err, &cmdErr)` so a future error-wrapping change at the `Commander` layer does not break the discriminator.

### Option-absent pattern family

Tmux signals option absence via stderr containing one of these substrings:

- `invalid option:` — unknown `@`-prefixed user option or unknown non-prefixed option name.
- `unknown option:` — alternate form emitted by some tmux versions.
- `ambiguous option:` — option name is an ambiguous prefix match (including the empty string case observed during investigation).

The pattern set is exported as a small, package-level slice in `internal/tmux` (not inlined in `GetServerOption`) so that:

- The discrimination set is reviewable and testable in isolation.
- Future tmux versions or platforms that emit additional absence phrasings can be added in one place.

Matching is **case-sensitive substring** against `cmdErr.Stderr`. Tmux's source uses these literals consistently across versions in the project's compatibility window; no normalisation (lowercasing, regex) is required.

### Failure modes that do NOT match the absent family

- "error connecting to /private/tmp/tmux-501//<sock> (No such file or directory)" — socket/transport.
- Empty `Stderr` (non-`ExitError` wrapping, e.g., binary not found).
- Any other stderr the future yields.

These propagate as the wrapped `*CommandError` to the caller. They are not `ErrOptionNotFound`.

---

## Design: `TryGetServerOption` and Consumer Surface

### `TryGetServerOption` body — unchanged

```go
func (c *Client) TryGetServerOption(name string) (string, bool, error) {
    val, err := c.GetServerOption(name)
    if errors.Is(err, ErrOptionNotFound) {
        return "", false, nil
    }
    if err != nil {
        return "", false, err
    }
    return val, true, nil
}
```

No code change is required in this function. Its behaviour changes because its dependency (`GetServerOption`) is now contract-faithful:

- The `errors.Is(err, ErrOptionNotFound)` branch is now exclusive to genuine absence.
- The `if err != nil { return "", false, err }` branch — previously unreachable — becomes the live transport-error path.

### Consumer surface — unchanged

- `internal/state/markers.go:140` — `IsRestoringSet` continues to propagate errors via the existing `_, found, err := c.TryGetServerOption("@portal-restoring")` shape. No code change.
- `cmd/state_daemon.go:95-99` — `tick()` already does `if err != nil { log.Warn(...); return }`. The warn log and early return start firing for transport errors instead of being silently dead.
- `cmd/state_daemon.go:187-201` — `defaultShutdownFlush()` already does `if err != nil { log.Warn(...); return nil }`. Same — the branch becomes live.

No daemon consumer code changes. The behavioural shift is "transport errors now flow into branches the consumers already wrote."

### `ErrOptionNotFound` — unchanged

`ErrOptionNotFound` remains a `var` sentinel in `internal/tmux`. Its meaning narrows from "any error from `GetServerOption`" to "tmux reports the option is genuinely absent." External code using `errors.Is(err, tmux.ErrOptionNotFound)` continues to work — under the new contract those checks now correctly mean "the option does not exist."

---

## Documentation Updates

Four sites currently document or anticipate the distinguishability contract. After the fix, all four must be coherent with the implementation.

### 1. `internal/tmux/tmux.go:312-316` — `TryGetServerOption` docstring

Existing wording asserts: *"distinguishing absence from a real tmux failure (which surfaces as a non-nil error)."* This becomes accurate once the dependency is fixed. The docstring may be tightened to reference `ErrOptionNotFound` explicitly and to clarify that any other error indicates a transport or environmental failure.

### 2. `internal/tmux/tmux.go` — `GetServerOption` docstring

Add or update a docstring describing the new contract:

- Returns `ErrOptionNotFound` only when tmux's stderr matches the option-absent pattern family.
- Returns a wrapped `*CommandError` (accessible via `errors.As`) for any other failure.
- Callers using `errors.Is(err, ErrOptionNotFound)` continue to work and now correctly identify genuine absence only.

### 3. `internal/state/markers.go:34-35` — `RestoringChecker` interface docstring

Existing wording: *"absence vs. real failure is distinguishable."* Becomes accurate. May be lightly amended to point at `tmux.ErrOptionNotFound` as the discriminator sentinel, but no semantic change is required.

### 4. `internal/state/markers.go:136-138` — `IsRestoringSet` docstring

Existing wording: *"Any underlying tmux error is propagated so a real failure does not silently masquerade as 'not restoring'."* Becomes accurate. No change required unless tightening for clarity.

### 5. `cmd/state_daemon_run_test.go:557-565` — documented-gap comment block

The comment block explains that a test for `defaultShutdownFlush`'s err-branch cannot be written through the public `Client` surface today. After the fix:

- The comment block is **removed**.
- It is **replaced** by the actual test (see "Testing" section).

---

## Testing

### `internal/tmux/tmux_test.go`

- **Reshape existing `TestGetServerOption` "option does not exist" case** (currently uses `errors.New("unknown option: @portal-active-%3")`): the mock must now return a `*CommandError` whose `Stderr` matches the option-absent pattern family. The test asserts `errors.Is(err, ErrOptionNotFound)` continues to hold.
- **Add `TestGetServerOption_TransportError`**: mock returns a `*CommandError` with stderr that does NOT match the absent family (e.g., `"error connecting to /tmp/tmux-501//default (No such file or directory)"`). Assert `!errors.Is(err, ErrOptionNotFound)` and that the returned error unwraps to a `*CommandError` carrying the original stderr.
- **Add `TestGetServerOption_NonExitErrorPropagates`**: mock returns a `*CommandError{Stderr: "", Err: errors.New("exec: \"tmux\": not found")}`. Assert `!errors.Is(err, ErrOptionNotFound)` and that the error propagates.
- **Add `TestTryGetServerOption_PropagatesTransportError`**: covers the previously-unreachable `if err != nil { return "", false, err }` branch. Assert `(value, found, err) == ("", false, non-nil)` with `errors.As` recovering the `*CommandError`.
- **Add discriminator-set unit tests**: each entry in the option-absent pattern slice is exercised against a synthetic stderr containing it, asserting `ErrOptionNotFound` is returned. A negative case asserts an unrelated stderr does not match.

### `cmd/state_daemon_run_test.go`

- **Remove the documented-gap comment block at lines 557-565.**
- **Add the previously-blocked test** for `defaultShutdownFlush`'s `if err != nil { return nil }` branch: mock the tmux client so the `@portal-restoring` read returns a non-`ErrOptionNotFound` error; assert the flush returns nil without committing state, and that the warn log is emitted.
- **Add a parallel test for `tick()`'s err-handling branch** (`cmd/state_daemon.go:95-99`) if not already covered: same shape — non-`ErrOptionNotFound` error on `@portal-restoring`, assert the tick logs warn and returns without performing capture.

### `internal/state/markers_test.go`

- **`TestIsRestoringSet :: tmux exploded` (existing, line 206) continues to pass.** No code change required — it asserts the contract the production code is finally able to deliver on. The fix vindicates the test rather than the test driving the fix.

### `internal/tmux` — `Commander` layer

- **`TestRealCommander_RunWrapsExitError`** (new): use a real `os/exec` failure (e.g., invoke a command that returns a known stderr and exit 1) and assert the returned error is `*CommandError` with `Stderr` populated.
- **`TestRealCommander_RunWrapsNonExitError`** (new): invoke a missing binary; assert the returned error is `*CommandError` with `Stderr == ""` and `Unwrap()` returning the original (non-`*exec.ExitError`) error.

### Test policy reminders

- Per `CLAUDE.md`: tests in `cmd` and any package using `*Deps` injection **must not** use `t.Parallel()`. The new daemon tests inherit this constraint.
- No new mock framework — existing `Commander` mock surface is sufficient with `*CommandError` literals.

---

## Scope

### In scope

- New `CommandError` type in `internal/tmux` (exported, struct-literal constructable).
- `RealCommander.Run` / `RealCommander.RunRaw` wrap their non-nil errors with `*CommandError`.
- `GetServerOption` discriminates via `errors.As` + stderr pattern-family substring match.
- `TryGetServerOption` body unchanged; dead branch becomes live.
- Docstring tightening at the four cited sites.
- Test reshapes and additions per the Testing section.
- Removal of the documented-gap comment block in `cmd/state_daemon_run_test.go`.

### Out of scope

- Changes to the `Commander` interface signature. `Run` and `RunRaw` keep `(string, error)`.
- Changes to any other `internal/tmux` method (`ShowAllServerOptions`, `SetServerOption`, etc.). Their error-propagation paths are already correct per the wide-scope audit.
- Changes to daemon consumer logic in `cmd/state_daemon.go`. The fix surfaces errors into branches the consumers already wrote correctly.
- Migration of other callers to the `*CommandError` discriminator. No other production caller of `internal/tmux` currently distinguishes failure modes; this fix does not introduce new discrimination sites.
- Re-architecting `ErrOptionNotFound` (e.g., turning it into a struct error type). It remains a sentinel `var`.

### Non-goals

- Producing observable user-facing behaviour changes in the happy path. Under normal operation (no transient tmux failures), behaviour is identical to today.
- Adding telemetry or metrics for transport errors. The existing `log.Warn` calls in daemon consumers are the only observable signal and they already exist.

## Acceptance Criteria

1. `GetServerOption("@some-marker")` returns `("", ErrOptionNotFound)` if and only if the underlying tmux call's stderr contains a substring from the option-absent pattern family (`invalid option:`, `unknown option:`, `ambiguous option:`).
2. `GetServerOption("@some-marker")` returns a non-nil, non-`ErrOptionNotFound` error wrapping `*CommandError` for any other failure mode, including transport errors, executable-not-found, and stderr that does not match the absent family.
3. `TryGetServerOption("@some-marker")` returns `("", false, non-nil-err)` for any non-absent failure — exercising the `if err != nil { return "", false, err }` branch that was previously unreachable.
4. `IsRestoringSet`'s daemon callers (`tick()`, `defaultShutdownFlush()`) receive non-nil errors for transport failures and skip their action conservatively (warn log + early return).
5. All existing tests pass without behavioural change in the happy path.
6. New tests assert each transport-error and pattern-discriminator scenario in the Testing section.
7. The four contract-violation docstrings now accurately describe the implementation.
8. The documented-gap comment block at `cmd/state_daemon_run_test.go:557-565` is removed and replaced with the previously-blocked test.

## Risk & Rollout

- **Regression risk:** Low. The behavioural change is "transport errors now propagate into branches consumers already wrote." No new failure mode is introduced — only previously-suppressed failure modes are surfaced into existing handling.
- **Rollout:** Regular release. No incident pressure, no hotfix.
- **Compatibility:** No external API changes. The `Commander` interface signature is unchanged; consumers of `tmux.ErrOptionNotFound` continue to work with semantics that now match their intent.

---

## Alternatives Considered

The investigation evaluated four other shapes before settling on typed `CommandError` at the Commander layer. Each is recorded here so future readers can audit the trade-off:

- **B. `fmt.Errorf("%w: %s", err, stderr)` wrap at the Commander layer.** Rejected: the discriminator becomes a substring-match against a formatted error string. Brittle if the wrap format ever changes; harder to test the boundary cleanly.
- **C. New `Commander.RunWithStderr(args...) (out, stderr string, err error)` method.** Rejected: a parallel API surface forces every mock to stub a second method, and the addition does not protect against similar latent conflations elsewhere.
- **D. Inline type-assert against `*exec.ExitError` inside `GetServerOption`.** Rejected: couples `internal/tmux` to `os/exec` semantics at the public discriminator site, and `Commander` mocks would need to construct synthetic `*exec.ExitError` instances (their fields are partly outside our zone of control).
- **E. String-match against `err.Error()` (no wrap) at the `GetServerOption` layer.** Rejected on examination: `(*exec.ExitError).Error()` returns `"exit status 1"` — the stderr is on `.Stderr`, not in the error string. Mechanically broken without first wrapping the error.

The chosen approach (typed `CommandError`) puts the diagnostic shape in the type system where the existing docstrings already pretend it lives; mocks remain trivial struct-literal constructions; future discriminators get the same channel without API drift; and daemon consumer code is untouched.

---

## Working Notes
