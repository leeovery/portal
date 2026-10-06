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
		HookKey:   "work:0.0",
		OpenFIFO:  openFIFOWithTimeout,
		HookStore: hydrateStoreWithMode(t, "work:0.0", command, resumemode.Eager),
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
	dir    string
	pid    int
	exited chan *os.ProcessState
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
	if err := pane.Start(); err != nil {
		t.Fatalf("start the pane's shell: %v", err)
	}
	p := hookPane{dir: dir, pid: pane.Process.Pid, exited: make(chan *os.ProcessState, 1)}
	go func() {
		_ = pane.Wait()
		p.exited <- pane.ProcessState
	}()
	t.Cleanup(func() {
		_ = syscall.Kill(-p.pid, syscall.SIGKILL)
	})
	return p
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

func (p hookPane) assertStillUp(t *testing.T, settle time.Duration) {
	t.Helper()
	select {
	case st := <-p.exited:
		p.exited <- st
		t.Fatalf("the pane's shell ended (%v), want it still up", st)
	case <-time.After(settle):
	}
}

// assertUserShellEndsOnSIGTERM checks the pane is now the user's shell, then
// that SIGTERM ends it, which it does only at the default disposition.
func (p hookPane) assertUserShellEndsOnSIGTERM(t *testing.T) {
	t.Helper()
	shellPID := p.awaitPID(t, "shell.pid", "the user's shell")
	if shellPID != p.pid {
		t.Fatalf("the user's shell runs as pid %d, want the pane's own process %d", shellPID, p.pid)
	}
	if err := syscall.Kill(p.pid, syscall.SIGTERM); err != nil {
		t.Fatalf("signal the user's shell: %v", err)
	}
	select {
	case st := <-p.exited:
		ws, ok := st.Sys().(syscall.WaitStatus)
		if !ok || !ws.Signaled() || ws.Signal() != syscall.SIGTERM {
			t.Errorf("the user's shell ended %v, want killed by SIGTERM at its default disposition", st)
		}
	case <-time.After(hookShellWait):
		t.Fatal("the user's shell survived SIGTERM: it inherited the signal as ignored")
	}
}

func processAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

func TestResumeHookShell_SurvivesSIGTERMWhileTheHookRuns(t *testing.T) {
	for _, route := range hookShellRoutes() {
		t.Run(route.name+" keeps its pane and hook program through a SIGTERM to its shell", func(t *testing.T) {
			p := startHookPane(t, route)
			hookPID := p.awaitPID(t, "hook.pid", "the hook program")

			if err := syscall.Kill(p.pid, syscall.SIGTERM); err != nil {
				t.Fatalf("signal the pane's shell: %v", err)
			}
			p.assertStillUp(t, 300*time.Millisecond)
			if !processAlive(hookPID) {
				t.Fatal("the hook program ended with the SIGTERM to its shell")
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

			if err := syscall.Kill(hookPID, syscall.SIGTERM); err != nil {
				t.Fatalf("signal the hook program: %v", err)
			}
			if !harnesstest.PollUntil(t, hookShellWait, 10*time.Millisecond, func() bool { return !processAlive(hookPID) }) {
				t.Fatal("the hook program survived SIGTERM: it inherited the signal as ignored")
			}
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

			if err := syscall.Kill(p.pid, syscall.SIGHUP); err != nil {
				t.Fatalf("signal the pane's shell: %v", err)
			}
			select {
			case st := <-p.exited:
				ws, ok := st.Sys().(syscall.WaitStatus)
				if !ok || !ws.Signaled() || ws.Signal() != syscall.SIGHUP {
					t.Errorf("the pane's shell ended %v, want killed by SIGHUP", st)
				}
			case <-time.After(hookShellWait):
				t.Fatal("the pane's shell survived SIGHUP")
			}
		})
	}
}
