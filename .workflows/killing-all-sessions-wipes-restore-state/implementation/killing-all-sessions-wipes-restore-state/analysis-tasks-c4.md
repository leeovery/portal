# Analysis Tasks: Killing All Sessions Wipes Restore State (Cycle 4)

## Task 1: The Lazy-Resume Fixture's Capture Rounds Write Through The Cycle's Scrollback Writer
severity: medium
sources: duplication

**Problem**: `lazyPanelFixture.captureRound` (`internal/restore/lazy_resume_panel_integration_test.go:497-541`) says it takes a capture "the way the daemon takes one". Five real-tmux suites build on it: panel, discard, burst, renumbered restore and hangup. Its dump (`:502`) takes the `ScrollbackWriter` the commit cycle hands it as `_` and writes through `state.WriteScrollbackIfChanged` (`:516`). That is the route the daemon stopped using when cycle 1 Task 1 moved its write into the writer (`cmd/state_daemon.go:342`). So the fixture skips everything `ScrollbackWriter.Write` does (`internal/state/commit_cycle.go:74-95`):
- the held-transcript lookup and its dedup-entry drop;
- the empty-capture confirmation against the file the pane's record names;
- the `captured` set that `fileAtPositional` (`:139`, `:189-205`) uses to point an answered pane's record back at its positional file.

Take a pane answered after it waited. The fixture's round writes the pane's positional file. The commit still names the token-named transcript, because the hold (`:122-124`) keeps it there, so that commit's housekeeping deletes the capture it just wrote. The stale dedup entry then stops later rounds from writing it again. The discard suite's "it clears the pending marker and resumes writing the pane's scrollback" (`internal/restore/lazy_resume_discard_integration_test.go:178-185`) asserts `after.written[subject]` and passes on that write. The next reboot still finds the pre-reboot line, because the token-named transcript holds it. No assertion tells the fixture's route from the daemon's. A regression in the daemon's answered-pane write path, such as the hand-back to the positional file or the dedup drop, would pass every one of these suites. A user would find it only after a reboot, when an answered pane restores its pre-answer transcript and everything it printed since is gone.
**Solution**: `captureRound`'s dump writes through the writer it is handed. It calls `writer.Write(key, data, hash)` in place of `state.WriteScrollbackIfChanged(fx.stateDir, key, data, hash, fx.hashes)` and keeps recording the result in `written[key]`, so the fixture's round applies the same write rule as the daemon's `dumpPane`. The discard suite's "resumes writing" subtest also checks what that rule guarantees, not just the write's return value: after the round commits, the subject's record names its positional file, and that file is on disk holding the capture. This follows the record. The routing is the finding's recommendation and matches cycle 1 Task 1, which made the writer the daemon's only write route. The added check answers the finding's own observation that no assertion tells the two routes apart. `WriteScrollbackIfChanged` stays exported for the other test fixtures, as cycle 1 Task 1 set it. No production code changes.
**Outcome**: The five real-tmux lazy-resume suites save an answered pane through the same writer as the daemon, so a regression in that write path fails them before it reaches a reboot.

**Acceptance Criteria**:
- [ ] In the discard suite, the subject waited across a reboot, so a capture round filed it under its token-named transcript. After its resume is discarded, the next capture round reports the subject written, commits the subject's record naming its positional scrollback file, and that file is on disk holding the capture the round wrote. (§2.4)
- [ ] Taken against the former route, with the round's dump discarding its writer and writing through `state.WriteScrollbackIfChanged`, that same subtest fails: the commit names the token-named transcript, and its housekeeping has removed the positional file the round wrote. (§2.4)
- [ ] A capture round taken while a pane waits or is skeleton-marked still reports nothing written for that pane, and a live pane beside it is still written. (§2.4)
- [ ] The panel, discard, burst, renumbered-restore and hangup suites pass with every capture round writing through the writer. Every assertion they carry today stands unchanged beside the discard suite's added check. (§2.4)
- [ ] No non-test file changes, and `state.WriteScrollbackIfChanged` stays exported. (§2.4)

**Do**:
- `internal/restore/lazy_resume_panel_integration_test.go`, `captureRound`'s dump (`:502-526`): take the `state.ScrollbackWriter` the cycle hands it in place of `_`, and write each pane through `writer.Write(key, data, hash)` in place of `state.WriteScrollbackIfChanged(fx.stateDir, key, data, hash, fx.hashes)` (`:516`). Keep recording the result in `written[key]`. The cycle's `HashMap` stays `fx.hashes` (`:533`).
- `internal/restore/lazy_resume_discard_integration_test.go`, the "it clears the pending marker and resumes writing the pane's scrollback" subtest (`:167-186`): after `after := fx.captureRound(t)` (`:178`), check that the subject's record in `after.idx` names its positional file and that this file is on disk holding the capture.
- The scope is measured. `rg -n '_ state\.ScrollbackWriter' --type go` gives 1 hit, this dump (`:502`), the only commit-cycle dump that discards its writer. `rg -n 'state\.CaptureAndHashPane\(' --type go` gives 6 hits:
  - the daemon's `dumpPane` (`cmd/state_daemon.go:331`), which already writes through the writer (`:342`);
  - this fixture (`:512`);
  - `cmd/bootstrap/daemon_tick_test_helpers_test.go:68`, which commits through `state.Commit` with no commit cycle and so no writer, and stays on `WriteScrollbackIfChanged` as cycle 1 Task 1 left it;
  - three `internal/state` tests of the capture itself (`scrollback_test.go:196`, `:212`; `capture_period_session_realtmux_test.go:65`).
