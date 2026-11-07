//go:build integration

package cmd_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/creack/pty"
	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/runnertoken"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/zmxtest"
)

// attachSettle bounds each wait on Portal, zmx or the session's shell.
const attachSettle = 20 * time.Second

// portalTermRows and portalTermCols size the terminal a portalTerm runs on.
const portalTermRows, portalTermCols = 40, 120

// pickerMark is on every picker page, and nowhere in a session.
const pickerMark = "session manager"

// killPrompt titles the picker's kill prompt.
const killPrompt = "Kill session?"

// Portal's default keys in their legacy forms, as a terminal without the newer
// encoding sends them.
const (
	altD      = "\x1bd"
	altShiftD = "\x1bD"
	altQ      = "\x1bq"
)

// portalWorld is a zmx directory and a state directory of a test's own, and
// the environment a portal binary runs in against them.
type portalWorld struct {
	binary   string
	env      []string
	b        *zmx.Backend
	stateDir string
}

func newPortalWorld(t *testing.T, binary string) *portalWorld {
	t.Helper()
	env, _ := portaltest.IsolateStateForTest(t)
	// The runner registry's sockets live in the state directory, and one under
	// macOS's $TMPDIR is too long for a socket's address. Made before the zmx
	// directory, so it is removed after the sessions are killed, the saver the
	// bootstrap starts among them.
	stateDir := shortTempDir(t)
	portaltest.OwnStateDir(t, stateDir)
	portaltest.RegisterStateDirTeardownGuard(t, stateDir)
	b := zmxtest.New(t)
	env = append(env,
		"PORTAL_STATE_DIR="+stateDir,
		"ZMX_DIR="+b.Dir(),
		"PATH="+filepath.Dir(zmxtest.Binary(t))+string(os.PathListSeparator)+os.Getenv("PATH"),
		"PORTAL_PROJECTS_FILE="+filepath.Join(t.TempDir(), "projects.json"),
		"PORTAL_PREFS_FILE="+filepath.Join(t.TempDir(), "prefs.json"),
		"SHELL=/bin/sh",
		"TERM=xterm-256color",
	)
	return &portalWorld{binary: binary, env: env, b: b, stateDir: stateDir}
}

// shortTempDir is a directory of the test's own under /tmp, short enough to
// hold unix sockets.
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "ps-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// portalTerm is a process running on a pseudo-terminal the test holds, in a
// world of the test's own.
type portalTerm struct {
	*portalWorld
	cmd *exec.Cmd
	tty *os.File

	mu  sync.Mutex
	out bytes.Buffer
	// exited closes once the process is reaped, with its error in waitErr.
	exited  chan struct{}
	waitErr error
}

// start runs argv on a terminal of its own, in the world's environment.
func (w *portalWorld) start(t *testing.T, argv ...string) *portalTerm {
	t.Helper()
	return w.startIn(t, w.env, argv...)
}

// startIn runs argv on a terminal of its own, in env.
func (w *portalWorld) startIn(t *testing.T, env []string, argv ...string) *portalTerm {
	t.Helper()
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env = env
	tty, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: portalTermRows, Cols: portalTermCols})
	if err != nil {
		t.Fatalf("start %s on a terminal: %v", argv[0], err)
	}
	p := &portalTerm{portalWorld: w, cmd: cmd, tty: tty, exited: make(chan struct{})}
	go p.record()
	go func() {
		p.waitErr = cmd.Wait()
		close(p.exited)
	}()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		<-p.exited
		_ = tty.Close()
	})
	return p
}

// portal runs the portal binary with args on a terminal of its own.
func (w *portalWorld) portal(t *testing.T, args ...string) *portalTerm {
	t.Helper()
	return w.start(t, append([]string{w.binary}, args...)...)
}

// openOnTerminal mints a session in a project directory with portal open -p,
// on a terminal, in a world of its own, and waits for the attach to connect.
func openOnTerminal(t *testing.T, binary string) (*portalTerm, string) {
	t.Helper()
	return newPortalWorld(t, binary).openProject(t)
}

// openProject mints the world's first session, in a project directory, with
// portal open -p on a terminal, and waits for the attach to connect.
func (w *portalWorld) openProject(t *testing.T) (*portalTerm, string) {
	t.Helper()
	work := filepath.Join(realPath(t, t.TempDir()), "myproj")
	if err := os.Mkdir(work, 0o700); err != nil {
		t.Fatal(err)
	}
	p := w.portal(t, "open", "-p", work)
	name := w.awaitOneSession(t)
	p.awaitLeading(t, name)
	return p, name
}

