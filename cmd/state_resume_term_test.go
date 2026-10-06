package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/shellquote"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/themetest"
)

// The waiting pane's processes are run for real here, as this test binary
// re-executed into termPaneProcessName: a signal's disposition is a property of
// a whole process, which an in-process seam cannot observe. Every tmux and tty
// seam is faked, and each process records what it did to the events file.
const (
	termPaneProcessName = "TestResumeTermPaneProcess"
	termPaneDirEnv      = "PORTAL_TEST_TERM_PANE_DIR"
	termPaneBinEnv      = "PORTAL_TEST_TERM_PANE_BIN"
	termPaneHookEnv     = "PORTAL_TEST_TERM_PANE_HOOK"
	termPaneHoldDrawEnv = "PORTAL_TEST_TERM_PANE_HOLD_DRAW"

	// termPaneHoldCatchEnv names one process to hold before it installs its
	// SIGTERM catch, as "<kind> <n>": the nth draw or waiter of the pane.
	termPaneHoldCatchEnv = "PORTAL_TEST_TERM_PANE_HOLD_CATCH"
)

const (
	termEventDraw      = "draw"
	termEventWait      = "wait"
	termEventCleared   = "cleared"
	termEventRecovered = "recovered"
	termEventFailed    = "failed"

	// A process records its pre-catch event, under its own kind, as it starts.
	termEventPreCatch = "precatch-"
)

const termPaneWait = 10 * time.Second

// termPaneSettle is how long a process that caught a signal is watched for an
// exit it should not take.
const termPaneSettle = 300 * time.Millisecond

// stubTermPaneExe stands in for the portal binary the parked chain runs: its
// draw is this test binary's pane process, and its recovery tail records that
// it ran.
const stubTermPaneExe = `#!/bin/sh
case "$2" in
resume-draw)
	exec "$` + termPaneBinEnv + `" -test.run='^` + termPaneProcessName + `$' -- "$0" "$@"
	;;
resume-recover)
	echo ` + termEventRecovered + ` >> "$` + termPaneDirEnv + `/events"
	;;
esac
`

// stubTermPaneTmux answers the parked chain's pending-marker read as the pane
// stands, set until something has cleared it, and records the chain's own clear.
const stubTermPaneTmux = `#!/bin/sh
case "$*" in
display-message*@portal-resume-pending*)
	if grep -qx ` + termEventCleared + ` "$` + termPaneDirEnv + `/events" 2>/dev/null; then echo; else echo 1; fi
	;;
set-option*@portal-resume-pending*)
	echo ` + termEventCleared + ` >> "$` + termPaneDirEnv + `/events"
	;;
esac
`

// TestResumeTermPaneProcess is not a test: it is the body of the draw and the
// waiter the harness runs, and does nothing unless started as one.
func TestResumeTermPaneProcess(t *testing.T) {
	dir := os.Getenv(termPaneDirEnv)
	if dir == "" {
		return
	}
	pane := termPaneProcess{t: t, dir: dir}

	withFuncSeam(t, &resumeDrawRunFunc, func(cfg resumeDrawConfig) error {
		return runResumeDraw(pane.drawConfig(cfg))
	})
	withFuncSeam(t, &resumeWaitRunFunc, func(cfg resumeWaitConfig) error {
		return runResumeWait(pane.waitConfig(cfg))
	})

	resetRootCmd()
	rootCmd.SetArgs(termPaneChainArgs()[1:])
	err := rootCmd.Execute()
	pane.record(fmt.Sprintf("%s %v", termEventFailed, err))
	t.Fatalf("the pane process returned instead of handing the pane on: %v", err)
}

// termPaneChainArgs is the chain argv the process was started with, which the
// stub executable passes after the test binary's own flags.
func termPaneChainArgs() []string {
	for i, arg := range os.Args {
		if arg == "--" {
			return os.Args[i+1:]
		}
	}
	return nil
}

type termPaneProcess struct {
	t   *testing.T
	dir string
}