- The fixture's suites are the five that call `captureRound`: `lazy_resume_panel_integration_test.go`, `lazy_resume_discard_integration_test.go`, `lazy_resume_burst_integration_test.go`, `lazy_resume_renumbered_restore_integration_test.go` and `resume_pane_hangup_integration_test.go`, all in `internal/restore` and all `//go:build integration`, so they run under `go test -tags integration -p 1 ./internal/restore`.
- Unchanged: `ScrollbackWriter.Write` (`internal/state/commit_cycle.go:74-95`), `fileAtPositional` (`:189-205`), and `WriteScrollbackIfChanged` (`internal/state/scrollback.go:80`), which stays exported for test fixtures.

## Task 2: Each Commit Cycle Merges And Carries From The Index It Read Under The Lock
severity: low
sources: architecture

**Problem**: `RunCommitCycle` reads `sessions.json` under the commit lock (`committed`, `internal/state/commit_cycle.go:116`). The answered-pane hold, the drop log and the no-change test are all decided from that read. But the cycle hands the caller's `LoadPrev` index (`:117-118`) to `captureAndRefile`, which feeds the skeleton merge, the waiting-pane merge and the carry (`internal/state/capture.go:153-167`). The daemon's `LoadPrev` is its in-memory index (`cmd/state_daemon.go:271`). That index lags `sessions.json` whenever a `commit-now` (`cmd/state_commit_now.go:115-124`) commits before the daemon's next successful cycle.

The carry is the one place where nothing repairs that lag. Traced through the tree:
- Panes X and Y both wait in session S.
- A `session-closed` `commit-now` files X under `pane-T.bin`, and its housekeeping removes X's positional file.
- The user answers X.
- On the daemon's next tick, S misses the capture (renamed mid-capture, or its environment read fails) while Y still waits.
- `carryMissedWaitingSessions` (`capture.go:192-239`) copies S whole from the daemon's in-memory index (`carriedSession`, `:245-270`). That index's record for X still names the removed positional file.
- The dump and the hold both skip carried panes (`SkipsScrollback`, `internal/state/scrollback.go:335-342`; `commit_cycle.go:159`). X no longer waits, so it is not re-filed either: only pending and carried-waiting panes are (`scrollback.go:370-373`).
- So the commit names a missing file, and `gcOrphanScrollback` deletes `pane-T.bin`.

If tmux goes down before S is captured again, X restores with no scrollback, and nothing in the log says why. The window is narrow: it closes at the daemon's first successful cycle after the `commit-now`. But the loss cannot be undone, and it breaks the guarantee the §2.4 corrigendum recorded from phase 3 Task 1: from the answer until a dump writes the pane's new capture, every commit names the token-named transcript and leaves it on disk. Every other site that reads the caller's index has needed its own repair for this lag: the adoption of a missing source or an existing token-named file in `linkStoredScrollback` (`scrollback.go:144-156`), and the hold, which cycle 2 Task 1 moved onto `committed`.
**Solution**: `RunCommitCycle` hands `captureAndRefile` the same previous index the hold already uses: `sessions.json` as read under the lock. The caller's `LoadPrev` is called only when that read fails (the file is absent or does not decode), so `commit-now`'s `LoadPrev`, which re-reads the same file, no longer runs on the normal path. Its absent and unreadable WARN lines still fire whenever they fire today. The skeleton merge, the waiting-pane merge, the carry and the hold then all see the last commit any committer made. The lag repairs stay, because they still cover a cycle that linked and then ended uncommitted.

This reverses cycle 2 Task 1's settled direction that `LoadPrev` keeps feeding those merges. The grounds:
- **Spec.** The traced loss contradicts the §2.4 corrigendum's guarantee and §1.1's rule that every session not killed stays restorable with its scrollback intact. That makes it a defect, not a preference.
- **The direction's cost grounds no longer hold.** Cycle 2 rested on two reasons from the phase 3 fix round. One was the extra decode per commit, but cycle 2 already decodes `sessions.json` under the lock. The other was moving the fix out of `internal/state`, but this change sits inside `RunCommitCycle`.
- **The remaining ground changes nothing where it matters.** That ground was that the merges would read a different index. They differ only where the two indexes differ. The daemon's in-memory index is loaded from `sessions.json` at start (`cmd/state_daemon.go:425-430`). Only a successful cycle's capture replaces it (`:283`), and that capture is either committed or judged no change against `sessions.json`. So it differs from the disk only after another committer has committed, which is exactly the case where §2.4 says to measure against the last committed record.

