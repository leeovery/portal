package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/tmux"
)

// mustMkdir creates dir (and parents) so CanonicalDirKey's EvalSymlinks
// resolves against a real on-disk path, keeping test keys deterministic.
func mustMkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", dir, err)
	}
}

// asSessionItem unwraps a list.Item into a SessionItem, failing the test if the
// item is not a SessionItem (no header should ever appear in the slice).
func asSessionItem(t *testing.T, item list.Item) SessionItem {
	t.Helper()
	si, ok := item.(SessionItem)
	if !ok {
		t.Fatalf("item %v is not a SessionItem", item)
	}
	return si
}

func TestBuildByProject(t *testing.T) {
	t.Run("builds one item per session under its project name heading", func(t *testing.T) {
		dir := t.TempDir()
		key := project.CanonicalDirKey(dir)
		projects := []project.Project{{Path: dir, Name: "Portal"}}
		sessions := []tmux.Session{{Name: "portal-abc", Dir: dir}}

		items := buildByProject(sessions, projects)

		if len(items) != 1 {
			t.Fatalf("len(items) = %d, want 1", len(items))
		}
		si := asSessionItem(t, items[0])
		if si.Session.Name != "portal-abc" {
			t.Errorf("Session.Name = %q, want %q", si.Session.Name, "portal-abc")
		}
		if si.GroupKey != key {
			t.Errorf("GroupKey = %q, want %q", si.GroupKey, key)
		}
		if si.GroupHeading != "Portal" {
			t.Errorf("GroupHeading = %q, want %q", si.GroupHeading, "Portal")
		}
		if si.Tag != "" {
			t.Errorf("Tag = %q, want empty", si.Tag)
		}
		if si.CatchAll {
			t.Errorf("CatchAll = true, want false")
		}
	})

	t.Run("orders items by group key then session name", func(t *testing.T) {
		base := t.TempDir()
		dirA := filepath.Join(base, "a")
		dirB := filepath.Join(base, "b")
		mustMkdir(t, dirA)
		mustMkdir(t, dirB)
		keyA := project.CanonicalDirKey(dirA)
		keyB := project.CanonicalDirKey(dirB)

		projects := []project.Project{
			{Path: dirA, Name: "Alpha"},
			{Path: dirB, Name: "Bravo"},
		}
		// Deliberately unsorted input: B before A, and within A z before a.
		sessions := []tmux.Session{
			{Name: "bravo-1", Dir: dirB},
			{Name: "alpha-z", Dir: dirA},
			{Name: "alpha-a", Dir: dirA},
		}

		items := buildByProject(sessions, projects)

		if len(items) != 3 {
			t.Fatalf("len(items) = %d, want 3", len(items))
		}
		wantOrder := []struct {
			key  string
			name string
		}{
			{keyA, "alpha-a"},
			{keyA, "alpha-z"},
			{keyB, "bravo-1"},
		}
		for i, want := range wantOrder {
			si := asSessionItem(t, items[i])
			if si.GroupKey != want.key || si.Session.Name != want.name {
				t.Errorf("item[%d] = (%q, %q), want (%q, %q)", i, si.GroupKey, si.Session.Name, want.key, want.name)
			}
		}
	})

	t.Run("forms two separate groups for two distinct dirs sharing a project name", func(t *testing.T) {
		base := t.TempDir()
		dir1 := filepath.Join(base, "code", "portal")
		dir2 := filepath.Join(base, "archive", "portal")
		mustMkdir(t, dir1)
		mustMkdir(t, dir2)
		key1 := project.CanonicalDirKey(dir1)
		key2 := project.CanonicalDirKey(dir2)

		projects := []project.Project{
			{Path: dir1, Name: "Portal"},
			{Path: dir2, Name: "Portal"},
		}
		sessions := []tmux.Session{
			{Name: "s1", Dir: dir1},
			{Name: "s2", Dir: dir2},
		}

		items := buildByProject(sessions, projects)

		if len(items) != 2 {
			t.Fatalf("len(items) = %d, want 2", len(items))
		}
		keys := map[string]bool{}
		for _, it := range items {
			si := asSessionItem(t, it)
			if si.GroupHeading != "Portal" {
				t.Errorf("GroupHeading = %q, want %q", si.GroupHeading, "Portal")
			}
			keys[si.GroupKey] = true
		}
		if !keys[key1] || !keys[key2] {
			t.Errorf("expected two distinct group keys %q and %q, got %v", key1, key2, keys)
		}
		if len(keys) != 2 {
			t.Errorf("len(distinct keys) = %d, want 2", len(keys))
		}
	})

	t.Run("routes a session with empty Dir to the Unknown bucket", func(t *testing.T) {
		sessions := []tmux.Session{{Name: "no-dir"}}

		items := buildByProject(sessions, nil)

		if len(items) != 1 {
			t.Fatalf("len(items) = %d, want 1", len(items))
		}
		si := asSessionItem(t, items[0])
		if !si.CatchAll {
			t.Errorf("CatchAll = false, want true")
		}
		if si.GroupHeading != "Unknown" {
			t.Errorf("GroupHeading = %q, want %q", si.GroupHeading, "Unknown")
		}
		if si.Session.Name != "no-dir" {
			t.Errorf("Session.Name = %q, want %q", si.Session.Name, "no-dir")
		}
	})

	t.Run("routes a stamped dir with no matching project record to the Unknown bucket", func(t *testing.T) {
		dir := t.TempDir()
		// No project record for dir → deleted project / live session.
		sessions := []tmux.Session{{Name: "orphan", Dir: dir}}

		items := buildByProject(sessions, nil)

		if len(items) != 1 {
			t.Fatalf("len(items) = %d, want 1", len(items))
		}
		si := asSessionItem(t, items[0])
		if !si.CatchAll {
			t.Errorf("CatchAll = false, want true")
		}
		if si.GroupHeading != "Unknown" {
			t.Errorf("GroupHeading = %q, want %q", si.GroupHeading, "Unknown")
		}
	})

	t.Run("appends Unknown items after known-project items", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Known"}}
		sessions := []tmux.Session{
			{Name: "unknown-1"},
			{Name: "known-1", Dir: dir},
		}

		items := buildByProject(sessions, projects)

		if len(items) != 2 {
			t.Fatalf("len(items) = %d, want 2", len(items))
		}
		first := asSessionItem(t, items[0])
		if first.CatchAll {
			t.Errorf("first item is CatchAll, want known-project item first")
		}
		last := asSessionItem(t, items[1])
		if !last.CatchAll {
			t.Errorf("last item is not CatchAll, want Unknown item last")
		}
	})

	t.Run("returns an empty slice for zero live sessions", func(t *testing.T) {
		items := buildByProject(nil, nil)

		if len(items) != 0 {
			t.Fatalf("len(items) = %d, want 0", len(items))
		}
	})
}
