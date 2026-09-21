package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/state"
)

type fakeCaptureClient struct {
	sessions   []string
	sessionErr error
	rows       string
	rowsErr    error
	env        map[string]string
	envErr     error
}

func (f *fakeCaptureClient) ListSessionNames() ([]string, error) {
	return f.sessions, f.sessionErr
}

func (f *fakeCaptureClient) ListAllPanesWithFormat(_ string) (string, error) {
	return f.rows, f.rowsErr
}

func (f *fakeCaptureClient) ShowEnvironment(name string) (string, error) {
	if f.envErr != nil {
		return "", f.envErr
	}
	return f.env[name], nil
}

type commitNowFixture struct {
	client          *fakeCaptureClient
	captureCalls    int
	capturePrevs    []*state.Index
	captureSkipSets []map[string]struct{}
	captureReturn   state.Index
	capturePending  map[string]struct{}
	captureErr      error
	commitCalls     int
	commitArgs      []commitInvocation
	commitErr       error
	readIdxErr      error
	readIdxSkip     bool
	readIdxReturn   state.Index
	readIdxOverride bool

	restoring      bool
	restoringErr   error
	restoringCalls int
	touchCalls     int
	touchDirs      []string
	touchErr       error
}

type commitInvocation struct {
	Dir                  string
	Idx                  state.Index
	AnyScrollbackChanged bool
}

func installCommitNowDeps(t *testing.T, f *commitNowFixture) {
	t.Helper()
	deps := &CommitNowDeps{
		NewClient: func() state.CaptureClient { return f.client },
		CaptureStructure: func(c state.CaptureClient, skipSet map[string]struct{}, p *state.Index, logger *slog.Logger) (state.Index, map[string]struct{}, error) {
			f.captureCalls++
			f.capturePrevs = append(f.capturePrevs, p)
			f.captureSkipSets = append(f.captureSkipSets, skipSet)
			if f.captureErr != nil {
				return state.Index{}, map[string]struct{}{}, f.captureErr
			}
			return f.captureReturn, f.capturePending, nil
		},
		Commit: func(dir string, idx state.Index, any bool, _ *slog.Logger) error {
			f.commitCalls++
			f.commitArgs = append(f.commitArgs, commitInvocation{Dir: dir, Idx: idx, AnyScrollbackChanged: any})
			if f.commitErr != nil {
				return f.commitErr
			}
			return state.Commit(dir, idx, any, nil)
		},
		IsRestoring: func() (bool, error) {
			f.restoringCalls++
			return f.restoring, f.restoringErr
		},
		TouchSaveRequested: func(dir string) error {
			f.touchCalls++
			f.touchDirs = append(f.touchDirs, dir)
			if f.touchErr != nil {
				return f.touchErr
			}
			return state.TouchSaveRequested(dir)
		},
	}
	if f.readIdxOverride {
		deps.ReadIndex = func(_ string) (state.Index, bool, error) {
			return f.readIdxReturn, f.readIdxSkip, f.readIdxErr
		}
	}
	withCommitNowDeps(t, *deps)
}

func readSessionsJSON(t *testing.T, dir string) state.Index {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "sessions.json"))
	if err != nil {
		t.Fatalf("read sessions.json: %v", err)
	}
	var idx state.Index
	if err := json.Unmarshal(data, &idx); err != nil {
		t.Fatalf("decode sessions.json: %v", err)
	}
	return idx
}

func sessionNamesSlice(idx state.Index) []string {
	out := make([]string, 0, len(idx.Sessions))
	for _, s := range idx.Sessions {
		out = append(out, s.Name)
	}
	return out
}

