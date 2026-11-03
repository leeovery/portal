// Package attachkeys reads the keys an attach runner watches for, and finds
// them in a terminal's input.
package attachkeys

import "strconv"

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

// actions is in the order warnings and summaries name them.
var actions = []Action{Detach, DetachToShell, Kill}

// String is the action's name: its key in prefs.json's keys object, and its
// name in the logs.
func (a Action) String() string {
	switch a {
	case Detach:
		return "detach"
	case DetachToShell:
		return "detach_to_shell"
	case Kill:
		return "kill"
	}
	return "Action(" + strconv.Itoa(int(a)) + ")"
}
