package sourceguardtest_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/portal/internal/sourceguardtest"
)

// A guard reaches the module root through this resolution, whether it goes on to
// scan the tree or merely to name a path within it.
func TestProjectRoot(t *testing.T) {
	t.Run("it resolves the module root the scan is anchored at", func(t *testing.T) {
		root := sourceguardtest.ProjectRoot(t)

		if !filepath.IsAbs(root) {
			t.Fatalf("root = %q, want an absolute path", root)
		}
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
			t.Fatalf("no go.mod at %s: %v", root, err)
		}
		if scanned, _ := sourceguardtest.RepoSources(t, sourceguardtest.AllSources); scanned != root {
			t.Errorf("RepoSources anchors at %q, want the same root %q", scanned, root)
		}
	})
}
