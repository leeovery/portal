package backend_test

import (
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/sourceguardtest"
)

const portalMainPkg = "github.com/leeovery/portal"

// backendTestPackages are the backend's test-only packages: the in-memory
// fake and the real-zmx harness.
var backendTestPackages = []string{
	"github.com/leeovery/portal/internal/backendtest",
	"github.com/leeovery/portal/internal/zmxtest",
}

// Each lane is read: portalbintest builds the binary with the integration
// tag, so a production file under it would ship in what the tests run.
func TestPortalBinaryImportsNoBackendTestPackage(t *testing.T) {
	root := sourceguardtest.InDir(sourceguardtest.ProjectRoot(t))
	for _, lane := range sourceguardtest.Lanes() {
		deps := sourceguardtest.PackageDeps(t, portalMainPkg, root, lane)
		for _, pkg := range backendTestPackages {
			// Resolving each one first keeps a renamed or mistyped path from
			// passing for ever.
			if !slices.Contains(sourceguardtest.PackageDeps(t, pkg, root, lane), pkg) {
				t.Fatalf("%s does not resolve to itself — the guard is vacuous", pkg)
			}
			if slices.Contains(deps, pkg) {
				t.Errorf("the portal binary (%s) imports %s in a lane — test-only code must stay out of production", portalMainPkg, pkg)
			}
		}
	}
}
