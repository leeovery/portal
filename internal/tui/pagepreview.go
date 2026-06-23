package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui/theme"
	"github.com/mattn/go-runewidth"
)

// previewMarker is the §9.1 peek-mode marker rendered at the left of the header
// compartment in accent.cyan — a `◉` filled circle + the word `preview`. It
// signals "peek mode" (read-only scrollback), deliberately distinct from the
// violet main UI. Matches the Preview Screen (MV) reference frame glyph.
const previewMarker = "◉ preview"

// minSessionNameCells is the lower bound on the display-cell budget allocated
// to the session name before the header cascade drops the counters segment
// (tier 2 → tier 3). Below this minimum a truncated name reads as garbage rather
// than a recognisable name. Per §9.1 header width-cascade.
const minSessionNameCells = 8

// previewBorderColorToken is the §2.9 role token the §9.1 preview chrome
// (the full-screen joined panel's cyan border + dividers) resolves. The cyan
// "peek mode" hue is owned by the single token layer (no raw hex at the call
// site); the mode-resolved colour is computed per-render from the model's
// resolved canvas mode, mirroring every other mode-aware token. It is threaded
// into renderJoinedPanel as the panel's single-tone border token (the modals
// pass theme.MV.BorderSeparator instead).
var previewBorderColorToken = theme.MV.AccentCyan

// previewFrameOverhead is the total number of frame rows AND frame columns the
// §9.1 full-screen joined panel occupies around the body compartment:
//
//   - Rows: top border + header + header-divider + footer-divider + footer +
//     bottom border = 6 chrome rows wrapping the body.
//   - Columns: the two side borders (2) + the per-row L/R inset
//     (2·helpRowInset = 4) = 6 chrome columns wrapping the body.
//
// Both dimensions sum to 6, so the body viewport is sized to
// (termW − previewFrameOverhead) × (termH − previewFrameOverhead) and the
// composed panel fills the full terminal exactly.
const previewFrameOverhead = 6

// previewChromeRowOverhead names the six chrome ROWS the joined panel wraps the
// body in (top + header + 2 dividers + footer + bottom). It is the height the
// viewport is reduced by; equal in value to previewFrameOverhead (both 6), but
// kept as a distinct name so the height arithmetic reads by intent.
const previewChromeRowOverhead = previewFrameOverhead

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

// chromeSegSpace is the single space joining the §9.1 header segments (marker ·
// session · counters). One space between each, matching the Preview Screen (MV)
// reference frame.
const chromeSegSpace = " "

// previewFooterGap is the inter-group gap in the §9.1 footer — two canvas spaces
// between each `<glyph> <label>` group (`←→ window  ⇥ pane  ⏎ attach  ␣ back`),
// matching the Preview Screen (MV) reference frame's group spacing.
const previewFooterGap = "  "

// previewCounters formats the §9.1 slash-form counters from the 0-based
// ordinals: "Window x/y · Pane x/y". Always 1-based for display (the raw tmux
// indices never surface here — that is the responsibility of the caller, which
// passes ordinal positions, not raw indices).
func previewCounters(windowIdx, windowCount, paneIdx, paneCount int) string {
	return fmt.Sprintf("Window %d/%d · Pane %d/%d", windowIdx+1, windowCount, paneIdx+1, paneCount)
}

// previewHeaderSegments is the structural, UNSTYLED content of the §9.1 header
// compartment after the width cascade: the `◉ preview` marker (always present),
// the session name (possibly truncated), and the `Window x/y · Pane x/y`
// counters ("" when the cascade drops them at narrow widths). View() styles each
// segment with its §2.9 token (marker accent.cyan, session text.primary,
// counters text.detail).
type previewHeaderSegments struct {
	marker   string
	session  string
	counters string // "" when the cascade drops the counters segment
}

