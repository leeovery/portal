package tmux_test

import (
	"os"
	"testing"

	"github.com/leeovery/portal/internal/tmux"
)

func TestCheckTmuxAvailable(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "returns nil when tmux is on PATH",
			path:    os.Getenv("PATH"),
			wantErr: false,
		},
		{
			name:    "returns error when tmux is not on PATH",
			path:    "/nonexistent/path",
			wantErr: true,
			errMsg:  "Portal requires tmux. Install with: brew install tmux",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PATH", tt.path)

			err := tmux.CheckTmuxAvailable()

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if err.Error() != tt.errMsg {
					t.Errorf("error message = %q, want %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}
