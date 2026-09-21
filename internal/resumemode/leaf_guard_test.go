package resumemode_test

import (
	"testing"

	"github.com/leeovery/portal/internal/sourceguardtest"
)

const resumeModePkg = "github.com/leeovery/portal/internal/resumemode"

// The resume-mode vocabulary is shared by two guarded leaves that must not
// import each other, so it can only ever depend on the standard library: an
// empty allowlist, taken across other modules as well as this one.
func TestResumeModePackage_DependsOnTheStandardLibraryAlone(t *testing.T) {
	t.Run("it depends on the standard library alone", func(t *testing.T) {
		for _, lane := range sourceguardtest.Lanes() {
			sourceguardtest.AssertDepsWithin(t, resumeModePkg, nil, sourceguardtest.ForbiddingThirdParty(), lane)
		}
	})
}
