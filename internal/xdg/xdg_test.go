package xdg_test

import (
	"path/filepath"
	"testing"

	"github.com/leeovery/portal/internal/xdg"
)

func TestConfigBase(t *testing.T) {
	t.Run("returns XDG_CONFIG_HOME when set", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg-config")
		t.Setenv("HOME", t.TempDir())

		got, err := xdg.ConfigBase()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := "/tmp/xdg-config"
		if got != want {
			t.Errorf("ConfigBase() = %q; want %q", got, want)
		}
	})

	t.Run("falls back to home/.config when XDG_CONFIG_HOME is unset", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", "")
		t.Setenv("HOME", home)

		got, err := xdg.ConfigBase()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := filepath.Join(home, ".config")
		if got != want {
			t.Errorf("ConfigBase() = %q; want %q", got, want)
		}
	})

	t.Run("treats empty XDG_CONFIG_HOME as unset", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", "")
		t.Setenv("HOME", home)

		got, err := xdg.ConfigBase()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := filepath.Join(home, ".config")
		if got != want {
			t.Errorf("ConfigBase() = %q; want %q", got, want)
		}
	})
}

func TestPortalDir(t *testing.T) {
	t.Run("it is portal under the config base, whatever overrides each file and directory", func(t *testing.T) {
		lookup := xdg.EnvSlice([]string{
			"XDG_CONFIG_HOME=/tmp/xdg-config",
			"PORTAL_STATE_DIR=/elsewhere/state",
			"PORTAL_HOOKS_FILE=/elsewhere/hooks.json",
		})

		got, err := xdg.PortalDir(lookup)
		if err != nil || got != "/tmp/xdg-config/portal" {
			t.Errorf("PortalDir = %q, %v; want /tmp/xdg-config/portal", got, err)
		}
	})

	t.Run("it falls back to the home directory's .config", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		got, err := xdg.PortalDir(xdg.EnvSlice(nil))
		if want := filepath.Join(home, ".config", "portal"); err != nil || got != want {
			t.Errorf("PortalDir = %q, %v; want %q", got, err, want)
		}
	})
}
