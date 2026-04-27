package state

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"syscall"
)

// CreateFIFO creates (or replaces) a POSIX FIFO at path with mode 0600.
//
// The function is unconditionally destructive: any pre-existing file at path
// — regular file, symlink, prior FIFO — is removed before the new FIFO is
// made. This guarantees callers a fresh inode every time, which matters for
// the hydration-FIFO lifecycle: a stale FIFO from a crashed bootstrap or a
// dead helper may have the desired mode but is still owned by no live reader,
// and reusing it would leave the writer side blocked indefinitely.
//
// ENOENT from the initial os.Remove is the expected, common case (fresh path)
// and is silently swallowed. Any other os.Remove error is wrapped with path
// so failures surface clearly in portal.log.
//
// A defensive os.Chmod follows syscall.Mkfifo to neutralise an unusually-tight
// process umask that would otherwise leave the FIFO mode below 0600. Its
// error is intentionally ignored: if the chmod fails the FIFO already exists
// at the umask-masked mode, which is no worse than not running this guard,
// and surfacing the failure would mask the underlying mkfifo success.
//
// CreateFIFO is unix-only (syscall.Mkfifo is unavailable on Windows). Portal
// supports darwin and linux, so no build tags are required.
func CreateFIFO(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("create fifo %s: remove existing: %w", path, err)
	}
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		return fmt.Errorf("create fifo %s: mkfifo: %w", path, err)
	}
	_ = os.Chmod(path, 0o600) // defensive against umask
	return nil
}
