//go:build !darwin

package fileutil

import "os"

// syncFD is fsync.
func syncFD(f *os.File) error { return f.Sync() }
