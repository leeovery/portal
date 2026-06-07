package prefs_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/prefs"
)

func TestLoad(t *testing.T) {
	t.Run("returns Flat for a missing prefs file", func(t *testing.T) {
		dir := t.TempDir()
		store := prefs.NewStore(filepath.Join(dir, "nonexistent", "prefs.json"))

		mode, err := store.Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if mode != prefs.ModeFlat {
			t.Errorf("mode = %v, want ModeFlat", mode)
		}
	})

	t.Run("returns Flat for an empty prefs file", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "prefs.json")
		if err := os.WriteFile(filePath, []byte(""), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		store := prefs.NewStore(filePath)
		mode, err := store.Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if mode != prefs.ModeFlat {
			t.Errorf("mode = %v, want ModeFlat", mode)
		}
	})

	t.Run("returns Flat for corrupt unparseable JSON", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "prefs.json")
		if err := os.WriteFile(filePath, []byte("{invalid json!!!"), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		store := prefs.NewStore(filePath)
		mode, err := store.Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if mode != prefs.ModeFlat {
			t.Errorf("mode = %v, want ModeFlat", mode)
		}
	})

	t.Run("returns Flat for an unrecognised mode value", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "prefs.json")
		if err := os.WriteFile(filePath, []byte(`{"session_list_mode":"by-galaxy"}`), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		store := prefs.NewStore(filePath)
		mode, err := store.Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if mode != prefs.ModeFlat {
			t.Errorf("mode = %v, want ModeFlat", mode)
		}
	})
}

func TestRoundTrip(t *testing.T) {
	t.Run("round-trips by-tag through Save and Load", func(t *testing.T) {
		dir := t.TempDir()
		store := prefs.NewStore(filepath.Join(dir, "prefs.json"))

		if err := store.Save(prefs.ModeByTag); err != nil {
			t.Fatalf("unexpected save error: %v", err)
		}

		mode, err := store.Load()
		if err != nil {
			t.Fatalf("unexpected load error: %v", err)
		}
		if mode != prefs.ModeByTag {
			t.Errorf("mode = %v, want ModeByTag", mode)
		}
	})

	t.Run("round-trips by-project through Save and Load", func(t *testing.T) {
		dir := t.TempDir()
		store := prefs.NewStore(filepath.Join(dir, "prefs.json"))

		if err := store.Save(prefs.ModeByProject); err != nil {
			t.Fatalf("unexpected save error: %v", err)
		}

		mode, err := store.Load()
		if err != nil {
			t.Fatalf("unexpected load error: %v", err)
		}
		if mode != prefs.ModeByProject {
			t.Errorf("mode = %v, want ModeByProject", mode)
		}
	})
}

func TestSaveWritesAtomically(t *testing.T) {
	t.Run("writes session_list_mode atomically via AtomicWrite", func(t *testing.T) {
		dir := t.TempDir()
		// Nested path that does not yet exist: AtomicWrite creates the parent
		// directory, proving the write went through AtomicWrite (not a bare
		// os.WriteFile that would fail on a missing dir).
		filePath := filepath.Join(dir, "sub", "prefs.json")
		store := prefs.NewStore(filePath)

		if err := store.Save(prefs.ModeByTag); err != nil {
			t.Fatalf("unexpected save error: %v", err)
		}

		data, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("failed to read prefs file: %v", err)
		}

		got := string(data)
		if !strings.Contains(got, `"session_list_mode": "by-tag"`) {
			t.Errorf("file content = %q, want it to contain session_list_mode by-tag", got)
		}

		// No leftover temp files from the temp+rename strategy.
		entries, err := os.ReadDir(filepath.Dir(filePath))
		if err != nil {
			t.Fatalf("failed to read dir: %v", err)
		}
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".atomic-") {
				t.Errorf("leftover temp file: %s", e.Name())
			}
		}
	})
}

func TestModeString(t *testing.T) {
	cases := []struct {
		mode prefs.SessionListMode
		want string
	}{
		{prefs.ModeFlat, "flat"},
		{prefs.ModeByProject, "by-project"},
		{prefs.ModeByTag, "by-tag"},
	}
	for _, c := range cases {
		if got := c.mode.String(); got != c.want {
			t.Errorf("mode %d String() = %q, want %q", c.mode, got, c.want)
		}
	}
}
