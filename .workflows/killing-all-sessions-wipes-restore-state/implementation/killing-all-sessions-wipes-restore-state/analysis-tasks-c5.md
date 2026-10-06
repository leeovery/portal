# Analysis Tasks: Killing All Sessions Wipes Restore State (Cycle 5)

## Task 1: A Pane Restored Away From Its Saved Address Keeps Its Transcript Until A Confirmed Save Replaces It
severity: low
sources: standards

**Problem**: §2.4 says a pane's saved transcript is the file its last committed record names (spec line 98). The tree applies that rule only when the record names the pane's token-named file: `keepAnsweredTranscripts` (`internal/state/commit_cycle.go:169`) and `heldTranscripts` (`:185`) both test `== PendingScrollbackFile(token)`. For every other pane, `ScrollbackWriter.Write` judges an empty capture at the pane's live positional path (`:77`).

Restore creates windows in sequence, with no index (`internal/restore/session.go:96-110`). So a saved session with a gap in its window indices, such as a closed middle window, comes back with every later window's panes one address lower. Traced for an eager or hookless pane carrying token T, saved at `S:2.0` and restored at `S:1.0`, with no live pane at `S:2.0`:
1. While the pane's skeleton marker is set, the token merge keeps its record on `scrollback/S__2.0.bin`. `linkMovedSkeletonScrollback` leaves an unshared record where it is (`internal/state/scrollback.go:196-198`), and that record is committed.
2. Once the marker clears, the fresh record names `scrollback/S__1.0.bin` (`internal/state/capture.go:593`). Nothing holds the pane, because its committed record does not name `pane-T.bin`.
3. If tmux begins exiting during that save's dump, the pane's capture can come back empty. The writer judges it against `S__1.0.bin`, which does not exist, so `savedTranscriptMayHoldBytes` answers false (`internal/state/scrollback.go:296`, `:305-312`). No confirmation is sent, and the empty bytes are written.
4. The commit names `S__1.0.bin`, and `gcOrphanScrollback` (`internal/state/commit.go:130`) deletes `S__2.0.bin`.

The same deletion follows from a refused `capture-pane`, which writes nothing (`cmd/state_daemon.go:331-341`), and from a `commit-now`, which dumps nothing. In both, the commit still names the absent positional file. A writer refusal alone would not fix this: the record has already moved off the saved file, so housekeeping removes it either way. That is why the answered lazy pane needed its hold as well as the refusal.

The window is the first save to reach the pane after its hydration, from a few seconds up to about 30s. If a shutdown lands in it, the pane restores with no scrollback and the bytes cannot be recovered. In that state, CLAUDE.md's "an unconfirmed one is not written, the saved transcript stands" is false for this pane.

**Solution**: The hold covers every pane that carries a token and that the dump may write, whenever its last committed record (matched on its token) names a file other than its live positional file. One more condition applies when that file is another address's positional file: no other record in the commit may name it. A token-named transcript is held exactly as it is today.

A held pane keeps naming its committed file until a dump writes its new capture with confirmed bytes. Until then the writer judges an empty capture against that file. The pane goes through the existing `held` / `fileAtPositional` hand-back, so a confirmed write files it at its positional path, and that commit's housekeeping removes the old file.

Some panes stay judged at their own positional file, as today:
- a pane carrying no token, which has no durable identity to match a record on;
- a pane whose committed file another record in the commit names, such as a new pane occupying the vacated address.

The doc comments on `RunCommitCycle`, `ScrollbackWriter.held`, `keepAnsweredTranscripts` and `heldTranscripts` are widened to say this.

This is derived from the record. §2.4 states the general measure, and this extends the approved phase 3 Task 1 and cycle 2 Task 1 hold from the token-named case to every file a last commit can name, without reversing it. The no-other-record condition is the finding's own. It keeps the commit from putting two records on one file, which `captureAndRefile`'s contract already forbids.

**Outcome**: A tokened pane that restore placed away from its saved address keeps its saved transcript, and so does a pane tmux has renumbered or whose session was renamed. Every commit names that transcript and leaves it on disk, whether the pane's capture comes back empty, is refused, or no dump runs (`commit-now`). This lasts until a dump writes the pane's capture with confirmed bytes. Only then does a commit file the pane at its positional path and reclaim the old file.

**Acceptance Criteria**:

The moved pane in the first three criteria carries a token and is one the dump may write: not waiting, not mid-restore, not in a carried session. Its last committed record names another address's positional file that holds its transcript, and no other record in the capture names that file. Three ways produce it: restore closed a window-index gap, so the pane was saved at `S:2.0` on `scrollback/S__2.0.bin` and is now live at `S:1.0` with no live pane at `S:2.0`; tmux renumbered its window; or its session was renamed since that commit.

- [ ] A daemon tick or shutdown flush takes an empty capture of the moved pane, and the read after it is refused, answered by another server on the socket, or answered naming no server. The cycle writes nothing for the pane and logs the refused write as today. The commit names the saved file, which keeps its bytes, and nothing is written at the pane's live positional path.
- [ ] Some cycles reach the moved pane without writing its capture: its `capture-pane` is refused, a `commit-now` dumps nothing, or a shutdown flush dumps nothing. Every such commit, one after another, names the saved file and leaves it on disk with its bytes, until a dump writes the pane's capture.
- [ ] A dump writes the moved pane's capture: either non-empty, or empty and confirmed by the committer's own server against the saved file, which takes one confirmation read. The commit names the pane's live positional file, which holds that capture, and that commit's housekeeping removes the old file.
- [ ] A tokened pane's saved file is also named by another record in the capture. That record may be a pane now live at the saved address, including another moved pane whose live address that is. The tokened pane is judged at its own positional file as today: an empty capture over its absent positional file is written with no confirmation read. No commit names one file on two records.
- [ ] A tokenless pane restored one window lower is judged at its own positional file as today. Its empty capture over its absent positional file is written with no confirmation read, and the commit names its positional file.
- [ ] An answered lazy pane whose last committed record names its token-named transcript is held exactly as today.

**Do**:
- The hold lives in `internal/state/commit_cycle.go`, in two functions:
  - `keepAnsweredTranscripts` (:157) points a pane's record back at the file its last committed record names.
  - `heldTranscripts` (:179) gives that file to `ScrollbackWriter`.

  Both take the hold only when the record names `PendingScrollbackFile(token)` (:169, :185). Widen that test to the Solution's rule:
  - The pane carries a token and the dump may write it.
  - Its last committed record names a file other than its live positional file. That record is matched on token from the cycle's previous index, as today: `sessions.json` read under the lock, else `LoadPrev`.
  - When that file is another address's positional file, no other record in the commit names it.
  - A token-named transcript keeps today's hold with no new condition.
- Send a held pane through the existing hand-back. `ScrollbackWriter.Write` (:73-94) judges its empty capture against the held file and records a confirmed write in `captured`. `fileAtPositional` (:195) then files the pane at its positional path, and the commit's housekeeping (`gcOrphanScrollback`, `internal/state/commit.go:130`) removes the old file.
- A tokenless pane, and a pane whose saved file another record names, stay judged at `ScrollbackFile(w.dir, paneKey)` as today.
- Widen the doc comments on `RunCommitCycle`, `ScrollbackWriter.held`, `keepAnsweredTranscripts` and `heldTranscripts` to describe the widened hold.
- Existing tests already cover the token-named hold this task leaves alone: `internal/state/commit_cycle_answered_test.go`, `internal/state/commit_cycle_answered_disk_test.go` and `cmd/state_daemon_empty_capture_test.go`.
