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

const teardownTimeout = 5 * time.Second

// New returns a Backend bound to a fresh zmx directory under /tmp. Sessions
// run /bin/sh with HOME in a temp dir, so none reads the developer's shell
// config. At cleanup every session in the directory is killed, a daemon that
// outlives its kill fails the test, and the directory is removed.
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
		return append(os.Environ(), "SHELL=/bin/sh", "HOME="+home)
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

func teardown(t *testing.T, b *zmx.Backend) {
	t.Helper()
	sessions, err := b.List()
	if err != nil {
		t.Errorf("zmxtest teardown: list %s: %v", b.Dir(), err)
		return
	}
	var daemons []int
	for _, s := range sessions {
		if pid := parentPID(s.PID); pid > 1 {
			daemons = append(daemons, pid)
		}
		if err := b.Kill(s.Name); err != nil && !errors.Is(err, backend.ErrNoSuchSession) {
			t.Errorf("zmxtest teardown: %v", err)
		}
	}
	if harnesstest.PollUntil(t, teardownTimeout, 20*time.Millisecond, func() bool {
		left, err := b.List()
		return err == nil && len(left) == 0 && len(alive(daemons)) == 0
	}) {
		return
	}
	left, _ := b.List()
	survivors := alive(daemons)
	t.Errorf("zmxtest teardown: in %s, sessions %+v and daemons %v survived their kill", b.Dir(), left, survivors)
	for _, pid := range survivors {
		// Re-read just before signalling: a pid can be reused.
		if isZmxDaemon(pid) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	}
}

// parentPID is the parent of a session's process, which is the session's zmx
// daemon; 0 when it cannot be read.
func parentPID(pid int) int {
	if pid <= 0 {
		return 0
	}
	out, err := exec.Command("ps", "-o", "ppid=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0
	}
	ppid, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0
	}
	return ppid
}

func alive(pids []int) []int {
	var out []int
	for _, pid := range pids {
		if syscall.Kill(pid, 0) == nil {
			out = append(out, pid)
		}
	}
	return out
}

func isZmxDaemon(pid int) bool {
	out, err := exec.Command("ps", "-o", "command=", "-p", strconv.Itoa(pid)).Output()
	return err == nil && strings.HasPrefix(string(out), "zmx attach ")
}
