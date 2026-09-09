package tmux

import (
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/leeovery/portal/internal/state"
)

var ErrOptionNotFound = errors.New("option not found")

// Matched case-sensitively against tmux stderr to tell a genuinely absent
// option from a transport fault.
var optionAbsentStderrPatterns = []string{
	"invalid option:",
	"unknown option:",
	"ambiguous option:",
}

type Session struct {
	Name     string
	Windows  int
	Attached bool
	// Dir is the session's stamped @portal-dir user-option, empty when the
	// session carries no stamp (e.g. one restored after a reboot).
	Dir string
}

// Commander executes tmux commands. Run trims surrounding whitespace from the
// output; RunRaw returns it verbatim, for callers to whom trailing whitespace and
// ANSI escapes are content. Implementations must return non-nil errors as
// *CommandError so callers can recover the child's stderr via errors.As.
type Commander interface {
	Run(args ...string) (string, error)
	RunRaw(args ...string) (string, error)
}

type RealCommander struct{}

func (r *RealCommander) Run(args ...string) (string, error) {
	return runCommand("tmux", true, args...)
}

func (r *RealCommander) RunRaw(args ...string) (string, error) {
	return runCommand("tmux", false, args...)
}

func runCommand(binary string, trim bool, args ...string) (string, error) {
	cmd := exec.Command(binary, args...)
	// cmd.Stderr left nil — see WrapCommandError's precondition.
	out, err := cmd.Output()
	if err != nil {
		return "", WrapCommandError(err, args...)
	}
	if trim {
		return strings.TrimSpace(string(out)), nil
	}
	return string(out), nil
}

type Client struct {
	cmd Commander
}

func NewClient(cmd Commander) *Client {
	return &Client{cmd: cmd}
}

// DefaultClient shells out to the real tmux binary, so it honours the ambient
// TMUX socket — a test reaching it lands on the developer's own server.
func DefaultClient() *Client {
	return NewClient(&RealCommander{})
}

// ServerRunning reports whether a tmux server is running, including one hosting
// zero sessions.
func (c *Client) ServerRunning() bool {
	_, err := c.cmd.Run("info")
	return err == nil
}

// HasSession is false both for an absent session and for no running server.
func (c *Client) HasSession(name string) bool {
	_, err := c.cmd.Run("has-session", "-t", string(CoordTargetExact(name)))
	return err == nil
}

// HasSessionProbe is the discriminating variant of HasSession: a non-zero tmux
// exit returns (false, err) — the session is genuinely absent — while an OS-layer
// fault returns (true, err), so a caller proceeds as if present and logs rather
// than reporting the session missing.
func (c *Client) HasSessionProbe(name string) (bool, error) {
	_, err := c.cmd.Run("has-session", "-t", string(CoordTargetExact(name)))
	if err == nil {
		return true, nil
	}
	classified := wrapSessionTargetErr(name, err)
	if _, ok := errors.AsType[*exec.ExitError](err); ok {
		return false, classified
	}
	return true, classified
}

// NewSession creates a detached session; shellCommand becomes the pane's
// command when non-empty.
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

// ListSessions returns the running tmux sessions, excluding Portal's own
// underscore-prefixed internal ones. No running server yields an empty slice and
// a nil error.
func (c *Client) ListSessions() ([]Session, error) {
	// @portal-dir must stay the LAST field: a directory path may contain a
	// literal '|', so it needs the unbounded trailing SplitN slot.
	output, err := c.cmd.Run("list-sessions", "-F", "#{session_name}|#{session_windows}|#{session_attached}|#{@portal-dir}")
	if err != nil {
		// Swallowed deliberately: the error is the no-server signal.
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
			Dir:      parts[3],
		})
	}

	// Portal-wide invariant: an underscore-prefixed session must never leak into
	// user-visible output.
	filtered := make([]Session, 0, len(sessions))
	for _, s := range sessions {
		if strings.HasPrefix(s.Name, "_") {
			continue
		}
		filtered = append(filtered, s)
	}
	return filtered, nil
}

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

// StartServer starts the tmux server by creating a detached session named
// PortalBootstrapName. That session is not incidental: tmux's default
// "exit-empty on" would tear the server down before restore reconstructs the
// saved sessions, so the server must own one from the moment it comes up.
func (c *Client) StartServer() error {
	_, err := c.cmd.Run("new-session", "-d", "-s", PortalBootstrapName)
	if err != nil {
		return fmt.Errorf("failed to start tmux server (bootstrap session): %w", err)
	}
	return nil
}

