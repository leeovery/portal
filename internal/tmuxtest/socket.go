// Package tmuxtest is the tmux server harness for portal migrate's tests, the
// only tests that run tmux. Each server sits on a socket of its own under a
// fresh temp dir, never the default socket, and runs against vanilla tmux
// defaults. It lives outside _test.go so migrate's tests and cmd's can share
// it; production code must not import it.
package tmuxtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func socketArgs(socketPath string, args ...string) []string {
	return append([]string{"-S", socketPath, "-f", "/dev/null"}, args...)
}

// Socket scopes an isolated tmux server to a single test. The socket is an
// absolute -S path in a short os.MkdirTemp dir under $TMPDIR: a path under
// t.TempDir(), which carries the test's name, can pass darwin's ~104-byte
// UNIX-socket cap.
type Socket struct {
	socketPath string
}

// New constructs a Socket and registers a cleanup that kills the server and
// removes its temp dir. prefix names the temp dir; empty defaults to "ptl-".
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

// SocketPath returns the socket file's absolute path, for code under test that
// takes a socket; prefer Run or TryRun for driving the server.
func (s *Socket) SocketPath() string { return s.socketPath }

// -f /dev/null runs against vanilla tmux defaults, not the user's ~/.tmux.conf.
func (s *Socket) cmd(args ...string) *exec.Cmd {
	return exec.Command("tmux", socketArgs(s.socketPath, args...)...)
}

// Run executes a tmux command on the isolated socket, fatalling the test on
// failure, and returns combined stdout+stderr.
func (s *Socket) Run(t *testing.T, args ...string) string {
	t.Helper()
	out, err := s.cmd(args...).CombinedOutput()
	if err != nil {
		t.Fatalf("tmux %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// TryRun executes a tmux command on the isolated socket and returns combined
// stdout+stderr with its error, for a test that expects a command to fail.
func (s *Socket) TryRun(args ...string) (string, error) {
	out, err := s.cmd(args...).CombinedOutput()
	return string(out), err
}

// HasSession reports whether the server holds a session named exactly name.
// The target is pinned as =name: because tmux prefix-matches a bare -t name,
// so a live _portal-saver-2 would answer for a gone _portal-saver, and reads a
// colon-free =name as a window's, splitting it on any period.
func (s *Socket) HasSession(name string) bool {
	_, err := s.TryRun("has-session", "-t", "="+name+":")
	return err == nil
}

// KillServer tears down the isolated tmux server. Errors are ignored: an
// already-dead server is the common case.
func (s *Socket) KillServer() {
	_, _ = s.cmd("kill-server").CombinedOutput()
}
