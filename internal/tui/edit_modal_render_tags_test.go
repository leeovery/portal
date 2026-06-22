package tui

import (
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/project"
)

// renderTagsModel builds a minimal Model with the edit modal state seeded for
// exercising renderEditProjectContent's Tags block directly. The modal is in
// navigate mode (no live edit buffer) with the given focus, tags, and element
// index.
func renderTagsModel(focus editField, tags []string, tagCursor int) Model {
	return Model{
		modal:         modalEditProject,
		editMode:      editModeNavigate,
		editProject:   project.Project{Name: "Portal"},
		editFocus:     focus,
		editTags:      tags,
		editTagCursor: tagCursor,
	}
}

func TestRenderEditProjectContent_TagsBlockAfterAliases(t *testing.T) {
	m := renderTagsModel(editFieldTags, []string{"work"}, 0)
	out := m.renderEditProjectContent()

	tagsIdx := strings.Index(out, "Tags:")
	aliasesIdx := strings.Index(out, "Aliases:")
	if tagsIdx == -1 {
		t.Fatalf("output missing Tags: heading\n%s", out)
	}
	if aliasesIdx == -1 {
		t.Fatalf("output missing Aliases: heading\n%s", out)
	}
	if tagsIdx < aliasesIdx {
		t.Errorf("Tags: (index %d) should render after Aliases: (index %d)\n%s", tagsIdx, aliasesIdx, out)
	}
}

func TestRenderEditProjectContent_EachTagHasRemovalMarker(t *testing.T) {
	m := renderTagsModel(editFieldTags, []string{"work", "personal"}, 0)
	out := m.renderEditProjectContent()

	if !strings.Contains(out, "[x] work") {
		t.Errorf("output missing [x] work\n%s", out)
	}
	if !strings.Contains(out, "[x] personal") {
		t.Errorf("output missing [x] personal\n%s", out)
	}
}

func TestRenderEditProjectContent_HighlightOnFocusedTag(t *testing.T) {
	m := renderTagsModel(editFieldTags, []string{"work", "personal"}, 1)
	out := m.renderEditProjectContent()

	if !strings.Contains(out, "  > [x] personal") {
		t.Errorf("focused tag at cursor 1 should show highlight marker\n%s", out)
	}
	// The non-focused entry must NOT carry the highlight marker.
	if strings.Contains(out, "  > [x] work") {
		t.Errorf("non-focused tag should not show highlight marker\n%s", out)
	}
}

func TestRenderEditProjectContent_EmptyTagsShowsNoneState(t *testing.T) {
	m := renderTagsModel(editFieldTags, nil, 0)
	out := m.renderEditProjectContent()

	tagsIdx := strings.Index(out, "Tags:")
	if tagsIdx == -1 {
		t.Fatalf("output missing Tags: heading\n%s", out)
	}
	// The (none) line must appear after the Tags: heading (the tags empty
	// state, distinct from the aliases empty state which precedes it).
	noneAfterTags := strings.Index(out[tagsIdx:], "(none)")
	if noneAfterTags == -1 {
		t.Errorf("empty tags should render a (none) empty-state line after Tags:\n%s", out)
	}
}

func TestRenderEditProjectContent_AddRowAlwaysRendered(t *testing.T) {
	// The Add slot always renders, both with zero tags and with existing tags.
	emptyOut := renderTagsModel(editFieldTags, nil, 0).renderEditProjectContent()
	if !strings.Contains(emptyOut, "Add:") {
		t.Errorf("Add-input row should render with zero tags\n%s", emptyOut)
	}

	fullOut := renderTagsModel(editFieldTags, []string{"work"}, 0).renderEditProjectContent()
	tagsIdx := strings.Index(fullOut, "Tags:")
	addAfterTags := strings.Index(fullOut[tagsIdx:], "Add:")
	if addAfterTags == -1 {
		t.Errorf("Add-input row should render after Tags: with existing tags\n%s", fullOut)
	}
}

func TestRenderEditProjectContent_AddSlotShowsLiveBufferForNewChip(t *testing.T) {
	// A brand-new chip being edited shows its in-progress text in the Add slot.
	m := renderTagsModel(editFieldTags, []string{"work"}, 1) // cursor on add slot
	m.editMode = editModeEdit
	m.editIsNewChip = true
	m.editBuffer = "draft"

	out := m.renderEditProjectContent()
	if !strings.Contains(out, "Add: draft") {
		t.Errorf("Add slot should show the live new-chip buffer\n%s", out)
	}
}

func TestRenderEditProjectContent_TagsHeadingFocusScoped(t *testing.T) {
	// Name focused: Tags heading must show the unfocused indicator, Name the
	// focused one.
	m := renderTagsModel(editFieldName, []string{"work"}, 0)
	out := m.renderEditProjectContent()

	if !strings.Contains(out, "> Name:") {
		t.Errorf("Name should show focus indicator when focused\n%s", out)
	}
	if !strings.Contains(out, "  Tags:") {
		t.Errorf("Tags heading should show unfocused indicator when Name is focused\n%s", out)
	}
	if strings.Contains(out, "> Tags:") {
		t.Errorf("Tags heading should NOT show focus indicator when Name is focused\n%s", out)
	}

	// Tags focused: heading shows focus indicator.
	tagsFocused := renderTagsModel(editFieldTags, []string{"work"}, 0).renderEditProjectContent()
	if !strings.Contains(tagsFocused, "> Tags:") {
		t.Errorf("Tags heading should show focus indicator when Tags is focused\n%s", tagsFocused)
	}
}
