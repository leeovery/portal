package tui

import (
	"fmt"
	"io"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/tui/theme"
)

// projectNameBase carries the project name's NON-colour attribute (heavy/bold);
// the delegate layers the resolved-mode foreground (text.primary, or
// text.on-selection on the selected row) and the row background through
// ProjectDelegate.rowToken so the colour pair is mode-matched. Mirrors
// session_item.go's nameBase.
var projectNameBase = lipgloss.NewStyle().Bold(true)

// ProjectItem wraps a project.Project and implements the list.Item interface
// for use with bubbles/list.
type ProjectItem struct {
	Project project.Project
}

// FilterValue returns the project name for filtering. It is the only method
// bubbles/list consumes off the item (list.Item); the project name and path are
// produced solely by the delegate's live render path
// (ProjectDelegate.renderRowLine).
func (i ProjectItem) FilterValue() string {
	return i.Project.Name
}

// ProjectDelegate implements list.ItemDelegate for rendering project items.
//
// Each item renders as TWO lines: the project name (text.primary, heavy) on line
// 1, the project path (text.detail, dim) on line 2 (§6 / §6.2). The selected row
// carries a full-height accent.violet ▌ left bar spanning BOTH lines over a
// bg.selection tint, with the name in text.on-selection and the path in
// text.muted-bright; unselected rows carry neither bar nor tint.
//
// Mode is the resolved canvas appearance (§1): every run the delegate emits is
// painted with the §2.9 role-token FOREGROUND resolved for this Mode over a
// Background (canvas on a normal row, bg.selection on the selected row) for this
// Mode, so each row reads correctly on the resolved canvas with no terminal-bg
// island behind the styled text. The zero value is theme.Dark, so a bare
// ProjectDelegate{} (the value used across the existing unit tests) paints the dark
// canvas it was tuned for; the model sets Mode from its resolved canvasMode in
// applyCanvasMode. Mirrors SessionDelegate.
type ProjectDelegate struct {
	Mode theme.Mode
	// Colourless is the NO_COLOR carve-out (§2.5): when set, the delegate paints NO
	// canvas/selection background and NO foreground hue — every run renders on the
	// terminal's native fg/bg. The two-line structure and the ▌ bar glyph are
	// unchanged, so selection stays glyph-distinct (§2.2). Set from the model's
	// single colourless flag (applyCanvasMode), mirroring SessionDelegate.
	Colourless bool
}

// Height returns 2, matching the two-line item display (name + path). The uniform
// two-line height is what keeps bubbles/list pagination exact (§3.5 / §6.2).
func (d ProjectDelegate) Height() int { return 2 }

// Spacing returns 0, no gap between items.
func (d ProjectDelegate) Spacing() int { return 0 }

