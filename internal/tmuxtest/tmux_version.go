package tmuxtest

import (
	"os/exec"
	"strings"
	"testing"
)

// TmuxVersionHook is a global hook the tmux version of Portal registered:
// the event and the command, byte for byte as its bootstrap appended it.
type TmuxVersionHook struct{ Event, Command string }

// The commands the tmux version's hooks ran, each a portal it found on the
// server's PATH.
const (
	tmuxVersionNotify        = `run-shell "command -v portal >/dev/null 2>&1 && portal state notify"`
	tmuxVersionCommitNow     = `run-shell "command -v portal >/dev/null 2>&1 && portal state commit-now"`
	tmuxVersionSignalHydrate = `run-shell "command -v portal >/dev/null 2>&1 && portal state signal-hydrate -- #{session_name}"`
)

// TmuxVersionHooks are the hooks the tmux version of Portal last registered,
// one on each event it managed.
var TmuxVersionHooks = []TmuxVersionHook{
	{"session-created", tmuxVersionNotify},
	{"session-closed", tmuxVersionCommitNow},
	{"session-renamed", tmuxVersionNotify},
	{"window-linked", tmuxVersionNotify},
	{"window-unlinked", tmuxVersionNotify},
	{"window-layout-changed", tmuxVersionNotify},
	{"pane-focus-out", tmuxVersionNotify},
	{"client-attached", tmuxVersionSignalHydrate},
	{"client-session-changed", tmuxVersionSignalHydrate},
}

// PortalFreePath is the PATH StartPortalFree gives the server: one holding
// no portal, so a hook of the tmux version's that fires runs none.
const PortalFreePath = "/usr/bin:/bin"

// StartPortalFree starts s's server with PortalFreePath as its PATH, holding
// the tmux version's anchor session _portal-bootstrap, as a server with no
// session exits. The server's environment holds nothing else of the test's,
// so neither $TMUX nor a Portal path reaches a hook or a session's shell.
func (s *Socket) StartPortalFree(t *testing.T) {
	t.Helper()
	binary, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatalf("find tmux: %v", err)
	}
	start := exec.Command(binary, socketArgs(s.socketPath, "new-session", "-d", "-s", "_portal-bootstrap")...)
	start.Env = []string{"PATH=" + PortalFreePath, "HOME=" + t.TempDir(), "SHELL=/bin/sh", "TERM=xterm-256color"}
	if out, err := start.CombinedOutput(); err != nil {
		t.Fatalf("start a portal-free tmux server: %v\n%s", err, out)
	}
	if got := strings.TrimSpace(s.Run(t, "show-environment", "-g", "PATH")); got != "PATH="+PortalFreePath {
		t.Fatalf("the server's PATH is %q, want %q", got, "PATH="+PortalFreePath)
	}
}

// RegisterTmuxVersionHooks appends TmuxVersionHooks to s's global hooks, as
// the tmux version's bootstrap did.
func (s *Socket) RegisterTmuxVersionHooks(t *testing.T) {
	t.Helper()
	for _, h := range TmuxVersionHooks {
		s.Run(t, "set-hook", "-ga", h.Event, h.Command)
	}
}
