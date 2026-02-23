package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// resetRootCmd resets the root command's output streams and subcommand flags for testing.
func resetRootCmd() {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	_ = initCmd.Flags().Set("cmd", "x")     // reset to default; value is always valid
	_ = listCmd.Flags().Set("short", "false") // reset list flags
	_ = listCmd.Flags().Set("long", "false")
	if f := openCmd.Flags().Lookup("exec"); f != nil { // reset exec flag
		_ = f.Value.Set("")
		f.Changed = false
	}
}

func TestTmuxDependentCommandsFailWithoutTmux(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "portal open fails without tmux", args: []string{"open"}},
		{name: "portal list fails without tmux", args: []string{"list"}},
		{name: "portal attach fails without tmux", args: []string{"attach", "test-session"}},
		{name: "portal kill fails without tmux", args: []string{"kill", "test-session"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PATH", "/nonexistent/path")

			resetRootCmd()
			rootCmd.SetArgs(tt.args)
			err := rootCmd.Execute()

			if err == nil {
				t.Fatal("expected error, got nil")
			}

			errMsg := err.Error()
			want := "Portal requires tmux. Install with: brew install tmux"
			if errMsg != want {
				t.Errorf("error = %q, want %q", errMsg, want)
			}
		})
	}
}

func TestNonTmuxCommandsWorkWithoutTmux(t *testing.T) {
	tests := []struct {
		name string
		args []string
		env  map[string]string
	}{
		{name: "portal version works without tmux", args: []string{"version"}},
		{name: "portal init works without tmux", args: []string{"init", "zsh"}},
		{name: "portal help works without tmux", args: []string{"help"}},
		{
			name: "portal alias set works without tmux",
			args: []string{"alias", "set", "proj", "/some/path"},
			env:  map[string]string{"PORTAL_ALIASES_FILE": "TEMPDIR/aliases"},
		},
		{
			name: "portal clean works without tmux",
			args: []string{"clean"},
			env:  map[string]string{"PORTAL_PROJECTS_FILE": "TEMPDIR/projects.json"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PATH", "/nonexistent/path")

			for k, v := range tt.env {
				if strings.HasPrefix(v, "TEMPDIR/") {
					v = filepath.Join(t.TempDir(), strings.TrimPrefix(v, "TEMPDIR/"))
				}
				t.Setenv(k, v)
			}

			resetRootCmd()
			rootCmd.SetArgs(tt.args)
			err := rootCmd.Execute()

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestRootCommandExecutesWithoutError(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{})
	err := rootCmd.Execute()

	if err != nil {
		t.Fatalf("root command returned error: %v", err)
	}
}

func TestOpenSubcommandIsRegistered(t *testing.T) {
	found := false
	for _, c := range rootCmd.Commands() {
		if c.Name() == "open" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("open subcommand is not registered on root command")
	}
}

func TestTmuxDependentCommandsSucceedWithTmux(t *testing.T) {
	// Ensure tmux is actually available for this test
	originalPath := os.Getenv("PATH")
	if originalPath == "" {
		t.Skip("PATH not set")
	}

	tests := []struct {
		name string
		args []string
	}{
		// open is excluded: it launches a full-screen TUI requiring a TTY
		{name: "portal list succeeds with tmux", args: []string{"list"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetRootCmd()
			rootCmd.SetArgs(tt.args)
			err := rootCmd.Execute()

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
