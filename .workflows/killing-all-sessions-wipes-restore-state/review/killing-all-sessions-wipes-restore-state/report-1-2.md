TASK: A Refused Confirmation Read Stands The Commit Cycle Down (killing-all-sessions-wipes-restore-state-1-2, tick-21ac06)

ACCEPTANCE CRITERIA:
- The saved state holds sessions and their scrollback files. A cycle's capture reads are all answered, then its confirmation read is refused. The cycle returns an error, `sessions.json` is unchanged, and every scrollback file is still present with its content. This holds whether the cycle is the daemon's tick, its shutdown flush or `commit-now`.
- A capture's `list-sessions` comes back with exit status 0 and no output, and the server then refuses connections. The cycle writes nothing and deletes no scrollback file.
- A capture's `list-panes` comes back with exit status 0 and no output, the environment reads after it still succeed, and the server then refuses connections. No session is recorded without its windows, `sessions.json` is unchanged, and no scrollback file is deleted.
- The server begins exiting just after one of a capture's reads and refuses every connection after it. Whichever read that is (the session listing, the pane listing, or any one session's environment read), the cycle writes nothing and deletes no scrollback file.
- The confirmation read is sent strictly after the last capture read and comes back with exit status 0 and no output. The cycle writes its commit.
- A capture's `list-sessions` names only Portal's own sessions, because the last user session has been killed, and the confirmation is answered. The cycle commits the empty index, and its housekeeping pass removes the scrollback files the index no longer names.
- In every cycle that writes a commit, the confirmation read is sent after the last of the capture's reads.

STATUS: complete

SPEC CONTEXT: §2 puts the three committers (daemon tick, shutdown flush, commit-now) through one shared cycle. §2.2 requires a tmux read sent strictly after the last capture read before any commit, resting on tmux refusing new connections once it begins exiting and never stopping. That catches the shutdown answers (exit 0, empty `list-sessions`/`list-panes`) that no error check sees. §2.5 says a refused confirmation writes no commit and runs no housekeeping, and ends through each committer's failure route. §5.1 says a genuinely empty session list (the last user session killed) must still commit. The 2026-10-05 corrigendum changes §2.2. An exit-0 answer with no output names no server, fails the own-server rule and stands the cycle down. It no longer counts as answered.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/state/scrollback.go:356-375 (`captureAndRefile`): skeleton-marker read, then `captureStructure`, then `confirmOwnServer` at :366. A refusal returns an empty cycle wrapped in `ErrTmuxStoppedAnswering` before any link, re-file, dump or commit.
  - internal/state/scrollback.go:267-282 (`confirmOwnServer`): exit 0 naming the own server is the only answer that passes.
  - internal/state/scrollback.go:380-388 (`classifyFailedCapture`): a failed capture is classified by a confirmation sent after it.
  - internal/state/capture.go:95-148: the session listing goes through `ListSessionNamesProbe`, then `list-panes`, then one `show-environment` per session. Everything after the env loop is pure merging, so the last env read is the last tmux read the index is built from.
  - internal/state/commit_cycle.go:127-130: `RunCommitCycle` returns on the capture error before the dump and before `commitOver` (internal/state/commit.go:35-61, which holds the write and `gcOrphanScrollback`). So a stand-down writes nothing and runs no housekeeping pass.
  - internal/tmux/tmux.go:92-105 (`ConfirmAnswering`): `display-message -p '#{pid}'`. A failed run is returned as an error, and empty output returns pid 0.
  - All three committers enter through `RunCommitCycle`: cmd/state_daemon.go:267 (the tick, and the flush via `captureAndCommit` at :389) and cmd/state_commit_now.go:109.
- Notes: The fifth criterion (an empty-output confirmation commits) is deliberately reversed in the code. An empty answer now stands the cycle down with `ErrNotOwnServer` (internal/state/scrollback.go:275-277). The 2026-10-05 corrigendum records this with its reason and its cost (one stood-down save), so it is a sound divergence and not a finding. No tmux read happens between the confirmation and the commit. `keepAnsweredTranscripts`, `linkMovedSkeletonScrollback` and `refilePendingScrollback` touch only the filesystem. The dump's `capture-pane` reads produce scrollback bytes, not index structure, and an empty one gets its own confirmation in `ScrollbackWriter.Write`.

TESTS:
- Status: Adequate
- Coverage:
  - Refused confirmation after all capture reads answered, cycle level: `TestRunCommitCycleStandsDownOnARefusedConfirmation` (internal/state/commit_cycle_confirm_test.go:187). It asserts the `display-message` CommandError is returned and that sessions.json and every transcript's bytes are unchanged.
  - The same across the daemon's tick, its shutdown flush and commit-now: `TestCommittersStandDownOnARefusedConfirmation` (cmd/state_commit_confirmation_test.go:60). `TestCommitNowFailsOnARefusedConfirmation` (:76) adds commit-now's non-zero exit.
  - Empty `list-sessions` and empty `list-panes` beside succeeding env reads: `TestRunCommitCycleWritesNothingFromAShutdownAnswer` (internal/state/commit_cycle_confirm_test.go:201).
  - Server exiting after each capture read (markers, session listing, pane listing, first env, last env): `TestRunCommitCycleWritesNothingWhenTheServerExitsAfterAnyCaptureRead` (:237).
  - Empty-output confirmation, reversed per the corrigendum: `TestRunCommitCycleStandsDownOnAConfirmationNamingNoServer` (:306).
  - Empty index commits and housekeeping clears transcripts: `TestRunCommitCycleCommitsOnAConfirmationFromItsOwnServer` (:267).
  - Confirmation ordered after the last capture read: `assertConfirmedAfterCaptureReads` (:174) at cycle level, and `TestCommittersConfirmAfterTheLastCaptureRead` (cmd/state_commit_confirmation_test.go:99) for all three committers.
  - `ConfirmAnswering` against real tmux (live, exited, and a successor on the same socket): internal/tmux/confirm_answering_realtmux_test.go:12.
- Notes: The tests would fail if the confirmation were removed: the "work"-only capture would commit and drop "notes". They would also fail if it were moved before the env reads: the last-env-read split would commit. The `exitingServer` fake models the exit-begins-then-refuse property directly instead of scripting argv, which keeps the cases short. Some later-task tests in the same file overlap with these (own server, classification), but each asserts a distinct property.

CODE QUALITY:
- Project conventions: Followed. Small interfaces (`AnsweringConfirmer`, `CaptureCycleClient`), `%w` wrapping with sentinels, no `t.Parallel`, cmd seams staged through `withCommitNowDeps`, and no process-artifact references in comments.
- SOLID principles: Good. The confirmation lives in the one shared cycle instead of in each committer.
- Complexity: Low.
- Modern idioms: Yes (`errors.AsType`, multi-`%w`).
- Readability: Good. The doc comments on `captureAndRefile`, `confirmOwnServer`, `ErrTmuxStoppedAnswering` and `ConfirmAnswering` match the code.
- Issues: None.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
