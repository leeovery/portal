package migrate_test

import (
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/sourceguardtest"
)

const (
	migratePkg = "github.com/leeovery/portal/internal/migrate"
	tmuxPkg    = "github.com/leeovery/portal/internal/tmux"
)

// Migrate carries the little tmux it needs, so the tmux client package can be
// deleted whole once migrate is the only code that knows tmux.
func TestMigrateImportsNoTmuxClient(t *testing.T) {
	root := sourceguardtest.InDir(sourceguardtest.ProjectRoot(t))
	for _, lane := range sourceguardtest.Lanes() {
		deps := sourceguardtest.PackageDeps(t, migratePkg, root, lane)
		if !slices.Contains(deps, migratePkg) {
			t.Fatalf("%s does not resolve to itself — the guard is vacuous", migratePkg)
		}
		if slices.Contains(deps, tmuxPkg) {
			t.Errorf("%s reaches %s in a lane; migrate's tmux calls are its own", migratePkg, tmuxPkg)
		}
	}
}
