package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/tui/theme"
)

// editModalStrip returns the rendered edit-project modal with ANSI stripped — for
// content/structure presence checks (glyph order, labels, footer copy).
func editModalStrip(m Model) string {
	return ansi.Strip(m.renderEditProjectContent())
}

// reverseBlockPresent reports whether the rendered content carries a reverse-video
// block (SGR 7) — the live text cursor's block, asserted the same way the rename
// modal test asserts its block cursor.
func reverseBlockPresent(content string) bool {
	return strings.Contains(content, "\x1b[7m") ||
		strings.Contains(content, ";7m") ||
		strings.Contains(content, "[7;") ||
		strings.Contains(content, "7;")
}

// editModalModel builds an edit-project modal Model in navigate mode with the given
// focus + chips, mirroring the reference data (project flow-v1-api, aliases
// [fapi, v1], tags [Fabric, api]) unless overridden.
func editModalModel(focus editField, aliasCur, tagCur int) Model {
	return Model{
		modal:           modalEditProject,
		editProject:     project.Project{Name: "flow-v1-api"},
		editMode:        editModeNavigate,
		editFocus:       focus,
		editName:        "flow-v1-api",
		editAliases:     []string{"fapi", "v1"},
		editTags:        []string{"Fabric", "api"},
		editAliasCursor: aliasCur,
		editTagCursor:   tagCur,
	}
}

// TestEditModal_Header asserts the `Edit Project <name>` header: "Edit Project" in
// text.primary, the project name in text.detail.
func TestEditModal_Header(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		m := editModalModel(editFieldName, 0, 0)
		content := m.renderEditProjectContent()
		if !strings.Contains(ansi.Strip(content), "Edit Project flow-v1-api") {
			t.Errorf("[%v] header must read 'Edit Project flow-v1-api'; got:\n%s", mode, editModalStrip(m))
		}
		m.canvasMode = mode
		content = m.renderEditProjectContent()
		if seq := tokenFgSeq(t, theme.MV.TextPrimary, mode); !strings.Contains(content, seq) {
			t.Errorf("[%v] 'Edit Project' must render in text.primary SGR core %q", mode, seq)
		}
		if seq := tokenFgSeq(t, theme.MV.TextDetail, mode); !strings.Contains(content, seq) {
			t.Errorf("[%v] header <name> must render in text.detail SGR core %q", mode, seq)
		}
	}
}

// TestEditModal_SingleBundledModal asserts ONE modal carries all three fields
// (NAME + ALIASES + TAGS), in order.
func TestEditModal_SingleBundledModal(t *testing.T) {
	m := editModalModel(editFieldName, 0, 0)
	out := editModalStrip(m)
	nameIdx := strings.Index(out, "NAME")
	aliasIdx := strings.Index(out, "ALIASES")
	tagIdx := strings.Index(out, "TAGS")
	if nameIdx < 0 || aliasIdx < 0 || tagIdx < 0 {
		t.Fatalf("modal must carry NAME, ALIASES and TAGS labels; got:\n%s", out)
	}
	if nameIdx >= aliasIdx || aliasIdx >= tagIdx {
		t.Errorf("labels must render NAME → ALIASES → TAGS in order; got:\n%s", out)
	}
}

// TestEditModal_FocusedFieldLabelViolet asserts the focused field's label renders
// in accent.violet while the others render in text.detail.
func TestEditModal_FocusedFieldLabelViolet(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		violet := tokenFgSeq(t, theme.MV.AccentViolet, mode)
		detail := tokenFgSeq(t, theme.MV.TextDetail, mode)

		// Name focused: the NAME label segment carries violet.
		m := editModalModel(editFieldName, 0, 0)
		m.canvasMode = mode
		if seg := labelSegment(t, m.renderEditProjectContent(), "NAME"); !strings.Contains(seg, violet) {
			t.Errorf("[%v] focused NAME label must be accent.violet; seg=%q", mode, seg)
		}
		// And the unfocused ALIASES label is text.detail (not violet).
		if seg := labelSegment(t, m.renderEditProjectContent(), "ALIASES"); !strings.Contains(seg, detail) || strings.Contains(seg, violet) {
			t.Errorf("[%v] unfocused ALIASES label must be text.detail (not violet); seg=%q", mode, seg)
		}

		// Tags focused: TAGS violet, NAME detail.
		mt := editModalModel(editFieldTags, 0, 0)
		mt.canvasMode = mode
		if seg := labelSegment(t, mt.renderEditProjectContent(), "TAGS"); !strings.Contains(seg, violet) {
			t.Errorf("[%v] focused TAGS label must be accent.violet; seg=%q", mode, seg)
		}
	}
}