// EnsureServer starts a tmux server if none is running. The bool reports
// whether a start was attempted, so a failed attempt returns (true, err).
func (c *Client) EnsureServer() (bool, error) {
	if c.ServerRunning() {
		return false, nil
	}
	if err := c.StartServer(); err != nil {
		return true, err
	}
	return true, nil
}

func (c *Client) CurrentSessionName() (string, error) {
	output, err := c.cmd.Run("display-message", "-p", "#{session_name}")
	if err != nil {
		return "", fmt.Errorf("failed to get current session name: %w", err)
	}
	return output, nil
}

// ResolveHookKey resolves a pane target to its hook key — its durable pane
// token — in two live reads. The first asks only whether any pane answers to
// the target, and a target none answers to fails here with tmux's own words;
// the second reads the token, which is empty for a live pane that has never
// been stamped. An empty key is therefore a resolved pane with no hook identity
// yet, never a pane that is gone.
func (c *Client) ResolveHookKey(paneID Target) (string, error) {
	// The probe must name no option: show-options rejects an option the pane
	// does not carry with the same exit status it rejects a missing pane, so
	// naming one makes an un-stamped pane indistinguishable from a gone one.
	if _, err := c.cmd.Run("show-options", "-p", "-t", string(paneID)); err != nil {
		return "", fmt.Errorf("no pane answers to %q: %w", paneID, err)
	}

	output, err := c.cmd.Run("display-message", "-p", "-t", string(paneID), HookKeyFormat)
	if err != nil {
		return "", fmt.Errorf("failed to resolve hook key for pane %q: %w", paneID, err)
	}
	return output, nil
}

// ActivePaneCurrentPath returns the named session's active pane current_path in
// a single tmux read.
//
// A session no pane answers to returns ("", nil), NOT an error: tmux answers an
// unmatched display-message target with an empty expansion at exit 0 and no
// stderr (measured on 3.7c; the same fact CoordTargetExact's doc states from
// the target-form side), so there is no "no such session" stderr to classify and
// no ErrNoSuchSession this path can ever produce. A live session whose pane has
// no readable path yet returns the same pair, which is why the empty path — not
// an error — is the unresolvable signal callers must handle. internal/session's
// ResolveSessionDir reports it as ok==false with a nil error so the grouped
// render carries on.
//
// A non-nil error is therefore a genuine display-message failure — no server to
// connect to, say — wrapped with the session name and preserving the underlying
// *CommandError for a caller that wants tmux's own words.
func (c *Client) ActivePaneCurrentPath(session string) (string, error) {
	output, err := c.cmd.Run("display-message", "-p", "-t", string(CoordTargetExact(session)), "-F", "#{pane_current_path}")
	if err != nil {
		return "", fmt.Errorf("failed to read active pane current path for session %q: %w", session, err)
	}
	return output, nil
}

func (c *Client) KillSession(name string) error {
	_, err := c.cmd.Run("kill-session", "-t", string(CoordTargetExact(name)))
	if err != nil {
		return fmt.Errorf("failed to kill tmux session %q: %w", name, wrapSessionTargetErr(name, err))
	}
	return nil
}

// The exact-match prefix goes on the target only: newName is a literal
// positional argument, and prefixing it would name the session "=...".
func (c *Client) RenameSession(oldName, newName string) error {
	// Refused before the argv is composed: the new name goes on as a bare
	// positional, and one Portal's target form cannot address afterwards is a
	// session it can no longer read.
	if err := ValidateSessionName(newName); err != nil {
		return fmt.Errorf("failed to rename tmux session %q to %q: %w", oldName, newName, err)
	}
	_, err := c.cmd.Run("rename-session", "-t", string(CoordTargetExact(oldName)), newName)
	if err != nil {
		return fmt.Errorf("failed to rename tmux session %q to %q: %w", oldName, newName, wrapSessionTargetErr(oldName, err))
	}
	return nil
}

func (c *Client) SwitchClient(name string) error {
	_, err := c.cmd.Run("switch-client", "-t", string(CoordTargetExact(name)))
	if err != nil {
		return fmt.Errorf("failed to switch to session %q: %w", name, wrapSessionTargetErr(name, err))
	}
	return nil
}

func (c *Client) SetServerOption(name, value string) error {
	_, err := c.cmd.Run("set-option", "-s", name, value)
	if err != nil {
		return fmt.Errorf("failed to set server option %q: %w", name, err)
	}
	return nil
}

