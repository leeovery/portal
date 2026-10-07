TASK: A Cycle That Ends Uncommitted Never Leaves Sessions.json Naming A Missing Scrollback File (killing-all-sessions-wipes-restore-state-1-6, tick-9af916)

ACCEPTANCE CRITERIA:
- A newly waiting pane's saved record names its positional scrollback path. A cycle moves the pane's transcript to its token-named path, and a stand-down is injected after the move. Afterwards every scrollback path sessions.json names is present on disk. This holds whichever committer runs the cycle. (§2.3, §6.1)
- A newly waiting pane's saved record names its positional scrollback path. The daemon's tick moves the pane's transcript to its token-named path and is then cancelled mid-dump by the shutdown signal. The shutdown flush that follows stands down. Afterwards every scrollback path sessions.json names is present on disk. (§2.3, §6.1)
- A newly waiting pane's saved record names its positional scrollback path. A cycle moves the pane's transcript to its token-named path, and its sessions.json write then fails. Afterwards every scrollback path sessions.json names is present on disk. (§2.3, §6.1)
- One of those cycles ends uncommitted at shutdown, and no later cycle commits. At the next restore, the path the waiting pane's record names holds that pane's transcript. (§2.3)

STATUS: issues_found

SPEC CONTEXT: §2.3 (as corrected 2026-10-06) settles the open "how": the re-file of a newly waiting pane's transcript onto its token-named path is a hard link (link(2)) that never overwrites an existing token-named file, and the positional name stays on disk until the housekeeping pass of a commit naming the token-named path removes it. That covers all three uncommitted endings (stand-down, the daemon tick cancelled mid-dump followed by a stood-down shutdown flush, a failed sessions.json write), which confirming before the re-file alone (§2.2) does not. §2.5: a stand-down writes no commit and runs no housekeeping pass. §6.1 asks for tests of all three endings.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/state/scrollback.go:106-142 — refilePendingScrollback / refilePendingPane: the re-file now calls linkStoredScrollback (line 136) and rewrites the record to the token path only on success.
  - internal/state/scrollback.go:148-157 — linkStoredScrollback: os.Link (line 152), with fs.ErrNotExist / fs.ErrExist treated as success (missing source; adopt an existing token-named file).
  - The no-replace rename machinery (rename_noreplace_darwin.go / rename_noreplace_linux.go, moveNoClobber, StubRenameNoReplaceUnsupported) is deleted in a553e731c.
  - internal/state/commit.go:48-58 — commitOver returns on a failed sessions.json write before gcOrphanScrollback, so the positional name survives a failed write.
  - internal/state/commit_cycle.go:145-150 — a Dump error (including cmd/state_daemon.go:311-312's errCycleCancelled) returns before commitOver, so neither a cancelled tick nor any stand-down reaches housekeeping.
  - internal/state/scrollback.go:366-373 — the confirmation still precedes linking and re-filing, so a production stand-down never even links.
- Notes: The approach matches the corrected spec exactly. The safety claim in the doc comment ("safe only because every scrollback writer replaces a name rather than rewriting its file") holds: every scrollback write goes through WriteScrollbackIfChanged -> fileutil.AtomicWrite0600 (temp file + rename, internal/state/scrollback.go:84-85), and no non-test file in internal/ or cmd/ opens a scrollback file for in-place writing. A later committing cycle adopts the existing token-named file (ErrExist) and its housekeeping removes the positional name. CLAUDE.md's description of the re-file matches the code. One compound sub-case of the fourth criterion is not covered; see FINDINGS.

TESTS:
- Status: Adequate
- Coverage:
  - internal/state/commit_cycle_uncommitted_test.go:110 — a 2x2 matrix: the daemon-shaped and commit-now-shaped cycles, each ending on a stand-down injected after the re-file and on a failed sessions.json write (state dir chmod 0500 after the re-file; AtomicWrite0600 creates its temp file in that dir, so the write fails). Each case asserts that the dump saw the token-named file holding the transcript, that sessions.json is byte-identical, that every path sessions.json names exists, and that X's named path holds X's transcript (criteria 1, 3 and 4).
  - internal/state/commit_cycle_uncommitted_test.go:154 — the follow-up committing cycle files X under its token alone and removes the positional name. This pins that the linked name does not linger.
  - cmd/state_daemon_cancelled_refile_test.go:63 — the real tick/dump: cancel lands during pane 0.0's capture-pane, the loop returns errCycleCancelled at pane 0.1, then defaultShutdownFlush stands down on a refused confirmation (flush_completed=false asserted). It asserts every named path exists and the waiting pane's named path holds its transcript (criteria 2 and 4).
  - internal/state/scrollback_test.go and internal/state/capture_refile_test.go are updated to assert the positional file still holds the bytes after the re-file. The displacer subtest now replaces the positional name the way the dump does (replaceScrollback).
  - A reverted rename would fail every one of these tests, because the positional path sessions.json names would be gone.
- Notes: The fixtures only cover a waiting pane still at the address its saved record names. Nothing covers a pane at that address being written by the dump before the cycle ends uncommitted (see FINDINGS). No over-testing: the matrix is the committer-by-ending cross-product the first criterion asks for, and the assertion helpers are shared.

CODE QUALITY:
- Project conventions: Followed (no t.Parallel; test-only exports kept in export_test.go; logging via the injected logger; repo-relative stored paths through joinStored)
- SOLID principles: Good — the change removes a strategy parameter and a platform split in favour of a single link primitive shared by the re-file and linkMovedPane
- Complexity: Low
- Modern idioms: Yes
- Readability: Good
- Issues: None beyond the finding below. The comments in the changed code hold against it.

BLOCKING ISSUES:
- None

FINDINGS:
- [in-scope] [spreading] internal/state/commit_cycle.go:90 — The waiting pane's positional name is left in place only for the uncommitted record to fall back on, but the same cycle's dump can replace that name with another pane's capture before the cycle ends uncommitted. The doc comment at internal/state/scrollback.go:94-95 says dropping the dedup entry "is what lets the next pane to occupy that address write its own file there". ScrollbackWriter.Write always writes the live pane key's positional path through WriteScrollbackIfChanged (internal/state/scrollback.go:84). Most reachable trigger: restore brings a session with a window-index gap back one window lower (the noncontiguous-window case). A lazy pane X saved at work:2.0, whose record names scrollback/work__2.0.bin, comes back waiting at work:1.0. A pane Y saved at work:3.0 now sits at work:2.0. On the first tick after restore, X's record is merged on its token and keeps work__2.0.bin (internal/state/capture.go:363). The re-file links work__2.0.bin to pane-<X>.bin. The dump then writes Y's capture over the name work__2.0.bin. If that tick is cancelled after Y's write (any pane after Y in dump order hits the ctx check at cmd/state_daemon.go:311-312), and the shutdown flush stands down, sessions.json still records X at 2.0 naming work__2.0.bin. That file now holds Y's bytes, and X's own transcript survives only as the unnamed pane-<X>.bin. A failed sessions.json write after Y's dump ends the same way. Fix: closing it needs a decision with more than one defensible form. One form keeps the dump from replacing a positional name that the cycle's previous index still names for a different pane until a commit lands, for example by writing that capture to another name and filing it at commit. Another records the case as accepted residue in §5.2. Either form needs a test with a moved waiting pane and a second pane at its old address — FAILS: at the next restore X replays Y's transcript, and the first commit after that restore deletes X's real transcript (pane-<X>.bin) as an unreferenced orphan. The fourth criterion ("the path the waiting pane's record names holds that pane's transcript") fails for this case. It is reachable only when shutdown lands within roughly the first tick after restoring such a session.

UNSETTLED:
- None
