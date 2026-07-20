package project_test

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/project"
)

func TestLoad(t *testing.T) {
	t.Run("returns empty list when file does not exist", func(t *testing.T) {
		dir := t.TempDir()
		store := project.NewStore(filepath.Join(dir, "nonexistent", "projects.json"))

		projects, err := store.Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(projects) != 0 {
			t.Errorf("got %d projects, want 0", len(projects))
		}
	})

	t.Run("loads projects from valid JSON", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "projects.json")

		lastUsed := time.Date(2026, 1, 22, 10, 30, 0, 0, time.UTC)
		content := `{"projects":[{"path":"/Users/lee/Code/myapp","name":"myapp","last_used":"2026-01-22T10:30:00Z"}]}`
		if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		store := project.NewStore(filePath)
		projects, err := store.Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(projects) != 1 {
			t.Fatalf("got %d projects, want 1", len(projects))
		}

		p := projects[0]
		if p.Path != "/Users/lee/Code/myapp" {
			t.Errorf("Path = %q, want %q", p.Path, "/Users/lee/Code/myapp")
		}
		if p.Name != "myapp" {
			t.Errorf("Name = %q, want %q", p.Name, "myapp")
		}
		if !p.LastUsed.Equal(lastUsed) {
			t.Errorf("LastUsed = %v, want %v", p.LastUsed, lastUsed)
		}
	})

	t.Run("handles malformed JSON", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "projects.json")

		if err := os.WriteFile(filePath, []byte("{invalid json!!!"), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		store := project.NewStore(filePath)
		projects, err := store.Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(projects) != 0 {
			t.Errorf("got %d projects, want 0", len(projects))
		}
	})
}

func TestTagsField(t *testing.T) {
	t.Run("decodes a legacy record with no tags field to an empty Tags slice", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "projects.json")

		content := `{"projects":[{"path":"/a","name":"legacy","last_used":"2026-01-22T10:30:00Z"}]}`
		if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		store := project.NewStore(filePath)
		projects, err := store.Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(projects) != 1 {
			t.Fatalf("got %d projects, want 1", len(projects))
		}
		if projects[0].Tags != nil {
			t.Errorf("Tags = %#v, want nil", projects[0].Tags)
		}
		if len(projects[0].Tags) != 0 {
			t.Errorf("len(Tags) = %d, want 0", len(projects[0].Tags))
		}
	})

	t.Run("decodes an explicit null tags value to an empty Tags slice", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "projects.json")

		content := `{"projects":[{"path":"/a","name":"n","last_used":"2026-01-22T10:30:00Z","tags":null}]}`
		if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		store := project.NewStore(filePath)
		projects, err := store.Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(projects) != 1 {
			t.Fatalf("got %d projects, want 1", len(projects))
		}
		if len(projects[0].Tags) != 0 {
			t.Errorf("len(Tags) = %d, want 0", len(projects[0].Tags))
		}
	})

	t.Run("decodes an explicit [] tags value to an empty Tags slice", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "projects.json")

		content := `{"projects":[{"path":"/a","name":"n","last_used":"2026-01-22T10:30:00Z","tags":[]}]}`
		if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		store := project.NewStore(filePath)
		projects, err := store.Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(projects) != 1 {
			t.Fatalf("got %d projects, want 1", len(projects))
		}
		if len(projects[0].Tags) != 0 {
			t.Errorf("len(Tags) = %d, want 0", len(projects[0].Tags))
		}
	})

	t.Run("round-trips a record with multiple tags unchanged", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "projects.json")
		store := project.NewStore(filePath)

		projects := []project.Project{
			{
				Path:     "/Users/lee/Code/myapp",
				Name:     "myapp",
				LastUsed: time.Date(2026, 1, 22, 10, 30, 0, 0, time.UTC),
				Tags:     []string{"work", "personal"},
			},
		}

		if err := store.Save(projects); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		loaded, err := store.Load()
		if err != nil {
			t.Fatalf("failed to load: %v", err)
		}
		if len(loaded) != 1 {
			t.Fatalf("got %d projects, want 1", len(loaded))
		}

		want := []string{"work", "personal"}
		if len(loaded[0].Tags) != len(want) {
			t.Fatalf("Tags = %#v, want %#v", loaded[0].Tags, want)
		}
		for i, tag := range want {
			if loaded[0].Tags[i] != tag {
				t.Errorf("Tags[%d] = %q, want %q", i, loaded[0].Tags[i], tag)
			}
		}
	})

	t.Run("preserves Tags when Upsert updates an existing project's name and last_used", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "projects.json")
		store := project.NewStore(filePath)

		// Seed an existing record that already carries tags.
		seeded := []project.Project{
			{
				Path:     "/Users/lee/Code/myapp",
				Name:     "myapp",
				LastUsed: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				Tags:     []string{"work", "personal"},
			},
		}
		if err := store.Save(seeded); err != nil {
			t.Fatalf("failed to seed: %v", err)
		}

		// Upsert the same path with a new name; must not clobber Tags.
		if err := store.Upsert("/Users/lee/Code/myapp", "renamed-app", "internal"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		projects, err := store.Load()
		if err != nil {
			t.Fatalf("failed to load: %v", err)
		}
		if len(projects) != 1 {
			t.Fatalf("got %d projects, want 1", len(projects))
		}
		if projects[0].Name != "renamed-app" {
			t.Errorf("Name = %q, want %q", projects[0].Name, "renamed-app")
		}
		if projects[0].LastUsed.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) {
			t.Errorf("LastUsed should have been bumped, got %v", projects[0].LastUsed)
		}

		want := []string{"work", "personal"}
		if len(projects[0].Tags) != len(want) {
			t.Fatalf("Tags = %#v, want %#v", projects[0].Tags, want)
		}
		for i, tag := range want {
			if projects[0].Tags[i] != tag {
				t.Errorf("Tags[%d] = %q, want %q", i, projects[0].Tags[i], tag)
			}
		}
	})
}

