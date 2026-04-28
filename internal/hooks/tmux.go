package hooks

// AllPaneLister returns the structural keys for all panes across all tmux sessions.
// Each key uses the format session_name:window_index.pane_index.
type AllPaneLister interface {
	ListAllPanes() ([]string, error)
}
