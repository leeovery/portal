STATUS: findings
FINDINGS_COUNT: 3
SUMMARY: One high-severity wrap-logic duplication between production and test commander; two low-severity items (structurally-forced mock split, repeated fault-injection literal).

---

AGENT: duplication

FINDINGS:

- FINDING: CommandError wrap logic copy-pasted from production runCommand into tmuxtest socketCommander
  SEVERITY: high
  FILES: /Users/leeovery/Code/portal/internal/tmux/tmux.go:82-89, /Users/leeovery/Code/portal/internal/tmuxtest/socket.go:128-143
  DESCRIPTION: `runCommand` in production and `wrapErr` in the test harness implement byte-identical error-wrapping: `errors.As(err, &exitErr)` -> populate `Stderr` from `(*exec.ExitError).Stderr` -> return `&CommandError{Stderr, Err}`. The comment in `socket.go` explicitly says it "mirrors" production at `internal/tmux/tmux.go`. This is exactly the drift-prone copy-paste pattern the spec's "Invariant" note in `runCommand` warns about — a future change to wrapping (e.g. capturing stderr via `cmd.StderrPipe()` if `cmd.Stderr` is ever assigned, or adding a third failure-mode branch) will silently diverge between production and the integration-test commander, breaking integration tests' fidelity to production discrimination semantics. Both callsites share the same `cmd.Output()` + nil-stderr precondition but have no shared linkage forcing lockstep.
  RECOMMENDATION: Extract the wrap as an exported helper in `internal/tmux` — e.g. `func WrapCommandErr(err error) error` colocated with `CommandError` in `internal/tmux/command_error.go`. Both `runCommand` and `socketCommander.Run`/`RunRaw` then call it. The `cmd.Stderr == nil` precondition lives in one docstring; the wrap shape changes in one place; integration tests provably exercise the same discrimination path production does.

- FINDING: Duplicated Commander mock shapes across same-package and external-package test files
  SEVERITY: low
  FILES: /Users/leeovery/Code/portal/internal/tmux/option_discriminator_internal_test.go:14-25, /Users/leeovery/Code/portal/internal/tmux/tmux_test.go:12-42
  DESCRIPTION: `internalMockCommander` (in `package tmux`, same-package white-box file) and `MockCommander` (in `package tmux_test`, external black-box file) are independent Commander implementations. The internal one has Output/Err only; the external one adds Calls/RunFunc/RunRawFunc. The internal file's own comment acknowledges the duplication is deliberate: "The external package's MockCommander is not reachable from here without an import cycle." This is structurally forced by Go's same-package-vs-external-package test split; the internal shim is minimal (12 lines, two methods returning the same fields).
  RECOMMENDATION: No action. The duplication is a feature of the test-package boundary, not drift; the comment already documents why.

- FINDING: Transport-error fault-injection literal repeated across two new daemon tests
  SEVERITY: low
  FILES: /Users/leeovery/Code/portal/cmd/state_daemon_run_test.go:567-571, /Users/leeovery/Code/portal/cmd/state_daemon_run_test.go:605-609
  DESCRIPTION: `TestDefaultShutdownFlush_SkipsOnTransportError` and `TestTick_SkipsOnTransportError` both build an identical `&tmux.CommandError{Stderr: "lost server", Err: errors.New("exit status 1")}` and seed it on `daemonFakeCommander.optionErr`. Same concept — "transport failure that must not collapse to absence" — duplicated in two test bodies a few lines apart.
  RECOMMENDATION: Extract a small same-file helper (e.g. `transportErrCommandError()` returning the `*tmux.CommandError`). Three lines saved per callsite and a single source for the canonical "non-absent transport failure" shape, so future tests inherit it without re-typing the literal.

SUMMARY: One high-severity duplication — the `*CommandError` wrap-from-exec-output logic is implemented twice (production `runCommand` and test `socketCommander.wrapErr`) and will silently drift. Two low-severity items — a structurally-forced Commander-mock duplication that should be left alone, and a transport-error fault-injection literal repeated in two adjacent daemon tests.