func TestSave(t *testing.T) {
	t.Run("creates config directory on save", func(t *testing.T) {
		dir := t.TempDir()
		nested := filepath.Join(dir, "portal", "sub")
		filePath := filepath.Join(nested, "projects.json")
		store := project.NewStore(filePath)

		projects := []project.Project{
			{
				Path:     "/Users/lee/Code/myapp",
				Name:     "myapp",
				LastUsed: time.Date(2026, 1, 22, 10, 30, 0, 0, time.UTC),
			},
		}

		if err := store.Save(projects); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify the directory was created
		info, err := os.Stat(nested)
		if err != nil {
			t.Fatalf("directory not created: %v", err)
		}
		if !info.IsDir() {
			t.Errorf("expected directory, got file")
		}

		// Verify the file is readable and contains correct data
		loaded, err := store.Load()
		if err != nil {
			t.Fatalf("failed to load saved file: %v", err)
		}
		if len(loaded) != 1 {
			t.Fatalf("got %d projects, want 1", len(loaded))
		}
		if loaded[0].Path != "/Users/lee/Code/myapp" {
			t.Errorf("Path = %q, want %q", loaded[0].Path, "/Users/lee/Code/myapp")
		}
		if loaded[0].Name != "myapp" {
			t.Errorf("Name = %q, want %q", loaded[0].Name, "myapp")
		}
		if !loaded[0].LastUsed.Equal(time.Date(2026, 1, 22, 10, 30, 0, 0, time.UTC)) {
			t.Errorf("LastUsed = %v, want %v", loaded[0].LastUsed, time.Date(2026, 1, 22, 10, 30, 0, 0, time.UTC))
		}
	})
}

func TestUpsert(t *testing.T) {
	t.Run("adds new project to empty store", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "projects.json")
		store := project.NewStore(filePath)

		if err := store.Upsert("/Users/lee/Code/myapp", "myapp", "internal"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		projects, err := store.Load()
		if err != nil {
			t.Fatalf("failed to load: %v", err)
		}

		if len(projects) != 1 {
			t.Fatalf("got %d projects, want 1", len(projects))
		}

		if projects[0].Path != "/Users/lee/Code/myapp" {
			t.Errorf("Path = %q, want %q", projects[0].Path, "/Users/lee/Code/myapp")
		}
		if projects[0].Name != "myapp" {
			t.Errorf("Name = %q, want %q", projects[0].Name, "myapp")
		}
		if projects[0].LastUsed.IsZero() {
			t.Errorf("LastUsed should not be zero")
		}
	})

	t.Run("updates existing project by path", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "projects.json")
		store := project.NewStore(filePath)

		// Add initial project
		if err := store.Upsert("/Users/lee/Code/myapp", "myapp", "internal"); err != nil {
			t.Fatalf("unexpected error on first upsert: %v", err)
		}

		// Record the first timestamp
		firstLoad, err := store.Load()
		if err != nil {
			t.Fatalf("failed to load: %v", err)
		}
		firstLastUsed := firstLoad[0].LastUsed

		// Wait a tiny bit so time advances
		time.Sleep(10 * time.Millisecond)

		// Upsert with same path but different name
		if err := store.Upsert("/Users/lee/Code/myapp", "renamed-app", "internal"); err != nil {
			t.Fatalf("unexpected error on second upsert: %v", err)
		}

		projects, err := store.Load()
		if err != nil {
			t.Fatalf("failed to load: %v", err)
		}

		if len(projects) != 1 {
			t.Fatalf("got %d projects, want 1 (should update, not add)", len(projects))
		}

		if projects[0].Name != "renamed-app" {
			t.Errorf("Name = %q, want %q", projects[0].Name, "renamed-app")
		}
		if !projects[0].LastUsed.After(firstLastUsed) {
			t.Errorf("LastUsed should be updated: got %v, first was %v", projects[0].LastUsed, firstLastUsed)
		}
	})

	t.Run("adds second project without replacing first", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "projects.json")
		store := project.NewStore(filePath)

		if err := store.Upsert("/Users/lee/Code/myapp", "myapp", "internal"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := store.Upsert("/Users/lee/Code/other", "other", "internal"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		projects, err := store.Load()
		if err != nil {
			t.Fatalf("failed to load: %v", err)
		}

		if len(projects) != 2 {
			t.Fatalf("got %d projects, want 2", len(projects))
		}
	})
}

