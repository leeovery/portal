// Tests in this file mutate package-level Cobra/DI state (doctorDeps, rootCmd)
// and MUST NOT use t.Parallel.
package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/state"
)

// seedLiveDaemonPID writes daemon.pid pointing at the current process so the
// liveness probe always succeeds in-test. Self-contained (does not borrow the
// state-status test helpers, which Task 4-7 deletes).
func seedLiveDaemonPID(t *testing.T, dir string) {
	t.Helper()
	pid := strconv.Itoa(os.Getpid())
	if err := os.WriteFile(state.DaemonPID(dir), []byte(pid+"\n"), 0o600); err != nil {
		t.Fatalf("write daemon.pid: %v", err)
	}
}

// seedDeadDaemonPID writes daemon.pid pointing at an almost-certainly-dead PID
// so the liveness probe fails.
func seedDeadDaemonPID(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(state.DaemonPID(dir), []byte("999999\n"), 0o600); err != nil {
		t.Fatalf("write daemon.pid: %v", err)
	}
}

// seedDaemonVersion writes daemon.version with the supplied marker.
func seedDaemonVersion(t *testing.T, dir, version string) {
	t.Helper()
	if err := os.WriteFile(state.DaemonVersion(dir), []byte(version+"\n"), 0o600); err != nil {
		t.Fatalf("write daemon.version: %v", err)
	}
}

// seedValidSessionsJSON writes a canonical sessions.json with the given number
// of single-window/single-pane sessions.
func seedValidSessionsJSON(t *testing.T, dir string, sessions int) {
	t.Helper()
	idx := state.Index{Version: state.SchemaVersion, SavedAt: time.Now()}
	for i := range sessions {
		idx.Sessions = append(idx.Sessions, state.Session{
			Name: "s" + strconv.Itoa(i),
			Windows: []state.Window{
				{Index: 0, Name: "main", Panes: []state.Pane{{Index: 0, CWD: "/tmp"}}},
			},
		})
	}
	data, err := state.EncodeIndex(idx)
	if err != nil {
		t.Fatalf("EncodeIndex: %v", err)
	}
	if err := os.WriteFile(state.SessionsJSON(dir), data, 0o600); err != nil {
		t.Fatalf("write sessions.json: %v", err)
	}
}

// allHooksHealthy is a HookCounts result with exactly one Portal entry on
// every managed event — the healthy hooks state. Mirrors the managedEvents
// table in internal/tmux (which cmd cannot import at test time); the doctor
// hooks check only inspects the per-event counts, so the key set stands in for
// the canonical event set.
func allHooksHealthy() map[string]int {
	return map[string]int{
		"session-created":        1,
		"session-closed":         1,
		"session-renamed":        1,
		"window-linked":          1,
		"window-unlinked":        1,
		"window-layout-changed":  1,
		"pane-focus-out":         1,
		"client-attached":        1,
		"client-session-changed": 1,
	}
}

// withHealthyRuntime fills the three runtime tmux probe seams on deps with a
// healthy running server (server up, saver present, one hook per managed
// event) unless the caller already set them. State-check-focused tests use it
// so the server gate opens and the runtime checks pass, isolating the
// server-independent state checks under test.
func withHealthyRuntime(deps *DoctorDeps) *DoctorDeps {
	if deps.ServerRunning == nil {
		deps.ServerRunning = func() bool { return true }
	}
	if deps.SaverPresent == nil {
		deps.SaverPresent = func() (bool, error) { return true, nil }
	}
	if deps.HookCounts == nil {
		deps.HookCounts = func() (map[string]int, error) { return allHooksHealthy(), nil }
	}
	return deps
}

// seedHealthyStateDir seeds a live daemon.pid, a daemon.version marker, and a
// valid single-session sessions.json so every state-based check passes — used
// by tests that want to isolate a single runtime probe as the ONLY unhealthy
// (or not-evaluable) check.
func seedHealthyStateDir(t *testing.T, dir string) {
	t.Helper()
	seedLiveDaemonPID(t, dir)
	seedDaemonVersion(t, dir, "v9.9.9")
	seedValidSessionsJSON(t, dir, 1)
}