// SetPaneOption sets a tmux option scoped to one pane. The option name comes
// from the caller, and a target naming no live pane fails here rather than
// silently succeeding.
func (c *Client) SetPaneOption(target Target, name, value string) error {
	_, err := c.cmd.Run("set-option", "-p", "-t", string(target), name, value)
	if err != nil {
		return fmt.Errorf("failed to set pane option %s on %s: %w", name, target, err)
	}
	return nil
}

// SetSessionOption sets a tmux option scoped to one session. Callers must not
// route global (-g) options through it: the -t scoping is what keeps the write
// out of the global namespace.
func (c *Client) SetSessionOption(session, name, value string) error {
	_, err := c.cmd.Run("set-option", "-t", string(CoordTargetExact(session)), name, value)
	if err != nil {
		return fmt.Errorf("failed to set session option %s on %s: %w", name, session, err)
	}
	return nil
}

// NewDetachedSessionNoCwd omits -c, so the pane inherits tmux's default working
// directory; shellCommand applies when non-empty.
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

// GetServerOption returns the trimmed value of a tmux server-level option. A
// stderr phrasing meaning the option does not exist returns ErrOptionNotFound;
// every other failure propagates wrapped, so absence stays distinguishable from
// a real tmux fault.
func (c *Client) GetServerOption(name string) (string, error) {
	output, err := c.cmd.Run("show-option", "-sv", name)
	if err == nil {
		return strings.TrimSpace(output), nil
	}
	if cmdErr, ok := errors.AsType[*CommandError](err); ok {
		for _, pat := range optionAbsentStderrPatterns {
			if strings.Contains(cmdErr.Stderr, pat) {
				return "", ErrOptionNotFound
			}
		}
	}
	return "", err
}

// TryGetServerOption is the GetServerOption variant for callers to whom an
// absent option is normal control flow: absence returns ("", false, nil), and a
// non-nil error means a real tmux failure.
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

// ShowAllServerOptions returns every server option in one read, raw: one per
// row as `@name "value"` (or unquoted, for a scalar), for the caller to parse.
// `-s` is load-bearing — `-sv` emits values without their names, which defeats
// any name-based parsing of this output.
func (c *Client) ShowAllServerOptions() (string, error) {
	out, err := c.cmd.Run("show-options", "-s")
	if err != nil {
		return "", fmt.Errorf("failed to show server options: %w", err)
	}
	return out, nil
}

// Target is a composed tmux `-t` argument. The exactness vocabulary below is
// the route that pins one, and a Target is spent back as a string at the argv
// that consumes it — inside the client on its way to the commander, or in a
// tmux command line the client never runs.
type Target string

type PaneCoord struct {
	Window int
	Pane   int
}

// PaneTarget formats a plain "session:window.pane" target, for display, for
// name-based keys, and for a `-t` flag whose session is known to be live under
// that exact name. Use PaneTargetExact when the session may have been renamed
// away or destroyed: tmux prefix-matches, so `-t foo` silently resolves to a
// live "foo-2" once "foo" is gone. The "=" prefix pins the session name only;
// it makes no difference to the window.pane half, so a coordinate that has
// renumbered onto a different pane is caught by neither form.
func PaneTarget(session string, window, pane int) string {
	return fmt.Sprintf("%s:%d.%d", session, window, pane)
}

// PaneTargetExact is the coordinate-bearing sibling of CoordTargetExact: the
// "=" exact-match prefix a `-t` needs when its session may be gone, with the
// window and pane spelled out. See PaneTarget.
func PaneTargetExact(session string, window, pane int) Target {
	return Target(fmt.Sprintf("=%s:%d.%d", session, window, pane))
}

// SessionTargetExact pins a session name for a `-t` that tmux parses as a
// session target and nothing else. tmux otherwise prefix-matches, so `-t foo`
// silently resolves to a live "foo-2" once "foo" is gone — on the kill path,
// that destroys the wrong session with no error.
//
// Measured against tmux 3.7c with a prefix-extension sibling live, every
// per-session command Portal runs parses its target as a window or pane first:
// a bare "=foo" is read as a window name and falls through to the same fuzzy
// lookup, and a colon-free "=my.app" is split on the period into window and
// pane before the session lookup is reached at all, so any live session whose
// name extends "my" answers in its place. They all take CoordTargetExact,
// whose doc names them.
//
// It stays in the vocabulary for the command a future measurement shows takes a
// bare session target — the choice is per command and measured, never made from
// the shape of the target string. Measure before choosing.
//
// Exported alongside CoordTargetExact so a caller composing a tmux argv the
// client does not run — an exec chain, say — pins its targets through the same
// two forms rather than re-deriving the prefix.
//
// The exact forms return Target, and every `-t` parameter takes one, so a
// computed string does not type-check where a pinned target is required and
// renaming a local cannot launder one into place. Two shapes still reach a
// Target without the vocabulary — an untyped constant, which converts
// implicitly, and an explicit conversion, which converts anything — so the type
// carries the rule without closing it.
func SessionTargetExact(session string) Target {
	return Target("=" + session)
}

