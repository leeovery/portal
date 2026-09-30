# Analysis Report: Lazy Resume On Attach (Cycle 10)

## Stats

- Total findings: 4
- Deduplicated findings: 4
- Proposed tasks: 2

## Summary

The duplication pass found nothing that clears the floor. The architecture pass found that the parked chain's shell backstop copies the recovery tail's steps without the tail's answered-pane gate. The consequence is confirmed in the tree: a pane answered on its panel takes two exits to close once its baked binary has gone. That finding is staged as a medium proposal. Two low findings cluster, because each concerns a failure record the specification says must carry something it currently drops. The panel's report row truncates away the reason for a refused answer, and the chain's WARN records name the pane only by its restore-time positional key. Settling the first moves the refusal's technical detail into the log, which is where the second applies, so they are staged together as one proposal. The third low finding stands alone and is discarded. It concerns the discard removing a replacement registration. One comment correction is collected. No Decision is staged.

## Comment Corrections

- cmd/state_resume_wait_test.go:459 — restates the premise the 2026-09-22 and 2026-09-28 corrigenda removed: the parked shell is the pane's own process and the waiter is its child
  OLD: // The waiter is the pane's only process: declining a hangup would outlive the
// destruction of its own pane, leaving a Portal process per culled session.
  NEW: // A waiter declining a hangup would outlive the destruction of its own pane,
// leaving a Portal process per culled session.

## Discarded Findings

- **A confirmed discard deletes a replacement registration the confirmation never showed** (standards, low). Discarded because it is low severity and does not cluster with the other findings.
  - **The drift is real.** The specification names a registration "replaced by a re-registration" as a discard that finds nothing to remove. `resumeAnswerDiscard` (`cmd/state_resume_wait.go:359`) calls `store.Discard` through `discardResumeRegistration` (`:433`). `removeEntry` (`internal/hooks/store.go:213-237`) then deletes whatever `on-resume` value the key holds and never compares it with `cfg.Command`, the command the confirmation showed.
  - **Why it stands alone.** The two clustered findings concern what a failure record carries. This one concerns what the act removes, and its fix, a command comparison inside the store's discard, shares nothing with theirs.
  - **Why its severity stays low.** It is reached only when the user rewrites a waiting pane's registration from outside that pane, by a hand edit or a `hook set` aimed at it through `$TMUX_PANE`, and then discards on the panel. The removed command is still recoverable from the `op=discard value=…` INFO line the store writes.
  - **Not a reversal.** No settled direction touches the discard's target. It was dropped for severity and the lack of a cluster, not on its merit, and would be revived as its own proposal.