func TestStateCommitNow_WritesEmptySessionsJSONWhenZeroLiveSessions(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	f := &commitNowFixture{
		client: &fakeCaptureClient{sessions: nil},
		captureReturn: state.Index{
			Version:  state.SchemaVersion,
			Sessions: []state.Session{},
		},
	}
	installCommitNowDeps(t, f)

	if _, _, err := runRootCmd(t, "state", "commit-now"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := readSessionsJSON(t, dir)
	if got.Version != state.SchemaVersion {
		t.Errorf("Version = %d, want %d", got.Version, state.SchemaVersion)
	}
	if len(got.Sessions) != 0 {
		t.Errorf("Sessions = %d entries, want 0", len(got.Sessions))
	}
}

func TestStateCommitNow_WritesSessionWithWindowsAndPanes(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	idx := state.Index{
		Version: state.SchemaVersion,
		Sessions: []state.Session{
			{
				Name:        "work",
				Environment: map[string]string{},
				Windows: []state.Window{
					{
						Index: 0, Name: "main", Layout: "even-horizontal", Active: true,
						Panes: []state.Pane{
							{Index: 0, CWD: "/home/u", Active: true, CurrentCommand: "zsh", ScrollbackFile: "scrollback/work__0.0.bin"},
						},
					},
				},
			},
		},
	}

	f := &commitNowFixture{
		client:        &fakeCaptureClient{sessions: []string{"work"}},
		captureReturn: idx,
	}
	installCommitNowDeps(t, f)

	if _, _, err := runRootCmd(t, "state", "commit-now"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := readSessionsJSON(t, dir)
	names := sessionNamesSlice(got)
	if len(names) != 1 || names[0] != "work" {
		t.Fatalf("sessions = %v, want [work]", names)
	}
	if len(got.Sessions[0].Windows) != 1 {
		t.Fatalf("windows = %d, want 1", len(got.Sessions[0].Windows))
	}
	if len(got.Sessions[0].Windows[0].Panes) != 1 {
		t.Fatalf("panes = %d, want 1", len(got.Sessions[0].Windows[0].Panes))
	}
}

func TestStateCommitNow_WritesMultiWindowMultiPaneSession(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	idx := state.Index{
		Version: state.SchemaVersion,
		Sessions: []state.Session{
			{
				Name:        "proj",
				Environment: map[string]string{},
				Windows: []state.Window{
					{
						Index: 0, Name: "edit", Layout: "tiled",
						Panes: []state.Pane{
							{Index: 0, CWD: "/a", ScrollbackFile: "scrollback/proj__0.0.bin"},
							{Index: 1, CWD: "/b", ScrollbackFile: "scrollback/proj__0.1.bin"},
						},
					},
					{
						Index: 1, Name: "run", Layout: "tiled",
						Panes: []state.Pane{
							{Index: 0, CWD: "/c", ScrollbackFile: "scrollback/proj__1.0.bin"},
							{Index: 1, CWD: "/d", ScrollbackFile: "scrollback/proj__1.1.bin"},
							{Index: 2, CWD: "/e", ScrollbackFile: "scrollback/proj__1.2.bin"},
						},
					},
				},
			},
		},
	}

	f := &commitNowFixture{
		client:        &fakeCaptureClient{sessions: []string{"proj"}},
		captureReturn: idx,
	}
	installCommitNowDeps(t, f)

	if _, _, err := runRootCmd(t, "state", "commit-now"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := readSessionsJSON(t, dir)
	if len(got.Sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(got.Sessions))
	}
	wins := got.Sessions[0].Windows
	if len(wins) != 2 {
		t.Fatalf("windows = %d, want 2", len(wins))
	}
	if len(wins[0].Panes) != 2 {
		t.Errorf("window 0 panes = %d, want 2", len(wins[0].Panes))
	}
	if len(wins[1].Panes) != 3 {
		t.Errorf("window 1 panes = %d, want 3", len(wins[1].Panes))
	}
}

func TestStateCommitNow_PassesPrevIndexFromDiskToCaptureStructure(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	prior := state.Index{
		Version: state.SchemaVersion,
		Sessions: []state.Session{
			{
				Name:        "work",
				Environment: map[string]string{},
				Windows: []state.Window{
					{Index: 0, Name: "main", Panes: []state.Pane{
						{Index: 0, CWD: "/home/u", Active: true, CurrentCommand: "zsh", ScrollbackFile: "scrollback/work__0.0.bin"},
					}},
				},
			},
		},
	}
	data, err := state.EncodeIndex(prior)
	if err != nil {
		t.Fatalf("encode seed: %v", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sessions.json"), data, 0o600); err != nil {
		t.Fatalf("seed sessions.json: %v", err)
	}

	f := &commitNowFixture{
		client:        &fakeCaptureClient{sessions: []string{"work"}},
		captureReturn: prior,
	}
	installCommitNowDeps(t, f)

	if _, _, err := runRootCmd(t, "state", "commit-now"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if f.captureCalls != 1 {
		t.Fatalf("CaptureStructure called %d times, want 1", f.captureCalls)
	}
	if got := f.capturePrevs[0]; got == nil {
		t.Fatal("prev passed to CaptureStructure was nil; want pointer to decoded prior Index")
	} else if len(got.Sessions) != 1 || got.Sessions[0].Name != "work" {
		t.Errorf("prev.Sessions = %v, want [{Name: work, ...}]", got.Sessions)
	}

	out := readSessionsJSON(t, dir)
	if len(out.Sessions) != 1 || out.Sessions[0].Name != "work" {
		t.Fatalf("post-commit sessions = %v, want [work]", sessionNamesSlice(out))
	}
	pane := out.Sessions[0].Windows[0].Panes[0]
	if pane.CurrentCommand != "zsh" || pane.CWD != "/home/u" {
		t.Errorf("pane fields not preserved: %+v", pane)
	}
}

func TestStateCommitNow_OmitsUnderscorePrefixedSessions(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	// The real CaptureStructure runs here, against a fake client that lists
	// both sessions; the pane rows omit the underscore session because the
	// parser filters them by the keep set.
	client := &fakeCaptureClient{
		sessions: []string{"work", "_portal-saver"},
		rows: strings.Join([]string{
			"work|||0|||main|||tiled|||0|||1|||0|||/home/u|||1|||zsh||||||",
		}, "\n"),
		env: map[string]string{"work": "", "_portal-saver": ""},
	}

	withCommitNowDeps(t, CommitNowDeps{
		NewClient:        func() state.CaptureClient { return client },
		CaptureStructure: state.CaptureStructure,
		Commit:           state.Commit,
		// Must be injected: a nil IsRestoring falls through to a live query
		// against whatever server the ambient TMUX names.
		IsRestoring: func() (bool, error) { return false, nil },
	})

	if _, _, err := runRootCmd(t, "state", "commit-now"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := readSessionsJSON(t, dir)
	for _, s := range got.Sessions {
		if strings.HasPrefix(s.Name, "_") {
			t.Errorf("underscore-prefixed session %q must not be present", s.Name)
		}
	}
	if len(got.Sessions) != 1 || got.Sessions[0].Name != "work" {
		t.Errorf("sessions = %v, want [work]", sessionNamesSlice(got))
	}
}

func TestStateCommitNow_FallsBackToZeroPrevAndLogsWarnWhenSessionsJSONMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	t.Setenv("PORTAL_LOG_LEVEL", "warn")
	sink := logtest.Install(t)

	f := &commitNowFixture{
		client: &fakeCaptureClient{sessions: nil},
		captureReturn: state.Index{
			Version:  state.SchemaVersion,
			Sessions: []state.Session{},
		},
	}
	installCommitNowDeps(t, f)

	if _, _, err := runRootCmd(t, "state", "commit-now"); err != nil {
		t.Fatalf("expected exit 0, got: %v", err)
	}

	if f.captureCalls != 1 {
		t.Fatalf("CaptureStructure calls = %d, want 1", f.captureCalls)
	}
	if got := f.capturePrevs[0]; got == nil || len(got.Sessions) != 0 || got.Version != 0 {
		t.Errorf("prev should be zero-value Index, got: %+v", got)
	}

	logged := sink.Body()
	if !strings.Contains(logged, "WARN") {
		t.Errorf("log missing WARN level entry: %q", logged)
	}
	if !strings.Contains(logged, "component="+"daemon") {
		t.Errorf("log missing %q component column: %q", "daemon", logged)
	}
	if !strings.Contains(logged, "sessions.json") {
		t.Errorf("log missing 'sessions.json' marker: %q", logged)
	}

	if _, err := os.Stat(filepath.Join(dir, "sessions.json")); err != nil {
		t.Errorf("sessions.json not written: %v", err)
	}
}

func TestStateCommitNow_FallsBackToZeroPrevAndLogsWarnOnCorruptSessionsJSON(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	t.Setenv("PORTAL_LOG_LEVEL", "warn")
	sink := logtest.Install(t)

	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sessions.json"), []byte("{not valid json"), 0o600); err != nil {
		t.Fatalf("seed corrupt sessions.json: %v", err)
	}

	f := &commitNowFixture{
		client: &fakeCaptureClient{sessions: nil},
		captureReturn: state.Index{
			Version:  state.SchemaVersion,
			Sessions: []state.Session{},
		},
	}
	installCommitNowDeps(t, f)

	if _, _, err := runRootCmd(t, "state", "commit-now"); err != nil {
		t.Fatalf("expected exit 0, got: %v", err)
	}

	if got := f.capturePrevs[0]; got == nil || len(got.Sessions) != 0 || got.Version != 0 {
		t.Errorf("prev should be zero-value Index, got: %+v", got)
	}

	logged := sink.Body()
	if !strings.Contains(logged, "WARN") {
		t.Errorf("log missing WARN level entry: %q", logged)
	}
	if !strings.Contains(logged, "component="+"daemon") {
		t.Errorf("log missing %q component column: %q", "daemon", logged)
	}
}

func TestStateCommitNow_DoesNotTouchSaveRequestedOnSuccess(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	f := &commitNowFixture{
		client: &fakeCaptureClient{sessions: nil},
		captureReturn: state.Index{
			Version:  state.SchemaVersion,
			Sessions: []state.Session{},
		},
	}
	installCommitNowDeps(t, f)

	if _, _, err := runRootCmd(t, "state", "commit-now"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(state.SaveRequested(dir)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("save.requested must not exist after a successful commit-now; stat err = %v", err)
	}
}

func TestStateCommitNow_ExitsZeroAndWritesNoBinFiles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	f := &commitNowFixture{
		client: &fakeCaptureClient{sessions: nil},
		captureReturn: state.Index{
			Version:  state.SchemaVersion,
			Sessions: []state.Session{},
		},
	}
	installCommitNowDeps(t, f)

	if _, _, err := runRootCmd(t, "state", "commit-now"); err != nil {
		t.Fatalf("expected exit 0, got: %v", err)
	}

	entries, err := os.ReadDir(filepath.Join(dir, "scrollback"))
	if err == nil {
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".bin") {
				t.Errorf("no .bin files should be written; found %s", e.Name())
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("unexpected scrollback dir stat error: %v", err)
	}

	if f.commitCalls != 1 {
		t.Fatalf("commit calls = %d, want 1", f.commitCalls)
	}
	if f.commitArgs[0].AnyScrollbackChanged {
		t.Errorf("anyScrollbackChanged passed to Commit = true, want false")
	}
}

func TestStateCommitNow_ShortCircuits_DoesNotWriteSessionsJSONWhenRestoring(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	seed := []byte(`{"sentinel":"untouched"}`)
	sessionsPath := filepath.Join(dir, "sessions.json")
	if err := os.WriteFile(sessionsPath, seed, 0o600); err != nil {
		t.Fatalf("seed sessions.json: %v", err)
	}

	f := &commitNowFixture{
		client:    &fakeCaptureClient{sessions: nil},
		restoring: true,
	}
	installCommitNowDeps(t, f)

	if _, _, err := runRootCmd(t, "state", "commit-now"); err != nil {
		t.Fatalf("expected exit 0, got: %v", err)
	}

	if f.captureCalls != 0 {
		t.Errorf("CaptureStructure called %d times; want 0", f.captureCalls)
	}
	if f.commitCalls != 0 {
		t.Errorf("Commit called %d times; want 0", f.commitCalls)
	}

	got, err := os.ReadFile(sessionsPath)
	if err != nil {
		t.Fatalf("read sessions.json: %v", err)
	}
	if !bytes.Equal(got, seed) {
		t.Errorf("sessions.json mutated during short-circuit:\nwant %q\ngot  %q", seed, got)
	}
}

// The touch is what lets the daemon's first post-restoration tick commit
// without waiting out the gap rule.
func TestStateCommitNow_ShortCircuits_TouchesSaveRequested(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	f := &commitNowFixture{
		client:    &fakeCaptureClient{sessions: nil},
		restoring: true,
	}
	installCommitNowDeps(t, f)

	if _, _, err := runRootCmd(t, "state", "commit-now"); err != nil {
		t.Fatalf("expected exit 0, got: %v", err)
	}

	if f.touchCalls != 1 {
		t.Errorf("TouchSaveRequested calls = %d, want 1", f.touchCalls)
	}
	if _, err := os.Stat(state.SaveRequested(dir)); err != nil {
		t.Errorf("save.requested must exist after restoring short-circuit; stat err = %v", err)
	}
}

func TestStateCommitNow_ShortCircuits_LogsInfoSkipEvent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	t.Setenv("PORTAL_LOG_LEVEL", "info")
	sink := logtest.Install(t)

	f := &commitNowFixture{
		client:    &fakeCaptureClient{sessions: nil},
		restoring: true,
	}
	installCommitNowDeps(t, f)

	if _, _, err := runRootCmd(t, "state", "commit-now"); err != nil {
		t.Fatalf("expected exit 0, got: %v", err)
	}

	logged := sink.Body()
	if !strings.Contains(logged, "INFO") {
		t.Errorf("log missing INFO level entry: %q", logged)
	}
	if !strings.Contains(logged, "component="+"daemon") {
		t.Errorf("log missing %q component column: %q", "daemon", logged)
	}
	if !strings.Contains(logged, "@portal-restoring") {
		t.Errorf("log missing @portal-restoring marker mention: %q", logged)
	}
}

func TestStateCommitNow_ShortCircuits_ExitsZero(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	f := &commitNowFixture{
		client:    &fakeCaptureClient{sessions: nil},
		restoring: true,
	}
	installCommitNowDeps(t, f)

	if _, _, err := runRootCmd(t, "state", "commit-now"); err != nil {
		t.Fatalf("expected exit 0, got: %v", err)
	}
}

func TestStateCommitNow_ShortCircuits_ExitsZeroWhenSaveRequestedTouchFails(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	t.Setenv("PORTAL_LOG_LEVEL", "warn")
	sink := logtest.Install(t)

	f := &commitNowFixture{
		client:    &fakeCaptureClient{sessions: nil},
		restoring: true,
		touchErr:  errors.New("disk full"),
	}
	installCommitNowDeps(t, f)

	if _, _, err := runRootCmd(t, "state", "commit-now"); err != nil {
		t.Fatalf("expected exit 0 even when touch fails, got: %v", err)
	}

	if f.captureCalls != 0 || f.commitCalls != 0 {
		t.Errorf("structural primitives must not run during short-circuit (capture=%d commit=%d)",
			f.captureCalls, f.commitCalls)
	}

	logged := sink.Body()
	if !strings.Contains(logged, "WARN") {
		t.Errorf("log missing WARN level entry on touch failure: %q", logged)
	}
	if !strings.Contains(logged, "save.requested") {
		t.Errorf("log missing save.requested marker: %q", logged)
	}
	if !strings.Contains(logged, "disk full") {
		t.Errorf("log missing underlying touch error: %q", logged)
	}
}

// A query error is treated as marker-presumed-set: protecting an in-flight
// restore outranks a marginally-extended resurrection window.
func TestStateCommitNow_TreatsIsRestoringErrorAsMarkerPresumedSet(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	t.Setenv("PORTAL_LOG_LEVEL", "warn")
	sink := logtest.Install(t)

	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	seed := []byte(`{"sentinel":"untouched"}`)
	sessionsPath := filepath.Join(dir, "sessions.json")
	if err := os.WriteFile(sessionsPath, seed, 0o600); err != nil {
		t.Fatalf("seed sessions.json: %v", err)
	}

	f := &commitNowFixture{
		client:       &fakeCaptureClient{sessions: nil},
		restoring:    false,
		restoringErr: errors.New("tmux unreachable"),
	}
	installCommitNowDeps(t, f)

	if _, _, err := runRootCmd(t, "state", "commit-now"); err != nil {
		t.Fatalf("expected exit 0 on isRestoring error, got: %v", err)
	}

	if f.restoringCalls != 1 {
		t.Errorf("IsRestoring calls = %d, want 1", f.restoringCalls)
	}

	if f.captureCalls != 0 {
		t.Errorf("CaptureStructure called %d times; want 0", f.captureCalls)
	}
	if f.commitCalls != 0 {
		t.Errorf("Commit called %d times; want 0", f.commitCalls)
	}

	if f.touchCalls != 1 {
		t.Errorf("TouchSaveRequested calls = %d, want 1", f.touchCalls)
	}
	if _, err := os.Stat(state.SaveRequested(dir)); err != nil {
		t.Errorf("save.requested must exist after isRestoring error; stat err = %v", err)
	}

	got, err := os.ReadFile(sessionsPath)
	if err != nil {
		t.Fatalf("read sessions.json: %v", err)
	}
	if !bytes.Equal(got, seed) {
		t.Errorf("sessions.json mutated despite isRestoring error:\nwant %q\ngot  %q", seed, got)
	}

	logged := sink.Body()
	if !strings.Contains(logged, "WARN") {
		t.Errorf("log missing WARN level entry: %q", logged)
	}
	if !strings.Contains(logged, "component="+"daemon") {
		t.Errorf("log missing %q component column: %q", "daemon", logged)
	}
	if !strings.Contains(logged, "tmux unreachable") {
		t.Errorf("log missing underlying isRestoring error: %q", logged)
	}
}

func TestStateCommitNow_ProceedsNormallyWhenRestoringClear(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	f := &commitNowFixture{
		client: &fakeCaptureClient{sessions: nil},
		captureReturn: state.Index{
			Version:  state.SchemaVersion,
			Sessions: []state.Session{},
		},
		restoring: false,
	}
	installCommitNowDeps(t, f)

	if _, _, err := runRootCmd(t, "state", "commit-now"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if f.captureCalls != 1 {
		t.Errorf("CaptureStructure calls = %d, want 1", f.captureCalls)
	}
	if f.commitCalls != 1 {
		t.Errorf("Commit calls = %d, want 1", f.commitCalls)
	}
	if f.touchCalls != 0 {
		t.Errorf("TouchSaveRequested must not fire on the happy path; calls = %d", f.touchCalls)
	}
	if _, err := os.Stat(state.SaveRequested(dir)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("save.requested must remain absent on the happy path; stat err = %v", err)
	}
}

func TestStateCommitNow_ExitsNonZeroWhenCaptureStructureFails(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	f := &commitNowFixture{
		client:     &fakeCaptureClient{sessions: nil},
		captureErr: errors.New("tmux unreachable"),
	}
	installCommitNowDeps(t, f)

	_, _, err := runRootCmd(t, "state", "commit-now")
	if err == nil {
		t.Fatal("expected non-zero exit (non-nil Execute error) when CaptureStructure fails")
	}
	if f.commitCalls != 0 {
		t.Errorf("Commit must not be called when CaptureStructure fails; calls = %d", f.commitCalls)
	}
}

func TestStateCommitNow_TouchesSaveRequestedWhenCaptureStructureFails(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	f := &commitNowFixture{
		client:     &fakeCaptureClient{sessions: nil},
		captureErr: errors.New("tmux unreachable"),
	}
	installCommitNowDeps(t, f)

	if _, _, err := runRootCmd(t, "state", "commit-now"); err == nil {
		t.Fatal("expected non-zero exit")
	}

	if f.touchCalls != 1 {
		t.Errorf("TouchSaveRequested calls = %d, want 1", f.touchCalls)
	}
	if _, err := os.Stat(state.SaveRequested(dir)); err != nil {
		t.Errorf("save.requested must exist after CaptureStructure failure; stat err = %v", err)
	}
}

func TestStateCommitNow_LogsErrorWhenCaptureStructureFails(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	t.Setenv("PORTAL_LOG_LEVEL", "error")
	sink := logtest.Install(t)

	f := &commitNowFixture{
		client:     &fakeCaptureClient{sessions: nil},
		captureErr: errors.New("tmux unreachable"),
	}
	installCommitNowDeps(t, f)

	if _, _, err := runRootCmd(t, "state", "commit-now"); err == nil {
		t.Fatal("expected non-zero exit")
	}

	logged := sink.Body()
	if !strings.Contains(logged, "ERROR") {
		t.Errorf("log missing ERROR level entry: %q", logged)
	}
	if !strings.Contains(logged, "component="+"daemon") {
		t.Errorf("log missing %q component column: %q", "daemon", logged)
	}
	if !strings.Contains(logged, "tmux unreachable") {
		t.Errorf("log missing underlying capture error: %q", logged)
	}
}

func TestStateCommitNow_ExitsNonZeroWhenCommitFails(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	f := &commitNowFixture{
		client: &fakeCaptureClient{sessions: nil},
		captureReturn: state.Index{
			Version:  state.SchemaVersion,
			Sessions: []state.Session{},
		},
		commitErr: errors.New("disk full"),
	}
	installCommitNowDeps(t, f)

	if _, _, err := runRootCmd(t, "state", "commit-now"); err == nil {
		t.Fatal("expected non-zero exit when Commit fails")
	}
}

func TestStateCommitNow_TouchesSaveRequestedWhenCommitFails(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	f := &commitNowFixture{
		client: &fakeCaptureClient{sessions: nil},
		captureReturn: state.Index{
			Version:  state.SchemaVersion,
			Sessions: []state.Session{},
		},
		commitErr: errors.New("disk full"),
	}
	installCommitNowDeps(t, f)

	if _, _, err := runRootCmd(t, "state", "commit-now"); err == nil {
		t.Fatal("expected non-zero exit")
	}

	if f.touchCalls != 1 {
		t.Errorf("TouchSaveRequested calls = %d, want 1", f.touchCalls)
	}
	if _, err := os.Stat(state.SaveRequested(dir)); err != nil {
		t.Errorf("save.requested must exist after Commit failure; stat err = %v", err)
	}
}

// The mocked Commit errors without calling the real one, modelling a failure
// before the atomic rename.
func TestStateCommitNow_LeavesSessionsJSONByteIdenticalWhenCommitFailsBeforeRename(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	seed := []byte(`{"sentinel":"untouched"}`)
	sessionsPath := filepath.Join(dir, "sessions.json")
	if err := os.WriteFile(sessionsPath, seed, 0o600); err != nil {
		t.Fatalf("seed sessions.json: %v", err)
	}

	f := &commitNowFixture{
		client: &fakeCaptureClient{sessions: nil},
		captureReturn: state.Index{
			Version:  state.SchemaVersion,
			Sessions: []state.Session{},
		},
		// The seed is deliberately invalid JSON, so the override is what keeps
		// the decode failure off the path while still pinning the on-disk bytes.
		readIdxOverride: true,
		readIdxSkip:     true,
		commitErr:       errors.New("disk full pre-rename"),
	}
	installCommitNowDeps(t, f)

	if _, _, err := runRootCmd(t, "state", "commit-now"); err == nil {
		t.Fatal("expected non-zero exit")
	}

	got, err := os.ReadFile(sessionsPath)
	if err != nil {
		t.Fatalf("read sessions.json: %v", err)
	}
	if !bytes.Equal(got, seed) {
		t.Errorf("sessions.json mutated despite pre-rename Commit failure:\nwant %q\ngot  %q", seed, got)
	}
}

func TestStateCommitNow_LogsErrorWhenCommitFails(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	t.Setenv("PORTAL_LOG_LEVEL", "error")
	sink := logtest.Install(t)

	f := &commitNowFixture{
		client: &fakeCaptureClient{sessions: nil},
		captureReturn: state.Index{
			Version:  state.SchemaVersion,
			Sessions: []state.Session{},
		},
		commitErr: errors.New("permission denied"),
	}
	installCommitNowDeps(t, f)

	if _, _, err := runRootCmd(t, "state", "commit-now"); err == nil {
		t.Fatal("expected non-zero exit")
	}

	logged := sink.Body()
	if !strings.Contains(logged, "ERROR") {
		t.Errorf("log missing ERROR level entry: %q", logged)
	}
	if !strings.Contains(logged, "component="+"daemon") {
		t.Errorf("log missing %q component column: %q", "daemon", logged)
	}
	if !strings.Contains(logged, "permission denied") {
		t.Errorf("log missing underlying commit error: %q", logged)
	}
}

func TestStateCommitNow_ExitsNonZeroWhenBothCommitAndTouchFail(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	f := &commitNowFixture{
		client: &fakeCaptureClient{sessions: nil},
		captureReturn: state.Index{
			Version:  state.SchemaVersion,
			Sessions: []state.Session{},
		},
		commitErr: errors.New("disk full"),
		touchErr:  errors.New("touch eperm"),
	}
	installCommitNowDeps(t, f)

	if _, _, err := runRootCmd(t, "state", "commit-now"); err == nil {
		t.Fatal("expected non-zero exit when both Commit and touch fail")
	}

	if f.touchCalls != 1 {
		t.Errorf("TouchSaveRequested must still have been invoked once; calls = %d", f.touchCalls)
	}
}

func TestStateCommitNow_LogsWarnForTouchFailureAlongsidePrimaryError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	t.Setenv("PORTAL_LOG_LEVEL", "warn")
	sink := logtest.Install(t)

	f := &commitNowFixture{
		client: &fakeCaptureClient{sessions: nil},
		captureReturn: state.Index{
			Version:  state.SchemaVersion,
			Sessions: []state.Session{},
		},
		commitErr: errors.New("disk full primary"),
		touchErr:  errors.New("touch eperm secondary"),
	}
	installCommitNowDeps(t, f)

	if _, _, err := runRootCmd(t, "state", "commit-now"); err == nil {
		t.Fatal("expected non-zero exit")
	}

	logged := sink.Body()
	if !strings.Contains(logged, "ERROR") {
		t.Errorf("log missing primary ERROR entry: %q", logged)
	}
	if !strings.Contains(logged, "disk full primary") {
		t.Errorf("log missing primary error text: %q", logged)
	}
	if !strings.Contains(logged, "WARN") {
		t.Errorf("log missing WARN entry for touch failure: %q", logged)
	}
	if !strings.Contains(logged, "touch eperm secondary") {
		t.Errorf("log missing touch failure text: %q", logged)
	}
	if !strings.Contains(logged, "save.requested") {
		t.Errorf("log missing save.requested marker: %q", logged)
	}
}

// No recover(): a panic must propagate as a test failure.
func TestStateCommitNow_DoesNotPanicOnAnyFailurePath(t *testing.T) {
	cases := []struct {
		name string
		f    *commitNowFixture
	}{
		{
			name: "CaptureStructure failure",
			f: &commitNowFixture{
				client:     &fakeCaptureClient{sessions: nil},
				captureErr: errors.New("tmux gone"),
			},
		},
		{
			name: "Commit failure",
			f: &commitNowFixture{
				client: &fakeCaptureClient{sessions: nil},
				captureReturn: state.Index{
					Version:  state.SchemaVersion,
					Sessions: []state.Session{},
				},
				commitErr: errors.New("disk full"),
			},
		},
		{
			name: "Commit + touch failure",
			f: &commitNowFixture{
				client: &fakeCaptureClient{sessions: nil},
				captureReturn: state.Index{
					Version:  state.SchemaVersion,
					Sessions: []state.Session{},
				},
				commitErr: errors.New("disk full"),
				touchErr:  errors.New("touch eperm"),
			},
		},
		{
			name: "CaptureStructure + touch failure",
			f: &commitNowFixture{
				client:     &fakeCaptureClient{sessions: nil},
				captureErr: errors.New("tmux gone"),
				touchErr:   errors.New("touch eperm"),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("PORTAL_STATE_DIR", dir)
			installCommitNowDeps(t, tc.f)

			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("commit-now panicked on %s: %v", tc.name, r)
				}
			}()

			_, _, _ = runRootCmd(t, "state", "commit-now")
		})
	}
}

// The failure must be detectable by errors.Is so the top-level handler can
// suppress stderr: the hook subprocess has nowhere meaningful to send it and
// diagnostics route through portal.log.
func TestStateCommitNow_FailureExitErrorIsDetectableSentinel(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	f := &commitNowFixture{
		client:     &fakeCaptureClient{sessions: nil},
		captureErr: errors.New("tmux unreachable"),
	}
	installCommitNowDeps(t, f)

	_, errBuf, err := runRootCmd(t, "state", "commit-now")
	if err == nil {
		t.Fatal("expected non-zero exit")
	}
	if !errors.Is(err, errCommitNowFailed) {
		t.Errorf("failure-exit error must be detectable via errors.Is(err, errCommitNowFailed); got %v", err)
	}
	if errBuf.Len() != 0 {
		t.Errorf("nothing should reach stderr on failure exit (cobra SilenceErrors honors); got %q", errBuf.String())
	}
}

func TestStateCommitNow_FailureExitPreservesCauseViaUnwrap(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	cause := errors.New("tmux unreachable cause")
	f := &commitNowFixture{
		client:     &fakeCaptureClient{sessions: nil},
		captureErr: cause,
	}
	installCommitNowDeps(t, f)

	_, _, err := runRootCmd(t, "state", "commit-now")
	if err == nil {
		t.Fatal("expected non-zero exit")
	}
	if !errors.Is(err, errCommitNowFailed) {
		t.Fatalf("err must satisfy errors.Is(err, errCommitNowFailed); got %v", err)
	}
	unwrapped := errors.Unwrap(err)
	if unwrapped == nil {
		t.Fatalf("errors.Unwrap returned nil; want a wrapped error chain")
	}
	if !strings.Contains(err.Error(), "tmux unreachable cause") {
		t.Errorf("err.Error() = %q; must contain the underlying cause text", err.Error())
	}
}

func TestErrCommitNowFailed_HasDescriptiveMessage(t *testing.T) {
	if errCommitNowFailed.Error() == "" {
		t.Fatal("errCommitNowFailed.Error() must be non-empty; the silent-exit contract is now driven by errors.Is, not string compare")
	}
}

func TestIsSilentExitError_DetectsCommitNowSentinel(t *testing.T) {
	if !IsSilentExitError(errCommitNowFailed) {
		t.Error("IsSilentExitError(errCommitNowFailed) = false; want true")
	}
	wrapped := fmt.Errorf("%w: %v", errCommitNowFailed, errors.New("boom"))
	if !IsSilentExitError(wrapped) {
		t.Errorf("IsSilentExitError(wrapped commit-now err) = false; want true (err=%v)", wrapped)
	}
}

func TestIsSilentExitError_RejectsOrdinaryErrors(t *testing.T) {
	if IsSilentExitError(nil) {
		t.Error("IsSilentExitError(nil) = true; want false")
	}
	if IsSilentExitError(errors.New("unrelated")) {
		t.Error("IsSilentExitError(unrelated err) = true; want false")
	}
}

func TestStateCommitNow_IsRegisteredAsStateSubcommand(t *testing.T) {
	var found bool
	for _, c := range stateCmd.Commands() {
		if c.Name() == "commit-now" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("commit-now must be registered as a subcommand of state")
	}
}

func TestStateCommitNow_DiscardsThePendingSet(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	captured := state.Index{
		Version: state.SchemaVersion,
		Sessions: []state.Session{
			{
				Name:        "work",
				Environment: map[string]string{},
				Windows: []state.Window{
					{Index: 0, Name: "main", Panes: []state.Pane{{Index: 0, CWD: "/tmp"}}},
				},
			},
		},
	}
	f := &commitNowFixture{
		client:         &fakeCaptureClient{sessions: []string{"work"}},
		captureReturn:  captured,
		capturePending: map[string]struct{}{state.SanitizePaneKey("work", 0, 0): {}},
	}
	installCommitNowDeps(t, f)

	if _, _, err := runRootCmd(t, "state", "commit-now"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if f.captureCalls != 1 {
		t.Fatalf("CaptureStructure calls = %d, want 1", f.captureCalls)
	}
	if got := f.captureSkipSets[0]; got != nil {
		t.Errorf("skipSet passed to CaptureStructure = %v, want nil", got)
	}
	if f.commitCalls != 1 {
		t.Fatalf("Commit calls = %d, want 1", f.commitCalls)
	}
	if f.commitArgs[0].AnyScrollbackChanged {
		t.Error("anyScrollbackChanged passed to Commit = true, want false")
	}
	if !reflect.DeepEqual(f.commitArgs[0].Idx, captured) {
		t.Errorf("committed index = %+v, want the captured index unchanged by the pending set: %+v",
			f.commitArgs[0].Idx, captured)
	}
}
