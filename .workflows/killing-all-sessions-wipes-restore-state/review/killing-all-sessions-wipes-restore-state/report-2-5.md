TASK: A Kill Or Tmux's Exit Still Ends A Hardened Pane At Once (killing-all-sessions-wipes-restore-state-2-5, tick-5b39bf)

ACCEPTANCE CRITERIA:
- A restored lazy pane is waiting on its panel, and it is killed. Its parked shell and its waiter end at once, and the kill removes the pane from the saved state as it does today. (§3.3, §6.2)
- A restored eager resume pane's hook program is still running, and the pane is killed. The pane ends at once, and the kill removes it from the saved state as it does today. (§3.3, §6.2)
- A restored lazy pane is waiting when its pty closes because tmux has begun exiting. The parked chain ends on SIGHUP before its recovery tail starts: `portal state resume-recover` never runs, and no clear of `@portal-resume-pending` is attempted. (§3.2, §6.2)
- After that exit, `sessions.json` still names the waiting pane's token-named transcript, and that file is present. (§3.2, §6.2)

STATUS: complete

SPEC CONTEXT: §1.1 makes a kill final: it removes the session's record, its scrollback files and (through the sweep) its hooks. §3.1/§3.2 harden the eager resume shell, the parked chain, the draw and the waiter against SIGTERM, and require the handling to be caught, never ignored. §3.3 requires that none of them catch SIGHUP, so a kill (tmux closes the pty, which delivers SIGHUP) still ends an eager or waiting pane at once and the kill path stays unchanged. §3.2's last paragraph covers tmux's own exit: the SIGHUP from the pty close ends the parked shell, and the waiter with it, before the recovery tail can start. No marker clear is attempted, so the saved record keeps the pane waiting. §2.2 explains why no commit follows the exit. §6.2 lists the two test bullets this task owns: the pty-close-on-exit case and the killed-pane case.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/state_resume_chain.go:213 — `hookShellTrap = "trap : TERM; "`, so the eager resume shell built by `hookExecArgs` (cmd/state_resume_chain.go:219-221) catches TERM only.
  - cmd/state_hydrate.go:281 — `parkedChainTrap = "trap : INT QUIT TERM; "`, so the parked chain leaves HUP at its default disposition.
  - cmd/state_resume_chain.go:204-206 — `catchSIGTERM` notifies SIGTERM only. The draw (cmd/state_resume_draw.go:44) and the waiter (cmd/state_resume_wait.go:103) install it through `CatchSIGTERM`.
  - cmd/state_resume_wait.go:427-431 — `winchSignals` notifies SIGWINCH only.
- Notes:
  - No production code notifies or ignores SIGHUP apart from the daemon's own flush (cmd/state_daemon.go:469). A repo-wide grep finds three `signal.Notify` sites: the daemon, `winchSignals` and `catchSIGTERM`, and no `signal.Ignore`.
  - No termios path sets CLOCAL (no `CLOCAL`/`Cflag` in non-test sources), which would suppress the carrier-loss SIGHUP to the session leader.
  - The kill path is unchanged. The commit cycle's carry, answered-pane hold and moved-pane hold all key on a live pane, so a killed session drops out of the next commit as before.
  - The task delivered no production change beyond confirming this. Its commit (2fc6778b9) adds coverage only, which matches the task's "Solution: Coverage that…".

