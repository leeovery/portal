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

// optionAbsentStderrPatterns lists the stderr substrings tmux uses to signal
// that a server option does not exist. Substring match is case-sensitive.
//
// Used by GetServerOption to discriminate genuine option absence from
// transport faults (lost server, socket connect failures, etc.). Adding a
// future tmux phrasing requires a one-line extension here; the same-package
// internal test file iterates this slice directly so the coverage scales
// automatically.
var optionAbsentStderrPatterns = []string{
	"invalid option:",
	"unknown option:",
	"ambiguous option:",
}

// Session represents a running tmux session.
type Session struct {
	Name     string
	Windows  int
	Attached bool
	// Dir is the session's stamped @portal-dir user-option (the resolved
	// git-root captured at session creation). Empty when the session carries
	// no @portal-dir stamp (e.g. restored post-reboot, where the in-memory
	// option is not persisted, or a pre-existing session from before stamping
	// shipped). See spec § The stamp / § The lazy stamp-on-render fallback.
	Dir string
}

// Commander defines the interface for executing tmux commands.
//
// Run trims surrounding whitespace from the output — convenient for the vast
// majority of tmux commands whose output is a single value or list of newline-
// separated lines that callers want stripped. RunRaw returns the bytes
// verbatim and is reserved for callers that must preserve trailing whitespace
// and ANSI escape sequences (notably capture-pane scrollback dumps used for
// content-hash dedup).
//
// Production implementations return non-nil errors as *CommandError so callers
// can recover the child's stderr via errors.As.
type Commander interface {
	Run(args ...string) (string, error)
	RunRaw(args ...string) (string, error)
}

// RealCommander executes tmux commands via os/exec.
type RealCommander struct{}

// Run executes a tmux command with the given arguments and returns its output
// trimmed of surrounding whitespace. Non-nil errors are wrapped in
// *CommandError so callers can recover the child's stderr via errors.As.
func (r *RealCommander) Run(args ...string) (string, error) {
	return runCommand("tmux", true, args...)
}

// RunRaw executes a tmux command and returns its output verbatim — no
// whitespace trim, no transformation. Used by capture-pane where trailing
// blank lines and ANSI escapes are content, not noise. Error wrapping is
// identical to Run: non-nil errors are returned as *CommandError.
func (r *RealCommander) RunRaw(args ...string) (string, error) {
	return runCommand("tmux", false, args...)
}

