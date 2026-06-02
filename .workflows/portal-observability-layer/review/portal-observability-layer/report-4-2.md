TASK: Embed exit code + tmux argv + trimmed stderr in RealCommander.Run/RunRaw and verify sentinel detection (portal-observability-layer-4-2)

ACCEPTANCE CRITERIA:
1. Non-zero tmux exit returns *CommandError whose Error() contains argv, exit code, trimmed stderr.
2. cmd.Stderr still nil in runCommand; (*exec.ExitError).Stderr still auto-populates → CommandError.Stderr.
3. errors.As recovers *CommandError (with Stderr + new argv).
4. errors.Is(ErrNoSuchSession) still succeeds through wrapNoSuchSession's multi-%w; errors.As still succeeds on same value.
5. errors.Is(ErrEmptyPaneList) still succeeds (path unperturbed).
6. PATH-lookup *exec.Error → *CommandError carrying argv, empty stderr, no exit N fragment.
7. RunRaw success output verbatim (untrimmed) unchanged.
8. *CommandError remains constructable as plain struct literal.

STATUS: Complete

SPEC CONTEXT:
Spec § Diagnostic context preservation — Boundary class 2 (RealCommander.Run/RunRaw). Embed exit code + argv + trimmed stderr; sentinels detectable. Load-bearing: cmd.Stderr must stay nil so (*exec.ExitError).Stderr auto-populates.

IMPLEMENTATION:
- Status: Implemented
- Location: command_error.go:18-33 (CommandError + Args []string nil-default, godoc); :54-89 (Error() dispatches renderWithArgs when Args non-empty: tmux <Args>[: exit N][: stderr], exit fragment gated behind errors.As *exec.ExitError so PATH-lookup omits it; empty Args → legacy render for byte parity); :125-135 (WrapCommandError variadic); tmux.go:79-93 (runCommand passes args, cmd.Stderr unassigned); tmuxtest/socket.go + transienttest/socket.go updated.
- Notes: Preferred "extend *CommandError" shape (argv via errors.As). wrapNoSuchSession + saverPanePID untouched (argv touches neither Stderr nor Unwrap). ErrEmptyPaneList fires on tmux-success-empty-stdout branch (never through commander wrap).

TESTS:
- Status: Adequate
- Coverage: realcommander_test.go (RunWrapsExitError real sh child, both Run/RunRaw, Stderr+Args+rendered argv+exit 1+marker+ExitError unwrap; ArgvChainRemainsRecoverable errors.Is+errors.As; RunRawVerbatimOnSuccess; RunWrapsNonExitError empty Stderr non-ExitError); command_error_test.go (variadic populate, nil-with-args, no-args-nil; ErrorRendering argv+exit+stderr / PATH-lookup no exit / spaces+quotes / empty-args legacy / empty-stderr no dangling sep); saver_pane_pid_test.go (ErrEmptyPaneList empty + whitespace); errors_test.go (ShowEnvironment ErrNoSuchSession + errors.As).
- Notes: Every AC + named test present. Drives real child processes (honest auto-population). Not over-tested.

CODE QUALITY:
- Project conventions: Followed (no t.Parallel; godoc; errors.As/Is; single-source-of-truth wrap across prod + both test commanders).
- SOLID: Good — renderWithArgs extracted; Error() open/closed dispatch preserves legacy branch.
- Complexity: Low.
- Modern idioms: Yes (variadic args, strings.Builder, multi-%w).
- Readability: Good — godoc documents nil-Args fallback + exit-fragment gating.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] renderWithArgs hardcodes "tmux " prefix while runCommand holds the binary param; correct for all prod callers (binary always tmux) but renders "tmux" even when tests invoke sh/printf. Cosmetic in test output.
- [idea] command_error.go hosts type + WrapCommandError + renderWithArgs (pre-existing observation); fine architecturally.
