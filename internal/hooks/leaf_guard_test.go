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
// it back. internal/state — which owns the pane-token option and the capture
// column carrying the token — is the package most likely to want this store,
// and an import of the session or tmux tree here locks it out for good.
var hooksMayImport = []string{
	modulePrefix + "internal/fileutil",
	modulePrefix + "internal/log",
	modulePrefix + "internal/nanoid",
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

	t.Run("it drags in no session, tmux or state tree transitively", func(t *testing.T) {
		for _, lane := range sourceguardtest.Lanes() {
			sourceguardtest.AssertDepsWithin(t, hooksPkg, hooksMayImport, lane)
		}
	})
}
