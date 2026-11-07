package runner_test

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/charmbracelet/x/term"
	"golang.org/x/sys/unix"
)

// childModeEnv switches the test binary into a fake attach child standing in
// for zmx attach. Unless its mode says otherwise, it puts its terminal in
// raw, non-blocking mode, as zmx attach does, and writes one line for each
// thing the tests watch:
//
//	ready:<pid>:<cols>x<rows>   once started, with its terminal's size
//	in:<hex>                    for each read of its input
//	nonblock:<bool>             on its ctrl+\, whether its input is still non-blocking
//	size:<cols>x<rows>          on each SIGWINCH
//
// A SIGUSR1 ends it with status 0, as a session's end ends zmx attach.
const childModeEnv = "PORTAL_RUNNER_TEST_CHILD"

const (
	// modeZmx detaches on a read whose first byte is ctrl+\, as zmx does.
	modeZmx = "zmx"
	// modeCooked is modeZmx leaving its terminal as it found it.
	modeCooked = "cooked"
	// modeBlob writes childBlob once ready, then acts as modeZmx.
	modeBlob = "blob"
	// modeDeaf takes ctrl+\ as input, so only a signal ends it.
	modeDeaf = "deaf"
	// modeStubborn is modeDeaf ignoring SIGHUP as well.
	modeStubborn = "stubborn"
	// modeHungUpClean is modeDeaf exiting with status 0 on SIGHUP, as a child
	// that tidies up after a hang-up does.
	modeHungUpClean = "hupclean"
	// modeExit writes "bye" and exits with status 3 at once.
	modeExit = "exit"
	// modeSelfKill dies of SIGKILL at once.
	modeSelfKill = "selfkill"
	// modeFrozen reads nothing until SIGHUP, then reports all its waiting
	// input in one read and exits.
	modeFrozen = "frozen"
	// modeFlood writes output without end, and otherwise acts as modeZmx.
	modeFlood = "flood"
	// modeSpill writes lastOutput in one write once it reads input, then
	// exits with status 0.
	modeSpill = "spill"
	// modeSwitched writes zmx's switch twice once ready, as zmx's client
	// does when switched to another session, each time in two writes that
	// split it, then acts as modeZmx.
	modeSwitched = "switched"
	// modeFrozenSwitched writes zmx's switch as modeSwitched does, then acts
	// as modeFrozen.
	modeFrozenSwitched = "frozenswitched"
	// modeSpillThenSwitch writes lastOutput once it reads input, then, after
	// switchLag, zmx's switch, then acts as modeZmx: the runner reads the
	// switch apart from the output before it.
	modeSpillThenSwitch = "spillswitch"
)

// switchLag separates modeSpillThenSwitch's two writes.
const switchLag = 200 * time.Millisecond

// switchSignature is what zmx's client writes as zmx switches it to another
// session: its reset as it leaves one, then the screen clear it attaches the
// next with.
const switchSignature = terminalReset + "\x1b[0m\x1b[2J\x1b[H"

// lastOutput is short enough for one write to put it all in the child's
// terminal at once, so the runner reads it whole.
const lastOutput = "the child's last output\r\n"

const ctrlBackslash = 0x1c

// readDelay holds each read back once input arrives, so input sent close
// together lands in one read, as it can for zmx.
const readDelay = 30 * time.Millisecond

// childBlob holds every byte value once.
var childBlob = func() []byte {
	b := make([]byte, 256)
	for i := range b {
		b[i] = byte(i)
	}
	return b
}()

