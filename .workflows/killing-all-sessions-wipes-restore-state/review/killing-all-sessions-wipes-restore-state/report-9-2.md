TASK: A Deferred Pane's Capture Is Written Once The Commit That Frees Its Name Has Landed (killing-all-sessions-wipes-restore-state-9-2, tick-6e97ab)

ACCEPTANCE CRITERIA:
- Tokenless A, saved at work:3.0 and live at work:2.0, beside tokened B, saved at work:2.0 and live at work:1.0. Once the cycle commits, A's record names scrollback/work__2.0.bin holding A's capture. B's record names a file holding B's own transcript: B's capture where it was written, otherwise B's saved bytes under its token-named transcript. That commit's housekeeping removes work__3.0.bin. The daemon's first tick over the same renumbered restore, with B skeleton-marked, leaves work__2.0.bin holding A's capture and B's token-named transcript holding B's saved bytes.
- A new pane opened at an address a tokened pane vacated, carrying no token or one the pane-token rule refuses. Once the cycle commits, its record names its positional file holding its own capture. Its dedup entry is unchanged through the dump and describes that capture once the cycle ends. The tokened pane's record names its token-named transcript holding its saved bytes.
- A cycle that defers a capture and then ends uncommitted writes no deferred capture. This covers a failed dump, a failed sessions.json write, and a daemon tick cancelled mid-dump followed by a shutdown flush that stands down. In each case sessions.json is unchanged and the contested positional file still holds the contesting pane's saved transcript.
- A committing cycle that defers a capture but skips the sessions.json write because nothing changed writes no deferred capture.
- A deferred pane is not written when the index just committed still names its contested name on a record carrying another pane's token (the contesting pane's skeleton link failed). The file still holds the contesting pane's saved transcript.
- A deferred pane's unconfirmed empty capture over the contested file is refused at dump time with an error wrapping ErrUnconfirmedEmptyCapture, and the committing cycle writes nothing at that pane's positional name.

STATUS: complete

SPEC CONTEXT: Section 2.3 requires that the next restore finds each pane's transcript at the path its record names. Section 2.4 says a pane's saved transcript is the file its last committed record names, and an empty capture replaces it only once confirmed. Section 5.2 accepts that a tokenless pane moved after restore can lose its transcript only until its next save completes. Before this task, the deferral left a committed tokenless record naming a neighbour's bytes until the next committing dump, and after a reboot in that window the neighbour's history replayed into the pane. The task revises the phase's earlier position: the deferred capture is written once the commit lands, and a cycle that ends uncommitted still writes nothing. No spec corrigendum is owed.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/state/commit_cycle.go:129-143. `writeContested` now runs `confirmEmptyCapture` first. A pane whose token `PendingScrollbackPath` refuses (or which has none) then keeps a cloned capture and its hash in `w.deferred` and returns (false, nil), with the dedup entry left alone.
  - internal/state/commit_cycle.go:80-87. The new `deferred` field and the `deferredCapture` type.
  - internal/state/commit.go:37-63. `commitOver` now returns (bool, error) and reports false on the nothing-changed early return at :46-48. `Commit` (:28-32) discards the bool.
  - internal/state/commit_cycle.go:213-219. `RunCommitCycle` calls `writeDeferredCaptures` only when `commitOver` reports the write. A failed dump (:208-210) or a failed commit (:214-216) returns first, and the deferred map is local to the call, so nothing carries over.
  - internal/state/commit_cycle.go:225-252. `writeDeferredCaptures` writes a kept capture only when the committed index names its positional file on that pane's record alone. It drops the dedup entry, writes through `WriteScrollbackIfChanged` (which sets the entry to the capture's hash), and logs a failure at WARN without failing the committed cycle.
