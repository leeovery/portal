package runner

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

var (
	errStopped  = errors.New("copy loop stopped")
	errTimedOut = errors.New("timed out")
)

// waiter waits for fd to be ready for events.
type waiter func(fd int, events int16) error

// stopper stops a copy loop. Every wait in a loop polls the stopper's pipe
// beside its own fd, and stopping closes the pipe's write end, so a stopped
// loop returns at once wherever it is waiting.
type stopper struct {
	r, w *os.File
	rfd  int
	once sync.Once
}

func newStopper() (*stopper, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("make a stop pipe: %w", err)
	}
	rfd, err := fdOf(r)
	if err != nil {
		_, _ = r.Close(), w.Close()
		return nil, fmt.Errorf("make a stop pipe: %w", err)
	}
	return &stopper{r: r, w: w, rfd: rfd}, nil
}

func (s *stopper) stop() { s.once.Do(func() { _ = s.w.Close() }) }

func (s *stopper) close() {
	s.stop()
	_ = s.r.Close()
}

// wait is a waiter that returns errStopped once s is stopped, even with fd
// ready.
func (s *stopper) wait(fd int, events int16) error {
	fds := []unix.PollFd{{Fd: int32(fd), Events: events}, {Fd: int32(s.rfd), Events: unix.POLLIN}}
	for {
		_, err := unix.Poll(fds, -1)
		switch {
		case errors.Is(err, unix.EINTR):
		case err != nil:
			return err
		case fds[1].Revents != 0:
			return errStopped
		case fds[0].Revents != 0:
			return nil
		}
	}
}

// until is a waiter that gives up with errTimedOut at deadline.
func until(deadline time.Time) waiter {
	return func(fd int, events int16) error {
		fds := []unix.PollFd{{Fd: int32(fd), Events: events}}
		for {
			left := time.Until(deadline).Milliseconds()
			if left <= 0 {
				return errTimedOut
			}
			n, err := unix.Poll(fds, int(left))
			if err != nil && !errors.Is(err, unix.EINTR) {
				return err
			}
			if n > 0 {
				return nil
			}
		}
	}
}

// readReady reads what the non-blocking fd has once wait finds it ready. It
// reads 0 bytes at fd's end.
func readReady(fd int, buf []byte, wait waiter) (int, error) {
	for {
		if err := wait(fd, unix.POLLIN); err != nil {
			return 0, err
		}
		n, err := unix.Read(fd, buf)
		if !again(err) {
			return max(n, 0), err
		}
	}
}

// writeAll writes all of p to the non-blocking fd, waiting with wait each
// time fd takes no more.
func writeAll(fd int, p []byte, wait waiter) error {
	for len(p) > 0 {
		n, err := unix.Write(fd, p)
		p = p[max(n, 0):]
		if again(err) {
			err = wait(fd, unix.POLLOUT)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func again(err error) bool { return errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EINTR) }

// unread reports whether input waits on the terminal fd for a reader still
// there to read it.
func unread(fd int) bool {
	fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	if _, err := unix.Poll(fds, 0); err != nil {
		return false
	}
	return fds[0].Revents&unix.POLLIN != 0 && fds[0].Revents&(unix.POLLHUP|unix.POLLERR|unix.POLLNVAL) == 0
}

// nonblocking puts fd's open file description in non-blocking mode, and
// returns what puts its flags back.
func nonblocking(fd int) (func(), error) {
	flags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0)
	if err != nil {
		return nil, err
	}
	if _, err := unix.FcntlInt(uintptr(fd), unix.F_SETFL, flags|unix.O_NONBLOCK); err != nil {
		return nil, err
	}
	return func() { _, _ = unix.FcntlInt(uintptr(fd), unix.F_SETFL, flags) }, nil
}

// fdOf is f's descriptor, taken without the blocking mode os.File.Fd sets on
// each call: the descriptions the runner shares with the shell and the child
// keep the modes their owners gave them.
func fdOf(f *os.File) (int, error) {
	raw, err := f.SyscallConn()
	if err != nil {
		return -1, err
	}
	fd := -1
	if err := raw.Control(func(u uintptr) { fd = int(u) }); err != nil {
		return -1, err
	}
	return fd, nil
}
