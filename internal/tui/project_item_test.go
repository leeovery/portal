package tui_test

import (
	"bytes"
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/tui"
)

func TestProjectItem(t *testing.T) {
	t.Run("implements list.Item interface", func(t *testing.T) {
		var _ list.Item = tui.ProjectItem{}
	})

	t.Run("FilterValue returns project name", func(t *testing.T) {
		item := tui.ProjectItem{Project: project.Project{Name: "portal", Path: "/home/user/code/portal"}}

		got := item.FilterValue()

		if got != "portal" {
			t.Errorf("FilterValue() = %q, want %q", got, "portal")
		}
	})

	t.Run("Title returns project name", func(t *testing.T) {
		item := tui.ProjectItem{Project: project.Project{Name: "myapp", Path: "/home/user/myapp"}}

		got := item.Title()

		if got != "myapp" {
			t.Errorf("Title() = %q, want %q", got, "myapp")
		}
	})

	t.Run("Description returns project path", func(t *testing.T) {
		item := tui.ProjectItem{Project: project.Project{Name: "portal", Path: "/home/user/code/portal"}}

		got := item.Description()

		if got != "/home/user/code/portal" {
			t.Errorf("Description() = %q, want %q", got, "/home/user/code/portal")
		}
	})
}

func TestProjectDelegate(t *testing.T) {
	t.Run("implements list.ItemDelegate interface", func(t *testing.T) {
		var _ list.ItemDelegate = tui.ProjectDelegate{}
	})

	t.Run("Height returns 2", func(t *testing.T) {
		d := tui.ProjectDelegate{}

		if got := d.Height(); got != 2 {
			t.Errorf("Height() = %d, want 2", got)
		}
	})

	t.Run("Spacing returns 0", func(t *testing.T) {
		d := tui.ProjectDelegate{}

		if got := d.Spacing(); got != 0 {
			t.Errorf("Spacing() = %d, want 0", got)
		}
	})

	t.Run("Update returns nil", func(t *testing.T) {
		d := tui.ProjectDelegate{}

		cmd := d.Update(nil, nil)

		if cmd != nil {
			t.Error("Update() should return nil")
		}
	})

	t.Run("renders project name and path", func(t *testing.T) {
		d := tui.ProjectDelegate{}
		items := []list.Item{
			tui.ProjectItem{Project: project.Project{Name: "portal", Path: "/home/user/code/portal"}},
		}
		m := list.New(items, d, 80, 10)

		var buf bytes.Buffer
		d.Render(&buf, m, 0, items[0])

		output := buf.String()
		if !strings.Contains(output, "portal") {
			t.Errorf("render output missing project name 'portal': %q", output)
		}
		if !strings.Contains(output, "/home/user/code/portal") {
			t.Errorf("render output missing project path '/home/user/code/portal': %q", output)
		}
	})

	t.Run("highlights selected item with the full-height bar, not a cursor", func(t *testing.T) {
		// The §6.2 reskin replaced the legacy "> " pink cursor with a full-height
		// accent.violet ▌ left bar over a bg.selection tint. The selected row carries
		// the ▌ bar on both lines; an unselected row carries no bar. (The exact SGR
		// roles are pinned in project_row_anatomy_test.go; this is the behavioural
		// selected-vs-unselected check that replaces the old cursor assertion.)
		d := tui.ProjectDelegate{}
		items := []list.Item{
			tui.ProjectItem{Project: project.Project{Name: "first", Path: "/home/user/first"}},
			tui.ProjectItem{Project: project.Project{Name: "second", Path: "/home/user/second"}},
		}
		m := list.New(items, d, 80, 10)
		// m.Index() defaults to 0, so index 0 is selected

		var selectedBuf bytes.Buffer
		d.Render(&selectedBuf, m, 0, items[0])
		selectedOutput := selectedBuf.String()

		var unselectedBuf bytes.Buffer
		d.Render(&unselectedBuf, m, 1, items[1])
		unselectedOutput := unselectedBuf.String()

		if !strings.Contains(selectedOutput, "▌") {
			t.Errorf("selected item should carry the ▌ full-height bar: %q", selectedOutput)
		}
		// The legacy "> " cursor must be gone.
		if strings.Contains(selectedOutput, "> ") {
			t.Errorf("selected item should not carry the legacy '> ' cursor: %q", selectedOutput)
		}
		if strings.Contains(unselectedOutput, "▌") {
			t.Errorf("unselected item should not carry the ▌ bar: %q", unselectedOutput)
		}
	})

	t.Run("over-long project path truncates with an ellipsis (2.7)", func(t *testing.T) {
		// The §6.2 reskin pins each row to the list width and truncates an over-long
		// path with an ellipsis (§2.7) so the two-line height stays uniform and
		// pagination never drifts (the legacy delegate rendered the full path verbatim).
		longPath := "/home/user/very/deeply/nested/directory/structure/that/goes/on/and/on/project"
		d := tui.ProjectDelegate{}
		items := []list.Item{
			tui.ProjectItem{Project: project.Project{Name: "deep-project", Path: longPath}},
		}
		const width = 40
		m := list.New(items, d, width, 10)

		var buf bytes.Buffer
		d.Render(&buf, m, 0, items[0])

		output := buf.String()
		if strings.Contains(output, longPath) {
			t.Errorf("over-long path should be truncated, but the full path rendered: %q", output)
		}
		if !strings.Contains(output, "…") {
			t.Errorf("truncated path should carry the ellipsis glyph: %q", output)
		}
	})

	t.Run("projects with identical names both render with different paths", func(t *testing.T) {
		d := tui.ProjectDelegate{}
		items := []list.Item{
			tui.ProjectItem{Project: project.Project{Name: "app", Path: "/home/user/work/app"}},
			tui.ProjectItem{Project: project.Project{Name: "app", Path: "/home/user/personal/app"}},
		}
		m := list.New(items, d, 80, 10)

		var buf1 bytes.Buffer
		d.Render(&buf1, m, 0, items[0])
		output1 := buf1.String()

		var buf2 bytes.Buffer
		d.Render(&buf2, m, 1, items[1])
		output2 := buf2.String()

		if !strings.Contains(output1, "/home/user/work/app") {
			t.Errorf("first item should contain path '/home/user/work/app': %q", output1)
		}
		if !strings.Contains(output2, "/home/user/personal/app") {
			t.Errorf("second item should contain path '/home/user/personal/app': %q", output2)
		}
	})
}

