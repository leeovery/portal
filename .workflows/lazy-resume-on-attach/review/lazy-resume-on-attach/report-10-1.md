TASK: A Waiting Pane's Transcript Survives the Captures Between Its Restore and Its Wait (lazy-resume-on-attach-10-1, tick-b128d7)

ACCEPTANCE CRITERIA:
- A waiting pane is saved at window 2 with its record naming `scrollback/pane-<token>.bin`. It is restored at window 1 and is still skeleton-marked when the daemon's tick captures it. The committed record at window 1 names `scrollback/pane-<token>.bin` and carries the saved working directory and command. The file is still on disk after the commit, and the daemon writes no scrollback for the pane.
- A restored waiting pane is still skeleton-marked when another session closes and fires `portal state commit-now`. The committed record names `scrollback/pane-<token>.bin` and the file is still on disk afterwards. The closed session is gone from `sessions.json`, and commit-now writes no scrollback file.
- The helper sets the pending marker before it unsets the skeleton marker, so the hand-over has three stages: skeleton marker only, both markers, pending marker only. A tokened waiting pane captured at each stage has a committed record naming `scrollback/pane-<token>.bin` at every stage, and the file is still on disk after each commit.
- A skeleton-marked pane carrying no token still takes the previous record at its own address, as it does today. A skeleton marker whose pane is gone brings no pane back into `sessions.json`, even when a previous record carries a token.
- When the skeleton-marker read fails, no capture is taken and nothing is committed. The daemon's tick logs its existing `tick failed` WARN and captures on a later tick. `portal state commit-now` exits non-zero with stderr silent, logs the failure and touches `save.requested`.
- A pane waited at shutdown under a lazy registration. It is restored under a renumbered window, and both a commit-now and the daemon's first post-restore tick capture it before its helper marks it pending. At the following reboot it restores with its original transcript above its panel.

STATUS: complete

SPEC CONTEXT: The specification's frozen-pane rule (the merge that carries a waiting pane's previous record forward) matches on the pane's durable `@portal-pane-id` token, never its position, because a positional miss leaves the token-named transcript unreferenced, the housekeeping pass then deletes it, and a pane that goes pending is never re-captured. The corrigendum of 2026-09-29 extends that rule back across the whole hand-over from restore to wait: a skeleton-marked pane carrying a token takes its previous record by token, a tokenless one still by address (later narrowed so an address match never takes a record whose token a live pane carries), and every committing capture — the saver's tick and `portal state commit-now` alike — reads the skeleton markers itself.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/state/capture.go:151-175 — `mergeSkippedPanes` resolves each skeleton-marked live pane through `takePrevRecord` (capture.go:288-301) on its live token. A tokened pane has `CWD`/`CurrentCommand`/`ScrollbackFile` carried onto its live address via `carryPrevContent` (capture.go:208-212). A tokenless pane takes the whole record at its own address. The loop walks only `fresh` panes, so a stale marker cannot resurrect a gone pane.
  - internal/state/capture.go:106-114 — the live-token set is taken before either merge. The skeleton merge runs before the frozen merge, so a pane that has both markers resolves to the same token record twice, with the same result each time.
  - internal/state/scrollback.go:275-288 — `captureAndRefile` reads `ListSkeletonMarkers` itself before the capture, returns a wrapped error with no capture on a failed read, and hands back `CaptureCycle{Index, Pending, Skeleton}` (scrollback.go:258-265).
  - internal/state/commit_cycle.go:51-72 — `RunCommitCycle` is now the only committing entry point and routes through `captureAndRefile`. This is the task's contract carried forward by later phases, and it still holds.
  - cmd/state_daemon.go:257-264, 343-349 — the daemon's dump skip reads `capture.Skeleton` / `capture.Pending`. On a failed cycle the tick logs `tick failed` WARN, re-touches `save.requested` and leaves `LastSaveAt` unchanged (cmd/state_daemon.go:200-208).
  - cmd/state_commit_now.go:114-125 — commit-now passes no `Dump` (so it writes no scrollback and commits with changed=false). A marker-read failure reaches `failCommitNow`, which logs, touches `save.requested` and returns the silent sentinel.
  - cmd/state_hydrate.go:341-352 — `markPendingThenUnsetSkeletonMarker` sets the pending marker before unsetting the skeleton marker, which gives the three-stage hand-over the third criterion assumes.
