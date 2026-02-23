package alias_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/portal/internal/alias"
)

func TestLoad(t *testing.T) {
	t.Run("returns empty map when file does not exist", func(t *testing.T) {
		dir := t.TempDir()
		store := alias.NewStore(filepath.Join(dir, "nonexistent", "aliases"))

		aliases, err := store.Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(aliases) != 0 {
			t.Errorf("got %d aliases, want 0", len(aliases))
		}
	})

	t.Run("loads aliases from valid file", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "aliases")

		content := "m2api=/Users/lee/Code/mac2/api\naa=/Users/lee/Code/aerobid/api\n"
		if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		store := alias.NewStore(filePath)
		aliases, err := store.Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(aliases) != 2 {
			t.Fatalf("got %d aliases, want 2", len(aliases))
		}

		if aliases["m2api"] != "/Users/lee/Code/mac2/api" {
			t.Errorf("m2api = %q, want %q", aliases["m2api"], "/Users/lee/Code/mac2/api")
		}
		if aliases["aa"] != "/Users/lee/Code/aerobid/api" {
			t.Errorf("aa = %q, want %q", aliases["aa"], "/Users/lee/Code/aerobid/api")
		}
	})

	t.Run("handles duplicate keys with last wins", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "aliases")

		content := "myalias=/first/path\nmyalias=/second/path\n"
		if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		store := alias.NewStore(filePath)
		aliases, err := store.Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(aliases) != 1 {
			t.Fatalf("got %d aliases, want 1", len(aliases))
		}

		if aliases["myalias"] != "/second/path" {
			t.Errorf("myalias = %q, want %q", aliases["myalias"], "/second/path")
		}
	})

	t.Run("handles empty file", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "aliases")

		if err := os.WriteFile(filePath, []byte(""), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		store := alias.NewStore(filePath)
		aliases, err := store.Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(aliases) != 0 {
			t.Errorf("got %d aliases, want 0", len(aliases))
		}
	})
}

func TestSave(t *testing.T) {
	t.Run("creates config directory on save", func(t *testing.T) {
		dir := t.TempDir()
		nested := filepath.Join(dir, "portal", "sub")
		filePath := filepath.Join(nested, "aliases")
		store := alias.NewStore(filePath)

		store.Set("myalias", "/some/path")

		if err := store.Save(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		info, err := os.Stat(nested)
		if err != nil {
			t.Fatalf("directory not created: %v", err)
		}
		if !info.IsDir() {
			t.Errorf("expected directory, got file")
		}

		// Verify file contents via reload
		store2 := alias.NewStore(filePath)
		aliases, err := store2.Load()
		if err != nil {
			t.Fatalf("failed to reload: %v", err)
		}

		if aliases["myalias"] != "/some/path" {
			t.Errorf("myalias = %q, want %q", aliases["myalias"], "/some/path")
		}
	})
}

func TestSet(t *testing.T) {
	t.Run("adds new alias", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "aliases")
		store := alias.NewStore(filePath)

		store.Set("proj", "/Users/lee/Code/project")

		got, ok := store.Get("proj")
		if !ok {
			t.Fatal("expected alias to exist")
		}
		if got != "/Users/lee/Code/project" {
			t.Errorf("got %q, want %q", got, "/Users/lee/Code/project")
		}
	})

	t.Run("overwrites existing alias", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "aliases")
		store := alias.NewStore(filePath)

		store.Set("proj", "/first/path")
		store.Set("proj", "/second/path")

		got, ok := store.Get("proj")
		if !ok {
			t.Fatal("expected alias to exist")
		}
		if got != "/second/path" {
			t.Errorf("got %q, want %q", got, "/second/path")
		}
	})
}