func TestProjectsToListItems(t *testing.T) {
	t.Run("converts projects to list items", func(t *testing.T) {
		projects := []project.Project{
			{Name: "portal", Path: "/home/user/code/portal"},
			{Name: "webapp", Path: "/home/user/code/webapp"},
			{Name: "cli-tool", Path: "/home/user/code/cli-tool"},
		}

		items := tui.ProjectsToListItems(projects)

		if len(items) != 3 {
			t.Fatalf("ProjectsToListItems() returned %d items, want 3", len(items))
		}

		for i, p := range projects {
			pi, ok := items[i].(tui.ProjectItem)
			if !ok {
				t.Fatalf("items[%d] is not a ProjectItem", i)
			}
			if pi.Project.Name != p.Name {
				t.Errorf("items[%d].Project.Name = %q, want %q", i, pi.Project.Name, p.Name)
			}
			if pi.Project.Path != p.Path {
				t.Errorf("items[%d].Project.Path = %q, want %q", i, pi.Project.Path, p.Path)
			}
		}
	})

	t.Run("empty projects returns empty items", func(t *testing.T) {
		items := tui.ProjectsToListItems([]project.Project{})

		if len(items) != 0 {
			t.Errorf("ProjectsToListItems([]) returned %d items, want 0", len(items))
		}
	})

	t.Run("nil projects returns empty items", func(t *testing.T) {
		items := tui.ProjectsToListItems(nil)

		if len(items) != 0 {
			t.Errorf("ProjectsToListItems(nil) returned %d items, want 0", len(items))
		}
	})
}
