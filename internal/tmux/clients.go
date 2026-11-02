package tmux

import (
	"fmt"
	"strconv"
	"strings"
)

type ClientInfo struct {
	PID int
	// Activity is tmux's #{client_activity}, in epoch seconds.
	Activity int64
}

// ListClients enumerates the tmux clients attached to the named session. A
// failing invocation yields an empty slice and a nil error — it is the no-server
// / no-clients signal — and only a malformed line returns an error.
func (c *Client) ListClients(session string) ([]ClientInfo, error) {
	output, err := c.cmd.Run("list-clients", "-t", string(CoordTargetExact(session)), "-F", "#{client_pid} #{client_activity}")
	if err != nil {
		// Swallowed deliberately: the error is the zero-clients signal.
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

// In list-clients, #{pane_id} is the client's own current pane: the active pane
// of the window its session is showing.
const clientShowingPaneFormat = "#{client_activity}|#{pane_id}|#{client_name}"

// ClientShowingPane names the client whose current pane is paneID: the client
// a key that pane received was typed into. Of several clients showing it, the
// most recently active is the one that typed. ok is false when no client is
// showing the pane, which is what a key that reached it from send-keys, or
// from a synchronized sibling, finds.
func (c *Client) ClientShowingPane(paneID string) (name string, ok bool, err error) {
	output, err := c.cmd.Run("list-clients", "-F", clientShowingPaneFormat)
	if err != nil {
		return "", false, fmt.Errorf("failed to list clients: %w", err)
	}

	var latest int64
	for line := range strings.SplitSeq(output, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.SplitN(line, "|", 3)
		if len(fields) != 3 {
			return "", false, fmt.Errorf("unexpected client format: %q", line)
		}
		activity, err := strconv.ParseInt(fields[0], 10, 64)
		if err != nil {
			return "", false, fmt.Errorf("invalid client activity %q: %w", fields[0], err)
		}
		if fields[1] != paneID {
			continue
		}
		if !ok || activity > latest {
			name, latest, ok = fields[2], activity, true
		}
	}
	return name, ok, nil
}

// DetachClientTo detaches client and replaces it with command: the client's
// process execs its shell on command, in the environment the client was started
// in, so a client attached from outside tmux runs it outside tmux, in the same
// terminal. The sessions the client was showing are left exactly as they were.
func (c *Client) DetachClientTo(client Target, command string) error {
	if _, err := c.cmd.Run("detach-client", "-t", string(client), "-E", command); err != nil {
		return fmt.Errorf("failed to detach client %s: %w", client, err)
	}
	return nil
}
