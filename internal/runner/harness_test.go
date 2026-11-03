package runner_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"os"
	"reflect"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/creack/pty"
	"github.com/leeovery/portal/internal/attachkeys"
	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/runner"
	"golang.org/x/sys/unix"
)

// settle bounds each wait of a test for the runner or its child.
const settle = 10 * time.Second

// shortGrace stands in for the runner's grace where a test waits it out: long
// enough for a signalled child to act on its signal under load.
const shortGrace = 500 * time.Millisecond

// terminalReset is what zmx writes to the terminal on its way out, and what
// the runner writes in its place when zmx could not.
const terminalReset = "\x1bc\x1b]110\x1b\\\x1b]111\x1b\\\x1b]112\x1b\\"

// Portal's keys as the fake scanner knows them: Alt+D, Alt+Shift+D and Alt+Q
// in their legacy escape forms.
const (
	keyDetach        = "\x1bd"
	keyDetachToShell = "\x1bD"
	keyKill          = "\x1bq"
)

// keys is a fake Scanner binding the keys above.
var keys = scanner{
	keyDetach:        attachkeys.Detach,
	keyDetachToShell: attachkeys.DetachToShell,
	keyKill:          attachkeys.Kill,
}

type scanner map[string]attachkeys.Action

func (s scanner) Scan(read []byte) ([]byte, attachkeys.Action, bool) {
	first, action := -1, attachkeys.Action(0)
	for key, a := range s {
		if i := bytes.Index(read, []byte(key)); i >= 0 && (first < 0 || i < first) {
			first, action = i, a
		}
	}
	if first < 0 {
		return read, 0, false
	}
	return read[:first], action, true
}

// panicking is a Scanner that panics, as a bug in a copy loop would.
type panicking struct{}

func (panicking) Scan([]byte) ([]byte, attachkeys.Action, bool) { panic("scanner exploded") }

// terminal is a pseudo-terminal standing in for the user's: the runner gets
// its tty side, and the test types into and reads from its user side.
type terminal struct {
	user, tty *os.File
	mu        sync.Mutex
	seen      []byte
	done      chan struct{}
}

// newTerminal opens a terminal whose output the test reads as it comes.
func newTerminal(t *testing.T) *terminal {
	t.Helper()
	tm := openTerminal(t)
	tm.done = make(chan struct{})
	go tm.record()
	return tm
}

// openTerminal opens a terminal whose output nobody reads, as a terminal
// over a stalled network takes none.
func openTerminal(t *testing.T) *terminal {
	t.Helper()
	user, tty, err := pty.Open()
	if err != nil {
		t.Fatalf("open the user's terminal: %v", err)
	}
	if err := pty.Setsize(user, &pty.Winsize{Cols: 80, Rows: 24}); err != nil {
		t.Fatalf("size the user's terminal: %v", err)
	}
	// Blocking, as a shell's terminal is.
	if err := unix.SetNonblock(fdOf(t, tty), false); err != nil {
		t.Fatalf("make the user's terminal blocking: %v", err)
	}
	tm := &terminal{user: user, tty: tty}
	t.Cleanup(func() {
		_ = tty.Close()
		if tm.done != nil {
			select {
			case <-tm.done:
			case <-time.After(settle):
				t.Error("the user's terminal was still being read after its tty closed")
			}
		}
		_ = user.Close()
	})
	return tm
}

func (tm *terminal) record() {
	defer close(tm.done)
	buf := make([]byte, 4096)
	for {
		n, err := tm.user.Read(buf)
		tm.mu.Lock()
		tm.seen = append(tm.seen, buf[:n]...)
		tm.mu.Unlock()
		if err != nil {
			return
		}
	}
}

// output is everything the terminal has shown.
func (tm *terminal) output() []byte {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	return bytes.Clone(tm.seen)
}

func (tm *terminal) typeIn(t *testing.T, input string) {
	t.Helper()
	if _, err := tm.user.Write([]byte(input)); err != nil {
		t.Fatalf("type %q: %v", input, err)
	}
}

