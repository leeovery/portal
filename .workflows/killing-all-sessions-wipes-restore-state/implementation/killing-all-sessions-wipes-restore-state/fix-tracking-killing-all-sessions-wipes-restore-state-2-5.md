## Attempt 1

ISSUES:
- internal/restore/resume_pane_hangup_integration_test.go:36, :70, :75-83 — In the real-tmux subtest, the evidence that the recovery tail never starts is only the absence of two substrings in portal.log. Nothing proves either substring would appear if the tail did run. `recoverStartArgs` (`args="state resume-recover`) is a test-local copy of `internal/log/handler.go`'s rendering (`quoteIfMultiWord` on `args`), and the log is only the panes' log because the env plumbing makes it so.
  - Under the regression this subtest exists to catch (HUP added to `parkedChainTrap` only), the process-ended check does not catch it on macOS. On master close, only the session leader gets SIGHUP. The waiter's read then fails, so it exits (state_resume_wait.go:219-225). The parked shell runs `resume-recover`, which hands off to a shell that exits on the hung-up tty. Every process still ends within `paneEndBudget`, so the log check is this subtest's only discriminator.
  - If the log's location or its `args` rendering drifts, the check passes while checking nothing, and no one notices.
  - The same suite already pairs such an absence check with a presence check for exactly this reason: internal/restore/lazy_resume_burst_integration_test.go:115-118 ("portal.log carries no line from the chain, so it cannot show a discard's absence").
  FIX: Split out a shared prefix, `chainStartArgs = "args=\"state "`, and define `recoverStartArgs = chainStartArgs + resumeRecoverArgv`. Right after `logBefore` is read (line 70), add `if !strings.Contains(logBefore, chainStartArgs+resumeWaitArgv) { t.Fatalf("portal.log carries no start line from the waiting pane's chain, so it cannot show the recovery tail's absence:\n%s", logBefore) }`. The waiter's own `process: start` line is always in the log by then, because the draw exec's into the waiter as a new image and `log.Init` runs in it. A change to the log location or the `args` rendering then fails loudly instead of silently voiding the absence check.
  ALTERNATIVE: Anchor on `chainStartArgs+resumeDrawArgv` instead. It works equally well, but the waiter is the process the hangup must end, so it is the more direct anchor.
  CONFIDENCE: high

COMMENT_CORRECTIONS:
- cmd/state_resume_term_test.go:258-259 — the new `tty` option means the test no longer always holds the pane's stdin (`stdin` is nil in tty mode), so the trailing clause is now false.
  OLD: // termPane is a waiting pane: the parked chain, started as restore leaves it,
// with the pane's stdin held by the test.
  NEW: // termPane is a waiting pane: the parked chain, started as restore leaves it.

NOTES:
- I ran these myself:
  - `go test ./cmd -run 'TestParkedResumeChain_PaneTerminalHangup|TestResumeWaitingPane_SIGTERM|TestParkedResumeChain_SIGTERM|TestResumeHookShell' -count=1` passes, including `TestResumeWaitingPane_SIGTERMBeforeTheCatch`.
  - `go test -tags integration -p 1 ./internal/restore -run TestResumePanes_EndWhenTheirTerminalCloses -count=1` passes in 2.9s.
  - `go vet` on both lanes is clean, and gofmt reports nothing.
- The eager-kill subtest never checks that the hook program is alive before the kill (`hookPID` is only read from the pid file). With `exec sleep 60` and a sub-second subtest this is safe in practice. I'm noting it, not asking for a change.
- AC4's `sessions.json` check holds partly by construction: this fixture runs no daemon and registers no `session-closed` hook, so nothing could commit after `kill-server`. The follow-up reboot, which brings the pane back still waiting, is the meaningful end-to-end evidence. That a commit attempted against an exiting server stands down is §2.2's own coverage in Phase 1.
- `.tick/tasks.jsonl` also changed; that is the orchestrator's bookkeeping, not executor output.