// labelSegment returns the SGR run wrapping the field label on its line, so the
// label's own colour is asserted independent of the rest of the modal.
func labelSegment(t *testing.T, content, label string) string {
	t.Helper()
	for line := range strings.SplitSeq(content, "\n") {
		if strings.Contains(ansi.Strip(line), label) {
			return line
		}
	}
	t.Fatalf("label %q not found in content:\n%s", label, content)
	return ""
}

// TestEditModal_NameInputNeverFilled_GreyUnfocused asserts the NAME input box, when
// the Name field is NOT focused, draws a border.separator (grey) rounded box with no
// background fill.
func TestEditModal_NameInputNeverFilled_GreyUnfocused(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		m := editModalModel(editFieldAliases, 0, 0) // aliases focused → name unfocused
		m.canvasMode = mode
		content := m.renderEditProjectContent()
		assertNoFill(t, content, mode, "name-input-unfocused")
		if seq := tokenFgSeq(t, theme.MV.BorderSeparator, mode); !strings.Contains(content, seq) {
			t.Errorf("[%v] unfocused NAME box border must be border.separator (grey) SGR core %q", mode, seq)
		}
	}
}

// TestEditModal_NameInputFocusedViolet asserts the NAME input box border is
// accent.violet when the Name field is focused in navigate mode, with no fill.
func TestEditModal_NameInputFocusedViolet(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		m := editModalModel(editFieldName, 0, 0)
		m.canvasMode = mode
		content := m.renderEditProjectContent()
		assertNoFill(t, content, mode, "name-input-focused")
		if seq := tokenFgSeq(t, theme.MV.AccentViolet, mode); !strings.Contains(content, seq) {
			t.Errorf("[%v] focused NAME box border must be accent.violet SGR core %q", mode, seq)
		}
	}
}

// TestEditModal_NameInputEditingOrangeWithCursor asserts the NAME input box border
// is accent.orange + a live cursor when the Name field is being edited in place.
func TestEditModal_NameInputEditingOrangeWithCursor(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		m := editModalModel(editFieldName, 0, 0)
		m.editMode = editModeEdit
		m.editBuffer = "flow-v1-api"
		m.editCursor = len([]rune("flow-v1-api"))
		m.canvasMode = mode
		content := m.renderEditProjectContent()
		assertNoFill(t, content, mode, "name-input-editing")
		if seq := tokenFgSeq(t, theme.MV.AccentOrange, mode); !strings.Contains(content, seq) {
			t.Errorf("[%v] editing NAME box border must be accent.orange SGR core %q", mode, seq)
		}
		if !reverseBlockPresent(content) {
			t.Errorf("[%v] editing NAME input must carry a live block cursor (SGR 7)", mode)
		}
	}
}

// TestEditModal_ChipNormalGreyNoCross asserts a normal (non-focused) chip is a
// border.separator (grey) bordered box, never filled, never green, with NO inline ✕.
func TestEditModal_ChipNormalGreyNoCross(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		// Name focused so the chips are unfocused/normal.
		m := editModalModel(editFieldName, 0, 0)
		m.canvasMode = mode
		content := m.renderEditProjectContent()
		assertNoFill(t, content, mode, "chip-normal")
		assertNoCross(t, content)
		assertNoGreen(t, content, mode)
		if seq := tokenFgSeq(t, theme.MV.BorderSeparator, mode); !strings.Contains(content, seq) {
			t.Errorf("[%v] normal chip border must be border.separator (grey) SGR core %q", mode, seq)
		}
		// Chips render in text.primary.
		if seq := tokenFgSeq(t, theme.MV.TextPrimary, mode); !strings.Contains(content, seq) {
			t.Errorf("[%v] chip text must be text.primary SGR core %q", mode, seq)
		}
	}
}

// TestEditModal_ChipFocusedVioletNoCross asserts a focused chip is an accent.violet
// bordered box, never filled, no ✕.
func TestEditModal_ChipFocusedVioletNoCross(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		m := editModalModel(editFieldTags, 0, 0) // Tags focused, cursor on chip 0 (Fabric)
		m.canvasMode = mode
		content := m.renderEditProjectContent()
		assertNoFill(t, content, mode, "chip-focused")
		assertNoCross(t, content)
		assertNoGreen(t, content, mode)
		if seq := tokenFgSeq(t, theme.MV.AccentViolet, mode); !strings.Contains(content, seq) {
			t.Errorf("[%v] focused chip border must be accent.violet SGR core %q", mode, seq)
		}
	}
}

