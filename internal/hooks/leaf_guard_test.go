package hooks_test

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/sourceguardtest"
)

const (
	modulePrefix = "github.com/leeovery/portal/"
	hooksPkg     = modulePrefix + "internal/hooks"
)

// The store is a JSON persistence leaf on the hydrate hot path, and the edge it
// would create is one-way and permanent: anything it imports can never import
// it back. internal/state, whose Forget removes a dropped session's hook,
// imports this store, and an import of the session or backend tree here locks
// it out for good. The staleness rule takes the names it judges against as
// strings for the same reason: the listing and the index stay the caller's.
var hooksMayImport = []string{
	modulePrefix + "internal/fileutil",
	modulePrefix + "internal/log",
	modulePrefix + "internal/resumemode",
	modulePrefix + "internal/shellquote",
	modulePrefix + "internal/storelog",
}

func TestHooksPackage_ImportsOnlyLeaves(t *testing.T) {
	t.Run("its own sources import no other internal package", func(t *testing.T) {
		for _, source := range sourceguardtest.ParsePackageSources(t, ".", false) {
			for _, spec := range source.File.Imports {
				imported, uerr := strconv.Unquote(spec.Path.Value)
				if uerr != nil {
					t.Fatalf("%s: unquote import path %s: %v", source.Path, spec.Path.Value, uerr)
				}
				if strings.HasPrefix(imported, modulePrefix) && !slices.Contains(hooksMayImport, imported) {
					t.Errorf("%s imports %s — internal/hooks may import only %v", source.Path, imported, hooksMayImport)
				}
			}
		}
	})

	t.Run("it drags in no session, backend or state tree transitively", func(t *testing.T) {
		for _, lane := range sourceguardtest.Lanes() {
			sourceguardtest.AssertDepsWithin(t, hooksPkg, hooksMayImport, lane)
		}
	})
}
