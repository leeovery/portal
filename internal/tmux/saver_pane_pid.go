package tmux

import (
	"fmt"
	"strconv"
	"strings"
)

// SaverPanePID returns the process PID of the first pane in the named tmux
// session via `tmux list-panes -t =<sessionName> -F '#{pane_pid}'`. It is the
// dependency consumed by Component D's saverMembershipProbe (cmd/state_daemon.go)
// to detect a pid-mismatch (orphan daemon) condition on every tick.
//
// The "=" prefix forces tmux's exact-match target resolution — uniform with
// HasSession / SelectWindow / SelectPane / attach-session — so a prefix
// collision (a killed "_portal-saver" coexisting with a live "_portal-saver-2")
// can never silently resolve to the wrong session. See internal/tmux/tmux.go's
// SwitchClient godoc for the canonical statement of this convention.
//
// Error classification (consumed by callers via errors.Is):
//
//   - Success: (pid, nil) where pid is the parsed first non-empty line of
//     stdout.
//   - ErrNoSuchSession: the underlying *CommandError's stderr contains the
//     lowercase substring "no such session". Wrapped via the package's
//     wrapNoSuchSession helper so multi-%w preserves *CommandError
//     recoverability via errors.As. Surfaces the race where the saver session
//     was destroyed between HasSession and SaverPanePID.
//   - ErrEmptyPaneList: stdout has no non-empty lines after newline-splitting
//     and per-line whitespace trim. Distinct from ErrNoSuchSession (which
//     means the session is gone) — here the session exists but tmux reported
//     zero panes.
//   - ErrPanePIDParse: the first non-empty line failed strconv.Atoi. The
//     original parse error is wrapped on the same chain for diagnostics.
//   - Other generic exec errors: returned wrapped with a contextual prefix.
//     Callers cannot discriminate further than "non-nil non-sentinel error".
//
// Multi-line stdout: defensively, the first non-empty line is taken — the
// _portal-saver session is expected to host exactly one pane, but the helper
// is tolerant of upstream surprises (e.g., transient mid-respawn output).
//
// Whitespace-only stdout is treated as ErrEmptyPaneList, not ErrPanePIDParse:
// "no pane lines at all" is observably equivalent to an empty result.
func SaverPanePID(c *Client, sessionName string) (int, error) {
	out, err := c.cmd.Run("list-panes", "-t", "="+sessionName, "-F", "#{pane_pid}")
	if err != nil {
		wrapped := wrapNoSuchSession(err)
		return 0, fmt.Errorf("list-panes -t %s: %w", sessionName, wrapped)
	}

	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		pid, parseErr := strconv.Atoi(line)
		if parseErr != nil {
			return 0, fmt.Errorf("list-panes -t %s: parse pane_pid %q: %w: %w",
				sessionName, line, ErrPanePIDParse, parseErr)
		}
		return pid, nil
	}
	return 0, fmt.Errorf("list-panes -t %s: %w", sessionName, ErrEmptyPaneList)
}
