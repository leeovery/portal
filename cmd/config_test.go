package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigFilePath(t *testing.T) {
	t.Run("returns ~/.config/portal/<file> when no env vars are set", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", "")

		homeDir, err := os.UserHomeDir()
		if err != nil {
			t.Fatalf("failed to get home dir: %v", err)
		}

		got, err := configFilePath("TEST_CONFIG_UNSET", "projects.json")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := filepath.Join(homeDir, ".config", "portal", "projects.json")
		if got != want {
			t.Errorf("configFilePath() = %q, want %q", got, want)
		}
	})

	t.Run("returns env var value when per-file env var is set", func(t *testing.T) {
		t.Setenv("TEST_CONFIG_PATH", "/custom/path/file.json")

		got, err := configFilePath("TEST_CONFIG_PATH", "file.json")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := "/custom/path/file.json"
		if got != want {
			t.Errorf("configFilePath() = %q, want %q", got, want)
		}
	})

	t.Run("respects XDG_CONFIG_HOME when set", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg-config")

		got, err := configFilePath("TEST_CONFIG_UNSET", "projects.json")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := filepath.Join("/tmp/xdg-config", "portal", "projects.json")
		if got != want {
			t.Errorf("configFilePath() = %q, want %q", got, want)
		}
	})

	t.Run("treats empty XDG_CONFIG_HOME as unset", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", "")

		homeDir, err := os.UserHomeDir()
		if err != nil {
			t.Fatalf("failed to get home dir: %v", err)
		}

		got, err := configFilePath("TEST_CONFIG_UNSET", "projects.json")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := filepath.Join(homeDir, ".config", "portal", "projects.json")
		if got != want {
			t.Errorf("configFilePath() = %q, want %q", got, want)
		}
	})

	t.Run("per-file env var takes precedence over XDG_CONFIG_HOME", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg-config")
		t.Setenv("TEST_OVERRIDE", "/explicit/override/file.json")

		got, err := configFilePath("TEST_OVERRIDE", "file.json")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := "/explicit/override/file.json"
		if got != want {
			t.Errorf("configFilePath() = %q, want %q", got, want)
		}
	})

	t.Run("XDG_CONFIG_HOME with trailing slash is normalized", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg-config/")

		got, err := configFilePath("TEST_CONFIG_UNSET", "hooks.json")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := filepath.Join("/tmp/xdg-config", "portal", "hooks.json")
		if got != want {
			t.Errorf("configFilePath() = %q, want %q", got, want)
		}
	})
}

