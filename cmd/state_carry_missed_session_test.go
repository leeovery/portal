package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

const carryBody = "x-transcript"

func carryRow(session string, window, pane int, token string, pending bool) string {
	flag := ""
	if pending {
		flag = "1"
	}
	return fmt.Sprintf("%s|||%d|||main|||layout|||0|||1|||%d|||/tmp|||%s|||zsh|||%s|||%s",
		session, window, pane, col01(pane == 0), token, flag)
}

// carryPrevIndex is the committed index session foo's waiting pane X (filed
// under its token) was saved in, beside an unmarked sibling and a session other.
func carryPrevIndex() state.Index {
	pane := func(idx int, file, token string) state.Pane {
		return state.Pane{Index: idx, CWD: "/tmp", CurrentCommand: "zsh", ScrollbackFile: file, PortalPaneID: token}
	}
	idx := state.Index{
		Version: state.SchemaVersion,
		Sessions: []state.Session{
			{Name: "foo", Environment: map[string]string{}, Windows: []state.Window{{
				Index: 0, Name: "main", Layout: "layout", Active: true,
				Panes: []state.Pane{pane(0, "scrollback/"+waitingRefiled, waitingToken), pane(1, "scrollback/foo__0.1.bin", "")},
			}}},
			{Name: "other", Environment: map[string]string{}, Windows: []state.Window{{
				Index: 0, Name: "main", Layout: "layout", Active: true,
				Panes: []state.Pane{pane(0, "scrollback/other__0.0.bin", "")},
			}}},
		},
	}
	idx.Canonicalize()
	return idx
}

