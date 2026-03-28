package cmd

import (
	"os"
	"path/filepath"
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
