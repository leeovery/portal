package tui

import (
	"fmt"
	"io"
	"strings"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/resolver"
	"github.com/leeovery/portal/internal/theme"
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
	// Mirrors the left inset; a wider right margin reads as oversized.
	rowRightMargin = leftBarColumnWidth
)

const (
	rowIndicatorGlyph = "●"
	// NO_COLOR keeps state glyph-backed: a letter per indicator, same cell.
	attachedIndicatorLetter = "A"
	pendingIndicatorLetter  = "P"
)

// A badged row reserves the badge's width in place of the indicator slot and
// right margin. The badge is text, so it reads the same under NO_COLOR.
const (
	goneBadge         = "session gone"
	notAnsweringBadge = "not answering"
)

type rowIndicators struct {
	attached bool
	pending  bool
}

// Sized to the widest cluster either colour mode renders, so every row pads to
// the same width whatever indicators it carries.
var indicatorSlotWidth = max(
	lipgloss.Width(SessionDelegate{}.indicatorCluster(rowIndicators{attached: true, pending: true}, false)),
	lipgloss.Width(SessionDelegate{Colourless: true}.indicatorCluster(rowIndicators{attached: true, pending: true}, false)),
)

const groupSeparator = "···"

const (
	// Indents a header to col 2, the title-box left edge.
	groupHeaderIndent = "  "
	// Rendered before the left-bar column: the ▌ lands at col 2 (aligned
	// with the header text) and the name at col 4; flat rows skip it.
	groupRowIndent = "  "
)

// Two SessionItems sharing a Session but differing in GroupKey are independently
// selectable views of one session, not distinct attach targets — selection and
// attach key on Session.Name.
type SessionItem struct {
	Session Session

	// GroupKey is the canonical sort/boundary key: canonical directory path
	// (By Project), tag (By Tag), empty for Flat.
	GroupKey string

	// GroupHeading is the label shown at a group boundary; empty for Flat.
	GroupHeading string

	// CatchAll marks an Unknown / Untagged bucket instance, pinning it last
	// without string-matching the heading.
	CatchAll bool

	// Home is the home directory the session's directory is abbreviated against;
	// empty abbreviates nothing.
	Home string
}

// FilterValue is the session's search fields flattened for the picker's own
// fuzzy rule, which narrows on one string. The name leads, so a query typed
// against a row reads left to right as the row does. The grouping-derived
// directory is no part of it — a regroup must never make a session findable by a
// path it was not findable by a moment earlier.
func (i SessionItem) FilterValue() string {
	return strings.Join(resolver.SearchFields(i.Home, i.Session.SearchSubject()), " ")
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
	// ShowDir renders each session's recorded directory beside its name, out of
	// the name's own flex budget. Off by default: it belongs to a picker opened
	// by a session search, whose rows can be present on the strength of a
	// directory, and it stays on for that picker's life however the filter text
	// is edited afterwards.
	ShowDir bool
	// Selected is keyed on Session.Name, so a multi-tag By-Tag session marked
	// once shows the ● on every one of its rows. Nil marks nothing.
	Selected map[string]struct{}
	// GoneFlagged is the transient pre-flight abort set, keyed on Session.Name.
	GoneFlagged map[string]struct{}
	// Pending holds the sessions with any pane awaiting a resume decision, keyed
	// on Session.Name so a session carries one dot however many panes wait.
	// Nil marks nothing.
	Pending map[string]struct{}
	// Home is the home directory a shown directory is abbreviated against; empty
	// abbreviates nothing.
	Home string
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
	mutedTok := d.Theme.TextMuted
	if selected {
		mutedTok = d.Theme.TextSecondary
	}
	trailing, trailingWidth := d.renderRowTrailing(it.Session, bg, mutedTok, selected)

	// An unsized list (before the first WindowSizeMsg) has no width to flex
	// against — fall back to a left-to-right flow with no truncation.
	total := m.Width()
	used := leftBarColumnWidth + lipgloss.Width(indent) + nameGap + trailingWidth

	nameStyle := d.rowNameStyle(it.Session, mutedTok, selected)
	var name, namePad, dirText string
	if total <= 0 {
		name = nameStyle.Render(it.Session.DisplayName())
		if d.ShowDir && it.Session.Dir != "" {
			dirText = resolver.AbbreviateHome(d.Home, it.Session.Dir)
		}
	} else {
		nameWidth := max(total-used, 1)
		visibleName := ansi.Truncate(it.Session.DisplayName(), nameWidth, "…")
		name = nameStyle.Render(visibleName)
		remaining := nameWidth - lipgloss.Width(visibleName)
		if d.ShowDir {
			// The separating space is the one cell withheld from the fit.
			dirText = fitSessionDir(d.Home, it.Session.Dir, remaining-1)
		}
		if dirText != "" {
			remaining -= 1 + lipgloss.Width(dirText)
		}
		namePad = bg.Render(padTo("", remaining))
	}

	// Painted by the row background, so the selected row's tint covers the gap.
	dirCell := ""
	if dirText != "" {
		dirCell = bg.Render(" ") + d.rowToken(lipgloss.Style{}, mutedTok, selected).Render(dirText)
	}

	gap := bg.Render(padTo("", nameGap))
	bar := d.renderRowBar(it.Session.Name, bg, selected)
	row := bg.Render(indent) + bar + name + dirCell + namePad + gap + trailing

	// The flex name floors at 1, so at pathological narrow widths the assembled
	// row would overflow; a final guard, and a no-op on the happy path.
	if total > 0 {
		row = ansi.Truncate(row, total, "…")
	}
	return row
}

