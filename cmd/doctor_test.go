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
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/state"
)

// doctorUnsupportedResolve is a neutral host-terminal Resolve seam returning
// "unsupported" for any identity. withHealthyRuntime stubs it (with a NULL
// detector) so the appended informational host-terminal line is computed from
// injected fakes rather than a real spawn.NewDetector (which reads the process
// tree / tmux) in every doctor test.
func doctorUnsupportedResolve(spawn.Identity) (spawn.Adapter, spawn.Resolution) {
	return nil, spawn.ResolutionUnsupported
}

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
//
// It also stubs the informational host-terminal seams (a NULL detector +
// unsupported resolve) unless the caller set them, so the appended host-terminal
// line never invokes a real spawn.NewDetector (which reads the process tree /
// tmux). The stub is inert: host terminal is checkInfo, never counted by
// doctorUnhealthy.
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
	if deps.Detector == nil {
		deps.Detector = fakeTerminalDetector{}
	}
	if deps.Resolve == nil {
		deps.Resolve = doctorUnsupportedResolve
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
	// resolveDoctorDeps now sources the host-terminal seams from the shared
	// buildProductionSpawnSeams bundle, which reads terminals.json eagerly — isolate
	// it so the Execute path never touches the developer's real config file.
	isolateTerminalsFile(t)
	doctorDeps = withHealthyRuntime(&DoctorDeps{StateDir: dir})
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
	if !strings.Contains(out, "1 session, 1 pane") {
		t.Errorf("report missing sessions detail:\n%s", out)
	}
}

