package tui

import (
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

const previewTailLines = 1000

// stateDir is fixed at TUI startup so preview reads the daemon's write directory.
type scrollbackReaderAdapter struct {
	stateDir string
	n        int
}

func NewProductionScrollbackReader(stateDir string) ScrollbackReader {
	return scrollbackReaderAdapter{stateDir: stateDir, n: previewTailLines}
}

// A waiting pane's transcript is re-filed under its token once its wait is
// first captured, so the token-named file is read first and the positional one
// only when that holds nothing.
func (a scrollbackReaderAdapter) Tail(pane PaneScrollback) ([]byte, error) {
	if tokenPath, ok := state.PendingScrollbackPath(a.stateDir, pane.PendingToken); ok {
		bytes, err := state.TailScrollback(tokenPath, a.n)
		if bytes != nil || err != nil {
			return bytes, err
		}
	}
	return state.TailScrollback(state.ScrollbackFile(a.stateDir, pane.PaneKey), a.n)
}

// In a non-test file so a seam regression breaks the production build.
var (
	_ TmuxEnumerator   = (*tmux.Client)(nil)
	_ ScrollbackReader = scrollbackReaderAdapter{}
)
