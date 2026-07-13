package tui

import (
	"fmt"
	"io"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui/theme"
)

var (
	// nameBase carries the session name's NON-colour attribute (bold); the
	// delegate layers text.primary + Background(canvas) for the resolved mode
	// (SessionDelegate.tokenStyle) so the colour pair is mode-matched.
	nameBase = lipgloss.NewStyle().Bold(true)
)

const (
	// selectorBar is the §3.3 thick block selection glyph (U+258C LEFT HALF
	// BLOCK), the single, consistent left-bar selection signal. It renders in
	// accent.violet at the far-left of the selected row; the leftBarColumnWidth
	// column it occupies replaces the former "> " / "  " cursor prefix so the name
	// keeps the same left edge whether or not the row is selected.
	selectorBar = "▌"
	// multiSelectMarker is the §5 multi-select selection glyph (U+25CF BLACK
	// CIRCLE — the SAME bullet the attached badge uses). It renders in
	// accent.violet at the far-left of a MARKED row, occupying the same fixed
	// leftBarColumnWidth column the ▌ selector would and taking PRECEDENCE over it
	// (a selected+marked cursor row shows ●, not ▌).
	multiSelectMarker = "●"
	// leftBarColumnWidth is the fixed 2-cell left-bar column (§3.3 "full 2-cell
	// column"): the selector glyph at col 0 plus one trailing cell, so the name
	// always starts two cells in. Unselected rows render two blank cells here,
	// preserving the column alignment.
	leftBarColumnWidth = 2
	// nameGap is the canvas-painted gap between the flexing name column and the
	// first fixed trailing slot (the window count), so the name never abuts the
	// count text.
	nameGap = 2
	// countSlotWidth is the FIXED width of the window-count trailing slot (§4.1).
	// The count text ("N window" / "N windows") is left-aligned within it; the
	// fixed width is what keeps the counts — and the attached bullets to their
	// right — vertically column-aligned regardless of name length. 11 fits up to
	// "999 windows" (11 cells) without bleeding into the attached slot.
	countSlotWidth = 11
	// rowRightMargin insets the trailing columns from the content's right edge so the
	// attached bullet does not sit flush against the edge. It is SYMMETRICAL with the
	// names' left inset: the name column starts at leftBarColumnWidth (2 cells, after
	// the selector-bar column), so a 2-cell right margin mirrors it — the row content
	// (name … trailing slots) sits in a symmetric 2-cell-inset band. (The design
	// insets further, but a wide right margin reads as oversized in the terminal;
	// matching the left edge is the cleaner, balanced choice.)
	rowRightMargin = leftBarColumnWidth
)

// attachedMarker is the §4.1 attached badge text. Its width fixes the attached
// trailing slot so an unattached row renders an empty slot of the SAME width,
// keeping the bullets column-aligned down the list.
const attachedMarker = "● attached"

// attachedSlotWidth is the FIXED width of the attached trailing slot, derived
// from the marker text so the empty (unattached) slot matches it exactly.
var attachedSlotWidth = lipgloss.Width(attachedMarker)

// groupSeparator is the heading glyph between the group label and its count,
// rendered as "Heading ··· N" (U+00B7 MIDDLE DOT ×3) per the spec examples
// (Portal ··· 2, Untagged ··· 3).
const groupSeparator = "···"

const (
	// groupHeaderIndent indents a group header's text to col 2 — the title-box
	// left edge / the flat-name column (§5.1) — so the heading reads as a section
	// label above its rows rather than sitting flush against the left edge.
	groupHeaderIndent = "  "
	// groupRowIndent nests a grouped session row ONE indent level further than
	// flat (§5.1): rendered BEFORE the 2-cell left-bar column, it shifts the whole
	// row right so the cursor/selector ▌ lands at col 2 (aligned with the header
	// text) and the name at col 4 — the rows read as indented children of the
	// heading. Flat rows (empty GroupKey) skip it and render flush (name at col 2).
	groupRowIndent = "  "
)

// windowLabel returns a formatted window count with correct pluralization.
func windowLabel(count int) string {
	if count == 1 {
		return "1 window"
	}
	return fmt.Sprintf("%d windows", count)
}

