package tmux

import (
	"fmt"
	"strconv"
	"strings"
)

// ClientInfo is one tmux client attached to a session: the client's process id
// and its last-activity timestamp (tmux's #{client_activity}, epoch seconds).
// PID is the walk entry point for inside-tmux host-terminal detection; Activity
// is the cross-client winner-selection signal — the most-active client is the
// burst's trigger, and only that winner's locality is walked.
type ClientInfo struct {
	PID      int
	Activity int64
}

// ListClients enumerates the tmux clients attached to the named session,
// returning each client's pid and last-activity timestamp.
//
// It runs "list-clients -t <session> -F '#{client_pid} #{client_activity}'"
// via the Commander seam. Mirroring ListSessions' no-server tolerance, a
// command error (the canonical "no server / no clients attached" signal, where
// tmux exits non-zero) collapses to an empty slice and a nil error rather than
// surfacing a spurious failure; a genuine parse failure (a malformed line)
// returns an error. The session target is routed through exactTarget so a
// prefix collision cannot mis-resolve to a different session.
func (c *Client) ListClients(session string) ([]ClientInfo, error) {
	output, err := c.cmd.Run("list-clients", "-t", exactTarget(session), "-F", "#{client_pid} #{client_activity}")
	if err != nil {
		// A list-clients error is the "no server / no clients" signal; collapse
		// it to the valid zero-clients state (see ListSessions' rationale).
		return []ClientInfo{}, nil
	}

	if output == "" {
		return []ClientInfo{}, nil
	}

	lines := strings.Split(output, "\n")
	clients := make([]ClientInfo, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) != 2 {
			return nil, fmt.Errorf("unexpected client format: %q", line)
		}

		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			return nil, fmt.Errorf("invalid client pid %q: %w", fields[0], err)
		}

		activity, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid client activity %q: %w", fields[1], err)
		}

		clients = append(clients, ClientInfo{PID: pid, Activity: activity})
	}

	return clients, nil
}
