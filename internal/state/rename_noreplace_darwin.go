//go:build darwin

package state

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// A volume lacking VOL_CAP_INT_RENAME_EXCL rejects the flag with ENOTSUP.
func platformRenameNoReplace(src, dst string) error {
	err := unix.RenamexNp(src, dst, unix.RENAME_EXCL)
	if errors.Is(err, unix.ENOTSUP) {
		return errNoReplaceUnsupported
	}
	if err != nil {
		return &os.LinkError{Op: "rename", Old: src, New: dst, Err: err}
	}
	return nil
}
