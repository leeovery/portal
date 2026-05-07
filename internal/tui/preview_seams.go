package tui

import "github.com/leeovery/portal/internal/tmux"

// TmuxEnumerator is the seam through which the preview page reads window-
// grouped pane structure for a single session. It mirrors the Phase 1
// *tmux.Client.ListWindowsAndPanesInSession signature exactly so the
// production wiring is a direct method-value handoff, while tests can
// substitute an in-memory shape without spinning up a real tmux server.
//
// The return type tmux.WindowGroup is reused unchanged — preview never
// redefines the structural shape locally.
type TmuxEnumerator interface {
	ListWindowsAndPanesInSession(session string) ([]tmux.WindowGroup, error)
}

// ScrollbackReader is the seam through which the preview page reads the
// last N lines of a pane's saved scrollback, keyed by the canonical paneKey
// the daemon writes with. The state directory in which .bin files live is
// intentionally hidden behind this interface — the production adapter
// closes over stateDir at TUI-startup construction (see § Cross-cutting
// Seams > State Package API Reuse > stateDir resolution in the spec) so
// stateDir is captured once and stable for the Portal process lifetime,
// and tests can mock by paneKey alone without owning a state-path policy.
//
// Tail returns three observable (bytes, err) shapes that the caller maps to
// distinct rendering outcomes:
//
//   - (bytes != nil, nil) — normal content; caller renders bytes verbatim.
//   - (nil, nil) — "no content available" (collapses ENOENT, zero-byte
//     file, and zero-line file with only an unterminated partial). Caller
//     renders the placeholder "(no saved content)".
//   - (nil, err != nil) — OS-level read failure (EACCES, EIO, etc.).
//     Caller renders the error string.
//
// The three "no content" cases are unified by design — the placeholder /
// error decision lives at the call site in internal/tui, not in the
// helper. Mocks honour the same three shapes.
type ScrollbackReader interface {
	Tail(paneKey string) ([]byte, error)
}
