package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/mattn/go-runewidth"
)

// verboseKeymap and compactKeymap are the two canonical keymap strings used by
// the preview frame's chrome line. They are pinned to the spec's exact byte
// content so tests catch unintentional drift loudly per
// specification.md § Keymap glyphs > Constants. Verbose is the default form at
// typical widths; compact is the cascade tier 3 compression (single-space
// separated, no interpunct, 9 display cells). Token order matches across both
// forms — `] [ ⇥ ⏎ ⎋` left-to-right — so a user resizing the terminal sees the
// same sequence of keys with action labels added or removed.
const (
	verboseKeymap = "] next win · [ prev win · ⇥ next pane · ⏎ attach · ⎋ back"
	compactKeymap = "] [ ⇥ ⏎ ⎋"
)

// minWindowNameCells is the lower bound on the display-cell budget allocated
// to the window-name string before the cascade falls through from tier 1
// ("truncate window name with …") to tier 2 ("drop · win: {name} segment
// entirely"). Below this minimum a truncated name reads as garbage rather
// than as a recognisable name. Per specification.md § Width cascade > Tier 2.
const minWindowNameCells = 8

// previewBorderColor is the single unified adaptive colour applied to all four
// edges of the preview frame (the three lipgloss-rendered edges plus the
// hand-composed top edge's border parts) per specification.md § Border colour
// and § Style sourcing. The name foregrounds the variable's role (border
// colour for the preview frame) rather than its current hue, so a future hue
// change does not produce a misleading identifier. Hex values are the design
// target; lipgloss/termenv handles NO_COLOR and palette downgrade automatically.
var previewBorderColor = lipgloss.AdaptiveColor{Light: "#3B5577", Dark: "#7B95BD"}

// previewFrameOverhead is the total number of frame rows the preview's
// rounded border occupies: top border (carrying chrome) + bottom border
// = 2 rows of frame overhead. Used to compute the viewport's inner
// height in NewPreviewModel and the tea.WindowSizeMsg handler.
const previewFrameOverhead = 2

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

// truncateToCells returns s clipped to fit within a budget measured in display
// cells, appending the single-rune ellipsis "…" only when truncation actually
// occurred. Cells are measured per runewidth.RuneWidth — ASCII = 1, CJK = 2,
// emoji = 2, combining marks = 0 — so output width matches what a terminal
// will paint, not byte length or rune count. Used by the preview frame's
// chrome cascade (tier 1 window-name truncation, tier 2 8-cell minimum) per
// specification.md § Display-cell-aware truncation and
// § Width cascade > Tier 1.
//
// Contract:
//   - budget <= 0 returns "".
//   - empty s returns "".
//   - if runewidth.StringWidth(s) <= budget, s is returned unchanged (no
//     ellipsis).
//   - otherwise runes are accumulated until adding the next rune would exceed
//     budget − 1 (reserving one cell for the ellipsis), then "…" is appended.
//   - budget == 1 with non-empty s that does not fit whole returns "…"
//     (width 1) — the canonical result; do NOT collapse to "".
//   - no mid-rune cuts: the loop never partially consumes a codepoint, so
//     output is always valid UTF-8.
func truncateToCells(s string, budget int) string {
	if budget <= 0 {
		return ""
	}
	if s == "" {
		return ""
	}
	if runewidth.StringWidth(s) <= budget {
		return s
	}
	var b []byte
	used := 0
	for _, r := range s {
		w := runewidth.RuneWidth(r)
		if used+w > budget-1 {
			break
		}
		b = append(b, string(r)...)
		used += w
	}
	return string(b) + "…"
}

// injectSGRResets appends "\x1b[0m" (SGR reset) to the end of every
// non-empty line in s, ignoring empty lines (including any trailing
// empty element produced by a terminating newline). Used to protect
// the right border from unterminated SGR sequences in the scrollback
// body — see spec § SGR reset injection.
//
// Pure: no I/O. Idempotent in observable behaviour — terminals collapse
// "\x1b[0m\x1b[0m" to a single reset, so re-applying does not corrupt rendering.
func injectSGRResets(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if len(line) > 0 {
			lines[i] = line + "\x1b[0m"
		}
	}
	return strings.Join(lines, "\n")
}

