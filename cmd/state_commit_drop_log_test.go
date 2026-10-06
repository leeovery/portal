package cmd

import (
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/state"
)

const droppedSessionMsg = "session dropped"

func savedSessionsIndex(names ...string) state.Index {
	sessions := make([]state.Session, 0, len(names))
	for _, name := range names {
		sessions = append(sessions, state.Session{
			Name: name,
			Windows: []state.Window{{
				Index: 0, Name: "main", Layout: "layout", Active: true,
				Panes: []state.Pane{{Index: 0, CWD: "/tmp", Active: true, CurrentCommand: "zsh"}},
			}},
		})
	}
	return state.Index{Version: state.SchemaVersion, Sessions: sessions}
}

func seedSavedSessions(t *testing.T, dir string, names ...string) {
	t.Helper()
	if err := state.Commit(dir, savedSessionsIndex(names...), false, nil); err != nil {
		t.Fatalf("seed sessions.json: %v", err)
	}
}

// liveDaemonCommander answers the daemon's reads as a server holding exactly
// the named sessions, one pane each.
func liveDaemonCommander(names ...string) *daemonFakeCommander {
	var sessions, panes []string
	for _, name := range names {
		sessions = append(sessions, name+"|1|0|")
		panes = append(panes, name+"|||0|||main|||layout|||0|||1|||0|||/tmp|||1|||zsh||||||")
	}
	return &daemonFakeCommander{
		sessionsOut: strings.Join(sessions, "\n"),
		panesOut:    strings.Join(panes, "\n"),
	}
}

func liveCaptureClient(names ...string) *fakeCaptureClient {
	rows := make([]string, 0, len(names))
	env := make(map[string]string, len(names))
	for _, name := range names {
		rows = append(rows, name+"|||0|||main|||layout|||0|||1|||0|||/tmp|||1|||zsh||||||")
		env[name] = ""
	}
	return &fakeCaptureClient{sessions: names, rows: strings.Join(rows, "\n"), env: env}
}

func installRealCommitNowCycle(t *testing.T, live ...string) {
	t.Helper()
	withOwnTmuxServer(t, fakeOwnServerPID)
	withCommitNowDeps(t, CommitNowDeps{
		NewClient:      func() state.CaptureCycleClient { return liveCaptureClient(live...) },
		RunCommitCycle: state.RunCommitCycle,
		IsRestoring:    func() (bool, error) { return false, nil },
	})
}

func droppedSessionsLogged(t *testing.T, sink *logtest.Sink) []string {
	t.Helper()
	names := []string{}
	for _, rec := range sink.Records().WithMessage(droppedSessionMsg) {
		if !rec.Matches("daemon", droppedSessionMsg) {
			t.Errorf("drop line component = %q, want %q", rec.AttrOrEmpty("component"), "daemon")
		}
		if rec.Level != slog.LevelInfo {
			t.Errorf("drop line level = %v, want INFO", rec.Level)
		}
		names = append(names, rec.AttrString(t, "session"))
	}
	slices.Sort(names)
	return names
}

