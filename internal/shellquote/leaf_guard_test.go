package shellquote_test

import (
	"testing"

	"github.com/leeovery/portal/internal/sourceguardtest"
)

const shellquotePkg = "github.com/leeovery/portal/internal/shellquote"

// The quoting rule is shared by packages that must not import each other, so it
// can only ever depend on the standard library: an empty allowlist, taken
// across other modules as well as this one.
func TestShellQuotePackage_DependsOnTheStandardLibraryAlone(t *testing.T) {
	for _, lane := range sourceguardtest.Lanes() {
		sourceguardtest.AssertDepsWithin(t, shellquotePkg, nil, sourceguardtest.ForbiddingThirdParty(), lane)
	}
}
