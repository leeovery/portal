AGENT: duplication
FINDINGS:
- FINDING: Repeated CleanStale-then-List reload closure in project picker
  SEVERITY: low
  FILES: internal/ui/projectpicker.go:112-117, internal/ui/projectpicker.go:261-264, internal/ui/projectpicker.go:427-430
  DESCRIPTION: Three identical 4-line tea.Cmd closures perform the same sequence: call CleanStale, call List, wrap in ProjectsLoadedMsg. This is intra-file duplication carried over from cycle 2 (flagged as low severity). The pattern is small but represents a maintenance risk if the reload logic changes (e.g., adding error handling for CleanStale).
  RECOMMENDATION: Extract a private method like loadProjectsCmd() tea.Cmd on ProjectPickerModel and call it from all three sites.

- FINDING: Window count pluralization in two views
  SEVERITY: low
  FILES: cmd/list.go:41-45, internal/tui/model.go:618-620
  DESCRIPTION: Both the list command's formatSessionLong and the TUI's viewSessionList implement "1 window" vs "N windows" pluralization for tmux.Session.Windows. Carried over from cycle 2. Each is 3-4 lines.
  RECOMMENDATION: No action needed at this scale. Would only warrant extraction if a third consumer appears.

SUMMARY: All high and medium-severity duplication from cycles 1 and 2 has been addressed (PrepareSession extraction, generic fuzzy.Filter, ProjectStore type alias, quickStartResult removal, configFilePath consolidation). Two low-severity intra-file/cross-file patterns remain (project reload closure, window pluralization) but neither warrants a dedicated task at this scale.