// selectPreviewHeaderTier runs the §9.1 header width cascade against a target
// content width (the joined panel's contentWidth — the body width). It degrades
// gracefully as the panel narrows, re-styled from the prior single-bar cascade
// (tier mechanism preserved):
//
//   - Tier 1: marker + full session + counters.
//   - Tier 2: marker + truncated session (>= minSessionNameCells) + counters.
//   - Tier 3: drop counters; marker + full session.
//   - Tier 4: drop counters; marker + (hard-)truncated session.
//
// The returned segments always fit within `width` cells (the marker + the
// inter-segment spaces + the [truncated] session [+ counters]). `width` is the
// panel content width (already clamped to >= 0 by the caller).
func selectPreviewHeaderTier(width int, session string, windowIdx, windowCount, paneIdx, paneCount int) previewHeaderSegments {
	counters := previewCounters(windowIdx, windowCount, paneIdx, paneCount)
	markerW := lipgloss.Width(previewMarker)
	gapW := lipgloss.Width(chromeSegSpace)

	// Tier 1: marker + space + full session + space + counters.
	fullW := markerW + gapW + lipgloss.Width(session) + gapW + lipgloss.Width(counters)
	if fullW <= width {
		return previewHeaderSegments{marker: previewMarker, session: session, counters: counters}
	}

	// Tier 2: truncate the session, keep counters. Truncate only if the residual
	// session budget leaves at least minSessionNameCells.
	fixedT2 := markerW + gapW + gapW + lipgloss.Width(counters)
	if sessBudget := width - fixedT2; sessBudget >= minSessionNameCells {
		return previewHeaderSegments{marker: previewMarker, session: truncateToCells(session, sessBudget), counters: counters}
	}

	// Tier 3: drop counters; marker + space + full session.
	if markerW+gapW+lipgloss.Width(session) <= width {
		return previewHeaderSegments{marker: previewMarker, session: session}
	}

	// Tier 4: drop counters; marker + space + hard-truncated session (down to a
	// single ellipsis cell). max(0, …) keeps the budget non-negative at degenerate
	// widths where even the marker overflows.
	sessBudget := max(0, width-markerW-gapW)
	return previewHeaderSegments{marker: previewMarker, session: truncateToCells(session, sessBudget)}
}

// composePreviewHeaderRow composes the STYLED §9.1 header compartment row at the
// given panel content width: the accent.cyan `◉ preview` marker + text.primary
// session name + text.detail `Window x/y · Pane x/y` counters, joined by single
// spaces. Under the NO_COLOR carve-out (§2.5 / §9.2) every segment renders
// colourless (no foreground SGR) but the structure stays present. The row's
// natural width is <= contentWidth (the cascade fits it); renderJoinedPanel's
// helpInsetRow pads it to the uniform frame width.
func composePreviewHeaderRow(contentWidth, windowIdx, windowCount, paneIdx, paneCount int, session string, mode theme.Mode, colourless bool) string {
	// Degenerate widths: the marker itself (9 cells) exceeds the body width — clip
	// the marker to contentWidth (cells) and render that alone so the panel still
	// fills exactly the terminal width and never overflows.
	if contentWidth < lipgloss.Width(previewMarker) {
		clipped := truncateToCells(previewMarker, contentWidth)
		return headerStyle(theme.MV.AccentCyan, mode, colourless).Render(clipped)
	}

	segs := selectPreviewHeaderTier(contentWidth, session, windowIdx, windowCount, paneIdx, paneCount)
	gap := headerCanvasBg(mode, colourless).Render(chromeSegSpace)

	row := headerStyle(theme.MV.AccentCyan, mode, colourless).Render(segs.marker)
	// The session segment (and its leading gap) is omitted when the cascade
	// truncated it to empty, so the marker-alone row stays exactly marker-wide
	// and the panel never overflows the body width.
	if segs.session != "" {
		sessionSeg := headerStyle(theme.MV.TextPrimary, mode, colourless).Render(segs.session)
		row = lipgloss.JoinHorizontal(lipgloss.Top, row, gap, sessionSeg)
	}
	if segs.counters != "" {
		counters := headerStyle(theme.MV.TextDetail, mode, colourless).Render(segs.counters)
		row = lipgloss.JoinHorizontal(lipgloss.Top, row, gap, counters)
	}
	return row
}

// previewFooterGroup is one `<glyph> <label>` nav-hint pair in the §9.1 footer.
type previewFooterGroup struct {
	glyph string
	label string
}