// Precedence: gone ⚠ over marked ● over the ▌ selector. The ●/⚠ styles take
// `selected`, not the bar's literal true, so they tint only on the cursor row.
func (d SessionDelegate) renderRowBar(name string, bg lipgloss.Style, selected bool) string {
	switch {
	case isSelected(d.GoneFlagged, name):
		return renderGoneLeftBarColumn(bg, d.rowToken(lipgloss.Style{}, d.Theme.StateDestructive, selected))
	case d.MultiSelect && isSelected(d.Selected, name):
		return renderMarkedLeftBarColumn(bg, d.rowToken(lipgloss.Style{}, d.Theme.AccentPrimary, selected))
	default:
		return renderLeftBarColumn(bg, d.rowToken(lipgloss.Style{}, d.Theme.AccentPrimary, true), selected)
	}
}

// A session that did not answer is dimmed: muted and not bold, so under
// NO_COLOR it still differs from its neighbours beside its badge.
func (d SessionDelegate) rowNameStyle(s Session, mutedTok theme.Token, selected bool) lipgloss.Style {
	if s.NotAnswering {
		return d.rowToken(lipgloss.Style{}, mutedTok, selected)
	}
	nameTok := d.Theme.TextPrimary
	if selected {
		nameTok = d.Theme.TextOnSelection
	}
	return d.rowToken(nameBase, nameTok, selected)
}

// renderRowTrailing returns the row's hard-right segment and the width it
// reserves: a badge flush to the edge when the row carries one, gone over not
// answering, else the indicator cluster in its slot before the right margin.
func (d SessionDelegate) renderRowTrailing(s Session, bg lipgloss.Style, mutedTok theme.Token, selected bool) (string, int) {
	switch {
	case isSelected(d.GoneFlagged, s.Name):
		return d.rowToken(lipgloss.Style{}, d.Theme.StateDestructive, selected).Render(goneBadge), lipgloss.Width(goneBadge)
	case s.NotAnswering:
		return d.rowToken(lipgloss.Style{}, mutedTok, selected).Render(notAnsweringBadge), lipgloss.Width(notAnsweringBadge)
	}
	cluster := d.indicatorCluster(rowIndicators{
		attached: s.Attached,
		pending:  isSelected(d.Pending, s.Name),
	}, selected)
	trailing := bg.Render(padTo("", indicatorSlotWidth-lipgloss.Width(cluster))) +
		cluster +
		bg.Render(padTo("", rowRightMargin))
	return trailing, indicatorSlotWidth + rowRightMargin
}

// indicatorCluster packs the row's indicators for a hard-right placement; a row
// carrying none renders the empty string.
func (d SessionDelegate) indicatorCluster(ind rowIndicators, selected bool) string {
	var cluster string
	if ind.attached {
		cluster += d.indicator(d.Theme.StatePositive, attachedIndicatorLetter, selected)
	}
	if ind.pending {
		cluster += d.indicator(d.Theme.AccentAttention, pendingIndicatorLetter, selected)
	}
	return cluster
}

func (d SessionDelegate) indicator(tok theme.Token, letter string, selected bool) string {
	glyph := rowIndicatorGlyph
	if d.Colourless {
		glyph = letter
	}
	return d.rowToken(lipgloss.Style{}, tok, selected).Render(glyph)
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

// withHome stamps home on every session row in items, so a row's filter value
// abbreviates its directory against the same home its delegate shows it under.
func withHome(items []list.Item, home string) []list.Item {
	for i, item := range items {
		if si, ok := item.(SessionItem); ok {
			si.Home = home
			items[i] = si
		}
	}
	return items
}

func ToListItems(sessions []Session) []list.Item {
	items := make([]list.Item, len(sessions))
	for i, s := range sessions {
		items[i] = SessionItem{Session: s}
	}
	return items
}
