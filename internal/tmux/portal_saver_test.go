package tmux_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
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

// assertKillBeforeNew scans calls for the first kill-session and first
// new-session arg-set and asserts the kill index precedes the new-session
// index. Fails the test if either command is missing or if order is reversed.
func assertKillBeforeNew(t *testing.T, calls [][]string) {
	t.Helper()
	killIdx, newIdx := -1, -1
	for i, c := range calls {
		if len(c) == 0 {
			continue
		}
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
		t.Errorf("kill-session at %d must precede new-session at %d (calls: %v)", killIdx, newIdx, calls)
	}
}

func TestAssertKillBeforeNew_PassesWhenKillPrecedesNew(t *testing.T) {
	stub := &testing.T{}
	calls := [][]string{
		{"has-session", "-t", "_portal-saver"},
		{"kill-session", "-t", "_portal-saver"},
		{"new-session", "-d", "-s", "_portal-saver"},
	}
	assertKillBeforeNew(stub, calls)
	if stub.Failed() {
		t.Errorf("expected no failure when kill precedes new, got Failed()=true")
	}
}

func TestAssertKillBeforeNew_FailsWhenKillMissing(t *testing.T) {
	stub := &testing.T{}
	calls := [][]string{
		{"has-session", "-t", "_portal-saver"},
		{"new-session", "-d", "-s", "_portal-saver"},
	}
	assertKillBeforeNew(stub, calls)
	if !stub.Failed() {
		t.Errorf("expected failure when kill-session is missing, got Failed()=false")
	}
}

func TestAssertKillBeforeNew_FailsWhenNewMissing(t *testing.T) {
	stub := &testing.T{}
	calls := [][]string{
		{"has-session", "-t", "_portal-saver"},
		{"kill-session", "-t", "_portal-saver"},
	}
	assertKillBeforeNew(stub, calls)
	if !stub.Failed() {
		t.Errorf("expected failure when new-session is missing, got Failed()=false")
	}
}

