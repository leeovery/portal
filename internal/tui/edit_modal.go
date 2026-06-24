package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/tui/theme"
)

// The §8.2 / §13.1 two-mode edit-project modal — the MV render over the 3-8
// state machine. A reskin (the editMode/editFocus/edit-buffer state machine in
// model.go is untouched); this file owns only the FINAL MV rendering.
//
// VISUAL GRAMMAR (the corrigendum-revised §13.1 — supersedes the older fill/✕
// wording). NOTHING FILLS: every editable element (the NAME input AND the chips)
// is a glyph-drawn bordered box whose STATE is carried by the BORDER COLOUR, never
// a background fill — grey (border.separator) idle/unfocused → accent.violet
// focused → accent.orange editing (+ a live block cursor). Inputs render ROUNDED
// corners, chips render SQUARE corners (the element-type differentiator). Chips are
// text.primary, never green, with NO inline ✕ (removal is `x` on a focused chip,
// carried by the footer). The `◉ EDIT MODE` header indicator is accent.orange,
// shown ONLY while editing in place.
//
// The panel chrome reuses the shared single-tone hand-drawn joined panel
// (renderJoinedPanel) — the SAME frame the help/kill/rename modals use, three
// compartments (header / body / footer) with joined ├───┤ dividers in
// border.separator. Under the NO_COLOR carve-out every hue drops to the native fg;
// state survives via the border PRESENCE + the live cursor + the `◉ EDIT MODE`
// text + bold/dim (§2.2 — state never colour-only).

const (
	// editHeaderPrefix opens the header — `Edit Project ` in text.primary, with the
	// project name trailing in text.detail.
	editHeaderPrefix = "Edit Project "
	// editModeIndicator is the accent.orange `◉ EDIT MODE` header badge, shown ONLY
	// while editing in place (glyph + colour + text, §2.2).
	editModeIndicator = "◉ EDIT MODE"

	// Field labels (§13.1): the focused field's label is accent.violet, the others
	// text.detail.
	editLabelName    = "NAME"
	editLabelAliases = "ALIASES"
	editLabelTags    = "TAGS"

	// editAddSlot is the inline faint `+ add` slot trailing the chips (text.faint).
	editAddSlot = "+ add"

	// editNameInnerWidth is the NAME input box's inner content width (cells) — the
	// span inside the rounded box's side borders, including its 1-cell horizontal
	// padding each side. It also anchors the panel's body width so the panel stays a
	// consistent size regardless of value length, matching the reference's wide NAME
	// field. Sized to comfortably hold a `{project}-{nanoid}` name with room to grow.
	editNameInnerWidth = 56

	// editChipPadX is the chip box's horizontal padding (cells each side) inside its
	// square border — so a chip reads `│ fapi │`, matching the reference's compact
	// chips.
	editChipPadX = 1

	// editFieldGap is the blank spacer row between the field blocks (label + control)
	// — the body's vertical rhythm.
	editFieldGap = ""
)

// inputBoxState is the border-colour state of an editable bordered box (§13.1): the
// SAME three-state model for the NAME input AND every chip. Grey unfocused/idle →
// violet focused → orange editing (+ cursor). Task 3-10 routes the rename input
// through renderInputBox in the always-editing variant.
type inputBoxState int

const (
	inputBoxIdle    inputBoxState = iota // grey border (border.separator) — unfocused/normal
	inputBoxFocused                      // accent.violet border — focused, not editing
	inputBoxEditing                      // accent.orange border + live cursor — editing in place
)

// inputBoxBorderToken maps a box state to its border role token (§13.1): grey →
// violet → orange. The shared mapping the NAME input and chips both use.
func inputBoxBorderToken(state inputBoxState) theme.Token {
	switch state {
	case inputBoxFocused:
		return theme.MV.AccentViolet
	case inputBoxEditing:
		return theme.MV.AccentOrange
	default:
		return theme.MV.BorderSeparator
	}
}

