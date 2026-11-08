//go:build darwin || linux

package cmd

import (
	"errors"
	"syscall"

	"golang.org/x/sys/unix"
)

// ttyWaker is a resumeWaker over a terminal: a select on the terminal and a
// pipe of its own, which Wake writes. The pipe is closed on exec, so the
// process image an answer execs inherits none of it. Select rather than poll,
// which macOS does not support on a terminal device.
type ttyWaker struct {
	fd   int
	pipe [2]int
}

func newTTYWaker(fd int) (*ttyWaker, error) {
	var pipe [2]int
	// Taken as os.Pipe takes it, so no fork in flight inherits the pipe before
	// it is marked close-on-exec.
	syscall.ForkLock.RLock()
	err := unix.Pipe(pipe[:])
	if err == nil {
		unix.CloseOnExec(pipe[0])
		unix.CloseOnExec(pipe[1])
	}
	syscall.ForkLock.RUnlock()
	if err != nil {
		return nil, err
	}
	w := &ttyWaker{fd: fd, pipe: pipe}
	if fd < 0 || max(fd, pipe[0]) >= unix.FD_SETSIZE {
		_ = w.Close()
		return nil, errUnselectableTTY
	}
	for _, end := range pipe {
		if err := unix.SetNonblock(end, true); err != nil {
			_ = w.Close()
			return nil, err
		}
	}
	return w, nil
}

// Await empties the pipe it was woken by. A byte that arrived on the terminal
// as the pipe was written is reported.
func (w *ttyWaker) Await() (bool, error) {
	for {
		var readable unix.FdSet
		readable.Set(w.fd)
		readable.Set(w.pipe[0])
		_, err := unix.Select(max(w.fd, w.pipe[0])+1, &readable, nil, nil, nil)
		switch {
		case errors.Is(err, unix.EINTR):
			continue
		case err != nil:
			return false, err
		}
		if readable.IsSet(w.pipe[0]) {
			w.empty()
		}
		return readable.IsSet(w.fd), nil
	}
}

// Wake never blocks: a pipe already full holds a wake still to be taken.
func (w *ttyWaker) Wake() {
	for {
		if _, err := unix.Write(w.pipe[1], []byte{0}); !errors.Is(err, unix.EINTR) {
			return
		}
	}
}

func (w *ttyWaker) empty() {
	buf := make([]byte, 64)
	for {
		n, err := unix.Read(w.pipe[0], buf)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if n <= 0 {
			return
		}
	}
}

func (w *ttyWaker) Close() error {
	return errors.Join(unix.Close(w.pipe[0]), unix.Close(w.pipe[1]))
}
