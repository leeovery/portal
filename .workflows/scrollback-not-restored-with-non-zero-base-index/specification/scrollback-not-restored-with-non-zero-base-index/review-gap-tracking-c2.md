---
status: in-progress
created: 2026-04-30
cycle: 2
phase: Gap Analysis
topic: scrollback-not-restored-with-non-zero-base-index
---

# Review Tracking: scrollback-not-restored-with-non-zero-base-index - Gap Analysis

## Findings

### 1. Eviction loop error handling unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Scope > Part 1 > Migration mechanics

**Details**:
The migration mechanics describe a per-event scan-then-evict-then-install ordering, but do not specify behaviour when `UnsetGlobalHookAt` fails partway through the eviction loop (e.g. tmux returns non-zero, or a different concurrent process mutates the hook list between `ShowGlobalHooks` and `UnsetGlobalHookAt`). Options range from fatal abort, to best-effort continue + log, to retry-after-rescan. Without explicit guidance an implementer must guess. AC #3's "exactly 1 entry after bootstrap" invariant could be violated silently if a partial eviction is left in place and the install proceeds.

Related: it is also unspecified whether the install step should run if eviction failed (i.e. should the orchestrator install the fixed entry on top of a partially-evicted state, or skip install and surface a warning?).

**Proposed Addition**:
{leave blank}

**Resolution**: Pending
**Notes**:

---

### 2. AC #4 not bound to a verifying test

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Acceptance Criteria #4, Testing Requirements

**Details**:
Cycle 1 introduced explicit binding language for AC #2 ("satisfied by Testing Requirements item 1") and AC #3 ("asserted by Testing Requirements item 4"). AC #4 — "No misleading `predicted=...__0.0 live=...__X.Y` WARN appears in `portal.log` under any tmux config" — has no equivalent binding. None of the four Testing Requirements explicitly verifies absence of this WARN line.

The deletion of `warnOnPaneKeyDrift` removes the call site, so the WARN is structurally impossible post-fix, but there is no test asserting either (a) the symbol is gone or (b) a reboot round-trip with non-zero base-index produces no `predicted=` line in `portal.log`. The deletion-verification step (cycle 1) confirms no remaining references but is not itself a test artefact.

**Proposed Addition**:
{leave blank}

**Resolution**: Pending
**Notes**:

---

### 3. Eviction substring match may be too permissive

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Scope > Part 1 > Migration mechanics

**Details**:
The eviction predicate is "command contains `portal state signal-hydrate` AND does not contain `portal state signal-hydrate --`". This will match any global hook entry on a hydration-trigger event whose command string mentions `portal state signal-hydrate` without `--`. If a user (or a future Portal feature) has an unrelated hook entry that legitimately references `portal state signal-hydrate` in a different shape (e.g. wrapped in their own conditional), the migration would silently delete it.

The spec does not bound the predicate to "entries Portal itself installed" (e.g. by also requiring the `command -v portal >/dev/null 2>&1 &&` prefix that `signalHydrateCommand` uses). An implementer choosing the loosest interpretation could destroy user-authored content. Cycle 1 settled the dedupe substring shape and operator visibility but did not address predicate scope.

**Proposed Addition**:
{leave blank}

**Resolution**: Pending
**Notes**:

---

### 4. Location of migration logic in the codebase is unspecified

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Scope > Part 1, Testing Requirements item 4

**Details**:
The spec describes the migration as "the existing `RegisterPortalHooks` step (bootstrap step 2) must evict ..." but does not say whether the eviction implementation lives in `internal/tmux/hooks_register.go` (alongside `RegisterHookIfAbsent`), in `cmd/bootstrap/` (as a new orchestrator concern), or in `internal/bootstrapadapter`. This shapes test placement (Testing Requirements item 4 mandates real-tmux fixture but does not say which package owns the test) and influences whether eviction is a `tmux.Client` method or a free function.

A reasonable implementer could put it in any of the three. Picking wrongly could create awkward import paths or duplicate test infrastructure with the existing hook-register tests.

**Proposed Addition**:
{leave blank}

**Resolution**: Pending
**Notes**:

---

### 5. Independence and shipping order of Part 1 vs Part 2 not stated

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Scope (overall structure)

**Details**:
The spec says "Both are required" but does not state whether Part 1 (the `--` fix + migration) and Part 2 (deletion of `PredictLiveIndices` and the diagnostic WARN) are independent commits/PRs or must land atomically. They appear functionally independent — Part 1 fixes hydration, Part 2 removes a misleading diagnostic — and could ship in either order or separately, but a planner breaking this into tasks would benefit from explicit guidance:
- May Part 1 ship without Part 2? (Hydration would work; misleading WARN persists temporarily.)
- May Part 2 ship without Part 1? (Diagnostic removed but actual bug remains.)
- Should they be a single PR for review coherence?

This affects how tasks are sequenced and whether intermediate states are acceptable.

**Proposed Addition**:
{leave blank}

**Resolution**: Pending
**Notes**:

---

### 6. Identification of existing tests to delete is implicit

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Fix Scope > Part 2

**Details**:
Part 2 lists symbols to delete and adds "Tests covering the deleted functions." It does not enumerate the test files/functions. An implementer must search for them. While straightforward in a small codebase, the deletion-verification step (cycle 1) addresses only production-side references — it doesn't compel a parallel test-side audit. Tests that reference `PredictLiveIndices`, `warnOnPaneKeyDrift`, or `flattenSavedPanePositions` could be missed and left as compile errors, or worse, mocks that quietly become dead code.

A short instruction to "audit `_test.go` files for references to the deleted symbols and remove or refactor" would close this gap. Likely candidates (e.g. `internal/restore/session_test.go`, `internal/restore/restore_test.go`) are not named.

**Proposed Addition**:
{leave blank}

**Resolution**: Pending
**Notes**:

---
