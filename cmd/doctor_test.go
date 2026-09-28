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
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/hooksweep"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

func doctorUnsupportedResolve(spawn.Identity) (spawn.Adapter, spawn.Resolution) {
	return nil, spawn.ResolutionUnsupported
}

func seedLiveDaemonPID(t *testing.T, dir string) {
	t.Helper()
	pid := strconv.Itoa(os.Getpid())
	if err := os.WriteFile(state.DaemonPID(dir), []byte(pid+"\n"), 0o600); err != nil {
		t.Fatalf("write daemon.pid: %v", err)
	}
}

func seedDeadDaemonPID(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(state.DaemonPID(dir), []byte("999999\n"), 0o600); err != nil {
		t.Fatalf("write daemon.pid: %v", err)
	}
}

func seedDaemonVersion(t *testing.T, dir, version string) {
	t.Helper()
	if err := os.WriteFile(state.DaemonVersion(dir), []byte(version+"\n"), 0o600); err != nil {
		t.Fatalf("write daemon.version: %v", err)
	}
}

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

// cmd cannot import the managedEvents table at test time, so this key set
// stands in for the canonical event set.
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

// withHealthyRuntime fills any unset probe seam with a healthy result. It also
// stubs the host-terminal seams so no doctor test invokes a real detector,
// which would read the process tree and the live tmux server.
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
	if deps.PendingResumes == nil {
		deps.PendingResumes = func() (tmux.PendingResumeView, error) {
			return tmux.PendingResumeView{Rows: []tmux.PendingResumeRow{}, Sessions: map[string]struct{}{}}, nil
		}
	}
	if deps.Detector == nil {
		deps.Detector = fakeTerminalDetector{}
	}
	if deps.Resolve == nil {
		deps.Resolve = doctorUnsupportedResolve
	}
	return deps
}

func seedHealthyStateDir(t *testing.T, dir string) {
	t.Helper()
	seedLiveDaemonPID(t, dir)
	seedDaemonVersion(t, dir, "v9.9.9")
	seedValidSessionsJSON(t, dir, 1)
}

func runDoctor(t *testing.T, dir string) (*bytes.Buffer, *bytes.Buffer, error) {
	t.Helper()
	return runDoctorWith(t, withHealthyRuntime(&DoctorDeps{StateDir: dir}))
}

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
	// The stale-hooks seams keep their production defaults, whose live-pane
	// enumeration fails against TestMain's poisoned tmux socket and reports
	// not-evaluable — so that check falls outside the count below.
	if !strings.HasSuffix(out, "\n  6 checks passed\n") {
		t.Errorf("report does not close with the all-passed summary:\n%s", out)
	}
}

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
	dir := filepath.Join(t.TempDir(), "state")

	outBuf, _, err := runDoctor(t, dir)
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
	if !strings.HasSuffix(out, "\n  5 of 6 checks passed\n") {
		t.Errorf("report does not close with the partial summary:\n%s", out)
	}
}

