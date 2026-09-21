package prefs_test

import (
	"testing"

	"github.com/leeovery/portal/internal/sourceguardtest"
)

const prefsPkg = "github.com/leeovery/portal/internal/prefs"

// prefs stays a leaf over stdlib plus internal/fileutil and internal/resumemode
// so internal/tui can import it without a cycle — which rules out internal/log
// and the store logging built on it.
var prefsMayImport = []string{
	"github.com/leeovery/portal/internal/fileutil",
	"github.com/leeovery/portal/internal/resumemode",
}

func TestPrefsIsALeaf(t *testing.T) {
	for _, lane := range sourceguardtest.Lanes() {
		sourceguardtest.AssertDepsWithin(t, prefsPkg, prefsMayImport, lane)
	}
}
