// Package zmxtest gives a test its own zmx directory and a zmx Backend bound
// to it, driving the zmx build pinned in scripts/zmx-commit or the one
// BinaryEnv names, and at the end of the test kills every process started for
// that directory. The developer's live sessions sit in their own zmx
// directory; a test reaches zmx only through a directory from New. Test-only:
// production code must not import this package.
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
// instead of reaching the developer's live sessions. It sits under a file, not
// a missing directory: zmx reads a missing directory as holding no sessions,
// but every read under a file fails with ENOTDIR.
const PoisonedDir = "/dev/null/zmx-poison"

// root is short on purpose: a session's socket is <dir>/<name>, capped at
// 103 bytes on macOS, and $TMPDIR there is ~50 bytes on its own.
const root = "/tmp"

// settle bounds each wait at teardown for processes to exit.
const settle = 5 * time.Second

// passedEnv names the developer's variables a session sees; LANG and LC_*
// pass too. Nothing else does: HOME is a temp dir and SHELL is /bin/sh, so no
// session reads the developer's shell config.
var passedEnv = []string{"PATH", "TERM", "USER", "TMPDIR"}

// Option configures New.
type Option func(*options)

type options struct {
	env []string
}

// WithEnv passes env, NAME=value entries, into every zmx run the Backend
// makes, and so into each session it creates, after the variables New passes
// itself: an entry here wins, as the last of a name does. A fixture passes a
// session what a test isolated, such as IsolateStateForTest's environment.
// The Backend sets ZMX_DIR after env, so env can move no session out of the
// directory, nor any process out of the reach of teardown's proof.
func WithEnv(env ...string) Option {
	return func(o *options) { o.env = append(o.env, env...) }
}

// New returns a Backend on the zmx build Binary finds, bound to a fresh
// directory under /tmp. It fails the test, rather than skipping it, when the
// build is missing or ZMX_DIR is not poisoned. At cleanup every session in the
// directory is killed, any process still holding the directory in its
// environment fails the test and is killed, and the directory is removed.
func New(t *testing.T, opts ...Option) *zmx.Backend {
	t.Helper()
	var o options
	for _, opt := range opts {
		opt(&o)
	}
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
	env := append(sessionEnv(os.Environ(), t.TempDir()), o.env...)
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

// BinaryEnv names a zmx build the tests drive in place of the pinned one,
// such as one scripts/zmx-build <ref> made.
const BinaryEnv = "PORTAL_TEST_ZMX_BINARY"

// Binary is the path of the zmx build the tests drive: the one BinaryEnv
// names, or else the pinned build. It fails the test when that build is
// missing or cannot run, outside the integration lane, and when the weekly
// zmx check doesn't run the test. Every real zmx a test runs comes from here,
// New's included.
func Binary(t *testing.T) string {
	t.Helper()
	if !integrationLane {
		t.Fatal("zmxtest: a real zmx runs only in the integration lane, the one the weekly zmx check runs: tag the test's file //go:build integration")
	}
	requireWeeklyCheck(t)
	binary, err := resolveBinary(os.Getenv, portalbintest.ProjectRoot)
	if err != nil {
		t.Fatal(err)
	}
	return binary
}

func resolveBinary(getenv func(string) string, projectRoot func() (string, error)) (string, error) {
	if binary := getenv(BinaryEnv); binary != "" {
		return namedBinary(binary)
	}
	repo, err := projectRoot()
	if err != nil {
		return "", fmt.Errorf("find the repository: %w", err)
	}
	return pinnedBinary(repo)
}

// namedBinary refuses a relative path, which each package's tests would read
// from their own directory, and any name but zmx: some tests put the build's
// directory on PATH and run zmx from it.
func namedBinary(binary string) (string, error) {
	if !filepath.IsAbs(binary) || filepath.Base(binary) != "zmx" {
		return "", fmt.Errorf("%s=%q: want an absolute path to a file named zmx", BinaryEnv, binary)
	}
	if !runnable(binary) {
		return "", fmt.Errorf("%s=%q is not an executable file", BinaryEnv, binary)
	}
	return binary, nil
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
	if !runnable(binary) {
		return "", fmt.Errorf("the pinned zmx build %s is missing; run scripts/zmx-build", binary)
	}
	return binary, nil
}

func runnable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
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
		_ = signalOwned(pid, b.Dir(), syscall.SIGKILL)
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
	pid, err := processes.Parent(sessions[i].PID)
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
	if err := Signal(b, pid, syscall.SIGSTOP); err != nil {
		t.Fatalf("stop daemon %d: %v", pid, err)
	}
	t.Cleanup(func() { _ = Signal(b, pid, syscall.SIGCONT) })
	return pid
}

// Signal sends sig to pid if it is a process of b's directory, proved as the
// signal is sent, and fails, sending nothing, for one that is not. A test
// signals a zmx process only through here: a pid proved even a moment
// earlier may since have been reused.
func Signal(b *zmx.Backend, pid int, sig syscall.Signal) error {
	return signalOwned(pid, b.Dir(), sig)
}

// SignalGroup sends sig to the process group pgid leads, as kill(-pgid)
// does, if pgid is a process of b's directory leading its own group, proved
// as the signal is sent, and fails, sending nothing, otherwise. The proof of
// the leader vouches for the group: a process joins a group only within its
// own session, and zmx starts each of its sessions' processes in a session
// of its own.
func SignalGroup(b *zmx.Backend, pgid int, sig syscall.Signal) error {
	return signalGroupOwned(pgid, b.Dir(), sig)
}
