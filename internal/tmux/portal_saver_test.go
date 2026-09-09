package tmux_test

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/commandertest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

func stubAliveCheck(t *testing.T, alive bool) {
	t.Helper()
	prev := tmux.BootstrapAliveCheck
	tmux.BootstrapAliveCheck = func(string) bool { return alive }
	t.Cleanup(func() { tmux.BootstrapAliveCheck = prev })
}

func shrinkRetryDelay(t *testing.T) {
	t.Helper()
	prev := tmux.PortalSaverRetryDelay
	tmux.PortalSaverRetryDelay = 1 * time.Microsecond
	t.Cleanup(func() { tmux.PortalSaverRetryDelay = prev })
}

func stubReadinessReady(t *testing.T) {
	t.Helper()
	swapSeam(t, tmux.WaitForSaverDaemonReadyFnSeam(), func(string) error { return nil })
}

// Shrunk package-wide so a test driving the barrier itself does not pay the
// production budgets, and the create-branch default is a barrier that reports
// ready: a create-path test whose subject is the tmux call sequence would
// otherwise fail on a readiness failure it never staged. A test that drives
// the barrier calls it directly, past this seam.
func init() {
	*tmux.SaverReadinessPollIntervalSeam() = 1 * time.Millisecond
	*tmux.SaverReadinessStallSeam() = 5 * time.Millisecond
	*tmux.SaverReadinessCeilingSeam() = 20 * time.Millisecond
	*tmux.WaitForSaverDaemonReadyFnSeam() = func(string) error { return nil }
}

type portalSaverScript struct {
	hasSession   func(call int) (string, error)
	newSession   func(call int) (string, error)
	killSession  func(call int) (string, error)
	setOption    func(call int) (string, error)
	respawnPane  func(call int) (string, error)
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

func saverScriptListPanesFormat(args []string) string {
	for i, a := range args {
		if a == "-F" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func countCalls(calls [][]string, name string) int {
	n := 0
	for _, c := range calls {
		if len(c) > 0 && c[0] == name {
			n++
		}
	}
	return n
}

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
	mock := commandertest.FromFunc(script.run(t))
	client := tmux.NewClient(mock)

	if err := tmux.BootstrapPortalSaver(client, "/tmp/portal-state"); err != nil {
		t.Fatalf("BootstrapPortalSaver returned error: %v", err)
	}

	if got := countCalls(mock.Calls(), "new-session"); got != 1 {
		t.Errorf("expected exactly 1 new-session call, got %d (calls: %v)", got, mock.Calls())
	}
	if got := countCalls(mock.Calls(), "set-option"); got != 1 {
		t.Errorf("expected exactly 1 set-option call, got %d (calls: %v)", got, mock.Calls())
	}
	if got := countCalls(mock.Calls(), "respawn-pane"); got != 1 {
		t.Errorf("expected exactly 1 respawn-pane call, got %d (calls: %v)", got, mock.Calls())
	}
	if got := countCalls(mock.Calls(), "kill-session"); got != 0 {
		t.Errorf("expected 0 kill-session calls, got %d", got)
	}

	wantNewSession := "new-session -d -s _portal-saver " + tmux.PortalSaverPlaceholderCommand
	for _, c := range mock.Calls() {
		if c[0] != "new-session" {
			continue
		}
		joined := strings.Join(c, " ")
		if joined != wantNewSession {
			t.Errorf("new-session argv = %q, want %q", joined, wantNewSession)
		}
	}

	wantRespawn := "respawn-pane -k -t " + string(tmux.CoordTargetExact(tmux.PortalSaverName)) + " " + tmux.PortalSaverDaemonCommand
	for _, c := range mock.Calls() {
		if c[0] != "respawn-pane" {
			continue
		}
		joined := strings.Join(c, " ")
		if joined != wantRespawn {
			t.Errorf("respawn-pane argv = %q, want %q", joined, wantRespawn)
		}
	}
}

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
	mock := commandertest.FromFunc(script.run(t))
	client := tmux.NewClient(mock)

	if err := tmux.BootstrapPortalSaver(client, "/tmp/portal-state"); err != nil {
		t.Fatalf("BootstrapPortalSaver returned error: %v", err)
	}

	newIdx, setIdx, respawnIdx := -1, -1, -1
	for i, c := range mock.Calls() {
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
		t.Fatalf("missing call: new=%d set=%d respawn=%d (calls=%v)", newIdx, setIdx, respawnIdx, mock.Calls())
	}
	if newIdx >= setIdx || setIdx >= respawnIdx {
		t.Errorf("expected create-then-set-option-then-respawn ordering; got new=%d set=%d respawn=%d (calls=%v)", newIdx, setIdx, respawnIdx, mock.Calls())
	}
}

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
	mock := commandertest.FromFunc(script.run(t))
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
	mock := commandertest.FromFunc(func(args ...string) (string, error) {
		if len(args) > 0 && args[0] == "new-session" {
			newSessionArgv = append([]string{}, args...)
		}
		return script.run(t)(args...)
	})
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
	if strings.Contains(strings.Join(newSessionArgv, " "), tmux.PortalSaverDaemonCommand) {
		t.Errorf("new-session argv unexpectedly contains daemon command: %v", newSessionArgv)
	}
}

func TestBootstrapPortalSaver_ConcurrentRaceTreatsExistingSessionAsSuccess_AndStillRespawns(t *testing.T) {
	stubAliveCheck(t, false)
	shrinkRetryDelay(t)

	hasSessionCall := 0
	newSessionCall := 0
	respawnPaneCall := 0
	setOptionCall := 0

	mock := commandertest.FromFunc(func(args ...string) (string, error) {
		switch args[0] {
		case "has-session":
			hasSessionCall++
			if hasSessionCall == 1 {
				return "", errors.New("can't find session")
			}
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
			if saverScriptListPanesFormat(args) == "#{pane_id}" {
				return "%0\n", nil
			}
			return "1\n", nil
		default:
			t.Fatalf("unexpected command: %v", args)
			return "", nil
		}
	})
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
		hasSession: func(call int) (string, error) { return "", nil },
		setOption:  func(call int) (string, error) { return "", nil },
	}
	mock := commandertest.FromFunc(script.run(t))
	client := tmux.NewClient(mock)

	if err := tmux.BootstrapPortalSaver(client, "/tmp/portal-state"); err != nil {
		t.Fatalf("BootstrapPortalSaver returned error: %v", err)
	}

	if got := countCalls(mock.Calls(), "new-session"); got != 0 {
		t.Errorf("expected 0 new-session calls, got %d (calls: %v)", got, mock.Calls())
	}
	if got := countCalls(mock.Calls(), "kill-session"); got != 0 {
		t.Errorf("expected 0 kill-session calls, got %d", got)
	}
	if got := countCalls(mock.Calls(), "set-option"); got != 1 {
		t.Errorf("expected exactly 1 set-option call, got %d", got)
	}
}

func TestBootstrapPortalSaver_KillsAndRecreatesWhenSessionExistsButDaemonDead(t *testing.T) {
	stubAliveCheck(t, false)
	shrinkRetryDelay(t)

	script := &portalSaverScript{
		hasSession:  func(call int) (string, error) { return "", nil },
		killSession: func(call int) (string, error) { return "", nil },
		newSession:  func(call int) (string, error) { return "", nil },
		setOption:   func(call int) (string, error) { return "", nil },
		respawnPane: func(call int) (string, error) { return "", nil },
	}
	mock := commandertest.FromFunc(script.run(t))
	client := tmux.NewClient(mock)

	if err := tmux.BootstrapPortalSaver(client, "/tmp/portal-state"); err != nil {
		t.Fatalf("BootstrapPortalSaver returned error: %v", err)
	}

	if got := countCalls(mock.Calls(), "kill-session"); got != 1 {
		t.Errorf("expected 1 kill-session call, got %d (calls: %v)", got, mock.Calls())
	}
	if got := countCalls(mock.Calls(), "new-session"); got != 1 {
		t.Errorf("expected 1 new-session call, got %d", got)
	}
	if got := countCalls(mock.Calls(), "set-option"); got != 1 {
		t.Errorf("expected 1 set-option call, got %d", got)
	}

	assertKillBeforeNew(t, mock.Calls())
}

func TestBootstrapPortalSaver_RecoversFromFlockLoserEmptySession(t *testing.T) {
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
	mock := commandertest.FromFunc(script.run(t))
	client := tmux.NewClient(mock)

	if err := tmux.BootstrapPortalSaver(client, "/tmp/portal-state"); err != nil {
		t.Fatalf("BootstrapPortalSaver returned error: %v", err)
	}

	if got := countCalls(mock.Calls(), "new-session"); got != 1 {
		t.Errorf("expected exactly 1 new-session call, got %d (calls: %v)", got, mock.Calls())
	}
	if got := countCalls(mock.Calls(), "set-option"); got != 1 {
		t.Errorf("expected exactly 1 set-option call, got %d (calls: %v)", got, mock.Calls())
	}
	if got := countCalls(mock.Calls(), "kill-session"); got != 0 {
		t.Errorf("expected 0 kill-session calls (no prior session to kill), got %d (calls: %v)", got, mock.Calls())
	}
}

func TestBootstrapPortalSaver_RecoversFromFlockLoserDeadPaneSession(t *testing.T) {
	stubAliveCheck(t, false)
	shrinkRetryDelay(t)

	script := &portalSaverScript{
		hasSession:  func(call int) (string, error) { return "", nil },
		killSession: func(call int) (string, error) { return "", nil },
		newSession:  func(call int) (string, error) { return "", nil },
		setOption:   func(call int) (string, error) { return "", nil },
		respawnPane: func(call int) (string, error) { return "", nil },
	}
	mock := commandertest.FromFunc(script.run(t))
	client := tmux.NewClient(mock)

	if err := tmux.BootstrapPortalSaver(client, "/tmp/portal-state"); err != nil {
		t.Fatalf("BootstrapPortalSaver returned error: %v", err)
	}

	if got := countCalls(mock.Calls(), "kill-session"); got != 1 {
		t.Errorf("expected 1 kill-session call, got %d (calls: %v)", got, mock.Calls())
	}
	if got := countCalls(mock.Calls(), "new-session"); got != 1 {
		t.Errorf("expected 1 new-session call, got %d (calls: %v)", got, mock.Calls())
	}
	if got := countCalls(mock.Calls(), "set-option"); got != 1 {
		t.Errorf("expected 1 set-option call, got %d (calls: %v)", got, mock.Calls())
	}

	assertKillBeforeNew(t, mock.Calls())
}

