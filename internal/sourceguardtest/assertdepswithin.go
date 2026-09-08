package sourceguardtest

import (
	"slices"

	"github.com/leeovery/portal/internal/harnesstest"
)

// AssertDepsWithin asserts that pkg's transitive dependencies stay inside
// allowed — the shape every leaf-package guard in this module states. allowed
// holds full import paths. The standard library and pkg itself are always
// permitted, and so are packages from other modules, whose layering says
// nothing about this one's; ForbiddingThirdParty brings those into the
// judgement, which turns an empty allowlist into "the standard library alone".
//
// Every dependency outside the allowlist is reported, so one run names them
// all. Three conditions are fatal instead, because under each the assertion
// would hold over nothing: a dependency set that comes back empty; a set not
// holding pkg itself, which is a set about some other package; and — for a
// non-empty allowlist — a set holding none of the allowed paths, which means
// the allowlist has drifted off the package it is meant to police. A package
// whose allowlist is empty has no such anchor to check and rests on the first
// two.
func AssertDepsWithin(t harnesstest.TestingT, pkg string, allowed []string, opts ...DepsOption) {
	t.Helper()

	cfg := newDepsConfig(opts)
	deps := packageDeps(t, pkg, opts)

	ownModule, found := moduleOf(deps, pkg)
	if !found {
		t.Fatalf("go list -deps %s%s resolved a set that does not hold %s itself — this guard is judging some other package", pkg, cfg.lane(), pkg)
		return
	}

	var sawAllowed bool
	for _, d := range deps {
		switch {
		case d.Path == pkg || isStdlib(d):
		case d.Module != ownModule && !cfg.thirdPartyForbidden:
		case slices.Contains(allowed, d.Path):
			sawAllowed = true
		default:
			t.Errorf("%s%s transitively depends on %s — it may reach no further than %v", pkg, cfg.lane(), d.Path, allowed)
		}
	}

	if len(allowed) > 0 && !sawAllowed {
		t.Fatalf("%s%s depends on none of %v — the allowlist no longer describes the package, so this guard proves nothing", pkg, cfg.lane(), allowed)
	}
}

// moduleOf reports the module providing pkg, taken from pkg's own row in its
// dependency listing.
func moduleOf(deps []dep, pkg string) (string, bool) {
	for _, d := range deps {
		if d.Path == pkg {
			return d.Module, true
		}
	}
	return "", false
}

// isStdlib reports whether a dependency comes from the standard library, which
// go list reports as belonging to no module.
func isStdlib(d dep) bool {
	return d.Module == ""
}
