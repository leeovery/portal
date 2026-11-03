package proctable_test

import (
	"testing"

	"github.com/leeovery/portal/internal/sourceguardtest"
)

const proctablePkg = "github.com/leeovery/portal/internal/proctable"

// The process table is read by the zmx driver and by zmxtest alike, so it
// depends on nothing of Portal's.
func TestProctablePackage_DependsOnNothingOfPortals(t *testing.T) {
	for _, lane := range sourceguardtest.Lanes() {
		sourceguardtest.AssertDepsWithin(t, proctablePkg, nil, lane)
	}
}
