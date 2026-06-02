package tmux_test

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/log"
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

// stubReadinessReady installs a no-op for the waitForSaverDaemonReadyFn seam
// so create-branch tests that do not exercise the readiness barrier directly
// skip its real poll loop. Without this, tests hitting BootstrapPortalSaver's
// create branch with default seams would block for ~2s (ErrPIDFileAbsent on
// every tick until saverReadinessTimeout elapses). Restored via t.Cleanup.
func stubReadinessReady(t *testing.T) {
	t.Helper()
	swapSeam(t, tmux.WaitForSaverDaemonReadyFnSeam(), func(string) error { return nil })
}

// init shrinks the package-level readiness-barrier defaults to test-friendly
// values for the entire tmux_test package, so create-branch tests that do
// not explicitly stub waitForSaverDaemonReadyFn do not pay the production 2s
// timeout. Tests that exercise the readiness barrier directly (via
// tmux.WaitForSaverDaemonReady) install their own values for poll interval
// and timeout via swapSeam, which restores via t.Cleanup — leaving these
// package-test defaults in place between tests.
func init() {
	*tmux.SaverReadinessPollIntervalSeam() = 1 * time.Millisecond
	*tmux.SaverReadinessTimeoutSeam() = 5 * time.Millisecond
}

// portalSaverScript builds a RunFunc dispatching on argv[0] using the supplied
// per-command response handlers. Each handler receives a 1-indexed call
// counter so tests can vary behavior across repeated calls of the same
// command. A nil handler causes the run helper to t.Fatalf — tests opt in to
// each command they expect.
type portalSaverScript struct {
	hasSession  func(call int) (string, error) // tmux has-session -t <name>
	newSession  func(call int) (string, error) // tmux new-session -d -s <name> [cmd]
	killSession func(call int) (string, error) // tmux kill-session -t <name>
	setOption   func(call int) (string, error) // tmux set-option -t <sess> <name> <value>
	respawnPane func(call int) (string, error) // tmux respawn-pane -k -t <target> <cmd>
	// listPanes handles tmux list-panes -t =<name> -F <format> calls issued by
	// the saver lifecycle observability events (Task 5-7): #{pane_id} for
	// placeholder-created / destroy-unattached-off, #{pane_pid} for the
	// respawn-daemon from_pid/to_pid reads. The first arg is the -F format. A
	// nil handler defaults to a benign response so the many pre-5-7 tests that
	// do not care about pane-id/pane-pid output need no per-test change.
	listPanes    func(format string, call int) (string, error)
	hasSessionN  int
	newSessionN  int
	killSessionN int
	setOptionN   int
	respawnPaneN int
	listPanesN   int
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
		case "respawn-pane":
			s.respawnPaneN++
			if s.respawnPane == nil {
				t.Fatalf("unexpected respawn-pane call: %v", args)
				return "", nil
			}
			return s.respawnPane(s.respawnPaneN)
		case "list-panes":
			s.listPanesN++
			format := saverScriptListPanesFormat(args)
			if s.listPanes == nil {
				// Benign default: a pane id for #{pane_id}, a pid for
				// #{pane_pid}. Lets pre-5-7 tests ignore these calls.
				if format == "#{pane_id}" {
					return "%0\n", nil
				}
				return "1\n", nil
			}
			return s.listPanes(format, s.listPanesN)
		default:
			t.Fatalf("unexpected command: %v", args)
			return "", nil
		}
	}
}

