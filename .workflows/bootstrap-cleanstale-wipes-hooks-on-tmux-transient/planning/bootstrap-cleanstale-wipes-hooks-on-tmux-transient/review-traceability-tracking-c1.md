---
status: in-progress
created: 2026-05-27
cycle: 1
phase: Traceability Review
topic: Bootstrap CleanStale Wipes Hooks On Tmux Transient
---

# Review Tracking: Bootstrap CleanStale Wipes Hooks On Tmux Transient - Traceability

## Summary

Bi-directional trace complete:

- **Direction 1 (Spec → Plan)**: every spec element has plan coverage. Change 1 → Task 1-1. Change 2 (negative-scope item: `parseLivePaneSet` not promoted) → implicit/honoured in Task 1-1. Change 3 → Tasks 2-1, 2-2, 2-4. Change 4 → Tasks 2-2, 2-4. Test Requirements (new file, inverted subtest, deterministic repro, both integration tests, coverage matrix rows) → Tasks 2-3, 2-5, 3-1, 3-2, 3-3. Acceptance Criteria 1-6 each map to one or more task acceptance bullets. Out-of-scope items correctly absent from plan.
- **Direction 2 (Plan → Spec)**: every task's Problem/Solution/Outcome/Do/Acceptance traces back to a specific spec section. No hallucinated requirements, no invented edge cases, no acceptance criteria testing un-spec'd behaviour.

Two borderline items below — both are defensible derived/defensive content rather than hallucinations, surfaced for user awareness rather than mandatory removal.

## Findings

### 1. Task 2-3 introduces `runCleanStale` free-function refactor not in spec

**Type**: Hallucinated content (low severity; offered as one option among two)
**Spec Reference**: §Change 3 — "tests for the new `cmd/bootstrap_production_test.go` file may abstract `ListAllPanes` behind a local `AllPaneLister` interface (matching the existing shape in `cmd/clean.go:13-15`) to keep stubs unintrusive."
**Plan Reference**: phase-2-tasks.md Task 2-3, Do step 5
**Change Type**: update-task

**Details**: The spec specifies the test-stubbing seam is a local `AllPaneLister` interface at the **test** boundary; it does not call for production-side refactoring to extract a free function. Task 2-3 introduces "Preferred shape: extract a free function `runCleanStale(lister AllPaneLister, store *hooks.Store, logger bootstrap.Logger) error` in `cmd/bootstrap_production.go`" as the recommended approach. This is a plan-invented refactor that shifts production code structure to serve test convenience — beyond the spec's explicit "abstract behind a local `AllPaneLister` interface" guidance. The task also offers the test-local adapter alternative ("either approach is acceptable"), so the refactor is not mandated, but the "preferred shape" framing nudges the implementer toward the un-spec'd refactor.

