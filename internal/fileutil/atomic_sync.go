package fileutil

import (
	"errors"
	"os"
	"syscall"
)

// unsupported reports whether a sync was refused as one the filesystem can't
// make, rather than failed.
func unsupported(err error) bool {
	return errors.Is(err, syscall.ENOTSUP) || errors.Is(err, syscall.EINVAL)
}

// orderedSync runs sync on fd, and fallback in its place when the filesystem
// can't make that sync. An interrupted call is made again.
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

// syncDirectory makes a rename into the directory dir persist. A filesystem
// that can't sync a directory has nothing more to flush.
func syncDirectory(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()
	if err := syncDirFD(d); err != nil && !unsupported(err) {
		return err
	}
	return nil
}