// renderInputBox draws the reusable bordered box (§13.1 — the 3-10 dependency):
// content wrapped in a thin glyph border whose colour is the state's role token,
// over a TRANSPARENT interior (NO fill, ever). rounded selects ROUNDED corners (the
// NAME input) vs SQUARE corners (chips). innerWidth fixes the box's inner content
// width (the NAME input pins it for a consistent panel width; a chip passes -1 to
// size to its content). Returns the box's rendered rows (one per line) so the
// caller can lay them out — three rows for a single-line content.
//
// No fill, by design: a flush fill cannot coexist with a thin glyph border in a
// terminal — a border glyph owns a full cell with one background, so a fill either
// leaves a half-cell gap or bleeds past the border (the corigendum's confirmed
// finding). Focus is therefore carried by the border COLOUR. Under the NO_COLOR
// carve-out the border foreground is dropped so the glyphs survive on the native fg
// (the box PRESENCE is the state signal; the cursor + EDIT MODE text distinguish
// editing).
func renderInputBox(content string, state inputBoxState, rounded bool, innerWidth int, mode theme.Mode, colourless bool) []string {
	border := lipgloss.NormalBorder() // square corners (chips)
	if rounded {
		border = lipgloss.RoundedBorder()
	}
	style := lipgloss.NewStyle().
		Border(border).
		Padding(0, editChipPadX)
	if innerWidth > 0 {
		style = style.Width(innerWidth)
	}
	if !colourless {
		style = style.BorderForeground(inputBoxBorderToken(state).ColorFor(mode))
	}
	return strings.Split(style.Render(content), "\n")
}

// editChipContent renders one chip's interior — its value in text.primary, with a
// live block cursor appended/inserted when editing (§13.1). The chip's value is
// never green; the cursor is the editing-state signal that survives NO_COLOR.
//
// A navigate-mode chip (normal or focused) is sized to its CONTENT — no reserved
// trailing cell. An edit-mode chip renders its live cursor via renderEditableValue,
// so its box may be one cell wider while editing; that is fine and expected — the
// panel width is anchored by the header/footer/name box (all wider than any chip),
// so a wider chip never resizes the panel.
func editChipContent(value string, editing bool, cursor int, mode theme.Mode, colourless bool) string {
	if editing {
		return renderEditableValue(value, cursor, mode, colourless)
	}
	return headerStyle(theme.MV.TextPrimary, mode, colourless).Render(value)
}

// renderEditableValue renders a live-edited value (the NAME input value or a chip
// value being edited) in text.primary with an accent.orange block cursor at the
// rune index. The cursor is a reverse-video block (SGR 7) over the orange
// foreground, so under the NO_COLOR carve-out the reverse block survives as the
// editing signal even with no hue.
//
// The rendered width is ALWAYS len(value)+1: an end-of-value cursor paints a
// trailing block cell, and a mid-value cursor overlays a rune block AND appends a
// trailing blank cell — so an editable box stays a CONSTANT width regardless of
// where the text cursor sits (and matches the navigate-mode reserved-cell width).
func renderEditableValue(value string, cursor int, mode theme.Mode, colourless bool) string {
	runes := []rune(value)
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(runes) {
		cursor = len(runes)
	}
	before := string(runes[:cursor])
	cursorGlyph := " "
	after := ""
	trailing := "" // reserved trailing cell when the cursor overlays a mid-value rune.
	if cursor < len(runes) {
		cursorGlyph = string(runes[cursor])
		after = string(runes[cursor+1:])
		trailing = " "
	}

	textStyle := headerStyle(theme.MV.TextPrimary, mode, colourless)
	cursorStyle := lipgloss.NewStyle().Reverse(true)
	if !colourless {
		cursorStyle = cursorStyle.
			Foreground(theme.MV.AccentOrange.ColorFor(mode)).
			Background(theme.MV.Canvas.ColorFor(mode))
	}

	var b strings.Builder
	b.WriteString(textStyle.Render(before))
	b.WriteString(cursorStyle.Render(cursorGlyph))
	b.WriteString(textStyle.Render(after))
	if trailing != "" {
		b.WriteString(headerCanvasBg(mode, colourless).Render(trailing))
	}
	return b.String()
}

