# Review Report: killing-all-sessions-wipes-restore-state (Cycle 1)

## Stats

- Total findings: 6
- Deduplicated findings: 4
- Proposed tasks: 1

## Summary

Prep left one replan action (A3), which merges 1-6-1, 7-1-1 and 7-2-1. In the first daemon cycle after a pane moves off its saved address, the dump can overwrite a positional scrollback name that `sessions.json` still names on another pane's record. If that cycle then ends uncommitted, the moved pane's record names a file that holds another pane's transcript. The staged task closes this rather than recording it as residue: §1.1 says every case the fix leaves short of its rule is recorded in §5.2, this case is not, and task 1-6's fourth criterion requires the moved pane's transcript at the path its record names. The task also folds in the stale `held` field doc. Both do-now corrections are already committed in 4379aabb9, and the order-dependent integration test is held as out of scope.

## Spec Defects

None. The specification states the guarantee correctly: §1.1, §2.3's stated purpose, §2.4's moved-pane paragraph, and the absence of this case from §5.2. The code falls short of it.

## Discarded Findings

- OwnServer doc said a zero own server stands every cycle down (1-3-1, A1). Routed do-now and applied in 4379aabb9. The doc now reads "zero commits nothing, and sends no confirmation".
- Real-tmux rename test wrote the developer's zsh history (test-surface-1, A2). Routed do-now and applied in 4379aabb9. `TestCommitRealTmuxTellsARenameFromADropBesideANewSession` now calls `IsolateStateForTest` and `RegisterStateDirTeardownGuard` before `tmuxtest.New`.
- `TestLazyResumePaste_NeverAnswersAWaitingPane` fails after the discard burst test (1-problem-and-governing-rule-2-the-save-path-stops-trusting-an-unconfirmed-session-list-1, A4). Routed out-of-scope. It reproduces at the base commit b218a434e, and no commit in this change-set touched the test or `cmd/tty_drain.go`. It is held for separate work, with its guard conditions recorded on the action.