func TestBootstrapPortalSaver_AlwaysSetsDestroyUnattachedOff(t *testing.T) {
	stubAliveCheck(t, true)
	shrinkRetryDelay(t)

	var setOptionArgs []string
	script := &portalSaverScript{
		hasSession: func(call int) (string, error) { return "", nil },
		setOption:  func(call int) (string, error) { return "", nil },
	}
	mock := commandertest.FromFunc(func(args ...string) (string, error) {
		if len(args) > 0 && args[0] == "set-option" {
			setOptionArgs = append([]string{}, args...)
		}
		return script.run(t)(args...)
	})
	client := tmux.NewClient(mock)

	if err := tmux.BootstrapPortalSaver(client, "/tmp/portal-state"); err != nil {
		t.Fatalf("BootstrapPortalSaver returned error: %v", err)
	}

	wantArgs := []string{"set-option", "-t", "=_portal-saver:", "destroy-unattached", "off"}
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
	mock := commandertest.FromFunc(script.run(t))
	client := tmux.NewClient(mock)

	if err := tmux.BootstrapPortalSaver(client, "/tmp/portal-state"); err != nil {
		t.Fatalf("BootstrapPortalSaver returned error: %v", err)
	}

	for _, call := range mock.Calls() {
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

	mock := commandertest.FromFunc(func(args ...string) (string, error) {
		switch args[0] {
		case "has-session":
			hasSessionCall++
			return "", errors.New("can't find session")
		case "new-session":
			newSessionCall++
			if newSessionCall < 3 {
				return "", errors.New("transient tmux error")
			}
			return "", nil
		case "set-option":
			return "", nil
		case "respawn-pane":
			return "", nil
		case "list-panes":
			if saverScriptListPanesFormat(args) == "#{pane_id}" {
				return "%0\n", nil
			}
			return "1\n", nil
		default:
			t.Fatalf("unexpected command: %v", args)
			return "", nil
		}
	})
	client := tmux.NewClient(mock)

	if err := tmux.BootstrapPortalSaver(client, "/tmp/portal-state"); err != nil {
		t.Fatalf("BootstrapPortalSaver returned error: %v", err)
	}

	if newSessionCall != 3 {
		t.Errorf("expected 3 new-session calls, got %d", newSessionCall)
	}
	if got := countCalls(mock.Calls(), "set-option"); got != 1 {
		t.Errorf("expected 1 set-option call after retry success, got %d", got)
	}
	if got := countCalls(mock.Calls(), "respawn-pane"); got != 1 {
		t.Errorf("expected 1 respawn-pane call after retry success, got %d", got)
	}
}

func TestBootstrapPortalSaver_ReturnsWrappedErrorAfterRetryExhaustion(t *testing.T) {
	stubAliveCheck(t, false)
	shrinkRetryDelay(t)

	mock := commandertest.FromFunc(func(args ...string) (string, error) {
		switch args[0] {
		case "has-session":
			return "", errors.New("can't find session")
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
	})
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

	if got := countCalls(mock.Calls(), "new-session"); got != 3 {
		t.Errorf("expected exactly 3 new-session attempts, got %d", got)
	}
	if got := countCalls(mock.Calls(), "set-option"); got != 0 {
		t.Errorf("set-option must not run after retry exhaustion, got %d calls", got)
	}
	if got := countCalls(mock.Calls(), "respawn-pane"); got != 0 {
		t.Errorf("respawn-pane must not run after retry exhaustion, got %d calls", got)
	}
}

func TestBootstrapPortalSaver_ToleratesKillSessionFailureWhenTransitioningFromOrphan(t *testing.T) {
	stubAliveCheck(t, false)
	shrinkRetryDelay(t)

	script := &portalSaverScript{
		hasSession:  func(call int) (string, error) { return "", nil },
		killSession: func(call int) (string, error) { return "", errors.New("session vanished mid-flight") },
		newSession:  func(call int) (string, error) { return "", nil },
		setOption:   func(call int) (string, error) { return "", nil },
		respawnPane: func(call int) (string, error) { return "", nil },
	}
	mock := commandertest.FromFunc(script.run(t))
	client := tmux.NewClient(mock)

	if err := tmux.BootstrapPortalSaver(client, "/tmp/portal-state"); err != nil {
		t.Fatalf("BootstrapPortalSaver should tolerate kill failure, got: %v", err)
	}

	if got := countCalls(mock.Calls(), "kill-session"); got != 1 {
		t.Errorf("expected 1 kill-session call, got %d", got)
	}
	if got := countCalls(mock.Calls(), "new-session"); got != 1 {
		t.Errorf("expected creation to proceed despite kill failure, got %d new-session calls", got)
	}
	if got := countCalls(mock.Calls(), "set-option"); got != 1 {
		t.Errorf("expected 1 set-option call, got %d", got)
	}
}

func TestBootstrapPortalSaver_PropagatesSetOptionFailureWithSessionAndOptionName(t *testing.T) {
	stubAliveCheck(t, true)
	shrinkRetryDelay(t)

	script := &portalSaverScript{
		hasSession: func(call int) (string, error) { return "", nil },
		setOption:  func(call int) (string, error) { return "", errors.New("permission denied") },
	}
	mock := commandertest.FromFunc(script.run(t))
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

	mock := commandertest.FromFunc(func(args ...string) (string, error) {
		switch args[0] {
		case "has-session":
			hasSessionCall++
			if hasSessionCall == 1 {
				return "", errors.New("can't find session")
			}
			return "", nil
		case "new-session":
			newSessionCall++
			return "", errors.New("duplicate session: _portal-saver")
		case "set-option":
			return "", nil
		case "respawn-pane":
			return "", nil
		case "list-panes":
			if saverScriptListPanesFormat(args) == "#{pane_id}" {
				return "%0\n", nil
			}
			return "1\n", nil
		default:
			t.Fatalf("unexpected command: %v", args)
			return "", nil
		}
	})
	client := tmux.NewClient(mock)

	if err := tmux.BootstrapPortalSaver(client, "/tmp/portal-state"); err != nil {
		t.Fatalf("expected concurrent-bootstrap race to be treated as success, got: %v", err)
	}

	if newSessionCall != 1 {
		t.Errorf("expected exactly 1 new-session attempt before race detected, got %d", newSessionCall)
	}
	if got := countCalls(mock.Calls(), "set-option"); got != 1 {
		t.Errorf("expected set-option to still run after race detected, got %d calls", got)
	}
}

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

func newVersionScenarioClient(t *testing.T, sessionPresent bool) (*versionScenario, *commandertest.Scripted, *tmux.Client) {
	t.Helper()
	scenario := &versionScenario{sessionPresent: sessionPresent}
	mock := commandertest.FromFunc(scenario.run(t))
	return scenario, mock, tmux.NewClient(mock)
}

func recordBarrierCalls(t *testing.T) *int {
	t.Helper()
	calls := 0
	installKillSaverFn(t, func(*tmux.Client, string) error {
		calls++
		return nil
	})
	return &calls
}

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

	assertKillBeforeNew(t, mock.Calls())
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

func TestEnsurePortalSaverVersion_SkipsKillWhenNoSessionExists(t *testing.T) {
	stubAliveCheck(t, false)
	shrinkRetryDelay(t)

	dir := t.TempDir()

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
	stubAliveCheck(t, true)
	shrinkRetryDelay(t)

	dir := t.TempDir()
	writeVersion(t, dir, "v0.4.1")

	scenario := &versionScenario{
		sessionPresent: true,
		killSessionErr: errors.New("can't find session: _portal-saver"),
	}
	mock := commandertest.FromFunc(scenario.run(t))
	client := tmux.NewClient(mock)

	if err := tmux.EnsurePortalSaverVersion(client, dir, "v0.4.2"); err != nil {
		t.Fatalf("EnsurePortalSaverVersion must tolerate kill-session error, got: %v", err)
	}

	if scenario.killSessionCalls != 1 {
		t.Errorf("expected exactly 1 kill-session call, got %d", scenario.killSessionCalls)
	}
	if scenario.setOptionCalls != 1 {
		t.Errorf("expected exactly 1 set-option call after tolerated kill error, got %d", scenario.setOptionCalls)
	}
}

func TestEnsurePortalSaverVersion_AlwaysInvokesBootstrapPortalSaver(t *testing.T) {
	stubAliveCheck(t, true)
	shrinkRetryDelay(t)

	dir := t.TempDir()
	writeVersion(t, dir, "v0.4.2")

	var setOptionArgs []string
	scenario := &versionScenario{sessionPresent: true}
	mock := commandertest.FromFunc(func(args ...string) (string, error) {
		if len(args) > 0 && args[0] == "set-option" {
			setOptionArgs = append([]string{}, args...)
		}
		return scenario.run(t)(args...)
	})
	client := tmux.NewClient(mock)

	if err := tmux.EnsurePortalSaverVersion(client, dir, "v0.4.2"); err != nil {
		t.Fatalf("EnsurePortalSaverVersion returned error: %v", err)
	}

	wantArgs := []string{"set-option", "-t", "=_portal-saver:", "destroy-unattached", "off"}
	if len(setOptionArgs) != len(wantArgs) {
		t.Fatalf("set-option argv = %v, want %v", setOptionArgs, wantArgs)
	}
	for i, arg := range wantArgs {
		if setOptionArgs[i] != arg {
			t.Errorf("set-option arg[%d] = %q, want %q", i, setOptionArgs[i], arg)
		}
	}
}

func TestEnsurePortalSaverVersion_DoesNotWriteDaemonVersionOnKillPath(t *testing.T) {
	stubAliveCheck(t, true)
	shrinkRetryDelay(t)

	dir := t.TempDir()
	writeVersion(t, dir, "dev")

	before, err := os.ReadFile(state.DaemonVersion(dir))
	if err != nil {
		t.Fatalf("read daemon.version: %v", err)
	}

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

// barrierLog captures the kill-barrier's WARN lines as "<component> | <message>".
type barrierLog struct {
	sink logtest.Sink
}

func (b *barrierLog) Logger() *slog.Logger { return slog.New(&b.sink) }

func (b *barrierLog) warns() []string {
	var out []string
	for _, rec := range b.sink.Records().AtExactLevel(slog.LevelWarn) {
		out = append(out, rec.AttrOrEmpty("component")+" | "+rec.Msg)
	}
	return out
}

func swapSeam[T any](t *testing.T, ptr *T, v T) {
	t.Helper()
	prev := *ptr
	*ptr = v
	t.Cleanup(func() { *ptr = prev })
}

func installBarrierReadPID(t *testing.T, fn func(string) (int, error)) {
	t.Helper()
	swapSeam(t, tmux.SaverReadPIDSeam(), fn)
}

func installBarrierIsAlive(t *testing.T, fn func(int) bool) {
	t.Helper()
	swapSeam(t, tmux.BarrierIsAliveSeam(), fn)
}

func installBarrierPollInterval(t *testing.T, d time.Duration) {
	t.Helper()
	swapSeam(t, tmux.BarrierPollIntervalSeam(), d)
}

func installBarrierTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	swapSeam(t, tmux.BarrierTimeoutSeam(), d)
}

func installBarrierLogger(t *testing.T, log *barrierLog) {
	t.Helper()
	swapSeam(t, tmux.BarrierLoggerSeam(), log.Logger().With("component", "bootstrap"))
}

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
		out[e.Name()] = info.ModTime().UTC().Format(time.RFC3339Nano) + "|" + strconv.FormatInt(info.Size(), 10)
	}
	return out
}

