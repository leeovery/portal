package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrefsFilePath(t *testing.T) {
	t.Run("returns the PORTAL_PREFS_FILE override when set", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg-config")
		t.Setenv("PORTAL_PREFS_FILE", "/custom/prefs.json")

		got, err := prefsFilePath()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got != "/custom/prefs.json" {
			t.Errorf("prefsFilePath() = %q, want %q", got, "/custom/prefs.json")
		}
	})

	t.Run("returns the XDG path for prefs.json when XDG_CONFIG_HOME is set", func(t *testing.T) {
		t.Setenv("PORTAL_PREFS_FILE", "")
		t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg-config")

		got, err := prefsFilePath()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := filepath.Join("/tmp/xdg-config", "portal", "prefs.json")
		if got != want {
			t.Errorf("prefsFilePath() = %q, want %q", got, want)
		}
	})

	t.Run("falls back to ~/.config/portal/prefs.json", func(t *testing.T) {
		t.Setenv("PORTAL_PREFS_FILE", "")
		t.Setenv("XDG_CONFIG_HOME", "")

		homeDir, err := os.UserHomeDir()
		if err != nil {
			t.Fatalf("failed to get home dir: %v", err)
		}

		got, err := prefsFilePath()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := filepath.Join(homeDir, ".config", "portal", "prefs.json")
		if got != want {
			t.Errorf("prefsFilePath() = %q, want %q", got, want)
		}
	})
}

func TestPrefsFilePathMigration(t *testing.T) {
	t.Run("migrates prefs.json from the old macOS path when the new path is absent", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)
		t.Setenv("PORTAL_PREFS_FILE", "")

		xdgDir := filepath.Join(tmpDir, "custom-xdg")
		t.Setenv("XDG_CONFIG_HOME", xdgDir)

		oldDir := filepath.Join(tmpDir, "Library", "Application Support", "portal")
		if err := os.MkdirAll(oldDir, 0o755); err != nil {
			t.Fatalf("failed to create old dir: %v", err)
		}
		oldPath := filepath.Join(oldDir, "prefs.json")
		if err := os.WriteFile(oldPath, []byte(`{"session_list_mode":"by-tag"}`), 0o644); err != nil {
			t.Fatalf("failed to write old file: %v", err)
		}

		got, err := prefsFilePath()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := filepath.Join(xdgDir, "portal", "prefs.json")
		if got != want {
			t.Errorf("prefsFilePath() = %q, want %q", got, want)
		}

		if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
			t.Errorf("old prefs.json should not exist after migration")
		}

		data, err := os.ReadFile(want)
		if err != nil {
			t.Fatalf("failed to read migrated prefs.json: %v", err)
		}
		if string(data) != `{"session_list_mode":"by-tag"}` {
			t.Errorf("migrated content = %q, want %q", string(data), `{"session_list_mode":"by-tag"}`)
		}
	})

	t.Run("does not overwrite an existing prefs.json at the new path", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)
		t.Setenv("PORTAL_PREFS_FILE", "")

		xdgDir := filepath.Join(tmpDir, "custom-xdg")
		t.Setenv("XDG_CONFIG_HOME", xdgDir)

		oldDir := filepath.Join(tmpDir, "Library", "Application Support", "portal")
		if err := os.MkdirAll(oldDir, 0o755); err != nil {
			t.Fatalf("failed to create old dir: %v", err)
		}
		oldPath := filepath.Join(oldDir, "prefs.json")
		if err := os.WriteFile(oldPath, []byte("old prefs"), 0o644); err != nil {
			t.Fatalf("failed to write old file: %v", err)
		}

		newDir := filepath.Join(xdgDir, "portal")
		if err := os.MkdirAll(newDir, 0o755); err != nil {
			t.Fatalf("failed to create new dir: %v", err)
		}
		newPath := filepath.Join(newDir, "prefs.json")
		if err := os.WriteFile(newPath, []byte("new prefs"), 0o644); err != nil {
			t.Fatalf("failed to write new file: %v", err)
		}

		got, err := prefsFilePath()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != newPath {
			t.Errorf("prefsFilePath() = %q, want %q", got, newPath)
		}

		data, err := os.ReadFile(newPath)
		if err != nil {
			t.Fatalf("failed to read new file: %v", err)
		}
		if string(data) != "new prefs" {
			t.Errorf("new file content = %q, want %q (should not be overwritten)", string(data), "new prefs")
		}

		if _, err := os.Stat(oldPath); err != nil {
			t.Errorf("old file should still exist when new path is occupied: %v", err)
		}
	})

	t.Run("does not migrate when the env override is set", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)
		t.Setenv("XDG_CONFIG_HOME", "")

		oldDir := filepath.Join(tmpDir, "Library", "Application Support", "portal")
		if err := os.MkdirAll(oldDir, 0o755); err != nil {
			t.Fatalf("failed to create old dir: %v", err)
		}
		oldPath := filepath.Join(oldDir, "prefs.json")
		if err := os.WriteFile(oldPath, []byte("old prefs"), 0o644); err != nil {
			t.Fatalf("failed to write old file: %v", err)
		}

		overridePath := filepath.Join(tmpDir, "custom", "prefs.json")
		t.Setenv("PORTAL_PREFS_FILE", overridePath)

		got, err := prefsFilePath()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != overridePath {
			t.Errorf("prefsFilePath() = %q, want %q", got, overridePath)
		}

		if _, err := os.Stat(oldPath); err != nil {
			t.Errorf("old file should still exist when env override is active: %v", err)
		}
	})
}

func TestPrefsMigrateSuppressesLog(t *testing.T) {
	t.Run("prefs.json is mapped to an empty component so migrate does not emit", func(t *testing.T) {
		if got, ok := configFileComponents["prefs.json"]; !ok || got != "" {
			t.Errorf("configFileComponents[\"prefs.json\"] = %q, ok=%v; want \"\" with explicit entry", got, ok)
		}

		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)
		xdgDir := filepath.Join(tmpDir, "custom-xdg")
		t.Setenv("XDG_CONFIG_HOME", xdgDir)
		t.Setenv("PORTAL_PREFS_FILE", "")

		oldDir := filepath.Join(tmpDir, "Library", "Application Support", "portal")
		if err := os.MkdirAll(oldDir, 0o755); err != nil {
			t.Fatalf("failed to create old dir: %v", err)
		}
		oldPath := filepath.Join(oldDir, "prefs.json")
		if err := os.WriteFile(oldPath, []byte("{}"), 0o644); err != nil {
			t.Fatalf("failed to write old file: %v", err)
		}

		sink := installMigrateCapture(t)

		got, err := prefsFilePath()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if recs := sink.Records(); len(recs) != 0 {
			t.Errorf("expected no log records for prefs.json migrate, got %d: %+v", len(recs), recs)
		}

		// The move itself must still have happened.
		if _, err := os.Stat(got); err != nil {
			t.Errorf("prefs.json should still migrate when component is empty: %v", err)
		}
	})
}

func TestLoadPrefsStore(t *testing.T) {
	t.Run("returns a store bound to the resolved path", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "prefs.json")
		t.Setenv("PORTAL_PREFS_FILE", path)

		store, err := loadPrefsStore()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if store == nil {
			t.Fatal("loadPrefsStore() returned nil store")
		}

		// Verify the store is bound to the resolved path via a round-trip Save.
		if err := store.Save(2); err != nil {
			t.Fatalf("Save failed: %v", err)
		}
		if _, err := os.Stat(path); err != nil {
			t.Errorf("store should write to the resolved path %q: %v", path, err)
		}
	})
}
