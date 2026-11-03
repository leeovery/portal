package runner

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"runtime/debug"
	"slices"
	"syscall"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/creack/pty"
	"github.com/leeovery/portal/internal/attachkeys"
	"github.com/leeovery/portal/internal/log"
	"golang.org/x/sys/unix"
)

// detachKey is ctrl+\, zmx's own detach.
const detachKey = 0x1c

const bufSize = 32 << 10

// attach is one running attach: the child, its pseudo-terminal, and the loops
// copying between that and the user's terminal.
type attach struct {
	cfg   Config
	began time.Time
	child *exec.Cmd
	// ptmx is Portal's side of the child's terminal and tty the child's,
	// held open to see what the child has yet to read.
	ptmx, tty *os.File
	saved     *term.State

	in, out         *stopper
	inDone, outDone chan struct{}
	// exited closes once the child is reaped.
	exited chan struct{}
	// requests carries the actions that end the attach.
	requests chan attachkeys.Action
	// broken carries what stops a copy loop short: a panic, or the end of
	// the terminal's input.
	broken         chan error
	signals, winch chan os.Signal
	undo           []func()
}

func start(cfg Config) (*attach, error) {
	a := &attach{
		cfg:      cfg,
		began:    time.Now(),
		inDone:   make(chan struct{}),
		outDone:  make(chan struct{}),
		exited:   make(chan struct{}),
		requests: make(chan attachkeys.Action, 1),
		broken:   make(chan error, 2),
		signals:  make(chan os.Signal, 1),
		winch:    make(chan os.Signal, 1),
	}
	if err := a.setUp(); err != nil {
		a.release()
		return nil, err
	}
	logger.Debug("started", "session", cfg.Session, "target_pid", a.child.Process.Pid)
	go func() {
		_ = a.child.Wait()
		close(a.exited)
	}()
	a.goCopy(a.copyInput, a.inDone)
	a.goCopy(a.copyOutput, a.outDone)
	return a, nil
}

// setUp acquires what an attach holds, recording how to release each. The
// signals are caught before the terminal goes raw, so none can end Portal
// with the terminal left raw.
func (a *attach) setUp() error {
	var err error
	if a.ptmx, a.tty, err = pty.Open(); err != nil {
		return fmt.Errorf("open a pseudo-terminal: %w", err)
	}
	a.onRelease(func() {
		_ = a.ptmx.Close()
		_ = a.tty.Close()
	})
	if err := pty.InheritSize(a.cfg.In, a.ptmx); err != nil {
		return fmt.Errorf("size the pseudo-terminal: %w", err)
	}
	if a.in, err = newStopper(); err != nil {
		return err
	}
	a.onRelease(a.in.close)
	if a.out, err = newStopper(); err != nil {
		return err
	}
	a.onRelease(a.out.close)
	signal.Notify(a.signals, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGINT)
	signal.Notify(a.winch, syscall.SIGWINCH)
	a.onRelease(func() { signal.Stop(a.signals); signal.Stop(a.winch) })
	if a.saved, err = term.MakeRaw(a.cfg.In.Fd()); err != nil {
		return fmt.Errorf("put the terminal in raw mode: %w", err)
	}
	a.onRelease(func() { _ = term.Restore(a.cfg.In.Fd(), a.saved) })
	a.child = a.command()
	if err := a.child.Start(); err != nil {
		return fmt.Errorf("start %s: %w", a.cfg.Command.Argv[0], err)
	}
	return nil
}

// command runs the attach command as the session leader of a new session,
// with the pseudo-terminal as its controlling terminal.
func (a *attach) command() *exec.Cmd {
	c := a.cfg.Command
	child := exec.Command(c.Argv[0], c.Argv[1:]...)
	child.Env, child.Dir = c.Env, c.Dir
	child.Stdin, child.Stdout, child.Stderr = a.tty, a.tty, a.tty
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	return child
}

func (a *attach) onRelease(f func()) { a.undo = append(a.undo, f) }

func (a *attach) release() {
	for _, f := range slices.Backward(a.undo) {
		f()
	}
}

