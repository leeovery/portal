//go:build integration

package cmd_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/portalbintest"
)

// buildPortalBinary delegates to the canonical portalbintest builder (which
// compiles with -tags integration — see buildPortalBinaryInto) so every
// test-staged binary carries the daemon-pgrep sandbox. The former inlined
// `go build` here was the one build site the sandbox tag did not cover.
func buildPortalBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := portalbintest.BuildPortalBinary(dir); err != nil {
		t.Fatalf("failed to build portal binary: %v", err)
	}
	return filepath.Join(dir, "portal")
}

func TestPortalBinaryTmuxMissing(t *testing.T) {
	binary := buildPortalBinary(t)

	tests := []struct {
		name     string
		args     []string
		wantErr  bool
		wantMsg  string
		wantCode int
	}{
		{
			name:     "open prints error to stderr and exits 1",
			args:     []string{"open"},
			wantErr:  true,
			wantMsg:  "Portal requires tmux. Install with: brew install tmux",
			wantCode: 1,
		},
		{
			name:     "list prints error to stderr and exits 1",
			args:     []string{"list"},
			wantErr:  true,
			wantMsg:  "Portal requires tmux. Install with: brew install tmux",
			wantCode: 1,
		},
		{
			name:     "attach prints error to stderr and exits 1",
			args:     []string{"attach", "test"},
			wantErr:  true,
			wantMsg:  "Portal requires tmux. Install with: brew install tmux",
			wantCode: 1,
		},
		{
			name:     "kill prints error to stderr and exits 1",
			args:     []string{"kill", "test"},
			wantErr:  true,
			wantMsg:  "Portal requires tmux. Install with: brew install tmux",
			wantCode: 1,
		},
		{
			name:    "version works without tmux",
			args:    []string{"version"},
			wantErr: false,
		},
		{
			name:    "init works without tmux",
			args:    []string{"init", "zsh"},
			wantErr: false,
		},
		{
			name:    "help works without tmux",
			args:    []string{"help"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(binary, tt.args...)
			cmd.Env = []string{"PATH=/nonexistent/path", "HOME=" + t.TempDir()}

			output, err := cmd.CombinedOutput()

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected command to fail, but it succeeded")
				}
				exitErr, ok := err.(*exec.ExitError)
				if !ok {
					t.Fatalf("expected ExitError, got %T: %v", err, err)
				}
				if exitErr.ExitCode() != tt.wantCode {
					t.Errorf("exit code = %d, want %d", exitErr.ExitCode(), tt.wantCode)
				}
				outputStr := strings.TrimSpace(string(output))
				if outputStr != tt.wantMsg {
					t.Errorf("output = %q, want %q", outputStr, tt.wantMsg)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v\noutput: %s", err, output)
				}
			}
		})
	}
}

func TestPortalBinaryUnsupportedShellExitCode2(t *testing.T) {
	binary := buildPortalBinary(t)

	tests := []struct {
		name     string
		args     []string
		wantMsg  string
		wantCode int
	}{
		{
			name:     "powershell exits with code 2",
			args:     []string{"init", "powershell"},
			wantMsg:  "unsupported shell: powershell (supported: bash, zsh, fish)",
			wantCode: 2,
		},
		{
			name:     "nushell exits with code 2",
			args:     []string{"init", "nushell"},
			wantMsg:  "unsupported shell: nushell (supported: bash, zsh, fish)",
			wantCode: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(binary, tt.args...)
			cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir()}

			output, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatal("expected command to fail, but it succeeded")
			}

			exitErr, ok := err.(*exec.ExitError)
			if !ok {
				t.Fatalf("expected ExitError, got %T: %v", err, err)
			}
			if exitErr.ExitCode() != tt.wantCode {
				t.Errorf("exit code = %d, want %d", exitErr.ExitCode(), tt.wantCode)
			}

			outputStr := strings.TrimSpace(string(output))
			if outputStr != tt.wantMsg {
				t.Errorf("output = %q, want %q", outputStr, tt.wantMsg)
			}
		})
	}
}

func TestPortalBinaryAliasRmNotFound(t *testing.T) {
	binary := buildPortalBinary(t)

	tmpDir := t.TempDir()
	aliasFile := filepath.Join(tmpDir, "aliases")

	cmd := exec.Command(binary, "alias", "rm", "nonexistent")
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + tmpDir,
		"PORTAL_ALIASES_FILE=" + aliasFile,
	}

	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected command to fail, but it succeeded")
	}

	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected ExitError, got %T: %v", err, err)
	}
	if exitErr.ExitCode() != 1 {
		t.Errorf("exit code = %d, want 1", exitErr.ExitCode())
	}

	outputStr := strings.TrimSpace(string(output))
	want := "alias not found: nonexistent"
	if outputStr != want {
		t.Errorf("output = %q, want %q", outputStr, want)
	}
}