// runCommand is the shared exec seam behind RealCommander.Run / RunRaw. The
// trim flag selects Run's TrimSpace behaviour vs RunRaw's verbatim output.
// Non-nil errors from cmd.Output() are wrapped in *CommandError via
// WrapCommandError so callers can inspect the child's stderr via errors.As
// without coupling to os/exec.
//
// Invariant: cmd.Stderr is deliberately left nil — see WrapCommandError's
// godoc for the full precondition (in short: assigning cmd.Stderr silently
// zeroes (*exec.ExitError).Stderr and defeats the wrap).
func runCommand(binary string, trim bool, args ...string) (string, error) {
	cmd := exec.Command(binary, args...)
	// cmd.Stderr left nil — see WrapCommandError precondition.
	out, err := cmd.Output()
	if err != nil {
		// Thread the tmux argv into the wrap so a downstream log site can
		// recover which invocation failed. cmd.Stderr stays nil — argv is
		// additive context, NOT stderr capture.
		return "", WrapCommandError(err, args...)
	}
	if trim {
		return strings.TrimSpace(string(out)), nil
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

// DefaultClient returns a Client backed by RealCommander — the production
// constructor used by every cmd-layer entry point that does not have a
// test-injected dependency. Centralising it gives production-client
// construction a single entry point.
func DefaultClient() *Client {
	return NewClient(&RealCommander{})
}

// ServerRunning reports whether a tmux server is currently running.
// It runs "tmux info" which succeeds even with zero sessions.
func (c *Client) ServerRunning() bool {
	_, err := c.cmd.Run("info")
	return err == nil
}

// HasSession reports whether a tmux session with the given name exists.
// Returns false when the session does not exist or no tmux server is running.
//
// The "=" prefix forces tmux's exact-match target resolution rather than the
// default prefix match. Without it, a killed session "foo" coexisting with a
// live "foo-2" would silently resolve `has-session -t foo` to "foo-2",
// bypassing the session-killed-externally bail path in the preview Enter
// sequence. See spec § Pre-select + attach sequence > Exact-match target
// syntax.
func (c *Client) HasSession(name string) bool {
	_, err := c.cmd.Run("has-session", "-t", "="+name)
	return err == nil
}

// HasSessionProbe is the discriminating variant of HasSession used by the
// preview Enter / pre-select-and-attach pipeline. It distinguishes a genuine
// non-zero tmux exit (session truly absent — caller bails to the externally-
// killed UX) from an OS-layer fault (missing tmux binary, exec lookup failure,
// transport hiccup) where the safe default is to assume the session is present
// and proceed.
//
// Three observable shapes:
//
//  1. (true, nil) — tmux returned zero exit: session present.
//  2. (false, err) — tmux returned a non-zero exit (underlying error unwraps
//     to *exec.ExitError): session absent; caller bails. The returned err
//     preserves *CommandError shape so callers can still recover stderr via
//     errors.As.
//  3. (true, err) — underlying error does NOT unwrap to *exec.ExitError
//     (OS-layer fault): caller proceeds as if present; err is intended to be
//     logged at WARN, not surfaced to the user as a missing session.
//
// The "=" prefix forces tmux's exact-match target resolution — see
// HasSession's godoc for the prefix-collision rationale. The plain
// HasSession(name) bool form is unchanged and remains available for callers
// that don't need the discriminator.
//
// Spec: .workflows/enter-attaches-from-preview/specification/enter-attaches-from-preview/specification.md
// § Pre-select + attach sequence > step 1.
func (c *Client) HasSessionProbe(name string) (bool, error) {
	_, err := c.cmd.Run("has-session", "-t", "="+name)
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return false, err
	}
	return true, err
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
	// @portal-dir is intentionally the LAST format field: a directory path may
	// contain a literal '|', so it must occupy the unbounded trailing slot. The
	// parser below splits with SplitN(line, "|", 4) so parts[3] is everything
	// after the third pipe, preserving any embedded pipes in the path.
	output, err := c.cmd.Run("list-sessions", "-F", "#{session_name}|#{session_windows}|#{session_attached}|#{@portal-dir}")
	if err != nil {
		// A list-sessions error is the canonical "no server running" signal
		// (tmux exits non-zero when there are no sessions). Collapse it to the
		// valid zero-sessions state rather than surfacing a spurious error.
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

		parts := strings.SplitN(line, "|", 4)
		if len(parts) != 4 {
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
			// An absent/empty @portal-dir yields an empty trailing field.
			Dir: parts[3],
		})
	}

	// Filter out sessions whose names start with "_". Portal-wide invariant:
	// underscore-prefixed names (e.g. _portal-saver) are internal and must
	// never leak into user-visible output. Applied as the final post-
	// processing step so every current and future caller — including
	// ListSessionNames, which delegates to ListSessions — inherits the
	// invariant without per-consumer code changes. Always returns a non-nil
	// slice so callers can rely on len(result) == 0 and JSON [] (not null).
	filtered := make([]Session, 0, len(sessions))
	for _, s := range sessions {
		if strings.HasPrefix(s.Name, "_") {
			continue
		}
		filtered = append(filtered, s)
	}
	return filtered, nil
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

// StartServer starts the tmux server by creating a detached bootstrap session
// with the reserved name PortalBootstrapName. Using
// "new-session -d" instead of "start-server" guarantees the server has at
// least one session at the moment it comes up, preventing tmux's default
// "exit-empty on" from terminating the server before Portal's own bootstrap
// Restore step (bootstrap step 6 in cmd/bootstrap) has had a chance to
// reconstruct user sessions from saved state. The bootstrap session is hidden
// from user-facing listings by the underscore-prefix filter in
// Client.ListSessions, so it is never visible in the TUI picker or
// `portal list` even if it persists past Restore.
// Returns nil on success or a wrapped error on failure. No retry logic.
func (c *Client) StartServer() error {
	_, err := c.cmd.Run("new-session", "-d", "-s", PortalBootstrapName)
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
	output, err := c.cmd.Run("display-message", "-p", "-t", paneID, StructuralKeyFormat)
	if err != nil {
		return "", fmt.Errorf("failed to resolve structural key for pane %q: %w", paneID, err)
	}
	return output, nil
}

// ActivePaneCurrentPath returns the active pane's current_path for the named
// session in a single tmux read. It runs "display-message -p -t <session> -F
// '#{pane_current_path}'" which, with no pane target, resolves against the
// session's active pane — so exactly one value is returned without enumerating
// panes (no list-panes, no -a).
//
// This is the lazy-fallback primitive consumed by the grouped render's
// directory resolver for sessions that carry no @portal-dir stamp. A
// "no such session" failure (session killed mid-resolve) is classified at this
// boundary so callers can discriminate it via errors.Is(err, ErrNoSuchSession)
// and treat it as a non-fatal unresolvable result rather than aborting the
// render. The original *CommandError remains recoverable via errors.As.
func (c *Client) ActivePaneCurrentPath(session string) (string, error) {
	output, err := c.cmd.Run("display-message", "-p", "-t", session, "-F", "#{pane_current_path}")
	if err != nil {
		return "", fmt.Errorf("failed to read active pane current path for session %q: %w", session, wrapNoSuchSession(err))
	}
	return output, nil
}

// KillSession kills the tmux session with the given name.
//
// The "=" exact-match prefix (via exactTarget) forces tmux's exact-match target
// resolution rather than the default prefix match — uniform with HasSession /
// SwitchClient — so this destructive kill never silently prefix-matches a
// colliding session (killing "foo" when only a live "foo-2" exists must NOT
// destroy "foo-2"). The kill path is destructive, has no undo, and is silent
// on a wrong-session kill, so the prefix is load-bearing here.
//
// This Client-method chokepoint fix covers every caller with no caller-side
// change — including the internal _portal-saver callers (cmd/state_cleanup.go,
// internal/tmux/portal_saver.go), which gain the prefix harmlessly (fixed
// literal name, no possible prefix collision). See spec § Required Behaviour &
// The Fix.
func (c *Client) KillSession(name string) error {
	_, err := c.cmd.Run("kill-session", "-t", exactTarget(name))
	if err != nil {
		return fmt.Errorf("failed to kill tmux session %q: %w", name, err)
	}
	return nil
}

// RenameSession renames a tmux session from oldName to newName.
//
// The "=" exact-match prefix (via exactTarget) forces tmux's exact-match target
// resolution on oldName rather than the default prefix match — uniform with
// HasSession / SwitchClient / KillSession — so a rename never silently
// prefix-matches a colliding session (renaming "foo-2" when only a live "foo-2"
// exists and "foo" is targeted). Session names are {project}-{nanoid} and freely
// renamed by the user, so the live-collision exposure is real; the rename path
// is recoverable (unlike kill) but still incorrect without the prefix.
//
// Implementer trap: the "=" prefix goes on the TARGET ONLY. newName is the
// literal positional new-name argument and must stay bare — prefixing it would
// literally name the session "=...".
//
// This Client-method chokepoint fix covers every caller with no caller-side
// change. See spec § Required Behaviour & The Fix.
func (c *Client) RenameSession(oldName, newName string) error {
	_, err := c.cmd.Run("rename-session", "-t", exactTarget(oldName), newName)
	if err != nil {
		return fmt.Errorf("failed to rename tmux session %q to %q: %w", oldName, newName, err)
	}
	return nil
}

// SwitchClient switches the current tmux client to the named session.
// Used when Portal is running inside an existing tmux session.
//
// The "=" prefix forces tmux's exact-match target resolution — uniform with
// HasSession / SelectWindow / SelectPane / attach-session so a prefix-
// collision (killed "foo" coexisting with live "foo-2") never silently
// resolves to the wrong session. See spec § Pre-select + attach sequence >
// Exact-match target syntax.
func (c *Client) SwitchClient(name string) error {
	_, err := c.cmd.Run("switch-client", "-t", "="+name)
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
//
// On success it returns the trimmed value. On failure the underlying error
// is unwrapped via errors.As to a *CommandError and its stderr is matched
// against optionAbsentStderrPatterns (case-sensitive substrings: "invalid
// option:", "unknown option:", "ambiguous option:"): a match returns
// ErrOptionNotFound; everything else (transport faults, exec lookup failures,
// unrecognised stderr phrasings) propagates the original wrapped error so
// callers can recover the *CommandError via errors.As and distinguish absence
// from a real tmux failure.
func (c *Client) GetServerOption(name string) (string, error) {
	output, err := c.cmd.Run("show-option", "-sv", name)
	if err == nil {
		return strings.TrimSpace(output), nil
	}
	var cmdErr *CommandError
	if errors.As(err, &cmdErr) {
		for _, pat := range optionAbsentStderrPatterns {
			if strings.Contains(cmdErr.Stderr, pat) {
				return "", ErrOptionNotFound
			}
		}
	}
	return "", err
}

// TryGetServerOption returns the value of a tmux server-level option along
// with a found flag, distinguishing genuine absence from real tmux failures:
//
//   - Option present  → (value, true, nil)
//   - Option absent   → ("", false, nil), recognised via
//     errors.Is(err, ErrOptionNotFound) on the underlying GetServerOption call
//     (i.e. tmux's stderr matched the option-absent pattern family).
//   - Any other error → ("", false, non-nil-err) — a transport or
//     environmental failure (socket connect, lost server, exec lookup, etc.).
//     The wrapped *CommandError is recoverable via errors.As(err, &cmdErr)
//     so callers can inspect tmux's stderr without coupling to os/exec.
//
// Callers that need to treat "not found" as a normal control-flow case should
// prefer this over GetServerOption.
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

// ShowAllServerOptions returns the raw output of "tmux show-options -s". The
// output is one option per line in the form `@name "value"` (or `@name value`
// for unquoted scalars). Callers parse it themselves; this method exists so
// the daemon can dump every server option in a single tmux invocation rather
// than calling GetServerOption per pane during marker enumeration.
//
// Implementation note: `-s` emits `name value` pairs; `-sv` would emit values
// only, which would defeat the marker-name-based parsing in
// state.ListSkeletonMarkers. The flag combination here is load-bearing.
func (c *Client) ShowAllServerOptions() (string, error) {
	out, err := c.cmd.Run("show-options", "-s")
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

// PaneTarget formats a tmux pane target string in the canonical
// "session:window.pane" form accepted by tmux's `-t` flag (e.g.
// "my-project:0.1"). It is the single canonical formatter for this target;
// callers must not hand-roll the equivalent fmt.Sprintf so the format stays
// uniform across the codebase.
//
// This format is dual-purpose: it doubles as the canonical hooks.json key
// for per-pane on-resume hooks (see internal/restore/session.go
// collectArmInfos). Callers issuing a -t flag against tmux MUST instead use
// PaneTargetExact, which prepends tmux's exact-match prefix "=" to the
// session segment. PaneTarget intentionally does NOT carry the prefix so
// the hook-key format stays stable across releases — changing it would
// silently invalidate every entry in hooks.json.
func PaneTarget(session string, window, pane int) string {
	return fmt.Sprintf("%s:%d.%d", session, window, pane)
}

// PaneTargetExact formats a tmux pane target string with the "=" exact-match
// prefix on the session segment (e.g. "=my-project:0.1"). Used by callers
// issuing a `-t` flag at the pane level.
//
// The "=" prefix forces tmux's exact-match target resolution rather than the
// default prefix match. Without it, a killed session "foo" coexisting with a
// live "foo-2" would silently resolve `-t foo:0.0` to "foo-2:0.0", attaching
// or operating on the wrong session. See spec § Pre-select + attach sequence
// > Exact-match target syntax.
//
// PaneTarget (no prefix) remains the canonical hook-key formatter; do not
// mix the two — hook lookups against an "=" -prefixed key would miss.
func PaneTargetExact(session string, window, pane int) string {
	return fmt.Sprintf("=%s:%d.%d", session, window, pane)
}

// exactTarget formats a tmux session target string with the "=" exact-match
// prefix (e.g. "=my-session"). It is the session-level sibling of
// PaneTargetExact (pane-level): together they are the two canonical ways to
// build an exact-match `-t` target in this package.
//
// The "=" prefix forces tmux's exact-match target resolution rather than the
// default prefix match. Without it, a killed session "foo" coexisting with a
// live "foo-2" would silently resolve `-t foo` to "foo-2", operating on the
// wrong session — and for the destructive kill path that means destroying the
// wrong session with no error.
//
// Callers issuing a session-level `-t` flag MUST route through this helper; no
// inline "="+name session target should remain anywhere in the internal/tmux
// package. See spec § Required Behaviour & The Fix > Introduce the exactTarget
// session-level primitive.
func exactTarget(session string) string {
	return "=" + session
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

// WindowGroup is a window's enumeration shape: its raw tmux window_index, its
// window name (#W), and the raw pane_index values of every pane in the window
// in ascending order. Returned by ListWindowsAndPanesInSession to give callers
// (notably the preview chrome) everything they need for "Window M of N" /
// "Pane X of Y" / window-name labels and for state.SanitizePaneKey lookups, in
// a single read-only tmux call.
type WindowGroup struct {
	WindowIndex int
	WindowName  string
	PaneIndices []int
}

// listWindowsAndPanesFieldSep is the field separator embedded in the tmux -F
// format string for ListWindowsAndPanesInSession. We deliberately use ASCII
// 0x1f (Unit Separator) rather than '|' so window names containing pipes
// (which tmux permits) round-trip without ambiguity. The character is non-
// printable, so it cannot collide with anything tmux emits in window names.
const listWindowsAndPanesFieldSep = "\x1f"

// ListWindowsAndPanesInSession enumerates every pane in the named session and
// returns one WindowGroup per window, sorted by raw window_index ascending,
// with each group's PaneIndices sorted by raw pane_index ascending. The first
// occurrence of a given window_index supplies that group's WindowName; later
// rows for the same window only contribute pane indices.
//
// Indices are returned verbatim — under base-index 1 / pane-base-index 1 the
// values are 1-based; non-contiguous window_index values (after window kills)
// are preserved without gap-padding. Callers that need 1-based ordinal counters
// for chrome ("Window M of N") derive them from slice position, not from
// WindowIndex.
//
// Field separator: ASCII 0x1f (Unit Separator) — non-printable, chosen so
// window names containing '|' round-trip intact. See
// listWindowsAndPanesFieldSep.
func (c *Client) ListWindowsAndPanesInSession(session string) ([]WindowGroup, error) {
	format := "#{window_index}" + listWindowsAndPanesFieldSep +
		"#{window_name}" + listWindowsAndPanesFieldSep +
		"#{pane_index}"
	out, err := c.cmd.Run("list-panes", "-s", "-t", session, "-F", format)
	if err != nil {
		return nil, fmt.Errorf("list windows and panes for session %s: %w", session, err)
	}
	if strings.TrimSpace(out) == "" {
		return []WindowGroup{}, nil
	}

	// Group rows by window_index, preserving first-seen window name and
	// accumulating pane indices in encounter order. byIndex tracks position
	// inside the result slice so we don't have to re-scan to append.
	groups := make([]WindowGroup, 0)
	byIndex := make(map[int]int)
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, listWindowsAndPanesFieldSep, 3)
		if len(parts) != 3 {
			return nil, fmt.Errorf("unexpected window/pane format %q", line)
		}
		win, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, fmt.Errorf("invalid window index %q in line %q: %w", parts[0], line, err)
		}
		pane, err := strconv.Atoi(parts[2])
		if err != nil {
			return nil, fmt.Errorf("invalid pane index %q in line %q: %w", parts[2], line, err)
		}
		if pos, ok := byIndex[win]; ok {
			groups[pos].PaneIndices = append(groups[pos].PaneIndices, pane)
			continue
		}
		byIndex[win] = len(groups)
		groups = append(groups, WindowGroup{
			WindowIndex: win,
			WindowName:  parts[1],
			PaneIndices: []int{pane},
		})
	}

	sort.Slice(groups, func(i, j int) bool {
		return groups[i].WindowIndex < groups[j].WindowIndex
	})
	for i := range groups {
		sort.Ints(groups[i].PaneIndices)
	}
	return groups, nil
}

// ListPanes returns the structural keys for panes belonging to the named tmux session.
// Each key has the form "session_name:window_index.pane_index" (e.g. "my-project:0.0").
// Structural keys survive tmux server restarts (unlike ephemeral pane IDs).
func (c *Client) ListPanes(sessionName string) ([]string, error) {
	output, err := c.cmd.Run("list-panes", "-t", sessionName, "-F", StructuralKeyFormat)
	if err != nil {
		return nil, fmt.Errorf("failed to list panes for session %q: %w", sessionName, err)
	}
	return parsePaneOutput(output), nil
}

// ListAllPanesWithFormat runs "list-panes -a -F <format>" and returns the raw,
// untrimmed tmux output. Use this when a non-default tmux format string is
// required and caller-side parsing is acceptable; ListAllPanes is the
// convenience wrapper for the canonical structural-key format. Both helpers
// share the same error-propagating contract — a non-nil error indicates a tmux
// failure (transport error, exit ≠ 0, server gone), wrapped so callers can use
// errors.Is / errors.As against any sentinel in the chain.
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
		// Classify "no such session" at this boundary so daemon-layer
		// callers can discriminate natural session churn via
		// errors.Is(err, ErrNoSuchSession). Wrap BEFORE the outer
		// fmt.Errorf so the sentinel remains reachable through the
		// chain; errors.As(err, &*CommandError) also still succeeds.
		return "", fmt.Errorf("failed to show environment for session %q: %w", session, wrapNoSuchSession(err))
	}
	return out, nil
}

