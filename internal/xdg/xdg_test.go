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
