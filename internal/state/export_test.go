package state

import "testing"

// StubRenameNoReplaceUnsupported makes the no-replace rename report that the
// filesystem rejects its flag, for the rest of the test.
func StubRenameNoReplaceUnsupported(t *testing.T) {
	t.Helper()
	prev := renameNoReplace
	renameNoReplace = func(string, string) error { return errNoReplaceUnsupported }
	t.Cleanup(func() { renameNoReplace = prev })
}
