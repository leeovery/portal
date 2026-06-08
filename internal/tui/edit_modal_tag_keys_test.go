package tui

import (
	"reflect"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// tagKeyModel builds a minimal Model with the edit modal open and Tags focused,
// seeded with the given tag buffer, new-tag input, and cursor.
func tagKeyModel(tags []string, newTag string, cursor int) Model {
	return Model{
		modal:         modalEditProject,
		editFocus:     editFieldTags,
		editTags:      tags,
		editNewTag:    newTag,
		editTagCursor: cursor,
	}
}

// pressRunes drives one runes key through updateEditProjectModal.
func pressRunes(t *testing.T, m Model, s string) Model {
	t.Helper()
	updated, _ := m.updateEditProjectModal(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)})
	return updated.(Model)
}

// pressBackspaceKey drives one Backspace key through updateEditProjectModal.
func pressBackspaceKey(t *testing.T, m Model) Model {
	t.Helper()
	updated, _ := m.updateEditProjectModal(tea.KeyMsg{Type: tea.KeyBackspace})
	return updated.(Model)
}

// pressDownKey / pressUpKey drive the corresponding key through the modal.
func pressDownKey(t *testing.T, m Model) Model {
	t.Helper()
	updated, _ := m.updateEditProjectModal(tea.KeyMsg{Type: tea.KeyDown})
	return updated.(Model)
}

func pressUpKey(t *testing.T, m Model) Model {
	t.Helper()
	updated, _ := m.updateEditProjectModal(tea.KeyMsg{Type: tea.KeyUp})
	return updated.(Model)
}

func TestEditModalTagKeys_RemoveHighlightedTagOnX(t *testing.T) {
	got := pressRunes(t, tagKeyModel([]string{"work", "personal"}, "", 0), "x")

	if !reflect.DeepEqual(got.editTags, []string{"personal"}) {
		t.Errorf("editTags = %v, want [personal]", got.editTags)
	}
}

func TestEditModalTagKeys_RecordsRemovedTag(t *testing.T) {
	got := pressRunes(t, tagKeyModel([]string{"work", "personal"}, "", 0), "x")

	if !reflect.DeepEqual(got.editRemovedTags, []string{"work"}) {
		t.Errorf("editRemovedTags = %v, want [work]", got.editRemovedTags)
	}
}

func TestEditModalTagKeys_XOnAddRowTypesLiteral(t *testing.T) {
	// Cursor on the Add row (index == len(editTags)): x is a literal char.
	m := tagKeyModel([]string{"work"}, "", 1)
	got := pressRunes(t, m, "x")

	if !reflect.DeepEqual(got.editTags, []string{"work"}) {
		t.Errorf("editTags = %v, want [work] (x on Add row must not remove)", got.editTags)
	}
	if got.editNewTag != "x" {
		t.Errorf("editNewTag = %q, want %q", got.editNewTag, "x")
	}
	if got.editRemovedTags != nil {
		t.Errorf("editRemovedTags = %v, want nil", got.editRemovedTags)
	}
}

func TestEditModalTagKeys_ClampsCursorAfterRemovingLastEntry(t *testing.T) {
	// Single entry, cursor on it (index 0). Removing it leaves len 0; cursor
	// must be clamped so it never exceeds len(editTags).
	m := tagKeyModel([]string{"only"}, "", 0)
	got := pressRunes(t, m, "x")

	if len(got.editTags) != 0 {
		t.Fatalf("editTags = %v, want empty", got.editTags)
	}
	if got.editTagCursor > len(got.editTags) {
		t.Errorf("editTagCursor = %d, want <= %d", got.editTagCursor, len(got.editTags))
	}
}

func TestEditModalTagKeys_AppendsTypedCharsToAddInput(t *testing.T) {
	m := tagKeyModel([]string{"work"}, "de", 1) // cursor on Add row
	m.editError = "stale"
	got := pressRunes(t, m, "s")

	if got.editNewTag != "des" {
		t.Errorf("editNewTag = %q, want %q", got.editNewTag, "des")
	}
	if got.editError != "" {
		t.Errorf("editError = %q, want empty", got.editError)
	}
}

func TestEditModalTagKeys_BackspaceTrimsOnlyAddInput(t *testing.T) {
	m := tagKeyModel([]string{"work"}, "design", 1) // cursor on Add row
	got := pressBackspaceKey(t, m)

	if got.editNewTag != "desig" {
		t.Errorf("editNewTag = %q, want %q", got.editNewTag, "desig")
	}
	if !reflect.DeepEqual(got.editTags, []string{"work"}) {
		t.Errorf("editTags = %v, want [work] (backspace must not touch entries)", got.editTags)
	}
}

func TestEditModalTagKeys_BackspaceOnExistingEntryDoesNotMutateEntry(t *testing.T) {
	// Cursor on an existing entry (index 0), with empty Add input. Backspace
	// must not mutate the entry text and must not touch editNewTag.
	m := tagKeyModel([]string{"work"}, "", 0)
	got := pressBackspaceKey(t, m)

	if !reflect.DeepEqual(got.editTags, []string{"work"}) {
		t.Errorf("editTags = %v, want [work]", got.editTags)
	}
	if got.editNewTag != "" {
		t.Errorf("editNewTag = %q, want empty", got.editNewTag)
	}
}

