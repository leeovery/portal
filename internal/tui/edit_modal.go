package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/theme"
)

const (
	editHeaderPrefix  = "Edit Project "
	editModeIndicator = "◉ EDIT MODE"

	editLabelName    = "NAME"
	editLabelAliases = "ALIASES"
	editLabelTags    = "TAGS"

	editAddSlot = "+ add"

	// Also anchors the panel's body width, so the panel size is stable
	// regardless of value length.
	editNameInnerWidth = 56

	editChipPadX = 1

	editFieldGap = ""
)

type inputBoxState int

const (
	inputBoxIdle inputBoxState = iota
	inputBoxFocused
	inputBoxEditing
)

func inputBoxBorderToken(state inputBoxState, th theme.Theme) theme.Token {
	switch state {
	case inputBoxFocused:
		return th.AccentPrimary
	case inputBoxEditing:
		return th.AccentAttention
	default:
		return th.Border
	}
}

// No fill, by design: a border glyph owns a full cell with one background, so a
// fill either leaves a half-cell gap or bleeds past it — state rides the border.
func renderInputBox(content string, state inputBoxState, rounded bool, innerWidth int, th theme.Theme, colourless bool) []string {
	border := lipgloss.NormalBorder()
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
		style = style.BorderForeground(inputBoxBorderToken(state, th).Color())
	}
	return strings.Split(style.Render(content), "\n")
}

func editChipContent(value string, editing bool, cursor int, th theme.Theme, colourless bool) string {
	if editing {
		return renderEditableValue(value, cursor, th, colourless)
	}
	return headerStyle(th.TextPrimary, th, colourless).Render(value)
}

// Rendered width is always len(value)+1 — a mid cursor overlays a rune and
// appends a trailing blank — so the box never resizes as the cursor moves. The
// reverse-video block survives NO_COLOR as the editing signal.
func renderEditableValue(value string, cursor int, th theme.Theme, colourless bool) string {
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
	trailing := ""
	if cursor < len(runes) {
		cursorGlyph = string(runes[cursor])
		after = string(runes[cursor+1:])
		trailing = " "
	}

	textStyle := headerStyle(th.TextPrimary, th, colourless)
	cursorStyle := lipgloss.NewStyle().Reverse(true)
	if !colourless {
		cursorStyle = cursorStyle.
			Foreground(th.AccentAttention.Color()).
			Background(th.Canvas.Color())
	}

	var b strings.Builder
	b.WriteString(textStyle.Render(before))
	b.WriteString(cursorStyle.Render(cursorGlyph))
	b.WriteString(textStyle.Render(after))
	if trailing != "" {
		b.WriteString(headerCanvasBg(th, colourless).Render(trailing))
	}
	return b.String()
}

func (m Model) renderEditProjectContent() string {
	th := m.themeState.active
	return renderJoinedPanel(m.editModalLayout().compartments, th.Border, th, m.colourless)
}

// editModalLayout is the modal's panel: its title and key hints hold still,
// and its fields scroll.
func (m Model) editModalLayout() panelLayout {
	th, colourless := m.themeState.active, m.colourless

	header := []string{m.editModalHeaderRow(th, colourless)}
	body := m.editModalBodyRows(th, colourless)
	footer := []string{m.editModalFooterRow(th, colourless)}

	return panelLayout{compartments: [][]string{header, body, footer}, head: 1, foot: 1}
}

// editModalOffset is how far the fields scroll for room rows of them to show
// the focused field whole, or its top where it's taller than the room: as
// little as that takes, from the top.
func (m Model) editModalOffset(room int) int {
	fields := m.editModalFields(m.themeState.active, m.colourless)
	start := 0
	for _, f := range fields[:m.editFocus] {
		start += len(f) + 1
	}
	end := start + len(fields[m.editFocus])
	return min(start, max(end-room, 0))
}

