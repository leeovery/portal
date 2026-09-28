# Analysis Tasks: lazy-resume-on-attach (Cycle 2)

## Task 1: One Rule Hands a Pane to Its Hook or Its Shell
severity: duplication
sources: duplication

**Problem**: A pane can be handed to its registered hook or its bare shell from five places, and each one writes the rule out itself:
- the eager hydrate tails: `execShellAndExit` (`cmd/state_hydrate.go:185-188`) and the hook branch of `execShellOrHookAndExit` (`:256-261`);
- Enter onto an entry that has since gone: `resumeAnswerEnter` (`cmd/state_resume_wait.go:324-330`);
- the confirmed discard: `resumeAnswerDiscard` (`:360-361`);
- the recovery tail: `runResumeRecover` (`cmd/state_resume_recover.go:58-61`).

The earlier consolidation shared the pieces: `resolveShell`, `hookExecArgs`, `execHandOff`, and the normalised `resumeRegistrationOrLog`. It did not share the rule that combines them: no command means exec `resolveShell()` alone with `hook_present=false`, and a command means `hookExecArgs` with `hook_present=true`. `resumeAnswerEnter` re-implements that choice. `resumeAnswerDiscard` and `runResumeRecover` each restate the body of `execShellAndExit`, because that function takes a `hydrateConfig` and neither of them has one.

The specification requires parity here. A pane whose entry has gone drops to a plain shell "exactly as an unregistered pane does — a path the helper already has". A discarded pane is "indistinguishable from a pane that never had a hook". Each route's suite pins only its own argv: `cmd/state_resume_enter_test.go:39-40` and `cmd/state_resume_recover_test.go:219-224` assert `[]string{shell}` independently of the hydrate tests. So a change to how the eager path hands a pane its shell or its hook fails no test. Afterwards, a pane answered on the panel or recovered by the tail silently gets a different shell invocation from one that never waited.

**Solution**: Add one `handOffToHookOrShell(logger *slog.Logger, exec func(prog string, args []string), command string)` in `cmd/state_resume_chain.go`, beside `hookExecArgs` and `execHandOff`. An empty command execs `resolveShell()` alone with `hook_present=false`. Any other command goes through `hookExecArgs(command, resolveShell())` with `hook_present=true`. Route all five sites through it:
- `execShellAndExit` becomes a call with the empty command;
- `execShellOrHookAndExit` hands over `lookup.Command`;
- `resumeAnswerEnter` hands over the command it reads at answer time;
- `resumeAnswerDiscard` and `runResumeRecover` pass the empty command.

A command string alone is enough to carry the whole decision. `resumeRegistrationOrLog` already guarantees that `Found` implies a non-empty `Command`, which is the settled phase-4 direction.

The helper sits on top of `execHandOff` and does not replace it. The parked chain's hand-off (`execResumeChainAndExit`, `:283`) stays on `execHandOff` directly, and the source guard's two exec-calling functions are unchanged. The tty steps before each hand-off stay where they are. Enter and the discard re-enable signal generation only, while the recovery tail restores a fully cooked tty; both are settled directions (c1 task 1, p7 task 1). So those steps are not part of the shared rule. This is a behaviour-preserving refactor: every site's target, argv and `hook_present` attr are unchanged, and the existing hydrate, Enter, discard and recover suites prove it.

**Outcome**: A pane handed to its hook or its shell goes through one declaration of how that is done, whichever route reached it. A change to that form reaches the eager path and the three lazy routes together.
