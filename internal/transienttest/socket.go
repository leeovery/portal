package transienttest

import (
	"os/exec"
	"strings"

	"github.com/leeovery/portal/internal/tmux"
)

// SocketCommander is a tmux.Commander targeting a specific tmux socket
// via `tmux -S <path>`. The `-f /dev/null` flag suppresses the user's
// ~/.tmux.conf so the production-builder's tmux invocations do not pull
// in user config that would couple integration tests to the developer's
// environment.
//
// Errors are wrapped via tmux.WrapCommandError so any callers that
// errors.As against *tmux.CommandError continue to work — matches the
// production tmux.RealCommander error-wrapping shape end-to-end.
type SocketCommander struct {
	// SocketPath is the absolute path to the tmux socket file used by
	// the test's isolated tmux server (typically obtained from
	// tmuxtest.Socket.SocketPath()).
	SocketPath string
}

func (s *SocketCommander) runArgs(args []string) []string {
	return append([]string{"-S", s.SocketPath, "-f", "/dev/null"}, args...)
}

// Run implements tmux.Commander with stdout trimmed of trailing
// whitespace — matches tmux.RealCommander.Run. Errors are wrapped via
// tmux.WrapCommandError (with the tmux argv) for production parity.
func (s *SocketCommander) Run(args ...string) (string, error) {
	out, err := exec.Command("tmux", s.runArgs(args)...).Output()
	if err != nil {
		return "", tmux.WrapCommandError(err, args...)
	}
	return strings.TrimSpace(string(out)), nil
}

// RunRaw implements tmux.Commander with verbatim stdout — matches
// tmux.RealCommander.RunRaw. Scrollback-capturing paths depend on the
// verbatim shape. Errors are wrapped via tmux.WrapCommandError (with the tmux
// argv) for production parity.
func (s *SocketCommander) RunRaw(args ...string) (string, error) {
	out, err := exec.Command("tmux", s.runArgs(args)...).Output()
	if err != nil {
		return "", tmux.WrapCommandError(err, args...)
	}
	return string(out), nil
}

// Compile-time guard: SocketCommander must satisfy tmux.Commander.
var _ tmux.Commander = (*SocketCommander)(nil)
