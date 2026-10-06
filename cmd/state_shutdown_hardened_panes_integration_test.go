//go:build integration

package cmd_test

import (
	"encoding/json"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/portalbintest"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/restoretest"
	"github.com/leeovery/portal/internal/resumemode"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

const (
	shellSession    = "shell"
	eagerSession    = "eager"
	answeredSession = "answered"

	// runningHook prints a line and then keeps its program running until a
	// signal ends it, so the pane's resume shell is mid-hook when the shutdown
	// signal lands.
	runningHook    = "echo resumed; sleep 600"
	runningHookArg = "sleep 600"
)

var (
	eagerPaneToken    = hookstest.SubjectSeedB
	answeredPaneToken = hookstest.SubjectSeedC
)

// hardenedSession is one restored session of the fixture: its single pane's
// saved token, the registration it resumes with, and the transcript saved for
// it before the restore.
type hardenedSession struct {
	name       string
	token      string
	hook       hooks.Registration
	transcript string
}

var hardenedSessions = []hardenedSession{
	{name: shellSession, transcript: "shell transcript before shutdown\n"},
	{name: eagerSession, token: eagerPaneToken, transcript: "eager transcript before shutdown\n",
		hook: hooks.Registration{Command: runningHook, Resume: resumemode.Eager}},
	{name: waitingSession, token: waitingPaneToken, transcript: waitingTranscript,
		hook: hooks.Registration{Command: waitingHook}},
	{name: answeredSession, token: answeredPaneToken, transcript: "answered transcript before shutdown\n",
		hook: hooks.Registration{Command: runningHook}},
}

func TestShutdown_PaneProgramsAndDaemonThenServerPreservesHardenedSessions(t *testing.T) {
	requireUnwrappedTmux(t)

	binDir := portalbintest.StagePortalBinary(t)
	binary, err := exec.LookPath("portal")
	if err != nil {
		t.Skipf("portal not on PATH after build+prepend; skipping: %v", err)
	}

	rt := newHardenedRuntime(t, binary, binDir)
	tokenPath := state.PendingScrollbackFile(waitingPaneToken)
	transcript := rt.awaitWaitingPaneSaved(t, tokenPath)
	rt.awaitEverySessionSaved(t)

	mark := portaltest.ReadPortalLogSafe(rt.stateDir)
	for _, pid := range rt.paneProcesses(t) {
		rt.signal(t, pid, syscall.SIGTERM)
	}
	rt.signal(t, rt.daemonPID, syscall.SIGTERM)

	rt.awaitGone(t, "daemon", rt.daemonPID)
	rt.awaitSaverClosed(t)
	rt.awaitCommitNowCommitted(t, mark)
	rt.assertSessionsLive(t)

	rt.signal(t, rt.serverPID, syscall.SIGTERM)
	rt.awaitGone(t, "tmux server", rt.serverPID)
	awaitStateDirSettled(t, rt.stateDir)

	rt.assertEverySessionPreserved(t)
	rt.assertSavedWaiting(t, tokenPath, transcript)
}

// hardenedRuntime is a Portal runtime restored onto saved state holding an
// interactive-shell session beside a session for each kind of hardened Portal
// pane: an eager resume pane running its hook, a lazy pane still waiting, and
// a lazy pane answered with its hook running.
type hardenedRuntime struct {
	*waitingPaneRuntime
}

func newHardenedRuntime(t *testing.T, binary, binDir string) hardenedRuntime {
	t.Helper()

	_, stateDir := portaltest.IsolateStateForTest(t)

	// Set before the tmux server starts, so the daemon, the restored panes and
	// every session-closed hook subprocess resolve this fixture's files, and
	// every pane handed to $SHELL runs a shell that ignores SIGTERM.
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	t.Setenv("PORTAL_HOOKS_FILE", filepath.Join(stateDir, "hooks.json"))
	t.Setenv("PORTAL_PROJECTS_FILE", filepath.Join(stateDir, "projects.json"))
	t.Setenv("PORTAL_ALIASES_FILE", filepath.Join(stateDir, "aliases"))
	t.Setenv("SHELL", interactiveShell(t))

	portaltest.RegisterStateDirTeardownGuard(t, stateDir)

	seedHardenedState(t, stateDir)

	rt := hardenedRuntime{&waitingPaneRuntime{symptomFixture: symptomFixture{
		sock:     tmuxtest.New(t, "ptl-hardened-"),
		stateDir: stateDir,
		binary:   binary,
		binDir:   binDir,
	}}}
	rt.bootstrap(t)
	rt.awaitPanel(t)
	rt.assertPending(t)
	rt.answer(t)
	rt.awaitSettledPanes(t)
	return rt
}

// interactiveShell answers a shell that ignores SIGTERM when interactive, as
// zsh and bash do, so the run does not depend on the developer's own $SHELL.
func interactiveShell(t *testing.T) string {
	t.Helper()
	for _, shell := range []string{"/bin/zsh", "/bin/bash"} {
		if _, err := os.Stat(shell); err == nil {
			return shell
		}
	}
	t.Skip("neither /bin/zsh nor /bin/bash is present")
	return ""
}

func seedHardenedState(t *testing.T, stateDir string) {
	t.Helper()
	var idx state.Index
	registrations := map[string]map[hooks.Event]hooks.Registration{}
	for _, s := range hardenedSessions {
		restoretest.SeedScrollback(t, stateDir, s.name, 0, 0, []byte(s.transcript))
		positional, err := filepath.Rel(stateDir, state.ScrollbackFile(stateDir, state.SanitizePaneKey(s.name, 0, 0)))
		if err != nil {
			t.Fatalf("relative scrollback path: %v", err)
		}
		idx.Sessions = append(idx.Sessions, state.Session{
			Name: s.name,
			Windows: []state.Window{{
				Index:  0,
				Layout: "tiled",
				Active: true,
				Panes: []state.Pane{{
					Index:          0,
					Active:         true,
					ScrollbackFile: filepath.ToSlash(positional),
					PortalPaneID:   s.token,
				}},
			}},
		})
		if s.token != "" {
			registrations[s.token] = map[hooks.Event]hooks.Registration{hooks.EventOnResume: s.hook}
		}
	}
	restoretest.WriteIndex(t, stateDir, idx)

	body, err := json.Marshal(registrations)
	if err != nil {
		t.Fatalf("marshal hooks.json: %v", err)
	}
	hookstest.StageStore(t, hookstest.Staging{Dir: stateDir, Seed: string(body)})
}

func answeredPaneTarget() tmux.Target { return tmux.PaneTargetExact(answeredSession, 0, 0) }

// answer presses Enter on the answered session's panel once it is up, and
// waits for the pane to stop waiting.
func (rt hardenedRuntime) answer(t *testing.T) {
	t.Helper()
	target := answeredPaneTarget()
	shown := func() bool {
		out, err := rt.sock.TryRun("capture-pane", "-p", "-t", string(target))
		return err == nil && strings.Contains(out, waitingPanelTitle)
	}
	if !harnesstest.PollUntil(t, waitingPaneBudget, daemonTickPollInterval, shown) {
		t.Fatalf("pane %s never showed the panel\n%s", target, rt.waitingDiagnostic())
	}

	rt.sock.Run(t, "send-keys", "-t", string(target), "Enter")

	cleared := func() bool {
		value, err := rt.sock.Client().ReadPaneOption(target, state.ResumePendingOption)
		return err == nil && !state.ResumePendingSet(value)
	}
	if !harnesstest.PollUntil(t, waitingPaneBudget, daemonTickPollInterval, cleared) {
		t.Fatalf("%s still set on pane %s after Enter\n%s", state.ResumePendingOption, target, rt.waitingDiagnostic())
	}
}

// awaitSettledPanes waits for every user pane to reach the program the
// shutdown signal is meant to find it running.
func (rt hardenedRuntime) awaitSettledPanes(t *testing.T) {
	t.Helper()
	shell := filepath.Base(os.Getenv("SHELL"))
	settled := map[string]func([]waitingProcess) bool{
		shellSession: func(tree []waitingProcess) bool {
			return len(tree) == 1 && strings.Contains(tree[0].command, shell) &&
				!strings.Contains(tree[0].command, "portal")
		},
		eagerSession:    runsHookProgram,
		answeredSession: runsHookProgram,
	}
	for _, name := range slices.Sorted(maps.Keys(settled)) {
		var tree []waitingProcess
		reached := func() bool {
			tree = waitingPaneTree(t, rt.panePID(t, name))
			return settled[name](tree)
		}
		if !harnesstest.PollUntil(t, waitingPaneBudget, daemonTickPollInterval, reached) {
			t.Fatalf("session %s's pane never settled: %+v\n%s", name, tree, rt.waitingDiagnostic())
		}
	}
	rt.restingTree(t)
}

func runsHookProgram(tree []waitingProcess) bool {
	return slices.ContainsFunc(tree, func(p waitingProcess) bool {
		return strings.HasPrefix(p.command, runningHookArg)
	})
}

func (rt hardenedRuntime) panePID(t *testing.T, session string) int {
	t.Helper()
	target := tmux.PaneTargetExact(session, 0, 0)
	raw := strings.TrimSpace(rt.sock.Run(t, "display-message", "-p", "-t", string(target), "#{pane_pid}"))
	pid, err := strconv.Atoi(raw)
	if err != nil {
		t.Fatalf("pane_pid of %s = %q: %v", target, raw, err)
	}
	return pid
}

// awaitEverySessionSaved waits for a commit naming every seeded session with
// a non-empty scrollback file for each of its panes.
func (rt hardenedRuntime) awaitEverySessionSaved(t *testing.T) {
	t.Helper()
	if err := state.TouchSaveRequested(rt.stateDir); err != nil {
		t.Fatalf("request a save: %v", err)
	}
	saved := func() bool {
		idx, skip, err := state.ReadIndex(rt.stateDir)
		if err != nil || skip {
			return false
		}
		files := scrollbackFilesBySession(idx)
		for _, s := range hardenedSessions {
			if len(files[s.name]) != 1 || !allNonEmpty(rt.stateDir, files[s.name]) {
				return false
			}
		}
		return true
	}
	if !harnesstest.PollUntil(t, waitingPaneBudget, daemonTickPollInterval, saved) {
		t.Fatalf("the daemon never saved every session's scrollback within %s\n%s",
			waitingPaneBudget, rt.waitingDiagnostic())
	}
}

// paneProcesses answers every process in every pane on the fixture's server
// other than the saver's, whose pane program is the daemon.
func (rt hardenedRuntime) paneProcesses(t *testing.T) []int {
	t.Helper()
	out := rt.sock.Run(t, "list-panes", "-a", "-F", "#{session_name} #{pane_pid}")
	var pids []int
	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		session, rawPID, ok := strings.Cut(line, " ")
		if !ok {
			t.Fatalf("list-panes line %q", line)
		}
		if session == tmux.PortalSaverName {
			continue
		}
		root, err := strconv.Atoi(rawPID)
		if err != nil {
			t.Fatalf("pane_pid %q: %v", rawPID, err)
		}
		for _, p := range waitingPaneTree(t, root) {
			pids = append(pids, p.pid)
		}
	}
	return pids
}

