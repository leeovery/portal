# Analysis Report: Killing All Sessions Wipes Restore State (Cycle 1)

## Stats

- Total findings: 4
- Deduplicated findings: 4
- Proposed tasks: 4

## Summary

The production code matches the specification. The duplication agent found nothing that clears the floor. Of the four findings kept, one is structural: the empty-capture half of the shutdown-wipe rule is enforced only because the daemon's dump calls `ConfirmEmptyCapture`, unlike the commit, which the cycle and a guard enforce (Task 1). The other three are a back-off classification gap that carries the approved phase 1 Task 1 further to the environment reads (Task 2), CLAUDE.md still describing the old rename-based re-file and the untrapped resume shells (Task 3), and a commit-now test subtest that passes whatever the listing does (Task 4, Corrections).

Phase 2's consolidation recorded corrections to the Resume hooks wording at CLAUDE.md:192, and they are not in the tree. Task 3 covers that sentence and three more places.

No proposal reverses a settled direction. Task 2 only extends phase 1 Task 1: on a healthy server it leaves that task's "all sessions anomalous stays failed" line, and every one of that task's acceptance tests, exactly as they are.

## Comment Corrections

- cmd/state_hydrate.go:322 — the claim that only a gone pane reads as clear is false for a read already in flight when tmux begins exiting (exit 0, no output), which is the shutdown case the draw loop now runs in
  OLD: // marker reads back clear; a read that fails counts as still pending. A plain
// format read serves: the one pane it misreads as clear is a gone one, which
// wants nothing more from the chain.
  NEW: // marker reads back clear; a read that fails counts as still pending. A plain
// format read serves: a pane it misreads as clear is gone or on a server
// already exiting, and wants nothing more from the chain.

- cmd/state_resume_chain.go:215 — the command does not occupy its own argv slot; it is spliced into the one -c script between the TERM trap and the exec
  OLD: // hookExecArgs composes the argv a pane's registered command is run as. The
// command occupies its own argv slot so sh's parser handles any embedded quotes
// - Portal never interpolates it - and the trailing exec leaves the pane on its
// own shell, so it closes on the first exit.
  NEW: // hookExecArgs composes the argv a pane's registered command is run as. The
// command is spliced into the -c script unquoted, so sh's own parser handles any
// embedded quotes, and the trailing exec leaves the pane on its own shell, so it
// closes on the first exit.

## Discarded Findings

- None discarded at synthesis. The duplication agent reported no findings. It ruled the candidates its summary names below the floor itself: the two shell trap constants (each pinned by its own SIGTERM/SIGHUP tests), the `readPriorIndex`/`ReadIndex` re-reads through the shared `DecodeIndex`, and the TMUX values in the integration tests.
- Three findings are low-severity and were kept anyway:
  - The environment-read back-off gap carries the approved phase 1 Task 1 further, and spec §4.2 requires it.
  - The CLAUDE.md drift matches four phase 2 bank entries about the same drift, and the document names two deleted files.
  - The test subtest is a guard that passes while checking nothing for a contract §6.1 requires. It is folded into Corrections.
