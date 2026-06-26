---
status: complete
created: 2026-06-26
cycle: 2
phase: Plan Integrity Review
topic: Cold-Boot Restore Lands on Projects
---

# Review Tracking: Cold-Boot Restore Lands on Projects - Integrity

This is cycle 2 — a follow-up integrity review after cycle 1 (clean) and the two
traceability findings that were applied between cycles: AC6 (`commandPending`) coverage
added to task 1-3, and failing-refetch-quit coverage added to task 1-4. The plan was
re-read end-to-end (planning.md, phase-1-tasks.md, and all four tick descriptions
tick-9b305e / tick-28a91d / tick-7f37e3 / tick-6fee61), and the load-bearing claims behind
the two additions were re-grounded against `internal/tui/model.go`. Per the brief, already-
clean cycle-1 assessments are not re-raised; this cycle focuses on whether the additions
hold up structurally and are well-scoped/self-contained.

## Verification of the Cycle-1 Additions (both hold up)

**AC6 / commandPending coverage on task 1-3 — sound.** The added test
(`TestCommandPending_LandsOnProjects_NoInterimFlash`) is well-scoped (one TDD-cycle test
locking one invariant), self-contained, and every cited source anchor is exact:
`WithCommand` at model.go:632-639 sets `commandPending = true` + `activePage = PageProjects`;
the `commandPending` arm of `evaluateDefaultPage()` is at 1619-1622; `Init`'s
`commandPending` branch at 1882-1883 returns `tea.Batch(requestBg, detectTimeout,
m.loadProjects())` before wiring the loading-dismissal machinery, which grounds the "never
reaches `transitionFromLoading()` / no interim Sessions flash" invariant the AC asserts.
The AC is pass/fail and behaviour-anchored (`ActivePage() == PageProjects`, never observed
on interim `PageSessions`). Present and consistent in both the detail file and tick-7f37e3.

**Failing-refetch-quit coverage on task 1-4 — sound (one Problem-statement nit, below).**
The added test (`TestColdBoot_RefetchError_QuitsWithoutStrandingInterim`) is well-scoped and
self-contained. The cited mechanism is exact: the `SessionsMsg` arm at model.go:1976-1977
returns `m, tea.Quit` when `msg.Err != nil`, before any deferred `evaluateDefaultPage()`
decision — matching "a `SessionsMsg` carrying an error continues to quit, exactly as
today". The plan's instruction to "mirror the existing `SessionsMsg`-error assertion style
already used elsewhere in the tui test surface" is grounded in real precedent
(`model_test.go:366` and `:2517` construct `tui.SessionsMsg{Err: ...}` and assert the quit).
The AC is pass/fail (returned cmd is/batches `tea.Quit`; does NOT strand the interim page).
The Solution, Do, Acceptance Criteria, Tests, and Edge Cases all correctly fold the third
invariant into task 1-4 in both surfaces.

## Findings

### 1. Task 1-4 Problem statement under-counts its invariants (says "Two", omits the failing-refetch invariant)

**Severity**: Minor
**Plan Reference**: phase-1-tasks.md — Task 1-4 (cold-boot-restore-lands-on-projects-1-4), **Problem** statement (line 172)
**Category**: Task Template Compliance / Task Self-Containment
**Change Type**: update-task

**Details**:
The cycle-1 failing-refetch-quit addition was woven correctly into task 1-4's Solution, Do,
Acceptance Criteria, Tests, and Edge Cases — but the **Problem** statement in the detail
file (phase-1-tasks.md) was not updated to introduce the third invariant. It still opens
"**Two** invariants must hold deterministically: (a) ... interim page (AC7); and (b) ...
`ProjectsLoadedMsg` arrival order ...", enumerating only two and omitting the
failing-refetch invariant the rest of the task now treats as first-class.

