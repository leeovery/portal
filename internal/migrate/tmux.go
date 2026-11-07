package migrate

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// TmuxServer is the tmux version's server, as migrate stops Portal's part in
// it: its global hooks and its saver session. tmux and its sessions are
// otherwise left running.
type TmuxServer interface {
	// Answering reports whether a tmux server answers.
	Answering(ctx context.Context) bool
	// RemoveHooks unsets every Portal hook on the events the tmux version
	// registered them on, leaving every other hook, and reports how many it
	// unset.
	RemoveHooks(ctx context.Context) (int, error)
	// KillSaver kills the tmux version's saver session, reporting whether
	// there was one to kill.
	KillSaver(ctx context.Context) (bool, error)
}

// The tmux version's hooks and saver, as it last registered and named them.
// They describe the installed tmux version, which no longer changes, so they
// are copied from it as fixed values rather than shared with anything.
var (
	// managedEvents are the events the tmux version registered a hook on,
	// each read on its own: tmux's whole-scope show-hooks -g doesn't list the
	// pane-* and geometry window-* events at all.
	managedEvents = []string{
		"session-created",
		"session-closed",
		"session-renamed",
		"window-linked",
		"window-unlinked",
		"window-layout-changed",
		"pane-focus-out",
		"client-attached",
		"client-session-changed",
	}
	// fingerprints tell Portal's hooks from the user's and other tools': a
	// hook whose command holds one is Portal's. migrate-rename's is one only
	// older tmux versions registered, which the tmux version's own teardown
	// still removed.
	fingerprints = []string{
		"portal state notify",
		"portal state commit-now",
		"portal state signal-hydrate",
		"portal state migrate-rename",
	}
)

// saverTarget is the tmux version's saver session as an exact target: a bare
// name is prefix-matched by tmux, and a bare =name read as a window's.
const saverTarget = "=_portal-saver:"

// tmuxCeiling bounds each run of the tmux binary.
const tmuxCeiling = 5 * time.Second

// hookLine is one line of show-hooks -g: an event, the index of its hook and
// the hook's command, in either of tmux's two forms.
var hookLine = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9-]*)\[(\d+)\](?:\s*=>\s*|\s+)(.*)$`)

// Tmux is the tmux version's server, reached through the tmux binary on PATH.
type Tmux struct {
	// Socket is the server's socket, handed to tmux with -S. Empty runs tmux
	// plain, which reaches the server $TMUX names, as inside one of its
	// sessions, and otherwise the default socket's.
	Socket string
}

var _ TmuxServer = Tmux{}

// Answering reports whether a server answers, one holding no session
// included.
func (t Tmux) Answering(ctx context.Context) bool {
	_, err := t.run(ctx, "info")
	return err == nil
}

// RemoveHooks unsets Portal's hooks on each event, by index: tmux's hook
// arrays are sparse, so an unset moves no other hook. It goes on past a
// failure, joining every one it meets.
func (t Tmux) RemoveHooks(ctx context.Context) (int, error) {
	unset := 0
	var errs []error
	for _, event := range managedEvents {
		listed, err := t.run(ctx, "show-hooks", "-g", event)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for _, index := range portalHooks(listed, event) {
			if _, err := t.run(ctx, "set-hook", "-gu", fmt.Sprintf("%s[%d]", event, index)); err != nil {
				errs = append(errs, err)
				continue
			}
			unset++
		}
	}
	return unset, errors.Join(errs...)
}

// KillSaver kills the saver session. A session gone by the time it is
// killed was killed all the same.
func (t Tmux) KillSaver(ctx context.Context) (bool, error) {
	if present, err := t.saverPresent(ctx); err != nil || !present {
		return false, err
	}
	if _, err := t.run(ctx, "kill-session", "-t", saverTarget); err != nil {
		if present, checkErr := t.saverPresent(ctx); present || checkErr != nil {
			return false, err
		}
	}
	return true, nil
}

// saverPresent reports whether the saver session is there: tmux's refusal of
// the target is its absence, and any other failure an error.
func (t Tmux) saverPresent(ctx context.Context) (bool, error) {
	_, err := t.run(ctx, "has-session", "-t", saverTarget)
	if _, refused := errors.AsType[*exec.ExitError](err); refused {
		return false, nil
	}
	return err == nil, err
}

// run runs tmux with args against the server, within tmuxCeiling, returning
// its output, or an error carrying what it said on stderr.
func (t Tmux) run(ctx context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, tmuxCeiling)
	defer cancel()
	if t.Socket != "" {
		args = append([]string{"-S", t.Socket}, args...)
	}
	out, err := exec.CommandContext(ctx, "tmux", args...).Output()
	if err != nil {
		said := ""
		if exit, ok := errors.AsType[*exec.ExitError](err); ok && len(exit.Stderr) > 0 {
			said = ": " + strings.TrimSpace(string(exit.Stderr))
		}
		return "", fmt.Errorf("tmux %s: %w%s", strings.Join(args, " "), err, said)
	}
	return string(out), nil
}

// portalHooks is the index of each of Portal's hooks on event that listed,
// show-hooks -g's answer for event, holds.
func portalHooks(listed, event string) []int {
	var indices []int
	for line := range strings.Lines(listed) {
		match := hookLine.FindStringSubmatch(strings.TrimSpace(line))
		if match == nil || match[1] != event || !isPortals(match[3]) {
			continue
		}
		if index, err := strconv.Atoi(match[2]); err == nil {
			indices = append(indices, index)
		}
	}
	return indices
}

// isPortals reports whether a hook's command is Portal's.
func isPortals(command string) bool {
	return slices.ContainsFunc(fingerprints, func(fingerprint string) bool { return strings.Contains(command, fingerprint) })
}