// previewFooterGroups derives the §9.1 footer nav-hint groups from the Core
// entries of the shared previewKeymap descriptor (§12.1) — the single source of
// truth that also feeds the ? help. Each Core entry's glyph is the descriptor
// Key (already a glyph form) and the label is its terse Action. The descriptor
// order (window, pane, attach, back) is the footer's left-to-right order,
// matching the Preview Screen (MV) reference frame.
func previewFooterGroups() []previewFooterGroup {
	groups := make([]previewFooterGroup, 0, 4)
	for _, e := range previewKeymap() {
		if !e.Core {
			continue
		}
		groups = append(groups, previewFooterGroup{glyph: e.Key, label: e.Action})
	}
	return groups
}

// composePreviewFooterRow composes the STYLED §9.1 footer compartment row within
// contentWidth (the panel body width), degrading progressively so the footer
// never overflows the full-screen panel:
//
//   - Full labelled form: `←→ window  ⇥ pane  ⏎ attach  ␣ back` (each glyph
//     accent.blue, each label text.detail, groups space-separated).
//   - When that exceeds contentWidth: compact glyphs only (`←→  ⇥  ⏎  ␣`).
//   - When even the compact form exceeds contentWidth: drop trailing groups,
//     keeping the leading nav glyphs that fit (degrade-never-break at degenerate
//     widths). At least the first glyph renders whenever any width is available.
//
// Under the NO_COLOR carve-out the hues drop but the structure stays present.
func composePreviewFooterRow(contentWidth int, mode theme.Mode, colourless bool) string {
	groups := previewFooterGroups()

	full := previewFooterFromGroups(groups, true, mode, colourless)
	if lipgloss.Width(full) <= contentWidth {
		return full
	}
	compact := previewFooterFromGroups(groups, false, mode, colourless)
	if lipgloss.Width(compact) <= contentWidth {
		return compact
	}
	// Too narrow for even the compact form — drop trailing glyph groups until the
	// remainder fits (or only the first glyph remains).
	for n := len(groups) - 1; n >= 1; n-- {
		trimmed := previewFooterFromGroups(groups[:n], false, mode, colourless)
		if lipgloss.Width(trimmed) <= contentWidth {
			return trimmed
		}
	}
	// Degenerate widths: even a single glyph overflows — clip the first glyph to
	// contentWidth (cells) so the footer row never exceeds the body width.
	clipped := truncateToCells(groups[0].glyph, contentWidth)
	return headerStyle(theme.MV.AccentBlue, mode, colourless).Render(clipped)
}

// previewFooterFromGroups renders the footer groups joined by previewFooterGap.
// When labelled is true each group is `<glyph accent.blue> <label text.detail>`;
// when false only the accent.blue glyph renders (the compact cascade form).
func previewFooterFromGroups(groups []previewFooterGroup, labelled bool, mode theme.Mode, colourless bool) string {
	gap := headerCanvasBg(mode, colourless).Render(previewFooterGap)
	rendered := make([]string, 0, len(groups))
	for _, g := range groups {
		if labelled {
			rendered = append(rendered, previewFooterHint(g.glyph, g.label, mode, colourless))
		} else {
			rendered = append(rendered, headerStyle(theme.MV.AccentBlue, mode, colourless).Render(g.glyph))
		}
	}
	return strings.Join(rendered, gap)
}

// previewFooterHint renders one `<glyph> <label>` footer group: the glyph in
// accent.blue, a single canvas spacer, then the label in text.detail — the
// shared footer key-hint shape (mirrors killModalKeyHint).
func previewFooterHint(glyph, label string, mode theme.Mode, colourless bool) string {
	glyphSeg := headerStyle(theme.MV.AccentBlue, mode, colourless).Render(glyph)
	gap := headerCanvasBg(mode, colourless).Render(" ")
	labelSeg := headerStyle(theme.MV.TextDetail, mode, colourless).Render(label)
	return lipgloss.JoinHorizontal(lipgloss.Top, glyphSeg, gap, labelSeg)
}

