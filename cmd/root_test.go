package cmd

import (
	"bytes"
	"os"
	"testing"
)

// resetRootCmd resets the root command's output streams for testing.
func resetRootCmd() {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
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
	}{
		{name: "portal version works without tmux", args: []string{"version"}},
		{name: "portal init works without tmux", args: []string{"init", "zsh"}},
		{name: "portal help works without tmux", args: []string{"help"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PATH", "/nonexistent/path")

			resetRootCmd()
			rootCmd.SetArgs(tt.args)
			err := rootCmd.Execute()

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
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
		{name: "portal open succeeds with tmux", args: []string{"open"}},
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
