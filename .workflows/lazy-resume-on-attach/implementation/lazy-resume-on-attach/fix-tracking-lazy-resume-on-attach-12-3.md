## Attempt 1

ISSUES:
- /Users/leeovery/Code/portal/cmd/state_daemon_save_request_test.go:107-134 — `TestDaemonTick_ConsumesSaveRequestedWhenTheCycleIsCancelled` never cancels a cycle, so it passes while checking nothing for criterion 6.
  - The fixture is `oneSession()`, which has a single pane. `cancel()` fires in the `capture-pane` dispatch of that pane, which is the last pane. `scrollbackDump.run` only checks `ctx.Err()` before each pane (`/Users/leeovery/Code/portal/cmd/state_daemon.go:293`, `:300`), so the cancellation is never seen and the cycle commits normally.
  - I confirmed this with a scratch overlay that adds a check that `sessions.json` is absent: the check fails, and the run logs `tick complete sessions=1 panes=1`.
  - The test therefore exercises the success path. Its assertions (no WARN, flag absent) are exactly what a successful tick produces.
  - What goes wrong: if a later change makes a cancelled cycle return an error from `captureAndCommit`, or makes it re-raise the flag, every daemon shutdown would log a spurious `tick failed` WARN and leave a stale `save.requested`. The one test named for that case would stay green.
  FIX: Give the tick a multi-pane fixture so the cancellation lands between panes, and add a check that proves the cycle was cancelled. Two panes are enough:
  - Replace `sess, panes := oneSession()` / `fc := &daemonFakeCommander{sessionsOut: sess, panesOut: panes}` with `sessionsOut: "work|1|0|"` and a two-row `panesOut` (`work|||0|||main|||layout|||0|||1|||0|||/tmp|||1|||zsh||||||\n` + `work|||0|||main|||layout|||0|||1|||1|||/tmp|||0|||bash||||||`). This follows the existing pattern in `TestCaptureAndCommit_CancelMidLoopAfterKofNPanesProcessed` (`/Users/leeovery/Code/portal/cmd/state_daemon_run_test.go:1353`).
  - Keep the no-WARN and flag-absent checks. Add `if _, err := os.Stat(state.SessionsJSON(dir)); !os.IsNotExist(err) { t.Errorf(...) }`, so the test fails if the cycle committed instead of cancelling.
  - Optionally also assert `len(fc.callsContaining("capture-pane")) < 2`.
  - I checked this fix through an overlay: with the two-pane fixture, `sessions.json` is not written, no WARN is logged and the flag stays absent.
  CONFIDENCE: high

NOTES:
- A tick triggered only by the 30s gap (no flag present) whose cycle fails now creates `save.requested` where there was none. This does no harm: `LastSaveAt` does not advance on failure, so the next tick retries through the gap branch either way, and the flag clear at daemon start removes any leftover flag.
- The integration test's `daemon: tick failed` check (`/Users/leeovery/Code/portal/cmd/state_commit_now_daemon_merge_integration_test.go:69`) searches the whole `portal.log`, not only the forced tick's lines. An earlier tick failing would need a commit-now holding `commit.lock` past the 5s bound, which is unlikely in this fixture. If it ever happens, the test fails loudly with the log attached, not silently.
- `daemonTickBudget` (4s) now covers two tick starts instead of one tick start and its completion. That is about 2s plus cycle time at the production 1s ticker. The test passed in 3.37s including fixture setup, so there is headroom on an idle machine, but less than before on a loaded integration run.
- Verification: the `TestDaemonTick|TestCaptureAndCommit|TestDaemonShutdown|TestDefaultShutdownFlush|TestStateCommitNow` unit subset passes; `TestCommitNowDaemonMergeStability` passes on the integration lane; `gofmt` lists nothing; `golangci-lint run ./cmd/...` reports 0 issues. No repository file was modified by the reviewer.
