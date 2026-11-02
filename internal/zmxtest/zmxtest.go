// Package zmxtest gives a test its own zmx directory and a zmx Backend bound
// to it, and tears down every daemon in that directory when the test ends.
// The developer's live sessions sit in their own zmx directory; a test reaches
// zmx only through a directory from New. Test-only: production code must not
// import this package.
package zmxtest

import (
	"errors"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/harnesstest"
)

// root is short on purpose: a session's socket is <dir>/<name>, capped at
// 103 bytes on macOS, and $TMPDIR there is ~50 bytes on its own.
const root = "/tmp"

// settle bounds each wait at teardown for daemons to exit.
const settle = 5 * time.Second

// New returns a Backend bound to a fresh zmx directory under /tmp. Sessions
// run /bin/sh with HOME in a temp dir, so none reads the developer's shell
// config, and no run inherits ZMX_SESSION or ZMX_SESSION_PREFIX, whatever the
// consuming package's TestMain does. At cleanup every session in the
// directory is killed, a daemon that outlives its kill fails the test and is
// killed itself, and the directory is removed.
func New(t *testing.T) *zmx.Backend {
	t.Helper()
	if _, err := exec.LookPath("zmx"); err != nil {
		t.Skip("zmx not available; skipping")
	}
	dir, err := os.MkdirTemp(root, "zx-")
	if err != nil {
		t.Fatalf("make zmx dir: %v", err)
	}
	home := t.TempDir()
	environ := func() []string {
		env := slices.DeleteFunc(os.Environ(), func(kv string) bool {
			return strings.HasPrefix(kv, "ZMX_SESSION=") || strings.HasPrefix(kv, "ZMX_SESSION_PREFIX=")
		})
		return append(env, "SHELL=/bin/sh", "HOME="+home)
	}
	b := zmx.New(dir, environ, zmx.ExecRunner{})
	t.Cleanup(func() {
		teardown(t, b)
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("remove zmx dir %s: %v", dir, err)
		}
	})
	return b
}

// teardown kills every session in the Backend's directory and waits for its
// daemons to exit. A daemon still running after that fails t and is sent
// SIGKILL. It finds daemons by their own environment as well as through zmx,
// so a session zmx cannot list or reach is not orphaned.
func teardown(t harnesstest.TestingT, b *zmx.Backend) {
	t.Helper()
	killSessions(t, b)
	if awaitNoDaemons(t, b.Dir()) {
		return
	}
	survivors := daemons(b.Dir())
	t.Errorf("zmxtest teardown: daemons %v in %s survived their kill; sending SIGKILL", survivors, b.Dir())
	for _, pid := range survivors {
		// Re-proved just before signalling: a pid can be reused.
		if ownedBy(pid, b.Dir()) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	}
	if !awaitNoDaemons(t, b.Dir()) {
		t.Errorf("zmxtest teardown: daemons %v in %s survived SIGKILL", daemons(b.Dir()), b.Dir())
	}
}

func killSessions(t harnesstest.TestingT, b *zmx.Backend) {
	t.Helper()
	sessions, err := b.List()
	if err != nil {
		t.Errorf("zmxtest teardown: %v", err)
		return
	}
	for _, s := range sessions {
		if err := b.Kill(s.Name); err != nil && !errors.Is(err, backend.ErrNoSuchSession) {
			t.Errorf("zmxtest teardown: %v", err)
		}
	}
}

func awaitNoDaemons(t harnesstest.TestingT, dir string) bool {
	t.Helper()
	return harnesstest.PollUntil(t, settle, 20*time.Millisecond, func() bool {
		return len(daemons(dir)) == 0
	})
}

// daemons returns the pids of the zmx processes started for dir, daemons and
// attach clients alike: this user's processes whose argv is zmx attach and
// whose environment sets ZMX_DIR to dir. The Backend sets ZMX_DIR on every
// run, and a daemon keeps the environment of the run that forked it.
func daemons(dir string) []int {
	out, err := exec.Command("ps", "-x", "-ww", "-E", "-o", "pid=,command=").Output()
	if err != nil {
		return nil
	}
	var pids []int
	for line := range strings.Lines(string(out)) {
		pidField, rest, _ := strings.Cut(strings.TrimSpace(line), " ")
		pid, err := strconv.Atoi(pidField)
		if err == nil && isDaemonOf(rest, dir) {
			pids = append(pids, pid)
		}
	}
	return pids
}

// Daemon returns the pid of the zmx daemon hosting a session in b's
// directory: the parent of the session's process, proven to belong to that
// directory.
func Daemon(t *testing.T, b *zmx.Backend, name string) int {
	t.Helper()
	sessions, err := b.List()
	if err != nil {
		t.Fatalf("list %s: %v", b.Dir(), err)
	}
	i := slices.IndexFunc(sessions, func(s backend.Session) bool { return s.Name == name })
	if i < 0 || sessions[i].PID <= 0 {
		t.Fatalf("no running session %q in %s", name, b.Dir())
	}
	out, err := exec.Command("ps", "-o", "ppid=", "-p", strconv.Itoa(sessions[i].PID)).Output()
	if err != nil {
		t.Fatalf("parent of session %q: %v", name, err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil || !ownedBy(pid, b.Dir()) {
		t.Fatalf("parent %q of session %q is not a zmx daemon of %s", out, name, b.Dir())
	}
	return pid
}

func ownedBy(pid int, dir string) bool {
	out, err := exec.Command("ps", "-ww", "-E", "-o", "command=", "-p", strconv.Itoa(pid)).Output()
	return err == nil && isDaemonOf(strings.TrimSpace(string(out)), dir)
}

// isDaemonOf reads ps's command column, the argv then the environment.
func isDaemonOf(command, dir string) bool {
	return strings.HasPrefix(command, "zmx attach ") &&
		slices.Contains(strings.Fields(command), "ZMX_DIR="+dir)
}
