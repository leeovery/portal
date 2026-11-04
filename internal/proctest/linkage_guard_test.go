package proctest_test

import (
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/sourceguardtest"
)

const proctestPkg = "github.com/leeovery/portal/internal/proctest"

// Each lane is read: portalbintest builds the binary with the integration
// tag, so a production file under it would ship in what the tests run.
func TestPortalBinaryImportsNoProctest(t *testing.T) {
	root := sourceguardtest.InDir(sourceguardtest.ProjectRoot(t))
	for _, lane := range sourceguardtest.Lanes() {
		// Resolving the package first keeps a renamed path from passing for ever.
		if !slices.Contains(sourceguardtest.PackageDeps(t, proctestPkg, root, lane), proctestPkg) {
			t.Fatalf("%s does not resolve to itself — the guard is vacuous", proctestPkg)
		}
		if slices.Contains(sourceguardtest.PackageDeps(t, "github.com/leeovery/portal", root, lane), proctestPkg) {
			t.Errorf("the portal binary imports %s in a lane — test-only code must stay out of production", proctestPkg)
		}
	}
}