// Update returns nil; no item-level keybinding handling is needed.
func (d ProjectDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

// rowBg delegates to the shared rowBgStyle free function (session_item.go),
// binding the delegate's Mode and Colourless — the structural-cell style for a
// project row: the bg.selection tint on the selected row, otherwise the owned
// canvas (or a bare style under the NO_COLOR carve-out). Shared with
// SessionDelegate so the selection-vs-canvas colour role lives in one place.
func (d ProjectDelegate) rowBg(selected bool) lipgloss.Style {
	return rowBgStyle(d.Mode, selected, d.Colourless)
}

// rowToken delegates to the shared rowTokenStyle free function (session_item.go),
// binding the delegate's Mode and Colourless — base with the role token's
// mode-resolved FOREGROUND over the row's background (bg.selection on the
// selected row, canvas otherwise; base unchanged under the NO_COLOR carve-out).
// Shared with SessionDelegate.
func (d ProjectDelegate) rowToken(base lipgloss.Style, fg theme.Token, selected bool) lipgloss.Style {
	return rowTokenStyle(base, fg, d.Mode, selected, d.Colourless)
}

// Render renders a project item as two lines: the name (text.primary, heavy) on
// line 1, the path (text.detail, dim) on line 2 (§6 / §6.2). The selected row
// carries a full-height accent.violet ▌ left bar across BOTH lines over a
// bg.selection tint, with the name in text.on-selection and the path in
// text.muted-bright. Each line is exactly the list width: a 2-cell left-bar column
// then the flexing text column, padded so the tint/canvas paints the whole row with
// no terminal-bg island. Over-long text truncates with an ellipsis (§2.7) so neither
// line overflows the width and the two-line height stays uniform (pagination parity).
func (d ProjectDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	pi, ok := item.(ProjectItem)
	if !ok {
		return
	}

	selected := index == m.Index()
	// §7.1 input-active clarity (mirrors SessionDelegate): while the filter input is
	// being edited NO list row is selected — the cursor lives in the filter input —
	// so the violet bar and the bg.selection tint are suppressed for every row.
	if m.FilterState() == list.Filtering {
		selected = false
	}

	// Name — text.primary (selected: text.on-selection), heavy. Path — text.detail
	// (selected: text.muted-bright, the §2.9 path-on-selection token), dim.
	nameTok := theme.MV.TextPrimary
	pathTok := theme.MV.TextDetail
	if selected {
		nameTok = theme.MV.TextOnSelection
		pathTok = theme.MV.TextMutedBright
	}

	line1 := d.renderRowLine(m, selected, d.rowToken(projectNameBase, nameTok, selected), pi.Project.Name)
	line2 := d.renderRowLine(m, selected, d.rowToken(lipgloss.Style{}, pathTok, selected), pi.Project.Path)

	_, _ = fmt.Fprintf(w, "%s\n%s", line1, line2)
}

// renderRowLine renders ONE line of a project row: the 2-cell left-bar column (the
// accent.violet ▌ on the selected row, two blank cells otherwise) followed by the
// text flexing to fill the rest of the list width, truncated with an ellipsis
// (§2.7) and padded so the line is exactly the width — every cell carrying the row
// background (bg.selection on the selected row, canvas otherwise). textStyle already
// carries the run's foreground + row background. Both the name line and the path
// line share this shape so the full-height bar + tint span both uniformly.
func (d ProjectDelegate) renderRowLine(m list.Model, selected bool, textStyle lipgloss.Style, text string) string {
	bg := d.rowBg(selected)

	// Left-bar column (§3.3 / §6.2): the violet ▌ + a trailing cell on the selected
	// row, two blank cells otherwise — a fixed 2-cell column keeping the text at the
	// same left edge whether or not the row is selected. Shared with the Session
	// delegate via renderLeftBarColumn.
	bar := renderLeftBarColumn(bg, d.rowToken(lipgloss.Style{}, theme.MV.AccentViolet, true), selected)

	// Flex text column. When the list has not been sized yet (Width() == 0, a
	// directly-constructed model that renders before its first WindowSizeMsg) there
	// is no width to flex against — render the full text with no truncation and no
	// trailing pad (mirrors SessionDelegate's zero-width fallback).
	total := m.Width()
	if total <= 0 {
		return bar + textStyle.Render(text)
	}

	textWidth := max(total-leftBarColumnWidth, 1)
	visible := ansi.Truncate(text, textWidth, "…")
	body := textStyle.Render(visible)
	pad := bg.Render(padTo("", textWidth-lipgloss.Width(visible)))

	line := bar + body + pad

	// Safety clamp (§2.7 / §3.5): at pathological narrow widths the fixed 2-cell bar
	// column plus a 1-cell floored text could assemble to more than total; truncate
	// the assembled line to total as a final guard so the line never overflows the
	// list width (a no-op on the happy path, where the line is already exactly total).
	return ansi.Truncate(line, total, "…")
}

// ProjectsToListItems converts a slice of projects to a slice of list.Item.
func ProjectsToListItems(projects []project.Project) []list.Item {
	items := make([]list.Item, len(projects))
	for i, p := range projects {
		items[i] = ProjectItem{Project: p}
	}
	return items
}
