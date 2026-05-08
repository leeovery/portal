package tui

import (
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// previewModel renders a single tmux pane's saved scrollback inside a
// viewport. v1 of the preview page covers the full terminal; chrome (header,
// footer, borders) is layered on by Phase 3 and does not exist yet.
//
// Construction is performed via NewPreviewModel — the type is intentionally
// unexported so the constructor is the only way to wire one up. Both seams
// (TmuxEnumerator and ScrollbackReader) are constructor-injected; there is no
// package-level seam variable for preview.
type previewModel struct {
	session    string
	enumerator TmuxEnumerator
	reader     ScrollbackReader
	groups     []tmux.WindowGroup
	windowIdx  int
	paneIdx    int
	viewport   viewport.Model
	width      int
	height     int
}

// NewPreviewModel performs the initial-open ordering inline:
//  1. enumerate windows/panes for session,
//  2. on enumeration error or empty result (no groups, or first group has no
//     panes) return (zero, false) so the caller can fall through to the
//     dismiss-to-sessions silent no-open path,
//  3. otherwise focus (0,0), synchronously read the tail-N for that pane,
//     SetContent the bytes verbatim, and GotoBottom() so initial scroll
//     position is anchored at the latest output.
//
// reader.Tail's return shapes are NOT translated here; (nil, nil) and
// (nil, err) both still yield ok=true and an empty viewport. Phase 4 owns
// the placeholder/error wording.
func NewPreviewModel(session string, enumerator TmuxEnumerator, reader ScrollbackReader, width, height int) (previewModel, bool) {
	groups, err := enumerator.ListWindowsAndPanesInSession(session)
	if err != nil {
		return previewModel{}, false
	}
	if len(groups) == 0 || len(groups[0].PaneIndices) == 0 {
		return previewModel{}, false
	}

	m := previewModel{
		session:    session,
		enumerator: enumerator,
		reader:     reader,
		groups:     groups,
		windowIdx:  0,
		paneIdx:    0,
		viewport:   viewport.New(width, height),
		width:      width,
		height:     height,
	}

	paneKey := state.SanitizePaneKey(session, groups[0].WindowIndex, groups[0].PaneIndices[0])
	bytes, _ := reader.Tail(paneKey)
	m.viewport.SetContent(string(bytes))
	// bubbles@v1.0.0 viewport.SetContent only auto-jumps to bottom when the
	// previous YOffset overshoots the new content; on a fresh viewport
	// (YOffset == 0) it leaves the scroll position at the top, so we must
	// jump explicitly to satisfy the "anchored at scroll-tail" contract.
	m.viewport.GotoBottom()

	return m, true
}

// Update delegates messages to the embedded viewport. Cycle-key handling,
// Esc dismissal, and WindowSizeMsg-driven resize land in later tasks
// (2-4 / 2-6 / Phase 3).
func (m previewModel) Update(msg tea.Msg) (previewModel, tea.Cmd) {
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// View returns the rendered viewport contents. Chrome (header/footer/border)
// is Phase 3.
func (m previewModel) View() string {
	return m.viewport.View()
}