func TestDoctorIsReadOnly(t *testing.T) {
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

func TestDoctorStateDirSaneFailBranches(t *testing.T) {
	t.Run("existing path that is not a directory fails", func(t *testing.T) {
		// A regular file at the state-dir path: os.Stat succeeds, IsDir does not.
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
		// os.Stat of a child inside a 0o000 directory returns EACCES, not
		// ErrNotExist.
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

	if _, _, err := runDoctorWith(t, &DoctorDeps{StateDir: dir}, "unexpected"); err == nil {
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

// Duplicated rather than imported, so a drift in the production string is a
// failure rather than a silent agreement.
const doctorRuntimeNotRunningDetail = "Portal runtime not running — run portal open to start"

func TestDoctorServerDownReportsRuntimeNotRunning(t *testing.T) {
	dir := t.TempDir()
	seedHealthyStateDir(t, dir)

	deps := &DoctorDeps{
		StateDir:      dir,
		ServerRunning: func() bool { return false },
		// Healthy probe returns: were the down-server gate bypassed these would
		// produce PASS details and the assertions below would fail loudly.
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

	if !doctorUnhealthy(results) {
		t.Error("doctorUnhealthy = false; want true for a down server")
	}

	if got := findCheck(t, results, "state dir"); got.status != checkPass {
		t.Errorf("state dir status = %v; want checkPass (server-independent)", got.status)
	}
	if got := findCheck(t, results, "sessions.json"); got.status != checkPass {
		t.Errorf("sessions.json status = %v; want checkPass (server-independent)", got.status)
	}
}

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
		return withHealthyRuntime(&DoctorDeps{
			StateDir:      dir,
			ServerRunning: func() bool { return true },
			SaverPresent:  func() (bool, error) { return true, nil },
			HookCounts:    func() (map[string]int, error) { return counts, nil },
		})
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
		deps := withHealthyRuntime(&DoctorDeps{
			StateDir:      dir,
			ServerRunning: func() bool { return true },
			SaverPresent:  func() (bool, error) { return true, nil },
			HookCounts:    func() (map[string]int, error) { return nil, errors.New("tmux transient") },
		})
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
		return withHealthyRuntime(&DoctorDeps{
			StateDir:      dir,
			ServerRunning: func() bool { return true },
			SaverPresent:  func() (bool, error) { return present, saverErr },
			HookCounts:    func() (map[string]int, error) { return allHooksHealthy(), nil },
		})
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
		// Resolve returns Native, which the NULL short-circuit must ignore.
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

func TestDoctorHostTerminalNeverDrivesExit(t *testing.T) {
	t.Run("unsupported host with a healthy runtime stays healthy", func(t *testing.T) {
		dir := t.TempDir()
		seedHealthyStateDir(t, dir)
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

func TestDoctorCheckOrder(t *testing.T) {
	dir := t.TempDir()
	seedHealthyStateDir(t, dir)
	results, err := runDoctorDiagnosis(withHealthyRuntime(&DoctorDeps{StateDir: dir}))
	if err != nil {
		t.Fatalf("runDoctorDiagnosis: %v", err)
	}
	want := []string{"daemon", "saver", "hooks", "state dir", "sessions.json", "stale hooks", "stale projects", "host terminal", "pending resumes"}
	if len(results) != len(want) {
		t.Fatalf("check count = %d, want %d: %+v", len(results), len(want), results)
	}
	for i, name := range want {
		if results[i].name != name {
			t.Errorf("results[%d].name = %q, want %q", i, results[i].name, name)
		}
	}
}

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

func staleDeps(dir string, lister hooksweep.Reader, hookStore *hooks.Store, projectStore *project.Store) *DoctorDeps {
	return withHealthyRuntime(&DoctorDeps{
		StateDir:     dir,
		HookLister:   lister,
		HookStore:    hookStore,
		ProjectStore: projectStore,
	})
}

// seedStalePruneFixture stages a healthy install whose only anomalies are one
// token-shaped hook entry and one stale project record, read through the given
// live-pane enumeration. staleHookLister makes the entry genuinely stale; the
// stand-down listers make the same fixture describe a prune that must not run.
func seedStalePruneFixture(t *testing.T, stateDir string, lister *stubStaleSweepReader) (deps *DoctorDeps, hooksPath, projectsPath, liveDir, goneDir string) {
	t.Helper()

	seedHealthyStateDir(t, stateDir)
	hookStore, hooksPath := hookstest.StageStore(t, hookstest.Staging{Seed: hooksBody(hookstest.ReapableSeedA)})
	liveDir = t.TempDir()
	goneDir = filepath.Join(t.TempDir(), "gone")
	projectStore, projectsPath := seedProjectsJSON(t, liveDir, goneDir)

	return staleDeps(stateDir, lister, hookStore, projectStore), hooksPath, projectsPath, liveDir, goneDir
}

// staleHookLister excludes the seeded key from the live set, which makes it
// stale, and its token shape makes it judgeable; the set's non-emptiness keeps
// the hazard guard from deferring.
func staleHookLister() *stubStaleSweepReader {
	return &stubStaleSweepReader{rows: tokenRows(hookstest.LiveSeedB)}
}

// restoringHookLister reads the same live set through a set @portal-restoring
// marker, so the sweep stands down before it can judge anything.
func restoringHookLister() *stubStaleSweepReader {
	lister := staleHookLister()
	lister.restoring = true
	return lister
}

func assertStalePrunesApplied(t *testing.T, hooksPath, projectsPath, liveDir, goneDir, out string) {
	t.Helper()

	hooksAfter := readFileBytes(t, hooksPath)
	if len(hooksAfter) == 0 {
		t.Fatalf("hooks.json is absent or empty after the prune — the prune removed the file rather than the stale entry")
	}
	if strings.Contains(string(hooksAfter), hookstest.ReapableSeedA) {
		t.Errorf("stale hook %s not pruned from hooks.json:\n%s", hookstest.ReapableSeedA, hooksAfter)
	}

	projectsAfter := readFileBytes(t, projectsPath)
	if len(projectsAfter) == 0 {
		t.Fatalf("projects.json is absent or empty after the prune — the prune removed the file rather than the stale record")
	}
	if strings.Contains(string(projectsAfter), goneDir) {
		t.Errorf("stale project %q not pruned from projects.json:\n%s", goneDir, projectsAfter)
	}
	if !strings.Contains(string(projectsAfter), liveDir) {
		t.Errorf("live project %q wrongly pruned:\n%s", liveDir, projectsAfter)
	}

	var prunedHookLines []string
	for line := range strings.SplitSeq(out, "\n") {
		if strings.HasPrefix(line, "Pruned stale hook:") {
			prunedHookLines = append(prunedHookLines, line)
		}
	}
	wantHookLine := "Pruned stale hook: " + hookstest.ReapableSeedA
	if len(prunedHookLines) != 1 || prunedHookLines[0] != wantHookLine {
		t.Errorf("pruned-hook stdout lines = %q, want exactly [%q]\n%s", prunedHookLines, wantHookLine, out)
	}
	if !strings.Contains(out, "Pruned stale project: proj1 ("+goneDir+")") {
		t.Errorf("missing pruned-project breadcrumb:\n%s", out)
	}
}

// downServerDeferFixture wires a down server, whose empty live-pane
// enumeration is indistinguishable from "every pane is gone" - so the
// mass-deletion hazard guard defers the stale-hook prune. The filesystem-only
// project prune has no such ambiguity and still runs.
func downServerDeferFixture(t *testing.T, stateDir string) (deps *DoctorDeps, hooksPath, projectsPath, goneDir string) {
	t.Helper()

	// Token-shaped, so it is the hazard guard rather than the shape rule that
	// spares the entry.
	hookStore, hooksPath := hookstest.StageStore(t, hookstest.Staging{Seed: hooksBody(hookstest.ReapableSeedA)})
	goneDir = filepath.Join(t.TempDir(), "gone")
	projectStore, projectsPath := seedProjectsJSON(t, goneDir)

	deps = &DoctorDeps{
		StateDir:      stateDir,
		ServerRunning: func() bool { return false },
		HookLister:    &stubStaleSweepReader{rows: tokenRows()},
		HookStore:     hookStore,
		ProjectStore:  projectStore,
		Detector:      fakeTerminalDetector{},
		Resolve:       doctorUnsupportedResolve,
	}
	return deps, hooksPath, projectsPath, goneDir
}

func assertDownServerDeferral(t *testing.T, hooksBefore []byte, hooksPath, projectsPath, goneDir string, err error) {
	t.Helper()

	if err != ErrDoctorUnhealthy {
		t.Fatalf("Execute err = %v; want ErrDoctorUnhealthy (server still down post-repair)", err)
	}

	assertHooksFileUnchanged(t, hooksPath, hooksBefore, "pruned on a down server (user commands must survive)")

	projectsAfter := readFileBytes(t, projectsPath)
	if len(projectsAfter) == 0 {
		t.Fatalf("projects.json is absent or empty after the prune — the prune removed the file rather than the stale record")
	}
	if strings.Contains(string(projectsAfter), goneDir) {
		t.Errorf("the filesystem-only stale-project prune did not run on a down server:\n%s", projectsAfter)
	}
}

func TestDoctorStaleHooksCheck(t *testing.T) {
	t.Run("persisted key with no live pane fails", func(t *testing.T) {
		dir := t.TempDir()
		hookStore, _ := hookstest.StageStore(t, hookstest.Staging{Seed: hooksBody(hookstest.ReapableSeedA)})
		lister := &stubStaleSweepReader{rows: tokenRows(hookstest.LiveSeedB)}
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
		hookStore, _ := hookstest.StageStore(t, hookstest.Staging{Seed: hooksBody(hookstest.ReapableSeedA, hookstest.ReapableSeedB)})
		lister := &stubStaleSweepReader{rows: tokenRows(hookstest.LiveSeedC)}
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
		hookStore, _ := hookstest.StageStore(t, hookstest.Staging{Seed: hooksBody(hookstest.ReapableSeedA, hookstest.ReapableSeedB)})
		lister := &stubStaleSweepReader{rows: tokenRows()}
		results, err := runDoctorDiagnosis(staleDeps(dir, lister, hookStore, nil))
		if err != nil {
			t.Fatalf("runDoctorDiagnosis: %v", err)
		}
		got := findCheck(t, results, "stale hooks")
		if got.status != checkNotEvaluable {
			t.Errorf("status = %v; want checkNotEvaluable (never a mass-stale failure)", got.status)
		}
		if want := phraseFor(notEvaluableDetails, hooksweep.ReasonEmptyPaneRead); got.detail != want {
			t.Errorf("detail = %q; want %q", got.detail, want)
		}
	})

	// Zero stamped panes is the ordinary install until registration starts
	// writing tokens, so the check must evaluate on the rows it was handed
	// rather than stand down on an empty token set.
	t.Run("it evaluates when rows are present and no pane is stamped", func(t *testing.T) {
		dir := t.TempDir()
		hookStore, _ := hookstest.StageStore(t, hookstest.Staging{Seed: hooksBody(hookstest.ReapableSeedA)})
		lister := &stubStaleSweepReader{rows: unstampedRows(3)}
		results, err := runDoctorDiagnosis(staleDeps(dir, lister, hookStore, nil))
		if err != nil {
			t.Fatalf("runDoctorDiagnosis: %v", err)
		}
		got := findCheck(t, results, "stale hooks")
		if got.status != checkFail {
			t.Errorf("status = %v; want checkFail (the pane rows make the count evaluable)", got.status)
		}
		if got.detail != "1 stale hook entry" {
			t.Errorf("detail = %q; want %q", got.detail, "1 stale hook entry")
		}
	})

	t.Run("it does not reach the no-hooks door with unstamped rows present", func(t *testing.T) {
		dir := t.TempDir()
		hookStore, _ := hookstest.StageStore(t, hookstest.Staging{Seed: hooksBody()})
		lister := &stubStaleSweepReader{rows: unstampedRows(3)}
		results, err := runDoctorDiagnosis(staleDeps(dir, lister, hookStore, nil))
		if err != nil {
			t.Fatalf("runDoctorDiagnosis: %v", err)
		}
		got := findCheck(t, results, "stale hooks")
		if got.status != checkPass {
			t.Errorf("status = %v; want checkPass", got.status)
		}
		if got.detail != "no stale hooks" {
			t.Errorf("detail = %q; want %q (the zero-row branch must not swallow a live server)", got.detail, "no stale hooks")
		}
	})

	t.Run("live-pane enumeration error is not-evaluable", func(t *testing.T) {
		dir := t.TempDir()
		hookStore, _ := hookstest.StageStore(t, hookstest.Staging{Seed: hooksBody(hookstest.ReapableSeedA)})
		lister := &stubStaleSweepReader{err: errors.New("tmux transient")}
		results, err := runDoctorDiagnosis(staleDeps(dir, lister, hookStore, nil))
		if err != nil {
			t.Fatalf("runDoctorDiagnosis: %v", err)
		}
		got := findCheck(t, results, "stale hooks")
		if got.status != checkNotEvaluable {
			t.Errorf("status = %v; want checkNotEvaluable on an enumeration error", got.status)
		}
		if want := phraseFor(notEvaluableDetails, hooksweep.ReasonPaneReadFailed); got.detail != want {
			t.Errorf("detail = %q; want %q", got.detail, want)
		}
	})

	t.Run("both empty passes as no hooks", func(t *testing.T) {
		dir := t.TempDir()
		hookStore, _ := hookstest.StageStore(t, hookstest.Staging{Seed: hooksBody()})
		lister := &stubStaleSweepReader{rows: tokenRows()}
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
		hookStore, _ := hookstest.StageStore(t, hookstest.Staging{Seed: hooksBody(hookstest.LiveSeedA, hookstest.LiveSeedB)})
		lister := &stubStaleSweepReader{rows: tokenRows(hookstest.LiveSeedA, hookstest.LiveSeedB, hookstest.LiveSeedC)}
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

	// The restore window: a skeleton's panes carry no token yet, so a count taken
	// here would report every token-shaped key on the machine as lost.
	t.Run("it reports not evaluable while the restore marker is set", func(t *testing.T) {
		dir := t.TempDir()
		hookStore, _ := hookstest.StageStore(t, hookstest.Staging{Seed: hooksBody(hookstest.ReapableSeedA)})
		lister := &stubStaleSweepReader{rows: tokenRows(hookstest.LiveSeedB), restoring: true}
		got := staleHooksCheckResult(t, dir, lister, hookStore)
		assertRestoreWindowResult(t, got)
	})

	// A read that failed stands the check down like a set marker, but says so
	// in its own words: it establishes nothing about the marker.
	t.Run("it reports a failed marker read as its own reason", func(t *testing.T) {
		dir := t.TempDir()
		hookStore, _ := hookstest.StageStore(t, hookstest.Staging{Seed: hooksBody(hookstest.ReapableSeedA)})
		lister := &stubStaleSweepReader{rows: tokenRows(hookstest.LiveSeedB), restoringErr: errors.New("tmux transient")}
		got := staleHooksCheckResult(t, dir, lister, hookStore)
		assertMarkerReadFailedResult(t, got)
		if lister.calls != 0 {
			t.Errorf("live pane enumerations = %d; want 0 (the failed marker read stands the check down first)", lister.calls)
		}
	})

	t.Run("it reads the marker before the empty-live-set branch", func(t *testing.T) {
		dir := t.TempDir()
		hookStore, _ := hookstest.StageStore(t, hookstest.Staging{Seed: hooksBody(hookstest.ReapableSeedA, hookstest.ReapableSeedB)})
		lister := &stubStaleSweepReader{rows: tokenRows(), restoring: true}
		got := staleHooksCheckResult(t, dir, lister, hookStore)
		assertRestoreWindowResult(t, got)
	})

	// The live enumeration the count is taken from must not run at all: the
	// rendered result alone cannot tell a marker read that came first from one
	// that merely overrode a count already computed.
	t.Run("it reads the marker before counting", func(t *testing.T) {
		dir := t.TempDir()
		hookStore, _ := hookstest.StageStore(t, hookstest.Staging{Seed: hooksBody(hookstest.ReapableSeedA)})
		lister := &stubStaleSweepReader{rows: tokenRows(hookstest.LiveSeedB), restoring: true}
		got := staleHooksCheckResult(t, dir, lister, hookStore)
		assertRestoreWindowResult(t, got)
		if lister.calls != 0 {
			t.Errorf("live pane enumerations = %d; want 0 (the marker read stands the check down first)", lister.calls)
		}
	})

	t.Run("it reports not evaluable with no server running", func(t *testing.T) {
		dir := t.TempDir()
		hookStore, _ := hookstest.StageStore(t, hookstest.Staging{Seed: hooksBody(hookstest.ReapableSeedA)})
		down := errors.New("no server running on /tmp/tmux-501/default")
		deps := staleDeps(dir, &stubStaleSweepReader{err: down, restoringErr: down}, hookStore, nil)
		deps.ServerRunning = func() bool { return false }

		results, err := runDoctorDiagnosis(deps)
		if err != nil {
			t.Fatalf("runDoctorDiagnosis: %v", err)
		}
		assertMarkerReadFailedResult(t, findCheck(t, results, "stale hooks"))
	})

	t.Run("it still fails on a genuinely stale token-shaped key alongside retained non-token-shaped entries", func(t *testing.T) {
		dir := t.TempDir()
		hookStore, _ := hookstest.StageStore(t, hookstest.Staging{Seed: hooksBody(hookstest.ReapableSeedA, hookstest.UnjudgeableSeedA, hookstest.UnjudgeableSeedB)})
		lister := &stubStaleSweepReader{rows: tokenRows(hookstest.LiveSeedB)}
		got := staleHooksCheckResult(t, dir, lister, hookStore)
		if got.status != checkFail {
			t.Errorf("status = %v; want checkFail for the one stale token-shaped key", got.status)
		}
		if got.detail != "1 stale hook entry" {
			t.Errorf("detail = %q; want %q (retained old-format keys are not counted)", got.detail, "1 stale hook entry")
		}
	})

	t.Run("it keeps portal doctor at exit 0 in a restore window", func(t *testing.T) {
		dir := t.TempDir()
		seedHealthyStateDir(t, dir)
		hookStore, _ := hookstest.StageStore(t, hookstest.Staging{Seed: hooksBody(hookstest.ReapableSeedA)})
		projectStore, _ := seedProjectsJSON(t, t.TempDir())
		lister := &stubStaleSweepReader{rows: tokenRows(hookstest.LiveSeedB), restoring: true}

		outBuf, _, err := runDoctorWith(t, staleDeps(dir, lister, hookStore, projectStore))
		if err != nil {
			t.Fatalf("Execute err = %v; want nil (not-evaluable never drives the exit code)\n%s", err, outBuf.String())
		}
		if want := renderStaleHooksLine(hooksweep.ReasonRestoring); !strings.Contains(outBuf.String(), want) {
			t.Errorf("report missing %q:\n%s", want, outBuf.String())
		}
	})
}

// staleHooksCheckResult runs the read-only diagnosis and returns its stale-hooks
// line.
func staleHooksCheckResult(t *testing.T, dir string, lister hooksweep.Reader, hookStore *hooks.Store) checkResult {
	t.Helper()
	results, err := runDoctorDiagnosis(staleDeps(dir, lister, hookStore, nil))
	if err != nil {
		t.Fatalf("runDoctorDiagnosis: %v", err)
	}
	return findCheck(t, results, "stale hooks")
}

func assertRestoreWindowResult(t *testing.T, got checkResult) {
	t.Helper()
	if got.status != checkNotEvaluable {
		t.Errorf("status = %v; want checkNotEvaluable in a restore window", got.status)
	}
	if want := phraseFor(notEvaluableDetails, hooksweep.ReasonRestoring); got.detail != want {
		t.Errorf("detail = %q; want %q", got.detail, want)
	}
}

// A server that is down is the routine state a user diagnoses in, so the
// detail must name the read rather than assert a restore nothing is running.
func assertMarkerReadFailedResult(t *testing.T, got checkResult) {
	t.Helper()
	if got.status != checkNotEvaluable {
		t.Errorf("status = %v; want checkNotEvaluable on a failed marker read", got.status)
	}
	if want := phraseFor(notEvaluableDetails, hooksweep.ReasonMarkerReadFailed); got.detail != want {
		t.Errorf("detail = %q; want %q", got.detail, want)
	}
}

func TestDoctorStaleHooksParityWithPredicate(t *testing.T) {
	t.Run("past-guard count equals the shared predicate", func(t *testing.T) {
		cases := []struct {
			name      string
			persisted []string
			live      []string
		}{
			{"one stale", []string{hookstest.ReapableSeedA}, []string{hookstest.LiveSeedA}},
			{"two stale", []string{hookstest.ReapableSeedA, hookstest.ReapableSeedB}, []string{hookstest.LiveSeedC}},
			{"one of three stale", []string{hookstest.LiveSeedA, hookstest.LiveSeedB, hookstest.ReapableSeedC}, []string{hookstest.LiveSeedA, hookstest.LiveSeedB}},
			{"none stale", []string{hookstest.LiveSeedA}, []string{hookstest.LiveSeedA, hookstest.LiveSeedB}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				dir := t.TempDir()
				hookStore, _ := hookstest.StageStore(t, hookstest.Staging{Seed: hooksBody(tc.persisted...)})
				lister := &stubStaleSweepReader{rows: tokenRows(tc.live...)}
				results, err := runDoctorDiagnosis(staleDeps(dir, lister, hookStore, nil))
				if err != nil {
					t.Fatalf("runDoctorDiagnosis: %v", err)
				}
				got := findCheck(t, results, "stale hooks")

				persisted, err := hookStore.Load(hooks.ViaInternal)
				if err != nil {
					t.Fatalf("load hooks: %v", err)
				}
				want := len(hooks.StaleKeys(persisted, tc.live))

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
			lister     *stubStaleSweepReader
			wantStatus checkStatus
			wantDetail string
		}{
			{"enumeration error", []string{hookstest.ReapableSeedA}, &stubStaleSweepReader{err: errors.New("tmux transient")}, checkNotEvaluable, phraseFor(notEvaluableDetails, hooksweep.ReasonPaneReadFailed)},
			{"empty live with hooks present", []string{hookstest.ReapableSeedA}, &stubStaleSweepReader{rows: tokenRows()}, checkNotEvaluable, phraseFor(notEvaluableDetails, hooksweep.ReasonEmptyPaneRead)},
			{"empty live with no hooks", nil, &stubStaleSweepReader{rows: tokenRows()}, checkPass, "no hooks"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				dir := t.TempDir()
				hookStore, hooksPath := hookstest.StageStore(t, hookstest.Staging{Seed: hooksBody(tc.persisted...)})
				before := readFileBytes(t, hooksPath)
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
				assertHooksFileUnchanged(t, hooksPath, before, "mutated by diagnosis (read-only violated)")
			})
		}
	})
}

func TestDoctorStaleProjectsParityWithPredicate(t *testing.T) {
	t.Run("count equals the shared predicate", func(t *testing.T) {
		dir := t.TempDir()
		liveDir := t.TempDir()
		goneA := filepath.Join(t.TempDir(), "gone-a")
		goneB := filepath.Join(t.TempDir(), "gone-b")
		projectStore, _ := seedProjectsJSON(t, liveDir, goneA, goneB)

		results, err := runDoctorDiagnosis(staleDeps(dir, &stubStaleSweepReader{}, nil, projectStore))
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
		// projects.json inside a 0o000 dir so Load fails with a permission error
		// rather than ErrNotExist.
		unreadableDir := filepath.Join(t.TempDir(), "noread")
		if err := os.Mkdir(unreadableDir, 0o000); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(unreadableDir, 0o700) })
		projectStore := project.NewStore(filepath.Join(unreadableDir, "projects.json"))

		results, err := runDoctorDiagnosis(staleDeps(dir, &stubStaleSweepReader{}, nil, projectStore))
		if err != nil {
			t.Fatalf("runDoctorDiagnosis: %v", err)
		}
		got := findCheck(t, results, "stale projects")
		if got.status != checkNotEvaluable {
			t.Errorf("status = %v, want checkNotEvaluable on a load error", got.status)
		}
		if got.detail != projectStoreReadStandDownPhrase {
			t.Errorf("detail = %q, want %q", got.detail, projectStoreReadStandDownPhrase)
		}
	})
}

// runDoctorWith executes `portal doctor` with the given args against caller-wired
// seams, returning its stdout and stderr. Unlike runDoctor, the caller wires
// exactly the seams the scenario needs.
func runDoctorWith(t *testing.T, deps *DoctorDeps, args ...string) (*bytes.Buffer, *bytes.Buffer, error) {
	t.Helper()
	// terminals.json is read eagerly when the deps resolve; isolate it so
	// Execute never touches the developer's real config file.
	isolateTerminalsFile(t)
	withDoctorDeps(t, *deps)

	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	resetRootCmd()
	rootCmd.SetOut(outBuf)
	rootCmd.SetErr(errBuf)
	rootCmd.SetArgs(append([]string{"doctor"}, args...))
	err := rootCmd.Execute()
	return outBuf, errBuf, err
}

func TestDoctorExecuteStaleEntryReturnsUnhealthy(t *testing.T) {
	dir := t.TempDir()
	seedHealthyStateDir(t, dir)

	// A persisted hook key with no matching live pane, against a non-empty live
	// set, is genuinely stale.
	hookStore, _ := hookstest.StageStore(t, hookstest.Staging{Seed: hooksBody(hookstest.ReapableSeedA)})
	lister := &stubStaleSweepReader{rows: tokenRows(hookstest.LiveSeedB)}
	goneDir := filepath.Join(t.TempDir(), "gone")
	projectStore, _ := seedProjectsJSON(t, goneDir)

	outBuf, errBuf, err := runDoctorWith(t, staleDeps(dir, lister, hookStore, projectStore))
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
	if n := strings.Count(out, "Portal doctor:"); n != 1 {
		t.Errorf("report count = %d; want 1 (plain doctor renders once, no --fix re-diagnosis):\n%s", n, out)
	}
	if n := strings.Count(out, " checks passed\n"); n != 1 {
		t.Errorf("summary count = %d; want 1 (one per report render):\n%s", n, out)
	}
	if !strings.HasSuffix(out, "\n  5 of 7 checks passed\n") {
		t.Errorf("report does not close with the partial summary:\n%s", out)
	}
}

func TestDoctorFixPrunesStaleEntriesThenRediagnosesClean(t *testing.T) {
	deps, hooksPath, projectsPath, liveDir, goneDir := seedStalePruneFixture(t, t.TempDir(), staleHookLister())

	outBuf, _, err := runDoctorWith(t, deps, "--fix")
	if err != nil {
		t.Fatalf("Execute err = %v; want nil (healthy post-repair)", err)
	}

	out := outBuf.String()
	assertStalePrunesApplied(t, hooksPath, projectsPath, liveDir, goneDir, out)

	if n := strings.Count(out, "Portal doctor:"); n != 2 {
		t.Errorf("report count = %d; want 2 (initial + post-repair):\n%s", n, out)
	}
	if !strings.Contains(out, "stale hooks: no stale hooks") {
		t.Errorf("post-repair stale-hooks check not clean:\n%s", out)
	}
	if !strings.Contains(out, "stale projects: no stale projects") {
		t.Errorf("post-repair stale-projects check not clean:\n%s", out)
	}
	if n := strings.Count(out, " checks passed\n"); n != 2 {
		t.Errorf("summary count = %d; want 2 (one per report render):\n%s", n, out)
	}
	if !strings.HasSuffix(out, "\n  7 checks passed\n") {
		t.Errorf("post-repair report does not close with the all-passed summary:\n%s", out)
	}
}

func TestDoctorFixProtectsUserHooksWhenLiveSetEmptyOrErrored(t *testing.T) {
	cases := []struct {
		name   string
		lister *stubStaleSweepReader
	}{
		{"empty live set", &stubStaleSweepReader{rows: tokenRows()}},
		{"enumeration error", &stubStaleSweepReader{err: errors.New("tmux transient")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			seedHealthyStateDir(t, dir)
			hookStore, hooksPath := hookstest.StageStore(t, hookstest.Staging{Seed: hooksBody(hookstest.ReapableSeedA, hookstest.ReapableSeedB)})
			before := readFileBytes(t, hooksPath)

			if _, _, err := runDoctorWith(t, staleDeps(dir, tc.lister, hookStore, nil), "--fix"); err != nil {
				t.Fatalf("Execute err = %v; want nil (healthy runtime, hooks deferred)", err)
			}

			assertHooksFileUnchanged(t, hooksPath, before, "mutated on an empty/errored live set (user commands must survive)")
		})
	}
}

func TestDoctorFixDownServerPrunesProjectsButNotHooks(t *testing.T) {
	deps, hooksPath, projectsPath, goneDir := downServerDeferFixture(t, t.TempDir())
	hooksBefore := readFileBytes(t, hooksPath)

	outBuf, _, execErr := runDoctorWith(t, deps, "--fix")
	assertDownServerDeferral(t, hooksBefore, hooksPath, projectsPath, goneDir, execErr)

	out := outBuf.String()
	if !strings.Contains(out, "Pruned stale project: proj0 ("+goneDir+")") {
		t.Errorf("missing pruned-project breadcrumb:\n%s", out)
	}
	if n := strings.Count(out, " checks passed\n"); n != 2 {
		t.Errorf("summary count = %d; want 2 (one per report render):\n%s", n, out)
	}
	if n := strings.Count(out, "\n  2 of 6 checks passed\n"); n != 1 {
		t.Errorf("pre-repair summary missing or repeated (want exactly one \"2 of 6 checks passed\"):\n%s", out)
	}
	if !strings.HasSuffix(out, "\n  3 of 6 checks passed\n") {
		t.Errorf("post-repair report does not close with the partial summary:\n%s", out)
	}
}

func TestDoctorFixLogSweepNeverDrivesExit(t *testing.T) {
	dir := t.TempDir()
	seedHealthyStateDir(t, dir)

	// A rotated log dated well before today: only a sweep running against
	// deps.StateDir would delete it.
	staleLog := filepath.Join(dir, "portal.log.2000-01-01")
	if err := os.WriteFile(staleLog, []byte("old\n"), 0o600); err != nil {
		t.Fatalf("seed stale rotated log: %v", err)
	}

	// nil stores leave the log-sweep as the only repair action, isolating its
	// effect on the exit code.
	deps := withHealthyRuntime(&DoctorDeps{StateDir: dir})
	outBuf, _, err := runDoctorWith(t, deps, "--fix")
	if err != nil {
		t.Fatalf("Execute err = %v; want nil — a log-sweep must never drive the exit code", err)
	}

	if _, statErr := os.Stat(staleLog); !os.IsNotExist(statErr) {
		t.Errorf("stale rotated log not swept (stat err = %v); log-sweep did not run against the state dir", statErr)
	}

	out := outBuf.String()
	if n := strings.Count(out, "\n  6 checks passed\n"); n != 2 {
		t.Errorf("all-passed summary count = %d; want 2 (one per report render, both unmoved by the sweep):\n%s", n, out)
	}
}

func TestDoctorStaleProjectsCheck(t *testing.T) {
	t.Run("gone dir fails, live dir retained", func(t *testing.T) {
		dir := t.TempDir()
		liveDir := t.TempDir()
		goneDir := filepath.Join(t.TempDir(), "does-not-exist")
		projectStore, _ := seedProjectsJSON(t, liveDir, goneDir)
		results, err := runDoctorDiagnosis(staleDeps(dir, &stubStaleSweepReader{}, nil, projectStore))
		if err != nil {
			t.Fatalf("runDoctorDiagnosis: %v", err)
		}
		got := findCheck(t, results, "stale projects")
		if got.status != checkFail {
			t.Errorf("status = %v; want checkFail for a gone-dir project", got.status)
		}
		if got.detail != "1 stale project" {
			t.Errorf("detail = %q; want %q", got.detail, "1 stale project")
		}
	})

	t.Run("multiple stale projects use the plural count copy", func(t *testing.T) {
		dir := t.TempDir()
		goneA := filepath.Join(t.TempDir(), "gone-a")
		goneB := filepath.Join(t.TempDir(), "gone-b")
		projectStore, _ := seedProjectsJSON(t, goneA, goneB)
		results, err := runDoctorDiagnosis(staleDeps(dir, &stubStaleSweepReader{}, nil, projectStore))
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
		results, err := runDoctorDiagnosis(staleDeps(dir, &stubStaleSweepReader{}, nil, projectStore))
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

	t.Run("evaluates with the server down (filesystem-only)", func(t *testing.T) {
		goneDir := filepath.Join(t.TempDir(), "gone")
		projectStore, _ := seedProjectsJSON(t, goneDir)
		deps := &DoctorDeps{
			StateDir:      t.TempDir(),
			ServerRunning: func() bool { return false },
			SaverPresent:  func() (bool, error) { return true, nil },
			HookCounts:    func() (map[string]int, error) { return allHooksHealthy(), nil },
			HookLister:    &stubStaleSweepReader{},
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

func TestDoctorStaleChecksAreReadOnly(t *testing.T) {
	dir := t.TempDir()
	hookStore, hooksPath := hookstest.StageStore(t, hookstest.Staging{Seed: hooksBody(hookstest.ReapableSeedA)})
	liveDir := t.TempDir()
	goneDir := filepath.Join(t.TempDir(), "gone")
	projectStore, projectsPath := seedProjectsJSON(t, liveDir, goneDir)

	hooksBefore := readFileBytes(t, hooksPath)
	projectsBefore := readFileBytes(t, projectsPath)

	lister := &stubStaleSweepReader{rows: tokenRows(hookstest.LiveSeedB)} // the seeded reapable key is stale
	results, err := runDoctorDiagnosis(staleDeps(dir, lister, hookStore, projectStore))
	if err != nil {
		t.Fatalf("runDoctorDiagnosis: %v", err)
	}
	if got := findCheck(t, results, "stale hooks"); got.status != checkFail {
		t.Fatalf("stale hooks status = %v; want checkFail (setup should be stale)", got.status)
	}
	if got := findCheck(t, results, "stale projects"); got.status != checkFail {
		t.Fatalf("stale projects status = %v; want checkFail (setup should be stale)", got.status)
	}

	assertHooksFileUnchanged(t, hooksPath, hooksBefore, "mutated by diagnosis (read-only violated)")
	if projectsAfter := readFileBytes(t, projectsPath); !bytes.Equal(projectsBefore, projectsAfter) {
		t.Errorf("projects.json mutated by diagnosis (read-only violated)\nbefore: %s\nafter:  %s", projectsBefore, projectsAfter)
	}
}

// assertSkippedPruneLine pins the exact line and its placement in the repair
// block: a stand-down and a removal cannot co-occur, so the line stands where
// the `Pruned stale hook:` lines would have. It is the whole of the repair's
// stand-down reporting, so a second skipped-prune line — one cycle described
// twice, or described in another reason's words — fails here too.
func assertSkippedPruneLine(t *testing.T, out, want string) {
	t.Helper()

	if strings.Contains(out, "Pruned stale hook:") {
		t.Errorf("doctor --fix reported a prune on a stand-down:\n%s", out)
	}

	lines := strings.Split(out, "\n")
	skippedAt, projectAt := -1, -1
	for i, line := range lines {
		switch {
		case line == want:
			if skippedAt != -1 {
				t.Errorf("skipped-prune line printed twice:\n%s", out)
			}
			skippedAt = i
		case strings.HasPrefix(line, "Skipped stale hook prune:"):
			t.Errorf("skipped-prune line = %q, want %q", line, want)
		case strings.HasPrefix(line, "Pruned stale project:"):
			projectAt = i
		}
	}
	if skippedAt == -1 {
		t.Fatalf("missing skipped-prune line %q:\n%s", want, out)
	}
	if projectAt != -1 && skippedAt > projectAt {
		t.Errorf("skipped-prune line follows the project prune; want it in the hook-prune block:\n%s", out)
	}
}
