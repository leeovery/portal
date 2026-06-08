---
topic: session-tagging-and-grouping
cycle: 4
total_proposed: 3
---
# Analysis Tasks: Session Tagging and Grouping (Cycle 4)

## Task 1: Return the canonical key from Index.Match so buildByProject stops double-canonicalising
status: approved
severity: medium
sources: architecture

**Problem**: In `internal/tui/grouping.go` (buildByProject, ~lines 54-64), every known-project session pays two identical `filepath.EvalSymlinks` syscalls per By-Project render instead of one. `idx.Match(s.Dir)` internally computes `project.CanonicalDirKey(s.Dir)` (one EvalSymlinks syscall) and discards it, then the GroupKey is set with `project.CanonicalDirKey(s.Dir)`, recomputing the same value a second time. At 15-20 sessions this is ~30-40 syscalls per render where ~15-20 would suffice. The two computations are a pure function of the same input and always agree; the second is pure waste. This partially undoes the cycle-2 `project.Index` optimisation (which canonicalised each stored `Project.Path` once at NewIndex time). buildByTag is not affected (its GroupKey is the tag, not a path).

**Solution**: Change `Index.Match` to return the canonical key it already computes, e.g. `Match(dirPath) (Project, string, bool)`, and have buildByProject use the returned key as the GroupKey instead of calling `project.CanonicalDirKey(s.Dir)` again. This composes the two currently-independent computations of the same value and keeps the Index as the single place a session-side path is canonicalised per render.

**Outcome**: A By-Project render performs exactly one EvalSymlinks syscall per known-project session. The Index remains the single canonicalisation chokepoint; no behavioural change to grouping output.

**Do**:
1. In `internal/project/index.go`, change the `Index.Match` signature to also return the canonical key string it computes internally (e.g. `Match(dirPath string) (Project, string, bool)`), returning the already-computed canonical key alongside the matched Project and the found bool.
2. Update `internal/tui/grouping.go` buildByProject (~lines 54-64) to capture the returned canonical key from `idx.Match(s.Dir)` and use it directly as `GroupKey`, removing the redundant `project.CanonicalDirKey(s.Dir)` call.
3. Update all other `Index.Match` callers to the new signature (discard or use the new return value as appropriate). Search for `.Match(` usages of the Index type across the codebase.
4. Update unit tests for `Index.Match` to assert the returned canonical key.

NOTE: This task and Task 2 both touch `internal/tui/grouping.go` but are distinct concerns (this one changes the Index.Match contract + GroupKey wiring; Task 2 retypes the catch-all path). They do not depend on each other; execute sequentially.

**Acceptance Criteria**:
- `Index.Match` returns the canonical key it computes; no caller recomputes `CanonicalDirKey` on the same input it just passed to `Match`.
- buildByProject sets GroupKey from the value returned by `Match`, not from a second `project.CanonicalDirKey(s.Dir)` call.
- A known-project session incurs one EvalSymlinks syscall per By-Project render (not two).
- All existing `Index.Match` callers and tests compile and pass against the new signature.
- By-Project grouping output is unchanged.

**Tests**:
- Unit test asserting `Index.Match` returns the expected canonical key for a matched path (including a symlinked path where EvalSymlinks changes the value).
- Existing buildByProject / By-Project render tests continue to pass and assert grouping output is identical.
- A test demonstrating the returned key is reused as GroupKey (so no second canonicalisation occurs).

## Task 2: Type the grouping catch-all path as []SessionItem to remove the runtime type-assertion and dead branch
status: approved
severity: low
sources: architecture

**Problem**: In `internal/tui/grouping.go`, the unknown/untagged catch-all slices are typed `[]list.Item` and threaded as `catchAll []list.Item` through `assembleGroups` (~233-241) into `appendCatchAll` (~184-198), which immediately asserts each element back to `SessionItem` (`si, ok := it.(SessionItem)`) with a defensive `continue` on the `!ok` branch. But `unknownItem`/`untaggedItem` return concrete `SessionItem` values, and those SessionItems are the only things ever appended to these slices — so the `!ok` branch is unreachable dead code. This is the "untyped parameter when the concrete type is known at design time" anti-pattern (code-quality.md Concrete Over Abstract): the abstract `list.Item` element type is widened only to be narrowed back one call later, and the GroupKey stamp + sort are SessionItem-specific operations that `list.Item` cannot express.