func seedCarryState(t *testing.T, dir string) state.Index {
	t.Helper()
	if err := os.MkdirAll(state.ScrollbackDir(dir), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	for name, body := range map[string]string{waitingRefiled: carryBody, "foo__0.1.bin": "y-body", "other__0.0.bin": "o-body"} {
		if err := os.WriteFile(state.ScrollbackDir(dir)+"/"+name, []byte(body), 0o600); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	prev := carryPrevIndex()
	if err := state.Commit(dir, prev, false, nil); err != nil {
		t.Fatalf("seed sessions.json: %v", err)
	}
	return prev
}

func readSessionsBytes(t *testing.T, dir string) []byte {
	t.Helper()
	data, err := os.ReadFile(state.SessionsJSON(dir))
	if err != nil {
		t.Fatalf("read sessions.json: %v", err)
	}
	return data
}

// collidedRows is a capture in which foo was renamed to bar and baz to foo.
func collidedRows() string {
	return strings.Join([]string{carryRow("bar", 0, 0, waitingToken, true), carryRow("foo", 0, 0, "", false)}, "\n")
}

func TestDaemonTick_CarriesAFailingWaitingSessionWithoutCapturingIt(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	fc := &daemonFakeCommander{
		sessionsOut: "foo|1|0|\nother|1|0|",
		panesOut: strings.Join([]string{
			carryRow("foo", 0, 0, waitingToken, true),
			carryRow("foo", 0, 1, "", false),
			carryRow("other", 0, 0, "", false),
		}, "\n"),
		envErrBySession: map[string]error{"foo": errors.New("boom")},
		captureByTarget: map[string]string{
			"foo:0.0":   "must-not-be-captured",
			"foo:0.1":   "must-not-be-captured",
			"other:0.0": "o-body",
		},
	}
	deps := makeDeps(t, dir, fc)
	prev := seedCarryState(t, dir)
	deps.PrevIndex = &prev

	if err := captureAndCommit(t.Context(), deps); err != nil {
		t.Fatalf("captureAndCommit: %v", err)
	}

	if got, want := captureTargets(fc), []string{"=other:0.0"}; !slices.Equal(got, want) {
		t.Errorf("capture-pane targets = %v, want %v", got, want)
	}
	if got, want := scrollbackFiles(t, dir), []string{"foo__0.1.bin", "other__0.0.bin", waitingRefiled}; !slices.Equal(got, want) {
		t.Errorf("scrollback files = %v, want %v", got, want)
	}
	if got := scrollbackBody(t, dir, waitingRefiled); got != carryBody {
		t.Errorf("%s = %q, want %q", waitingRefiled, got, carryBody)
	}
	committed := readSessionsJSON(t, dir)
	if !reflect.DeepEqual(committed.Sessions[0], prev.Sessions[0]) {
		t.Errorf("committed foo = %+v, want the previous index's %+v", committed.Sessions[0], prev.Sessions[0])
	}
}

func TestDaemonTick_ACarryOntoALiveSessionsNameFailsTheTick(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	fc := &daemonFakeCommander{
		sessionsOut:     "baz|1|0|\nfoo|1|0|",
		panesOut:        collidedRows(),
		envErrBySession: map[string]error{"baz": &tmux.CommandError{Stderr: "no such session: baz", Err: errors.New("exit status 1")}},
	}
	deps := makeDeps(t, dir, fc)
	logger, sink := newCaptureLoggerForComponent(t, "daemon")
	deps.Logger = logger
	prev := seedCarryState(t, dir)
	deps.PrevIndex = &prev
	before := readSessionsBytes(t, dir)
	filesBefore := scrollbackFiles(t, dir)
	touchSaveRequested(t, dir)

	tick(t.Context(), deps)

	sink.Records().Matching("daemon", "tick failed").AtExactLevel(slog.LevelWarn).Only(t, "tick failed WARN")
	if _, err := os.Stat(state.SaveRequested(dir)); err != nil {
		t.Errorf("save.requested stat err = %v, want it re-touched", err)
	}
	if !bytes.Equal(before, readSessionsBytes(t, dir)) {
		t.Error("sessions.json changed by a failed tick")
	}
	if got := scrollbackFiles(t, dir); !slices.Equal(got, filesBefore) {
		t.Errorf("scrollback files = %v, want %v untouched", got, filesBefore)
	}
	if deps.PrevIndex != &prev {
		t.Error("PrevIndex replaced by a failed tick")
	}
}

func TestStateCommitNow_CarriesARenamedWaitingSession(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	prev := seedCarryState(t, dir)
	withCommitNowDeps(t, CommitNowDeps{
		NewClient: func() state.CaptureCycleClient {
			return &fakeCaptureClient{
				sessions: []string{"foo", "other"},
				rows: strings.Join([]string{
					carryRow("bar", 0, 0, waitingToken, true),
					carryRow("bar", 0, 1, "", false),
					carryRow("other", 0, 0, "", false),
				}, "\n"),
				envErrs: map[string]error{"foo": tmux.ErrNoSuchSession},
			}
		},
		IsRestoring: func() (bool, error) { return false, nil },
	})

	if _, _, err := runRootCmd(t, "state", "commit-now"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := scrollbackBody(t, dir, waitingRefiled); got != carryBody {
		t.Errorf("%s = %q, want %q", waitingRefiled, got, carryBody)
	}
	committed := readSessionsJSON(t, dir)
	if !reflect.DeepEqual(committed.Sessions[0], prev.Sessions[0]) {
		t.Errorf("committed foo = %+v, want the previous index's %+v", committed.Sessions[0], prev.Sessions[0])
	}
}

func TestStateCommitNow_ACarryOntoALiveSessionsNameFailsTheCommit(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	seedCarryState(t, dir)
	before := readSessionsBytes(t, dir)
	filesBefore := scrollbackFiles(t, dir)
	withCommitNowDeps(t, CommitNowDeps{
		NewClient: func() state.CaptureCycleClient {
			return &fakeCaptureClient{
				sessions: []string{"baz", "foo"},
				rows:     collidedRows(),
				envErrs:  map[string]error{"baz": tmux.ErrNoSuchSession},
			}
		},
		IsRestoring: func() (bool, error) { return false, nil },
	})

	_, _, err := runRootCmd(t, "state", "commit-now")
	if !errors.Is(err, errCommitNowFailed) {
		t.Fatalf("error = %v, want errCommitNowFailed", err)
	}

	if _, err := os.Stat(state.SaveRequested(dir)); err != nil {
		t.Errorf("save.requested stat err = %v, want it touched", err)
	}
	if !bytes.Equal(before, readSessionsBytes(t, dir)) {
		t.Error("sessions.json changed by a failed commit-now")
	}
	if got := scrollbackFiles(t, dir); !slices.Equal(got, filesBefore) {
		t.Errorf("scrollback files = %v, want %v untouched", got, filesBefore)
	}
}
