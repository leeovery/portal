---
status: in-progress
created: 2026-06-07
cycle: 2
phase: Plan Integrity Review
topic: Session Tagging and Grouping
---

# Review Tracking: Session Tagging and Grouping - Integrity

## Cycle-1 fix verification (no finding)

The two cycle-1 dependency-ordering findings are confirmed applied and sound in the tick store (`tick-7d0ed3`):

- **2-2** (`tick-4358f8`, By Project builder) now `blocked_by` **[2-1 `tick-0ccac8`, 1-6 `tick-596ae9`, 1-4 `tick-5a49ee`]** — matches cycle-1 finding 1's proposed edge (2-2 ← 2-1).
- **2-3** (`tick-dc8a90`, By Tag builder) now `blocked_by` **[2-1 `tick-0ccac8`, 1-4 `tick-5a49ee`, 1-2 `tick-b97407`]** — matches cycle-1 finding 2's proposed edges (2-3 ← 2-1, 2-3 ← 1-4).

Behavioural verification: `tick ready --parent tick-7d0ed3` no longer surfaces 2-2 or 2-3 in the ready set (both are correctly held by their open blockers), so the priority-1 sort can no longer offer the builders ahead of the struct they depend on. The executable inversion cycle 1 reported is closed.

**Cascade check — acyclicity and ordering:**

Full leaf-task edge set (dependent → predecessor) in the feature graph:
- 2-2 → {2-1, 1-6, 1-4}
- 2-3 → {2-1, 1-4, 1-2}
- 3-3 → {2-2, 2-3, 3-1}
- 3-8 → {2-5}
- 3-9 → {3-2}
- 4-5 → {1-3}
- 4-7 → {3-3}

Every edge points to a strictly-earlier task (earlier phase, or earlier internal ID within a phase). The three cycle-1 additions (2-2←2-1, 2-3←2-1, 2-3←1-4) are all backward edges within/across phases and introduce no back-reference. **The graph remains acyclic (a DAG); no cycle, no newly-inverted execution order, and no new gap was introduced by the cycle-1 fixes.** 3-3's predecessor set (2-2, 2-3, 3-1) is complete — it dispatches to both grouping builders and imports the `prefs.SessionListMode` enum delivered by 3-1.

## Findings

### 1. Convergence predecessor 2-1 sits below the priority-1 critical-path tasks it unblocks

**Severity**: Minor
**Plan Reference**: Phase 2 — task `session-tagging-and-grouping-2-1` (`tick-0ccac8`, priority 2), which unblocks `session-tagging-and-grouping-2-2` (`tick-4358f8`, priority 1) and `session-tagging-and-grouping-2-3` (`tick-dc8a90`, priority 1)
**Category**: Dependencies and Ordering (priority assignment reflects graph position)
**Change Type**: update-task (priority metadata only)

**Details**:
The plan carries a deliberate priority-1 critical path — the seven tasks elevated to priority 1 are exactly the chain that produces a working grouped render: 1-2 (`NormaliseTag`), 1-4 (`CanonicalDirKey`/`MatchProjectByDir`), 1-6 (`Session.Dir`) → 2-2 (By Project builder), 2-3 (By Tag builder) → 3-1 (prefs store), 3-3 (mode-aware re-render core). Every other task in the feature is priority 2.

Task 2-1 (`Extend SessionItem with group metadata`) is a convergence-point predecessor *on that same critical path*: both 2-2 and 2-3 now `blocked_by` it (the cycle-1 fix), and both are priority 1. Yet 2-1 itself is left at priority 2 — it is the single node on the critical path that was not elevated. The result is a priority that does not reflect graph position: a foundation task is ranked below the two tasks it gates.

This is **not a correctness defect** — the cycle-1 `blocked_by` edges guarantee 2-2/2-3 are never offered before 2-1 completes, regardless of priority, so execution order is safe. The impact is purely scheduling intent: because 2-1 shares priority 2 with unrelated tasks (e.g. 1-1, 1-5, 1-7, 1-8, and the entire Phase 4 leaf set), the global `tick ready` ordering can interleave lower-value priority-2 work ahead of 2-1 once Phase 1's primitives are done — delaying the gate to the two priority-1 builders and partially undermining the critical-path intent the priority-1 banding was designed to express. Cycle 1 noted it was leaving priorities as-is once the edges existed (correct for the inversion it was fixing); this finding completes the alignment by bringing the one straggler onto the same band as its siblings.

Raising 2-1 to priority 1 makes the critical-path banding internally consistent (the predecessor is offered no later than the work it unblocks) and changes nothing about correctness, since the existing edges already enforce the order.

**Current** (task `session-tagging-and-grouping-2-1`, priority metadata):
> priority: 2

**Proposed** (task `session-tagging-and-grouping-2-1`, priority metadata):
> priority: 1
>
> Rationale: 2-1 is a convergence-point predecessor of two priority-1 critical-path tasks (2-2 and 2-3 both `blocked_by` 2-1). Elevating it to priority 1 aligns its graph position with the critical-path banding (1-2 / 1-4 / 1-6 / 2-2 / 2-3 / 3-1 / 3-3) so the gate to the grouped-render builders is scheduled no later than the builders themselves. Correctness is unchanged — the existing `blocked_by` edges already enforce 2-1 before 2-2/2-3; this only corrects the priority-versus-graph-position mismatch.

**Resolution**: Pending
**Notes**:

---
