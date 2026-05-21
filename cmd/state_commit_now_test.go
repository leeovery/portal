// Tests in this file mutate package-level state via Cobra and MUST NOT use t.Parallel.
package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/state"
)

// runStateCommitNow executes "portal state commit-now" with stdout/stderr
// captured and returns the Execute error.
func runStateCommitNow(t *testing.T) (*bytes.Buffer, *bytes.Buffer, error) {
	t.Helper()
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	resetRootCmd()
	resetStateCmdFlags()
	rootCmd.SetOut(outBuf)
	rootCmd.SetErr(errBuf)
	rootCmd.SetArgs([]string{"state", "commit-now"})
	err := rootCmd.Execute()
	return outBuf, errBuf, err
}

// fakeCaptureClient is a state.CaptureClient stub returning canned values from
// its three methods. Sessions returned by ListSessionNames are filtered by
// state.CaptureStructure via keepSessionNames.
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

// installCommitNowDeps wires a CommitNowDeps with real ReadIndex/Commit
// (against the temp state dir) but a faked CaptureStructure + client, then
// registers cleanup. Returns pointers tests may inspect.
type commitNowFixture struct {
	client          *fakeCaptureClient
	captureCalls    int
	capturePrevs    []*state.Index
	captureSkipSets []map[string]struct{}
	captureReturn   state.Index
	captureErr      error
	commitCalls     int
	commitArgs      []commitInvocation
	commitErr       error
	readIdxErr      error
	readIdxSkip     bool
	readIdxReturn   state.Index
	readIdxOverride bool
}

type commitInvocation struct {
	Dir                  string
	Idx                  state.Index
	AnyScrollbackChanged bool
}

