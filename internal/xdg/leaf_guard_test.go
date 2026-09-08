package xdg_test

import (
	"testing"

	"github.com/leeovery/portal/internal/sourceguardtest"
)

const xdgPkg = "github.com/leeovery/portal/internal/xdg"

func TestXDGPackage(t *testing.T) {
	// xdg declares the per-file config-path precedence, and internal/hookstest
	// resolves hooks.json through that same declaration so a destructive suite
	// seeds the file the binary under test actually reads. A dependency here
	// that pulled the resolution behind some other package's wiring would put
	// the seeder and the binary on paths that can drift apart, and a suite
	// seeding a file nothing reads passes on it. So it can only ever depend on
	// the standard library: an empty allowlist, taken across other modules as
	// well as this one.
	t.Run("it confines internal/xdg to the standard library", func(t *testing.T) {
		for _, lane := range sourceguardtest.Lanes() {
			sourceguardtest.AssertDepsWithin(t, xdgPkg, nil, sourceguardtest.ForbiddingThirdParty(), lane)
		}
	})
}
