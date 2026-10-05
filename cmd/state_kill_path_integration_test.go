//go:build integration

package cmd_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/hooksweep"
	"github.com/leeovery/portal/internal/portalbintest"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

// firstDumpBudget bounds the daemon's first committing tick, which dumps every
// pane's scrollback: one ticker period after start-up, longer on a loaded host.
const firstDumpBudget = 15 * time.Second

// daemonExitBudget bounds the daemon's shutdown flush and exit.
const daemonExitBudget = 10 * time.Second

// killPathSession is a user session the fixture builds with one pane per
// resume-hook key, each pane stamped with its own key, and the way it is ended.
type killPathSession struct {
	name     string
	hookKeys []string
	end      func(t *testing.T, sock *tmuxtest.Socket, name string)
}

func killSession(t *testing.T, sock *tmuxtest.Socket, name string) {
	t.Helper()
	sock.Run(t, "kill-session", "-t", string(tmux.CoordTargetExact(name)))
}

// exitLastProgram answers the pane's pending read, so its only program exits
// and tmux closes the session the way a user typing `exit` would.
func exitLastProgram(t *testing.T, sock *tmuxtest.Socket, name string) {
	t.Helper()
	sock.Run(t, "send-keys", "-t", string(tmux.CoordTargetExact(name)), "Enter")
}

func TestKillPathStaysFinalUnderHardenedSavePath(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	binDir := portalbintest.StagePortalBinary(t)
	binary, err := exec.LookPath("portal")
	if err != nil {
		t.Skipf("portal not on PATH after build+prepend; skipping: %v", err)
	}

	sessions := []killPathSession{
		{name: "alpha", hookKeys: []string{hookstest.ReapableSeedA, hookstest.ReapableSeedB}, end: killSession},
		{name: "bravo", hookKeys: []string{hookstest.ReapableSeedC}, end: killSession},
		{name: "charlie", hookKeys: []string{hookstest.ReapableSeedD}, end: exitLastProgram},
	}

	fixture, hooksPath := newKillPathFixture(t, binary, binDir, sessions)
	saved := waitForDumpedScrollback(t, fixture, sessions)

	retireDaemon(t, fixture)

	for i, s := range sessions {
		survivors := sessionNamesOf(sessions[i+1:])
		s.end(t, fixture.sock, s.name)

		ctx, cancel := context.WithTimeout(context.Background(), symptomKillBudget)
		err := pollKillCommitted(ctx, fixture.stateDir, s.name, survivors, saved[s.name])
		cancel()
		if err != nil {
			t.Fatalf("kill of %s was not committed within %s: %v\n%s\n--- portal.log ---\n%s",
				s.name, symptomKillBudget, err, fixture.diagnostic(),
				portaltest.ReadPortalLogSafe(fixture.stateDir))
		}
		for _, survivor := range survivors {
			assertFilesExist(t, fixture.stateDir, saved[survivor])
		}
	}

	assertEmptyRestoreState(t, fixture)
	assertSweepReapsKilledHooks(t, fixture, hooksPath, sessions)
}

// newKillPathFixture keeps tmux alive on Portal's own _portal-bootstrap alone,
// as a user's runtime is, so the last kill leaves only Portal's sessions.
func newKillPathFixture(t *testing.T, binary, binDir string, sessions []killPathSession) (symptomFixture, string) {
	t.Helper()

	_, stateDir := portaltest.IsolateStateForTest(t)
	hooksPath := filepath.Join(stateDir, "hooks.json")

	// Set before the tmux server starts so it, and every session-closed hook
	// subprocess it runs, writes to this fixture's files.
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	t.Setenv("PORTAL_HOOKS_FILE", hooksPath)
	t.Setenv("PORTAL_PROJECTS_FILE", filepath.Join(stateDir, "projects.json"))
	t.Setenv("PORTAL_ALIASES_FILE", filepath.Join(stateDir, "aliases"))

	portaltest.RegisterStateDirTeardownGuard(t, stateDir)

	sock := tmuxtest.New(t, "ptl-killpath-")
	sock.Run(t, "new-session", "-d", "-s", tmux.PortalBootstrapName)
	sock.WaitForSession(t, tmux.PortalBootstrapName, 5*time.Second)

	entries := map[string]string{}
	for _, s := range sessions {
		for i, key := range s.hookKeys {
			paneID := newKillPathPane(t, sock, s.name, i)
			sock.StampPaneToken(t, tmux.PaneIDTarget(paneID), key)
			entries[key] = "resume " + key
		}
	}
	hookstest.StageStore(t, hookstest.Staging{Dir: stateDir, Entries: entries})

	fixture := symptomFixture{sock: sock, stateDir: stateDir, binary: binary, binDir: binDir}
	runPortalList(t, binary, fixture)

	if !rawSessionPresent(t, sock, tmux.PortalSaverName) {
		t.Fatalf("_portal-saver not present after bootstrap\n%s", fixture.diagnostic())
	}
	return fixture, hooksPath
}

