//go:build integration

package bootstrap_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/portalbintest"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/proctable"
	"github.com/leeovery/portal/internal/proctest"
	"github.com/leeovery/portal/internal/saver"
	"github.com/leeovery/portal/internal/saverhost"
	"github.com/leeovery/portal/internal/state"
)

const (
	// markedHelperEnv, set, makes TestMarkedHelperProcess sleep carrying the
	// environment it was run with.
	markedHelperEnv = "PORTAL_TEST_MARKED_HELPER"
	// survivalSettle is how long a process the sweep spared must outlive it.
	survivalSettle = 300 * time.Millisecond
	daemonSettle   = 10 * time.Second
)

// TestMarkedHelperProcess is no test of its own: a sweep's candidate that
// carries the saver session's marks without being a portal state daemon.
func TestMarkedHelperProcess(t *testing.T) {
	if os.Getenv(markedHelperEnv) == "" {
		return
	}
	time.Sleep(time.Minute)
}

// sweepMarks are the saver session's marks for one test: its name, and a
// ZMX_DIR of the test's own under a file, so every read a daemon run with it
// makes fails with ENOTDIR. Such a daemon never reads its saver session as
// gone, so it never self-ejects while the test holds it.
func sweepMarks(t *testing.T) []string {
	t.Helper()
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	return []string{zmx.SessionEntry(saver.SessionName), "ZMX_DIR=" + filepath.Join(blocker, "zmx")}
}

// sweepWith runs the production sweep's proof over candidates, signalling
// only through held, which proves each pid the test's own as it is sent.
// No saver session is running, so every proved daemon is an orphan.
func sweepWith(t *testing.T, marks []string, pgrep func() ([]int, error), held ...*proctest.Held) {
	t.Helper()
	signal := func(pid int, sig syscall.Signal) error {
		for _, h := range held {
			if h.PID() == pid {
				return h.Signal(pid, sig)
			}
		}
		return fmt.Errorf("pid %d is none of the test's processes, so it is not signalled", pid)
	}
	host, err := saverhost.NewHost(saverhost.Config{Backend: backendtest.New(), StateDir: t.TempDir(), Marks: marks, Signal: signal})
	if err != nil {
		t.Fatal(err)
	}
	core := &bootstrap.OrphanSweepCore{Pgrep: pgrep, SaverPID: host.SaverPID, Prove: host.Prove, Signal: signal, AwaitExit: host.AwaitExit}
	if err := core.SweepOrphanDaemons(t.Context()); err != nil {
		t.Fatalf("SweepOrphanDaemons = %v, want nil", err)
	}
}

// startDaemon holds `binary state daemon` over a state directory of its own,
// which the sandbox's pgrep surfaces, with env and extra, and returns once it
// holds its lock.
func startDaemon(t *testing.T, binary string, env []string, extra ...string) *proctest.Held {
	t.Helper()
	dir := t.TempDir()
	portaltest.OwnStateDir(t, dir)
	cmd := exec.Command(binary, "state", "daemon")
	cmd.Env = append(slices.Clone(env), append([]string{"PORTAL_STATE_DIR=" + dir}, extra...)...)
	h := proctest.Start(t, cmd, "")
	if !harnesstest.PollUntil(t, daemonSettle, 20*time.Millisecond, func() bool {
		pid, err := state.ReadPIDFile(dir)
		return err == nil && pid == h.PID()
	}) {
		t.Fatalf("the daemon %d never held its lock\n--- portal.log ---\n%s", h.PID(), portaltest.ReadPortalLogSafe(dir))
	}
	return h
}

// awaitEnviron waits for pid to carry every entry of marks, read as the
// proof reads it.
func awaitEnviron(t *testing.T, pid int, marks []string) {
	t.Helper()
	if !harnesstest.PollUntil(t, daemonSettle, 10*time.Millisecond, func() bool {
		args, err := proctable.ReadArgs(pid)
		return err == nil && !slices.ContainsFunc(marks, func(m string) bool { return !slices.Contains(args.Env, m) })
	}) {
		t.Fatalf("pid %d never carried %q", pid, marks)
	}
}

