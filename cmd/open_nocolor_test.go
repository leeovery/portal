package cmd

import (
	"os"
	"testing"

	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/themetest"
)

// The no-color.org convention: NO_COLOR enables the carve-out only when
// present and non-empty.
func TestNoColorEnabled(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  bool
		val  string
		want bool
	}{
		{"unset", false, "", false},
		{"set empty (convention: not enabled)", true, "", false},
		{"set to 1", true, "1", true},
		{"set to any non-empty", true, "true", true},
		{"set to 0 (still non-empty → enabled)", true, "0", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// t.Setenv restores the original value on cleanup, so this is safe even for
			// a developer who exports NO_COLOR; Setenv-then-Unsetenv leaves the "unset"
			// case genuinely absent.
			t.Setenv("NO_COLOR", tc.val)
			if !tc.set {
				if err := os.Unsetenv("NO_COLOR"); err != nil {
					t.Fatalf("failed to unset NO_COLOR: %v", err)
				}
			}
			if got := noColorEnabled(); got != tc.want {
				t.Errorf("noColorEnabled() = %v, want %v (set=%v val=%q)", got, tc.want, tc.set, tc.val)
			}
		})
	}
}

func TestBuildTUIModel_NoColorSuppressesCanvas(t *testing.T) {
	t.Run("noColor suppresses the canvas background", func(t *testing.T) {
		cfg := defaultTestTUIConfig()
		cfg.noColor = true

		m := buildTUIModel(cfg, pickerLanding{}, nil)

		if v := m.View(); v.BackgroundColor != nil {
			t.Errorf("noColor View.BackgroundColor = %v, want nil (canvas suppressed)", v.BackgroundColor)
		}
	})

	t.Run("coloured path still paints the canvas background", func(t *testing.T) {
		cfg := defaultTestTUIConfig()
		cfg.noColor = false
		// A constant nomination resolves the gate at construction; an adaptive pair
		// would hold the blank frame waiting on an OSC 11 reply that never comes in
		// a non-program test.
		cfg.theme = theme.ConstantNomination(themetest.Builtin(t, theme.DefaultDarkSlug))

		m := buildTUIModel(cfg, pickerLanding{}, nil)

		if v := m.View(); v.BackgroundColor == nil {
			t.Errorf("coloured View.BackgroundColor = nil, want the canvas colour set")
		}
	})
}
