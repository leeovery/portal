package runner

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime/debug"
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
	// The descriptors the runner works on, taken once: inFd and outFd are
	// descriptions of the user's terminal that are Portal's alone, and ptmxFd
	// and ttyFd the two sides of the child's.
	inFd, outFd, ptmxFd, ttyFd int
	// forced is set once the runner signals the child, and detached once it
	// detaches it.
	forced, detached bool
	// drained is set once the child's output has ended with all of it on the
	// terminal.
	drained bool
	// caught is the signal caught and not yet acted on when the runner
	// stopped catching them.
	caught os.Signal

	stopIn, stopOut *stopper
	inDone, outDone chan struct{}
	// exited closes once the child is reaped.
	exited chan struct{}
	// keys carries the action of the bound key the input found.
	keys chan attachkeys.Action
	// broken carries the end a copy loop's panic makes.
	broken         chan ending
	signals, winch chan os.Signal
	undo           []func()
}

func newAttach(cfg Config) *attach {
	if cfg.Began == nil {
		cfg.Began = func() {}
	}
	if cfg.Forwarded == nil {
		cfg.Forwarded = func() {}
	}
	return &attach{
		cfg:     cfg,
		began:   time.Now(),
		inDone:  make(chan struct{}),
		outDone: make(chan struct{}),
		exited:  make(chan struct{}),
		keys:    make(chan attachkeys.Action, 1),
		broken:  make(chan ending, 2),
		signals: make(chan os.Signal, 1),
		winch:   make(chan os.Signal, 1),
	}
}

// start begins an attach. One that fails to begin, by an error or a panic,
// is abandoned.
func start(cfg Config) (*attach, error) {
	a := newAttach(cfg)
	begun := false
	defer func() {
		if !begun {
			a.abandon()
		}
	}()
	if err := a.setUp(); err != nil {
		return nil, err
	}
	logger.Debug("started", "session", cfg.Session, "target_pid", a.child.Process.Pid)
	go func() {
		_ = a.child.Wait()
		close(a.exited)
	}()
	a.goCopy(a.copyInput, a.inDone)
	a.goCopy(a.copyOutput, a.outDone)
	begun = true
	return a, nil
}

// abandon releases an attach that never began. A signal caught meanwhile
// does what it would have done had the runner not caught it.
func (a *attach) abandon() {
	a.release()
	if a.caught != nil {
		_ = syscall.Kill(os.Getpid(), a.caught.(syscall.Signal))
	}
}

