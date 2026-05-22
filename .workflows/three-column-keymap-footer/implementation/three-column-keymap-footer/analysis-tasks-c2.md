---
topic: three-column-keymap-footer
cycle: 2
total_proposed: 1
---
# Analysis Tasks: three-column-keymap-footer (Cycle 2)

## Task 1: Add per-page wrappers over applyListSize to own the list+bindings pairing invariant
status: approved
severity: low
sources: architecture

**Problem**: Cycle-1's consolidation of size helpers into a single `applyListSize(l, bindings, w, h)` left the list pointer and binding source as non-independent arguments paired by caller discipline at 8 call sites (5 sessions, 3 projects) in `internal/tui/model.go` (lines 739-740, 785, 1002-1003, 1064, plus others). Passing `&m.sessionList` with `projectFooterBindings(&m.projectList, ...)` would compile and run, sizing one list against the other's footer height — the pairing is the invariant but the caller owns it. Additionally, `applyListSize` is declared as a method on `*Model` but reads no `m` field.

**Solution**: Reintroduce two thin per-page wrappers over the consolidated `applyListSize` core. Keep `applyListSize` as the shared math/SetSize core — do not revert the cycle-1 consolidation. The wrappers own the list+bindings pairing so callers cannot mismatch them.

**Outcome**: Eight call sites collapse to one-arg `m.applySessionListSize(w, h)` / `m.applyProjectListSize(w, h)` calls. The list+bindings pairing becomes a compile-time invariant owned by the wrapper, not a discipline invariant owned by callers. The shared sizing math remains in a single place.

**Do**:
1. Keep the existing `applyListSize(l *list.Model, bindings []key.Binding, w, h int)` core. Optionally convert from method to free function since it reads no `m` field.
2. Add `func (m *Model) applySessionListSize(w, h int)` that calls the core with `&m.sessionList` and `sessionFooterBindings(&m.sessionList)`.
3. Add `func (m *Model) applyProjectListSize(w, h int)` that calls the core with `&m.projectList`, `projectFooterBindings(&m.projectList, m.commandPending)`. Wrapper owns the `m.commandPending` branch.
4. Replace all 5 sessions call sites with `m.applySessionListSize(w, h)`.
5. Replace all 3 projects call sites with `m.applyProjectListSize(w, h)`.
6. Verify no remaining direct calls to `applyListSize` from outside the two wrappers.

**Acceptance Criteria**:
- `applyListSize` core remains as shared sizing math (cycle-1 consolidation preserved).
- Two new wrapper methods exist: `applySessionListSize(w, h int)` and `applyProjectListSize(w, h int)`.
- All 8 previous call sites now invoke a wrapper, not the core directly.
- The `m.commandPending` branch is owned by `applyProjectListSize`.
- `go build ./...` succeeds; `go test ./internal/tui/...` passes.

**Tests**:
- Existing TUI tests covering sessions-page and projects-page sizing continue to pass unchanged.
- No new tests required.