// SessionItem wraps a tmux.Session and implements the list.Item interface
// for use with bubbles/list.
//
// Group metadata (GroupKey, GroupHeading, CatchAll) is build-layer information
// the grouping builders use to sort sessions and to emit a HeaderItem at each
// group boundary; it lets By-Tag materialise a multi-tag session as several
// instances. All three are zero-valued for Flat items, so a flat item is
// byte-for-byte identical to a metadata-free item.
//
// Two SessionItems that share the same Session but differ in GroupKey are
// independently selectable views of one session — not distinct attach targets.
// Selection and attach key on Session.Name (task 2-6), so every view of a
// session resolves to the same underlying target.
type SessionItem struct {
	Session tmux.Session

	// GroupKey is the canonical sort/boundary key: the canonical directory
	// path for By Project, the canonical tag for By Tag, empty for Flat.
	GroupKey string

	// GroupHeading is the dimmed label shown at a group boundary: the project
	// name, tag value, or Unknown / Untagged; empty for Flat.
	GroupHeading string

	// CatchAll marks an Unknown (By Project) or Untagged (By Tag) bucket
	// instance, pinning it last without string-matching the heading.
	CatchAll bool
}

// FilterValue returns the session name for filtering. It is the only method
// bubbles/list consumes off the item (list.Item); the session name, window
// count, and attached marker are produced solely by the delegate's live render
// path (SessionDelegate.renderSessionRow), where the marker text flows from the
// single attachedMarker const.
func (i SessionItem) FilterValue() string {
	return i.Session.Name
}

// HeaderItem is a group heading rendered as a real, non-selectable list row in
// By-Project / By-Tag modes. Making the heading a genuine list.Item (rather
// than a line injected inside a SessionItem's Render) is load-bearing: every
// rendered row is then exactly one item of delegate Height 1, so bubbles/list's
// pagination is exact and the page can never render more lines than the
// viewport (the overflow that previously scrolled the title and the cursor off
// the top — see model.go ensureSessionRowSelected and skipHeaderRow for the
// cursor-skip that keeps the selection on session rows).
//
// FilterValue is empty so the built-in filter excludes headers the moment a
// query is typed — the list then renders a flat, relevance-sorted hit list of
// session rows only (flatten-on-filter), with no header rebuild needed.
type HeaderItem struct {
	// Heading is the display label (project name, tag value, or the
	// Unknown / Untagged catch-all label).
	Heading string
	// Count is the number of session rows that follow this header within its
	// group, precomputed at build time and shown as "Heading ··· N".
	Count int
	// Key is the group's canonical key (canonical path / tag / catch-all
	// label). Retained for tests and potential future use.
	Key string
}

// FilterValue returns "" so the built-in filter never matches a header — they
// vanish during filtering, giving a flat hit list for free.
func (HeaderItem) FilterValue() string { return "" }

// headingText is the group label run: the heading word, plus a trailing space so
// the dots-count run abuts it with a single gap. Rendered in text.detail (§5.1).
func (h HeaderItem) headingText() string {
	return h.Heading + " "
}

// countText is the dots-count run: "··· N" (the §5.1 `··· N` count). Rendered in
// text.dim (dimmer than the heading) as a SEPARATE run, so the count reads as a
// quieter tally beside the heading rather than one uniform faint separator.
func (h HeaderItem) countText() string {
	return fmt.Sprintf("%s %d", groupSeparator, h.Count)
}

// SessionDelegate implements list.ItemDelegate for rendering session items.
//
// Mode is the resolved canvas appearance (§1): every run the delegate emits —
// cursor, name, the structural spacers, the window count, the attached marker,
// and the dimmed group heading — is painted with the §2.9 role-token FOREGROUND
// resolved for this Mode over a Background(canvas) for this Mode. So a content
// row both reads correctly (the light variants on the light canvas, the dark
// variants on the dark canvas) and carries the canvas colour on every cell (no
// terminal-bg islands behind the styled text). The OUTER fill in View() then
// pads each line-end and fills the empty rows. The zero value is theme.Dark, so
// a bare SessionDelegate{} (the value used across the existing unit tests) paints
// the dark canvas it was tuned for. New sets it from the model's resolved
// canvasMode after the options apply.
type SessionDelegate struct {
	Mode theme.Mode
	// Colourless is the NO_COLOR carve-out (§2.5): when set, the delegate paints NO
	// canvas background and NO foreground hue — every run renders on the terminal's
	// native fg/bg. The row TEXT and column structure are unchanged, so state stays
	// glyph-distinct (§2.2: ● attached, ▌ selector). Foreground hue would be
	// stripped by the writer layer anyway; suppressing the canvas background here is
	// the work (lipgloss would otherwise still emit it). It is set from the model's
	// single colourless flag (applyCanvasMode), so the delegate inherits the one
	// carve-out decision rather than re-deriving NO_COLOR.
	Colourless bool
	// MultiSelect gates the §5 selection ● marker: only while it is set does a
	// marked row render the violet ● in the left-bar column. It is set from the
	// model's multiSelectMode (applyCanvasMode), so the marker appears exactly
	// while the Sessions page is in multi-select mode.
	MultiSelect bool
	// Selected is the marked set, keyed on Session.Name — the SAME identity the
	// attach / selectedSessionItem path uses, so a multi-tag By-Tag session marked
	// once shows the ● on every one of its rows. It is a live reference to the
	// model's selectedSessions map (re-pointed on every toggle), nil-tolerant: a
	// nil set marks nothing (isSelected).
	Selected map[string]struct{}
}

