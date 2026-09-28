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

**Outcome**: A pane handed to its hook or its shell goes through one declaration of how that is done, whichever route reached it. A change to that form reaches the eager path and the three lazy routes together. Every route still hands the pane the same target, argv and `hook_present` marker it does today, and a waiting pane still parks on the same chain.

**Acceptance Criteria**:
- [ ] A restored eager pane with no registration, whether it ends on the replay, the signal timeout or the missing scrollback file, is handed `$SHELL` alone as `[$SHELL]`, and its exec marker carries `hook_present=false`, exactly as before
- [ ] A restored eager pane whose registration holds a command is handed `/bin/sh` with `sh -c '<command>; exec $SHELL'`, and its exec marker carries `hook_present=true`, exactly as before
- [ ] Enter on a waiting pane runs the command the store holds at the moment of the answer in that same argv with `hook_present=true`; an entry that has gone, a registration carrying no command, or an unreadable store instead gives the pane the bare-shell hand-off an unregistered eager pane gets
- [ ] A confirmed discard, and the recovery tail on a pane still marked pending, each hand the pane the bare-shell hand-off with `hook_present=false`
- [ ] With `SHELL` unset, every hand-off above resolves to `/bin/sh`
- [ ] A lazy pane still parks on `/bin/sh` `sh -c '<parked chain>'` with `hook_present=true`, and each route's terminal steps keep their order: Enter and the confirmed discard restore raw mode and re-enable signal generation before the exec, and the recovery tail gives back its cooked terminal before the exec
- [ ] The existing hydrate, Enter, discard, recover, signal-generation and exec-seam guard suites pass with what they assert unchanged

**Do**:
- Add `handOffToHookOrShell(logger *slog.Logger, exec func(prog string, args []string), command string)` to `cmd/state_resume_chain.go`, beside `hookExecArgs` and `execHandOff`. It hands off through `execHandOff` and never calls the exec seam itself, so `TestExecSeamsAreCalledOnlyByTheHandOffHelpers` (`cmd/exec_handoff_guard_test.go`) keeps its permitted set of `resumeHandOff` and `execHandOff` unchanged.
- Convert the complete set. `rg -n 'execHandOff\(' cmd --glob '!*_test.go'` returns 7 lines: the definition (`cmd/state_resume_chain.go:132`) and six call sites. Five are hook-or-shell hand-offs, and all five route through the helper:
  - `cmd/state_hydrate.go:187` — `execShellAndExit`, a call with the empty command;
  - `cmd/state_hydrate.go:261` — the tail of `execShellOrHookAndExit`, which hands over `lookup.Command` in place of its `!lookup.Found` branch at `:256-259`;
  - `cmd/state_resume_wait.go:330` — `resumeAnswerEnter`, handing over the command it reads at `:320`;
  - `cmd/state_resume_wait.go:361` — `resumeAnswerDiscard`, the empty command;
  - `cmd/state_resume_recover.go:61` — `runResumeRecover`, the empty command.
- The sixth call site, `cmd/state_hydrate.go:283` in `execResumeChainAndExit`, stays on `execHandOff` directly.
- `rg -n 'resolveShell\(\)' cmd --glob '!*_test.go'` returns 6 lines: the definition (`cmd/state_hydrate.go:190`) and the five sites' own resolutions (`cmd/state_hydrate.go:186`, `:260`; `cmd/state_resume_wait.go:324`, `:360`; `cmd/state_resume_recover.go:58`). After the change, production calls `resolveShell()` and `hookExecArgs(` from `handOffToHookOrShell` alone.
- Leave the steps before each hand-off where they are. In `resumeAnswerEnter`, the registration read, `cfg.restore()` and `enableTTYSignalsOrLog` stay ahead of the call. In `resumeAnswerDiscard`, the restore and the enable stay ahead of it. In `runResumeRecover`, the marker clear and `enableTTYSignalsOrLog` stay ahead of it.
- Leave the waiter's separate restore-and-enable pairs unmerged, and leave `parkedChainBackstop`'s `${SHELL:-/bin/sh}` (`cmd/state_hydrate.go:306`) as it is.
