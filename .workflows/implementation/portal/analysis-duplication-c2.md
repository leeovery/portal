AGENT: duplication
FINDINGS:
- FINDING: Duplicate ProjectStore interface definition
  SEVERITY: medium
  FILES: internal/tui/model.go:32, internal/ui/projectpicker.go:14
  DESCRIPTION: Two identical ProjectStore interfaces (List, CleanStale, Remove) are defined independently in the tui and ui packages. Both consume project.Project and have the same method signatures. A third ProjectStore interface in internal/session/create.go has a different shape (Upsert only), so it is not part of this duplication -- but the tui and ui copies are exact duplicates. The tui package already aliases ui.DirLister (line 54), showing precedent for cross-package type reuse.
  RECOMMENDATION: Remove the ProjectStore definition from internal/tui/model.go and use ui.ProjectStore directly (either via type alias as done for DirLister, or by importing it). The tui package already depends on ui.

- FINDING: Repeated fuzzy filter-and-collect pattern across three views
  SEVERITY: medium
  FILES: internal/tui/model.go:542-553, internal/ui/projectpicker.go:121-133, internal/ui/browser.go:125-137
  DESCRIPTION: Three locations implement the same pattern: if filterText is empty return the full list, otherwise iterate and collect items where fuzzy.Match(strings.ToLower(item.Name), strings.ToLower(filterText)). The only difference is the item type (tmux.Session, project.Project, browser.DirEntry). This is the classic generic filter candidate. Each implementation is ~12 lines and they are structurally identical. The model.go version also forgets to hoist strings.ToLower(filterText) outside the loop (unlike the other two), so there is copy-paste drift.
  RECOMMENDATION: Add a generic FuzzyFilter function to the internal/fuzzy package: func FuzzyFilter[T any](items []T, filter string, nameOf func(T) string) []T. Replace all three callsites. This also fixes the ToLower-per-iteration drift in model.go.

- FINDING: Repeated CleanStale-then-List reload command in project picker
  SEVERITY: low
  FILES: internal/ui/projectpicker.go:112-117, internal/ui/projectpicker.go:271-275, internal/ui/projectpicker.go:437-441
  DESCRIPTION: The same tea.Cmd closure (CleanStale, then List, then wrap in ProjectsLoadedMsg) appears three times in projectpicker.go. All three are identical 4-line closures.
  RECOMMENDATION: Extract a private method like loadProjectsCmd() tea.Cmd on ProjectPickerModel and call it from all three sites.

- FINDING: quickStartResult duplicates session.QuickStartResult
  SEVERITY: low
  FILES: cmd/open.go:162-166, internal/session/quickstart.go:10-17
  DESCRIPTION: cmd/open.go defines a local quickStartResult struct with the same three fields (SessionName, Dir, ExecArgs) as session.QuickStartResult, plus a quickStartAdapter that converts between them field-by-field. The adapter exists solely to avoid importing the session type, but the cmd package already imports internal/session.
  RECOMMENDATION: Use session.QuickStartResult directly in the quickStarter interface and PathOpener, eliminating the local struct and the adapter's field-by-field copy.

- FINDING: Duplicate window count pluralization logic
  SEVERITY: low
  FILES: cmd/list.go:41-44, internal/tui/model.go:632-635
  DESCRIPTION: Both the list command and the TUI session view implement the same "1 window" vs "N windows" pluralization for tmux.Session.Windows. Both are small (3-4 lines) but represent the same display concern applied to the same data type.
  RECOMMENDATION: Add a WindowLabel() or similar method or function to the tmux.Session type (or a formatting helper near it) and call from both sites.

SUMMARY: The main remaining duplication is the identical ProjectStore interface in tui and ui packages, and the repeated fuzzy-filter-and-collect pattern across three views (a good candidate for a generic helper). Several lower-severity items exist: repeated project reload command in project picker, a redundant quickStartResult struct mirroring session.QuickStartResult, and window-count pluralization in two places.
