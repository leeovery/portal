package cmd

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/state"
)

const (
	lagSiblingToken      = "tokbbb"
	lagSiblingTranscript = "y-transcript"
)

// seedLaggingIndex commits session foo's panes X and Y, both stamped and each
// named at its positional transcript, beside a session other. The daemon holds
// this index in memory.
func seedLaggingIndex(t *testing.T, dir string) state.Index {
	t.Helper()
	if err := os.MkdirAll(state.ScrollbackDir(dir), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	for name, body := range map[string]string{"foo__0.0.bin": carryBody, "foo__0.1.bin": lagSiblingTranscript, "other__0.0.bin": "o-body"} {
		if err := os.WriteFile(filepath.Join(state.ScrollbackDir(dir), name), []byte(body), 0o600); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	pane := func(idx int, file, token string) state.Pane {
		return state.Pane{Index: idx, CWD: "/tmp", CurrentCommand: "zsh", ScrollbackFile: file, PortalPaneID: token}
	}
	idx := state.Index{
		Version: state.SchemaVersion,
		Sessions: []state.Session{
			{Name: "foo", Environment: map[string]string{}, Windows: []state.Window{{
				Index: 0, Name: "main", Layout: "layout", Active: true,
				Panes: []state.Pane{pane(0, "scrollback/foo__0.0.bin", waitingToken), pane(1, "scrollback/foo__0.1.bin", lagSiblingToken)},
			}}},
			{Name: "other", Environment: map[string]string{}, Windows: []state.Window{{
				Index: 0, Name: "main", Layout: "layout", Active: true,
				Panes: []state.Pane{pane(0, "scrollback/other__0.0.bin", "")},
			}}},
		},
	}
	idx.Canonicalize()
	if err := state.Commit(dir, idx, false, nil); err != nil {
		t.Fatalf("seed sessions.json: %v", err)
	}
	return idx
}

// commitNowOverBothWaiting runs `portal state commit-now` with X and Y both
// waiting, which files each under its token and removes their positional files.
func commitNowOverBothWaiting(t *testing.T, dir string) {
	t.Helper()
	withCommitNowDeps(t, CommitNowDeps{
		NewClient: func() state.CaptureCycleClient {
			return &fakeCaptureClient{
				sessions: []string{"foo", "other"},
				rows: strings.Join([]string{
					carryRow("foo", 0, 0, waitingToken, true),
					carryRow("foo", 0, 1, lagSiblingToken, true),
					carryRow("other", 0, 0, "", false),
				}, "\n"),
			}
		},
		IsRestoring: func() (bool, error) { return false, nil },
	})
	if _, _, err := runRootCmd(t, "state", "commit-now"); err != nil {
		t.Fatalf("commit-now: %v", err)
	}
	if got := scrollbackBody(t, dir, waitingRefiled); got != carryBody {
		t.Fatalf("after the commit-now %s = %q, want %q", waitingRefiled, got, carryBody)
	}
	if _, err := os.Stat(filepath.Join(state.ScrollbackDir(dir), "foo__0.0.bin")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("after the commit-now X's positional file stat err = %v, want it removed", err)
	}
}

func TestDaemonCarriesAnAnsweredPaneFromTheIndexACommitNowLeft(t *testing.T) {
	cycles := map[string]func(t *testing.T, deps *daemonDeps, sink *logtest.Sink){
		"tick": func(t *testing.T, deps *daemonDeps, _ *logtest.Sink) {
			if err := captureAndCommit(context.Background(), deps); err != nil {
				t.Fatalf("captureAndCommit: %v", err)
			}
		},
		"shutdown flush": func(t *testing.T, deps *daemonDeps, sink *logtest.Sink) {
			if err := defaultShutdownFlush(deps); err != nil {
				t.Fatalf("defaultShutdownFlush: %v", err)
			}
			rec := sink.Records().Matching("daemon", "shutdown").AtExactLevel(slog.LevelInfo).Only(t, "daemon shutdown record")
			if got := rec.AttrOrEmpty("flush_completed"); got != "true" {
				t.Fatalf("flush_completed = %q, want \"true\"", got)
			}
		},
	}
	for name, run := range cycles {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("PORTAL_STATE_DIR", dir)
			withOwnTmuxServer(t, fakeOwnServerPID)
			lagging := seedLaggingIndex(t, dir)
			commitNowOverBothWaiting(t, dir)

			// X has been answered; Y still waits; foo's environment read fails.
			fc := &daemonFakeCommander{
				sessionsOut: "foo|1|0|\nother|1|0|",
				panesOut: strings.Join([]string{
					carryRow("foo", 0, 0, waitingToken, false),
					carryRow("foo", 0, 1, lagSiblingToken, true),
					carryRow("other", 0, 0, "", false),
				}, "\n"),
				envErrBySession: map[string]error{"foo": errors.New("boom")},
				captureByTarget: map[string]string{"other:0.0": "o-body-now"},
			}
			deps := makeDeps(t, dir, fc)
			logger, sink := newCaptureLoggerForComponent(t, "daemon")
			deps.Logger = logger
			deps.PrevIndex = &lagging

			run(t, deps, sink)

			if got := recordedScrollback(t, dir, 0, 0); got != "scrollback/"+waitingRefiled {
				t.Errorf("sessions.json names %q for X, want %q", got, "scrollback/"+waitingRefiled)
			}
			if got := scrollbackBody(t, dir, waitingRefiled); got != carryBody {
				t.Errorf("%s = %q, want %q", waitingRefiled, got, carryBody)
			}
			if got := scrollbackBody(t, dir, "other__0.0.bin"); got != "o-body-now" {
				t.Errorf("other__0.0.bin = %q, want the cycle's capture committed", got)
			}
			assertEverySavedScrollbackOnDisk(t, dir)
		})
	}
}