// TestEditModal_ChipEditingOrangeCursorNoCross asserts an editing chip is an
// accent.orange bordered box + live cursor, no ✕.
func TestEditModal_ChipEditingOrangeCursorNoCross(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		m := editModalModel(editFieldTags, 0, 0)
		m.editMode = editModeEdit
		m.editBuffer = "Fabric"
		m.editCursor = len([]rune("Fabric"))
		m.canvasMode = mode
		content := m.renderEditProjectContent()
		assertNoFill(t, content, mode, "chip-editing")
		assertNoCross(t, content)
		assertNoGreen(t, content, mode)
		if seq := tokenFgSeq(t, theme.MV.AccentOrange, mode); !strings.Contains(content, seq) {
			t.Errorf("[%v] editing chip border must be accent.orange SGR core %q", mode, seq)
		}
		if !reverseBlockPresent(content) {
			t.Errorf("[%v] editing chip must carry a live block cursor (SGR 7)", mode)
		}
	}
}

// TestEditModal_AddSlotFaint asserts the trailing `+ add` slot renders in text.faint.
func TestEditModal_AddSlotFaint(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		m := editModalModel(editFieldName, 0, 0)
		m.canvasMode = mode
		content := m.renderEditProjectContent()
		if !strings.Contains(ansi.Strip(content), "+ add") {
			t.Errorf("[%v] modal must render a `+ add` slot; got:\n%s", mode, ansi.Strip(content))
		}
		seg := addSlotSegment(t, content)
		if seq := tokenFgSeq(t, theme.MV.TextFaint, mode); !strings.Contains(seg, seq) {
			t.Errorf("[%v] `+ add` slot must be text.faint SGR core %q; seg=%q", mode, seq, seg)
		}
	}
}

// addSlotSegment returns the SGR run wrapping a `+ add` slot.
func addSlotSegment(t *testing.T, content string) string {
	t.Helper()
	for line := range strings.SplitSeq(content, "\n") {
		if strings.Contains(ansi.Strip(line), "+ add") {
			return line
		}
	}
	t.Fatalf("`+ add` slot not found in content:\n%s", content)
	return ""
}

// TestEditModal_EditModeIndicatorOnlyWhileEditing asserts `◉ EDIT MODE`
// (accent.orange) renders ONLY while editing, and is absent in navigate mode.
func TestEditModal_EditModeIndicatorOnlyWhileEditing(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		// Navigate: no indicator.
		nav := editModalModel(editFieldName, 0, 0)
		nav.canvasMode = mode
		if strings.Contains(ansi.Strip(nav.renderEditProjectContent()), "EDIT MODE") {
			t.Errorf("[%v] navigate mode must NOT show `◉ EDIT MODE`", mode)
		}

		// Editing: indicator present in accent.orange.
		ed := editModalModel(editFieldTags, 0, 0)
		ed.editMode = editModeEdit
		ed.editBuffer = "Fabric"
		ed.canvasMode = mode
		content := ed.renderEditProjectContent()
		if !strings.Contains(ansi.Strip(content), "◉ EDIT MODE") {
			t.Errorf("[%v] editing must show `◉ EDIT MODE`; got:\n%s", mode, ansi.Strip(content))
		}
		seg := labelSegment(t, content, "EDIT MODE")
		if seq := tokenFgSeq(t, theme.MV.AccentOrange, mode); !strings.Contains(seg, seq) {
			t.Errorf("[%v] `◉ EDIT MODE` must be accent.orange SGR core %q; seg=%q", mode, seq, seg)
		}
	}
}

