STATUS: findings
FINDINGS_COUNT: 2
SUMMARY: Two architectural concerns — duplicated CommandError wrap recipe between RealCommander and tmuxtest, and a daemon fake commander that bypasses the new discriminator path.

---

AGENT: architecture
FINDINGS:

- FINDING: CommandError wrapping logic duplicated between production and tmuxtest
  SEVERITY: medium
  FILES: /Users/leeovery/Code/portal/internal/tmux/tmux.go:82-89, /Users/leeovery/Code/portal/internal/tmuxtest/socket.go:128-143
  DESCRIPTION: The five-line wrap recipe (`if errors.As(err, &exitErr) { stderr = string(exitErr.Stderr) }; return &CommandError{Stderr: stderr, Err: err}`) is implemented twice — once in `runCommand` (production) and once in `tmuxtest.wrapErr`. Both sites carry near-identical docstrings asserting the shape must match. The spec called out that `(*exec.ExitError).Stderr` auto-population is contingent on `cmd.Stderr` being nil and that "future changes that assign cmd.Stderr would silently break the wrapping" — that invariant is now stated in two places that must move together. If production ever switches to `cmd.StderrPipe()`, tmuxtest will silently keep using the old shape and integration tests against real tmux will see different error structure than production, defeating the discriminator-test parity the comments in tmuxtest claim to provide.
  RECOMMENDATION: Extract the wrap into an exported helper in `internal/tmux` (e.g., `func WrapCommandError(err error) error` returning nil when err is nil, otherwise a `*CommandError` populated from `*exec.ExitError`). `runCommand` and `tmuxtest.socketCommander` both call it. The algorithm lives in one place; the "cmd.Stderr nil" invariant is documented on the helper; tmuxtest's mirror comment collapses to a single-line "uses tmux.WrapCommandError for production parity." Pure refactor — no behavioural change.

- FINDING: daemonFakeCommander returns bare ErrOptionNotFound instead of wrapped *CommandError
  SEVERITY: low
  FILES: /Users/leeovery/Code/portal/cmd/state_daemon_run_test.go:95
  DESCRIPTION: The fake's "option not in map" branch returns `("", tmux.ErrOptionNotFound)` directly. Production `RealCommander` never surfaces a bare sentinel through the Commander layer — it always returns a `*CommandError` whose Stderr matches an absence pattern, and the discriminator inside `GetServerOption` maps it to `ErrOptionNotFound`. This works today only because `GetServerOption`'s fallthrough (`errors.As` returns false → return original err) happens to propagate the sentinel intact and `TryGetServerOption` uses `errors.Is`. The fake bypasses the discriminator entirely. A future test asserting "the error from a missing-option path unwraps to `*CommandError` with absence-pattern stderr" would pass against production but fail against the fake without any obvious indication why. The fake-vs-real divergence is the class of issue this work unit was created to eliminate at the production layer.
  RECOMMENDATION: Change the dispatch default for `show-option` from returning the bare sentinel to returning a `*CommandError` with an absence-pattern stderr — e.g., `return "", &tmux.CommandError{Stderr: "unknown option: " + args[2], Err: errors.New("exit status 1")}`. Existing test behaviour is unchanged (`TryGetServerOption` still returns `found=false / nil err`) but the fake now exercises the full production discriminator path.

SUMMARY: Two architectural concerns — a duplicated wrap recipe between `RealCommander` and the `tmuxtest` `socketCommander` that the spec already flagged as invariant-fragile, and a daemon fake commander that bypasses the new discriminator instead of exercising it. The core `CommandError` + discriminator design is sound; both findings are cleanup-grade.
