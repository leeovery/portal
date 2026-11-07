package vtcapture_test

import (
	"testing"

	"github.com/leeovery/portal/internal/sourceguardtest"
)

const vtcapturePkg = "github.com/leeovery/portal/internal/vtcapture"

// The capture format is read by the saver, which must not reach the zmx
// driver, and by the hydrate helper, so its reader can only ever depend on
// the standard library: an empty allowlist, taken across other modules as
// well as this one.
func TestVTCapturePackage_DependsOnTheStandardLibraryAlone(t *testing.T) {
	for _, lane := range sourceguardtest.Lanes() {
		sourceguardtest.AssertDepsWithin(t, vtcapturePkg, nil, sourceguardtest.ForbiddingThirdParty(), lane)
	}
}