**Solution**: Type the catch-all inputs as `[]SessionItem` end to end. The builders already produce `SessionItem`, so collect them into `[]SessionItem`, pass `catchAll []SessionItem` through `assembleGroups` and `appendCatchAll`, and operate on the concrete type directly. The type assertion and its unreachable defensive branch disappear; GroupKey-stamp and sort run on the concrete type. The only `[]list.Item` boxing that remains is the single `sessionItemsToList` call at the return boundary, where the widening is genuinely required by bubbles/list.

**Outcome**: The catch-all path carries `[]SessionItem` with no runtime type assertion and no dead defensive branch; the `list.Item` widening occurs exactly once, at the `sessionItemsToList` return boundary.

**Do**:
1. In `internal/tui/grouping.go`, change the unknown/untagged catch-all collection in buildByProject (~47-67) and buildByTag (~96-115) to accumulate into `[]SessionItem` (the builders already yield `SessionItem`).
2. Change `assembleGroups` (~233-241) to take `catchAll []SessionItem`.
3. Change `appendCatchAll` (~184-198) to take `[]SessionItem`; remove the `si, ok := it.(SessionItem)` assertion and the unreachable `!ok` `continue` branch, operating on the concrete `SessionItem` directly for GroupKey stamp + sort.
4. Keep the `sessionItemsToList` call at the return boundary as the single point of `[]list.Item` widening.

NOTE: This task and Task 1 both touch `internal/tui/grouping.go` but are distinct concerns. They do not depend on each other.

**Acceptance Criteria**:
- The catch-all slices and `appendCatchAll` / `assembleGroups` catch-all parameter are typed `[]SessionItem`.
- The `it.(SessionItem)` type assertion and its `!ok` defensive branch are removed from `appendCatchAll`.
- The only `[]list.Item` boxing on the catch-all path is the `sessionItemsToList` return-boundary call.
- By-Project and By-Tag grouped output (including catch-all pinning/suppression and ordering) is unchanged.

**Tests**:
- Existing By-Project and By-Tag grouping tests covering unknown/untagged catch-all pinning, suppression, and sort order continue to pass with identical output.
- Verify (via build + existing tests) that no caller relied on `list.Item` heterogeneity in the catch-all path.

## Task 3: Extract a findByPath helper for the new AddTag/RemoveTag lookup duplication
status: approved
severity: low
sources: duplication

**Problem**: In `internal/project/tags.go`, both new tag-mutation methods (`AddTag` ~48-53, `RemoveTag` ~84-89) open with a byte-identical exact-path lookup: `slices.IndexFunc(projects, func(p Project) bool { return p.Path == path })` followed by an `if idx < 0 { return ErrProjectNotFound }` guard. The feature introduced two more copies of a project-by-path lookup that the package now performs in six places (the same idiom recurs in pre-existing store.go: Upsert, Rename, Remove). The two new copies are past the Rule of Three; the existing inconsistency across sites (IndexFunc vs hand-rolled range; returning index vs name vs bool) is the kind of drift that invites a future divergence bug.

**Solution**: Extract a single private helper `findByPath(projects []Project, path string) (int, bool)` and route the two NEW `tags.go` call sites through it. Do NOT touch the pre-existing `store.go` sites — they are out of plan scope. Consolidating only the new tags.go callers removes the newly-introduced duplication and leaves a seam store.go can adopt later.

**Outcome**: AddTag and RemoveTag locate the target project record through one shared `findByPath` helper; the duplicated `slices.IndexFunc` + `ErrProjectNotFound` guard exists in exactly one place for the new code.

**Do**:
1. In `internal/project/tags.go` (or the appropriate file within the project package), add a private helper `findByPath(projects []Project, path string) (int, bool)` that returns the matching index and a found bool (false when no record matches).
2. Update `AddTag` (~48-53) to call `findByPath`, returning `ErrProjectNotFound` when found is false.
3. Update `RemoveTag` (~84-89) to call `findByPath` the same way.
4. Leave `store.go` (Upsert, Rename, Remove) untouched — out of scope.

**Acceptance Criteria**:
- A private `findByPath(projects []Project, path string) (int, bool)` helper exists.
- Both `AddTag` and `RemoveTag` route their project lookup through `findByPath`; the inline `slices.IndexFunc` + `idx < 0` guard is removed from both.
- The `ErrProjectNotFound` behaviour of AddTag/RemoveTag is unchanged.
- `store.go` Upsert/Rename/Remove are not modified.

**Tests**:
- Existing AddTag/RemoveTag tests (including the not-found → `ErrProjectNotFound` path for each) continue to pass.
- If not already covered, add a test asserting AddTag and RemoveTag each return `ErrProjectNotFound` for a path with no matching project record.
