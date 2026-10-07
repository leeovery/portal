TASK: The Answered-Pane Hold Is Decided From The Last Committed Index (killing-all-sessions-wipes-restore-state-4-1, tick-0875ce)

ACCEPTANCE CRITERIA:
1. sessions.json names an answered pane's token-named transcript (filed by a commit-now while it waited); the daemon's previous index predates that commit and names the pane at its old positional path; the pane's live address names a positional file on disk holding another pane's old transcript. With an empty capture whose following read is refused / answered by another server / answered naming no server, or a refused capture with nothing dumped, the daemon's tick and its shutdown flush each write nothing for the pane, commit its record naming the token-named transcript, and leave that file on disk with its bytes. (§2.4)
2. From the same start, a dump that writes the pane's capture with confirmed bytes (non-empty, or empty and confirmed by the own server against the token-named transcript) commits the record naming its positional file holding that capture, and that commit's housekeeping removes the token-named transcript. (§2.4)
3. With sessions.json naming the token-named transcript, a daemon tick whose previous index names that transcript and one whose previous index predates its filing commit the same record and leave the same files, whether the positional path is absent or holds another pane's file; a commit-now in the same state commits the same record. (§2.4)
4. With sessions.json absent or not decodable, the hold follows the caller's previous index: a record carrying the token and naming the token-named transcript holds; one naming the positional path leaves the empty capture judged at the positional file. (§2.4)
5. Skeleton, waiting-pane and carry merges still take their records from the caller's previous index. Drops and change are measured against the last committed index (one INFO `session dropped` per lost session, none for a rename, no write for a matching capture). Existing merge, drop-log and no-change tests pass unchanged. (§4.1)
6. `rg -n 'holdTokenFiledTranscripts' internal cmd` finds nothing. (§2.4)

STATUS: complete

SPEC CONTEXT: §2.4 (as corrected 2026-10-06) defines a pane's saved transcript as the file its last committed record names; from a lazy pane's answer until a dump writes its new capture with confirmed bytes, every commit must name the token-named transcript and leave it on disk, whether the capture is empty, refused, or no dump runs (commit-now). §4.1 measures dropped sessions against the on-disk index rather than the daemon's in-memory index, because that index does not see commit-now's writes. The task applies the same "last committed index" measure to the answered-pane hold, reusing the sessions.json read Commit already made.

IMPLEMENTATION:
- Status: Implemented (later evolved, soundly, by tasks 6-2, 7-1, 7-2 and 8-1)
- Location:
  - internal/state/commit_cycle.go:121-126 — sessions.json read once under the commit lock (`readPriorIndex`), falling back to `LoadPrev` (with a WARN via `logUnreadIndex`, :156-163) only when it is absent or unreadable
  - internal/state/commit_cycle.go:131-133 — the hold (`keepAnsweredTranscripts`, :175-209) decided from that index
  - internal/state/commit_cycle.go:150 — `commitOver(cycle.Dir, capture.Index, committed, ...)` measures the no-change skip and drop lines against the same read; internal/state/commit.go:35-61 (`commitOver`), :65-75 (`readPriorIndex`); `Commit` (:28-31) remains a test-only wrapper doing its own read
  - `heldTranscripts` (internal/state/commit_cycle.go:237-251), the writer's `held` map (:60-62, :79-86) and `fileAtPositional` (:253-269) are intact
  - `holdTokenFiledTranscripts` and its `fileExists` helper are gone from internal/state; `rg` finds no `holdTokenFiledTranscripts` in internal or cmd (the `fileExists` at cmd/state_daemon.go:397 is an unrelated cmd-package helper)
  - cmd/state_daemon.go:342-346 — the daemon's dump logs an unconfirmed empty capture and moves on, so a refused write leaves the record on the held file
