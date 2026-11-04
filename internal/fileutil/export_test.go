package fileutil

import (
	"os"
	"testing"
)

// SetSyncsForTest replaces the temp file's sync and the directory's sync for
// the duration of the test.
func SetSyncsForTest(tb testing.TB, file func(*os.File) error, dir func(string) error) {
	tb.Helper()
	prevFile, prevDir := syncFile, syncDir
	syncFile, syncDir = file, dir
	tb.Cleanup(func() { syncFile, syncDir = prevFile, prevDir })
}
