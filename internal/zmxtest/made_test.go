package zmxtest

import (
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/proctest"
)

// The refusing helpers fail their test, so each runs in this test binary
// re-executed into madeRefusalProcessName, whose failure the test reads.
const (
	madeRefusalProcessName = "TestMadeRefusalProcess"
	madeRefusalEnv         = "PORTAL_TEST_ZMXTEST_REFUSAL"
	madeSleeperName        = "TestMadeSleeperProcess"
	madeSleeperEnv         = "PORTAL_TEST_ZMXTEST_SLEEPER"
	refusalWording         = "is no zmx directory zmxtest made in this process"
)

// foreignBackend is a Backend on a directory zmxtest didn't make, as one
// built from an unpoisoned environment would be on the developer's.
func foreignBackend(t *testing.T, dir string) *zmx.Backend {
	t.Helper()
	b, err := zmx.New(zmx.Config{Binary: "/nonexistent/zmx", Dir: dir, Environ: func() []string { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestMadeDirs(t *testing.T) {
	var m madeDirs
	if err := m.refuse("/tmp/zx-1"); err == nil {
		t.Error("a directory never made was not refused")
	}
	m.add("/tmp/zx-1")
	if err := m.refuse("/tmp/zx-1"); err != nil {
		t.Errorf("a directory made was refused: %v", err)
	}
	if err := m.refuse("/tmp/zx-2"); err == nil || !strings.Contains(err.Error(), refusalWording) {
		t.Errorf("another directory: %v, want the refusal", err)
	}
}

// TestMadeSleeperProcess is not a test: it is a process carrying a ZMX_DIR,
// which a proof by environment would take as that directory's.
func TestMadeSleeperProcess(t *testing.T) {
	if os.Getenv(madeSleeperEnv) == "" {
		return
	}
	time.Sleep(time.Minute)
}

func TestSignal_RefusesAForeignBackend(t *testing.T) {
	dir := t.TempDir()
	sleeper := exec.Command(os.Args[0], "-test.run=^"+madeSleeperName+"$")
	marker := madeSleeperEnv + "=" + dir
	sleeper.Env = append(os.Environ(), marker, "ZMX_DIR="+dir)
	held := proctest.Start(t, sleeper, marker)

	err := Signal(foreignBackend(t, dir), held.PID(), syscall.SIGTERM)

	if err == nil || !strings.Contains(err.Error(), refusalWording) {
		t.Fatalf("Signal = %v, want the refusal", err)
	}
	select {
	case <-held.Exited():
		t.Error("the process of the foreign directory was signalled")
	case <-time.After(300 * time.Millisecond):
	}
}

// TestMadeRefusalProcess is not a test: it is the body of a process that
// hands a helper a foreign Backend.
func TestMadeRefusalProcess(t *testing.T) {
	helper := os.Getenv(madeRefusalEnv)
	if helper == "" {
		return
	}
	b := foreignBackend(t, t.TempDir())
	switch helper {
	case "Daemon":
		Daemon(t, b, "work")
	case "StopDaemon":
		StopDaemon(t, b, "work")
	case "Attach":
		Attach(t, b, "work")
	}
}

func TestHelpers_RefuseAForeignBackend(t *testing.T) {
	for _, helper := range []string{"Daemon", "StopDaemon", "Attach"} {
		t.Run(helper, func(t *testing.T) {
			child := exec.Command(os.Args[0], "-test.run=^"+madeRefusalProcessName+"$")
			child.Env = append(os.Environ(), madeRefusalEnv+"="+helper)

			out, err := child.CombinedOutput()

			if err == nil || !strings.Contains(string(out), refusalWording) {
				t.Errorf("%s on a foreign Backend: %v\n%s\nwant the test failed with the refusal", helper, err, out)
			}
		})
	}
}