// chromeSegmentSeparator and chromeKeymapPadding are the fixed glyphs joining
// chrome line segments per specification.md § Chrome line content > Segments.
// `chromeSegmentSeparator` precedes the "win: {name}" segment; a single space
// (`chromeKeymapPadding`) separates the structural segments from the keymap
// and the right-edge `─` filler performs the visual right-alignment described
// in the spec at wide widths.
const (
	chromeSegmentSeparator = " · win: "
	chromeKeymapPadding    = " "
)

// composeChromeLine returns a single-line top-edge row for the preview frame,
// including the corner glyphs sourced from `lipgloss.RoundedBorder()`. The
// `width` parameter is the INNER frame width (`terminalWidth − 2`); the
// returned string has display-cell width `width + 2` (the outer terminal
// width) for `width >= 0`, and is the empty string for `width < 0`. The
// cascade guarantees one-row output for all widths >= 2; below that, returns
// the empty string. No embedded newlines.
//
// Tier cascade per specification.md § Width cascade:
//
//   - Tier 1 — full chrome with window name truncated (with `…` suffix) to fit
//     the remaining budget. Active when the name budget is >= minWindowNameCells.
//   - Tier 2 — drop the `· win: {name}` segment; keep verbose keymap.
//   - Tier 3 — drop `· win: {name}` and use compact keymap form.
//   - Tier 4 — corners + `─` filler only; load-bearing fallback that always
//     fits at any width >= 0.
//
// Pure: no I/O. composeChromeLineParts is its structural sibling and shares
// the single tier-selection helper `selectChromeTier` so the two surfaces
// cannot drift.
func composeChromeLine(width, windowIdx, windowCount, paneIdx, paneCount int, windowName string) string {
	if width < 0 {
		return ""
	}
	border := lipgloss.RoundedBorder()
	outer := width + 2
	chrome, fillerCells := selectChromeTier(outer, windowIdx, windowCount, paneIdx, paneCount, windowName)
	if chrome == "" {
		// Tier 4 collapse: corners + (outer-2) filler.
		return border.TopLeft + strings.Repeat(border.Top, max(0, outer-2)) + border.TopRight
	}
	return border.TopLeft + border.Top + chrome + strings.Repeat(border.Top, fillerCells) + border.Top + border.TopRight
}

// composeChromeLineParts runs the same cascade as composeChromeLine and
// returns the three structural pieces of the top-edge row: the styled left
// border prefix (corner + 1-cell padding for non-tier-4 tiers; the entire
// collapsed row for tier 4), the unstyled chrome content (empty at tier 4),
// and the styled right border suffix (filler + 1-cell padding + corner for
// non-tier-4 tiers; empty for tier 4). Concatenating left + chrome + right
// reproduces composeChromeLine's output byte-identically.
//
// Used by View() to apply BorderForeground colour to the border parts while
// leaving the chrome content unstyled (terminal default foreground) per
// specification.md § Top edge composition > Color application.
func composeChromeLineParts(width, windowIdx, windowCount, paneIdx, paneCount int, windowName string) (left, chrome, right string) {
	if width < 0 {
		return "", "", ""
	}
	border := lipgloss.RoundedBorder()
	outer := width + 2
	c, fillerCells := selectChromeTier(outer, windowIdx, windowCount, paneIdx, paneCount, windowName)
	if c == "" {
		// Tier 4 collapse: the entire row is border parts. Returning the full
		// row as `left` keeps the (left + chrome + right) concatenation
		// contract intact while leaving chrome empty (the documented tier-4
		// signal callers can branch on).
		return border.TopLeft + strings.Repeat(border.Top, max(0, outer-2)) + border.TopRight, "", ""
	}
	left = border.TopLeft + border.Top
	right = strings.Repeat(border.Top, fillerCells) + border.Top + border.TopRight
	return left, c, right
}

