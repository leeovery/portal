//go:build manual

package spawn

import "testing"

// TestManual_OpenWindow_OpensRealGhosttyWindow is the irreducible live-terminal
// inch: the only way to confirm a real Ghostty window actually opens (and runs
// the command) is on a live Mac inside Ghostty. It is therefore fenced behind
// the `manual` build tag so it runs in NEITHER default lane:
//
//	go test ./...                     — unit lane, this file is NOT compiled
//	go test -tags integration ./...   — integration lane, this file is NOT compiled
//
// MANUAL: to run it, on a live Mac inside Ghostty (Automation permission
// granted for Ghostty → Ghostty, which is self-exempt in the normal flow):
//
//	go test -tags manual -run TestManual_OpenWindow_OpensRealGhosttyWindow -v ./internal/spawn/
//
// Then visually confirm a NEW Ghostty window opened running the command below
// (it prints the marker line and sleeps so the window is observable; the
// `wait after command:true` property keeps the window up after the command
// exits). The automated assertion only checks OpenWindow reported success —
// the real-window-opened check is the human's eyes.
func TestManual_OpenWindow_OpensRealGhosttyWindow(t *testing.T) {
	// A representative env-self-sufficient argv (the shape Task 2.3 composes):
	// TMUX/TMUX_PANE stripped, run verbatim as a real argv. It runs a visible
	// marker command instead of a real `portal attach` so the manual gate needs
	// no live session — swap in a real `<exe> attach <session>` argv to verify
	// the full attach path.
	argv := []string{
		"/usr/bin/env", "-u", "TMUX", "-u", "TMUX_PANE",
		"/bin/sh", "-c", "echo portal spawn 2.5 manual verification; sleep 5",
	}

	result := newGhosttyAdapter().OpenWindow(argv)

	if !result.OK() {
		t.Fatalf("OpenWindow did not succeed: outcome=%v detail=%q", result.Outcome, result.Detail)
	}
	t.Logf("OpenWindow reported success (detail=%q) — visually confirm a new Ghostty window opened", result.Detail)
}
