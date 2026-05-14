package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// previewChromeHeight is the number of lines occupied by the chrome line
// rendered above the viewport. v1 uses a single header line; if chrome
// later grows to multiple lines, only this constant changes — viewport
// resize math is centralised on it.
const previewChromeHeight = 1

// previewPlaceholder is the canonical user-facing string rendered into the
// viewport when the ScrollbackReader returns the unified "no content
// available" shape — (nil, nil) — collapsing ENOENT, zero-byte .bin, and
// zero-line file (only an unterminated partial) per § Architecture Summary
// > Test seams > ScrollbackReader return contract. Chrome counts are
// unaffected: the placeholder lives strictly inside the viewport surface.
const previewPlaceholder = "(no saved content)"

// previewReadError is the canonical user-facing string rendered into the
// viewport when the ScrollbackReader returns (nil, err) — an OS-level read
// failure such as EACCES or EIO — per § Read-Failure Handling > Placeholder
// > Error string. Uniform across errno types: every error produces this
// byte-identical string. Distinct from previewPlaceholder so the (nil, err)
// outcome is observably different from (nil, nil). No per-pane error cache
// exists on previewModel; future focus changes onto the same pane retry the
// read fresh via the dispatcher.
const previewReadError = "(unable to read scrollback)"

// previewModel renders a single tmux pane's saved scrollback inside a
// viewport. v1 of the preview page covers the full terminal; chrome (header,
// footer, borders) is layered on by Phase 3 and does not exist yet.
//
// Construction is performed via NewPreviewModel — the type is intentionally
// unexported so the constructor is the only way to wire one up. Both seams
// (TmuxEnumerator and ScrollbackReader) are constructor-injected; there is no
// package-level seam variable for preview.
//
// Zero value reserved for "between opens"; methods must not be called on a
// zero previewModel — currentPaneKey would silently return SanitizePaneKey("",
// 0, 0) and currentGroup would index an empty slice.
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
//  3. otherwise focus (0,0) and dispatch to readFocusedPaneIntoViewport so
//     the synchronous tail-N read and the (bytes, err) → viewport translation
//     run through the single shared dispatcher (placeholder for (nil, nil),
//     bytes verbatim otherwise) and the viewport is anchored at scroll-tail.
//
// The (nil, err) error-string branch is owned by Phase 4 task 4-2; this
// constructor does not encode error wording itself — it delegates to the
// shared helper.
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
		viewport:   viewport.New(width, max(0, height-previewChromeHeight)),
		width:      width,
		height:     height,
	}

	// Single dispatcher shared with cycle handlers (Tab, ], [) so the three
	// (bytes, err) shapes from ScrollbackReader.Tail are translated to viewport
	// state in exactly one place. bubbles@v1.0.0 viewport.SetContent only
	// auto-jumps to bottom when the previous YOffset overshoots the new
	// content; on a fresh viewport (YOffset == 0) it leaves the scroll
	// position at the top, so the helper jumps explicitly to satisfy the
	// "anchored at scroll-tail" contract.
	m.viewport = m.readFocusedPaneIntoViewport()

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

// chromeLine renders the single-line chrome floor described in
// § Multi-pane Rendering Shape > Chrome Floor: window/pane ordinal
// counters, the focused window's name, and visible cycle-key hints.
//
// Counters are 1-based ordinals — wOrdinal in 1..len(groups), pOrdinal in
// 1..len(currentGroup().PaneIndices) — derived from slice position, not
// the raw tmux WindowIndex / PaneIndices values. Under non-contiguous
// window_index (e.g. 0,2,5) or pane-base-index 1, this preserves the
// "1..N as the user cycles, never the raw index" contract per
// § Multi-pane Rendering Shape > Counter semantics. Window name is
// rendered verbatim — pipe handling and other escaping is the
// enumeration layer's responsibility.
//
// Pure: no I/O, no enumerator / reader calls. Wired into View() by the
// build-phase task that follows; this method is callable in isolation
// from tests and produces the same string regardless of mid-flight model
// state on enumerator/reader.
//
// Wording deliberately excludes liveness-implying tokens
// ("live", "now showing", "realtime", "current command", etc.) per
// § Source of Preview Bytes > Surface label honesty: preview is a
// snapshot, not a live tail.
func (m previewModel) chromeLine() string {
	wTotal := len(m.groups)
	pTotal := len(m.currentGroup().PaneIndices)
	wOrdinal := m.windowIdx + 1
	pOrdinal := m.paneIdx + 1
	windowName := m.currentGroup().WindowName
	return fmt.Sprintf(
		"Window %d of %d · Pane %d of %d · win: %s    ] next win · [ prev win · tab next pane · esc back",
		wOrdinal, wTotal, pOrdinal, pTotal, windowName,
	)
}

