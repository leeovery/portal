package tmuxtest_test

import (
	"go/ast"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/sourceguardtest"
)

const tmuxtestPkg = "github.com/leeovery/portal/internal/tmuxtest"

func TestTmuxtestPackage(t *testing.T) {
	// The harness serves portal migrate's tests, which must not drag a
	// dependency of its own into them: the standard library alone, taken
	// across other modules as well as this one.
	t.Run("it confines internal/tmuxtest to the standard library", func(t *testing.T) {
		for _, lane := range sourceguardtest.Lanes() {
			sourceguardtest.AssertDepsWithin(t, tmuxtestPkg, nil, sourceguardtest.ForbiddingThirdParty(), lane)
		}
	})

	t.Run("it finds tmuxtest imported only by portal migrate's tests", func(t *testing.T) {
		for _, found := range scanTmuxtestImporters(t) {
			t.Errorf("%s imports %s; only portal migrate's tests run tmux", found, tmuxtestPkg)
		}
	})

	t.Run("it reports every importer but migrate's tests, cmd's migrate tests and tmuxtest itself", func(t *testing.T) {
		root := stageTree(t, map[string]bool{
			"internal/migrate/tmux_realtmux_test.go": true,
			"cmd/migrate_integration_test.go":        true,
			"internal/tmuxtest/socket.go":            true,
			"internal/migrate/tmux.go":               true,
			"cmd/migrate.go":                         true,
			"cmd/migrate_test.go":                    true,
			"cmd/open_test.go":                       true,
			"internal/portaltest/isolated_test.go":   true,
			"internal/state/state_test.go":           false,
		})
		var rec harnesstest.Recorder
		var found []string
		rec.Run(func() { found = scanTmuxtestImporters(&rec, sourceguardtest.Rooted(root)) })

		want := []string{"cmd/migrate.go", "cmd/migrate_test.go", "cmd/open_test.go", "internal/migrate/tmux.go", "internal/portaltest/isolated_test.go"}
		if rec.Failed() || !slices.Equal(found, want) {
			t.Errorf("scanTmuxtestImporters reported %q and %s, want %q", found, rec.Report(), want)
		}
	})

	t.Run("it fails when it finds no test of portal migrate importing it", func(t *testing.T) {
		root := stageTree(t, map[string]bool{"internal/state/state_test.go": false})
		var rec harnesstest.Recorder
		rec.Run(func() { scanTmuxtestImporters(&rec, sourceguardtest.Rooted(root)) })
		if len(rec.Fatals) != 1 {
			t.Errorf("scanTmuxtestImporters over a tree with no importer reported %s, want one fatal", rec.Report())
		}
	})
}

// scanTmuxtestImporters returns, sorted, each source in the tree the options
// name that imports tmuxtest and isn't allowed to. Finding no allowed
// importer is fatal: the scan has stopped seeing the tests the rule serves.
func scanTmuxtestImporters(t harnesstest.TestingT, opts ...sourceguardtest.ScanOption) []string {
	t.Helper()
	_, sources := sourceguardtest.RepoSources(t, sourceguardtest.AllSources, opts...)
	var allowed int
	var found []string
	for _, source := range sources {
		file := filepath.ToSlash(source.Path)
		if !slices.ContainsFunc(source.File.Imports, func(spec *ast.ImportSpec) bool {
			p, err := strconv.Unquote(spec.Path.Value)
			return err == nil && p == tmuxtestPkg
		}) {
			continue
		}
		if mayImportTmuxtest(file) {
			allowed++
			continue
		}
		found = append(found, file)
	}
	if allowed == 0 {
		t.Fatalf("no test of portal migrate imports %s; the scan no longer sees the tests it serves", tmuxtestPkg)
		return nil
	}
	slices.Sort(found)
	return found
}

// mayImportTmuxtest reports whether the source at the module path file may
// import tmuxtest: migrate's tests, cmd's migrate_*_test.go and tmuxtest's
// own sources.
func mayImportTmuxtest(file string) bool {
	dir, base := path.Split(file)
	isTest := strings.HasSuffix(base, "_test.go")
	switch dir {
	case "internal/tmuxtest/":
		return true
	case "internal/migrate/":
		return isTest
	case "cmd/":
		matched, _ := path.Match("migrate_*_test.go", base)
		return matched
	}
	return false
}

// stageTree writes a Go source at each module path of files under a temp
// root, importing tmuxtest where its value is true, and returns the root.
func stageTree(t *testing.T, files map[string]bool) string {
	t.Helper()
	root := t.TempDir()
	for file, imports := range files {
		pkg := path.Base(path.Dir(file))
		src := "package " + pkg + "\n"
		if imports {
			src += "\nimport _ " + strconv.Quote(tmuxtestPkg) + "\n"
		}
		at := filepath.Join(root, filepath.FromSlash(file))
		if err := os.MkdirAll(filepath.Dir(at), 0o755); err != nil {
			t.Fatalf("stage %s: %v", file, err)
		}
		if err := os.WriteFile(at, []byte(src), 0o644); err != nil {
			t.Fatalf("stage %s: %v", file, err)
		}
	}
	return root
}
