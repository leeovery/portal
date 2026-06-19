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
	// (accent.violet); the former pink ANSI 212 was a scattered literal.
	cursorStyle = lipgloss.NewStyle().Foreground(theme.MV.AccentViolet.Color())
	nameStyle   = lipgloss.NewStyle().Bold(true)
	// detailStyle paints functional metadata (the window count) — this is
	// functional text, so it maps to text.detail, NOT decorative text.faint.
	detailStyle = lipgloss.NewStyle().Foreground(theme.MV.TextDetail.Color())
	// attachedStyle paints the "● attached" live marker — the one positive/live
	// signal, so state.green; reserved for live/positive, never a chip/decoration.
	attachedStyle = lipgloss.NewStyle().Foreground(theme.MV.StateGreen.Color())
	// headingStyle dims the group heading so it reads as a separator rather
	// than a selectable row. Layered alongside the existing delegate styles
	// per spec § Group headers (dimmed).
	headingStyle = lipgloss.NewStyle().Faint(true)
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
type SessionDelegate struct{}

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
// attached badge. Flat items (HeaderItem absent from the slice entirely) render
// byte-identically to the pre-grouping delegate.
func (d SessionDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	switch it := item.(type) {
	case HeaderItem:
		_, _ = fmt.Fprint(w, groupHeaderIndent+headingStyle.Render(it.label()))
	case SessionItem:
		cursor := "  "
		if index == m.Index() {
			cursor = cursorStyle.Render("> ")
		}

		name := nameStyle.Render(it.Session.Name)

		detail := detailStyle.Render(windowLabel(it.Session.Windows))
		if it.Session.Attached {
			detail += "  " + attachedStyle.Render("● attached")
		}

		// Grouped rows (GroupKey set in By Project / By Tag) nest under their
		// header; Flat rows (empty GroupKey) render flush as before.
		indent := ""
		if it.GroupKey != "" {
			indent = groupRowIndent
		}

		_, _ = fmt.Fprintf(w, "%s%s%s  %s", indent, cursor, name, detail)
	}
}

// ToListItems converts a slice of tmux sessions to a slice of list.Item.
func ToListItems(sessions []tmux.Session) []list.Item {
	items := make([]list.Item, len(sessions))
	for i, s := range sessions {
		items[i] = SessionItem{Session: s}
	}
	return items
}
