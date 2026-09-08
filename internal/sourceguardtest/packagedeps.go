package sourceguardtest

import (
	"os/exec"
	"strings"

	"github.com/leeovery/portal/internal/harnesstest"
)

// DepsOption adjusts how a dependency assertion resolves and judges its
// package.
type DepsOption func(*depsConfig)

// InDir runs the enumeration from dir, so a guard can name its package
// relatively, or anchor resolution somewhere other than the test binary's own
// working directory.
func InDir(dir string) DepsOption {
	return func(cfg *depsConfig) { cfg.dir = dir }
}

// ForbiddingThirdParty widens an assertion to judge dependencies belonging to
// other modules as well as to the package's own, so that an empty allowlist
// states "the standard library alone" rather than "nothing else from this
// module".
func ForbiddingThirdParty() DepsOption {
	return func(cfg *depsConfig) { cfg.thirdPartyForbidden = true }
}

// WithBuildTags resolves the package under the named build tags, so a guard can
// judge a tagged configuration as well as the default one. go list resolves one
// configuration at a time: a dependency reachable only from a file behind a tag
// sits outside a reading taken without it, and so outside any rule stated over
// that reading.
func WithBuildTags(tags ...string) DepsOption {
	return func(cfg *depsConfig) { cfg.tags = tags }
}

// Lanes are the build configurations this module compiles its tests in: the
// default one, and the integration lane. A guard over a package's dependencies
// runs over both, so a dependency reachable only from an integration-tagged
// file is judged rather than resolved away.
func Lanes() []DepsOption {
	return []DepsOption{WithBuildTags(), WithBuildTags(IntegrationTag)}
}

type depsConfig struct {
	dir                 string
	tags                []string
	thirdPartyForbidden bool
}

// lane names the build configuration a reading was taken under, as a clause to
// be appended to the package it was taken of, and is empty for the default one:
// a finding is otherwise unreproducible, since the command a reader would run
// to confirm it resolves a different set from the one that produced it.
func (c depsConfig) lane() string {
	if len(c.tags) == 0 {
		return ""
	}
	return " under -tags " + strings.Join(c.tags, ",")
}

func newDepsConfig(opts []DepsOption) depsConfig {
	var cfg depsConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

// dep is one row of a transitive dependency listing. Module is the path of the
// module providing the package, and is empty for the standard library — which
// is how a dependency's origin is told apart without any hardcoded prefix.
type dep struct {
	Path   string
	Module string
}

const depFormat = "{{.ImportPath}}\t{{with .Module}}{{.Path}}{{end}}"

// listDeps is the enumeration seam: the shapes go list cannot be made to
// produce on demand — an empty set among them — are reachable only by
// swapping it.
var listDeps = func(cfg depsConfig, pkg string) ([]dep, error) {
	args := []string{"list", "-deps", "-f", depFormat}
	if len(cfg.tags) > 0 {
		args = append(args, "-tags", strings.Join(cfg.tags, ","))
	}
	cmd := exec.Command("go", append(args, pkg)...)
	cmd.Dir = cfg.dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, &listError{output: string(out), err: err}
	}
	return parseDeps(string(out)), nil
}

func parseDeps(out string) []dep {
	var deps []dep
	for line := range strings.SplitSeq(out, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		path, module, _ := strings.Cut(line, "\t")
		deps = append(deps, dep{Path: path, Module: module})
	}
	return deps
}

type listError struct {
	output string
	err    error
}

func (e *listError) Error() string {
	return e.err.Error() + "\n" + e.output
}

// PackageDeps returns pkg's transitive dependency list — the import paths
// `go list -deps` reports, pkg itself included, in that command's order. The
// argument is ordinarily an import path, so a guard resolves the same set
// regardless of the test binary's working directory; InDir moves that
// resolution to a chosen directory for a caller that needs one, and
// WithBuildTags takes it under a chosen build configuration. A package go
// list cannot resolve, and a set that comes back empty, are both fatal: a
// guard must fail rather than pass over nothing.
func PackageDeps(t harnesstest.TestingT, pkg string, opts ...DepsOption) []string {
	t.Helper()

	var paths []string
	for _, d := range packageDeps(t, pkg, opts) {
		paths = append(paths, d.Path)
	}
	return paths
}

func packageDeps(t harnesstest.TestingT, pkg string, opts []DepsOption) []dep {
	t.Helper()

	cfg := newDepsConfig(opts)
	deps, err := listDeps(cfg, pkg)
	if err != nil {
		t.Fatalf("go list -deps %s%s: %v", pkg, cfg.lane(), err)
		return nil
	}
	if len(deps) == 0 {
		t.Fatalf("go list -deps %s%s resolved no dependencies at all — a guard over this set would pass vacuously", pkg, cfg.lane())
		return nil
	}
	return deps
}
