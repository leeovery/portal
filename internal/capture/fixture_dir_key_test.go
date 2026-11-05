package capture

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/portal/internal/prefs"
)

// No fixture resolves its paths on disk: each one's deps match sessions to
// projects by a DirKey that leaves a symlink as written, and each grouped
// fixture's build reduces its paths through that key, not CanonicalDirKey.
func TestNoFixtureResolvesItsPathsOnDisk(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "link")
	if err := os.Symlink(t.TempDir(), link); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	th := framesTheme(t)
	grouped := 0
	for _, build := range fixtureBuilders() {
		f := build()
		deps := f.Deps(th)
		if deps.DirKey == nil {
			t.Errorf("%s: Deps carries no DirKey, so its build resolves its paths on disk", f.Name())
			continue
		}
		if got := deps.DirKey(link); got != link {
			t.Errorf("%s: DirKey(%q) = %q, want it as written: the key resolved it on disk", f.Name(), link, got)
		}
		reduced := 0
		key := deps.DirKey
		deps.DirKey = func(dir string) string {
			reduced++
			return key(dir)
		}
		f.modelAt(deps, harnessSize.Width, harnessSize.Height)
		if f.initialMode == prefs.ModeFlat {
			continue
		}
		grouped++
		if reduced == 0 {
			t.Errorf("%s: its build matched its projects without DirKey", f.Name())
		}
	}
	if grouped == 0 {
		t.Error("no fixture opens grouped, so nothing proves a build matches through DirKey")
	}
}
