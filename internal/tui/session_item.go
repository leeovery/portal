package tui

import (
	"fmt"
	"io"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/resolver"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/tmux"
)

var (
	// Non-colour attributes only — tokenStyle/rowToken layer one token's
	// foreground over the selection-or-canvas background.
	nameBase = lipgloss.NewStyle().Bold(true)
)

const (
	selectorBar       = "▌"
	multiSelectMarker = "●"
	// Fixed 2-cell column (glyph + trailing cell); unselected rows render two
	// blank cells, so the name always starts at the same left edge.
	leftBarColumnWidth = 2
	nameGap            = 2
	// Fixed so counts and attached bullets stay column-aligned; sized to fit
	// "999 windows" without bleeding into the next slot.
	countSlotWidth = 11
	// Mirrors the left inset; a wider right margin reads as oversized.
	rowRightMargin = leftBarColumnWidth
)

// attachedMarker's width fixes the attached trailing slot: an unattached row
// renders an empty slot of the same width, keeping the bullets column-aligned.
const attachedMarker = "● attached"

// goneBadge must not exceed attachedSlotWidth + rowRightMargin cells: it fills
// the trailing region, keeping the row width unchanged.
const goneBadge = "session gone"

var attachedSlotWidth = lipgloss.Width(attachedMarker)

const groupSeparator = "···"

const (
	// Indents a header to col 2, the title-box left edge.
	groupHeaderIndent = "  "
	// Rendered before the left-bar column: the ▌ lands at col 2 (aligned
	// with the header text) and the name at col 4; flat rows skip it.
	groupRowIndent = "  "
)

func windowLabel(count int) string {
	if count == 1 {
		return "1 window"
	}
	return fmt.Sprintf("%d windows", count)
}

// Two SessionItems sharing a Session but differing in GroupKey are independently
// selectable views of one session, not distinct attach targets — selection and
// attach key on Session.Name.
type SessionItem struct {
	Session tmux.Session

	// GroupKey is the canonical sort/boundary key: canonical directory path
	// (By Project), tag (By Tag), empty for Flat.
	GroupKey string

	// GroupHeading is the label shown at a group boundary; empty for Flat.
	GroupHeading string

	// CatchAll marks an Unknown / Untagged bucket instance, pinning it last
	// without string-matching the heading.
	CatchAll bool
}

// FilterValue is the text every filter entry point narrows on: the session name
// joined to its recorded directory, home-abbreviated so a term hitting the home
// prefix cannot match a session on characters no row shows. The grouping-derived
// directory is no part of it — a regroup must never make a session findable by a
// path it was not findable by a moment earlier.
func (i SessionItem) FilterValue() string {
	if i.Session.Dir == "" {
		return i.Session.Name
	}
	return i.Session.Name + " " + resolver.AbbreviateHome(i.Session.Dir)
}

// A genuine height-1 list.Item is load-bearing: pagination counts every rendered
// line exactly, so a page can never overflow the viewport. Its empty FilterValue
// makes headers vanish the moment a filter query is typed.
type HeaderItem struct {
	Heading string
	Count   int
	Key     string
}

func (HeaderItem) FilterValue() string { return "" }

func (h HeaderItem) headingText() string {
	return h.Heading + " "
}

func (h HeaderItem) countText() string {
	return fmt.Sprintf("%s %d", groupSeparator, h.Count)
}

// Theme is re-pointed by the model on every restyle; a zero value renders
// silently colourless, so a hand-built delegate must carry one.
type SessionDelegate struct {
	Theme theme.Theme
	// Colourless is the NO_COLOR carve-out; structure stays glyph-distinct.
	// Suppressing the background is the real work — hue is stripped downstream.
	Colourless  bool
	MultiSelect bool
	// Selected is keyed on Session.Name, so a multi-tag By-Tag session marked
	// once shows the ● on every one of its rows. Nil marks nothing.
	Selected map[string]struct{}
	// GoneFlagged is the transient pre-flight abort set, keyed on Session.Name.
	GoneFlagged map[string]struct{}
}

func isSelected(set map[string]struct{}, name string) bool {
	_, ok := set[name]
	return ok
}

func (d SessionDelegate) canvasBg() lipgloss.Style {
	return headerCanvasBg(d.Theme, d.Colourless)
}

func (d SessionDelegate) tokenStyle(base lipgloss.Style, fg theme.Token) lipgloss.Style {
	return headerStyle(fg, d.Theme, d.Colourless).Inherit(base)
}

// Both item types render one line, keeping pagination exact.
func (d SessionDelegate) Height() int { return 1 }

func (d SessionDelegate) Spacing() int { return 0 }

func (d SessionDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d SessionDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	bg := d.canvasBg()
	var row string
	switch it := item.(type) {
	case HeaderItem:
		heading := d.tokenStyle(lipgloss.Style{}, d.Theme.TextMuted).Render(it.headingText())
		count := d.tokenStyle(lipgloss.Style{}, d.Theme.TextSubtle).Render(it.countText())
		row = bg.Render(groupHeaderIndent) + heading + count
	case SessionItem:
		row = d.renderSessionRow(m, index, it)
	default:
		return
	}

	// bubbles/list may block-pad with raw, background-less trailing spaces; the
	// outer canvas fill strips those, so the delegate emits only its own content.
	_, _ = fmt.Fprint(w, row)
}

func rowBgStyle(th theme.Theme, selected, colourless bool) lipgloss.Style {
	if colourless {
		return lipgloss.NewStyle()
	}
	if selected {
		return lipgloss.NewStyle().Background(th.BgSelection.Color())
	}
	return lipgloss.NewStyle().Background(th.Canvas.Color())
}

