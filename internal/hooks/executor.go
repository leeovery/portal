package hooks

import "fmt"

// PaneLister returns the pane IDs belonging to a tmux session.
type PaneLister interface {
	ListPanes(sessionName string) ([]string, error)
}

// KeySender delivers a command to a tmux pane.
type KeySender interface {
	SendKeys(paneID string, command string) error
}

// OptionChecker reads and writes tmux server-level options.
type OptionChecker interface {
	GetServerOption(name string) (string, error)
	SetServerOption(name, value string) error
}

// HookLoader reads the persistent hook registry.
type HookLoader interface {
	Load() (map[string]map[string]string, error)
}

// ExecuteHooks checks each pane in the target session for hooks that need
// re-execution (persistent entry exists AND volatile marker absent) and fires
// restart commands via send-keys. Entirely best-effort with silent error handling.
func ExecuteHooks(sessionName string, lister PaneLister, loader HookLoader, sender KeySender, checker OptionChecker) {
	hookMap, err := loader.Load()
	if err != nil {
		return
	}
	if len(hookMap) == 0 {
		return
	}

	panes, err := lister.ListPanes(sessionName)
	if err != nil {
		return
	}
	if len(panes) == 0 {
		return
	}

	paneSet := make(map[string]struct{}, len(panes))
	for _, p := range panes {
		paneSet[p] = struct{}{}
	}

	for paneID, events := range hookMap {
		if _, inSession := paneSet[paneID]; !inSession {
			continue
		}

		command, hasResume := events["on-resume"]
		if !hasResume {
			continue
		}

		markerName := fmt.Sprintf("@portal-active-%s", paneID)
		if _, err := checker.GetServerOption(markerName); err == nil {
			continue
		}

		_ = sender.SendKeys(paneID, command)
		_ = checker.SetServerOption(markerName, "1")
	}
}