// runDoctor executes "portal doctor" with a hermetic DoctorDeps.StateDir
// pointing at dir, returning stdout, stderr, and the rootCmd.Execute error.
// The runtime tmux probe seams default to a healthy running server so the
// existing state-check tests exercise their subject with the server gate open;
// tests asserting a down/absent runtime override the seams before calling.
func runDoctor(t *testing.T, dir string) (*bytes.Buffer, *bytes.Buffer, error) {
	t.Helper()
	doctorDeps = withHealthyRuntime(&DoctorDeps{StateDir: dir, Now: time.Now})
	t.Cleanup(func() { doctorDeps = nil })

	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	resetRootCmd()
	rootCmd.SetOut(outBuf)
	rootCmd.SetErr(errBuf)
	rootCmd.SetArgs([]string{"doctor"})
	err := rootCmd.Execute()
	return outBuf, errBuf, err
}

// findCheck locates the checkResult with the given name, failing the test when
// absent.
func findCheck(t *testing.T, results []checkResult, name string) checkResult {
	t.Helper()
	for _, r := range results {
		if r.name == name {
			return r
		}
	}
	t.Fatalf("no check named %q in %+v", name, results)
	return checkResult{}
}

func TestDoctorAllStateChecksPassExitsZero(t *testing.T) {
	dir := t.TempDir()
	seedLiveDaemonPID(t, dir)
	seedDaemonVersion(t, dir, "v9.9.9")
	seedValidSessionsJSON(t, dir, 1)

	outBuf, _, err := runDoctor(t, dir)
	if err != nil {
		t.Fatalf("Execute returned %v; want nil when every state check passes", err)
	}

	out := outBuf.String()
	if !strings.HasPrefix(out, "Portal doctor:\n") {
		t.Errorf("report missing header line:\n%s", out)
	}
	wantDaemon := "running (pid " + strconv.Itoa(os.Getpid()) + ", version v9.9.9)"
	if !strings.Contains(out, wantDaemon) {
		t.Errorf("report missing daemon detail %q:\n%s", wantDaemon, out)
	}
	if !strings.Contains(out, "1 sessions, 1 panes") {
		t.Errorf("report missing sessions detail:\n%s", out)
	}
}

func TestDoctorDeadDaemonFailsNonZero(t *testing.T) {
	dir := t.TempDir()
	seedDeadDaemonPID(t, dir)
	seedValidSessionsJSON(t, dir, 1)

	outBuf, _, err := runDoctor(t, dir)
	if err != ErrDoctorUnhealthy {
		t.Fatalf("Execute err = %v; want ErrDoctorUnhealthy when daemon.pid is dead", err)
	}
	if !strings.Contains(outBuf.String(), "daemon: not running") {
		t.Errorf("report missing \"daemon: not running\":\n%s", outBuf.String())
	}
}

func TestDoctorFreshInstallReportedHonestly(t *testing.T) {
	// A path whose parent exists but the state dir itself does not — a fresh
	// install with no state dir, no daemon.pid, no sessions.json.
	dir := filepath.Join(t.TempDir(), "state")

	outBuf, _, err := runDoctor(t, dir)
	// daemon-alive fails → overall unhealthy, but no crash.
	if err != ErrDoctorUnhealthy {
		t.Fatalf("Execute err = %v; want ErrDoctorUnhealthy on fresh install (daemon down)", err)
	}
	out := outBuf.String()
	if !strings.Contains(out, "daemon: not running") {
		t.Errorf("report missing daemon-down line:\n%s", out)
	}
	if !strings.Contains(out, "state dir: not created yet") {
		t.Errorf("state-dir-sane must pass with \"not created yet\":\n%s", out)
	}
	if !strings.Contains(out, "sessions.json: no sessions saved yet") {
		t.Errorf("absent sessions.json must pass:\n%s", out)
	}
}

func TestDoctorIsReadOnly(t *testing.T) {
	// A non-existent state dir must NOT be created by a diagnosis pass —
	// doctor is strictly read-only (state.Dir(), never EnsureDir).
	dir := filepath.Join(t.TempDir(), "state")

	if _, _, err := runDoctor(t, dir); err != ErrDoctorUnhealthy {
		t.Fatalf("Execute err = %v; want ErrDoctorUnhealthy", err)
	}

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("doctor created the state dir %q (stat err = %v); want it left absent", dir, err)
	}
}

