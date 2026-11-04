package fileutil

import "testing"

// FailDirSyncForTest makes the directory sync after every rename answer err
// for the duration of the test, as a failing disk would once the file is
// replaced.
func FailDirSyncForTest(t testing.TB, err error) {
	t.Helper()
	prev := syncDir
	syncDir = func(string) error { return err }
	t.Cleanup(func() { syncDir = prev })
}