func TestKillSaverAndWaitForDaemon_ReturnsNilWithNoWarnWhenPriorPIDDiesBeforeTimeout(t *testing.T) {
	installBarrierPollInterval(t, 1*time.Millisecond)
	installBarrierTimeout(t, 500*time.Millisecond)
	installBarrierReadPID(t, func(string) (int, error) { return 4321, nil })

	calls := 0
	installBarrierIsAlive(t, func(pid int) bool {
		calls++
		if pid != 4321 {
			t.Errorf("IsProcessAlive called with pid=%d; want 4321", pid)
		}
		return calls < 3
	})
	log := &barrierLog{}
	installBarrierLogger(t, log)

	script := &portalSaverScript{
		killSession: func(call int) (string, error) { return "", nil },
	}
	mock := commandertest.FromFunc(script.run(t))
	client := tmux.NewClient(mock)

	if err := tmux.KillSaverAndWaitForDaemon(client, t.TempDir()); err != nil {
		t.Fatalf("killSaverAndWaitForDaemon returned error: %v", err)
	}

	if got := countCalls(mock.Calls(), "kill-session"); got != 1 {
		t.Errorf("expected exactly 1 kill-session call, got %d (calls: %v)", got, mock.Calls())
	}
	if len(log.warns()) != 0 {
		t.Errorf("expected 0 WARN lines on clean exit, got %d: %v", len(log.warns()), log.warns())
	}
}

