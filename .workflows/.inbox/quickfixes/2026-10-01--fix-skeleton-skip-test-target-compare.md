# Fix the skeleton-skip daemon test so its target comparison can actually match

`TestDaemonTick_SkipsSkeletonMarkedPanesInScrollback` in `cmd/state_daemon_run_test.go` (around line 465) can never fail. It is meant to prove the daemon does not capture-pane a skeleton-marked (mid-restore) pane. It does this by comparing the raw capture-pane target against `"work:0.1"` (`call[6] == "work:0.1"`). But the daemon builds that target with `tmux.PaneTargetExact` in `cmd/state_daemon.go`, which writes the exact-match form `=work:0.1`. The comparison never matches, and the test asserts nothing else, so it passes whatever the daemon does.

So if the scrollback dump's skip (`CaptureCycle.SkipsScrollback`) stopped checking the Skeleton set, this test would stay green. Only one test really guards that arm: its sibling `TestDaemonTick_KeepsARenumberedWaitingPaneTranscriptWhileSkeletonMarked`, around line 530 of the same file, which unwraps the target with `sessionFromExactTarget` before comparing.

The change: compare `sessionFromExactTarget(call[6]) == "work:0.1"`, the way the sibling does. Optionally, also assert that no scrollback file exists for the skipped pane's key, as the sibling does around line 534. In the fixture, pane 1 carries no token and no pending flag, and its session is captured. So the Skeleton check is the only thing that skips it, and the corrected assertion fails if that check is removed.

Only `cmd/state_daemon_run_test.go` is touched.

This came out of the `lazy-resume-on-attach` review as out-of-scope finding A4, first raised in that review's task report 18-3. It was held out of scope because the broken assertion predates that feature.