func TestList(t *testing.T) {
	t.Run("returns projects sorted by last_used descending", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "projects.json")

		// Write projects in non-sorted order
		content := `{"projects":[
			{"path":"/a","name":"oldest","last_used":"2026-01-01T00:00:00Z"},
			{"path":"/c","name":"newest","last_used":"2026-03-01T00:00:00Z"},
			{"path":"/b","name":"middle","last_used":"2026-02-01T00:00:00Z"}
		]}`
		if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		store := project.NewStore(filePath)
		projects, err := store.List()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(projects) != 3 {
			t.Fatalf("got %d projects, want 3", len(projects))
		}

		wantOrder := []string{"newest", "middle", "oldest"}
		for i, want := range wantOrder {
			if projects[i].Name != want {
				t.Errorf("projects[%d].Name = %q, want %q", i, projects[i].Name, want)
			}
		}
	})

	t.Run("returns empty list when file missing", func(t *testing.T) {
		dir := t.TempDir()
		store := project.NewStore(filepath.Join(dir, "nonexistent", "projects.json"))

		projects, err := store.List()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(projects) != 0 {
			t.Errorf("got %d projects, want 0", len(projects))
		}
	})
}

func TestRemove(t *testing.T) {
	t.Run("removes project by path", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "projects.json")

		content := `{"projects":[
			{"path":"/a","name":"first","last_used":"2026-01-01T00:00:00Z"},
			{"path":"/b","name":"second","last_used":"2026-02-01T00:00:00Z"}
		]}`
		if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		store := project.NewStore(filePath)

		if err := store.Remove("/a", "cli"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		projects, err := store.Load()
		if err != nil {
			t.Fatalf("failed to load: %v", err)
		}

		if len(projects) != 1 {
			t.Fatalf("got %d projects, want 1", len(projects))
		}

		if projects[0].Path != "/b" {
			t.Errorf("remaining project Path = %q, want %q", projects[0].Path, "/b")
		}
	})

	t.Run("no error when removing nonexistent path", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "projects.json")

		content := `{"projects":[{"path":"/a","name":"first","last_used":"2026-01-01T00:00:00Z"}]}`
		if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		store := project.NewStore(filePath)

		if err := store.Remove("/nonexistent", "cli"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		projects, err := store.Load()
		if err != nil {
			t.Fatalf("failed to load: %v", err)
		}

		if len(projects) != 1 {
			t.Fatalf("got %d projects, want 1 (original should remain)", len(projects))
		}
	})
}

func TestRename(t *testing.T) {
	t.Run("renames project by path without changing last_used", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "projects.json")

		lastUsed := time.Date(2026, 1, 22, 10, 30, 0, 0, time.UTC)
		content := `{"projects":[{"path":"/Users/lee/Code/myapp","name":"myapp","last_used":"2026-01-22T10:30:00Z"}]}`
		if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		store := project.NewStore(filePath)

		if err := store.Rename("/Users/lee/Code/myapp", "renamed-app", "cli"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		projects, err := store.Load()
		if err != nil {
			t.Fatalf("failed to load: %v", err)
		}

		if len(projects) != 1 {
			t.Fatalf("got %d projects, want 1", len(projects))
		}

		if projects[0].Name != "renamed-app" {
			t.Errorf("Name = %q, want %q", projects[0].Name, "renamed-app")
		}
		if !projects[0].LastUsed.Equal(lastUsed) {
			t.Errorf("LastUsed changed: got %v, want %v", projects[0].LastUsed, lastUsed)
		}
	})

	t.Run("no error when renaming nonexistent path", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "projects.json")

		content := `{"projects":[{"path":"/a","name":"first","last_used":"2026-01-01T00:00:00Z"}]}`
		if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		store := project.NewStore(filePath)

		if err := store.Rename("/nonexistent", "new-name", "cli"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Original should be unchanged
		projects, err := store.Load()
		if err != nil {
			t.Fatalf("failed to load: %v", err)
		}

		if len(projects) != 1 {
			t.Fatalf("got %d projects, want 1", len(projects))
		}

		if projects[0].Name != "first" {
			t.Errorf("Name = %q, want %q (should be unchanged)", projects[0].Name, "first")
		}
	})
}