func TestKillSaverAndWaitForDaemon_EmitsOneWarnAndReturnsNilWhenPriorPIDNeverDies(t *testing.T) {
	installBarrierPollInterval(t, 1*time.Millisecond)
	installBarrierTimeout(t, 20*time.Millisecond)
	installBarrierReadPID(t, func(string) (int, error) { return 4321, nil })
	installBarrierIsAlive(t, func(int) bool { return true })
	log := &barrierLog{}
	installBarrierLogger(t, log)

	script := &portalSaverScript{
		killSession: func(call int) (string, error) { return "", nil },
	}
	mock := commandertest.FromFunc(script.run(t))
	client := tmux.NewClient(mock)

	start := time.Now()
	if err := tmux.KillSaverAndWaitForDaemon(client, t.TempDir()); err != nil {
		t.Fatalf("killSaverAndWaitForDaemon returned error: %v", err)
	}
	elapsed := time.Since(start)

	if got := countCalls(mock.Calls(), "kill-session"); got != 1 {
		t.Errorf("expected exactly 1 kill-session call, got %d", got)
	}
	if len(log.warns()) != 1 {
		t.Errorf("expected exactly 1 WARN line on timeout, got %d: %v", len(log.warns()), log.warns())
	}
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
	log := &barrierLog{}
	installBarrierLogger(t, log)

	script := &portalSaverScript{
		killSession: func(call int) (string, error) { return "", nil },
	}
	mock := commandertest.FromFunc(script.run(t))
	client := tmux.NewClient(mock)

	if err := tmux.KillSaverAndWaitForDaemon(client, t.TempDir()); err != nil {
		t.Fatalf("killSaverAndWaitForDaemon returned error: %v", err)
	}

	if got := countCalls(mock.Calls(), "kill-session"); got != 1 {
		t.Errorf("expected exactly 1 kill-session call, got %d", got)
	}
	if aliveCalls != 0 {
		t.Errorf("expected 0 IsProcessAlive probes when PID file absent, got %d", aliveCalls)
	}
	if len(log.warns()) != 0 {
		t.Errorf("expected 0 WARN lines when PID file absent, got %d: %v", len(log.warns()), log.warns())
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
	log := &barrierLog{}
	installBarrierLogger(t, log)

	script := &portalSaverScript{
		killSession: func(call int) (string, error) { return "", nil },
	}
	mock := commandertest.FromFunc(script.run(t))
	client := tmux.NewClient(mock)

	if err := tmux.KillSaverAndWaitForDaemon(client, t.TempDir()); err != nil {
		t.Fatalf("killSaverAndWaitForDaemon returned error: %v", err)
	}

	if got := countCalls(mock.Calls(), "kill-session"); got != 1 {
		t.Errorf("expected exactly 1 kill-session call, got %d", got)
	}
	if aliveCalls != 0 {
		t.Errorf("expected 0 IsProcessAlive probes on parse error, got %d", aliveCalls)
	}
	if len(log.warns()) != 0 {
		t.Errorf("expected 0 WARN lines on parse error, got %d: %v", len(log.warns()), log.warns())
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
	log := &barrierLog{}
	installBarrierLogger(t, log)

	script := &portalSaverScript{
		killSession: func(call int) (string, error) { return "", nil },
	}
	mock := commandertest.FromFunc(script.run(t))
	client := tmux.NewClient(mock)

	if err := tmux.KillSaverAndWaitForDaemon(client, t.TempDir()); err != nil {
		t.Fatalf("killSaverAndWaitForDaemon returned error: %v", err)
	}

	if got := countCalls(mock.Calls(), "kill-session"); got != 1 {
		t.Errorf("expected exactly 1 kill-session call, got %d", got)
	}
	if aliveCalls != 0 {
		t.Errorf("expected 0 IsProcessAlive probes on read error, got %d", aliveCalls)
	}
	if len(log.warns()) != 0 {
		t.Errorf("expected 0 WARN lines on read error, got %d: %v", len(log.warns()), log.warns())
	}
}

func TestKillSaverAndWaitForDaemon_SkipsPollingWhenPriorPIDAlreadyDead(t *testing.T) {
	installBarrierPollInterval(t, 1*time.Millisecond)
	installBarrierTimeout(t, 50*time.Millisecond)
	installBarrierReadPID(t, func(string) (int, error) { return 4321, nil })

	aliveCalls := 0
	installBarrierIsAlive(t, func(pid int) bool {
		aliveCalls++
		return false
	})
	log := &barrierLog{}
	installBarrierLogger(t, log)

	script := &portalSaverScript{
		killSession: func(call int) (string, error) { return "", nil },
	}
	mock := commandertest.FromFunc(script.run(t))
	client := tmux.NewClient(mock)

	if err := tmux.KillSaverAndWaitForDaemon(client, t.TempDir()); err != nil {
		t.Fatalf("killSaverAndWaitForDaemon returned error: %v", err)
	}

	if got := countCalls(mock.Calls(), "kill-session"); got != 1 {
		t.Errorf("expected exactly 1 kill-session call, got %d", got)
	}
	if aliveCalls != 1 {
		t.Errorf("expected exactly 1 IsProcessAlive probe (then short-circuit), got %d", aliveCalls)
	}
	if len(log.warns()) != 0 {
		t.Errorf("expected 0 WARN lines when prior PID already dead, got %d: %v", len(log.warns()), log.warns())
	}
}

func TestKillSaverAndWaitForDaemon_ToleratesFailingKillSession(t *testing.T) {
	installBarrierPollInterval(t, 1*time.Millisecond)
	installBarrierTimeout(t, 50*time.Millisecond)
	installBarrierReadPID(t, func(string) (int, error) { return 4321, nil })
	installBarrierIsAlive(t, func(int) bool { return false })
	log := &barrierLog{}
	installBarrierLogger(t, log)

	script := &portalSaverScript{
		killSession: func(call int) (string, error) {
			return "", errors.New("session vanished mid-flight")
		},
	}
	mock := commandertest.FromFunc(script.run(t))
	client := tmux.NewClient(mock)

	if err := tmux.KillSaverAndWaitForDaemon(client, t.TempDir()); err != nil {
		t.Fatalf("killSaverAndWaitForDaemon must tolerate kill-session error, got: %v", err)
	}

	if got := countCalls(mock.Calls(), "kill-session"); got != 1 {
		t.Errorf("expected exactly 1 kill-session call even when it errors, got %d", got)
	}
	if len(log.warns()) != 0 {
		t.Errorf("expected 0 WARN lines on tolerated kill error, got %d: %v", len(log.warns()), log.warns())
	}
}

func TestKillSaverAndWaitForDaemon_DoesNotMutateStateDirectory(t *testing.T) {
	installBarrierPollInterval(t, 1*time.Millisecond)
	installBarrierTimeout(t, 20*time.Millisecond)
	installBarrierReadPID(t, func(string) (int, error) { return 4321, nil })
	installBarrierIsAlive(t, func(int) bool { return true })

	log := &barrierLog{}
	installBarrierLogger(t, log)

	dir := t.TempDir()
	sentinel := dir + "/sentinel"
	if err := os.WriteFile(sentinel, []byte("untouched\n"), 0o600); err != nil {
		t.Fatalf("seed sentinel: %v", err)
	}

	before := snapshotDir(t, dir)

	script := &portalSaverScript{
		killSession: func(call int) (string, error) { return "", nil },
	}
	mock := commandertest.FromFunc(script.run(t))
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

type barrierCall struct {
	client   *tmux.Client
	stateDir string
}

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
	if got := countCalls(mock.Calls(), "kill-session"); got != 0 {
		t.Errorf("expected 0 direct kill-session calls when helper is stubbed, got %d (calls: %v)", got, mock.Calls())
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
	stubAliveCheck(t, false)
	shrinkRetryDelay(t)

	dir := t.TempDir()

	var calls []barrierCall
	installKillSaverFn(t, func(c *tmux.Client, sd string) error {
		calls = append(calls, barrierCall{client: c, stateDir: sd})
		return nil
	})

	script := &portalSaverScript{
		hasSession:  func(int) (string, error) { return "", nil },
		newSession:  func(int) (string, error) { return "", nil },
		setOption:   func(int) (string, error) { return "", nil },
		respawnPane: func(int) (string, error) { return "", nil },
	}
	mock := commandertest.FromFunc(script.run(t))
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
	if got := countCalls(mock.Calls(), "kill-session"); got != 0 {
		t.Errorf("expected 0 direct kill-session calls when helper is stubbed, got %d (calls: %v)", got, mock.Calls())
	}
}

func TestBootstrapPortalSaver_DoesNotInvokeBarrierHelperWhenSessionAbsent(t *testing.T) {
	stubAliveCheck(t, false)
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
	mock := commandertest.FromFunc(script.run(t))
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
		hasSession: func(int) (string, error) { return "", nil },
		setOption:  func(int) (string, error) { return "", nil },
	}
	mock := commandertest.FromFunc(script.run(t))
	client := tmux.NewClient(mock)

	if err := tmux.BootstrapPortalSaver(client, dir); err != nil {
		t.Fatalf("BootstrapPortalSaver returned error: %v", err)
	}

	if calls != 0 {
		t.Errorf("expected 0 barrier invocations when daemon alive, got %d", calls)
	}
}

func TestBootstrapPortalSaver_PreservesKillSessionWhenRealHelperRuns(t *testing.T) {
	stubAliveCheck(t, false)
	shrinkRetryDelay(t)

	installBarrierReadPID(t, func(string) (int, error) { return 0, state.ErrPIDFileAbsent })

	dir := t.TempDir()

	script := &portalSaverScript{
		hasSession:  func(int) (string, error) { return "", nil },
		killSession: func(int) (string, error) { return "", nil },
		newSession:  func(int) (string, error) { return "", nil },
		setOption:   func(int) (string, error) { return "", nil },
		respawnPane: func(int) (string, error) { return "", nil },
	}
	mock := commandertest.FromFunc(script.run(t))
	client := tmux.NewClient(mock)

	if err := tmux.BootstrapPortalSaver(client, dir); err != nil {
		t.Fatalf("BootstrapPortalSaver returned error: %v", err)
	}

	if got := countCalls(mock.Calls(), "kill-session"); got != 1 {
		t.Errorf("expected exactly 1 kill-session via real helper fast path, got %d (calls: %v)", got, mock.Calls())
	}
	if got := countCalls(mock.Calls(), "new-session"); got != 1 {
		t.Errorf("expected 1 new-session call, got %d", got)
	}
}

func TestBootstrapPortalSaver_PreservesKillBeforeNewSessionOrderThroughBarrier(t *testing.T) {
	stubAliveCheck(t, false)
	shrinkRetryDelay(t)
	installBarrierReadPID(t, func(string) (int, error) { return 0, state.ErrPIDFileAbsent })

	dir := t.TempDir()

	script := &portalSaverScript{
		hasSession:  func(int) (string, error) { return "", nil },
		killSession: func(int) (string, error) { return "", nil },
		newSession:  func(int) (string, error) { return "", nil },
		setOption:   func(int) (string, error) { return "", nil },
		respawnPane: func(int) (string, error) { return "", nil },
	}
	mock := commandertest.FromFunc(script.run(t))
	client := tmux.NewClient(mock)

	if err := tmux.BootstrapPortalSaver(client, dir); err != nil {
		t.Fatalf("BootstrapPortalSaver returned error: %v", err)
	}

	assertKillBeforeNew(t, mock.Calls())
}

func TestBootstrapPortalSaver_ToleratesBarrierWarnOnTimeoutPath(t *testing.T) {
	stubAliveCheck(t, false)
	shrinkRetryDelay(t)
	stubReadinessReady(t)
	installBarrierReadPID(t, func(string) (int, error) { return 4321, nil })
	installBarrierIsAlive(t, func(int) bool { return true })
	installBarrierPollInterval(t, 1*time.Millisecond)
	installBarrierTimeout(t, 10*time.Millisecond)
	log := &barrierLog{}
	installBarrierLogger(t, log)

	dir := t.TempDir()

	script := &portalSaverScript{
		hasSession:  func(int) (string, error) { return "", nil },
		killSession: func(int) (string, error) { return "", nil },
		newSession:  func(int) (string, error) { return "", nil },
		setOption:   func(int) (string, error) { return "", nil },
		respawnPane: func(int) (string, error) { return "", nil },
	}
	mock := commandertest.FromFunc(script.run(t))
	client := tmux.NewClient(mock)

	if err := tmux.BootstrapPortalSaver(client, dir); err != nil {
		t.Fatalf("BootstrapPortalSaver must tolerate barrier WARN-on-timeout, got: %v", err)
	}

	if len(log.warns()) != 1 {
		t.Errorf("expected exactly 1 WARN on timeout, got %d: %v", len(log.warns()), log.warns())
	}
	if got := countCalls(mock.Calls(), "new-session"); got != 1 {
		t.Errorf("expected new-session to proceed after barrier timeout, got %d new-session calls", got)
	}
}

func TestSetBarrierLogger_RoutesWarnOnTimeoutThroughInstalledLogger(t *testing.T) {
	loggerSeam := tmux.BarrierLoggerSeam()
	prevLogger := *loggerSeam
	t.Cleanup(func() { *loggerSeam = prevLogger })

	recorder := &barrierLog{}
	tmux.SetBarrierLogger(recorder.Logger().With("component", "bootstrap"))

	installBarrierPollInterval(t, 1*time.Millisecond)
	installBarrierTimeout(t, 10*time.Millisecond)
	installBarrierReadPID(t, func(string) (int, error) { return 4321, nil })
	installBarrierIsAlive(t, func(int) bool { return true })

	script := &portalSaverScript{
		killSession: func(int) (string, error) { return "", nil },
	}
	mock := commandertest.FromFunc(script.run(t))
	client := tmux.NewClient(mock)

	if err := tmux.KillSaverAndWaitForDaemon(client, t.TempDir()); err != nil {
		t.Fatalf("KillSaverAndWaitForDaemon returned error: %v", err)
	}

	if len(recorder.warns()) != 1 {
		t.Fatalf("expected exactly 1 WARN routed through SetBarrierLogger, got %d: %v", len(recorder.warns()), recorder.warns())
	}
	if !strings.HasPrefix(recorder.warns()[0], "bootstrap"+" | ") {
		t.Errorf("WARN component prefix = %q, want %q", recorder.warns()[0], "bootstrap"+" | ")
	}
}

func TestSetBarrierLogger_IgnoresNilLogger(t *testing.T) {
	loggerSeam := tmux.BarrierLoggerSeam()
	prevLogger := *loggerSeam
	t.Cleanup(func() { *loggerSeam = prevLogger })

	recorder := &barrierLog{}
	installed := recorder.Logger().With("component", "bootstrap")
	tmux.SetBarrierLogger(installed)
	tmux.SetBarrierLogger(nil)

	if *loggerSeam != installed {
		t.Errorf("SetBarrierLogger(nil) overwrote the previously installed logger")
	}
}

func installReadVersionFile(t *testing.T, fn func(string) (string, error)) {
	t.Helper()
	swapSeam(t, tmux.PortalSaverReadVersionFileSeam(), fn)
}

func TestEnsurePortalSaverVersion_NotAlive_AbsentVersion_DoesNotKill(t *testing.T) {
	stubAliveCheck(t, false)
	shrinkRetryDelay(t)

	dir := t.TempDir()

	barrierCalls := recordBarrierCalls(t)

	_, _, client := newVersionScenarioClient(t, true)

	if err := tmux.EnsurePortalSaverVersion(client, dir, "v0.4.2"); err != nil {
		t.Fatalf("EnsurePortalSaverVersion returned error: %v", err)
	}

	if *barrierCalls > 1 {
		t.Errorf("expected at most 1 barrier invocation when daemon not alive (only BootstrapPortalSaver's stale-daemon branch), got %d", *barrierCalls)
	}
}

func TestEnsurePortalSaverVersion_NotAlive_VersionMismatch_DoesNotKill(t *testing.T) {
	stubAliveCheck(t, false)
	shrinkRetryDelay(t)

	dir := t.TempDir()
	writeVersion(t, dir, "v0.4.1")

	barrierCalls := recordBarrierCalls(t)

	_, _, client := newVersionScenarioClient(t, true)

	if err := tmux.EnsurePortalSaverVersion(client, dir, "v0.4.2"); err != nil {
		t.Fatalf("EnsurePortalSaverVersion returned error: %v", err)
	}

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

	dir := t.TempDir()

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

func TestEnsurePortalSaverVersion_ConsultsAliveCheckBeforeVersionMismatchDecision(t *testing.T) {
	shrinkRetryDelay(t)

	var aliveCalls int
	prevAlive := tmux.BootstrapAliveCheck
	tmux.BootstrapAliveCheck = func(string) bool {
		aliveCalls++
		return false
	}
	t.Cleanup(func() { tmux.BootstrapAliveCheck = prevAlive })

	installReadVersionFile(t, func(string) (string, error) {
		return "v0.4.1", nil
	})

	barrierCalls := recordBarrierCalls(t)

	dir := t.TempDir()
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

	tmux.BootstrapAliveCheck = func(string) bool { return true }
	scenario2 := &versionScenario{sessionPresent: true}
	mock2 := commandertest.FromFunc(scenario2.run(t))
	client2 := tmux.NewClient(mock2)
	if err := tmux.EnsurePortalSaverVersion(client2, dir, "v0.4.2"); err != nil {
		t.Fatalf("EnsurePortalSaverVersion (alive=true) returned error: %v", err)
	}
	if *barrierCalls != 1 {
		t.Errorf("expected exactly 1 barrier invocation on alive=true + mismatch, got %d", *barrierCalls)
	}
}

func TestShouldKillSaverOnVersionDecision_PredicateMatrix(t *testing.T) {
	cases := []struct {
		name           string
		stored         string
		currentVersion string
		readErr        error
		want           bool
	}{
		{
			name:           "equal_non_dev_match",
			stored:         "0.5.0",
			currentVersion: "0.5.0",
			readErr:        nil,
			want:           false,
		},
		{
			name:           "mismatched_non_dev",
			stored:         "0.5.0",
			currentVersion: "0.5.1",
			readErr:        nil,
			want:           true,
		},
		{
			name:           "readErr_ErrVersionFileAbsent_no_kill",
			stored:         "",
			currentVersion: "0.5.0",
			readErr:        state.ErrVersionFileAbsent,
			want:           false,
		},
		{
			name:           "readErr_non_absent_io_error",
			stored:         "",
			currentVersion: "0.5.0",
			readErr:        fs.ErrPermission,
			want:           true,
		},
		{
			name:           "dev_version_stored",
			stored:         "dev",
			currentVersion: "0.5.0",
			readErr:        nil,
			want:           true,
		},
		{
			name:           "dev_version_current",
			stored:         "0.5.0",
			currentVersion: "dev",
			readErr:        nil,
			want:           true,
		},
		{
			name:           "empty_stored",
			stored:         "",
			currentVersion: "0.5.0",
			readErr:        nil,
			want:           true,
		},
		{
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

func installWriteVersionFile(t *testing.T, fn func(string, string) error) {
	t.Helper()
	swapSeam(t, tmux.PortalSaverWriteVersionFileSeam(), fn)
}

type defensiveWriteCall struct {
	dir     string
	version string
}

func TestEnsurePortalSaverVersion_Alive_Absent_InvokesDefensiveWriteBeforeBootstrap(t *testing.T) {
	stubAliveCheck(t, true)
	shrinkRetryDelay(t)

	dir := t.TempDir()

	var order []string
	var writes []defensiveWriteCall
	installWriteVersionFile(t, func(d, v string) error {
		order = append(order, "write")
		writes = append(writes, defensiveWriteCall{dir: d, version: v})
		return nil
	})

	scenario := &versionScenario{sessionPresent: true}
	mock := commandertest.FromFunc(func(args ...string) (string, error) {
		if len(args) > 0 {
			order = append(order, args[0])
		}
		return scenario.run(t)(args...)
	})
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

	dir := t.TempDir()

	sentinel := errors.New("read-only filesystem")
	installWriteVersionFile(t, func(string, string) error {
		return sentinel
	})

	mock := commandertest.FromFunc(func(args ...string) (string, error) {
		t.Fatalf("BootstrapPortalSaver was invoked despite defensive write failure: %v", args)
		return "", nil
	})
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

func TestSetVersionWriterLogger_BootstrapWrapperEmitsDebugBreadcrumb(t *testing.T) {
	dir := t.TempDir()

	sink := &logtest.Sink{}
	lg := slog.New(sink).With("component", "daemon")

	loggerSeam := tmux.VersionWriterLoggerSeam()
	prev := *loggerSeam
	t.Cleanup(func() { *loggerSeam = prev })

	tmux.SetVersionWriterLogger(lg)

	wrapper := *tmux.PortalSaverWriteVersionFileSeam()
	if err := wrapper(dir, "v9.9.9"); err != nil {
		t.Fatalf("portalSaverWriteVersionFile: %v", err)
	}

	b := sink.Records().WithMessage("daemon.version write").Only(t, "daemon.version write record")
	if b.Level != slog.LevelDebug {
		t.Errorf("breadcrumb level = %v, want DEBUG", b.Level)
	}
	gotComponent := b.AttrOrEmpty("component")
	gotPath := b.AttrOrEmpty("path")
	if gotComponent != "daemon" {
		t.Errorf("breadcrumb component = %q, want %q", gotComponent, "daemon")
	}
	wantPath := filepath.Join(dir, "daemon.version")
	if gotPath != wantPath {
		t.Errorf("breadcrumb path = %q, want %q", gotPath, wantPath)
	}
}

func TestSetVersionWriterLogger_IgnoresNilLogger(t *testing.T) {
	lg := slog.New(&logtest.Sink{}).With("component", "daemon")

	loggerSeam := tmux.VersionWriterLoggerSeam()
	prev := *loggerSeam
	t.Cleanup(func() { *loggerSeam = prev })

	tmux.SetVersionWriterLogger(lg)
	tmux.SetVersionWriterLogger(nil)

	if *loggerSeam != lg {
		t.Errorf("SetVersionWriterLogger(nil) overwrote the previously installed logger")
	}
}

func TestEnsurePortalSaverVersion_NotAlive_Absent_DoesNotInvokeDefensiveWrite(t *testing.T) {
	stubAliveCheck(t, false)
	shrinkRetryDelay(t)

	dir := t.TempDir()

	writeCalls := 0
	installWriteVersionFile(t, func(string, string) error {
		writeCalls++
		return nil
	})

	installKillSaverFn(t, func(*tmux.Client, string) error { return nil })

	_, _, client := newVersionScenarioClient(t, true)

	if err := tmux.EnsurePortalSaverVersion(client, dir, "v0.4.2"); err != nil {
		t.Fatalf("EnsurePortalSaverVersion returned error: %v", err)
	}

	if writeCalls != 0 {
		t.Errorf("expected 0 defensive write calls when daemon not alive, got %d", writeCalls)
	}
}

func TestPortalSaverPlaceholderCommand_LiteralValue(t *testing.T) {
	const want = "sh -c 'exec tail -f /dev/null'"
	if got := tmux.PortalSaverPlaceholderCommand; got != want {
		t.Errorf("PortalSaverPlaceholderCommand = %q, want %q", got, want)
	}
}

func TestPortalSaverDaemonCommand_LiteralValue(t *testing.T) {
	const want = "portal state daemon"
	if got := tmux.PortalSaverDaemonCommand; got != want {
		t.Errorf("PortalSaverDaemonCommand = %q, want %q", got, want)
	}
}

func installReadinessReadPID(t *testing.T, fn func(string) (int, error)) {
	t.Helper()
	swapSeam(t, tmux.SaverReadPIDSeam(), fn)
}

func installReadinessIdentify(t *testing.T, fn func(int) (state.IdentifyResult, error)) {
	t.Helper()
	swapSeam(t, tmux.SaverIdentifyDaemonSeam(), fn)
}

func installReadinessPollInterval(t *testing.T, d time.Duration) {
	t.Helper()
	swapSeam(t, tmux.SaverReadinessPollIntervalSeam(), d)
}

func installReadinessBudget(t *testing.T, stall, ceiling time.Duration) {
	t.Helper()
	swapSeam(t, tmux.SaverReadinessStallSeam(), stall)
	swapSeam(t, tmux.SaverReadinessCeilingSeam(), ceiling)
}

func TestWaitForSaverDaemonReady_ReturnsNilImmediatelyWhenPIDPresentAndIdentifies(t *testing.T) {
	installReadinessPollInterval(t, 1*time.Millisecond)
	installReadinessBudget(t, 500*time.Millisecond, 5*time.Second)

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
	log := &barrierLog{}
	installBarrierLogger(t, log)

	start := time.Now()
	if err := tmux.WaitForSaverDaemonReady(t.TempDir()); err != nil {
		t.Fatalf("WaitForSaverDaemonReady returned error: %v", err)
	}
	elapsed := time.Since(start)

	if elapsed >= 100*time.Millisecond {
		t.Errorf("readiness barrier took %v on an immediately-ready daemon; want it to return on the first poll", elapsed)
	}
	if readPIDCalls != 1 {
		t.Errorf("expected exactly 1 ReadPIDFile call on immediate success, got %d", readPIDCalls)
	}
	if identifyCalls != 1 {
		t.Errorf("expected exactly 1 IdentifyDaemon call on immediate success, got %d", identifyCalls)
	}
	if len(log.warns()) != 0 {
		t.Errorf("expected 0 WARN lines on immediate success, got %d: %v", len(log.warns()), log.warns())
	}
}

func TestWaitForSaverDaemonReady_RetriesWhilePIDFileAbsentThenSucceeds(t *testing.T) {
	installReadinessPollInterval(t, 1*time.Millisecond)
	installReadinessBudget(t, 500*time.Millisecond, 5*time.Second)

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
	log := &barrierLog{}
	installBarrierLogger(t, log)

	if err := tmux.WaitForSaverDaemonReady(t.TempDir()); err != nil {
		t.Fatalf("WaitForSaverDaemonReady returned error: %v", err)
	}

	if readCall < 3 {
		t.Errorf("expected at least 3 ReadPIDFile calls (2 absent + 1 success), got %d", readCall)
	}
	if len(log.warns()) != 0 {
		t.Errorf("expected 0 WARN lines on eventual success, got %d: %v", len(log.warns()), log.warns())
	}
}

func TestWaitForSaverDaemonReady_RetriesOnTransientIdentifyDaemonPSFailure(t *testing.T) {
	installReadinessPollInterval(t, 1*time.Millisecond)
	installReadinessBudget(t, 500*time.Millisecond, 5*time.Second)

	installReadinessReadPID(t, func(string) (int, error) { return 4321, nil })

	identifyCall := 0
	installReadinessIdentify(t, func(int) (state.IdentifyResult, error) {
		identifyCall++
		if identifyCall < 3 {
			return 0, errors.New("ps: transient exec failure")
		}
		return state.IdentifyIsPortalDaemon, nil
	})
	log := &barrierLog{}
	installBarrierLogger(t, log)

	if err := tmux.WaitForSaverDaemonReady(t.TempDir()); err != nil {
		t.Fatalf("WaitForSaverDaemonReady returned error: %v", err)
	}

	if identifyCall < 3 {
		t.Errorf("expected at least 3 IdentifyDaemon calls (2 transient + 1 success), got %d", identifyCall)
	}
	if len(log.warns()) != 0 {
		t.Errorf("expected 0 WARN lines on eventual success, got %d: %v", len(log.warns()), log.warns())
	}
}

func TestWaitForSaverDaemonReady_RetriesOnIdentifyDeadUntilNextPIDWrite(t *testing.T) {
	installReadinessPollInterval(t, 1*time.Millisecond)
	installReadinessBudget(t, 500*time.Millisecond, 5*time.Second)

	installReadinessReadPID(t, func(string) (int, error) { return 4321, nil })
	identifyCall := 0
	installReadinessIdentify(t, func(int) (state.IdentifyResult, error) {
		identifyCall++
		if identifyCall < 3 {
			return state.IdentifyDead, nil
		}
		return state.IdentifyIsPortalDaemon, nil
	})
	log := &barrierLog{}
	installBarrierLogger(t, log)

	if err := tmux.WaitForSaverDaemonReady(t.TempDir()); err != nil {
		t.Fatalf("WaitForSaverDaemonReady returned error: %v", err)
	}

	if identifyCall < 3 {
		t.Errorf("expected at least 3 IdentifyDaemon calls (2 dead + 1 success), got %d", identifyCall)
	}
	if len(log.warns()) != 0 {
		t.Errorf("expected 0 WARN lines on eventual success, got %d: %v", len(log.warns()), log.warns())
	}
}

func TestWaitForSaverDaemonReady_RetriesOnIdentifyNotPortalDaemon(t *testing.T) {
	installReadinessPollInterval(t, 1*time.Millisecond)
	installReadinessBudget(t, 500*time.Millisecond, 5*time.Second)

	installReadinessReadPID(t, func(string) (int, error) { return 4321, nil })
	identifyCall := 0
	installReadinessIdentify(t, func(int) (state.IdentifyResult, error) {
		identifyCall++
		if identifyCall < 3 {
			return state.IdentifyNotPortalDaemon, nil
		}
		return state.IdentifyIsPortalDaemon, nil
	})
	log := &barrierLog{}
	installBarrierLogger(t, log)

	if err := tmux.WaitForSaverDaemonReady(t.TempDir()); err != nil {
		t.Fatalf("WaitForSaverDaemonReady returned error: %v", err)
	}

	if identifyCall < 3 {
		t.Errorf("expected at least 3 IdentifyDaemon calls (2 not-portal + 1 success), got %d", identifyCall)
	}
	if len(log.warns()) != 0 {
		t.Errorf("expected 0 WARN lines on eventual success, got %d: %v", len(log.warns()), log.warns())
	}
}

func TestWaitForSaverDaemonReady_GivesUpAtCeilingAndReturnsErrSaverDaemonNotReady(t *testing.T) {
	installReadinessPollInterval(t, 1*time.Millisecond)
	installReadinessBudget(t, 40*time.Millisecond, 200*time.Millisecond)

	installReadinessReadPID(t, func(string) (int, error) { return 4321, nil })
	installReadinessIdentify(t, func(int) (state.IdentifyResult, error) {
		return state.IdentifyDead, nil
	})
	log := &barrierLog{}
	installBarrierLogger(t, log)

	start := time.Now()
	err := tmux.WaitForSaverDaemonReady(t.TempDir())
	elapsed := time.Since(start)

	if !errors.Is(err, tmux.ErrSaverDaemonNotReady) {
		t.Fatalf("WaitForSaverDaemonReady returned %v; want ErrSaverDaemonNotReady", err)
	}
	if !strings.Contains(err.Error(), "pid 4321 is dead") {
		t.Errorf("error %q must name the last observation in words: pid 4321 is dead", err)
	}
	if elapsed < 150*time.Millisecond {
		t.Errorf("readiness barrier gave up after %v; want it to run to the 200ms ceiling, not the 40ms stall", elapsed)
	}
	if len(log.warns()) != 1 {
		t.Fatalf("expected exactly 1 WARN line on timeout, got %d: %v", len(log.warns()), log.warns())
	}
	want := "bootstrap" + " | saver respawn: daemon did not come up"
	if !strings.HasPrefix(log.warns()[0], want) {
		t.Errorf("WARN line %q must begin with %q", log.warns()[0], want)
	}
}

func TestWaitForSaverDaemonReady_WallClockBoundedByCeilingSeam(t *testing.T) {
	installReadinessPollInterval(t, 1*time.Millisecond)
	installReadinessBudget(t, 5*time.Millisecond, 20*time.Millisecond)

	installReadinessReadPID(t, func(string) (int, error) { return 4321, nil })
	installReadinessIdentify(t, func(int) (state.IdentifyResult, error) {
		return state.IdentifyDead, nil
	})
	log := &barrierLog{}
	installBarrierLogger(t, log)

	start := time.Now()
	if err := tmux.WaitForSaverDaemonReady(t.TempDir()); !errors.Is(err, tmux.ErrSaverDaemonNotReady) {
		t.Fatalf("WaitForSaverDaemonReady returned %v; want ErrSaverDaemonNotReady", err)
	}
	elapsed := time.Since(start)

	// Generous slack on purpose: the contract is that the ceiling caps the
	// loop, not that it ends exactly there.
	if elapsed > 1*time.Second {
		t.Errorf("readiness barrier exceeded wall-time budget: elapsed=%v (ceiling=20ms)", elapsed)
	}
	if len(log.warns()) != 1 {
		t.Errorf("expected exactly 1 WARN on timeout, got %d: %v", len(log.warns()), log.warns())
	}
}

func TestWaitForSaverDaemonReady_TreatsTransientReadPIDErrorAsNotReady(t *testing.T) {
	installReadinessPollInterval(t, 1*time.Millisecond)
	installReadinessBudget(t, 500*time.Millisecond, 5*time.Second)

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
	log := &barrierLog{}
	installBarrierLogger(t, log)

	if err := tmux.WaitForSaverDaemonReady(t.TempDir()); err != nil {
		t.Fatalf("WaitForSaverDaemonReady returned error: %v", err)
	}

	if readCall < 3 {
		t.Errorf("expected at least 3 ReadPIDFile calls (2 transient + 1 success), got %d", readCall)
	}
	if len(log.warns()) != 0 {
		t.Errorf("expected 0 WARN lines on eventual success, got %d: %v", len(log.warns()), log.warns())
	}
}

func TestBootstrapPortalSaver_InvokesReadinessBarrierAfterRespawnOnCreatePath(t *testing.T) {
	stubAliveCheck(t, false)
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
	mock := commandertest.FromFunc(script.run(t))
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
	stubAliveCheck(t, true)
	shrinkRetryDelay(t)

	readinessCalls := 0
	swapSeam(t, tmux.WaitForSaverDaemonReadyFnSeam(), func(string) error {
		readinessCalls++
		return nil
	})

	script := &portalSaverScript{
		hasSession: func(int) (string, error) { return "", nil },
		setOption:  func(int) (string, error) { return "", nil },
	}
	mock := commandertest.FromFunc(script.run(t))
	client := tmux.NewClient(mock)

	if err := tmux.BootstrapPortalSaver(client, "/tmp/portal-state"); err != nil {
		t.Fatalf("BootstrapPortalSaver returned error: %v", err)
	}

	if readinessCalls != 0 {
		t.Errorf("expected 0 readiness-barrier invocations on session-present-and-alive path, got %d", readinessCalls)
	}
}

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
	mock := commandertest.FromFunc(script.run(t))
	client := tmux.NewClient(mock)

	if err := tmux.BootstrapPortalSaver(client, wantDir); err != nil {
		t.Fatalf("BootstrapPortalSaver returned error: %v", err)
	}

	if observed != wantDir {
		t.Errorf("readiness barrier received stateDir=%q; want %q", observed, wantDir)
	}
}

func TestWaitForSaverDaemonReady_KeepsWaitingWhileTheObservationChangesPastTheStall(t *testing.T) {
	const stall = 20 * time.Millisecond
	const ceiling = 400 * time.Millisecond
	installReadinessPollInterval(t, 1*time.Millisecond)
	installReadinessBudget(t, stall, ceiling)

	// A new pid on every poll: the daemon is visibly moving, so no stall budget
	// ever elapses and only the ceiling can end the wait.
	pid := 1000
	installReadinessReadPID(t, func(string) (int, error) {
		pid++
		return pid, nil
	})
	installReadinessIdentify(t, func(int) (state.IdentifyResult, error) {
		return state.IdentifyDead, nil
	})
	log := &barrierLog{}
	installBarrierLogger(t, log)

	start := time.Now()
	err := tmux.WaitForSaverDaemonReady(t.TempDir())
	elapsed := time.Since(start)

	if !errors.Is(err, tmux.ErrSaverDaemonNotReady) {
		t.Fatalf("WaitForSaverDaemonReady returned %v; want ErrSaverDaemonNotReady", err)
	}
	if elapsed < 3*stall {
		t.Errorf("readiness barrier gave up after %v; want it to outlive several stall budgets (%v) "+
			"because the observation kept changing", elapsed, stall)
	}
	if elapsed > 2*ceiling {
		t.Errorf("readiness barrier ran for %v, more than twice its ceiling %v", elapsed, ceiling)
	}
	if len(log.warns()) != 1 {
		t.Errorf("expected exactly 1 WARN on give-up, got %d: %v", len(log.warns()), log.warns())
	}
}

func TestWaitForSaverDaemonReady_WaitsOutADaemonWhosePIDFileAppearsPastTheStall(t *testing.T) {
	// Scaled so each leg clears the next by ~50ms: the pid file appears at
	// 150ms, re-arming the stall deadline at ~350ms, and the daemon identifies
	// at 300ms — a deschedule between the two observations cannot decide it.
	const stall = 200 * time.Millisecond
	installReadinessPollInterval(t, 1*time.Millisecond)
	installReadinessBudget(t, stall, 5*time.Second)

	// The pid file appears part-way through, which restarts the stall budget;
	// the daemon then identifies past the point a single fixed budget would
	// have given up, with neither leg on its own outlasting the stall.
	start := time.Now()
	installReadinessReadPID(t, func(string) (int, error) {
		if time.Since(start) < stall*3/4 {
			return 0, state.ErrPIDFileAbsent
		}
		return 4321, nil
	})
	installReadinessIdentify(t, func(int) (state.IdentifyResult, error) {
		if time.Since(start) < stall*3/2 {
			return state.IdentifyDead, nil
		}
		return state.IdentifyIsPortalDaemon, nil
	})
	log := &barrierLog{}
	installBarrierLogger(t, log)

	if err := tmux.WaitForSaverDaemonReady(t.TempDir()); err != nil {
		t.Fatalf("WaitForSaverDaemonReady returned %v; want nil", err)
	}
	if elapsed := time.Since(start); elapsed < stall {
		t.Errorf("readiness barrier returned after %v; the daemon only came up past the stall %v", elapsed, stall)
	}
	if len(log.warns()) != 0 {
		t.Errorf("expected 0 WARN lines on eventual success, got %d: %v", len(log.warns()), log.warns())
	}
}

func TestWaitForSaverDaemonReady_GivesUpAtTheStallOnceAChangedObservationStopsChanging(t *testing.T) {
	const stall = 30 * time.Millisecond
	const ceiling = 5 * time.Second
	installReadinessPollInterval(t, 1*time.Millisecond)
	installReadinessBudget(t, stall, ceiling)

	// One change — the pid file appearing — then the daemon is dead and static.
	readCall := 0
	installReadinessReadPID(t, func(string) (int, error) {
		readCall++
		if readCall < 3 {
			return 0, state.ErrPIDFileAbsent
		}
		return 4321, nil
	})
	installReadinessIdentify(t, func(int) (state.IdentifyResult, error) {
		return state.IdentifyDead, nil
	})
	log := &barrierLog{}
	installBarrierLogger(t, log)

	start := time.Now()
	err := tmux.WaitForSaverDaemonReady(t.TempDir())
	elapsed := time.Since(start)

	if !errors.Is(err, tmux.ErrSaverDaemonNotReady) {
		t.Fatalf("WaitForSaverDaemonReady returned %v; want ErrSaverDaemonNotReady", err)
	}
	if elapsed < stall {
		t.Errorf("readiness barrier gave up after %v; want at least the stall budget %v", elapsed, stall)
	}
	if elapsed > ceiling/2 {
		t.Errorf("readiness barrier ran for %v; want it to give up at the stall %v, well inside the ceiling %v",
			elapsed, stall, ceiling)
	}
	if len(log.warns()) != 1 {
		t.Errorf("expected exactly 1 WARN on give-up, got %d: %v", len(log.warns()), log.warns())
	}
}

func TestBootstrapPortalSaver_ReturnsReadinessFailureInsteadOfReportingSuccess(t *testing.T) {
	stubAliveCheck(t, false)
	shrinkRetryDelay(t)

	sentinel := fmt.Errorf("%w: last=pid-file-absent", tmux.ErrSaverDaemonNotReady)
	swapSeam(t, tmux.WaitForSaverDaemonReadyFnSeam(), func(string) error { return sentinel })

	script := &portalSaverScript{
		hasSession: func(int) (string, error) {
			return "", errors.New("can't find session: _portal-saver")
		},
		newSession:  func(int) (string, error) { return "", nil },
		setOption:   func(int) (string, error) { return "", nil },
		respawnPane: func(int) (string, error) { return "", nil },
	}
	client := tmux.NewClient(commandertest.FromFunc(script.run(t)))

	err := tmux.BootstrapPortalSaver(client, "/tmp/portal-state")

	if !errors.Is(err, tmux.ErrSaverDaemonNotReady) {
		t.Fatalf("BootstrapPortalSaver returned %v; want the readiness failure %v", err, sentinel)
	}
}

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

func TestBootstrapPortalSaver_RecyclesPlaceholderOnlySaverViaNewOrdering(t *testing.T) {
	stubAliveCheck(t, false)
	shrinkRetryDelay(t)
	stubReadinessReady(t)

	script := &portalSaverScript{
		hasSession:  func(int) (string, error) { return "", nil },
		killSession: func(int) (string, error) { return "", nil },
		newSession:  func(int) (string, error) { return "", nil },
		setOption:   func(int) (string, error) { return "", nil },
		respawnPane: func(int) (string, error) { return "", nil },
	}
	mock := commandertest.FromFunc(script.run(t))
	client := tmux.NewClient(mock)

	if err := tmux.BootstrapPortalSaver(client, t.TempDir()); err != nil {
		t.Fatalf("BootstrapPortalSaver returned error: %v", err)
	}

	if got := countCalls(mock.Calls(), "kill-session"); got != 1 {
		t.Errorf("expected exactly 1 kill-session call, got %d (calls: %v)", got, mock.Calls())
	}
	if got := countCalls(mock.Calls(), "new-session"); got != 1 {
		t.Errorf("expected exactly 1 new-session call, got %d", got)
	}
	if got := countCalls(mock.Calls(), "set-option"); got != 1 {
		t.Errorf("expected exactly 1 set-option call, got %d", got)
	}
	if got := countCalls(mock.Calls(), "respawn-pane"); got != 1 {
		t.Errorf("expected exactly 1 respawn-pane call, got %d", got)
	}

	assertKillNewSetRespawnOrdering(t, mock.Calls())

	wantNew := "new-session -d -s _portal-saver " + tmux.PortalSaverPlaceholderCommand
	for _, c := range mock.Calls() {
		if c[0] != "new-session" {
			continue
		}
		if joined := strings.Join(c, " "); joined != wantNew {
			t.Errorf("new-session argv = %q, want %q", joined, wantNew)
		}
	}

	wantRespawn := "respawn-pane -k -t " + string(tmux.CoordTargetExact(tmux.PortalSaverName)) + " " + tmux.PortalSaverDaemonCommand
	for _, c := range mock.Calls() {
		if c[0] != "respawn-pane" {
			continue
		}
		if joined := strings.Join(c, " "); joined != wantRespawn {
			t.Errorf("respawn-pane argv = %q, want %q", joined, wantRespawn)
		}
	}
}

func TestEnsurePortalSaverVersion_AliveMismatch_FlowsThroughNewBootstrapOrdering(t *testing.T) {
	stubAliveCheck(t, true)
	shrinkRetryDelay(t)
	stubReadinessReady(t)

	dir := t.TempDir()
	writeVersion(t, dir, "v0.4.1")

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

	assertKillNewSetRespawnOrdering(t, mock.Calls())

	wantNew := "new-session -d -s _portal-saver " + tmux.PortalSaverPlaceholderCommand
	for _, c := range mock.Calls() {
		if c[0] != "new-session" {
			continue
		}
		if joined := strings.Join(c, " "); joined != wantNew {
			t.Errorf("new-session argv = %q, want %q", joined, wantNew)
		}
	}
}

func TestEnsurePortalSaverVersion_NotAlive_SkipsKillAndStillUsesNewOrdering(t *testing.T) {
	stubAliveCheck(t, false)
	shrinkRetryDelay(t)
	stubReadinessReady(t)

	dir := t.TempDir()

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

	newIdx, setIdx, respawnIdx := -1, -1, -1
	for i, c := range mock.Calls() {
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
		t.Fatalf("missing call: new=%d set=%d respawn=%d (calls=%v)", newIdx, setIdx, respawnIdx, mock.Calls())
	}
	if newIdx >= setIdx || setIdx >= respawnIdx {
		t.Errorf("expected ordering new < set < respawn on no-kill path; got new=%d set=%d respawn=%d (calls=%v)",
			newIdx, setIdx, respawnIdx, mock.Calls())
	}

	wantNew := "new-session -d -s _portal-saver " + tmux.PortalSaverPlaceholderCommand
	for _, c := range mock.Calls() {
		if c[0] != "new-session" {
			continue
		}
		if joined := strings.Join(c, " "); joined != wantNew {
			t.Errorf("new-session argv = %q, want %q", joined, wantNew)
		}
	}
}

func TestBootstrapPortalSaver_NoPersistentPlaceholderLeakAcrossSingleRecovery(t *testing.T) {
	stubAliveCheck(t, false)
	shrinkRetryDelay(t)
	stubReadinessReady(t)

	script := &portalSaverScript{
		hasSession:  func(int) (string, error) { return "", nil },
		killSession: func(int) (string, error) { return "", nil },
		newSession:  func(int) (string, error) { return "", nil },
		setOption:   func(int) (string, error) { return "", nil },
		respawnPane: func(int) (string, error) { return "", nil },
	}
	mock := commandertest.FromFunc(script.run(t))
	client := tmux.NewClient(mock)

	if err := tmux.BootstrapPortalSaver(client, t.TempDir()); err != nil {
		t.Fatalf("BootstrapPortalSaver returned error: %v", err)
	}

	lastPaneMutator := ""
	lastPaneCommand := ""
	for _, c := range mock.Calls() {
		if len(c) == 0 {
			continue
		}
		switch c[0] {
		case "new-session":
			lastPaneMutator = "new-session"
			if len(c) >= 5 {
				lastPaneCommand = strings.Join(c[4:], " ")
			}
		case "respawn-pane":
			lastPaneMutator = "respawn-pane"
			if len(c) >= 5 {
				lastPaneCommand = strings.Join(c[4:], " ")
			}
		}
	}

	if lastPaneMutator != "respawn-pane" {
		t.Errorf("final pane mutator = %q, want %q — placeholder leaked past recovery cycle (calls: %v)",
			lastPaneMutator, "respawn-pane", mock.Calls())
	}
	if lastPaneCommand != tmux.PortalSaverDaemonCommand {
		t.Errorf("final pane command = %q, want %q (daemon) — placeholder leak detected (calls: %v)",
			lastPaneCommand, tmux.PortalSaverDaemonCommand, mock.Calls())
	}
	if lastPaneCommand == tmux.PortalSaverPlaceholderCommand {
		t.Errorf("final pane command is still the placeholder %q — persistent placeholder leak across recovery cycle",
			tmux.PortalSaverPlaceholderCommand)
	}
}

func TestNewDetachedSessionNoCwd_ArgvHasNoEnvOverrides(t *testing.T) {
	var newSessionArgv []string
	mock := commandertest.FromFunc(func(args ...string) (string, error) {
		if len(args) > 0 && args[0] == "new-session" {
			newSessionArgv = append([]string{}, args...)
		}
		return "", nil
	})
	client := tmux.NewClient(mock)

	if err := client.NewDetachedSessionNoCwd("_some-session", "sh -c 'exec tail -f /dev/null'"); err != nil {
		t.Fatalf("NewDetachedSessionNoCwd returned error: %v", err)
	}

	if newSessionArgv == nil {
		t.Fatalf("new-session was not invoked; Calls=%v", mock.Calls())
	}

	for i, arg := range newSessionArgv {
		if strings.HasPrefix(arg, "-e") {
			t.Errorf("new-session argv[%d] = %q starts with \"-e\"; "+
				"NewDetachedSessionNoCwd must not pass session-environment "+
				"overrides. Full argv: %v", i, arg, newSessionArgv)
		}
	}
}

func installBarrierIdentifyDaemon(t *testing.T, fn func(int) (state.IdentifyResult, error)) {
	t.Helper()
	swapSeam(t, tmux.SaverIdentifyDaemonSeam(), fn)
}

func installBarrierSendSIGKILL(t *testing.T, fn func(int) error) {
	t.Helper()
	swapSeam(t, tmux.BarrierSendSIGKILLSeam(), fn)
}

func installBarrierEscalationTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	swapSeam(t, tmux.BarrierEscalationTimeoutSeam(), d)
}

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

	aliveProbes := 0
	installBarrierIsAlive(t, func(pid int) bool {
		aliveProbes++
		return killCalls == 0
	})

	log := &barrierLog{}
	installBarrierLogger(t, log)

	script := &portalSaverScript{
		killSession: func(int) (string, error) { return "", nil },
	}
	mock := commandertest.FromFunc(script.run(t))
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
	if len(log.warns()) != 0 {
		t.Errorf("expected 0 WARN lines on clean escalation, got %d: %v", len(log.warns()), log.warns())
	}
}

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

	log := &barrierLog{}
	installBarrierLogger(t, log)

	script := &portalSaverScript{
		killSession: func(int) (string, error) { return "", nil },
	}
	mock := commandertest.FromFunc(script.run(t))
	client := tmux.NewClient(mock)

	if err := tmux.KillSaverAndWaitForDaemon(client, t.TempDir()); err != nil {
		t.Fatalf("KillSaverAndWaitForDaemon returned error: %v", err)
	}

	if killCalls != 0 {
		t.Errorf("expected 0 SIGKILL seam calls on IdentifyDead, got %d", killCalls)
	}
	if len(log.warns()) != 1 {
		t.Fatalf("expected exactly 1 WARN on IdentifyDead, got %d: %v", len(log.warns()), log.warns())
	}
}

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

	log := &barrierLog{}
	installBarrierLogger(t, log)

	script := &portalSaverScript{
		killSession: func(int) (string, error) { return "", nil },
	}
	mock := commandertest.FromFunc(script.run(t))
	client := tmux.NewClient(mock)

	if err := tmux.KillSaverAndWaitForDaemon(client, t.TempDir()); err != nil {
		t.Fatalf("KillSaverAndWaitForDaemon returned error: %v", err)
	}

	if killCalls != 0 {
		t.Errorf("expected 0 SIGKILL seam calls on IdentifyNotPortalDaemon, got %d", killCalls)
	}
	if len(log.warns()) != 1 {
		t.Fatalf("expected exactly 1 WARN on IdentifyNotPortalDaemon, got %d: %v", len(log.warns()), log.warns())
	}
}

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

	log := &barrierLog{}
	installBarrierLogger(t, log)

	script := &portalSaverScript{
		killSession: func(int) (string, error) { return "", nil },
	}
	mock := commandertest.FromFunc(script.run(t))
	client := tmux.NewClient(mock)

	if err := tmux.KillSaverAndWaitForDaemon(client, t.TempDir()); err != nil {
		t.Fatalf("KillSaverAndWaitForDaemon returned error: %v", err)
	}

	if killCalls != 0 {
		t.Errorf("expected 0 SIGKILL seam calls on transient identity error, got %d", killCalls)
	}
	if len(log.warns()) != 1 {
		t.Fatalf("expected exactly 1 WARN on transient identity error, got %d: %v", len(log.warns()), log.warns())
	}
}

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

	postKillProbes := 0
	installBarrierIsAlive(t, func(int) bool {
		if killCalls == 0 {
			return true
		}
		postKillProbes++
		return postKillProbes < 2
	})

	log := &barrierLog{}
	installBarrierLogger(t, log)

	script := &portalSaverScript{
		killSession: func(int) (string, error) { return "", nil },
	}
	mock := commandertest.FromFunc(script.run(t))
	client := tmux.NewClient(mock)

	if err := tmux.KillSaverAndWaitForDaemon(client, t.TempDir()); err != nil {
		t.Fatalf("KillSaverAndWaitForDaemon returned error: %v", err)
	}

	if killCalls != 1 {
		t.Errorf("expected exactly 1 SIGKILL seam call, got %d", killCalls)
	}
	if len(log.warns()) != 0 {
		t.Errorf("expected 0 WARN lines when process exits within window, got %d: %v", len(log.warns()), log.warns())
	}
}