func TestDoctorSessionsJSONStatesDistinguished(t *testing.T) {
	now := func() time.Time { return time.Now() }

	t.Run("valid index reports N sessions, M panes", func(t *testing.T) {
		dir := t.TempDir()
		seedValidSessionsJSON(t, dir, 3)
		results, err := runDoctorDiagnosis(withHealthyRuntime(&DoctorDeps{StateDir: dir, Now: now}))
		if err != nil {
			t.Fatalf("runDoctorDiagnosis: %v", err)
		}
		got := findCheck(t, results, "sessions.json")
		if got.status != checkPass {
			t.Errorf("status = %v; want checkPass", got.status)
		}
		if got.detail != "3 sessions, 3 panes" {
			t.Errorf("detail = %q; want %q", got.detail, "3 sessions, 3 panes")
		}
	})

	t.Run("absent sessions.json passes as no sessions saved yet", func(t *testing.T) {
		dir := t.TempDir()
		results, err := runDoctorDiagnosis(withHealthyRuntime(&DoctorDeps{StateDir: dir, Now: now}))
		if err != nil {
			t.Fatalf("runDoctorDiagnosis: %v", err)
		}
		got := findCheck(t, results, "sessions.json")
		if got.status != checkPass {
			t.Errorf("status = %v; want checkPass for absent file", got.status)
		}
		if got.detail != "no sessions saved yet" {
			t.Errorf("detail = %q; want %q", got.detail, "no sessions saved yet")
		}
	})

	t.Run("corrupt sessions.json fails", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(state.SessionsJSON(dir), []byte("{not json"), 0o600); err != nil {
			t.Fatalf("write garbage sessions.json: %v", err)
		}
		results, err := runDoctorDiagnosis(withHealthyRuntime(&DoctorDeps{StateDir: dir, Now: now}))
		if err != nil {
			t.Fatalf("runDoctorDiagnosis: %v", err)
		}
		got := findCheck(t, results, "sessions.json")
		if got.status != checkFail {
			t.Errorf("status = %v; want checkFail for corrupt file", got.status)
		}
	})
}

func TestDoctorDaemonCheckDetail(t *testing.T) {
	t.Run("live pid passes with pid+version detail", func(t *testing.T) {
		dir := t.TempDir()
		seedLiveDaemonPID(t, dir)
		seedDaemonVersion(t, dir, "v1.2.3")
		results, err := runDoctorDiagnosis(withHealthyRuntime(&DoctorDeps{StateDir: dir, Now: time.Now}))
		if err != nil {
			t.Fatalf("runDoctorDiagnosis: %v", err)
		}
		got := findCheck(t, results, "daemon")
		if got.status != checkPass {
			t.Fatalf("status = %v; want checkPass", got.status)
		}
		want := "running (pid " + strconv.Itoa(os.Getpid()) + ", version v1.2.3)"
		if got.detail != want {
			t.Errorf("detail = %q; want %q", got.detail, want)
		}
	})

	t.Run("missing pid fails as not running", func(t *testing.T) {
		dir := t.TempDir()
		results, err := runDoctorDiagnosis(withHealthyRuntime(&DoctorDeps{StateDir: dir, Now: time.Now}))
		if err != nil {
			t.Fatalf("runDoctorDiagnosis: %v", err)
		}
		got := findCheck(t, results, "daemon")
		if got.status != checkFail {
			t.Errorf("status = %v; want checkFail", got.status)
		}
		if got.detail != "not running" {
			t.Errorf("detail = %q; want %q", got.detail, "not running")
		}
	})
}

func TestDoctorStateDirSaneHealthyDirPasses(t *testing.T) {
	dir := t.TempDir()
	results, err := runDoctorDiagnosis(withHealthyRuntime(&DoctorDeps{StateDir: dir, Now: time.Now}))
	if err != nil {
		t.Fatalf("runDoctorDiagnosis: %v", err)
	}
	got := findCheck(t, results, "state dir")
	if got.status != checkPass {
		t.Errorf("status = %v; want checkPass for an existing directory", got.status)
	}
}

func TestDoctorRegisteredInSkipTmuxCheck(t *testing.T) {
	if !skipTmuxCheck["doctor"] {
		t.Error("skipTmuxCheck[\"doctor\"] = false; want true (Bootstrap Exemption)")
	}
}

func TestDoctorIsRegisteredCommand(t *testing.T) {
	for _, c := range rootCmd.Commands() {
		if c.Name() == "doctor" {
			return
		}
	}
	t.Error("doctor is not registered on rootCmd")
}

func TestDoctorRejectsArgs(t *testing.T) {
	dir := t.TempDir()
	seedLiveDaemonPID(t, dir)
	seedDaemonVersion(t, dir, "v9.9.9")
	seedValidSessionsJSON(t, dir, 1)

	doctorDeps = &DoctorDeps{StateDir: dir, Now: time.Now}
	t.Cleanup(func() { doctorDeps = nil })
	resetRootCmd()
	rootCmd.SetArgs([]string{"doctor", "unexpected"})
	if err := rootCmd.Execute(); err == nil {
		t.Error("Execute returned nil for `doctor unexpected`; want a NoArgs error")
	}
}

