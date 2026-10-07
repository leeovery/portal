TASK: A Pane With No Resume Hook Restored Away From Its Saved Address (killing-all-sessions-wipes-restore-state-7-3, tick-410e83)

ACCEPTANCE CRITERIA:
Fixture: a session saved with token-less panes at work:0.0, work:2.0 and work:3.0, restored at work:0.0, work:1.0 and work:2.0.
1. Each restored pane carries a token on its @portal-pane-id. Its first committed record after restore carries that token and names a file holding the bytes that pane was saved with, never another pane's, whether that commit lands while the pane still carries its skeleton marker or after hydration has cleared it. That commit's housekeeping deletes none of the session's saved transcripts.
2. After hydration, the first save to reach a pane restored one window lower takes an empty capture, and the read after it is refused, answered by another server, or answered naming no server. Nothing is written, the refusal is logged as today, and the commit names a file holding that pane's saved bytes. A refused capture-pane, a dump-less commit-now and a dump-less shutdown flush each leave every later commit naming that transcript, on disk, the same way.
3. Once a dump writes such a pane's capture with confirmed bytes, the commit files it at its live positional path, and that commit's housekeeping reclaims its old transcript.
4. The pane restored at work:0.0, its own saved address, keeps today's judgement: its empty capture is judged against work__0.0.bin, which holds its saved transcript.
5. Running `portal hook set --on-resume <cmd>` in a pane restored without a saved token registers the hook under the token restore gave it, and stamps nothing.
6. A pane saved with a token is re-stamped with that same token, as today. A pane saved with no token comes back running the user's shell with no resume hook fired, as today.

STATUS: complete

SPEC CONTEXT: §1.1 says every session that is not killed stays restorable with its scrollback. §2.4 says an unconfirmed empty capture never replaces a saved transcript. Its closing paragraph, from the phase 7 corrigendum, extends that hold to a pane at a different address from its last committed record, including one restore brought back a window lower, and says every pane restore recreates carries an identity tying it to the record it was built from, whether or not it has a resume hook. §5.2's third bullet narrows the accepted residue to a token-less pane that tmux moves after restore.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/restore/session.go:156 — armPanes calls stampPaneIdentity before RespawnPane, so the identity is on the pane before the hydrate helper runs.
  - internal/restore/session.go:172-185 — stampPaneIdentity re-stamps a saved token. A pane saved without one gets a token minted through MintToken (default nanoid.NewPaneTokenGenerator, session.go:195-200). It records a (session, saved window, saved pane, token) tie only when the stamp succeeds. A failed mint or stamp degrades to a WARN.
  - internal/restore/session.go:158,380-389 — the hydrate command still bakes only info.paneToken (the saved token), so a minted token never becomes a --hook-key.
  - internal/restore/restore.go:71,85-89 — Orchestrator.Restore calls recordMintedTokens after the session loop, inside bootstrap step 6 while @portal-restoring is still set. Committers stand down while that marker is set (cmd/state_daemon.go:178,377; cmd/state_commit_now.go:98).
  - internal/state/restored_pane_tokens.go:28-70 — RecordRestoredPaneTokens takes the commit lock, re-reads sessions.json and writes each token onto the record at its saved address, leaving any token a record already carries. It rewrites the file only when something changed, using the same AtomicWrite0600 as Commit (internal/state/commit.go:48), and runs no housekeeping pass.
  - Downstream, unchanged: the first commit's previous index is sessions.json read under the lock (internal/state/commit_cycle.go:121), which now holds the tokens. A skeleton-marked pane matches its record by token in mergeSkippedPanes/takePrevRecord (internal/state/capture.go:303-327, 441-454). A hydrated pane is held on its token-named transcript by keepAnsweredTranscripts (internal/state/commit_cycle.go:175-209). A pane at its own address finds record.ScrollbackFile equal to its positional file and is not held (commit_cycle.go:191).
  - CLAUDE.md:61 (state row) and CLAUDE.md:190 (Resume hooks): the statement that stamping is lazy because Portal does not create panes is rewritten. It now says restore gives every recreated pane a token and records minted ones on the saved record, while `hook set` stays lazy for every other pane. The key-producing-sites sentence says a minted token bakes no key.
