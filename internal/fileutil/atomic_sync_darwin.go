//go:build darwin

package fileutil

import (
	"os"

	"golang.org/x/sys/unix"
)

// syncFD is F_BARRIERFSYNC, fsync on a filesystem without barriers. It writes
// f's data to the drive and puts a barrier behind it, so nothing issued later
// persists ahead of it. Unlike F_FULLFSYNC, which File.Sync makes on darwin,
// it doesn't flush the drive's whole cache, so the newest writes may be lost.
// On a directory it puts a rename just made there behind the barrier too:
// without it the rename waits for APFS's next commit, and a later write, such
// as an index naming the file just renamed, could persist ahead of it.
func syncFD(f *os.File) error {
	return withFD(f, func(fd uintptr) error { return orderedSync(fd, barrierFsync, plainFsync) })
}

func barrierFsync(fd uintptr) error {
	_, err := unix.FcntlInt(fd, unix.F_BARRIERFSYNC, 0)
	return err
}

func plainFsync(fd uintptr) error { return unix.Fsync(int(fd)) }