func TestDoctorSilenceFlags(t *testing.T) {
	if !doctorCmd.SilenceErrors {
		t.Error("doctorCmd.SilenceErrors = false; want true")
	}
	if !doctorCmd.SilenceUsage {
		t.Error("doctorCmd.SilenceUsage = false; want true")
	}
}

func TestDoctorUnhealthyStderrSilent(t *testing.T) {
	dir := t.TempDir()
	seedDeadDaemonPID(t, dir)

	_, errBuf, err := runDoctor(t, dir)
	if err != ErrDoctorUnhealthy {
		t.Fatalf("Execute err = %v; want ErrDoctorUnhealthy", err)
	}
	if errBuf.Len() != 0 {
		t.Errorf("expected silent stderr on unhealthy exit; got %q", errBuf.String())
	}
}

func TestIsSilentExitErrorRecognisesDoctorUnhealthy(t *testing.T) {
	if !IsSilentExitError(ErrDoctorUnhealthy) {
		t.Error("IsSilentExitError(ErrDoctorUnhealthy) = false; want true")
	}
}

// doctorRuntimeNotRunningDetail is the byte-exact detail every runtime check
// emits when the tmux server is down. Duplicated here (not imported) so the
// test independently pins the contract rather than trusting the production
// constant.
const doctorRuntimeNotRunningDetail = "Portal runtime not running — run portal open to start"

func TestDoctorServerDownReportsRuntimeNotRunning(t *testing.T) {
	dir := t.TempDir()
	// A live daemon.pid on disk must NOT rescue the daemon check: a down server
	// gates daemon, saver AND hooks to the distinct not-running message.
	seedHealthyStateDir(t, dir)

	deps := &DoctorDeps{
		StateDir:      dir,
		Now:           time.Now,
		ServerRunning: func() bool { return false },
		// Healthy probe returns: if the gate is (wrongly) bypassed these would
		// produce PASS details, so the not-running assertions below would fail
		// loudly — proving the down gate short-circuits the probes.
		SaverPresent: func() (bool, error) { return true, nil },
		HookCounts:   func() (map[string]int, error) { return allHooksHealthy(), nil },
	}
	results, err := runDoctorDiagnosis(deps)
	if err != nil {
		t.Fatalf("runDoctorDiagnosis: %v", err)
	}

	for _, name := range []string{"daemon", "saver", "hooks"} {
		got := findCheck(t, results, name)
		if got.status != checkFail {
			t.Errorf("%s status = %v; want checkFail when server is down", name, got.status)
		}
		if got.detail != doctorRuntimeNotRunningDetail {
			t.Errorf("%s detail = %q; want %q", name, got.detail, doctorRuntimeNotRunningDetail)
		}
	}

	// The down-server report is unhealthy → non-zero, distinct from corruption.
	if !doctorUnhealthy(results) {
		t.Error("doctorUnhealthy = false; want true for a down server")
	}

	// State-based checks stay server-independent and pass on a healthy dir.
	if got := findCheck(t, results, "state dir"); got.status != checkPass {
		t.Errorf("state dir status = %v; want checkPass (server-independent)", got.status)
	}
	if got := findCheck(t, results, "sessions.json"); got.status != checkPass {
		t.Errorf("sessions.json status = %v; want checkPass (server-independent)", got.status)
	}
}