// renderEditProjectContent composes the §8.2 / §13.1 MV edit-project modal: three
// compartments drawn by the shared single-tone joined panel —
//
//	header:  Edit Project <name>        [◉ EDIT MODE]   (prefix text.primary, name text.detail, badge accent.orange while editing)
//	body:    NAME                                        (focused-field label accent.violet, others text.detail)
//	         ╭ <value>▌ ╮                                (rounded box; grey/violet/orange border, no fill)
//	         ALIASES
//	         ┌ fapi ┐ ┌ v1 ┐ + add                       (square chip boxes + faint `+ add`)
//	         TAGS
//	         ┌ Fabric ┐ ┌ api ┐ + add
//	footer:  contextual keymap                           (key glyphs accent.blue, labels text.detail)
//
// The render reads the live editBuffer/editCursor for the one live element so an
// in-progress edit shows; everything else comes from the persisted edit state.
func (m Model) renderEditProjectContent() string {
	mode, colourless := m.canvasMode, m.colourless

	header := []string{m.editModalHeaderRow(mode, colourless)}
	body := m.editModalBodyRows(mode, colourless)
	footer := []string{m.editModalFooterRow(mode, colourless)}

	return renderJoinedPanel([][]string{header, body, footer}, theme.MV.BorderSeparator, mode, colourless)
}

// editPanelContentWidth returns the modal's panel content width — the span every
// compartment row is padded to — computed ONCE and independent of edit state, so
// toggling navigate↔edit never resizes the panel (the "jaggedy" resize bug). It is
// the max of:
//
//   - the NAME input box (the widest body element, editNameInnerWidth + 2 side
//     borders), and
//   - the header's worst case (left title + badge gap + the `◉ EDIT MODE` badge,
//     which the navigate header reserves as blank), and
//   - the footer's worst case (the always-longest editing-in-place footer copy).
//
// Pinning the width to these state-independent constants means the header, name
// box, chip rows, and footer all render at one stable width whether editing or not.
func (m Model) editPanelContentWidth() int {
	w := editNameInnerWidth + 2 // name box: inner width + the two side borders.
	if hw := editHeaderNaturalWidth(m.editProject.Name); hw > w {
		w = hw
	}
	if fw := editFooterWidestWidth(); fw > w {
		w = fw
	}
	return w
}

// editHeaderNaturalWidth is the header row's width WITH the badge present (the
// worst case): `Edit Project <name>` + the badge gap + `◉ EDIT MODE`. The navigate
// header reserves this exact span (blank where the badge would be) so the row width
// is identical in both modes.
func editHeaderNaturalWidth(name string) int {
	return lipgloss.Width(editHeaderPrefix) + lipgloss.Width(name) +
		editHeaderBadgeGap + lipgloss.Width(editModeIndicator)
}

// editFooterWidestWidth is the width of the longest footer copy (the
// editing-in-place footer), measured plain so the panel width never depends on the
// live mode.
func editFooterWidestWidth() int {
	return lipgloss.Width(strings.Join([]string{
		"⏎ save", "esc discard", "←→ cursor", "empty on save = delete",
	}, footerEntrySeparator))
}

// editHeaderBadgeGap is the minimum gap (cells) between the title and the
// right-corner `◉ EDIT MODE` badge — also the blank span the navigate header
// reserves so the header width is constant.
const editHeaderBadgeGap = 3

// editModalHeaderRow renders the header as a FIXED full-content-width row:
// `Edit Project <name>` (prefix text.primary, name text.detail) left-aligned, and
// the `◉ EDIT MODE` badge (accent.orange) RIGHT-aligned in the far corner — shown
// ONLY while editing in place. In navigate mode the badge's slot is rendered as a
// same-width blank, so the header (and therefore the whole panel) is byte-for-byte
// the SAME width in both modes — entering edit never resizes the panel.
func (m Model) editModalHeaderRow(mode theme.Mode, colourless bool) string {
	prefix := headerStyle(theme.MV.TextPrimary, mode, colourless).Bold(true).Render(editHeaderPrefix)
	name := headerStyle(theme.MV.TextDetail, mode, colourless).Render(m.editProject.Name)
	left := lipgloss.JoinHorizontal(lipgloss.Top, prefix, name)
	return renderHeaderWithBadge(left, m.editPanelContentWidth(), m.editMode == editModeEdit, mode, colourless)
}

