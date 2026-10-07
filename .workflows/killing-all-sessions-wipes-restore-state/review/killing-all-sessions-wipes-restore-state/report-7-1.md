TASK: A Pane Restored Away From Its Saved Address Keeps Its Transcript Until A Confirmed Save Replaces It (killing-all-sessions-wipes-restore-state-7-1, tick-8fc847)

ACCEPTANCE CRITERIA:
1. A daemon tick or shutdown flush takes an empty capture of the moved pane, and the read after it is refused, answered by another server, or answered naming no server. The cycle writes nothing for the pane and logs the refused write as today. The commit names the saved file, which keeps its bytes, and nothing is written at the pane's live positional path.
2. Cycles that reach the moved pane without writing its capture (refused capture-pane, dump-less commit-now, dump-less shutdown flush) each name the saved file and leave it on disk with its bytes, until a dump writes the pane's capture.
3. A dump writes the moved pane's capture (non-empty, or empty and confirmed by the committer's own server against the saved file with one confirmation read). The commit names the live positional file holding that capture, and that commit's housekeeping removes the old file.
4. A tokened pane whose saved file another record in the capture names is judged at its own positional file as today; no commit names one file on two records.
5. A tokenless pane restored one window lower is judged at its own positional file as today.
6. An answered lazy pane whose last committed record names its token-named transcript is held exactly as today.

STATUS: issues_found

SPEC CONTEXT: Section 2.4 says a pane's saved transcript is the file its last committed record names, and that an empty capture may replace it only once the committer's own server confirms a read sent after the capture. Its corrigendum paragraph (spec line 98 onward) extends this to a pane now at a different address from its last committed record: a pane restore brought back one window lower, and a tokened pane tmux renumbered or whose session was renamed. Until a dump writes such a pane's capture with confirmed bytes, every commit keeps those bytes on disk and names them on the pane's record, under the pane's token-named transcript where it carries a token. Section 2.3 requires that a cycle ending uncommitted never leaves sessions.json naming a file that is not on disk.

IMPLEMENTATION:
- Status: Implemented. The code has deliberately moved past criteria 4 and 5, and both later changes are sound.
- Location:
  - internal/state/commit_cycle.go:175-209 (keepAnsweredTranscripts). Candidates are every tokened pane the dump may write whose last committed record, matched on its token, names a file other than its live positional file (:187-195). The hold is taken on that file, or on its token-named second name (:200-207).
  - internal/state/commit_cycle.go:219-232 (linkHeldTranscript, added by task 7-2)
  - internal/state/commit_cycle.go:237-251 (heldTranscripts): held is now any tokened, unskipped pane whose record names a non-positional file.
  - internal/state/commit_cycle.go:74-95 (ScrollbackWriter.Write). It judges an empty capture against the held file, drops the dedup entry and records a confirmed write in `captured`.
  - internal/state/commit_cycle.go:253-269 (fileAtPositional). This is the hand-back.
  - internal/state/commit_cycle.go:131-149 (call order): the hold is taken before the writer is built, on every committing cycle, including commit-now's dump-less one.
