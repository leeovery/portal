package tui

import (
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// previewTailLines is the number of trailing lines the preview page reads
// from a pane's saved scrollback. The 1000-line floor is set by the spec
// (§ Out of Scope (v1) → "Deeper history beyond N=1000 lines"). Captured as
// a const so the production wiring and tests reference one value.
const previewTailLines = 1000

// scrollbackReaderAdapter is the production implementation of
// ScrollbackReader. stateDir is closed over once at TUI startup
// (see cmd/open.go's openTUI) so the same directory the daemon and
// bootstrap orchestrator write to is the one preview reads from. n is
// the tail-line floor, supplied at construction so the helper has no
// implicit dependency on the package-level previewTailLines constant.
type scrollbackReaderAdapter struct {
	stateDir string
	n        int
}

// NewProductionScrollbackReader returns the production ScrollbackReader
// adapter closing over stateDir, with the tail-line floor fixed at
// previewTailLines (1000, per spec). cmd/open.go calls this once at TUI
// startup with stateDir resolved via state.Dir(); preview never resolves
// stateDir on its own.
func NewProductionScrollbackReader(stateDir string) ScrollbackReader {
	return scrollbackReaderAdapter{stateDir: stateDir, n: previewTailLines}
}

// Tail resolves the on-disk path via state.ScrollbackFile and reads the
// last n newline-terminated lines via state.TailScrollback. The three
// return shapes documented on ScrollbackReader.Tail flow through
// state.TailScrollback unchanged — this adapter contributes no policy
// of its own beyond the path/byte-count handoff.
func (a scrollbackReaderAdapter) Tail(paneKey string) ([]byte, error) {
	path := state.ScrollbackFile(a.stateDir, paneKey)
	return state.TailScrollback(path, a.n)
}

// Compile-time assertions that the production seams remain satisfied.
// Placing them in this non-test file means a regression in either
// direction breaks the production build, not only the test build.
var (
	_ TmuxEnumerator   = (*tmux.Client)(nil)
	_ ScrollbackReader = scrollbackReaderAdapter{}
)