// previewModel renders a single tmux pane's saved scrollback inside a
// viewport, wrapped in the §9.1 full-screen accent.cyan joined panel (header,
// body, footer compartments) composed by View().
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
	// mode is the resolved light/dark canvas the §9.1 peek-mode chrome is
	// painted for. The zero value (theme.Dark) is the parity-preserving
	// dark-default; the parent model assigns m.canvasMode onto it after
	// construction so the cyan frame + top bar resolve the right variant.
	mode theme.Mode
	// colourless is the §2.5 NO_COLOR carve-out flag mirrored from the parent
	// model. When set the chrome drops its hue (no foreground SGR) but keeps
	// the structure — marker, session, counters, hints, frame glyphs (§9.2).
	colourless bool
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

	// Mirrors m.innerWidth() / m.innerHeight() — those methods aren't usable
	// here because m.width / m.height haven't been assigned yet.
	innerW := max(0, width-previewFrameOverhead)
	innerH := max(0, height-previewFrameOverhead)
	m := previewModel{
		session:    session,
		enumerator: enumerator,
		reader:     reader,
		attacher:   attacher,
		groups:     groups,
		windowIdx:  0,
		paneIdx:    0,
		// bubbles v2 viewport.New takes functional options rather than the v1
		// positional (width, height). Semantically identical: the viewport is
		// constructed at the same inner dimensions.
		viewport: viewport.New(viewport.WithWidth(innerW), viewport.WithHeight(innerH)),
		width:    width,
		height:   height,
	}

	// Single dispatcher shared with the window/pane cycle handlers (←/→, Tab)
	// so the three (bytes, err) shapes from ScrollbackReader.Tail are translated to viewport
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
// window with one pane. In that shape ←/→ (window) and Tab (pane) silently
// no-op; callers can also use this to suppress structural chrome that would
// otherwise be trivial ("Window 1/1 · Pane 1/1").
func (m previewModel) degenerate() bool {
	return len(m.groups) == 1 && len(m.groups[0].PaneIndices) == 1
}

// innerWidth returns the body width available inside the §9.1 joined panel —
// the model's total width minus previewFrameOverhead (2 side borders + the
// 2·helpRowInset per-row inset), clamped to ≥ 0. It is the joined panel's
// contentWidth: the viewport, the header cascade target, and the footer fit
// target all key off it so the composed panel fills the full terminal width.
func (m previewModel) innerWidth() int {
	return max(0, m.width-previewFrameOverhead)
}

// innerHeight returns the body height available inside the §9.1 joined panel —
// the model's total height minus previewChromeRowOverhead (top + header +
// 2 dividers + footer + bottom = 6 chrome rows), clamped to ≥ 0. Peer of
// innerWidth; sizing the viewport to it makes the body fill the available
// height so the footer sits flush at the bottom of the terminal.
func (m previewModel) innerHeight() int {
	return max(0, m.height-previewChromeRowOverhead)
}

// readFocusedPaneIntoViewport performs the synchronous tail-N read for the
// currently-focused pane and translates the ScrollbackReader.Tail (bytes, err)
// outcome to viewport content, anchored at scroll-tail. Shared by every
// focus-changing branch of Update (←/→ window, Tab pane) AND by
// NewPreviewModel's initial-open path, so the three observable shapes are
// dispatched in exactly one place per § Architecture Summary > Test seams >
// ScrollbackReader return contract:
//
//   - (bytes != nil, _) — bytes rendered verbatim.
//   - (nil, nil) — placeholder ("(no saved content)") rendered. Collapses
//     ENOENT, zero-byte .bin, and zero-line file (only an unterminated
//     partial) into one shape.
//   - (nil, err != nil) — OS-level read failure. Renders the canonical
//     error string ("(unable to read scrollback)") uniformly across errno
//     types per § Read-Failure Handling > Placeholder > Error string. No
//     per-pane error state is cached on the model; refocusing the same
//     pane (cycle away and back) re-issues a fresh Tail call through
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

