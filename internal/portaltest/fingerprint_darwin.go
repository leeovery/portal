//go:build darwin

package portaltest

import "syscall"

// statTimeNanos returns (mtime, ctime) in nanoseconds since the
// Unix epoch on darwin, where syscall.Stat_t uses Mtimespec /
// Ctimespec.
func statTimeNanos(st *syscall.Stat_t) (mtime, ctime int64) {
	return st.Mtimespec.Nano(), st.Ctimespec.Nano()
}
