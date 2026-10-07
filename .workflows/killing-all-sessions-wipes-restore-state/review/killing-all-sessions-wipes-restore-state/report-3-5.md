TASK: An Answered Lazy Pane Keeps Its Transcript Until A Confirmed Save Replaces It (killing-all-sessions-wipes-restore-state-3-5, tick-bc5cc6)

ACCEPTANCE CRITERIA:
Each scenario starts from a lazy pane carrying token T whose transcript was committed under scrollback/pane-T.bin while it waited, and whose resume-pending marker the user has since cleared by answering it.
1. First dumping cycle after the answer (tick and shutdown flush alike): capture-pane comes back empty and the read sent after it is refused, answered by another server, or answered naming no server — nothing is written, the existing `empty capture not confirmed; saved transcript kept` WARN names the pane in pane_key, and the commit names the token-named file, still on disk with its bytes.
2. First dumping cycle after the answer: capture-pane is refused or its write fails — the commit names the token-named file, still on disk, and every later committing cycle that has not written the capture (tick, shutdown flush, commit-now) keeps naming it.
3. First commit after the answer is a commit-now and tmux exits before any dump — sessions.json names the token-named transcript, present with its bytes.
4. First dumping cycle writes the capture with confirmed bytes (non-empty, or empty and confirmed by its own server, judged against the token-named transcript so the confirmation is sent though no positional file exists) — the commit names the positional file holding it and housekeeping removes the token-named file.
5. However a cycle after the answer ends (committed, stood down, cancelled mid-dump, failed sessions.json write — including after its dump wrote the capture), sessions.json never names the positional file while it is missing; the token-named transcript is deleted only by the housekeeping of a commit naming a positional file holding the pane's capture.
6. A pane that never waited and a pane still waiting are saved as today: an ordinary pane's empty capture is judged at its own positional file; a waiting pane is skipped by the dump and stays filed under its token.

STATUS: complete

SPEC CONTEXT: §1.1 — every session not killed stays restorable with its scrollback. §2.3 — no cycle ending uncommitted may leave sessions.json naming a missing file. §2.4 (as corrected 2026-10-06) — a pane's saved transcript is the file its last committed record names; for a just-answered lazy pane that is its token-named file, and from the answer until a dump writes the pane's new capture with confirmed bytes every commit names the token-named transcript and leaves it on disk, whether the capture is empty, refused, or no dump runs (commit-now). §3.2 — the three-step loss through a dump-less commit-now before shutdown. §4.2 — the refused empty write is logged.

IMPLEMENTATION:
- Status: Implemented (the delivered shape has since been reworked by tasks 4-1 and 7-1, which moved the hold into the commit cycle and decide it from sessions.json read under the commit lock; the current tree meets every criterion of this task)
- Location:
  - internal/state/commit_cycle.go:131-133 — RunCommitCycle runs keepAnsweredTranscripts after captureAndRefile, for every committer (daemon tick, shutdown flush, commit-now).
  - internal/state/commit_cycle.go:175-209 — keepAnsweredTranscripts: a tokened, non-skipped pane whose previous record (matched on token) names a file other than its fresh positional one is pointed back at it; for an answered pane the stored file is already pane-T.bin, so linkHeldTranscript (:219-232) returns it unchanged and :205 sets it unconditionally.
  - internal/state/commit_cycle.go:136-148 — the writer is built with heldTranscripts (:237-251) and, after the dump, fileAtPositional (:253-269) moves only the panes whose capture was written back to their positional file. commit-now passes no Dump (cmd/state_commit_now.go:110-117), so its commit keeps the token-named record.
  - internal/state/commit_cycle.go:74-95 — ScrollbackWriter.Write judges a held pane's empty capture against the stored token-named file (:78-81, confirmEmptyCapture at internal/state/scrollback.go:295-303 now takes the saved path), drops the dedup entry for a held pane (:85) so a capture identical to bytes written by a cycle that ended uncommitted still writes, and records a successful held write in `captured` (:91-93).
  - cmd/state_daemon.go:342-347 — a refusal still surfaces as ErrUnconfirmedEmptyCapture through the existing `empty capture not confirmed; saved transcript kept` WARN with pane_key.
- Notes: Traced each criterion against the tree. Empty-and-unconfirmed, refused capture, and failed write all leave `captured` unset, so the record stays on pane-T.bin and housekeeping (internal/state/commit.go:131-166) keeps it. An uncommitted cycle (stand-down, cancel, failed write) leaves sessions.json and pane-T.bin as the last commit left them; the next cycle re-holds from sessions.json and the dedup drop forces the rewrite. Ordinary panes are untouched: a fresh record whose previous record names its own positional file is skipped at :191, and waiting/skeleton/carried panes are excluded through SkipsScrollback at :187, :243 and :75. Restore replays from the record's path (internal/restore/session.go:79), so a held record restores the transcript.

TESTS:
- Status: Adequate
- Coverage:
  - AC1: internal/state/commit_cycle_answered_test.go:119 (all three refusal kinds, positional never written, record and bytes on pane-T.bin); cmd/state_daemon_empty_capture_test.go:209 and :270 (daemon tick and shutdown flush, WARN with pane_key via assertEmptyCaptureRefusedLine, all three refusal kinds plus a refused capture in the :270 suite).
  - AC2: internal/state/commit_cycle_answered_test.go:154 (refused capture and failed write, followed by a later tick, a commit-now and a dump-less flush, each still naming pane-T.bin).
  - AC3: internal/state/commit_cycle_answered_test.go:209.
  - AC4: internal/state/commit_cycle_answered_test.go:220 (non-empty, and empty-confirmed with exactly one confirmation read sent against the token-named file; positional named, token file collected).
  - AC5: internal/state/commit_cycle_answered_test.go:256 (dump wrote X's capture, then stand-down / cancel / failed sessions.json write; then commit-now; then a tick that files it at positional — this also exercises the dedup drop, since the same bytes and hash map are reused).
  - AC6: internal/state/commit_cycle_answered_test.go:311 for the ordinary pane; the waiting pane is held by the unchanged TestCaptureAndCommit_RefilesResumePendingScrollback cases (cmd/state_daemon_resume_pending_test.go:545, :574) and the skip suite (internal/state/commit_cycle_skip_test.go:136).
  - The existing tests the task named (cmd/state_daemon_resume_pending_test.go, internal/state/empty_capture_test.go) are unmodified since before the task; cmd/state_daemon_empty_capture_test.go only gained cases.
- Notes: Each test would fail if its mechanism broke — removing the hold, judging at the positional path, skipping fileAtPositional, or dropping the dedup delete each trips an assertion. Test size is proportionate to six scenario criteria.

CODE QUALITY:
- Project conventions: Followed (no t.Parallel, logtest Sink queries composed by filter, production logging through the existing daemon line, no new log keys)
- SOLID principles: Good — the guarantee sits in the one commit cycle every committer runs, as the task directed
- Complexity: Acceptable — the per-pane index walks follow the package's existing nested-loop shape
- Modern idioms: Yes
- Readability: Good
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