- Notes: The code has moved on from the task commit (4160642d9). `CaptureAndRefile` is now the unexported `captureAndRefile`, called only from `RunCommitCycle` under the commit lock. `linkMovedSkeletonScrollback` has been added, and so has the live-token exclusion from `byAddress`. All of these are later, spec-backed refinements (see the corrigenda). None of them weakens this task's criteria: the tokened skeleton merge, the marker read inside the cycle, the returned skeleton set and the marker-read failure routes all stand at HEAD. The only committing caller of `CaptureStructure` is `captureAndRefile` (internal/state/scrollback.go:280), so no nil-skip path remains.

TESTS:
- Status: Adequate
- Coverage:
  - Criterion 1: cmd/state_daemon_run_test.go:467-533 (`TestDaemonTick_KeepsARenumberedWaitingPaneTranscriptWhileSkeletonMarked`). The pane is saved at window 2 and live at window 1. `LastSaveAt` is zero, which models the first tick. The test asserts the committed record at window 1 names the token path with the saved /saved + vim, and that the file survives the commit, which does GC because the topology changed. It asserts no capture-pane call on the pinned `=work:1.0` target, using `sessionFromExactTarget` so the comparison is not vacuous, and no positional file for the pane.
  - Criterion 2: cmd/state_commit_now_test.go:1283-1340 (`TestStateCommitNow_KeepsASkeletonMarkedWaitingPaneTranscript`). This drives the real command body and the real `RunCommitCycle` over a fake client that answers the marker read. It asserts the closed session is dropped, the token-path record carries the saved /tmp + zsh, and the scrollback dir holds only the token file, so nothing was written.
  - Criterion 3: internal/state/capture_refile_test.go:125-178 (`TestCaptureAndRefileKeepsARestoredWaitingPaneTranscript`). It covers all three stages with the pane away from its saved address, commits each stage (GC runs on the structural change), and checks both the record and the file.
  - Criterion 4: internal/state/capture_test.go:971-1069 and later subtests cover the tokenless address match. internal/state/capture_test.go:2013-2037 covers a gone pane whose saved record carries a token, and 1989-2011 covers a tokened pane refusing a foreign record at its own address.
  - Criterion 5: internal/state/capture_refile_test.go:93-119 checks that no list-sessions or list-panes call runs after a failed marker read. cmd/state_daemon_run_test.go:667-694 checks the `tick failed` WARN, no capture and no sessions.json, then a commit on the next tick. cmd/state_commit_now_test.go:1342-1379 checks the silent-sentinel exit, empty stderr, no capture, nothing committed, `save.requested` touched and an ERROR record carrying the marker error.
  - Criterion 6: internal/restore/lazy_resume_renumbered_restore_integration_test.go:37-92 (integration lane) covers a lazy registration and a renumbered restore (asserted by `assertRestoredRenumbered`), then a commit-now round, a daemon round with the skeleton pane skipped, the pending hand-over and a second reboot checking the history holds the pre-reboot line.
- Notes: The integration test's commit-now round (lazy_resume_renumbered_restore_integration_test.go:168-186) rebuilds commit-now's cycle rather than exec'ing the command. The unit test for criterion 2 runs the real command body, so together they cover it. There is some overlap between the three marker-read-failure tests, but each pins a different layer's contract (no capture; WARN plus retry; exit, stderr, touch and log), so they are not redundant.

CODE QUALITY:
- Project conventions: Followed. Seam types and injection go through `withCommitNowDeps`, `commandertest` is used Strict, no `t.Parallel`, and the log vocabulary is unchanged.
- SOLID principles: Good. The skeleton read moved into the cycle that owns it, and the dump and its skip stay with the caller.
- Complexity: Low. The replacement merge is a single loop over live panes with in-place mutation, and the old find-or-append/resort machinery is gone.
- Modern idioms: Yes
- Readability: Good. The comments on `CaptureStructure`, `mergeSkippedPanes`, `captureAndRefile` and `CaptureCycle` hold true against the code.
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "A pane waited at shutdown under a lazy registration. It is restored under a renumbered window, and both a commit-now and the daemon's first post-restore tick capture it before its helper marks it pending. At the following reboot it restores with its original transcript above its panel." — Reading settles the capture/merge/commit logic and shows the integration test models the scenario. Whether real tmux renumbers the restored window and replays the transcript above the panel at the second reboot needs a run: `go test -tags integration -p 1 ./internal/restore -run TestLazyResumePanel_RenumberedRestoreKeepsTranscriptThroughEarlyCaptures`.