func (p termPaneProcess) record(event string) {
	f, err := os.OpenFile(filepath.Join(p.dir, "events"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintln(f, event)
	_ = f.Close()
}

func (p termPaneProcess) recordSelf(event string) {
	p.record(event + " " + strconv.Itoa(os.Getpid()))
}

// size reads the pane size the harness last set, so a resize is a file write
// followed by a SIGWINCH.
func (p termPaneProcess) size() (int, int, error) {
	raw, err := os.ReadFile(filepath.Join(p.dir, "size"))
	if errors.Is(err, os.ErrNotExist) {
		return 80, 24, nil
	}
	if err != nil {
		return 0, 0, err
	}
	var w, h int
	if _, err := fmt.Sscan(string(raw), &w, &h); err != nil {
		return 0, 0, err
	}
	return w, h, nil
}

// execSelf replaces the process image as production does, routing a hand-off
// to the next subcommand back into this test binary.
func (p termPaneProcess) execSelf(prog string, args []string) {
	if len(args) > 2 && args[1] == "state" {
		bin := os.Getenv(termPaneBinEnv)
		args = append([]string{bin, "-test.run=^" + termPaneProcessName + "$", "--"}, args...)
		prog = bin
	}
	err := syscall.Exec(prog, args, os.Environ())
	p.record(fmt.Sprintf("%s exec %s: %v", termEventFailed, prog, err))
	p.t.Fatalf("exec %s: %v", prog, err)
}

func (p termPaneProcess) drawConfig(cfg resumeDrawConfig) resumeDrawConfig {
	th := themetest.DefaultDark(p.t)
	cfg.Size = p.size
	cfg.DisableEcho = func() error { return nil }
	cfg.DropInputQueue = func() error { return nil }
	cfg.ResolveTheme = func(bool, func() error) (theme.Theme, error) {
		p.recordSelf(termEventDraw)
		p.holdDraw()
		return th, nil
	}
	cfg.ExecSelf = p.execSelf
	cfg.CatchSIGTERM = p.holdingCatch(termEventDraw, cfg.CatchSIGTERM)
	return cfg
}

// holdingCatch installs catch once the harness has had its chance to hold this
// process in the moment before it, when a SIGTERM still ends it.
func (p termPaneProcess) holdingCatch(kind string, catch func()) func() {
	return func() {
		p.recordSelf(termEventPreCatch + kind)
		if p.holdsCatch(kind) {
			time.Sleep(termPaneWait)
		}
		catch()
	}
}

func (p termPaneProcess) holdsCatch(kind string) bool {
	var want string
	var n int
	if _, err := fmt.Sscan(os.Getenv(termPaneHoldCatchEnv), &want, &n); err != nil || want != kind {
		return false
	}
	return len(termPane{dir: p.dir}.pidsOf(termEventPreCatch+kind)) == n
}

// holdDraw keeps a draw mid-paint until the harness releases it, once: a later
// draw of the same pane paints straight through.
func (p termPaneProcess) holdDraw() {
	if os.Getenv(termPaneHoldDrawEnv) == "" {
		return
	}
	release := filepath.Join(p.dir, "release-draw")
	deadline := time.Now().Add(termPaneWait)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(release); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (p termPaneProcess) waitConfig(cfg resumeWaitConfig) resumeWaitConfig {
	cfg.IsTerminal = func() bool { return true }
	cfg.MakeRaw = func() (func(), error) { return func() {}, nil }
	cfg.In = &announcingReader{inner: cfg.In, announce: func() { p.recordSelf(termEventWait) }}
	cfg.Size = p.size
	cfg.ClearMarker = func() error {
		p.record(termEventCleared)
		return nil
	}
	cfg.LookupResume = func(string) (hooks.OnResume, error) {
		return hooks.OnResume{Command: os.Getenv(termPaneHookEnv), Found: true}, nil
	}
	cfg.DiscardRegistration = func(string, string) (bool, error) {
		return false, errors.New("the harness answers no discard")
	}
	cfg.EnableTTYSignals = func() error { return nil }
	cfg.EnableEcho = func() error { return nil }
	cfg.AltScreen = altScreenPin{
		AlternateOn: func() (string, error) { return "0", nil },
		Unpin:       func() error { return nil },
		Pause:       func(time.Duration) {},
	}
	cfg.ExecSelf = p.execSelf
	cfg.CatchSIGTERM = p.holdingCatch(termEventWait, cfg.CatchSIGTERM)
	return cfg
}

// announcingReader reports the waiter's first read, the point it is holding the
// pane on its panel.
type announcingReader struct {
	inner    io.Reader
	announce func()
	once     sync.Once
}

func (r *announcingReader) Read(b []byte) (int, error) {
	r.once.Do(r.announce)
	return r.inner.Read(b)
}

// termPane is a waiting pane: the parked chain, started as restore leaves it.
type termPane struct {
	dir    string
	parked int
	stdin  io.WriteCloser
	exited chan struct{}

	// ended holds how the parked chain ended, once exited is closed.
	ended *termPaneEnd
}

type termPaneEnd struct {
	state *os.ProcessState
}

type termPaneOpts struct {
	holdDraw bool

	// holdCatch is the termPaneHoldCatchEnv value, empty to hold none.
	holdCatch string

	// tty, when set, is the pane's terminal: the parked chain leads a session
	// with it as its controlling terminal, as a tmux pane's process does, and
	// the test holds no stdin.
	tty *os.File
}

func startTermPane(t *testing.T, opts termPaneOpts) termPane {
	t.Helper()
	dir := t.TempDir()
	exe := stageStub(t, dir, "portal", stubTermPaneExe)
	hook := stageStub(t, dir, "hook", stubHookProgram)
	bin, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve the test binary: %v", err)
	}
	// The re-executed test binary's TestMain makes a temp home it never removes
	// once its image is replaced, so its temp root is one this test removes.
	tmp := filepath.Join(dir, "tmp")
	if err := os.Mkdir(tmp, 0o700); err != nil {
		t.Fatalf("stage the pane's temp root: %v", err)
	}
	stubBin := filepath.Join(dir, "bin")
	if err := os.Mkdir(stubBin, 0o700); err != nil {
		t.Fatalf("stage the pane's stub bin dir: %v", err)
	}
	stageStub(t, stubBin, "tmux", stubTermPaneTmux)

	chain := exec.Command("/bin/sh", "-c", parkedResumeChain(exe, samplePayload()))
	chain.Env = append(os.Environ(),
		termPaneDirEnv+"="+dir,
		termPaneBinEnv+"="+bin,
		termPaneHookEnv+"="+shellquote.Single(hook),
		"HOOK_DIR="+dir,
		"SHELL="+stageStub(t, dir, "user-shell", stubUserShell),
		"TMPDIR="+tmp,
		"PATH="+stubBin+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	if opts.holdDraw {
		chain.Env = append(chain.Env, termPaneHoldDrawEnv+"=1")
	}
	if opts.holdCatch != "" {
		chain.Env = append(chain.Env, termPaneHoldCatchEnv+"="+opts.holdCatch)
	}
	stderr, err := os.Create(filepath.Join(dir, "stderr"))
	if err != nil {
		t.Fatalf("stage the pane's stderr: %v", err)
	}
	chain.Stderr = stderr
	var stdin io.WriteCloser
	if opts.tty != nil {
		chain.Stdin, chain.Stdout = opts.tty, opts.tty
		chain.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	} else {
		chain.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if stdin, err = chain.StdinPipe(); err != nil {
			t.Fatalf("open the pane's stdin: %v", err)
		}
	}
	if err := chain.Start(); err != nil {
		t.Fatalf("start the parked chain: %v", err)
	}
	_ = stderr.Close()

	p := termPane{dir: dir, parked: chain.Process.Pid, stdin: stdin, exited: make(chan struct{}), ended: &termPaneEnd{}}
	go func() {
		_ = chain.Wait()
		p.ended.state = chain.ProcessState
		close(p.exited)
	}()
	t.Cleanup(func() {
		_ = syscall.Kill(-chain.Process.Pid, syscall.SIGKILL)
		<-p.exited
	})
	return p
}

func (p termPane) events() []string {
	raw, _ := os.ReadFile(filepath.Join(p.dir, "events"))
	return strings.FieldsFunc(string(raw), func(r rune) bool { return r == '\n' })
}

// pidsOf answers the pid each recorded event of kind named, in order.
func (p termPane) pidsOf(kind string) []int {
	var pids []int
	for _, event := range p.events() {
		name, pid, ok := strings.Cut(event, " ")
		if !ok || name != kind {
			continue
		}
		if n, err := strconv.Atoi(pid); err == nil {
			pids = append(pids, n)
		}
	}
	return pids
}

func (p termPane) recorded(kind string) bool {
	for _, event := range p.events() {
		if event == kind || strings.HasPrefix(event, kind+" ") {
			return true
		}
	}
	return false
}

func (p termPane) diagnostics() string {
	stderr, _ := os.ReadFile(filepath.Join(p.dir, "stderr"))
	return fmt.Sprintf("events %q, stderr %q", p.events(), stderr)
}

// awaitEvent waits for the nth event of kind, answering the pid it named.
func (p termPane) awaitEvent(t *testing.T, kind string, n int) int {
	t.Helper()
	var pids []int
	if !harnesstest.PollUntil(t, termPaneWait, 10*time.Millisecond, func() bool {
		pids = p.pidsOf(kind)
		return len(pids) >= n || p.recorded(termEventFailed)
	}) || len(pids) < n {
		t.Fatalf("the pane never recorded %s #%d; %s", kind, n, p.diagnostics())
	}
	return pids[n-1]
}

func (p termPane) signal(t *testing.T, pid int, sig syscall.Signal) {
	t.Helper()
	if err := syscall.Kill(pid, sig); err != nil {
		t.Fatalf("send %v to pid %d: %v", sig, pid, err)
	}
}

// assertStillWaiting checks the pane is left as a SIGTERM must leave it: the
// process holding it is still up, its marker was never cleared, and the parked
// chain never reached its recovery tail.
func (p termPane) assertStillWaiting(t *testing.T, pid int) {
	t.Helper()
	select {
	case <-p.exited:
		t.Fatalf("the parked chain ended; %s", p.diagnostics())
	case <-time.After(termPaneSettle):
	}
	if !processAlive(pid) {
		t.Errorf("the process holding the pane (pid %d) ended; %s", pid, p.diagnostics())
	}
	if p.recorded(termEventCleared) {
		t.Errorf("the pending marker was cleared; %s", p.diagnostics())
	}
	if p.recorded(termEventRecovered) {
		t.Errorf("the recovery tail ran; %s", p.diagnostics())
	}
}

func (p termPane) press(t *testing.T, key string) {
	t.Helper()
	if _, err := io.WriteString(p.stdin, key); err != nil {
		t.Fatalf("type %q into the pane: %v", key, err)
	}
}

func (p termPane) resize(t *testing.T, pid, width, height int) {
	t.Helper()
	size := fmt.Sprintf("%d %d", width, height)
	if err := os.WriteFile(filepath.Join(p.dir, "size"), []byte(size), 0o600); err != nil {
		t.Fatalf("resize the pane: %v", err)
	}
	p.signal(t, pid, syscall.SIGWINCH)
}

func (p termPane) readPID(t *testing.T, name, what string) int {
	t.Helper()
	pane := hookPane{dir: p.dir}
	return pane.awaitPID(t, name, what)
}

// assertEndsOnSIGTERM checks pid ends on a SIGTERM, which a process does only
// at the default disposition.
func assertEndsOnSIGTERM(t *testing.T, pid int, what string) {
	t.Helper()
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		t.Fatalf("signal %s: %v", what, err)
	}
	if !harnesstest.PollUntil(t, hookShellWait, 10*time.Millisecond, func() bool { return !processAlive(pid) }) {
		t.Fatalf("%s survived SIGTERM: it inherited the signal as ignored", what)
	}
}

func TestResumeWaitingPane_SIGTERM(t *testing.T) {
	t.Run("a waiter keeps the pane waiting through a SIGTERM", func(t *testing.T) {
		p := startTermPane(t, termPaneOpts{})
		waiter := p.awaitEvent(t, termEventWait, 1)

		p.signal(t, waiter, syscall.SIGTERM)

		p.assertStillWaiting(t, waiter)
	})

	t.Run("a draw keeps the pane waiting through a SIGTERM landing mid-draw", func(t *testing.T) {
		p := startTermPane(t, termPaneOpts{holdDraw: true})
		draw := p.awaitEvent(t, termEventDraw, 1)

		p.signal(t, draw, syscall.SIGTERM)
		p.assertStillWaiting(t, draw)

		if err := os.WriteFile(filepath.Join(p.dir, "release-draw"), nil, 0o600); err != nil {
			t.Fatalf("release the draw: %v", err)
		}
		if waiter := p.awaitEvent(t, termEventWait, 1); waiter != draw {
			t.Fatalf("the waiter runs as pid %d, want the draw's own process %d", waiter, draw)
		}
		p.assertStillWaiting(t, draw)
	})

	t.Run("a waiter redrawn after a resize keeps the pane waiting through a SIGTERM", func(t *testing.T) {
		p := startTermPane(t, termPaneOpts{})
		waiter := p.awaitEvent(t, termEventWait, 1)

		p.resize(t, waiter, 120, 40)
		if redraw := p.awaitEvent(t, termEventDraw, 2); redraw != waiter {
			t.Fatalf("the redraw runs as pid %d, want the waiter's own process %d", redraw, waiter)
		}
		redrawn := p.awaitEvent(t, termEventWait, 2)

		p.signal(t, redrawn, syscall.SIGTERM)

		p.assertStillWaiting(t, redrawn)
	})

	t.Run("a pane answered after its waiter caught a SIGTERM hands its hook and shell a default SIGTERM", func(t *testing.T) {
		p := startTermPane(t, termPaneOpts{})
		waiter := p.awaitEvent(t, termEventWait, 1)
		p.signal(t, waiter, syscall.SIGTERM)
		p.assertStillWaiting(t, waiter)

		p.press(t, "\r")

		hook := p.readPID(t, "hook.pid", "the hook program")
		assertEndsOnSIGTERM(t, hook, "the hook program")
		shell := p.readPID(t, "shell.pid", "the user's shell")
		if shell != waiter {
			t.Fatalf("the user's shell runs as pid %d, want the pane's own process %d", shell, waiter)
		}
		assertEndsOnSIGTERM(t, shell, "the user's shell")
	})

	t.Run("a waiter still ends on SIGHUP", func(t *testing.T) {
		p := startTermPane(t, termPaneOpts{})
		waiter := p.awaitEvent(t, termEventWait, 1)

		p.signal(t, waiter, syscall.SIGHUP)

		if !harnesstest.PollUntil(t, termPaneWait, 10*time.Millisecond, func() bool { return !processAlive(waiter) }) {
			t.Fatalf("the waiter survived SIGHUP; %s", p.diagnostics())
		}
	})

	t.Run("a draw still ends on SIGHUP", func(t *testing.T) {
		p := startTermPane(t, termPaneOpts{holdDraw: true})
		draw := p.awaitEvent(t, termEventDraw, 1)

		p.signal(t, draw, syscall.SIGHUP)

		if !harnesstest.PollUntil(t, termPaneWait, 10*time.Millisecond, func() bool { return !processAlive(draw) }) {
			t.Fatalf("the draw survived SIGHUP; %s", p.diagnostics())
		}
	})
}
