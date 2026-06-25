package main

import (
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/portalbintest"
)

// capturePkg is the harness-only package that must stay out of the shipped
// portal binary. Production code (cmd/capturetool aside) must never depend on it.
const capturePkg = "github.com/leeovery/portal/internal/capture"

// portalMainPkg is the root package compiled into the shipped portal binary.
const portalMainPkg = "github.com/leeovery/portal"

// goListDeps returns the full transitive dependency import-path set of pkg via
// `go list -deps`, anchored at the project root so it resolves regardless of the
// test binary's runtime CWD.
func goListDeps(t *testing.T, pkg string) []string {
	t.Helper()
	root, err := portalbintest.ProjectRoot()
	if err != nil {
		t.Fatalf("resolve project root: %v", err)
	}
	cmd := exec.Command("go", "list", "-deps", pkg)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps %s: %v\n%s", pkg, err, out)
	}
	return strings.Fields(string(out))
}

// TestPortalBinaryDoesNotImportCapture asserts the shipped portal binary's
// transitive dependency set excludes internal/capture, so the harness fakes /
// fixtures never ship in production (spec § 15 / tick acceptance: "the portal
// binary does not import the capture fakes/fixtures package").
func TestPortalBinaryDoesNotImportCapture(t *testing.T) {
	for _, dep := range goListDeps(t, portalMainPkg) {
		if dep == capturePkg {
			t.Fatalf("portal binary (%s) transitively imports %s — harness code must stay out of production", portalMainPkg, capturePkg)
		}
	}
}

// TestCaptureToolDoesImportCapture is the positive control: the capture tool DOES
// depend on internal/capture, so the guard above is meaningful (not vacuously
// true because nothing imports the package at all).
func TestCaptureToolDoesImportCapture(t *testing.T) {
	const captureToolPkg = "github.com/leeovery/portal/cmd/capturetool"
	if slices.Contains(goListDeps(t, captureToolPkg), capturePkg) {
		return
	}
	t.Fatalf("capture tool (%s) does NOT import %s — the import guard is vacuous", captureToolPkg, capturePkg)
}
