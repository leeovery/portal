STATUS: clean
FINDINGS_COUNT: 0

AGENT: standards
FINDINGS: none

SUMMARY: Implementation conforms to specification and project conventions across all checked surfaces.

Verified spec conformance points:

- Chrome line wording exact (internal/tui/pagepreview.go:172) — `enter attach` token placed between `tab next pane` and `esc back`.
- Flash text exact (internal/tui/sessions_flash.go:87-89) — `session "<name>" no longer exists` rendered via literal double-quote bytes (`%s`, not `%q`).
- Exact-match `=` prefix uniform across HasSession, HasSessionProbe, SelectWindow, SelectPane, ResizePaneZoom, SwitchClient, AttachConnector.Connect.
- Four-call ordering enforced in previewAttachPipeline.Run.
- ExitError discriminator correctly uses errors.As for bail vs warn-and-proceed.
- Raw tmux indices used (via currentRawIndices()) per spec § Captured coordinate values.
- Connector target session-only (no `:win.pane` suffix); pre-selects in steps 2-3 position tmux.
- Flash auto-clear via tea.Tick with generation-guard.
- tea.Batch (not Sequence) for refresh + tick.
- Nil logger tolerated.
- ComponentPreview added to internal/state/logger.go.
- No t.Parallel() introduced.
- Godocs on all new exported symbols.
- Restore tests rippled to `=` prefix.
- Enter intercept placed before viewport delegation.
- Enter unconditional on viewport content state.
- previewAttachBailMsg reads msg.Session, not m.preview.session.
