TASK: lazy-resume-on-attach-2-6 (tick-77075b) — A Frozen Pane's Scrollback Leaves the Positional Namespace

ACCEPTANCE CRITERIA:
- On the first tick a waiting pane carrying a token is seen, its bytes sit at `scrollback/pane-<token>.bin`, its record names that exact relative path, and the positional name it left holds nothing.
- With a live pane occupying the address a waiting pane vacated, the two records name different files: the live pane's capture lands in the positional file and the token-named file's bytes are unchanged by that tick.
- The dedup map holds no entry for the name the bytes left, so the first write under that name lands whatever it hashes to — including the waiting pane's own write when it returns to that address and its capture is byte-identical to the frozen file.
- A commit taken while the pane waits keeps the token-named file: it is in the referenced set and the housekeeping pass leaves it in place.
- Re-filing is idempotent — a pane already at its token path is not renamed again, its record is unchanged, and the hash map is untouched.
- A waiting pane whose record still names a positional file that is gone from disk adopts the token path, so the commit that follows keeps the file the bytes are actually in.
- A waiting pane whose `@portal-pane-id` is empty or not token-shaped keeps its positional path and its current exposure; nothing is renamed, no dedup key is dropped, and no WARN is emitted.
- A rename failing for any reason other than a missing source leaves the record on its stored path and the dedup entry in place, emits one WARN introducing no attr key the daemon does not already use, and neither aborts the tick nor blocks the commit.
- Once the marker clears, the pane is captured and filed under its live address again and the token-named file falls out of the referenced set and is reclaimed on that commit.
- A tick carrying a waiting pane emits no log message and no attr key a tick without one does not — `TestCaptureAndCommit_SkipsResumePendingPanes`'s taxonomy subtest stays green.
- `portal state commit-now` firing at any point during a wait commits a record naming the token path and reclaims no token-named file.

STATUS: issues_found

SPEC CONTEXT: Section 7.2 (as corrected by the 2026-09-21 corrigendum) says a frozen pane's previous record is matched on its durable `@portal-pane-id`. It also says the pane's bytes leave the positional namespace for as long as it waits: they are re-filed on the first frozen tick under `scrollback/pane-<PortalPaneID>.bin`, and the merged record carries that path. Restore and the housekeeping pass read the stored path verbatim. When the wait ends the pane returns to ordinary capture and the token file is reclaimed. A pending pane with no token keeps the pre-feature positional exposure. The same section accepts one consequence: the picker's scrollback preview is blank for a waiting pane that has been rearranged.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/state/paths.go:23-25` declares the `pane-` prefix once. `internal/state/paths.go:92-96` defines `PendingScrollbackFile`, which returns the relative path with forward slashes.
  - `internal/state/scrollback.go:98-117` holds `RefilePendingScrollback`; the per-pane step is `refilePendingPane` at `:119-134`.
  - `internal/state/scrollback.go:139-148` holds `renameStoredScrollback`, which treats a missing source or an empty stored path as "adopt the token path".
  - `internal/state/scrollback.go:156-158` holds `dedupKeyOf`, which derives the key the same way `SeedHashMap` does.
  - `internal/state/scrollback.go:166-173` holds `CaptureAndRefile`: the capture and the re-file as one step.
  - Call sites: `cmd/state_daemon.go:249` (daemon), which runs before the capture loop at `:263`; `cmd/state_commit_now.go:121` (commit-now), with a nil hash map and before `Commit` at `:126`.
- Notes:
  - Later work (task 4-11) folded the two plan-prescribed call sites into `state.CaptureAndRefile`. This change is sound: the daemon still re-files before its capture loop, and commit-now still re-files before its commit. It also closes the "commit an index without re-filing" path for good.
  - The implementation adds one thing the plan did not specify: an empty stored `ScrollbackFile` is treated as a missing source (`scrollback.go:139-142`). This is defensible, because joining `""` onto `dir` would rename the state directory.
  - The shutdown flush reaches the re-file through `captureAndCommit` (`cmd/state_daemon.go:363`).
  - All listed acceptance criteria are met in substance, judged by reading.
  - Two issues fall outside the criteria's literal wording; both are under FINDINGS: a repopulated-source race, and a preview regression that is wider than the spec's accepted consequence.

TESTS:
- Status: Adequate
- Coverage: All 11 plan-named tests exist.
  - `internal/state/scrollback_test.go:499-705` holds the unit cases: re-file, idempotency, adopt-on-missing, untokened and malformed tokens (4 variants), rename-failure WARN with an attr-key allowlist, nil hash map and nil logger, empty stored path, and a pane not in the pending set.
  - `cmd/state_daemon_resume_pending_test.go:323-525` holds the tick-level cases: re-file, vacated-address intruder, dedup drop, housekeeping across three ticks, unfreeze and reclaim, and tokened vs untokened panes.
  - `cmd/state_commit_now_test.go:1187-1246` holds the commit-now case, which runs the real `CaptureAndRefile` against a fake client.
  - `internal/state/capture_refile_test.go` covers the composite and its failed-capture path.
  - The dedup-drop test would fail if the key were kept: the returning write would be deduped, and `scrollbackBody` fatals on the missing file.
