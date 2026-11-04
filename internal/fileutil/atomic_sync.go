package fileutil

import (
	"errors"
	"os"
	"syscall"
)

// syncUnsupported are the answers a filesystem gives for a sync it doesn't
// make, as some network and FUSE mounts answer: no sync failed, so the write
// goes on without one. ENOTSUP and EOPNOTSUPP differ on darwin.
var syncUnsupported = []syscall.Errno{syscall.ENOTSUP, syscall.EOPNOTSUPP, syscall.EINVAL, syscall.ENOTTY, syscall.ENOSYS}

func unsupported(err error) bool {
	for _, errno := range syncUnsupported {
		if errors.Is(err, errno) {
			return true
		}
	}
	return false
}

// skipUnsupported is err, nil when it only says the filesystem makes no such
// sync.
func skipUnsupported(err error) error {
	if unsupported(err) {
		return nil
	}
	return err
}

// orderedSync runs sync on fd, and fallback in its place when the filesystem
// makes no such sync. An interrupted call is made again.
func orderedSync(fd uintptr, sync, fallback func(fd uintptr) error) error {
	err := retryEINTR(func() error { return sync(fd) })
	if unsupported(err) {
		return retryEINTR(func() error { return fallback(fd) })
	}
	return err
}

func retryEINTR(call func() error) error {
	for {
		if err := call(); !errors.Is(err, syscall.EINTR) {
			return err
		}
	}
}

// withFD runs op on f's descriptor without taking it out of the runtime's
// hands, as Fd would.
func withFD(f *os.File, op func(fd uintptr) error) error {
	conn, err := f.SyscallConn()
	if err != nil {
		return err
	}
	var opErr error
	if err := conn.Control(func(fd uintptr) { opErr = op(fd) }); err != nil {
		return err
	}
	return opErr
}

// syncFileData orders the temp file's bytes ahead of the rename that follows.
// A real failure, such as EIO, fails the write before anything is renamed.
func syncFileData(f *os.File) error { return skipUnsupported(syncFD(f)) }

// syncDirectory orders the rename just made in the directory dir ahead of
// any later write.
func syncDirectory(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()
	return skipUnsupported(syncFD(d))
}
