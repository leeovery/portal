## Attempt 1

ISSUES:
- /Users/leeovery/Code/portal/internal/restore/lazy_resume_panel_integration_test.go:580-588 (`endWaiterUnanswered`), used at line 294 by `TestLazyResumePanel_HoldsThePanelOnAnAlternateScreenTheInstallTurnedOff`. The subtest "a waiter recovered by the chain's tail" sends SIGTERM to the waiter and expects the chain's recovery tail to run. The waiter now catches SIGTERM, which is exactly the behaviour this task requires, so the marker is never cleared. I ran it on the working tree:
  - `go test -tags integration -p 1 ./internal/restore -run TestLazyResumePanel_HoldsThePanelOnAnAlternateScreenTheInstallTurnedOff -count=1` fails at lazy_resume_panel_integration_test.go:338 with "@portal-resume-pending still set on the subject pane after it was answered".
  - The same test passes when the catch is neutralised.
  - The executor ran only the unit lane. No later phase-2 task revisits this helper.
  FIX: In `endWaiterUnanswered`, send `syscall.SIGKILL` instead of `syscall.SIGTERM`. A SIGKILL cannot be caught, so the waiter is lost without an answer and the chain's tail recovers the pane. The path is the same as the old default-SIGTERM path: in both, the process dies without running its deferred raw-mode restore, and Task 2.4's change cannot affect it. Then re-run the test above and confirm all six subtests pass.
  ALTERNATIVE: Send SIGHUP to the waiter alone. It is also uncaught, but I don't recommend it. A real SIGHUP reaches the parked shell too (Task 2.5's subject), so sending it to the waiter alone is a shape that never happens in practice. The subtest would then depend on how SIGHUP is handled rather than modelling a lost waiter.
  CONFIDENCE: high

NOTES:
- The new suite runs the parked chain through `/bin/sh` and re-executes the cmd test binary into `TestResumeTermPaneProcess`, a no-op unless `PORTAL_TEST_TERM_PANE_DIR` is set (the os/exec `TestHelperProcess` pattern). It builds no portal binary, starts no daemon and uses no tmux, so it stays within the unit-lane rule as CLAUDE.md words it. Each process it starts also runs TestMain's `go env` subprocess, which adds a little time per process start. The suite takes about 3s, and 5 repeated runs plus the full `go test ./cmd` passed.
- Read errors after a caught SIGTERM are covered. The select loops the draw and the waiter use (`awaitTTYInput`, `selectBoundedReader.awaitReadable`) already retry EINTR, and `os.File.Read` retries it too. So a caught SIGTERM cannot end the waiter as a read error and fall through to the recovery tail.
- `gofmt -l cmd/` and `go vet ./cmd` are clean.
- The `.tick/tasks.jsonl` change was already in the tree before this task started, and the executor did not touch it.
