//go:build linux

package state

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// A filesystem that does not support RENAME_NOREPLACE rejects it with EINVAL.
func platformRenameNoReplace(src, dst string) error {
	err := unix.Renameat2(unix.AT_FDCWD, src, unix.AT_FDCWD, dst, unix.RENAME_NOREPLACE)
	if errors.Is(err, unix.EINVAL) {
		return errNoReplaceUnsupported
	}
	if err != nil {
		return &os.LinkError{Op: "rename", Old: src, New: dst, Err: err}
	}
	return nil
}