// await waits for the terminal to show want.
func (tm *terminal) await(t *testing.T, want string) {
	t.Helper()
	if !harnesstest.PollUntil(t, settle, 5*time.Millisecond, func() bool {
		return bytes.Contains(tm.output(), []byte(want))
	}) {
		t.Fatalf("the terminal never showed %q; it shows %q", want, tm.output())
	}
}

// awaitBacklog waits for typed input to stay unread on the terminal, as it
// does once the runner stops taking it.
func (tm *terminal) awaitBacklog(t *testing.T) {
	t.Helper()
	tty := fdOf(t, tm.tty)
	if !harnesstest.PollUntil(t, settle, 20*time.Millisecond, func() bool {
		if !unread(tty) {
			return false
		}
		time.Sleep(50 * time.Millisecond)
		return unread(tty)
	}) {
		t.Fatal("the runner never stopped taking the terminal's input")
	}
}

// next reads what typed input waits on the terminal for its next reader.
func (tm *terminal) next(t *testing.T) string {
	t.Helper()
	tty := fdOf(t, tm.tty)
	if n, err := unix.Poll([]unix.PollFd{{Fd: int32(tty), Events: unix.POLLIN}}, int(time.Second.Milliseconds())); n == 0 || err != nil {
		return ""
	}
	buf := make([]byte, 64)
	n, _ := unix.Read(tty, buf)
	return string(buf[:max(n, 0)])
}

var readLine = regexp.MustCompile(`in:([0-9a-f]*)\r+\n`)

// reads is each read of its input the fake child reported, in order.
func (tm *terminal) reads(t *testing.T) []string {
	t.Helper()
	var reads []string
	for _, m := range readLine.FindAllSubmatch(tm.output(), -1) {
		b, err := hex.DecodeString(string(m[1]))
		if err != nil {
			t.Fatalf("decode the child's read %q: %v", m[1], err)
		}
		reads = append(reads, string(b))
	}
	return reads
}

// unread reports whether input waits on the terminal fd.
func unread(fd int) bool {
	n, err := unix.Poll([]unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}, 0)
	return err == nil && n > 0
}

// fdOf is f's descriptor, taken without the blocking mode os.File.Fd would
// set on the description the runner made non-blocking.
func fdOf(t *testing.T, f *os.File) int {
	t.Helper()
	raw, err := f.SyscallConn()
	if err != nil {
		t.Fatalf("take %s's descriptor: %v", f.Name(), err)
	}
	fd := -1
	if err := raw.Control(func(u uintptr) { fd = int(u) }); err != nil {
		t.Fatalf("take %s's descriptor: %v", f.Name(), err)
	}
	return fd
}

func stateOf(t *testing.T, f *os.File) *term.State {
	t.Helper()
	state, err := term.GetState(uintptr(fdOf(t, f)))
	if err != nil {
		t.Fatalf("read %s's settings: %v", f.Name(), err)
	}
	return state
}

// nonblockingOf reports whether f's open file description is non-blocking.
func nonblockingOf(t *testing.T, f *os.File) bool {
	t.Helper()
	flags, err := unix.FcntlInt(uintptr(fdOf(t, f)), unix.F_GETFL, 0)
	if err != nil {
		t.Fatalf("read %s's flags: %v", f.Name(), err)
	}
	return flags&unix.O_NONBLOCK != 0
}

type result struct {
	out runner.Outcome
	err error
}

// harness runs runner.Run on a terminal of its own.
type harness struct {
	term *terminal
	// in is the runner's input, and before its settings before the attach.
	in     *os.File
	before *term.State
	result chan result
	// pid is the fake child's, once it is ready. Run's goroutine reads it
	// too, in a fake kill.
	pid atomic.Int64
}

// option changes an attach's config. It is handed the attach's harness, made
// before Run starts, so a fake kill can reach the child through it.
type option func(h *harness, cfg *runner.Config)

// attach starts runner.Run on a fake child in mode, scanning for keys, with
// each of opts applied to its config. Its context ends with the test, so a
// test that stops early still ends the attach.
func attach(t *testing.T, mode string, opts ...option) *harness {
	t.Helper()
	return attachWithin(t, t.Context(), mode, opts...)
}