// Computed from state-independent constants so toggling navigate↔edit never
// resizes the panel.
func (m Model) editPanelContentWidth() int {
	w := editNameInnerWidth + 2
	if hw := editHeaderNaturalWidth(m.editProject.Name); hw > w {
		w = hw
	}
	if fw := editFooterWidestWidth(); fw > w {
		w = fw
	}
	return w
}

func editHeaderNaturalWidth(name string) int {
	return lipgloss.Width(editHeaderPrefix) + lipgloss.Width(name) +
		editHeaderBadgeGap + lipgloss.Width(editModeIndicator)
}

func editFooterWidestWidth() int {
	return lipgloss.Width(strings.Join([]string{
		"⏎ save", "esc discard", "←→ cursor", "empty on save = delete",
	}, footerEntrySeparator))
}

const editHeaderBadgeGap = 3

func (m Model) editModalHeaderRow(th theme.Theme, colourless bool) string {
	prefix := headerStyle(th.TextPrimary, th, colourless).Bold(true).Render(editHeaderPrefix)
	name := headerStyle(th.TextMuted, th, colourless).Render(m.editProject.Name)
	left := lipgloss.JoinHorizontal(lipgloss.Top, prefix, name)
	return renderHeaderWithBadge(left, m.editPanelContentWidth(), m.editMode == editModeEdit, editModeIndicator, th, colourless)
}

// A hidden badge renders as a same-width blank so toggling never resizes the panel.
func renderHeaderWithBadge(left string, contentWidth int, showBadge bool, badgeText string, th theme.Theme, colourless bool) string {
	leftWidth := lipgloss.Width(left)
	badgeWidth := lipgloss.Width(badgeText)
	spacerWidth := max(contentWidth-leftWidth-badgeWidth, 0)
	spacer := headerCanvasBg(th, colourless).Render(strings.Repeat(" ", spacerWidth))

	var badge string
	if showBadge {
		badge = headerStyle(th.AccentAttention, th, colourless).Bold(true).Render(badgeText)
	} else {
		badge = headerCanvasBg(th, colourless).Render(strings.Repeat(" ", badgeWidth))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, left, spacer, badge)
}

// Every appended element must be a single line — a multi-line row would defeat
// panelInsetRow's per-line padding and overrun the panel frame.
func (m Model) editModalBodyRows(th theme.Theme, colourless bool) []string {
	fields := m.editModalFields(th, colourless)
	rows := fields[0]
	for _, f := range fields[1:] {
		rows = append(rows, editFieldGap)
		rows = append(rows, f...)
	}
	return rows
}

// editModalFields are each field's rows, its label's and its box's, indexed
// by editField.
func (m Model) editModalFields(th theme.Theme, colourless bool) [][]string {
	return [][]string{
		editFieldName: append([]string{m.editFieldLabelRow(editLabelName, editFieldName, th, colourless)},
			m.editNameInputRows(th, colourless)...),
		editFieldAliases: append([]string{m.editFieldLabelRow(editLabelAliases, editFieldAliases, th, colourless)},
			m.editChipFieldRows(editFieldAliases, m.editAliases, m.editAliasCursor, th, colourless)...),
		editFieldTags: append([]string{m.editFieldLabelRow(editLabelTags, editFieldTags, th, colourless)},
			m.editChipFieldRows(editFieldTags, m.editTags, m.editTagCursor, th, colourless)...),
	}
}

func (m Model) editFieldLabelRow(label string, field editField, th theme.Theme, colourless bool) string {
	token := th.TextMuted
	if m.editFocus == field {
		token = th.AccentPrimary
	}
	return headerStyle(token, th, colourless).Render(label)
}

func (m Model) editNameInputRows(th theme.Theme, colourless bool) []string {
	focused := m.editFocus == editFieldName
	editing := focused && m.editMode == editModeEdit

	var content string
	switch {
	case editing:
		content = renderEditableValue(m.editBuffer, m.editCursor, th, colourless)
	default:
		content = headerStyle(th.TextPrimary, th, colourless).Render(m.editName)
	}
	return renderInputBox(content, boxStateFor(focused, editing), true, editNameInnerWidth, th, colourless)
}

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

