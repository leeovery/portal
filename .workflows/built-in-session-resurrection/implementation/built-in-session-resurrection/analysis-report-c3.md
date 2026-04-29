---
topic: built-in-session-resurrection
cycle: 3
total_findings: 15
deduplicated_findings: 13
proposed_tasks: 9
---
# Analysis Report: built-in-session-resurrection (Cycle 3)

## Summary

Cycle 3 surfaced two high-severity duplications (twin `restoreOrchestratorAdapter`; `socketCommander` re-deriving `RealCommander`), four medium items (stale `createSkeleton` doc-comment, OpenFile triplet, pane-target Sprintf x5, `tmux.NewClient(&RealCommander{})` x7), and several low items spanning rule-of-three duplications, orphaned NoOp types, vestigial helpers, and minor spec/code drift. The duplication-high "twin adapter" finding and the architecture-low "FIFO sweep conflated with restore adapter" finding intersect at the same code site and have been merged into a single task that addresses both concerns. Production-adapter naming asymmetry is folded into the same consolidation task. The implementation tracks the spec on load-bearing decisions; remaining drift is small.

## Discarded Findings

- `fmt.Errorf("ensure state dir: %w", err)` repeated 3x (duplication, low) — agent itself recommends defer; only 2 lines tightly co-located with each RunE; extraction would not pay back indirection cost. Re-evaluate if a fourth `state_*` subcommand lands.
- Spec L1031-1038 mkfifo ordering inconsistency (standards, low) — spec-text drift only; folded into consolidated spec-update task.
- Hydrate helper marker-unset / hook-lookup ordering reversed vs spec (standards, low) — functionally inert; folded into the same spec-update task.
- Spec L1032 mkfifo CreateFIFO semantics narrower than implementation (standards, low) — folded into the same spec-update task.
- Production adapter naming asymmetric with internal/bootstrapadapter (architecture, low) — folded into the consolidated `restoreOrchestratorAdapter` task as a naming/comment decision.
