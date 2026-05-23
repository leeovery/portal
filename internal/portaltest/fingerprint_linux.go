//go:build linux

package portaltest

import "syscall"

// statTimeNanos returns (mtime, ctime) in nanoseconds since the
// Unix epoch on linux, where syscall.Stat_t uses Mtim / Ctim.
func statTimeNanos(st *syscall.Stat_t) (mtime, ctime int64) {
	return st.Mtim.Nano(), st.Ctim.Nano()
}
