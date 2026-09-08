package sourceguardtest_test

import (
	"testing"

	"github.com/leeovery/portal/internal/sourceguardtest"
)

const sourceGuardTestPkg = "github.com/leeovery/portal/internal/sourceguardtest"

// The guard is written with the primitives it polices, which is the ordinary
// bootstrapping shape: a dependency this package should not have arrives in
// this suite's own dependency set too, so the assertion below judges it rather
// than being blinded to it.
func TestSourceGuardTestPackage(t *testing.T) {
	// The source guards across this module are unit-lane tests because the
	// scanning primitives they share reach no further than the standard
	// library and the stand-in they report through. Anything else here — a
	// third-party walker, or an internal package of the tree these guards
	// read — would drag its own dependencies, and any build tag among them,
	// into every one of those guards at once.
	t.Run("it confines internal/sourceguardtest to the standard library", func(t *testing.T) {
		// portalbintest is admitted for the root resolution the repo-wide scan
		// is anchored at, and it is admissible for the same reason the rule
		// exists: it is stdlib-only and untagged, so it drags neither a
		// dependency nor a lane onto the guards built here.
		for _, lane := range sourceguardtest.Lanes() {
			sourceguardtest.AssertDepsWithin(t, sourceGuardTestPkg, []string{
				"github.com/leeovery/portal/internal/harnesstest",
				"github.com/leeovery/portal/internal/portalbintest",
			}, sourceguardtest.ForbiddingThirdParty(), lane)
		}
	})

	// A primitive behind a tag carries that tag onto every guard reaching for
	// it, so a tag on any of these sources takes the guards built on them out
	// of the lane they are meant to run in.
	t.Run("it carries no build tag, so the guards built on it stay in the unit lane", func(t *testing.T) {
		for _, source := range sourceguardtest.ParsePackageSources(t, ".", false) {
			if expr, stated := sourceguardtest.BuildConstraint(source.File); stated {
				t.Errorf("%s carries the build constraint %s — a tag here gates every guard built on these primitives out of the unit lane", source.Path, expr)
			}
		}
	})
}
