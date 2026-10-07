TASK: The Eager Resume Shell Catches SIGTERM (killing-all-sessions-wipes-restore-state-2-1, tick-23370c)

ACCEPTANCE CRITERIA:
- A restored eager resume pane's hook program is still running, and the pane's top process (the shell running the hook) receives SIGTERM. The pane and its session stay up, the hook program keeps running, and when it ends the pane goes on to the user's shell.
- A restored eager resume pane's hook program receives SIGTERM. It ends on it with default handling, and the pane goes on to the user's shell.
- The user's shell an eager resume pane goes on to starts with SIGTERM at its default disposition, not inherited as ignored.
- A lazy pane has been answered on its panel and its hook program is still running. The shell running that hook receives SIGTERM. It survives, its hook program and the user's shell keep default SIGTERM handling, and when the hook program ends the pane goes on to the user's shell.

STATUS: complete

SPEC CONTEXT: The shutdown SIGTERM reaches pane programs independently of the tmux server; a session whose panes exit on it closes while tmux still answers, and the session-closed commit-now plus the daemon's flush then remove it and its scrollback. Interactive shells ignore SIGTERM, but Portal's eager resume pane ran as a non-interactive `sh -c '<hook>; exec <shell>'`, which dies on it, and an answered lazy pane runs its hook through the same shell. The fix makes that shell survive SIGTERM through a caught trap (never an ignored disposition, which would be inherited across exec by the hook program and the user's shell), and leaves SIGHUP uncaught so a kill still ends the pane. The window between exec and the trap's first command, and fish's exiting SIGTERM handler, are accepted residue. The real-tmux session-survival check belongs to a later task (2-9, `cmd/state_shutdown_hardened_panes_integration_test.go`).

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/state_resume_chain.go:208-213 — `hookShellTrap = "trap : TERM; "`, a caught no-op trap covering TERM only (SIGHUP untouched), with a comment that matches the code.
  - cmd/state_resume_chain.go:219-221 — `hookExecArgs` prefixes the trap to the `-c` script: `/bin/sh`, `["sh", "-c", "trap : TERM; <command>; exec <shell>"]`.
  - cmd/state_resume_chain.go:172-180 — `handOffToHookOrShell` is the only production caller of `hookExecArgs`.
  - Both hook routes go through it: the hydrate helper's eager/unparked tail (cmd/state_hydrate.go:251) and the waiter's Enter answer (cmd/state_resume_wait.go:319). The other two callers (cmd/state_resume_wait.go:350 discard, cmd/state_resume_recover.go:62 recover) pass an empty command and exec the bare shell, so no hook runs outside the trapped shell.
- Notes: There is no `signal.Ignore` or SIG_IGN anywhere in cmd/ or internal/ production code. The only SIGTERM handling elsewhere is the Go `signal.Notify` catch (`catchSIGTERM`) and the daemon's handler, so nothing upstream hands the hook shell an ignored TERM. Prefixing the trap adds no parsing hazard that the existing `<command>; exec <shell>` splice did not already have. Both CLAUDE.md's Resume hooks section and the in-code comments describe the new form, and no production comment still describes the trap-less form.

TESTS:
- Status: Adequate
- Coverage: cmd/state_resume_hook_shell_test.go runs the argv each route really produces as a real `/bin/sh` process group. It gets the eager argv from `runHydrate` with an Eager-mode registration and the answered-lazy argv from `runResumeWait` with an Enter keystroke. Stub scripts stand in for the hook program and the user's shell; both record their pids and have bounded lifetimes. Each of the four tests runs over both routes:
  - TestResumeHookShell_SurvivesSIGTERMWhileTheHookRuns (:208), for criteria 1 and 4: the shell survives a SIGTERM, the hook program keeps running and runs to its end, and the pane execs into the user's shell, which then dies on SIGTERM. The SIGTERM is sent only after hook.pid exists, so the trap is guaranteed to be installed by then.
  - TestResumeHookShell_HookProgramEndsOnSIGTERM (:231), for criterion 2: the hook program dies on SIGTERM without writing hook.done, and the pane goes on to the user's shell.
  - TestResumeHookShell_UserShellStartsWithDefaultSIGTERM (:252), for criterion 3.
  - TestResumeHookShell_LeavesSIGHUPUncaught (:264): the shell dies by SIGHUP.
  The tests tell the three plausible regressions apart. With no trap, the first test fails. With an ignored trap (`trap '' TERM`), the hook and user-shell assertions fail, because the stubs keep their inherited disposition through `exec sleep`. With HUP added to the trap, the SIGHUP test fails. All existing tests that pinned the argv shape were updated to the trapped form. The update at cmd/state_resume_enter_test.go:200 also fixed a check that would otherwise have passed without testing anything: it now compares against `hookExecArgs(...)` with `slices.Equal`.
- Notes: The tests start only `/bin/sh` and `sleep`: no tmux, no daemon, no portal binary, so they belong in the unit lane. Cleanup SIGKILLs the pane's own process group (Setpgid), and the stubs are bounded (a hook loop capped at about 30s, `exec sleep 30`), so an aborted test binary leaves nothing running for long.

CODE QUALITY:
- Project conventions: Followed. Like `parkedChainTrap`, the trap is a named constant; the test does not use t.Parallel; all helpers are local or shared test helpers.
- SOLID principles: Good
- Complexity: Low
- Modern idioms: Yes
- Readability: Good
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
