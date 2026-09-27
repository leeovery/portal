//go:build linux

package cmd

import "golang.org/x/sys/unix"

// flushTTYInput discards the terminal's input queue without reading it.
func flushTTYInput(fd int) error {
	return unix.IoctlSetInt(fd, unix.TCFLSH, unix.TCIFLUSH)
}