// awaitLeading waits for p's portal to be the one runner on session, its own
// zmx client leading it: the runner catches the signals that end an attach
// before it starts that client. A client count can't tell, since the zmx
// attach Create runs is a client of the session while it lasts.
func (p *portalTerm) awaitLeading(t *testing.T, session string) {
	t.Helper()
	p.awaitRunner(t, session)
	token := p.token(t)
	var leader runnertoken.Leader
	if !harnesstest.PollUntil(t, attachSettle, 20*time.Millisecond, func() bool {
		var err error
		leader, err = runnertoken.ReadLeader(t.Context(), p.b, session)
		return err == nil && token != "" && leader.Token == token
	}) {
		t.Fatalf("session %s is led by token %q, want portal %d's %q", session, leader.Token, p.cmd.Process.Pid, token)
	}
}

func (p *portalTerm) record() {
	buf := make([]byte, 4096)
	for {
		n, err := p.tty.Read(buf)
		p.mu.Lock()
		p.out.Write(buf[:n])
		p.mu.Unlock()
		if err != nil {
			return
		}
	}
}

// mark is where the terminal's output stands, for awaitAfter.
func (p *portalTerm) mark() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.out.Len()
}

// shownSince is what the terminal has shown since mark, its escapes stripped.
func (p *portalTerm) shownSince(mark int) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return ansi.Strip(string(p.out.Bytes()[mark:]))
}

// awaitAfter waits for the terminal to show want after mark.
func (p *portalTerm) awaitAfter(t *testing.T, mark int, want string) {
	t.Helper()
	if !harnesstest.PollUntil(t, attachSettle, 20*time.Millisecond, func() bool {
		return strings.Contains(p.shownSince(mark), want)
	}) {
		t.Fatalf("the terminal never showed %q; it shows:\n%s", want, p.shownSince(mark))
	}
}

func (p *portalTerm) typeIn(t *testing.T, input string) {
	t.Helper()
	if _, err := p.tty.WriteString(input); err != nil {
		t.Fatalf("type %q: %v", input, err)
	}
}

// awaitOneSession waits for the world to hold one session of the user's, the
// saver its first command's bootstrap started aside, and returns its name.
func (w *portalWorld) awaitOneSession(t *testing.T) string {
	t.Helper()
	var names []string
	if !harnesstest.PollUntil(t, attachSettle, 20*time.Millisecond, func() bool {
		sessions, err := w.b.List(t.Context())
		names = names[:0]
		for _, s := range slices.DeleteFunc(sessions, backend.Session.PortalOwned) {
			names = append(names, s.Name)
		}
		return err == nil && len(names) == 1
	}) {
		t.Fatalf("sessions = %q, want the one portal open made", names)
	}
	return names[0]
}

func (w *portalWorld) awaitClients(t *testing.T, name string, want int) {
	t.Helper()
	awaitClientsOn(t, w.b, name, want)
}

// awaitClientsOn waits for b's session name to have want clients attached.
func awaitClientsOn(t *testing.T, b *zmx.Backend, name string, want int) {
	t.Helper()
	var got int
	if !harnesstest.PollUntil(t, attachSettle, 20*time.Millisecond, func() bool {
		s, err := b.Session(t.Context(), name)
		got = s.Clients
		return err == nil && got == want
	}) {
		t.Fatalf("session %s has %d clients, want %d", name, got, want)
	}
}

func (w *portalWorld) alive(t *testing.T, name string) bool {
	t.Helper()
	alive, err := w.b.Exists(t.Context(), name)
	if err != nil {
		t.Fatalf("Exists %s: %v", name, err)
	}
	return alive
}

// exitCode waits for portal to exit and returns its status.
func (p *portalTerm) exitCode(t *testing.T) int {
	t.Helper()
	select {
	case <-p.exited:
	case <-time.After(attachSettle):
		t.Fatalf("portal is still running; the terminal shows:\n%s", p.shownSince(0))
	}
	if exit, ok := errors.AsType[*exec.ExitError](p.waitErr); ok {
		return exit.ExitCode()
	}
	if p.waitErr != nil {
		t.Fatalf("portal: %v", p.waitErr)
	}
	return 0
}

// echoes types a command whose output differs from its input into the
// session, and waits for the output: what Portal forwards reaches the session,
// so no reader left over from the picker steals the keys.
func (p *portalTerm) echoes(t *testing.T, a, b int) {
	t.Helper()
	mark := p.mark()
	p.typeIn(t, "echo out-$(("+strconv.Itoa(a)+"*"+strconv.Itoa(b)+"))\r")
	p.awaitAfter(t, mark, "out-"+strconv.Itoa(a*b))
}

