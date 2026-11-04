package proctest_test

import (
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/sourceguardtest"
)

const proctestPkg = "github.com/leeovery/portal/internal/proctest"

func TestPortalBinaryImportsNoProctest(t *testing.T) {
	root := sourceguardtest.InDir(sourceguardtest.ProjectRoot(t))
	// Resolving the package first keeps a renamed path from passing for ever.
	if !slices.Contains(sourceguardtest.PackageDeps(t, proctestPkg, root), proctestPkg) {
		t.Fatalf("%s does not resolve to itself — the guard is vacuous", proctestPkg)
	}
	if slices.Contains(sourceguardtest.PackageDeps(t, "github.com/leeovery/portal", root), proctestPkg) {
		t.Errorf("the portal binary imports %s — test-only code must stay out of production", proctestPkg)
	}
}