func TestDoctorHooksCheck(t *testing.T) {
	dir := t.TempDir()

	newDeps := func(counts map[string]int) *DoctorDeps {
		return &DoctorDeps{
			StateDir:      dir,
			Now:           time.Now,
			ServerRunning: func() bool { return true },
			SaverPresent:  func() (bool, error) { return true, nil },
			HookCounts:    func() (map[string]int, error) { return counts, nil },
		}
	}

	t.Run("one entry per event passes", func(t *testing.T) {
		results, err := runDoctorDiagnosis(newDeps(allHooksHealthy()))
		if err != nil {
			t.Fatalf("runDoctorDiagnosis: %v", err)
		}
		got := findCheck(t, results, "hooks")
		if got.status != checkPass {
			t.Errorf("status = %v; want checkPass", got.status)
		}
		if got.detail != "hooks registered (one per event)" {
			t.Errorf("detail = %q; want %q", got.detail, "hooks registered (one per event)")
		}
	})

	t.Run("a duplicated event fails", func(t *testing.T) {
		counts := allHooksHealthy()
		counts["pane-focus-out"] = 3
		results, err := runDoctorDiagnosis(newDeps(counts))
		if err != nil {
			t.Fatalf("runDoctorDiagnosis: %v", err)
		}
		got := findCheck(t, results, "hooks")
		if got.status != checkFail {
			t.Errorf("status = %v; want checkFail", got.status)
		}
		if got.detail != "duplicate hook entries on pane-focus-out (3)" {
			t.Errorf("detail = %q; want %q", got.detail, "duplicate hook entries on pane-focus-out (3)")
		}
	})

	t.Run("duplicate reports first offending event in sorted order", func(t *testing.T) {
		counts := allHooksHealthy()
		counts["window-linked"] = 2
		counts["client-attached"] = 2 // "client-attached" < "window-linked"
		results, err := runDoctorDiagnosis(newDeps(counts))
		if err != nil {
			t.Fatalf("runDoctorDiagnosis: %v", err)
		}
		got := findCheck(t, results, "hooks")
		if got.detail != "duplicate hook entries on client-attached (2)" {
			t.Errorf("detail = %q; want %q (first in sorted order)", got.detail, "duplicate hook entries on client-attached (2)")
		}
	})

	t.Run("a zero-count event fails as not registered", func(t *testing.T) {
		counts := allHooksHealthy()
		counts["client-attached"] = 0
		results, err := runDoctorDiagnosis(newDeps(counts))
		if err != nil {
			t.Fatalf("runDoctorDiagnosis: %v", err)
		}
		got := findCheck(t, results, "hooks")
		if got.status != checkFail {
			t.Errorf("status = %v; want checkFail", got.status)
		}
		if got.detail != "hooks not registered on client-attached" {
			t.Errorf("detail = %q; want %q", got.detail, "hooks not registered on client-attached")
		}
	})

	t.Run("duplicate takes precedence over a zero-count event", func(t *testing.T) {
		counts := allHooksHealthy()
		counts["session-renamed"] = 0
		counts["window-linked"] = 2
		results, err := runDoctorDiagnosis(newDeps(counts))
		if err != nil {
			t.Fatalf("runDoctorDiagnosis: %v", err)
		}
		got := findCheck(t, results, "hooks")
		if got.detail != "duplicate hook entries on window-linked (2)" {
			t.Errorf("detail = %q; want the duplicate message to win over the zero-count message", got.detail)
		}
	})

	t.Run("transient read failure is not-evaluable and does not drive exit", func(t *testing.T) {
		seedHealthyStateDir(t, dir)
		deps := &DoctorDeps{
			StateDir:      dir,
			Now:           time.Now,
			ServerRunning: func() bool { return true },
			SaverPresent:  func() (bool, error) { return true, nil },
			HookCounts:    func() (map[string]int, error) { return nil, errors.New("tmux transient") },
		}
		results, err := runDoctorDiagnosis(deps)
		if err != nil {
			t.Fatalf("runDoctorDiagnosis: %v", err)
		}
		got := findCheck(t, results, "hooks")
		if got.status != checkNotEvaluable {
			t.Errorf("status = %v; want checkNotEvaluable on a transient hooks read", got.status)
		}
		if got.detail != "could not read hooks (transient tmux error)" {
			t.Errorf("detail = %q; want %q", got.detail, "could not read hooks (transient tmux error)")
		}
		if doctorUnhealthy(results) {
			t.Error("a not-evaluable hooks check must not drive the exit code")
		}
	})
}

