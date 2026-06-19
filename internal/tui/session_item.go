package tui

import (
	"fmt"
	"io"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui/theme"
)

var (
	// cursorStyle marks the cursor/selection — the primary accent role
	// (accent.violet); the former pink ANSI 212 was a scattered literal. Retained
	// for the projects page (project_item.go), which still resolves the dark
	// default until its own canvas restyle lands in a later phase; the sessions
	// delegate resolves accent.violet per mode inline (see Render).
	cursorStyle = lipgloss.NewStyle().Foreground(theme.MV.AccentViolet.Color())
	// nameBase carries the session name's NON-colour attribute (bold); the
	// delegate layers text.primary + Background(canvas) for the resolved mode
	// (SessionDelegate.tokenStyle) so the colour pair is mode-matched.
	nameBase = lipgloss.NewStyle().Bold(true)
	// headingBase carries the group heading's NON-colour attribute (faint, so it
	// reads as a dimmed separator); the delegate layers text.detail +
	// Background(canvas) for the resolved mode.
	headingBase = lipgloss.NewStyle().Faint(true)
)

// groupSeparator is the heading glyph between the group label and its count,
// rendered as "Heading ··· N" (U+00B7 MIDDLE DOT ×3) per the spec examples
// (Portal ··· 2, Untagged ··· 3).
const groupSeparator = "···"

const (
	// groupHeaderIndent aligns a group header's text with the list title box.
	// bubbles/list's default TitleBar has PaddingLeft = 2, so the purple
	// "Sessions" title starts at column 2; headers indent to match rather than
	// sitting flush against the left edge.
	groupHeaderIndent = "  "
	// groupRowIndent nests a grouped session row one level under its header, so
	// the rows read as indented children of the group heading (cursor aligned
	// with the header text, name two columns further in).
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

// FilterValue returns the session name for filtering.
func (i SessionItem) FilterValue() string {
	return i.Session.Name
}

// Title returns the session name for display.
func (i SessionItem) Title() string {
	return i.Session.Name
}

// Description returns the window count with correct pluralization
// and the attached badge if the session is attached.
func (i SessionItem) Description() string {
	label := windowLabel(i.Session.Windows)

	if i.Session.Attached {
		return label + "  ● attached"
	}

	return label
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

// label renders the dimmed "Heading ··· N" separator text.
func (h HeaderItem) label() string {
	return fmt.Sprintf("%s %s %d", h.Heading, groupSeparator, h.Count)
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
}

// canvasBg is the bare Background(canvas) style for the delegate's mode, used to
// paint the structural spacers (no foreground run) so every cell of a content
// row is the canvas colour.
func (d SessionDelegate) canvasBg() lipgloss.Style {
	return lipgloss.NewStyle().Background(theme.MV.Canvas.ColorFor(d.Mode))
}

// tokenStyle returns base with the role token's mode-resolved FOREGROUND and the
// mode-resolved Background(canvas) applied — the leaf paint of one run: correct
// foreground for the resolved mode, sitting on the owned canvas. base carries
// the non-colour attributes (Bold for the name); a zero base is fine for runs
// that only need the colour pair.
func (d SessionDelegate) tokenStyle(base lipgloss.Style, fg theme.Token) lipgloss.Style {
	return base.
		Foreground(fg.ColorFor(d.Mode)).
		Background(theme.MV.Canvas.ColorFor(d.Mode))
}

// Height returns 1, matching the single-line item display. Both SessionItem and
// HeaderItem render as exactly one line, so a uniform Height of 1 makes
// bubbles/list pagination exact.
func (d SessionDelegate) Height() int { return 1 }

// Spacing returns 0, no gap between items.
func (d SessionDelegate) Spacing() int { return 0 }

// Update returns nil; no item-level keybinding handling is needed.
func (d SessionDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

// Render renders one list row. A HeaderItem renders as a dimmed
// "Heading ··· N" separator (no cursor, never selectable — the cursor-skip in
// model.go guarantees the selection never rests on a header). A SessionItem
// renders the cursor indicator, styled name, dimmed window count, and green
// attached badge. Every run is painted with its §2.9 role-token foreground and
// the owned canvas background for the delegate's Mode (the leaf layer of §1):
// the row TEXT and column structure are unchanged from the pre-canvas delegate,
// only the colour pair is now mode-matched and canvas-backed.
func (d SessionDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	bg := d.canvasBg()
	var row string
	switch it := item.(type) {
	case HeaderItem:
		// Group heading: the §2.9 group-heading role (text.detail), dimmed so it
		// reads as a separator rather than a selectable row.
		heading := d.tokenStyle(headingBase, theme.MV.TextDetail).Render(it.label())
		row = bg.Render(groupHeaderIndent) + heading
	case SessionItem:
		cursor := bg.Render("  ")
		if index == m.Index() {
			// Cursor / selector — accent.violet (§2.9).
			cursor = d.tokenStyle(lipgloss.Style{}, theme.MV.AccentViolet).Render("> ")
		}

		// Name — text.primary, bold (§4.1).
		name := d.tokenStyle(nameBase, theme.MV.TextPrimary).Render(it.Session.Name)

		// Window count — text.detail (§4.1).
		detail := d.tokenStyle(lipgloss.Style{}, theme.MV.TextDetail).Render(windowLabel(it.Session.Windows))
		if it.Session.Attached {
			// Attached marker — state.green (§4.1).
			detail += bg.Render("  ") + d.tokenStyle(lipgloss.Style{}, theme.MV.StateGreen).Render("● attached")
		}

		// Grouped rows (GroupKey set in By Project / By Tag) nest under their
		// header; Flat rows (empty GroupKey) render flush as before.
		indent := ""
		if it.GroupKey != "" {
			indent = groupRowIndent
		}

		row = fmt.Sprintf("%s%s%s%s%s", bg.Render(indent), cursor, name, bg.Render("  "), detail)
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

// ToListItems converts a slice of tmux sessions to a slice of list.Item.
func ToListItems(sessions []tmux.Session) []list.Item {
	items := make([]list.Item, len(sessions))
	for i, s := range sessions {
		items[i] = SessionItem{Session: s}
	}
	return items
}
