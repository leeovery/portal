package runner

import (
	"errors"
	"fmt"
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

// terminalReset is what zmx attach writes to the terminal on its way out: a
// full reset, then the default foreground, background and cursor colours.
const terminalReset = "\x1bc\x1b]110\x1b\\\x1b]111\x1b\\\x1b]112\x1b\\"

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
	// The descriptors the runner works on, taken once: the user's terminal
	// and the two sides of the child's.
	inFd, outFd, ptmxFd, ttyFd int
	// forced is set once the runner signals the child.
	forced bool

	stopIn, stopOut *stopper
	inDone, outDone chan struct{}
	// exited closes once the child is reaped.
	exited chan struct{}
	// requests carries the actions that end the attach.
	requests chan attachkeys.Action
	// broken carries a copy loop's panic.
	broken         chan error
	signals, winch chan os.Signal
	undo           []func()
}

func start(cfg Config, inFd int) (*attach, error) {
	a := &attach{
		cfg:      cfg,
		began:    time.Now(),
		inFd:     inFd,
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
// signals are caught before the pseudo-terminal is sized and the terminal
// goes raw, so no resize is missed and no signal ends Portal with the
// terminal left raw.
func (a *attach) setUp() error {
	steps := []func() error{
		a.openPty,
		a.catchSignals,
		a.size,
		a.makeStoppers,
		a.makeChildRaw,
		a.makeTerminalRaw,
		a.startChild,
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return err
		}
	}
	return nil
}

func (a *attach) openPty() error {
	var err error
	if a.ptmx, a.tty, err = pty.Open(); err != nil {
		return fmt.Errorf("open a pseudo-terminal: %w", err)
	}
	a.onRelease(func() {
		_ = a.ptmx.Close()
		_ = a.tty.Close()
	})
	if a.ptmxFd, err = fdOf(a.ptmx); err != nil {
		return fmt.Errorf("open a pseudo-terminal: %w", err)
	}
	if a.ttyFd, err = fdOf(a.tty); err != nil {
		return fmt.Errorf("open a pseudo-terminal: %w", err)
	}
	if err := unix.SetNonblock(a.ptmxFd, true); err != nil {
		return fmt.Errorf("open a pseudo-terminal: %w", err)
	}
	return nil
}

func (a *attach) catchSignals() error {
	signal.Notify(a.signals, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGINT)
	signal.Notify(a.winch, syscall.SIGWINCH)
	a.onRelease(func() {
		signal.Stop(a.signals)
		signal.Stop(a.winch)
	})
	return nil
}

// size gives the child's terminal the user's terminal's size.
func (a *attach) size() error {
	ws, err := unix.IoctlGetWinsize(a.inFd, unix.TIOCGWINSZ)
	if err == nil {
		err = unix.IoctlSetWinsize(a.ptmxFd, unix.TIOCSWINSZ, ws)
	}
	if err != nil {
		return fmt.Errorf("size the pseudo-terminal: %w", err)
	}
	return nil
}

func (a *attach) makeStoppers() error {
	var err error
	if a.stopIn, err = newStopper(); err != nil {
		return err
	}
	a.onRelease(a.stopIn.close)
	if a.stopOut, err = newStopper(); err != nil {
		return err
	}
	a.onRelease(a.stopOut.close)
	return nil
}

// makeChildRaw makes the child's terminal raw before the child starts, so
// ctrl+\ is a byte to it, never SIGQUIT, before zmx makes it raw itself.
func (a *attach) makeChildRaw() error {
	if _, err := term.MakeRaw(uintptr(a.ttyFd)); err != nil {
		return fmt.Errorf("put the pseudo-terminal in raw mode: %w", err)
	}
	return nil
}

// makeTerminalRaw puts the user's terminal in raw, non-blocking mode. The
// shell shares its open file descriptions, so their flags are put back with
// the terminal's settings.
func (a *attach) makeTerminalRaw() error {
	saved, err := term.MakeRaw(uintptr(a.inFd))
	if err != nil {
		return fmt.Errorf("put the terminal in raw mode: %w", err)
	}
	a.onRelease(func() { _ = term.Restore(uintptr(a.inFd), saved) })
	if a.outFd, err = fdOf(a.cfg.Out); err != nil {
		return fmt.Errorf("take the terminal's output: %w", err)
	}
	for _, fd := range []int{a.inFd, a.outFd} {
		restore, err := nonblocking(fd)
		if err != nil {
			return fmt.Errorf("make the terminal non-blocking: %w", err)
		}
		a.onRelease(restore)
	}
	return nil
}

// startChild runs the attach command as the session leader of a new
// session, with the pseudo-terminal as its controlling terminal.
func (a *attach) startChild() error {
	c := a.cfg.Command
	a.child = exec.Command(c.Argv[0], c.Argv[1:]...)
	a.child.Env, a.child.Dir = c.Env, c.Dir
	a.child.Stdin, a.child.Stdout, a.child.Stderr = a.tty, a.tty, a.tty
	a.child.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	if err := a.child.Start(); err != nil {
		return fmt.Errorf("start %s: %w", c.Argv[0], err)
	}
	return nil
}

func (a *attach) onRelease(f func()) { a.undo = append(a.undo, f) }

func (a *attach) release() {
	for _, f := range slices.Backward(a.undo) {
		f()
	}
}

// finish ends the child, lets its last output reach the terminal, stops the
// copy loops, and resets the terminal if zmx could not. A signal to Portal
// that lands meanwhile wins over how the attach ended.
func (a *attach) finish(end ending) ending {
	a.end()
	// Until every holder of the child's side closes it, the copy may never
	// see the end of the child's output.
	_ = a.tty.Close()
	within(a.outDone, grace)
	a.halt()
	if !a.exitedCleanly() {
		_ = writeAll(a.outFd, []byte(terminalReset), until(time.Now().Add(grace)))
	}
	select {
	case sig := <-a.signals:
		return signalled(sig)
	default:
		return end
	}
}

// shutDown stops the copy loops and releases what the attach holds,
// restoring the terminal.
func (a *attach) shutDown() {
	a.halt()
	a.release()
	logger.Debug("ended", "session", a.cfg.Session, log.Took(a.began))
}

// end waits for the child to exit, ending it if it outlives the grace:
// SIGHUP, then SIGKILL.
func (a *attach) end() {
	if within(a.exited, grace) {
		return
	}
	a.forced = true
	for _, sig := range []syscall.Signal{syscall.SIGHUP, syscall.SIGKILL} {
		logger.Warn("child did not exit", "session", a.cfg.Session, "detail", unix.SignalName(sig))
		_ = a.child.Process.Signal(sig)
		if within(a.exited, grace) {
			return
		}
	}
}

// exitedCleanly reports whether the child exited by its own hand, as zmx
// does after writing terminalReset.
func (a *attach) exitedCleanly() bool {
	select {
	case <-a.exited:
		return !a.forced && a.child.ProcessState.Exited()
	default:
		return false
	}
}

// detach sends the child zmx's own detach key once nothing else writes to it
// and it has read what it was sent: zmx takes ctrl+\ only as the first byte
// of a read, so one queued behind unread input would reach the session. A
// child that reads nothing within the grace is left for end to signal.
func (a *attach) detach() {
	a.stopIn.stop()
	<-a.inDone
	if a.awaitRead() {
		_, _ = unix.Write(a.ptmxFd, []byte{detachKey})
	}
}

// awaitRead waits, within the grace, for the child to read all its input,
// and reports whether it did while still running.
func (a *attach) awaitRead() bool {
	deadline := time.Now().Add(grace)
	for {
		select {
		case <-a.exited:
			return false
		default:
		}
		if !unread(a.ttyFd) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(time.Millisecond)
	}
}

// halt stops both copy loops and waits for them, which return at once:
// every wait in them is on their stopper.
func (a *attach) halt() {
	a.stopIn.stop()
	a.stopOut.stop()
	<-a.inDone
	<-a.outDone
}

func (a *attach) resize() {
	if err := a.size(); err != nil {
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
// it hands on as a request. The end of the input is the terminal hanging up.
func (a *attach) copyInput() {
	buf := make([]byte, bufSize)
	for {
		n, err := readReady(a.inFd, buf, a.stopIn.wait)
		if errors.Is(err, errStopped) {
			return
		}
		if n == 0 || err != nil {
			a.hangUp()
			return
		}
		forward, action, found := a.cfg.Scanner.Scan(buf[:n])
		if err := writeAll(a.ptmxFd, forward, a.stopIn.wait); err != nil {
			return // stopped, or the child's side has closed and its exit ends the attach
		}
		if found {
			a.requests <- action
			return
		}
	}
}

// hangUp takes the end of the terminal's input as the hang-up a SIGHUP to
// Portal reports, so whichever arrives first, the attach ends the same way.
func (a *attach) hangUp() {
	logger.Debug("terminal input ended", "session", a.cfg.Session)
	select {
	case a.signals <- syscall.SIGHUP:
	default:
	}
}

// copyOutput passes the child's output to the terminal until the child's side
// closes. Output the terminal refuses is dropped, so the child never blocks.
func (a *attach) copyOutput() {
	buf := make([]byte, bufSize)
	refused := false
	for {
		n, err := readReady(a.ptmxFd, buf, a.stopOut.wait)
		if n == 0 || err != nil {
			return
		}
		if refused {
			continue
		}
		switch err := writeAll(a.outFd, buf[:n], a.stopOut.wait); {
		case errors.Is(err, errStopped):
			return
		case err != nil:
			refused = true
		}
	}
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
