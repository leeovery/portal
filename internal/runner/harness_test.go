package runner_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
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
	tm.record()
	return tm
}

// openTerminal opens a terminal whose output nobody reads until the test
// records it, as a terminal over a stalled network takes none.
func openTerminal(t *testing.T) *terminal {
	t.Helper()
	user, tty, err := pty.Open()
	if err != nil {
		t.Fatalf("open the user's terminal: %v", err)
	}
	if err := pty.Setsize(user, &pty.Winsize{Cols: 80, Rows: 24}); err != nil {
		t.Fatalf("size the user's terminal: %v", err)
	}
	blocking(t, tty)
	tm := &terminal{user: user, tty: tty}
	t.Cleanup(func() {
		// The user's side first: a tty side closing with output no one reads
		// would wait in close for it to be read, and a runner stuck writing to
		// the terminal is freed.
		_ = user.Close()
		_ = tty.Close()
		if tm.done != nil {
			select {
			case <-tm.done:
			case <-time.After(settle):
				t.Error("the user's terminal was still being read after it closed")
			}
		}
	})
	return tm
}

// blocking makes f's description blocking, as a shell's terminal is.
func blocking(t *testing.T, f *os.File) {
	t.Helper()
	if err := unix.SetNonblock(fdOf(t, f), false); err != nil {
		t.Fatalf("make %s blocking: %v", f.Name(), err)
	}
}

// another opens a description of the terminal's tty side of its own, as a
// shell's output can be.
func (tm *terminal) another(t *testing.T) *os.File {
	t.Helper()
	f, err := os.OpenFile(tm.tty.Name(), os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Fatalf("open %s again: %v", tm.tty.Name(), err)
	}
	t.Cleanup(func() { _ = f.Close() })
	blocking(t, f)
	return f
}

// record shows the test the terminal's output from now on.
func (tm *terminal) record() {
	tm.done = make(chan struct{})
	go func() {
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
	}()
}

// output is everything the terminal has shown since the test recorded it.
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

// readUntil reads the terminal's output, unrecorded, until it matches
// pattern, and returns the match.
func (tm *terminal) readUntil(t *testing.T, pattern *regexp.Regexp) [][]byte {
	t.Helper()
	user := fdOf(t, tm.user)
	var shown []byte
	buf := make([]byte, 4096)
	deadline := time.Now().Add(settle)
	for {
		if m := pattern.FindSubmatch(shown); m != nil {
			return m
		}
		if time.Now().After(deadline) {
			t.Fatalf("the terminal never showed %s; it shows %q", pattern, shown)
		}
		if !unread(user) {
			time.Sleep(5 * time.Millisecond)
			continue
		}
		n, _ := unix.Read(user, buf)
		shown = append(shown, buf[:max(n, 0)]...)
	}
}

// fill writes to the terminal until it takes no more, as a congested link
// leaves it.
func (tm *terminal) fill(t *testing.T) {
	t.Helper()
	fd, err := unix.Open(tm.tty.Name(), unix.O_RDWR|unix.O_NOCTTY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatalf("open %s to fill it: %v", tm.tty.Name(), err)
	}
	defer func() { _ = unix.Close(fd) }()
	filler := bytes.Repeat([]byte("f"), 64)
	for {
		_, err := unix.Write(fd, filler)
		if errors.Is(err, unix.EAGAIN) {
			return
		}
		if err != nil {
			t.Fatalf("fill the terminal: %v", err)
		}
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

// nextRaw is next for a reader that puts the terminal in raw mode first, as
// a shell's line editor and the picker do. Raw, it finds input a canonical
// reader would not.
func (tm *terminal) nextRaw(t *testing.T) string {
	t.Helper()
	tty := uintptr(fdOf(t, tm.tty))
	saved, err := term.MakeRaw(tty)
	if err != nil {
		t.Fatalf("put the terminal in raw mode: %v", err)
	}
	defer func() { _ = term.Restore(tty, saved) }()
	return tm.next(t)
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
// set on its description.
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

// flagsOf is the file status flags of f's open file description.
func flagsOf(t *testing.T, f *os.File) int {
	t.Helper()
	flags, err := unix.FcntlInt(uintptr(fdOf(t, f)), unix.F_GETFL, 0)
	if err != nil {
		t.Fatalf("read %s's flags: %v", f.Name(), err)
	}
	return flags
}

// result is how a Run ended: what it returned or panicked with, and when.
type result struct {
	out      runner.Outcome
	err      error
	panicked any
	// goexited is set when Run's goroutine exited through runtime.Goexit.
	goexited bool
	returned time.Time
}

// harness runs runner.Run on a terminal of its own.
type harness struct {
	term *terminal
	// in and out are the runner's terminal; before is in's settings, and
	// inFlags and outFlags their descriptions' flags, before the attach.
	in, out           *os.File
	before            *term.State
	inFlags, outFlags int
	result            chan result
	// pid is the fake child's, once it is ready. Run's goroutine reads it
	// too, in a fake Confirm.
	pid atomic.Int64
}

// option changes an attach's config. It is handed the attach's harness, made
// before Run starts, so a fake Confirm can reach the child through it.
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
	}
	for _, opt := range opts {
		opt(h, &cfg)
	}
	h.in, h.out, h.before = cfg.In, cfg.Out, stateOf(t, cfg.In)
	h.inFlags, h.outFlags = flagsOf(t, cfg.In), flagsOf(t, cfg.Out)
	go func() {
		r := result{goexited: true}
		defer func() {
			r.panicked, r.returned = recover(), time.Now()
			r.goexited = r.goexited && r.panicked == nil
			h.result <- r
		}()
		r.out, r.err = runner.Run(ctx, cfg)
		r.goexited = false
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
	return h.started(m)
}

// readyUnrecorded is ready for a terminal the test does not record.
func (h *harness) readyUnrecorded(t *testing.T) string {
	t.Helper()
	return h.started(h.term.readUntil(t, readyLine))
}

// started takes the child's pid from its ready line, and returns its
// terminal's size.
func (h *harness) started(ready [][]byte) string {
	pid, _ := strconv.Atoi(string(ready[1]))
	h.pid.Store(int64(pid))
	return string(ready[2])
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
	r := h.end(t)
	if r.panicked != nil || r.goexited {
		t.Fatalf("Run panicked with %v, or its goroutine exited (%v), want it to return", r.panicked, r.goexited)
	}
	return r.out, r.err
}

// end waits for Run to return or panic, and checks it restored the
// terminal's settings, left its descriptions' flags as they were, and left
// no copy loop running.
func (h *harness) end(t *testing.T) result {
	t.Helper()
	select {
	case r := <-h.result:
		h.result <- r
		if after := stateOf(t, h.in); !reflect.DeepEqual(after, h.before) {
			t.Errorf("the terminal's settings are %+v after the attach, want them restored to %+v", after, h.before)
		}
		h.assertFlags(t)
		assertNoCopyLoops(t)
		return r
	case <-time.After(settle):
		t.Fatalf("Run did not return; the terminal shows %q", h.term.output())
		return result{}
	}
}

// assertFlags fails the test unless the descriptions of the terminal Run was
// given have the flags they had before the attach.
func (h *harness) assertFlags(t *testing.T) {
	t.Helper()
	if got := flagsOf(t, h.in); got != h.inFlags {
		t.Errorf("the input's flags are %#x, want %#x as before the attach", got, h.inFlags)
	}
	if got := flagsOf(t, h.out); got != h.outFlags {
		t.Errorf("the output's flags are %#x, want %#x as before the attach", got, h.outFlags)
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
