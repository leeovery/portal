TASK: lazy-resume-on-attach-8-1 (tick-430df6) — One Rule Hands a Pane to Its Hook or Its Shell

ACCEPTANCE CRITERIA:
- A restored eager pane with no registration, whether it ends on the replay, the signal timeout or the missing scrollback file, is handed `$SHELL` alone as `[$SHELL]`, and its exec marker carries `hook_present=false`, exactly as before
- A restored eager pane whose registration holds a command is handed `/bin/sh` with `sh -c '<command>; exec $SHELL'`, and its exec marker carries `hook_present=true`, exactly as before
- Enter on a waiting pane runs the command the store holds at the moment of the answer in that same argv with `hook_present=true`; an entry that has gone, a registration carrying no command, or an unreadable store instead gives the pane the bare-shell hand-off an unregistered eager pane gets
- A confirmed discard, and the recovery tail on a pane still marked pending, each hand the pane the bare-shell hand-off with `hook_present=false`
- With `SHELL` unset, every hand-off above resolves to `/bin/sh`
- A lazy pane still parks on `/bin/sh` `sh -c '<parked chain>'` with `hook_present=true`, and each route's terminal steps keep their order: Enter and the confirmed discard restore raw mode and re-enable signal generation before the exec, and the recovery tail gives back its cooked terminal before the exec
- The existing hydrate, Enter, discard, recover, signal-generation and exec-seam guard suites pass with what they assert unchanged

STATUS: complete

SPEC CONTEXT: The specification requires that a pane whose entry has gone by the time Enter is pressed drops to a plain shell "exactly as an unregistered pane does — a path the helper already has", and an unreadable store does the same (section 6.1). A confirmed discard leaves the pane "indistinguishable from a pane that never had a hook" (section 6.2). The recovery tail execs the user's shell after it leaves the panel's screen and clears the marker (sections 4.2/4.3). So every lazy route has to give the same shell/hook invocation the eager helper gives. Before this task each route wrote that invocation out separately.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/state_resume_chain.go:137-148 — `handOffToHookOrShell(logger, exec, command)`. It sits beside `execHandOff` (:132-135) and `hookExecArgs` (:171-173). An empty command goes through `execHandOff(logger, exec, shell, []string{shell}, false)`. Any other command goes through `hookExecArgs(command, shell)` with `hook_present=true`. It never calls the seam itself.
  - cmd/state_hydrate.go:239-252 — `execShellOrHookAndExit` passes `lookup.Command` (:251). The `!lookup.Found` branch and `execShellAndExit` are gone. That is equivalent: `lookup` always comes through `resumeRegistrationOrLog` (cmd/state_resume_chain.go:152-165), either directly via `lookupOnResumeOrLog` or via `resolveResumeDecision` (:196-200), and that function answers the zero value unless Found holds and the command is non-empty.
  - cmd/state_resume_wait.go:315-326 — `resumeAnswerEnter` reads the registration (:320), restores (:321) and enables signals (:322), then calls the helper (:324).
  - cmd/state_resume_wait.go:343-356 — `resumeAnswerDiscard` restores (:351) and enables (:352), then calls the helper with the empty command (:354).
  - cmd/state_resume_recover.go:35-60 — the leave sequence (:46), the marker clear (:52) and the enable (:56) run first, then the helper is called with the empty command (:58).
  - cmd/state_hydrate.go:262-274 — the parked chain still calls `execHandOff(cfg.Logger, cfg.ExecShell, "/bin/sh", args, true)` directly (:273).
  - cmd/state_hydrate.go:290-297 — `parkedChainBackstop` still carries `${SHELL:-/bin/sh}` (:296), unchanged as the task directs.
- Notes:
  - I read state_hydrate.go, state_resume_chain.go, state_resume_wait.go, state_resume_recover.go and state_resume_draw.go in full. In those files the only call to `resolveShell()` is cmd/state_resume_chain.go:141, and the only call to `hookExecArgs(` is :146; the definitions are at cmd/state_hydrate.go:185 and cmd/state_resume_chain.go:171.
  - In those files, `execHandOff` has exactly two callers: the helper (:143, :147) and `execResumeChainAndExit` (:273). `TestExecSeamsAreCalledOnlyByTheHandOffHelpers` (cmd/exec_handoff_guard_test.go:16-40) still sees exec-seam calls only inside `resumeHandOff` and `execHandOff`.
  - Deleting `execShellAndExit` instead of keeping it as a one-line wrapper is a sound divergence from the task's wording. Its only job was the miss branch, and the helper now covers that.

TESTS:
- Status: Adequate
- Coverage:
  - The helper's rule is pinned directly with literal expectations in `TestHandOffToHookOrShell` (cmd/state_resume_chain_handoff_test.go:136-214). It runs four cases (no command and a command, each with SHELL set and unset) and checks the program, the argv, the marker's target/args/hook_present, and that the marker was already emitted when the exec ran.
  - The eager routes are pinned with literals in cmd/state_hydrate_test.go:
    - bare shell on replay (:1340-1372), file-missing (:1409-1438), timeout (:1466-1515), lookup error (:1517-1595) and nil store (:1788-1816);
    - hook argv on replay (:1305-1338), file-missing (:1374-1407) and timeout (:1440-1464);
    - `/bin/sh` default (:438-465).
  - The Enter cases (moment-of-answer read, gone entry, empty command, unreadable store, SHELL unset) are in cmd/state_resume_enter_test.go:163-241. The discard hand-off is at cmd/state_resume_discard_test.go:196-202. The recover hand-off is at cmd/state_resume_recover_test.go:205-240.
  - The parked chain argv is at cmd/state_hydrate_lazy_test.go:209-221. The signal-generation ordering for Enter, discard and recover is at cmd/state_resume_signals_test.go:163-209 and :302-320.
  - The assertions the task names as each route pinning its own argv (cmd/state_resume_enter_test.go:39-40 and cmd/state_resume_recover_test.go:219-224) read as described, unchanged.
- Notes: The new helper test is focused and not redundant. It is the one place both hand-off forms are pinned as literals under both SHELL states, so a change to the shared form now fails that test as well as each route's suite.

CODE QUALITY:
- Project conventions: Followed. The exec marker still comes from `execHandOff` immediately before the exec, and the seam guard's permitted set is unchanged.
- SOLID principles: Good. The rule has one home, and the tty steps before each hand-off stay with the routes that own them.
- Complexity: Low
- Modern idioms: Yes
- Readability: Good
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "The existing hydrate, Enter, discard, recover, signal-generation and exec-seam guard suites pass with what they assert unchanged" — Reading confirms the cited assertions are unchanged and that production and tests route correctly through the helper. Settling "pass" needs a run of `go test ./cmd`, including the pty-backed signal tests. That run also confirms the package still compiles now that `execShellAndExit` is gone; any test file outside the ones read here that still named it would fail to build.
