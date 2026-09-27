//go:build darwin

package cmd

import "golang.org/x/sys/unix"

// FREAD from <sys/fcntl.h>, which x/sys/unix does not export on darwin.
const freadOnly = 0x1

// flushTTYInput discards the terminal's input queue without reading it.
func flushTTYInput(fd int) error {
	return unix.IoctlSetPointerInt(fd, unix.TIOCFLUSH, freadOnly)
}
