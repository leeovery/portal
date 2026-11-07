package sourceguardtest

import (
	"bytes"
	"encoding/json"
	"os/exec"

	"github.com/leeovery/portal/internal/harnesstest"
)

// ListedPackage is one package of a module as go list reports it under a
// build: its import path, its directory, and every package it depends on,
// directly or not.
type ListedPackage struct {
	ImportPath string
	Dir        string
	Deps       []string
}

// ModulePackages lists every package of the module rooted at root, under the
// build WithBuildTags and ForGOOS name. Each package's directory is read, as
// PackageDeps reads its dependencies', so an edit to any of them moves the
// judging test's cache key. A listing go list cannot make, and one that comes
// back empty, are fatal.
func ModulePackages(t harnesstest.TestingT, root string, opts ...DepsOption) []ListedPackage {
	t.Helper()

	cfg := newDepsConfig(opts)
	args := append([]string{"list", "-json=ImportPath,Dir,Deps"}, buildFlags(cfg)...)
	cmd := exec.Command("go", append(args, "./...")...)
	cmd.Dir = root
	cmd.Env = buildEnv(cfg)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list ./... in %s%s: %v", root, cfg.lane(), listFailure(err))
		return nil
	}
	var listed []ListedPackage
	decoder := json.NewDecoder(bytes.NewReader(out))
	for decoder.More() {
		var pkg ListedPackage
		if err := decoder.Decode(&pkg); err != nil {
			t.Fatalf("decode go list output for %s%s: %v", root, cfg.lane(), err)
			return nil
		}
		readPackageSources(pkg.Dir)
		listed = append(listed, pkg)
	}
	if len(listed) == 0 {
		t.Fatalf("go list ./... in %s%s listed no packages, so a guard over them would pass by having stopped looking", root, cfg.lane())
		return nil
	}
	return listed
}
