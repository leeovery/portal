package tmux

import "os"

// InsideTmux reports whether Portal is running inside an existing tmux session.
// It checks whether the TMUX environment variable is set and non-empty.
func InsideTmux() bool {
	return os.Getenv("TMUX") != ""
}