// setUp acquires what an attach holds, recording how to release each. The
// signals are caught first: before the pseudo-terminal is sized and the
// terminal goes raw, so no resize is missed and no signal ends Portal with
// the terminal left raw, and released last, so one that lands until the
// terminal is restored still decides how the attach ends.
func (a *attach) setUp() error {
	steps := []func() error{
		a.catchSignals,
		a.openTerminal,
		a.openPty,
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

// openTerminal opens the runner's own descriptions of the user's terminal.
func (a *attach) openTerminal() error {
	var err error
	if a.inFd, err = a.reopen(a.cfg.In); err != nil {
		return err
	}
	a.outFd, err = a.reopen(a.cfg.Out)
	return err
}

// reopen opens Portal's own description of f's terminal, closed on release.
func (a *attach) reopen(f *os.File) (int, error) {
	fd, err := privateTerminal(f)
	if err != nil {
		return -1, fmt.Errorf("%w: %w", ErrNoTerminal, err)
	}
	a.onRelease(func() { _ = unix.Close(fd) })
	return fd, nil
}

// catchSignals catches the signals that end an attach until it is released,
// keeping the one caught and not yet acted on, if any, in caught.
func (a *attach) catchSignals() error {
	signal.Notify(a.signals, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGINT)
	signal.Notify(a.winch, syscall.SIGWINCH)
	a.onRelease(func() {
		signal.Stop(a.signals)
		signal.Stop(a.winch)
		select {
		case a.caught = <-a.signals:
		default:
		}
	})
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
	// Portal's side is no one else's, so its flags are the runner's to set.
	if err := unix.SetNonblock(a.ptmxFd, true); err != nil {
		return fmt.Errorf("open a pseudo-terminal: %w", err)
	}
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

// makeTerminalRaw puts the user's terminal in raw mode until the attach is
// released. Its settings come back as zmx puts them back, once it has taken
// all its output and with its unread input dropped, so the terminal's late
// replies to the session never reach the next reader.
func (a *attach) makeTerminalRaw() error {
	saved, err := unix.IoctlGetTermios(a.inFd, getTermios)
	if err == nil {
		_, err = term.MakeRaw(uintptr(a.inFd))
	}
	if err != nil {
		return fmt.Errorf("put the terminal in raw mode: %w", err)
	}
	a.onRelease(func() { restoreTermios(a.inFd, saved) })
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
	// A child still running when the attach is released, as one is after a
	// panic in tearDown, ends with it. A reaped child is not signalled.
	a.onRelease(func() { _ = a.child.Process.Kill() })
	return nil
}

func (a *attach) onRelease(f func()) { a.undo = append(a.undo, f) }

// release lets go of what the attach holds, in the reverse of the order it
// took them: the child, the terminal's settings, then its descriptions, and
// the signals last. Each is let go once, so release can run again after a
// panic part-way through.
func (a *attach) release() {
	for len(a.undo) > 0 {
		last := len(a.undo) - 1
		f := a.undo[last]
		a.undo = a.undo[:last]
		f()
	}
}

// tearDown ends the attach in one order, however it ended: the input stops;
// the child ends while its last output drains; the terminal is reset unless
// zmx's own reset reached it; the attach is released, the terminal restored.
// A panic part-way through releases the attach all the same.
func (a *attach) tearDown(end ending) ending {
	defer a.release()
	a.stopIn.stop()
	<-a.inDone
	a.endChild()
	// Load-bearing on Linux only, so the macOS suite cannot pin it: darwin
	// ends the child's terminal when the child exits, but Linux shows its
	// output's end only once every holder of its side, this one too, closes.
	_ = a.tty.Close()
	within(a.outDone, grace)
	a.stopOut.stop()
	<-a.outDone
	a.reset()
	a.release()
	logger.Debug("ended", "session", a.cfg.Session, log.Took(a.began))
	return a.final(end)
}

// final is how the attach ended, decided in this one place once it is torn
// down, whatever came in meanwhile. A panic comes first, Run's own and then a
// copy loop's, even one after await returned, so its error always reaches
// the caller; then a signal caught at any point until the runner stopped
// catching them; then the end await came to.
func (a *attach) final(end ending) ending {
	if end.reason == reasonPanic {
		return end
	}
	select {
	case broken := <-a.broken:
		return broken
	default:
	}
	if a.caught != nil {
		return signalled(a.caught)
	}
	return end
}

// endChild waits for the child to exit, ending it if it outlives the grace:
// SIGHUP, then SIGKILL.
func (a *attach) endChild() {
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

// reset writes terminalReset unless zmx's own reached the terminal, which it
// did only if the child exited by its own hand, as zmx does after writing it,
// and all the child's output reached the terminal. The reset is the runner's
// last write to the terminal, so it waits for the terminal to take it however
// long that takes, as whatever writes to it next would, unless the terminal
// hangs up. Its leading ESC ends any escape sequence a cut left half written.
func (a *attach) reset() {
	var reason string
	switch {
	case !a.exitedCleanly():
		reason = "exit"
	case !a.drained:
		reason = "drain"
	default:
		return
	}
	logger.Debug("terminal reset", "session", a.cfg.Session, "reason", reason)
	_ = writeAll(a.outFd, []byte(terminalReset), untilHangUp)
}

// exitedCleanly reports whether the child exited by its own hand.
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
// child that reads nothing within the grace is left for endChild to signal.
// The child is detached once, however many ends ask for it.
func (a *attach) detach() {
	if a.detached {
		return
	}
	a.detached = true
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
		time.Sleep(5 * time.Millisecond)
	}
}

func (a *attach) resize() {
	if err := a.size(); err != nil {
		logger.Warn("resize failed", "session", a.cfg.Session, "error", err)
	}
}

// goCopy runs a copy loop, turning a panic into an end of the attach.
func (a *attach) goCopy(loop func(), done chan<- struct{}) {
	go func() {
		defer close(done)
		defer func() {
			if r := recover(); r != nil {
				a.broken <- a.panicked(r)
			}
		}()
		loop()
	}()
}

// panicked logs a recovered panic, and is the end of the attach it makes.
func (a *attach) panicked(r any) ending {
	logger.Warn("panic", "session", a.cfg.Session, "error", r, "detail", string(debug.Stack()))
	return ending{err: fmt.Errorf("attach session %q: panic: %v", a.cfg.Session, r), reason: reasonPanic}
}

// copyInput passes the terminal's input to the child until a bound key, which
// it hands on to end the attach. The end of the input is the terminal hanging
// up.
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
		if err := a.forward(forward); err != nil {
			return // stopped, or the child's side has closed and its exit ends the attach
		}
		if found {
			a.keys <- action
			return
		}
	}
}

// forward passes input to the child, and reports it forwarded.
func (a *attach) forward(input []byte) error {
	if err := writeAll(a.ptmxFd, input, a.stopIn.wait); err != nil {
		return err
	}
	if len(input) > 0 {
		a.cfg.Forwarded()
	}
	return nil
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
		if err != nil || n == 0 {
			// The child's output ends at EOF, or EIO on Linux; any other
			// error leaves its end unseen.
			a.drained = !refused && (err == nil || errors.Is(err, unix.EIO))
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
