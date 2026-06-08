package tui

import (
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/leeovery/portal/internal/tmux"
)

var (
	cursorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
	nameStyle     = lipgloss.NewStyle().Bold(true)
	detailStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#777777"))
	attachedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("76"))
	// headingStyle dims the injected group heading so it reads as a separator
	// rather than a selectable row. Layered alongside the existing delegate
	// styles per spec § Group headers (dimmed).
	headingStyle = lipgloss.NewStyle().Faint(true)
)

// groupSeparator is the heading glyph between the group label and its count,
// rendered as "Heading ··· N" (U+00B7 MIDDLE DOT ×3) per the spec examples
// (Portal ··· 2, Untagged ··· 3).
const groupSeparator = "···"

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
// Group metadata (GroupKey, GroupHeading, CatchAll) is render-layer
// information that lets the delegate inject a dimmed heading at a group
// boundary and lets By-Tag materialise a multi-tag session as several
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

// SessionDelegate implements list.ItemDelegate for rendering session items.
type SessionDelegate struct{}

// Height returns 1, matching the single-line item display.
func (d SessionDelegate) Height() int { return 1 }

// Spacing returns 0, no gap between items.
func (d SessionDelegate) Spacing() int { return 0 }

// Update returns nil; no item-level keybinding handling is needed.
func (d SessionDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

// Render renders a session item with cursor indicator, styled name,
// dimmed window count, and green attached badge.
//
// When the item begins a new group, Render prepends a dimmed group heading
// line ("Heading ··· N") as a pure render-layer separator — never a list item,
// so the cursor and selection (which index into m.Items()) are unaffected. A
// new group is detected by comparing this item's GroupKey against the previous
// list item's; the leading item always starts a group. Flat items (empty
// GroupKey) inject no heading, leaving the output byte-identical to the
// pre-grouping delegate.
//
// Height/Spacing tradeoff: the heading is drawn as an extra prefixed line
// within this single Render write, but Height() stays 1 (bubbles/list measures
// pagination by Height()). The extra heading lines are therefore drawn but not
// counted, accepting minor pagination imprecision in v1 rather than routing the
// picker through lipgloss/tree (build constraint) or per-item variable height.
func (d SessionDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	si, ok := item.(SessionItem)
	if !ok {
		return
	}

	if heading, ok := groupHeading(m, index, si); ok {
		_, _ = fmt.Fprintln(w, heading)
	}

	isSelected := index == m.Index()

	cursor := "  "
	if isSelected {
		cursor = cursorStyle.Render("> ")
	}

	name := nameStyle.Render(si.Session.Name)

	detail := detailStyle.Render(windowLabel(si.Session.Windows))

	if si.Session.Attached {
		detail += "  " + attachedStyle.Render("● attached")
	}

	line := fmt.Sprintf("%s%s  %s", cursor, name, detail)
	_, _ = fmt.Fprint(w, line)
}

// groupHeading returns the dimmed "Heading ··· N" separator to render above the
// item at index, plus true, when that item begins a new group. It returns
// ("", false) for Flat items (empty GroupKey) and for interior items of a group.
//
// A new group is detected by comparing si.GroupKey against the previous list
// item's GroupKey; index 0 always begins a group. Boundary detection keys on
// GroupKey, while the heading label is taken from si.GroupHeading (the display
// label, e.g. the project name or tag value).
func groupHeading(m list.Model, index int, si SessionItem) (string, bool) {
	// Flatten-on-filter is realised at this single point: while the built-in
	// filter is active (typing → list.Filtering, committed → list.FilterApplied)
	// we suppress all heading injection, so the filter's relevance-sorted result
	// renders as a plain flat hit-list. This relies on the render-layer invariant
	// that headers are never list items (they are drawn inside this delegate, not
	// stored in m.Items()), so the filter only ever sees session instances and no
	// item rebuild is needed. On list.Unfiltered the headings resume and the
	// grouped view restores against the still-grouped underlying slice — which was
	// never un-grouped; only heading drawing was suppressed. In By Tag mode the
	// slice carries one item per (session, tag) pair, so a filtered result may show
	// the same session more than once (one row per matching tag instance) — this is
	// expected in v1, not a defect.
	if m.FilterState() != list.Unfiltered {
		return "", false
	}

	if si.GroupKey == "" {
		return "", false
	}

	if index > 0 {
		if prev, ok := m.Items()[index-1].(SessionItem); ok && prev.GroupKey == si.GroupKey {
			return "", false
		}
	}

	count := groupCount(m.Items(), index, si.GroupKey)
	label := fmt.Sprintf("%s %s %d", si.GroupHeading, groupSeparator, count)
	return headingStyle.Render(label), true
}

// groupCount returns the size of the contiguous run of items sharing groupKey,
// starting at start (the group's first item). The pre-sorted grouped order
// guarantees same-key items are contiguous, so a forward scan is exact. The scan
// is bounded by the run length (~15-20 sessions in practice), so the linear walk
// per group boundary is acceptable.
func groupCount(items []list.Item, start int, groupKey string) int {
	count := 0
	for i := start; i < len(items); i++ {
		si, ok := items[i].(SessionItem)
		if !ok || si.GroupKey != groupKey {
			break
		}
		count++
	}
	return count
}

// ToListItems converts a slice of tmux sessions to a slice of list.Item.
func ToListItems(sessions []tmux.Session) []list.Item {
	items := make([]list.Item, len(sessions))
	for i, s := range sessions {
		items[i] = SessionItem{Session: s}
	}
	return items
}