// renderHeaderWithBadge lays out a modal header as a FIXED full-content-width row:
// the pre-rendered left title segment left-aligned, a flexible canvas spacer, then
// the `◉ EDIT MODE` badge (accent.orange, bold) RIGHT-aligned in the far corner.
// When showBadge is false the badge's slot is rendered as a same-width blank, so the
// header (and therefore the whole panel) is byte-for-byte the SAME width whether the
// badge shows or not. Shared by the edit modal (badge gated on edit mode) and the
// rename modal (always editing → badge always shown). The right-align is the §13.1
// fixed-content-width row + flexible-spacer technique.
func renderHeaderWithBadge(left string, contentWidth int, showBadge bool, mode theme.Mode, colourless bool) string {
	leftWidth := lipgloss.Width(left)
	badgeWidth := lipgloss.Width(editModeIndicator)
	spacerWidth := contentWidth - leftWidth - badgeWidth
	if spacerWidth < 0 {
		spacerWidth = 0
	}
	spacer := headerCanvasBg(mode, colourless).Render(strings.Repeat(" ", spacerWidth))

	var badge string
	if showBadge {
		badge = headerStyle(theme.MV.AccentOrange, mode, colourless).Bold(true).Render(editModeIndicator)
	} else {
		// Reserve the badge's slot as blank so the header is the same width either way.
		badge = headerCanvasBg(mode, colourless).Render(strings.Repeat(" ", badgeWidth))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, left, spacer, badge)
}

// editModalBodyRows assembles the three field blocks (NAME input / ALIASES chips /
// TAGS chips), each a label row followed by its control rows, separated by a single
// blank spacer row (the body's vertical rhythm). Every appended element is a SINGLE
// line — the multi-row boxes are split into their constituent rows so the joined
// panel's per-row inset + side borders wrap each line exactly (a multi-line content
// "row" would defeat panelInsetRow's single-line padding and let the box overrun the
// panel frame).
func (m Model) editModalBodyRows(mode theme.Mode, colourless bool) []string {
	var rows []string
	rows = append(rows, m.editFieldLabelRow(editLabelName, editFieldName, mode, colourless))
	rows = append(rows, m.editNameInputRows(mode, colourless)...)
	rows = append(rows, editFieldGap)
	rows = append(rows, m.editFieldLabelRow(editLabelAliases, editFieldAliases, mode, colourless))
	rows = append(rows, m.editChipFieldRows(editFieldAliases, m.editAliases, m.editAliasCursor, mode, colourless)...)
	rows = append(rows, editFieldGap)
	rows = append(rows, m.editFieldLabelRow(editLabelTags, editFieldTags, mode, colourless))
	rows = append(rows, m.editChipFieldRows(editFieldTags, m.editTags, m.editTagCursor, mode, colourless)...)
	return rows
}

// editFieldLabelRow renders a field label (§13.1): the focused field's label in
// accent.violet, the others in text.detail.
func (m Model) editFieldLabelRow(label string, field editField, mode theme.Mode, colourless bool) string {
	token := theme.MV.TextDetail
	if m.editFocus == field {
		token = theme.MV.AccentViolet
	}
	return headerStyle(token, mode, colourless).Render(label)
}

// editNameInputRows renders the NAME input as a ROUNDED bordered box (§13.1): grey
// border when unfocused, accent.violet when focused (navigate), accent.orange + a
// live cursor when editing. The value is the persisted name in navigate mode, the
// live editBuffer while editing.
func (m Model) editNameInputRows(mode theme.Mode, colourless bool) []string {
	focused := m.editFocus == editFieldName
	editing := focused && m.editMode == editModeEdit

	var content string
	switch {
	case editing:
		content = renderEditableValue(m.editBuffer, m.editCursor, mode, colourless)
	default:
		content = headerStyle(theme.MV.TextPrimary, mode, colourless).Render(m.editName)
	}
	return renderInputBox(content, boxStateFor(focused, editing), true, editNameInnerWidth, mode, colourless)
}

// boxStateFor resolves a bordered box's state (§13.1) from its focus/edit flags —
// shared by the NAME input and every chip: editing wins (orange + cursor), then
// focused (violet), else idle/normal (grey).
func boxStateFor(focused, editing bool) inputBoxState {
	switch {
	case editing:
		return inputBoxEditing
	case focused:
		return inputBoxFocused
	default:
		return inputBoxIdle
	}
}