This is a self-inconsistency within the detail file (the Problem under-counts what the same
task's Solution/Do/AC/Tests deliver) AND a drift between the two authoritative surfaces: the
tick description (tick-6fee61) was correctly updated to "**Three** invariants must hold
deterministically" with an explicit "(c) a failing refetch `SessionsMsg` quits without
stranding the interim page". The detail-file Problem should match.

It is Minor, not Important: the Do/Acceptance Criteria/Tests fully and unambiguously specify
the failing-refetch work, so an implementer is not forced to guess or make a design
decision — but the Problem statement is the field that justifies WHY the task exists, and a
reviewer reading the detail file alone would see a two-invariant rationale paired with a
three-invariant body. Aligning the count keeps the Problem honest and the two surfaces in
sync.

**Current**:
```
**Problem**: The deferral introduces a one-Update-cycle interim window between loading-page dismissal and the post-restore refetch's `SessionsMsg`. Two invariants must hold deterministically: (a) the interim page is a valid picker page — interim `PageSessions`, never `PageLoading`, blank, or undefined — even though it briefly renders the not-yet-repaired empty session list (AC7); and (b) the landing decision is independent of `ProjectsLoadedMsg` arrival order — a `ProjectsLoadedMsg` that arrives in the interim window (after the transition, before the refetch `SessionsMsg`) must NOT latch Projects against the stale interim list, because `sessionsLoaded` is still false and `evaluateDefaultPage()` early-returns.
```

**Proposed**:
```
**Problem**: The deferral introduces a one-Update-cycle interim window between loading-page dismissal and the post-restore refetch's `SessionsMsg`. Three invariants must hold deterministically: (a) the interim page is a valid picker page — interim `PageSessions`, never `PageLoading`, blank, or undefined — even though it briefly renders the not-yet-repaired empty session list (AC7); (b) the landing decision is independent of `ProjectsLoadedMsg` arrival order — a `ProjectsLoadedMsg` that arrives in the interim window (after the transition, before the refetch `SessionsMsg`) must NOT latch Projects against the stale interim list, because `sessionsLoaded` is still false and `evaluateDefaultPage()` early-returns; and (c) a failing post-restore refetch `SessionsMsg` (one carrying an error) quits without stranding the picker on the interim `PageSessions` — degrading to today's quit UX rather than running the deferred decision.
```

**Resolution**: Fixed
**Notes**: Applied to phase-1-tasks.md Task 1-4 Problem statement (now "Three invariants" with the explicit (c) failing-refetch invariant). tick-6fee61 already reads "Three invariants" — surfaces aligned. The proposed Problem mirrors tick-6fee61's "Three invariants" framing verbatim in
intent and reuses task 1-4's own existing wording for the failing-refetch invariant (its
AC: "runs `tea.Quit` before the deferred decision and does NOT strand the picker on the
interim `PageSessions`"; spec §Constraints "Failing refetch degrades to today's quit"). No
other field of task 1-4 changes. If the orchestrator applies this, also confirm tick-6fee61
already reads "Three" (it does) so the two surfaces remain aligned.

---

## Dimension-by-Dimension Assessment (cycle 2)

### 1. Task Template Compliance — PASS (with finding #1)
All four tasks carry every required field. The cycle-1 additions added well-formed,
pass/fail ACs and edge-case tests. The single exception is the task 1-4 Problem
under-count flagged in finding #1 (Minor).

### 2. Vertical Slicing — PASS
Unchanged from cycle 1 and unaffected by the additions. 1-1 is the production vertical
slice; 1-2/1-3/1-4 lock distinct behavioural facets. The AC6 and failing-refetch tests are
each complete, independently verifiable behaviours (not technical layers).

### 3. Phase Structure — PASS
Single phase remains correctly right-sized; the additions did not change phase boundaries
or the "Why this order" rationale.

### 4. Dependencies and Ordering — PASS
1-2/1-3/1-4 remain `blocked_by` 1-1 (verified in tick: tick-28a91d / tick-7f37e3 /
tick-6fee61 each list tick-9b305e as the sole blocker). No circular deps. Priority ordering
unchanged (1-1 priority 1; 1-2/1-3/1-4 priority 2). The additions are within their existing
tasks, so no new edges are needed.

### 5. Task Self-Containment — PASS (with finding #1)
Each task still carries the full Problem/Solution/Do/Context to execute standalone. The
AC6 and failing-refetch additions pull their spec invariants forward verbatim and name
exact source anchors. The only self-containment soft spot is finding #1: the 1-4 Problem
field does not name the third invariant it now delivers.

### 6. Scope and Granularity — PASS
The additions did not push any task over one TDD cycle. 1-3 gains one focused
`commandPending` test; 1-4 gains one focused failing-refetch test. Each test's intent is
statable in one sentence and touches one architectural surface (the TUI Update cycle).

### 7. Acceptance Criteria Quality — PASS
The added ACs are pass/fail and behaviour-anchored: `ActivePage() == PageProjects` and
"never observed on interim `PageSessions`" (AC6); "returned cmd is (or batches) `tea.Quit`"
and "does NOT strand ... interim `PageSessions`" (failing refetch). Both name the exact
asserted state, not "code exists".

### 8. External Dependencies — N/A
Bugfix work type; criterion is epic-only.

## Source-Grounding Verification (cycle-1 additions)

- `WithCommand` (model.go:632-639) sets `commandPending = true` + `activePage = PageProjects`
  — exactly as task 1-3 cites.
- `evaluateDefaultPage()` `commandPending` arm at model.go:1619-1622 — exact.
- `Init`'s `commandPending` short-circuit at model.go:1882-1883 returns before wiring the
  loading-dismissal machinery — grounds the "never reaches `transitionFromLoading()`" claim.
- `SessionsMsg` arm at model.go:1976-1977 returns `m, tea.Quit` on `msg.Err != nil` before
  any deferred decision — grounds the failing-refetch invariant.
- Existing `SessionsMsg{Err: ...}` → quit assertion precedent at model_test.go:366 and :2517
  — grounds the "mirror the existing assertion style" instruction.
- Constructors/accessors named by the additions all present:
  `WithProgressReceiver` (803), `WithServerStarted` (785), `WithProjectStore` (748),
  `ActivePage` (533), `InitialFilter` (461), `SessionListFilterValue/State` (491/481),
  `ProjectListFilterValue/State` (502/517); helpers `drainBatchToModel`,
  `coldBootStepLister`, `visibleSessionNames` and the pre-existing keep-green tests all
  present in `internal/tui/*_test.go`.

## Summary

One Minor finding. The two cycle-1 additions are structurally sound, well-scoped,
self-contained, and fully source-grounded. The only issue is a documentation-consistency
nit: task 1-4's Problem statement still says "Two invariants" and omits the failing-refetch
invariant that the cycle-1 edit added to the rest of the task (and that tick-6fee61 already
documents as the third).
