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