func TestStateCommitNow_WarnsOnlyWhenSessionsJSONCannotBeRead(t *testing.T) {
	cases := []struct {
		name  string
		stage func(t *testing.T, dir string)
		want  string
	}{
		{name: "absent", stage: func(*testing.T, string) {}, want: absentIndexWarn},
		{name: "not decodable", stage: func(t *testing.T, dir string) {
			if err := os.WriteFile(state.SessionsJSON(dir), []byte("{not valid json"), 0o600); err != nil {
				t.Fatalf("seed corrupt sessions.json: %v", err)
			}
		}, want: unreadableIndexWarn},
		{name: "readable", stage: func(t *testing.T, dir string) {
			if err := state.Commit(dir, state.Index{Version: state.SchemaVersion}, false, nil); err != nil {
				t.Fatalf("seed sessions.json: %v", err)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("PORTAL_STATE_DIR", dir)
			withOwnTmuxServer(t, fakeOwnServerPID)
			sink := logtest.Install(t)
			tc.stage(t, dir)
			withCommitNowDeps(t, CommitNowDeps{
				NewClient: func() state.CaptureCycleClient {
					return &fakeCaptureClient{sessions: []string{"work"}, rows: carryRow("work", 0, 0, "", false)}
				},
				IsRestoring: func() (bool, error) { return false, nil },
			})

			if _, _, err := runRootCmd(t, "state", "commit-now"); err != nil {
				t.Fatalf("commit-now: %v", err)
			}

			for _, msg := range []string{absentIndexWarn, unreadableIndexWarn} {
				got := len(sink.Records().Matching("daemon", msg).AtExactLevel(slog.LevelWarn))
				want := 0
				if msg == tc.want {
					want = 1
				}
				if got != want {
					t.Errorf("WARN %q logged %d times, want %d", msg, got, want)
				}
			}
			wantWarns := 0
			if tc.want != "" {
				wantWarns = 1
			}
			if n := len(sink.Records().AtOrAboveLevel(slog.LevelWarn)); n != wantWarns {
				t.Errorf("WARN records = %d, want %d in:\n%s", n, wantWarns, sink.Body())
			}
		})
	}
}
