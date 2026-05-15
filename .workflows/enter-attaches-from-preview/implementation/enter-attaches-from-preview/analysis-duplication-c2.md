STATUS: findings
FINDINGS_COUNT: 3

AGENT: duplication

FINDINGS:

- FINDING: flashModelOnSessionsPage is a redundant alias for flashModelWithSessions
  SEVERITY: low
  FILES: internal/tui/sessions_flash_clear_test.go:23-25, internal/tui/sessions_flash_render_test.go:36-45
  DESCRIPTION: flashModelOnSessionsPage(names ...string) Model is a one-line wrapper that calls flashModelWithSessions(names...) verbatim. The renaming adds a hop with no benefit.
  RECOMMENDATION: Delete flashModelOnSessionsPage and update callers to use flashModelWithSessions directly.

- FINDING: Single-pane-group literal repeated 21 times across preview-attach bail/selected tests
  SEVERITY: low
  FILES: internal/tui/preview_attach_bail_test.go (8), internal/tui/preview_attach_bail_flash_test.go (10), internal/tui/preview_attach_selected_test.go (3)
  DESCRIPTION: The literal &stubEnumerator{groups: []tmux.WindowGroup{{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}}}} appears verbatim 21 times — same fixture used for tests where the structural shape is incidental.
  RECOMMENDATION: Add singlePaneGroups() helper; replace the 21 literal sites.

- FINDING: findFlashTickMsg and findRefreshedMsg share an identical generic batch-message-finder skeleton
  SEVERITY: low
  FILES: internal/tui/preview_attach_bail_flash_test.go:38-48, internal/tui/preview_attach_bail_flash_test.go:52-62
  DESCRIPTION: Two side-by-side helpers iterate a []tea.Cmd, invoke each non-nil cmd, type-assert against a specific msg type, and return on first match. Bodies differ only in message type.
  RECOMMENDATION: Defer. Two instances don't yet warrant a generic finder; introduce findInBatch[T any] only when a third caller emerges.

SUMMARY: Three low-severity test-only duplication findings. Production code clean post cycle 1 refactors.