// TestEditModal_EditModeBadgeRightAligned asserts the `◉ EDIT MODE` badge renders
// in the header's RIGHT corner (right-aligned) while editing — not inline after the
// title. The badge text must be the LAST non-blank glyph run on the header line, and
// the `Edit Project` title must precede it with a gap, so the header spans the full
// panel width. In navigate the badge is absent (its slot blank).
func TestEditModal_EditModeBadgeRightAligned(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		m := editModalModel(editFieldTags, 0, 0)
		m.editMode = editModeEdit
		m.editBuffer = "Fabric"
		m.canvasMode = mode

		headerLine := headerLineOf(t, m.renderEditProjectContent())
		// The badge sits at the right: after trimming the panel's right border/inset,
		// `◉ EDIT MODE` must be the trailing text, and a run of spaces must separate it
		// from the `Edit Project flow-v1-api` title (so it is far-right, not inline).
		trimmed := strings.TrimRight(headerLine, " │")
		if !strings.HasSuffix(trimmed, "◉ EDIT MODE") {
			t.Errorf("[%v] `◉ EDIT MODE` must be right-aligned (trailing) in the header; got line:\n%q", mode, headerLine)
		}
		titleIdx := strings.Index(headerLine, "Edit Project flow-v1-api")
		badgeIdx := strings.Index(headerLine, "◉ EDIT MODE")
		if titleIdx < 0 || badgeIdx < 0 || badgeIdx <= titleIdx {
			t.Fatalf("[%v] header must read title then far-right badge; got:\n%q", mode, headerLine)
		}
		// A wide gap (the flexible spacer) must separate the title end from the badge —
		// confirming the badge is in the far CORNER, not packed inline right after the
		// title. The gap must clearly exceed a small inline spacer (the inline-badge bug
		// used a fixed 3-cell gap), so require the badge to sit in the right HALF of the
		// header line — its start index past the line's midpoint.
		gap := badgeIdx - (titleIdx + len("Edit Project flow-v1-api"))
		if gap < 10 {
			t.Errorf("[%v] badge must be far-right with a wide flexible gap after the title (gap=%d); got:\n%q", mode, gap, headerLine)
		}
		if badgeIdx < len(headerLine)/2 {
			t.Errorf("[%v] badge must sit in the right half of the header (corner), idx=%d lineLen=%d; got:\n%q", mode, badgeIdx, len(headerLine), headerLine)
		}
	}
}

// headerLineOf returns the (ANSI-stripped) header content line of the rendered modal
// — the line carrying the `Edit Project` title.
func headerLineOf(t *testing.T, content string) string {
	t.Helper()
	for line := range strings.SplitSeq(ansi.Strip(content), "\n") {
		if strings.Contains(line, "Edit Project") {
			return line
		}
	}
	t.Fatalf("header line not found in content:\n%s", ansi.Strip(content))
	return ""
}

// TestEditModal_PanelWidthStableAcrossModes asserts the rendered panel is the SAME
// width in navigate and edit states (name-edit AND chip-edit) — entering edit mode
// must NOT resize the modal (the "jaggedy" resize bug). The panel width is the
// width of any frame line (all are equal by construction).
func TestEditModal_PanelWidthStableAcrossModes(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		nav := editModalModel(editFieldName, 0, 0)
		nav.canvasMode = mode
		navWidth := lipgloss.Width(nav.renderEditProjectContent())

		// Editing the NAME field.
		nameEdit := editModalModel(editFieldName, 0, 0)
		nameEdit.editMode = editModeEdit
		nameEdit.editBuffer = "flow-v1-api"
		nameEdit.editCursor = len([]rune("flow-v1-api"))
		nameEdit.canvasMode = mode
		if w := lipgloss.Width(nameEdit.renderEditProjectContent()); w != navWidth {
			t.Errorf("[%v] name-edit panel width %d != navigate width %d (entering edit must not resize)", mode, w, navWidth)
		}

		// Editing a CHIP (Tags).
		chipEdit := editModalModel(editFieldTags, 0, 0)
		chipEdit.editMode = editModeEdit
		chipEdit.editBuffer = "Fabric"
		chipEdit.editCursor = len([]rune("Fabric"))
		chipEdit.canvasMode = mode
		if w := lipgloss.Width(chipEdit.renderEditProjectContent()); w != navWidth {
			t.Errorf("[%v] chip-edit panel width %d != navigate width %d (entering edit must not resize)", mode, w, navWidth)
		}

		// A chip mid-string cursor must not change the width either.
		chipMid := editModalModel(editFieldTags, 0, 0)
		chipMid.editMode = editModeEdit
		chipMid.editBuffer = "Fabric"
		chipMid.editCursor = 2 // cursor over a mid-value rune
		chipMid.canvasMode = mode
		if w := lipgloss.Width(chipMid.renderEditProjectContent()); w != navWidth {
			t.Errorf("[%v] chip-edit (mid cursor) panel width %d != navigate width %d", mode, w, navWidth)
		}
	}
}

// TestEditModal_NameFocusedFooter asserts the Name-focused (navigate) footer copy.
func TestEditModal_NameFocusedFooter(t *testing.T) {
	m := editModalModel(editFieldName, 0, 0)
	out := editModalStrip(m)
	want := "⏎/e edit · ⇥ next field · esc close"
	if !strings.Contains(out, want) {
		t.Errorf("name-focused footer must read %q; got:\n%s", want, out)
	}
}

// TestEditModal_ChipFocusedFooter asserts the chip-focused (navigate) footer copy.
func TestEditModal_ChipFocusedFooter(t *testing.T) {
	m := editModalModel(editFieldTags, 0, 0)
	out := editModalStrip(m)
	want := "⏎/e edit · x remove · ←→ move · ⇥ next field · esc close"
	if !strings.Contains(out, want) {
		t.Errorf("chip-focused footer must read %q; got:\n%s", want, out)
	}
}