func TestDoctorSaverCheck(t *testing.T) {
	dir := t.TempDir()

	newDeps := func(present bool, saverErr error) *DoctorDeps {
		return &DoctorDeps{
			StateDir:      dir,
			Now:           time.Now,
			ServerRunning: func() bool { return true },
			SaverPresent:  func() (bool, error) { return present, saverErr },
			HookCounts:    func() (map[string]int, error) { return allHooksHealthy(), nil },
		}
	}

	t.Run("present passes", func(t *testing.T) {
		results, err := runDoctorDiagnosis(newDeps(true, nil))
		if err != nil {
			t.Fatalf("runDoctorDiagnosis: %v", err)
		}
		got := findCheck(t, results, "saver")
		if got.status != checkPass {
			t.Errorf("status = %v; want checkPass", got.status)
		}
		if got.detail != "_portal-saver up" {
			t.Errorf("detail = %q; want %q", got.detail, "_portal-saver up")
		}
	})

	t.Run("absent fails", func(t *testing.T) {
		results, err := runDoctorDiagnosis(newDeps(false, nil))
		if err != nil {
			t.Fatalf("runDoctorDiagnosis: %v", err)
		}
		got := findCheck(t, results, "saver")
		if got.status != checkFail {
			t.Errorf("status = %v; want checkFail", got.status)
		}
		if got.detail != "_portal-saver not running" {
			t.Errorf("detail = %q; want %q", got.detail, "_portal-saver not running")
		}
	})

	t.Run("transient error is not-evaluable and does not drive exit", func(t *testing.T) {
		seedHealthyStateDir(t, dir)
		results, err := runDoctorDiagnosis(newDeps(false, errors.New("tmux transient")))
		if err != nil {
			t.Fatalf("runDoctorDiagnosis: %v", err)
		}
		got := findCheck(t, results, "saver")
		if got.status != checkNotEvaluable {
			t.Errorf("status = %v; want checkNotEvaluable on a transient saver read", got.status)
		}
		if got.detail != "could not read saver (transient tmux error)" {
			t.Errorf("detail = %q; want %q", got.detail, "could not read saver (transient tmux error)")
		}
		if doctorUnhealthy(results) {
			t.Error("a not-evaluable saver check must not drive the exit code")
		}
	})
}

// TestDoctorCheckOrder pins the stable report order: daemon, saver, hooks,
// state dir, sessions.json, stale hooks, stale projects.
func TestDoctorCheckOrder(t *testing.T) {
	dir := t.TempDir()
	seedHealthyStateDir(t, dir)
	results, err := runDoctorDiagnosis(withHealthyRuntime(&DoctorDeps{StateDir: dir, Now: time.Now}))
	if err != nil {
		t.Fatalf("runDoctorDiagnosis: %v", err)
	}
	want := []string{"daemon", "saver", "hooks", "state dir", "sessions.json", "stale hooks", "stale projects"}
	if len(results) != len(want) {
		t.Fatalf("check count = %d, want %d: %+v", len(results), len(want), results)
	}
	for i, name := range want {
		if results[i].name != name {
			t.Errorf("results[%d].name = %q, want %q", i, results[i].name, name)
		}
	}
}

// fakeHookLister is an AllPaneLister fake for the stale-hooks check: it returns
// the crafted live hook-key set (or an error, to exercise the transient path)
// without touching a real tmux server.
type fakeHookLister struct {
	keys []string
	err  error
}

func (f fakeHookLister) ListAllPaneHookKeys() ([]string, error) { return f.keys, f.err }

// seedHooksJSON writes a hooks.json at a fresh temp path with one on-resume
// entry per supplied hook key and returns the store plus the file path (so a
// read-only test can snapshot the bytes). Zero keys writes an empty object.
func seedHooksJSON(t *testing.T, keys ...string) (*hooks.Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hooks.json")
	m := map[string]map[string]string{}
	for _, k := range keys {
		m[k] = map[string]string{"on-resume": "echo hi"}
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatalf("marshal hooks.json: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write hooks.json: %v", err)
	}
	return hooks.NewStore(path), path
}

// seedProjectsJSON writes a projects.json at a fresh temp path with one project
// record per supplied path and returns the store plus the file path.
func seedProjectsJSON(t *testing.T, paths ...string) (*project.Store, string) {
	t.Helper()
	file := filepath.Join(t.TempDir(), "projects.json")
	var ps []project.Project
	for i, p := range paths {
		ps = append(ps, project.Project{Path: p, Name: "proj" + strconv.Itoa(i)})
	}
	payload := struct {
		Projects []project.Project `json:"projects"`
	}{Projects: ps}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatalf("marshal projects.json: %v", err)
	}
	if err := os.WriteFile(file, data, 0o600); err != nil {
		t.Fatalf("write projects.json: %v", err)
	}
	return project.NewStore(file), file
}

// staleDeps builds a DoctorDeps with a healthy runtime and the stale-entry
// seams wired to the supplied lister/stores.
func staleDeps(dir string, lister AllPaneLister, hookStore *hooks.Store, projectStore *project.Store) *DoctorDeps {
	return withHealthyRuntime(&DoctorDeps{
		StateDir:     dir,
		Now:          time.Now,
		HookLister:   lister,
		HookStore:    hookStore,
		ProjectStore: projectStore,
	})
}

