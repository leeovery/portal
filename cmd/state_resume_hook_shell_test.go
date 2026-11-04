package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/proctest"
	"github.com/leeovery/portal/internal/resumemode"
	"github.com/leeovery/portal/internal/shellquote"
)

// stubHookProgram records its pid and runs until a release file appears or the
// bound passes, so a test controls how long the hook program lives and a test
// binary ended before its cleanup leaves nothing running for long.
const stubHookProgram = `#!/bin/sh
echo $$ > "$HOOK_DIR/hook.pid"
n=0
while [ ! -e "$HOOK_DIR/release" ]; do
	[ $n -ge 600 ] && exit 1
	n=$((n+1))
	sleep 0.05
done
echo done > "$HOOK_DIR/hook.done"
`

// stubUserShell records its pid and becomes a process that keeps its inherited
// SIGTERM disposition: it ends on SIGTERM only when that disposition is the
// default.
const stubUserShell = `#!/bin/sh
echo $$ > "$HOOK_DIR/shell.pid"
exec sleep 30
`

const hookShellWait = 5 * time.Second

// hookShellRoute is one way a pane comes to run its resume hook: handed to the
// hook shell by the hydrate helper, or by the waiter on an answer.
type hookShellRoute struct {
	name string
	argv func(t *testing.T, command string) (prog string, args []string)
}

func hookShellRoutes() []hookShellRoute {
	return []hookShellRoute{
		{name: "a restored eager pane", argv: eagerHookArgv},
		{name: "a lazy pane answered on its panel", argv: answeredHookArgv},
	}
}

func eagerHookArgv(t *testing.T, command string) (string, []string) {
	t.Helper()
	dir := t.TempDir()
	fifo := makeFIFO(t, dir, "hydrate-work__0.0.fifo")
	scrollback := filepath.Join(dir, "sb")
	if err := os.WriteFile(scrollback, nil, 0o600); err != nil {
		t.Fatalf("stage the scrollback file: %v", err)
	}
	signalFIFOAsync(t, fifo)

	execStub := &stubExecShell{}
	cfg := hydrateCfg(t, hydrateCfgOpts{
		FIFO:      fifo,
		File:      scrollback,
		HookKey:   samplePayload().Session,
		OpenFIFO:  openFIFOWithTimeout,
		HookStore: hydrateStoreWithMode(t, samplePayload().Session, command, resumemode.Eager),
		ExecShell: execStub.fn(),
	})
	if err := runHydrate(cfg); err != nil {
		t.Fatalf("runHydrate: %v", err)
	}
	if !execStub.called {
		t.Fatal("the hydrate helper handed the pane to nothing")
	}
	return execStub.target, execStub.args
}

func answeredHookArgv(t *testing.T, command string) (string, []string) {
	t.Helper()
	var probe resumeWaitProbe
	probe.lookup = foundHook(command)
	answerEnter(t, &probe, samplePayload())
	if probe.execCalls != 1 {
		t.Fatalf("the waiter handed the pane on %d times, want 1", probe.execCalls)
	}
	return probe.execProg, probe.execArgs
}

// hookPane runs a route's hook shell as a pane's top process would, with a hook
// program and a user's shell standing in for the real ones.
type hookPane struct {
	dir  string
	pid  int
	held *proctest.Held
}

func startHookPane(t *testing.T, route hookShellRoute) hookPane {
	t.Helper()
	dir := t.TempDir()
	hook := stageStub(t, dir, "hook", stubHookProgram)
	t.Setenv("SHELL", stageStub(t, dir, "user-shell", stubUserShell))

	prog, args := route.argv(t, shellquote.Single(hook))

	pane := exec.Command(prog)
	pane.Args = args
	pane.Env = append(os.Environ(), "HOOK_DIR="+dir)
	pane.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	held := proctest.Start(t, pane, "HOOK_DIR="+dir)
	return hookPane{dir: dir, pid: held.PID(), held: held}
}

func stageStub(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
		t.Fatalf("stage %s: %v", name, err)
	}
	return path
}

func (p hookPane) readPID(name string) (int, bool) {
	raw, err := os.ReadFile(filepath.Join(p.dir, name))
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	return pid, err == nil
}

func (p hookPane) awaitPID(t *testing.T, name, what string) int {
	t.Helper()
	var pid int
	if !harnesstest.PollUntil(t, hookShellWait, 10*time.Millisecond, func() bool {
		var ok bool
		pid, ok = p.readPID(name)
		return ok
	}) {
		t.Fatalf("%s never started", what)
	}
	return pid
}