// CoordTargetExact pins a session name for a `-t` tmux parses as a window or
// pane target but which Portal addresses by session alone — which, measured
// command by command, is every per-session `-t` the client composes:
// has-session, kill-session, rename-session's -t, switch-client,
// show-environment, set-environment, list-clients, list-panes,
// display-message, set-option and respawn-pane, plus the hand-composed
// attach-session argvs
// in cmd and internal/session.
//
// The trailing ":" is load-bearing: it is what makes tmux read the leading
// component as a session name, so the "=" applies to it and a period in the
// name is not split into window and pane. A bare "=name" there is read as a
// window or pane name and misses: list-panes falls through to the same fuzzy
// lookup, while display-message returns empty at exit 0 for a live session as
// much as a gone one. The empty coordinate half leaves tmux's own default in
// place — the session's current window, and that window's active pane — which
// is where a bare session name resolved to already. See SessionTargetExact.
func CoordTargetExact(session string) Target {
	return Target("=" + session + ":")
}

// PaneIDTarget pins a tmux pane ID — "%7", the value tmux itself publishes as
// $TMUX_PANE — as a target. A pane ID is server-unique and is never
// prefix-matched, so it carries no "=" and needs none; the constructor exists so
// a pane ID enters the vocabulary by a named route rather than by a bare
// conversion at the call site.
func PaneIDTarget(paneID string) Target {
	return Target(paneID)
}

// windowTarget formats a plain "session:window" target, for an error message or
// for a `-t` whose session is known to be live under that exact name.
// windowTargetExact is its pinned sibling: the "=" applies to the session half
// only, so a window that has renumbered is caught by neither form.
func windowTarget(session string, window int) string {
	return fmt.Sprintf("%s:%d", session, window)
}

func windowTargetExact(session string, window int) Target {
	return Target("=" + windowTarget(session, window))
}

