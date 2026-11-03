// Package attachkeys reads the keys an attach runner watches for, and finds
// them in a terminal's input.
package attachkeys

// Action is what a bound key asks of the runner attached to a session. Every
// action ends the attach.
type Action int

const (
	// Detach detaches from the session and returns to the picker.
	Detach Action = iota + 1
	// DetachToShell detaches from the session and exits to the shell.
	DetachToShell
	// Kill kills the session and returns to the picker.
	Kill
)
