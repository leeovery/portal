package runner_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"os"
	"reflect"
	"regexp"
	"strconv"
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

func newTerminal(t *testing.T) *terminal {
	t.Helper()
	user, tty, err := pty.Open()
	if err != nil {
		t.Fatalf("open the user's terminal: %v", err)
	}
	if err := pty.Setsize(user, &pty.Winsize{Cols: 80, Rows: 24}); err != nil {
		t.Fatalf("size the user's terminal: %v", err)
	}
	tm := &terminal{user: user, tty: tty, done: make(chan struct{})}
	go tm.record()
	t.Cleanup(func() {
		_ = tty.Close()
		select {
		case <-tm.done:
		case <-time.After(settle):
			t.Error("the user's terminal was still being read after its tty closed")
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
	if !harnesstest.PollUntil(t, settle, 20*time.Millisecond, func() bool {
		if !unread(tm.tty) {
			return false
		}
		time.Sleep(50 * time.Millisecond)
		return unread(tm.tty)
	}) {
		t.Fatal("the runner never stopped taking the terminal's input")
	}
}

func unread(f *os.File) bool {
	fds := []unix.PollFd{{Fd: int32(f.Fd()), Events: unix.POLLIN}}
	n, err := unix.Poll(fds, 0)
	return err == nil && n > 0
}

func (tm *terminal) state(t *testing.T) *term.State {
	t.Helper()
	state, err := term.GetState(tm.tty.Fd())
	if err != nil {
		t.Fatalf("read the terminal's settings: %v", err)
	}
	return state
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

type result struct {
	out runner.Outcome
	err error
}

// harness runs runner.Run on a terminal of its own.
type harness struct {
	term   *terminal
	before *term.State
	result chan result
	// pid is the fake child's, once it is ready. Run's goroutine reads it
	// too, in a fake kill.
	pid atomic.Int64
}

// attach starts runner.Run on a fake child in mode, scanning for keys, with
// each of opts applied to its config. Its context ends with the test, so a
// test that stops early still ends the attach.
func attach(t *testing.T, mode string, opts ...func(*runner.Config)) *harness {
	t.Helper()
	return attachWithin(t, t.Context(), mode, opts...)
}

func attachWithin(t *testing.T, ctx context.Context, mode string, opts ...func(*runner.Config)) *harness {
	t.Helper()
	return attachCommand(t, ctx, fakeChildCommand(t, mode), opts...)
}

// attachCommand starts runner.Run on command, for a session named fake
// unless opts say otherwise.
func attachCommand(t *testing.T, ctx context.Context, command backend.Command, opts ...func(*runner.Config)) *harness {
	t.Helper()
	tm := newTerminal(t)
	h := &harness{term: tm, before: tm.state(t), result: make(chan result, 1)}
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
		opt(&cfg)
	}
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

// finish waits for Run to return, and checks it restored the terminal.
func (h *harness) finish(t *testing.T) (runner.Outcome, error) {
	t.Helper()
	select {
	case r := <-h.result:
		h.result <- r
		if after := h.term.state(t); !reflect.DeepEqual(after, h.before) {
			t.Errorf("the terminal's settings are %+v after the attach, want them restored to %+v", after, h.before)
		}
		return r.out, r.err
	case <-time.After(settle):
		t.Fatalf("Run did not return; the terminal shows %q", h.term.output())
		return runner.Outcome{}, nil
	}
}
