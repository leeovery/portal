# Analysis Tasks: Killing All Sessions Wipes Restore State (Cycle 2)

## Task 1: The Answered-Pane Hold Is Decided From The Last Committed Index
severity: low
sources: architecture

**Problem**: Phase 3 Task 1 added a hold that keeps an answered lazy pane on its token-named transcript. The hold is decided from the caller's `LoadPrev` (`keepAnsweredTranscripts`, called at `internal/state/capture.go:157-159` and defined at `:365`). For the daemon, `LoadPrev` is its in-memory index (`cmd/state_daemon.go:271`). That index changes only when the daemon itself commits, so it does not see a `commit-now` that filed the pane under its token. `holdTokenFiledTranscripts` (`internal/state/scrollback.go:374-397`) covers that lag by checking whether files exist, so it helps only when the positional file named by the pane's fresh record is absent.

The loss is reachable inside one daemon commit gap:
- a pane waits;
- a `session-closed` `commit-now` files it under its token-named file, and that commit's housekeeping removes the positional name;
- the user answers the pane;
- the pane's live address now names a positional file that is already on disk, for example after a sibling pane was killed and the window renumbered;
- as tmux goes down, the pane's capture comes back empty and unconfirmed, or is refused.

Neither pass holds the pane. The commit names the positional file, and `gcOrphanScrollback` deletes the token-named transcript. After the next reboot the pane comes back with another pane's old scrollback, or with none, and the bytes cannot be recovered.

Traced through the tree: X waits at `work:0.1`, and a `commit-now` files it under `pane-T.bin` and removes `work__0.1.bin`. The pane at `work:0.0` is killed, so X becomes pane 0, and `work__0.0.bin` still holds the killed pane's old transcript. In the daemon's next dumping cycle `keepAnsweredTranscripts` finds T's previous record naming `work__0.1.bin` and does nothing, and `holdTokenFiledTranscripts` finds `work__0.0.bin` on disk and does nothing. `heldTranscripts` (`internal/state/commit_cycle.go:134-148`) then leaves X out, so the writer judges X's empty capture against the killed pane's file. If that capture is not confirmed, or the capture is refused, nothing is written for X, the commit names `work__0.0.bin`, and its housekeeping deletes `pane-T.bin`.

§2.4 states the measure the hold misses: a pane's saved transcript is the file its last committed record names. `RunCommitCycle` already reads that index under the same lock, inside `Commit` (`internal/state/commit.go:37`). The test at `internal/state/commit_cycle_answered_test.go:332` pins that a positional file on disk wins over the token-named transcript whenever the caller's previous index names the positional file.

**Solution**:
- `RunCommitCycle` reads `sessions.json` once under the commit lock, and the hold is decided from that index. A pane that is neither skipped nor waiting, and whose last committed record (matched on its token) names its token-named transcript, keeps naming that transcript until a dump writes the pane's new capture. The writer's `held` map and the `fileAtPositional` hand-back stay as they are.
- The commit measures change and drops against that same read, so `sessions.json` is not decoded a second time.
- `holdTokenFiledTranscripts` is deleted.
- `LoadPrev` still feeds the skeleton, frozen and carry merges, unchanged. When `sessions.json` cannot be read, the hold falls back to `LoadPrev`, as it uses it today.

This is derived from the record:
- §2.4 names the last committed record as the measure.
- §4.1 measures drops against the on-disk index for the same reason: the daemon's in-memory index does not see `commit-now`'s writes.
- The phase 3 fix round decided against pointing the daemon's whole `LoadPrev` at disk, because of its cost to those merges and the extra decode. This proposal reads the disk for the hold alone and reuses the read `Commit` already makes.

**Outcome**: Whether an answered pane keeps its transcript no longer depends on which committer filed it under its token, or on what its new positional path holds. From the answer until a dump writes the pane's new capture with confirmed bytes, every commit names the token-named transcript the last commit named. That includes the case where the pane's new address names a positional file another pane left on disk.