// TestEditModal_AddSlotFocusedUsesChipFooterMinusRemove asserts that focusing the
// + add slot (a chip field, but not a removable chip) shows the name-focused-style
// footer (no `x remove` / `←→ move`), since there is nothing to remove or move.
func TestEditModal_AddSlotFocusedFooter(t *testing.T) {
	// Tags focused, cursor on the trailing + add slot (index == len(tags)).
	m := editModalModel(editFieldTags, 0, 2)
	out := editModalStrip(m)
	want := "⏎/e edit · ⇥ next field · esc close"
	if !strings.Contains(out, want) {
		t.Errorf("add-slot-focused footer must read %q; got:\n%s", want, out)
	}
	if strings.Contains(out, "x remove") {
		t.Errorf("add-slot-focused footer must NOT carry `x remove`; got:\n%s", out)
	}
}

// TestEditModal_EditingFooter asserts the editing-in-place footer carries the left
// hint group and the consequence note (the layout/alignment is asserted separately
// by TestEditModal_EditingFooterConsequenceRightAligned).
func TestEditModal_EditingFooter(t *testing.T) {
	m := editModalModel(editFieldTags, 0, 0)
	m.editMode = editModeEdit
	m.editBuffer = "Fabric"
	out := editModalStrip(m)
	for _, want := range []string{
		"⏎ save · esc discard · ←→ cursor",
		"empty on save = delete",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("editing footer must contain %q; got:\n%s", want, out)
		}
	}
}

// TestEditModal_EditingFooterConsequenceRightAligned asserts the editing-in-place
// footer pins the `empty on save = delete` consequence note to the far RIGHT of the
// panel's content width (matching the reference frame), separated by a flexible
// spacer from the left-aligned ` · `-joined hint group. The left group starts at the
// left edge of the footer content; the consequence note ends at the right edge; and
// the spacer between them is wider than the inline ` · ` separator (proving the note
// is right-aligned, not just the last left-packed group). Measured in display cells,
// not bytes — the ⏎ / ←→ glyphs are multi-byte.
func TestEditModal_EditingFooterConsequenceRightAligned(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		m := editModalModel(editFieldTags, 0, 0)
		m.editMode = editModeEdit
		m.editBuffer = "Fabric"
		m.canvasMode = mode

		footerLine := footerLineOf(t, m.renderEditProjectContent())
		leftGroup := "⏎ save · esc discard · ←→ cursor"
		consequence := "empty on save = delete"

		// The consequence note must be the trailing content (after trimming the panel's
		// side border/inset) — i.e. right-aligned to the panel's content edge.
		if trimmed := strings.TrimRight(footerLine, " │"); !strings.HasSuffix(trimmed, consequence) {
			t.Errorf("[%v] consequence note must be right-aligned (trailing) in the footer; got line:\n%q", mode, footerLine)
		}
		// The left group must lead the footer content (before the consequence note).
		if !strings.Contains(footerLine, leftGroup) {
			t.Fatalf("[%v] footer must carry the left hint group %q; got:\n%q", mode, leftGroup, footerLine)
		}
		if li, ci := strings.Index(footerLine, leftGroup), strings.Index(footerLine, consequence); ci <= li {
			t.Fatalf("[%v] footer must read left group then far-right consequence note; got:\n%q", mode, footerLine)
		}
		// The spacer between the left group and the consequence note must exceed the
		// inline ` · ` separator (3 cells) — confirming a flexible right-align gap, not a
		// left-packed last group. Measured in display cells (the body between the two
		// fixed substrings, trimmed).
		body := footerBetween(t, footerLine, leftGroup, consequence)
		if gap := lipgloss.Width(body); gap <= lipgloss.Width(footerEntrySeparator) {
			t.Errorf("[%v] spacer between left group and consequence note (%d cells) must exceed the inline separator (%d cells) — note must be right-aligned; got:\n%q",
				mode, gap, lipgloss.Width(footerEntrySeparator), footerLine)
		}
	}
}

// footerBetween returns the substring of an ANSI-stripped footer line that sits
// between the end of `left` and the start of `right`.
func footerBetween(t *testing.T, line, left, right string) string {
	t.Helper()
	li := strings.Index(line, left)
	ri := strings.Index(line, right)
	if li < 0 || ri < 0 || ri < li+len(left) {
		t.Fatalf("could not locate %q before %q in line:\n%q", left, right, line)
	}
	return line[li+len(left) : ri]
}