func (p hookPane) release(t *testing.T) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(p.dir, "release"), nil, 0o600); err != nil {
		t.Fatalf("release the hook program: %v", err)
	}
}

func (p hookPane) exists(name string) bool {
	_, err := os.Stat(filepath.Join(p.dir, name))
	return err == nil
}

// signal sends sig to pid once it is proved a process of the session's.
func (p hookPane) signal(t *testing.T, pid int, sig syscall.Signal) {
	t.Helper()
	if err := p.held.Signal(pid, sig); err != nil {
		t.Fatalf("send %v to pid %d: %v", sig, pid, err)
	}
}

func (p hookPane) assertStillUp(t *testing.T, settle time.Duration) {
	t.Helper()
	select {
	case <-p.held.Exited():
		t.Fatalf("the session's shell ended (%v), want it still up", p.held.Status())
	case <-time.After(settle):
	}
}

// endedOn waits for the session's shell to end and checks sig ended it.
func (p hookPane) endedOn(t *testing.T, sig syscall.Signal, what string) {
	t.Helper()
	if !endsWithin(p.held.Exited(), hookShellWait) {
		t.Fatalf("%s survived %v", what, sig)
	}
	if ws := p.held.Status(); !ws.Signaled() || ws.Signal() != sig {
		t.Errorf("%s ended %v, want killed by %v", what, ws, sig)
	}
}

// assertUserShellEndsOnSIGTERM checks the session is now the user's shell,
// then that SIGTERM ends it, which it does only at the default disposition.
func (p hookPane) assertUserShellEndsOnSIGTERM(t *testing.T) {
	t.Helper()
	shellPID := p.awaitPID(t, "shell.pid", "the user's shell")
	if shellPID != p.pid {
		t.Fatalf("the user's shell runs as pid %d, want the session's own process %d", shellPID, p.pid)
	}
	p.signal(t, p.pid, syscall.SIGTERM)
	p.endedOn(t, syscall.SIGTERM, "the user's shell, at its default disposition,")
}

func TestResumeHookShell_SurvivesSIGTERMWhileTheHookRuns(t *testing.T) {
	for _, route := range hookShellRoutes() {
		t.Run(route.name+" keeps its session and hook program through a SIGTERM to its shell", func(t *testing.T) {
			p := startHookPane(t, route)
			hookPID := p.awaitPID(t, "hook.pid", "the hook program")
			hookGone := p.held.Watch(t, hookPID)

			p.signal(t, p.pid, syscall.SIGTERM)

			p.assertStillUp(t, 300*time.Millisecond)
			select {
			case <-hookGone:
				t.Fatal("the hook program ended with the SIGTERM to its shell")
			default:
			}

			p.release(t)
			p.assertUserShellEndsOnSIGTERM(t)
			if !p.exists("hook.done") {
				t.Error("the hook program did not run to its end")
			}
		})
	}
}

func TestResumeHookShell_HookProgramEndsOnSIGTERM(t *testing.T) {
	for _, route := range hookShellRoutes() {
		t.Run(route.name+" hands on to the user's shell after its hook program ends on SIGTERM", func(t *testing.T) {
			p := startHookPane(t, route)
			hookPID := p.awaitPID(t, "hook.pid", "the hook program")

			assertEndsOnSIGTERM(t, p.held, hookPID, "the hook program")
			if p.exists("hook.done") {
				t.Error("the hook program ran to its end, want it ended by SIGTERM")
			}

			p.assertUserShellEndsOnSIGTERM(t)
		})
	}
}

func TestResumeHookShell_UserShellStartsWithDefaultSIGTERM(t *testing.T) {
	for _, route := range hookShellRoutes() {
		t.Run(route.name+" hands its user's shell a default SIGTERM disposition", func(t *testing.T) {
			p := startHookPane(t, route)
			p.awaitPID(t, "hook.pid", "the hook program")

			p.release(t)
			p.assertUserShellEndsOnSIGTERM(t)
		})
	}
}

func TestResumeHookShell_LeavesSIGHUPUncaught(t *testing.T) {
	for _, route := range hookShellRoutes() {
		t.Run(route.name+" ends on SIGHUP while its hook program runs", func(t *testing.T) {
			p := startHookPane(t, route)
			p.awaitPID(t, "hook.pid", "the hook program")

			p.signal(t, p.pid, syscall.SIGHUP)

			p.endedOn(t, syscall.SIGHUP, "the session's shell")
		})
	}
}
