TASK: A Session Listing Tmux Answered But Portal Could Not Parse Is Classified By The Confirmation (killing-all-sessions-wipes-restore-state-4-2, tick-8c12d5)

ACCEPTANCE CRITERIA:
- A live session named `work|notes` sits beside the saved sessions, so tmux answers the session listing with a line Portal cannot parse. The committer's own server answers the read sent after it. The daemon's tick logs `tick failed` at WARN, its shutdown flush logs `final flush failed` at WARN, and `commit-now` logs `commit cycle failed` at ERROR, each with the parse error in `error`. None of them logs a back-off line, and `sessions.json` and every scrollback file are unchanged.
- After that failure, the tick leaves `save.requested` present and its next tick retries. `commit-now` touches `save.requested` and exits non-zero. The shutdown flush's `shutdown` line reports `flush_completed=false`.
- With the same unparseable listing, the read sent after it is refused, answered naming no server, or answered by another server. The cycle returns an error matching `state.ErrTmuxStoppedAnswering`, with the parse error and the confirmation's cause both reachable from it, and each of the three committers logs its back-off line. A cycle whose own server is unknown sends no confirmation and returns the parse error unchanged.
- A `list-sessions` that tmux refuses still makes the cycle return an error matching `state.ErrTmuxStoppedAnswering`, with tmux's own words reachable from it. The cycle ends at that read and sends no confirmation after it, and each of the three committers logs its back-off line.
- Through the tmux client, a session listing that fails to parse returns an error matching the new `internal/tmuxerr` sentinel. A failed `list-sessions` read returns an error that does not match it and still carries tmux's `*tmux.CommandError`.

STATUS: complete

SPEC CONTEXT: §2.1 makes a failed `list-sessions` a stand-down on the committing path only. §2.2 has every committing cycle confirm, with a read sent after its last capture read, that its own tmux server still answers. §2.5 routes a stand-down through each committer's existing failure route. §4.2 reserves the back-off line for a committer that backed off because tmux stopped answering. The task applies that rule to a listing tmux answered in full but Portal could not parse. Such a listing is not a refused read, so the confirmation classifies it: `… failed` when the own server answers, a back-off when it does not. Only the logged line changes. Writes, retries, `save.requested` and `flush_completed` stay as they were.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tmuxerr/errors.go:19-22 — the new `ErrSessionListUnparseable` sentinel sits beside `ErrNoSuchSession` and `ErrUnaddressableSessionName`.
  - internal/tmux/tmux.go:172-202 — `parseSessionList` wraps the sentinel into all three parse-error returns (:191, :196, :201). `ListSessionsProbe` (:164-170) still wraps only a failed read, as `failed to list tmux sessions: %w` over the `*CommandError`, without the sentinel.
  - internal/state/capture.go:95-103 — a `ListSessionNamesProbe` error matching the sentinel is returned unwrapped. Any other error keeps its `ErrTmuxStoppedAnswering` wrap. This mirrors the pane-listing read and parse split at :107-114.
  - internal/state/capture.go:20-25 — the `CaptureClient` interface doc states the new contract.
  - internal/state/scrollback.go:248-254 — the `ErrTmuxStoppedAnswering` doc now says "its read of the skeleton markers, the session listing or the pane listing failed", so it names the read rather than the listing.
  - internal/state/scrollback.go:380-388 — `classifyFailedCapture` is unchanged, as planned. An error without `ErrTmuxStoppedAnswering` goes through the own-server confirmation. With the own server unknown, it is returned as given.
- Notes:
  - `ListSessionNamesProbe` has one production caller, capture.go:95. Its other production callers are `ListSessions` (tmux.go:151) and the search form's `ListSessionsProbe` call (cmd/open_search.go:178). Neither checks the sentinel, so their handling is unchanged. Only their error text gains the sentinel's prefix, and no test or production code matches the old text.
  - The marker-read wrap (scrollback.go:359), the pane-listing read wrap (capture.go:111), `cycleFailureMessage` (cmd/state_daemon.go:244-249) and the committers' failure routes (cmd/state_daemon.go:204-208, :391-393; cmd/state_commit_now.go:143-149) are all untouched, as required.
  - A later comment correction (8bd3c5522) dropped the tmuxerr doc's claim that a pipe-bearing name always fails to parse. The correction is sound: a name like `api|2` misparses silently instead of failing. That is a separate, pre-existing defect, already captured in the inbox (`.workflows/.inbox/bugs/2026-10-07--pipe-in-session-name-breaks-listing.md`), and outside this task.