func rowTokenStyle(base lipgloss.Style, fg theme.Token, th theme.Theme, selected, colourless bool) lipgloss.Style {
	if colourless {
		return base
	}
	styled := base.Foreground(fg.Color())
	if selected {
		return styled.Background(th.BgSelection.Color())
	}
	return styled.Background(th.Canvas.Color())
}

func renderLeftBarColumn(bg, selectorStyle lipgloss.Style, selected bool) string {
	if selected {
		return renderLeftBarGlyphColumn(selectorBar, selectorStyle, bg)
	}
	return bg.Render(padTo("", leftBarColumnWidth))
}

func renderLeftBarGlyphColumn(glyph string, glyphStyle, bg lipgloss.Style) string {
	return glyphStyle.Render(glyph) +
		bg.Render(padTo("", leftBarColumnWidth-lipgloss.Width(glyph)))
}

func renderMarkedLeftBarColumn(bg, markerStyle lipgloss.Style) string {
	return renderLeftBarGlyphColumn(multiSelectMarker, markerStyle, bg)
}

func renderGoneLeftBarColumn(bg, markerStyle lipgloss.Style) string {
	return renderLeftBarGlyphColumn(flashWarningGlyph, markerStyle, bg)
}

func (d SessionDelegate) rowBg(selected bool) lipgloss.Style {
	return rowBgStyle(d.Theme, selected, d.Colourless)
}

func (d SessionDelegate) rowToken(base lipgloss.Style, fg theme.Token, selected bool) lipgloss.Style {
	return rowTokenStyle(base, fg, d.Theme, selected, d.Colourless)
}

func (d SessionDelegate) renderSessionRow(m list.Model, index int, it SessionItem) string {
	selected := index == m.Index()
	// While the filter input is being edited no row renders selected — the
	// cursor lives in the filter input.
	if m.FilterState() == list.Filtering {
		selected = false
	}
	bg := d.rowBg(selected)

	indent := ""
	if it.GroupKey != "" {
		indent = groupRowIndent
	}
	indentCell := bg.Render(indent)

	// Precedence: gone ⚠ over marked ● over the ▌ selector. The ●/⚠ styles take
	// `selected`, not the bar's literal true, so they tint only on the cursor row.
	goneRow := isSelected(d.GoneFlagged, it.Session.Name)
	marked := d.MultiSelect && isSelected(d.Selected, it.Session.Name)
	var bar string
	switch {
	case goneRow:
		bar = renderGoneLeftBarColumn(bg, d.rowToken(lipgloss.Style{}, d.Theme.StateDestructive, selected))
	case marked:
		bar = renderMarkedLeftBarColumn(bg, d.rowToken(lipgloss.Style{}, d.Theme.AccentPrimary, selected))
	default:
		bar = renderLeftBarColumn(bg, d.rowToken(lipgloss.Style{}, d.Theme.AccentPrimary, true), selected)
	}

	nameTok := d.Theme.TextPrimary
	if selected {
		nameTok = d.Theme.TextOnSelection
	}
	countTok := d.Theme.TextMuted
	if selected {
		countTok = d.Theme.TextSecondary
	}
	countText := windowLabel(it.Session.Windows)

	// An unsized list (before the first WindowSizeMsg) has no width to flex
	// against — fall back to a left-to-right flow with no truncation.
	total := m.Width()
	used := leftBarColumnWidth + lipgloss.Width(indent) + nameGap + countSlotWidth + attachedSlotWidth + rowRightMargin

	var name, namePad string
	if total <= 0 {
		name = d.rowToken(nameBase, nameTok, selected).Render(it.Session.Name)
		namePad = ""
	} else {
		nameWidth := max(total-used, 1)
		visibleName := ansi.Truncate(it.Session.Name, nameWidth, "…")
		name = d.rowToken(nameBase, nameTok, selected).Render(visibleName)
		namePad = bg.Render(padTo("", nameWidth-lipgloss.Width(visibleName)))
	}

	gap := bg.Render(padTo("", nameGap))

	count := d.rowToken(lipgloss.Style{}, countTok, selected).Render(countText) +
		bg.Render(padTo("", countSlotWidth-lipgloss.Width(countText)))

	// On a gone row the badge replaces the attached slot and the right margin,
	// keeping the row width unchanged.
	var trailing string
	if goneRow {
		badge := d.rowToken(lipgloss.Style{}, d.Theme.StateDestructive, selected).Render(goneBadge)
		trailing = badge + bg.Render(padTo("", attachedSlotWidth+rowRightMargin-lipgloss.Width(goneBadge)))
	} else {
		attached := bg.Render(padTo("", attachedSlotWidth))
		if it.Session.Attached {
			attached = d.rowToken(lipgloss.Style{}, d.Theme.StatePositive, selected).Render(attachedMarker) +
				bg.Render(padTo("", attachedSlotWidth-lipgloss.Width(attachedMarker)))
		}
		rightMargin := bg.Render(padTo("", rowRightMargin))
		trailing = attached + rightMargin
	}
	row := indentCell + bar + name + namePad + gap + count + trailing

	// The flex name floors at 1, so at pathological narrow widths the assembled
	// row would overflow; a final guard, and a no-op on the happy path.
	if total > 0 {
		row = ansi.Truncate(row, total, "…")
	}
	return row
}

func padTo(s string, n int) string {
	w := lipgloss.Width(s)
	if n <= w {
		return s
	}
	return s + spaces(n-w)
}

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

func ToListItems(sessions []tmux.Session) []list.Item {
	items := make([]list.Item, len(sessions))
	for i, s := range sessions {
		items[i] = SessionItem{Session: s}
	}
	return items
}