TESTS:
- Status: Adequate
- Coverage:
  - internal/restore/resume_pane_hangup_integration_test.go:49-57 — on real tmux, a killed eager pane whose hook program (`exec sleep 60`) still runs: the eager shell and the hook program both end within 3s. `assertKillCommitted` then confirms that a commit-now-shaped cycle drops the session and deletes every scrollback file it named (AC2).
  - internal/restore/resume_pane_hangup_integration_test.go:59-103 — real tmux exit (kill-server) while the pane waits. The test asserts:
    - the parked shell and the waiter end;
    - portal.log gains no `args="state resume-recover` start line and no `unset resume pending marker failed` line;
    - `sessions.json` still names the token-named transcript, with unchanged bytes;
    - the next restore brings the pane back on its panel with its marker set.
    A precondition (line 72) proves the chain's start lines reach this log, so the absence check cannot pass vacuously (AC3, AC4).
  - internal/restore/resume_pane_hangup_integration_test.go:105-114 — a killed waiting pane: the parked shell and the waiter end, and the kill commit removes the session and its files (AC1).
  - cmd/state_resume_hangup_pty_test.go:50-82 — in the unit lane, on a real pty with the parked chain as session leader and controlling-terminal owner, closing the master:
    - ends the chain with wait status "signalled SIGHUP";
    - ends the waiter;
    - records neither `recovered` (the stubbed tail) nor `cleared`.
    The stub tmux now records the backstop's shell-side clear as well (cmd/state_resume_term_test.go:78-80). This is the discriminating pin for AC3's "before its recovery tail starts".
  - cmd/state_resume_term_test.go:533-542 — the draw still ends on SIGHUP after its SIGTERM catch is installed. This complements the waiter (522-531), parked shell (cmd/state_parked_chain_term_test.go:93-109) and eager shell (cmd/state_resume_hook_shell_test.go:264-283) SIGHUP tests that earlier tasks landed, so all four hardened processes are pinned.
- Notes:
  - Each integration subtest would fail if its behaviour broke:
    - A shell that caught HUP would leave `sleep 60` (the eager case) running past the 3s budget.
    - A parked chain that caught HUP after tmux's exit would start resume-recover. Its `process: start` line would appear, and so would its failed-clear WARN against the gone server.
  - The AC4 `sessions.json` check holds trivially in this fixture, which registers no committer. The mechanism keeping the record in place in production (no committer passes the §2.2 confirmation after exit) is owned and tested by Phase 1. Here the check pins the outcome, and the restore assertion confirms the pane comes back still asking.
  - Not over-tested. Each subtest maps to one acceptance criterion, and the pty unit test covers a property (exit cause, no backstop clear) that the integration test cannot observe.
  - Lane placement follows CLAUDE.md:
    - The integration file is `//go:build integration`, isolates state through `setupLazyResumePanelOn` → `IsolateStateForTest`, and builds the binary via `restoretest.BuildPortalBinaryDir`.
    - The pty test re-execs only the test binary, puts a stub tmux first on PATH, and is darwin-gated with the `openPTY` helper it uses.
    - Neither signals a process it did not spawn: the integration test only probes, with `kill(pid, 0)`, pids its own panes reported.

CODE QUALITY:
- Project conventions: Followed. There is no `t.Parallel`, helpers are drawn from `harnesstest`/`restoretest`/`tmuxtest`, targets are pinned through `tmux.CoordTargetExact`, and comments carry no process artifacts.
- SOLID principles: Good
- Complexity: Low
- Modern idioms: Yes (`strings.SplitSeq`, range-over-func)
- Readability: Good. Subtest names state the behaviour, and the `drainPTY` comment explains why the reads are non-blocking (a blocking read in flight would hold the master open past its Close).
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "A restored lazy pane is waiting when its pty closes because tmux has begun exiting. The parked chain ends on SIGHUP before its recovery tail starts: `portal state resume-recover` never runs, and no clear of `@portal-resume-pending` is attempted." — Reading settles the mechanism: no hardened process catches HUP, and no termios path sets CLOCAL. The ordering (the kernel posting SIGHUP to the session leader before the waiter's read of the hung-up tty can return and let the parked shell proceed) is OS and tmux behaviour outside the repo. To settle it, run `go test ./cmd -run TestParkedResumeChain_PaneTerminalHangup -count=50` and `go test -tags integration -p 1 ./internal/restore -run TestResumePanes_EndWhenTheirTerminalCloses -count=10`, and observe no `recovered`/`cleared` event and no resume-recover start line on any run.
