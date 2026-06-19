package cmd

// Tests in this file mutate env state and MUST NOT use t.Parallel.

import (
	"os"
	"testing"

	"github.com/leeovery/portal/internal/prefs"
)

// TestNoColorEnabled pins the no-color.org convention the cmd layer reads: the
// NO_COLOR env var enables the colourless carve-out (§2.5) only when it is
// PRESENT and NON-EMPTY. A set-but-empty value does not enable it, and an unset
// var does not enable it. This is the single place NO_COLOR is read.
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
			// t.Setenv registers a cleanup that restores the original value (or
			// unsets it if it was originally absent), so this is safe even when a
			// developer has NO_COLOR exported. For the "unset" case, Setenv-then-
			// Unsetenv leaves it genuinely absent for the duration of the test.
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

// TestBuildTUIModel_NoColorSuppressesCanvas asserts the cmd-layer flag threads
// through buildTUIModel into the colourless render path: with cfg.noColor the
// rendered View sets NO screen background colour (no OSC 11 canvas), while
// without it the canvas is painted (BackgroundColor set). This proves the single
// colourless flag flows cmd → tui.Deps → model.
func TestBuildTUIModel_NoColorSuppressesCanvas(t *testing.T) {
	t.Run("noColor suppresses the canvas background", func(t *testing.T) {
		cfg := defaultTestTUIConfig()
		cfg.noColor = true

		m := buildTUIModel(cfg, "", nil)

		if v := m.View(); v.BackgroundColor != nil {
			t.Errorf("noColor View.BackgroundColor = %v, want nil (canvas suppressed)", v.BackgroundColor)
		}
	})

	t.Run("coloured path still paints the canvas background", func(t *testing.T) {
		cfg := defaultTestTUIConfig()
		cfg.noColor = false
		// Pin the appearance so the gate resolves at construction (the auto gate
		// would hold the blank frame until OSC 11 resolves, which never fires in a
		// non-program test) — then the coloured path paints its canvas immediately.
		cfg.appearance = prefs.AppearanceDark

		m := buildTUIModel(cfg, "", nil)

		if v := m.View(); v.BackgroundColor == nil {
			t.Errorf("coloured View.BackgroundColor = nil, want the canvas colour set")
		}
	})
}
