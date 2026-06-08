package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIndexMatch(t *testing.T) {
	base := t.TempDir()
	projDir := filepath.Join(base, "proj")
	if err := os.Mkdir(projDir, 0o755); err != nil {
		t.Fatalf("Mkdir(%q) error = %v", projDir, err)
	}
	otherDir := filepath.Join(base, "other")
	if err := os.Mkdir(otherDir, 0o755); err != nil {
		t.Fatalf("Mkdir(%q) error = %v", otherDir, err)
	}

	projects := []Project{
		{Path: projDir, Name: "Proj"},
		{Path: otherDir, Name: "Other"},
	}

	t.Run("it resolves a session dir to the same project as MatchProjectByDir", func(t *testing.T) {
		idx := NewIndex(projects)

		// Pass a trailing-slash variant to confirm canonicalisation on both sides.
		dir := projDir + string(os.PathSeparator)

		gotIdx, okIdx := idx.Match(dir)
		gotMatch, okMatch := MatchProjectByDir(projects, dir)
		if !okIdx {
			t.Fatalf("Index.Match(%q) ok = false, want true", dir)
		}
		if okIdx != okMatch || gotIdx.Name != gotMatch.Name || gotIdx.Path != gotMatch.Path {
			t.Errorf("Index.Match(%q) = (%+v,%v), MatchProjectByDir = (%+v,%v)", dir, gotIdx, okIdx, gotMatch, okMatch)
		}
		if gotIdx.Name != "Proj" {
			t.Errorf("Index.Match(%q) name = %q, want %q", dir, gotIdx.Name, "Proj")
		}
	})

	t.Run("it resolves a symlinked session dir to the project at its real target", func(t *testing.T) {
		idx := NewIndex(projects)

		link := filepath.Join(base, "link")
		if err := os.Symlink(projDir, link); err != nil {
			t.Fatalf("Symlink(%q -> %q) error = %v", link, projDir, err)
		}

		got, ok := idx.Match(link)
		if !ok {
			t.Fatalf("Index.Match(%q) ok = false, want true", link)
		}
		if got.Name != "Proj" {
			t.Errorf("Index.Match(symlink) name = %q, want %q", got.Name, "Proj")
		}
	})

	t.Run("it returns not-found when the dir matches no project", func(t *testing.T) {
		idx := NewIndex(projects)

		got, ok := idx.Match(filepath.Join(base, "nope"))
		if ok {
			t.Errorf("Index.Match(unknown) ok = true, want false")
		}
		if got.Path != "" || got.Name != "" || !got.LastUsed.IsZero() || got.Tags != nil {
			t.Errorf("Index.Match(unknown) project = %+v, want zero value", got)
		}
	})

	t.Run("it returns not-found for an empty dir", func(t *testing.T) {
		idx := NewIndex(projects)

		if _, ok := idx.Match(""); ok {
			t.Errorf("Index.Match(\"\") ok = true, want false")
		}
	})
}

func TestNewIndexCollisionLastWins(t *testing.T) {
	base := t.TempDir()
	projDir := filepath.Join(base, "proj")
	if err := os.Mkdir(projDir, 0o755); err != nil {
		t.Fatalf("Mkdir(%q) error = %v", projDir, err)
	}

	// Two projects with the same canonical key (same dir, one trailing-slash
	// variant). Documented policy: last-write-wins.
	projects := []Project{
		{Path: projDir, Name: "First"},
		{Path: projDir + string(os.PathSeparator), Name: "Last"},
	}

	idx := NewIndex(projects)

	got, ok := idx.Match(projDir)
	if !ok {
		t.Fatalf("Index.Match(%q) ok = false, want true", projDir)
	}
	if got.Name != "Last" {
		t.Errorf("collision Index.Match name = %q, want %q (last-write-wins)", got.Name, "Last")
	}
}

func TestNewIndexEmpty(t *testing.T) {
	t.Run("nil projects yields a usable empty index", func(t *testing.T) {
		idx := NewIndex(nil)
		if _, ok := idx.Match("/anything"); ok {
			t.Errorf("Index.Match against nil-built index ok = true, want false")
		}
	})

	t.Run("empty projects yields a usable empty index", func(t *testing.T) {
		idx := NewIndex([]Project{})
		if _, ok := idx.Match("/anything"); ok {
			t.Errorf("Index.Match against empty-built index ok = true, want false")
		}
	})
}
