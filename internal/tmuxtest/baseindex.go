package tmuxtest

import (
	"fmt"
	"testing"
)

// ApplyBaseIndices sets server-scope and global base-index / pane-base-index
// on the live tmux server backing ts. -g controls the values new sessions
// inherit; -s controls what `show-option -sv` reports — both matter for the
// live coords tmux assigns to fresh sessions/panes.
//
// This is the canonical four-call sequence used by integration-tagged tests
// that need to drive base-index drift scenarios. Keeping the sequence in one
// place ensures all such tests stay byte-identical and cannot drift.
func ApplyBaseIndices(t *testing.T, ts *Socket, base, paneBase int) {
	t.Helper()
	ts.Run(t, "set-option", "-g", "base-index", fmt.Sprintf("%d", base))
	ts.Run(t, "set-option", "-g", "pane-base-index", fmt.Sprintf("%d", paneBase))
	ts.Run(t, "set-option", "-s", "base-index", fmt.Sprintf("%d", base))
	ts.Run(t, "set-option", "-s", "pane-base-index", fmt.Sprintf("%d", paneBase))
}
