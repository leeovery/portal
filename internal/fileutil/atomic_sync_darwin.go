//go:build darwin

package fileutil

import (
	"os"

	"golang.org/x/sys/unix"
)

// syncFileData orders the temp file's bytes ahead of the rename that follows.
// F_BARRIERFSYNC writes them to the drive and puts a barrier behind them, so
// the drive can't persist the rename before the data it names: after a power
// loss the file is its old version or its new one, never empty. Unlike
// F_FULLFSYNC, which File.Sync makes on darwin, it doesn't flush the drive's
// whole cache, so the newest write may still be lost. A filesystem without
// barriers gets fsync.
func syncFileData(f *os.File) error {
	return withFD(f, func(fd uintptr) error { return orderedSync(fd, barrierFsync, plainFsync) })
}

// syncDirFD puts the rename just made in the directory d behind a barrier
// too. Without it the rename waits for APFS's next commit, and a later write,
// such as an index naming the file just renamed, could persist ahead of it.
func syncDirFD(d *os.File) error {
	return withFD(d, func(fd uintptr) error { return orderedSync(fd, barrierFsync, plainFsync) })
}

func barrierFsync(fd uintptr) error {
	_, err := unix.FcntlInt(fd, unix.F_BARRIERFSYNC, 0)
	return err
}

func plainFsync(fd uintptr) error { return unix.Fsync(int(fd)) }
