package capture

import (
	"os"
	"testing"

	"github.com/leeovery/portal/internal/spawn"
)

// The frames are drawn with narrow East Asian widths wherever the tests run.
// Every adapter panics rather than open a window, as in every package whose
// tests reach the picker.
func TestMain(m *testing.M) {
	if err := os.Setenv("TMUX_TMPDIR", "/dev/null/tmpdir-poison"); err != nil {
		panic(err)
	}
	spawn.PoisonAdaptersForTest()
	os.Exit(RunNarrow(m))
}
