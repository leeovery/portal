TASK: Every Moved Tokened Pane In A Shifted Run Keeps Its Transcript (killing-all-sessions-wipes-restore-state-7-2, tick-460a44)

ACCEPTANCE CRITERIA:
1. Shifted run (saved 0.0/2.0/3.0, live 0.0/1.0/2.0): the 2→1 pane's empty capture, whose follow-up read is refused, answered by another server, or answered naming none, is refused with ErrUnconfirmedEmptyCapture. Nothing is written at work__1.0.bin, and the commit names scrollback/pane-<token>.bin holding its saved bytes. This holds whether or not the 3→2 pane is written to work__2.0.bin in the same dump.
2. Same layout: a tick with every moved pane's capture-pane refused, then a dump-less commit-now, then a dump-less shutdown flush. Each commit names every moved pane's token-named transcript, still on disk with its saved bytes, and no saved transcript is deleted.
3. Swap (X 1→2, Y 2→1): X's empty capture is refused and Y's capture is written. X's record names its token-named transcript holding X's bytes, never Y's. Y lands at work__1.0.bin without touching X's bytes.
4. A moved pane (2→1) has a new tokenless pane at 2.0, arriving in the same cycle or in a later one. The moved pane is held on its token-named transcript and the new pane names work__2.0.bin. The new pane's write leaves the moved bytes intact, and no file sits on two records.
5. A written capture (non-empty, or empty and confirmed with one read against the token-named transcript) is filed at the live positional file. Housekeeping removes the token-named transcript and the saved file wherever no other record names it.
6. A cycle that linked and then ends uncommitted (failed dump after the occupant's write, or a failed sessions.json write) leaves sessions.json naming only files on disk. The next committing cycle names the token-named transcript holding the saved bytes.
7. A link failure other than a missing source or an existing token file falls back to the old judgement and emits one WARN with pane_key/path/error.
8. Unchanged: two moved panes sharing one last-recorded file are each judged at their own positional file. A token the rule refuses is judged as before. An answered lazy pane on its token-named transcript is held exactly as before.

STATUS: issues_found

SPEC CONTEXT: §2.4 says a pane's saved transcript is the file its last committed record names. A pane now at a different address from that record keeps that file's bytes on disk and named on its record, under its token-named transcript where it carries a token, until a dump writes its capture with confirmed bytes. Corrigendum 2026-10-06 (phase 7 walk, F1) records the shifted-run loss measured against HEAD and the user's decision to close it. §2.3 requires that no cycle ending uncommitted leaves sessions.json naming a missing file.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/state/commit_cycle.go:132 (call now passes cycle.Dir, cycle.Logger), :175-209 (keepAnsweredTranscripts), :211-214 (heldFile), :219-232 (linkHeldTranscript), :110-113 (RunCommitCycle doc). It reuses linkStoredScrollback (internal/state/scrollback.go:148-157) and PendingScrollbackPath (internal/state/paths.go:108-113).
- Notes: Every candidate with claims==1 is linked before the writer is built (:202-204), whatever `named` holds, and the unconditional token-named branch (:205) then holds it. claims>1 skips the link and falls through to today's rule, so the both-name-one-file case is unchanged. A refused token or a link error returns the stored path to today's `named==0 && claims==1` rule, with the WARN at :228 carrying pane_key/path/error. A missing source and an existing token file both count as success via linkStoredScrollback, which is what the AC 6 adoption relies on: the existing token-named file still holds the pane's original inode after the occupant's AtomicWrite replaced the positional name. heldTranscripts, ScrollbackWriter.Write and fileAtPositional are byte-identical to before. The link runs only after captureAndRefile's own-server confirmation, so a stand-down creates no token file. In steady state stored==tokenPath short-circuits, so no syscall is made per tick. Duplicate live tokens are already cleared at capture (internal/state/capture.go:258-265), so two candidates cannot share one token.

TESTS:
- Status: Adequate
- Coverage: There is one test per criterion, in internal/state/commit_cycle_shifted_test.go:
  - AC1 at :66 (3 refusal answers × 3→2 written or not, asserting one confirmation read and nothing at work__1.0.bin)
  - AC2 at :113 (tick → commit-now → flush, unmoved pane included)
  - AC3 at :150 (both write orders)
  - AC4 at :188 (same cycle and later cycle)
  - AC6 at :234 (dump failure after the occupant's write, and a failed sessions.json write via chmod)
  - AC7 at :285 (read-only scrollback dir, exactly one WARN, no token file, both `named` branches)
  - AC8, refused token, at :329
  In internal/state/commit_cycle_moved_test.go, AC5 adds the token-file-gone assertion (:333-335), the :333/:334 cases flip to assert the hold (:342-392), and TestRunCommitCycleHoldsNeitherMovedPaneWhenBothLastRecordsNameOneFile stands (:394). The tests would fail if the feature broke. Under the old rule AC1's moved pane writes unconfirmed (laterReads 0). A link taken after the dump would put the occupant's bytes under the token. Replacing rather than adopting an existing token file would fail AC6's "dump fails" case.
- Notes: The moved-test case at :352 overlaps AC4's same-cycle variant, and :353 overlaps AC1's "3→2 not written" variant. The plan prescribed both, so this is not a finding.

CODE QUALITY:
- Project conventions: Followed (no t.Parallel; capture logger via logtest; WARN uses existing attr keys)
- SOLID principles: Good
- Complexity: Low
- Modern idioms: Yes
- Readability: Good
- Issues: One stale field comment; see FINDINGS.

BLOCKING ISSUES:
- None

FINDINGS:
- [in-scope] [contained] internal/state/commit_cycle.go:60-61 — The comment on ScrollbackWriter.held says it "maps a pane key to the file its last committed record named, which its record still names". Since this change, the cycle that links a moved pane sets held[key] to `pane-<token>.bin`, while its last committed record named the other address's positional file (e.g. work__2.0.bin). Suggested replacement: "held maps a pane key to the file keepAnsweredTranscripts held it on — its token-named transcript, or the file its last committed record named — which its record still names and its empty capture is judged against." — FAILS: in the link cycle, a reader of the writer is told held equals the previous record's ScrollbackFile, and the code falsifies that. Anyone reasoning from this comment about which file an empty capture is confirmed against, or about which name housekeeping reclaims, will reason from the wrong path.

UNSETTLED:
- None
