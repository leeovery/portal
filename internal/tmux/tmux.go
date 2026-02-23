package tmux

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Session represents a running tmux session.
type Session struct {
	Name     string
	Windows  int
	Attached bool
}

// Commander defines the interface for executing tmux commands.
type Commander interface {
	Run(args ...string) (string, error)
}

// RealCommander executes tmux commands via os/exec.
type RealCommander struct{}

// Run executes a tmux command with the given arguments and returns its output.
func (r *RealCommander) Run(args ...string) (string, error) {
	cmd := exec.Command("tmux", args...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Client provides tmux operations using a Commander.
type Client struct {
	cmd Commander
}

// NewClient creates a new Client with the given Commander.
func NewClient(cmd Commander) *Client {
	return &Client{cmd: cmd}
}

// HasSession reports whether a tmux session with the given name exists.
// Returns false when the session does not exist or no tmux server is running.
func (c *Client) HasSession(name string) bool {
	_, err := c.cmd.Run("has-session", "-t", name)
	return err == nil
}

// NewSession creates a new detached tmux session with the given name and start directory.
func (c *Client) NewSession(name, dir string) error {
	_, err := c.cmd.Run("new-session", "-d", "-s", name, "-c", dir)
	if err != nil {
		return fmt.Errorf("failed to create tmux session %q: %w", name, err)
	}
	return nil
}

// ListSessions queries tmux for running sessions and returns them as structured data.
// Returns an empty slice and nil error when no tmux server is running.
func (c *Client) ListSessions() ([]Session, error) {
	output, err := c.cmd.Run("list-sessions", "-F", "#{session_name}|#{session_windows}|#{session_attached}")
	if err != nil {
		return []Session{}, nil
	}

	if output == "" {
		return []Session{}, nil
	}

	lines := strings.Split(output, "\n")
	sessions := make([]Session, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "|", 3)
		if len(parts) != 3 {
			return nil, fmt.Errorf("unexpected session format: %q", line)
		}

		windows, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid window count %q: %w", parts[1], err)
		}

		attachedCount, err := strconv.Atoi(parts[2])
		if err != nil {
			return nil, fmt.Errorf("invalid attached count %q: %w", parts[2], err)
		}

		sessions = append(sessions, Session{
			Name:     parts[0],
			Windows:  windows,
			Attached: attachedCount > 0,
		})
	}

	return sessions, nil
}

// CurrentSessionName returns the name of the tmux session that the current client
// is attached to. It runs tmux display-message to query the session name.
func (c *Client) CurrentSessionName() (string, error) {
	output, err := c.cmd.Run("display-message", "-p", "#{session_name}")
	if err != nil {
		return "", fmt.Errorf("failed to get current session name: %w", err)
	}
	return output, nil
}

// SwitchClient switches the current tmux client to the named session.
// Used when Portal is running inside an existing tmux session.
func (c *Client) SwitchClient(name string) error {
	_, err := c.cmd.Run("switch-client", "-t", name)
	if err != nil {
		return fmt.Errorf("failed to switch to session %q: %w", name, err)
	}
	return nil
}
