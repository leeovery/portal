package project

import (
	"os"
	"path/filepath"
	"slices"
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

	t.Run("it resolves a session dir to the same project as a canonical-key scan", func(t *testing.T) {
		idx := NewIndex(projects)

		dir := projDir + string(os.PathSeparator)

		gotIdx, gotKey, okIdx := idx.Match(dir)

		// Independent oracle: the same match computed by a linear scan.
		wantKey := CanonicalDirKey(dir)
		var wantProj Project
		wantOk := false
		for _, p := range projects {
			if CanonicalDirKey(p.Path) == wantKey {
				wantProj = p
				wantOk = true
				break
			}
		}

		if !okIdx {
			t.Fatalf("Index.Match(%q) ok = false, want true", dir)
		}
		if okIdx != wantOk || gotIdx.Name != wantProj.Name || gotIdx.Path != wantProj.Path {
			t.Errorf("Index.Match(%q) = (%+v,%v), scan oracle = (%+v,%v)", dir, gotIdx, okIdx, wantProj, wantOk)
		}
		if gotIdx.Name != "Proj" {
			t.Errorf("Index.Match(%q) name = %q, want %q", dir, gotIdx.Name, "Proj")
		}
		if gotKey != wantKey {
			t.Errorf("Index.Match(%q) key = %q, want %q", dir, gotKey, wantKey)
		}
	})

	t.Run("it resolves a symlinked session dir to the project at its real target", func(t *testing.T) {
		idx := NewIndex(projects)

		link := filepath.Join(base, "link")
		if err := os.Symlink(projDir, link); err != nil {
			t.Fatalf("Symlink(%q -> %q) error = %v", link, projDir, err)
		}

		got, gotKey, ok := idx.Match(link)
		if !ok {
			t.Fatalf("Index.Match(%q) ok = false, want true", link)
		}
		if got.Name != "Proj" {
			t.Errorf("Index.Match(symlink) name = %q, want %q", got.Name, "Proj")
		}
		if want := CanonicalDirKey(link); gotKey != want {
			t.Errorf("Index.Match(symlink) key = %q, want %q", gotKey, want)
		}
	})

	t.Run("it returns not-found when the dir matches no project", func(t *testing.T) {
		idx := NewIndex(projects)

		nope := filepath.Join(base, "nope")
		got, gotKey, ok := idx.Match(nope)
		if ok {
			t.Errorf("Index.Match(unknown) ok = true, want false")
		}
		if got.Path != "" || got.Name != "" || !got.LastUsed.IsZero() || got.Tags != nil {
			t.Errorf("Index.Match(unknown) project = %+v, want zero value", got)
		}
		if want := CanonicalDirKey(nope); gotKey != want {
			t.Errorf("Index.Match(unknown) key = %q, want %q", gotKey, want)
		}
	})

	t.Run("it returns not-found for an empty dir", func(t *testing.T) {
		idx := NewIndex(projects)

		if _, _, ok := idx.Match(""); ok {
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

	projects := []Project{
		{Path: projDir, Name: "First"},
		{Path: projDir + string(os.PathSeparator), Name: "Last"},
	}

	idx := NewIndex(projects)

	got, _, ok := idx.Match(projDir)
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
		if _, _, ok := idx.Match("/anything"); ok {
			t.Errorf("Index.Match against nil-built index ok = true, want false")
		}
	})

	t.Run("empty projects yields a usable empty index", func(t *testing.T) {
		idx := NewIndex([]Project{})
		if _, _, ok := idx.Match("/anything"); ok {
			t.Errorf("Index.Match against empty-built index ok = true, want false")
		}
	})
}

func TestNewIndexBy(t *testing.T) {
	t.Run("stored paths and looked-up dirs are both reduced by the key", func(t *testing.T) {
		var reduced []string
		byBase := func(p string) string {
			reduced = append(reduced, p)
			return filepath.Base(p)
		}
		idx := NewIndexBy([]Project{{Path: "/stored/kiln", Name: "kiln"}}, byBase)

		got, key, ok := idx.Match("/elsewhere/kiln")
		if !ok || got.Name != "kiln" {
			t.Fatalf("Index.Match = (%+v, %v), want kiln matched through the key", got, ok)
		}
		if key != "kiln" {
			t.Errorf("Index.Match key = %q, want the key's %q", key, "kiln")
		}
		if want := []string{"/stored/kiln", "/elsewhere/kiln"}; !slices.Equal(reduced, want) {
			t.Errorf("key reduced %q, want %q", reduced, want)
		}
	})

	t.Run("a key in place of CanonicalDirKey resolves no symlink", func(t *testing.T) {
		base := t.TempDir()
		target := filepath.Join(base, "target")
		if err := os.Mkdir(target, 0o755); err != nil {
			t.Fatalf("Mkdir(%q) error = %v", target, err)
		}
		link := filepath.Join(base, "link")
		if err := os.Symlink(target, link); err != nil {
			t.Fatalf("Symlink error = %v", err)
		}
		projects := []Project{{Path: target, Name: "target"}}

		if _, _, ok := NewIndex(projects).Match(link); !ok {
			t.Fatalf("NewIndex Match(link) ok = false; the oracle needs CanonicalDirKey to resolve the link")
		}
		if _, _, ok := NewIndexBy(projects, filepath.Clean).Match(link); ok {
			t.Errorf("NewIndexBy(filepath.Clean) Match(link) ok = true, want the link left unresolved")
		}
	})

	t.Run("a nil key is CanonicalDirKey", func(t *testing.T) {
		dir := t.TempDir()
		idx := NewIndexBy([]Project{{Path: dir, Name: "p"}}, nil)
		if _, key, ok := idx.Match(dir + string(os.PathSeparator)); !ok || key != CanonicalDirKey(dir) {
			t.Errorf("nil-key Match = (key %q, %v), want (%q, true)", key, ok, CanonicalDirKey(dir))
		}
	})
}
