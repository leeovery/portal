---
status: complete
created: 2026-06-26
cycle: 3
phase: Plan Integrity Review
topic: Cold-Boot Restore Lands on Projects
---

# Review Tracking: Cold-Boot Restore Lands on Projects - Integrity

## Outcome

**Clean — no findings.**

This is cycle 3 — a follow-up integrity review after cycle 1 (clean) and cycle 2
(one Minor finding, applied). The brief was specifically to verify that the cycle-2
fix fully resolved the detail-file/tick-store inconsistency on task 1-4's Problem
statement, and that the corrected plan still meets the structural-quality bar.

The plan was re-read end-to-end (planning.md, phase-1-tasks.md, and all four tick
task descriptions tick-9b305e / tick-28a91d / tick-7f37e3 / tick-6fee61), and the
load-bearing source anchors were re-grounded against `internal/tui/model.go`.
Per the brief, the resolved cycle-1/cycle-2 items are not re-raised.

## Cycle-2 Fix Verification (fully resolved)

The cycle-2 Minor finding was that task 1-4's **Problem** statement in the detail
file said "**Two** invariants must hold deterministically" and omitted the
failing-refetch invariant, while tick-6fee61 already read "**Three** invariants".

Both surfaces now agree and are correct:

- **phase-1-tasks.md, Task 1-4 Problem (line 172)** now reads:
  "Three invariants must hold deterministically: (a) the interim page is a valid
  picker page ... (AC7); (b) the landing decision is independent of
  `ProjectsLoadedMsg` arrival order ...; and (c) a failing post-restore refetch
  `SessionsMsg` (one carrying an error) quits without stranding the picker on the
  interim `PageSessions` — degrading to today's quit UX rather than running the
  deferred decision."
- **tick-6fee61 description** reads:
  "Three invariants must hold deterministically: (a) ... (AC7); (b) ...; and
  (c) a failing refetch `SessionsMsg` quits without stranding the interim page."

The Problem statement (the WHY) now matches the three-invariant body the same task's
Solution / Do / Acceptance Criteria / Tests / Edge Cases deliver. The two
authoritative surfaces are in sync. No inconsistency remains.

## Detail-File ↔ Tick-Store Consistency (all four tasks)

Verified each task's tick description against the phase-1-tasks.md detail. The tick
descriptions are condensed but carry the same Problem, Solution, Do, Acceptance
Criteria, Edge Cases, and Spec Reference content — no contradictions:

- **1-1 / tick-9b305e** — identical fix mechanism, quoted production body, the
  mandatory-`ProjectsLoadedMsg` reproduction-test requirement, the coupling
  invariant, and the `progressReceiver != nil` sole-discriminator constraint.
- **1-2 / tick-28a91d** — same AC2 over-correction guard and AC3 filter-routing
  assertions (`SessionListFilterValue/State`, `ProjectListFilterValue/State`,
  `InitialFilter() == ""`).
- **1-3 / tick-7f37e3** — same AC4/AC5 warm-route parity, `refetchSessionsAfterRestore()`
  nil/non-nil symmetry, and AC6 `commandPending` preservation (with the
  `Init` short-circuit invariant).
- **1-4 / tick-6fee61** — three invariants in both surfaces (AC7 interim page,
  late-`ProjectsLoadedMsg` ordering, failing-refetch quit), matching Do / AC /
  Edge Cases.

## Source-Grounding Re-Verification (cycle 3)

Every load-bearing claim an implementer will rely on was re-confirmed against
`internal/tui/model.go` at its current line positions:

- `transitionFromLoading()` at model.go:1828 is a `*Model` receiver with exactly the
  unconditional body the plan quotes (1829-1831:
  `activePage = PageSessions; sessionsLoaded = true; evaluateDefaultPage()`).
- `refetchSessionsAfterRestore()` at model.go:1818 is a value receiver returning `nil`
  when `progressReceiver == nil`, else `fetchSessionsCmd()` — 1-3's predicate symmetry.
- `evaluateDefaultPage()` at model.go:1615 (unchanged target; do-not-touch).
- `SessionsMsg` arm at model.go:1975: error → `tea.Quit` (1976-1977); `PageLoading`
  ingest-but-don't-flip early-return (1990-1992); post-restore `sessionsLoaded = true`
  + `evaluateDefaultPage()` (1994-1995) — grounds 1-4's failing-refetch and
  decision-point claims.
- Both transition arms keep the refetch coupled in the same return:
  `LoadingMinElapsedMsg` (model.go:1997, refetch batch at 2011) and
  `BootstrapCompleteMsg` (model.go:2031, transition + refetch at 2047-2049) both
  `return m, tea.Batch(m.surfaceBufferedWarnings(), m.refetchSessionsAfterRestore())`
  immediately after `transitionFromLoading()` — 1-1's coupling invariant.
- `ProjectsLoadedMsg` arm at model.go:2068.

Note: the plan cites approximate ("~line") anchors for the two transition arms
(`LoadingMinElapsedMsg ~2005` vs actual 1997; `BootstrapCompleteMsg ~2046` vs actual
2031). These are explicitly approximate, name the arm by its `case` label, and the
quoted bodies/coupling all match — no implementer ambiguity. Not a finding.

## Dimension-by-Dimension Assessment (cycle 3)

### 1. Task Template Compliance — PASS
All four tasks carry every required field. With the cycle-2 fix applied, task 1-4's
Problem now correctly enumerates all three invariants its body delivers — the sole
remaining template gap from cycle 2 is closed.

### 2. Vertical Slicing — PASS
Unchanged from cycles 1/2. 1-1 is the production vertical slice; 1-2/1-3/1-4 lock
distinct, independently verifiable behavioural facets. No horizontal slicing.

### 3. Phase Structure — PASS
Single phase remains correctly right-sized; "Why this order" rationale intact.

### 4. Dependencies and Ordering — PASS
1-2/1-3/1-4 each list tick-9b305e (1-1) as their sole blocker (re-verified in tick).
No circular deps. Priority ordering (1-1 priority 1; 1-2/1-3/1-4 priority 2) reflects
graph position — 1-1 is the sole production change and the unblocker.

### 5. Task Self-Containment — PASS
Each task carries the full Problem/Solution/Do/Context to execute standalone. With the
cycle-2 fix, task 1-4's Problem no longer under-counts its own invariants, closing the
last self-containment soft spot.

### 6. Scope and Granularity — PASS
Each task is one TDD cycle. No task over- or under-scoped.

### 7. Acceptance Criteria Quality — PASS
Criteria are pass/fail and behaviour-anchored (`ActivePage()`, `sessionsLoaded` state,
filter value/state, `InitialFilter()`, lister call-count, `tea.Quit` on error). Not
"code exists".

### 8. External Dependencies — N/A
Bugfix work type; criterion is epic-only.

## Findings

None. The cycle-2 fix fully resolved the inconsistency, both authoritative surfaces
(phase-1-tasks.md and tick-6fee61) agree, the source anchors remain accurate, and the
plan is implementation-ready on every review dimension.