func installCommitNowDeps(t *testing.T, f *commitNowFixture) {
	t.Helper()
	prev := commitNowDeps
	deps := &CommitNowDeps{
		NewClient: func() state.CaptureClient { return f.client },
		CaptureStructure: func(c state.CaptureClient, skipSet map[string]struct{}, p *state.Index) (state.Index, error) {
			f.captureCalls++
			f.capturePrevs = append(f.capturePrevs, p)
			f.captureSkipSets = append(f.captureSkipSets, skipSet)
			if f.captureErr != nil {
				return state.Index{}, f.captureErr
			}
			return f.captureReturn, nil
		},
		Commit: func(dir string, idx state.Index, any bool, _ *state.Logger) error {
			f.commitCalls++
			f.commitArgs = append(f.commitArgs, commitInvocation{Dir: dir, Idx: idx, AnyScrollbackChanged: any})
			if f.commitErr != nil {
				return f.commitErr
			}
			// Write a real file so on-disk assertions still pass.
			return state.Commit(dir, idx, any, nil)
		},
	}
	if f.readIdxOverride {
		deps.ReadIndex = func(_ string) (state.Index, bool, error) {
			return f.readIdxReturn, f.readIdxSkip, f.readIdxErr
		}
	}
	commitNowDeps = deps
	t.Cleanup(func() { commitNowDeps = prev })
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

// sessionNames extracts the set of session names from an Index in declaration
// order — used in assertions that care about identity but not ordering of
// other fields.
func sessionNames(idx state.Index) []string {
	out := make([]string, 0, len(idx.Sessions))
	for _, s := range idx.Sessions {
		out = append(out, s.Name)
	}
	return out
}

// --- Tests ---

// 1. zero live sessions
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

	if _, _, err := runStateCommitNow(t); err != nil {
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

// 2. single session with windows + panes
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

	if _, _, err := runStateCommitNow(t); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := readSessionsJSON(t, dir)
	names := sessionNames(got)
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

// 3. multi-window, multi-pane
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

	if _, _, err := runStateCommitNow(t); err != nil {
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

// 4. prevIndex is passed through so future hash/content preservation works.
func TestStateCommitNow_PassesPrevIndexFromDiskToCaptureStructure(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	// Seed sessions.json so ReadIndex returns a real prior Index.
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

	if _, _, err := runStateCommitNow(t); err != nil {
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

	// Post-commit file still contains "work" with its pane fields preserved.
	out := readSessionsJSON(t, dir)
	if len(out.Sessions) != 1 || out.Sessions[0].Name != "work" {
		t.Fatalf("post-commit sessions = %v, want [work]", sessionNames(out))
	}
	pane := out.Sessions[0].Windows[0].Panes[0]
	if pane.CurrentCommand != "zsh" || pane.CWD != "/home/u" {
		t.Errorf("pane fields not preserved: %+v", pane)
	}
}

//  5. underscore-prefixed sessions filtered (delegated to keepSessionNames in
//     real CaptureStructure — to assert the integration here we use the real
//     CaptureStructure with a fake client.
func TestStateCommitNow_OmitsUnderscorePrefixedSessions(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	// Real CaptureStructure with a fake client returning both "work" and
	// "_portal-saver"; the live-tmux row data omits the underscore session
	// (tmux would never emit panes for a session filtered by keep, but the
	// list-panes -a output is filtered by parser via the keep set).
	client := &fakeCaptureClient{
		sessions: []string{"work", "_portal-saver"},
		rows: strings.Join([]string{
			"work|||0|||main|||tiled|||0|||1|||0|||/home/u|||1|||zsh",
		}, "\n"),
		env: map[string]string{"work": "", "_portal-saver": ""},
	}

	prev := commitNowDeps
	commitNowDeps = &CommitNowDeps{
		NewClient:        func() state.CaptureClient { return client },
		CaptureStructure: state.CaptureStructure,
		Commit:           state.Commit,
	}
	t.Cleanup(func() { commitNowDeps = prev })

	if _, _, err := runStateCommitNow(t); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := readSessionsJSON(t, dir)
	for _, s := range got.Sessions {
		if strings.HasPrefix(s.Name, "_") {
			t.Errorf("underscore-prefixed session %q must not be present", s.Name)
		}
	}
	if len(got.Sessions) != 1 || got.Sessions[0].Name != "work" {
		t.Errorf("sessions = %v, want [work]", sessionNames(got))
	}
}

// 6. ReadIndex ENOENT → zero-value prev + WARN log, exit 0.
func TestStateCommitNow_FallsBackToZeroPrevAndLogsWarnWhenSessionsJSONMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	t.Setenv("PORTAL_LOG_LEVEL", "warn")

	// No sessions.json exists in dir → ReadIndex returns (Index{}, true, nil).
	f := &commitNowFixture{
		client: &fakeCaptureClient{sessions: nil},
		captureReturn: state.Index{
			Version:  state.SchemaVersion,
			Sessions: []state.Session{},
		},
	}
	installCommitNowDeps(t, f)

	if _, _, err := runStateCommitNow(t); err != nil {
		t.Fatalf("expected exit 0, got: %v", err)
	}

	// Prev passed must be the zero-value Index (no sessions).
	if f.captureCalls != 1 {
		t.Fatalf("CaptureStructure calls = %d, want 1", f.captureCalls)
	}
	if got := f.capturePrevs[0]; got == nil || len(got.Sessions) != 0 || got.Version != 0 {
		t.Errorf("prev should be zero-value Index, got: %+v", got)
	}

	// Log must contain a WARN entry under ComponentDaemon mentioning sessions.json.
	logData, err := os.ReadFile(state.PortalLog(dir))
	if err != nil {
		t.Fatalf("read portal.log: %v", err)
	}
	logged := string(logData)
	if !strings.Contains(logged, "WARN") {
		t.Errorf("log missing WARN level entry: %q", logged)
	}
	if !strings.Contains(logged, "| "+state.ComponentDaemon+" |") {
		t.Errorf("log missing %q component column: %q", state.ComponentDaemon, logged)
	}
	if !strings.Contains(logged, "sessions.json") {
		t.Errorf("log missing 'sessions.json' marker: %q", logged)
	}

	// sessions.json was still written.
	if _, err := os.Stat(filepath.Join(dir, "sessions.json")); err != nil {
		t.Errorf("sessions.json not written: %v", err)
	}
}

// 7. ReadIndex decode error → zero-value prev + WARN, exit 0.
func TestStateCommitNow_FallsBackToZeroPrevAndLogsWarnOnCorruptSessionsJSON(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	t.Setenv("PORTAL_LOG_LEVEL", "warn")

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

	if _, _, err := runStateCommitNow(t); err != nil {
		t.Fatalf("expected exit 0, got: %v", err)
	}

	if got := f.capturePrevs[0]; got == nil || len(got.Sessions) != 0 || got.Version != 0 {
		t.Errorf("prev should be zero-value Index, got: %+v", got)
	}

	logData, err := os.ReadFile(state.PortalLog(dir))
	if err != nil {
		t.Fatalf("read portal.log: %v", err)
	}
	logged := string(logData)
	if !strings.Contains(logged, "WARN") {
		t.Errorf("log missing WARN level entry: %q", logged)
	}
	if !strings.Contains(logged, "| "+state.ComponentDaemon+" |") {
		t.Errorf("log missing %q component column: %q", state.ComponentDaemon, logged)
	}
}

// 8. save.requested untouched on success.
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

	if _, _, err := runStateCommitNow(t); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(state.SaveRequested(dir)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("save.requested must not exist after a successful commit-now; stat err = %v", err)
	}
}

//  9. exits 0 on success — also asserts no .bin files and no scrollback dir
//     growth (per spec: synchronous commit writes only sessions.json).
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

	if _, _, err := runStateCommitNow(t); err != nil {
		t.Fatalf("expected exit 0, got: %v", err)
	}

	// scrollback/ may exist (Commit creates none on its own here; EnsureDir
	// only creates the state dir). Either way, no .bin entries.
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

	// Commit was called with anyScrollbackChanged=false.
	if f.commitCalls != 1 {
		t.Fatalf("commit calls = %d, want 1", f.commitCalls)
	}
	if f.commitArgs[0].AnyScrollbackChanged {
		t.Errorf("anyScrollbackChanged passed to Commit = true, want false")
	}
}

// 10. registered subcommand discoverability.
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
