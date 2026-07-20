package resolver_test

import (
	"fmt"
	"testing"

	"github.com/leeovery/portal/internal/resolver"
)

// MockCommandRunner implements CommandRunner for testing.
type MockCommandRunner struct {
	Output string
	Err    error
	OnRun  func(name string, args ...string)
}

// Run returns the configured output and error, optionally calling OnRun for argument capture.
func (m *MockCommandRunner) Run(name string, args ...string) (string, error) {
	if m.OnRun != nil {
		m.OnRun(name, args...)
	}
	return m.Output, m.Err
}

func TestResolveGitRoot(t *testing.T) {
	tests := []struct {
		name        string
		dirExists   bool
		mockOutput  string
		mockErr     error
		want        string
		wantOrigDir bool
		wantErr     bool
	}{
		{
			name:       "resolves subdirectory to git repository root",
			dirExists:  true,
			mockOutput: "/home/user/project",
			want:       "/home/user/project",
		},
		{
			name:        "returns original directory for non-git directory",
			dirExists:   true,
			mockOutput:  "",
			mockErr:     fmt.Errorf("fatal: not a git repository"),
			wantOrigDir: true,
		},
		{
			name:        "returns original directory when git is not installed",
			dirExists:   true,
			mockOutput:  "",
			mockErr:     fmt.Errorf("exec: \"git\": executable file not found in $PATH"),
			wantOrigDir: true,
		},
		{
			name:       "trims whitespace from git output",
			dirExists:  true,
			mockOutput: "  /home/user/project  \n",
			want:       "/home/user/project",
		},
		{
			name:      "returns error when directory does not exist",
			dirExists: false,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var dir string
			if tt.dirExists {
				dir = t.TempDir()
			} else {
				dir = "/nonexistent/path/that/does/not/exist"
			}

			mock := &MockCommandRunner{Output: tt.mockOutput, Err: tt.mockErr}

			got, err := resolver.ResolveGitRoot(dir, mock)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			want := tt.want
			if tt.wantOrigDir {
				want = dir
			}

			if got != want {
				t.Errorf("ResolveGitRoot(%q) = %q, want %q", dir, got, want)
			}
		})
	}
}

func TestRealCommandRunner_implements_CommandRunner(t *testing.T) {
	var _ resolver.CommandRunner = &resolver.RealCommandRunner{}
}
