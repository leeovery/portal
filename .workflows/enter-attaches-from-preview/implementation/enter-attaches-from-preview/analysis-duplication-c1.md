STATUS: findings
FINDINGS_COUNT: 3

AGENT: duplication

FINDINGS:

- FINDING: previewDismissedMsg and previewAttachBailMsg handlers share a near-identical preview-teardown sequence
  SEVERITY: medium
  FILES: internal/tui/model.go:937-958, internal/tui/model.go:959-981
  DESCRIPTION: Both handlers perform the same three-step preview teardown before diverging: capture preserveName, flip m.activePage = PageSessions, zero m.preview = previewModel{}, then call m.refreshSessionsAfterPreviewCmd(preserveName). Only the source of preserveName (m.preview.session vs msg.Session) and the trailing flash/tick batching differ.
  RECOMMENDATION: Extract a small helper on Model — e.g. func (m *Model) exitPreviewToSessions(preserveName string) tea.Cmd — that performs the page flip, preview zero, and returns the refresh cmd. The bail handler then becomes: refreshCmd := m.exitPreviewToSessions(msg.Session); m.setFlash(formatSessionGoneFlash(msg.Session)); return m, tea.Batch(refreshCmd, flashTickCmd(m.flashGen)).

- FINDING: cmd/open_test.go has two execer-recording fakes for the same interface
  SEVERITY: low
  FILES: cmd/open_test.go:247-260 (mockExecer), cmd/open_test.go:1087-1098 (recordingExecer)
  DESCRIPTION: mockExecer and recordingExecer both implement the execer interface by recording call args. They differ only in field names and the absence of err on recordingExecer.
  RECOMMENDATION: Have TestAttachConnectorConnectArgv reuse the existing mockExecer; delete recordingExecer.

- FINDING: SelectWindow / SelectPane / ResizePaneZoom share an identical compose-target + run + wrap-error skeleton
  SEVERITY: low
  FILES: internal/tmux/tmux.go:861-870, internal/tmux/tmux.go:881-890, internal/tmux/tmux.go:900-909
  DESCRIPTION: All three methods follow the same five-line shape. Hits the rule-of-three boundary, though the duplication is shallow.
  RECOMMENDATION: Defer for now. Worth revisiting only if a fourth `=`-prefixed targeted call lands; current state is acceptable.

SUMMARY: Three duplication findings, all medium-or-lower severity. Most actionable: shared preview-teardown sequence in model.go.