// Update routes Esc/Space to a synthesised previewDismissedMsg, intercepts
// Home / End for preview-owned top/bottom jumps, intercepts the §9.3 spatial
// nav keys (`←`/`→` window, `Tab` next pane — REPLACING the former
// `]`/`[` window + `Ctrl+←`/`Ctrl+→` pane) BEFORE delegating to the viewport
// (which binds plain `←`/`→` for horizontal scroll, so window nav must win, and
// would otherwise swallow `Tab`), intercepts Enter to
// dispatch the pre-select pipeline against the walked (window, pane)
// coordinates (the connector handoff runs post-TUI in cmd/open.go's
// processTUIResult), and absorbs tea.WindowSizeMsg to resize the embedded
// viewport in place. All other messages — including the remaining viewport
// scroll keys (Up, Down, PgUp, PgDn, ctrl-u, ctrl-d, j, k, and h/l horizontal
// scroll) — delegate to bubbles/viewport so its native keymap and resize-clamp
// behaviour are preserved.
//
// reader.Tail is intentionally NOT called for scroll/resize: those operate on
// the already-loaded N-line buffer per § Refresh Semantics (resize is not a
// read trigger; viewport-internal scroll does not re-read). The window/pane
// cycle IS a read trigger — each lands a single synchronous Tail for the newly
// focused pane.
func (m previewModel) Update(msg tea.Msg) (previewModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// bubbles v2 exposes viewport.SetWidth / SetHeight (the spec's
		// `viewport.SetSize(W, H)` — the v1.0.0 TODO that this upgrade
		// resolves). Switching from the v1 direct field assignment to the
		// methods engages the viewport's YOffset auto-clamping uniformly;
		// the preview's observable resize contract is unchanged because cycle
		// handlers re-anchor via GotoBottom and Home/End jumps are explicit.
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.SetWidth(m.innerWidth())
		m.viewport.SetHeight(m.innerHeight())
		return m, nil
	case tea.KeyPressMsg:
		if handled, next, cmd := m.handlePreviewKey(msg); handled {
			return next, cmd
		}
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// handlePreviewKey routes the preview-owned key bindings. It returns
// handled=false (and a zero model/cmd) when the key is not preview-owned, so the
// caller delegates it to the embedded viewport. The §9.3 spatial nav keys
// (`←`/`→` window, `Tab` next pane) are matched here — BEFORE viewport
// delegation — so the arrows win over the viewport's plain-arrow horizontal
// scroll and Tab is never swallowed by the viewport.
func (m previewModel) handlePreviewKey(msg tea.KeyPressMsg) (handled bool, next previewModel, cmd tea.Cmd) {
	switch {
	// Esc / Space — back to the Sessions list.
	case keyIsCode(msg, tea.KeyEscape), keyIsCode(msg, tea.KeySpace):
		return true, m, func() tea.Msg { return previewDismissedMsg{} }
	// Home / End — preview-owned top/bottom jumps (viewport.DefaultKeyMap binds
	// neither), satisfying the acceptance criterion that these jump within the
	// loaded buffer.
	case keyIsCode(msg, tea.KeyHome):
		m.viewport.GotoTop()
		return true, m, nil
	case keyIsCode(msg, tea.KeyEnd):
		m.viewport.GotoBottom()
		return true, m, nil
	// Enter commits an attach to the previewed session, honouring any (window,
	// pane) focus walked via the spatial nav keys. Intercepting before viewport
	// delegation keeps any future viewport Enter binding from leaking through.
	// The pipeline receives raw tmux WindowIndex / PaneIndices (not 0-based slice
	// positions) via currentRawIndices, so non-contiguous indices and non-zero
	// pane-base-index sessions address the right tmux target. Dispatch is
	// unconditional regardless of viewport content state (real bytes,
	// "(no saved content)" placeholder, or OS read error) per spec § Other edge
	// cases > Mid-load. nil attacher is a defensive silent no-op so non-attach-
	// wired test callsites can construct previewModel without nil-panicking.
	case keyIsCode(msg, tea.KeyEnter):
		if m.attacher == nil {
			return true, m, nil
		}
		windowIndex, paneIndex := m.currentRawIndices()
		return true, m, m.attacher.Run(m.session, windowIndex, paneIndex)
	// Tab — next pane (forward cycle, wrapping). REPLACES the former
	// Ctrl+←/Ctrl+→ pane pair (Ctrl+←/→ is hijacked by macOS Mission Control
	// Spaces switching). Matched here — before viewport delegation — so the
	// embedded viewport never swallows Tab. Degenerate single-pane windows are a
	// silent no-op (cyclePane guards paneCount <= 1).
	case keyIsCode(msg, tea.KeyTab):
		return true, m.cyclePane(+1), nil
	// ← / → — prev / next window (REPLACES the former ]/[). Intercepted before
	// the viewport sees them so window nav wins over horizontal scroll.
	case keyIsCode(msg, tea.KeyLeft):
		return true, m.cycleWindow(-1), nil
	case keyIsCode(msg, tea.KeyRight):
		return true, m.cycleWindow(+1), nil
	}
	return false, previewModel{}, nil
}

// cycleWindow moves the focused window by delta (wrapping), resets paneIdx to 0
// (per § Multi-pane Rendering Shape > Pane focus on window cycle — per-window
// pane focus is not retained), and synchronously re-reads the new pane's tail-N
// (§ Refresh Semantics > Read Trigger Events). A single-window session is a
// silent no-op (no read, no index change) — the window keys iterate windows, not
// panes.
func (m previewModel) cycleWindow(delta int) previewModel {
	if len(m.groups) <= 1 {
		return m
	}
	m.windowIdx = (m.windowIdx + delta + len(m.groups)) % len(m.groups)
	m.paneIdx = 0
	m.viewport = m.readFocusedPaneIntoViewport()
	return m
}

// cyclePane moves the focused pane by delta (wrapping) within the current
// window and synchronously re-reads the new pane's tail-N. A single-pane window
// is a silent no-op (no read, no index change).
func (m previewModel) cyclePane(delta int) previewModel {
	paneCount := len(m.currentGroup().PaneIndices)
	if paneCount <= 1 {
		return m
	}
	m.paneIdx = (m.paneIdx + delta + paneCount) % paneCount
	m.viewport = m.readFocusedPaneIntoViewport()
	return m
}

// View renders the §9.1 full-screen "peek mode" preview as a single-tone
// accent.cyan joined panel (the same hand-drawn rounded shape as the modals,
// via renderJoinedPanel) with THREE compartments:
//
//   - Header: `◉ preview` (accent.cyan) + session (text.primary) +
//     `Window x/y · Pane x/y` (text.detail), width-cascaded to fit.
//   - Body: the untouched captured ANSI scrollback (§9.2) — passed through
//     injectSGRResets so unterminated SGR sequences cannot bleed into the right
//     border, and NEVER themed. The viewport is sized to innerHeight so the body
//     fills the available height (footer flush at the bottom).
//   - Footer: the §9.3 nav hints — glyphs accent.blue, labels text.detail,
//     space-separated (`←→ window  ⇥ pane  ⏎ attach  ␣ back`).
//
// The border AND the two compartment dividers all render in accent.cyan (the
// "peek mode" hue, threaded into renderJoinedPanel as the border token). The
// chrome is recomposed every tick (no cached field) so resize and window/pane
// navigation propagate without cache invalidation. contentWidth is innerWidth()
// (the body width) so the composed panel fills the full terminal width.
func (m previewModel) View() string {
	contentWidth := m.innerWidth()

	header := composePreviewHeaderRow(
		contentWidth,
		m.windowIdx, len(m.groups),
		m.paneIdx, len(m.currentGroup().PaneIndices),
		m.session,
		m.mode, m.colourless,
	)
	footer := composePreviewFooterRow(contentWidth, m.mode, m.colourless)

	// The body rows are the untouched captured ANSI — split per line so each is a
	// compartment row (renderJoinedPanel insets + side-borders each). injectSGRResets
	// terminates every non-empty line so no SGR bleeds past it into the cyan border.
	body := strings.Split(injectSGRResets(m.viewport.View()), "\n")

	return renderJoinedPanel(
		[][]string{{header}, body, {footer}},
		previewBorderColorToken,
		m.mode, m.colourless,
	)
}
