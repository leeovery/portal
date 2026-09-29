# Analysis Report: Lazy Resume On Attach (Cycle 6)

## Stats

- Total findings: 3
- Deduplicated findings: 3
- Proposed tasks: 2

## Summary

The architecture pass found a data-loss race, confirmed in the tree. The saver's tick and `portal state commit-now` each capture, re-file, commit and run the housekeeping pass, and nothing orders one against the other. During a pane's restore-to-wait hand-over, the committer with the older view can therefore delete the token-named transcript the other one just filed. That finding is staged with a settled direction: one bounded exclusive lock over the whole committing cycle. No Decision is staged.

The standards pass found a stale CLAUDE.md sentence about the address match; it is staged as a one-edit Corrections proposal. The standards pass's comment correction is collected below. The duplication pass's one finding is low severity, stands alone, and is discarded.

## Comment Corrections

- cmd/state_hydrate.go:385-387 — the resume chain's draw, wait and recover hops now exec through `defaultExecShell` too. A failed draw-to-wait hand-off exits into the parked shell's recovery tail, and the pane does not close, so the purpose clause is false for those callers.
  OLD: // syscall.Exec returns only on failure. That path must still terminate non-zero
  // so the pane closes, and must do so via log.Close(1) + osExit(1), or the
  // just-emitted exec marker stands as a phantom handoff.
  NEW: // syscall.Exec returns only on failure, and that path must end through
  // log.Close(1) + osExit(1), or the just-emitted exec marker stands as a phantom
  // handoff.

## Discarded Findings

- **The parked chain's backstop hand-composes the pending-marker clear that the tmux client already owns** (duplication, low). Discarded because it is low severity and does not cluster with any other finding.
  - **The copy is real.** `parkedChainBackstop` spells `tmux set-option -pu -t <pane> @portal-resume-pending` itself (`cmd/state_hydrate.go:292-294`). `Client.UnsetPaneOption` owns the same argv (`internal/tmux/tmux.go:329-334`). The backstop test pins the literal against a stub tmux (`cmd/state_resume_backstop_test.go:127-130`).
  - **The drift needs two things together.** First, a change to how `Client.UnsetPaneOption` unsets a pane option. That is tmux's own `set-option -pu`, and nothing in the tree or in tmux is moving it. Second, a waiting pane whose baked binary has gone from its path, which is the only way the backstop is reached.
  - **Not a reversal.** The recommendation extends phase 7's settled direction, which composes the backstop from the tail's Go values so the two routes cannot drift; it does not reverse it. The ledger does not bar it, and the discard rests on severity and the lack of a cluster.
