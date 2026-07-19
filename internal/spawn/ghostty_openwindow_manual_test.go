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
// (it prints the marker line; the `wait after command:true` property keeps the
// window up after the command exits so it stays observable). The automated
// assertion only checks OpenWindow reported success — the real-window-opened
// check is the human's eyes.
func TestManual_OpenWindow_OpensRealGhosttyWindow(t *testing.T) {
	// A representative env-self-sufficient argv (the shape composeOpenArgv
	// produces): TMUX/TMUX_PANE stripped, run verbatim as a real argv. It runs a
	// visible marker command instead of a real `portal open` so the manual gate
	// needs no live session — swap in a real `<exe> open --session <name>` argv to
	// verify the full attach path.
	//
	// ghosttyEmbed POSIX-single-quotes each argv element before joining, and
	// Ghostty runs the resulting `command:"…"` string via `login … bash -c
	// "exec -l <string>"`; the per-element quoting means bash's word-split
	// reproduces the exact argv, so an element carrying spaces or shell
	// metacharacters (e.g. a `My Project-abc123` session name) survives intact
	// rather than being shredded. This marker uses plain word tokens purely for a
	// simple, readable observable command; `wait after command:true` holds the
	// window open after it exits, so no `sleep` is needed to observe the window.
	argv := []string{
		"/usr/bin/env", "-u", "TMUX", "-u", "TMUX_PANE",
		"echo", "portal", "spawn", "manual", "verification",
	}

	result := newGhosttyAdapter().OpenWindow(argv)

	if !result.OK() {
		t.Fatalf("OpenWindow did not succeed: outcome=%v detail=%q", result.Outcome, result.Detail)
	}
	t.Logf("OpenWindow reported success (detail=%q) — visually confirm a new Ghostty window opened", result.Detail)
}