func fakeChild(mode string) int {
	if err := setUpTerminal(mode); err != nil {
		fmt.Fprintf(os.Stderr, "fake child: %v\r\n", err)
		return 100
	}
	// Every handler is in place before the child says it is ready, so a test
	// can signal it the moment it does.
	winch, usr1, hup := notified(syscall.SIGWINCH), notified(syscall.SIGUSR1), make(<-chan os.Signal)
	switch mode {
	case modeStubborn:
		signal.Ignore(syscall.SIGHUP)
	case modeFrozen, modeFrozenSwitched, modeHungUpClean:
		hup = notified(syscall.SIGHUP)
	}
	go func() {
		<-usr1
		os.Exit(0)
	}()
	cols, rows, _ := term.GetSize(0)
	say("ready:%d:%dx%d", os.Getpid(), cols, rows)
	switch mode {
	case modeExit:
		say("bye")
		return 3
	case modeSelfKill:
		_ = syscall.Kill(os.Getpid(), syscall.SIGKILL)
		select {}
	case modeFrozen:
		<-hup
		return reportWaiting()
	case modeFrozenSwitched:
		writeSwitchTwice()
		<-hup
		return reportWaiting()
	case modeBlob:
		_, _ = os.Stdout.Write(childBlob)
	case modeFlood:
		go flood()
	case modeHungUpClean:
		go func() {
			<-hup
			os.Exit(0)
		}()
	case modeSpill:
		await(0, unix.POLLIN)
		_, _ = unix.Write(1, []byte(lastOutput))
		return 0
	case modeSpillThenSwitch:
		await(0, unix.POLLIN)
		_, _ = unix.Read(0, make([]byte, 4096))
		write([]byte(lastOutput))
		time.Sleep(switchLag)
		write([]byte(switchSignature))
	case modeSwitched:
		writeSwitchTwice()
	}
	go reportSizes(winch)
	return readInput(mode != modeDeaf && mode != modeStubborn && mode != modeHungUpClean)
}

// writeSwitchTwice writes zmx's switch twice, each time in two writes that
// split it.
func writeSwitchTwice() {
	for range 2 {
		write([]byte(switchSignature[:10]))
		time.Sleep(readDelay)
		write([]byte(switchSignature[10:]))
	}
}

// setUpTerminal makes the child's terminal raw and, as zmx's is, its input
// non-blocking, in the modes that stand in for zmx at its plainest.
func setUpTerminal(mode string) error {
	if mode == modeCooked {
		return nil
	}
	if _, err := term.MakeRaw(0); err != nil {
		return fmt.Errorf("raw mode: %w", err)
	}
	if mode == modeZmx {
		return unix.SetNonblock(0, true)
	}
	return nil
}

func notified(sig os.Signal) <-chan os.Signal {
	c := make(chan os.Signal, 1)
	signal.Notify(c, sig)
	return c
}

func readInput(detaches bool) int {
	buf := make([]byte, 4096)
	for {
		if !await(0, unix.POLLIN) {
			continue
		}
		time.Sleep(readDelay)
		n, err := unix.Read(0, buf)
		if errors.Is(err, unix.EAGAIN) {
			continue
		}
		if err != nil || n == 0 {
			return 101
		}
		say("in:%x", buf[:n])
		if detaches && buf[0] == ctrlBackslash {
			flags, _ := unix.FcntlInt(0, unix.F_GETFL, 0)
			say("nonblock:%t", flags&unix.O_NONBLOCK != 0)
			return 0
		}
	}
}

// reportWaiting reports all the input waiting for it as one read.
func reportWaiting() int {
	var waiting []byte
	buf := make([]byte, 4096)
	for unread(0) {
		n, err := unix.Read(0, buf)
		if err != nil || n == 0 {
			break
		}
		waiting = append(waiting, buf[:n]...)
	}
	say("in:%x", waiting)
	return 0
}

func reportSizes(winch <-chan os.Signal) {
	for range winch {
		cols, rows, _ := term.GetSize(0)
		say("size:%dx%d", cols, rows)
	}
}

func flood() {
	line := []byte("flood flood flood flood flood flood flood flood flood flood\r\n")
	for {
		if _, err := unix.Write(1, line); err != nil && !errors.Is(err, unix.EAGAIN) {
			return
		}
	}
}

// say writes one line.
func say(format string, args ...any) {
	write(fmt.Appendf(nil, format+"\r\n", args...))
}

// write writes out, waiting for room whenever the terminal is full.
func write(out []byte) {
	for len(out) > 0 {
		n, err := unix.Write(1, out)
		out = out[max(n, 0):]
		if errors.Is(err, unix.EAGAIN) {
			await(1, unix.POLLOUT)
		} else if err != nil {
			return
		}
	}
}

// await waits for fd to be ready for events, reporting false when a signal
// cut the wait short.
func await(fd int, events int16) bool {
	n, err := unix.Poll([]unix.PollFd{{Fd: int32(fd), Events: events}}, -1)
	return err == nil && n > 0
}
