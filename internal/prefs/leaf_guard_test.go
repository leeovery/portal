package prefs_test

import (
	"os/exec"
	"strings"
	"testing"
)

// prefsPkg is the import path of the package under test.
const prefsPkg = "github.com/leeovery/portal/internal/prefs"

// forbiddenLeafDeps are internal packages that internal/prefs must never depend
// on, transitively. prefs is documented as a pure leaf (stdlib + internal/fileutil
// only) so it can be imported from internal/tui without an import cycle, and it is
// deliberately outside the closed state-mutation audit-trail set — so it must not
// pull in the logging machinery.
var forbiddenLeafDeps = []string{
	"github.com/leeovery/portal/internal/log",
	"github.com/leeovery/portal/internal/storelog",
}

// TestPrefsIsALeaf asserts internal/prefs' full transitive internal-dependency set
// contains only internal/fileutil (and itself) — proving it stays a leaf and never
// pulls in internal/log or internal/storelog. Modelled on cmd/capturetool's
// go-list-deps import guard.
func TestPrefsIsALeaf(t *testing.T) {
	// Anchored at the import path (not a relative dir) so it resolves regardless
	// of the test binary's runtime CWD.
	cmd := exec.Command("go", "list", "-deps", prefsPkg)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps %s: %v\n%s", prefsPkg, err, out)
	}

	deps := strings.Fields(string(out))
	for _, dep := range deps {
		for _, forbidden := range forbiddenLeafDeps {
			if dep == forbidden {
				t.Fatalf("internal/prefs transitively imports %s — prefs must stay a leaf (stdlib + internal/fileutil only)", forbidden)
			}
		}
	}

	// Positive sanity check: the only internal dependency (besides itself) is
	// internal/fileutil, so the guard above is meaningful rather than vacuous.
	const fileutilPkg = "github.com/leeovery/portal/internal/fileutil"
	var sawFileutil bool
	for _, dep := range deps {
		if !strings.HasPrefix(dep, "github.com/leeovery/portal/internal/") {
			continue
		}
		switch dep {
		case prefsPkg, fileutilPkg:
			if dep == fileutilPkg {
				sawFileutil = true
			}
		default:
			t.Errorf("internal/prefs has an unexpected internal dependency %s — prefs is meant to be a leaf over stdlib + internal/fileutil", dep)
		}
	}
	if !sawFileutil {
		t.Errorf("internal/prefs no longer depends on %s — the leaf guard may be vacuous", fileutilPkg)
	}
}
