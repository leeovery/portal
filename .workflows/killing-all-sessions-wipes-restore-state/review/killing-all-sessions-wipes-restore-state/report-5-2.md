TASK: A Confirmation Naming No Server Is Refused In Its Own Right (killing-all-sessions-wipes-restore-state-5-2, tick-566713)

ACCEPTANCE CRITERIA:
- A committing cycle that does not know its own server runs against a tmux that answers its confirmation with exit status 0 and no output. It stands down with an error wrapping `ErrNotOwnServer`, and `sessions.json` and every scrollback file are unchanged.
- `portal state commit-now` runs with `TMUX` empty against a tmux that answers its confirmation naming no server. It fails with `errCommitNowFailed`, and the saved state is unchanged.
- Each of those two scenarios still stands down with either check alone in place: the unknown-own-server guard, or the refusal of an answer naming no server. Its test fails only if both are removed.
- A committing cycle that knows its own server gets an answer naming no server, and refuses it with `ErrNotOwnServer` at each of the confirmation's three uses (after the capture; after a failed capture with the capture's error reachable; before an empty capture replaces a saved transcript, refused with `ErrUnconfirmedEmptyCapture`, transcript kept).
- A committer that does not know its own server and whose capture fails sends no confirmation read and returns the capture's error unchanged.
- `ConfirmAnswering` still answers pid 0 with no error for an answer carrying no output. The suites still classify a silent answer as `ErrNotOwnServer`, separately from a refused read.

STATUS: complete

SPEC CONTEXT: Spec section 2.2 (line 67, plus the 2026-10-05 corrigendum at line 285): the confirmation counts only when tmux returns exit 0 with an answer naming the server that answered it; tmux's shutdown answer (exit 0, no output) names no server, cannot meet the own-server rule, and stands the cycle down. Section 6.1 (line 225) tests that a no-output confirmation writes nothing. The task makes that rule hold in `confirmOwnServer` itself rather than leaning on the unknown-own-server early return, which was written for a different case.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/state/scrollback.go:267-282 (`confirmOwnServer`: the `ownServer <= 0` early return stays at :268-270; new `answered <= 0` refusal wrapping `ErrNotOwnServer` at :275-277, ahead of the mismatch comparison at :278-280); internal/state/scrollback.go:380-388 (`classifyFailedCapture`'s `ownServer <= 0` guard unchanged at :381); internal/tmux/tmux.go:92-105 (`ConfirmAnswering` unchanged, still `0, nil` for empty output); internal/state/scrollback.go:241-246 (`AnsweringConfirmer` contract unchanged).
- Notes: The change matches the Do list exactly. All three uses route through the one function: after the capture (scrollback.go:366), the failed-capture classification (:384), and the empty-capture write guard (:299). For a committer that knows its own server there is no behaviour change, because the mismatch comparison already refused pid 0. For one that does not, the early return already refused. So the new check is the intended second line of defence, and no existing caller can regress. I found no production path where `ownServer > 0` with `answered == 0` should be accepted. The `ConfirmAnswering` fakes in the tree (cmd/state_commit_now_test.go:41, internal/state/commit_cycle_test.go:87, commit_cycle_answered_test.go:33, commit_cycle_moved_test.go:66, commit_cycle_skip_test.go:49, capture_test.go:1615, empty_capture_test.go:28) were unaffected either way.

TESTS:
- Status: Adequate
- Coverage:
  - Criterion 1: TestRunCommitCycleStandsDownWithNoOwnServer (internal/state/commit_cycle_confirm_test.go:320). It now uses `shutdownAnswers{confirmRead}` with `exitsAfterConfirmation`, so the confirmation answers exit 0 with no output. It asserts `ErrNotOwnServer` and `savedPair.assertUnchanged`, which byte-compares `sessions.json` and every scrollback file.
  - Criterion 2: TestCommitNowStandsDownOutsideAnyTmuxServer (cmd/state_commit_own_server_test.go:85). `TMUX=""`, `fc.silentConfirm = true` (the `display-message` dispatch returns "" at cmd/state_daemon_run_test.go:132-134). It asserts `errCommitNowFailed` and `savedStateFixture.assertUnchanged`.
  - Criterion 3: settled by tracing both tests by hand; I did not run them. Guard alone: the early return refuses before any read. Refusal alone: `ConfirmAnswering` parses "" to 0 and `answered <= 0` refuses. Both removed: the comparison is `0 != 0`, false, so the cycle commits the "work"-only capture. That rewrites `sessions.json` and deletes the "notes" transcript, so each test fails (nil error, then assertUnchanged). commit-now reaches the cycle in this fixture: `IsRestoringSet` sees the fake's unknown-option `CommandError` and reports not restoring.
  - Criterion 4, after the capture: TestRunCommitCycleStandsDownOnAConfirmationNamingNoServer (commit_cycle_confirm_test.go:306), plus the "a confirmation naming no server" case in TestRunCommitCycleClassifiesAStandDownAsTmuxStoppedAnswering (:410).
  - Criterion 4, after a failed capture: the "a confirmation naming no server" cases in TestRunCommitCycleClassifiesAFailedCaptureByItsConfirmation (:491, asserts `ErrTmuxStoppedAnswering`, `errEnvironmentRead` reachable, and `ErrNotOwnServer`) and in TestRunCommitCycleClassifiesAnUnparseableSessionListingByItsConfirmation (:620).
  - Criterion 4, before an empty capture: the "the read after the capture is answered naming no server" case in TestDaemonDumpKeepsASavedTranscriptOverAnUnconfirmedEmptyCapture (cmd/state_daemon_empty_capture_test.go:82). The silent answer applies only after `capture-pane`, so it exercises the third use specifically. The test asserts the transcript is unchanged and that the refusal line was logged; cmd/state_daemon.go:343-345 emits that line only for an error wrapping `ErrUnconfirmedEmptyCapture`. It does not assert `ErrNotOwnServer` directly. That is not a gap: `confirmEmptyCapture` wraps the same `confirmOwnServer` result with `%w`, and the first-use test pins that classification.
  - Criterion 5: TestRunCommitCycleWithNoOwnServerReturnsAFailedCaptureUnconfirmed (commit_cycle_confirm_test.go:523) asserts no `display-message` call, the capture error reachable, and neither `ErrTmuxStoppedAnswering` nor `ErrNotOwnServer`. TestRunCommitCycleWithNoOwnServerReturnsAnUnparseableSessionListingUnconfirmed (:646) does the same for the parse error.
  - Criterion 6: the "an answer with exit status 0 and no output names no server" subtest of TestConfirmAnsweringNamesTheAnsweringServer (internal/tmux/own_server_test.go:48-56) checks `(0, nil)`. The silent-versus-refused split still holds at commit_cycle_confirm_test.go:487-491 and :616-620.
- Notes: The test changes are minimal: two existing tests were re-pointed at a silent confirmation, with no new scaffolding and no redundant assertions. Not over-tested.

CODE QUALITY:
- Project conventions: Followed (sentinel wrapped with `%w`, error string lower-case and context-bearing, consistent with the sibling refusals in the same function)
- SOLID principles: Good
- Complexity: Low (one added guard)
- Modern idioms: Yes
- Readability: Good. The added doc sentence on `confirmOwnServer` (scrollback.go:265-266) holds against the code. `ErrNotOwnServer`'s doc (:256-258), `ownTmuxServer`'s doc (cmd/state_commit_now.go:125-127) and `CommitCycle.OwnServer`'s doc (internal/state/commit_cycle.go:34-36) all remain true.
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