func TestMigrateConfigFile(t *testing.T) {
	t.Run("migration is no-op when old directory does not exist", func(t *testing.T) {
		tmpDir := t.TempDir()

		oldPath := filepath.Join(tmpDir, "nonexistent", "portal", "projects.json")
		newPath := filepath.Join(tmpDir, ".config", "portal", "projects.json")

		migrateConfigFile(oldPath, newPath)

		if _, err := os.Stat(newPath); !os.IsNotExist(err) {
			t.Errorf("new file should not exist when old file does not exist")
		}
	})

	t.Run("migration does not overwrite existing file at new path", func(t *testing.T) {
		tmpDir := t.TempDir()

		oldDir := filepath.Join(tmpDir, "Library", "Application Support", "portal")
		if err := os.MkdirAll(oldDir, 0o755); err != nil {
			t.Fatalf("failed to create old dir: %v", err)
		}
		oldPath := filepath.Join(oldDir, "projects.json")
		if err := os.WriteFile(oldPath, []byte("old content"), 0o644); err != nil {
			t.Fatalf("failed to write old file: %v", err)
		}

		newDir := filepath.Join(tmpDir, ".config", "portal")
		if err := os.MkdirAll(newDir, 0o755); err != nil {
			t.Fatalf("failed to create new dir: %v", err)
		}
		newPath := filepath.Join(newDir, "projects.json")
		if err := os.WriteFile(newPath, []byte("new content"), 0o644); err != nil {
			t.Fatalf("failed to write new file: %v", err)
		}

		migrateConfigFile(oldPath, newPath)

		data, err := os.ReadFile(newPath)
		if err != nil {
			t.Fatalf("failed to read new file: %v", err)
		}
		if string(data) != "new content" {
			t.Errorf("new file content = %q, want %q (should not be overwritten)", string(data), "new content")
		}

		if _, err := os.Stat(oldPath); err != nil {
			t.Errorf("old file should still exist when new path is occupied: %v", err)
		}
	})

	t.Run("migration handles partial state", func(t *testing.T) {
		tmpDir := t.TempDir()

		oldDir := filepath.Join(tmpDir, "Library", "Application Support", "portal")
		if err := os.MkdirAll(oldDir, 0o755); err != nil {
			t.Fatalf("failed to create old dir: %v", err)
		}
		// Only aliases exists at old path
		oldAliases := filepath.Join(oldDir, "aliases")
		if err := os.WriteFile(oldAliases, []byte("a=/path/a"), 0o644); err != nil {
			t.Fatalf("failed to write old aliases: %v", err)
		}

		newDir := filepath.Join(tmpDir, ".config", "portal")
		if err := os.MkdirAll(newDir, 0o755); err != nil {
			t.Fatalf("failed to create new dir: %v", err)
		}
		// projects.json already at new path
		newProjects := filepath.Join(newDir, "projects.json")
		if err := os.WriteFile(newProjects, []byte("existing"), 0o644); err != nil {
			t.Fatalf("failed to write new projects: %v", err)
		}

		newAliases := filepath.Join(newDir, "aliases")

		// Migrate aliases — should succeed
		migrateConfigFile(oldAliases, newAliases)

		data, err := os.ReadFile(newAliases)
		if err != nil {
			t.Fatalf("failed to read migrated aliases: %v", err)
		}
		if string(data) != "a=/path/a" {
			t.Errorf("aliases content = %q, want %q", string(data), "a=/path/a")
		}

		// Migrate projects — should be no-op (new path occupied)
		oldProjects := filepath.Join(oldDir, "projects.json")
		migrateConfigFile(oldProjects, newProjects)

		projData, err := os.ReadFile(newProjects)
		if err != nil {
			t.Fatalf("failed to read projects: %v", err)
		}
		if string(projData) != "existing" {
			t.Errorf("projects content = %q, want %q", string(projData), "existing")
		}
	})

	t.Run("migration cleans up empty old directory", func(t *testing.T) {
		tmpDir := t.TempDir()

		oldDir := filepath.Join(tmpDir, "Library", "Application Support", "portal")
		if err := os.MkdirAll(oldDir, 0o755); err != nil {
			t.Fatalf("failed to create old dir: %v", err)
		}
		oldPath := filepath.Join(oldDir, "projects.json")
		if err := os.WriteFile(oldPath, []byte("data"), 0o644); err != nil {
			t.Fatalf("failed to write old file: %v", err)
		}

		newDir := filepath.Join(tmpDir, ".config", "portal")
		if err := os.MkdirAll(newDir, 0o755); err != nil {
			t.Fatalf("failed to create new dir: %v", err)
		}
		newPath := filepath.Join(newDir, "projects.json")

		migrateConfigFile(oldPath, newPath)

		if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
			t.Errorf("old directory should be removed when empty after migration")
		}
	})

	t.Run("migration preserves non-empty old directory", func(t *testing.T) {
		tmpDir := t.TempDir()

		oldDir := filepath.Join(tmpDir, "Library", "Application Support", "portal")
		if err := os.MkdirAll(oldDir, 0o755); err != nil {
			t.Fatalf("failed to create old dir: %v", err)
		}
		oldPath := filepath.Join(oldDir, "projects.json")
		if err := os.WriteFile(oldPath, []byte("data"), 0o644); err != nil {
			t.Fatalf("failed to write old file: %v", err)
		}
		// Leave another file in old directory
		otherFile := filepath.Join(oldDir, "hooks.json")
		if err := os.WriteFile(otherFile, []byte("hooks"), 0o644); err != nil {
			t.Fatalf("failed to write other file: %v", err)
		}

		newDir := filepath.Join(tmpDir, ".config", "portal")
		if err := os.MkdirAll(newDir, 0o755); err != nil {
			t.Fatalf("failed to create new dir: %v", err)
		}
		newPath := filepath.Join(newDir, "projects.json")

		migrateConfigFile(oldPath, newPath)

		if _, err := os.Stat(oldDir); err != nil {
			t.Errorf("old directory should be preserved when non-empty: %v", err)
		}

		if _, err := os.Stat(otherFile); err != nil {
			t.Errorf("other file in old directory should still exist: %v", err)
		}
	})

	t.Run("migration creates target directory if missing", func(t *testing.T) {
		tmpDir := t.TempDir()

		oldDir := filepath.Join(tmpDir, "Library", "Application Support", "portal")
		if err := os.MkdirAll(oldDir, 0o755); err != nil {
			t.Fatalf("failed to create old dir: %v", err)
		}
		oldPath := filepath.Join(oldDir, "aliases")
		if err := os.WriteFile(oldPath, []byte("x=/y"), 0o644); err != nil {
			t.Fatalf("failed to write old file: %v", err)
		}

		// Do NOT create newDir — migration should create it
		newPath := filepath.Join(tmpDir, ".config", "portal", "aliases")

		migrateConfigFile(oldPath, newPath)

		data, err := os.ReadFile(newPath)
		if err != nil {
			t.Fatalf("failed to read migrated file: %v", err)
		}
		if string(data) != "x=/y" {
			t.Errorf("migrated file content = %q, want %q", string(data), "x=/y")
		}
	})

	t.Run("migration logs warning on rename failure", func(t *testing.T) {
		tmpDir := t.TempDir()

		oldDir := filepath.Join(tmpDir, "Library", "Application Support", "portal")
		if err := os.MkdirAll(oldDir, 0o755); err != nil {
			t.Fatalf("failed to create old dir: %v", err)
		}
		oldPath := filepath.Join(oldDir, "projects.json")
		if err := os.WriteFile(oldPath, []byte("data"), 0o644); err != nil {
			t.Fatalf("failed to write old file: %v", err)
		}

		// Create target directory as read+execute only (no write) to cause rename to fail.
		// 0o555 allows stat to succeed (file not found) but blocks rename.
		newDir := filepath.Join(tmpDir, ".config", "portal")
		if err := os.MkdirAll(newDir, 0o755); err != nil {
			t.Fatalf("failed to create new dir: %v", err)
		}
		if err := os.Chmod(newDir, 0o555); err != nil {
			t.Fatalf("failed to chmod new dir: %v", err)
		}
		t.Cleanup(func() {
			_ = os.Chmod(newDir, 0o755)
		})

		newPath := filepath.Join(newDir, "projects.json")

		// Capture stderr
		oldStderr := os.Stderr
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatalf("failed to create pipe: %v", err)
		}
		os.Stderr = w

		migrateConfigFile(oldPath, newPath)

		_ = w.Close()
		os.Stderr = oldStderr

		var buf [4096]byte
		n, _ := r.Read(buf[:])
		output := string(buf[:n])

		if len(output) == 0 {
			t.Errorf("expected warning on stderr, got nothing")
		}
		if !strings.Contains(output, "warning") || !strings.Contains(output, "migrate") {
			t.Errorf("stderr output %q should contain 'warning' and 'migrate'", output)
		}

		// Old file should still exist
		if _, err := os.Stat(oldPath); err != nil {
			t.Errorf("old file should still exist after failed rename: %v", err)
		}
	})

	t.Run("migration is skipped when stat of new path returns non-not-found error", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create old file that would normally be migrated.
		oldDir := filepath.Join(tmpDir, "Library", "Application Support", "portal")
		if err := os.MkdirAll(oldDir, 0o755); err != nil {
			t.Fatalf("failed to create old dir: %v", err)
		}
		oldPath := filepath.Join(oldDir, "projects.json")
		if err := os.WriteFile(oldPath, []byte("old data"), 0o644); err != nil {
			t.Fatalf("failed to write old file: %v", err)
		}

		// Create the parent of newPath but make it unreadable so that
		// os.Stat(newPath) returns a permission error (not "not exist").
		newDir := filepath.Join(tmpDir, ".config", "portal")
		if err := os.MkdirAll(newDir, 0o755); err != nil {
			t.Fatalf("failed to create new dir: %v", err)
		}
		if err := os.Chmod(newDir, 0o000); err != nil {
			t.Fatalf("failed to chmod new dir: %v", err)
		}
		t.Cleanup(func() {
			_ = os.Chmod(newDir, 0o755)
		})

		newPath := filepath.Join(newDir, "projects.json")

		migrateConfigFile(oldPath, newPath)

		// Old file must still exist — migration should have been skipped.
		if _, err := os.Stat(oldPath); err != nil {
			t.Errorf("old file should still exist when stat of new path fails with non-not-found error: %v", err)
		}
	})

	t.Run("migrates file from old macOS path to new path", func(t *testing.T) {
		tmpDir := t.TempDir()

		oldDir := filepath.Join(tmpDir, "Library", "Application Support", "portal")
		if err := os.MkdirAll(oldDir, 0o755); err != nil {
			t.Fatalf("failed to create old dir: %v", err)
		}
		oldPath := filepath.Join(oldDir, "projects.json")
		if err := os.WriteFile(oldPath, []byte(`{"projects":[]}`), 0o644); err != nil {
			t.Fatalf("failed to write old file: %v", err)
		}

		newDir := filepath.Join(tmpDir, ".config", "portal")
		if err := os.MkdirAll(newDir, 0o755); err != nil {
			t.Fatalf("failed to create new dir: %v", err)
		}
		newPath := filepath.Join(newDir, "projects.json")

		migrateConfigFile(oldPath, newPath)

		if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
			t.Errorf("old file should not exist after migration")
		}

		data, err := os.ReadFile(newPath)
		if err != nil {
			t.Fatalf("failed to read new file: %v", err)
		}
		if string(data) != `{"projects":[]}` {
			t.Errorf("migrated file content = %q, want %q", string(data), `{"projects":[]}`)
		}
	})
}