func TestDoctorStaleHooksCheck(t *testing.T) {
	t.Run("persisted key with no live pane fails", func(t *testing.T) {
		dir := t.TempDir()
		hookStore, _ := seedHooksJSON(t, "sessA:0.0")
		lister := fakeHookLister{keys: []string{"sessB:0.0"}}
		results, err := runDoctorDiagnosis(staleDeps(dir, lister, hookStore, nil))
		if err != nil {
			t.Fatalf("runDoctorDiagnosis: %v", err)
		}
		got := findCheck(t, results, "stale hooks")
		if got.status != checkFail {
			t.Errorf("status = %v; want checkFail for a stale hook entry", got.status)
		}
		if got.detail != "1 stale hook entries" {
			t.Errorf("detail = %q; want %q", got.detail, "1 stale hook entries")
		}
	})

	t.Run("zero live panes with hooks present is not-evaluable, never all-stale", func(t *testing.T) {
		dir := t.TempDir()
		hookStore, _ := seedHooksJSON(t, "sessA:0.0", "sessB:0.0")
		lister := fakeHookLister{keys: []string{}}
		results, err := runDoctorDiagnosis(staleDeps(dir, lister, hookStore, nil))
		if err != nil {
			t.Fatalf("runDoctorDiagnosis: %v", err)
		}
		got := findCheck(t, results, "stale hooks")
		if got.status != checkNotEvaluable {
			t.Errorf("status = %v; want checkNotEvaluable (never a mass-stale failure)", got.status)
		}
		if got.detail != "zero live panes with hooks present (not evaluable)" {
			t.Errorf("detail = %q; want %q", got.detail, "zero live panes with hooks present (not evaluable)")
		}
	})

	t.Run("live-pane enumeration error is not-evaluable", func(t *testing.T) {
		dir := t.TempDir()
		hookStore, _ := seedHooksJSON(t, "sessA:0.0")
		lister := fakeHookLister{err: errors.New("tmux transient")}
		results, err := runDoctorDiagnosis(staleDeps(dir, lister, hookStore, nil))
		if err != nil {
			t.Fatalf("runDoctorDiagnosis: %v", err)
		}
		got := findCheck(t, results, "stale hooks")
		if got.status != checkNotEvaluable {
			t.Errorf("status = %v; want checkNotEvaluable on an enumeration error", got.status)
		}
		if got.detail != "could not enumerate live panes" {
			t.Errorf("detail = %q; want %q", got.detail, "could not enumerate live panes")
		}
	})

	t.Run("both empty passes as no hooks", func(t *testing.T) {
		dir := t.TempDir()
		hookStore, _ := seedHooksJSON(t)
		lister := fakeHookLister{keys: []string{}}
		results, err := runDoctorDiagnosis(staleDeps(dir, lister, hookStore, nil))
		if err != nil {
			t.Fatalf("runDoctorDiagnosis: %v", err)
		}
		got := findCheck(t, results, "stale hooks")
		if got.status != checkPass {
			t.Errorf("status = %v; want checkPass", got.status)
		}
		if got.detail != "no hooks" {
			t.Errorf("detail = %q; want %q", got.detail, "no hooks")
		}
	})

	t.Run("all persisted keys live passes", func(t *testing.T) {
		dir := t.TempDir()
		hookStore, _ := seedHooksJSON(t, "sessA:0.0", "sessB:0.0")
		lister := fakeHookLister{keys: []string{"sessA:0.0", "sessB:0.0", "sessC:0.0"}}
		results, err := runDoctorDiagnosis(staleDeps(dir, lister, hookStore, nil))
		if err != nil {
			t.Fatalf("runDoctorDiagnosis: %v", err)
		}
		got := findCheck(t, results, "stale hooks")
		if got.status != checkPass {
			t.Errorf("status = %v; want checkPass", got.status)
		}
		if got.detail != "no stale hooks" {
			t.Errorf("detail = %q; want %q", got.detail, "no stale hooks")
		}
	})
}

