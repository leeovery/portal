package sourceguardtest_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/sourceguardtest"
)

func TestModulePackages(t *testing.T) {
	const self = "github.com/leeovery/portal/internal/sourceguardtest"
	root := sourceguardtest.ProjectRoot(t)
	find := func(listed []sourceguardtest.ListedPackage, path string) (sourceguardtest.ListedPackage, bool) {
		i := slices.IndexFunc(listed, func(p sourceguardtest.ListedPackage) bool { return p.ImportPath == path })
		if i < 0 {
			return sourceguardtest.ListedPackage{}, false
		}
		return listed[i], true
	}

	t.Run("it lists each package of the module with its directory and every dependency", func(t *testing.T) {
		pkg, ok := find(sourceguardtest.ModulePackages(t, root), self)

		if !ok {
			t.Fatalf("%s is not listed", self)
		}
		if pkg.Dir != filepath.Join(root, "internal", "sourceguardtest") {
			t.Errorf("dir = %s, want this package's", pkg.Dir)
		}
		if !slices.Contains(pkg.Deps, "github.com/leeovery/portal/internal/harnesstest") || !slices.Contains(pkg.Deps, "go/types") {
			t.Errorf("deps = %v, want harnesstest and go/types among them", pkg.Deps)
		}
	})

	t.Run("it lists the build it is given", func(t *testing.T) {
		const bootid = "github.com/leeovery/portal/internal/bootid"
		host, _ := find(sourceguardtest.ModulePackages(t, root), bootid)
		linux, _ := find(sourceguardtest.ModulePackages(t, root, sourceguardtest.ForGOOS("linux")), bootid)

		if slices.Equal(host.Deps, linux.Deps) {
			t.Errorf("bootid's deps are %v on the host and on linux, want each platform's own", host.Deps)
		}
	})

	t.Run("a listing go list can't make is fatal", func(t *testing.T) {
		recorder := &harnesstest.Recorder{}
		recorder.Run(func() { sourceguardtest.ModulePackages(recorder, t.TempDir()) })

		if len(recorder.Fatals) != 1 {
			t.Errorf("fatals = %v, want one", recorder.Fatals)
		}
	})
}
