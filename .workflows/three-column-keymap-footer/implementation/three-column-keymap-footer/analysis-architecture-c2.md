# Architecture Analysis — Cycle 2

STATUS: findings
FINDINGS_COUNT: 1

## Findings

### FINDING 1: applyListSize call sites couple list pointer + binding source by caller discipline

- SEVERITY: low
- FILES:
  - `internal/tui/model.go:739-740` (NewModelWithSessions)
  - `internal/tui/model.go:751-753` (applyListSize definition)
  - `internal/tui/model.go:785` (applySessions)
  - `internal/tui/model.go:1002-1003` (WindowSizeMsg)
  - `internal/tui/model.go:1064` (ProjectsLoadedMsg)
- DESCRIPTION: Cycle-1 follow-up replaced two trivial wrappers (`applySessionListSize` / `applyProjectListSize`) with a single generic `applyListSize(l, bindings, w, h)`. Two issues compound:
  1. The helper is a method on `*Model` but reads no `m` field — free function in method clothing.
  2. The list pointer and binding source are non-independent arguments paired by caller discipline at 5 sessions sites + 3 projects sites. Every sessions call repeats `m.applyListSize(&m.sessionList, sessionFooterBindings(&m.sessionList), ...)`; every projects call repeats `m.applyListSize(&m.projectList, projectFooterBindings(&m.projectList, m.commandPending), ...)`. Passing `&m.sessionList` with `projectFooterBindings(&m.projectList, ...)` would compile and run, sizing one list against the other's footer height. The pairing is the invariant; the caller is the wrong place to own it.
- RECOMMENDATION: Reintroduce two thin per-page wrappers over the consolidated core: `(m *Model) applySessionListSize(w, h)` calls `m.applyListSize(&m.sessionList, sessionFooterBindings(&m.sessionList), w, h)`, and the projects counterpart embeds the `m.commandPending` branch. Keep `applyListSize` as the shared math/SetSize core. Call sites collapse to `m.applySessionListSize(w, h)` / `m.applyProjectListSize(w, h)`.

## Summary

Cycle-1's list.Model-by-value and duplicated nav-prefix findings are cleanly resolved. The size-helper consolidation overshot: `applyListSize`'s two-arg pairing (list pointer + binding source) is invariant per page but now restated by callers at 8 sites with no compile-time guard against mismatched pairs. A thin per-page wrapper layer over the consolidated core would restore invariant ownership without reintroducing the original duplication.
