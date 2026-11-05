package migrate

import "testing"

// SetRenameForTest makes rename the commit point's renames for the rest of
// t, so a test can watch each one land, or fail it.
func SetRenameForTest(t testing.TB, rename func(from, to string) error) {
	t.Helper()
	old := renameDir
	renameDir = rename
	t.Cleanup(func() { renameDir = old })
}
