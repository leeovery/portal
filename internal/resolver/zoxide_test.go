package resolver_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/resolver"
)

func TestZoxideResolver_Query(t *testing.T) {
	tests := []struct {
		name        string
		terms       string
		lookPathErr error
		runOutput   string
		runErr      error
		want        string
		wantErr     error
	}{
		{
			name:        "returns best match from zoxide query",
			terms:       "myproject",
			lookPathErr: nil,
			runOutput:   "/home/user/Code/myproject",
			runErr:      nil,
			want:        "/home/user/Code/myproject",
			wantErr:     nil,
		},
		{
			name:        "returns ErrZoxideNotInstalled when zoxide not installed",
			terms:       "myproject",
			lookPathErr: fmt.Errorf("exec: \"zoxide\": executable file not found in $PATH"),
			want:        "",
			wantErr:     resolver.ErrZoxideNotInstalled,
		},
		{
			name:        "returns ErrNoMatch when zoxide returns no match",
			terms:       "nonexistent",
			lookPathErr: nil,
			runOutput:   "",
			runErr:      fmt.Errorf("exit status 1"),
			want:        "",
			wantErr:     resolver.ErrNoMatch,
		},
		{
			name:        "trims whitespace from zoxide output",
			terms:       "myproject",
			lookPathErr: nil,
			runOutput:   "  /home/user/Code/myproject  \n",
			runErr:      nil,
			want:        "/home/user/Code/myproject",
			wantErr:     nil,
		},
		{
			name:        "handles multi-word query terms",
			terms:       "my project",
			lookPathErr: nil,
			runOutput:   "/home/user/Code/my-project",
			runErr:      nil,
			want:        "/home/user/Code/my-project",
			wantErr:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedArgs []string
			mock := &MockCommandRunner{
				Output: tt.runOutput,
				Err:    tt.runErr,
				OnRun: func(name string, args ...string) {
					capturedArgs = append([]string{name}, args...)
				},
			}
			lookPath := func(file string) (string, error) {
				if tt.lookPathErr != nil {
					return "", tt.lookPathErr
				}
				return "/usr/bin/zoxide", nil
			}

			r := resolver.NewZoxideResolver(mock, lookPath)
			got, err := r.Query(tt.terms)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Query(%q) error = %v, want %v", tt.terms, err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("Query(%q) unexpected error: %v", tt.terms, err)
			}

			if got != tt.want {
				t.Errorf("Query(%q) = %q, want %q", tt.terms, got, tt.want)
			}

			// Verify the command was called correctly when lookPath succeeds
			if tt.lookPathErr == nil {
				wantArgs := append([]string{"zoxide", "query"}, strings.Fields(tt.terms)...)
				if len(capturedArgs) != len(wantArgs) {
					t.Fatalf("command args = %v, want %v", capturedArgs, wantArgs)
				}
				for i, arg := range wantArgs {
					if capturedArgs[i] != arg {
						t.Errorf("arg[%d] = %q, want %q (full: %v)", i, capturedArgs[i], arg, capturedArgs)
					}
				}
			}
		})
	}
}