func TestStaleEntries(t *testing.T) {
	t.Run("classifies present as live, missing as stale, permission-denied as retained", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "projects.json")

		liveDir := t.TempDir()
		goneDir := filepath.Join(dir, "gone")

		// A child under a 0000 parent so os.Stat returns permission denied
		// (retained, NOT stale) — the tri-state default branch.
		parentDir := filepath.Join(dir, "restricted")
		deniedDir := filepath.Join(parentDir, "child")
		if err := os.MkdirAll(deniedDir, 0o755); err != nil {
			t.Fatalf("failed to create child dir: %v", err)
		}
		if err := os.Chmod(parentDir, 0o000); err != nil {
			t.Fatalf("failed to chmod: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(parentDir, 0o755) })

		content := `{"projects":[
			{"path":"` + liveDir + `","name":"live","last_used":"2026-01-01T00:00:00Z"},
			{"path":"` + goneDir + `","name":"stale","last_used":"2026-02-01T00:00:00Z"},
			{"path":"` + deniedDir + `","name":"denied","last_used":"2026-03-01T00:00:00Z"}
		]}`
		if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		store := project.NewStore(filePath)
		stale, err := store.StaleEntries()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(stale) != 1 {
			t.Fatalf("len(stale) = %d, want 1 (only the gone dir): %+v", len(stale), stale)
		}
		if stale[0].Path != goneDir {
			t.Errorf("stale[0].Path = %q, want %q", stale[0].Path, goneDir)
		}
	})

	t.Run("returns empty when every directory exists", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "projects.json")
		liveDir := t.TempDir()

		content := `{"projects":[{"path":"` + liveDir + `","name":"live","last_used":"2026-01-01T00:00:00Z"}]}`
		if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		store := project.NewStore(filePath)
		stale, err := store.StaleEntries()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(stale) != 0 {
			t.Errorf("len(stale) = %d, want 0: %+v", len(stale), stale)
		}
	})
}

// TestCleanStaleRemovesExactlyStaleEntries proves CleanStale removes precisely
// the set the shared StaleEntries predicate reports — the prune and the doctor
// diagnosis provably share one classification and cannot drift.
func TestCleanStaleRemovesExactlyStaleEntries(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "projects.json")

	liveDir := t.TempDir()
	goneA := filepath.Join(dir, "gone-a")
	goneB := filepath.Join(dir, "gone-b")

	content := `{"projects":[
		{"path":"` + liveDir + `","name":"live","last_used":"2026-01-01T00:00:00Z"},
		{"path":"` + goneA + `","name":"staleA","last_used":"2026-02-01T00:00:00Z"},
		{"path":"` + goneB + `","name":"staleB","last_used":"2026-03-01T00:00:00Z"}
	]}`
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	store := project.NewStore(filePath)

	predicted, err := store.StaleEntries()
	if err != nil {
		t.Fatalf("StaleEntries: %v", err)
	}
	removed, err := store.CleanStale()
	if err != nil {
		t.Fatalf("CleanStale: %v", err)
	}

	predictedPaths := pathsOf(predicted)
	removedPaths := pathsOf(removed)
	if len(removedPaths) != len(predictedPaths) {
		t.Fatalf("CleanStale removed %v, StaleEntries predicted %v", removedPaths, predictedPaths)
	}
	for i := range predictedPaths {
		if removedPaths[i] != predictedPaths[i] {
			t.Errorf("removed[%d] = %q, StaleEntries[%d] = %q", i, removedPaths[i], i, predictedPaths[i])
		}
	}
}

// pathsOf returns the sorted Path fields of the given projects.
func pathsOf(projects []project.Project) []string {
	paths := make([]string, 0, len(projects))
	for _, p := range projects {
		paths = append(paths, p.Path)
	}
	sort.Strings(paths)
	return paths
}

