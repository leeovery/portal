TASK: The Dump Never Replaces A Scrollback Name Another Pane's Saved Record Still Names (killing-all-sessions-wipes-restore-state-9-1, tick-4e3d8b)

ACCEPTANCE CRITERIA:
1. Gap-closed restore (X saved work:2.0 → live work:1.0, waiting or not; Y saved work:3.0 → live work:2.0), cycle commits: Y's capture is in Y's token-named transcript and Y's record names it; nothing written at work__2.0.bin that cycle; X's record names X's token-named transcript holding X's saved bytes; no file on two records.
2. Same layout, cycle ends uncommitted after Y's write (daemon tick cancelled at the per-pane check after Y then a stood-down flush, or a failed sessions.json write): sessions.json unchanged; work__2.0.bin holds X's saved transcript; work__3.0.bin holds Y's; every named path on disk.
3. Once the token-named commit has landed, the next cycle writes Y at work__2.0.bin, Y's record names it, and that commit's housekeeping removes Y's token-named transcript.
4. Two tokened panes swap addresses: each capture, either order, lands in its own token-named transcript; neither positional file written; an uncommitted end leaves each record naming a file holding its own saved transcript.
5. Tokened X moves 2.0 → 1.0 and a tokenless (or refused-token) new pane appears at work:2.0: the dump's write writes nothing and leaves the dedup entry; uncommitted end leaves work__2.0.bin holding X's saved transcript.
6. Nothing contested when sessions.json is absent/unreadable (whatever the fallback names), when the naming committed record carries no token, or when it carries the pane's own token.
7. A contested tokened pane's unconfirmed empty capture over a non-empty saved transcript writes nothing at either name and is refused as today.

STATUS: complete

SPEC CONTEXT: §2.3 requires that however a cycle ends (stand-down, cancelled tick plus stood-down flush, failed sessions.json write), sessions.json never names a file that is missing, and the next restore finds a pane's transcript at the path its record names. §2.4 requires that a pane now at a different address from its last committed record keeps that record's bytes, under its token-named transcript when it carries a token, until a dump writes its confirmed capture, and that an unconfirmed empty capture never replaces a saved transcript. §5.2 accepts loss only for a tokenless pane moved after restore. This task closes the case where the dump overwrote a positional name that the committed index still named on another tokened pane's record. That case was not listed as §5.2 residue.

IMPLEMENTATION:
- Status: Implemented. Task 9-2 later revised criterion 5's committed-cycle clause, and the revision is deliberate and sound (see Notes).
- Location:
  - internal/state/commit_cycle.go:55-62: ScrollbackWriter struct doc describing the contested write
  - internal/state/commit_cycle.go:69-73: `held` field doc, matching the mandated wording verbatim
  - internal/state/commit_cycle.go:74-79: `contested` and `filed` fields
  - internal/state/commit_cycle.go:97-109: Write doc paragraph and the contested branch, taken before the held/dedup path
  - internal/state/commit_cycle.go:127-143: writeContested. It runs confirmEmptyCapture first, then AtomicWrite0600 to PendingScrollbackPath(token), then filed[key] = PendingScrollbackFile(token). A refused or empty token goes to the 9-2 `deferred` map.
  - internal/state/commit_cycle.go:204: the contested set is built from `committed` (sessions.json read under the lock), never from the LoadPrev fallback
  - internal/state/commit_cycle.go:211 and :384-400: fileWritten replaces fileAtPositional, so a contested pane's record names its token file
  - internal/state/commit_cycle.go:351-382: contestedPanes. A nil committed index contests nothing, tokenless committed records are ignored, and a name counts as contested if any naming record carries a token other than the live pane's own.
- Notes:
  - I traced every criterion through the code:
    - Y is held on a link of work__3.0 under its token. AtomicWrite then replaces that name, leaving work__3.0's inode with Y's saved bytes. That covers criteria 1 and 2.
    - After the commit, Y's committed record names pane-<Y>.bin, so the next cycle finds work__2.0 uncontested. The existing hold then deletes the dedup entry, writes the positional file and files it there, and housekeeping collects the token file. That covers criterion 3.
    - In the swap case both panes are contested and both go to their token files. That covers criterion 4.
    - For the cancelled-tick ending, the stood-down flush returns from captureAndRefile before any dump (internal/state/scrollback.go:366-367), so sessions.json is untouched.
  - Criterion 5 drift: it said a committed cycle leaves the deferred pane "as an unwritten capture leaves it", with the write happening only in the next cycle. Task 9-2 (tick-6e97ab) superseded this. It now writes the kept capture at its positional name after the cycle's sessions.json write lands, inside the same locked cycle (commit_cycle.go:217-240). 9-2 gives its reason: a measured regression where a moved tokenless pane's record named a neighbour's bytes and its own file was collected. The parts of criterion 5 that 9-1 owns still hold: nothing is written during the dump, the dedup entry is untouched, and an uncommitted end leaves work__2.0.bin with X's bytes. This is a sound divergence, not a loss.
  - Guard conditions hold. cmd still never calls WriteScrollbackIfChanged; its only production callers are internal/state/commit_cycle.go:120 and :236. The task added no real-tmux test, and 9-1 emits no new log line.

TESTS:
- Status: Adequate
- Coverage:
  - Criterion 1: internal/state/commit_cycle_contested_test.go:84, X waiting and not waiting. It checks work__2.0.bin inside the dump end, Y's record and token file, X held on its token transcript, and that no file is named by two records.
  - Criterion 2: internal/state/commit_cycle_contested_test.go:108 covers a failed dump and a failed sessions.json write, with X waiting and not waiting. cmd/state_daemon_contested_test.go:48 covers the daemon tick cancelled at the pane after Y followed by a stood-down flush (it asserts flush_completed=false, the bytes behind each record, and every named path on disk), with tokened and tokenless Y.
  - Criterion 3: internal/state/commit_cycle_contested_test.go:141.
  - Criterion 4: internal/state/commit_cycle_contested_test.go:167, both write orders, each run once committing and once with each uncommitted ending.
  - Criterion 5: internal/state/commit_cycle_contested_test.go:239 (dedup entry left during the dump) and :284 (uncommitted end), each for a tokenless and a refused-token occupant. Both were reshaped by 9-2.
  - Criterion 6: internal/state/commit_cycle_contested_test.go:484 has four rows. The absent and unreadable rows pass a fallback index that names the contest, so the test would fail if contest were judged against LoadPrev.
  - Criterion 7: internal/state/commit_cycle_contested_test.go:549 asserts both names hold their saved bytes during the dump.
  - Extra case: internal/state/commit_cycle_contested_test.go:462 covers a new tokened pane at a vacated address.
  - Migrated sites: the cases the task named in internal/state/commit_cycle_shifted_test.go (:66, :150, :188, :239) now assert the new behaviour.
- Notes: Each test would fail if its behaviour broke: a positional write instead of the token write, a contest judged against the fallback, a tokenless or own-token record contesting, or a missing empty-capture confirmation on the contested path. No redundant or over-mocked cases. No t.Parallel.

CODE QUALITY:
- Project conventions: Followed. The alternate-name write stays inside internal/state's ScrollbackWriter, and tests use the shared moved-pane fixtures and logtest.
- SOLID principles: Good. Contest detection is a pure function (contestedPanes) kept apart from the write path.
- Complexity: Low. One early branch in Write and one small method.
- Modern idioms: Yes.
- Readability: Good. The comments in the changed code hold against it.
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