func (m Model) editChipFieldRows(field editField, chips []string, cursor int, th theme.Theme, colourless bool) []string {
	focused := m.editFocus == field
	editing := focused && m.editMode == editModeEdit

	// Each segment is a 3-row slice (top/middle/bottom of one box); the
	// segments are transposed and joined column-wise into 3 bands.
	segments := make([][]string, 0, len(chips)+2)
	for i, chip := range chips {
		chipFocused := focused && cursor == i && m.editMode == editModeNavigate
		chipEditing := editing && cursor == i && !m.editIsNewChip
		value := chip
		if chipEditing {
			value = m.editBuffer
		}
		segments = append(segments, m.chipBoxRows(value, chipFocused, chipEditing, th, colourless))
	}

	if editing && m.editIsNewChip && cursor == len(chips) {
		segments = append(segments, m.chipBoxRows(m.editBuffer, false, true, th, colourless))
	}

	addFocused := focused && cursor == len(chips) && m.editMode == editModeNavigate
	segments = append(segments, m.addSlotRows(addFocused, th, colourless))
	return joinChipRowBands(segments, th, colourless)
}

func (m Model) chipBoxRows(value string, focused, editing bool, th theme.Theme, colourless bool) []string {
	content := editChipContent(value, editing, m.editCursor, th, colourless)
	return renderInputBox(content, boxStateFor(focused, editing), false, -1, th, colourless)
}

func (m Model) addSlotRows(focused bool, th theme.Theme, colourless bool) []string {
	token := th.TextFaint
	if focused {
		token = th.AccentPrimary
	}
	slot := headerStyle(token, th, colourless).Render(editAddSlot)
	width := lipgloss.Width(slot)
	blank := headerCanvasBg(th, colourless).Render(strings.Repeat(" ", width))
	return []string{blank, slot, blank}
}

func joinChipRowBands(segments [][]string, th theme.Theme, colourless bool) []string {
	if len(segments) == 0 {
		return nil
	}
	gap := headerCanvasBg(th, colourless).Render(" ")
	bands := make([]string, 3)
	for band := range 3 {
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

func (m Model) editModalFooterRow(th theme.Theme, colourless bool) string {
	groups := m.editFooterGroups()
	if m.editMode == editModeEdit {
		return m.editModalEditingFooterRow(groups, th, colourless)
	}
	return joinFooterGroups(groups, th, colourless)
}

func (m Model) editModalEditingFooterRow(groups []footerHintGroup, th theme.Theme, colourless bool) string {
	left, right := splitConsequenceGroup(groups)

	leftSeg := joinFooterGroups(left, th, colourless)
	rightSeg := joinFooterGroups(right, th, colourless)

	width := m.editPanelContentWidth()
	spacerWidth := max(width-lipgloss.Width(leftSeg)-lipgloss.Width(rightSeg), 0)
	spacer := headerCanvasBg(th, colourless).Render(strings.Repeat(" ", spacerWidth))
	return lipgloss.JoinHorizontal(lipgloss.Top, leftSeg, spacer, rightSeg)
}

func splitConsequenceGroup(groups []footerHintGroup) (left, right []footerHintGroup) {
	if n := len(groups); n > 0 && groups[n-1].key == "" {
		return groups[:n-1], groups[n-1:]
	}
	return groups, nil
}

func joinFooterGroups(groups []footerHintGroup, th theme.Theme, colourless bool) string {
	if len(groups) == 0 {
		return ""
	}
	rendered := make([]string, 0, len(groups))
	for _, g := range groups {
		rendered = append(rendered, renderBlueKeyHint(g.key, g.label, th, colourless))
	}
	sep := headerStyle(th.TextMuted, th, colourless).Render(footerEntrySeparator)
	parts := make([]string, 0, len(rendered)*2-1)
	for i, r := range rendered {
		if i > 0 {
			parts = append(parts, sep)
		}
		parts = append(parts, r)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

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
