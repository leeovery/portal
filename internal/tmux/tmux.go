package tmux

import (
	"errors"
	"fmt"
	"os/exec"
	"sort"
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
//
// Run trims surrounding whitespace from the output — convenient for the vast
// majority of tmux commands whose output is a single value or list of newline-
// separated lines that callers want stripped. RunRaw returns the bytes
// verbatim and is reserved for callers that must preserve trailing whitespace
// and ANSI escape sequences (notably capture-pane scrollback dumps used for
// content-hash dedup).
type Commander interface {
	Run(args ...string) (string, error)
	RunRaw(args ...string) (string, error)
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

// RunRaw executes a tmux command and returns its output verbatim — no
// whitespace trim, no transformation. Used by capture-pane where trailing
// blank lines and ANSI escapes are content, not noise.
func (r *RealCommander) RunRaw(args ...string) (string, error) {
	cmd := exec.Command("tmux", args...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
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

// ListSessionNames returns just the names of running tmux sessions, in the
// order tmux reports them. It is a primitive-typed convenience over
// ListSessions for callers that need only names — notably internal/state
// CaptureStructure, which avoids importing internal/tmux to prevent an
// import cycle.
func (c *Client) ListSessionNames() ([]string, error) {
	sessions, err := c.ListSessions()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(sessions))
	for _, s := range sessions {
		names = append(names, s.Name)
	}
	return names, nil
}

// StartServer starts the tmux server by creating a detached bootstrap session.
// Uses "new-session -d" instead of "start-server" so the server has at least one
// session, preventing tmux's default "exit-empty on" from terminating the server
// before plugins like tmux-continuum can restore saved sessions.
// The unnamed session defaults to "0", which tmux-resurrect recognizes and cleans up.
// Returns nil on success or a wrapped error on failure. No retry logic.
func (c *Client) StartServer() error {
	_, err := c.cmd.Run("new-session", "-d")
	if err != nil {
		return fmt.Errorf("failed to start tmux server (bootstrap session): %w", err)
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

// SetSessionOption sets a tmux session-level option scoped to the given session.
// The -t flag is load-bearing: it scopes the option to one session, never the
// global namespace. Callers must not pass -g style global options through this
// method.
func (c *Client) SetSessionOption(session, name, value string) error {
	_, err := c.cmd.Run("set-option", "-t", session, name, value)
	if err != nil {
		return fmt.Errorf("failed to set session option %s on %s: %w", name, session, err)
	}
	return nil
}

// NewDetachedSessionNoCwd creates a new detached tmux session with the given
// name without specifying a working directory. When shellCommand is non-empty,
// it is appended as the tmux shell-command argument. Used for internal
// sessions like _portal-saver where inheriting tmux's default cwd is intended.
func (c *Client) NewDetachedSessionNoCwd(name, shellCommand string) error {
	args := []string{"new-session", "-d", "-s", name}
	if shellCommand != "" {
		args = append(args, shellCommand)
	}
	_, err := c.cmd.Run(args...)
	if err != nil {
		return fmt.Errorf("failed to create tmux session %q: %w", name, err)
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

// TryGetServerOption returns the value of a tmux server-level option along
// with a found flag. When the option does not exist, it returns ("", false, nil)
// — distinguishing absence from a real tmux failure (which surfaces as a
// non-nil error). Callers that need to treat "not found" as a normal control-
// flow case should prefer this over GetServerOption.
func (c *Client) TryGetServerOption(name string) (string, bool, error) {
	val, err := c.GetServerOption(name)
	if errors.Is(err, ErrOptionNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return val, true, nil
}

// ShowAllServerOptions returns the raw output of "tmux show-options -sv". The
// output is one option per line in the form `@name "value"` (or `@name value`
// for unquoted scalars). Callers parse it themselves; this method exists so
// the daemon can dump every server option in a single tmux invocation rather
// than calling GetServerOption per pane during marker enumeration.
func (c *Client) ShowAllServerOptions() (string, error) {
	out, err := c.cmd.Run("show-options", "-sv")
	if err != nil {
		return "", fmt.Errorf("failed to show server options: %w", err)
	}
	return out, nil
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

// PaneCoord is a live tmux pane address: a (window_index, pane_index) pair
// reported by tmux for a single pane within a session. Used by the restore
// pipeline to map structural saved positions to actual live indices when
// base-index / pane-base-index drift between save and restore.
type PaneCoord struct {
	Window int
	Pane   int
}

// ListPanesInSession enumerates live panes in the named session and returns
// their (window, pane) coords sorted by window then pane. Uses
// "list-panes -s -t <session>" so all panes across all windows of the session
// are returned, regardless of which window is currently selected.
func (c *Client) ListPanesInSession(session string) ([]PaneCoord, error) {
	out, err := c.cmd.Run("list-panes", "-s", "-t", session, "-F", "#{window_index}:#{pane_index}")
	if err != nil {
		return nil, fmt.Errorf("failed to list panes in session %q: %w", session, err)
	}
	if out == "" {
		return []PaneCoord{}, nil
	}

	var coords []PaneCoord
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("unexpected pane format %q", line)
		}
		win, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, fmt.Errorf("invalid window index %q in pane line %q: %w", parts[0], line, err)
		}
		pane, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid pane index %q in pane line %q: %w", parts[1], line, err)
		}
		coords = append(coords, PaneCoord{Window: win, Pane: pane})
	}

	sort.Slice(coords, func(i, j int) bool {
		if coords[i].Window != coords[j].Window {
			return coords[i].Window < coords[j].Window
		}
		return coords[i].Pane < coords[j].Pane
	})
	return coords, nil
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

// ListAllPanesWithFormat runs "list-panes -a -F <format>" and returns the raw,
// untrimmed tmux output. Callers are responsible for parsing the format string
// they supplied. Unlike ListAllPanes, this method propagates the underlying
// error so callers can distinguish "no panes" from "tmux failed".
func (c *Client) ListAllPanesWithFormat(format string) (string, error) {
	out, err := c.cmd.Run("list-panes", "-a", "-F", format)
	if err != nil {
		return "", fmt.Errorf("failed to list panes: %w", err)
	}
	return out, nil
}

// ShowEnvironment returns the raw output of "tmux show-environment -t <session>".
// Each line is of the form "NAME=value" or "-NAME" (for entries removed from
// the session environment). Callers are responsible for parsing.
func (c *Client) ShowEnvironment(session string) (string, error) {
	out, err := c.cmd.Run("show-environment", "-t", session)
	if err != nil {
		return "", fmt.Errorf("failed to show environment for session %q: %w", session, err)
	}
	return out, nil
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

// UnsetServerOption removes a tmux server-level option via "set-option -su".
// The -s flag targets the server-option scope; -u removes (unsets) the option.
// Like DeleteServerOption, this is a no-op when the option is already absent —
// tmux does not error in that case. This method exists alongside
// DeleteServerOption to provide a Set/Unset-named pair for the
// @portal-restoring marker coordination in the bootstrap flow.
func (c *Client) UnsetServerOption(name string) error {
	_, err := c.cmd.Run("set-option", "-su", name)
	if err != nil {
		return fmt.Errorf("failed to unset server option %s: %w", name, err)
	}
	return nil
}

// ShowGlobalHooks returns the raw output of "tmux show-hooks -g".
// The output is returned verbatim (no trimming) so callers can parse the
// array-indexed hook entries with line and whitespace fidelity intact.
func (c *Client) ShowGlobalHooks() (string, error) {
	output, err := c.cmd.Run("show-hooks", "-g")
	if err != nil {
		return "", fmt.Errorf("failed to show global hooks: %w", err)
	}
	return output, nil
}

// AppendGlobalHook appends a command to the global hook array for the given event
// via "tmux set-hook -ga". The command is passed as a single argv element so that
// single quotes and shell metacharacters within it are preserved verbatim.
func (c *Client) AppendGlobalHook(event, command string) error {
	_, err := c.cmd.Run("set-hook", "-ga", event, command)
	if err != nil {
		return fmt.Errorf("failed to append hook on %q: %w", event, err)
	}
	return nil
}

// CapturePane returns the raw scrollback for the given pane target via
// "tmux capture-pane -e -p -S - -t <target>". The output is returned verbatim
// (no trimming) so the caller can hash and persist it byte-for-byte. The -e
// flag preserves ANSI escape sequences and -S - replays from the start of the
// history buffer.
func (c *Client) CapturePane(target string) (string, error) {
	out, err := c.cmd.RunRaw("capture-pane", "-e", "-p", "-S", "-", "-t", target)
	if err != nil {
		return "", fmt.Errorf("failed to capture pane %q: %w", target, err)
	}
	return out, nil
}

// NewSessionWithCommand creates a new detached tmux session with the given
// name. When cwd is non-empty it is passed as -c; when shellCommand is
// non-empty it is appended as the trailing argument and becomes the pane's
// initial process. Distinct from NewSession in that cwd is optional — used by
// the restore path where saved panes may have no recorded cwd.
func (c *Client) NewSessionWithCommand(name, cwd, shellCommand string) error {
	args := []string{"new-session", "-d", "-s", name}
	if cwd != "" {
		args = append(args, "-c", cwd)
	}
	if shellCommand != "" {
		args = append(args, shellCommand)
	}
	_, err := c.cmd.Run(args...)
	if err != nil {
		return fmt.Errorf("failed to create session %q: %w", name, err)
	}
	return nil
}

// NewWindow creates a new tmux window in the session identified by target.
// Optional name (-n), cwd (-c), and shellCommand are appended only when
// non-empty.
func (c *Client) NewWindow(target, name, cwd, shellCommand string) error {
	args := []string{"new-window", "-t", target}
	if name != "" {
		args = append(args, "-n", name)
	}
	if cwd != "" {
		args = append(args, "-c", cwd)
	}
	if shellCommand != "" {
		args = append(args, shellCommand)
	}
	_, err := c.cmd.Run(args...)
	if err != nil {
		return fmt.Errorf("failed to create window in %q: %w", target, err)
	}
	return nil
}

// SplitWindow splits the tmux window identified by target into a new pane.
// Optional cwd (-c) and shellCommand are appended only when non-empty.
func (c *Client) SplitWindow(target, cwd, shellCommand string) error {
	args := []string{"split-window", "-t", target}
	if cwd != "" {
		args = append(args, "-c", cwd)
	}
	if shellCommand != "" {
		args = append(args, shellCommand)
	}
	_, err := c.cmd.Run(args...)
	if err != nil {
		return fmt.Errorf("failed to split window %q: %w", target, err)
	}
	return nil
}

// SetSessionEnvironment sets a single environment variable on the named
// session via "tmux set-environment -t <session> <key> <value>".
func (c *Client) SetSessionEnvironment(session, key, value string) error {
	_, err := c.cmd.Run("set-environment", "-t", session, key, value)
	if err != nil {
		return fmt.Errorf("failed to set env %s on %q: %w", key, session, err)
	}
	return nil
}

// SelectLayout applies the given layout string to the named window via
// "tmux select-layout -t <session>:<window> <layout>". Used during restoration
// to recreate the saved geometry once panes exist; tmux fits panes to the
// layout in place, so the call is order-sensitive (must come after all
// split-windows for that window).
func (c *Client) SelectLayout(session string, window int, layout string) error {
	target := fmt.Sprintf("%s:%d", session, window)
	args := []string{"select-layout", "-t", target, layout}
	_, err := c.cmd.Run(args...)
	if err != nil {
		return fmt.Errorf("failed to select-layout %s: %w", target, err)
	}
	return nil
}

// SelectPane sets the active pane within the named window via
// "tmux select-pane -t <session>:<window>.<pane>". The pane index is the live
// index after restoration (caller is responsible for translating saved →
// live).
func (c *Client) SelectPane(session string, window, pane int) error {
	target := fmt.Sprintf("%s:%d.%d", session, window, pane)
	args := []string{"select-pane", "-t", target}
	_, err := c.cmd.Run(args...)
	if err != nil {
		return fmt.Errorf("failed to select-pane %s: %w", target, err)
	}
	return nil
}

// ResizePaneZoom toggles zoom on the named pane via "tmux resize-pane -Z -t
// <session>:<window>.<pane>". Zoom is a toggle; callers must apply it only
// when the saved state indicated a zoomed window and the layout has just been
// freshly applied (which leaves zoom off), to land on the correct final state.
func (c *Client) ResizePaneZoom(session string, window, pane int) error {
	target := fmt.Sprintf("%s:%d.%d", session, window, pane)
	args := []string{"resize-pane", "-Z", "-t", target}
	_, err := c.cmd.Run(args...)
	if err != nil {
		return fmt.Errorf("failed to resize-pane -Z %s: %w", target, err)
	}
	return nil
}

// UnsetGlobalHookAt removes the hook at the given index for the given event
// via "tmux set-hook -gu <event>[<index>]". Surrounding entries are not affected.
func (c *Client) UnsetGlobalHookAt(event string, index int) error {
	_, err := c.cmd.Run("set-hook", "-gu", fmt.Sprintf("%s[%d]", event, index))
	if err != nil {
		return fmt.Errorf("failed to unset hook %s[%d]: %w", event, index, err)
	}
	return nil
}
