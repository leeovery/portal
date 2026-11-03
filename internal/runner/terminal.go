package runner

import (
	"errors"
	"fmt"
	"os"

	"github.com/charmbracelet/x/term"
	"golang.org/x/sys/unix"
)

// privateTerminal opens a description of the terminal f is open on that is
// Portal's alone, and non-blocking, so the descriptions the shell and its jobs
// share keep the flags they had. It opens the terminal by path, and checks the
// path still names f's device.
func privateTerminal(f *os.File) (int, error) {
	fd, err := fdOf(f)
	if err != nil {
		return -1, err
	}
	if !term.IsTerminal(uintptr(fd)) {
		return -1, fmt.Errorf("%s is not a terminal", f.Name())
	}
	path, err := terminalPath(fd)
	if err != nil {
		return -1, fmt.Errorf("find the terminal's path: %w", err)
	}
	private, err := unix.Open(path, unix.O_RDWR|unix.O_NOCTTY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, fmt.Errorf("open %s: %w", path, err)
	}
	if !sameDevice(fd, private) {
		_ = unix.Close(private)
		return -1, fmt.Errorf("%s is not the terminal's device", path)
	}
	return private, nil
}

// sameDevice reports whether a and b are open on the same character device.
func sameDevice(a, b int) bool {
	var sa, sb unix.Stat_t
	if unix.Fstat(a, &sa) != nil || unix.Fstat(b, &sb) != nil {
		return false
	}
	return sa.Mode&unix.S_IFMT == unix.S_IFCHR && sb.Mode&unix.S_IFMT == unix.S_IFCHR && sa.Rdev == sb.Rdev
}

// restoreTermios puts saved back on the terminal once it has taken all its
// output, dropping input not yet read.
func restoreTermios(fd int, saved *unix.Termios) {
	for {
		if err := unix.IoctlSetTermios(fd, setTermiosFlushed, saved); !errors.Is(err, unix.EINTR) {
			return
		}
	}
}
