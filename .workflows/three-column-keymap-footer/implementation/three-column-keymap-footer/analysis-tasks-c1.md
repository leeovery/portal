---
topic: three-column-keymap-footer
cycle: 1
total_proposed: 2
---
# Analysis Tasks: three-column-keymap-footer (Cycle 1)

## Task 1: Extract shared nav+filter binding prefix and switch footer helpers to *list.Model
status: approved
severity: medium
sources: duplication, architecture

**Problem**: `sessionFooterBindings` (`internal/tui/model.go:625-639`) and `projectFooterBindings` (`internal/tui/model.go:646-660`) both open with the identical 10-element `[]key.Binding{l.KeyMap.CursorUp, CursorDown, NextPage, PrevPage, GoToStart, GoToEnd, Filter, ClearFilter, AcceptWhileFiltering, CancelWhileFiltering}` literal — any future drift in the navigation/filter binding set has to be edited in two locations and is silently allowed to diverge. Compounding this, `sessionFooterBindings`, `projectFooterBindings`, `renderKeymapFooter` (`model.go:702`), `sessionFooterHeight` (`model.go:751`), and `projectFooterHeight` (`model.go:758`) all take `list.Model` by value — `list.Model` embeds a paginator, textinput, help.Model, full Styles set, and item slice, so each render path copies the whole struct twice per frame (footer height measurement plus renderKeymapFooter display).

**Solution**: Introduce a single private helper `listNavAndFilterBindings(l *list.Model) []key.Binding` returning the shared 10-binding slice, and have both footer-bindings builders call it then append their page-specific tail. While touching these signatures, switch `sessionFooterBindings`, `projectFooterBindings`, `renderKeymapFooter`, `sessionFooterHeight`, and `projectFooterHeight` from value to `*list.Model` parameters so the heavyweight struct is no longer copied per frame.

**Outcome**: The nav+filter binding prefix lives in one location; both page footers are guaranteed to share it. Footer helpers take `*list.Model` end-to-end, eliminating two per-frame full-struct copies on the sessions and projects views. No user-visible behaviour change; layout, ordering, sizing, and styling stay byte-identical.

**Do**:
1. Add `listNavAndFilterBindings(l *list.Model) []key.Binding` near the existing footer helpers in `internal/tui/model.go`. Return a fresh `[]key.Binding{l.KeyMap.CursorUp, l.KeyMap.CursorDown, l.KeyMap.NextPage, l.KeyMap.PrevPage, l.KeyMap.GoToStart, l.KeyMap.GoToEnd, l.KeyMap.Filter, l.KeyMap.ClearFilter, l.KeyMap.AcceptWhileFiltering, l.KeyMap.CancelWhileFiltering}`.
2. Rewrite `sessionFooterBindings` to take `*list.Model`, build the slice as `append(listNavAndFilterBindings(l), sessionHelpKeys()...)` (preserving whatever the existing tail is), and update all call sites to pass `&m.sessionList`.
3. Rewrite `projectFooterBindings` to take `*list.Model`, build via the same helper plus the existing project tail (including the `commandPending` branch selecting `projectHelpKeys` vs `commandPendingHelpKeys`), and update call sites to pass a pointer.
4. Change `renderKeymapFooter`, `sessionFooterHeight`, `projectFooterHeight` signatures to take `*list.Model`. Update every call site (`applySessionListSize`, `applyProjectListSize`, `viewSessionList`, `viewProjectList`, WindowSizeMsg handler, `applySessions`, projectsLoaded handler, `NewModelWithSessions`) to pass pointers.
5. Run `go build -o portal .` and `go test ./internal/tui/...`.

**Acceptance Criteria**:
- The 10-binding nav+filter prefix appears as a literal exactly once in `internal/tui/model.go`.
- `sessionFooterBindings`, `projectFooterBindings`, `renderKeymapFooter`, `sessionFooterHeight`, `projectFooterHeight` all take `*list.Model`.
- No call site passes a `list.Model` value to any of those five helpers.
- `go build ./...` succeeds; `go test ./internal/tui/...` passes (including the pinned three-column assertions in `model_test.go` and `sessions_flash_render_test.go`).
- Rendered footer output on sessions and projects views is identical pre/post change.

**Tests**:
- Existing `internal/tui/model_test.go` three-column footer assertions continue to pass unchanged.
- Existing `internal/tui/sessions_flash_render_test.go` snapshot/output assertions continue to pass unchanged.
- No new tests required — pure refactor under full coverage from pinned tests.

## Task 2: Consolidate pairwise-duplicate list-size helpers into a single applyListSize
status: approved
severity: low
sources: duplication

**Problem**: `sessionFooterHeight` (`internal/tui/model.go:752-757`), `projectFooterHeight` (`model.go:760-763`), `applySessionListSize` (`model.go:767-769`), `applyProjectListSize` (`model.go:773-775`) form two structurally identical pairs that differ only in which list field and which footer-bindings helper they reference. The symmetry burden propagates to every `SetSize` call site (WindowSizeMsg handler, `applySessions`, projectsLoaded handler, `NewModelWithSessions`) — every future SetSize change must be made in two places.

**Solution**: Replace the four helpers with a single `applyListSize(l *list.Model, bindings []key.Binding, width, height int)` method on `*Model`. Call sites pass the list pointer plus the appropriate `sessionFooterBindings(...)` / `projectFooterBindings(...)` result.

**Outcome**: One helper handles list sizing for both pages. Future SetSize changes happen once. No behaviour change — output dimensions, footer height computation, and SetSize semantics remain identical.

**Do**:
1. Add `func (m *Model) applyListSize(l *list.Model, bindings []key.Binding, width, height int)` to `internal/tui/model.go`. Compute footer height via the existing `renderKeymapFooter` height path on `bindings`, then `l.SetSize(width, height - footerHeight)`.
2. Remove `sessionFooterHeight`, `projectFooterHeight`, `applySessionListSize`, `applyProjectListSize`.
3. Update every call site to `m.applyListSize(&m.sessionList, sessionFooterBindings(&m.sessionList), width, height)` (and project equivalent). Confirm: WindowSizeMsg handler, `applySessions`, projectsLoaded handler, `NewModelWithSessions`.
4. Run `go build -o portal .` and `go test ./internal/tui/...`.

**Note**: Assumes Task 1's pointer-receiver refactor is in place. If executed before Task 1, adjust signatures locally — the consolidation itself is independent.

**Acceptance Criteria**:
- `sessionFooterHeight`, `projectFooterHeight`, `applySessionListSize`, `applyProjectListSize` no longer exist in `internal/tui/model.go`.
- A single `applyListSize` helper replaces all four.
- All previous call sites compile and route through the consolidated helper.
- `go build ./...` succeeds; `go test ./internal/tui/...` passes.
- Sessions and projects list sizing is visibly unchanged.

**Tests**:
- Existing sizing assertions in `internal/tui/model_test.go` continue to pass.
- Existing `internal/tui/sessions_flash_render_test.go` continues to pass.
- No new tests required.

## Discarded findings

- **Duplication F3 — viewSessionList/viewProjectList JoinVertical tail (low)**: borderline-low, each call site is two lines with per-view modal/flash composition divergence. Extraction would obscure the divergence without meaningful reuse benefit.