// newKillPathPane returns the new pane's id. The pane waits on a read after
// printing its scrollback line, so it lives until it is killed or answered.
func newKillPathPane(t *testing.T, sock *tmuxtest.Socket, session string, i int) string {
	t.Helper()
	program := fmt.Sprintf(`printf '%%s\n' 'scrollback of %s pane %d'; read -r _`, session, i)
	var out string
	if i == 0 {
		out = sock.Run(t, "new-session", "-d", "-s", session, "-P", "-F", "#{pane_id}", "sh", "-c", program)
	} else {
		out = sock.Run(t, "split-window", "-d", "-t", string(tmux.CoordTargetExact(session)),
			"-P", "-F", "#{pane_id}", "sh", "-c", program)
	}
	return strings.TrimSpace(out)
}

// waitForDumpedScrollback waits for the daemon's first committing tick to save
// every user pane with a non-empty scrollback file, and returns each session's
// files as that index names them.
func waitForDumpedScrollback(t *testing.T, f symptomFixture, sessions []killPathSession) map[string][]string {
	t.Helper()
	var files map[string][]string
	dumped := func() bool {
		idx, skip, err := state.ReadIndex(f.stateDir)
		if err != nil || skip {
			return false
		}
		files = scrollbackFilesBySession(idx)
		for _, s := range sessions {
			if len(files[s.name]) != len(s.hookKeys) || !allNonEmpty(f.stateDir, files[s.name]) {
				return false
			}
		}
		return true
	}
	if !harnesstest.PollUntil(t, firstDumpBudget, daemonTickPollInterval, dumped) {
		t.Fatalf("daemon did not save every user pane's scrollback within %s\n%s",
			firstDumpBudget, f.diagnostic())
	}
	return files
}

// retireDaemon ends the fixture's daemon and keeps _portal-saver running on a
// placeholder, so no daemon tick commits a kill: one would hide a commit-now
// that stood down. The daemon's shutdown flush has finished once its process
// is gone.
func retireDaemon(t *testing.T, f symptomFixture) {
	t.Helper()
	pid, present, err := tmux.SaverPanePIDOrAbsent(f.sock.Client(), tmux.PortalSaverName)
	if err != nil || !present {
		t.Fatalf("read daemon pid: present=%v err=%v\n%s", present, err, f.diagnostic())
	}

	f.sock.Run(t, "respawn-pane", "-k", "-t", string(tmux.CoordTargetExact(tmux.PortalSaverName)),
		"sh", "-c", "read -r _")

	gone := func() bool { return errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) }
	if !harnesstest.PollUntil(t, daemonExitBudget, daemonTickPollInterval, gone) {
		t.Fatalf("daemon %d still running %s after its pane was respawned\n%s",
			pid, daemonExitBudget, f.diagnostic())
	}
}

