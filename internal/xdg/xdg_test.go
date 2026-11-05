package xdg_test

import (
	"path/filepath"
	"testing"

	"github.com/leeovery/portal/internal/xdg"
)

func TestConfigBaseFrom(t *testing.T) {
	t.Run("returns XDG_CONFIG_HOME when the lookup carries it", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", "/tmp/process-xdg")
		t.Setenv("HOME", t.TempDir())

		got, err := xdg.ConfigBaseFrom(xdg.EnvSlice([]string{"XDG_CONFIG_HOME=/tmp/xdg-config"}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := "/tmp/xdg-config"
		if got != want {
			t.Errorf("ConfigBaseFrom() = %q; want %q", got, want)
		}
	})

	t.Run("falls back to home/.config when the lookup has no XDG_CONFIG_HOME", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		got, err := xdg.ConfigBaseFrom(xdg.EnvSlice(nil))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := filepath.Join(home, ".config")
		if got != want {
			t.Errorf("ConfigBaseFrom() = %q; want %q", got, want)
		}
	})

	t.Run("treats empty XDG_CONFIG_HOME as unset", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		got, err := xdg.ConfigBaseFrom(xdg.EnvSlice([]string{"XDG_CONFIG_HOME="}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := filepath.Join(home, ".config")
		if got != want {
			t.Errorf("ConfigBaseFrom() = %q; want %q", got, want)
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
