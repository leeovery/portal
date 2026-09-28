//go:build darwin || linux

package cmd

import (
	"os"

	"golang.org/x/sys/unix"
)

// clearTTYSignals turns off the terminal's signal generation, so Ctrl-C, Ctrl-\
// and Ctrl-Z reach its reader as bytes instead of signalling its foreground
// process group. Output processing is left as it is.
func clearTTYSignals(fd int) error {
	return updateLocalModes(fd, func(m *unix.Termios) { m.Lflag &^= unix.ISIG })
}

func setTTYSignals(fd int) error {
	return updateLocalModes(fd, func(m *unix.Termios) { m.Lflag |= unix.ISIG })
}

func updateLocalModes(fd int, update func(*unix.Termios)) error {
	modes, err := unix.IoctlGetTermios(fd, ioctlReadTermios)
	if err != nil {
		return err
	}
	update(modes)
	return unix.IoctlSetTermios(fd, ioctlWriteTermios, modes)
}

func clearStdinSignals() error {
	return clearTTYSignals(int(os.Stdin.Fd()))
}

func setStdinSignals() error {
	return setTTYSignals(int(os.Stdin.Fd()))
}
