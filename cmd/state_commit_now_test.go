// Tests in this file mutate package-level state via Cobra and MUST NOT use t.Parallel.
package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
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

	// @portal-restoring / save.requested seams.
	restoring     bool
	restoringErr  error
	restoringCalls int
	touchCalls    int
	touchDirs     []string
	touchErr      error
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

// sessionNamesSlice extracts session names from an Index in declaration order
// — used in assertions that care about identity but not ordering of other
// fields. Returns []string so reflect.DeepEqual / slice comparisons work.
// The cmd_test integration files declare a different sessionNames helper
// returning map[string]struct{} for presence-set assertions; the two helpers
// have intentionally distinct shapes and live in different packages.
func sessionNamesSlice(idx state.Index) []string {
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
		t.Fatalf("post-commit sessions = %v, want [work]", sessionNamesSlice(out))
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
		t.Errorf("sessions = %v, want [work]", sessionNamesSlice(got))
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

// --- @portal-restoring short-circuit (task 1-2) ---

// 11. short-circuit: no sessions.json write while @portal-restoring is set.
func TestStateCommitNow_ShortCircuits_DoesNotWriteSessionsJSONWhenRestoring(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	// Seed sessions.json with a known byte sequence so we can prove
	// byte-equivalence pre/post invocation.
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

	if _, _, err := runStateCommitNow(t); err != nil {
		t.Fatalf("expected exit 0, got: %v", err)
	}

	// None of the structural primitives may have fired.
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

// 12. short-circuit touches save.requested so the daemon's first
// post-restoration tick commits without waiting for the gap rule.
func TestStateCommitNow_ShortCircuits_TouchesSaveRequested(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	f := &commitNowFixture{
		client:    &fakeCaptureClient{sessions: nil},
		restoring: true,
	}
	installCommitNowDeps(t, f)

	if _, _, err := runStateCommitNow(t); err != nil {
		t.Fatalf("expected exit 0, got: %v", err)
	}

	if f.touchCalls != 1 {
		t.Errorf("TouchSaveRequested calls = %d, want 1", f.touchCalls)
	}
	if _, err := os.Stat(state.SaveRequested(dir)); err != nil {
		t.Errorf("save.requested must exist after restoring short-circuit; stat err = %v", err)
	}
}

// 13. short-circuit emits an INFO-level structured log entry.
func TestStateCommitNow_ShortCircuits_LogsInfoSkipEvent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	t.Setenv("PORTAL_LOG_LEVEL", "info")

	f := &commitNowFixture{
		client:    &fakeCaptureClient{sessions: nil},
		restoring: true,
	}
	installCommitNowDeps(t, f)

	if _, _, err := runStateCommitNow(t); err != nil {
		t.Fatalf("expected exit 0, got: %v", err)
	}

	logData, err := os.ReadFile(state.PortalLog(dir))
	if err != nil {
		t.Fatalf("read portal.log: %v", err)
	}
	logged := string(logData)
	if !strings.Contains(logged, "INFO") {
		t.Errorf("log missing INFO level entry: %q", logged)
	}
	if !strings.Contains(logged, "| "+state.ComponentDaemon+" |") {
		t.Errorf("log missing %q component column: %q", state.ComponentDaemon, logged)
	}
	if !strings.Contains(logged, "@portal-restoring") {
		t.Errorf("log missing @portal-restoring marker mention: %q", logged)
	}
}

// 14. exit code is 0 on the short-circuit.
func TestStateCommitNow_ShortCircuits_ExitsZero(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	f := &commitNowFixture{
		client:    &fakeCaptureClient{sessions: nil},
		restoring: true,
	}
	installCommitNowDeps(t, f)

	if _, _, err := runStateCommitNow(t); err != nil {
		t.Fatalf("expected exit 0, got: %v", err)
	}
}

// 15. short-circuit with touch failure still exits 0 and logs the touch
// failure at WARN.
func TestStateCommitNow_ShortCircuits_ExitsZeroWhenSaveRequestedTouchFails(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	t.Setenv("PORTAL_LOG_LEVEL", "warn")

	f := &commitNowFixture{
		client:    &fakeCaptureClient{sessions: nil},
		restoring: true,
		touchErr:  errors.New("disk full"),
	}
	installCommitNowDeps(t, f)

	if _, _, err := runStateCommitNow(t); err != nil {
		t.Fatalf("expected exit 0 even when touch fails, got: %v", err)
	}

	if f.captureCalls != 0 || f.commitCalls != 0 {
		t.Errorf("structural primitives must not run during short-circuit (capture=%d commit=%d)",
			f.captureCalls, f.commitCalls)
	}

	logData, err := os.ReadFile(state.PortalLog(dir))
	if err != nil {
		t.Fatalf("read portal.log: %v", err)
	}
	logged := string(logData)
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

// 15b. IsRestoring query error → treat as marker presumed set: skip
// structural commit, touch save.requested, exit 0, log WARN with the
// underlying error. Risk priority: protect an in-flight restore over a
// marginally-extended resurrection window on transient query failure.
func TestStateCommitNow_TreatsIsRestoringErrorAsMarkerPresumedSet(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	t.Setenv("PORTAL_LOG_LEVEL", "warn")

	// Seed sessions.json so we can verify byte-equivalence post-invocation.
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

	if _, _, err := runStateCommitNow(t); err != nil {
		t.Fatalf("expected exit 0 on isRestoring error, got: %v", err)
	}

	// IsRestoring is queried exactly once per invocation — no retry, no
	// double-read.
	if f.restoringCalls != 1 {
		t.Errorf("IsRestoring calls = %d, want 1", f.restoringCalls)
	}

	// Structural primitives must NOT have fired.
	if f.captureCalls != 0 {
		t.Errorf("CaptureStructure called %d times; want 0", f.captureCalls)
	}
	if f.commitCalls != 0 {
		t.Errorf("Commit called %d times; want 0", f.commitCalls)
	}

	// save.requested must be touched (daemon-fallback handoff).
	if f.touchCalls != 1 {
		t.Errorf("TouchSaveRequested calls = %d, want 1", f.touchCalls)
	}
	if _, err := os.Stat(state.SaveRequested(dir)); err != nil {
		t.Errorf("save.requested must exist after isRestoring error; stat err = %v", err)
	}

	// sessions.json must be byte-identical to the seed.
	got, err := os.ReadFile(sessionsPath)
	if err != nil {
		t.Fatalf("read sessions.json: %v", err)
	}
	if !bytes.Equal(got, seed) {
		t.Errorf("sessions.json mutated despite isRestoring error:\nwant %q\ngot  %q", seed, got)
	}

	// WARN log entry under ComponentDaemon, mentioning the underlying error.
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
	if !strings.Contains(logged, "tmux unreachable") {
		t.Errorf("log missing underlying isRestoring error: %q", logged)
	}
}

// 16. marker clear: happy path proceeds unchanged.
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

	if _, _, err := runStateCommitNow(t); err != nil {
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

// --- Failure-path discipline (task 1-3) ---
//
// On any failure of CaptureStructure or Commit, commit-now must:
//   (a) emit an ERROR log under state.ComponentDaemon with the underlying err,
//   (b) touch save.requested best-effort (daemon-fallback handoff),
//   (c) exit non-zero — never panic, never print a Go stack trace.
//
// If the save.requested touch itself fails on a failure exit, log WARN and
// preserve the non-zero exit (original failure dominates).

// 17. CaptureStructure error → non-zero exit (sentinel empty-message error).
func TestStateCommitNow_ExitsNonZeroWhenCaptureStructureFails(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	f := &commitNowFixture{
		client:     &fakeCaptureClient{sessions: nil},
		captureErr: errors.New("tmux unreachable"),
	}
	installCommitNowDeps(t, f)

	_, _, err := runStateCommitNow(t)
	if err == nil {
		t.Fatal("expected non-zero exit (non-nil Execute error) when CaptureStructure fails")
	}
	if f.commitCalls != 0 {
		t.Errorf("Commit must not be called when CaptureStructure fails; calls = %d", f.commitCalls)
	}
}

// 18. CaptureStructure error → touches save.requested.
func TestStateCommitNow_TouchesSaveRequestedWhenCaptureStructureFails(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	f := &commitNowFixture{
		client:     &fakeCaptureClient{sessions: nil},
		captureErr: errors.New("tmux unreachable"),
	}
	installCommitNowDeps(t, f)

	if _, _, err := runStateCommitNow(t); err == nil {
		t.Fatal("expected non-zero exit")
	}

	if f.touchCalls != 1 {
		t.Errorf("TouchSaveRequested calls = %d, want 1", f.touchCalls)
	}
	if _, err := os.Stat(state.SaveRequested(dir)); err != nil {
		t.Errorf("save.requested must exist after CaptureStructure failure; stat err = %v", err)
	}
}

// 19. CaptureStructure error → ERROR log under ComponentDaemon, mentions err.
func TestStateCommitNow_LogsErrorWhenCaptureStructureFails(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	t.Setenv("PORTAL_LOG_LEVEL", "error")

	f := &commitNowFixture{
		client:     &fakeCaptureClient{sessions: nil},
		captureErr: errors.New("tmux unreachable"),
	}
	installCommitNowDeps(t, f)

	if _, _, err := runStateCommitNow(t); err == nil {
		t.Fatal("expected non-zero exit")
	}

	logData, err := os.ReadFile(state.PortalLog(dir))
	if err != nil {
		t.Fatalf("read portal.log: %v", err)
	}
	logged := string(logData)
	if !strings.Contains(logged, "ERROR") {
		t.Errorf("log missing ERROR level entry: %q", logged)
	}
	if !strings.Contains(logged, "| "+state.ComponentDaemon+" |") {
		t.Errorf("log missing %q component column: %q", state.ComponentDaemon, logged)
	}
	if !strings.Contains(logged, "tmux unreachable") {
		t.Errorf("log missing underlying capture error: %q", logged)
	}
}

// 20. Commit error → non-zero exit.
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

	if _, _, err := runStateCommitNow(t); err == nil {
		t.Fatal("expected non-zero exit when Commit fails")
	}
}

// 21. Commit error → touches save.requested.
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

	if _, _, err := runStateCommitNow(t); err == nil {
		t.Fatal("expected non-zero exit")
	}

	if f.touchCalls != 1 {
		t.Errorf("TouchSaveRequested calls = %d, want 1", f.touchCalls)
	}
	if _, err := os.Stat(state.SaveRequested(dir)); err != nil {
		t.Errorf("save.requested must exist after Commit failure; stat err = %v", err)
	}
}

// 22. Commit error pre-rename leaves sessions.json byte-identical.
//
// Mocked Commit returns an error WITHOUT calling the real state.Commit, which
// models a Commit that fails before the atomic rename. The seeded sessions.json
// must be untouched.
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
		// readIdxOverride avoids the seeded sentinel failing JSON decode; the
		// seed is intentionally invalid JSON so we exercise the ReadIndex
		// fallback path while still pinning the on-disk bytes.
		readIdxOverride: true,
		readIdxSkip:     true,
		commitErr:       errors.New("disk full pre-rename"),
	}
	installCommitNowDeps(t, f)

	if _, _, err := runStateCommitNow(t); err == nil {
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

// 23. Commit error → ERROR log.
func TestStateCommitNow_LogsErrorWhenCommitFails(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	t.Setenv("PORTAL_LOG_LEVEL", "error")

	f := &commitNowFixture{
		client: &fakeCaptureClient{sessions: nil},
		captureReturn: state.Index{
			Version:  state.SchemaVersion,
			Sessions: []state.Session{},
		},
		commitErr: errors.New("permission denied"),
	}
	installCommitNowDeps(t, f)

	if _, _, err := runStateCommitNow(t); err == nil {
		t.Fatal("expected non-zero exit")
	}

	logData, err := os.ReadFile(state.PortalLog(dir))
	if err != nil {
		t.Fatalf("read portal.log: %v", err)
	}
	logged := string(logData)
	if !strings.Contains(logged, "ERROR") {
		t.Errorf("log missing ERROR level entry: %q", logged)
	}
	if !strings.Contains(logged, "| "+state.ComponentDaemon+" |") {
		t.Errorf("log missing %q component column: %q", state.ComponentDaemon, logged)
	}
	if !strings.Contains(logged, "permission denied") {
		t.Errorf("log missing underlying commit error: %q", logged)
	}
}

// 24. Both Commit and save.requested touch fail → still non-zero exit.
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

	if _, _, err := runStateCommitNow(t); err == nil {
		t.Fatal("expected non-zero exit when both Commit and touch fail")
	}

	if f.touchCalls != 1 {
		t.Errorf("TouchSaveRequested must still have been invoked once; calls = %d", f.touchCalls)
	}
}

