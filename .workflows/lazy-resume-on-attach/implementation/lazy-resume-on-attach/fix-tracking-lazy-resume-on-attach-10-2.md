## Attempt 1

ISSUES:
- /Users/leeovery/Code/portal/internal/state/scrollback.go:224-227: the link-failure branch has no test. The doc comment at :192-193 promises that such a pane's record is left alone and one WARN is emitted. The re-file's equivalent branch has a test (`/Users/leeovery/Code/portal/internal/state/scrollback_test.go:736`, "it warns once and leaves the record and the dedup entry alone when the rename fails"); this one does not. Without it, a regression that sets `p.ScrollbackFile = tokenPath` before checking the error, or drops the WARN, passes the whole suite. The damage would be:
  - The moved pane's record names a token file that was never created.
  - When no pane sits at the vacated address, the positional file holding the pane's bytes is no longer referenced, and the same commit's housekeeping pass deletes it.
  - The transcript is gone, and nobody finds out until the next reboot restores the pane empty.
  FIX: Add a subtest to `TestCaptureAndRefileLinksAMovedSkeletonPaneOntoItsToken` in `/Users/leeovery/Code/portal/internal/state/capture_refile_test.go`, modelled on scrollback_test.go:736:
  - Seed `work__2.0.bin` and call `denyScrollbackWrites(t, dir)`. `link(2)` into a read-only directory fails with EACCES, which is neither a missing source nor an existing name.
  - Run `CaptureAndRefile` over the `skeletonCycle` shape with a logger from `openTempLogger`. `skeletonCycle` passes a nil logger, so either give it a logger parameter or inline the call in the subtest.
  - Assert that work:1.0's `ScrollbackFile` stays `scrollback/work__2.0.bin`.
  - Assert that no `pane-<token>.bin` exists (`assertNoTokenFile`).
  - Assert exactly one WARN (`sink.Records().AtExactLevel(slog.LevelWarn).Only(...)`) carrying `pane_key=work__1.0`, `path=scrollback/work__2.0.bin` and an `error` attr, and no other attr keys.
  CONFIDENCE: high

COMMENT_CORRECTIONS:
- /Users/leeovery/Code/portal/internal/state/scrollback.go:253 — the new clause says no caller can commit an index that files a moved mid-restore pane under another address's positional path. A pane whose token the pane-token rule refuses keeps exactly that path, and this diff's own subtest "it keeps the positional path for a token the pane-token rule refuses" asserts it.
  OLD: // waiting pane's vacated positional path. A failed marker read returns its
  NEW: // waiting pane's vacated positional path, bar a pane whose token the
// pane-token rule refuses. A failed marker read returns its

NOTES:
- A residual window the task's reasoning does not cover. The task chose a link over a move so that the baked positional path "stays valid until the intruder's atomic rename". That only holds when a live pane occupies the vacated address. When none does — saved windows 0, 2 restored as 0, 1, which is the shape of the renumbered integration fixture and of the common single-gap case — the link takes the positional file out of every record. The same commit's `gcOrphanScrollback` (`/Users/leeovery/Code/portal/internal/state/commit.go:76-111`) then deletes it. Before this change the record still named that file and it survived.
  - A helper that has not yet reached `os.Open(cfg.File)` (`/Users/leeovery/Code/portal/cmd/state_hydrate.go:145`) by the daemon's first capturing commit after `@portal-restoring` clears takes the missing-file path: an empty pane this boot. If the registration resolves eager, the token file is reclaimed afterwards and the transcript is lost for good.
  - On the normal path the window is milliseconds: the step-7 signal is written before the clear, and the tick still has tmux reads and dumps to do. It widens only when step 7's signal fails and a CLI attach delivers the signal after that tick.
  - The intruder case the task accepted has the same order of exposure.
  - An in-code alternative, if the orchestrator wants to close it: link only when the stored positional path is the live key of another pane in this capture. Otherwise keep the positional record, which is referenced, survives housekeeping, and has nothing writing to it. This departs from the task's stated reach condition, and the commit-now test (no pane at the vacated address) would need that pane added.
- Documentation drift for the orchestrator. §7.2 says a pane's bytes are re-filed under its token "at the tick a pane is first frozen". The CLAUDE.md `state` row describes `CaptureAndRefile` as "the structure read and the pending re-file as one step". Neither mentions that a moved, tokened mid-restore pane now gets a token name at the skeleton stage, including a pane that never goes on to wait (reclaimed per AC6). A corrigendum or doc update may be owed.
- The executor reported that `cmd/state_daemon_run_test.go` changed on disk mid-task. Its diff against HEAD is 132 added lines, all in the new tests and helpers, with nothing else modified, and the package passes.
- Only the enumerated inputs arrived with this dispatch.