Two reasonable resolutions:
- (a) Remove the free-function recommendation; mandate the test-local adapter approach (which directly matches the spec's "local `AllPaneLister` interface" wording).
- (b) Keep the option open but flip "preferred" to "either approach is acceptable" with no preference, letting the implementer choose without nudging toward the un-spec'd refactor.

Proposed below applies (a) — narrows to the spec-aligned shape.

**Current**:
```markdown
The bridge problem — `cleanStaleAdapter` declares `client *tmux.Client` (concrete type), not an interface — is handled by introducing a thin parallel test-only adapter type **or** by refactoring the production adapter to accept an `AllPaneLister`-shaped seam. Per spec §Change 3 ("The struct continues to consume `*tmux.Client` directly — no new seam interface is introduced — but tests for the new `cmd/bootstrap_production_test.go` file may abstract `ListAllPanes` behind a local `AllPaneLister` interface"), the preferred approach is the latter at the test boundary only: define a `cleanStaleAdapterT` test-local struct that mirrors `cleanStaleAdapter`'s method shape but holds an `AllPaneLister` seam instead of `*tmux.Client`. The test exercises the **algorithm** by invoking the test-local adapter, while a separate compile-time assertion confirms the production `cleanStaleAdapter` has the same field layout. This is the same shape used by `cmd/clean_test.go` (via `cleanDeps.AllPaneLister`) and avoids widening the production type surface.
```

…and Do step 5:
```markdown
5. Define a test-local adapter mirroring the production shape with a seam interface for `ListAllPanes`. This is the test surrogate that exercises the same algorithm Task 2-2 lands. To keep the algorithm test-truthy, factor the body of `(*cleanStaleAdapter).CleanStale` into a free function or method that takes the lister as an interface — either approach is acceptable as long as the production adapter and the test surrogate share the same algorithm code path. Preferred shape: extract a free function `runCleanStale(lister AllPaneLister, store *hooks.Store, logger bootstrap.Logger) error` in `cmd/bootstrap_production.go` and have both the production `(*cleanStaleAdapter).CleanStale` and the test call this function directly. (This refactor is part of Task 2-2's deliverable if convenient, or part of this task — author at the implementation site whichever is cleanest.)
```

**Proposed**:
```markdown
The bridge problem — `cleanStaleAdapter` declares `client *tmux.Client` (concrete type), not an interface — is handled at the **test boundary only**, per spec §Change 3 ("tests for the new `cmd/bootstrap_production_test.go` file may abstract `ListAllPanes` behind a local `AllPaneLister` interface"). Define a `cleanStaleAdapterT` test-local struct that mirrors `cleanStaleAdapter`'s method shape but holds an `AllPaneLister` seam instead of `*tmux.Client`. The test exercises the **algorithm** by invoking the test-local adapter, while a separate compile-time assertion (`var _ AllPaneLister = (*tmux.Client)(nil)`) confirms the production wiring still compiles. This is the same shape used by `cmd/clean_test.go` (via `cleanDeps.AllPaneLister`) and avoids widening the production type surface. **No production refactor.**
```

…and Do step 5:
```markdown
5. Define a test-local `cleanStaleAdapterT` struct that mirrors the production `cleanStaleAdapter` field layout (`store *hooks.Store`, `Logger bootstrap.Logger`) but substitutes `lister AllPaneLister` for `client *tmux.Client`. Re-implement the six-branch algorithm from Task 2-2 verbatim on this test-local type's `CleanStale` method so the test exercises an identically-shaped algorithm against stubbable enumeration. **Do not refactor the production adapter** — the test-local type is the seam. Drift risk is mitigated by Task 2-2's six-branch algorithm being short, fully specified in Do step 3 of that task, and covered by the integration tests in Phase 3 against the real production adapter.
```

**Resolution**: Pending
**Notes**:

---

### 2. Task 1-2 adds whitespace-only-stdout subtest not explicitly mandated by spec

**Type**: Hallucinated content (very low severity; defensive coverage)
**Spec Reference**: §Failure Modes Covered item (b) — "list-panes -a exit 0 with empty stdout (saver mid-respawn momentary 'no panes' reply) — plausible but unobserved; precautionary coverage." §Closing Both Failure Modes — mode (b) closed by Change 3 (hazard guard). §Change 1 return-value contract — pre-fix `([]string{}, nil)` on error; post-fix `(nil, err)`.
**Plan Reference**: phase-1-tasks.md Task 1.2, Do step 2 and Tests section
**Change Type**: update-task

**Details**: The spec mandates the legitimate-empty contract at the helper boundary for mode (b) — exit 0 with empty stdout returning `([]string{}, nil)`. Task 1-2 adds a second subtest `"returns empty slice when output is whitespace-only"` that exercises `MockCommander{Output: "  \n\n\t\n "}`. The spec does not mention whitespace-only stdout as a legitimate-empty case to pin. The subtest is reasonable defensive coverage against future `parsePaneOutput` regressions, but it does test a scenario the spec does not enumerate. Either keep it as defensive coverage (explicitly note it's beyond-spec) or trim to the spec-aligned empty-stdout case only.

This finding is surfaced for awareness; the defensive subtest is arguably valuable and worth keeping. No proposed-removal content is provided — leaving it as-is is reasonable. Recommendation: **keep as-is**, but the user may choose to trim.

**Current**:
```markdown
2. Add a new subtest `"returns empty slice when output is whitespace-only"` inside `TestListAllPanes` (insert near line 1488, after the empty-output subtest and before `"calls list-panes with -a flag..."`):
   - `MockCommander{Output: "  \n\n\t\n "}` — exit 0, whitespace-only stdout.
   - Assert `err == nil`.
   - Assert `got` is non-nil but `len(got) == 0` (the slice should coerce to empty via `parsePaneOutput`'s `strings.TrimSpace` + skip-empty logic at `internal/tmux/tmux.go:458-465`).
```

**Proposed**: (No change recommended — defensive coverage is arguably valuable. Listed here so the user can opt to trim if strict spec-fidelity is preferred. If trimming, remove Do step 2 entirely and remove the corresponding entry from Acceptance Criteria and Tests sections.)

**Resolution**: Pending
**Notes**: User-discretion finding — the subtest is reasonable defensive coverage and may be kept. Surfaced for transparency on minor scope expansion beyond spec text.

---
