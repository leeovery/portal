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

	t.Run("it resolves a session dir to the same project as a canonical-key scan", func(t *testing.T) {
		idx := NewIndex(projects)

		// Pass a trailing-slash variant to confirm canonicalisation on both sides.
		dir := projDir + string(os.PathSeparator)

		gotIdx, gotKey, okIdx := idx.Match(dir)

		// Inline oracle: scan the project set for one whose canonical key equals
		// the canonical key of the lookup dir — the same logic the index
		// performs, computed independently here as a differential cross-check.
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
		// The returned key is always the canonical form of the input dir.
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
		// The returned key captures EvalSymlinks resolution: it equals the
		// canonical (symlink-resolved) form of the symlinked input, which is the
		// same key callers reuse as the By-Project GroupKey.
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
		// Even on a miss the key is still the canonicalised input — a valid key
		// that simply isn't present in the map.
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

	// Two projects with the same canonical key (same dir, one trailing-slash
	// variant). Documented policy: last-write-wins.
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
