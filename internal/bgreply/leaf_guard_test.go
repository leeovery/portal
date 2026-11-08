package bgreply_test

import (
	"testing"

	"github.com/leeovery/portal/internal/sourceguardtest"
)

const bgreplyPkg = "github.com/leeovery/portal/internal/bgreply"

// The replies are read by the draw's probe and by the waiter, which must not
// import the rendering path, so the package can only ever depend on the
// standard library.
func TestBgreplyPackage_DependsOnTheStandardLibraryAlone(t *testing.T) {
	for _, lane := range sourceguardtest.Lanes() {
		sourceguardtest.AssertDepsWithin(t, bgreplyPkg, nil, sourceguardtest.ForbiddingThirdParty(), lane)
	}
}