func TestDoctorStaleProjectsCheck(t *testing.T) {
	t.Run("gone dir fails, live dir retained", func(t *testing.T) {
		dir := t.TempDir()
		liveDir := t.TempDir()
		goneDir := filepath.Join(t.TempDir(), "does-not-exist")
		projectStore, _ := seedProjectsJSON(t, liveDir, goneDir)
		results, err := runDoctorDiagnosis(staleDeps(dir, fakeHookLister{}, nil, projectStore))
		if err != nil {
			t.Fatalf("runDoctorDiagnosis: %v", err)
		}
		got := findCheck(t, results, "stale projects")
		if got.status != checkFail {
			t.Errorf("status = %v; want checkFail for a gone-dir project", got.status)
		}
		// Only the gone dir is stale; the live dir is retained (not counted).
		if got.detail != "1 stale projects" {
			t.Errorf("detail = %q; want %q", got.detail, "1 stale projects")
		}
	})

	t.Run("all live passes", func(t *testing.T) {
		dir := t.TempDir()
		liveDir := t.TempDir()
		projectStore, _ := seedProjectsJSON(t, liveDir)
		results, err := runDoctorDiagnosis(staleDeps(dir, fakeHookLister{}, nil, projectStore))
		if err != nil {
			t.Fatalf("runDoctorDiagnosis: %v", err)
		}
		got := findCheck(t, results, "stale projects")
		if got.status != checkPass {
			t.Errorf("status = %v; want checkPass", got.status)
		}
		if got.detail != "no stale projects" {
			t.Errorf("detail = %q; want %q", got.detail, "no stale projects")
		}
	})

	// Permission-denied paths are RETAINED (not stale) by the same os.Stat
	// default branch project.Store.CleanStale uses — that classification is
	// covered by the CleanStale model in internal/project; simulating EACCES
	// portably here is infeasible, so this suite covers gone-dir + live-dir.

	t.Run("evaluates with the server down (filesystem-only)", func(t *testing.T) {
		goneDir := filepath.Join(t.TempDir(), "gone")
		projectStore, _ := seedProjectsJSON(t, goneDir)
		deps := &DoctorDeps{
			StateDir:      t.TempDir(),
			Now:           time.Now,
			ServerRunning: func() bool { return false },
			SaverPresent:  func() (bool, error) { return true, nil },
			HookCounts:    func() (map[string]int, error) { return allHooksHealthy(), nil },
			HookLister:    fakeHookLister{},
			ProjectStore:  projectStore,
		}
		results, err := runDoctorDiagnosis(deps)
		if err != nil {
			t.Fatalf("runDoctorDiagnosis: %v", err)
		}
		got := findCheck(t, results, "stale projects")
		if got.status != checkFail {
			t.Errorf("status = %v; want checkFail — the stale-project check is filesystem-only and runs with the server down", got.status)
		}
	})
}

// TestDoctorStaleChecksAreReadOnly proves neither stale check mutates its store:
// both are seeded with genuinely-stale entries (so they detect staleness and
// would prune under --fix) and the on-disk bytes must be byte-identical after a
// full diagnosis pass.
func TestDoctorStaleChecksAreReadOnly(t *testing.T) {
	dir := t.TempDir()
	hookStore, hooksPath := seedHooksJSON(t, "sessA:0.0")
	liveDir := t.TempDir()
	goneDir := filepath.Join(t.TempDir(), "gone")
	projectStore, projectsPath := seedProjectsJSON(t, liveDir, goneDir)

	hooksBefore, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("read hooks.json: %v", err)
	}
	projectsBefore, err := os.ReadFile(projectsPath)
	if err != nil {
		t.Fatalf("read projects.json: %v", err)
	}

	lister := fakeHookLister{keys: []string{"sessB:0.0"}} // sessA:0.0 is stale
	results, err := runDoctorDiagnosis(staleDeps(dir, lister, hookStore, projectStore))
	if err != nil {
		t.Fatalf("runDoctorDiagnosis: %v", err)
	}
	// Sanity: both checks actually detected staleness (proving they ran).
	if got := findCheck(t, results, "stale hooks"); got.status != checkFail {
		t.Fatalf("stale hooks status = %v; want checkFail (setup should be stale)", got.status)
	}
	if got := findCheck(t, results, "stale projects"); got.status != checkFail {
		t.Fatalf("stale projects status = %v; want checkFail (setup should be stale)", got.status)
	}

	hooksAfter, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("re-read hooks.json: %v", err)
	}
	projectsAfter, err := os.ReadFile(projectsPath)
	if err != nil {
		t.Fatalf("re-read projects.json: %v", err)
	}
	if !bytes.Equal(hooksBefore, hooksAfter) {
		t.Errorf("hooks.json mutated by diagnosis (read-only violated)\nbefore: %s\nafter:  %s", hooksBefore, hooksAfter)
	}
	if !bytes.Equal(projectsBefore, projectsAfter) {
		t.Errorf("projects.json mutated by diagnosis (read-only violated)\nbefore: %s\nafter:  %s", projectsBefore, projectsAfter)
	}
}