// isSelected reports whether name is in the marked set. It is nil-safe: a nil set
// (multi-select mode never entered, or exited) marks nothing.
func isSelected(set map[string]struct{}, name string) bool {
	_, ok := set[name]
	return ok
}

// canvasBg is the structural-spacer style: the Background(canvas) for the
// delegate's mode normally, or a bare style under the NO_COLOR carve-out (§2.5)
// so the spacers render on the terminal's native bg with no canvas paint. It
// delegates to the shared header.go source (headerCanvasBg) rather than
// re-implementing the rule, so the leaf canvas-paint carve-out lives in exactly
// one place (mirroring how rowBg delegates to the shared rowBgStyle free
// function and loadingStyle delegates to headerCanvasBg).
func (d SessionDelegate) canvasBg() lipgloss.Style {
	return headerCanvasBg(d.Mode, d.Colourless)
}

// tokenStyle returns base with the role token's mode-resolved FOREGROUND and the
// mode-resolved Background(canvas) applied — the leaf paint of one run: correct
// foreground for the resolved mode, sitting on the owned canvas. base carries
// the non-colour attributes (Bold for the name); a zero base is fine for runs
// that only need the colour pair.
//
// Under the NO_COLOR carve-out (§2.5) it returns base unchanged — no foreground
// hue and no canvas background — so the run renders on the terminal's native
// fg/bg, keeping base's non-colour attributes (Bold/dim) which carry state
// glyph-distinctly (§2.2) without colour.
//
// It delegates the leaf colour pair to the shared header.go source (headerStyle)
// and composites the caller-supplied base via Inherit, so the colour pair carries
// (it is set on the header style and wins) while base's non-colour attributes are
// inherited where the header style leaves them unset. Under NO_COLOR headerStyle
// returns a bare style, so the result is base with no colour applied. This keeps
// the leaf token-over-canvas carve-out in exactly one place (mirroring how
// rowToken delegates to rowTokenStyle and loadingFg delegates to headerStyle).
func (d SessionDelegate) tokenStyle(base lipgloss.Style, fg theme.Token) lipgloss.Style {
	return headerStyle(fg, d.Mode, d.Colourless).Inherit(base)
}

// Height returns 1, matching the single-line item display. Both SessionItem and
// HeaderItem render as exactly one line, so a uniform Height of 1 makes
// bubbles/list pagination exact.
func (d SessionDelegate) Height() int { return 1 }

// Spacing returns 0, no gap between rows. A terminal renders in whole character
// cells, so the only "airier" option is a FULL blank line between rows (Spacing 1),
// which reads as too much and halves the rows-per-screen — there is no half-row in a
// terminal. The design's padded selection band comes from its 32px rows (a
// cell-height property the terminal owns, not the app), so the snug 1-line band is
// the terminal-faithful floor.
func (d SessionDelegate) Spacing() int { return 0 }