// editChipFieldRows renders one chip field as THREE single-line rows (top edges /
// values + `+ add` / bottom edges): each chip a SQUARE bordered box (grey/violet/
// orange per state, no fill, no ✕) laid out horizontally, followed by the faint
// `+ add` slot. A zero-chip field shows only the `+ add` slot. The one live element
// (a focused chip being edited, or a brand-new chip on the + add slot) reads the
// editBuffer. Returning three discrete single-line rows (rather than one multi-line
// string) lets the joined panel inset + border-wrap each line so the boxes stay
// inside the panel frame.
func (m Model) editChipFieldRows(field editField, chips []string, cursor int, mode theme.Mode, colourless bool) []string {
	focused := m.editFocus == field
	editing := focused && m.editMode == editModeEdit

	// Each segment is a 3-element slice (top / middle / bottom row of one box or the
	// + add slot). The rows are then transposed and joined column-wise into 3 lines.
	segments := make([][]string, 0, len(chips)+2)
	for i, chip := range chips {
		chipFocused := focused && cursor == i && m.editMode == editModeNavigate
		chipEditing := editing && cursor == i && !m.editIsNewChip
		// While editing an existing chip its box shows the live editBuffer (the
		// in-progress text), not the persisted value.
		value := chip
		if chipEditing {
			value = m.editBuffer
		}
		segments = append(segments, m.chipBoxRows(value, chipFocused, chipEditing, mode, colourless))
	}

	// A brand-new chip being edited (spawned on the + add slot) renders as an extra
	// editing chip box BEFORE the + add slot, showing the live editBuffer.
	if editing && m.editIsNewChip && cursor == len(chips) {
		segments = append(segments, m.chipBoxRows(m.editBuffer, false, true, mode, colourless))
	}

	addFocused := focused && cursor == len(chips) && m.editMode == editModeNavigate
	segments = append(segments, m.addSlotRows(addFocused, mode, colourless))
	return joinChipRowBands(segments, mode, colourless)
}

// chipBoxRows renders one chip as its three constituent box rows (top edge / value
// row / bottom edge), in the state's border colour (no fill, no ✕). When editing,
// the chip's value carries a live cursor.
func (m Model) chipBoxRows(value string, focused, editing bool, mode theme.Mode, colourless bool) []string {
	content := editChipContent(value, editing, m.editCursor, mode, colourless)
	return renderInputBox(content, boxStateFor(focused, editing), false, -1, mode, colourless)
}

// addSlotRows renders the trailing `+ add` slot as three rows — inline faint text
// (text.faint) on the MIDDLE row so it aligns with the chip boxes' value row, blank
// canvas rows above and below. It is NOT a bordered box (the reference shows a bare
// faint slot). When focused in navigate mode it shows in accent.violet so the cursor
// is visible on it.
func (m Model) addSlotRows(focused bool, mode theme.Mode, colourless bool) []string {
	token := theme.MV.TextFaint
	if focused {
		token = theme.MV.AccentViolet
	}
	slot := headerStyle(token, mode, colourless).Render(editAddSlot)
	width := lipgloss.Width(slot)
	blank := headerCanvasBg(mode, colourless).Render(strings.Repeat(" ", width))
	return []string{blank, slot, blank}
}

// joinChipRowBands transposes the per-segment 3-row slices into three single-line
// rows (top band / middle band / bottom band), inserting a single canvas-painted
// spacer column between adjacent segments on every band so the chips align with a
// one-cell gap.
func joinChipRowBands(segments [][]string, mode theme.Mode, colourless bool) []string {
	if len(segments) == 0 {
		return nil
	}
	gap := headerCanvasBg(mode, colourless).Render(" ")
	bands := make([]string, 3)
	for band := 0; band < 3; band++ {
		parts := make([]string, 0, len(segments)*2-1)
		for i, seg := range segments {
			if i > 0 {
				parts = append(parts, gap)
			}
			parts = append(parts, seg[band])
		}
		bands[band] = lipgloss.JoinHorizontal(lipgloss.Top, parts...)
	}
	return bands
}