func survives(t *testing.T, h *proctest.Held, what string) {
	t.Helper()
	select {
	case <-h.Exited():
		t.Errorf("%s (pid %d) was killed by the sweep: %v", what, h.PID(), h.Status(t))
	case <-time.After(survivalSettle):
	}
}

// A pgrep hit can name a pid recycled to an unrelated process by the time the
// sweep reaches it. The production proof must refuse it, so nothing is
// signalled: whether its environment can't be read, or carries every mark of
// the saver session and only the identity check stands in the way. The
// candidates are the test's own children, so a failure kills nothing the test
// doesn't own.
func TestSweepOrphanDaemons_Integration_RecycledPIDRefusal(t *testing.T) {
	env, _ := portaltest.IsolateStateForTest(t)
	marks := sweepMarks(t)

	sleeper := proctest.Start(t, exec.Command("sh", "-c", "exec sleep 30"), "")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	helper := exec.Command(exe, "-test.run=^TestMarkedHelperProcess$")
	helper.Env = append(slices.Clone(env), append(slices.Clone(marks), markedHelperEnv+"=1")...)
	marked := proctest.Start(t, helper, "")
	awaitEnviron(t, marked.PID(), marks)

	sweepWith(t, marks, func() ([]int, error) { return []int{sleeper.PID(), marked.PID()}, nil }, sleeper, marked)

	survives(t, sleeper, "a sleep the pgrep hit named")
	survives(t, marked, "a process carrying the saver session's marks")
	host, err := saverhost.NewHost(saverhost.Config{Backend: backendtest.New(), StateDir: t.TempDir(), Marks: marks, Signal: func(int, syscall.Signal) error {
		return errors.New("no signal")
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := host.Prove(marked.PID()); !errors.Is(err, saverhost.ErrNotSaverDaemon) {
		t.Errorf("Prove(the marked process) = %v, want the identity check's refusal", err)
	}
}

// Two of the test's daemons, both found by the sandboxed pgrep and both
// identified by argv: the one carrying the saver session's environment is an
// orphaned saver daemon and is killed, while the one without it, as another
// install's daemon would be, the tmux version's among them, survives.
func TestSweepOrphanDaemons_Integration_KillsOnlyTheMarkedDaemon(t *testing.T) {
	// Built before the isolation scrubs HOME, which go build's caches hang off.
	binary := portalbintest.Build(t)
	env, _ := portaltest.IsolateStateForTest(t)
	marks := sweepMarks(t)
	orphan := startDaemon(t, binary, env, marks...)
	unmarked := startDaemon(t, binary, env, marks[1])

	if pids, err := portaltest.PgrepPortalDaemons(); err != nil || !slices.Contains(pids, orphan.PID()) || !slices.Contains(pids, unmarked.PID()) {
		t.Fatalf("PgrepPortalDaemons = %v, %v; want both daemons, %d and %d", pids, err, orphan.PID(), unmarked.PID())
	}

	sink := logtest.Install(t)

	sweepWith(t, marks, state.PgrepPortalDaemons, orphan, unmarked)

	// Counted only once it has finished exiting, a zombie the test holds.
	summary := sink.Records().Matching("clean", "orphan-daemon sweep complete").Only(t, "the sweep's summary")
	if killed := summary.IntAttr(t, "killed"); killed != 1 {
		t.Errorf("killed = %d, want the orphan, once it has exited", killed)
	}
	select {
	case <-orphan.Exited():
	case <-time.After(daemonSettle):
		t.Fatalf("the orphaned saver daemon %d outlived the sweep by %s", orphan.PID(), daemonSettle)
	}
	if status := orphan.Status(t); !status.Signaled() || status.Signal() != syscall.SIGKILL {
		t.Errorf("the orphan ended %v, want by SIGKILL", status)
	}
	survives(t, unmarked, "the daemon without the saver session's environment")
}