func TestKillSaverAndWaitForDaemon_Escalation_SIGKILLSucceedsButProcessSurvives_EmitsOneWarnAndReturnsNil(t *testing.T) {
	installBarrierPollInterval(t, 1*time.Millisecond)
	installBarrierTimeout(t, 5*time.Millisecond)
	installBarrierEscalationTimeout(t, 10*time.Millisecond)
	installBarrierReadPID(t, func(string) (int, error) { return 4321, nil })
	installBarrierIsAlive(t, func(int) bool { return true })

	installBarrierIdentifyDaemon(t, func(int) (state.IdentifyResult, error) {
		return state.IdentifyIsPortalDaemon, nil
	})

	killCalls := 0
	installBarrierSendSIGKILL(t, func(int) error {
		killCalls++
		return nil
	})

	log := &barrierLog{}
	installBarrierLogger(t, log)

	script := &portalSaverScript{
		killSession: func(int) (string, error) { return "", nil },
	}
	mock := commandertest.FromFunc(script.run(t))
	client := tmux.NewClient(mock)

	if err := tmux.KillSaverAndWaitForDaemon(client, t.TempDir()); err != nil {
		t.Fatalf("KillSaverAndWaitForDaemon returned error: %v", err)
	}

	if killCalls != 1 {
		t.Errorf("expected exactly 1 SIGKILL seam call, got %d", killCalls)
	}
	if len(log.warns()) != 1 {
		t.Errorf("expected exactly 1 WARN on persistent aliveness post-SIGKILL, got %d: %v", len(log.warns()), log.warns())
	}
}

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

	killSent := false
	installBarrierIsAlive(t, func(int) bool {
		probeLog = append(probeLog, "isalive")
		return !killSent
	})

	prevKill := *tmux.BarrierSendSIGKILLSeam()
	*tmux.BarrierSendSIGKILLSeam() = func(pid int) error {
		err := prevKill(pid)
		killSent = true
		return err
	}

	log := &barrierLog{}
	installBarrierLogger(t, log)

	script := &portalSaverScript{
		killSession: func(int) (string, error) {
			probeLog = append(probeLog, "killsession")
			return "", nil
		},
	}
	mock := commandertest.FromFunc(script.run(t))
	client := tmux.NewClient(mock)

	if err := tmux.KillSaverAndWaitForDaemon(client, t.TempDir()); err != nil {
		t.Fatalf("KillSaverAndWaitForDaemon returned error: %v", err)
	}

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

	if sigkillIdx != identifyIdx+1 {
		t.Errorf("expected sigkill (index=%d) to immediately follow identify (index=%d); intervening events: %v",
			sigkillIdx, identifyIdx, probeLog[identifyIdx+1:sigkillIdx])
	}
}

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

	log := &barrierLog{}
	installBarrierLogger(t, log)

	script := &portalSaverScript{
		killSession: func(int) (string, error) { return "", nil },
	}
	mock := commandertest.FromFunc(script.run(t))
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

