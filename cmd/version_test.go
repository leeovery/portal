package cmd

import (
	"bytes"
	"testing"

	"github.com/leeovery/portal/internal/devversion"
)

func TestVersionCommand(t *testing.T) {
	t.Run("a dev build prints its own dev version", func(t *testing.T) {
		resetRootCmd()
		buf := new(bytes.Buffer)
		rootCmd.SetOut(buf)
		rootCmd.SetArgs([]string{"version"})

		err := rootCmd.Execute()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := "portal version " + devversion.String() + "\n"
		if buf.String() != want {
			t.Errorf("output = %q, want %q", buf.String(), want)
		}
	})

	t.Run("default version is dev", func(t *testing.T) {
		if version != "dev" {
			t.Errorf("default version = %q, want %q", version, "dev")
		}
	})

	t.Run("custom version reflected in output", func(t *testing.T) {
		original := version
		version = "1.2.3"
		t.Cleanup(func() { version = original })

		resetRootCmd()
		buf := new(bytes.Buffer)
		rootCmd.SetOut(buf)
		rootCmd.SetArgs([]string{"version"})

		err := rootCmd.Execute()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := "portal version 1.2.3\n"
		if buf.String() != want {
			t.Errorf("output = %q, want %q", buf.String(), want)
		}
	})
}

func TestVersion(t *testing.T) {
	t.Run("a dev build reports devversion's version, which no release can be", func(t *testing.T) {
		if got := Version(); got != devversion.String() || got == "dev" {
			t.Errorf("Version() = %q, want devversion's %q", got, devversion.String())
		}
	})

	t.Run("a release build reports the version injected, as given", func(t *testing.T) {
		original := version
		version = "0.13.0"
		t.Cleanup(func() { version = original })

		if got := Version(); got != "0.13.0" {
			t.Errorf("Version() = %q, want 0.13.0", got)
		}
	})
}
