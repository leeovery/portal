package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// Compile-time assertion that *tmux.Client satisfies previewAttachTmux. If
// any of HasSessionProbe / SelectWindow / SelectPane drifts on Client, this
// line breaks the build before any caller is affected. Mirrors the pattern
// used by preview_adapter.go for TmuxEnumerator / ScrollbackReader.
var _ previewAttachTmux = (*tmux.Client)(nil)

// previewAttachTmux is the tmux-facing seam for the Enter pre-select pipeline.
// Three methods, one per pre-connector tmux call: HasSessionProbe (step 1,
// three-shape discriminator), SelectWindow (step 2, best-effort), SelectPane
// (step 3, best-effort).
//
// Production wiring is *tmux.Client (each method on Client matches the shape
// here byte-for-byte). Tests substitute fakes recording call order and
// returning canned outcomes.
type previewAttachTmux interface {
	HasSessionProbe(name string) (bool, error)
	SelectWindow(session string, window int) error
	SelectPane(session string, window, pane int) error
}

// previewAttachBailMsg signals that Enter must abandon the connector handoff
// and dispatch the session-killed-externally bail path: pagePreview →
// pageSessions transition, sessions-list refresh, and inline flash. Session
// is the captured session name (the value HasSessionProbe was called with);
// the bail-message renderer consumes it to format the flash text. Empty
// when the defensive empty-session guard fires before any tmux call.
//
// Phase 2 / model dispatch consume this message — see spec § Session-killed-
// externally bail path.
type previewAttachBailMsg struct {
	Session string
}

// previewAttachSelectedMsg is the success terminal of the preview-Enter
// pipeline. The pipeline executes only the three pre-select tmux calls
// (HasSessionProbe + SelectWindow + SelectPane) and then emits this message
// carrying the captured session name. The model handler records the name
// on m.selected and returns tea.Quit so the TUI program shuts down before
// the connector runs — matching the Sessions-page Enter handoff shape.
//
// The connector.Connect call happens post-TUI in cmd/open.go's
// processTUIResult, NOT inside the live tea.Cmd goroutine. This eliminates
// the inside-tmux orphan-portal-process regression where switch-client
// would move the surrounding tmux client to the target session while the
// portal process kept event-looping with no UI.
type previewAttachSelectedMsg struct {
	Session string
}

// previewAttachPipeline composes the three pre-select tmux calls into a
// single tea.Cmd factory exercisable in isolation. Dependencies are
// constructor-injected so tests can substitute fakes for tmux and the
// logger without touching package-level state.
//
// The pipeline NO LONGER owns the connector handoff. On success it emits
// previewAttachSelectedMsg and the connector.Connect call is performed
// post-TUI by cmd/open.go's processTUIResult — see preview_attach.go
// docstring on previewAttachSelectedMsg for rationale.
//
// Spec: § Pre-select + attach sequence (steps 1-3; step 4 is post-TUI).
type previewAttachPipeline struct {
	tmux   previewAttachTmux
	logger *state.Logger
}

// NewPreviewAttachPipeline is the production constructor for the Enter
// pre-select pipeline. The seam interface (previewAttachTmux) stays
// unexported so the pipeline's internals are not part of the package's
// exported surface; production callers pass concrete types that satisfy
// it structurally (*tmux.Client satisfies previewAttachTmux).
//
// logger may be nil — every Warn call inside Run honours *state.Logger's
// nil-receiver no-op contract. Passing nil from production sites where a
// logger could not be opened is intentional and tolerated.
//
// The returned PreviewAttacher is the exported seam consumed by tui.Model
// via WithPreviewAttachPipeline.
func NewPreviewAttachPipeline(t previewAttachTmux, logger *state.Logger) PreviewAttacher {
	return &previewAttachPipeline{tmux: t, logger: logger}
}

// Run returns a tea.Cmd that executes the three pre-select tmux calls in
// order and resolves to exactly one of two terminal message types:
// previewAttachBailMsg for the externally-killed bail path, or
// previewAttachSelectedMsg for the post-pre-select handoff envelope.
//
// Step semantics (spec § Pre-select + attach sequence):
//
//  0. Empty-session defensive guard: bail without invoking tmux. Catches
//     callers that forgot to gate on a non-empty captured session — the
//     pipeline is the safety net, not a contract enforcer above it.
//  1. HasSessionProbe(session) — three observable shapes:
//     (true, nil)      proceed.
//     (false, *exec.ExitError) bail with previewAttachBailMsg{Session: name}.
//     (true, non-ExitError-err) OS-layer fault; log WARN, proceed.
//  2. SelectWindow(session, window) — best-effort; log WARN on err, proceed.
//  3. SelectPane(session, window, pane) — best-effort; log WARN on err, proceed.
//  4. Emit previewAttachSelectedMsg{Session: session}. The model handler
//     records the session on m.selected and returns tea.Quit; the
//     connector.Connect call runs post-TUI in cmd/open.go's
//     processTUIResult. Inside-tmux: TUI quits before switch-client runs,
//     so no orphan portal process. Outside-tmux: same effect via post-TUI
//     syscall.Exec.
//
// The Cmd executes synchronously inside its goroutine. Blocking tmux calls
// are acceptable per spec — the three-call sequence is sub-millisecond
// locally and runs off the UI thread.
func (p *previewAttachPipeline) Run(session string, window, pane int) tea.Cmd {
	return func() tea.Msg {
		// Step 0: empty-session defensive guard.
		if session == "" {
			return previewAttachBailMsg{Session: ""}
		}

		// Step 1: HasSessionProbe with three-shape discriminator.
		present, err := p.tmux.HasSessionProbe(session)
		if !present {
			// (false, *exec.ExitError) — genuine non-zero tmux exit; bail.
			// (false, nil) — defensive: treat as bail (present=false dominates).
			return previewAttachBailMsg{Session: session}
		}
		if err != nil {
			// (true, non-ExitError-err) — OS-layer fault; WARN-and-proceed.
			p.logger.Warn(state.ComponentPreview, "has-session probe OS-layer error for %q: %v", session, err)
		}

		// Step 2: SelectWindow — best-effort.
		if err := p.tmux.SelectWindow(session, window); err != nil {
			p.logger.Warn(state.ComponentPreview, "select-window %q:%d failed: %v", session, window, err)
		}

		// Step 3: SelectPane — best-effort.
		if err := p.tmux.SelectPane(session, window, pane); err != nil {
			p.logger.Warn(state.ComponentPreview, "select-pane %q:%d.%d failed: %v", session, window, pane, err)
		}

		// Step 4: emit selected envelope. The connector handoff happens
		// post-TUI in cmd/open.go's processTUIResult; the model handler
		// records the session on m.selected and returns tea.Quit.
		return previewAttachSelectedMsg{Session: session}
	}
}
