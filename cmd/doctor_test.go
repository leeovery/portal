// Tests in this file mutate package-level Cobra/DI state (doctorDeps, rootCmd)
// and MUST NOT use t.Parallel.
package cmd

import (
	"bytes"
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

// runDoctor executes "portal doctor" with a hermetic DoctorDeps.StateDir
// pointing at dir, returning stdout, stderr, and the rootCmd.Execute error.
func runDoctor(t *testing.T, dir string) (*bytes.Buffer, *bytes.Buffer, error) {
	t.Helper()
	doctorDeps = &DoctorDeps{StateDir: dir, Now: time.Now}
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
		results, err := runDoctorDiagnosis(&DoctorDeps{StateDir: dir, Now: now})
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
		results, err := runDoctorDiagnosis(&DoctorDeps{StateDir: dir, Now: now})
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
		results, err := runDoctorDiagnosis(&DoctorDeps{StateDir: dir, Now: now})
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
		results, err := runDoctorDiagnosis(&DoctorDeps{StateDir: dir, Now: time.Now})
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
		results, err := runDoctorDiagnosis(&DoctorDeps{StateDir: dir, Now: time.Now})
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
	results, err := runDoctorDiagnosis(&DoctorDeps{StateDir: dir, Now: time.Now})
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
