package cmd

import (
	"log/slog"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

const ownServerSettle = 5 * time.Second

// ownServer is a real tmux server holding the user's "work" session, which the
// saved state names alongside "notes".
type ownServer struct {
	socket *tmuxtest.Socket
	pid    int
}

func startOwnServer(t *testing.T, sessions ...string) ownServer {
	t.Helper()
	tmuxtest.SkipIfNoTmux(t)
	ts := tmuxtest.New(t, "ptl-own-server-")
	for _, name := range sessions {
		ts.Run(t, "new-session", "-d", "-s", name)
		ts.WaitForSession(t, name, ownServerSettle)
	}
	return ownServer{socket: ts, pid: serverPID(t, ts)}
}

func serverPID(t *testing.T, ts *tmuxtest.Socket) int {
	t.Helper()
	pid, err := strconv.Atoi(strings.TrimSpace(ts.Run(t, "display-message", "-p", "#{pid}")))
	if err != nil {
		t.Fatalf("read server pid: %v", err)
	}
	return pid
}

// replaceWithANewServer exits the server and starts another on the same socket,
// holding none of the user's sessions, as one started before its restore ran.
func (s ownServer) replaceWithANewServer(t *testing.T) {
	t.Helper()
	s.socket.KillServer()
	s.socket.Run(t, "new-session", "-d", "-s", tmux.PortalBootstrapName)
	s.socket.WaitForSession(t, tmux.PortalBootstrapName, ownServerSettle)
	if pid := serverPID(t, s.socket); pid == s.pid {
		t.Fatalf("new server pid = %d, the same as the exited one", pid)
	}
}

func (s ownServer) daemonDeps(t *testing.T, saved savedStateFixture) (*daemonDeps, *logtest.Sink) {
	t.Helper()
	logger, sink := logtest.NewCaptureLogger(t)
	return &daemonDeps{
		Dir:       saved.dir,
		Logger:    logger,
		Client:    s.socket.Client(),
		OwnServer: s.pid,
		HashMap:   state.HashMap{},
		PrevIndex: &saved.index,
		MaxGap:    30 * time.Second,
	}, sink
}

func (s ownServer) runCommitNow(t *testing.T) error {
	t.Helper()
	withOwnTmuxServer(t, s.pid)
	client := s.socket.Client()
	withCommitNowDeps(t, CommitNowDeps{
		NewClient:   func() state.CaptureCycleClient { return client },
		IsRestoring: func() (bool, error) { return state.IsRestoringSet(client) },
	})
	_, _, err := runRootCmd(t, "state", "commit-now")
	return err
}

func assertCommittedWorkAlone(t *testing.T, dir string) {
	t.Helper()
	committed, skip, err := state.ReadIndex(dir)
	if err != nil || skip {
		t.Fatalf("ReadIndex = (skip %v, err %v), want the committed index", skip, err)
	}
	if len(committed.Sessions) != 1 || committed.Sessions[0].Name != "work" {
		t.Errorf("committed sessions = %+v, want work alone", committed.Sessions)
	}
}

func TestShutdownFlushCommitsAfterItsSaverSessionIsKilledOnARunningServer_RealTmux(t *testing.T) {
	server := startOwnServer(t, "work", tmux.PortalSaverName)
	saved := seedSavedState(t)
	deps, sink := server.daemonDeps(t, saved)

	server.socket.Run(t, "kill-session", "-t", string(tmux.CoordTargetExact(tmux.PortalSaverName)))

	if err := defaultShutdownFlush(deps); err != nil {
		t.Fatalf("defaultShutdownFlush: %v", err)
	}

	shutdown := sink.Records().WithMessage("shutdown").AtExactLevel(slog.LevelInfo).Only(t, "shutdown line")
	if got := shutdown.AttrString(t, "flush_completed"); got != "true" {
		t.Errorf("shutdown flush_completed = %q, want true", got)
	}
	assertCommittedWorkAlone(t, saved.dir)
}

func TestCommittersCommitOnTheirOwnServer_RealTmux(t *testing.T) {
	t.Run("the daemon's tick", func(t *testing.T) {
		server := startOwnServer(t, "work")
		saved := seedSavedState(t)
		deps, _ := server.daemonDeps(t, saved)

		tick(t.Context(), deps)

		assertCommittedWorkAlone(t, saved.dir)
	})

	t.Run("commit-now run by its server's session-closed hook", func(t *testing.T) {
		server := startOwnServer(t, "work")
		saved := seedSavedState(t)

		if err := server.runCommitNow(t); err != nil {
			t.Fatalf("commit-now: %v", err)
		}

		assertCommittedWorkAlone(t, saved.dir)
	})
}

func TestCommittersWriteNothingFromANewServerOnTheSameSocket_RealTmux(t *testing.T) {
	t.Run("the daemon's tick", func(t *testing.T) {
		server := startOwnServer(t, "work", "notes")
		saved := seedSavedState(t)
		deps, _ := server.daemonDeps(t, saved)
		server.replaceWithANewServer(t)

		tick(t.Context(), deps)

		saved.assertUnchanged(t)
	})

	t.Run("the daemon's shutdown flush", func(t *testing.T) {
		server := startOwnServer(t, "work", "notes")
		saved := seedSavedState(t)
		deps, sink := server.daemonDeps(t, saved)
		server.replaceWithANewServer(t)

		if err := defaultShutdownFlush(deps); err != nil {
			t.Fatalf("defaultShutdownFlush: %v", err)
		}

		shutdown := sink.Records().WithMessage("shutdown").AtExactLevel(slog.LevelInfo).Only(t, "shutdown line")
		if got := shutdown.AttrString(t, "flush_completed"); got != "false" {
			t.Errorf("shutdown flush_completed = %q, want false", got)
		}
		saved.assertUnchanged(t)
	})

	t.Run("commit-now whose hook's server exited before it read", func(t *testing.T) {
		server := startOwnServer(t, "work", "notes")
		saved := seedSavedState(t)
		server.replaceWithANewServer(t)

		if err := server.runCommitNow(t); err == nil {
			t.Error("commit-now returned nil, want it to fail")
		}

		saved.assertUnchanged(t)
	})
}