// footerLineOf returns the (ANSI-stripped) footer content line of the rendered modal
// — the line carrying the `⏎ save` hint.
func footerLineOf(t *testing.T, content string) string {
	t.Helper()
	for line := range strings.SplitSeq(ansi.Strip(content), "\n") {
		if strings.Contains(line, "⏎ save") {
			return line
		}
	}
	t.Fatalf("footer line not found in content:\n%s", ansi.Strip(content))
	return ""
}

// TestEditModal_FooterKeyGlyphsBlue asserts footer key glyphs render in accent.blue
// and labels in text.detail.
func TestEditModal_FooterKeyGlyphsBlue(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		m := editModalModel(editFieldName, 0, 0)
		m.canvasMode = mode
		content := m.renderEditProjectContent()
		if seq := tokenFgSeq(t, theme.MV.AccentBlue, mode); !strings.Contains(content, seq) {
			t.Errorf("[%v] footer key glyphs must render in accent.blue SGR core %q", mode, seq)
		}
		if seq := tokenFgSeq(t, theme.MV.TextDetail, mode); !strings.Contains(content, seq) {
			t.Errorf("[%v] footer labels must render in text.detail SGR core %q", mode, seq)
		}
	}
}

// TestEditModal_UsesEnterGlyphNotLegacy asserts the footer uses ⏎ (U+23CE) and never
// the legacy ↵.
func TestEditModal_UsesEnterGlyphNotLegacy(t *testing.T) {
	m := editModalModel(editFieldName, 0, 0)
	out := editModalStrip(m)
	if !strings.Contains(out, "⏎") {
		t.Errorf("footer must use ⏎ (U+23CE); got:\n%s", out)
	}
	if strings.Contains(out, "↵") {
		t.Errorf("footer must NOT use the legacy ↵ glyph; got:\n%s", out)
	}
}

// TestEditModal_NoLegacyGrammar asserts the legacy [x] / Add: / [Enter] Save
// rendering is gone.
func TestEditModal_NoLegacyGrammar(t *testing.T) {
	m := editModalModel(editFieldTags, 0, 0)
	out := editModalStrip(m)
	for _, legacy := range []string{"[x]", "Add:", "[Enter]", "(none)"} {
		if strings.Contains(out, legacy) {
			t.Errorf("legacy grammar %q must be removed; got:\n%s", legacy, out)
		}
	}
}

// TestEditModal_ZeroChipFieldOnlyAddSlot asserts a field with zero chips renders
// only the `+ add` slot (no chip boxes), and the focused-field label stays violet.
func TestEditModal_ZeroChipFieldOnlyAddSlot(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		m := Model{
			modal:       modalEditProject,
			editProject: project.Project{Name: "flow-v1-api"},
			editMode:    editModeNavigate,
			editFocus:   editFieldTags,
			editName:    "flow-v1-api",
			editAliases: []string{"fapi"},
			editTags:    nil, // zero tags
			canvasMode:  mode,
		}
		content := m.renderEditProjectContent()
		out := ansi.Strip(content)
		// The TAGS block must carry `+ add` but no chip text.
		tagsIdx := strings.Index(out, "TAGS")
		if tagsIdx < 0 {
			t.Fatalf("[%v] missing TAGS label; got:\n%s", mode, out)
		}
		tail := out[tagsIdx:]
		if !strings.Contains(tail, "+ add") {
			t.Errorf("[%v] zero-chip TAGS must still show `+ add`; got:\n%s", mode, tail)
		}
		// Focused-field label still violet.
		if seg := labelSegment(t, content, "TAGS"); !strings.Contains(seg, tokenFgSeq(t, theme.MV.AccentViolet, mode)) {
			t.Errorf("[%v] zero-chip focused TAGS label must stay accent.violet; seg=%q", mode, seg)
		}
	}
}

// TestEditModal_NewEmptyChipEditingOrangeCursor asserts a brand-new empty chip
// spawned in edit (on the + add slot) renders an orange border + cursor, no ✕.
func TestEditModal_NewEmptyChipEditingOrangeCursor(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		m := editModalModel(editFieldTags, 0, 2) // cursor on add slot
		m.editMode = editModeEdit
		m.editIsNewChip = true
		m.editBuffer = ""
		m.editCursor = 0
		m.canvasMode = mode
		content := m.renderEditProjectContent()
		assertNoCross(t, content)
		if seq := tokenFgSeq(t, theme.MV.AccentOrange, mode); !strings.Contains(content, seq) {
			t.Errorf("[%v] brand-new editing chip must have an accent.orange border", mode)
		}
		if !reverseBlockPresent(content) {
			t.Errorf("[%v] brand-new editing chip must carry a live cursor", mode)
		}
	}
}

