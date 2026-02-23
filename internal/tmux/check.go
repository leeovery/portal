// Package tmux provides tmux integration for Portal.
package tmux

import (
	"errors"
	"os/exec"
)

// CheckTmuxAvailable verifies that tmux is installed and available on PATH.
// Returns nil if tmux is found, or an error with install instructions if not.
func CheckTmuxAvailable() error {
	_, err := exec.LookPath("tmux")
	if err != nil {
		return errors.New("Portal requires tmux. Install with: brew install tmux") //nolint:staticcheck // user-facing message requires capitalization per spec
	}
	return nil
}