func TestEditModalTagKeys_BackspaceOnExistingTagDoesNotCorruptAbandonedAlias(t *testing.T) {
	// Regression: Tags focused on an EXISTING entry with an in-progress
	// (abandoned) alias buffer. Backspace must not fall through to the alias
	// Add-input branch and silently trim editNewAlias.
	m := tagKeyModel([]string{"work", "home"}, "", 0)
	m.editAliases = nil
	m.editAliasCursor = 0
	m.editNewAlias = "abandoned"

	got := pressBackspaceKey(t, m)

	if got.editNewAlias != "abandoned" {
		t.Errorf("editNewAlias = %q, want %q (Tags-focused Backspace must not trim alias Add input)", got.editNewAlias, "abandoned")
	}
	if !reflect.DeepEqual(got.editTags, []string{"work", "home"}) {
		t.Errorf("editTags = %v, want [work home]", got.editTags)
	}
	if got.editNewTag != "" {
		t.Errorf("editNewTag = %q, want empty", got.editNewTag)
	}
}

func TestEditModalTagKeys_DownBoundedToAddRow(t *testing.T) {
	m := tagKeyModel([]string{"work", "personal"}, "", 0)

	m = pressDownKey(t, m)
	if m.editTagCursor != 1 {
		t.Fatalf("after 1 down, cursor = %d, want 1", m.editTagCursor)
	}
	m = pressDownKey(t, m)
	if m.editTagCursor != 2 {
		t.Fatalf("after 2 down, cursor = %d, want 2 (Add row)", m.editTagCursor)
	}
	// Add row is index len(editTags) == 2; further Down is bounded.
	m = pressDownKey(t, m)
	if m.editTagCursor != 2 {
		t.Errorf("after 3 down, cursor = %d, want 2 (bounded at Add row)", m.editTagCursor)
	}
}

func TestEditModalTagKeys_UpBoundedToZero(t *testing.T) {
	m := tagKeyModel([]string{"work", "personal"}, "", 2)

	m = pressUpKey(t, m)
	if m.editTagCursor != 1 {
		t.Fatalf("after 1 up, cursor = %d, want 1", m.editTagCursor)
	}
	m = pressUpKey(t, m)
	if m.editTagCursor != 0 {
		t.Fatalf("after 2 up, cursor = %d, want 0", m.editTagCursor)
	}
	m = pressUpKey(t, m)
	if m.editTagCursor != 0 {
		t.Errorf("after 3 up, cursor = %d, want 0 (bounded at 0)", m.editTagCursor)
	}
}

// --- No-regression guards for Aliases / Name focus ---

func TestEditModalTagKeys_AliasFocusXStillRemoves(t *testing.T) {
	m := Model{
		modal:           modalEditProject,
		editFocus:       editFieldAliases,
		editAliases:     []string{"a", "b"},
		editAliasCursor: 0,
	}
	got := pressRunes(t, m, "x")

	if !reflect.DeepEqual(got.editAliases, []string{"b"}) {
		t.Errorf("editAliases = %v, want [b]", got.editAliases)
	}
	if !reflect.DeepEqual(got.editRemoved, []string{"a"}) {
		t.Errorf("editRemoved = %v, want [a]", got.editRemoved)
	}
	if got.editTags != nil || got.editRemovedTags != nil {
		t.Errorf("tag buffers mutated by alias-focused x: tags=%v removed=%v", got.editTags, got.editRemovedTags)
	}
}

func TestEditModalTagKeys_AliasFocusTypeAndBackspaceUnaffected(t *testing.T) {
	m := Model{
		modal:           modalEditProject,
		editFocus:       editFieldAliases,
		editAliases:     []string{"a"},
		editAliasCursor: 1, // Add row
		editNewAlias:    "fo",
	}
	m = pressRunes(t, m, "o")
	if m.editNewAlias != "foo" {
		t.Fatalf("editNewAlias = %q, want %q", m.editNewAlias, "foo")
	}
	m = pressBackspaceKey(t, m)
	if m.editNewAlias != "fo" {
		t.Errorf("editNewAlias = %q, want %q after backspace", m.editNewAlias, "fo")
	}
	if m.editNewTag != "" {
		t.Errorf("editNewTag = %q, want empty (alias keys must not touch tag buffer)", m.editNewTag)
	}
}

func TestEditModalTagKeys_AliasFocusUpDownUnaffected(t *testing.T) {
	m := Model{
		modal:           modalEditProject,
		editFocus:       editFieldAliases,
		editAliases:     []string{"a", "b"},
		editAliasCursor: 0,
	}
	m = pressDownKey(t, m)
	if m.editAliasCursor != 1 {
		t.Fatalf("editAliasCursor = %d, want 1", m.editAliasCursor)
	}
	if m.editTagCursor != 0 {
		t.Errorf("editTagCursor = %d, want 0 (alias Down must not move tag cursor)", m.editTagCursor)
	}
}

func TestEditModalTagKeys_NameFocusTypeAndBackspaceUnaffected(t *testing.T) {
	m := Model{
		modal:     modalEditProject,
		editFocus: editFieldName,
		editName:  "proj",
	}
	m = pressRunes(t, m, "X")
	if m.editName != "projX" {
		t.Fatalf("editName = %q, want %q", m.editName, "projX")
	}
	m = pressBackspaceKey(t, m)
	if m.editName != "proj" {
		t.Errorf("editName = %q, want %q after backspace", m.editName, "proj")
	}
	if m.editNewTag != "" {
		t.Errorf("editNewTag = %q, want empty", m.editNewTag)
	}
}