// TestEditModal_NoColorStateViaBorderAndCursor asserts that under NO_COLOR the
// focused vs editing states are still distinguishable: both carry a border, and
// editing additionally carries the cursor + `◉ EDIT MODE` text (state never
// colour-only, §2.2).
func TestEditModal_NoColorStateViaBorderAndCursor(t *testing.T) {
	// Focused (navigate): border present, no EDIT MODE text, no cursor.
	focused := editModalModel(editFieldTags, 0, 0)
	focused.colourless = true
	fout := focused.renderEditProjectContent()
	if !strings.Contains(ansi.Strip(fout), "┌") && !strings.Contains(ansi.Strip(fout), "╭") {
		t.Errorf("NO_COLOR focused chip must still draw a box border; got:\n%s", ansi.Strip(fout))
	}
	if strings.Contains(ansi.Strip(fout), "EDIT MODE") {
		t.Errorf("NO_COLOR navigate must NOT show EDIT MODE")
	}

	// Editing: border present, EDIT MODE text present, cursor present.
	editing := editModalModel(editFieldTags, 0, 0)
	editing.editMode = editModeEdit
	editing.editBuffer = "Fabric"
	editing.editCursor = len([]rune("Fabric"))
	editing.colourless = true
	eout := editing.renderEditProjectContent()
	if !strings.Contains(ansi.Strip(eout), "◉ EDIT MODE") {
		t.Errorf("NO_COLOR editing must show `◉ EDIT MODE` text (state not colour-only); got:\n%s", ansi.Strip(eout))
	}
	if !reverseBlockPresent(eout) {
		t.Errorf("NO_COLOR editing must carry a live cursor (state not colour-only)")
	}
}

// TestEditModal_SinglePanelOnClearedCanvas asserts the placed modal renders as ONE
// hand-drawn joined panel (no redundant outer border box wrapping it). The
// single-panel render has exactly two rounded top-corners — the joined panel's own
// ╭───╮ top and the NAME input box's ╭───╮ — so a nested outer box (a third ╭) is a
// regression. The chip boxes use SQUARE corners (┌), so they never add a ╭.
func TestEditModal_SinglePanelOnClearedCanvas(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		m := editModalModel(editFieldName, 0, 0)
		m.canvasMode = mode
		placed := ansi.Strip(renderEditModalOnClearedCanvas(m, 100, 40, mode, false))
		if got := strings.Count(placed, "╭"); got != 2 {
			t.Errorf("[%v] single-panel edit modal must have exactly 2 rounded top-corners (joined panel + NAME box), got %d; a nested outer box is a regression:\n%s", mode, got, placed)
		}
	}
}

// TestEditModal_NoGreenEver asserts state.green is never used on a chip in any
// state (normal / focused / editing).
func TestEditModal_NoGreenEver(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		states := []Model{
			editModalModel(editFieldName, 0, 0), // chips normal
			editModalModel(editFieldTags, 0, 0), // a chip focused
		}
		editing := editModalModel(editFieldTags, 0, 0)
		editing.editMode = editModeEdit
		editing.editBuffer = "Fabric"
		states = append(states, editing)
		for i, m := range states {
			m.canvasMode = mode
			assertNoGreenLabelled(t, m.renderEditProjectContent(), mode, i)
		}
	}
}

// assertNoFill fails if the rendered content carries a non-canvas background fill
// inside the modal body — chips and the name input are border-only (never filled).
// A fill would show up as a background SGR (48;2;...) for a colour OTHER than the
// owned canvas. The panel/inset rows legitimately carry the canvas background, so
// only NON-canvas backgrounds are a violation.
func assertNoFill(t *testing.T, content string, mode theme.Mode, label string) {
	t.Helper()
	canvasBg := bgSeq(t, theme.MV.Canvas, mode)
	for _, forbidden := range []theme.Token{
		theme.MV.AccentViolet, theme.MV.AccentOrange, theme.MV.BorderSeparator,
		theme.MV.BgSelection, theme.MV.StateGreen,
	} {
		seq := bgSeq(t, forbidden, mode)
		if seq == canvasBg {
			continue
		}
		if strings.Contains(content, seq) {
			t.Errorf("[%v/%s] modal must not fill (found %s background SGR %q)", mode, label, forbidden.Name, seq)
		}
	}
}