func TestGet(t *testing.T) {
	t.Run("returns path and true for existing alias", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "aliases")
		store := alias.NewStore(filePath)

		store.Set("proj", "/Users/lee/Code/project")

		got, ok := store.Get("proj")
		if !ok {
			t.Fatal("expected alias to exist")
		}
		if got != "/Users/lee/Code/project" {
			t.Errorf("got %q, want %q", got, "/Users/lee/Code/project")
		}
	})

	t.Run("returns empty and false for non-existent alias", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "aliases")
		store := alias.NewStore(filePath)

		got, ok := store.Get("nonexistent")
		if ok {
			t.Fatal("expected alias to not exist")
		}
		if got != "" {
			t.Errorf("got %q, want empty string", got)
		}
	})
}

func TestDelete(t *testing.T) {
	t.Run("removes existing alias", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "aliases")
		store := alias.NewStore(filePath)

		store.Set("proj", "/Users/lee/Code/project")

		removed := store.Delete("proj")
		if !removed {
			t.Error("expected Delete to return true for existing alias")
		}

		_, ok := store.Get("proj")
		if ok {
			t.Error("expected alias to be removed")
		}
	})

	t.Run("returns false for non-existent alias", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "aliases")
		store := alias.NewStore(filePath)

		removed := store.Delete("nonexistent")
		if removed {
			t.Error("expected Delete to return false for non-existent alias")
		}
	})
}

func TestList(t *testing.T) {
	t.Run("returns sorted aliases", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "aliases")
		store := alias.NewStore(filePath)

		store.Set("zebra", "/z/path")
		store.Set("apple", "/a/path")
		store.Set("mango", "/m/path")

		list := store.List()

		if len(list) != 3 {
			t.Fatalf("got %d aliases, want 3", len(list))
		}

		wantNames := []string{"apple", "mango", "zebra"}
		wantPaths := []string{"/a/path", "/m/path", "/z/path"}

		for i, a := range list {
			if a.Name != wantNames[i] {
				t.Errorf("list[%d].Name = %q, want %q", i, a.Name, wantNames[i])
			}
			if a.Path != wantPaths[i] {
				t.Errorf("list[%d].Path = %q, want %q", i, a.Path, wantPaths[i])
			}
		}
	})

	t.Run("returns empty list when no aliases", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "aliases")
		store := alias.NewStore(filePath)

		list := store.List()

		if len(list) != 0 {
			t.Errorf("got %d aliases, want 0", len(list))
		}
	})
}

func TestFileFormat(t *testing.T) {
	t.Run("writes flat key=value format one per line", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "aliases")
		store := alias.NewStore(filePath)

		store.Set("aa", "/Users/lee/Code/aerobid/api")
		store.Set("m2api", "/Users/lee/Code/mac2/api")

		if err := store.Save(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		data, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("failed to read file: %v", err)
		}

		content := string(data)
		// File should be sorted by name, one per line
		want := "aa=/Users/lee/Code/aerobid/api\nm2api=/Users/lee/Code/mac2/api\n"
		if content != want {
			t.Errorf("file content = %q, want %q", content, want)
		}
	})
}

func TestRoundTrip(t *testing.T) {
	t.Run("load after save preserves aliases", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "aliases")
		store := alias.NewStore(filePath)

		store.Set("proj", "/Users/lee/Code/project")
		store.Set("work", "/Users/lee/Code/work")

		if err := store.Save(); err != nil {
			t.Fatalf("save error: %v", err)
		}

		store2 := alias.NewStore(filePath)
		aliases, err := store2.Load()
		if err != nil {
			t.Fatalf("load error: %v", err)
		}

		if len(aliases) != 2 {
			t.Fatalf("got %d aliases, want 2", len(aliases))
		}

		if aliases["proj"] != "/Users/lee/Code/project" {
			t.Errorf("proj = %q, want %q", aliases["proj"], "/Users/lee/Code/project")
		}
		if aliases["work"] != "/Users/lee/Code/work" {
			t.Errorf("work = %q, want %q", aliases["work"], "/Users/lee/Code/work")
		}
	})
}
