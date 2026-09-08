package nanoid_test

import (
	"testing"

	"github.com/leeovery/portal/internal/sourceguardtest"
)

const nanoidPkg = "github.com/leeovery/portal/internal/nanoid"

// The id vocabulary is shared by packages that must not import each other, so
// it can only ever depend on the standard library: an empty allowlist, taken
// across other modules as well as this one.
func TestNanoIDPackage_DependsOnTheStandardLibraryAlone(t *testing.T) {
	for _, lane := range sourceguardtest.Lanes() {
		sourceguardtest.AssertDepsWithin(t, nanoidPkg, nil, sourceguardtest.ForbiddingThirdParty(), lane)
	}
}
