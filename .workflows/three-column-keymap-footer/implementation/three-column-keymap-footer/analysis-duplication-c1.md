# Duplication Analysis — Cycle 1

STATUS: findings
FINDINGS_COUNT: 3

## Findings

### FINDING 1: Identical 10-binding navigation prefix in sessionFooterBindings and projectFooterBindings

- SEVERITY: medium
- FILES:
  - `internal/tui/model.go:625-639`
  - `internal/tui/model.go:646-660`
- DESCRIPTION: Both `sessionFooterBindings` and `projectFooterBindings` open with the exact same 10-element `[]key.Binding{l.KeyMap.CursorUp, CursorDown, NextPage, PrevPage, GoToStart, GoToEnd, Filter, ClearFilter, AcceptWhileFiltering, CancelWhileFiltering}` literal. The two functions diverge only in what page-specific tail they append (sessionHelpKeys vs projectHelpKeys/commandPendingHelpKeys). If the navigation prefix ever changes it must be edited in two locations, and the two are now an obvious copy-paste pair across the same file.
- RECOMMENDATION: Extract a private `listNavBindings(l list.Model) []key.Binding` helper returning the shared 10-binding slice, then have `sessionFooterBindings` and `projectFooterBindings` call it and append their page tail.

### FINDING 2: applySessionListSize / applyProjectListSize and sessionFooterHeight / projectFooterHeight are pairwise duplicates

- SEVERITY: low
- FILES:
  - `internal/tui/model.go:752-757` (sessionFooterHeight)
  - `internal/tui/model.go:760-763` (projectFooterHeight)
  - `internal/tui/model.go:767-769` (applySessionListSize)
  - `internal/tui/model.go:773-775` (applyProjectListSize)
- DESCRIPTION: Four helpers form two structurally identical pairs that differ only in which list field and which footer-bindings helper they reference. The symmetry burden propagates to every SetSize call site (WindowSizeMsg handler, applySessions, projectsLoaded handler, NewModelWithSessions).
- RECOMMENDATION: Consolidate into a single helper `func (m *Model) applyListSize(l *list.Model, bindings []key.Binding, width, height int)`. Borderline given each pair is two lines, but the symmetry burden on the four SetSize call sites is real.

### FINDING 3: viewSessionList and viewProjectList both manually compose listView + manual footer via JoinVertical

- SEVERITY: low
- FILES:
  - `internal/tui/model.go:1795-1797` (viewProjectList tail)
  - `internal/tui/model.go:1876-1878` (viewSessionList tail)
- DESCRIPTION: Both view functions end with the same three-step pattern: render the list-with-modal, render the keymap footer for the page's binding set, then `lipgloss.JoinVertical(lipgloss.Left, listView, footer)`. The footer-build call differs only by the bindings source.
- RECOMMENDATION: Optionally inline a `composeWithFooter(listView string, l list.Model, bindings []key.Binding) string` helper. Low severity — each call site is two lines and surrounding modal/flash composition differs per view.

## Summary

Three duplication patterns, all in `internal/tui/model.go`: a shared 10-binding nav prefix duplicated across the two FooterBindings helpers (medium — clearest extraction candidate), plus two pairwise-identical size/height helper pairs and a small JoinVertical view tail (both low — small absolute size but a real symmetry burden on future edits).