A narrower fix would extend the hold to answered panes in a carried session. That closes this one site but leaves the next merge that reads the caller's index open in the same way, so it is not taken.
**Outcome**: One commit cycle reasons from one previous index. An answered pane in a session carried forward keeps naming its token-named transcript, and that file stays on disk. Whichever committer last filed a pane, the daemon's next cycle merges and carries from the records that commit left.

**Acceptance Criteria**:
- [ ] Panes X and Y wait in session S. A `commit-now` files X under its token-named transcript, its housekeeping removes X's positional file, and the user then answers X. The daemon's in-memory previous index predates that `commit-now` and names X at its positional path. On the daemon's next cycle, S misses the capture (renamed mid-capture, or its environment read failing) while Y still waits. The daemon's tick and its shutdown flush each carry S forward with X's record naming its token-named transcript, commit it, and leave that file on disk with its bytes, so the next restore finds them. (§2.4, §1.1)
- [ ] `sessions.json` holds a `commit-now`'s commit, and the daemon's in-memory previous index predates it. The daemon's next cycle commits the same index, and leaves the same files on disk, as a cycle whose in-memory index matches `sessions.json`, for skeleton-marked, waiting and carried panes alike. (§2.4)
- [ ] A cycle over a `sessions.json` that reads and decodes under the lock never calls the caller's `LoadPrev`. Neither the daemon's in-memory index nor `commit-now`'s re-read of the file is consulted. (§2.4)
- [ ] With `sessions.json` absent or not decodable, the cycle calls `LoadPrev` once, under the lock. The skeleton merge, the waiting-pane merge, the carry and the answered-pane hold take their records from that index as they do today. `commit-now` logs `sessions.json absent; proceeding with zero-value PrevIndex` at WARN for an absent file, and `read sessions.json failed; proceeding with zero-value PrevIndex` at WARN for one that does not decode. (§2.4)
- [ ] The lag repairs stay. `linkStoredScrollback` still adopts a missing source or an existing token-named file. A cycle that linked and then ended uncommitted (stood down, cancelled mid-dump, or with a failed `sessions.json` write) still leaves `sessions.json` naming only files on disk, and the existing tests for those ends pass unchanged. (§2.3)

**Do**:
- `internal/state/commit_cycle.go`, `RunCommitCycle` (`:109-145`): the index read under the lock (`committed`, `:116`) is the previous index handed to `captureAndRefile` (`:118`), and so the one the hold reads (`:122-124`). `cycle.LoadPrev()` (`:117`) is called only when that read returns nil.
  - `readPriorIndex` (`internal/state/commit.go:63-74`) returns nil exactly when `sessions.json` is absent or does not decode.
  - Those are the two cases in which `state.ReadIndex` reports a skip (`internal/state/index_reader.go:17-32`), and so the two in which `commit-now`'s `loadPrevIndex` (`cmd/state_commit_now.go:162-173`) emits its WARN lines.
- `CommitCycle.LoadPrev` (`internal/state/commit_cycle.go:40-42`) changes its contract: it is called under the lock, and only when `sessions.json` cannot be read.
- Unchanged:
  - `commitOver`'s drop log and no-change test against `committed` (`internal/state/commit.go:34-60`);
  - `captureAndRefile` (`internal/state/scrollback.go:356-375`), and the skeleton merge, waiting-pane merge and carry it feeds (`internal/state/capture.go:153-167`);
  - `linkStoredScrollback` (`internal/state/scrollback.go:148-157`) and its callers `refilePendingPane` (`:127-142`) and `linkMovedPane` (`:217-231`);
  - the daemon's `LoadPrev` (`cmd/state_daemon.go:271`), its start-up load (`:425-430`) and its replacement on a successful cycle (`:283`);
  - `commit-now`'s `LoadPrev` (`cmd/state_commit_now.go:119-122`).
- Existing test encoding the old contract: the "two commit-nows started back to back across X's pending mark run one after the other, the second reading the first's commit" subtest (`internal/state/commit_cycle_test.go:327-364`). It wants the second commit-now's `LoadPrev` called once (`:354`) and reads the index that call returned (`:357-362`). The first commit-now's `sessions.json` is readable, so after this change the second's `LoadPrev` does not run there. `rg -n '[lL]oads\s*(!=|==)' --type go` gives 3 hits. The other two are unaffected: `TestRunCommitCycleLockBound` wants zero loads under a held lock (`internal/state/commit_cycle_test.go:437`), and `cmd/state_hydrate_lazy_test.go:537` does not count `LoadPrev`.
- The carry's committer table is `internal/state/capture_carry_test.go:87-118`. Its daemon-tick row hands its in-memory index through `LoadPrev` (`:95`), and its commit-now row re-reads `sessions.json` (`:108-114`).
