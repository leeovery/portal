package tui

import "github.com/leeovery/portal/internal/tmux"

type TmuxEnumerator interface {
	ListWindowsAndPanesInSession(session string) ([]tmux.WindowGroup, error)
}

// PaneScrollback names where a pane's saved transcript is looked for. PaneKey is
// the canonical key the daemon writes under; PendingToken is the pane's durable
// token while it waits on a resume decision, and empty otherwise.
type PaneScrollback struct {
	PaneKey      string
	PendingToken string
}

// The state directory is hidden behind the interface. Tail returns three
// distinct shapes:
//
//   - (bytes, nil) — content, rendered verbatim.
//   - (nil, nil) — no content; collapses ENOENT, a zero-byte file, and a file
//     holding only an unterminated partial line.
//   - (nil, err) — OS-level read failure.
type ScrollbackReader interface {
	Tail(pane PaneScrollback) ([]byte, error)
}
