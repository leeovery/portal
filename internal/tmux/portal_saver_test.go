package tmux_test

import (
	"errors"
	"strings"
	"testing"
	"time"

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
