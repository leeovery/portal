package fileutil

import "testing"

// FailDirSyncForTest makes the directory sync after every rename answer err
// for the duration of the test, as a failing disk would once the file is
// replaced.
func FailDirSyncForTest(t testing.TB, err error) {
	t.Helper()
	SetDirSyncForTest(t, func(string) error { return err })
}

// SetDirSyncForTest makes sync the directory sync after every rename for the
// duration of the test, so a test can see each rename land, by the directory
// synced after it, and answer as the disk would.
func SetDirSyncForTest(t testing.TB, sync func(dir string) error) {
	t.Helper()
	prev := syncDir
	syncDir = sync
	t.Cleanup(func() { syncDir = prev })
}
