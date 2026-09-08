package harnesstest_test

import (
	"testing"

	"github.com/leeovery/portal/internal/sourceguardtest"
)

const harnessTestPkg = "github.com/leeovery/portal/internal/harnesstest"

func TestHarnessTestPackage(t *testing.T) {
	// Every test package in the tree may reach for the stand-in, including
	// packages that must not import each other, so it can only ever depend on
	// the standard library: an empty allowlist, taken across other modules as
	// well as this one.
	t.Run("it confines internal/harnesstest to the standard library", func(t *testing.T) {
		for _, lane := range sourceguardtest.Lanes() {
			sourceguardtest.AssertDepsWithin(t, harnessTestPkg, nil, sourceguardtest.ForbiddingThirdParty(), lane)
		}
	})
}
