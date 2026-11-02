// Package zmxtest gives a test its own zmx directory and a zmx Backend bound
// to it, driving the zmx build pinned in scripts/zmx-commit, and at the end
// of the test kills every process started for that directory. The
// developer's live sessions sit in their own zmx directory; a test reaches
// zmx only through a directory from New. Test-only: production code must not
// import this package.
package zmxtest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/portalbintest"
)

// PoisonedDir is the ZMX_DIR that the TestMain of every package whose tests
// run zmx sets, so a zmx run that misses the Backend's own directory fails
// instead of reaching the developer's live sessions.
const PoisonedDir = "/nonexistent/zmx-poison"

// root is short on purpose: a session's socket is <dir>/<name>, capped at
// 103 bytes on macOS, and $TMPDIR there is ~50 bytes on its own.
const root = "/tmp"

// settle bounds each wait at teardown for processes to exit.
const settle = 5 * time.Second

// passedEnv names the developer's variables a session sees; LANG and LC_*
// pass too. Nothing else does: HOME is a temp dir and SHELL is /bin/sh, so no
// session reads the developer's shell config.
var passedEnv = []string{"PATH", "TERM", "USER", "TMPDIR"}

// New returns a Backend on the pinned zmx build, bound to a fresh directory
// under /tmp. It fails the test, rather than skipping it, when the build is
// missing or ZMX_DIR is not poisoned. At cleanup every session in the
// directory is killed, any process still holding the directory in its
// environment fails the test and is killed, and the directory is removed.
func New(t *testing.T) *zmx.Backend {
	t.Helper()
	if err := checkPoisoned(os.Getenv); err != nil {
		t.Fatal(err)
	}
	binary := Binary(t)
	if err := checkProcessReader(); err != nil {
		t.Fatalf("zmxtest cannot prove which processes it owns: %v", err)
	}
	dir, err := os.MkdirTemp(root, "zx-")
	if err != nil {
		t.Fatalf("make zmx dir: %v", err)
	}
	env := sessionEnv(os.Environ(), t.TempDir())
	b, err := zmx.New(zmx.Config{
		Binary:  binary,
		Dir:     dir,
		Environ: func() []string { return slices.Clone(env) },
	})
	if err != nil {
		_ = os.RemoveAll(dir)
		t.Fatal(err)
	}
	t.Cleanup(func() {
		teardown(context.WithoutCancel(t.Context()), t, b)
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("remove zmx dir %s: %v", dir, err)
		}
	})
	return b
}

// Binary is the path of the pinned zmx build. It fails the test, naming the
// command that builds it, when the build is missing.
func Binary(t *testing.T) string {
	t.Helper()
	repo, err := portalbintest.ProjectRoot()
	if err != nil {
		t.Fatalf("find the repository: %v", err)
	}
	binary, err := pinnedBinary(repo)
	if err != nil {
		t.Fatal(err)
	}
	return binary
}

// pinnedBinary is where scripts/zmx-build puts the commit scripts/zmx-commit
// pins.
func pinnedBinary(repo string) (string, error) {
	pin, err := os.ReadFile(filepath.Join(repo, "scripts", "zmx-commit"))
	if err != nil {
		return "", fmt.Errorf("read the pinned zmx commit: %w", err)
	}
	commit := strings.TrimSpace(string(pin))
	binary := filepath.Join(repo, ".cache", "zmx", commit, "zmx")
	info, err := os.Stat(binary)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("the pinned zmx build %s is missing; run scripts/zmx-build", binary)
	}
	return binary, nil
}

func checkPoisoned(getenv func(string) string) error {
	if got := getenv("ZMX_DIR"); got != PoisonedDir {
		return fmt.Errorf("ZMX_DIR is %q, want %q: the package's TestMain must poison it before any test runs zmx", got, PoisonedDir)
	}
	return nil
}

