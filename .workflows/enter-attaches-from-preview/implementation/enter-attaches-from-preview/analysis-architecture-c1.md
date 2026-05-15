STATUS: findings
FINDINGS_COUNT: 3

AGENT: architecture

FINDINGS:

- FINDING: Inside-tmux preview-Enter leaves the portal TUI orphaned after a successful switch-client
  SEVERITY: high
  FILES: internal/tui/preview_attach.go:51-63, internal/tui/preview_attach.go:117-150, internal/tui/model.go:982-992, internal/tui/preview_attach_bail_test.go:253-264, cmd/open.go:412-478
  DESCRIPTION: The preview-page Enter handler invokes previewSessionConnector.Connect from inside a tea.Cmd goroutine while the Bubble Tea program is still running, and the previewAttachErrorMsg{Err: nil} handler in model.go returns (m, nil) (no tea.Quit) on success. For the outside-tmux path this is fine — syscall.Exec replaces the process and the TUI dies with it. For the inside-tmux path (SwitchConnector.Connect → tmux switch-client), the tmux client moves to the target session but portal's process keeps event-looping with no UI surface; a stale portal process remains alive. This diverges from the Sessions-page Enter shape, where handleSessionListEnter sets selected + tea.Quit, the TUI exits cleanly, and processTUIResult(model, connector) runs the connector AFTER the program has shut down. The pipeline's own docstring (preview_attach.go:56-58) asserts "Inside tmux: switch-client succeeded; the top-level handler quits the program so the surrounding tmux client repaints" — but the handler does the opposite, and the unit test at preview_attach_bail_test.go:253-264 explicitly pins the current nil-cmd behaviour, locking in the doc/code mismatch.
  RECOMMENDATION: Restructure preview-Enter to follow the same post-TUI handoff shape as Sessions-page Enter. Two reasonable shapes: (a) split the pipeline so the tea.Cmd runs only steps 1-3 (probe + pre-selects), captures selected + the (window, pane) coords on the model, and returns tea.Quit; have processTUIResult consume that intent and call connector.Connect post-TUI. (b) Have the nil-Err handler return tea.Quit so the TUI exits after switch-client returns. Shape (a) is cleaner because it keeps the two Enter paths structurally symmetric.

- FINDING: previewDismissedMsg and previewAttachBailMsg handlers duplicate the page-flip + zero-preview + refresh sequence
  SEVERITY: low
  FILES: internal/tui/model.go:937-958, internal/tui/model.go:959-981
  DESCRIPTION: The Esc-dismiss handler and the externally-killed bail handler execute the same three-step prelude — capture preserveName, set activePage = PageSessions, zero m.preview — followed by a refresh dispatch. The bail handler additionally calls setFlash + flashTickCmd and uses tea.Batch.
  RECOMMENDATION: Extract a small helper m.exitPreviewTo(preserveName string) tea.Cmd that performs the capture-then-zero-then-refresh sequence and returns the refresh cmd.

- FINDING: HasSessionProbe and HasSession are near-identical methods on Client that encode different undocumented policies on the same primitive
  SEVERITY: low
  FILES: internal/tmux/tmux.go:117-129, internal/tmux/tmux.go:131-166
  DESCRIPTION: Both methods issue the same tmux call (has-session -t =<name>) but expose different return contracts. The two methods issue the same tmux call but encode different policies on the same input; that policy split is not surfaced anywhere except in Probe's godoc.
  RECOMMENDATION: Optional cleanup. Consider keeping a single low-level primitive returning (bool, error) with the ExitError discriminator, and letting callers translate to their preferred policy at the call site.

SUMMARY: One high-severity architectural divergence: preview-Enter dispatches the connector from inside a live tea.Cmd rather than after the TUI has quit, leaving portal orphaned in the inside-tmux switch-client path. Two lower-severity items: handler-shape duplication and an undocumented policy split.
