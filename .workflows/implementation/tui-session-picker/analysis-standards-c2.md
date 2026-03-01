AGENT: standards
FINDINGS:
- FINDING: Help bar missing [q] quit across all page modes
  SEVERITY: low
  FILES: internal/tui/model.go:367, internal/tui/model.go:402, internal/tui/model.go:353-361, internal/tui/model.go:377-386, internal/tui/model.go:390-396
  DESCRIPTION: The spec defines three help bar layouts, all ending with "[q] quit":
    Sessions: [enter] attach  [r] rename  [k] kill  [p] projects  [n] new in cwd  [/] filter  [q] quit
    Projects: [enter] new session  [e] edit  [d] delete  [s] sessions  [n] new in cwd  [b] browse  [/] filter  [q] quit
    Command-pending: [enter] run here  [n] new in cwd  [b] browse  [/] filter  [q] quit
    The implementation calls DisableQuitKeybindings() on both list models (lines 367, 402) to prevent bubbles/list from handling q internally, then handles q manually in updateSessionList and updateProjectsPage. However, DisableQuitKeybindings() also removes q from the list's default help keys, so [q] quit does not appear in the help bar. The q key works correctly; it is just not discoverable in the help text. The sessionHelpKeys, projectHelpKeys, and commandPendingHelpKeys functions do not include a q binding either.
  RECOMMENDATION: Add a q/quit key.Binding to each of the three help key functions (sessionHelpKeys, projectHelpKeys, commandPendingHelpKeys) so it appears in the help bar. The binding is display-only since q is already handled by the Update methods.
SUMMARY: Implementation conforms to the specification on all major decisions. One low-severity drift: the q/quit binding is missing from all help bars despite being specified in all three help bar layouts.
