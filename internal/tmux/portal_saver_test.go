package tmux_test

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// stubAliveCheck installs a fake daemon-alive check via the package seam and
// restores the original via t.Cleanup. The fake returns the supplied bool
// regardless of stateDir.
func stubAliveCheck(t *testing.T, alive bool) {
	t.Helper()
	prev := tmux.BootstrapAliveCheck
	tmux.BootstrapAliveCheck = func(string) bool { return alive }
	t.Cleanup(func() { tmux.BootstrapAliveCheck = prev })
}

// shrinkRetryDelay collapses the retry sleep to a microsecond for tests and
// restores the production value via t.Cleanup.
func shrinkRetryDelay(t *testing.T) {
	t.Helper()
	prev := tmux.PortalSaverRetryDelay
	tmux.PortalSaverRetryDelay = 1 * time.Microsecond
	t.Cleanup(func() { tmux.PortalSaverRetryDelay = prev })
}

// portalSaverScript builds a RunFunc dispatching on argv[0] using the supplied
// per-command response handlers. Each handler receives a 1-indexed call
// counter so tests can vary behavior across repeated calls of the same
// command. A nil handler causes the run helper to t.Fatalf — tests opt in to
// each command they expect.
type portalSaverScript struct {
	hasSession   func(call int) (string, error) // tmux has-session -t <name>
	newSession   func(call int) (string, error) // tmux new-session -d -s <name> [cmd]
	killSession  func(call int) (string, error) // tmux kill-session -t <name>
	setOption    func(call int) (string, error) // tmux set-option -t <sess> <name> <value>
	hasSessionN  int
	newSessionN  int
	killSessionN int
	setOptionN   int
}

func (s *portalSaverScript) run(t *testing.T) func(args ...string) (string, error) {
	t.Helper()
	return func(args ...string) (string, error) {
		if len(args) == 0 {
			t.Fatalf("empty argv")
			return "", nil
		}
		switch args[0] {
		case "has-session":
			s.hasSessionN++
			if s.hasSession == nil {
				t.Fatalf("unexpected has-session call: %v", args)
				return "", nil
			}
			return s.hasSession(s.hasSessionN)
		case "new-session":
			s.newSessionN++
			if s.newSession == nil {
				t.Fatalf("unexpected new-session call: %v", args)
				return "", nil
			}
			return s.newSession(s.newSessionN)
		case "kill-session":
			s.killSessionN++
			if s.killSession == nil {
				t.Fatalf("unexpected kill-session call: %v", args)
				return "", nil
			}
			return s.killSession(s.killSessionN)
		case "set-option":
			s.setOptionN++
			if s.setOption == nil {
				t.Fatalf("unexpected set-option call: %v", args)
				return "", nil
			}
			return s.setOption(s.setOptionN)
		default:
			t.Fatalf("unexpected command: %v", args)
			return "", nil
		}
	}
}

// countCalls returns counts of calls dispatched on argv[0].
func countCalls(calls [][]string, name string) int {
	n := 0
	for _, c := range calls {
		if len(c) > 0 && c[0] == name {
			n++
		}
	}
	return n
}