## Attempt 2

ISSUES:
- `/Users/leeovery/Code/portal/internal/state/scrollback.go:194-229` (`linkMovedSkeletonScrollback` / `linkMovedPane`): the record is re-pointed to the token file whenever it names another address's positional file, even when no live pane sits at that address.
  - **Where the shape comes from:** every renumbered restore has it. The topmost moved window has no occupant at its saved address (saved 0 and 2 restore as 0 and 1; nothing sits at 2).
  - **What happens:** the housekeeping pass of that same cycle's `Commit` (`gcOrphanScrollback`, `/Users/leeovery/Code/portal/internal/state/commit.go:77`) finds the baked positional path named by no record and deletes it. A hydrate helper that has not opened that path yet then takes the file-missing tail (`/Users/leeovery/Code/portal/cmd/state_hydrate.go:145` → `handleHydrateFileMissing`). That unopened helper is exactly the case the task chose a link over a move to protect.
  - **Consequence:** the pane comes up with no history. Once it is captured at its live address (straight away if eager, after the answer if lazy), the token file drops out of reference and is reclaimed, so the transcript is lost for good.
  - **Before this change:** the record kept naming the positional path, which stayed referenced and survived until the helper had replayed it. The task's rationale ("the link leaves that path valid until the intruder's atomic rename") does not hold when there is no intruder.
  - **Reproduced in a scratchpad copy:** `skeletonCycle`'s shape followed by `state.Commit` leaves `scrollback/work__2.0.bin` gone. With the link call removed, the file survives.
  - **Why the tests pass:** the subtest "it names the token file and leaves the baked positional path holding the same bytes" (`/Users/leeovery/Code/portal/internal/state/capture_refile_test.go:208`) and `TestStateCommitNow_FilesAMovedSkeletonPaneUnderItsToken` (`/Users/leeovery/Code/portal/cmd/state_commit_now_test.go:1403`) both run in this no-occupant shape.
  FIX: Link only when the stored name is a live write address in this cycle.
  - **Code:** in `linkMovedSkeletonScrollback`, build a set of `positionalScrollbackFile(SanitizePaneKey(s.Name, w.Index, p.Index))` over every pane of `idx`. Skip any skeleton pane whose `ScrollbackFile` is not in that set, and keep the own-key and token-path early returns.
  - **Effect:** a pane whose saved address has no occupant keeps its positional record, which stays referenced, so the baked path survives, exactly as before this change. The first cycle that enumerates an occupant links before that occupant's dump, because the dump only walks the panes of that same index.
  - **Trial:** I tried this guard in the scratchpad copy. The AC1 and AC6 daemon tests, `TestDaemonTick_KeepsARenumberedWaitingPaneTranscriptWhileSkeletonMarked` and the refile tests all pass under it. Only the no-occupant unit tests fail, as expected.
  - **Tests to change:**
    - Give `skeletonCycle`'s link subtests (names-token-file, link-fails WARN, adopts-existing) an unmarked occupant row at work:2.0.
    - Add a subtest with nothing at work:2.0. It should assert the record keeps `scrollback/work__2.0.bin`, no `pane-<token>.bin` appears, and the file still holds its bytes after `state.Commit`.
    - Move `TestStateCommitNow_FilesAMovedSkeletonPaneUnderItsToken` onto AC2's occupied shape.
  - **Docs:** update the `linkMovedSkeletonScrollback` and `CaptureAndRefile` docs to state the occupancy condition.
  - **Scope:** this adds one condition to the Do bullet "It reaches a pane in the cycle's skeleton set whose token is token-shaped … and whose merged record names a positional file other than its own live key's". The Outcome and all six ACs are unchanged.
  ALTERNATIVE: Keep the reach as written, and have the hydrate helper fall back to `PendingScrollbackPath(stateDir, hookKey)` when its baked path is missing. That keeps the Do's wording. But it puts the fix in the helper (another task's output) and adds a second read path to every restore. I recommend the occupancy guard.
  CONFIDENCE: medium

COMMENT_CORRECTIONS:
- `/Users/leeovery/Code/portal/internal/state/scrollback.go:185-186` — the claim covers panes whose token the pane-token rule refuses, which the code never links.
  OLD: // linkMovedSkeletonScrollback gives each mid-restore pane whose record names