// editModalFooterRow renders the contextual footer for the current mode/focus
// (§13.1): key glyphs in accent.blue, labels in text.detail. The ⏎ glyph is U+23CE
// (NOT the legacy ↵); ⇥ tab, ←→ arrows, esc.
//
// The editing-in-place footer is the one exception to the left-packed ` · ` layout:
// its trailing `empty on save = delete` consequence note is RIGHT-aligned to the far
// right of the panel content width (matching the reference frame), using the same
// fixed-content-width row + flexible-spacer technique as the header badge. The
// name-focused and chip-focused footers stay left-packed.
func (m Model) editModalFooterRow(mode theme.Mode, colourless bool) string {
	groups := m.editFooterGroups()
	if m.editMode == editModeEdit {
		return m.editModalEditingFooterRow(groups, mode, colourless)
	}
	return joinFooterGroups(groups, mode, colourless)
}

// editModalEditingFooterRow lays out the editing-in-place footer as a fixed
// full-content-width row: the left hint group (` · `-joined as usual) left-aligned,
// then a flexible canvas spacer, then the consequence note (the trailing key-less
// group) RIGHT-aligned in the far corner — the SAME spacer technique as
// editModalHeaderRow, using the same editPanelContentWidth() so the footer pins the
// note to the panel's right edge without changing the panel width.
func (m Model) editModalEditingFooterRow(groups []footerHintGroup, mode theme.Mode, colourless bool) string {
	left, right := splitConsequenceGroup(groups)

	leftSeg := joinFooterGroups(left, mode, colourless)
	rightSeg := joinFooterGroups(right, mode, colourless)

	width := m.editPanelContentWidth()
	spacerWidth := width - lipgloss.Width(leftSeg) - lipgloss.Width(rightSeg)
	if spacerWidth < 0 {
		spacerWidth = 0
	}
	spacer := headerCanvasBg(mode, colourless).Render(strings.Repeat(" ", spacerWidth))
	return lipgloss.JoinHorizontal(lipgloss.Top, leftSeg, spacer, rightSeg)
}

// splitConsequenceGroup partitions the editing footer groups into the left hint
// group and the trailing key-less consequence group (`empty on save = delete`). The
// consequence note is the sole right-aligned group; everything before it stays in the
// left-packed group. A footer with no trailing key-less group returns an empty right
// partition (the left-packed render is unchanged).
func splitConsequenceGroup(groups []footerHintGroup) (left, right []footerHintGroup) {
	if n := len(groups); n > 0 && groups[n-1].key == "" {
		return groups[:n-1], groups[n-1:]
	}
	return groups, nil
}

// joinFooterGroups renders the given footer hint groups left-packed, ` · `-joined —
// the standard footer layout shared by every footer variant (and the left/right
// partitions of the editing footer).
func joinFooterGroups(groups []footerHintGroup, mode theme.Mode, colourless bool) string {
	if len(groups) == 0 {
		return ""
	}
	rendered := make([]string, 0, len(groups))
	for _, g := range groups {
		rendered = append(rendered, renderBlueKeyHint(g.key, g.label, mode, colourless))
	}
	sep := headerStyle(theme.MV.TextDetail, mode, colourless).Render(footerEntrySeparator)
	parts := make([]string, 0, len(rendered)*2-1)
	for i, r := range rendered {
		if i > 0 {
			parts = append(parts, sep)
		}
		parts = append(parts, r)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

// editFooterGroups resolves the contextual footer hint groups for the current
// mode/focus (§13.1):
//
//   - Name focused (navigate):  ⏎/e edit · ⇥ next field · esc close
//   - Chip focused (navigate):  ⏎/e edit · x remove · ←→ move · ⇥ next field · esc close
//   - Editing in place:         ⏎ save · esc discard · ←→ cursor · empty on save = delete
//
// The + add slot (a chip field but not a removable chip) uses the name-focused
// variant — there is nothing to remove or move.
func (m Model) editFooterGroups() []footerHintGroup {
	if m.editMode == editModeEdit {
		return []footerHintGroup{
			{"⏎", "save"},
			{"esc", "discard"},
			{"←→", "cursor"},
			{"", "empty on save = delete"},
		}
	}
	if m.focusedOnChip() {
		return []footerHintGroup{
			{"⏎/e", "edit"},
			{"x", "remove"},
			{"←→", "move"},
			{"⇥", "next field"},
			{"esc", "close"},
		}
	}
	return []footerHintGroup{
		{"⏎/e", "edit"},
		{"⇥", "next field"},
		{"esc", "close"},
	}
}