// ListPanesInSession returns the coords of every live pane in the session,
// across all its windows, sorted by window then pane.
func (c *Client) ListPanesInSession(session string) ([]PaneCoord, error) {
	out, err := c.cmd.Run("list-panes", "-s", "-t", string(CoordTargetExact(session)), "-F", "#{window_index}:#{pane_index}")
	if err != nil {
		return nil, fmt.Errorf("failed to list panes in session %q: %w", session, err)
	}
	if out == "" {
		return []PaneCoord{}, nil
	}

	var coords []PaneCoord
	for line := range strings.SplitSeq(out, "\n") {
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

type WindowGroup struct {
	WindowIndex int
	WindowName  string
	PaneIndices []int
}

// ASCII Unit Separator rather than '|': tmux permits pipes in window names, and
// being non-printable this cannot collide with anything tmux emits.
const listWindowsAndPanesFieldSep = "\x1f"

// ListWindowsAndPanesInSession returns one WindowGroup per window of the named
// session, sorted ascending, each group's PaneIndices likewise. Indices are
// verbatim tmux values, so they honour base-index settings and keep the gaps left
// by killed windows; a caller wanting ordinal counters must derive them from
// slice position.
func (c *Client) ListWindowsAndPanesInSession(session string) ([]WindowGroup, error) {
	format := "#{window_index}" + listWindowsAndPanesFieldSep +
		"#{window_name}" + listWindowsAndPanesFieldSep +
		"#{pane_index}"
	out, err := c.cmd.Run("list-panes", "-s", "-t", string(CoordTargetExact(session)), "-F", format)
	if err != nil {
		return nil, fmt.Errorf("list windows and panes for session %s: %w", session, err)
	}
	if strings.TrimSpace(out) == "" {
		return []WindowGroup{}, nil
	}

	groups := make([]WindowGroup, 0)
	byIndex := make(map[int]int)
	for line := range strings.SplitSeq(out, "\n") {
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

// ListAllPanesWithFormat enumerates every pane on the server under a caller-
// supplied tmux format, returning the raw untrimmed output. Any tmux failure is
// returned wrapped, never flattened into an empty result.
func (c *Client) ListAllPanesWithFormat(format string) (string, error) {
	out, err := c.cmd.Run("list-panes", "-a", "-F", format)
	if err != nil {
		return "", fmt.Errorf("failed to list panes: %w", err)
	}
	return out, nil
}

// ShowEnvironment returns the session's environment raw, for the caller to
// parse: one "NAME=value" per line, or "-NAME" for a removed entry.
func (c *Client) ShowEnvironment(session string) (string, error) {
	out, err := c.cmd.Run("show-environment", "-t", string(CoordTargetExact(session)))
	if err != nil {
		return "", fmt.Errorf("failed to show environment for session %q: %w", session, wrapSessionTargetErr(session, err))
	}
	return out, nil
}

// StructuralKeyFormat yields a pane's name-based structural key, the join key
// between live-pane enumeration and @portal-skeleton-* marker names. Every call
// whose output is read as a structural key must request exactly this format, or
// the cleanup paths disagree about what a paneKey is. It is not the hook key —
// see HookKeyFormat.
const StructuralKeyFormat = "#{session_name}:#{window_index}.#{pane_index}"

// HookKeyFormat yields a pane's hook key: the pane's durable token and nothing
// else. It carries no session component and no coordinates, so no tmux operation
// that moves a pane can recompute it. An un-stamped pane yields an empty key.
const HookKeyFormat = "#{" + state.PortalPaneIDOption + "}"

// PaneHookRow describes one live pane for the hook machinery. Token is the
// pane's @portal-pane-id, empty for a pane that carries no stamp. Location is
// the pane's "<session>:<window>.<pane>" address and is display-only — it is
// never a key.
type PaneHookRow struct {
	Token    string
	Location string
}

// paneHookRowSeparator must stay a non-whitespace character: the commander trims
// the whole output, so a blank leading token followed by whitespace would lose
// its separator on the first row.
const paneHookRowSeparator = "|"

// The location half deliberately does not reuse StructuralKeyFormat despite
// rendering the same shape: the location is display-only and never a key, so
// coupling the two would tie a display column to a key contract.
const paneHookRowFormat = HookKeyFormat + paneHookRowSeparator +
	"#{session_name}:#{window_index}.#{pane_index}"

// ListAllPaneHookKeys returns one row per live pane on the server, including
// panes carrying no token — the row count is what tells a caller the read
// succeeded, which is a different question from which panes are stamped. A tmux
// failure returns (nil, err), never an empty slice: a caller reading a failure
// as "no live panes" would orphan every hooks.json entry at once.
func (c *Client) ListAllPaneHookKeys() ([]PaneHookRow, error) {
	raw, err := c.ListAllPanesWithFormat(paneHookRowFormat)
	if err != nil {
		return nil, err
	}
	return parsePaneHookRows(raw)
}

// parsePaneHookRows splits each line at its first separator only: the location
// half carries ":" and ".", and a session name may itself carry the separator.
func parsePaneHookRows(output string) ([]PaneHookRow, error) {
	rows := []PaneHookRow{}
	for line := range strings.SplitSeq(output, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		token, location, ok := strings.Cut(line, paneHookRowSeparator)
		if !ok {
			return nil, fmt.Errorf("failed to parse pane hook row %q: no %q separator", line, paneHookRowSeparator)
		}
		rows = append(rows, PaneHookRow{Token: token, Location: location})
	}
	return rows, nil
}

// RespawnPane replaces the process running in the pane with command. The -k
// flag is load-bearing: it kills the existing process atomically, rather than
// failing because the pane is occupied, so command really is the pane's
// initial process.
func (c *Client) RespawnPane(target Target, command string) error {
	_, err := c.cmd.Run("respawn-pane", "-k", "-t", string(target), command)
	if err != nil {
		return fmt.Errorf("failed to respawn-pane %q: %w", target, err)
	}
	return nil
}

// UnsetServerOption removes a tmux server-level option. An option that is
// already absent is a no-op, not an error.
func (c *Client) UnsetServerOption(name string) error {
	_, err := c.cmd.Run("set-option", "-su", name)
	if err != nil {
		return fmt.Errorf("failed to unset server option %s: %w", name, err)
	}
	return nil
}

// ShowGlobalHooksForEvent reads one event's global hooks, raw and in the same
// shape as the whole-scope read, so ParseShowHooks consumes it unchanged. An
// event with no entries yields the empty string and a nil error. Per-event reads
// are mandatory: tmux 3.6b's no-arg show-hooks -g does not enumerate the pane-* or
// geometry window-* events at all.
func (c *Client) ShowGlobalHooksForEvent(event string) (string, error) {
	output, err := c.cmd.Run("show-hooks", "-g", event)
	if err != nil {
		return "", fmt.Errorf("failed to show global hooks: %w", err)
	}
	return output, nil
}

// AppendGlobalHook appends a command to the event's global hook array. The
// command is passed as one argv element, so its quotes and shell
// metacharacters survive verbatim.
func (c *Client) AppendGlobalHook(event, command string) error {
	_, err := c.cmd.Run("set-hook", "-ga", event, command)
	if err != nil {
		return fmt.Errorf("failed to append hook on %q: %w", event, err)
	}
	return nil
}

// CapturePane returns the pane's whole scrollback, ANSI escapes included and
// untrimmed, so a caller can hash and persist it byte-for-byte.
func (c *Client) CapturePane(target Target) (string, error) {
	out, err := c.cmd.RunRaw("capture-pane", "-e", "-p", "-S", "-", "-t", string(target))
	if err != nil {
		return "", fmt.Errorf("failed to capture pane %q: %w", target, err)
	}
	return out, nil
}

// NewSessionWithCommand applies cwd and shellCommand only when non-empty — for
// restoring saved panes that may carry no recorded cwd.
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

// NewWindow applies name, cwd and shellCommand only when non-empty.
func (c *Client) NewWindow(target Target, name, cwd, shellCommand string) error {
	args := []string{"new-window", "-t", string(target)}
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

// SplitWindow applies cwd and shellCommand only when non-empty.
func (c *Client) SplitWindow(target Target, cwd, shellCommand string) error {
	args := []string{"split-window", "-t", string(target)}
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

func (c *Client) SetSessionEnvironment(session, key, value string) error {
	_, err := c.cmd.Run("set-environment", "-t", string(CoordTargetExact(session)), key, value)
	if err != nil {
		return fmt.Errorf("failed to set env %s on %q: %w", key, session, wrapSessionTargetErr(session, err))
	}
	return nil
}

// SelectLayout applies a layout string to the named window. Order matters:
// tmux fits the panes that exist at the moment of the call, so every
// split-window for that window must already have run.
func (c *Client) SelectLayout(session string, window int, layout string) error {
	bareTarget := windowTarget(session, window)
	args := []string{"select-layout", "-t", string(windowTargetExact(session, window)), layout}
	_, err := c.cmd.Run(args...)
	if err != nil {
		return fmt.Errorf("failed to select-layout %s: %w", bareTarget, err)
	}
	return nil
}

func (c *Client) SelectWindow(session string, window int) error {
	bareTarget := windowTarget(session, window)
	args := []string{"select-window", "-t", string(windowTargetExact(session, window))}
	_, err := c.cmd.Run(args...)
	if err != nil {
		return fmt.Errorf("failed to select-window %s: %w", bareTarget, err)
	}
	return nil
}

// SelectPane sets the active pane within the named window. The pane index must
// be a live index — translating a saved index is the caller's job.
func (c *Client) SelectPane(session string, window, pane int) error {
	bareTarget := PaneTarget(session, window, pane)
	target := PaneTargetExact(session, window, pane)
	args := []string{"select-pane", "-t", string(target)}
	_, err := c.cmd.Run(args...)
	if err != nil {
		return fmt.Errorf("failed to select-pane %s: %w", bareTarget, err)
	}
	return nil
}

// ResizePaneZoom toggles zoom on the named pane. It is a toggle, not a set, so
// a caller restoring a zoomed window must know zoom is currently off — which a
// freshly applied layout guarantees.
func (c *Client) ResizePaneZoom(session string, window, pane int) error {
	bareTarget := PaneTarget(session, window, pane)
	target := PaneTargetExact(session, window, pane)
	args := []string{"resize-pane", "-Z", "-t", string(target)}
	_, err := c.cmd.Run(args...)
	if err != nil {
		return fmt.Errorf("failed to resize-pane -Z %s: %w", bareTarget, err)
	}
	return nil
}

func (c *Client) UnsetGlobalHookAt(event string, index int) error {
	_, err := c.cmd.Run("set-hook", "-gu", fmt.Sprintf("%s[%d]", event, index))
	if err != nil {
		return fmt.Errorf("failed to unset hook %s[%d]: %w", event, index, err)
	}
	return nil
}