- Notes:
  - Criterion 1 traced: with sessions.json naming `pane-T.bin`, `keepAnsweredTranscripts` finds the pane's record by token, sees it differs from the fresh positional record, and (claims == 1, `linkHeldTranscript` returning the token path at :224-226) points the record at `pane-T.bin`. `heldTranscripts` then routes the writer's empty-capture check to `pane-T.bin` (:79-87), an unconfirmed empty capture is refused, nothing is captured, and the commit names `pane-T.bin`, so housekeeping keeps it. A refused capture never reaches the writer and ends the same way.
  - Criterion 2 traced: a confirmed write lands at the positional path, `captured` records it (:91-93), `fileAtPositional` re-points the record, and `gcOrphanScrollback` removes `pane-T.bin` once nothing names it.
  - Criterion 3 now holds by construction: over a readable sessions.json, `LoadPrev` is never called, so the daemon's stale or current in-memory index and commit-now's loader cannot produce different results.
  - Criterion 5's first sentence ("merges still take their records from the caller's previous index") no longer describes the code. Task 6-2 (tick-adfe17, "Each Commit Cycle Merges And Carries From The Index It Read Under The Lock") deliberately reversed it, with a traced carry-path loss as its reason. Merges, carry and hold now all read the one locked index, and `LoadPrev` is used only as the fallback. This is a sound, recorded divergence, not a loss. Aliasing is safe: the merges read `prev` by value, `carriedSession` (internal/state/capture.go:245-270) deep-copies, and `canonicalPrevPanes` builds new entries without sorting `prev` in place. So `committed`, shared with `prev`, reaches `commitOver` unmutated and still measures against the last commit.
  - The hold's rule has been widened since this task (7-1/7-2): any tokened pane whose last record names a file other than its live positional file is held, not only one naming `PendingScrollbackFile(token)`. That is a superset of this task's rule and covers its scenario unchanged.

TESTS:
- Status: Adequate
- Coverage:
  - Criterion 1 (state level): internal/state/commit_cycle_answered_disk_test.go:70 covers four unanswered dumps (refused confirmation, another server, no server, refused capture) with a stale previous index and an occupied positional path. It asserts no write, positional file not overwritten, and the record, bytes and restore read all on the token-named transcript.
  - Criterion 1 (daemon level): cmd/state_daemon_empty_capture_test.go:270 runs the same four cases through both the real `tick` and `defaultShutdownFlush`, with the daemon's `PrevIndex` predating the token filing and the positional file occupied. Under the pre-change code both fixtures reproduce the loss (the stale record names the positional path, and the occupied file defeated the existence check), so they would fail if the fix regressed.
  - Criterion 2: internal/state/commit_cycle_answered_disk_test.go:92 covers a non-empty capture and an own-server-confirmed empty capture over an occupied positional path, asserting the positional record and contents, the token-named file removed, and the confirmation count.
  - Criterion 3: internal/state/commit_cycle_answered_disk_test.go:145 compares the committed record and the whole scrollback directory across a current-index tick, a stale-index tick and a commit-now, for both positional states.
  - Criterion 4: internal/state/commit_cycle_answered_disk_test.go:199 covers sessions.json absent and corrupt, each with a previous record naming the token transcript (held, refusal judged there) and one naming the positional path (written there with no confirmation).
  - Criterion 5: no merge, drop-log or no-change test file was touched by this task's commit (01b56b8c2 changed only cmd/state_daemon_empty_capture_test.go, which it extended, and added commit_cycle_answered_disk_test.go). Drop and no-change measurement through the cycle is exercised by cmd/state_commit_drop_log_test.go and internal/state/commit_cycle_unread_index_test.go:168.
  - The existing stale-prev, positional-absent tests (internal/state/commit_cycle_answered_test.go:363 and cmd/state_daemon_empty_capture_test.go:209) remain. The second subtest at internal/state/commit_cycle_answered_test.go:332 seeds no sessions.json and so runs on the `LoadPrev` fallback, as the task anticipated.
- Notes: The positional-absent and positional-occupied cases recur across the 3-5 tests and the three new state-level tests, but each table pins a distinct criterion. This is not bloat.

CODE QUALITY:
- Project conventions: Followed (no t.Parallel; fixtures go through RunCommitCycle; state.Commit appears only in test seeding)
- SOLID principles: Good — the one locked read feeds the hold and the commit measure; commitOver separates "measure against this prior" from Commit's own read
- Complexity: Acceptable
- Modern idioms: Yes
- Readability: Good — the RunCommitCycle and CommitCycle.LoadPrev doc comments accurately state the single-index rule and the fallback
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "The existing merge, drop-log and no-change tests pass unchanged." — reading confirms this task's commit left those tests unchanged, and the code still measures against the locked read. Whether they pass needs a run of `go test ./internal/state ./cmd` covering commit_drop_log_test.go, cmd/state_commit_drop_log_test.go, and the capture/merge suites (capture_test.go, capture_carry_test.go, commit_cycle_skip_test.go, commit_cycle_prev_test.go).
