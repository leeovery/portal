package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/leeovery/portal/internal/project"
)

// addTagModel builds a minimal Model with the edit modal open, Tags focused,
// and the given in-progress new-tag text and tag buffer, for exercising the
// add-tag-on-Enter behaviour in updateEditProjectModal directly.
func addTagModel(newTag string, tags []string) Model {
	return Model{
		modal:      modalEditProject,
		editFocus:  editFieldTags,
		editNewTag: newTag,
		editTags:   tags,
	}
}

// pressEnter drives one Enter key through updateEditProjectModal and returns the
// resulting Model.
func pressEnter(t *testing.T, m Model) Model {
	t.Helper()
	updated, _ := m.updateEditProjectModal(tea.KeyMsg{Type: tea.KeyEnter})
	return updated.(Model)
}

func TestEditModalAddTag_AppendsNormalisedTag(t *testing.T) {
	got := pressEnter(t, addTagModel("design", nil))

	if len(got.editTags) != 1 || got.editTags[0] != "design" {
		t.Fatalf("editTags = %v, want [design]", got.editTags)
	}
	if got.editNewTag != "" {
		t.Errorf("editNewTag = %q, want empty", got.editNewTag)
	}
}

func TestEditModalAddTag_NormalisesWhitespaceAndCase(t *testing.T) {
	got := pressEnter(t, addTagModel("  Work ", nil))

	if len(got.editTags) != 1 || got.editTags[0] != "work" {
		t.Fatalf("editTags = %v, want [work]", got.editTags)
	}
	if got.editNewTag != "" {
		t.Errorf("editNewTag = %q, want empty", got.editNewTag)
	}
}

func TestEditModalAddTag_BlankIsNoOp(t *testing.T) {
	got := pressEnter(t, addTagModel("   ", nil))

	if len(got.editTags) != 0 {
		t.Errorf("editTags = %v, want empty (blank input is a no-op)", got.editTags)
	}
	if got.modal != modalEditProject {
		t.Errorf("modal = %v, want modalEditProject (blank Enter must not confirm)", got.modal)
	}
}

func TestEditModalAddTag_DuplicateAfterNormalisationIsNoOp(t *testing.T) {
	got := pressEnter(t, addTagModel("WORK", []string{"work"}))

	if len(got.editTags) != 1 || got.editTags[0] != "work" {
		t.Errorf("editTags = %v, want [work] (no duplicate appended)", got.editTags)
	}
	if got.editNewTag != "" {
		t.Errorf("editNewTag = %q, want empty", got.editNewTag)
	}
	if got.modal != modalEditProject {
		t.Errorf("modal = %v, want modalEditProject (dup Enter must not confirm)", got.modal)
	}
}

func TestEditModalAddTag_DoesNotCloseModal(t *testing.T) {
	got := pressEnter(t, addTagModel("work", nil))

	if got.modal != modalEditProject {
		t.Errorf("modal = %v, want modalEditProject (add must not close modal)", got.modal)
	}
}

func TestEditModalAddTag_ClearsError(t *testing.T) {
	m := addTagModel("work", nil)
	m.editError = "previous error"

	got := pressEnter(t, m)

	if got.editError != "" {
		t.Errorf("editError = %q, want empty after successful add", got.editError)
	}
}

// confirmStubAliasEditor / confirmStubProjectEditor are no-op editors so
// handleEditProjectConfirm runs to completion (closing the modal) when Enter is
// pressed while Name/Aliases are focused.
type confirmStubAliasEditor struct{}

func (confirmStubAliasEditor) Load() (map[string]string, error)        { return map[string]string{}, nil }
func (confirmStubAliasEditor) SetAndSave(_, _, _ string) error         { return nil }
func (confirmStubAliasEditor) DeleteAndSave(_, _ string) (bool, error) { return false, nil }

type confirmStubProjectEditor struct{}

func (confirmStubProjectEditor) Rename(_, _, _ string) error { return nil }
func (confirmStubProjectEditor) AddTag(_, _ string) error    { return nil }
func (confirmStubProjectEditor) RemoveTag(_, _ string) error { return nil }

func confirmModel(focus editField) Model {
	return Model{
		modal:         modalEditProject,
		editFocus:     focus,
		editName:      "my-project",
		editProject:   project.Project{Path: "/p/one", Name: "my-project"},
		projectEditor: confirmStubProjectEditor{},
		aliasEditor:   confirmStubAliasEditor{},
	}
}

func TestEditModalEnter_NameFocusedConfirms(t *testing.T) {
	got := pressEnter(t, confirmModel(editFieldName))

	if got.modal != modalNone {
		t.Errorf("modal = %v, want modalNone (Name-focused Enter must confirm)", got.modal)
	}
}

func TestEditModalEnter_AliasesFocusedConfirms(t *testing.T) {
	got := pressEnter(t, confirmModel(editFieldAliases))

	if got.modal != modalNone {
		t.Errorf("modal = %v, want modalNone (Aliases-focused Enter must confirm)", got.modal)
	}
}
