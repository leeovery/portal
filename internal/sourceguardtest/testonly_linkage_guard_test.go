package sourceguardtest_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/sourceguardtest"
)

const (
	portalBinaryPkg = "github.com/leeovery/portal"
	captureToolPkg  = "github.com/leeovery/portal/cmd/capturetool"
	capturePkg      = "github.com/leeovery/portal/internal/capture"
)

func TestPortalBinaryLinksNoTestOnlyPackage(t *testing.T) {
	t.Run("the portal binary links no test-only package in either lane", func(t *testing.T) {
		assertLinksNone(t, portalBinaryPkg, sourceguardtest.TestOnlyPackages)
	})

	t.Run("it reports a binary that links a test-only package", func(t *testing.T) {
		var rec harnesstest.Recorder
		rec.Run(func() { assertLinksNone(&rec, captureToolPkg, []string{capturePkg}) })

		if len(rec.Fatals) != 0 || len(rec.Errors) == 0 || !strings.Contains(rec.Errors[0], capturePkg) {
			t.Errorf("assertLinksNone(%s, [%s]) reported %s; want an error naming %s", captureToolPkg, capturePkg, rec.Report(), capturePkg)
		}
	})

	t.Run("it fails on an entry that resolves to no package", func(t *testing.T) {
		const missing = "github.com/leeovery/portal/internal/nosuchtest"
		var rec harnesstest.Recorder
		rec.Run(func() { assertLinksNone(&rec, portalBinaryPkg, []string{missing}) })

		if len(rec.Fatals) != 1 || !strings.Contains(rec.Fatals[0], missing) {
			t.Errorf("assertLinksNone over %s reported %s; want one fatal naming it", missing, rec.Report())
		}
	})

	t.Run("it fails when it is given no package to judge", func(t *testing.T) {
		var rec harnesstest.Recorder
		rec.Run(func() { assertLinksNone(&rec, portalBinaryPkg, nil) })

		if len(rec.Fatals) != 1 {
			t.Errorf("assertLinksNone over no package reported %s; want one fatal", rec.Report())
		}
	})
}

// assertLinksNone reports each of packages that binary links, in each lane:
// portalbintest builds the binary with the integration tag, so a production
// file under it would ship in what the tests run. Each package is resolved
// first, so a renamed or mistyped entry fails rather than passing for ever.
func assertLinksNone(t harnesstest.TestingT, binary string, packages []string) {
	t.Helper()
	if len(packages) == 0 {
		t.Fatalf("no test-only package to judge %s against, so the guard would pass by having stopped looking", binary)
		return
	}
	root := sourceguardtest.InDir(sourceguardtest.ProjectRoot(t))
	for _, lane := range sourceguardtest.Lanes() {
		linked := sourceguardtest.PackageDeps(t, binary, root, lane)
		for _, pkg := range packages {
			if !slices.Contains(sourceguardtest.PackageDeps(t, pkg, root, lane), pkg) {
				t.Fatalf("%s does not resolve to itself, so the guard is vacuous", pkg)
				return
			}
			if slices.Contains(linked, pkg) {
				t.Errorf("%s links the test-only package %s in a lane (default or -tags integration); test-only code must stay out of it", binary, pkg)
			}
		}
	}
}
