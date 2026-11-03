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
// for zmx attach. It puts its terminal in raw mode, as zmx attach does, and
// writes one line for each thing the tests watch:
//
//	ready:<pid>:<cols>x<rows>   once started, with its terminal's size
//	in:<hex>                    for each read of its input
//	size:<cols>x<rows>          on each SIGWINCH
//
// A SIGUSR1 ends it with status 0, as a session's end ends zmx attach.
const childModeEnv = "PORTAL_RUNNER_TEST_CHILD"

const (
	// modeZmx detaches on a read whose first byte is ctrl+\, as zmx does.
	modeZmx = "zmx"
	// modeBlob writes childBlob once ready, then acts as modeZmx.
	modeBlob = "blob"
	// modeDeaf takes ctrl+\ as input, so only a signal ends it.
	modeDeaf = "deaf"
	// modeStubborn is modeDeaf ignoring SIGHUP as well.
	modeStubborn = "stubborn"
	// modeExit writes "bye" and exits with status 3 at once.
	modeExit = "exit"
	// modeSelfKill dies of SIGKILL at once.
	modeSelfKill = "selfkill"
	// modeFrozen reads nothing until SIGHUP, then reports all its waiting
	// input in one read and exits.
	modeFrozen = "frozen"
)

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
	if _, err := term.MakeRaw(os.Stdin.Fd()); err != nil {
		fmt.Fprintf(os.Stderr, "fake child: raw mode: %v\r\n", err)
		return 100
	}
	// Every handler is in place before the child says it is ready, so a test
	// can signal it the moment it does.
	winch, usr1, hup := notified(syscall.SIGWINCH), notified(syscall.SIGUSR1), make(<-chan os.Signal)
	switch mode {
	case modeStubborn:
		signal.Ignore(syscall.SIGHUP)
	case modeFrozen:
		hup = notified(syscall.SIGHUP)
	}
	cols, rows, _ := term.GetSize(os.Stdin.Fd())
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
	case modeBlob:
		_, _ = os.Stdout.Write(childBlob)
	}
	go reportSizes(winch)
	go func() {
		<-usr1
		os.Exit(0)
	}()
	return readInput(mode == modeZmx || mode == modeBlob)
}

func notified(sig os.Signal) <-chan os.Signal {
	c := make(chan os.Signal, 1)
	signal.Notify(c, sig)
	return c
}

func readInput(detaches bool) int {
	buf := make([]byte, 4096)
	fds := []unix.PollFd{{Fd: 0, Events: unix.POLLIN}}
	for {
		if _, err := unix.Poll(fds, -1); errors.Is(err, unix.EINTR) {
			continue
		}
		time.Sleep(readDelay)
		n, err := os.Stdin.Read(buf)
		if err != nil {
			return 101
		}
		say("in:%x", buf[:n])
		if detaches && buf[0] == ctrlBackslash {
			return 0
		}
	}
}

// reportWaiting reports all the input waiting for it as one read.
func reportWaiting() int {
	var waiting []byte
	buf := make([]byte, 4096)
	fds := []unix.PollFd{{Fd: 0, Events: unix.POLLIN}}
	for {
		if n, err := unix.Poll(fds, 0); n == 0 || err != nil {
			break
		}
		n, err := os.Stdin.Read(buf)
		if err != nil {
			break
		}
		waiting = append(waiting, buf[:n]...)
	}
	say("in:%x", waiting)
	return 0
}

func reportSizes(winch <-chan os.Signal) {
	for range winch {
		cols, rows, _ := term.GetSize(os.Stdin.Fd())
		say("size:%dx%d", cols, rows)
	}
}

func say(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stdout, format+"\r\n", args...)
}
