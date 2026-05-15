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

// previewAttachTmux is the tmux-facing seam for the Enter pre-select + attach
// pipeline. Three methods, one per pre-connector tmux call: HasSessionProbe
// (step 1, three-shape discriminator), SelectWindow (step 2, best-effort),
// SelectPane (step 3, best-effort).
//
// Production wiring is *tmux.Client (each method on Client matches the shape
// here byte-for-byte). Tests substitute fakes recording call order and
// returning canned outcomes.
type previewAttachTmux interface {
	HasSessionProbe(name string) (bool, error)
	SelectWindow(session string, window int) error
	SelectPane(session string, window, pane int) error
}

// previewSessionConnector is the connector-facing seam for the pipeline's
// terminal call. The production adapter is wired in task 1-5 to delegate to
// the cmd-layer SessionConnector (AttachConnector outside tmux,
// SwitchConnector inside tmux). Kept as a TUI-local interface to avoid an
// internal/tui → cmd import.
type previewSessionConnector interface {
	Connect(name string) error
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

// previewAttachErrorMsg is the connector-terminal envelope. Err carries the
// outcome of previewSessionConnector.Connect:
//
//   - nil: connector returned without error. Two production shapes collapse
//     here. (a) Inside tmux: switch-client succeeded; the top-level handler
//     quits the program so the surrounding tmux client repaints. (b) Outside
//     tmux: AttachConnector's syscall.Exec replaces the portal process and
//     never returns — control does not reach this branch in production.
//   - non-nil: connector failure. Inside tmux this is typically the
//     switch-client error; the top-level handler surfaces it.
type previewAttachErrorMsg struct {
	Err error
}

// previewAttachPipeline composes the four-call Enter sequence into a single
// tea.Cmd factory exercisable in isolation. Dependencies are constructor-
// injected so tests can substitute fakes for tmux, the connector, and the
// logger without touching package-level state.
//
// Spec: § Pre-select + attach sequence.
type previewAttachPipeline struct {
	tmux      previewAttachTmux
	connector previewSessionConnector
	logger    *state.Logger
}

// Run returns a tea.Cmd that executes the four-call sequence end-to-end and
// resolves to exactly one of two terminal message types: previewAttachBailMsg
// for the externally-killed bail path, or previewAttachErrorMsg for the
// connector-handoff envelope.
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
//  4. connector.Connect(session) — terminal. Wrap outcome in previewAttachErrorMsg.
//
// The Cmd executes synchronously inside its goroutine. Blocking tmux calls
// are acceptable per spec — the four-call sequence is sub-millisecond locally
// and runs off the UI thread.
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

		// Step 4: connector handoff. In production the outside-tmux path
		// never returns (syscall.Exec replaces the process); the inside-tmux
		// path returns nil on success and non-nil on switch-client failure.
		return previewAttachErrorMsg{Err: p.connector.Connect(session)}
	}
}