- Notes: The code meets every criterion. `hook set` still reads the token first and stamps only when it reads back empty (cmd/hooks.go:106-121). A restored pane with a minted token therefore registers under it and no stamp is issued. Two side effects are deliberate:
  - On the next reboot these panes count as saved with a token. They are re-stamped and baked with that key, and a key with no registration resolves as a miss and runs the bare shell (cmd/state_resume_chain.go:191-193).
  - `hook rm` in such a pane now reports the key-shaped miss rather than "this pane".

  If RecordRestoredPaneTokens fails (lock timeout), the panes keep fresh records at their live positional files. The loss in the shifted case is no worse than before the task, and the WARN names the failure.

TESTS:
- Status: Adequate
- Coverage:
  - Criterion 1:
    - TestOrchestrator_RecordsEachMintedTokenOnTheSavedRecordItsPaneWasBuiltFrom (internal/restore/restore_tokens_test.go:69) checks: three stamps, the saved token re-stamped, distinct minted tokens, each recorded on its saved-address record, and no WARN.
    - TestRunCommitCycleFirstCommitAfterRestoreKeepsEveryRestoredPanesSavedBytes (internal/state/commit_cycle_restored_test.go:85) runs the first commit twice: with every pane skeleton-marked, and after hydration. It asserts each record's token, the saved bytes, every named file on disk, and no file named by two records.
    - The integration subtests in cmd/noncontiguous_window_reboot_integration_test.go:154-171 and :188-211 repeat this against real tmux with a dump-less commit.
  - Criterion 2: TestRunCommitCycleKeepsARestoredPanesTranscriptOverAnUnconfirmedEmptyCapture (commit_cycle_restored_test.go:109) covers all three refusal answers through emptyCaptureRefusals. TestRunCommitCycleKeepsNamingARestoredPanesTranscriptUntilItsCaptureIsWritten (:137) covers a refused dump, a dump-less commit-now and a dump-less flush.
  - Criterion 3: TestRunCommitCycleFilesARestoredPaneAtItsLivePositionalFileOnceItsCaptureIsWritten (:163) covers a non-empty write and a confirmed empty write. It checks the record is filed at the positional path and the token-named file is reclaimed.
  - Criterion 4: TestRunCommitCycleJudgesARestoredPaneAtItsOwnSavedAddressAgainstItsPositionalFile (:206).
  - Criterion 5: the integration subtest at cmd/noncontiguous_window_reboot_integration_test.go:263-293. The stamper and minter it injects fail on any call.
  - Criterion 6: the session_test.go subtests "it mints a token per untokened saved pane and re-stamps each saved token" and "it bakes no hook key for a pane it minted a token for", plus the integration subtest "it fires no hook on the pane saved with no token".
  - Failure paths: a failed mint (session_test.go), a failed stamp (restore_tokens_test.go:114), and a held lock or absent file in RecordRestoredPaneTokens (restored_pane_tokens_test.go, restore_tokens_test.go:133).
  - The old pin TestRunCommitCycleJudgesATokenlessPaneRestoredOneWindowLowerAtItsOwnPositionalFile is replaced by the hold assertion. restore_progress_test.go pins MintToken so its two runs compare byte-identically.
- Notes: Each test would fail if its behaviour broke. The skeleton-marked subtest would fail without the recorded tokens, because a tokened skeleton pane takes no by-address fallback. The tests are focused, with no redundancy beyond the lock-timeout case, which is checked once per layer.

CODE QUALITY:
- Project conventions: Followed. DI via optional function fields (MintToken), no new log component, and log attrs (session/pane_key/error) from the existing vocabulary. sessions.json is written only under the commit lock, and no state.Commit call is made outside internal/state.
- SOLID principles: Good. Stamping (restore) and recording (state) are split along the package boundary, and the tie type lives in state.
- Complexity: Low
- Modern idioms: Yes (range-over-int, min)
- Readability: Good
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
