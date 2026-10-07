TASK: Every Capture Read Refused By An Exiting Tmux Logs As A Back-Off (killing-all-sessions-wipes-restore-state-1-9, tick-bbdc90)

ACCEPTANCE CRITERIA:
- With a saved sessions.json and its scrollback files on disk, tmux refuses the skeleton-marker read during the daemon's tick, during its shutdown flush and during commit-now. Each logs its back-off line (tick / final flush / commit cycle "backed off: tmux stopped answering") at INFO or above with tmux's own words in `error` and no "… failed" line beside it. No session listing is read, and sessions.json and every scrollback file are unchanged.
- With the same saved state, the session listing names a live session and tmux then refuses the pane listing, during the tick, the shutdown flush and commit-now. Each logs its back-off line with tmux's own words in `error` and no "… failed" line beside it, and sessions.json and every scrollback file are unchanged.
- After either refusal, the tick leaves save.requested present and its next tick, with every read answered, commits; commit-now touches save.requested and exits non-zero; the shutdown flush's `shutdown` line reports flush_completed=false.
- A committing cycle whose skeleton-marker read or pane listing tmux refuses returns an error matching state.ErrTmuxStoppedAnswering from which the read's own error is still reachable.
- A tick whose pane listing is answered but fails to parse, one whose waiting pane's session is carried onto a name a captured session holds, and one in which every session's environment read fails anomalously each log `tick failed` and no back-off line.
- The restore-in-progress marker read fails in the tick, the shutdown flush and commit-now. Each logs its existing line for that failure, unchanged, and no back-off line.

STATUS: complete

SPEC CONTEXT: §2.5 routes a stand-down through each committer's existing failure route (tick WARN + save.requested re-touch, commit-now failCommitNow + non-zero exit, flush flush_completed=false). §4.2 requires a committer that backs off because tmux stopped answering mid-save to log a line saying so at INFO or above with the cause in `error`, and keeps the three existing restore-marker-read failure lines unchanged. §2.5 names only the session listing and the confirmation as stand-down triggers; the task widens this to the skeleton-marker read and pane listing, which is a sound reading of §4.2 (a refused read anywhere in the capture is the case §4.2 describes), so the divergence from §2.5's words is not a loss.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/state/scrollback.go:357-360 — ListSkeletonMarkers failure wrapped as `fmt.Errorf("%w: list skeleton markers: %w", ErrTmuxStoppedAnswering, err)`; ListSkeletonMarkers (internal/state/markers.go:59-63) returns only the ShowAllServerOptions read error, so every error wrapped here is a tmux read failure.
  - internal/state/capture.go:108-112 — ListAllPanesWithFormat failure wrapped in ErrTmuxStoppedAnswering, the same shape as the session-listing wrap at capture.go:102.
  - internal/state/capture.go:113-116 (parsePaneRows), capture.go:138-143 (all-sessions-anomalous aggregate) and the errCarryNameTaken return remain unwrapped, as the task requires.
  - internal/state/scrollback.go:248-254 — ErrTmuxStoppedAnswering doc covers the skeleton-marker read, session listing and pane listing as well as a refused confirmation.
  - internal/state/scrollback.go:337-347 — captureAndRefile doc states the marker-read failure is wrapped in ErrTmuxStoppedAnswering.
  - cmd/state_daemon.go:244-249 (cycleFailureMessage) and its three call sites (state_daemon.go:204, :391; state_commit_now.go:144) are unchanged, as the task requires.
- Notes: A later task added classifyFailedCapture (scrollback.go:376-386), which sends a confirmation after any capture failure not already classified. The direct wrap from this task still decides the outcome: classifyFailedCapture returns an already-wrapped error without sending a confirmation, so a refused marker read or pane listing ends the cycle at that read. A parse failure, carry collision or anomalous aggregate whose confirmation the committer's own server answers still logs "… failed". No other tmux read in RunCommitCycle (internal/state/commit_cycle.go:114-154) precedes or replaces these two, and no stale comment remains in cmd or internal/state that describes the old two-read classification.

TESTS:
- Status: Adequate
- Coverage:
  - AC1/AC2: cmd/state_commit_backoff_test.go:35-73 adds "a refused skeleton-marker read" (readsNoListing) and "a refused pane listing" entries to the standDowns table. TestDaemonTickReportsAStandDownAsABackOff (:143), TestCommitNowReportsAStandDownAsABackOff (:164) and TestShutdownFlushReportsAStandDownAsABackOff (:191) each check for exactly one back-off line at INFO or above, with keys exactly component+error and tmux's stderr reachable through `error` (assertBackOffLine :116). They also check that no "… failed" line is logged, that no list-sessions read follows the marker refusal (assertListingUnread :104), and that sessions.json and the scrollback bytes are unchanged against a fixture whose successful commit would drop "notes". The tests discriminate: removing either wrap leaves the daemonFakeCommander's confirmation answered by fakeOwnServerPID, so the line reverts to "… failed" and each test fails.
  - AC3: TestDaemonTickCommitsOnTheTickAfterAStandDown (:217) heals each refusal and checks that the re-touched save.requested drives a committing next tick (LastSaveAt is set to now, so the gap does not drive it). The commit-now test checks errCommitNowFailed and save.requested. The flush test checks flush_completed="false".
  - AC4: internal/state/commit_cycle_confirm_test.go:427-455 (TestRunCommitCycleClassifiesARefusedCaptureReadAsTmuxStoppedAnswering) checks errors.Is on the sentinel and errors.AsType for a *tmux.CommandError whose Args[0] is the refused read. It also checks that the cycle's last read is the refused one, which discriminates the direct wrap from classification by a refused follow-up confirmation.
  - AC5: TestDaemonTickReportsAFailureThatIsNoRefusedReadAsFailed (:394) covers a parse failure, a carry collision and an anomalous aggregate, each with exactly one `tick failed` WARN and no back-off line.
  - AC6: TestCommittersKeepTheirRestoreMarkerReadFailureLines (:332), which predates this task, pins each committer's existing WARN with its `error` and checks that no back-off line is logged.
  - Moved pins: cmd/state_daemon_run_test.go:705 and :765, and cmd/state_daemon_save_request_test.go:97 now expect tickBackedOff. The DoesNotClassify test was replaced by its opposite. The remaining `"… failed"` pins (state_daemon_lifecycle_log_test.go:195, state_commit_cycle_lock_test.go:132/:172, state_carry_missed_session_test.go:142, state_commit_unparseable_listing_test.go) are driven by non-read failures and correctly stay.
- Notes: The marker-read stand-down entries overlap with the older TestDaemonTick_CapturesOnALaterTickAfterAMarkerReadFailure and TestDaemonTick_LogsAndSkipsOnShowOptionsError, and the carry-collision case overlaps with TestDaemonTick_ACarryOntoALiveSessionsNameFailsTheTick, which already checks the failed line but not the absence of a back-off line. The task asked for the older tests to be moved rather than removed, so this overlap is planned, not a defect.

CODE QUALITY:
- Project conventions: Followed. Sentinel wrapping uses multi-%w as at the existing session-listing site, tests use errors.AsType, there is no t.Parallel, and the seams are staged through withCommitNowDeps / withOwnTmuxServer.
- SOLID principles: Good. Classification stays in one place (the sentinel), and cycleFailureMessage needed no change.
- Complexity: Low. Two one-line wraps.
- Modern idioms: Yes.
- Readability: Good. The fixture constructors (refusedMarkerReadCommander, refusedPaneListingCommander) and the table fields heal/readsNoListing make each refusal's shape explicit.
- Issues: None.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
