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

	// Helpers compose over m.windowIdx / m.paneIdx (both 0 here), reading raw
	// indices off groups[0] and feeding state.SanitizePaneKey — byte-identical
	// to the daemon writer's key for that pane. Single source of truth shared
	// with focus-change reads in later phases.
	bytes, _ := reader.Tail(m.currentPaneKey())
	m.viewport.SetContent(string(bytes))
	// bubbles@v1.0.0 viewport.SetContent only auto-jumps to bottom when the
	// previous YOffset overshoots the new content; on a fresh viewport
	// (YOffset == 0) it leaves the scroll position at the top, so we must
	// jump explicitly to satisfy the "anchored at scroll-tail" contract.
	m.viewport.GotoBottom()

	return m, true
}

// currentGroup returns the cached tmux.WindowGroup at the model's current
// windowIdx. Pure read-only view over m.groups; never re-enumerates and never
// mutates the model.
func (m previewModel) currentGroup() tmux.WindowGroup {
	return m.groups[m.windowIdx]
}

// currentRawIndices returns the raw tmux WindowIndex and PaneIndex for the
// focused pane — *not* the 0-based ordinal positions m.windowIdx / m.paneIdx.
// Under non-contiguous window_index (e.g. 0,2,5) or pane-base-index 1, these
// are the values needed to compose the daemon's canonical pane key. Chrome
// ordinals ("Window M of N") are derived elsewhere from slice position.
func (m previewModel) currentRawIndices() (windowIndex, paneIndex int) {
	g := m.currentGroup()
	return g.WindowIndex, g.PaneIndices[m.paneIdx]
}

// currentPaneKey returns the canonical paneKey for the focused pane, byte-
// identical to the key the daemon writer uses for that pane. Composed from
// the raw indices via state.SanitizePaneKey so the resolution chain
// (paneKey → ScrollbackFile → tail-N read) addresses the same `.bin` file
// the daemon wrote.
func (m previewModel) currentPaneKey() string {
	rawWindow, rawPane := m.currentRawIndices()
	return state.SanitizePaneKey(m.session, rawWindow, rawPane)
}

// degenerate reports whether the session is the dominant ~95% case of one
// window with one pane. In that shape ] / [ / Tab silently no-op; callers
// can also use this to suppress structural chrome that would otherwise be
// trivial ("Window 1 of 1 / Pane 1 of 1").
func (m previewModel) degenerate() bool {
	return len(m.groups) == 1 && len(m.groups[0].PaneIndices) == 1
}

// previewDismissedMsg is emitted when the user presses Esc inside the
// preview page. The top-level Update consumes it to flip activePage back
// to PageSessions without mutating the underlying sessionList — preserving
// cursor position and filter state byte-identically across the
// open/dismiss round trip.
type previewDismissedMsg struct{}

// Update routes Esc to a synthesised previewDismissedMsg, intercepts
// Home / End for preview-owned top/bottom jumps, and absorbs
// tea.WindowSizeMsg to resize the embedded viewport in place. All other
// messages — including the remaining viewport scroll keys (Up, Down,
// PgUp, PgDn, ctrl-u, ctrl-d, j, k) — delegate to bubbles/viewport so
// its native keymap and resize-clamp behaviour are preserved.
//
// reader.Tail is intentionally NOT called here: scroll and resize
// operate on the already-loaded N-line buffer per § Refresh Semantics
// (resize is not a read trigger; viewport-internal scroll does not
// re-read).
func (m previewModel) Update(msg tea.Msg) (previewModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEsc:
			return m, func() tea.Msg { return previewDismissedMsg{} }
		case tea.KeyHome:
			m.viewport.GotoTop()
			return m, nil
		case tea.KeyEnd:
			m.viewport.GotoBottom()
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// View returns the rendered viewport contents. Chrome (header/footer/border)
// is Phase 3.
func (m previewModel) View() string {
	return m.viewport.View()
}