- Notes:
  - Traced against the acceptance criteria:
    - The write happens after `gcOrphanScrollback`. The positional file survives that pass because the deferred pane's committed record names it.
    - `AtomicWrite0600` replaces by rename (internal/fileutil/atomic.go:72-89), so the contesting pane's token-named hard link to the same inode keeps its saved bytes.
    - The daemon's dump is sequential (cmd/state_daemon.go:303-325), so the shared map writes are safe.
    - The writer refuses an unconfirmed empty capture before it reaches the deferred map, which meets the sixth criterion.
  - Divergence, judged sound: the skip rule is stricter than the task's wording. The task skips a name the committed index names on another pane's tokened record. The code writes only when the deferred pane's record is the sole record naming the file. That also leaves the file alone when a tokenless record (for example a pending pane's merged record) names it, or when the deferred pane's own record names another file (a refused-token pane held on its previous file). Both are cases where writing would overwrite bytes some other record still names, or would write a file no record reads. It is a superset of the required protection and loses nothing the intent needs.
  - Divergence, judged sound: the `wrote` gate cannot change the outcome. A commit is skipped only when the capture index equals the committed index, and then the contesting tokened record still names the contested file, so the skip rule already blocks the write. The gate is what the task's Do asks for and it costs nothing.

TESTS:
- Status: Adequate
- Coverage:
  - Criterion 1, state level: internal/state/commit_cycle_contested_test.go:322 (`TestRunCommitCycleWritesAMovedTokenlessPaneAtAContestedNameOnceItsCommitHasLanded`). It covers B both unwritten (held on its token transcript) and written (its own capture at work__1.0). It checks that the file is untouched during the dump, that A's record and file are correct after the commit, and that work__3.0.bin is gone.
  - Criterion 1, daemon level: cmd/state_daemon_run_test.go:640-668. The expectation at :653-655 moved to "x-captured", with B's token file still "y-saved".
  - Criterion 2: internal/state/commit_cycle_contested_test.go:239. It covers both no-token and refused-token occupants. The dedup entry is checked as unchanged inside the dump (:250-252) and equal to the capture's hash afterwards (:264-266). The tokened pane is held on its token transcript, and a follow-on cycle gets a dedup hit.
  - Criterion 3:
    - internal/state/commit_cycle_contested_test.go:284 covers the failed dump and the failed sessions.json write, for both occupant kinds. The state dir is chmodded while the scrollback subdir stays writable, so a premature deferred write would be caught.
    - cmd/state_daemon_contested_test.go:48 (subtest "Y carries no token", :60) covers the cancelled tick followed by a stood-down flush.
    - internal/state/commit_cycle_shifted_test.go:239-294 keeps the uncommitted-end assertions.
  - Criterion 4: internal/state/commit_cycle_contested_test.go:446.
  - Criterion 5: internal/state/commit_cycle_contested_test.go:424. B is skeleton-marked and its link onto its token is refused. Removing the skip rule would fail this test.
  - Criterion 6: internal/state/commit_cycle_contested_test.go:365. It runs all three refusal shapes, asserts exactly one confirmation read, and checks that the contested file still holds B's bytes.
  - Updated sites: commit_cycle_shifted_test.go:188-237 (the same-cycle row now expects the deferral and then the write), cmd/state_daemon_resume_pending_test.go:413-418, and the movedClient skeleton support in commit_cycle_moved_test.go:35-57.
- Notes: The criterion-4 test (contested_test.go:446) would still pass without the `wrote` gate, because the skip rule blocks the write in the same state. As noted under IMPLEMENTATION, no reachable state separates the two, so this is not a gap. There is one test per criterion and no redundant variants.

CODE QUALITY:
- Project conventions: Followed. The write still goes through the cycle's writer and `WriteScrollbackIfChanged` inside `internal/state`, under the commit lock. The WARN reuses the daemon's existing message and attr keys (`pane_key`, `error`).
- SOLID principles: Good
- Complexity: Low
- Modern idioms: Yes (`bytes.Clone`, `slices.Equal`)
- Readability: Good. The doc comments on `ScrollbackWriter`, `Write`, `writeContested`, `RunCommitCycle`, `commitOver` and `writeDeferredCaptures` match the code.
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