// pollKillCommitted waits for consecutive reads to agree that sessions.json
// omits the killed session, still names every survivor, and that the killed
// session's scrollback files are gone from disk.
func pollKillCommitted(ctx context.Context, stateDir, killed string, survivors, killedFiles []string) error {
	var consecutive int
	ticker := time.NewTicker(reentrancyPollInterval)
	defer ticker.Stop()
	for {
		idx, skip, err := state.ReadIndex(stateDir)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("read sessions.json during poll: %w", err)
		}
		committed := err == nil && !skip &&
			matchesShape(sessionNames(idx), survivors, []string{killed}) &&
			noneExist(stateDir, killedFiles)
		if committed {
			consecutive++
			if consecutive >= reentrancyConsecutiveReads {
				return nil
			}
		} else {
			consecutive = 0
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func assertEmptyRestoreState(t *testing.T, f symptomFixture) {
	t.Helper()
	idx, skip, err := state.ReadIndex(f.stateDir)
	if err != nil || skip {
		t.Fatalf("read sessions.json after the last kill: skip=%v err=%v\n%s", skip, err, f.diagnostic())
	}
	if len(idx.Sessions) != 0 {
		t.Errorf("sessions.json after the last kill holds %d sessions, want 0: %v",
			len(idx.Sessions), keysOf(sessionNames(idx)))
	}

	remaining, err := os.ReadDir(state.ScrollbackDir(f.stateDir))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("read scrollback dir: %v", err)
	}
	if len(remaining) != 0 {
		names := make([]string, 0, len(remaining))
		for _, e := range remaining {
			names = append(names, e.Name())
		}
		t.Errorf("scrollback files remain after the last kill: %v", names)
	}

	live := keysOf(liveSessionNames(t, f.sock))
	slices.Sort(live)
	want := []string{tmux.PortalBootstrapName, tmux.PortalSaverName}
	if !slices.Equal(live, want) {
		t.Errorf("tmux sessions after the last kill = %v, want %v", live, want)
	}
}

func assertSweepReapsKilledHooks(t *testing.T, f symptomFixture, hooksPath string, sessions []killPathSession) {
	t.Helper()
	var killedKeys []string
	for _, s := range sessions {
		killedKeys = append(killedKeys, s.hookKeys...)
	}
	slices.Sort(killedKeys)

	store := hooks.NewStore(hooksPath)
	outcome, err := hooksweep.Run(f.sock.Client(), store)
	if err != nil {
		t.Fatalf("hook-staleness sweep: %v", err)
	}
	removed := slices.Sorted(slices.Values(outcome.Removed))
	if !slices.Equal(removed, killedKeys) {
		t.Errorf("sweep removed %v (decline reason %v), want the killed sessions' keys %v",
			removed, outcome.DeclineReason, killedKeys)
	}

	left, err := store.List(hooks.ViaInternal)
	if err != nil {
		t.Fatalf("list hooks after the sweep: %v", err)
	}
	if len(left) != 0 {
		t.Errorf("hooks.json after the sweep holds %d hooks, want 0: %+v", len(left), left)
	}
}

func scrollbackFilesBySession(idx state.Index) map[string][]string {
	files := map[string][]string{}
	for _, s := range idx.Sessions {
		for _, w := range s.Windows {
			for _, p := range w.Panes {
				if p.ScrollbackFile != "" {
					files[s.Name] = append(files[s.Name], p.ScrollbackFile)
				}
			}
		}
	}
	return files
}

func sessionNamesOf(sessions []killPathSession) []string {
	names := make([]string, 0, len(sessions))
	for _, s := range sessions {
		names = append(names, s.name)
	}
	return names
}

func allNonEmpty(stateDir string, rel []string) bool {
	for _, r := range rel {
		info, err := os.Stat(filepath.Join(stateDir, r))
		if err != nil || info.Size() == 0 {
			return false
		}
	}
	return true
}

func noneExist(stateDir string, rel []string) bool {
	for _, r := range rel {
		if _, err := os.Stat(filepath.Join(stateDir, r)); !errors.Is(err, fs.ErrNotExist) {
			return false
		}
	}
	return true
}

func assertFilesExist(t *testing.T, stateDir string, rel []string) {
	t.Helper()
	for _, r := range rel {
		if _, err := os.Stat(filepath.Join(stateDir, r)); err != nil {
			t.Errorf("survivor scrollback %s: %v", r, err)
		}
	}
}
