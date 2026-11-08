//go:build darwin || linux

package cmd

import (
	"errors"
	"io"
	"time"

	"github.com/leeovery/portal/internal/bgreply"
	"golang.org/x/sys/unix"
)

var (
	errInputKeptArriving = errors.New("input kept arriving")
	errUnselectableTTY   = errors.New("terminal descriptor is outside the select set")
)

// ttyDrain discards a terminal's input until none has arrived for settle,
// giving up once bound has elapsed with input still arriving. It reads what it
// discards, so a reply to a background question among it is counted rather
// than lost unseen, and a reply cut off as the input falls quiet is waited on
// for its rest, bgreply.TailWait at most, so none is left half read.
type ttyDrain struct {
	settle, bound time.Duration
}

// resumeInputDrain settles for as long as the waiter waits for a session to
// fall quiet, the gap it judges one arrival by. What keeps the input arriving
// is the session's zmx daemon writing the rest of an attached client's input
// into the pty. The figure is kept from the tmux version, whose server wrote
// it within its own event loop; the gap under zmx has not been measured.
var resumeInputDrain = ttyDrain{settle: resumeInputQuiet, bound: time.Second}

// drain runs with canonical processing off, because a canonical terminal
// reports nothing readable for a line whose newline has not yet arrived, and
// puts back the modes it found before it returns. It answers how many replies
// it read, beside any error.
func (d ttyDrain) drain(fd int) (replies int, err error) {
	if fd < 0 || fd >= unix.FD_SETSIZE {
		return 0, errUnselectableTTY
	}
	found, err := unix.IoctlGetTermios(fd, ioctlReadTermios)
	if err != nil {
		return 0, err
	}
	draining := *found
	draining.Lflag &^= unix.ICANON
	// Non-canonical readiness is withheld below VMIN bytes when VTIME is zero.
	draining.Cc[unix.VMIN] = 1
	draining.Cc[unix.VTIME] = 0
	if err := unix.IoctlSetTermios(fd, ioctlWriteTermios, &draining); err != nil {
		return 0, err
	}
	defer func() {
		if restoreErr := unix.IoctlSetTermios(fd, ioctlWriteTermios, found); err == nil {
			err = restoreErr
		}
	}()

	deadline := time.Now().Add(d.bound)
	var read bgreply.Stream
	buf := make([]byte, 4096)
	for {
		window := d.settle
		if read.Unfinished() {
			window = max(window, bgreply.TailWait)
		}
		arrived, err := awaitTTYInput(fd, window)
		switch {
		case err != nil:
			return replies, err
		case !arrived:
			return replies, nil
		}
		n, err := readTTY(fd, buf)
		if err != nil {
			return replies, err
		}
		finished, _ := read.Take(buf[:n])
		replies += finished
		if time.Now().After(deadline) {
			return replies, errInputKeptArriving
		}
	}
}

// readTTY reads what fd holds once select has reported it readable.
func readTTY(fd int, buf []byte) (int, error) {
	for {
		n, err := unix.Read(fd, buf)
		switch {
		case errors.Is(err, unix.EINTR):
			continue
		case err != nil:
			return 0, err
		case n == 0:
			return 0, io.EOF
		}
		return n, nil
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
