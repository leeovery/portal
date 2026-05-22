# Architecture Analysis — Cycle 1

STATUS: findings
FINDINGS_COUNT: 2

## Findings

### FINDING 1: list.Model passed by value through footer helpers

- SEVERITY: low
- FILES:
  - `internal/tui/model.go:627` (sessionFooterBindings)
  - `internal/tui/model.go:648` (projectFooterBindings)
  - `internal/tui/model.go:702` (renderKeymapFooter)
  - `internal/tui/model.go:751` (sessionFooterHeight)
  - `internal/tui/model.go:758` (projectFooterHeight)
- DESCRIPTION: sessionFooterBindings, projectFooterBindings, and renderKeymapFooter all take list.Model by value, even though they only read l.KeyMap.*, l.Help, and l.Styles.HelpStyle. list.Model is a heavyweight struct (embeds a paginator, textinput, help.Model, full Styles set, and item slice). Each render path copies the whole struct twice per frame — once for footer height measurement and once for renderKeymapFooter display.
- RECOMMENDATION: Switch helpers to `*list.Model` receivers, or make them methods on Model reaching into m.sessionList / m.projectList. Removes per-frame struct copies and tightens the seam so the helper's dependency surface is visible.

### FINDING 2: Duplicated nav+filter binding prefix across the two footer builders

- SEVERITY: low
- FILES:
  - `internal/tui/model.go:627-641`
  - `internal/tui/model.go:648-665`
- DESCRIPTION: sessionFooterBindings and projectFooterBindings each begin with an identical 10-entry slice literal pulling CursorUp / CursorDown / NextPage / PrevPage / GoToStart / GoToEnd / Filter / ClearFilter / AcceptWhileFiltering / CancelWhileFiltering. Future binding-set drift between the two pages is now possible by accident.
- RECOMMENDATION: Extract `listNavAndFilterBindings(l *list.Model) []key.Binding` once and have both footer builders call it then append their page-specific tail.

## Summary

Implementation composes cleanly into existing TUI surfaces; seam between renderKeymapFooter, the size-apply helpers, and view functions is sound. Two minor architectural smells: helpers take list.Model by value (heavy copies on every render), and the nav+filter prefix is duplicated verbatim across the two binding builders.

(Note: Finding 2 overlaps with the duplication agent's Finding 1 — same root cause, both agents independently surfaced it.)
