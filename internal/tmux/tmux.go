package tmux

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// ErrOptionNotFound is returned when a tmux server option does not exist.
var ErrOptionNotFound = errors.New("option not found")

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

// ServerRunning reports whether a tmux server is currently running.
// It runs "tmux info" which succeeds even with zero sessions.
func (c *Client) ServerRunning() bool {
	_, err := c.cmd.Run("info")
	return err == nil
}

// HasSession reports whether a tmux session with the given name exists.
// Returns false when the session does not exist or no tmux server is running.
func (c *Client) HasSession(name string) bool {
	_, err := c.cmd.Run("has-session", "-t", name)
	return err == nil
}

// NewSession creates a new detached tmux session with the given name and start directory.
// When shellCommand is non-empty, it is appended as the tmux shell-command argument.
func (c *Client) NewSession(name, dir, shellCommand string) error {
	args := []string{"new-session", "-d", "-s", name, "-c", dir}
	if shellCommand != "" {
		args = append(args, shellCommand)
	}
	_, err := c.cmd.Run(args...)
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

// StartServer starts the tmux server without creating any sessions.
// Returns nil on success or a wrapped error on failure. No retry logic.
func (c *Client) StartServer() error {
	_, err := c.cmd.Run("start-server")
	if err != nil {
		return fmt.Errorf("failed to start tmux server: %w", err)
	}
	return nil
}

// EnsureServer checks if a tmux server is running and starts one if not.
// Returns (false, nil) when the server was already running.
// Returns (true, nil) when a server was successfully started.
// Returns (true, err) when a server start was attempted but failed.
func (c *Client) EnsureServer() (bool, error) {
	if c.ServerRunning() {
		return false, nil
	}
	if err := c.StartServer(); err != nil {
		return true, err
	}
	return true, nil
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

// ResolveStructuralKey resolves a pane ID (e.g. "%3") to its structural key
// (e.g. "my-project:0.1") by querying tmux for the pane's session name,
// window index, and pane index.
func (c *Client) ResolveStructuralKey(paneID string) (string, error) {
	output, err := c.cmd.Run("display-message", "-p", "-t", paneID, "#{session_name}:#{window_index}.#{pane_index}")
	if err != nil {
		return "", fmt.Errorf("failed to resolve structural key for pane %q: %w", paneID, err)
	}
	return output, nil
}

// KillSession kills the tmux session with the given name.
func (c *Client) KillSession(name string) error {
	_, err := c.cmd.Run("kill-session", "-t", name)
	if err != nil {
		return fmt.Errorf("failed to kill tmux session %q: %w", name, err)
	}
	return nil
}

// RenameSession renames a tmux session from oldName to newName.
func (c *Client) RenameSession(oldName, newName string) error {
	_, err := c.cmd.Run("rename-session", "-t", oldName, newName)
	if err != nil {
		return fmt.Errorf("failed to rename tmux session %q to %q: %w", oldName, newName, err)
	}
	return nil
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

// SetServerOption sets a tmux server-level option.
func (c *Client) SetServerOption(name, value string) error {
	_, err := c.cmd.Run("set-option", "-s", name, value)
	if err != nil {
		return fmt.Errorf("failed to set server option %q: %w", name, err)
	}
	return nil
}

// GetServerOption returns the value of a tmux server-level option.
// Returns ErrOptionNotFound when the option does not exist.
func (c *Client) GetServerOption(name string) (string, error) {
	output, err := c.cmd.Run("show-option", "-sv", name)
	if err != nil {
		return "", ErrOptionNotFound
	}
	return strings.TrimSpace(output), nil
}

// parsePaneOutput splits newline-delimited tmux output into trimmed, non-empty strings.
func parsePaneOutput(output string) []string {
	if output == "" {
		return []string{}
	}

	lines := strings.Split(output, "\n")
	panes := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		panes = append(panes, line)
	}
	return panes
}

// ListPanes returns the structural keys for panes belonging to the named tmux session.
// Each key has the form "session_name:window_index.pane_index" (e.g. "my-project:0.0").
// Structural keys survive tmux server restarts (unlike ephemeral pane IDs).
func (c *Client) ListPanes(sessionName string) ([]string, error) {
	output, err := c.cmd.Run("list-panes", "-t", sessionName, "-F", "#{session_name}:#{window_index}.#{pane_index}")
	if err != nil {
		return nil, fmt.Errorf("failed to list panes for session %q: %w", sessionName, err)
	}
	return parsePaneOutput(output), nil
}

// ListAllPanes returns the structural keys for all panes across all tmux sessions.
// Each key has the form "session_name:window_index.pane_index" (e.g. "my-project:0.0").
// Structural keys survive tmux server restarts (unlike ephemeral pane IDs).
// Returns an empty slice and nil error when no tmux server is running.
func (c *Client) ListAllPanes() ([]string, error) {
	output, err := c.cmd.Run("list-panes", "-a", "-F", "#{session_name}:#{window_index}.#{pane_index}")
	if err != nil {
		return []string{}, nil
	}
	return parsePaneOutput(output), nil
}

// SendKeys delivers a command to the specified tmux pane followed by Enter.
// The target parameter is a structural key (e.g. "my-session:0.1").
func (c *Client) SendKeys(target string, command string) error {
	_, err := c.cmd.Run("send-keys", "-t", target, command, "Enter")
	if err != nil {
		return fmt.Errorf("failed to send keys to pane %q: %w", target, err)
	}
	return nil
}

// DeleteServerOption removes a tmux server-level option.
// This is a no-op if the option does not exist.
func (c *Client) DeleteServerOption(name string) error {
	_, err := c.cmd.Run("set-option", "-su", name)
	if err != nil {
		return fmt.Errorf("failed to delete server option %q: %w", name, err)
	}
	return nil
}
