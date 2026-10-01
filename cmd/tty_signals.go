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
	return updateTTYModes(fd, func(m *unix.Termios) { m.Lflag &^= unix.ISIG })
}

func setTTYSignals(fd int) error {
	return updateTTYModes(fd, func(m *unix.Termios) { m.Lflag |= unix.ISIG })
}

// clearTTYEcho turns off the terminal's echo alone, so input reaching it is
// not displayed over whatever was painted.
func clearTTYEcho(fd int) error {
	return updateTTYModes(fd, func(m *unix.Termios) { m.Lflag &^= unix.ECHO })
}

func setTTYEcho(fd int) error {
	return updateTTYModes(fd, func(m *unix.Termios) { m.Lflag |= unix.ECHO })
}

// cookTTY turns on the modes a shell reading the terminal needs and a terminal
// left raw lacks: line editing, echo, signal generation, CR-to-NL input and
// flow control, and output processing.
func cookTTY(fd int) error {
	return updateTTYModes(fd, func(m *unix.Termios) {
		m.Lflag |= unix.ICANON | unix.ECHO | unix.ECHOE | unix.ECHOK | unix.ISIG | unix.IEXTEN
		m.Iflag |= unix.ICRNL | unix.IXON
		m.Oflag |= unix.OPOST | unix.ONLCR
	})
}

func updateTTYModes(fd int, update func(*unix.Termios)) error {
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

func cookStdin() error {
	return cookTTY(int(os.Stdin.Fd()))
}

func clearStdinEcho() error {
	return clearTTYEcho(int(os.Stdin.Fd()))
}

func setStdinEcho() error {
	return setTTYEcho(int(os.Stdin.Fd()))
}