TESTS:
- Status: Adequate
- Coverage:
  - Criterion 1: `TestCommittersReportAnUnparseableSessionListingTheirOwnServerAnswersAsFailed` (cmd/state_commit_unparseable_listing_test.go:91-114) runs all three committers against a fake listing `work` and `work|notes`.
    - It checks the `… failed` line at the exact level, with `ErrSessionListUnparseable` reachable from `error` and `ErrTmuxStoppedAnswering` not.
    - It checks that a confirmation was sent and that no back-off line was logged.
    - It checks `sessions.json` and the scrollback files are unchanged. Without the fix, the unconditional wrap would make the `Only` match fail.
  - Criterion 2: `assertFailureRoute` (:72-89) covers each committer's failure route: `save.requested` for the tick, `errCommitNowFailed` plus `save.requested` for commit-now, and `flush_completed=false` for the flush. `TestDaemonTickRetriesAfterAnUnparseableSessionListing` (:116-135) shows the retry. Its second tick runs with `LastSaveAt=now`, so it commits only because `save.requested` survived.
  - Criterion 3, state level: `TestRunCommitCycleClassifiesAnUnparseableSessionListingByItsConfirmation` (internal/state/commit_cycle_confirm_test.go:600-643) covers a refused confirmation, one naming no server, and one from another server. Each must match `ErrTmuxStoppedAnswering`, with both the parse sentinel and the confirmation's cause reachable, and the confirmation as the last read.
  - Criterion 3, committers: `TestCommittersReportAnUnparseableSessionListingTheirOwnServerDoesNotConfirmAsABackOff` (cmd test :137-180) checks each committer's back-off line across the same 3×3 matrix.
  - Criterion 3, own server unknown: `TestRunCommitCycleWithNoOwnServerReturnsAnUnparseableSessionListingUnconfirmed` (:645-664) checks that no confirmation is sent and the parse error comes back unchanged.
  - Criterion 4 was already pinned before this task. `TestRunCommitCycleSendsNoConfirmationAfterARefusedListing` (commit_cycle_confirm_test.go:546-571) checks the wrap, that no confirmation is sent, and that the cycle ends at `list-sessions`. The "a failed session listing" entry in the `standDowns` table (cmd/state_commit_backoff_test.go:44-51) runs the tick, commit-now and the flush through `assertBackOffLine`, which checks tmux's stderr is reachable.
  - Criterion 5: `TestListSessionsProbe_MarksAnUnparseableListing` (internal/tmux/list_sessions_probe_test.go:209-240) covers all three parse-error returns (missing fields, the pipe name in the window-count slot, an unparseable attached count) through both probe readers. `TestListSessionsProbe_DoesNotMarkAFailedReadUnparseable` (:242-265) checks that a failed read is not marked and still carries the `*CommandError` with its stderr.
- Notes: The cmd-level 3×3 confirmation matrix partly repeats the state-level classification test. Each layer asserts a distinct required property, though: the state level checks the classification, and the cmd level checks each committer's logged line, which criterion 3 names explicitly. The fakes are cheap. This is not over-testing worth acting on.

CODE QUALITY:
- Project conventions: Followed. The sentinel lives in the leaf `tmuxerr` package so `state` can classify without importing `tmux` (consistent with tmuxerr/doc.go). The tests take no `t.Parallel()`. Seams are staged through `withCommitNowDeps`, `withOwnTmuxServer` and `logtest.Install`. The new cmd test is unit-lane and touches no real tmux. Comments carry no process-artifact references.
- SOLID principles: Good. Read failure versus parse failure is decided in the tmux client, which knows which is which. The state package only discriminates on the sentinel.
- Complexity: Low. The change is one `errors.Is` branch.
- Modern idioms: Yes. It uses multi-`%w` wrapping, and the tests use `errors.AsType`.
- Readability: Good. The comments at capture.go:97-98 and tmux.go:172-174 state why the two failures are kept apart.
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