func TestConfigFilePathMigration(t *testing.T) {
	t.Run("migration does not run when per-file env var is set", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)
		t.Setenv("XDG_CONFIG_HOME", "")

		// Create old macOS path with file
		oldDir := filepath.Join(tmpDir, "Library", "Application Support", "portal")
		if err := os.MkdirAll(oldDir, 0o755); err != nil {
			t.Fatalf("failed to create old dir: %v", err)
		}
		oldPath := filepath.Join(oldDir, "projects.json")
		if err := os.WriteFile(oldPath, []byte("old data"), 0o644); err != nil {
			t.Fatalf("failed to write old file: %v", err)
		}

		// Set per-file env var override
		overridePath := filepath.Join(tmpDir, "custom", "projects.json")
		t.Setenv("TEST_MIGRATE_ENVVAR", overridePath)

		got, err := configFilePath("TEST_MIGRATE_ENVVAR", "projects.json")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got != overridePath {
			t.Errorf("configFilePath() = %q, want %q", got, overridePath)
		}

		// Old file should still exist — migration should not have run
		if _, err := os.Stat(oldPath); err != nil {
			t.Errorf("old file should still exist when env var override is active: %v", err)
		}
	})

	t.Run("migration runs when XDG_CONFIG_HOME is set", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)

		xdgDir := filepath.Join(tmpDir, "custom-xdg")
		t.Setenv("XDG_CONFIG_HOME", xdgDir)

		// Create old macOS path with file
		oldDir := filepath.Join(tmpDir, "Library", "Application Support", "portal")
		if err := os.MkdirAll(oldDir, 0o755); err != nil {
			t.Fatalf("failed to create old dir: %v", err)
		}
		oldPath := filepath.Join(oldDir, "aliases")
		if err := os.WriteFile(oldPath, []byte("a=/x"), 0o644); err != nil {
			t.Fatalf("failed to write old file: %v", err)
		}

		got, err := configFilePath("TEST_MIGRATE_XDG_UNSET", "aliases")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := filepath.Join(xdgDir, "portal", "aliases")
		if got != want {
			t.Errorf("configFilePath() = %q, want %q", got, want)
		}

		// Old file should have been migrated
		if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
			t.Errorf("old file should not exist after migration")
		}

		// New file should exist with correct content
		data, err := os.ReadFile(want)
		if err != nil {
			t.Fatalf("failed to read migrated file: %v", err)
		}
		if string(data) != "a=/x" {
			t.Errorf("migrated file content = %q, want %q", string(data), "a=/x")
		}
	})
}
