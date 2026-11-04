package saver_test

import (
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/sourceguardtest"
)

const (
	saverPkg     = "github.com/leeovery/portal/internal/saver"
	zmxDriverPkg = "github.com/leeovery/portal/internal/backend/zmx"
)

// The saver reaches its sessions through backend alone, and reads their
// captures through vtcapture rather than the zmx driver.
func TestSaverPackage_ReachesNoBackendDriver(t *testing.T) {
	for _, lane := range sourceguardtest.Lanes() {
		// Resolving the driver first keeps a renamed or mistyped path from
		// passing for ever.
		if !slices.Contains(sourceguardtest.PackageDeps(t, zmxDriverPkg, lane), zmxDriverPkg) {
			t.Fatalf("%s does not resolve to itself — the guard is vacuous", zmxDriverPkg)
		}
		if slices.Contains(sourceguardtest.PackageDeps(t, saverPkg, lane), zmxDriverPkg) {
			t.Errorf("%s depends on %s in a lane — it may reach sessions through backend alone", saverPkg, zmxDriverPkg)
		}
	}
}
