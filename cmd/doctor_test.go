// Tests in this file mutate package-level Cobra/DI state (doctorDeps, rootCmd)
// and MUST NOT use t.Parallel.
package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

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
// state dir, sessions.json.
func TestDoctorCheckOrder(t *testing.T) {
	dir := t.TempDir()
	seedHealthyStateDir(t, dir)
	results, err := runDoctorDiagnosis(withHealthyRuntime(&DoctorDeps{StateDir: dir, Now: time.Now}))
	if err != nil {
		t.Fatalf("runDoctorDiagnosis: %v", err)
	}
	want := []string{"daemon", "saver", "hooks", "state dir", "sessions.json"}
	if len(results) != len(want) {
		t.Fatalf("check count = %d, want %d: %+v", len(results), len(want), results)
	}
	for i, name := range want {
		if results[i].name != name {
			t.Errorf("results[%d].name = %q, want %q", i, results[i].name, name)
		}
	}
}
