// Package tmuxtest provides an isolated tmux-server harness shared by
// integration tests across the portal codebase. It exists so the per-package
// `tmuxSocket` scaffolding (originally duplicated in
// internal/restore/integration_test.go and cmd/bootstrap/phase5_integration_test.go)
// has a single source of truth and cannot drift.
//
// Files in this package live outside `_test.go` so they are importable from
// any other package's test files. This package is internal to portal and is
// only intended for use by tests.
package tmuxtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/tmux"
)

// socketArgs prepends the isolated-socket prefix (`-S <socketPath> -f /dev/null`)
// to args, returning the full tmux argv. Centralising this prefix means all
// invocations against the isolated server stay byte-identical and the prefix
// only ever changes in one place.
func socketArgs(socketPath string, args ...string) []string {
	return append([]string{"-S", socketPath, "-f", "/dev/null"}, args...)
}

// Socket scopes an isolated tmux server to a single test. The server uses
// `tmux -S <abs-socket-path>` rooted in /tmp so the path stays inside the
// platform's UNIX-socket length cap (104 bytes on darwin, 108 on linux), and
// the user's tmux server is never touched.
//
// Why not -L + TMUX_TMPDIR=t.TempDir()? Go's t.TempDir on darwin lives under
// /private/var/folders/... and tmux composes a socket path of
// $TMUX_TMPDIR/tmux-$UID/<L>. That blows past 104 bytes for any non-trivial
// test name. A short absolute -S path is the simplest robust workaround.
type Socket struct {
	socketPath string
}

// New constructs a Socket and registers a t.Cleanup that issues kill-server
// on the isolated socket and removes the temp dir. The cleanup runs on both
// pass and fail (and on panic) so a stray tmux server is never left behind.
//
// prefix is the os.MkdirTemp prefix for the temp dir holding the socket file
// (e.g. "ptl-" or "ptl-p5-"). Pass an empty string to default to "ptl-".
func New(t *testing.T, prefix string) *Socket {
	t.Helper()
	if prefix == "" {
		prefix = "ptl-"
	}
	dir, err := os.MkdirTemp("", prefix)
	if err != nil {
		t.Fatalf("mkdir temp socket dir: %v", err)
	}
	socketPath := filepath.Join(dir, "s")
	s := &Socket{socketPath: socketPath}
	t.Cleanup(func() {
		s.KillServer()
		_ = os.RemoveAll(dir)
	})
	return s
}

// SocketPath returns the absolute path to the unix socket file backing the
// isolated tmux server. Exposed for diagnostics; callers should prefer Run /
// TryRun / Client over reaching into the socket path directly.
func (s *Socket) SocketPath() string { return s.socketPath }

// cmd builds an *exec.Cmd targeting the isolated socket via -S. The
// `-f /dev/null` flag suppresses the user's ~/.tmux.conf so tests run against
// vanilla tmux defaults (notably base-index 0 and pane-base-index 0). tmux
// only acts on -f when starting a new server, but specifying it on every
// invocation is harmless and keeps the helper symmetric.
func (s *Socket) cmd(args ...string) *exec.Cmd {
	return exec.Command("tmux", socketArgs(s.socketPath, args...)...)
}

// Run executes a tmux command on the isolated socket, fatalling the test on
// failure. Returns combined stdout+stderr verbatim.
func (s *Socket) Run(t *testing.T, args ...string) string {
	t.Helper()
	out, err := s.cmd(args...).CombinedOutput()
	if err != nil {
		t.Fatalf("tmux %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// TryRun executes a tmux command on the isolated socket and returns the
// combined output along with any exec error, leaving error handling to the
// caller. Used to drive negative paths ("expect this to fail").
func (s *Socket) TryRun(args ...string) (string, error) {
	out, err := s.cmd(args...).CombinedOutput()
	return string(out), err
}

// KillServer tears down the isolated tmux server. Errors are intentionally
// ignored: the typical case is "server already dead from a prior kill-server
// in the test body," which is healthy.
func (s *Socket) KillServer() {
	_, _ = s.cmd("kill-server").CombinedOutput()
}

// socketCommander wraps each tmux invocation with `-S <socketPath>` so a
// *tmux.Client built from it talks only to the test's isolated server. It
// satisfies tmux.Commander and is the production code's natural seam for
// pointing at a non-default socket.
type socketCommander struct {
	socketPath string
}

// runRaw executes tmux on the isolated socket and returns the raw stdout
// bytes. Shared by Run and RunRaw so the exec/error path lives in one place.
func (sc *socketCommander) runRaw(args []string) ([]byte, error) {
	return exec.Command("tmux", socketArgs(sc.socketPath, args...)...).Output()
}

// Run executes tmux on the isolated socket and trims surrounding whitespace.
func (sc *socketCommander) Run(args ...string) (string, error) {
	out, err := sc.runRaw(args)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// RunRaw executes tmux on the isolated socket and returns its output verbatim.
func (sc *socketCommander) RunRaw(args ...string) (string, error) {
	out, err := sc.runRaw(args)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// Client returns a *tmux.Client wired to the isolated socket commander.
func (s *Socket) Client() *tmux.Client {
	return tmux.NewClient(&socketCommander{socketPath: s.socketPath})
}

// WaitForSession polls list-sessions until the target name appears or the
// deadline elapses. tmux's new-session is synchronous from the caller's POV
// but on slow CI systems a brief settle window has been observed before the
// session is queryable; the poll is cheaper than a flat sleep and short-
// circuits as soon as the session is visible.
func (s *Socket) WaitForSession(t *testing.T, name string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		out, err := s.TryRun("has-session", "-t", name)
		if err == nil {
			return
		}
		// out is intentionally discarded: has-session writes to stderr
		// during the settle window ("can't find session") and the noise
		// is not useful diagnostic surface for the polling caller.
		_ = out
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("session %q did not appear within %s", name, timeout)
}