func TestPortalOpenAttachesThroughTheRunnerAgainstZmx(t *testing.T) {
	binary := buildPortalBinary(t)

	t.Run("detach returns to the picker, a pick attaches again, and the kill key's y removes the session", func(t *testing.T) {
		p, name := openOnTerminal(t, binary)
		p.echoes(t, 6, 7)

		mark := p.mark()
		p.typeIn(t, altD)
		p.awaitClients(t, name, 0)
		p.awaitAfter(t, mark, pickerMark)
		p.awaitAfter(t, mark, name)
		if !p.alive(t, name) {
			t.Fatal("the session ended on a detach")
		}

		p.typeIn(t, "\r")
		p.awaitClients(t, name, 1)
		p.echoes(t, 7, 7)

		mark = p.mark()
		p.typeIn(t, altQ)
		p.awaitClients(t, name, 0)
		p.awaitAfter(t, mark, killPrompt)
		if !p.alive(t, name) {
			t.Fatal("the session ended on the kill key, before its prompt was answered")
		}

		mark = p.mark()
		p.typeIn(t, "y")
		p.awaitAfter(t, mark, pickerMark)
		if !harnesstest.PollUntil(t, attachSettle, 20*time.Millisecond, func() bool { return !p.alive(t, name) }) {
			t.Fatal("the session outlived y")
		}
		// Kill ends the session before it removes the entry, so the entry
		// can outlive the session by a moment.
		var idx sessionindex.Index
		if !harnesstest.PollUntil(t, attachSettle, 20*time.Millisecond, func() bool {
			var err error
			if idx, err = sessionindex.Read(p.stateDir); err != nil {
				t.Fatalf("read index: %v", err)
			}
			_, ok := idx.Lookup(name)
			return !ok
		}) {
			t.Errorf("index = %+v, want the killed session's entry gone", idx.Entries())
		}

		p.typeIn(t, "q")
		if code := p.exitCode(t); code != 0 {
			t.Errorf("quitting the picker exited %d, want 0", code)
		}
	})

	t.Run("the kill key's Esc keeps the session and goes back into it", func(t *testing.T) {
		p, name := openOnTerminal(t, binary)
		p.echoes(t, 6, 7)

		mark := p.mark()
		p.typeIn(t, altQ)
		p.awaitClients(t, name, 0)
		p.awaitAfter(t, mark, killPrompt)

		p.typeIn(t, "\x1b")
		p.awaitClients(t, name, 1)
		p.echoes(t, 7, 7)

		p.typeIn(t, altShiftD)
		if code := p.exitCode(t); code != 0 {
			t.Errorf("detaching to the shell exited %d, want 0", code)
		}
		if !p.alive(t, name) {
			t.Error("the session ended")
		}
	})

	t.Run("detach to the shell exits 0 and leaves the session running", func(t *testing.T) {
		p, name := openOnTerminal(t, binary)

		p.typeIn(t, altShiftD)

		if code := p.exitCode(t); code != 0 {
			t.Errorf("exit code = %d, want 0", code)
		}
		if !p.alive(t, name) {
			t.Error("the session ended on a detach to the shell")
		}
	})

	t.Run("the session's own end exits with zmx attach's status", func(t *testing.T) {
		p, name := openOnTerminal(t, binary)

		p.typeIn(t, "exit 7\r")

		// zmx attach exits 0 however the session's program ended, so 0 is the
		// status Portal passes on.
		if code := p.exitCode(t); code != 0 {
			t.Errorf("exit code = %d, want zmx attach's 0", code)
		}
		if p.alive(t, name) {
			t.Error("the session outlived its shell")
		}
	})

	t.Run("a signal to Portal exits as a shell reports it, and the session lives on", func(t *testing.T) {
		p, name := openOnTerminal(t, binary)
		mark := p.mark()

		if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
			t.Fatalf("signal portal: %v", err)
		}

		if code := p.exitCode(t); code != 128+int(syscall.SIGTERM) {
			t.Errorf("exit code = %d, want %d", code, 128+int(syscall.SIGTERM))
		}
		if shown := p.shownSince(mark); strings.Contains(shown, "exit status") {
			t.Errorf("portal wrote its exit:\n%s", shown)
		}
		p.awaitClients(t, name, 0)
	})
}