func TestEveryCommitterLogsEachDroppedSessionOnce(t *testing.T) {
	saved := []string{"alpha", "bravo", "charlie", "delta"}
	live := []string{"alpha", "charlie"}
	want := []string{"bravo", "delta"}

	t.Run("the daemon's tick", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("PORTAL_STATE_DIR", dir)
		seedSavedSessions(t, dir, saved...)
		deps := makeDeps(t, dir, liveDaemonCommander(live...))
		logger, sink := newCaptureLoggerForComponent(t, "daemon")
		deps.Logger = logger

		tick(t.Context(), deps)

		if got := droppedSessionsLogged(t, sink); !slices.Equal(got, want) {
			t.Errorf("dropped sessions logged = %v, want %v", got, want)
		}
	})

	t.Run("the daemon's shutdown flush", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("PORTAL_STATE_DIR", dir)
		seedSavedSessions(t, dir, saved...)
		deps := makeDeps(t, dir, liveDaemonCommander(live...))
		logger, sink := newCaptureLoggerForComponent(t, "daemon")
		deps.Logger = logger

		if err := defaultShutdownFlush(deps); err != nil {
			t.Fatalf("defaultShutdownFlush: %v", err)
		}

		if got := droppedSessionsLogged(t, sink); !slices.Equal(got, want) {
			t.Errorf("dropped sessions logged = %v, want %v", got, want)
		}
	})

	t.Run("commit-now", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("PORTAL_STATE_DIR", dir)
		seedSavedSessions(t, dir, saved...)
		installRealCommitNowCycle(t, live...)
		sink := logtest.Install(t)

		if _, _, err := runRootCmd(t, "state", "commit-now"); err != nil {
			t.Fatalf("commit-now: %v", err)
		}

		if got := droppedSessionsLogged(t, sink); !slices.Equal(got, want) {
			t.Errorf("dropped sessions logged = %v, want %v", got, want)
		}
	})
}

func TestAKilledSessionIsLoggedOnceAcrossCommitNowAndTheNextTick(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	seedSavedSessions(t, dir, "alpha", "killed")
	stale := savedSessionsIndex("alpha", "killed")

	installRealCommitNowCycle(t, "alpha")
	commitNowSink := logtest.Install(t)
	if _, _, err := runRootCmd(t, "state", "commit-now"); err != nil {
		t.Fatalf("commit-now: %v", err)
	}
	if got, want := droppedSessionsLogged(t, commitNowSink), []string{"killed"}; !slices.Equal(got, want) {
		t.Errorf("commit-now dropped sessions logged = %v, want %v", got, want)
	}

	deps := makeDeps(t, dir, liveDaemonCommander("alpha"))
	deps.PrevIndex = &stale
	logger, tickSink := newCaptureLoggerForComponent(t, "daemon")
	deps.Logger = logger

	tick(t.Context(), deps)

	if deps.PrevIndex == &stale {
		t.Fatal("the tick did not commit: its previous index still holds the killed session")
	}
	if got := droppedSessionsLogged(t, tickSink); len(got) != 0 {
		t.Errorf("tick dropped sessions logged = %v, want none", got)
	}
}

func TestACommitKeepingEverySavedSessionLogsNoDrop(t *testing.T) {
	cases := map[string][]string{
		"the same sessions":        {"alpha", "bravo"},
		"new sessions beside them": {"alpha", "bravo", "charlie"},
	}
	for name, live := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("PORTAL_STATE_DIR", dir)
			seedSavedSessions(t, dir, "alpha", "bravo")
			deps := makeDeps(t, dir, liveDaemonCommander(live...))
			logger, sink := newCaptureLoggerForComponent(t, "daemon")
			deps.Logger = logger

			tick(t.Context(), deps)

			if got := len(readSessionsJSON(t, dir).Sessions); got != len(live) {
				t.Fatalf("committed sessions = %d, want %d", got, len(live))
			}
			if got := droppedSessionsLogged(t, sink); len(got) != 0 {
				t.Errorf("dropped sessions logged = %v, want none", got)
			}
		})
	}
}

func TestKillingTheLastSessionLogsItsDrop(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	seedSavedSessions(t, dir, "last")
	installRealCommitNowCycle(t)
	sink := logtest.Install(t)

	if _, _, err := runRootCmd(t, "state", "commit-now"); err != nil {
		t.Fatalf("commit-now: %v", err)
	}

	if got := len(readSessionsJSON(t, dir).Sessions); got != 0 {
		t.Fatalf("committed sessions = %d, want 0", got)
	}
	if got, want := droppedSessionsLogged(t, sink), []string{"last"}; !slices.Equal(got, want) {
		t.Errorf("dropped sessions logged = %v, want %v", got, want)
	}
}