func TestCleanStale(t *testing.T) {
	t.Run("removes project with non-existent directory", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "projects.json")

		// existingDir is a real directory; staleDir does not exist
		existingDir := t.TempDir()
		staleDir := filepath.Join(dir, "gone")

		content := `{"projects":[
			{"path":"` + existingDir + `","name":"exists","last_used":"2026-01-01T00:00:00Z"},
			{"path":"` + staleDir + `","name":"stale","last_used":"2026-02-01T00:00:00Z"}
		]}`
		if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		store := project.NewStore(filePath)

		removed, err := store.CleanStale()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(removed) != 1 {
			t.Fatalf("len(removed) = %d, want 1", len(removed))
		}
		if removed[0].Path != staleDir {
			t.Errorf("removed[0].Path = %q, want %q", removed[0].Path, staleDir)
		}

		projects, err := store.Load()
		if err != nil {
			t.Fatalf("failed to load: %v", err)
		}

		if len(projects) != 1 {
			t.Fatalf("got %d projects, want 1", len(projects))
		}

		if projects[0].Path != existingDir {
			t.Errorf("remaining project Path = %q, want %q", projects[0].Path, existingDir)
		}
	})

	t.Run("retains project with existing directory", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "projects.json")

		existingDir := t.TempDir()

		content := `{"projects":[
			{"path":"` + existingDir + `","name":"exists","last_used":"2026-01-01T00:00:00Z"}
		]}`
		if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		store := project.NewStore(filePath)

		removed, err := store.CleanStale()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(removed) != 0 {
			t.Errorf("len(removed) = %d, want 0", len(removed))
		}

		projects, err := store.Load()
		if err != nil {
			t.Fatalf("failed to load: %v", err)
		}

		if len(projects) != 1 {
			t.Fatalf("got %d projects, want 1", len(projects))
		}

		if projects[0].Path != existingDir {
			t.Errorf("project Path = %q, want %q", projects[0].Path, existingDir)
		}
	})

	t.Run("retains project with permission denied", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "projects.json")

		// Create a parent dir, then a child inside it, then remove perms on parent
		parentDir := filepath.Join(dir, "restricted")
		childDir := filepath.Join(parentDir, "child")
		if err := os.MkdirAll(childDir, 0o755); err != nil {
			t.Fatalf("failed to create child dir: %v", err)
		}
		// Remove execute permission on parent so stat on child returns permission denied
		if err := os.Chmod(parentDir, 0o000); err != nil {
			t.Fatalf("failed to chmod: %v", err)
		}
		t.Cleanup(func() {
			_ = os.Chmod(parentDir, 0o755)
		})

		content := `{"projects":[
			{"path":"` + childDir + `","name":"restricted","last_used":"2026-01-01T00:00:00Z"}
		]}`
		if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		store := project.NewStore(filePath)

		removed, err := store.CleanStale()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(removed) != 0 {
			t.Errorf("len(removed) = %d, want 0", len(removed))
		}

		projects, err := store.Load()
		if err != nil {
			t.Fatalf("failed to load: %v", err)
		}

		if len(projects) != 1 {
			t.Fatalf("got %d projects, want 1", len(projects))
		}

		if projects[0].Name != "restricted" {
			t.Errorf("project Name = %q, want %q", projects[0].Name, "restricted")
		}
	})

	t.Run("returns empty slice on empty list", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "projects.json")
		store := project.NewStore(filePath)

		removed, err := store.CleanStale()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(removed) != 0 {
			t.Errorf("len(removed) = %d, want 0", len(removed))
		}
	})

	t.Run("removes multiple stale in single call", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "projects.json")

		existingDir := t.TempDir()
		staleDir1 := filepath.Join(dir, "gone1")
		staleDir2 := filepath.Join(dir, "gone2")

		content := `{"projects":[
			{"path":"` + existingDir + `","name":"exists","last_used":"2026-01-01T00:00:00Z"},
			{"path":"` + staleDir1 + `","name":"stale1","last_used":"2026-02-01T00:00:00Z"},
			{"path":"` + staleDir2 + `","name":"stale2","last_used":"2026-03-01T00:00:00Z"}
		]}`
		if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		store := project.NewStore(filePath)

		removed, err := store.CleanStale()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(removed) != 2 {
			t.Fatalf("len(removed) = %d, want 2", len(removed))
		}

		projects, err := store.Load()
		if err != nil {
			t.Fatalf("failed to load: %v", err)
		}

		if len(projects) != 1 {
			t.Fatalf("got %d projects, want 1", len(projects))
		}

		if projects[0].Path != existingDir {
			t.Errorf("remaining project Path = %q, want %q", projects[0].Path, existingDir)
		}
	})
}