// another address's positional file a second name under its durable token, and
  NEW: // linkMovedSkeletonScrollback gives each mid-restore pane whose token the
// pane-token rule accepts and whose record names another address's positional
// file a second name under that token, and
- `/Users/leeovery/Code/portal/internal/state/scrollback.go:253-254` — "no caller can commit an index that … files a moved mid-restore pane under another address's positional path" is false on the link-failure path, and the same goes for the re-file failure. Both leave the record where it was and commit.
  OLD: // waiting pane's vacated positional path, bar a pane whose token the
// pane-token rule refuses. A failed marker read returns its
  NEW: // waiting pane's vacated positional path, bar a pane whose token the
// pane-token rule refuses or whose link or re-file failed. A failed marker
// read returns its

NOTES:
- The executor reported only the unit lane. I ran the integration fixtures that enter through `CaptureAndRefile`: `go test -tags integration -p 1 ./internal/restore -run TestLazyResume`. They pass, and no `ptl-` servers leaked.
- Mutation checks I ran in the scratchpad copy:
  - Swapping `os.Link` for `moveNoClobber` fails the baked-path subtest.
  - Removing the link step fails the new daemon, commit-now and refile tests.
- Documentation drift for the orchestrator's corrigendum and docs path: the spec's section 7.2 and CLAUDE.md's `state` row (which calls `CaptureAndRefile` "the structure read and the pending re-file as one step") do not describe the skeleton-stage link.
- The link keeps the positional key's dedup hash entry. That is correct, because the name still holds those bytes until the occupant's hash-differing write replaces it.
- On a filesystem without hard links, the link fails. The pane falls back to the pre-change behaviour, with one WARN per skeleton tick for the length of the skeleton phase.
- The trial code and probe tests are in the session scratchpad (`/private/tmp/claude-501/-Users-leeovery-Code-portal/d491bf7e-5102-4dc8-93ec-2b3e1c7542c9/scratchpad/portal`). Nothing in the repository was modified.
- BANK (reviewer, not deposited — the phase no longer takes deposits): File a moved tokened pane under its token at restore (arm) time and bake that path, instead of first linking it during a capture. A tokened pane restored away from its saved address whose hydrate signal lands after the first post-restore capture opens its baked positional path after the occupant's same-cycle AtomicWrite replaced it (occupied: replays the occupant's history) or finds it gone (unoccupied). Restore bakes the saved positional path (internal/restore/session.go:69-71) and the helper opens it at cmd/state_hydrate.go:145.

## Attempt 3

ISSUES:
- `/Users/leeovery/Code/portal/internal/state/scrollback.go:202`, `:213` and `:222-232` (`occupiedPositionalFiles` and the `taken` guard): if the occupant of the saved address is skipped this cycle, the link removes the baked path's last reference and the housekeeping pass deletes it.
  - **When it happens:** the occupant is a skeleton-marked tokened pane restored from a later saved address, or a pending pane. Its record names a different file and the dump does not write to it. This is the normal layout when neighbouring windows each carry a hook: saved windows 0, 2, 3 with hooks in 2 and 3, restored as 0, 1, 2, both still skeleton-marked on the first capturing tick.
  - **What happens:** Y is linked, no record names `work__2.0.bin`, and `gcOrphanScrollback` deletes it at `state.Commit`.
  - **Reproduced in a scratch copy:** Y's record went to `scrollback/pane-ab12cd.bin`, X's record stayed `scrollback/work__3.0.bin`, and `work__2.0.bin` was gone after `Commit`.
  - **Consequence:** if Y's hydrate helper has not yet opened its baked path (for example, the eager hydrate signal failed and it is waiting for client-attached), it finds the file missing and the pane comes up with no transcript. If Y then resolves eager, or carries a token with no registration, the next tick captures it at `work__1.0.bin` and the token file drops out of reference and is deleted. Y's transcript then has no copy anywhere.
  - **Regression:** before this task, the same layout kept `work__2.0.bin` referenced by Y's record and replayed it correctly. This is the failure the guard's own doc (`:190-192`) says it prevents, reached through a different occupant.
  FIX:
  - Base the reach on the collision itself: link only when another record in `idx` names the same file. Replace `occupiedPositionalFiles` with a count of records per `ScrollbackFile` over the index, and skip unless the count for `p.ScrollbackFile` is greater than 1.
  - This covers every case it has to. Every file the dump writes belongs to a non-skipped pane whose record names its own live positional file. After the link, that other record keeps the positional name referenced, so the housekeeping pass keeps it.
  - Checked in a scratch copy: with this change, every unit test in internal/state and cmd passes, including all subtests of `TestCaptureAndRefileLinksAMovedSkeletonPaneOntoItsToken`, both new daemon tests and the commit-now test. The reproduction above passes too.
  - Add a subtest to `/Users/leeovery/Code/portal/internal/state/capture_refile_test.go`: an occupant at work:2.0 that is skeleton-marked (both markers set) and has its own token-shaped token and a saved record at work:3.0. Assert that Y keeps `scrollback/work__2.0.bin`, that no `pane-<token>.bin` exists for Y, and that `work__2.0.bin` survives `state.Commit`.
  - Update the doc for `linkMovedSkeletonScrollback` ("whose record names a file another record in idx also names"). In `CaptureAndRefile`'s doc, change "files a moved mid-restore pane under the positional path of another pane it holds" to a clause stating that no two records name one scrollback file.
  ALTERNATIVE: Pass `capture.Pending` in and build the occupied set only from panes outside skeleton ∪ pending, which are the panes the dump writes this cycle. It reaches the same writes, but it adds a fifth parameter. It also still commits two records naming one file when a tokenless skeleton pane has taken the moved pane's saved record by address (see BANK). I recommend the record count.
  CONFIDENCE: medium

