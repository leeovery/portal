package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// tabModel builds a minimal Model with the edit modal open, the given focus
// seeded, and the supplied tag buffer, for exercising the Tab field cycle in
// updateEditProjectModal directly.
func tabModel(focus editField, tags []string) Model {
	return Model{
		modal:     modalEditProject,
		editFocus: focus,
		editTags:  tags,
	}
}

// pressTab drives one Tab key through updateEditProjectModal and returns the
// resulting Model.
func pressTab(t *testing.T, m Model) Model {
	t.Helper()
	updated, _ := m.updateEditProjectModal(tea.KeyPressMsg{Code: tea.KeyTab})
	return updated.(Model)
}

func TestEditModalTab_NameToAliases(t *testing.T) {
	got := pressTab(t, tabModel(editFieldName, nil))
	if got.editFocus != editFieldAliases {
		t.Errorf("editFocus = %d, want editFieldAliases (%d)", got.editFocus, editFieldAliases)
	}
}

func TestEditModalTab_AliasesToTags(t *testing.T) {
	got := pressTab(t, tabModel(editFieldAliases, nil))
	if got.editFocus != editFieldTags {
		t.Errorf("editFocus = %d, want editFieldTags (%d)", got.editFocus, editFieldTags)
	}
}

func TestEditModalTab_TagsWrapsToName(t *testing.T) {
	got := pressTab(t, tabModel(editFieldTags, nil))
	if got.editFocus != editFieldName {
		t.Errorf("editFocus = %d, want editFieldName (%d)", got.editFocus, editFieldName)
	}
}

func TestEditModalTab_ThreePressesReturnToName(t *testing.T) {
	m := tabModel(editFieldName, nil)
	for i := 0; i < 3; i++ {
		m = pressTab(t, m)
	}
	if m.editFocus != editFieldName {
		t.Errorf("after three Tab presses editFocus = %d, want editFieldName (%d)", m.editFocus, editFieldName)
	}
}

func TestEditModalTab_InitialisesTagCursorInBoundsOnEntry(t *testing.T) {
	// Enter Tags from Aliases with an empty tag buffer: the cursor must land on
	// the Add-input row (index 0 == len(editTags)) and stay in bounds.
	m := tabModel(editFieldAliases, nil)
	m.editTagCursor = 99 // dirty value that must be reset on entry
	got := pressTab(t, m)

	if got.editFocus != editFieldTags {
		t.Fatalf("editFocus = %d, want editFieldTags (%d)", got.editFocus, editFieldTags)
	}
	if got.editTagCursor < 0 || got.editTagCursor > len(got.editTags) {
		t.Errorf("editTagCursor = %d, want within [0, %d]", got.editTagCursor, len(got.editTags))
	}
	if got.editTagCursor != 0 {
		t.Errorf("editTagCursor = %d, want 0 on entry with empty tag buffer", got.editTagCursor)
	}
}