func TestAssertKillBeforeNew_FailsWhenNewPrecedesKill(t *testing.T) {
	stub := &testing.T{}
	calls := [][]string{
		{"new-session", "-d", "-s", "_portal-saver"},
		{"kill-session", "-t", "_portal-saver"},
	}
	assertKillBeforeNew(stub, calls)
	if !stub.Failed() {
		t.Errorf("expected failure when new-session precedes kill-session, got Failed()=false")
	}
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

	assertKillBeforeNew(t, mock.Calls)
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

	assertKillBeforeNew(t, mock.Calls)
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

// newVersionScenarioClient constructs the standard versionScenario / MockCommander /
// tmux.Client triplet used by most version-flow tests. Tests that need a custom
// RunFunc wrapper around scenario.run still construct the pieces inline.
func newVersionScenarioClient(t *testing.T, sessionPresent bool) (*versionScenario, *MockCommander, *tmux.Client) {
	t.Helper()
	scenario := &versionScenario{sessionPresent: sessionPresent}
	mock := &MockCommander{RunFunc: scenario.run(t)}
	return scenario, mock, tmux.NewClient(mock)
}

// recordBarrierCalls installs a kill-saver stub that increments a counter on
// every invocation and returns a pointer to the counter so callers can assert
// on the observed call count.
func recordBarrierCalls(t *testing.T) *int {
	t.Helper()
	calls := 0
	installKillSaverFn(t, func(*tmux.Client, string) error {
		calls++
		return nil
	})
	return &calls
}

// writeVersion seeds dir with daemon.version containing the supplied content.
func writeVersion(t *testing.T, dir, version string) {
	t.Helper()
	if err := state.WriteVersionFile(dir, version, nil); err != nil {
		t.Fatalf("WriteVersionFile(%q) returned error: %v", version, err)
	}
}

func TestEnsurePortalSaverVersion_DoesNotKillWhenStoredMatchesCurrent(t *testing.T) {
	stubAliveCheck(t, true)
	shrinkRetryDelay(t)

	dir := t.TempDir()
	writeVersion(t, dir, "v0.4.2")

	scenario, _, client := newVersionScenarioClient(t, true)

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

	scenario, mock, client := newVersionScenarioClient(t, true)

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

	assertKillBeforeNew(t, mock.Calls)
}

func TestEnsurePortalSaverVersion_AlwaysRestartsWhenCurrentIsEmpty(t *testing.T) {
	stubAliveCheck(t, true)
	shrinkRetryDelay(t)

	dir := t.TempDir()
	writeVersion(t, dir, "v0.4.2")

	scenario, _, client := newVersionScenarioClient(t, true)

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

	scenario, _, client := newVersionScenarioClient(t, true)

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

	scenario, _, client := newVersionScenarioClient(t, true)

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

	scenario, _, client := newVersionScenarioClient(t, true)

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

// NOTE: TestEnsurePortalSaverVersion_TreatsAbsentVersionFileAsMismatch was
// removed by Task 1-3. The old caller-layer contract treated an absent
// daemon.version as a mismatch and killed the saver session; the new
// contract gates the kill decision on BootstrapAliveCheck first, and the
// "alive+absent" row of the matrix is now a NO-KILL row. The current
// behaviour is pinned by TestEnsurePortalSaverVersion_Alive_AbsentVersionNeitherDev_DoesNotKill
// further down in this file.

func TestEnsurePortalSaverVersion_SkipsKillWhenNoSessionExists(t *testing.T) {
	stubAliveCheck(t, false) // irrelevant when session absent
	shrinkRetryDelay(t)

	dir := t.TempDir() // no daemon.version → mismatch=true

	scenario, _, client := newVersionScenarioClient(t, false)

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

// TestEnsurePortalSaverVersion_DoesNotWriteDaemonVersionOnKillPath asserts that
// the caller never writes daemon.version itself on the kill branches — the new
// daemon owns the file on its own startup. The defensive write introduced by
// Task 1-4 fires only on the alive+absent branch (covered separately by
// TestEnsurePortalSaverVersion_Alive_Absent_*); the kill-path contract that
// EnsurePortalSaverVersion does not touch daemon.version itself is preserved.
//
// We exercise the alive+stored-dev kill branch so the defensive write seam is
// NOT reachable but the kill is. A version-mismatch branch would work too;
// stored-dev is chosen because it leaves daemon.version on disk before the
// call so the post-condition pins "no rewrite" rather than "no write at all".
func TestEnsurePortalSaverVersion_DoesNotWriteDaemonVersionOnKillPath(t *testing.T) {
	stubAliveCheck(t, true)
	shrinkRetryDelay(t)

	dir := t.TempDir()
	writeVersion(t, dir, "dev") // stored-dev → kill path; daemon.version present

	// Pre-read so we can detect any subsequent rewrite.
	before, err := os.ReadFile(state.DaemonVersion(dir))
	if err != nil {
		t.Fatalf("read daemon.version: %v", err)
	}

	// Suppress the real kill barrier so we observe nothing but the caller's
	// own behaviour against the state directory.
	installKillSaverFn(t, func(*tmux.Client, string) error { return nil })

	_, _, client := newVersionScenarioClient(t, true)

	if err := tmux.EnsurePortalSaverVersion(client, dir, "v0.4.2"); err != nil {
		t.Fatalf("EnsurePortalSaverVersion returned error: %v", err)
	}

	after, err := os.ReadFile(state.DaemonVersion(dir))
	if err != nil {
		t.Fatalf("read daemon.version after call: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("EnsurePortalSaverVersion mutated daemon.version on kill path: before=%q after=%q", before, after)
	}
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

// swapSeam swaps the value at ptr to v for the duration of the test and
// restores the prior value via t.Cleanup. Centralises the install/restore
// pattern shared by the install* helpers below; LIFO cleanup ordering is
// preserved by t.Cleanup.
func swapSeam[T any](t *testing.T, ptr *T, v T) {
	t.Helper()
	prev := *ptr
	*ptr = v
	t.Cleanup(func() { *ptr = prev })
}

// installBarrierReadPID swaps the killBarrierReadPID seam for the duration of
// the test and restores it via t.Cleanup.
func installBarrierReadPID(t *testing.T, fn func(string) (int, error)) {
	t.Helper()
	swapSeam(t, tmux.BarrierReadPIDSeam(), fn)
}

// installBarrierIsAlive swaps the killBarrierIsAlive seam for the test.
func installBarrierIsAlive(t *testing.T, fn func(int) bool) {
	t.Helper()
	swapSeam(t, tmux.BarrierIsAliveSeam(), fn)
}

// installBarrierPollInterval shrinks the poll cadence for tests.
func installBarrierPollInterval(t *testing.T, d time.Duration) {
	t.Helper()
	swapSeam(t, tmux.BarrierPollIntervalSeam(), d)
}

// installBarrierTimeout shrinks the total timeout for tests.
func installBarrierTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	swapSeam(t, tmux.BarrierTimeoutSeam(), d)
}

// installBarrierLogger swaps the WARN-emission seam for a recorder.
func installBarrierLogger(t *testing.T, log tmux.BarrierLogger) {
	t.Helper()
	swapSeam(t, tmux.BarrierLoggerSeam(), log)
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

// ----------------------------------------------------------------------------
// Task 2.2: wire the kill-barrier helper into both production call sites.
// ----------------------------------------------------------------------------

// barrierCall records one invocation of the killSaverAndWaitForDaemonFn seam.
type barrierCall struct {
	client   *tmux.Client
	stateDir string
}

// installKillSaverFn swaps killSaverAndWaitForDaemonFn for the supplied
// function and restores the original via t.Cleanup.
func installKillSaverFn(t *testing.T, fn func(*tmux.Client, string) error) {
	t.Helper()
	swapSeam(t, tmux.KillSaverAndWaitForDaemonFnSeam(), fn)
}

func TestEnsurePortalSaverVersion_InvokesBarrierHelperOnVersionMismatch(t *testing.T) {
	stubAliveCheck(t, true)
	shrinkRetryDelay(t)

	dir := t.TempDir()
	writeVersion(t, dir, "v0.4.1")

	var calls []barrierCall
	installKillSaverFn(t, func(c *tmux.Client, sd string) error {
		calls = append(calls, barrierCall{client: c, stateDir: sd})
		return nil
	})

	_, mock, client := newVersionScenarioClient(t, true)

	if err := tmux.EnsurePortalSaverVersion(client, dir, "v0.4.2"); err != nil {
		t.Fatalf("EnsurePortalSaverVersion returned error: %v", err)
	}

	if len(calls) != 1 {
		t.Fatalf("expected exactly 1 barrier invocation on version mismatch, got %d", len(calls))
	}
	if calls[0].client != client {
		t.Errorf("barrier invoked with unexpected client: %p (want %p)", calls[0].client, client)
	}
	if calls[0].stateDir != dir {
		t.Errorf("barrier invoked with stateDir=%q, want %q", calls[0].stateDir, dir)
	}
	// With the helper stubbed the underlying KillSession on the mock must not
	// fire — only set-option (for destroy-unattached) is allowed.
	if got := countCalls(mock.Calls, "kill-session"); got != 0 {
		t.Errorf("expected 0 direct kill-session calls when helper is stubbed, got %d (calls: %v)", got, mock.Calls)
	}
}

func TestEnsurePortalSaverVersion_DoesNotInvokeBarrierHelperOnVersionMatch(t *testing.T) {
	stubAliveCheck(t, true)
	shrinkRetryDelay(t)

	dir := t.TempDir()
	writeVersion(t, dir, "v0.4.2")

	calls := 0
	installKillSaverFn(t, func(*tmux.Client, string) error {
		calls++
		return nil
	})

	_, _, client := newVersionScenarioClient(t, true)

	if err := tmux.EnsurePortalSaverVersion(client, dir, "v0.4.2"); err != nil {
		t.Fatalf("EnsurePortalSaverVersion returned error: %v", err)
	}

	if calls != 0 {
		t.Errorf("expected 0 barrier invocations on version match, got %d", calls)
	}
}

func TestBootstrapPortalSaver_InvokesBarrierHelperOnStaleDaemon(t *testing.T) {
	stubAliveCheck(t, false) // session present but daemon dead
	shrinkRetryDelay(t)

	dir := t.TempDir()

	var calls []barrierCall
	installKillSaverFn(t, func(c *tmux.Client, sd string) error {
		calls = append(calls, barrierCall{client: c, stateDir: sd})
		return nil
	})

	script := &portalSaverScript{
		hasSession: func(int) (string, error) { return "", nil }, // present
		newSession: func(int) (string, error) { return "", nil },
		setOption:  func(int) (string, error) { return "", nil },
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.BootstrapPortalSaver(client, dir); err != nil {
		t.Fatalf("BootstrapPortalSaver returned error: %v", err)
	}

	if len(calls) != 1 {
		t.Fatalf("expected exactly 1 barrier invocation on stale daemon, got %d", len(calls))
	}
	if calls[0].client != client {
		t.Errorf("barrier invoked with unexpected client: %p (want %p)", calls[0].client, client)
	}
	if calls[0].stateDir != dir {
		t.Errorf("barrier invoked with stateDir=%q, want %q", calls[0].stateDir, dir)
	}
	// Helper stubbed — KillSession must not be invoked through the mock.
	if got := countCalls(mock.Calls, "kill-session"); got != 0 {
		t.Errorf("expected 0 direct kill-session calls when helper is stubbed, got %d (calls: %v)", got, mock.Calls)
	}
}

func TestBootstrapPortalSaver_DoesNotInvokeBarrierHelperWhenSessionAbsent(t *testing.T) {
	stubAliveCheck(t, false) // irrelevant when absent
	shrinkRetryDelay(t)

	dir := t.TempDir()

	calls := 0
	installKillSaverFn(t, func(*tmux.Client, string) error {
		calls++
		return nil
	})

	script := &portalSaverScript{
		hasSession: func(int) (string, error) {
			return "", errors.New("can't find session: _portal-saver")
		},
		newSession: func(int) (string, error) { return "", nil },
		setOption:  func(int) (string, error) { return "", nil },
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.BootstrapPortalSaver(client, dir); err != nil {
		t.Fatalf("BootstrapPortalSaver returned error: %v", err)
	}

	if calls != 0 {
		t.Errorf("expected 0 barrier invocations when session absent, got %d", calls)
	}
}

func TestBootstrapPortalSaver_DoesNotInvokeBarrierHelperWhenDaemonAlive(t *testing.T) {
	stubAliveCheck(t, true)
	shrinkRetryDelay(t)

	dir := t.TempDir()

	calls := 0
	installKillSaverFn(t, func(*tmux.Client, string) error {
		calls++
		return nil
	})

	script := &portalSaverScript{
		hasSession: func(int) (string, error) { return "", nil }, // present
		setOption:  func(int) (string, error) { return "", nil },
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.BootstrapPortalSaver(client, dir); err != nil {
		t.Fatalf("BootstrapPortalSaver returned error: %v", err)
	}

	if calls != 0 {
		t.Errorf("expected 0 barrier invocations when daemon alive, got %d", calls)
	}
}

// TestBootstrapPortalSaver_PreservesKillSessionWhenRealHelperRuns confirms that
// when killSaverAndWaitForDaemonFn is left as the production helper and inner
// barrier seams put it on the fast path (no PID file), it still issues
// exactly one underlying KillSession on the stale-daemon branch.
func TestBootstrapPortalSaver_PreservesKillSessionWhenRealHelperRuns(t *testing.T) {
	stubAliveCheck(t, false)
	shrinkRetryDelay(t)

	// Fast-path the helper: pretend no PID file exists. The helper will call
	// KillSession once and return without polling.
	installBarrierReadPID(t, func(string) (int, error) { return 0, state.ErrPIDFileAbsent })

	dir := t.TempDir()

	script := &portalSaverScript{
		hasSession:  func(int) (string, error) { return "", nil }, // present
		killSession: func(int) (string, error) { return "", nil },
		newSession:  func(int) (string, error) { return "", nil },
		setOption:   func(int) (string, error) { return "", nil },
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.BootstrapPortalSaver(client, dir); err != nil {
		t.Fatalf("BootstrapPortalSaver returned error: %v", err)
	}

	if got := countCalls(mock.Calls, "kill-session"); got != 1 {
		t.Errorf("expected exactly 1 kill-session via real helper fast path, got %d (calls: %v)", got, mock.Calls)
	}
	if got := countCalls(mock.Calls, "new-session"); got != 1 {
		t.Errorf("expected 1 new-session call, got %d", got)
	}
}

// TestBootstrapPortalSaver_PreservesKillBeforeNewSessionOrderThroughBarrier
// confirms the helper-issued KillSession still precedes new-session on the
// stale-daemon branch.
func TestBootstrapPortalSaver_PreservesKillBeforeNewSessionOrderThroughBarrier(t *testing.T) {
	stubAliveCheck(t, false)
	shrinkRetryDelay(t)
	installBarrierReadPID(t, func(string) (int, error) { return 0, state.ErrPIDFileAbsent })

	dir := t.TempDir()

	script := &portalSaverScript{
		hasSession:  func(int) (string, error) { return "", nil }, // present
		killSession: func(int) (string, error) { return "", nil },
		newSession:  func(int) (string, error) { return "", nil },
		setOption:   func(int) (string, error) { return "", nil },
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.BootstrapPortalSaver(client, dir); err != nil {
		t.Fatalf("BootstrapPortalSaver returned error: %v", err)
	}

	assertKillBeforeNew(t, mock.Calls)
}

// TestBootstrapPortalSaver_ToleratesBarrierWarnOnTimeoutPath confirms that
// when the real helper hits its WARN-on-timeout branch, BootstrapPortalSaver
// still returns nil and continues to recreate the saver session.
func TestBootstrapPortalSaver_ToleratesBarrierWarnOnTimeoutPath(t *testing.T) {
	stubAliveCheck(t, false)
	shrinkRetryDelay(t)
	installBarrierReadPID(t, func(string) (int, error) { return 4321, nil })
	installBarrierIsAlive(t, func(int) bool { return true }) // never dies → timeout
	installBarrierPollInterval(t, 1*time.Millisecond)
	installBarrierTimeout(t, 10*time.Millisecond)
	log := &recordingBarrierLogger{}
	installBarrierLogger(t, log)

	dir := t.TempDir()

	script := &portalSaverScript{
		hasSession:  func(int) (string, error) { return "", nil }, // present
		killSession: func(int) (string, error) { return "", nil },
		newSession:  func(int) (string, error) { return "", nil },
		setOption:   func(int) (string, error) { return "", nil },
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.BootstrapPortalSaver(client, dir); err != nil {
		t.Fatalf("BootstrapPortalSaver must tolerate barrier WARN-on-timeout, got: %v", err)
	}

	if len(log.warns) != 1 {
		t.Errorf("expected exactly 1 WARN on timeout, got %d: %v", len(log.warns), log.warns)
	}
	if got := countCalls(mock.Calls, "new-session"); got != 1 {
		t.Errorf("expected new-session to proceed after barrier timeout, got %d new-session calls", got)
	}
}

// TestSetBarrierLogger_RoutesWarnOnTimeoutThroughInstalledLogger asserts that
// the exported SetBarrierLogger setter installs the supplied logger such that
// barrier WARN emissions reach it. Guards against the no-op default
// persisting after production wiring runs.
func TestSetBarrierLogger_RoutesWarnOnTimeoutThroughInstalledLogger(t *testing.T) {
	// Capture and restore the package-level seam directly so SetBarrierLogger
	// is exercised as the install path.
	loggerSeam := tmux.BarrierLoggerSeam()
	prevLogger := *loggerSeam
	t.Cleanup(func() { *loggerSeam = prevLogger })

	recorder := &recordingBarrierLogger{}
	tmux.SetBarrierLogger(recorder)

	installBarrierPollInterval(t, 1*time.Millisecond)
	installBarrierTimeout(t, 10*time.Millisecond)
	installBarrierReadPID(t, func(string) (int, error) { return 4321, nil })
	installBarrierIsAlive(t, func(int) bool { return true }) // never dies → timeout

	script := &portalSaverScript{
		killSession: func(int) (string, error) { return "", nil },
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.KillSaverAndWaitForDaemon(client, t.TempDir()); err != nil {
		t.Fatalf("KillSaverAndWaitForDaemon returned error: %v", err)
	}

	if len(recorder.warns) != 1 {
		t.Fatalf("expected exactly 1 WARN routed through SetBarrierLogger, got %d: %v", len(recorder.warns), recorder.warns)
	}
	// WARN must land under ComponentBootstrap. recordingBarrierLogger encodes
	// the component as the prefix before " | " in each captured warn.
	if !strings.HasPrefix(recorder.warns[0], state.ComponentBootstrap+" | ") {
		t.Errorf("WARN component prefix = %q, want %q", recorder.warns[0], state.ComponentBootstrap+" | ")
	}
}

// TestSetBarrierLogger_IgnoresNilLogger asserts that calling
// SetBarrierLogger(nil) leaves the previously-installed logger in place.
// Guards against an accidental nil-overwrite stripping the production sink.
func TestSetBarrierLogger_IgnoresNilLogger(t *testing.T) {
	loggerSeam := tmux.BarrierLoggerSeam()
	prevLogger := *loggerSeam
	t.Cleanup(func() { *loggerSeam = prevLogger })

	recorder := &recordingBarrierLogger{}
	tmux.SetBarrierLogger(recorder)
	tmux.SetBarrierLogger(nil) // must be a no-op

	if *loggerSeam != tmux.BarrierLogger(recorder) {
		t.Errorf("SetBarrierLogger(nil) overwrote the previously installed logger")
	}
}

// ----------------------------------------------------------------------------
// Task 1-3: alive-check-first ordering inside EnsurePortalSaverVersion.
//
// The kill decision must consult BootstrapAliveCheck(stateDir) before the
// version-mismatch predicate. These tests pin the six-row decision matrix
// end-to-end (caller-layer, not the predicate in isolation).
// ----------------------------------------------------------------------------

// installReadVersionFile swaps the portalSaverReadVersionFile seam for the
// duration of the test and restores it via t.Cleanup. Used to drive the
// non-absent I/O-error and call-ordering branches without filesystem fixtures.
func installReadVersionFile(t *testing.T, fn func(string) (string, error)) {
	t.Helper()
	swapSeam(t, tmux.PortalSaverReadVersionFileSeam(), fn)
}

func TestEnsurePortalSaverVersion_NotAlive_AbsentVersion_DoesNotKill(t *testing.T) {
	stubAliveCheck(t, false)
	shrinkRetryDelay(t)

	dir := t.TempDir() // no daemon.version pre-populated

	barrierCalls := recordBarrierCalls(t)

	// Session present so HasSession would historically permit the kill — the
	// alive-check gate is what suppresses it under the new contract.
	_, _, client := newVersionScenarioClient(t, true)

	if err := tmux.EnsurePortalSaverVersion(client, dir, "v0.4.2"); err != nil {
		t.Fatalf("EnsurePortalSaverVersion returned error: %v", err)
	}

	// EnsurePortalSaverVersion itself must not drive a kill when the daemon
	// is not alive. BootstrapPortalSaver may still invoke the barrier on its
	// own stale-daemon branch (session present + alive-check false) — that is
	// BootstrapPortalSaver's contract, not this caller's. We pin the caller's
	// contract by asserting at most one barrier call from the union of both.
	if *barrierCalls > 1 {
		t.Errorf("expected at most 1 barrier invocation when daemon not alive (only BootstrapPortalSaver's stale-daemon branch), got %d", *barrierCalls)
	}
}

func TestEnsurePortalSaverVersion_NotAlive_VersionMismatch_DoesNotKill(t *testing.T) {
	stubAliveCheck(t, false)
	shrinkRetryDelay(t)

	dir := t.TempDir()
	writeVersion(t, dir, "v0.4.1") // mismatches current; neither dev

	barrierCalls := recordBarrierCalls(t)

	// Session present so HasSession would historically permit the kill — the
	// alive-check gate is what suppresses it under the new contract.
	_, _, client := newVersionScenarioClient(t, true)

	if err := tmux.EnsurePortalSaverVersion(client, dir, "v0.4.2"); err != nil {
		t.Fatalf("EnsurePortalSaverVersion returned error: %v", err)
	}

	// EnsurePortalSaverVersion itself must not drive a kill when the daemon
	// is not alive, regardless of version mismatch. BootstrapPortalSaver may
	// still invoke the barrier on its own stale-daemon branch. We pin the
	// caller's contract by asserting at most one barrier call across both.
	if *barrierCalls > 1 {
		t.Errorf("expected at most 1 barrier invocation when daemon not alive (BootstrapPortalSaver's branch only), got %d", *barrierCalls)
	}
}

func TestEnsurePortalSaverVersion_Alive_StoredDev_Kills(t *testing.T) {
	stubAliveCheck(t, true)
	shrinkRetryDelay(t)

	dir := t.TempDir()
	writeVersion(t, dir, "dev")

	barrierCalls := recordBarrierCalls(t)

	_, _, client := newVersionScenarioClient(t, true)

	if err := tmux.EnsurePortalSaverVersion(client, dir, "v0.4.2"); err != nil {
		t.Fatalf("EnsurePortalSaverVersion returned error: %v", err)
	}

	if *barrierCalls != 1 {
		t.Errorf("expected exactly 1 barrier invocation on stored=dev, got %d", *barrierCalls)
	}
}

func TestEnsurePortalSaverVersion_Alive_CurrentDev_Kills(t *testing.T) {
	stubAliveCheck(t, true)
	shrinkRetryDelay(t)

	dir := t.TempDir()
	writeVersion(t, dir, "v0.4.2")

	barrierCalls := recordBarrierCalls(t)

	_, _, client := newVersionScenarioClient(t, true)

	if err := tmux.EnsurePortalSaverVersion(client, dir, "dev"); err != nil {
		t.Fatalf("EnsurePortalSaverVersion returned error: %v", err)
	}

	if *barrierCalls != 1 {
		t.Errorf("expected exactly 1 barrier invocation on current=dev, got %d", *barrierCalls)
	}
}

func TestEnsurePortalSaverVersion_Alive_AbsentVersionNeitherDev_DoesNotKill(t *testing.T) {
	stubAliveCheck(t, true)
	shrinkRetryDelay(t)

	dir := t.TempDir() // no daemon.version → ErrVersionFileAbsent

	barrierCalls := recordBarrierCalls(t)

	scenario, _, client := newVersionScenarioClient(t, true)

	if err := tmux.EnsurePortalSaverVersion(client, dir, "v0.4.2"); err != nil {
		t.Fatalf("EnsurePortalSaverVersion returned error: %v", err)
	}

	if *barrierCalls != 0 {
		t.Errorf("expected 0 barrier invocations on alive+absent (neither dev), got %d", *barrierCalls)
	}
	if scenario.killSessionCalls != 0 {
		t.Errorf("expected 0 kill-session calls on alive+absent (neither dev), got %d", scenario.killSessionCalls)
	}
	if scenario.setOptionCalls != 1 {
		t.Errorf("expected exactly 1 set-option call (BootstrapPortalSaver still runs), got %d", scenario.setOptionCalls)
	}
}

func TestEnsurePortalSaverVersion_Alive_NonAbsentReadError_Kills(t *testing.T) {
	stubAliveCheck(t, true)
	shrinkRetryDelay(t)

	installReadVersionFile(t, func(string) (string, error) {
		return "", fs.ErrPermission
	})

	dir := t.TempDir()

	barrierCalls := recordBarrierCalls(t)

	_, _, client := newVersionScenarioClient(t, true)

	if err := tmux.EnsurePortalSaverVersion(client, dir, "v0.4.2"); err != nil {
		t.Fatalf("EnsurePortalSaverVersion returned error: %v", err)
	}

	if *barrierCalls != 1 {
		t.Errorf("expected exactly 1 barrier invocation on alive+non-absent-read-error, got %d", *barrierCalls)
	}
}

func TestEnsurePortalSaverVersion_Alive_VersionsMatch_DoesNotKill(t *testing.T) {
	stubAliveCheck(t, true)
	shrinkRetryDelay(t)

	dir := t.TempDir()
	writeVersion(t, dir, "v0.4.2")

	barrierCalls := recordBarrierCalls(t)

	_, _, client := newVersionScenarioClient(t, true)

	if err := tmux.EnsurePortalSaverVersion(client, dir, "v0.4.2"); err != nil {
		t.Fatalf("EnsurePortalSaverVersion returned error: %v", err)
	}

	if *barrierCalls != 0 {
		t.Errorf("expected 0 barrier invocations on alive+match (neither dev), got %d", *barrierCalls)
	}
}

func TestEnsurePortalSaverVersion_Alive_VersionsMismatch_Kills(t *testing.T) {
	stubAliveCheck(t, true)
	shrinkRetryDelay(t)

	dir := t.TempDir()
	writeVersion(t, dir, "v0.4.1")

	barrierCalls := recordBarrierCalls(t)

	_, _, client := newVersionScenarioClient(t, true)

	if err := tmux.EnsurePortalSaverVersion(client, dir, "v0.4.2"); err != nil {
		t.Fatalf("EnsurePortalSaverVersion returned error: %v", err)
	}

	if *barrierCalls != 1 {
		t.Errorf("expected exactly 1 barrier invocation on alive+mismatch (neither dev), got %d", *barrierCalls)
	}
}

// TestEnsurePortalSaverVersion_ConsultsAliveCheckBeforeVersionMismatchDecision
// asserts the new ordering invariant: BootstrapAliveCheck is invoked before
// any version-mismatch verdict drives a kill. The test runs the same
// version-mismatch fixture twice — once with alive=false (kill suppressed)
// and once with alive=true (kill fires) — proving the alive-check gates the
// predicate's verdict in the caller.
func TestEnsurePortalSaverVersion_ConsultsAliveCheckBeforeVersionMismatchDecision(t *testing.T) {
	shrinkRetryDelay(t)

	var aliveCalls int
	prevAlive := tmux.BootstrapAliveCheck
	tmux.BootstrapAliveCheck = func(string) bool {
		aliveCalls++
		return false // not alive
	}
	t.Cleanup(func() { tmux.BootstrapAliveCheck = prevAlive })

	installReadVersionFile(t, func(string) (string, error) {
		// Drive a "mismatch" verdict at the predicate layer to prove the
		// alive-check gate is what suppresses the kill.
		return "v0.4.1", nil
	})

	barrierCalls := recordBarrierCalls(t)

	dir := t.TempDir()
	// sessionPresent=false so BootstrapPortalSaver's own stale-daemon branch
	// does not also invoke the barrier — isolates the assertion to
	// EnsurePortalSaverVersion's call site only.
	_, _, client := newVersionScenarioClient(t, false)

	if err := tmux.EnsurePortalSaverVersion(client, dir, "v0.4.2"); err != nil {
		t.Fatalf("EnsurePortalSaverVersion returned error: %v", err)
	}

	if *barrierCalls != 0 {
		t.Errorf("expected 0 barrier invocations when alive-check returns false (regardless of version mismatch), got %d", *barrierCalls)
	}
	if aliveCalls == 0 {
		t.Errorf("BootstrapAliveCheck was never consulted")
	}

	// Now flip alive=true on the same fixture and confirm the same
	// mismatching version drives exactly one kill — proving the predicate
	// verdict is gated by the alive-check.
	tmux.BootstrapAliveCheck = func(string) bool { return true }
	scenario2 := &versionScenario{sessionPresent: true}
	mock2 := &MockCommander{RunFunc: scenario2.run(t)}
	client2 := tmux.NewClient(mock2)
	if err := tmux.EnsurePortalSaverVersion(client2, dir, "v0.4.2"); err != nil {
		t.Fatalf("EnsurePortalSaverVersion (alive=true) returned error: %v", err)
	}
	if *barrierCalls != 1 {
		t.Errorf("expected exactly 1 barrier invocation on alive=true + mismatch, got %d", *barrierCalls)
	}
}

// TestShouldKillSaverOnVersionDecision_PredicateMatrix pins the truth matrix
// of shouldKillSaverOnVersionDecision — the single predicate encoding the
// alive-daemon kill-decision rules consulted by EnsurePortalSaverVersion.
//
// Framing: the predicate's verdict is the kill-decision on the alive-daemon
// branch only. EnsurePortalSaverVersion consults BootstrapAliveCheck FIRST;
// this predicate is only consulted when the daemon is known to be alive. The
// `absent → false` row reflects the load-bearing "don't recycle on missing
// version file" rule — the caller layers a defensive WriteVersionFile on that
// branch (Task 1-4) rather than killing. See EnsurePortalSaverVersion in
// internal/tmux/portal_saver.go and the ordering tests on that caller for the
// authoritative kill-decision contract.
//
// Each row is driven directly through the test-only re-export
// tmux.ShouldKillSaverOnVersionDecision (no caller, no tmux mock) so the
// table asserts predicate behaviour in isolation. Rows mirror spec
// §Testing Requirements (saver-kill-respawn-loop-leaks-daemons).
func TestShouldKillSaverOnVersionDecision_PredicateMatrix(t *testing.T) {
	cases := []struct {
		name           string
		stored         string
		currentVersion string
		readErr        error
		want           bool
	}{
		{
			// Equal non-dev versions on a clean read: no kill.
			name:           "equal_non_dev_match",
			stored:         "0.5.0",
			currentVersion: "0.5.0",
			readErr:        nil,
			want:           false,
		},
		{
			// Genuine version mismatch on a clean read with neither side
			// dev/empty: kill.
			name:           "mismatched_non_dev",
			stored:         "0.5.0",
			currentVersion: "0.5.1",
			readErr:        nil,
			want:           true,
		},
		{
			// Load-bearing row: under the unified predicate, an absent
			// version file does NOT trigger a kill on the alive-daemon
			// branch. The caller repairs the file defensively via
			// portalSaverWriteVersionFile rather than recycling the daemon
			// (Task 1-4). The prior parallel predicate returned true here;
			// pinning false explicitly prevents silent regression.
			name:           "readErr_ErrVersionFileAbsent_no_kill",
			stored:         "",
			currentVersion: "0.5.0",
			readErr:        state.ErrVersionFileAbsent,
			want:           false,
		},
		{
			// Non-absent I/O error (e.g. permission denied): conservative
			// kill. Distinct row from the absent case above because the
			// caller treats the two error shapes differently — absent is
			// repaired in place, non-absent escalates to a recycle.
			name:           "readErr_non_absent_io_error",
			stored:         "",
			currentVersion: "0.5.0",
			readErr:        fs.ErrPermission,
			want:           true,
		},
		{
			// Dev short-circuit on the stored side. Gated on readErr == nil
			// (an unreadable file is not "stored is empty"). With a clean
			// read, stored="dev" triggers kill regardless of current.
			name:           "dev_version_stored",
			stored:         "dev",
			currentVersion: "0.5.0",
			readErr:        nil,
			want:           true,
		},
		{
			// Dev short-circuit on the current side. currentVersion always
			// counts regardless of readErr.
			name:           "dev_version_current",
			stored:         "0.5.0",
			currentVersion: "dev",
			readErr:        nil,
			want:           true,
		},
		{
			// Empty-stored short-circuit on a clean read. Same dev-rule
			// branch as stored="dev" — both shapes collapse to kill at the
			// predicate layer.
			name:           "empty_stored",
			stored:         "",
			currentVersion: "0.5.0",
			readErr:        nil,
			want:           true,
		},
		{
			// Empty-current short-circuit. currentVersion=="" counts
			// regardless of readErr.
			name:           "empty_current",
			stored:         "0.5.0",
			currentVersion: "",
			readErr:        nil,
			want:           true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tmux.ShouldKillSaverOnVersionDecision(tc.stored, tc.currentVersion, tc.readErr)
			if got != tc.want {
				t.Errorf("shouldKillSaverOnVersionDecision(stored=%q, current=%q, readErr=%v) = %v; want %v",
					tc.stored, tc.currentVersion, tc.readErr, got, tc.want)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// Task 1-4: defensive WriteVersionFile on the alive+absent branch.
//
// EnsurePortalSaverVersion must call portalSaverWriteVersionFile(stateDir,
// currentVersion) on (alive=true, errors.Is(readErr, ErrVersionFileAbsent))
// BEFORE BootstrapPortalSaver is invoked, and only on that one branch. A
// write error must propagate out (wrapped) and prevent BootstrapPortalSaver
// from being called.
// ----------------------------------------------------------------------------

// installWriteVersionFile swaps the portalSaverWriteVersionFile seam for the
// duration of the test and restores it via t.Cleanup.
func installWriteVersionFile(t *testing.T, fn func(string, string) error) {
	t.Helper()
	swapSeam(t, tmux.PortalSaverWriteVersionFileSeam(), fn)
}

// defensiveWriteCall records one invocation of portalSaverWriteVersionFile.
type defensiveWriteCall struct {
	dir     string
	version string
}

func TestEnsurePortalSaverVersion_Alive_Absent_InvokesDefensiveWriteBeforeBootstrap(t *testing.T) {
	stubAliveCheck(t, true)
	shrinkRetryDelay(t)

	dir := t.TempDir() // no daemon.version → ErrVersionFileAbsent

	// Record the call order across the defensive write and BootstrapPortalSaver
	// (which goes through the tmux mock — has-session is the first call
	// BootstrapPortalSaver makes).
	var order []string
	var writes []defensiveWriteCall
	installWriteVersionFile(t, func(d, v string) error {
		order = append(order, "write")
		writes = append(writes, defensiveWriteCall{dir: d, version: v})
		return nil
	})

	scenario := &versionScenario{sessionPresent: true}
	mock := &MockCommander{
		RunFunc: func(args ...string) (string, error) {
			if len(args) > 0 {
				order = append(order, args[0])
			}
			return scenario.run(t)(args...)
		},
	}
	client := tmux.NewClient(mock)

	if err := tmux.EnsurePortalSaverVersion(client, dir, "v0.4.2"); err != nil {
		t.Fatalf("EnsurePortalSaverVersion returned error: %v", err)
	}

	if len(writes) != 1 {
		t.Fatalf("expected exactly 1 defensive write, got %d", len(writes))
	}
	if writes[0].dir != dir {
		t.Errorf("defensive write dir = %q, want %q", writes[0].dir, dir)
	}
	if writes[0].version != "v0.4.2" {
		t.Errorf("defensive write version = %q, want %q", writes[0].version, "v0.4.2")
	}

	// The defensive write must happen BEFORE BootstrapPortalSaver runs.
	// BootstrapPortalSaver's first observable action through the mock is
	// has-session, so "write" must precede the first "has-session" in order.
	writeIdx, hasSessionIdx := -1, -1
	for i, op := range order {
		if op == "write" && writeIdx == -1 {
			writeIdx = i
		}
		if op == "has-session" && hasSessionIdx == -1 {
			hasSessionIdx = i
		}
	}
	if writeIdx == -1 {
		t.Fatalf("defensive write never recorded; order=%v", order)
	}
	if hasSessionIdx == -1 {
		t.Fatalf("has-session never recorded; BootstrapPortalSaver may not have been invoked; order=%v", order)
	}
	if writeIdx >= hasSessionIdx {
		t.Errorf("defensive write at %d must precede BootstrapPortalSaver's first has-session at %d (order=%v)", writeIdx, hasSessionIdx, order)
	}
}

func TestEnsurePortalSaverVersion_Alive_Absent_DefensiveWriteErrorPropagatesAndSkipsBootstrap(t *testing.T) {
	stubAliveCheck(t, true)
	shrinkRetryDelay(t)

	dir := t.TempDir() // no daemon.version → ErrVersionFileAbsent

	sentinel := errors.New("read-only filesystem")
	installWriteVersionFile(t, func(string, string) error {
		return sentinel
	})

	// Mock that fatals on any call — BootstrapPortalSaver MUST NOT run when
	// the defensive write fails.
	mock := &MockCommander{
		RunFunc: func(args ...string) (string, error) {
			t.Fatalf("BootstrapPortalSaver was invoked despite defensive write failure: %v", args)
			return "", nil
		},
	}
	client := tmux.NewClient(mock)

	err := tmux.EnsurePortalSaverVersion(client, dir, "v0.4.2")
	if err == nil {
		t.Fatalf("EnsurePortalSaverVersion returned nil; want wrapped defensive-write error")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("returned error %v does not wrap sentinel %v", err, sentinel)
	}
}

func TestEnsurePortalSaverVersion_Alive_Match_DoesNotInvokeDefensiveWrite(t *testing.T) {
	stubAliveCheck(t, true)
	shrinkRetryDelay(t)

	dir := t.TempDir()
	writeVersion(t, dir, "v0.4.2")

	writeCalls := 0
	installWriteVersionFile(t, func(string, string) error {
		writeCalls++
		return nil
	})

	_, _, client := newVersionScenarioClient(t, true)

	if err := tmux.EnsurePortalSaverVersion(client, dir, "v0.4.2"); err != nil {
		t.Fatalf("EnsurePortalSaverVersion returned error: %v", err)
	}

	if writeCalls != 0 {
		t.Errorf("expected 0 defensive write calls on alive+match, got %d", writeCalls)
	}
}

func TestEnsurePortalSaverVersion_Alive_MismatchNeitherDev_DoesNotInvokeDefensiveWrite(t *testing.T) {
	stubAliveCheck(t, true)
	shrinkRetryDelay(t)

	dir := t.TempDir()
	writeVersion(t, dir, "v0.4.1")

	writeCalls := 0
	installWriteVersionFile(t, func(string, string) error {
		writeCalls++
		return nil
	})

	barrierCalls := recordBarrierCalls(t)

	_, _, client := newVersionScenarioClient(t, true)

	if err := tmux.EnsurePortalSaverVersion(client, dir, "v0.4.2"); err != nil {
		t.Fatalf("EnsurePortalSaverVersion returned error: %v", err)
	}

	if writeCalls != 0 {
		t.Errorf("expected 0 defensive write calls on alive+mismatch, got %d", writeCalls)
	}
	if *barrierCalls != 1 {
		t.Errorf("expected exactly 1 barrier invocation on alive+mismatch, got %d", *barrierCalls)
	}
}

func TestEnsurePortalSaverVersion_Alive_StoredDev_DoesNotInvokeDefensiveWrite(t *testing.T) {
	stubAliveCheck(t, true)
	shrinkRetryDelay(t)

	dir := t.TempDir()
	writeVersion(t, dir, "dev")

	writeCalls := 0
	installWriteVersionFile(t, func(string, string) error {
		writeCalls++
		return nil
	})

	barrierCalls := recordBarrierCalls(t)

	_, _, client := newVersionScenarioClient(t, true)

	if err := tmux.EnsurePortalSaverVersion(client, dir, "v0.4.2"); err != nil {
		t.Fatalf("EnsurePortalSaverVersion returned error: %v", err)
	}

	if writeCalls != 0 {
		t.Errorf("expected 0 defensive write calls on alive+stored-dev, got %d", writeCalls)
	}
	if *barrierCalls != 1 {
		t.Errorf("expected exactly 1 barrier invocation on alive+stored-dev, got %d", *barrierCalls)
	}
}

func TestEnsurePortalSaverVersion_Alive_CurrentDev_DoesNotInvokeDefensiveWrite(t *testing.T) {
	stubAliveCheck(t, true)
	shrinkRetryDelay(t)

	dir := t.TempDir()
	writeVersion(t, dir, "v0.4.2")

	writeCalls := 0
	installWriteVersionFile(t, func(string, string) error {
		writeCalls++
		return nil
	})

	barrierCalls := recordBarrierCalls(t)

	_, _, client := newVersionScenarioClient(t, true)

	if err := tmux.EnsurePortalSaverVersion(client, dir, "dev"); err != nil {
		t.Fatalf("EnsurePortalSaverVersion returned error: %v", err)
	}

	if writeCalls != 0 {
		t.Errorf("expected 0 defensive write calls on alive+current-dev, got %d", writeCalls)
	}
	if *barrierCalls != 1 {
		t.Errorf("expected exactly 1 barrier invocation on alive+current-dev, got %d", *barrierCalls)
	}
}

// TestSetVersionWriterLogger_BootstrapWrapperEmitsDebugBreadcrumb pins
// spec § Change 3 / Acceptance Criterion #9: every state.WriteVersionFile
// call emits one DEBUG breadcrumb prefixed "daemon.version write:" containing
// version, caller pid, and destination path — including the bootstrap-side
// defensive call. Guards against the wrapper reverting to passing nil.
func TestSetVersionWriterLogger_BootstrapWrapperEmitsDebugBreadcrumb(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "portal.log")
	t.Setenv("PORTAL_LOG_LEVEL", "debug")

	lg, err := state.OpenLogger(logPath, false)
	if err != nil {
		t.Fatalf("OpenLogger: %v", err)
	}
	t.Cleanup(func() { _ = lg.Close() })

	// Save and restore the prior package-level logger sink via the test seam
	// so this test cannot leak its capturing logger into siblings.
	loggerSeam := tmux.VersionWriterLoggerSeam()
	prev := *loggerSeam
	t.Cleanup(func() { *loggerSeam = prev })

	tmux.SetVersionWriterLogger(lg)

	// Invoke the production bootstrap wrapper through the seam. In a fresh
	// test process this is the original `portalSaverWriteVersionFile`
	// function; no other test in this file mutates the seam at package-init
	// time.
	wrapper := *tmux.PortalSaverWriteVersionFileSeam()
	if err := wrapper(dir, "v9.9.9"); err != nil {
		t.Fatalf("portalSaverWriteVersionFile: %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	log := string(data)

	if count := strings.Count(log, "daemon.version write:"); count != 1 {
		t.Fatalf("expected exactly 1 'daemon.version write:' breadcrumb, got %d. log:\n%s", count, log)
	}
	if !strings.Contains(log, "| DEBUG |") {
		t.Errorf("breadcrumb not DEBUG level:\n%s", log)
	}
	if !strings.Contains(log, "| "+state.ComponentDaemon+" |") {
		t.Errorf("breadcrumb component != %q:\n%s", state.ComponentDaemon, log)
	}
	if !strings.Contains(log, "version=v9.9.9") {
		t.Errorf("breadcrumb missing version token:\n%s", log)
	}
	wantPID := "pid=" + strconv.Itoa(os.Getpid())
	if !strings.Contains(log, wantPID) {
		t.Errorf("breadcrumb missing %q:\n%s", wantPID, log)
	}
	wantPath := "path=" + filepath.Join(dir, "daemon.version")
	if !strings.Contains(log, wantPath) {
		t.Errorf("breadcrumb missing %q:\n%s", wantPath, log)
	}
}

// TestSetVersionWriterLogger_IgnoresNilLogger pins that calling
// SetVersionWriterLogger(nil) leaves the previously-installed logger in
// place, matching the SetBarrierLogger nil-tolerance contract.
func TestSetVersionWriterLogger_IgnoresNilLogger(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "portal.log")
	t.Setenv("PORTAL_LOG_LEVEL", "debug")
	lg, err := state.OpenLogger(logPath, false)
	if err != nil {
		t.Fatalf("OpenLogger: %v", err)
	}
	t.Cleanup(func() { _ = lg.Close() })

	loggerSeam := tmux.VersionWriterLoggerSeam()
	prev := *loggerSeam
	t.Cleanup(func() { *loggerSeam = prev })

	tmux.SetVersionWriterLogger(lg)
	tmux.SetVersionWriterLogger(nil) // must be a no-op

	if *loggerSeam != lg {
		t.Errorf("SetVersionWriterLogger(nil) overwrote the previously installed logger")
	}
}

func TestEnsurePortalSaverVersion_NotAlive_Absent_DoesNotInvokeDefensiveWrite(t *testing.T) {
	stubAliveCheck(t, false)
	shrinkRetryDelay(t)

	dir := t.TempDir() // absent

	writeCalls := 0
	installWriteVersionFile(t, func(string, string) error {
		writeCalls++
		return nil
	})

	// Suppress BootstrapPortalSaver's own stale-daemon barrier so the only
	// thing we are observing is the defensive-write seam.
	installKillSaverFn(t, func(*tmux.Client, string) error { return nil })

	_, _, client := newVersionScenarioClient(t, true)

	if err := tmux.EnsurePortalSaverVersion(client, dir, "v0.4.2"); err != nil {
		t.Fatalf("EnsurePortalSaverVersion returned error: %v", err)
	}

	if writeCalls != 0 {
		t.Errorf("expected 0 defensive write calls when daemon not alive, got %d", writeCalls)
	}
}