- Notes:
  - Criteria 1-3 and 6 hold against the code.
    - Criterion 1: a refused empty capture returns ErrUnconfirmedEmptyCapture, and cmd/state_daemon.go:343-346 logs it as before.
    - Criterion 2: a pane the dump never writes keeps its held record on every commit.
    - Criterion 3: a confirmed or non-empty write lands at the positional path through WriteScrollbackIfChanged, the record moves there, and gcOrphanScrollback (internal/state/commit.go:131) reclaims both the token-named name and the old positional name.
    - Criterion 6: a record already on PendingScrollbackFile(token) short-circuits in linkHeldTranscript (:224-226) and is held unconditionally (:205).
  - Divergence from this task's wording on what the commit names. The current code does not name "the saved file". It hard-links that file to the pane's token-named transcript and names that. Task 7-2 (tick-460a44) introduced this so that a later write to the original name replaces the name and leaves the moved pane's inode intact. The specification corrigendum now says "under the pane's token-named transcript where the pane carries a token". The bytes and the guarantee are the same, so this is not a loss.
  - Criterion 4 was deliberately reversed by task 7-2. With the link, a pane whose saved file another record names is now held under its token rather than judged at its positional file. 7-2 measured the loss the old rule caused in a shifted run of two or more windows. The fallback to the old rule (`named[file] == 0 && claims[file] == 1`, :205) survives for a failed link or a token the pane-token rule refuses. The two-panes-claim-one-file exclusion stands (:202). No commit names one file on two records. This is sound.
  - Criterion 5 was deliberately superseded by task 7-3 (tick-410e83). Restore now stamps a token on every pane it recreates, so a restored pane is tokened and held. At the commit-cycle level a tokenless pane still skips the hold (:187), and byToken never indexes an empty token (internal/state/capture.go:392), so a tokenless renumbered pane is still judged at its positional file. This is sound.
  - Edge checks that pass on reading:
    - Map iteration over `holds` has no order dependence, because `named` and `claims` are computed before any link.
    - A missing source file is treated as linked, which is consistent with refilePendingPane's convention, and no bytes exist to lose.
    - A link left behind by a cycle that ends uncommitted is adopted on the next cycle through the ErrExist path.
    - AtomicWrite's rename keeps a later write at the old positional name from touching the linked inode.
    - Restore reads the record's ScrollbackFile verbatim (internal/restore/session.go:79), so a held pane restores from its token-named transcript.

TESTS:
- Status: Adequate
- Coverage:
  - internal/state/commit_cycle_moved_test.go:233-257 covers criterion 1: three moves (gap closed, renumbered, session renamed) across three refusals (refused, another server, no server). It asserts the ErrUnconfirmedEmptyCapture refusal, exactly one confirmation read, the record on the token-named transcript holding the saved bytes, nothing at the live positional path, and no file on two records.
  - :259-290 covers criterion 2: a refused-capture tick, a commit-now, a second refused tick and a dump-less flush, each followed by the held-transcript assertions.
  - :292-340 covers criterion 3: a non-empty write (zero confirmation reads) and an empty write the own server confirms (one confirmation read). It asserts the record and file at the live positional path, and that both the saved file and the token-named file were removed.
  - :342-392 covers criterion 4 as task 7-2 reversed it. :394-443 keeps the both-claim-one-file exclusion.
  - internal/state/commit_cycle_shifted_test.go adds the shifted-run, swap, new-pane-at-the-vacated-address, uncommitted-after-link, link-failure and refused-token cases.
  - Criterion 6 is covered by the existing internal/state/commit_cycle_answered_test.go and commit_cycle_answered_disk_test.go suites.
  - The tests would fail if the hold were removed: assertHeldOnTranscript checks the committed record's path and the transcript's bytes.
- Notes: Nothing at the daemon level drives a moved pane, but the daemon's WARN mapping from ErrUnconfirmedEmptyCapture is error-class-driven and already covered by cmd/state_daemon_empty_capture_test.go:75. The composition holds without a dedicated case. The 9-way matrix in the criterion 1 test is the cross-product the criterion itself enumerates, so it is not bloat.

CODE QUALITY:
- Project conventions: Followed
- SOLID principles: Good
- Complexity: Acceptable. keepAnsweredTranscripts has two passes (collect candidates, then decide), with the decision held to one condition.
- Modern idioms: Yes
- Readability: Good. The comment on ScrollbackWriter.held is stale (see FINDINGS).
- Issues: One stale field comment.

BLOCKING ISSUES:
- None

FINDINGS:
- [in-scope] [contained] internal/state/commit_cycle.go:60 — The `held` field comment says it maps a pane key to "the file its last committed record named, which its record still names and its empty capture is judged against". That stopped being true once task 7-2 added the link. In the cycle that links a moved pane, heldTranscripts (:237-251) maps the key to `PendingScrollbackFile(token)`, a path the last committed record did not name, and keepAnsweredTranscripts has already moved the record off the committed name (:205-206). Replace lines 60-61 with: "held maps a pane key to the file holding the bytes its last committed record named — that file, or its second name under the pane's token — which its record now names and its empty capture is judged against." — FAILS: a maintainer tracing why a non-waiting pane's record suddenly names `scrollback/pane-<token>.bin` is told by the writer's own field comment that the held path is the committed path and is unchanged, which the linking cycle falsifies. Comment text only, so it does not block.

UNSETTLED:
- None