// saverScriptListPanesFormat extracts the -F format argument from a list-panes
// argv (the token immediately after "-F"), defaulting to "" when absent.
func saverScriptListPanesFormat(args []string) string {
	for i, a := range args {
		if a == "-F" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
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
		newSession:  func(call int) (string, error) { return "", nil },
		setOption:   func(call int) (string, error) { return "", nil },
		respawnPane: func(call int) (string, error) { return "", nil },
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
	if got := countCalls(mock.Calls, "respawn-pane"); got != 1 {
		t.Errorf("expected exactly 1 respawn-pane call, got %d (calls: %v)", got, mock.Calls)
	}
	if got := countCalls(mock.Calls, "kill-session"); got != 0 {
		t.Errorf("expected 0 kill-session calls, got %d", got)
	}

	// Verify new-session argv shape — must use placeholder command, not the
	// real daemon command. The daemon command is installed via respawn-pane
	// after destroy-unattached=off has been set.
	wantNewSession := "new-session -d -s _portal-saver " + tmux.PortalSaverPlaceholderCommand
	for _, c := range mock.Calls {
		if c[0] != "new-session" {
			continue
		}
		joined := strings.Join(c, " ")
		if joined != wantNewSession {
			t.Errorf("new-session argv = %q, want %q", joined, wantNewSession)
		}
	}

	// Verify respawn-pane argv shape: target=_portal-saver, command=daemon.
	wantRespawn := "respawn-pane -k -t _portal-saver " + tmux.PortalSaverDaemonCommand
	for _, c := range mock.Calls {
		if c[0] != "respawn-pane" {
			continue
		}
		joined := strings.Join(c, " ")
		if joined != wantRespawn {
			t.Errorf("respawn-pane argv = %q, want %q", joined, wantRespawn)
		}
	}
}

// TestBootstrapPortalSaver_CreateOrderingIsCreateThenSetOptionThenRespawn pins
// the load-bearing three-step ordering of the create branch: new-session must
// precede set-option, and set-option must precede respawn-pane. This guards
// against a regression to the pre-Component-F shape where new-session created
// the session with the real daemon as its initial process AND destroy-unattached
// was set afterwards — a sequence in which a lock-loser daemon exit between
// the two calls causes tmux to self-destroy the session before the option
// applies.
func TestBootstrapPortalSaver_CreateOrderingIsCreateThenSetOptionThenRespawn(t *testing.T) {
	stubAliveCheck(t, false)
	shrinkRetryDelay(t)

	script := &portalSaverScript{
		hasSession: func(call int) (string, error) {
			return "", errors.New("can't find session: _portal-saver")
		},
		newSession:  func(call int) (string, error) { return "", nil },
		setOption:   func(call int) (string, error) { return "", nil },
		respawnPane: func(call int) (string, error) { return "", nil },
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.BootstrapPortalSaver(client, "/tmp/portal-state"); err != nil {
		t.Fatalf("BootstrapPortalSaver returned error: %v", err)
	}

	newIdx, setIdx, respawnIdx := -1, -1, -1
	for i, c := range mock.Calls {
		if len(c) == 0 {
			continue
		}
		switch c[0] {
		case "new-session":
			if newIdx == -1 {
				newIdx = i
			}
		case "set-option":
			if setIdx == -1 {
				setIdx = i
			}
		case "respawn-pane":
			if respawnIdx == -1 {
				respawnIdx = i
			}
		}
	}
	if newIdx == -1 || setIdx == -1 || respawnIdx == -1 {
		t.Fatalf("missing call: new=%d set=%d respawn=%d (calls=%v)", newIdx, setIdx, respawnIdx, mock.Calls)
	}
	if newIdx >= setIdx || setIdx >= respawnIdx {
		t.Errorf("expected create-then-set-option-then-respawn ordering; got new=%d set=%d respawn=%d (calls=%v)", newIdx, setIdx, respawnIdx, mock.Calls)
	}
}

// TestBootstrapPortalSaver_PropagatesRespawnPaneFailureWithRespawnDaemonContext
// pins that a RespawnPane error on the create branch surfaces as a wrapped
// "respawn daemon" error and that BootstrapPortalSaver returns the error to
// the caller — the respawn is structurally required, not best-effort.
func TestBootstrapPortalSaver_PropagatesRespawnPaneFailureWithRespawnDaemonContext(t *testing.T) {
	stubAliveCheck(t, false)
	shrinkRetryDelay(t)

	script := &portalSaverScript{
		hasSession: func(call int) (string, error) {
			return "", errors.New("can't find session: _portal-saver")
		},
		newSession:  func(call int) (string, error) { return "", nil },
		setOption:   func(call int) (string, error) { return "", nil },
		respawnPane: func(call int) (string, error) { return "", errors.New("pane vanished mid-flight") },
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	err := tmux.BootstrapPortalSaver(client, "/tmp/portal-state")
	if err == nil {
		t.Fatal("expected error from respawn-pane failure, got nil")
	}
	if !strings.Contains(err.Error(), "respawn daemon") {
		t.Errorf("error %q should contain \"respawn daemon\" context", err.Error())
	}
	if !strings.Contains(err.Error(), "_portal-saver") {
		t.Errorf("error %q should reference session name _portal-saver", err.Error())
	}
	if !strings.Contains(err.Error(), "pane vanished mid-flight") {
		t.Errorf("error %q should wrap underlying tmux error", err.Error())
	}
}

// TestCreatePortalSaverWithRetry_UsesPlaceholderCommand pins the contract that
// createPortalSaverWithRetry passes the placeholder command (not the real
// daemon command) to NewDetachedSessionNoCwd. Drives this assertion via the
// create branch of BootstrapPortalSaver since createPortalSaverWithRetry is
// unexported. The new-session argv string is checked verbatim.
func TestCreatePortalSaverWithRetry_UsesPlaceholderCommand(t *testing.T) {
	stubAliveCheck(t, false)
	shrinkRetryDelay(t)

	var newSessionArgv []string
	script := &portalSaverScript{
		hasSession: func(call int) (string, error) {
			return "", errors.New("can't find session: _portal-saver")
		},
		newSession:  func(call int) (string, error) { return "", nil },
		setOption:   func(call int) (string, error) { return "", nil },
		respawnPane: func(call int) (string, error) { return "", nil },
	}
	mock := &MockCommander{
		RunFunc: func(args ...string) (string, error) {
			if len(args) > 0 && args[0] == "new-session" {
				newSessionArgv = append([]string{}, args...)
			}
			return script.run(t)(args...)
		},
	}
	client := tmux.NewClient(mock)

	if err := tmux.BootstrapPortalSaver(client, "/tmp/portal-state"); err != nil {
		t.Fatalf("BootstrapPortalSaver returned error: %v", err)
	}

	want := []string{"new-session", "-d", "-s", "_portal-saver", tmux.PortalSaverPlaceholderCommand}
	if len(newSessionArgv) != len(want) {
		t.Fatalf("new-session argv = %v, want %v", newSessionArgv, want)
	}
	for i, a := range want {
		if newSessionArgv[i] != a {
			t.Errorf("new-session arg[%d] = %q, want %q", i, newSessionArgv[i], a)
		}
	}
	// And reject any accidental embedding of "portal state daemon" in new-session.
	if strings.Contains(strings.Join(newSessionArgv, " "), tmux.PortalSaverDaemonCommand) {
		t.Errorf("new-session argv unexpectedly contains daemon command: %v", newSessionArgv)
	}
}

// TestBootstrapPortalSaver_ConcurrentRaceTreatsExistingSessionAsSuccess_AndStillRespawns
// pins the concurrent-bootstrap race contract: if NewDetachedSessionNoCwd
// fails but HasSession then reports true (another bootstrap won the race),
// createPortalSaverWithRetry returns nil. BootstrapPortalSaver still goes on
// to apply set-option AND respawn-pane against the now-existing session — the
// respawn is unconditional on the create-needed path.
func TestBootstrapPortalSaver_ConcurrentRaceTreatsExistingSessionAsSuccess_AndStillRespawns(t *testing.T) {
	stubAliveCheck(t, false)
	shrinkRetryDelay(t)

	hasSessionCall := 0
	newSessionCall := 0
	respawnPaneCall := 0
	setOptionCall := 0

	mock := &MockCommander{
		RunFunc: func(args ...string) (string, error) {
			switch args[0] {
			case "has-session":
				hasSessionCall++
				if hasSessionCall == 1 {
					return "", errors.New("can't find session")
				}
				// Concurrent bootstrap won the race.
				return "", nil
			case "new-session":
				newSessionCall++
				return "", errors.New("duplicate session: _portal-saver")
			case "set-option":
				setOptionCall++
				return "", nil
			case "respawn-pane":
				respawnPaneCall++
				return "", nil
			case "list-panes":
				// Task 5-7 saver lifecycle observability reads; benign.
				if saverScriptListPanesFormat(args) == "#{pane_id}" {
					return "%0\n", nil
				}
				return "1\n", nil
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
	if setOptionCall != 1 {
		t.Errorf("expected exactly 1 set-option call after race detected, got %d", setOptionCall)
	}
	if respawnPaneCall != 1 {
		t.Errorf("expected respawn-pane to still run on the create-needed path after race resolution, got %d", respawnPaneCall)
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
		respawnPane: func(call int) (string, error) { return "", nil },
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
		newSession:  func(call int) (string, error) { return "", nil },
		setOption:   func(call int) (string, error) { return "", nil },
		respawnPane: func(call int) (string, error) { return "", nil },
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
		respawnPane: func(call int) (string, error) { return "", nil },
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
			case "respawn-pane":
				return "", nil
			case "list-panes":
				// Task 5-7 saver lifecycle observability reads; benign.
				if saverScriptListPanesFormat(args) == "#{pane_id}" {
					return "%0\n", nil
				}
				return "1\n", nil
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
	if got := countCalls(mock.Calls, "respawn-pane"); got != 1 {
		t.Errorf("expected 1 respawn-pane call after retry success, got %d", got)
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
			case "respawn-pane":
				t.Fatalf("respawn-pane must not be called when create exhausts retries")
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
	if got := countCalls(mock.Calls, "respawn-pane"); got != 0 {
		t.Errorf("respawn-pane must not run after retry exhaustion, got %d calls", got)
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
		respawnPane: func(call int) (string, error) { return "", nil },
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
			case "respawn-pane":
				return "", nil
			case "list-panes":
				// Task 5-7 saver lifecycle observability reads; benign.
				if saverScriptListPanesFormat(args) == "#{pane_id}" {
					return "%0\n", nil
				}
				return "1\n", nil
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
	respawnPaneErr error

	hasSessionCalls  int
	killSessionCalls int
	newSessionCalls  int
	setOptionCalls   int
	respawnPaneCalls int
	listPanesCalls   int
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
		case "respawn-pane":
			s.respawnPaneCalls++
			return "", s.respawnPaneErr
		case "list-panes":
			s.listPanesCalls++
			// Benign default for the Task 5-7 saver lifecycle observability
			// reads (#{pane_id} for the destroy-unattached-off / placeholder
			// events, #{pane_pid} for respawn-daemon from_pid/to_pid).
			if saverScriptListPanesFormat(args) == "#{pane_id}" {
				return "%0\n", nil
			}
			return "1\n", nil
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

// recordingBarrierLogger is a slog.Handler that captures WARN records so
// assertions can verify emission counts and ordering. Each captured WARN is
// stored as "<component> | <message>" so the pre-migration prefix assertions
// keep working against the post-migration terse-message-plus-component-attr
// shape. Use Logger() to obtain a *slog.Logger to install via the barrier
// seam.
type recordingBarrierLogger struct {
	warns []string
	// shared points at the warns-owning recorder so handlers derived via
	// WithAttrs/WithGroup (notably the .With("component", ...) binding) record
	// into the same slice; nil on the root.
	shared *recordingBarrierLogger
	bound  []slog.Attr
}

// Logger returns a *slog.Logger whose records are captured by this recorder.
func (r *recordingBarrierLogger) Logger() *slog.Logger { return slog.New(r) }

func (r *recordingBarrierLogger) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (r *recordingBarrierLogger) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Attr, 0, len(r.bound)+len(attrs))
	next = append(next, r.bound...)
	next = append(next, attrs...)
	return &recordingBarrierLogger{shared: r.owner(), bound: next}
}

func (r *recordingBarrierLogger) WithGroup(_ string) slog.Handler {
	return &recordingBarrierLogger{shared: r.owner(), bound: r.bound}
}

func (r *recordingBarrierLogger) owner() *recordingBarrierLogger {
	if r.shared != nil {
		return r.shared
	}
	return r
}

func (r *recordingBarrierLogger) Handle(_ context.Context, rec slog.Record) error {
	if rec.Level != slog.LevelWarn {
		return nil
	}
	component := ""
	read := func(a slog.Attr) bool {
		if a.Key == "component" {
			component = a.Value.String()
		}
		return true
	}
	for _, a := range r.bound {
		read(a)
	}
	rec.Attrs(read)
	owner := r.owner()
	owner.warns = append(owner.warns, component+" | "+rec.Message)
	return nil
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

// installBarrierReadPID swaps the saverReadPID seam (shared with the
// readiness barrier) for the duration of the test and restores it via
// t.Cleanup.
func installBarrierReadPID(t *testing.T, fn func(string) (int, error)) {
	t.Helper()
	swapSeam(t, tmux.SaverReadPIDSeam(), fn)
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

// installBarrierLogger swaps the WARN-emission seam for a recorder. The
// recorder's logger is bound to the bootstrap component (matching production
// wiring via SetBarrierLogger) so captured WARNs carry the expected component
// attr.
func installBarrierLogger(t *testing.T, log *recordingBarrierLogger) {
	t.Helper()
	swapSeam(t, tmux.BarrierLoggerSeam(), log.Logger().With("component", "bootstrap"))
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
		hasSession:  func(int) (string, error) { return "", nil }, // present
		newSession:  func(int) (string, error) { return "", nil },
		setOption:   func(int) (string, error) { return "", nil },
		respawnPane: func(int) (string, error) { return "", nil },
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
		newSession:  func(int) (string, error) { return "", nil },
		setOption:   func(int) (string, error) { return "", nil },
		respawnPane: func(int) (string, error) { return "", nil },
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
		respawnPane: func(int) (string, error) { return "", nil },
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
		respawnPane: func(int) (string, error) { return "", nil },
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
	stubReadinessReady(t) // isolate kill-barrier WARN; readiness barrier is exercised separately
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
		respawnPane: func(int) (string, error) { return "", nil },
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
	tmux.SetBarrierLogger(recorder.Logger().With("component", "bootstrap"))

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
	if !strings.HasPrefix(recorder.warns[0], "bootstrap"+" | ") {
		t.Errorf("WARN component prefix = %q, want %q", recorder.warns[0], "bootstrap"+" | ")
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
	installed := recorder.Logger().With("component", "bootstrap")
	tmux.SetBarrierLogger(installed)
	tmux.SetBarrierLogger(nil) // must be a no-op

	if *loggerSeam != installed {
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

// recordingSlogHandler captures records so tests can assert on message,
// level, component, and attrs after the observability migration retyped the
// version-writer sink to *slog.Logger.
type recordingSlogHandler struct {
	records []slog.Record
	// shared points at the records-owning handler so handlers derived via
	// WithAttrs/WithGroup record into the same slice; nil on the root.
	shared *recordingSlogHandler
	bound  []slog.Attr
}

func (h *recordingSlogHandler) owner() *recordingSlogHandler {
	if h.shared != nil {
		return h.shared
	}
	return h
}

func (h *recordingSlogHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *recordingSlogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Attr, 0, len(h.bound)+len(attrs))
	next = append(next, h.bound...)
	next = append(next, attrs...)
	return &recordingSlogHandler{shared: h.owner(), bound: next}
}

func (h *recordingSlogHandler) WithGroup(_ string) slog.Handler {
	return &recordingSlogHandler{shared: h.owner(), bound: h.bound}
}

func (h *recordingSlogHandler) Handle(_ context.Context, r slog.Record) error {
	// Merge the accumulated WithAttrs (notably the bound component) onto the
	// stored record so assertions reading r.Attrs see them, matching how the
	// production handler resolves component from the .With binding.
	rec := r.Clone()
	rec.AddAttrs(h.bound...)
	owner := h.owner()
	owner.records = append(owner.records, rec)
	return nil
}

// TestSetVersionWriterLogger_BootstrapWrapperEmitsDebugBreadcrumb pins
// spec § Change 3 / Acceptance Criterion #9: every state.WriteVersionFile
// call emits one DEBUG breadcrumb "daemon.version write" carrying the
// destination path attr — including the bootstrap-side defensive call.
// (version and pid are now baseline attrs injected per-record by the
// configured handler, no longer at the call site.) Guards against the
// wrapper reverting to passing nil.
func TestSetVersionWriterLogger_BootstrapWrapperEmitsDebugBreadcrumb(t *testing.T) {
	dir := t.TempDir()

	rec := &recordingSlogHandler{}
	lg := slog.New(rec).With("component", "daemon")

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

	var breadcrumbs []slog.Record
	for _, r := range rec.records {
		if r.Message == "daemon.version write" {
			breadcrumbs = append(breadcrumbs, r)
		}
	}
	if len(breadcrumbs) != 1 {
		t.Fatalf("expected exactly 1 'daemon.version write' breadcrumb, got %d: %v", len(breadcrumbs), rec.records)
	}
	b := breadcrumbs[0]
	if b.Level != slog.LevelDebug {
		t.Errorf("breadcrumb level = %v, want DEBUG", b.Level)
	}
	var gotComponent, gotPath string
	b.Attrs(func(a slog.Attr) bool {
		switch a.Key {
		case "component":
			gotComponent = a.Value.String()
		case "path":
			gotPath = a.Value.String()
		}
		return true
	})
	if gotComponent != "daemon" {
		t.Errorf("breadcrumb component = %q, want %q", gotComponent, "daemon")
	}
	wantPath := filepath.Join(dir, "daemon.version")
	if gotPath != wantPath {
		t.Errorf("breadcrumb path = %q, want %q", gotPath, wantPath)
	}
}

// TestSetVersionWriterLogger_IgnoresNilLogger pins that calling
// SetVersionWriterLogger(nil) leaves the previously-installed logger in
// place, matching the SetBarrierLogger nil-tolerance contract.
func TestSetVersionWriterLogger_IgnoresNilLogger(t *testing.T) {
	lg := slog.New(&recordingSlogHandler{}).With("component", "daemon")

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

// TestPortalSaverPlaceholderCommand_LiteralValue pins the placeholder command
// constant to its exact literal so an accidental drift (e.g. switching to
// `sleep infinity`, which BSD sleep rejects on macOS) cannot land silently.
// The placeholder is the create-time pane process used in Component F before
// destroy-unattached=off has been applied; it is structurally incapable of
// writing to the state directory or contending for the daemon lock.
func TestPortalSaverPlaceholderCommand_LiteralValue(t *testing.T) {
	const want = "sh -c 'exec tail -f /dev/null'"
	if got := tmux.PortalSaverPlaceholderCommand; got != want {
		t.Errorf("PortalSaverPlaceholderCommand = %q, want %q", got, want)
	}
}

// TestPortalSaverDaemonCommand_LiteralValue pins the daemon command constant
// to its exact literal. This is the real saver pane process installed by
// respawn-pane -k once destroy-unattached=off is in effect.
func TestPortalSaverDaemonCommand_LiteralValue(t *testing.T) {
	const want = "portal state daemon"
	if got := tmux.PortalSaverDaemonCommand; got != want {
		t.Errorf("PortalSaverDaemonCommand = %q, want %q", got, want)
	}
}

// ----------------------------------------------------------------------------
// Task 3-3: post-respawn readiness barrier — waitForSaverDaemonReady.
//
// These tests pin the readiness contract from § Component F:
//   - return nil immediately on PID present + IdentifyIsPortalDaemon,
//   - continue polling on every not-ready shape (absent PID file, transient
//     read error, transient ps error, IdentifyDead, IdentifyNotPortalDaemon),
//   - bound wall-clock by saverReadinessTimeout,
//   - on timeout, emit exactly one WARN via the shared saverBarrier.Logger
//     sink (installed via SetBarrierLogger; same Logger consumed by the kill
//     barrier) under "bootstrap" with the literal grep-anchor
//     and return nil.
//
// Tests use the directly-exported helper tmux.WaitForSaverDaemonReady so they
// exercise the real loop independent of the waitForSaverDaemonReadyFn seam.
// ----------------------------------------------------------------------------

// installReadinessReadPID swaps the saverReadPID seam (shared with the kill
// barrier) for the test.
func installReadinessReadPID(t *testing.T, fn func(string) (int, error)) {
	t.Helper()
	swapSeam(t, tmux.SaverReadPIDSeam(), fn)
}

// installReadinessIdentify swaps the saverIdentifyDaemon seam (shared with
// the kill barrier's escalation path) for the test.
func installReadinessIdentify(t *testing.T, fn func(int) (state.IdentifyResult, error)) {
	t.Helper()
	swapSeam(t, tmux.SaverIdentifyDaemonSeam(), fn)
}

// installReadinessPollInterval shrinks the readiness poll cadence for the test.
func installReadinessPollInterval(t *testing.T, d time.Duration) {
	t.Helper()
	swapSeam(t, tmux.SaverReadinessPollIntervalSeam(), d)
}

// installReadinessTimeout shrinks the readiness timeout for the test.
func installReadinessTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	swapSeam(t, tmux.SaverReadinessTimeoutSeam(), d)
}

func TestWaitForSaverDaemonReady_ReturnsNilImmediatelyWhenPIDPresentAndIdentifies(t *testing.T) {
	installReadinessPollInterval(t, 1*time.Millisecond)
	installReadinessTimeout(t, 500*time.Millisecond)

	readPIDCalls := 0
	installReadinessReadPID(t, func(string) (int, error) {
		readPIDCalls++
		return 4321, nil
	})
	identifyCalls := 0
	installReadinessIdentify(t, func(pid int) (state.IdentifyResult, error) {
		identifyCalls++
		if pid != 4321 {
			t.Errorf("IdentifyDaemon called with pid=%d; want 4321", pid)
		}
		return state.IdentifyIsPortalDaemon, nil
	})
	log := &recordingBarrierLogger{}
	installBarrierLogger(t, log)

	if err := tmux.WaitForSaverDaemonReady(t.TempDir()); err != nil {
		t.Fatalf("WaitForSaverDaemonReady returned error: %v", err)
	}

	if readPIDCalls != 1 {
		t.Errorf("expected exactly 1 ReadPIDFile call on immediate success, got %d", readPIDCalls)
	}
	if identifyCalls != 1 {
		t.Errorf("expected exactly 1 IdentifyDaemon call on immediate success, got %d", identifyCalls)
	}
	if len(log.warns) != 0 {
		t.Errorf("expected 0 WARN lines on immediate success, got %d: %v", len(log.warns), log.warns)
	}
}

func TestWaitForSaverDaemonReady_RetriesWhilePIDFileAbsentThenSucceeds(t *testing.T) {
	installReadinessPollInterval(t, 1*time.Millisecond)
	installReadinessTimeout(t, 500*time.Millisecond)

	// Absent for first 2 ticks, then present.
	readCall := 0
	installReadinessReadPID(t, func(string) (int, error) {
		readCall++
		if readCall < 3 {
			return 0, state.ErrPIDFileAbsent
		}
		return 9999, nil
	})
	installReadinessIdentify(t, func(int) (state.IdentifyResult, error) {
		return state.IdentifyIsPortalDaemon, nil
	})
	log := &recordingBarrierLogger{}
	installBarrierLogger(t, log)

	if err := tmux.WaitForSaverDaemonReady(t.TempDir()); err != nil {
		t.Fatalf("WaitForSaverDaemonReady returned error: %v", err)
	}

	if readCall < 3 {
		t.Errorf("expected at least 3 ReadPIDFile calls (2 absent + 1 success), got %d", readCall)
	}
	if len(log.warns) != 0 {
		t.Errorf("expected 0 WARN lines on eventual success, got %d: %v", len(log.warns), log.warns)
	}
}

func TestWaitForSaverDaemonReady_RetriesOnTransientIdentifyDaemonPSFailure(t *testing.T) {
	installReadinessPollInterval(t, 1*time.Millisecond)
	installReadinessTimeout(t, 500*time.Millisecond)

	installReadinessReadPID(t, func(string) (int, error) { return 4321, nil })

	// Transient ps error for first 2 ticks, then clean success.
	identifyCall := 0
	installReadinessIdentify(t, func(int) (state.IdentifyResult, error) {
		identifyCall++
		if identifyCall < 3 {
			return 0, errors.New("ps: transient exec failure")
		}
		return state.IdentifyIsPortalDaemon, nil
	})
	log := &recordingBarrierLogger{}
	installBarrierLogger(t, log)

	if err := tmux.WaitForSaverDaemonReady(t.TempDir()); err != nil {
		t.Fatalf("WaitForSaverDaemonReady returned error: %v", err)
	}

	if identifyCall < 3 {
		t.Errorf("expected at least 3 IdentifyDaemon calls (2 transient + 1 success), got %d", identifyCall)
	}
	if len(log.warns) != 0 {
		t.Errorf("expected 0 WARN lines on eventual success, got %d: %v", len(log.warns), log.warns)
	}
}

func TestWaitForSaverDaemonReady_RetriesOnIdentifyDeadUntilNextPIDWrite(t *testing.T) {
	installReadinessPollInterval(t, 1*time.Millisecond)
	installReadinessTimeout(t, 500*time.Millisecond)

	// PID file present throughout, but identify returns IdentifyDead twice
	// (daemon hasn't actually started yet / pid file was rewritten with a
	// not-yet-running pid) before flipping to IdentifyIsPortalDaemon.
	installReadinessReadPID(t, func(string) (int, error) { return 4321, nil })
	identifyCall := 0
	installReadinessIdentify(t, func(int) (state.IdentifyResult, error) {
		identifyCall++
		if identifyCall < 3 {
			return state.IdentifyDead, nil
		}
		return state.IdentifyIsPortalDaemon, nil
	})
	log := &recordingBarrierLogger{}
	installBarrierLogger(t, log)

	if err := tmux.WaitForSaverDaemonReady(t.TempDir()); err != nil {
		t.Fatalf("WaitForSaverDaemonReady returned error: %v", err)
	}

	if identifyCall < 3 {
		t.Errorf("expected at least 3 IdentifyDaemon calls (2 dead + 1 success), got %d", identifyCall)
	}
	if len(log.warns) != 0 {
		t.Errorf("expected 0 WARN lines on eventual success, got %d: %v", len(log.warns), log.warns)
	}
}

func TestWaitForSaverDaemonReady_RetriesOnIdentifyNotPortalDaemon(t *testing.T) {
	installReadinessPollInterval(t, 1*time.Millisecond)
	installReadinessTimeout(t, 500*time.Millisecond)

	// Recycled PID — present and alive but not a portal state daemon for
	// first 2 ticks. Then resolves to the real daemon.
	installReadinessReadPID(t, func(string) (int, error) { return 4321, nil })
	identifyCall := 0
	installReadinessIdentify(t, func(int) (state.IdentifyResult, error) {
		identifyCall++
		if identifyCall < 3 {
			return state.IdentifyNotPortalDaemon, nil
		}
		return state.IdentifyIsPortalDaemon, nil
	})
	log := &recordingBarrierLogger{}
	installBarrierLogger(t, log)

	if err := tmux.WaitForSaverDaemonReady(t.TempDir()); err != nil {
		t.Fatalf("WaitForSaverDaemonReady returned error: %v", err)
	}

	if identifyCall < 3 {
		t.Errorf("expected at least 3 IdentifyDaemon calls (2 not-portal + 1 success), got %d", identifyCall)
	}
	if len(log.warns) != 0 {
		t.Errorf("expected 0 WARN lines on eventual success, got %d: %v", len(log.warns), log.warns)
	}
}

func TestWaitForSaverDaemonReady_TimesOutAndEmitsWarnWhenDaemonNeverIdentifies(t *testing.T) {
	installReadinessPollInterval(t, 1*time.Millisecond)
	installReadinessTimeout(t, 20*time.Millisecond)

	installReadinessReadPID(t, func(string) (int, error) { return 4321, nil })
	installReadinessIdentify(t, func(int) (state.IdentifyResult, error) {
		return state.IdentifyDead, nil
	})
	log := &recordingBarrierLogger{}
	installBarrierLogger(t, log)

	if err := tmux.WaitForSaverDaemonReady(t.TempDir()); err != nil {
		t.Fatalf("WaitForSaverDaemonReady returned error: %v", err)
	}

	if len(log.warns) != 1 {
		t.Fatalf("expected exactly 1 WARN line on timeout, got %d: %v", len(log.warns), log.warns)
	}
	want := "bootstrap" + " | saver respawn: daemon did not come up"
	if !strings.HasPrefix(log.warns[0], want) {
		t.Errorf("WARN line %q must begin with %q", log.warns[0], want)
	}
}

func TestWaitForSaverDaemonReady_WallClockBoundedByTimeoutSeam(t *testing.T) {
	installReadinessPollInterval(t, 1*time.Millisecond)
	installReadinessTimeout(t, 20*time.Millisecond)

	// Force the timeout path: ReadPIDFile always succeeds, IdentifyDaemon
	// always returns IdentifyDead, so the loop never short-circuits.
	installReadinessReadPID(t, func(string) (int, error) { return 4321, nil })
	installReadinessIdentify(t, func(int) (state.IdentifyResult, error) {
		return state.IdentifyDead, nil
	})
	log := &recordingBarrierLogger{}
	installBarrierLogger(t, log)

	start := time.Now()
	if err := tmux.WaitForSaverDaemonReady(t.TempDir()); err != nil {
		t.Fatalf("WaitForSaverDaemonReady returned error: %v", err)
	}
	elapsed := time.Since(start)

	// Bound wall-clock by timeout plus generous slack. The contract is that
	// the timeout caps the loop, not that it terminates exactly at the
	// timeout — 1s of slack accommodates CI scheduler jitter without
	// hiding regressions to e.g. the 2s production default.
	if elapsed > 1*time.Second {
		t.Errorf("readiness barrier exceeded wall-time budget: elapsed=%v (timeout=20ms)", elapsed)
	}
	if len(log.warns) != 1 {
		t.Errorf("expected exactly 1 WARN on timeout, got %d: %v", len(log.warns), log.warns)
	}
}

func TestWaitForSaverDaemonReady_TreatsTransientReadPIDErrorAsNotReady(t *testing.T) {
	installReadinessPollInterval(t, 1*time.Millisecond)
	installReadinessTimeout(t, 500*time.Millisecond)

	// Non-absent read error for first 2 ticks (e.g. permission denied),
	// then a clean read with successful identification.
	readCall := 0
	installReadinessReadPID(t, func(string) (int, error) {
		readCall++
		if readCall < 3 {
			return 0, errors.New("read daemon.pid: permission denied")
		}
		return 4321, nil
	})
	installReadinessIdentify(t, func(int) (state.IdentifyResult, error) {
		return state.IdentifyIsPortalDaemon, nil
	})
	log := &recordingBarrierLogger{}
	installBarrierLogger(t, log)

	if err := tmux.WaitForSaverDaemonReady(t.TempDir()); err != nil {
		t.Fatalf("WaitForSaverDaemonReady returned error: %v", err)
	}

	if readCall < 3 {
		t.Errorf("expected at least 3 ReadPIDFile calls (2 transient + 1 success), got %d", readCall)
	}
	if len(log.warns) != 0 {
		t.Errorf("expected 0 WARN lines on eventual success, got %d: %v", len(log.warns), log.warns)
	}
}

// TestBootstrapPortalSaver_InvokesReadinessBarrierAfterRespawnOnCreatePath pins
// that the create branch routes through the waitForSaverDaemonReadyFn seam
// AFTER RespawnPane. The session-present-and-alive happy path must NOT invoke
// the barrier (no respawn ran).
func TestBootstrapPortalSaver_InvokesReadinessBarrierAfterRespawnOnCreatePath(t *testing.T) {
	stubAliveCheck(t, false) // session absent → pure create path
	shrinkRetryDelay(t)

	readinessCalls := 0
	var orderTrace []string
	swapSeam(t, tmux.WaitForSaverDaemonReadyFnSeam(), func(string) error {
		readinessCalls++
		orderTrace = append(orderTrace, "readiness")
		return nil
	})

	script := &portalSaverScript{
		hasSession: func(int) (string, error) {
			return "", errors.New("can't find session: _portal-saver")
		},
		newSession: func(int) (string, error) { return "", nil },
		setOption:  func(int) (string, error) { return "", nil },
		respawnPane: func(int) (string, error) {
			orderTrace = append(orderTrace, "respawn-pane")
			return "", nil
		},
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.BootstrapPortalSaver(client, "/tmp/portal-state"); err != nil {
		t.Fatalf("BootstrapPortalSaver returned error: %v", err)
	}

	if readinessCalls != 1 {
		t.Errorf("expected exactly 1 readiness-barrier invocation on create path, got %d", readinessCalls)
	}
	if len(orderTrace) != 2 || orderTrace[0] != "respawn-pane" || orderTrace[1] != "readiness" {
		t.Errorf("expected ordering [respawn-pane, readiness]; got %v", orderTrace)
	}
}

func TestBootstrapPortalSaver_DoesNotInvokeReadinessBarrierOnSessionPresentAndAliveHappyPath(t *testing.T) {
	stubAliveCheck(t, true) // session present AND daemon alive → no create, no respawn
	shrinkRetryDelay(t)

	readinessCalls := 0
	swapSeam(t, tmux.WaitForSaverDaemonReadyFnSeam(), func(string) error {
		readinessCalls++
		return nil
	})

	script := &portalSaverScript{
		hasSession: func(int) (string, error) { return "", nil }, // present
		setOption:  func(int) (string, error) { return "", nil },
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.BootstrapPortalSaver(client, "/tmp/portal-state"); err != nil {
		t.Fatalf("BootstrapPortalSaver returned error: %v", err)
	}

	if readinessCalls != 0 {
		t.Errorf("expected 0 readiness-barrier invocations on session-present-and-alive path, got %d", readinessCalls)
	}
}

// TestBootstrapPortalSaver_ReadinessBarrierStateDirThreadedFromCaller pins that
// the stateDir argument supplied to BootstrapPortalSaver is forwarded to the
// readiness-barrier seam unchanged.
func TestBootstrapPortalSaver_ReadinessBarrierStateDirThreadedFromCaller(t *testing.T) {
	stubAliveCheck(t, false)
	shrinkRetryDelay(t)

	const wantDir = "/test/threaded-state-dir"
	var observed string
	swapSeam(t, tmux.WaitForSaverDaemonReadyFnSeam(), func(dir string) error {
		observed = dir
		return nil
	})

	script := &portalSaverScript{
		hasSession: func(int) (string, error) {
			return "", errors.New("can't find session: _portal-saver")
		},
		newSession:  func(int) (string, error) { return "", nil },
		setOption:   func(int) (string, error) { return "", nil },
		respawnPane: func(int) (string, error) { return "", nil },
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.BootstrapPortalSaver(client, wantDir); err != nil {
		t.Fatalf("BootstrapPortalSaver returned error: %v", err)
	}

	if observed != wantDir {
		t.Errorf("readiness barrier received stateDir=%q; want %q", observed, wantDir)
	}
}

// TestWaitForSaverDaemonReady_DeadlineComputedOnceAtEntry pins that the 2s
// ceiling is enforced by a single deadline computed at function entry. If the
// implementation reset the deadline on every loop iteration, an unbounded
// stream of transient errors could push wall-clock arbitrarily high — this
// test forces transient errors throughout and asserts wall-clock stays
// bounded by the timeout seam.
func TestWaitForSaverDaemonReady_DeadlineComputedOnceAtEntry(t *testing.T) {
	installReadinessPollInterval(t, 1*time.Millisecond)
	installReadinessTimeout(t, 15*time.Millisecond)

	// Mix of transient errors: ReadPIDFile alternates between absent and
	// non-absent errors; Identify alternates between transient ps error,
	// IdentifyDead, and IdentifyNotPortalDaemon.
	readCall := 0
	installReadinessReadPID(t, func(string) (int, error) {
		readCall++
		if readCall%2 == 0 {
			return 0, state.ErrPIDFileAbsent
		}
		return 4321, nil
	})
	identifyCall := 0
	installReadinessIdentify(t, func(int) (state.IdentifyResult, error) {
		identifyCall++
		switch identifyCall % 3 {
		case 0:
			return 0, errors.New("transient ps")
		case 1:
			return state.IdentifyDead, nil
		default:
			return state.IdentifyNotPortalDaemon, nil
		}
	})
	log := &recordingBarrierLogger{}
	installBarrierLogger(t, log)

	start := time.Now()
	_ = tmux.WaitForSaverDaemonReady(t.TempDir())
	elapsed := time.Since(start)

	if elapsed > 500*time.Millisecond {
		t.Errorf("deadline appears to reset on transient errors: elapsed=%v (timeout=15ms)", elapsed)
	}
	if len(log.warns) != 1 {
		t.Errorf("expected exactly 1 WARN on timeout, got %d: %v", len(log.warns), log.warns)
	}
}

// ----------------------------------------------------------------------------
// Task 3-4: Compose unhealthy-saver recreate path with new ordering.
//
// These tests pin that the unhealthy-saver branch of BootstrapPortalSaver
// (session present + dead daemon, e.g. placeholder-only-lingering saver from
// a crashed prior bootstrap) falls through cleanly to the 3-2 / 3-3 four-step
// ordering:
//
//	kill-session → new-session (placeholder) → set-option (destroy-unattached=off)
//	  → respawn-pane (daemon) [→ readiness barrier]
//
// And that EnsurePortalSaverVersion's delegation to BootstrapPortalSaver
// inherits the new ordering both on alive=true + version-mismatch (kill row of
// the matrix) and on alive=false (no-kill row).
// ----------------------------------------------------------------------------

// assertKillNewSetRespawnOrdering scans calls and asserts the load-bearing
// four-step recreate ordering: kill-session BEFORE new-session BEFORE
// set-option BEFORE respawn-pane. Fails the test if any of the four is
// missing or if any pair is out of order.
func assertKillNewSetRespawnOrdering(t *testing.T, calls [][]string) {
	t.Helper()
	killIdx, newIdx, setIdx, respawnIdx := -1, -1, -1, -1
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
		case "set-option":
			if setIdx == -1 {
				setIdx = i
			}
		case "respawn-pane":
			if respawnIdx == -1 {
				respawnIdx = i
			}
		}
	}
	if killIdx == -1 || newIdx == -1 || setIdx == -1 || respawnIdx == -1 {
		t.Fatalf("missing call: kill=%d new=%d set=%d respawn=%d (calls=%v)",
			killIdx, newIdx, setIdx, respawnIdx, calls)
	}
	if killIdx >= newIdx || newIdx >= setIdx || setIdx >= respawnIdx {
		t.Errorf("expected ordering kill < new < set < respawn; got kill=%d new=%d set=%d respawn=%d (calls=%v)",
			killIdx, newIdx, setIdx, respawnIdx, calls)
	}
}

// TestBootstrapPortalSaver_RecyclesPlaceholderOnlySaverViaNewOrdering pins
// Test 1 of Task 3-4. A prior bootstrap crashed mid-respawn leaving a
// placeholder-only saver behind: the _portal-saver session exists with the
// `tail -f /dev/null` placeholder as its only pane process, no daemon writing
// daemon.pid. BootstrapAliveCheck reports unhealthy (stubbed false), so the
// unhealthy-saver branch fires: tolerant kill → fall through to the create
// path → placeholder new-session → set destroy-unattached=off → respawn-pane
// with the daemon command → readiness barrier.
//
// The full four-step argv ordering (kill < new-placeholder < set < respawn-daemon)
// is asserted via assertKillNewSetRespawnOrdering, and the new-session /
// respawn-pane argv literals are pinned so a regression to e.g. embedding the
// daemon command in new-session would fail loudly.
func TestBootstrapPortalSaver_RecyclesPlaceholderOnlySaverViaNewOrdering(t *testing.T) {
	stubAliveCheck(t, false) // placeholder-only saver: daemon.pid absent/stale
	shrinkRetryDelay(t)
	stubReadinessReady(t)

	script := &portalSaverScript{
		hasSession:  func(int) (string, error) { return "", nil }, // present (placeholder lingering)
		killSession: func(int) (string, error) { return "", nil },
		newSession:  func(int) (string, error) { return "", nil },
		setOption:   func(int) (string, error) { return "", nil },
		respawnPane: func(int) (string, error) { return "", nil },
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.BootstrapPortalSaver(client, t.TempDir()); err != nil {
		t.Fatalf("BootstrapPortalSaver returned error: %v", err)
	}

	if got := countCalls(mock.Calls, "kill-session"); got != 1 {
		t.Errorf("expected exactly 1 kill-session call, got %d (calls: %v)", got, mock.Calls)
	}
	if got := countCalls(mock.Calls, "new-session"); got != 1 {
		t.Errorf("expected exactly 1 new-session call, got %d", got)
	}
	if got := countCalls(mock.Calls, "set-option"); got != 1 {
		t.Errorf("expected exactly 1 set-option call, got %d", got)
	}
	if got := countCalls(mock.Calls, "respawn-pane"); got != 1 {
		t.Errorf("expected exactly 1 respawn-pane call, got %d", got)
	}

	// Load-bearing ordering: kill BEFORE new-session-with-placeholder BEFORE
	// set destroy-unattached=off BEFORE respawn-pane with the daemon.
	assertKillNewSetRespawnOrdering(t, mock.Calls)

	// Pin new-session argv: must use the placeholder, NOT the daemon command.
	// A regression putting the daemon command back into new-session would
	// reintroduce the pre-Component-F lock-loser race.
	wantNew := "new-session -d -s _portal-saver " + tmux.PortalSaverPlaceholderCommand
	for _, c := range mock.Calls {
		if c[0] != "new-session" {
			continue
		}
		if joined := strings.Join(c, " "); joined != wantNew {
			t.Errorf("new-session argv = %q, want %q", joined, wantNew)
		}
	}

	// Pin respawn-pane argv: target _portal-saver, command = daemon. The pane
	// process AFTER respawn must be the daemon, not the placeholder.
	wantRespawn := "respawn-pane -k -t _portal-saver " + tmux.PortalSaverDaemonCommand
	for _, c := range mock.Calls {
		if c[0] != "respawn-pane" {
			continue
		}
		if joined := strings.Join(c, " "); joined != wantRespawn {
			t.Errorf("respawn-pane argv = %q, want %q", joined, wantRespawn)
		}
	}
}

// TestEnsurePortalSaverVersion_AliveMismatch_FlowsThroughNewBootstrapOrdering
// pins Test 2 of Task 3-4. EnsurePortalSaverVersion with alive=true and a
// genuine version mismatch (neither side dev) hits the kill row of the
// kill-decision matrix and then delegates to BootstrapPortalSaver — which
// observes the session as present-and-killed (the kill stub mutates the
// scenario), then no-ops since alive=true would short-circuit... we instead
// route the kill through the real KillSession in the mock so the scenario
// flips sessionPresent=false and BootstrapPortalSaver falls into its full
// create path with the new ordering.
//
// Asserts: kill-session fires (kill row of the matrix), followed by the
// full create-with-new-ordering sequence on the same mock — proving
// EnsurePortalSaverVersion inherits the new ordering via composition.
func TestEnsurePortalSaverVersion_AliveMismatch_FlowsThroughNewBootstrapOrdering(t *testing.T) {
	stubAliveCheck(t, true) // alive=true → matrix consults version
	shrinkRetryDelay(t)
	stubReadinessReady(t)

	dir := t.TempDir()
	writeVersion(t, dir, "v0.4.1") // mismatch with "v0.4.2"; neither dev → kill row

	// Use the versionScenario so kill-session flips sessionPresent=false and
	// BootstrapPortalSaver re-creates with the new ordering.
	scenario, mock, client := newVersionScenarioClient(t, true)

	if err := tmux.EnsurePortalSaverVersion(client, dir, "v0.4.2"); err != nil {
		t.Fatalf("EnsurePortalSaverVersion returned error: %v", err)
	}

	if scenario.killSessionCalls != 1 {
		t.Errorf("expected exactly 1 kill-session on alive+mismatch (kill row), got %d", scenario.killSessionCalls)
	}
	if scenario.newSessionCalls != 1 {
		t.Errorf("expected exactly 1 new-session after kill, got %d", scenario.newSessionCalls)
	}
	if scenario.setOptionCalls != 1 {
		t.Errorf("expected exactly 1 set-option, got %d", scenario.setOptionCalls)
	}
	if scenario.respawnPaneCalls != 1 {
		t.Errorf("expected exactly 1 respawn-pane after set-option, got %d", scenario.respawnPaneCalls)
	}

	// The full ordering must be preserved through the delegation: kill
	// (from EnsurePortalSaverVersion's matrix) → new-session (placeholder) →
	// set-option (destroy-unattached=off) → respawn-pane (daemon).
	assertKillNewSetRespawnOrdering(t, mock.Calls)

	// new-session must carry the placeholder, not the daemon command — pins
	// that delegation does not bypass the placeholder-first ordering.
	wantNew := "new-session -d -s _portal-saver " + tmux.PortalSaverPlaceholderCommand
	for _, c := range mock.Calls {
		if c[0] != "new-session" {
			continue
		}
		if joined := strings.Join(c, " "); joined != wantNew {
			t.Errorf("new-session argv = %q, want %q", joined, wantNew)
		}
	}
}

// TestEnsurePortalSaverVersion_NotAlive_SkipsKillAndStillUsesNewOrdering pins
// Test 3 of Task 3-4. EnsurePortalSaverVersion with alive=false hits the
// "no kill" row of the matrix (row 1: alive=no → no kill, regardless of
// version). It then delegates to BootstrapPortalSaver; with the session
// absent BootstrapPortalSaver does NOT consult the unhealthy-saver branch
// and falls straight into the create path with the new ordering
// (new-session placeholder → set-option → respawn-pane daemon → readiness).
//
// Asserts: zero kill-session calls (the caller never fires; BootstrapPortalSaver
// never fires because the session is absent); the full create sequence runs
// with new-session → set-option → respawn-pane ordering.
func TestEnsurePortalSaverVersion_NotAlive_SkipsKillAndStillUsesNewOrdering(t *testing.T) {
	stubAliveCheck(t, false) // alive=false → kill matrix row 1 (no kill)
	shrinkRetryDelay(t)
	stubReadinessReady(t)

	dir := t.TempDir() // no daemon.version → ErrVersionFileAbsent (irrelevant under alive=false)

	// sessionPresent=false isolates the assertion: BootstrapPortalSaver's
	// stale-daemon branch cannot fire because the session is absent. The
	// only path from kill is EnsurePortalSaverVersion's own matrix.
	scenario, mock, client := newVersionScenarioClient(t, false)

	if err := tmux.EnsurePortalSaverVersion(client, dir, "v0.4.2"); err != nil {
		t.Fatalf("EnsurePortalSaverVersion returned error: %v", err)
	}

	if scenario.killSessionCalls != 0 {
		t.Errorf("expected 0 kill-session calls on alive=false (matrix row 1), got %d", scenario.killSessionCalls)
	}
	if scenario.newSessionCalls != 1 {
		t.Errorf("expected exactly 1 new-session call, got %d", scenario.newSessionCalls)
	}
	if scenario.setOptionCalls != 1 {
		t.Errorf("expected exactly 1 set-option call, got %d", scenario.setOptionCalls)
	}
	if scenario.respawnPaneCalls != 1 {
		t.Errorf("expected exactly 1 respawn-pane call, got %d", scenario.respawnPaneCalls)
	}

	// Pin the new ordering on the no-kill path: new-session BEFORE set-option
	// BEFORE respawn-pane.
	newIdx, setIdx, respawnIdx := -1, -1, -1
	for i, c := range mock.Calls {
		if len(c) == 0 {
			continue
		}
		switch c[0] {
		case "new-session":
			if newIdx == -1 {
				newIdx = i
			}
		case "set-option":
			if setIdx == -1 {
				setIdx = i
			}
		case "respawn-pane":
			if respawnIdx == -1 {
				respawnIdx = i
			}
		}
	}
	if newIdx == -1 || setIdx == -1 || respawnIdx == -1 {
		t.Fatalf("missing call: new=%d set=%d respawn=%d (calls=%v)", newIdx, setIdx, respawnIdx, mock.Calls)
	}
	if newIdx >= setIdx || setIdx >= respawnIdx {
		t.Errorf("expected ordering new < set < respawn on no-kill path; got new=%d set=%d respawn=%d (calls=%v)",
			newIdx, setIdx, respawnIdx, mock.Calls)
	}

	// Pin new-session argv: placeholder, not daemon.
	wantNew := "new-session -d -s _portal-saver " + tmux.PortalSaverPlaceholderCommand
	for _, c := range mock.Calls {
		if c[0] != "new-session" {
			continue
		}
		if joined := strings.Join(c, " "); joined != wantNew {
			t.Errorf("new-session argv = %q, want %q", joined, wantNew)
		}
	}
}

// TestBootstrapPortalSaver_NoPersistentPlaceholderLeakAcrossSingleRecovery is
// Test 4 of Task 3-4 — the regression guard for spec § Component F's
// "No persistent placeholder leak" property. After a single
// BootstrapPortalSaver invocation against a placeholder-only-lingering saver
// (prior bootstrap crashed mid-respawn), the FINAL pane process in the
// recorded argv stream must be the daemon command, not the placeholder.
//
// Implementation: scan mock.Calls in source order. The "final" pane process
// is set by whichever of new-session (with placeholder) or respawn-pane
// (with daemon) appears LAST. If the chain terminates on a stray new-session
// (which it should not), the placeholder would be the final process — a
// persistent leak. The recovery cycle's contract is that respawn-pane with
// the daemon command is the last argv affecting the pane process.
func TestBootstrapPortalSaver_NoPersistentPlaceholderLeakAcrossSingleRecovery(t *testing.T) {
	stubAliveCheck(t, false) // placeholder-only lingering
	shrinkRetryDelay(t)
	stubReadinessReady(t)

	script := &portalSaverScript{
		hasSession:  func(int) (string, error) { return "", nil }, // present
		killSession: func(int) (string, error) { return "", nil },
		newSession:  func(int) (string, error) { return "", nil },
		setOption:   func(int) (string, error) { return "", nil },
		respawnPane: func(int) (string, error) { return "", nil },
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.BootstrapPortalSaver(client, t.TempDir()); err != nil {
		t.Fatalf("BootstrapPortalSaver returned error: %v", err)
	}

	// Find the LAST argv that mutates the saver pane process. Of new-session
	// (placeholder) and respawn-pane (daemon), whichever comes last wins —
	// that determines the persistent state of the pane process after the
	// recovery cycle.
	lastPaneMutator := ""
	lastPaneCommand := ""
	for _, c := range mock.Calls {
		if len(c) == 0 {
			continue
		}
		switch c[0] {
		case "new-session":
			lastPaneMutator = "new-session"
			// new-session argv shape: new-session -d -s _portal-saver <cmd>
			// where <cmd> is args[4]. Reassemble the trailing command.
			if len(c) >= 5 {
				lastPaneCommand = strings.Join(c[4:], " ")
			}
		case "respawn-pane":
			lastPaneMutator = "respawn-pane"
			// respawn-pane argv shape: respawn-pane -k -t _portal-saver <cmd>
			if len(c) >= 5 {
				lastPaneCommand = strings.Join(c[4:], " ")
			}
		}
	}

	if lastPaneMutator != "respawn-pane" {
		t.Errorf("final pane mutator = %q, want %q — placeholder leaked past recovery cycle (calls: %v)",
			lastPaneMutator, "respawn-pane", mock.Calls)
	}
	if lastPaneCommand != tmux.PortalSaverDaemonCommand {
		t.Errorf("final pane command = %q, want %q (daemon) — placeholder leak detected (calls: %v)",
			lastPaneCommand, tmux.PortalSaverDaemonCommand, mock.Calls)
	}
	if lastPaneCommand == tmux.PortalSaverPlaceholderCommand {
		t.Errorf("final pane command is still the placeholder %q — persistent placeholder leak across recovery cycle",
			tmux.PortalSaverPlaceholderCommand)
	}
}

// TestNewDetachedSessionNoCwd_ArgvHasNoEnvOverrides pins the spec §
// "Component F — Environment inheritance across respawn" contract at the
// argv level: NewDetachedSessionNoCwd must NOT emit any tmux "-e KEY=VAL"
// session-environment override. Any such override would shadow the
// inherited tmux server environment for that session, which would in turn
// shadow what the respawned daemon observes via getenv(). The session
// must inherit the tmux server env verbatim — the same shape every other
// detached session on the same server sees.
//
// This is the unit-level companion to
// TestBootstrapPortalSaver_EnvironmentInheritanceAcrossRespawn in
// portal_saver_endstate_integration_test.go: the integration test
// verifies the observable end-state (show-environment parity); this test
// pins the argv shape so a regression that introduces an env override at
// create time is caught even when the integration test cannot run (no
// tmux on PATH in CI containers).
func TestNewDetachedSessionNoCwd_ArgvHasNoEnvOverrides(t *testing.T) {
	var newSessionArgv []string
	mock := &MockCommander{
		RunFunc: func(args ...string) (string, error) {
			if len(args) > 0 && args[0] == "new-session" {
				newSessionArgv = append([]string{}, args...)
			}
			return "", nil
		},
	}
	client := tmux.NewClient(mock)

	if err := client.NewDetachedSessionNoCwd("_some-session", "sh -c 'exec tail -f /dev/null'"); err != nil {
		t.Fatalf("NewDetachedSessionNoCwd returned error: %v", err)
	}

	if newSessionArgv == nil {
		t.Fatalf("new-session was not invoked; Calls=%v", mock.Calls)
	}

	// The load-bearing assertion: NO argv element may begin with "-e".
	// tmux accepts "-e KEY=VAL" as two argv elements OR "-eKEY=VAL" as one
	// element on the affected versions; HasPrefix covers both shapes
	// without coupling to a specific tmux argv-parsing rule. The first
	// element ("new-session") is the subcommand, not a flag, and does
	// not start with "-e", so the scan is uniform.
	for i, arg := range newSessionArgv {
		if strings.HasPrefix(arg, "-e") {
			t.Errorf("new-session argv[%d] = %q starts with \"-e\"; "+
				"NewDetachedSessionNoCwd must not pass session-environment "+
				"overrides. Full argv: %v", i, arg, newSessionArgv)
		}
	}
}

// ----------------------------------------------------------------------------
// Task 4-1: SIGKILL escalation in killSaverAndWaitForDaemon.
//
// After the session-kill poll loop times out (existing 5s window), the helper
// identity-checks priorPID via saverIdentifyDaemon and, only when the
// result is IdentifyIsPortalDaemon, sends SIGKILL via killBarrierSendSIGKILL
// (the IMMEDIATELY-preceding seam call). Then it polls killBarrierIsAlive at
// killBarrierPollInterval cadence for up to killBarrierEscalationTimeout.
//
// Spec § Component A — Kill-Barrier Escalation.
// ----------------------------------------------------------------------------

// installBarrierIdentifyDaemon swaps the saverIdentifyDaemon seam (shared
// with the readiness barrier) for
// the duration of the test.
func installBarrierIdentifyDaemon(t *testing.T, fn func(int) (state.IdentifyResult, error)) {
	t.Helper()
	swapSeam(t, tmux.SaverIdentifyDaemonSeam(), fn)
}

// installBarrierSendSIGKILL swaps the killBarrierSendSIGKILL seam for the
// duration of the test.
func installBarrierSendSIGKILL(t *testing.T, fn func(int) error) {
	t.Helper()
	swapSeam(t, tmux.BarrierSendSIGKILLSeam(), fn)
}

// installBarrierEscalationTimeout shrinks the post-SIGKILL poll budget for tests.
func installBarrierEscalationTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	swapSeam(t, tmux.BarrierEscalationTimeoutSeam(), d)
}

// TestKillSaverAndWaitForDaemon_Escalation_IdentityChecksAsPortalDaemonThenSIGKILLs
// pins the happy escalation path: after the session-kill poll times out with
// the PID still alive, the helper identity-checks the PID and (on
// IdentifyIsPortalDaemon) sends SIGKILL via killBarrierSendSIGKILL.
func TestKillSaverAndWaitForDaemon_Escalation_IdentityChecksAsPortalDaemonThenSIGKILLs(t *testing.T) {
	installBarrierPollInterval(t, 1*time.Millisecond)
	installBarrierTimeout(t, 5*time.Millisecond)
	installBarrierEscalationTimeout(t, 5*time.Millisecond)
	installBarrierReadPID(t, func(string) (int, error) { return 4321, nil })

	identityCalls := 0
	installBarrierIdentifyDaemon(t, func(pid int) (state.IdentifyResult, error) {
		identityCalls++
		if pid != 4321 {
			t.Errorf("identity check called with pid=%d, want 4321", pid)
		}
		return state.IdentifyIsPortalDaemon, nil
	})

	killCalls := 0
	var killedPID int
	installBarrierSendSIGKILL(t, func(pid int) error {
		killCalls++
		killedPID = pid
		return nil
	})

	// Alive during session-kill poll; dead immediately after SIGKILL.
	aliveProbes := 0
	installBarrierIsAlive(t, func(pid int) bool {
		aliveProbes++
		// First N probes return true (session-kill poll exhaust); after kill
		// seam runs we want IsAlive to return false. We use killCalls counter
		// as gate.
		return killCalls == 0
	})

	log := &recordingBarrierLogger{}
	installBarrierLogger(t, log)

	script := &portalSaverScript{
		killSession: func(int) (string, error) { return "", nil },
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.KillSaverAndWaitForDaemon(client, t.TempDir()); err != nil {
		t.Fatalf("KillSaverAndWaitForDaemon returned error: %v", err)
	}

	if identityCalls != 1 {
		t.Errorf("expected exactly 1 identity check call, got %d", identityCalls)
	}
	if killCalls != 1 {
		t.Errorf("expected exactly 1 SIGKILL seam call, got %d", killCalls)
	}
	if killedPID != 4321 {
		t.Errorf("SIGKILL seam called with pid=%d, want 4321", killedPID)
	}
	if len(log.warns) != 0 {
		t.Errorf("expected 0 WARN lines on clean escalation, got %d: %v", len(log.warns), log.warns)
	}
}

// TestKillSaverAndWaitForDaemon_Escalation_IdentifyDead_SkipsSIGKILL_WarnsAndReturnsNil
// pins that when the identity check returns IdentifyDead (PID gone since the
// last poll), the SIGKILL seam is NOT invoked, exactly one WARN is emitted,
// and the helper returns nil.
func TestKillSaverAndWaitForDaemon_Escalation_IdentifyDead_SkipsSIGKILL_WarnsAndReturnsNil(t *testing.T) {
	installBarrierPollInterval(t, 1*time.Millisecond)
	installBarrierTimeout(t, 5*time.Millisecond)
	installBarrierEscalationTimeout(t, 5*time.Millisecond)
	installBarrierReadPID(t, func(string) (int, error) { return 4321, nil })
	installBarrierIsAlive(t, func(int) bool { return true })

	installBarrierIdentifyDaemon(t, func(int) (state.IdentifyResult, error) {
		return state.IdentifyDead, nil
	})

	killCalls := 0
	installBarrierSendSIGKILL(t, func(int) error {
		killCalls++
		return nil
	})

	log := &recordingBarrierLogger{}
	installBarrierLogger(t, log)

	script := &portalSaverScript{
		killSession: func(int) (string, error) { return "", nil },
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.KillSaverAndWaitForDaemon(client, t.TempDir()); err != nil {
		t.Fatalf("KillSaverAndWaitForDaemon returned error: %v", err)
	}

	if killCalls != 0 {
		t.Errorf("expected 0 SIGKILL seam calls on IdentifyDead, got %d", killCalls)
	}
	if len(log.warns) != 1 {
		t.Fatalf("expected exactly 1 WARN on IdentifyDead, got %d: %v", len(log.warns), log.warns)
	}
}

// TestKillSaverAndWaitForDaemon_Escalation_IdentifyNotPortalDaemon_SkipsSIGKILL_WarnsAndReturnsNil
// pins that when the identity check returns IdentifyNotPortalDaemon (the PID
// has been recycled to a different process), the SIGKILL seam is NOT
// invoked, exactly one WARN is emitted, and the helper returns nil.
func TestKillSaverAndWaitForDaemon_Escalation_IdentifyNotPortalDaemon_SkipsSIGKILL_WarnsAndReturnsNil(t *testing.T) {
	installBarrierPollInterval(t, 1*time.Millisecond)
	installBarrierTimeout(t, 5*time.Millisecond)
	installBarrierEscalationTimeout(t, 5*time.Millisecond)
	installBarrierReadPID(t, func(string) (int, error) { return 4321, nil })
	installBarrierIsAlive(t, func(int) bool { return true })

	installBarrierIdentifyDaemon(t, func(int) (state.IdentifyResult, error) {
		return state.IdentifyNotPortalDaemon, nil
	})

	killCalls := 0
	installBarrierSendSIGKILL(t, func(int) error {
		killCalls++
		return nil
	})

	log := &recordingBarrierLogger{}
	installBarrierLogger(t, log)

	script := &portalSaverScript{
		killSession: func(int) (string, error) { return "", nil },
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.KillSaverAndWaitForDaemon(client, t.TempDir()); err != nil {
		t.Fatalf("KillSaverAndWaitForDaemon returned error: %v", err)
	}

	if killCalls != 0 {
		t.Errorf("expected 0 SIGKILL seam calls on IdentifyNotPortalDaemon, got %d", killCalls)
	}
	if len(log.warns) != 1 {
		t.Fatalf("expected exactly 1 WARN on IdentifyNotPortalDaemon, got %d: %v", len(log.warns), log.warns)
	}
}

// TestKillSaverAndWaitForDaemon_Escalation_TransientIdentityError_SkipsSIGKILL_WarnsAndReturnsNil
// pins that when the identity check returns a transient (non-nil) error, the
// SIGKILL seam is NOT invoked, exactly one WARN is emitted, and the helper
// returns nil. Safety-bias: never signal a PID we can't positively identify.
func TestKillSaverAndWaitForDaemon_Escalation_TransientIdentityError_SkipsSIGKILL_WarnsAndReturnsNil(t *testing.T) {
	installBarrierPollInterval(t, 1*time.Millisecond)
	installBarrierTimeout(t, 5*time.Millisecond)
	installBarrierEscalationTimeout(t, 5*time.Millisecond)
	installBarrierReadPID(t, func(string) (int, error) { return 4321, nil })
	installBarrierIsAlive(t, func(int) bool { return true })

	installBarrierIdentifyDaemon(t, func(int) (state.IdentifyResult, error) {
		return 0, errors.New("ps exec failed: transient")
	})

	killCalls := 0
	installBarrierSendSIGKILL(t, func(int) error {
		killCalls++
		return nil
	})

	log := &recordingBarrierLogger{}
	installBarrierLogger(t, log)

	script := &portalSaverScript{
		killSession: func(int) (string, error) { return "", nil },
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.KillSaverAndWaitForDaemon(client, t.TempDir()); err != nil {
		t.Fatalf("KillSaverAndWaitForDaemon returned error: %v", err)
	}

	if killCalls != 0 {
		t.Errorf("expected 0 SIGKILL seam calls on transient identity error, got %d", killCalls)
	}
	if len(log.warns) != 1 {
		t.Fatalf("expected exactly 1 WARN on transient identity error, got %d: %v", len(log.warns), log.warns)
	}
}

// TestKillSaverAndWaitForDaemon_Escalation_SIGKILLSucceedsAndProcessExitsWithinWindow
// pins the success path of the post-SIGKILL poll: when the process exits
// within killBarrierEscalationTimeout, the helper returns nil with no WARN.
func TestKillSaverAndWaitForDaemon_Escalation_SIGKILLSucceedsAndProcessExitsWithinWindow(t *testing.T) {
	installBarrierPollInterval(t, 1*time.Millisecond)
	installBarrierTimeout(t, 5*time.Millisecond)
	installBarrierEscalationTimeout(t, 50*time.Millisecond)
	installBarrierReadPID(t, func(string) (int, error) { return 4321, nil })

	installBarrierIdentifyDaemon(t, func(int) (state.IdentifyResult, error) {
		return state.IdentifyIsPortalDaemon, nil
	})

	killCalls := 0
	installBarrierSendSIGKILL(t, func(int) error {
		killCalls++
		return nil
	})

	// Stay alive throughout the session-kill poll; after SIGKILL fires, return
	// true once more to exercise the poll loop, then false.
	postKillProbes := 0
	installBarrierIsAlive(t, func(int) bool {
		if killCalls == 0 {
			return true
		}
		postKillProbes++
		return postKillProbes < 2
	})

	log := &recordingBarrierLogger{}
	installBarrierLogger(t, log)

	script := &portalSaverScript{
		killSession: func(int) (string, error) { return "", nil },
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.KillSaverAndWaitForDaemon(client, t.TempDir()); err != nil {
		t.Fatalf("KillSaverAndWaitForDaemon returned error: %v", err)
	}

	if killCalls != 1 {
		t.Errorf("expected exactly 1 SIGKILL seam call, got %d", killCalls)
	}
	if len(log.warns) != 0 {
		t.Errorf("expected 0 WARN lines when process exits within window, got %d: %v", len(log.warns), log.warns)
	}
}

// TestKillSaverAndWaitForDaemon_Escalation_SIGKILLSucceedsButProcessSurvives_EmitsOneWarnAndReturnsNil
// pins that when SIGKILL is sent but the process is still alive at the end of
// killBarrierEscalationTimeout, exactly one WARN is emitted and the helper
// returns nil. Bootstrap is best-effort at this stage.
func TestKillSaverAndWaitForDaemon_Escalation_SIGKILLSucceedsButProcessSurvives_EmitsOneWarnAndReturnsNil(t *testing.T) {
	installBarrierPollInterval(t, 1*time.Millisecond)
	installBarrierTimeout(t, 5*time.Millisecond)
	installBarrierEscalationTimeout(t, 10*time.Millisecond)
	installBarrierReadPID(t, func(string) (int, error) { return 4321, nil })
	installBarrierIsAlive(t, func(int) bool { return true }) // never dies, even post-SIGKILL

	installBarrierIdentifyDaemon(t, func(int) (state.IdentifyResult, error) {
		return state.IdentifyIsPortalDaemon, nil
	})

	killCalls := 0
	installBarrierSendSIGKILL(t, func(int) error {
		killCalls++
		return nil
	})

	log := &recordingBarrierLogger{}
	installBarrierLogger(t, log)

	script := &portalSaverScript{
		killSession: func(int) (string, error) { return "", nil },
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.KillSaverAndWaitForDaemon(client, t.TempDir()); err != nil {
		t.Fatalf("KillSaverAndWaitForDaemon returned error: %v", err)
	}

	if killCalls != 1 {
		t.Errorf("expected exactly 1 SIGKILL seam call, got %d", killCalls)
	}
	if len(log.warns) != 1 {
		t.Errorf("expected exactly 1 WARN on persistent aliveness post-SIGKILL, got %d: %v", len(log.warns), log.warns)
	}
}

// TestKillSaverAndWaitForDaemon_Escalation_IdentityCheckIsImmediatelyPrecedingStatementToSIGKILL
// pins the load-bearing residual-window invariant: nothing other than the
// identity check itself must run between the identity verdict and the
// SIGKILL syscall. Verified by recording the relative call ordering of the
// identity seam and the SIGKILL seam across multiple escalation paths and
// asserting that within a single helper invocation, the identity seam is the
// last call recorded before the SIGKILL seam (no IsAlive/ReadPID/kill-session
// calls interleaved).
func TestKillSaverAndWaitForDaemon_Escalation_IdentityCheckIsImmediatelyPrecedingStatementToSIGKILL(t *testing.T) {
	installBarrierPollInterval(t, 1*time.Millisecond)
	installBarrierTimeout(t, 5*time.Millisecond)
	installBarrierEscalationTimeout(t, 50*time.Millisecond)

	var probeLog []string

	installBarrierReadPID(t, func(string) (int, error) {
		probeLog = append(probeLog, "readpid")
		return 4321, nil
	})

	installBarrierIdentifyDaemon(t, func(int) (state.IdentifyResult, error) {
		probeLog = append(probeLog, "identify")
		return state.IdentifyIsPortalDaemon, nil
	})

	installBarrierSendSIGKILL(t, func(int) error {
		probeLog = append(probeLog, "sigkill")
		return nil
	})

	// Alive throughout session-kill poll; dead after sigkill.
	killSent := false
	installBarrierIsAlive(t, func(int) bool {
		probeLog = append(probeLog, "isalive")
		return !killSent
	})

	// Mark killSent in the SIGKILL seam itself by re-wrapping. Use a flag
	// closure pattern: wrap SIGKILL seam after installation.
	prevKill := *tmux.BarrierSendSIGKILLSeam()
	*tmux.BarrierSendSIGKILLSeam() = func(pid int) error {
		err := prevKill(pid)
		killSent = true
		return err
	}

	log := &recordingBarrierLogger{}
	installBarrierLogger(t, log)

	script := &portalSaverScript{
		killSession: func(int) (string, error) {
			probeLog = append(probeLog, "killsession")
			return "", nil
		},
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.KillSaverAndWaitForDaemon(client, t.TempDir()); err != nil {
		t.Fatalf("KillSaverAndWaitForDaemon returned error: %v", err)
	}

	// Find the indices of identify and sigkill calls.
	identifyIdx, sigkillIdx := -1, -1
	for i, ev := range probeLog {
		if ev == "identify" && identifyIdx == -1 {
			identifyIdx = i
		}
		if ev == "sigkill" && sigkillIdx == -1 {
			sigkillIdx = i
		}
	}

	if identifyIdx == -1 {
		t.Fatalf("identify call not recorded; probeLog=%v", probeLog)
	}
	if sigkillIdx == -1 {
		t.Fatalf("sigkill call not recorded; probeLog=%v", probeLog)
	}

	// The load-bearing invariant: sigkill must be the IMMEDIATELY-following
	// recorded probe after identify. Any other event between them (readpid,
	// isalive, killsession) would mean a non-trivial statement ran inside
	// the residual-recycle window.
	if sigkillIdx != identifyIdx+1 {
		t.Errorf("expected sigkill (index=%d) to immediately follow identify (index=%d); intervening events: %v",
			sigkillIdx, identifyIdx, probeLog[identifyIdx+1:sigkillIdx])
	}
}

// TestKillSaverAndWaitForDaemon_Escalation_NeverSendsSIGTERM pins the
// no-SIGTERM-ever invariant. The only signal the helper may emit is SIGKILL
// through killBarrierSendSIGKILL; no other syscall.Kill / signal seam exists
// in this file. The test asserts that across the escalation path the
// SIGKILL seam is the only signalling seam invoked, and (by construction)
// confirms a SIGTERM-emitting alternative was not silently added.
func TestKillSaverAndWaitForDaemon_Escalation_NeverSendsSIGTERM(t *testing.T) {
	installBarrierPollInterval(t, 1*time.Millisecond)
	installBarrierTimeout(t, 5*time.Millisecond)
	installBarrierEscalationTimeout(t, 50*time.Millisecond)
	installBarrierReadPID(t, func(string) (int, error) { return 4321, nil })

	installBarrierIdentifyDaemon(t, func(int) (state.IdentifyResult, error) {
		return state.IdentifyIsPortalDaemon, nil
	})

	var signals []syscall.Signal
	killCalls := 0
	installBarrierSendSIGKILL(t, func(pid int) error {
		killCalls++
		signals = append(signals, syscall.SIGKILL)
		return nil
	})

	postKillProbes := 0
	installBarrierIsAlive(t, func(int) bool {
		if killCalls == 0 {
			return true
		}
		postKillProbes++
		return postKillProbes < 2
	})

	log := &recordingBarrierLogger{}
	installBarrierLogger(t, log)

	script := &portalSaverScript{
		killSession: func(int) (string, error) { return "", nil },
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.KillSaverAndWaitForDaemon(client, t.TempDir()); err != nil {
		t.Fatalf("KillSaverAndWaitForDaemon returned error: %v", err)
	}

	if len(signals) != 1 {
		t.Fatalf("expected exactly 1 signal emission via SIGKILL seam, got %d (%v)", len(signals), signals)
	}
	if signals[0] != syscall.SIGKILL {
		t.Errorf("expected SIGKILL, got %v", signals[0])
	}
}

// TestKillSaverAndWaitForDaemon_Escalation_PriorPIDDiesDuringSessionKillPoll_EscalationNeverRuns
// pins that when the prior PID exits inside the existing 5s session-kill
// poll window, the escalation path never runs — identity check is never
// invoked, SIGKILL seam is never invoked, and no WARN is emitted. Guards the
// legitimate saver-pane SIGHUP path: pane dies, no escalation.
func TestKillSaverAndWaitForDaemon_Escalation_PriorPIDDiesDuringSessionKillPoll_EscalationNeverRuns(t *testing.T) {
	installBarrierPollInterval(t, 1*time.Millisecond)
	installBarrierTimeout(t, 500*time.Millisecond)
	installBarrierEscalationTimeout(t, 50*time.Millisecond)
	installBarrierReadPID(t, func(string) (int, error) { return 4321, nil })

	// Alive on initial probe + first tick, then dead — exits inside the
	// session-kill poll.
	calls := 0
	installBarrierIsAlive(t, func(int) bool {
		calls++
		return calls < 3
	})

	identifyCalls := 0
	installBarrierIdentifyDaemon(t, func(int) (state.IdentifyResult, error) {
		identifyCalls++
		return state.IdentifyIsPortalDaemon, nil
	})

	killCalls := 0
	installBarrierSendSIGKILL(t, func(int) error {
		killCalls++
		return nil
	})

	log := &recordingBarrierLogger{}
	installBarrierLogger(t, log)

	script := &portalSaverScript{
		killSession: func(int) (string, error) { return "", nil },
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.KillSaverAndWaitForDaemon(client, t.TempDir()); err != nil {
		t.Fatalf("KillSaverAndWaitForDaemon returned error: %v", err)
	}

	if identifyCalls != 0 {
		t.Errorf("expected 0 identity checks when PID dies during session-kill poll, got %d", identifyCalls)
	}
	if killCalls != 0 {
		t.Errorf("expected 0 SIGKILL seam calls when PID dies during session-kill poll, got %d", killCalls)
	}
	if len(log.warns) != 0 {
		t.Errorf("expected 0 WARN lines when PID dies during session-kill poll, got %d: %v", len(log.warns), log.warns)
	}
}

// TestKillSaverAndWaitForDaemon_Escalation_NoPIDFile_EscalationNeverRuns pins
// that when saverReadPID returns ErrPIDFileAbsent, the existing
// short-circuit fast path runs (tolerant kill, no polling) and the escalation
// path is never entered — identity check is never invoked, SIGKILL seam is
// never invoked, no WARN.
func TestKillSaverAndWaitForDaemon_Escalation_NoPIDFile_EscalationNeverRuns(t *testing.T) {
	installBarrierPollInterval(t, 1*time.Millisecond)
	installBarrierTimeout(t, 50*time.Millisecond)
	installBarrierEscalationTimeout(t, 50*time.Millisecond)
	installBarrierReadPID(t, func(string) (int, error) { return 0, state.ErrPIDFileAbsent })

	identifyCalls := 0
	installBarrierIdentifyDaemon(t, func(int) (state.IdentifyResult, error) {
		identifyCalls++
		return state.IdentifyIsPortalDaemon, nil
	})

	killCalls := 0
	installBarrierSendSIGKILL(t, func(int) error {
		killCalls++
		return nil
	})

	log := &recordingBarrierLogger{}
	installBarrierLogger(t, log)

	script := &portalSaverScript{
		killSession: func(int) (string, error) { return "", nil },
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.KillSaverAndWaitForDaemon(client, t.TempDir()); err != nil {
		t.Fatalf("KillSaverAndWaitForDaemon returned error: %v", err)
	}

	if identifyCalls != 0 {
		t.Errorf("expected 0 identity checks when PID file absent, got %d", identifyCalls)
	}
	if killCalls != 0 {
		t.Errorf("expected 0 SIGKILL seam calls when PID file absent, got %d", killCalls)
	}
	if len(log.warns) != 0 {
		t.Errorf("expected 0 WARN lines when PID file absent, got %d: %v", len(log.warns), log.warns)
	}
}

// ----------------------------------------------------------------------------
// Task 4-4: SIGKILL-escalation DEBUG breadcrumb in escalateKillToSIGKILL.
//
// The escalation branch (the one firing SIGKILL after the identity check
// passes) emits ONE DEBUG breadcrumb "kill-barrier escalating to SIGKILL"
// carrying target_pid under component=saver, as the IMMEDIATELY-preceding
// statement to the SIGKILL syscall. It is a forensic decision-point detail
// beneath the saver: kill-barrier escalated INFO lifecycle event (Phase 5).
// The skip-WARN branch (identity check fails / transient error / dead) does
// NOT emit it.
//
// Spec § Diagnostic context preservation at boundaries (escalateKillToSIGKILL)
// and § Saver and daemon lifecycle event taxonomy (kill-barrier escalated).
// ----------------------------------------------------------------------------

const escalationBreadcrumbMessage = "kill-barrier escalating to SIGKILL"

// escalationDebugRecords filters captured records for the escalation DEBUG
// breadcrumb message.
func escalationDebugRecords(recs []slog.Record) []slog.Record {
	var out []slog.Record
	for _, r := range recs {
		if r.Message == escalationBreadcrumbMessage {
			out = append(out, r)
		}
	}
	return out
}

// TestEscalateKillToSIGKILL_EmitsDebugBreadcrumbWithTargetPIDOnEscalationBranch
// pins the escalation branch (IdentifyIsPortalDaemon) emits exactly one DEBUG
// "kill-barrier escalating to SIGKILL" breadcrumb carrying target_pid equal to
// the PID being SIGKILL'd, under component=saver.
func TestEscalateKillToSIGKILL_EmitsDebugBreadcrumbWithTargetPIDOnEscalationBranch(t *testing.T) {
	installBarrierPollInterval(t, 1*time.Millisecond)
	installBarrierTimeout(t, 5*time.Millisecond)
	installBarrierEscalationTimeout(t, 5*time.Millisecond)
	installBarrierReadPID(t, func(string) (int, error) { return 4321, nil })

	installBarrierIdentifyDaemon(t, func(int) (state.IdentifyResult, error) {
		return state.IdentifyIsPortalDaemon, nil
	})

	killCalls := 0
	installBarrierSendSIGKILL(t, func(int) error {
		killCalls++
		return nil
	})

	// Alive during session-kill poll; dead immediately after SIGKILL.
	installBarrierIsAlive(t, func(int) bool { return killCalls == 0 })

	rec := &recordingSlogHandler{}
	log.SetTestHandler(t, rec)

	script := &portalSaverScript{
		killSession: func(int) (string, error) { return "", nil },
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.KillSaverAndWaitForDaemon(client, t.TempDir()); err != nil {
		t.Fatalf("KillSaverAndWaitForDaemon returned error: %v", err)
	}

	breadcrumbs := escalationDebugRecords(rec.records)
	if len(breadcrumbs) != 1 {
		t.Fatalf("expected exactly 1 %q breadcrumb, got %d: %v", escalationBreadcrumbMessage, len(breadcrumbs), rec.records)
	}
	b := breadcrumbs[0]
	if b.Level != slog.LevelDebug {
		t.Errorf("breadcrumb level = %v, want DEBUG", b.Level)
	}

	var gotComponent string
	var gotTargetPID int64
	var sawTargetPID bool
	b.Attrs(func(a slog.Attr) bool {
		switch a.Key {
		case "component":
			gotComponent = a.Value.String()
		case "target_pid":
			gotTargetPID = a.Value.Int64()
			sawTargetPID = true
		}
		return true
	})
	if gotComponent != "saver" {
		t.Errorf("breadcrumb component = %q, want %q", gotComponent, "saver")
	}
	if !sawTargetPID {
		t.Fatalf("breadcrumb missing target_pid attr: %v", b)
	}
	if gotTargetPID != 4321 {
		t.Errorf("breadcrumb target_pid = %d, want 4321 (the SIGKILL'd PID)", gotTargetPID)
	}
}

// TestEscalateKillToSIGKILL_NoBreadcrumbOnSkipBranch pins that the escalation
// DEBUG breadcrumb is NOT emitted on any skip-WARN branch (IdentifyDead,
// IdentifyNotPortalDaemon, or a transient identity error) — only on the
// IdentifyIsPortalDaemon escalation path that fires SIGKILL.
func TestEscalateKillToSIGKILL_NoBreadcrumbOnSkipBranch(t *testing.T) {
	cases := []struct {
		name   string
		result state.IdentifyResult
		idErr  error
	}{
		{"IdentifyDead", state.IdentifyDead, nil},
		{"IdentifyNotPortalDaemon", state.IdentifyNotPortalDaemon, nil},
		{"TransientError", 0, errors.New("ps exec failed: transient")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			installBarrierPollInterval(t, 1*time.Millisecond)
			installBarrierTimeout(t, 5*time.Millisecond)
			installBarrierEscalationTimeout(t, 5*time.Millisecond)
			installBarrierReadPID(t, func(string) (int, error) { return 4321, nil })
			installBarrierIsAlive(t, func(int) bool { return true })

			installBarrierIdentifyDaemon(t, func(int) (state.IdentifyResult, error) {
				return tc.result, tc.idErr
			})

			killCalls := 0
			installBarrierSendSIGKILL(t, func(int) error {
				killCalls++
				return nil
			})

			rec := &recordingSlogHandler{}
			log.SetTestHandler(t, rec)

			script := &portalSaverScript{
				killSession: func(int) (string, error) { return "", nil },
			}
			mock := &MockCommander{RunFunc: script.run(t)}
			client := tmux.NewClient(mock)

			if err := tmux.KillSaverAndWaitForDaemon(client, t.TempDir()); err != nil {
				t.Fatalf("KillSaverAndWaitForDaemon returned error: %v", err)
			}

			if killCalls != 0 {
				t.Errorf("expected 0 SIGKILL seam calls on skip branch, got %d", killCalls)
			}
			if got := escalationDebugRecords(rec.records); len(got) != 0 {
				t.Errorf("expected 0 escalation breadcrumbs on skip branch, got %d: %v", len(got), got)
			}
		})
	}
}

// TestEscalateKillToSIGKILL_BreadcrumbEmittedBeforeSIGKILL pins the adjacency
// invariant from the breadcrumb's perspective: the DEBUG breadcrumb is emitted
// BEFORE the SIGKILL syscall. The SendSIGKILL seam snapshots the captured
// breadcrumb count at call time; the breadcrumb must already be recorded when
// SIGKILL fires.
func TestEscalateKillToSIGKILL_BreadcrumbEmittedBeforeSIGKILL(t *testing.T) {
	installBarrierPollInterval(t, 1*time.Millisecond)
	installBarrierTimeout(t, 5*time.Millisecond)
	installBarrierEscalationTimeout(t, 5*time.Millisecond)
	installBarrierReadPID(t, func(string) (int, error) { return 4321, nil })

	installBarrierIdentifyDaemon(t, func(int) (state.IdentifyResult, error) {
		return state.IdentifyIsPortalDaemon, nil
	})

	rec := &recordingSlogHandler{}
	log.SetTestHandler(t, rec)

	killCalls := 0
	breadcrumbsAtKillTime := -1
	installBarrierSendSIGKILL(t, func(int) error {
		// Snapshot the breadcrumb count at the moment SIGKILL is invoked. If the
		// breadcrumb is the immediately-preceding statement, exactly one
		// escalation breadcrumb must already be recorded here.
		breadcrumbsAtKillTime = len(escalationDebugRecords(rec.records))
		killCalls++
		return nil
	})

	installBarrierIsAlive(t, func(int) bool { return killCalls == 0 })

	script := &portalSaverScript{
		killSession: func(int) (string, error) { return "", nil },
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.KillSaverAndWaitForDaemon(client, t.TempDir()); err != nil {
		t.Fatalf("KillSaverAndWaitForDaemon returned error: %v", err)
	}

	if killCalls != 1 {
		t.Fatalf("expected exactly 1 SIGKILL seam call, got %d", killCalls)
	}
	if breadcrumbsAtKillTime != 1 {
		t.Errorf("expected the escalation breadcrumb to be recorded BEFORE SIGKILL (count at kill time = %d, want 1)", breadcrumbsAtKillTime)
	}
}