COMMENT_CORRECTIONS:
- `/Users/leeovery/Code/portal/internal/state/scrollback.go:273-277`: the `CaptureAndRefile` doc paragraph has a broken line wrap ("read returns its" stands alone on a line).
  OLD: // pane-token rule refuses or whose link or re-file failed. A failed marker
// read returns its
// wrapped error before any capture is taken. A failed capture returns before
// anything is re-filed, with the empty index, the empty pending set and the
// error the capture gave.
  NEW: // pane-token rule refuses or whose link or re-file failed. A failed marker
// read returns its wrapped error before any capture is taken. A failed capture
// returns before anything is re-filed, with the empty index, the empty pending
// set and the error the capture gave.

BANK (not deposited — the phase no longer takes deposits):
- A tokenless skeleton pane that matches by address takes a previous record whose token another live pane has already claimed.
  FAILURE: In a renumbered restore, a tokenless pane that is mid-restore at a moved tokened pane's saved address is committed with that pane's whole saved record: its CWD, command, scrollback path and `PortalPaneID`. While it stays skeleton-marked, sessions.json holds two records with one token. A reboot in that window re-stamps the token onto both panes and bakes the same hook key into both. One resume hook then fires in two panes (seen as a duplicated resumed program), and the tokenless pane replays the other pane's transcript.
  DETAIL: `/Users/leeovery/Code/portal/internal/state/capture.go:162-170` (`*p = record` when `PortalPaneID == ""`) and `takePrevRecord` at `:264-277`. `byToken` and `byAddress` are independent maps, so one previous record can be taken twice. Reproduced in a scratch copy: X (tokenless, at work:2.0) committed as `{CWD:/y ScrollbackFile:scrollback/work__2.0.bin PortalPaneID:ab12cd}`, beside Y holding `ab12cd`. Fix direction: an address match must not take a record whose token a live pane in the same enumeration answers to.
  FILES: /Users/leeovery/Code/portal/internal/state/capture.go

NOTES:
- linkMovedPane's `stored == tokenPath` early return (`scrollback.go:240`) can never be reached under the current guard. The occupied set holds only `<session>__<w>.<p>.bin` names, and those can never equal `pane-<token>.bin`. The AC-4 subtest therefore passes through the guard, not through that return. Under the recommended record count it is reachable only when two records name one token path, and it is harmless either way.
- The spec describes the token re-file happening "at the tick a pane is first frozen". It does not describe this task's earlier link for a moved skeleton-marked pane, or the reach condition on it. The orchestrator can decide whether that needs a corrigendum.
- Independently verified:
  - `go test ./internal/state/ ./cmd/` passes.
  - The integration tests `TestLazyResumePanel*` (internal/restore) and `TestNonContiguousWindowReboot_KeepsTokenKeyedHooks` (cmd) pass. The latter's layout has no occupant at the vacated address, so the guard does not change it.
  - No `ptl-` tmux servers were left running afterwards, and the scratch copy has been removed.