// 25. Touch failure on a failure exit → WARN log alongside the primary ERROR.
func TestStateCommitNow_LogsWarnForTouchFailureAlongsidePrimaryError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	t.Setenv("PORTAL_LOG_LEVEL", "warn")

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

	if _, _, err := runStateCommitNow(t); err == nil {
		t.Fatal("expected non-zero exit")
	}

	logData, err := os.ReadFile(state.PortalLog(dir))
	if err != nil {
		t.Fatalf("read portal.log: %v", err)
	}
	logged := string(logData)
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

// 26. No panic on any failure path. Exercises capture-fail, commit-fail, and
// both-fail without recover() so a panic propagates as a test failure.
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

			// Non-zero exit is expected; we only care that no panic escaped.
			_, _, _ = runStateCommitNow(t)
		})
	}
}

// 27. Failure exit error must be detectable via errors.Is(err, errCommitNowFailed)
// so main.go's top-level handler can suppress stderr silently — the hook
// subprocess has nowhere meaningful to send stderr; diagnostics route
// exclusively through portal.log. Cobra (with SilenceErrors=true) is
// responsible for not printing the error; main.go is responsible for not
// duplicating it.
func TestStateCommitNow_FailureExitErrorIsDetectableSentinel(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	f := &commitNowFixture{
		client:     &fakeCaptureClient{sessions: nil},
		captureErr: errors.New("tmux unreachable"),
	}
	installCommitNowDeps(t, f)

	_, errBuf, err := runStateCommitNow(t)
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

// 27b. failCommitNow must wrap the underlying cause via fmt.Errorf("%w: %v", ...)
// so errors.Unwrap surfaces the cause. The empty-message convention is gone;
// silent-exit is now driven by errors.Is, not string comparison.
func TestStateCommitNow_FailureExitPreservesCauseViaUnwrap(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	cause := errors.New("tmux unreachable cause")
	f := &commitNowFixture{
		client:     &fakeCaptureClient{sessions: nil},
		captureErr: cause,
	}
	installCommitNowDeps(t, f)

	_, _, err := runStateCommitNow(t)
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
	// The wrapped error chain should mention the cause's text (via %v wrap).
	if !strings.Contains(err.Error(), "tmux unreachable cause") {
		t.Errorf("err.Error() = %q; must contain the underlying cause text", err.Error())
	}
}

// 27c. errCommitNowFailed must carry a descriptive (non-empty) message — the
// empty-string convention is no longer load-bearing.
func TestErrCommitNowFailed_HasDescriptiveMessage(t *testing.T) {
	if errCommitNowFailed.Error() == "" {
		t.Fatal("errCommitNowFailed.Error() must be non-empty; the silent-exit contract is now driven by errors.Is, not string compare")
	}
}

// 27d. IsSilentExitError must return true for the commit-now sentinel and
// any error wrapping it, so main.go's stderr-suppression guard is
// compile-time-linked to the cmd package.
func TestIsSilentExitError_DetectsCommitNowSentinel(t *testing.T) {
	if !IsSilentExitError(errCommitNowFailed) {
		t.Error("IsSilentExitError(errCommitNowFailed) = false; want true")
	}
	wrapped := fmt.Errorf("%w: %v", errCommitNowFailed, errors.New("boom"))
	if !IsSilentExitError(wrapped) {
		t.Errorf("IsSilentExitError(wrapped commit-now err) = false; want true (err=%v)", wrapped)
	}
}

// 27e. IsSilentExitError must also cover ErrStatusUnhealthy so the silent
// stderr-suppression contract spans both subcommands.
func TestIsSilentExitError_DetectsStatusUnhealthy(t *testing.T) {
	if !IsSilentExitError(ErrStatusUnhealthy) {
		t.Error("IsSilentExitError(ErrStatusUnhealthy) = false; want true")
	}
}

// 27f. IsSilentExitError must return false for ordinary errors so the
// suppression guard does not over-fire.
func TestIsSilentExitError_RejectsOrdinaryErrors(t *testing.T) {
	if IsSilentExitError(nil) {
		t.Error("IsSilentExitError(nil) = true; want false")
	}
	if IsSilentExitError(errors.New("unrelated")) {
		t.Error("IsSilentExitError(unrelated err) = true; want false")
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