func TestKillSaverAndWaitForDaemon_Escalation_PriorPIDDiesDuringSessionKillPoll_EscalationNeverRuns(t *testing.T) {
	installBarrierPollInterval(t, 1*time.Millisecond)
	installBarrierTimeout(t, 500*time.Millisecond)
	installBarrierEscalationTimeout(t, 50*time.Millisecond)
	installBarrierReadPID(t, func(string) (int, error) { return 4321, nil })

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

	log := &barrierLog{}
	installBarrierLogger(t, log)

	script := &portalSaverScript{
		killSession: func(int) (string, error) { return "", nil },
	}
	mock := commandertest.FromFunc(script.run(t))
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
	if len(log.warns()) != 0 {
		t.Errorf("expected 0 WARN lines when PID dies during session-kill poll, got %d: %v", len(log.warns()), log.warns())
	}
}

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

	log := &barrierLog{}
	installBarrierLogger(t, log)

	script := &portalSaverScript{
		killSession: func(int) (string, error) { return "", nil },
	}
	mock := commandertest.FromFunc(script.run(t))
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
	if len(log.warns()) != 0 {
		t.Errorf("expected 0 WARN lines when PID file absent, got %d: %v", len(log.warns()), log.warns())
	}
}

const escalationBreadcrumbMessage = "kill-barrier escalating to SIGKILL"