func sessionEnv(environ []string, home string) []string {
	var env []string
	for _, kv := range environ {
		key, _, _ := strings.Cut(kv, "=")
		if slices.Contains(passedEnv, key) || key == "LANG" || strings.HasPrefix(key, "LC_") {
			env = append(env, kv)
		}
	}
	return append(env, "HOME="+home, "SHELL=/bin/sh")
}

// teardown kills every session in the Backend's directory and waits for the
// directory's processes to exit: daemons, attach clients and what the
// sessions run, found by their own environment, so one zmx cannot list or
// reach is not orphaned. A process still running after that fails t and is
// sent SIGKILL.
func teardown(ctx context.Context, t harnesstest.TestingT, b *zmx.Backend) {
	t.Helper()
	killSessions(ctx, t, b)
	survivors, err := awaitExit(t, b.Dir())
	if err != nil {
		t.Errorf("zmxtest teardown: %v", err)
		return
	}
	if len(survivors) == 0 {
		return
	}
	t.Errorf("zmxtest teardown: processes %v of %s survived their kill; sending SIGKILL", survivors, b.Dir())
	for _, pid := range survivors {
		// Re-proved just before signalling: a pid can be reused.
		if ownedBy(pid, b.Dir()) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	}
	if survivors, err := awaitExit(t, b.Dir()); err != nil || len(survivors) > 0 {
		t.Errorf("zmxtest teardown: processes %v of %s survived SIGKILL: %v", survivors, b.Dir(), err)
	}
}

func killSessions(ctx context.Context, t harnesstest.TestingT, b *zmx.Backend) {
	t.Helper()
	sessions, err := b.List(ctx)
	if err != nil {
		t.Errorf("zmxtest teardown: %v", err)
		return
	}
	// A session that does not answer is left to the process check, which
	// finds its daemon if it is still running.
	for _, s := range sessions {
		err := b.Kill(ctx, s.Name)
		if err != nil && !errors.Is(err, backend.ErrNoSuchSession) && !errors.Is(err, backend.ErrTimeout) {
			t.Errorf("zmxtest teardown: %v", err)
		}
	}
}

// awaitExit waits for every process of dir to exit, and returns those that
// have not.
func awaitExit(t harnesstest.TestingT, dir string) ([]int, error) {
	t.Helper()
	var pids []int
	var err error
	harnesstest.PollUntil(t, settle, 20*time.Millisecond, func() bool {
		pids, err = owned(dir)
		return err != nil || len(pids) == 0
	})
	return pids, err
}

// Daemon returns the pid of the zmx daemon hosting a session in b's
// directory: the parent of the session's process, proven to belong to that
// directory.
func Daemon(t *testing.T, b *zmx.Backend, name string) int {
	t.Helper()
	sessions, err := b.List(t.Context())
	if err != nil {
		t.Fatalf("list %s: %v", b.Dir(), err)
	}
	i := slices.IndexFunc(sessions, func(s backend.Session) bool { return s.Name == name })
	if i < 0 || !sessions[i].Answered || sessions[i].PID <= 0 {
		t.Fatalf("no answering session %q in %s: %+v", name, b.Dir(), sessions)
	}
	pid, err := parentOf(sessions[i].PID)
	if err != nil || !ownedBy(pid, b.Dir()) {
		t.Fatalf("parent %d of session %q is not a process of %s: %v", pid, name, b.Dir(), err)
	}
	return pid
}

// StopDaemon stops, with SIGSTOP, the daemon hosting a session in b's
// directory, and returns its pid. At cleanup it continues the daemon, if it
// is still a process of that directory, so teardown can kill it.
func StopDaemon(t *testing.T, b *zmx.Backend, name string) int {
	t.Helper()
	pid := Daemon(t, b, name)
	if err := syscall.Kill(pid, syscall.SIGSTOP); err != nil {
		t.Fatalf("stop daemon %d: %v", pid, err)
	}
	t.Cleanup(func() {
		if ownedBy(pid, b.Dir()) {
			_ = syscall.Kill(pid, syscall.SIGCONT)
		}
	})
	return pid
}
