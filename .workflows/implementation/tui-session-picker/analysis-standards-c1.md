AGENT: standards
FINDINGS:
- FINDING: Command-pending status line rendered above title instead of below it
  SEVERITY: low
  FILES: /Users/leeovery/Code/portal/internal/tui/model.go:1106-1113
  DESCRIPTION: The spec states "Title stays 'Projects' for consistency" and "A status line below the title indicates the pending command: Select project to run: {command}". The implementation prepends the status line before the project list view, resulting in the status line appearing above the "Projects" title rather than below it. The rendering order is: status line -> blank line -> "Projects" title -> list items. The spec intended: "Projects" title -> status line -> list items.
  RECOMMENDATION: Move the status line rendering so it appears after the list title but before the list items. This could be done by inserting the status line into the list's subtitle or by splitting the list View output to inject the line after the title row.
SUMMARY: One low-severity drift found: the command-pending status line is positioned above the list title instead of below it as the spec describes. All other spec decisions -- two-page architecture, bubbles/list adoption, modal system, session exclusion inside tmux, command-pending key restrictions, progressive Esc behavior, initial filter application, page navigation, help bar content, and ProjectPickerModel deletion -- are correctly implemented.
