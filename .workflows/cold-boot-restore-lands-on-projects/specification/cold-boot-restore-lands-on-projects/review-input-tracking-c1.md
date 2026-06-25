---
status: complete
created: 2026-06-25
cycle: 1
phase: Input Review
topic: Cold-Boot Restore Lands on Projects
---

# Review Tracking: Cold-Boot Restore Lands on Projects - Input Review

## Findings

### 1. Concrete reproduction recipe (demo harness) absent from the spec

**Source**: Investigation §Symptoms → Reproduction Steps (lines 20-27), Manifestation (lines 13-18), Environment (lines 29-33), References (lines 41-45)
**Category**: Enhancement to existing topic
**Affects**: Context: Defect & Root Cause (a new "Reproduction" sub-section under it), and/or Testing Requirements

**Details**:
The investigation pins down a fully concrete, repeatable reproduction the spec drops entirely. The defect was discovered and is repeatable in the `demo/` sandboxed Linux container via `demo/portal-cold.tape`, with a baked restore seed of **12 sessions** and 10 projects. The exact observable signature is named: the loading screen shows `✓ Restoring sessions 12/12 · ✓ Replaying scrollback · ✓ Running resume commands`, the picker opens on Projects with footer `x sessions`, and pressing `x` reveals all 12 sessions with correct names/scrollback. The repro steps are: (1) cold container, no tmux server, `sessions.json` + scrollback present; (2) `portal open`; (3) picker opens on Projects; (4) press `x` to reach Sessions.

The spec describes the defect abstractly ("N sessions restored") but contains no reproduction recipe and no reference to the demo harness. For a bugfix, the concrete reproduction artifact is the load-bearing manual-verification path that complements the programmatic regression tests already specified — a reviewer/implementer verifying the fix end-to-end needs to know it lives in `demo/portal-cold.tape` with the 12-session seed.

**Current**:
The spec's "Context: Defect & Root Cause" section describes symptoms abstractly (specification.md lines 7-17) and has no reproduction sub-section. The "Testing Requirements" section (lines 67-80) covers only the unit/regression tests in `internal/tui/`, with no mention of manual/demo-harness verification.

**Proposed Addition**:
New "Reproduction" sub-section under Context: Defect & Root Cause (after "Affected path", before "Root cause") with the `demo/portal-cold.tape` recipe, 12-session/10-project seed, the exact `✓ Restoring sessions 12/12 …` loading signature, and the 4-step repro.

**Resolution**: Approved
**Notes**: Added verbatim via finding auto-mode. Placed after "Affected path".

---

### 2. Existing-test symbol names omitted from the Testing Requirements

**Source**: Investigation §Why It Wasn't Caught (line 120): names `driveColdBootToSessions` and `TestColdBoot_PostCompleteRefetch_ReflectsRestoredSessions` inside `internal/tui/coldboot_session_refetch_test.go`
**Category**: Enhancement to existing topic
**Affects**: Testing Requirements

**Details**:
The spec correctly identifies that `internal/tui/coldboot_session_refetch_test.go` passes for the wrong reason and must be fixed, but it references only the file. The investigation names the exact failing-for-the-wrong-reason symbols: the driver helper `driveColdBootToSessions` and the test `TestColdBoot_PostCompleteRefetch_ReflectsRestoredSessions`. Naming the specific symbols pins down precisely which test/driver must be updated (or whether the new `ProjectsLoadedMsg`-delivering harness extends `driveColdBootToSessions` vs. a new driver), removing ambiguity for the implementer.

**Current**:
"The existing cold-boot test (`internal/tui/coldboot_session_refetch_test.go`) passes for the **wrong reason**: it builds the model with no project store and never delivers `ProjectsLoadedMsg`, so `projectsLoaded` stays false, `evaluateDefaultPage()` early-returns without latching, and `activePage` keeps the tentative `PageSessions` set in `transitionFromLoading()`. The assertion passes despite the empty stale snapshot." (specification.md line 69)

**Proposed Addition**:
Amend the Testing Requirements opening sentence to name the symbols: `internal/tui/coldboot_session_refetch_test.go` — the `TestColdBoot_PostCompleteRefetch_ReflectsRestoredSessions` test and its `driveColdBootToSessions` driver.

**Resolution**: Approved
**Notes**: Added verbatim via finding auto-mode (diff applied to the opening sentence).

---