// Update returns nil; no item-level keybinding handling is needed.
func (d SessionDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

// Render renders one list row. A HeaderItem renders as a dimmed
// "Heading ··· N" separator (no cursor, never selectable — the cursor-skip in
// model.go guarantees the selection never rests on a header). A SessionItem
// renders the §4.1 flat-row anatomy: a 2-cell left-bar column (a violet ▌ on the
// selected row, two blank cells otherwise), the name as a flexing left column,
// then fixed-width right-pinned trailing slots for the window count and the
// attached marker. Every run is painted with its §2.9 role-token foreground and
// the owned canvas background for the delegate's Mode (the leaf layer of §1); on
// the selected row the structural cells carry the bg.selection tint instead.
func (d SessionDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	bg := d.canvasBg()
	var row string
	switch it := item.(type) {
	case HeaderItem:
		// Group heading (§5.1): TWO separately-styled runs over the owned canvas —
		// the heading word in text.detail (the §2.9 group-heading role) and the
		// "··· N" count in text.dim (dimmer), so the count reads as a quieter tally
		// rather than one uniform faint separator. No Faint(true) and no literal
		// hex at the call site — both colours flow from §2.9 tokens. The same style
		// renders a catch-all heading (Unknown / Untagged): they are HeaderItems too.
		heading := d.tokenStyle(lipgloss.Style{}, theme.MV.TextDetail).Render(it.headingText())
		count := d.tokenStyle(lipgloss.Style{}, theme.MV.TextDim).Render(it.countText())
		row = bg.Render(groupHeaderIndent) + heading + count
	case SessionItem:
		row = d.renderSessionRow(m, index, it)
	default:
		return
	}

	// The row is composed entirely of canvas-backgrounded runs (no bare spaces),
	// so its own cells are canvas. bubbles/list may block-pad this row to its
	// widest sibling with raw, background-less trailing spaces; the outer canvas
	// fill (model.fillCanvas) strips those and re-pads every line to the terminal
	// width, so the trailing region is canvas too. The delegate therefore emits
	// only the row's own content and leaves width-padding to the single fill.
	_, _ = fmt.Fprint(w, row)
}

// rowBgStyle is the SHARED structural-cell style for a list row (Session and
// Project delegates both route through it): the bg.selection tint on the
// selected row, otherwise the owned canvas (or a bare style under the NO_COLOR
// carve-out, so the cells render on the terminal's native bg). Homed here as a
// free function so the §2.9 selection-vs-canvas colour-role decision lives in
// exactly one place — a future change to the selection background role is a
// single edit shared by both delegates.
//
// Padding (slot fills, the name-flex tail, the gap) is rendered through this
// style so every structural cell carries an explicit background — no
// terminal-bg island opens up inside a selected row's tint or the canvas.
func rowBgStyle(mode theme.Mode, selected, colourless bool) lipgloss.Style {
	if colourless {
		return lipgloss.NewStyle()
	}
	if selected {
		return lipgloss.NewStyle().Background(theme.MV.BgSelection.ColorFor(mode))
	}
	return lipgloss.NewStyle().Background(theme.MV.Canvas.ColorFor(mode))
}

// rowTokenStyle is the SHARED selected-row token style (both delegates route
// through it): base with the role token's mode-resolved FOREGROUND over the
// row's background (bg.selection on the selected row, canvas otherwise). Under
// the NO_COLOR carve-out it returns base unchanged (no hue, no background), so
// base's non-colour attributes (Bold/Faint) still carry state glyph-distinctly
// (§2.2). Homed here as a free function so the colour-role composition lives in
// one place for both the Session and Project delegates.
func rowTokenStyle(base lipgloss.Style, fg theme.Token, mode theme.Mode, selected, colourless bool) lipgloss.Style {
	if colourless {
		return base
	}
	styled := base.Foreground(fg.ColorFor(mode))
	if selected {
		return styled.Background(theme.MV.BgSelection.ColorFor(mode))
	}
	return styled.Background(theme.MV.Canvas.ColorFor(mode))
}

// renderLeftBarColumn renders the SHARED §3.3 left-bar selector column (both
// renderSessionRow and renderRowLine route through it): the violet ▌ + a
// trailing cell on the selected row, leftBarColumnWidth blank cells otherwise —
// a fixed 2-cell column keeping the row text at the same left edge whether or
// not the row is selected. bg is the caller's rowBgStyle result (so the blank
// cells carry the row's canvas / selection tint); selectorStyle is the caller's
// rowTokenStyle(lipgloss.Style{}, AccentViolet, true, …) result (so the bar
// renders in accent.violet over the selection tint). Homed here so the 2-cell
// selector-column width and the selected/unselected grammar live in one place
// for both delegates.
func renderLeftBarColumn(bg, selectorStyle lipgloss.Style, selected bool) string {
	if selected {
		return selectorStyle.Render(selectorBar) +
			bg.Render(padTo("", leftBarColumnWidth-lipgloss.Width(selectorBar)))
	}
	return bg.Render(padTo("", leftBarColumnWidth))
}

// renderMarkedLeftBarColumn renders the §5 multi-select left-bar column for a
// MARKED row: the violet ● at col 0 + a trailing cell, in the SAME fixed 2-cell
// leftBarColumnWidth geometry as the ▌ selector, so the name keeps its left edge
// and no downstream column shifts (§3.5 / §4.1). It takes PRECEDENCE over
// renderLeftBarColumn's ▌ selector — a selected+marked cursor row shows the ●,
// not the bar. markerStyle is the caller's rowToken(lipgloss.Style{},
// AccentViolet, selected) result, so the ● carries the bg.selection tint on a
// selected row and the canvas otherwise (and drops hue under NO_COLOR); bg is the
// caller's rowBg result, so the trailing cell carries the same tint.
func renderMarkedLeftBarColumn(bg, markerStyle lipgloss.Style) string {
	return markerStyle.Render(multiSelectMarker) +
		bg.Render(padTo("", leftBarColumnWidth-lipgloss.Width(multiSelectMarker)))
}

// rowBg delegates to the shared rowBgStyle free function, binding the
// delegate's Mode and Colourless. Retained so the existing call sites keep their
// terse d.rowBg(selected) form.
func (d SessionDelegate) rowBg(selected bool) lipgloss.Style {
	return rowBgStyle(d.Mode, selected, d.Colourless)
}

// rowToken delegates to the shared rowTokenStyle free function, binding the
// delegate's Mode and Colourless. Retained so the existing call sites keep their
// terse d.rowToken(...) form.
func (d SessionDelegate) rowToken(base lipgloss.Style, fg theme.Token, selected bool) lipgloss.Style {
	return rowTokenStyle(base, fg, d.Mode, selected, d.Colourless)
}

// renderSessionRow renders the §4.1 flat-row anatomy on the owned canvas:
//
//	[grouped indent?][2-cell bar][name flex …][gap][count slot][attached slot]
//
// The trailing slots (count, attached) are FIXED-WIDTH and right-pinned; the
// name flexes to fill the remainder of the row's width so the counts and the
// attached bullets stay vertically column-aligned regardless of name length. An
// over-long name truncates with an ellipsis to the flex width (§2.7) so it can
// never push the trailing slots off-row. Height stays exactly one line — the
// §3.5 / §4.1 one-delegate-line pagination invariant.
func (d SessionDelegate) renderSessionRow(m list.Model, index int, it SessionItem) string {
	selected := index == m.Index()
	// §7.1 input-active clarity: while the filter input is being edited (Filtering)
	// NO list row is selected — the cursor lives in the filter input, not on a row,
	// so the violet ▌ bar and the bg.selection band are suppressed for every row.
	// The engine still tracks an internal cursor index (it disables CursorUp/Down
	// while typing), but the RENDER must show no selected row, so the §7.2 "never
	// both an input cursor AND a selected row" invariant holds. The committed
	// (FilterApplied / list-active) and unfiltered states render the selected row
	// as normal.
	if m.FilterState() == list.Filtering {
		selected = false
	}
	bg := d.rowBg(selected)

	// Grouped rows (GroupKey set in By Project / By Tag — including the Unknown /
	// Untagged catch-alls, which orderedSessionItems stamps with GroupKey = the
	// catch-all heading) nest one indent level FURTHER than flat (§5.1): the indent
	// sits BEFORE the left-bar column, so the cursor/selector ▌ lands at col 2 and
	// the name at col 4. Flat rows (empty GroupKey) render flush — the bar at col 0,
	// the name at col 2. The indent is folded into the width budget below, so it
	// shrinks the flex name rather than pushing the row wide.
	indent := ""
	if it.GroupKey != "" {
		indent = groupRowIndent
	}
	indentCell := bg.Render(indent)

	// Left-bar column (§3.3 / §5): a MARKED multi-select row shows the violet ● at
	// col 0 (taking precedence over the ▌ selector, so a selected+marked cursor row
	// shows ●); otherwise the violet ▌ + a trailing cell on the selected row, two
	// blank cells on an unselected row — the fixed 2-cell column keeps the name at
	// the same left edge in every case. The ● style uses `selected` (not the bar's
	// literal true) so it carries the bg.selection tint on a marked cursor row and
	// the canvas on an unselected marked row (dropping hue under NO_COLOR). Shared
	// with the Project delegate via renderLeftBarColumn for the unmarked case.
	marked := d.MultiSelect && isSelected(d.Selected, it.Session.Name)
	var bar string
	if marked {
		bar = renderMarkedLeftBarColumn(bg, d.rowToken(lipgloss.Style{}, theme.MV.AccentViolet, selected))
	} else {
		bar = renderLeftBarColumn(bg, d.rowToken(lipgloss.Style{}, theme.MV.AccentViolet, true), selected)
	}

	// Name — text.primary (selected: text.on-selection), bold (§4.1).
	nameTok := theme.MV.TextPrimary
	if selected {
		nameTok = theme.MV.TextOnSelection
	}
	// Window count — text.detail (selected: text.strong) (§4.1).
	countTok := theme.MV.TextDetail
	if selected {
		countTok = theme.MV.TextStrong
	}
	countText := windowLabel(it.Session.Windows)

	// The two trailing slots are fixed-width; the name column flexes to whatever is
	// left of the row width after the bar, indent, gap, and the slots. When the
	// list has not been sized yet (Width() == 0, a directly-constructed model that
	// renders before its first WindowSizeMsg) there is no width to flex against, so
	// fall back to a left-to-right flow: full name, single gap, count, attached —
	// no truncation, no right-pinning.
	total := m.Width()
	used := leftBarColumnWidth + lipgloss.Width(indent) + nameGap + countSlotWidth + attachedSlotWidth + rowRightMargin

	var name, namePad string
	if total <= 0 {
		name = d.rowToken(nameBase, nameTok, selected).Render(it.Session.Name)
		namePad = ""
	} else {
		// Truncate to the flex width with an ellipsis (§2.7), then pad the remainder
		// so the gap and the fixed slots are right-pinned and column-aligned.
		nameWidth := max(total-used, 1)
		visibleName := ansi.Truncate(it.Session.Name, nameWidth, "…")
		name = d.rowToken(nameBase, nameTok, selected).Render(visibleName)
		namePad = bg.Render(padTo("", nameWidth-lipgloss.Width(visibleName)))
	}

	gap := bg.Render(padTo("", nameGap))

	// Window count slot — left-aligned text padded to the fixed slot width (§4.1).
	count := d.rowToken(lipgloss.Style{}, countTok, selected).Render(countText) +
		bg.Render(padTo("", countSlotWidth-lipgloss.Width(countText)))

	// Attached marker — a fixed-width slot right of the count. "● attached" in
	// state.green when attached (the single state.green token clears the floor on
	// both the canvas and the bg.selection tint, so the selected row keeps the same
	// green — no per-context override), an EMPTY slot of the SAME width when not, so
	// the bullets and the counts stay column-aligned regardless of name length (§4.1).
	attached := bg.Render(padTo("", attachedSlotWidth))
	if it.Session.Attached {
		attached = d.rowToken(lipgloss.Style{}, theme.MV.StateGreen, selected).Render(attachedMarker) +
			bg.Render(padTo("", attachedSlotWidth-lipgloss.Width(attachedMarker)))
	}

	// The right margin insets the trailing columns from the content edge (§4.1) so
	// the attached bullet does not sit flush against the edge (matching the design).
	rightMargin := bg.Render(padTo("", rowRightMargin))
	row := indentCell + bar + name + namePad + gap + count + attached + rightMargin

	// Safety clamp (§2.7 / §3.5): the trailing slots are a FIXED 25 cells (bar +
	// gap + count + attached + indent); at pathological narrow widths the flex name
	// floors to 1 cell and the assembled row would be ~26 cells regardless of total,
	// overflowing the list width and bleeding past the content gutter (corrupting the
	// inset frame). Truncate the assembled row to total as a final guard. This is a
	// no-op on the happy path — the row is already exactly total cells there — and
	// engages only when total < ~26, so the row can never exceed the list width.
	if total > 0 {
		row = ansi.Truncate(row, total, "…")
	}
	return row
}

// padTo returns s padded on the right with spaces to exactly n cells (or s
// unchanged when it already meets/exceeds n). A non-positive n yields the empty
// string. Rendered through a background style by the caller, the spaces carry
// the row's canvas / selection tint so no terminal-bg island opens in a slot.
func padTo(s string, n int) string {
	w := lipgloss.Width(s)
	if n <= w {
		return s
	}
	return s + spaces(n-w)
}

// spaces returns a string of n spaces (n<=0 → "").
func spaces(n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, n)
	for i := range b {
		b[i] = ' '
	}
	return string(b)
}

// ToListItems converts a slice of tmux sessions to a slice of list.Item.
func ToListItems(sessions []tmux.Session) []list.Item {
	items := make([]list.Item, len(sessions))
	for i, s := range sessions {
		items[i] = SessionItem{Session: s}
	}
	return items
}
