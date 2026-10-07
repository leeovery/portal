TASK: The Lazy-Resume Fixture's Capture Rounds Write Through The Cycle's Scrollback Writer (killing-all-sessions-wipes-restore-state-6-1, tick-0d1cc8)

ACCEPTANCE CRITERIA:
- In the discard suite, the subject waited across a reboot, so a capture round filed it under its token-named transcript. After its resume is discarded, the next capture round reports the subject written, commits the subject's record naming its positional scrollback file, and that file is on disk holding the capture the round wrote. (§2.4)
- Taken against the former route, with the round's dump discarding its writer and writing through `state.WriteScrollbackIfChanged`, that same subtest fails: the commit names the token-named transcript, and its housekeeping has removed the positional file the round wrote. (§2.4)
- A capture round taken while a pane waits or is skeleton-marked still reports nothing written for that pane, and a live pane beside it is still written. (§2.4)
- The panel, discard, burst, renumbered-restore and hangup suites pass with every capture round writing through the writer. Every assertion they carry today stands unchanged beside the discard suite's added check. (§2.4)
- No non-test file changes, and `state.WriteScrollbackIfChanged` stays exported. (§2.4)

STATUS: complete

SPEC CONTEXT: §2.4 — the daemon's dump reaches `WriteScrollbackIfChanged` only through the `ScrollbackWriter` the commit cycle hands it. A lazy pane just answered keeps its token-named transcript on its record until a dump writes its new capture; only the housekeeping pass of a commit naming a positional file that holds the pane's capture removes the token-named file. The fixture previously bypassed the writer, so the held-transcript dedup drop, the empty-capture confirmation and the `captured` hand-back to the positional file (`fileAtPositional`) were never exercised by the five real-tmux lazy-resume suites.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/restore/lazy_resume_panel_integration_test.go:504 — dump takes `writer state.ScrollbackWriter` in place of `_`
  - internal/restore/lazy_resume_panel_integration_test.go:518 — `writer.Write(key, data, hash)` replaces `state.WriteScrollbackIfChanged(fx.stateDir, key, data, hash, fx.hashes)`; result still recorded in `written[key]` (:522); `HashMap: fx.hashes` unchanged (:538)
  - internal/restore/lazy_resume_panel_integration_test.go:485-490, :523-525, :545 — new `captured` map recording the bytes of each written pane, returned on `captureRoundResult`
  - internal/restore/lazy_resume_discard_integration_test.go:186-203 — added check: subject record in `after.idx` names its positional file (compared against `state.ScrollbackFile(fx.stateDir, key)`), and the file on disk holds `after.captured[key]`
- Notes:
  - Commit 88cabafdb touches only the two `_test.go` files; `WriteScrollbackIfChanged` remains exported at internal/state/scrollback.go:80. No other commit-cycle dump discards its writer (`_ state.ScrollbackWriter` has no remaining hit in cmd/ or internal/); the bootstrap tick helper (cmd/bootstrap/daemon_tick_test_helpers_test.go:74) stays on `WriteScrollbackIfChanged` as planned since it commits with no cycle.
  - The fixture's dump now mirrors the daemon's `scrollbackDump.run`/`dumpPane` (cmd/state_daemon.go:303-353): skip via `SkipsScrollback`, capture, `writer.Write`. The fixture fails the cycle on a write error where the daemon logs and continues — correct for a fixture, which should surface a refusal rather than absorb it.
  - AC2 traced by reading: under the former route `writer.captured` stays empty, so `fileAtPositional` (internal/state/commit_cycle.go:148) is a no-op, and `keepAnsweredTranscripts` (:131-133) leaves the subject's record on `PendingScrollbackFile("dscsub")` — the token-named file the `whileWaiting` round re-filed it under. The added assertion at internal/restore/lazy_resume_discard_integration_test.go:196 therefore fails on the record path, and the commit's housekeeping removes the unnamed positional write. Under the writer route `Write` (commit_cycle.go:74-95) finds the pane held, drops its dedup entry, writes the positional file and adds the key to `captured`, so the record is handed back to positional.
  - AC3: the dump `continue`s on `SkipsScrollback` before calling the writer, so waiting/skeleton panes stay absent from `written`; existing assertions cover it (lazy_resume_panel_integration_test.go:176-183 for a waiting subject beside a written sibling; lazy_resume_renumbered_restore_integration_test.go:71 for a skeleton-marked pane; lazy_resume_discard_integration_test.go:173).

TESTS:
- Status: Adequate
- Coverage: The added check distinguishes the writer route from the bypass route: it pins the record's path and the on-disk bytes after commit/housekeeping, not just the write's return value. It sits in the one suite whose flow puts an answered, previously waiting pane through a round. Existing assertions across the five suites are unmodified (the diff adds lines only).
- Notes: The check reads the record from `after.idx` — the `capture.Index` that `RunCommitCycle` committed after `fileAtPositional`. It reads the file after the round returns, so the housekeeping pass has already run. Both are the right observation points. No over-testing: three targeted assertions (session/pane presence guard, record path, file content).

CODE QUALITY:
- Project conventions: Followed (integration-tagged suites, no t.Parallel, real-tmux fixture via tmuxtest, isolation via IsolateStateForTest unchanged)
- SOLID principles: Good
- Complexity: Low
- Modern idioms: Yes
- Readability: Good — the updated `captureRound` doc comment and the new in-test comment accurately describe the route and the failure the check guards against
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "The panel, discard, burst, renumbered-restore and hangup suites pass with every capture round writing through the writer." — run `go test -tags integration -p 1 ./internal/restore` and confirm the five suites (TestLazyResumePanel_*, TestLazyResumeDiscard_RealPaneDiscardsItsResume, the burst suite, TestLazyResumePanel_RenumberedRestoreKeepsTranscriptThroughEarlyCaptures, TestResumePanes_EndWhenTheirTerminalCloses) pass. Reading shows no unchanged assertion that depends on the old route's record staying on the token-named file. The burst suite and the panel suite's answer loop now restore the post-answer capture rather than the pre-answer transcript on their next reboot, and their post-reboot history checks rely on that capture (full `-S -` history) still carrying the pre-reboot line. Only a run settles it.
