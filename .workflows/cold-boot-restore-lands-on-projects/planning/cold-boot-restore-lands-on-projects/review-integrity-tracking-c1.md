---
status: complete
created: 2026-06-26
cycle: 1
phase: Plan Integrity Review
topic: Cold-Boot Restore Lands on Projects
---

# Review Tracking: Cold-Boot Restore Lands on Projects - Integrity

## Outcome

**Clean — no findings.**

The plan was read end-to-end (planning.md, phase-1-tasks.md, and all four tick task
descriptions tick-9b305e / tick-28a91d / tick-7f37e3 / tick-6fee61), and every
load-bearing technical claim was cross-checked against `internal/tui/model.go`. The plan
meets the structural-quality and implementation-readiness bar on every review dimension.
Detail file and tick descriptions are in sync (verified the AC6 coverage on 1-3 and the
failing-refetch-quit coverage on 1-4 are present in both surfaces).

## Dimension-by-Dimension Assessment

### 1. Task Template Compliance — PASS
All four tasks carry every required field (Problem, Solution, Outcome, Do, Acceptance
Criteria, Tests) plus Edge Cases, Context, and Spec Reference. Problems state WHY
concretely (premature `evaluateDefaultPage()` against the stale empty Init snapshot;
over-correction / filter co-defect; warm-route zero-new-risk contract; interim-window and
ordering invariants). Solutions state WHAT. Acceptance criteria are concrete and assert
the **active page**, not merely list contents. Tests include edge cases (vacuous-pass trap,
zero-session over-correction, filter-not-routed-to-projects, late-ProjectsLoadedMsg
interleave, failing-refetch quit).

### 2. Vertical Slicing — PASS
Task 1-1 is a complete vertical slice: the one-line production change in
`transitionFromLoading()` plus its reproduction test (write-test → implement → pass). Tasks
1-2/1-3/1-4 are pure regression-coverage tasks locking distinct behavioural facets of that
single change (over-correction + filter routing; warm parity + commandPending; interim
page + ordering + failing refetch). For a single-line bugfix whose correctness spans
several independent invariants, splitting the lock-in coverage into focused, independently
verifiable test tasks is the appropriate decomposition — each is independently runnable
once 1-1 lands and each tests complete behaviour, not a technical layer. No horizontal
slicing.

### 3. Phase Structure — PASS
A single phase is correctly right-sized and the "Why this order" rationale is sound: one
root cause, one cohesive Update-cycle change confined to `model.go`, no intermediate state
worth checkpointing, and splitting the production change from its tests would create
forward references and non-milestone phases. The phase carries clear, verifiable
acceptance criteria (AC1–AC7 plus the ordering contract, the deferral-coupling invariant,
and the latch-preserved / failing-refetch-quit / no-`t.Parallel()` guards).

### 4. Dependencies and Ordering — PASS
1-2, 1-3, and 1-4 are all `blocked_by` 1-1 — a genuine capability dependency: each asserts
the behaviour of the production change introduced in 1-1, so none can pass until 1-1 lands.
No circular dependencies. Priority assignment reflects graph position: 1-1 is priority High
(the sole production change and the unblocker); 1-2/1-3/1-4 are priority 2. The three
test-only tasks are mutually independent and correctly carry no edges between themselves.
No convergence point lacks an edge.

### 5. Task Self-Containment — PASS
Each task contains the full Problem/Solution/Do/Context needed to execute without reading
the others. The relevant spec decisions are pulled forward verbatim into each Context block
(root cause, fix mechanism, canonical predicate, filter co-defect, warm zero-new-risk
contract, valid-interim-page / decision-always-resolves / failing-refetch-quit
constraints). Do sections name exact files, functions, line numbers, constructor options,
and reusable test helpers (`drainBatchToModel`, `coldBootStepLister`, `visibleSessionNames`,
`stubProjectStore`/`smProjectStore`).

### 6. Scope and Granularity — PASS
Each task is one TDD cycle. 1-1's Do is a single production edit plus one reproduction test.
1-2/1-3/1-4 each add 2–3 focused tests with one verifiable invariant per test. None is
mechanical boilerplate (each locks a distinct correctness property the single-line change
could otherwise silently regress); none is too large (each test's intent is statable in one
sentence and touches one architectural surface — the TUI Update cycle).

### 7. Acceptance Criteria Quality — PASS
Criteria are pass/fail and behaviour-anchored, not "code exists". They name the exact
asserted state: `ActivePage() == PageSessions/PageProjects`, `sessionsLoaded` not set,
`evaluateDefaultPage()` not called at transition, `SessionListFilterValue()/State()`,
`ProjectListFilterValue()/State()`, `InitialFilter() == ""`, lister call-count exactly 1,
`refetchSessionsAfterRestore()` nil vs non-nil, `tea.Quit` on error. Edge-case criteria
specify boundary behaviour (genuine-empty-refetch vs stale-Init-snapshot distinction;
late-ProjectsLoadedMsg strictly between transition and decision-bearing SessionsMsg;
single interim frame before quit accepted). The mandatory-ProjectsLoadedMsg requirement
forecloses the documented vacuous-pass trap.

### 8. External Dependencies — N/A
Bugfix work type; criterion is epic-only.

## Source-Grounding Verification

Every technical claim the implementer will rely on was confirmed against the codebase:

- `transitionFromLoading()` at model.go:1828 is a `*Model` receiver with exactly the body
  the plan quotes (`activePage = PageSessions; sessionsLoaded = true; evaluateDefaultPage()`).
- `refetchSessionsAfterRestore()` at model.go:1818 is a value receiver returning `nil` when
  `progressReceiver == nil`, else `fetchSessionsCmd()` — matching 1-3's predicate-symmetry
  assertion.
- `evaluateDefaultPage()` at model.go:1615 carries the `defaultPageEvaluated` latch, the
  `commandPending`/`!sessionsLoaded || !projectsLoaded` early-returns, the `len(Items())>0`
  page test, and the `initialFilter` block (1635–1646) that routes to Sessions only when
  `activePage == PageSessions && !commandPending` then zeroes `initialFilter` — all exactly
  as the plan describes.
- The `SessionsMsg` arm (1975–1996): error → `tea.Quit` (1976–1977); `PageLoading` ingest-
  but-do-not-flip early-return (1990–1992); post-restore `sessionsLoaded = true` +
  `evaluateDefaultPage()` (1994–1995). Confirms 1-4's failing-refetch and decision-point
  claims.
- Both transition arms keep the refetch coupled in the same return: LoadingMinElapsedMsg
  (2011) and BootstrapCompleteMsg (2051) both `return m, tea.Batch(surfaceBufferedWarnings(),
  refetchSessionsAfterRestore())` immediately after `transitionFromLoading()`.
- `ProjectsLoadedMsg` arm (2098–2099) calls `evaluateDefaultPage()` UNCONDITIONALLY (no page
  guard) — confirming 1-4's invariant that interim latching is prevented solely by the
  `!sessionsLoaded` early-return.
- Constructor/test surfaces exist as named: `WithInitialFilter` (621), `WithCommand` (632,
  sets `commandPending=true` + `activePage=PageProjects`), `WithServerStarted` (785),
  `WithProgressReceiver` (803), `ActivePage` (533), `SessionListFilterValue/State` (491/481),
  `ProjectListFilterValue/State` (502/517), `InitialFilter` (461); test helpers
  `driveColdBootToSessions`, `drainBatchToModel`, `coldBootStepLister`, `visibleSessionNames`,
  `stubProjectStore`, `smProjectStore`, and the pre-existing tests the plan says to keep
  green all present in `internal/tui/*_test.go`.

## Findings

None.
