# Analysis Report: Killing All Sessions Wipes Restore State (Cycle 4)

## Stats

- Total findings: 2
- Deduplicated findings: 2
- Proposed tasks: 2

## Summary

The standards agent found nothing that clears the floor. The duplication and architecture agents each reported one finding, and each guards the answered lazy pane's transcript, so both are proposed.

- **Task 1 (medium).** The real-tmux lazy-resume fixture's capture round writes around the cycle's `ScrollbackWriter`. Its suites therefore never exercise the daemon's answered-pane write path, and the discard suite's "resumes writing" check passes on a write the same commit deletes.
- **Task 2 (low).** `RunCommitCycle` still runs its merges and carry from the caller's previous index. For the daemon, that index can lag the `sessions.json` the cycle has already read under the lock. Traced through the tree, the carry of an answered pane in a session that missed the capture deletes the pane's only transcript.

**Settled directions.** Task 1 reverses nothing. It extends cycle 1 Task 1, which made the writer the daemon's only write route, and leaves `WriteScrollbackIfChanged` exported for fixtures as that task set it.

Task 2 reverses cycle 2 Task 1's direction that `LoadPrev` keeps feeding the skeleton, waiting-pane and carry merges. It is kept, and names its grounds:
- The traced loss contradicts the §2.4 corrigendum's guarantee, recorded from phase 3 Task 1, and §1.1.
- That direction rested on two cost reasons from the phase 3 fix round: an extra decode, and moving the fix out of `internal/state`. Neither holds now. Cycle 2 already reads `sessions.json` under the lock, and the change sits inside `RunCommitCycle`.
- The daemon's in-memory index differs from the disk only after another committer has committed, which is the case §2.4 measures against the last committed record.

I verified each step of the trace against the tree:
- the carry copies from `LoadPrev` (`internal/state/capture.go:163-166`, `:245-270`);
- `SkipsScrollback` covers carried panes (`internal/state/scrollback.go:335-342`), so the hold (`internal/state/commit_cycle.go:159`) and the dump skip them;
- only pending and carried-waiting panes are re-filed (`internal/state/scrollback.go:370-373`).

**Severity.** Task 2 is graded low. It is kept, rather than discarded as a lone low, on this work unit's precedent: cycle 2 Task 1 and both cycle 3 tasks were kept for the same reason. The loss cannot be undone and stays invisible until the next restore. It is also the latest instance of one pattern, the caller's index lagging the disk, which the phase 3 fix round, `linkStoredScrollback`'s adoption and cycle 2 Task 1 each repaired locally.

## Discarded Findings

- **None discarded at synthesis.** The standards agent reported no findings.
