package resumekeys_test

import (
	"testing"

	"github.com/leeovery/portal/internal/sourceguardtest"
)

const resumekeysPkg = "github.com/leeovery/portal/internal/resumekeys"

// The act keys are read by the renderers and by the waiter, which must not
// import the rendering path, so the package can only ever depend on the
// standard library: an empty allowlist, taken across other modules as well as
// this one.
func TestResumeKeysPackage_DependsOnTheStandardLibraryAlone(t *testing.T) {
	for _, lane := range sourceguardtest.Lanes() {
		sourceguardtest.AssertDepsWithin(t, resumekeysPkg, nil, sourceguardtest.ForbiddingThirdParty(), lane)
	}
}