func attachWithin(t *testing.T, ctx context.Context, mode string, opts ...option) *harness {
	t.Helper()
	return attachOn(t, ctx, newTerminal(t), fakeChildCommand(t, mode), opts...)
}

// attachOn starts runner.Run on command and the terminal tm, for a session
// named fake unless opts say otherwise.
func attachOn(t *testing.T, ctx context.Context, tm *terminal, command backend.Command, opts ...option) *harness {
	t.Helper()
	h := &harness{term: tm, result: make(chan result, 1)}
	cfg := runner.Config{
		Session: "fake",
		Command: command,
		In:      tm.tty,
		Out:     tm.tty,
		Scanner: keys,
		Kill: func(context.Context, string) error {
			t.Error("the session was killed")
			return nil
		},
	}
	for _, opt := range opts {
		opt(h, &cfg)
	}
	h.in, h.before = cfg.In, stateOf(t, cfg.In)
	go func() {
		out, err := runner.Run(ctx, cfg)
		h.result <- result{out, err}
	}()
	t.Cleanup(func() {
		select {
		case r := <-h.result:
			h.result <- r
		case <-time.After(settle):
			t.Error("Run outlived the test")
		}
	})
	return h
}

func fakeChildCommand(t *testing.T, mode string) backend.Command {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("find the test binary: %v", err)
	}
	return backend.Command{Argv: []string{exe}, Env: append(os.Environ(), childModeEnv+"="+mode)}
}

var readyLine = regexp.MustCompile(`ready:(\d+):(\d+x\d+)\r+\n`)

// ready waits for the fake child to start, and returns its terminal's size.
func (h *harness) ready(t *testing.T) string {
	t.Helper()
	var m [][]byte
	if !harnesstest.PollUntil(t, settle, 5*time.Millisecond, func() bool {
		m = readyLine.FindSubmatch(h.term.output())
		return m != nil
	}) {
		t.Fatalf("the fake child never started; the terminal shows %q", h.term.output())
	}
	pid, _ := strconv.Atoi(string(m[1]))
	h.pid.Store(int64(pid))
	return string(m[2])
}

// child is the fake child's pid, once it is ready.
func (h *harness) child() int { return int(h.pid.Load()) }

// detach ends the attach with the detach key, failing the test unless that
// ends it cleanly.
func (h *harness) detach(t *testing.T) {
	t.Helper()
	h.term.typeIn(t, keyDetach)
	if out, err := h.finish(t); err != nil || out != (runner.Outcome{Action: attachkeys.Detach}) {
		t.Errorf("Run = %+v, %v; want a detach", out, err)
	}
}

// finish waits for Run to return, and checks it restored the terminal and
// left no copy loop running.
func (h *harness) finish(t *testing.T) (runner.Outcome, error) {
	t.Helper()
	select {
	case r := <-h.result:
		h.result <- r
		if after := stateOf(t, h.in); !reflect.DeepEqual(after, h.before) {
			t.Errorf("the terminal's settings are %+v after the attach, want them restored to %+v", after, h.before)
		}
		if nonblockingOf(t, h.in) {
			t.Errorf("the terminal is non-blocking after the attach, want it blocking again")
		}
		assertNoCopyLoops(t)
		return r.out, r.err
	case <-time.After(settle):
		t.Fatalf("Run did not return; the terminal shows %q", h.term.output())
		return runner.Outcome{}, nil
	}
}

// assertNoCopyLoops fails the test if a copy loop of the runner's outlives
// Run. One that has closed its done channel is let finish returning.
func assertNoCopyLoops(t *testing.T) {
	t.Helper()
	var stacks string
	if !harnesstest.PollUntil(t, time.Second, 10*time.Millisecond, func() bool {
		buf := make([]byte, 1<<20)
		stacks = string(buf[:runtime.Stack(buf, true)])
		return !strings.Contains(stacks, "runner.(*attach).copy")
	}) {
		t.Errorf("a copy loop outlived Run:\n%s", stacks)
	}
}