**Acceptance Criteria**:
- [ ] `sessions.json` names an answered pane's token-named transcript, filed there by a `commit-now` while the pane waited. The daemon's previous index predates that commit and names the pane at its old positional path. The pane's live address names a positional file on disk holding another pane's old transcript. The pane's capture then comes back empty and the read sent after it is refused, answered by another server, or answered naming no server; or its capture is refused and nothing is dumped. The daemon's tick and its shutdown flush each write nothing for the pane, commit its record naming the token-named transcript, and leave that file on disk with its bytes, so the next restore finds them. (§2.4)
- [ ] From the same start, a dump that writes the pane's capture with confirmed bytes (a non-empty capture, or an empty one the committer's own server confirms against the token-named transcript) commits the pane's record naming its positional file holding that capture, and that commit's housekeeping removes the token-named transcript. (§2.4)
- [ ] With `sessions.json` naming the token-named transcript, a daemon tick whose previous index names that transcript and one whose previous index predates its filing commit the same record for the pane and leave the same files on disk, whether the pane's positional path is absent or holds another pane's file. A `commit-now` in the same state commits the same record. (§2.4)
- [ ] With `sessions.json` absent or not decodable, the hold follows the caller's previous index as it does today: a previous record carrying the pane's token and naming its token-named transcript keeps the pane on that transcript, and one naming the positional path leaves the pane's empty capture judged at its positional file. (§2.4)
- [ ] The skeleton, waiting-pane and carry merges still take their records from the caller's previous index. A commit still measures drops and change against the last committed index: a session the last commit held and the capture lacks logs one `session dropped` line at INFO, a renamed session logs none, and a capture matching the last commit writes nothing. The existing merge, drop-log and no-change tests pass unchanged. (§4.1)
- [ ] `rg -n 'holdTokenFiledTranscripts' internal cmd` finds nothing. (§2.4)

