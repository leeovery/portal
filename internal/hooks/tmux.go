package hooks

import "fmt"

// AllPaneLister returns the structural keys for all panes across all tmux sessions.
// Each key uses the format session_name:window_index.pane_index.
type AllPaneLister interface {
	ListAllPanes() ([]string, error)
}

// MarkerName returns the tmux server option name used as the volatile marker
// for a given structural key (session_name:window_index.pane_index). This is
// the single source of truth for the marker naming convention.
//
// TODO(task 4-6): delete after hooks.go is detached
func MarkerName(key string) string {
	return fmt.Sprintf("@portal-active-%s", key)
}