// close ends the child, lets its last output reach the terminal, stops the
// copy loops and restores the terminal.
func (a *attach) close() {
	a.end()
	// Until every holder of the child's side closes it, the copy may never
	// see the end of the child's output.
	_ = a.tty.Close()
	within(a.outDone, grace)
	halt(a.in, a.inDone)
	halt(a.out, a.outDone)
	a.release()
	logger.Debug("ended", "session", a.cfg.Session, log.Took(a.began))
}

// end waits for the child to exit, ending it if it outlives the grace:
// SIGHUP, then SIGKILL.
func (a *attach) end() {
	if within(a.exited, grace) {
		return
	}
	for _, sig := range []syscall.Signal{syscall.SIGHUP, syscall.SIGKILL} {
		logger.Warn("child did not exit", "session", a.cfg.Session, "detail", unix.SignalName(sig))
		_ = a.child.Process.Signal(sig)
		if within(a.exited, grace) {
			return
		}
	}
}

// detach sends the child zmx's own detach key once nothing else writes to it
// and it has read what it was sent: zmx takes ctrl+\ only as the first byte
// of a read, so one queued behind unread input would reach the session. A
// child that reads nothing within the grace is left for end to signal.
func (a *attach) detach() {
	halt(a.in, a.inDone)
	if a.drained() {
		_, _ = a.ptmx.Write([]byte{detachKey})
	}
}

// drained waits, within the grace, for the child to read all its input.
func (a *attach) drained() bool {
	deadline := time.Now().Add(grace)
	for unread(a.tty) {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(time.Millisecond)
	}
	return true
}

func (a *attach) resize() {
	if err := pty.InheritSize(a.cfg.In, a.ptmx); err != nil {
		logger.Warn("resize failed", "session", a.cfg.Session, "error", err)
	}
}

// goCopy runs a copy loop, turning a panic into an error that ends the
// attach, so the terminal is still restored.
func (a *attach) goCopy(loop func(), done chan<- struct{}) {
	go func() {
		defer close(done)
		defer func() {
			if r := recover(); r != nil {
				logger.Error("panic", "session", a.cfg.Session, "reason", r, "detail", string(debug.Stack()))
				a.broken <- fmt.Errorf("copy loop panicked: %v", r)
			}
		}()
		loop()
	}()
}

// copyInput passes the terminal's input to the child until a bound key, which
// it hands on as a request.
func (a *attach) copyInput() {
	buf := make([]byte, bufSize)
	for {
		n, err := a.in.read(a.cfg.In, buf)
		if errors.Is(err, errStopped) {
			return
		}
		if err != nil {
			a.broken <- fmt.Errorf("read the terminal: %w", err)
			return
		}
		forward, action, found := a.cfg.Scanner.Scan(buf[:n])
		if _, err := a.ptmx.Write(forward); err != nil {
			return // the child's side has closed, and its exit ends the attach
		}
		if found {
			a.requests <- action
			return
		}
	}
}

// copyOutput passes the child's output to the terminal until the child's side
// closes. Output the terminal refuses is dropped, so the child never blocks.
func (a *attach) copyOutput() {
	buf := make([]byte, bufSize)
	out := io.Writer(a.cfg.Out)
	for {
		n, err := a.out.read(a.ptmx, buf)
		if err != nil {
			return
		}
		if _, err := out.Write(buf[:n]); err != nil {
			out = io.Discard
		}
	}
}

// unread reports whether input waits on tty for its reader.
func unread(tty *os.File) bool {
	fds := []unix.PollFd{{Fd: int32(tty.Fd()), Events: unix.POLLIN}}
	n, err := unix.Poll(fds, 0)
	return err == nil && n > 0 && fds[0].Revents&unix.POLLHUP == 0
}

// halt stops a copy loop and waits, within the grace, for it to return.
func halt(s *stopper, done <-chan struct{}) {
	s.stop()
	within(done, grace)
}

// within reports whether done closes within d.
func within(done <-chan struct{}, d time.Duration) bool {
	select {
	case <-done:
		return true
	case <-time.After(d):
		return false
	}
}
