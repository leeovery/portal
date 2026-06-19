package tui

import (
	"errors"
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/portal/internal/project"
)

// errTagStub is a sentinel for exercising the modal's persist-failure branches.
var errTagStub = errors.New("stub failure")

// recordingProjectEditor records AddTag/RemoveTag calls so tests can assert the
// edit modal persists tag mutations immediately (live), rather than batching
// them until a confirm.
type recordingProjectEditor struct {
	added   [][2]string // {path, tag}
	removed [][2]string
	addErr  error
	rmErr   error
}

func (recordingProjectEditor) Rename(_, _, _ string) error { return nil }

func (r *recordingProjectEditor) AddTag(path, tag string) error {
	if r.addErr != nil {
		return r.addErr
	}
	r.added = append(r.added, [2]string{path, tag})
	return nil
}

func (r *recordingProjectEditor) RemoveTag(path, tag string) error {
	if r.rmErr != nil {
		return r.rmErr
	}
	r.removed = append(r.removed, [2]string{path, tag})
	return nil
}

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
	updated, _ := m.updateEditProjectModal(tea.KeyPressMsg{Code: tea.KeyExtended, Text: s})
	return updated.(Model)
}

// pressBackspaceKey drives one Backspace key through updateEditProjectModal.
func pressBackspaceKey(t *testing.T, m Model) Model {
	t.Helper()
	updated, _ := m.updateEditProjectModal(tea.KeyPressMsg{Code: tea.KeyBackspace})
	return updated.(Model)
}

// pressDownKey / pressUpKey drive the corresponding key through the modal.
func pressDownKey(t *testing.T, m Model) Model {
	t.Helper()
	updated, _ := m.updateEditProjectModal(tea.KeyPressMsg{Code: tea.KeyDown})
	return updated.(Model)
}

func pressUpKey(t *testing.T, m Model) Model {
	t.Helper()
	updated, _ := m.updateEditProjectModal(tea.KeyPressMsg{Code: tea.KeyUp})
	return updated.(Model)
}

func TestEditModalTagKeys_RemoveHighlightedTagOnX(t *testing.T) {
	got := pressRunes(t, tagKeyModel([]string{"work", "personal"}, "", 0), "x")

	if !reflect.DeepEqual(got.editTags, []string{"personal"}) {
		t.Errorf("editTags = %v, want [personal]", got.editTags)
	}
}

func TestEditModalTagKeys_RemovePersistsImmediately(t *testing.T) {
	ed := &recordingProjectEditor{}
	m := tagKeyModel([]string{"work", "personal"}, "", 0)
	m.editProject = project.Project{Path: "/p/one"}
	m.projectEditor = ed

	got := pressRunes(t, m, "x")

	// Buffer reflects the removal.
	if !reflect.DeepEqual(got.editTags, []string{"personal"}) {
		t.Errorf("editTags = %v, want [personal]", got.editTags)
	}
	// And the removal is persisted to projects.json right away (live edit),
	// so Esc can never discard it.
	wantRm := [][2]string{{"/p/one", "work"}}
	if !reflect.DeepEqual(ed.removed, wantRm) {
		t.Errorf("RemoveTag calls = %v, want %v", ed.removed, wantRm)
	}
	if !got.editTagsMutated {
		t.Errorf("editTagsMutated = false, want true after a live removal")
	}
}

func TestEditModalTagKeys_RemoveErrorKeepsTagAndSetsError(t *testing.T) {
	ed := &recordingProjectEditor{rmErr: errTagStub}
	m := tagKeyModel([]string{"work"}, "", 0)
	m.editProject = project.Project{Path: "/p/one"}
	m.projectEditor = ed

	got := pressRunes(t, m, "x")

	// A failed persist must not drop the tag from the buffer.
	if !reflect.DeepEqual(got.editTags, []string{"work"}) {
		t.Errorf("editTags = %v, want [work] (failed remove must not mutate buffer)", got.editTags)
	}
	if got.editError == "" {
		t.Errorf("editError = empty, want a remove-failure message")
	}
}

func TestEditModalTagKeys_AddPersistsImmediately(t *testing.T) {
	ed := &recordingProjectEditor{}
	m := tagKeyModel(nil, "design", 0) // Add row (cursor == len(editTags) == 0)
	m.editProject = project.Project{Path: "/p/one"}
	m.projectEditor = ed

	got := pressEnter(t, m)

	if !reflect.DeepEqual(got.editTags, []string{"design"}) {
		t.Errorf("editTags = %v, want [design]", got.editTags)
	}
	wantAdd := [][2]string{{"/p/one", "design"}}
	if !reflect.DeepEqual(ed.added, wantAdd) {
		t.Errorf("AddTag calls = %v, want %v", ed.added, wantAdd)
	}
	if !got.editTagsMutated {
		t.Errorf("editTagsMutated = false, want true after a live add")
	}
	// Add must not close the modal — the user may add several tags.
	if got.modal != modalEditProject {
		t.Errorf("modal = %v, want modalEditProject", got.modal)
	}
}

func TestEditModalTagKeys_AddErrorDoesNotAppendAndSetsError(t *testing.T) {
	ed := &recordingProjectEditor{addErr: errTagStub}
	m := tagKeyModel(nil, "design", 0)
	m.editProject = project.Project{Path: "/p/one"}
	m.projectEditor = ed

	got := pressEnter(t, m)

	if len(got.editTags) != 0 {
		t.Errorf("editTags = %v, want empty (failed add must not append)", got.editTags)
	}
	if got.editError == "" {
		t.Errorf("editError = empty, want an add-failure message")
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
	if got.editTagsMutated {
		t.Errorf("editTagsMutated = true, want false (x on Add row removes nothing)")
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
	if got.editTags != nil || got.editTagsMutated {
		t.Errorf("tag state mutated by alias-focused x: tags=%v mutated=%v", got.editTags, got.editTagsMutated)
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
