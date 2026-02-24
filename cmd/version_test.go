package cmd

import (
	"bytes"
	"testing"
)

func TestVersionCommand(t *testing.T) {
	t.Run("outputs version string in expected format", func(t *testing.T) {
		resetRootCmd()
		buf := new(bytes.Buffer)
		rootCmd.SetOut(buf)
		rootCmd.SetArgs([]string{"version"})

		err := rootCmd.Execute()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := "portal version dev\n"
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