func escalationDebugRecords(sink *logtest.Sink) logtest.Records {
	return sink.Records().WithMessage(escalationBreadcrumbMessage)
}

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

	installBarrierIsAlive(t, func(int) bool { return killCalls == 0 })

	sink := logtest.Install(t)

	script := &portalSaverScript{
		killSession: func(int) (string, error) { return "", nil },
	}
	mock := commandertest.FromFunc(script.run(t))
	client := tmux.NewClient(mock)

	if err := tmux.KillSaverAndWaitForDaemon(client, t.TempDir()); err != nil {
		t.Fatalf("KillSaverAndWaitForDaemon returned error: %v", err)
	}

	b := escalationDebugRecords(sink).Only(t, escalationBreadcrumbMessage+" breadcrumb")
	if b.Level != slog.LevelDebug {
		t.Errorf("breadcrumb level = %v, want DEBUG", b.Level)
	}

	if gotComponent := b.AttrOrEmpty("component"); gotComponent != "saver" {
		t.Errorf("breadcrumb component = %q, want %q", gotComponent, "saver")
	}
	if got := b.IntAttr(t, "target_pid"); got != 4321 {
		t.Errorf("breadcrumb target_pid = %d, want 4321 (the SIGKILL'd PID)", got)
	}
}

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

			sink := logtest.Install(t)

			script := &portalSaverScript{
				killSession: func(int) (string, error) { return "", nil },
			}
			mock := commandertest.FromFunc(script.run(t))
			client := tmux.NewClient(mock)

			if err := tmux.KillSaverAndWaitForDaemon(client, t.TempDir()); err != nil {
				t.Fatalf("KillSaverAndWaitForDaemon returned error: %v", err)
			}

			if killCalls != 0 {
				t.Errorf("expected 0 SIGKILL seam calls on skip branch, got %d", killCalls)
			}
			if got := escalationDebugRecords(sink); len(got) != 0 {
				t.Errorf("expected 0 escalation breadcrumbs on skip branch, got %d: %v", len(got), got)
			}
		})
	}
}

func TestEscalateKillToSIGKILL_BreadcrumbEmittedBeforeSIGKILL(t *testing.T) {
	installBarrierPollInterval(t, 1*time.Millisecond)
	installBarrierTimeout(t, 5*time.Millisecond)
	installBarrierEscalationTimeout(t, 5*time.Millisecond)
	installBarrierReadPID(t, func(string) (int, error) { return 4321, nil })

	installBarrierIdentifyDaemon(t, func(int) (state.IdentifyResult, error) {
		return state.IdentifyIsPortalDaemon, nil
	})

	sink := logtest.Install(t)

	killCalls := 0
	breadcrumbsAtKillTime := -1
	installBarrierSendSIGKILL(t, func(int) error {
		breadcrumbsAtKillTime = len(escalationDebugRecords(sink))
		killCalls++
		return nil
	})

	installBarrierIsAlive(t, func(int) bool { return killCalls == 0 })

	script := &portalSaverScript{
		killSession: func(int) (string, error) { return "", nil },
	}
	mock := commandertest.FromFunc(script.run(t))
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
