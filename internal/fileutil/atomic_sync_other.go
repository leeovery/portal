//go:build !darwin

package fileutil

import "os"

// syncFileData writes the temp file's bytes through before the rename, so
// the rename never names data still in cache.
func syncFileData(f *os.File) error { return f.Sync() }

func syncDirFD(d *os.File) error { return d.Sync() }