// selectChromeTier is the single tier-selection helper shared by
// composeChromeLine and composeChromeLineParts. It returns the unstyled
// chrome content string (empty at tier 4) and the filler-cell count between
// chrome and the right-side 1-cell border padding (zero at tier 4 since the
// caller owns the entire collapsed row).
//
// `outer` is the OUTER terminal width (width + 2). The caller is responsible
// for translating the inner-width argument to outer and for the `width < 0`
// short-circuit; this helper assumes outer >= 2.
func selectChromeTier(outer, windowIdx, windowCount, paneIdx, paneCount int, windowName string) (chrome string, fillerCells int) {
	counters := fmt.Sprintf("Window %d of %d · Pane %d of %d", windowIdx+1, windowCount, paneIdx+1, paneCount)

	// Tier 1: full chrome with truncated window name. Fixed overhead is
	// the two corners + two 1-cell border-padding cells + counters + the
	// " · win: " segment glyphs + the single space between segments and
	// keymap + the verbose keymap. The name occupies whatever remains.
	fixedTier1 := 4 + lipgloss.Width(counters) + lipgloss.Width(chromeSegmentSeparator) + lipgloss.Width(chromeKeymapPadding) + lipgloss.Width(verboseKeymap)
	nameBudget := outer - fixedTier1
	if nameBudget >= minWindowNameCells {
		truncated := truncateToCells(windowName, nameBudget)
		candidate := counters + chromeSegmentSeparator + truncated + chromeKeymapPadding + verboseKeymap
		cw := lipgloss.Width(candidate)
		filler := outer - 4 - cw
		if filler >= 0 {
			return candidate, filler
		}
	}

	// Tier 2: drop the "· win: {name}" segment; keep verbose keymap.
	candidate2 := counters + chromeKeymapPadding + verboseKeymap
	if filler := outer - 4 - lipgloss.Width(candidate2); filler >= 0 {
		return candidate2, filler
	}

	// Tier 3: drop "· win: {name}" + use compact keymap.
	candidate3 := counters + chromeKeymapPadding + compactKeymap
	if filler := outer - 4 - lipgloss.Width(candidate3); filler >= 0 {
		return candidate3, filler
	}

	// Tier 4: corners + filler only. fillerCells is unused; the caller
	// rebuilds the entire row from outer.
	return "", 0
}

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
	attacher   PreviewAttacher
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
func NewPreviewModel(session string, enumerator TmuxEnumerator, reader ScrollbackReader, attacher PreviewAttacher, width, height int) (previewModel, bool) {
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
		attacher:   attacher,
		groups:     groups,
		windowIdx:  0,
		paneIdx:    0,
		viewport:   viewport.New(width, max(0, height-previewFrameOverhead)),
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
		"Window %d of %d · Pane %d of %d · win: %s    ] next win · [ prev win · tab next pane · enter attach · esc back",
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
// Home / End for preview-owned top/bottom jumps, intercepts Enter to
// dispatch the pre-select pipeline against the captured-then-walked
// (window, pane) coordinates (the connector handoff runs post-TUI in
// cmd/open.go's processTUIResult), and absorbs
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
		m.viewport.Height = max(0, msg.Height-previewFrameOverhead)
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
		// Enter commits an attach to the previewed session, honouring any
		// (window, pane) focus walked via ] / [ / Tab. The case must intercept
		// before viewport.Update at the bottom of Update — bubbles/viewport
		// treats Enter as a no-op today, but preview owns the key so any
		// future viewport binding cannot leak through. The pipeline receives
		// raw tmux WindowIndex / PaneIndices values (not 0-based slice
		// positions) via currentRawIndices, so non-contiguous indices and
		// non-zero pane-base-index sessions address the right tmux target.
		// Dispatch is unconditional regardless of viewport content state
		// (real bytes, "(no saved content)" placeholder, or OS read error)
		// per spec § Other edge cases > Mid-load. nil attacher is a defensive
		// silent no-op so non-attach-wired test callsites can construct
		// previewModel without nil-panicking on Enter.
		case tea.KeyEnter:
			if m.attacher == nil {
				return m, nil
			}
			windowIndex, paneIndex := m.currentRawIndices()
			return m, m.attacher.Run(m.session, windowIndex, paneIndex)
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
// Chrome Floor defers placement); only previewFrameOverhead and this
// orientation change if footer is later preferred. Pinned by tests so
// drift is caught loudly.
func (m previewModel) View() string {
	return m.chromeLine() + "\n" + m.viewport.View()
}
