//go:build integration

package cmd_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/portalbintest"
	"github.com/leeovery/portal/internal/portaltest"
)

// Delegates to portalbintest, which compiles with -tags integration so every
// test-staged binary carries the daemon-pgrep sandbox.
func buildPortalBinary(t *testing.T) string {
	t.Helper()
	return portalbintest.Build(t)
}

func TestPortalBinaryZmxMissing(t *testing.T) {
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
			wantMsg:  "zmx not found on PATH; Portal needs zmx 1.0.1 or later",
			wantCode: 1,
		},
		{
			name:     "list's bootstrap prints error to stderr and exits 1",
			args:     []string{"list"},
			wantErr:  true,
			wantMsg:  "zmx not found on PATH; Portal needs zmx 1.0.1 or later",
			wantCode: 1,
		},
		{
			name:    "version works without zmx",
			args:    []string{"version"},
			wantErr: false,
		},
		{
			name:    "init works without zmx",
			args:    []string{"init", "zsh"},
			wantErr: false,
		},
		{
			name:    "help works without zmx",
			args:    []string{"help"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, _ := portaltest.IsolateStateForTest(t)
			cmd := exec.Command(binary, tt.args...)
			// A zmx dir of the test's own, never there.
			cmd.Env = append(env, "PATH=/nonexistent/path", "ZMX_DIR="+filepath.Join(t.TempDir(), "zmx"))

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
			env, _ := portaltest.IsolateStateForTest(t)
			cmd := exec.Command(binary, tt.args...)
			cmd.Env = env

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

	env, _ := portaltest.IsolateStateForTest(t)
	cmd := exec.Command(binary, "alias", "rm", "nonexistent")
	cmd.Env = append(env, "PORTAL_ALIASES_FILE="+filepath.Join(t.TempDir(), "aliases"))

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
