package tui

import (
	"testing"

	"github.com/leeovery/portal/internal/project"
)

// stubTagsAliasEditor is a no-op AliasEditor for tag-state modal-open tests:
// Load returns an empty alias map so handleEditProjectKey proceeds past the
// alias load without depending on real alias storage.
type stubTagsAliasEditor struct{}

func (stubTagsAliasEditor) Load() (map[string]string, error)        { return map[string]string{}, nil }
func (stubTagsAliasEditor) SetAndSave(_, _, _ string) error         { return nil }
func (stubTagsAliasEditor) DeleteAndSave(_, _ string) (bool, error) { return false, nil }

// stubTagsProjectEditor is a no-op ProjectEditor satisfying the non-nil
// dependency guard in handleEditProjectKey.
type stubTagsProjectEditor struct{}

func (stubTagsProjectEditor) Rename(_, _, _ string) error { return nil }

// newTagModalTestModel builds a Model on the projects page with the given
// projects loaded into the project list and the supplied project selected, plus
// stub editors so handleEditProjectKey runs to completion.
func newTagModalTestModel(t *testing.T, projects []project.Project, selectIdx int) Model {
	t.Helper()
	m := Model{
		projects:      projects,
		projectList:   newProjectList(),
		activePage:    PageProjects,
		projectEditor: stubTagsProjectEditor{},
		aliasEditor:   stubTagsAliasEditor{},
	}
	items := ProjectsToListItems(projects)
	m.projectList.SetItems(items)
	m.projectList.Select(selectIdx)
	return m
}

func TestHandleEditProjectKey_LoadsExistingTagsIntoBuffer(t *testing.T) {
	projects := []project.Project{
		{Path: "/p/one", Name: "one", Tags: []string{"work", "personal"}},
	}
	m := newTagModalTestModel(t, projects, 0)

	updated, _ := m.handleEditProjectKey()
	got := updated.(Model)

	want := []string{"work", "personal"}
	if len(got.editTags) != len(want) {
		t.Fatalf("editTags length = %d, want %d (%v)", len(got.editTags), len(want), got.editTags)
	}
	for i := range want {
		if got.editTags[i] != want[i] {
			t.Fatalf("editTags[%d] = %q, want %q", i, got.editTags[i], want[i])
		}
	}
	if got.editRemovedTags != nil {
		t.Errorf("editRemovedTags = %v, want nil", got.editRemovedTags)
	}
	if got.editNewTag != "" {
		t.Errorf("editNewTag = %q, want empty", got.editNewTag)
	}
	if got.editTagCursor != 0 {
		t.Errorf("editTagCursor = %d, want 0", got.editTagCursor)
	}
}

func TestHandleEditProjectKey_SeedsEmptyTagBufferForNilTags(t *testing.T) {
	projects := []project.Project{
		{Path: "/p/notags", Name: "notags"}, // nil Tags (back-compat record)
	}
	m := newTagModalTestModel(t, projects, 0)

	updated, _ := m.handleEditProjectKey()
	got := updated.(Model)

	if len(got.editTags) != 0 {
		t.Fatalf("editTags = %v, want empty", got.editTags)
	}
}

func TestHandleEditProjectKey_ResetsTagBufferOnReopen(t *testing.T) {
	projects := []project.Project{
		{Path: "/p/one", Name: "one", Tags: []string{"work"}},
		{Path: "/p/two", Name: "two"}, // no tags
	}
	m := newTagModalTestModel(t, projects, 0)

	// First open on the tagged project, then dirty the buffer.
	updated, _ := m.handleEditProjectKey()
	first := updated.(Model)
	first.editRemovedTags = []string{"stale"}
	first.editNewTag = "half-typed"
	first.editTagCursor = 5
	first.editTags = append(first.editTags, "leaked")

	// Re-open on the second, tag-less project.
	first.projectList.Select(1)
	reopened, _ := first.handleEditProjectKey()
	got := reopened.(Model)

	if len(got.editTags) != 0 {
		t.Errorf("editTags = %v, want empty after reopen on tag-less project", got.editTags)
	}
	if got.editRemovedTags != nil {
		t.Errorf("editRemovedTags = %v, want nil after reopen", got.editRemovedTags)
	}
	if got.editNewTag != "" {
		t.Errorf("editNewTag = %q, want empty after reopen", got.editNewTag)
	}
	if got.editTagCursor != 0 {
		t.Errorf("editTagCursor = %d, want 0 after reopen", got.editTagCursor)
	}
}

func TestHandleEditProjectKey_CopiesTagsSliceNoAliasing(t *testing.T) {
	projects := []project.Project{
		{Path: "/p/one", Name: "one", Tags: []string{"work", "personal"}},
	}
	m := newTagModalTestModel(t, projects, 0)

	updated, _ := m.handleEditProjectKey()
	got := updated.(Model)

	// Mutate the buffer in place.
	got.editTags[0] = "MUTATED"

	// The stored project's Tags must be untouched.
	stored := got.projectList.Items()[0].(ProjectItem).Project
	if stored.Tags[0] != "work" {
		t.Errorf("stored project Tags[0] = %q, want %q (buffer aliased stored slice)", stored.Tags[0], "work")
	}
	// And the source slice we constructed the project from.
	if projects[0].Tags[0] != "work" {
		t.Errorf("source projects[0].Tags[0] = %q, want %q (buffer aliased source slice)", projects[0].Tags[0], "work")
	}
}