// bgSeq returns the bare `48;2;r;g;b` background SGR parameter substring for a token.
func bgSeq(t *testing.T, tok theme.Token, m theme.Mode) string {
	t.Helper()
	probe := lipgloss.NewStyle().Background(tok.ColorFor(m)).Render("x")
	start := strings.IndexByte(probe, '[')
	end := strings.IndexByte(probe, 'm')
	if start < 0 || end <= start {
		t.Fatalf("could not derive background SGR core from %q", probe)
	}
	return probe[start+1 : end]
}

// assertNoCross fails if any inline ✕ (U+2715) is rendered — chips drop the inline
// cross (removal is `x` on a focused chip, carried by the footer).
func assertNoCross(t *testing.T, content string) {
	t.Helper()
	if strings.ContainsRune(ansi.Strip(content), '✕') {
		t.Errorf("chips must not render an inline ✕; got:\n%s", ansi.Strip(content))
	}
}

// assertNoGreen fails if state.green is present anywhere in the rendered content
// (chips are never green).
func assertNoGreen(t *testing.T, content string, mode theme.Mode) {
	t.Helper()
	assertNoGreenLabelled(t, content, mode, 0)
}

// TestEditModalFooterRow_ByteExact pins the full-ANSI rendered output of the edit
// modal footer for the navigate-name and editing-in-place states so the separator-
// constant consolidation (editFooterSep → the shared footerEntrySeparator) is proven
// byte-identical: both constants are the same " · " value rendered in text.detail, so
// the footer must render byte-for-byte unchanged. The ANSI-stripped layout is already
// pinned by TestRenderEditProjectContent_ByteExact; this oracle additionally locks the
// colour bytes around the shared separator.
func TestEditModalFooterRow_ByteExact(t *testing.T) {
	const wantNavDark = "\x1b[38;2;122;162;247;48;2;11;12;20m⏎/e\x1b[m\x1b[48;2;11;12;20m \x1b[m\x1b[38;2;115;122;162;48;2;11;12;20medit\x1b[m\x1b[38;2;115;122;162;48;2;11;12;20m · \x1b[m\x1b[38;2;122;162;247;48;2;11;12;20m⇥\x1b[m\x1b[48;2;11;12;20m \x1b[m\x1b[38;2;115;122;162;48;2;11;12;20mnext field\x1b[m\x1b[38;2;115;122;162;48;2;11;12;20m · \x1b[m\x1b[38;2;122;162;247;48;2;11;12;20mesc\x1b[m\x1b[48;2;11;12;20m \x1b[m\x1b[38;2;115;122;162;48;2;11;12;20mclose\x1b[m"
	const wantEditDark = "\x1b[38;2;122;162;247;48;2;11;12;20m⏎\x1b[m\x1b[48;2;11;12;20m \x1b[m\x1b[38;2;115;122;162;48;2;11;12;20msave\x1b[m\x1b[38;2;115;122;162;48;2;11;12;20m · \x1b[m\x1b[38;2;122;162;247;48;2;11;12;20mesc\x1b[m\x1b[48;2;11;12;20m \x1b[m\x1b[38;2;115;122;162;48;2;11;12;20mdiscard\x1b[m\x1b[38;2;115;122;162;48;2;11;12;20m · \x1b[m\x1b[38;2;122;162;247;48;2;11;12;20m←→\x1b[m\x1b[48;2;11;12;20m \x1b[m\x1b[38;2;115;122;162;48;2;11;12;20mcursor\x1b[m\x1b[48;2;11;12;20m    \x1b[m\x1b[38;2;115;122;162;48;2;11;12;20mempty on save = delete\x1b[m"

	t.Run("navigate-name-focused dark", func(t *testing.T) {
		m := editModalModel(editFieldName, 0, 0)
		if got := m.editModalFooterRow(theme.Dark, false); got != wantNavDark {
			t.Errorf("navigate footer byte mismatch\n got: %q\nwant: %q", got, wantNavDark)
		}
	})

	t.Run("editing-in-place dark", func(t *testing.T) {
		m := editModalModel(editFieldTags, 0, 0)
		m.editMode = editModeEdit
		m.editBuffer = "Fabric"
		m.editCursor = len([]rune("Fabric"))
		if got := m.editModalFooterRow(theme.Dark, false); got != wantEditDark {
			t.Errorf("editing footer byte mismatch\n got: %q\nwant: %q", got, wantEditDark)
		}
	})
}

func assertNoGreenLabelled(t *testing.T, content string, mode theme.Mode, idx int) {
	t.Helper()
	if seq := tokenFgSeq(t, theme.MV.StateGreen, mode); strings.Contains(content, seq) {
		t.Errorf("[%v/state%d] state.green must never appear on a chip; SGR core %q present", mode, idx, seq)
	}
}