**Do**:
- `internal/state/commit_cycle.go`, `RunCommitCycle` (`:100-130`): under the commit lock (`:101-105`), read `sessions.json` once. The hold is decided from that read. The commit measures its no-change skip and its drop lines against the same read, so `sessions.json` is not decoded a second time. Today `Commit` reads it itself through `readPriorIndex` (`internal/state/commit.go:37`, defined `:57-68`) for `structuralChange` (`:38`) and `logDroppedSessions` (`:46-48`). When `sessions.json` cannot be read, the hold is decided from `LoadPrev`.
- The hold's rule is the one `keepAnsweredTranscripts` applies today (`internal/state/capture.go:359-387`, called with `LoadPrev`'s index at `:157`): a pane neither skipped nor waiting whose record carrying its token names `PendingScrollbackFile(token)` keeps naming it. Only the index it is decided from changes.
- Delete `holdTokenFiledTranscripts` (`internal/state/scrollback.go:374-397`, called at `:370`). Its `fileExists` helper (`:399-403`) has no other caller in `internal/state`.
- Unchanged: `heldTranscripts` (`internal/state/commit_cycle.go:132-148`), the writer's `held` map (`:58-60`, `:72-79`) and `fileAtPositional` (`:150-166`); `LoadPrev`'s feed into `mergeSkippedPanes` (`internal/state/capture.go:148-150`), `mergeFrozenPanes` (`:152-154`) and `carryMissedWaitingSessions` (`:160-166`); the daemon's `LoadPrev` (`cmd/state_daemon.go:271`) and `commit-now`'s (`cmd/state_commit_now.go:119-122`).
- Outside the cycle's own call (`internal/state/commit_cycle.go:126`), `state.Commit` is called only from test files: `rg -n '\bstate\.Commit\(' --type go` → 56 hits across 21 files, every one a `_test.go`. Two of them are source fixtures inside `internal/state/commit_guard_test.go`.
- Existing tests: `TestRunCommitCycleHoldsAnAnsweredPanesTranscriptWhenThePreviousIndexPredatesItsTokenFiling` (`internal/state/commit_cycle_answered_test.go:363-394`) and the stale case of `TestDaemonDumpKeepsAnAnsweredPanesTokenNamedTranscriptOverAnUnconfirmedEmptyCapture` (`cmd/state_daemon_empty_capture_test.go:209-255`) stage the stale previous index with the positional file absent, the only case the deleted existence check covered. The second subtest of `TestRunCommitCycleJudgesAnOrdinaryPanesEmptyCaptureAtItsPositionalFile` (`internal/state/commit_cycle_answered_test.go:332-352`) seeds no `sessions.json`, so after this change it runs on the fallback to `LoadPrev`.

## Task 2: A Session Listing Tmux Answered But Portal Could Not Parse Is Classified By The Confirmation
severity: low
sources: architecture

**Problem**:
- `captureStructure` wraps every `ListSessionNamesProbe` error in `ErrTmuxStoppedAnswering` (`internal/state/capture.go:95-98`).
- That probe returns `parseSessionList`'s errors through the same return as a failed `list-sessions` (`internal/tmux/tmux.go:163-169`, `:185-197`).
- `classifyFailedCapture` returns early on the sentinel (`internal/state/scrollback.go:408-411`).

So a listing that tmux answered in full, but that Portal could not parse, is labelled a back-off without any confirmation being sent.

A session name containing `|` triggers this every time. tmux accepts the name, and neither `ValidateSessionName` nor `SanitiseProjectName` refuses it. The `|` shifts the `|`-delimited fields of `listSessionsArgs`, so the window count fails to parse. As long as that session exists:
- every daemon tick logs `tick backed off: tmux stopped answering` at WARN;
- every session close logs `commit cycle backed off: tmux stopped answering` at ERROR.

The log blames tmux going down when the cause is the session list. These are also the lines that §4.3's reboot reading counts as saves in flight while tmux went down, and the deferred hold (§5.3) waits on that evidence.

The pane listing already separates the two cases: its read is wrapped (`capture.go:104-107`), and its parse (`parsePaneRows`) is not. Phase 1 Task 1 settled that failures which are not refused reads stay `… failed`.

In the tree, `parseSessionList` splits each line with `SplitN(line, "|", 4)` (`internal/tmux/tmux.go:185`). A session named `work|notes` therefore puts `notes` in the window-count slot and returns `invalid window count "notes"` (`:190-193`). The function's three error returns (`:187`, `:192`, `:197`) are the session listing's parse failures. `ListSessionsProbe` wraps only a failed read, as `failed to list tmux sessions` (`:164-167`). `ListSessionNamesProbe` has one production caller, `captureStructure` (`internal/state/capture.go:95`).

**Solution**:
- Only a failed `list-sessions` read is wrapped in `ErrTmuxStoppedAnswering`.
- The tmux client marks a failure to parse the session listing with a sentinel in `internal/tmuxerr`, and `captureStructure` returns that error unwrapped. `classifyFailedCapture`'s confirmation then classifies it the way it classifies a pane-row parse failure: `… failed` when the committer's own server answers, a back-off when it does not.
- Refused reads keep their unconditional wrap and still send no confirmation after them.
- `ErrTmuxStoppedAnswering`'s doc names the session listing's read rather than the listing as a whole.
- Writes, retries, `save.requested` and `flush_completed` do not change. Only the logged line moves.

This is derived from the record:
- phase 1 Task 1: failures that are not refused reads stay `… failed`;
- cycle 1 Task 2: a failure that is not already a refused read is classified by the confirmation;
- §4.2: a back-off line is for a committer that backed off because tmux stopped answering.

`tmuxerr` is the leaf package `internal/state` already uses to tell tmux error classes apart without importing the client. Marking the parse failure there mirrors the pane listing, whose read is wrapped and whose parse is not.

**Outcome**: While the committer's own server answers, a session list Portal cannot parse logs `tick failed` and `commit cycle failed` with the parse error. Every capture read tmux refuses still logs its committer's back-off line.

**Acceptance Criteria**:
- [ ] A live session named `work|notes` sits beside the saved sessions, so tmux answers the session listing with a line Portal cannot parse. The committer's own server answers the read sent after it. The daemon's tick logs `tick failed` at WARN, its shutdown flush logs `final flush failed` at WARN, and `commit-now` logs `commit cycle failed` at ERROR, each with the parse error in `error`. None of them logs a back-off line, and `sessions.json` and every scrollback file are unchanged. (§2.5, §4.2)
- [ ] After that failure, the tick leaves `save.requested` present and its next tick retries. `commit-now` touches `save.requested` and exits non-zero. The shutdown flush's `shutdown` line reports `flush_completed=false`. (§2.5)
- [ ] With the same unparseable listing, the read sent after it is refused, answered naming no server, or answered by another server. The cycle returns an error matching `state.ErrTmuxStoppedAnswering`, with the parse error and the confirmation's cause both reachable from it, and each of the three committers logs its back-off line. A cycle whose own server is unknown sends no confirmation and returns the parse error unchanged. (§2.2, §4.2)
- [ ] A `list-sessions` that tmux refuses still makes the cycle return an error matching `state.ErrTmuxStoppedAnswering`, with tmux's own words reachable from it. The cycle ends at that read and sends no confirmation after it, and each of the three committers logs its back-off line. (§2.1, §4.2)
- [ ] Through the tmux client, a session listing that fails to parse returns an error matching the new `internal/tmuxerr` sentinel. A failed `list-sessions` read returns an error that does not match it and still carries tmux's `*tmux.CommandError`. (§4.2)

**Do**:
- `internal/state/capture.go:95-98`: a `ListSessionNamesProbe` error matching the new `internal/tmuxerr` sentinel is returned unwrapped, as a pane-row parse failure from `parsePaneRows` is (`:108-111`). Any other error from it is a failed `list-sessions` read and keeps its `ErrTmuxStoppedAnswering` wrap.
- `internal/tmuxerr/errors.go`: the sentinel sits beside `ErrNoSuchSession` and `ErrUnaddressableSessionName`. The tmux client marks the session listing's parse failures with it. Those failures are `parseSessionList`'s three error returns (`internal/tmux/tmux.go:187`, `:192`, `:197`).
- `classifyFailedCapture` (`internal/state/scrollback.go:405-416`) does not change. An error that does not match `ErrTmuxStoppedAnswering` already goes through the own-server confirmation, and with the own server unknown it is returned as given.
- `ErrTmuxStoppedAnswering`'s doc (`internal/state/scrollback.go:248-253`) names the session listing's read rather than the session listing.
- Unchanged: the skeleton-marker read's wrap (`internal/state/scrollback.go:353-356`), the pane listing read's wrap (`internal/state/capture.go:104-107`), `cycleFailureMessage` (`cmd/state_daemon.go:244-249`), and the committers' failure routes (`cmd/state_daemon.go:203-209`, `:389-392`; `failCommitNow`, `cmd/state_commit_now.go:151-157`).
- The listing's other readers check no sentinel, so their handling does not change. These are `ListSessions` (`internal/tmux/tmux.go:150-157`), used by the picker, resolver, completion and restore, and the search form's `ListSessionsProbe` call (`cmd/open_search.go:178`).
- Neighbouring cases already pinned: the pane-listing parse case of `TestDaemonTickReportsAFailureThatIsNoRefusedReadAsFailed` (`cmd/state_commit_backoff_test.go:394-436`), which this mirrors; the failed session listing in the `standDowns` table (`:35-73`); `TestRunCommitCycleSendsNoConfirmationAfterARefusedListing` (`internal/state/commit_cycle_confirm_test.go:542-570`); and `TestListSessionsProbe_ReportsAMalformedLine` (`internal/tmux/list_sessions_probe_test.go:115-129`).