- Notes:
  - The taxonomy subtest (`state_daemon_resume_pending_test.go:251-287`) still uses an untokened pending pane, so the re-file never runs in it. The "no new message on a waiting tick" property therefore rests on the unit assertion that the success path logs nothing (`scrollback_test.go:522-524`), which does cover it.
  - No test covers the repopulated-source interleavings described in FINDINGS.

CODE QUALITY:
- Project conventions: Followed. There is one declaration of the prefix and of the path shape. The nil logger goes through `loggerOrDiscard`, and the WARN uses only existing attr keys (`pane_key`, `path`, `error`).
- SOLID principles: Good
- Complexity: Low
- Modern idioms: Yes
- Readability: Good
- Issues: None beyond FINDINGS.

BLOCKING ISSUES:
- None

FINDINGS:
- [in-scope] [spreading] internal/state/scrollback.go:143 — **What is wrong:** `renameStoredScrollback` renames whatever the stored positional path holds when it runs, assuming it still holds the frozen pane's bytes.
  - The "missing source → adopt" rule makes a second re-file safe against a stale index only if nothing has written that path since the first rename.
  - In the displacement case this task exists for, the same daemon tick re-files the frozen pane first (`cmd/state_daemon.go:249`). The capture loop then writes the intruder's capture into the vacated positional path (`cmd/state_daemon.go:290`), before the commit at `:303`.
  - Any route that re-files against an index still naming that positional path after the write renames the intruder's bytes over `pane-<token>.bin`, overwriting the waiting pane's transcript. There are four such routes:
    1. The loop's `ctx.Done()` early return (`cmd/state_daemon.go:267-271`) returns nil with no commit and no `PrevIndex` advance. The shutdown flush then re-runs `captureAndCommit` against the same `PrevIndex` (`cmd/state_daemon.go:363`).
    2. A `Commit` error at `cmd/state_daemon.go:303-305` leaves `PrevIndex` unadvanced (`:307`), so the next tick re-files from it.
    3. A daemon killed before its commit restarts from the on-disk `sessions.json`.
    4. `commit-now` fires mid-tick and reads the on-disk `sessions.json` (`cmd/state_commit_now.go:118`).
  - **Why the fix is spreading:** it has more than one defensible shape, and no current test observes this sequence.
    - Rename only when the token file is absent, and otherwise adopt. This covers all four routes but would adopt a stale token file left behind when an earlier wait's reclaim failed.
    - Keep the capture loop from writing a positional path vacated by a re-file until that re-file has been committed.
    - Advance the index the next re-file reads at re-file time. This covers only the in-process routes.
  - **Test that pins it:** stage the first waiting tick with an intruder on the vacated address, return `Commit` or ctx-cancel after the intruder's write, then run a second tick or the flush, and assert that the token file still holds the frozen bytes.
  - FAILS: the first waiting tick sees the pane already displaced, a live pane has taken its old address, and that tick is then cut short by a shutdown signal mid-loop, a commit error, a crash, or a concurrent `commit-now`. The next re-file then moves the live pane's capture over the waiting pane's `pane-<token>.bin`. At the next reboot the waiting pane replays another pane's history, and its own transcript has no copy anywhere. This is the permanent loss the task set out to close, arriving through the retry path.
- [in-scope] [spreading] internal/tui/preview_adapter.go:23 — **What is wrong:** the picker preview resolves a pane's saved transcript from its live position (`state.ScrollbackFile(a.stateDir, paneKey)`, using the key composed at `internal/tui/pagepreview.go:255-258`).
  - Since this task, every tokened waiting pane's bytes are renamed off that name on the first waiting tick (`internal/state/scrollback.go:128-133`), whether or not the pane ever moved.
  - Before this task, an un-moved waiting pane's preview read its frozen file. Now every waiting pane previews blank (the placeholder), for the whole wait.
  - The specification accepts this artifact only for "a waiting pane that has been rearranged" (`.workflows/lazy-resume-on-attach/specification/lazy-resume-on-attach/specification.md:312`). The 2026-09-21 corrigendum introduced the re-file without revisiting that paragraph, so the spec now understates a user-visible regression.
  - **Two defensible remedies — the user's choice:**
    - Resolve the preview through the pane's token for a pending pane. This is code, and no test currently observes it.
    - Amend that spec paragraph to accept a blank preview for every waiting pane.
  - FAILS: after a reboot, Space on a session holding an un-moved waiting pane shows the empty-scrollback placeholder instead of the saved transcript. Before this change it showed the transcript. The user loses the one pre-resume look at what the pane held, and the spec says this only happens to rearranged panes.

UNSETTLED:
- None