// TestDoctorZeroValueCheckResultNotHealthy pins the defensive iota-0 sentinel: a
// zero-value checkResult{} — the shape a forgotten status assignment would
// produce — must NOT read as pass and must NOT yield a healthy
// (doctorUnhealthy == false) result. Without an explicit checkUnknown at iota 0
// the zero value would silently classify as checkPass, letting the scriptable
// exit-code contract be satisfied by an unset status.
func TestDoctorZeroValueCheckResultNotHealthy(t *testing.T) {
	zero := checkResult{}

	if zero.status == checkPass {
		t.Errorf("zero-value checkResult{}.status = %v; want a non-pass sentinel, never checkPass", zero.status)
	}
	if zero.status != checkUnknown {
		t.Errorf("zero-value checkResult{}.status = %v; want checkUnknown (iota 0)", zero.status)
	}
	if checkUnknown != 0 {
		t.Errorf("checkUnknown = %d; want the iota-0 value", checkUnknown)
	}

	if marker := checkMarker(zero.status); marker == "✓" {
		t.Errorf("checkMarker(zero-value) = %q; a zero-value check must not render the pass marker", marker)
	}

	if !doctorUnhealthy([]checkResult{zero}) {
		t.Error("doctorUnhealthy([zero-value]) = false; a forgotten status assignment must not yield a healthy exit")
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
	t.Run("valid index reports N sessions, M panes", func(t *testing.T) {
		dir := t.TempDir()
		seedValidSessionsJSON(t, dir, 3)
		results, err := runDoctorDiagnosis(withHealthyRuntime(&DoctorDeps{StateDir: dir}))
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
		results, err := runDoctorDiagnosis(withHealthyRuntime(&DoctorDeps{StateDir: dir}))
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
		results, err := runDoctorDiagnosis(withHealthyRuntime(&DoctorDeps{StateDir: dir}))
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
		results, err := runDoctorDiagnosis(withHealthyRuntime(&DoctorDeps{StateDir: dir}))
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
		results, err := runDoctorDiagnosis(withHealthyRuntime(&DoctorDeps{StateDir: dir}))
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
	results, err := runDoctorDiagnosis(withHealthyRuntime(&DoctorDeps{StateDir: dir}))
	if err != nil {
		t.Fatalf("runDoctorDiagnosis: %v", err)
	}
	got := findCheck(t, results, "state dir")
	if got.status != checkPass {
		t.Errorf("status = %v; want checkPass for an existing directory", got.status)
	}
}

// TestDoctorStateDirSaneFailBranches pins the two checkStateDirSane failure
// paths: an existing-but-non-directory state-dir path, and an unreadable stat
// (a non-ErrNotExist os.Stat error). Both are pure unit tests — no real tmux or
// daemon — driven through runDoctorDiagnosis with a healthy runtime so only the
// state-dir check is the subject.
func TestDoctorStateDirSaneFailBranches(t *testing.T) {
	t.Run("existing path that is not a directory fails", func(t *testing.T) {
		// A regular file at the state-dir path: os.Stat succeeds but IsDir() is
		// false → the "not a directory" fail branch.
		file := filepath.Join(t.TempDir(), "state")
		if err := os.WriteFile(file, []byte("i am a file, not a dir"), 0o600); err != nil {
			t.Fatalf("write state file: %v", err)
		}
		results, err := runDoctorDiagnosis(withHealthyRuntime(&DoctorDeps{StateDir: file}))
		if err != nil {
			t.Fatalf("runDoctorDiagnosis: %v", err)
		}
		got := findCheck(t, results, "state dir")
		if got.status != checkFail {
			t.Errorf("status = %v; want checkFail when the state-dir path is a regular file", got.status)
		}
		if got.detail != "not a directory" {
			t.Errorf("detail = %q; want %q", got.detail, "not a directory")
		}
	})

	t.Run("unreadable stat fails", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root bypasses 0o000 directory permissions; the stat would not fail")
		}
		// A state-dir path nested inside a directory stripped of all permissions:
		// os.Stat of the child returns EACCES (not ErrNotExist), exercising the
		// "unreadable" fail branch.
		blocked := filepath.Join(t.TempDir(), "blocked")
		if err := os.Mkdir(blocked, 0o700); err != nil {
			t.Fatalf("mkdir blocked: %v", err)
		}
		dir := filepath.Join(blocked, "state")
		if err := os.Chmod(blocked, 0o000); err != nil {
			t.Fatalf("chmod 0o000: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(blocked, 0o700) })

		results, err := runDoctorDiagnosis(withHealthyRuntime(&DoctorDeps{StateDir: dir}))
		if err != nil {
			t.Fatalf("runDoctorDiagnosis: %v", err)
		}
		got := findCheck(t, results, "state dir")
		if got.status != checkFail {
			t.Errorf("status = %v; want checkFail on an unreadable stat", got.status)
		}
		if got.detail != "unreadable" {
			t.Errorf("detail = %q; want %q", got.detail, "unreadable")
		}
	})
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

	doctorDeps = &DoctorDeps{StateDir: dir}
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

// TestRuntimeDownResult pins the shared down-server result helper: for each of
// the three runtime-check names it must produce checkFail with the byte-exact
// doctorRuntimeNotRunning detail, so the daemon / saver / hooks checks stay
// byte-identical after routing their !serverUp arm through it.
func TestRuntimeDownResult(t *testing.T) {
	for _, name := range []string{"daemon", "saver", "hooks"} {
		got := runtimeDownResult(name)
		if got.name != name {
			t.Errorf("runtimeDownResult(%q).name = %q; want %q", name, got.name, name)
		}
		if got.status != checkFail {
			t.Errorf("runtimeDownResult(%q).status = %v; want checkFail", name, got.status)
		}
		if got.detail != doctorRuntimeNotRunningDetail {
			t.Errorf("runtimeDownResult(%q).detail = %q; want %q", name, got.detail, doctorRuntimeNotRunningDetail)
		}
	}
}

func TestDoctorHooksCheck(t *testing.T) {
	dir := t.TempDir()

	newDeps := func(counts map[string]int) *DoctorDeps {
		return &DoctorDeps{
			StateDir:      dir,
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

// TestDoctorHostTerminalLine covers the informational host-terminal line: the
// three classifications (supported / recognised-but-undriven / NULL-remote),
// each computed from the injected Detect()+Resolve seams and reported as
// checkInfo.
func TestDoctorHostTerminalLine(t *testing.T) {
	dir := t.TempDir()
	seedHealthyStateDir(t, dir)

	hostDeps := func(id spawn.Identity, resolution spawn.Resolution) *DoctorDeps {
		return withHealthyRuntime(&DoctorDeps{
			StateDir: dir,
			Detector: fakeTerminalDetector{id: id},
			Resolve: func(spawn.Identity) (spawn.Adapter, spawn.Resolution) {
				return nil, resolution
			},
		})
	}

	t.Run("driven terminal reports supported", func(t *testing.T) {
		deps := hostDeps(spawn.Identity{Name: "Ghostty", BundleID: "com.mitchellh.ghostty"}, spawn.ResolutionNative)
		results, err := runDoctorDiagnosis(deps)
		if err != nil {
			t.Fatalf("runDoctorDiagnosis: %v", err)
		}
		got := findCheck(t, results, "host terminal")
		if got.status != checkInfo {
			t.Errorf("status = %v; want checkInfo", got.status)
		}
		if got.detail != "Ghostty (supported)" {
			t.Errorf("detail = %q; want %q", got.detail, "Ghostty (supported)")
		}
	})

	t.Run("null identity reports unsupported remote session regardless of resolve", func(t *testing.T) {
		// Resolve returns Native to prove the NULL short-circuit ignores it: a
		// remote/mosh / no-host-local client can never be classified "supported".
		deps := hostDeps(spawn.Identity{}, spawn.ResolutionNative)
		results, err := runDoctorDiagnosis(deps)
		if err != nil {
			t.Fatalf("runDoctorDiagnosis: %v", err)
		}
		got := findCheck(t, results, "host terminal")
		if got.status != checkInfo {
			t.Errorf("status = %v; want checkInfo", got.status)
		}
		if got.detail != "unsupported (remote session)" {
			t.Errorf("detail = %q; want %q", got.detail, "unsupported (remote session)")
		}
	})

	t.Run("recognised but undriven terminal reports unsupported", func(t *testing.T) {
		deps := hostDeps(spawn.Identity{Name: "SomeTerm", BundleID: "com.some.term"}, spawn.ResolutionUnsupported)
		results, err := runDoctorDiagnosis(deps)
		if err != nil {
			t.Fatalf("runDoctorDiagnosis: %v", err)
		}
		got := findCheck(t, results, "host terminal")
		if got.status != checkInfo {
			t.Errorf("status = %v; want checkInfo", got.status)
		}
		if got.detail != "SomeTerm (unsupported)" {
			t.Errorf("detail = %q; want %q", got.detail, "SomeTerm (unsupported)")
		}
	})
}

// TestDoctorHostTerminalNeverDrivesExit proves the informational host-terminal
// line is outside the pass/fail set: an unsupported host can't push the exit
// non-zero, and a supported host can't rescue a genuine runtime-health failure.
func TestDoctorHostTerminalNeverDrivesExit(t *testing.T) {
	t.Run("unsupported host with a healthy runtime stays healthy", func(t *testing.T) {
		dir := t.TempDir()
		seedHealthyStateDir(t, dir)
		// NULL detector → "unsupported (remote session)"; every runtime check
		// healthy; the stale checks are not-evaluable (nil stores) — none a fail.
		deps := withHealthyRuntime(&DoctorDeps{
			StateDir: dir,
			Detector: fakeTerminalDetector{},
			Resolve:  doctorUnsupportedResolve,
		})
		results, err := runDoctorDiagnosis(deps)
		if err != nil {
			t.Fatalf("runDoctorDiagnosis: %v", err)
		}
		if got := findCheck(t, results, "host terminal"); got.detail != "unsupported (remote session)" {
			t.Fatalf("setup: host terminal detail = %q; want the unsupported line", got.detail)
		}
		if doctorUnhealthy(results) {
			t.Error("doctorUnhealthy = true; an unsupported host must never drive the exit code")
		}
	})

	t.Run("supported host does not rescue a real check failure", func(t *testing.T) {
		dir := t.TempDir()
		seedDeadDaemonPID(t, dir) // the daemon check fails → genuine unhealthy
		deps := withHealthyRuntime(&DoctorDeps{
			StateDir: dir,
			Detector: fakeTerminalDetector{id: spawn.Identity{Name: "Ghostty", BundleID: "com.mitchellh.ghostty"}},
			Resolve: func(spawn.Identity) (spawn.Adapter, spawn.Resolution) {
				return nil, spawn.ResolutionNative
			},
		})
		results, err := runDoctorDiagnosis(deps)
		if err != nil {
			t.Fatalf("runDoctorDiagnosis: %v", err)
		}
		if got := findCheck(t, results, "host terminal"); got.detail != "Ghostty (supported)" {
			t.Fatalf("setup: host terminal detail = %q; want the supported line", got.detail)
		}
		if got := findCheck(t, results, "daemon"); got.status != checkFail {
			t.Fatalf("setup: daemon status = %v; want checkFail", got.status)
		}
		if !doctorUnhealthy(results) {
			t.Error("doctorUnhealthy = false; a real check failure must stay unhealthy even with a supported host")
		}
	})
}

// TestDoctorCheckOrder pins the stable report order: daemon, saver, hooks,
// state dir, sessions.json, stale hooks, stale projects, host terminal (the
// informational host line is appended last).
func TestDoctorCheckOrder(t *testing.T) {
	dir := t.TempDir()
	seedHealthyStateDir(t, dir)
	results, err := runDoctorDiagnosis(withHealthyRuntime(&DoctorDeps{StateDir: dir}))
	if err != nil {
		t.Fatalf("runDoctorDiagnosis: %v", err)
	}
	want := []string{"daemon", "saver", "hooks", "state dir", "sessions.json", "stale hooks", "stale projects", "host terminal"}
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
		if got.detail != "1 stale hook entry" {
			t.Errorf("detail = %q; want %q", got.detail, "1 stale hook entry")
		}
	})

	t.Run("multiple stale entries use the plural count copy", func(t *testing.T) {
		dir := t.TempDir()
		hookStore, _ := seedHooksJSON(t, "sessA:0.0", "sessB:0.0")
		lister := fakeHookLister{keys: []string{"sessC:0.0"}}
		results, err := runDoctorDiagnosis(staleDeps(dir, lister, hookStore, nil))
		if err != nil {
			t.Fatalf("runDoctorDiagnosis: %v", err)
		}
		got := findCheck(t, results, "stale hooks")
		if got.status != checkFail {
			t.Errorf("status = %v; want checkFail for stale hook entries", got.status)
		}
		if got.detail != "2 stale hook entries" {
			t.Errorf("detail = %q; want %q", got.detail, "2 stale hook entries")
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

// TestDoctorStaleHooksParityWithPredicate proves checkStaleHooks derives its
// stale count from the same hooks.StaleKeys predicate the prune uses: for
// representative persisted/live inputs past the hazard guard, the reported count
// equals len(hooks.StaleKeys(persisted, live)); and the hazard-guard paths still
// map to checkNotEvaluable/checkPass with no prune (byte-identical store).
func TestDoctorStaleHooksParityWithPredicate(t *testing.T) {
	t.Run("past-guard count equals the shared predicate", func(t *testing.T) {
		cases := []struct {
			name      string
			persisted []string
			live      []string
		}{
			{"one stale", []string{"sessA:0.0"}, []string{"sessB:0.0"}},
			{"two stale", []string{"sessA:0.0", "sessB:0.0"}, []string{"sessC:0.0"}},
			{"one of three stale", []string{"sessA:0.0", "sessB:0.0", "sessC:0.0"}, []string{"sessA:0.0", "sessB:0.0"}},
			{"none stale", []string{"sessA:0.0"}, []string{"sessA:0.0", "sessB:0.0"}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				dir := t.TempDir()
				hookStore, _ := seedHooksJSON(t, tc.persisted...)
				lister := fakeHookLister{keys: tc.live}
				results, err := runDoctorDiagnosis(staleDeps(dir, lister, hookStore, nil))
				if err != nil {
					t.Fatalf("runDoctorDiagnosis: %v", err)
				}
				got := findCheck(t, results, "stale hooks")

				persisted, err := hookStore.Load()
				if err != nil {
					t.Fatalf("load hooks: %v", err)
				}
				want := len(hooks.StaleKeys(persisted, tc.live))

				// The detail carries the count; assert it matches the predicate.
				wantDetail := "no stale hooks"
				wantStatus := checkPass
				if want > 0 {
					wantDetail = pluralCount(want, "stale hook entry", "stale hook entries")
					wantStatus = checkFail
				}
				if got.status != wantStatus {
					t.Errorf("status = %v, want %v (predicate count %d)", got.status, wantStatus, want)
				}
				if got.detail != wantDetail {
					t.Errorf("detail = %q, want %q (predicate count %d)", got.detail, wantDetail, want)
				}
			})
		}
	})

	t.Run("hazard-guard paths map to not-evaluable or pass with no prune", func(t *testing.T) {
		cases := []struct {
			name       string
			persisted  []string
			lister     fakeHookLister
			wantStatus checkStatus
			wantDetail string
		}{
			{"enumeration error", []string{"sessA:0.0"}, fakeHookLister{err: errors.New("tmux transient")}, checkNotEvaluable, "could not enumerate live panes"},
			{"empty live with hooks present", []string{"sessA:0.0"}, fakeHookLister{keys: []string{}}, checkNotEvaluable, "zero live panes with hooks present (not evaluable)"},
			{"empty live with no hooks", nil, fakeHookLister{keys: []string{}}, checkPass, "no hooks"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				dir := t.TempDir()
				hookStore, hooksPath := seedHooksJSON(t, tc.persisted...)
				before, err := os.ReadFile(hooksPath)
				if err != nil {
					t.Fatalf("read hooks.json: %v", err)
				}
				results, err := runDoctorDiagnosis(staleDeps(dir, tc.lister, hookStore, nil))
				if err != nil {
					t.Fatalf("runDoctorDiagnosis: %v", err)
				}
				got := findCheck(t, results, "stale hooks")
				if got.status != tc.wantStatus {
					t.Errorf("status = %v, want %v", got.status, tc.wantStatus)
				}
				if got.detail != tc.wantDetail {
					t.Errorf("detail = %q, want %q", got.detail, tc.wantDetail)
				}
				after, err := os.ReadFile(hooksPath)
				if err != nil {
					t.Fatalf("re-read hooks.json: %v", err)
				}
				if !bytes.Equal(before, after) {
					t.Errorf("hooks.json mutated by diagnosis (read-only violated)")
				}
			})
		}
	})
}

// TestDoctorStaleProjectsParityWithPredicate proves checkStaleProjects derives
// its stale count from the same project.Store.StaleEntries predicate the prune
// uses (present/missing fixtures) and stays not-evaluable on a load error.
func TestDoctorStaleProjectsParityWithPredicate(t *testing.T) {
	t.Run("count equals the shared predicate", func(t *testing.T) {
		dir := t.TempDir()
		liveDir := t.TempDir()
		goneA := filepath.Join(t.TempDir(), "gone-a")
		goneB := filepath.Join(t.TempDir(), "gone-b")
		projectStore, _ := seedProjectsJSON(t, liveDir, goneA, goneB)

		results, err := runDoctorDiagnosis(staleDeps(dir, fakeHookLister{}, nil, projectStore))
		if err != nil {
			t.Fatalf("runDoctorDiagnosis: %v", err)
		}
		got := findCheck(t, results, "stale projects")

		stale, err := projectStore.StaleEntries()
		if err != nil {
			t.Fatalf("StaleEntries: %v", err)
		}
		want := len(stale)
		if want != 2 {
			t.Fatalf("StaleEntries count = %d, want 2 (fixture sanity)", want)
		}
		if got.status != checkFail {
			t.Errorf("status = %v, want checkFail", got.status)
		}
		if got.detail != pluralCount(want, "stale project", "stale projects") {
			t.Errorf("detail = %q, want %q", got.detail, pluralCount(want, "stale project", "stale projects"))
		}
	})

	t.Run("load error is not-evaluable", func(t *testing.T) {
		dir := t.TempDir()
		// projects.json inside a 0000 dir so Load fails with a non-ErrNotExist
		// (permission) error — the not-evaluable path.
		unreadableDir := filepath.Join(t.TempDir(), "noread")
		if err := os.Mkdir(unreadableDir, 0o000); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(unreadableDir, 0o700) })
		projectStore := project.NewStore(filepath.Join(unreadableDir, "projects.json"))

		results, err := runDoctorDiagnosis(staleDeps(dir, fakeHookLister{}, nil, projectStore))
		if err != nil {
			t.Fatalf("runDoctorDiagnosis: %v", err)
		}
		got := findCheck(t, results, "stale projects")
		if got.status != checkNotEvaluable {
			t.Errorf("status = %v, want checkNotEvaluable on a load error", got.status)
		}
		if got.detail != "could not read projects.json" {
			t.Errorf("detail = %q, want %q", got.detail, "could not read projects.json")
		}
	})
}

// runDoctorFixCmd executes "portal doctor --fix" with the supplied hermetic
// DoctorDeps, returning stdout, stderr, and the rootCmd.Execute error. Unlike
// runDoctor it does NOT force withHealthyRuntime — the caller wires exactly the
// seams the scenario needs (a down server, a stale-hook lister, temp-path
// stores) so no real tmux server or state dir is ever touched.
func runDoctorFixCmd(t *testing.T, deps *DoctorDeps) (*bytes.Buffer, *bytes.Buffer, error) {
	t.Helper()
	// resolveDoctorDeps eagerly builds the shared spawn seams (terminals.json read)
	// — isolate the file so the Execute path stays hermetic.
	isolateTerminalsFile(t)
	doctorDeps = deps
	t.Cleanup(func() { doctorDeps = nil })

	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	resetRootCmd()
	rootCmd.SetOut(outBuf)
	rootCmd.SetErr(errBuf)
	rootCmd.SetArgs([]string{"doctor", "--fix"})
	err := rootCmd.Execute()
	return outBuf, errBuf, err
}

// runDoctorCmd executes plain "portal doctor" (no --fix) with the supplied
// hermetic DoctorDeps, returning stdout, stderr, and the rootCmd.Execute error.
// Like runDoctorFixCmd it wires exactly the seams the scenario needs — no real
// tmux server or state dir is touched — but drives the read-only diagnosis path
// that ends in ErrDoctorUnhealthy rather than the repair path.
func runDoctorCmd(t *testing.T, deps *DoctorDeps) (*bytes.Buffer, *bytes.Buffer, error) {
	t.Helper()
	// resolveDoctorDeps eagerly builds the shared spawn seams (terminals.json read)
	// — isolate the file so the Execute path stays hermetic.
	isolateTerminalsFile(t)
	doctorDeps = deps
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

// TestDoctorExecuteStaleEntryReturnsUnhealthy Executes plain `portal doctor`
// (not --fix) over an otherwise-healthy runtime carrying a genuinely stale hook
// AND a stale project, and asserts the command returns ErrDoctorUnhealthy: the
// read-only diagnosis surfaces the stale state as a non-zero exit without
// repairing anything (only --fix prunes).
func TestDoctorExecuteStaleEntryReturnsUnhealthy(t *testing.T) {
	dir := t.TempDir()
	seedHealthyStateDir(t, dir)

	// A persisted hook key with no matching live pane — the live set is non-empty,
	// so the hazard guard does NOT defer — is genuinely stale.
	hookStore, _ := seedHooksJSON(t, "sessA:0.0")
	lister := fakeHookLister{keys: []string{"sessB:0.0"}}
	// A project whose directory no longer exists is genuinely stale.
	goneDir := filepath.Join(t.TempDir(), "gone")
	projectStore, _ := seedProjectsJSON(t, goneDir)

	outBuf, errBuf, err := runDoctorCmd(t, staleDeps(dir, lister, hookStore, projectStore))
	if err != ErrDoctorUnhealthy {
		t.Fatalf("Execute err = %v; want ErrDoctorUnhealthy on a stale hook/project over a healthy runtime", err)
	}
	if errBuf.Len() != 0 {
		t.Errorf("expected silent stderr on unhealthy exit; got %q", errBuf.String())
	}

	out := outBuf.String()
	if !strings.Contains(out, "stale hooks: 1 stale hook entry") {
		t.Errorf("report missing stale-hooks fail line:\n%s", out)
	}
	if !strings.Contains(out, "stale projects: 1 stale project") {
		t.Errorf("report missing stale-projects fail line:\n%s", out)
	}
	// Plain doctor is read-only: exactly one report renders (no post-repair pass).
	if n := strings.Count(out, "Portal doctor:"); n != 1 {
		t.Errorf("report count = %d; want 1 (plain doctor renders once, no --fix re-diagnosis):\n%s", n, out)
	}
}

// TestDoctorFixPrunesStaleEntriesThenRediagnosesClean is the happy path: a stale
// hook and a stale project are seeded over an otherwise-healthy runtime; after
// `--fix` both are pruned from disk, the post-repair diagnosis reports them
// clean, and the command exits 0.
func TestDoctorFixPrunesStaleEntriesThenRediagnosesClean(t *testing.T) {
	dir := t.TempDir()
	seedHealthyStateDir(t, dir)

	hookStore, hooksPath := seedHooksJSON(t, "sessA:0.0")
	liveDir := t.TempDir()
	goneDir := filepath.Join(t.TempDir(), "gone")
	projectStore, projectsPath := seedProjectsJSON(t, liveDir, goneDir)

	// A live-pane set that excludes sessA:0.0 makes it stale (prunable); the
	// non-empty set means the hazard guard does NOT defer.
	lister := fakeHookLister{keys: []string{"sessB:0.0"}}

	outBuf, _, err := runDoctorFixCmd(t, staleDeps(dir, lister, hookStore, projectStore))
	if err != nil {
		t.Fatalf("Execute err = %v; want nil (healthy post-repair)", err)
	}

	hooksAfter, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("read hooks.json: %v", err)
	}
	if strings.Contains(string(hooksAfter), "sessA:0.0") {
		t.Errorf("stale hook sessA:0.0 not pruned from hooks.json:\n%s", hooksAfter)
	}

	projectsAfter, err := os.ReadFile(projectsPath)
	if err != nil {
		t.Fatalf("read projects.json: %v", err)
	}
	if strings.Contains(string(projectsAfter), goneDir) {
		t.Errorf("stale project %q not pruned from projects.json:\n%s", goneDir, projectsAfter)
	}
	if !strings.Contains(string(projectsAfter), liveDir) {
		t.Errorf("live project %q wrongly pruned:\n%s", liveDir, projectsAfter)
	}

	out := outBuf.String()
	if !strings.Contains(out, "Pruned stale hook: sessA:0.0") {
		t.Errorf("missing pruned-hook breadcrumb:\n%s", out)
	}
	if !strings.Contains(out, "Pruned stale project: proj1 ("+goneDir+")") {
		t.Errorf("missing pruned-project breadcrumb:\n%s", out)
	}
	// The initial (pre-fix) report AND the post-repair report both render.
	if n := strings.Count(out, "Portal doctor:"); n != 2 {
		t.Errorf("report count = %d; want 2 (initial + post-repair):\n%s", n, out)
	}
	// Post-repair the two stale checks read clean.
	if !strings.Contains(out, "stale hooks: no stale hooks") {
		t.Errorf("post-repair stale-hooks check not clean:\n%s", out)
	}
	if !strings.Contains(out, "stale projects: no stale projects") {
		t.Errorf("post-repair stale-projects check not clean:\n%s", out)
	}
}

// TestDoctorFixProtectsUserHooksWhenLiveSetEmptyOrErrored proves the down-server
// data-loss safety: when live-pane enumeration is empty OR errored (the
// down/rebooted-server state), `--fix` prunes NO hooks — user-authored,
// non-reconstructable on-resume commands survive byte-for-byte. The protection
// is the runHookStaleCleanup hazard guard, not a bespoke doctor branch.
func TestDoctorFixProtectsUserHooksWhenLiveSetEmptyOrErrored(t *testing.T) {
	cases := []struct {
		name   string
		lister fakeHookLister
	}{
		{"empty live set", fakeHookLister{keys: []string{}}},
		{"enumeration error", fakeHookLister{err: errors.New("tmux transient")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			seedHealthyStateDir(t, dir)
			hookStore, hooksPath := seedHooksJSON(t, "sessA:0.0", "sessB:0.0")
			before, err := os.ReadFile(hooksPath)
			if err != nil {
				t.Fatalf("read hooks.json: %v", err)
			}

			if _, _, err := runDoctorFixCmd(t, staleDeps(dir, tc.lister, hookStore, nil)); err != nil {
				t.Fatalf("Execute err = %v; want nil (healthy runtime, hooks deferred)", err)
			}

			after, err := os.ReadFile(hooksPath)
			if err != nil {
				t.Fatalf("re-read hooks.json: %v", err)
			}
			if !bytes.Equal(before, after) {
				t.Errorf("hooks.json mutated on an empty/errored live set (user commands must survive)\nbefore: %s\nafter:  %s", before, after)
			}
		})
	}
}

// TestDoctorFixDownServerPrunesProjectsButNotHooks proves the split behaviour on
// a down server: the filesystem-only stale-project prune STILL runs (gone dir
// removed), the stale-hook prune does NOT (hazard guard defers), and the
// post-repair exit is driven by the re-diagnosis — still non-zero because the
// daemon / saver / hooks checks remain failed on a down server.
func TestDoctorFixDownServerPrunesProjectsButNotHooks(t *testing.T) {
	dir := t.TempDir()
	hookStore, hooksPath := seedHooksJSON(t, "sessA:0.0")
	goneDir := filepath.Join(t.TempDir(), "gone")
	projectStore, projectsPath := seedProjectsJSON(t, goneDir)
	hooksBefore, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("read hooks.json: %v", err)
	}

	deps := &DoctorDeps{
		StateDir:      dir,
		ServerRunning: func() bool { return false },
		// A down server yields an empty live-pane enumeration.
		HookLister:   fakeHookLister{keys: []string{}},
		HookStore:    hookStore,
		ProjectStore: projectStore,
		Detector:     fakeTerminalDetector{},
		Resolve:      doctorUnsupportedResolve,
	}
	outBuf, _, execErr := runDoctorFixCmd(t, deps)
	if execErr != ErrDoctorUnhealthy {
		t.Fatalf("Execute err = %v; want ErrDoctorUnhealthy (server still down post-repair)", execErr)
	}

	hooksAfter, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("re-read hooks.json: %v", err)
	}
	if !bytes.Equal(hooksBefore, hooksAfter) {
		t.Errorf("hooks.json pruned on a down server (user commands must survive)\nbefore: %s\nafter:  %s", hooksBefore, hooksAfter)
	}

	projectsAfter, err := os.ReadFile(projectsPath)
	if err != nil {
		t.Fatalf("re-read projects.json: %v", err)
	}
	if strings.Contains(string(projectsAfter), goneDir) {
		t.Errorf("filesystem-only stale-project prune did not run on a down server:\n%s", projectsAfter)
	}
	if !strings.Contains(outBuf.String(), "Pruned stale project: proj0 ("+goneDir+")") {
		t.Errorf("missing pruned-project breadcrumb:\n%s", outBuf.String())
	}
}

// TestDoctorFixLogSweepNeverDrivesExit proves the log-sweep is an unconditional
// maintenance side-action OUTSIDE the diagnose→repair loop: it runs against the
// resolved state dir (deleting a stale rotated log seeded there) yet an
// otherwise-healthy post-repair state still exits 0 — a stale-log state can
// never make doctor non-zero.
func TestDoctorFixLogSweepNeverDrivesExit(t *testing.T) {
	dir := t.TempDir()
	seedHealthyStateDir(t, dir)

	// A rotated log dated well before today: the sweep (cutoff == today) deletes
	// it, which observably proves the sweep ran against deps.StateDir.
	staleLog := filepath.Join(dir, "portal.log.2000-01-01")
	if err := os.WriteFile(staleLog, []byte("old\n"), 0o600); err != nil {
		t.Fatalf("seed stale rotated log: %v", err)
	}

	// nil stores → no hook/project prune; the log-sweep is the ONLY repair action,
	// isolating its (non-)effect on the exit code. Stale checks report
	// not-evaluable (never fail), so the post-repair state is fully healthy.
	deps := withHealthyRuntime(&DoctorDeps{StateDir: dir})
	_, _, err := runDoctorFixCmd(t, deps)
	if err != nil {
		t.Fatalf("Execute err = %v; want nil — a log-sweep must never drive the exit code", err)
	}

	if _, statErr := os.Stat(staleLog); !os.IsNotExist(statErr) {
		t.Errorf("stale rotated log not swept (stat err = %v); log-sweep did not run against the state dir", statErr)
	}
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
		if got.detail != "1 stale project" {
			t.Errorf("detail = %q; want %q", got.detail, "1 stale project")
		}
	})

	t.Run("multiple stale projects use the plural count copy", func(t *testing.T) {
		dir := t.TempDir()
		goneA := filepath.Join(t.TempDir(), "gone-a")
		goneB := filepath.Join(t.TempDir(), "gone-b")
		projectStore, _ := seedProjectsJSON(t, goneA, goneB)
		results, err := runDoctorDiagnosis(staleDeps(dir, fakeHookLister{}, nil, projectStore))
		if err != nil {
			t.Fatalf("runDoctorDiagnosis: %v", err)
		}
		got := findCheck(t, results, "stale projects")
		if got.status != checkFail {
			t.Errorf("status = %v; want checkFail for gone-dir projects", got.status)
		}
		if got.detail != "2 stale projects" {
			t.Errorf("detail = %q; want %q", got.detail, "2 stale projects")
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