func TestBootstrapPortalSaver_CreatesOnFreshServer(t *testing.T) {
	stubAliveCheck(t, false) // irrelevant when session absent
	shrinkRetryDelay(t)

	script := &portalSaverScript{
		hasSession: func(call int) (string, error) {
			// Only one has-session expected: pre-create check returns false (absent).
			return "", errors.New("can't find session: _portal-saver")
		},
		newSession: func(call int) (string, error) { return "", nil },
		setOption:  func(call int) (string, error) { return "", nil },
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.BootstrapPortalSaver(client, "/tmp/portal-state"); err != nil {
		t.Fatalf("BootstrapPortalSaver returned error: %v", err)
	}

	if got := countCalls(mock.Calls, "new-session"); got != 1 {
		t.Errorf("expected exactly 1 new-session call, got %d (calls: %v)", got, mock.Calls)
	}
	if got := countCalls(mock.Calls, "set-option"); got != 1 {
		t.Errorf("expected exactly 1 set-option call, got %d (calls: %v)", got, mock.Calls)
	}
	if got := countCalls(mock.Calls, "kill-session"); got != 0 {
		t.Errorf("expected 0 kill-session calls, got %d", got)
	}

	// Verify new-session argv shape (no -c).
	for _, c := range mock.Calls {
		if c[0] != "new-session" {
			continue
		}
		joined := strings.Join(c, " ")
		if joined != "new-session -d -s _portal-saver portal state daemon" {
			t.Errorf("new-session argv = %q, want %q", joined, "new-session -d -s _portal-saver portal state daemon")
		}
	}
}

func TestBootstrapPortalSaver_NoOpWhenSessionExistsAndDaemonAlive(t *testing.T) {
	stubAliveCheck(t, true)
	shrinkRetryDelay(t)

	script := &portalSaverScript{
		hasSession: func(call int) (string, error) { return "", nil }, // present
		setOption:  func(call int) (string, error) { return "", nil },
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.BootstrapPortalSaver(client, "/tmp/portal-state"); err != nil {
		t.Fatalf("BootstrapPortalSaver returned error: %v", err)
	}

	if got := countCalls(mock.Calls, "new-session"); got != 0 {
		t.Errorf("expected 0 new-session calls, got %d (calls: %v)", got, mock.Calls)
	}
	if got := countCalls(mock.Calls, "kill-session"); got != 0 {
		t.Errorf("expected 0 kill-session calls, got %d", got)
	}
	if got := countCalls(mock.Calls, "set-option"); got != 1 {
		t.Errorf("expected exactly 1 set-option call, got %d", got)
	}
}

func TestBootstrapPortalSaver_KillsAndRecreatesWhenSessionExistsButDaemonDead(t *testing.T) {
	stubAliveCheck(t, false)
	shrinkRetryDelay(t)

	script := &portalSaverScript{
		hasSession:  func(call int) (string, error) { return "", nil }, // present
		killSession: func(call int) (string, error) { return "", nil },
		newSession:  func(call int) (string, error) { return "", nil },
		setOption:   func(call int) (string, error) { return "", nil },
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.BootstrapPortalSaver(client, "/tmp/portal-state"); err != nil {
		t.Fatalf("BootstrapPortalSaver returned error: %v", err)
	}

	if got := countCalls(mock.Calls, "kill-session"); got != 1 {
		t.Errorf("expected 1 kill-session call, got %d (calls: %v)", got, mock.Calls)
	}
	if got := countCalls(mock.Calls, "new-session"); got != 1 {
		t.Errorf("expected 1 new-session call, got %d", got)
	}
	if got := countCalls(mock.Calls, "set-option"); got != 1 {
		t.Errorf("expected 1 set-option call, got %d", got)
	}

	// Order check: kill-session must precede new-session.
	killIdx, newIdx := -1, -1
	for i, c := range mock.Calls {
		switch c[0] {
		case "kill-session":
			if killIdx == -1 {
				killIdx = i
			}
		case "new-session":
			if newIdx == -1 {
				newIdx = i
			}
		}
	}
	if killIdx >= newIdx {
		t.Errorf("kill-session at %d must precede new-session at %d (calls: %v)", killIdx, newIdx, mock.Calls)
	}
}

// TestBootstrapPortalSaver_RecoversFromFlockLoserEmptySession exercises the
// convergence path a flock-loser leaves behind when default tmux behaviour
// (no remain-on-exit) closes the session after the loser daemon exits status 0
// as the session's initial process. The next bootstrap observes
// HasSession(_portal-saver) == false and falls through directly to
// createPortalSaverWithRetry — no prior session to kill.
//
// Regression guard for § Fix Part 1: Loser-daemon session aftermath and
// § Test Strategy → Regression test — flock-loser recovery.
func TestBootstrapPortalSaver_RecoversFromFlockLoserEmptySession(t *testing.T) {
	stubAliveCheck(t, false) // irrelevant when session absent (short-circuits)
	shrinkRetryDelay(t)

	script := &portalSaverScript{
		hasSession: func(call int) (string, error) {
			// Loser closed the session on exit; bootstrap observes it absent.
			return "", errors.New("can't find session: _portal-saver")
		},
		newSession: func(call int) (string, error) { return "", nil },
		setOption:  func(call int) (string, error) { return "", nil },
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.BootstrapPortalSaver(client, "/tmp/portal-state"); err != nil {
		t.Fatalf("BootstrapPortalSaver returned error: %v", err)
	}

	if got := countCalls(mock.Calls, "new-session"); got != 1 {
		t.Errorf("expected exactly 1 new-session call, got %d (calls: %v)", got, mock.Calls)
	}
	if got := countCalls(mock.Calls, "set-option"); got != 1 {
		t.Errorf("expected exactly 1 set-option call, got %d (calls: %v)", got, mock.Calls)
	}
	if got := countCalls(mock.Calls, "kill-session"); got != 0 {
		t.Errorf("expected 0 kill-session calls (no prior session to kill), got %d (calls: %v)", got, mock.Calls)
	}
}

// TestBootstrapPortalSaver_RecoversFromFlockLoserDeadPaneSession exercises the
// convergence path a flock-loser leaves behind when remain-on-exit kept the
// session alive but the daemon pane is dead. The next bootstrap observes
// HasSession(_portal-saver) == true, BootstrapAliveCheck returns false, and
// the stale-pidfile recovery branch fires: tolerant kill followed by
// recreate.
//
// Regression guard for § Fix Part 1: Loser-daemon session aftermath and
// § Test Strategy → Regression test — flock-loser recovery.
func TestBootstrapPortalSaver_RecoversFromFlockLoserDeadPaneSession(t *testing.T) {
	stubAliveCheck(t, false)
	shrinkRetryDelay(t)

	script := &portalSaverScript{
		hasSession:  func(call int) (string, error) { return "", nil }, // present
		killSession: func(call int) (string, error) { return "", nil },
		newSession:  func(call int) (string, error) { return "", nil },
		setOption:   func(call int) (string, error) { return "", nil },
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.BootstrapPortalSaver(client, "/tmp/portal-state"); err != nil {
		t.Fatalf("BootstrapPortalSaver returned error: %v", err)
	}

	if got := countCalls(mock.Calls, "kill-session"); got != 1 {
		t.Errorf("expected 1 kill-session call, got %d (calls: %v)", got, mock.Calls)
	}
	if got := countCalls(mock.Calls, "new-session"); got != 1 {
		t.Errorf("expected 1 new-session call, got %d (calls: %v)", got, mock.Calls)
	}
	if got := countCalls(mock.Calls, "set-option"); got != 1 {
		t.Errorf("expected 1 set-option call, got %d (calls: %v)", got, mock.Calls)
	}

	// Order check: kill-session must precede new-session.
	killIdx, newIdx := -1, -1
	for i, c := range mock.Calls {
		switch c[0] {
		case "kill-session":
			if killIdx == -1 {
				killIdx = i
			}
		case "new-session":
			if newIdx == -1 {
				newIdx = i
			}
		}
	}
	if killIdx == -1 || newIdx == -1 || killIdx >= newIdx {
		t.Errorf("kill-session at %d must precede new-session at %d (calls: %v)", killIdx, newIdx, mock.Calls)
	}
}

func TestBootstrapPortalSaver_AlwaysSetsDestroyUnattachedOff(t *testing.T) {
	stubAliveCheck(t, true)
	shrinkRetryDelay(t)

	var setOptionArgs []string
	script := &portalSaverScript{
		hasSession: func(call int) (string, error) { return "", nil },
		setOption:  func(call int) (string, error) { return "", nil },
	}
	mock := &MockCommander{
		RunFunc: func(args ...string) (string, error) {
			if len(args) > 0 && args[0] == "set-option" {
				setOptionArgs = append([]string{}, args...)
			}
			return script.run(t)(args...)
		},
	}
	client := tmux.NewClient(mock)

	if err := tmux.BootstrapPortalSaver(client, "/tmp/portal-state"); err != nil {
		t.Fatalf("BootstrapPortalSaver returned error: %v", err)
	}

	wantArgs := []string{"set-option", "-t", "_portal-saver", "destroy-unattached", "off"}
	if len(setOptionArgs) != len(wantArgs) {
		t.Fatalf("set-option argv = %v, want %v", setOptionArgs, wantArgs)
	}
	for i, arg := range wantArgs {
		if setOptionArgs[i] != arg {
			t.Errorf("set-option arg[%d] = %q, want %q", i, setOptionArgs[i], arg)
		}
	}
}

func TestBootstrapPortalSaver_NeverUsesGlobalScopeForSetOption(t *testing.T) {
	stubAliveCheck(t, true)
	shrinkRetryDelay(t)

	script := &portalSaverScript{
		hasSession: func(call int) (string, error) { return "", nil },
		setOption:  func(call int) (string, error) { return "", nil },
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.BootstrapPortalSaver(client, "/tmp/portal-state"); err != nil {
		t.Fatalf("BootstrapPortalSaver returned error: %v", err)
	}

	for _, call := range mock.Calls {
		if len(call) == 0 || call[0] != "set-option" {
			continue
		}
		for _, arg := range call {
			if arg == "-g" {
				t.Errorf("set-option call must never include -g (global scope), got %v", call)
			}
		}
	}
}

func TestBootstrapPortalSaver_RetriesNewSessionUpTo3TimesOnTransientFailure(t *testing.T) {
	stubAliveCheck(t, false)
	shrinkRetryDelay(t)

	hasSessionCall := 0
	newSessionCall := 0

	mock := &MockCommander{
		RunFunc: func(args ...string) (string, error) {
			switch args[0] {
			case "has-session":
				hasSessionCall++
				// First call (pre-create): absent.
				// Subsequent calls (post-error race re-checks): also absent so retries continue.
				return "", errors.New("can't find session")
			case "new-session":
				newSessionCall++
				if newSessionCall < 3 {
					return "", errors.New("transient tmux error")
				}
				return "", nil // success on 3rd attempt
			case "set-option":
				return "", nil
			default:
				t.Fatalf("unexpected command: %v", args)
				return "", nil
			}
		},
	}
	client := tmux.NewClient(mock)

	if err := tmux.BootstrapPortalSaver(client, "/tmp/portal-state"); err != nil {
		t.Fatalf("BootstrapPortalSaver returned error: %v", err)
	}

	if newSessionCall != 3 {
		t.Errorf("expected 3 new-session calls, got %d", newSessionCall)
	}
	if got := countCalls(mock.Calls, "set-option"); got != 1 {
		t.Errorf("expected 1 set-option call after retry success, got %d", got)
	}
}

func TestBootstrapPortalSaver_ReturnsWrappedErrorAfterRetryExhaustion(t *testing.T) {
	stubAliveCheck(t, false)
	shrinkRetryDelay(t)

	mock := &MockCommander{
		RunFunc: func(args ...string) (string, error) {
			switch args[0] {
			case "has-session":
				return "", errors.New("can't find session") // never present
			case "new-session":
				return "", errors.New("persistent tmux failure")
			case "set-option":
				t.Fatalf("set-option must not be called when create exhausts retries")
				return "", nil
			default:
				t.Fatalf("unexpected command: %v", args)
				return "", nil
			}
		},
	}
	client := tmux.NewClient(mock)

	err := tmux.BootstrapPortalSaver(client, "/tmp/portal-state")
	if err == nil {
		t.Fatal("expected error after retry exhaustion, got nil")
	}
	if !strings.Contains(err.Error(), "_portal-saver") {
		t.Errorf("error %q should mention _portal-saver", err.Error())
	}
	if !strings.Contains(err.Error(), "persistent tmux failure") {
		t.Errorf("error %q should wrap underlying tmux error", err.Error())
	}

	if got := countCalls(mock.Calls, "new-session"); got != 3 {
		t.Errorf("expected exactly 3 new-session attempts, got %d", got)
	}
	if got := countCalls(mock.Calls, "set-option"); got != 0 {
		t.Errorf("set-option must not run after retry exhaustion, got %d calls", got)
	}
}

func TestBootstrapPortalSaver_ToleratesKillSessionFailureWhenTransitioningFromOrphan(t *testing.T) {
	stubAliveCheck(t, false)
	shrinkRetryDelay(t)

	script := &portalSaverScript{
		hasSession:  func(call int) (string, error) { return "", nil }, // present
		killSession: func(call int) (string, error) { return "", errors.New("session vanished mid-flight") },
		newSession:  func(call int) (string, error) { return "", nil },
		setOption:   func(call int) (string, error) { return "", nil },
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.BootstrapPortalSaver(client, "/tmp/portal-state"); err != nil {
		t.Fatalf("BootstrapPortalSaver should tolerate kill failure, got: %v", err)
	}

	if got := countCalls(mock.Calls, "kill-session"); got != 1 {
		t.Errorf("expected 1 kill-session call, got %d", got)
	}
	if got := countCalls(mock.Calls, "new-session"); got != 1 {
		t.Errorf("expected creation to proceed despite kill failure, got %d new-session calls", got)
	}
	if got := countCalls(mock.Calls, "set-option"); got != 1 {
		t.Errorf("expected 1 set-option call, got %d", got)
	}
}

func TestBootstrapPortalSaver_PropagatesSetOptionFailureWithSessionAndOptionName(t *testing.T) {
	stubAliveCheck(t, true)
	shrinkRetryDelay(t)

	script := &portalSaverScript{
		hasSession: func(call int) (string, error) { return "", nil }, // present
		setOption:  func(call int) (string, error) { return "", errors.New("permission denied") },
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	err := tmux.BootstrapPortalSaver(client, "/tmp/portal-state")
	if err == nil {
		t.Fatal("expected error from set-option failure, got nil")
	}
	if !strings.Contains(err.Error(), "destroy-unattached") {
		t.Errorf("error %q should reference option name destroy-unattached", err.Error())
	}
	if !strings.Contains(err.Error(), "_portal-saver") {
		t.Errorf("error %q should reference session name _portal-saver", err.Error())
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("error %q should wrap underlying tmux error", err.Error())
	}
}

func TestBootstrapPortalSaver_NoRedundantCreateOnConcurrentBootstrapRace(t *testing.T) {
	stubAliveCheck(t, false)
	shrinkRetryDelay(t)

	hasSessionCall := 0
	newSessionCall := 0

	mock := &MockCommander{
		RunFunc: func(args ...string) (string, error) {
			switch args[0] {
			case "has-session":
				hasSessionCall++
				if hasSessionCall == 1 {
					// Pre-create: not present.
					return "", errors.New("can't find session")
				}
				// Post-error race recheck: a concurrent bootstrap won.
				return "", nil
			case "new-session":
				newSessionCall++
				return "", errors.New("duplicate session: _portal-saver")
			case "set-option":
				return "", nil
			default:
				t.Fatalf("unexpected command: %v", args)
				return "", nil
			}
		},
	}
	client := tmux.NewClient(mock)

	if err := tmux.BootstrapPortalSaver(client, "/tmp/portal-state"); err != nil {
		t.Fatalf("expected concurrent-bootstrap race to be treated as success, got: %v", err)
	}

	if newSessionCall != 1 {
		t.Errorf("expected exactly 1 new-session attempt before race detected, got %d", newSessionCall)
	}
	if got := countCalls(mock.Calls, "set-option"); got != 1 {
		t.Errorf("expected set-option to still run after race detected, got %d calls", got)
	}
}

// versionScenario configures a MockCommander dispatcher for
// EnsurePortalSaverVersion tests. By default, has-session reports the session
// present (so tests opt out by overriding when needed); kill-session,
// new-session and set-option succeed. Counters track how many times each
// command was invoked.
type versionScenario struct {
	sessionPresent bool
	killSessionErr error
	newSessionErr  error
	setOptionErr   error

	hasSessionCalls  int
	killSessionCalls int
	newSessionCalls  int
	setOptionCalls   int
}

func (s *versionScenario) run(t *testing.T) func(args ...string) (string, error) {
	t.Helper()
	return func(args ...string) (string, error) {
		if len(args) == 0 {
			t.Fatalf("empty argv")
			return "", nil
		}
		switch args[0] {
		case "has-session":
			s.hasSessionCalls++
			if s.sessionPresent {
				return "", nil
			}
			return "", errors.New("can't find session: _portal-saver")
		case "kill-session":
			s.killSessionCalls++
			// After a successful kill the session is no longer present.
			if s.killSessionErr == nil {
				s.sessionPresent = false
			}
			return "", s.killSessionErr
		case "new-session":
			s.newSessionCalls++
			if s.newSessionErr == nil {
				s.sessionPresent = true
			}
			return "", s.newSessionErr
		case "set-option":
			s.setOptionCalls++
			return "", s.setOptionErr
		default:
			t.Fatalf("unexpected command: %v", args)
			return "", nil
		}
	}
}

// writeVersion seeds dir with daemon.version containing the supplied content.
func writeVersion(t *testing.T, dir, version string) {
	t.Helper()
	if err := state.WriteVersionFile(dir, version); err != nil {
		t.Fatalf("WriteVersionFile(%q) returned error: %v", version, err)
	}
}

// assertNoDaemonVersionFile fails the test if daemon.version exists in dir.
func assertNoDaemonVersionFile(t *testing.T, dir string) {
	t.Helper()
	_, err := os.Stat(state.DaemonVersion(dir))
	if err == nil {
		t.Errorf("daemon.version exists at %q after EnsurePortalSaverVersion; the function must not write it", state.DaemonVersion(dir))
		return
	}
	if !os.IsNotExist(err) {
		t.Fatalf("unexpected stat error for daemon.version: %v", err)
	}
}

func TestEnsurePortalSaverVersion_DoesNotKillWhenStoredMatchesCurrent(t *testing.T) {
	stubAliveCheck(t, true)
	shrinkRetryDelay(t)

	dir := t.TempDir()
	writeVersion(t, dir, "v0.4.2")

	scenario := &versionScenario{sessionPresent: true}
	mock := &MockCommander{RunFunc: scenario.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.EnsurePortalSaverVersion(client, dir, "v0.4.2"); err != nil {
		t.Fatalf("EnsurePortalSaverVersion returned error: %v", err)
	}

	if scenario.killSessionCalls != 0 {
		t.Errorf("expected 0 kill-session calls on version match, got %d", scenario.killSessionCalls)
	}
	if scenario.newSessionCalls != 0 {
		t.Errorf("expected 0 new-session calls on version match (session already alive), got %d", scenario.newSessionCalls)
	}
	if scenario.setOptionCalls != 1 {
		t.Errorf("expected exactly 1 set-option call (BootstrapPortalSaver still applies destroy-unattached off), got %d", scenario.setOptionCalls)
	}
}

func TestEnsurePortalSaverVersion_KillsAndRecreatesWhenStoredDiffersFromCurrent(t *testing.T) {
	stubAliveCheck(t, true)
	shrinkRetryDelay(t)

	dir := t.TempDir()
	writeVersion(t, dir, "v0.4.1")

	scenario := &versionScenario{sessionPresent: true}
	mock := &MockCommander{RunFunc: scenario.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.EnsurePortalSaverVersion(client, dir, "v0.4.2"); err != nil {
		t.Fatalf("EnsurePortalSaverVersion returned error: %v", err)
	}

	if scenario.killSessionCalls != 1 {
		t.Errorf("expected exactly 1 kill-session call on mismatch, got %d", scenario.killSessionCalls)
	}
	if scenario.newSessionCalls != 1 {
		t.Errorf("expected exactly 1 new-session call after kill, got %d", scenario.newSessionCalls)
	}
	if scenario.setOptionCalls != 1 {
		t.Errorf("expected exactly 1 set-option call, got %d", scenario.setOptionCalls)
	}

	// Order check: kill-session must precede new-session.
	killIdx, newIdx := -1, -1
	for i, c := range mock.Calls {
		switch c[0] {
		case "kill-session":
			if killIdx == -1 {
				killIdx = i
			}
		case "new-session":
			if newIdx == -1 {
				newIdx = i
			}
		}
	}
	if killIdx == -1 || newIdx == -1 || killIdx >= newIdx {
		t.Errorf("kill-session at %d must precede new-session at %d (calls: %v)", killIdx, newIdx, mock.Calls)
	}
}

func TestEnsurePortalSaverVersion_AlwaysRestartsWhenCurrentIsEmpty(t *testing.T) {
	stubAliveCheck(t, true)
	shrinkRetryDelay(t)

	dir := t.TempDir()
	writeVersion(t, dir, "v0.4.2")

	scenario := &versionScenario{sessionPresent: true}
	mock := &MockCommander{RunFunc: scenario.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.EnsurePortalSaverVersion(client, dir, ""); err != nil {
		t.Fatalf("EnsurePortalSaverVersion returned error: %v", err)
	}

	if scenario.killSessionCalls != 1 {
		t.Errorf("expected exactly 1 kill-session call when current version is empty, got %d", scenario.killSessionCalls)
	}
	if scenario.newSessionCalls != 1 {
		t.Errorf("expected exactly 1 new-session call after kill, got %d", scenario.newSessionCalls)
	}
}

func TestEnsurePortalSaverVersion_AlwaysRestartsWhenCurrentIsLiteralDev(t *testing.T) {
	stubAliveCheck(t, true)
	shrinkRetryDelay(t)

	dir := t.TempDir()
	writeVersion(t, dir, "v0.4.2")

	scenario := &versionScenario{sessionPresent: true}
	mock := &MockCommander{RunFunc: scenario.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.EnsurePortalSaverVersion(client, dir, "dev"); err != nil {
		t.Fatalf("EnsurePortalSaverVersion returned error: %v", err)
	}

	if scenario.killSessionCalls != 1 {
		t.Errorf("expected exactly 1 kill-session call when current version is \"dev\", got %d", scenario.killSessionCalls)
	}
	if scenario.newSessionCalls != 1 {
		t.Errorf("expected exactly 1 new-session call after kill, got %d", scenario.newSessionCalls)
	}
}

func TestEnsurePortalSaverVersion_TreatsStoredDevAsMismatch(t *testing.T) {
	stubAliveCheck(t, true)
	shrinkRetryDelay(t)

	dir := t.TempDir()
	writeVersion(t, dir, "dev")

	scenario := &versionScenario{sessionPresent: true}
	mock := &MockCommander{RunFunc: scenario.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.EnsurePortalSaverVersion(client, dir, "v0.4.2"); err != nil {
		t.Fatalf("EnsurePortalSaverVersion returned error: %v", err)
	}

	if scenario.killSessionCalls != 1 {
		t.Errorf("expected exactly 1 kill-session call when stored version is \"dev\", got %d", scenario.killSessionCalls)
	}
	if scenario.newSessionCalls != 1 {
		t.Errorf("expected exactly 1 new-session call after kill, got %d", scenario.newSessionCalls)
	}
}

func TestEnsurePortalSaverVersion_TreatsEmptyStoredVersionAsMismatch(t *testing.T) {
	stubAliveCheck(t, true)
	shrinkRetryDelay(t)

	dir := t.TempDir()
	// File exists but contains an empty string (post-trim).
	writeVersion(t, dir, "")

	scenario := &versionScenario{sessionPresent: true}
	mock := &MockCommander{RunFunc: scenario.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.EnsurePortalSaverVersion(client, dir, "v0.4.2"); err != nil {
		t.Fatalf("EnsurePortalSaverVersion returned error: %v", err)
	}

	if scenario.killSessionCalls != 1 {
		t.Errorf("expected exactly 1 kill-session call when stored version is empty, got %d", scenario.killSessionCalls)
	}
	if scenario.newSessionCalls != 1 {
		t.Errorf("expected exactly 1 new-session call after kill, got %d", scenario.newSessionCalls)
	}
}

func TestEnsurePortalSaverVersion_TreatsAbsentVersionFileAsMismatch(t *testing.T) {
	stubAliveCheck(t, true)
	shrinkRetryDelay(t)

	dir := t.TempDir() // no daemon.version pre-populated

	scenario := &versionScenario{sessionPresent: true}
	mock := &MockCommander{RunFunc: scenario.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.EnsurePortalSaverVersion(client, dir, "v0.4.2"); err != nil {
		t.Fatalf("EnsurePortalSaverVersion returned error: %v", err)
	}

	if scenario.killSessionCalls != 1 {
		t.Errorf("expected exactly 1 kill-session call when version file is absent, got %d", scenario.killSessionCalls)
	}
	if scenario.newSessionCalls != 1 {
		t.Errorf("expected exactly 1 new-session call after kill, got %d", scenario.newSessionCalls)
	}
}

func TestEnsurePortalSaverVersion_SkipsKillWhenNoSessionExists(t *testing.T) {
	stubAliveCheck(t, false) // irrelevant when session absent
	shrinkRetryDelay(t)

	dir := t.TempDir() // no daemon.version → mismatch=true

	scenario := &versionScenario{sessionPresent: false}
	mock := &MockCommander{RunFunc: scenario.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.EnsurePortalSaverVersion(client, dir, "v0.4.2"); err != nil {
		t.Fatalf("EnsurePortalSaverVersion returned error: %v", err)
	}

	if scenario.killSessionCalls != 0 {
		t.Errorf("expected 0 kill-session calls when no _portal-saver session exists, got %d", scenario.killSessionCalls)
	}
	if scenario.newSessionCalls != 1 {
		t.Errorf("expected BootstrapPortalSaver to create the session once, got %d new-session calls", scenario.newSessionCalls)
	}
	if scenario.setOptionCalls != 1 {
		t.Errorf("expected exactly 1 set-option call, got %d", scenario.setOptionCalls)
	}
}

func TestEnsurePortalSaverVersion_ToleratesKillSessionErrorForAbsentSession(t *testing.T) {
	stubAliveCheck(t, true) // session reported alive when probed by BootstrapPortalSaver
	shrinkRetryDelay(t)

	dir := t.TempDir()
	writeVersion(t, dir, "v0.4.1")

	scenario := &versionScenario{
		sessionPresent: true,
		killSessionErr: errors.New("can't find session: _portal-saver"),
	}
	mock := &MockCommander{RunFunc: scenario.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.EnsurePortalSaverVersion(client, dir, "v0.4.2"); err != nil {
		t.Fatalf("EnsurePortalSaverVersion must tolerate kill-session error, got: %v", err)
	}

	if scenario.killSessionCalls != 1 {
		t.Errorf("expected exactly 1 kill-session call, got %d", scenario.killSessionCalls)
	}
	// killSessionErr left sessionPresent=true in our stub; BootstrapPortalSaver
	// will then probe alive (true) and skip recreation, but must still apply the
	// defensive set-option.
	if scenario.setOptionCalls != 1 {
		t.Errorf("expected exactly 1 set-option call after tolerated kill error, got %d", scenario.setOptionCalls)
	}
}

func TestEnsurePortalSaverVersion_AlwaysInvokesBootstrapPortalSaver(t *testing.T) {
	stubAliveCheck(t, true)
	shrinkRetryDelay(t)

	dir := t.TempDir()
	writeVersion(t, dir, "v0.4.2") // match → no kill path

	var setOptionArgs []string
	scenario := &versionScenario{sessionPresent: true}
	mock := &MockCommander{
		RunFunc: func(args ...string) (string, error) {
			if len(args) > 0 && args[0] == "set-option" {
				setOptionArgs = append([]string{}, args...)
			}
			return scenario.run(t)(args...)
		},
	}
	client := tmux.NewClient(mock)

	if err := tmux.EnsurePortalSaverVersion(client, dir, "v0.4.2"); err != nil {
		t.Fatalf("EnsurePortalSaverVersion returned error: %v", err)
	}

	wantArgs := []string{"set-option", "-t", "_portal-saver", "destroy-unattached", "off"}
	if len(setOptionArgs) != len(wantArgs) {
		t.Fatalf("set-option argv = %v, want %v", setOptionArgs, wantArgs)
	}
	for i, arg := range wantArgs {
		if setOptionArgs[i] != arg {
			t.Errorf("set-option arg[%d] = %q, want %q", i, setOptionArgs[i], arg)
		}
	}
}

func TestEnsurePortalSaverVersion_DoesNotWriteDaemonVersionItself(t *testing.T) {
	stubAliveCheck(t, true)
	shrinkRetryDelay(t)

	dir := t.TempDir() // start with no daemon.version

	scenario := &versionScenario{sessionPresent: true}
	mock := &MockCommander{RunFunc: scenario.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.EnsurePortalSaverVersion(client, dir, "v0.4.2"); err != nil {
		t.Fatalf("EnsurePortalSaverVersion returned error: %v", err)
	}

	assertNoDaemonVersionFile(t, dir)
}

// ----------------------------------------------------------------------------
// killSaverAndWaitForDaemon tests
// ----------------------------------------------------------------------------

// recordingBarrierLogger captures Warn calls so assertions can verify emission
// counts and ordering. Satisfies tmux.BarrierLogger.
type recordingBarrierLogger struct {
	warns []string
}

func (r *recordingBarrierLogger) Warn(component, format string, args ...any) {
	r.warns = append(r.warns, component+" | "+format)
}

// installBarrierReadPID swaps the killBarrierReadPID seam for the duration of
// the test and restores it via t.Cleanup.
func installBarrierReadPID(t *testing.T, fn func(string) (int, error)) {
	t.Helper()
	seam := tmux.BarrierReadPIDSeam()
	prev := *seam
	*seam = fn
	t.Cleanup(func() { *seam = prev })
}

// installBarrierIsAlive swaps the killBarrierIsAlive seam for the test.
func installBarrierIsAlive(t *testing.T, fn func(int) bool) {
	t.Helper()
	seam := tmux.BarrierIsAliveSeam()
	prev := *seam
	*seam = fn
	t.Cleanup(func() { *seam = prev })
}

// installBarrierPollInterval shrinks the poll cadence for tests.
func installBarrierPollInterval(t *testing.T, d time.Duration) {
	t.Helper()
	seam := tmux.BarrierPollIntervalSeam()
	prev := *seam
	*seam = d
	t.Cleanup(func() { *seam = prev })
}

// installBarrierTimeout shrinks the total timeout for tests.
func installBarrierTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	seam := tmux.BarrierTimeoutSeam()
	prev := *seam
	*seam = d
	t.Cleanup(func() { *seam = prev })
}

// installBarrierLogger swaps the WARN-emission seam for a recorder.
func installBarrierLogger(t *testing.T, log tmux.BarrierLogger) {
	t.Helper()
	seam := tmux.BarrierLoggerSeam()
	prev := *seam
	*seam = log
	t.Cleanup(func() { *seam = prev })
}

// snapshotDir returns a map of every regular file in dir keyed by relative
// path with values "<mtime-unix-nano>|<size>|<content-hash>". Used to assert
// no state-directory mutation across a barrier invocation.
func snapshotDir(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%q): %v", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			t.Fatalf("Info(%q): %v", e.Name(), err)
		}
		// We don't hash content — mtime+size is sufficient for the helper's
		// "must not write" guarantee, and it side-steps reading PID files that
		// the test itself seeded.
		out[e.Name()] = info.ModTime().UTC().Format(time.RFC3339Nano) + "|" + strconv.FormatInt(info.Size(), 10)
	}
	return out
}

func TestKillSaverAndWaitForDaemon_ReturnsNilWithNoWarnWhenPriorPIDDiesBeforeTimeout(t *testing.T) {
	installBarrierPollInterval(t, 1*time.Millisecond)
	installBarrierTimeout(t, 500*time.Millisecond)
	installBarrierReadPID(t, func(string) (int, error) { return 4321, nil })

	// Alive for the first two probes (initial check + first tick), then dead.
	calls := 0
	installBarrierIsAlive(t, func(pid int) bool {
		calls++
		if pid != 4321 {
			t.Errorf("IsProcessAlive called with pid=%d; want 4321", pid)
		}
		return calls < 3
	})
	log := &recordingBarrierLogger{}
	installBarrierLogger(t, log)

	script := &portalSaverScript{
		killSession: func(call int) (string, error) { return "", nil },
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.KillSaverAndWaitForDaemon(client, t.TempDir()); err != nil {
		t.Fatalf("killSaverAndWaitForDaemon returned error: %v", err)
	}

	if got := countCalls(mock.Calls, "kill-session"); got != 1 {
		t.Errorf("expected exactly 1 kill-session call, got %d (calls: %v)", got, mock.Calls)
	}
	if len(log.warns) != 0 {
		t.Errorf("expected 0 WARN lines on clean exit, got %d: %v", len(log.warns), log.warns)
	}
}

func TestKillSaverAndWaitForDaemon_EmitsOneWarnAndReturnsNilWhenPriorPIDNeverDies(t *testing.T) {
	installBarrierPollInterval(t, 1*time.Millisecond)
	installBarrierTimeout(t, 20*time.Millisecond)
	installBarrierReadPID(t, func(string) (int, error) { return 4321, nil })
	installBarrierIsAlive(t, func(int) bool { return true })
	log := &recordingBarrierLogger{}
	installBarrierLogger(t, log)

	script := &portalSaverScript{
		killSession: func(call int) (string, error) { return "", nil },
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	start := time.Now()
	if err := tmux.KillSaverAndWaitForDaemon(client, t.TempDir()); err != nil {
		t.Fatalf("killSaverAndWaitForDaemon returned error: %v", err)
	}
	elapsed := time.Since(start)

	if got := countCalls(mock.Calls, "kill-session"); got != 1 {
		t.Errorf("expected exactly 1 kill-session call, got %d", got)
	}
	if len(log.warns) != 1 {
		t.Errorf("expected exactly 1 WARN line on timeout, got %d: %v", len(log.warns), log.warns)
	}
	// Wall time should be bounded by the timeout plus reasonable slack.
	if elapsed > 1*time.Second {
		t.Errorf("barrier exceeded wall-time budget: elapsed=%v (timeout=20ms)", elapsed)
	}
}

func TestKillSaverAndWaitForDaemon_SkipsPollingWhenPIDFileAbsent(t *testing.T) {
	installBarrierPollInterval(t, 1*time.Millisecond)
	installBarrierTimeout(t, 50*time.Millisecond)
	installBarrierReadPID(t, func(string) (int, error) { return 0, state.ErrPIDFileAbsent })

	aliveCalls := 0
	installBarrierIsAlive(t, func(int) bool {
		aliveCalls++
		return true
	})
	log := &recordingBarrierLogger{}
	installBarrierLogger(t, log)

	script := &portalSaverScript{
		killSession: func(call int) (string, error) { return "", nil },
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.KillSaverAndWaitForDaemon(client, t.TempDir()); err != nil {
		t.Fatalf("killSaverAndWaitForDaemon returned error: %v", err)
	}

	if got := countCalls(mock.Calls, "kill-session"); got != 1 {
		t.Errorf("expected exactly 1 kill-session call, got %d", got)
	}
	if aliveCalls != 0 {
		t.Errorf("expected 0 IsProcessAlive probes when PID file absent, got %d", aliveCalls)
	}
	if len(log.warns) != 0 {
		t.Errorf("expected 0 WARN lines when PID file absent, got %d: %v", len(log.warns), log.warns)
	}
}

func TestKillSaverAndWaitForDaemon_SkipsPollingWhenPIDFileCorrupted(t *testing.T) {
	installBarrierPollInterval(t, 1*time.Millisecond)
	installBarrierTimeout(t, 50*time.Millisecond)
	installBarrierReadPID(t, func(string) (int, error) {
		return 0, errors.New("parse daemon.pid: strconv.Atoi: parsing \"abc\": invalid syntax")
	})

	aliveCalls := 0
	installBarrierIsAlive(t, func(int) bool {
		aliveCalls++
		return true
	})
	log := &recordingBarrierLogger{}
	installBarrierLogger(t, log)

	script := &portalSaverScript{
		killSession: func(call int) (string, error) { return "", nil },
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.KillSaverAndWaitForDaemon(client, t.TempDir()); err != nil {
		t.Fatalf("killSaverAndWaitForDaemon returned error: %v", err)
	}

	if got := countCalls(mock.Calls, "kill-session"); got != 1 {
		t.Errorf("expected exactly 1 kill-session call, got %d", got)
	}
	if aliveCalls != 0 {
		t.Errorf("expected 0 IsProcessAlive probes on parse error, got %d", aliveCalls)
	}
	if len(log.warns) != 0 {
		t.Errorf("expected 0 WARN lines on parse error, got %d: %v", len(log.warns), log.warns)
	}
}

func TestKillSaverAndWaitForDaemon_SkipsPollingWhenPIDFileUnreadable(t *testing.T) {
	installBarrierPollInterval(t, 1*time.Millisecond)
	installBarrierTimeout(t, 50*time.Millisecond)
	installBarrierReadPID(t, func(string) (int, error) {
		return 0, errors.New("read daemon.pid: permission denied")
	})

	aliveCalls := 0
	installBarrierIsAlive(t, func(int) bool {
		aliveCalls++
		return true
	})
	log := &recordingBarrierLogger{}
	installBarrierLogger(t, log)

	script := &portalSaverScript{
		killSession: func(call int) (string, error) { return "", nil },
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.KillSaverAndWaitForDaemon(client, t.TempDir()); err != nil {
		t.Fatalf("killSaverAndWaitForDaemon returned error: %v", err)
	}

	if got := countCalls(mock.Calls, "kill-session"); got != 1 {
		t.Errorf("expected exactly 1 kill-session call, got %d", got)
	}
	if aliveCalls != 0 {
		t.Errorf("expected 0 IsProcessAlive probes on read error, got %d", aliveCalls)
	}
	if len(log.warns) != 0 {
		t.Errorf("expected 0 WARN lines on read error, got %d: %v", len(log.warns), log.warns)
	}
}

func TestKillSaverAndWaitForDaemon_SkipsPollingWhenPriorPIDAlreadyDead(t *testing.T) {
	installBarrierPollInterval(t, 1*time.Millisecond)
	installBarrierTimeout(t, 50*time.Millisecond)
	installBarrierReadPID(t, func(string) (int, error) { return 4321, nil })

	aliveCalls := 0
	installBarrierIsAlive(t, func(pid int) bool {
		aliveCalls++
		return false // already dead on the first probe
	})
	log := &recordingBarrierLogger{}
	installBarrierLogger(t, log)

	script := &portalSaverScript{
		killSession: func(call int) (string, error) { return "", nil },
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.KillSaverAndWaitForDaemon(client, t.TempDir()); err != nil {
		t.Fatalf("killSaverAndWaitForDaemon returned error: %v", err)
	}

	if got := countCalls(mock.Calls, "kill-session"); got != 1 {
		t.Errorf("expected exactly 1 kill-session call, got %d", got)
	}
	if aliveCalls != 1 {
		t.Errorf("expected exactly 1 IsProcessAlive probe (then short-circuit), got %d", aliveCalls)
	}
	if len(log.warns) != 0 {
		t.Errorf("expected 0 WARN lines when prior PID already dead, got %d: %v", len(log.warns), log.warns)
	}
}

func TestKillSaverAndWaitForDaemon_ToleratesFailingKillSession(t *testing.T) {
	installBarrierPollInterval(t, 1*time.Millisecond)
	installBarrierTimeout(t, 50*time.Millisecond)
	installBarrierReadPID(t, func(string) (int, error) { return 4321, nil })
	installBarrierIsAlive(t, func(int) bool { return false }) // already dead → fast path
	log := &recordingBarrierLogger{}
	installBarrierLogger(t, log)

	script := &portalSaverScript{
		killSession: func(call int) (string, error) {
			return "", errors.New("session vanished mid-flight")
		},
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.KillSaverAndWaitForDaemon(client, t.TempDir()); err != nil {
		t.Fatalf("killSaverAndWaitForDaemon must tolerate kill-session error, got: %v", err)
	}

	if got := countCalls(mock.Calls, "kill-session"); got != 1 {
		t.Errorf("expected exactly 1 kill-session call even when it errors, got %d", got)
	}
	if len(log.warns) != 0 {
		t.Errorf("expected 0 WARN lines on tolerated kill error, got %d: %v", len(log.warns), log.warns)
	}
}

func TestKillSaverAndWaitForDaemon_DoesNotMutateStateDirectory(t *testing.T) {
	installBarrierPollInterval(t, 1*time.Millisecond)
	installBarrierTimeout(t, 20*time.Millisecond)
	installBarrierReadPID(t, func(string) (int, error) { return 4321, nil })
	installBarrierIsAlive(t, func(int) bool { return true }) // force timeout path — exercises full code path

	log := &recordingBarrierLogger{}
	installBarrierLogger(t, log)

	dir := t.TempDir()
	// Seed a sentinel file so any spurious truncation/recreation is visible.
	sentinel := dir + "/sentinel"
	if err := os.WriteFile(sentinel, []byte("untouched\n"), 0o600); err != nil {
		t.Fatalf("seed sentinel: %v", err)
	}

	before := snapshotDir(t, dir)

	script := &portalSaverScript{
		killSession: func(call int) (string, error) { return "", nil },
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.KillSaverAndWaitForDaemon(client, dir); err != nil {
		t.Fatalf("killSaverAndWaitForDaemon returned error: %v", err)
	}

	after := snapshotDir(t, dir)
	if len(before) != len(after) {
		t.Errorf("state directory file count changed: before=%v after=%v", before, after)
	}
	for name, sigBefore := range before {
		if sigAfter, ok := after[name]; !ok {
			t.Errorf("file %q removed from state directory", name)
		} else if sigBefore != sigAfter {
			t.Errorf("file %q mutated: before=%q after=%q", name, sigBefore, sigAfter)
		}
	}
}
