package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestHooksListCommand(t *testing.T) {
	t.Run("outputs hooks in tab-separated format", func(t *testing.T) {
		dir := t.TempDir()
		hooksFile := filepath.Join(dir, "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)

		data := map[string]map[string]string{
			"%3": {"on-resume": "claude --resume abc123"},
		}
		writeHooksJSON(t, hooksFile, data)

		buf := new(bytes.Buffer)
		resetRootCmd()
		rootCmd.SetOut(buf)
		rootCmd.SetArgs([]string{"hooks", "list"})
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got := buf.String()
		want := "%3\ton-resume\tclaude --resume abc123\n"
		if got != want {
			t.Errorf("output = %q, want %q", got, want)
		}
	})

	t.Run("produces empty output when no hooks registered", func(t *testing.T) {
		dir := t.TempDir()
		hooksFile := filepath.Join(dir, "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)

		writeHooksJSON(t, hooksFile, map[string]map[string]string{})

		buf := new(bytes.Buffer)
		resetRootCmd()
		rootCmd.SetOut(buf)
		rootCmd.SetArgs([]string{"hooks", "list"})
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got := buf.String()
		if got != "" {
			t.Errorf("output = %q, want empty string", got)
		}
	})

	t.Run("produces empty output when hooks file does not exist", func(t *testing.T) {
		dir := t.TempDir()
		hooksFile := filepath.Join(dir, "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)

		// Do not create the file

		buf := new(bytes.Buffer)
		resetRootCmd()
		rootCmd.SetOut(buf)
		rootCmd.SetArgs([]string{"hooks", "list"})
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got := buf.String()
		if got != "" {
			t.Errorf("output = %q, want empty string", got)
		}
	})

	t.Run("outputs hooks sorted by pane ID", func(t *testing.T) {
		dir := t.TempDir()
		hooksFile := filepath.Join(dir, "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)

		data := map[string]map[string]string{
			"%7": {"on-resume": "claude --resume def456"},
			"%3": {"on-resume": "claude --resume abc123"},
			"%1": {"on-resume": "npm start"},
		}
		writeHooksJSON(t, hooksFile, data)

		buf := new(bytes.Buffer)
		resetRootCmd()
		rootCmd.SetOut(buf)
		rootCmd.SetArgs([]string{"hooks", "list"})
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got := buf.String()
		want := "%1\ton-resume\tnpm start\n%3\ton-resume\tclaude --resume abc123\n%7\ton-resume\tclaude --resume def456\n"
		if got != want {
			t.Errorf("output = %q, want %q", got, want)
		}
	})

	t.Run("hooks bypasses tmux bootstrap", func(t *testing.T) {
		dir := t.TempDir()
		hooksFile := filepath.Join(dir, "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)

		// No bootstrapDeps set — if tmux check runs, it will fail
		// because tmux may not be available in CI.
		// The fact that this succeeds proves skipTmuxCheck works.

		buf := new(bytes.Buffer)
		resetRootCmd()
		rootCmd.SetOut(buf)
		rootCmd.SetArgs([]string{"hooks", "list"})
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error (tmux check should be skipped): %v", err)
		}
	})

	t.Run("accepts no arguments", func(t *testing.T) {
		dir := t.TempDir()
		hooksFile := filepath.Join(dir, "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksFile)

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"hooks", "list", "extraarg"})
		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("expected error for extra argument, got nil")
		}
	})
}

// writeHooksJSON is a test helper that writes a hooks JSON file.
func writeHooksJSON(t *testing.T, path string, data map[string]map[string]string) {
	t.Helper()
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal hooks JSON: %v", err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("failed to write hooks file: %v", err)
	}
}
