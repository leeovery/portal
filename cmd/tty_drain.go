//go:build darwin || linux

package cmd

import (
	"errors"
	"time"

	"golang.org/x/sys/unix"
)

var (
	errInputKeptArriving = errors.New("input kept arriving")
	errUnselectableTTY   = errors.New("terminal descriptor is outside the select set")
)

// ttyDrain discards a terminal's input until none has arrived for settle,
// giving up once bound has elapsed with input still arriving.
type ttyDrain struct {
	settle, bound time.Duration
}

// resumeInputDrain settles for as long as an escape waits for its next byte:
// tmux refills a flushed queue within its own event loop, far inside that.
var resumeInputDrain = ttyDrain{settle: resumeEscapeFollow, bound: time.Second}

// drain runs with canonical processing off, because a canonical terminal
// reports nothing readable for a line whose newline has not yet arrived, and
// puts back the modes it found before it returns.
func (d ttyDrain) drain(fd int) (err error) {
	if fd < 0 || fd >= unix.FD_SETSIZE {
		return errUnselectableTTY
	}
	found, err := unix.IoctlGetTermios(fd, ioctlReadTermios)
	if err != nil {
		return err
	}
	draining := *found
	draining.Lflag &^= unix.ICANON
	// Non-canonical readiness is withheld below VMIN bytes when VTIME is zero.
	draining.Cc[unix.VMIN] = 1
	draining.Cc[unix.VTIME] = 0
	if err := unix.IoctlSetTermios(fd, ioctlWriteTermios, &draining); err != nil {
		return err
	}
	defer func() {
		if restoreErr := unix.IoctlSetTermios(fd, ioctlWriteTermios, found); err == nil {
			err = restoreErr
		}
	}()

	deadline := time.Now().Add(d.bound)
	for {
		if err := flushTTYInput(fd); err != nil {
			return err
		}
		arrived, err := awaitTTYInput(fd, d.settle)
		switch {
		case err != nil:
			return err
		case !arrived:
			return nil
		case time.Now().After(deadline):
			return errInputKeptArriving
		}
	}
}

// awaitTTYInput reports whether fd became readable within window. Select rather
// than poll, which macOS does not support on a terminal device.
func awaitTTYInput(fd int, window time.Duration) (bool, error) {
	until := time.Now().Add(window)
	for {
		remaining := time.Until(until)
		if remaining <= 0 {
			return false, nil
		}
		var readable unix.FdSet
		readable.Set(fd)
		timeout := unix.NsecToTimeval(remaining.Nanoseconds())
		n, err := unix.Select(fd+1, &readable, nil, nil, &timeout)
		switch {
		case errors.Is(err, unix.EINTR):
			continue
		case err != nil:
			return false, err
		}
		return n > 0, nil
	}
}