func (rt hardenedRuntime) assertSessionsLive(t *testing.T) {
	t.Helper()
	live := liveSessionNames(t, rt.sock)
	for _, s := range hardenedSessions {
		if _, ok := live[s.name]; !ok {
			t.Errorf("session %s closed after the pane programs and the daemon were sent SIGTERM\n%s",
				s.name, rt.waitingDiagnostic())
		}
	}
}

// assertEverySessionPreserved holds sessions.json to naming every seeded
// session, and every scrollback file it names to being present and non-empty.
func (rt hardenedRuntime) assertEverySessionPreserved(t *testing.T) {
	t.Helper()
	idx, skip, err := state.ReadIndex(rt.stateDir)
	if err != nil || skip {
		t.Fatalf("read sessions.json after the shutdown: skip=%v err=%v\n%s", skip, err, rt.waitingDiagnostic())
	}
	named := slices.Sorted(maps.Keys(sessionNames(idx)))
	var want []string
	for _, s := range hardenedSessions {
		want = append(want, s.name)
	}
	if slices.Sort(want); !slices.Equal(named, want) {
		t.Errorf("sessions.json after the shutdown names %v, want %v\n%s", named, want, rt.waitingDiagnostic())
	}
	for session, files := range scrollbackFilesBySession(idx) {
		for _, rel := range files {
			info, err := os.Stat(filepath.Join(rt.stateDir, rel))
			switch {
			case err != nil:
				t.Errorf("session %s's scrollback %s after the shutdown: %v", session, rel, err)
			case info.Size() == 0:
				t.Errorf("session %s's scrollback %s is empty after the shutdown", session, rel)
			}
		}
	}
}