// StructuralKeyFormat is the canonical tmux format string that yields a pane's
// structural key (e.g. "my-project:0.1") — the load-bearing join key between
// live-pane enumeration (list-panes / display-message), persisted hook entries
// in hooks.json, and @portal-skeleton-* marker names. Every tmux call whose
// output is consumed as a structural key MUST request exactly this format so
// the two cleanup paths (stale-marker cleanup and orphan-FIFO sweep) and the
// hook lookup table all agree on what constitutes a paneKey. Drift here would
// silently desync the cleanup paths' interpretation of "what is a paneKey".
const StructuralKeyFormat = "#{session_name}:#{window_index}.#{pane_index}"

// ListAllPanes enumerates every live pane across every tmux session and returns
// the canonical structural key for each one. Keys have the form
// "session_name:window_index.pane_index" (e.g. "my-project:0.0") — the same
// format produced by (*Client).ResolveStructuralKey and used as the lookup key
// in hooks.json, so callers can intersect the returned slice with persisted
// hook entries directly.
//
// The implementation delegates to the error-propagating ListAllPanesWithFormat
// helper. On any tmux failure (transport error, exit ≠ 0, server gone) it
// returns (nil, err) with the underlying error wrapped — callers can use
// errors.Is / errors.As against any sentinel in the chain. On success it
// returns the parsed structural-key slice.
//
// This helper deliberately does not paper over failure modes: policy for an
// empty result vs. a tmux error is the caller's decision. Treating a tmux
// failure as "no live panes" silently elides every entry that depends on the
// live set (notably hooks.json), so the discriminating contract is load-
// bearing.
func (c *Client) ListAllPanes() ([]string, error) {
	raw, err := c.ListAllPanesWithFormat(StructuralKeyFormat)
	if err != nil {
		return nil, err
	}
	return parsePaneOutput(raw), nil
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

// RespawnPane replaces the running process in the specified pane with a fresh
// shell-command via "tmux respawn-pane -k -t <target> <command>". The -k flag
// is load-bearing: it kills any existing process in the pane atomically rather
// than failing because the pane is already occupied. Used by the restore arm
// phase to swap the default shell (created by new-session / split-window) for
// the hydrate helper as a single atomic tmux call — closer to the spec's
// "helper as initial process" invariant than send-keys (which would let the
// default shell briefly run before the helper takes over).
func (c *Client) RespawnPane(target, command string) error {
	_, err := c.cmd.Run("respawn-pane", "-k", "-t", target, command)
	if err != nil {
		return fmt.Errorf("failed to respawn-pane %q: %w", target, err)
	}
	return nil
}

// UnsetServerOption removes a tmux server-level option via "set-option -su".
// The -s flag targets the server-option scope; -u removes (unsets) the option.
// This is a no-op when the option is already absent — tmux does not error in
// that case. Pairs symmetrically with SetServerOption.
func (c *Client) UnsetServerOption(name string) error {
	_, err := c.cmd.Run("set-option", "-su", name)
	if err != nil {
		return fmt.Errorf("failed to unset server option %s: %w", name, err)
	}
	return nil
}

// ShowGlobalHooksForEvent returns the raw output of "tmux show-hooks -g <event>",
// a per-event read of the global hook scope. tmux 3.6b's no-arg show-hooks -g
// does not enumerate an entire class of events (pane-* and the geometry/rename
// window-* events), so callers that need those hooks must query each event by
// name. The output is byte-identical in shape to the no-arg global form, so
// ParseShowHooks consumes it unchanged. An event with zero entries yields the
// empty string and a nil error.
func (c *Client) ShowGlobalHooksForEvent(event string) (string, error) {
	output, err := c.cmd.Run("show-hooks", "-g", event)
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

// SelectWindow sets the active window within the named session via
// "tmux select-window -t =<session>:<window>". The "=" prefix forces tmux's
// exact-match target resolution — uniform with HasSession / SelectPane /
// SwitchClient so prefix-collision (killed "foo" coexisting with live
// "foo-2") never silently lands on the wrong session. See spec § Pre-select
// + attach sequence > Exact-match target syntax.
//
// Caller semantics (best-effort vs escalate) live with the caller, not here.
// SelectWindow itself returns the wrapped error on non-zero exit (window or
// session absent) and lets the caller decide whether to log-and-swallow.
//
// Error context uses the bare (non-prefixed) "<session>:<window>" form so
// logs and error messages stay readable; the prefix is a tmux-resolution
// artefact, not a user-facing identifier.
func (c *Client) SelectWindow(session string, window int) error {
	bareTarget := fmt.Sprintf("%s:%d", session, window)
	target := "=" + bareTarget
	args := []string{"select-window", "-t", target}
	_, err := c.cmd.Run(args...)
	if err != nil {
		return fmt.Errorf("failed to select-window %s: %w", bareTarget, err)
	}
	return nil
}

// SelectPane sets the active pane within the named window via
// "tmux select-pane -t =<session>:<window>.<pane>". The pane index is the
// live index after restoration (caller is responsible for translating saved →
// live).
//
// The "=" prefix forces tmux's exact-match target resolution — uniform with
// HasSession / SelectWindow / SwitchClient. See spec § Pre-select + attach
// sequence > Exact-match target syntax. Error context uses the bare
// PaneTarget form for readability.
func (c *Client) SelectPane(session string, window, pane int) error {
	bareTarget := PaneTarget(session, window, pane)
	target := PaneTargetExact(session, window, pane)
	args := []string{"select-pane", "-t", target}
	_, err := c.cmd.Run(args...)
	if err != nil {
		return fmt.Errorf("failed to select-pane %s: %w", bareTarget, err)
	}
	return nil
}

// ResizePaneZoom toggles zoom on the named pane via "tmux resize-pane -Z -t
// =<session>:<window>.<pane>". Zoom is a toggle; callers must apply it only
// when the saved state indicated a zoomed window and the layout has just been
// freshly applied (which leaves zoom off), to land on the correct final state.
//
// The "=" prefix forces tmux's exact-match target resolution, uniform with
// the rest of the Client's -t-bearing call sites. See spec § Pre-select +
// attach sequence > Exact-match target syntax.
func (c *Client) ResizePaneZoom(session string, window, pane int) error {
	bareTarget := PaneTarget(session, window, pane)
	target := PaneTargetExact(session, window, pane)
	args := []string{"resize-pane", "-Z", "-t", target}
	_, err := c.cmd.Run(args...)
	if err != nil {
		return fmt.Errorf("failed to resize-pane -Z %s: %w", bareTarget, err)
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
