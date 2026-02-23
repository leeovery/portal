package tmux_test

import (
	"os"
	"testing"

	"github.com/leeovery/portal/internal/tmux"
)

func TestInsideTmux(t *testing.T) {
	tests := []struct {
		name   string
		envVal string
		want   bool
	}{
		{
			name:   "returns true when TMUX is set and non-empty",
			envVal: "/tmp/tmux-501/default,12345,0",
			want:   true,
		},
		{
			name:   "returns false when TMUX is empty string",
			envVal: "",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TMUX", tt.envVal)

			got := tmux.InsideTmux()

			if got != tt.want {
				t.Errorf("InsideTmux() = %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("returns false when TMUX is unset", func(t *testing.T) {
		original, wasSet := os.LookupEnv("TMUX")
		if err := os.Unsetenv("TMUX"); err != nil {
			t.Fatalf("failed to unset TMUX: %v", err)
		}
		t.Cleanup(func() {
			if wasSet {
				_ = os.Setenv("TMUX", original)
			} else {
				_ = os.Unsetenv("TMUX")
			}
		})

		got := tmux.InsideTmux()

		if got != false {
			t.Errorf("InsideTmux() = %v, want false", got)
		}
	})
}