// readFocusedPaneIntoViewport performs the synchronous tail-N read for the
// currently-focused pane and translates the ScrollbackReader.Tail (bytes, err)
// outcome to viewport content, anchored at scroll-tail. Shared by every
// focus-changing branch of Update (Tab, `]`, `[`) AND by NewPreviewModel's
// initial-open path, so the three observable shapes are dispatched in exactly
// one place per § Architecture Summary > Test seams > ScrollbackReader
// return contract:
//
//   - (bytes != nil, _) — bytes rendered verbatim.
//   - (nil, nil) — placeholder ("(no saved content)") rendered. Collapses
//     ENOENT, zero-byte .bin, and zero-line file (only an unterminated
//     partial) into one shape.
//   - (nil, err != nil) — OS-level read failure. Renders the canonical
//     error string ("(unable to read scrollback)") uniformly across errno
//     types per § Read-Failure Handling > Placeholder > Error string. No
//     per-pane error state is cached on the model; refocusing the same
//     pane (Tab/]/[ away and back) re-issues a fresh Tail call through
//     this dispatcher, so a transient error can recover on retry.
//
// Value receiver — the helper operates on a local copy of m.viewport and
// returns the mutated viewport.Model. Callers (NewPreviewModel and every
// focus-changing branch of Update) assign the returned value back onto
// m.viewport so the surrounding value-receiver style stays consistent
// across the type. Returning the viewport rather than mutating in place
// keeps every method on previewModel a value receiver, eliminating the
// latent bug class where future non-copyable fields could silently desync
// after a value copy.
func (m previewModel) readFocusedPaneIntoViewport() viewport.Model {
	vp := m.viewport
	bytes, err := m.reader.Tail(m.currentPaneKey())
	switch {
	case bytes == nil && err == nil:
		vp.SetContent(previewPlaceholder)
	// The (nil, nil) arm above takes precedence so the (nil, err) shape from
	// the spec's three-shape contract lands here cleanly. The helper never
	// returns (bytes != nil, err != nil); this arm is shaped defensively to
	// route any such future drift to the user-visible error string rather
	// than silently rendering bytes alongside an ignored error.
	case err != nil:
		vp.SetContent(previewReadError)
	default:
		vp.SetContent(string(bytes))
	}
	vp.GotoBottom()
	return vp
}

// previewDismissedMsg is emitted when the user presses Esc inside the
// preview page. The top-level Update consumes it to flip activePage back
// to PageSessions without mutating the underlying sessionList — preserving
// cursor position and filter state byte-identically across the
// open/dismiss round trip.
type previewDismissedMsg struct{}

// previewSessionsRefreshedMsg carries the result of the live Sessions-list
// re-fetch dispatched when previewDismissedMsg is consumed. PreserveName is
// the name of the session that was highlighted when preview opened — the
// top-level handler uses it to re-anchor the bubbles/list cursor by name
// so that a still-existing previously-highlighted session keeps its cursor
// and a removed one falls back to a clamped valid neighbour.
//
// Lister errors are intentionally surfaced (not swallowed inside the cmd)
// so the handler can decide policy: the current policy is to keep the
// pre-refresh list intact rather than zero it out, because the user has
// just dismissed preview and expects to land on a usable Sessions list.
type previewSessionsRefreshedMsg struct {
	Sessions     []tmux.Session
	Err          error
	PreserveName string
}

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
		m.viewport.Height = max(0, msg.Height-previewChromeHeight)
		return m, nil
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEsc:
			return m, func() tea.Msg { return previewDismissedMsg{} }
		// viewport.DefaultKeyMap (bubbles@v1.0.0) does not bind Home/End;
		// preview must own them to satisfy the acceptance criterion that
		// these keys jump to top/bottom inside the loaded buffer.
		case tea.KeyHome:
			m.viewport.GotoTop()
			return m, nil
		case tea.KeyEnd:
			m.viewport.GotoBottom()
			return m, nil
		case tea.KeyTab:
			paneCount := len(m.currentGroup().PaneIndices)
			if paneCount <= 1 {
				return m, nil
			}
			m.paneIdx = (m.paneIdx + 1) % paneCount
			m.viewport = m.readFocusedPaneIntoViewport()
			return m, nil
		case tea.KeyRunes:
			// `]` advances to the next window; `[` rewinds to the previous
			// window. Both reset paneIdx to 0 (per § Multi-pane Rendering
			// Shape > Pane focus on window cycle — per-window pane focus is
			// not retained) and synchronously re-read the new pane's tail-N
			// per § Refresh Semantics > Read Trigger Events. In a session
			// with one window the keys are a silent no-op regardless of pane
			// count — `]` / `[` iterate windows, not panes.
			switch string(msg.Runes) {
			case "]":
				if len(m.groups) <= 1 {
					return m, nil
				}
				m.windowIdx = (m.windowIdx + 1) % len(m.groups)
				m.paneIdx = 0
				m.viewport = m.readFocusedPaneIntoViewport()
				return m, nil
			case "[":
				if len(m.groups) <= 1 {
					return m, nil
				}
				m.windowIdx = (m.windowIdx - 1 + len(m.groups)) % len(m.groups)
				m.paneIdx = 0
				m.viewport = m.readFocusedPaneIntoViewport()
				return m, nil
			}
		}
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// View returns the chrome line composed vertically above the embedded
// viewport contents. Chrome on top, viewport below — single newline
// separator. Header-on-top is the build-phase choice (spec § Open Items >
// Chrome Floor defers placement); only previewChromeHeight and this
// orientation change if footer is later preferred. Pinned by tests so
// drift is caught loudly.
func (m previewModel) View() string {
	return m.chromeLine() + "\n" + m.viewport.View()
}
