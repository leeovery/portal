package tmux

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// saverPanePID returns the process PID of the first pane in the named tmux
// session via `tmux list-panes -t =<sessionName> -F '#{pane_pid}'`. It is the
// low-level primitive backing SaverPanePIDOrAbsent — the sole exported entry
// point for out-of-package callers. Unexported so the centralized
// "any-error → absent" rule encoded by SaverPanePIDOrAbsent is the only
// external path; future consumers cannot accidentally reach past it to the
// rich-sentinel form and drift from the established collapse policy.
//
// The "=" prefix forces tmux's exact-match target resolution — uniform with
// HasSession / SelectWindow / SelectPane / attach-session — so a prefix
// collision (a killed "_portal-saver" coexisting with a live "_portal-saver-2")
// can never silently resolve to the wrong session. See internal/tmux/tmux.go's
// SwitchClient godoc for the canonical statement of this convention.
//
// Error classification (consumed by SaverPanePIDOrAbsent via errors.Is):
//
//   - Success: (pid, nil) where pid is the parsed first non-empty line of
//     stdout.
//   - ErrNoSuchSession: the underlying *CommandError's stderr contains the
//     lowercase substring "no such session". Wrapped via the package's
//     wrapNoSuchSession helper so multi-%w preserves *CommandError
//     recoverability via errors.As. Surfaces the race where the saver session
//     was destroyed between HasSession and saverPanePID.
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
func saverPanePID(c *Client, sessionName string) (int, error) {
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

// SaverPanePIDOrAbsent is a thin wrapper over the unexported saverPanePID
// that centralizes the "session absent or pane list empty → absent" sentinel
// collapse used by both bootstrap step 4's orphan-sweep adapter and
// Component D's saver-membership probe. It is the sole exported entry point —
// out-of-package callers cannot reach past it to the rich-sentinel form.
//
// Return shape:
//
//   - (pid, true, nil)  — saverPanePID succeeded.
//   - (0,   false, nil) — saverPanePID returned ErrNoSuchSession or
//     ErrEmptyPaneList. Both shapes are observably equivalent ("no live
//     pane to bind to") and both legitimate.
//   - (0,   false, err) — any other error. Callers decide policy: bootstrap's
//     orphan-sweep adapter surfaces the error (logged WARN, then proceeds
//     against the full pgrep result); Component D's probe treats it as
//     absent (per spec § Component D self-check sequence "treat any error
//     as absent").
//
// The helper exists so the two callers do not independently re-derive the
// sentinel set — a future addition (e.g., a new "session in teardown"
// sentinel) is then applied in one place.
func SaverPanePIDOrAbsent(c *Client, sessionName string) (pid int, present bool, err error) {
	pid, err = saverPanePID(c, sessionName)
	if err != nil {
		if errors.Is(err, ErrNoSuchSession) || errors.Is(err, ErrEmptyPaneList) {
			return 0, false, nil
		}
		return 0, false, err
	}
	return pid, true, nil
}
