package sourceguardtest

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/harnesstest"
)

const (
	hooksPkg     = "github.com/leeovery/portal/internal/hooks"
	nanoidPkg    = "github.com/leeovery/portal/internal/nanoid"
	themePkg     = "github.com/leeovery/portal/internal/theme"
	fileutilPkg  = "github.com/leeovery/portal/internal/fileutil"
	logPkg       = "github.com/leeovery/portal/internal/log"
	nanoidPkgDir = "../nanoid"

	fixtureModule       = "portalguardfixture"
	fixtureLanePkg      = fixtureModule + "/lane"
	fixtureForbiddenPkg = fixtureModule + "/forbidden"

	fixtureDefaultLanePkg = fixtureModule + "/defaultlane"
)

func TestAssertDepsWithin_ReportsEveryDepOutsideTheAllowlist(t *testing.T) {
	rec := &harnesstest.Recorder{}

	rec.Run(func() { AssertDepsWithin(rec, hooksPkg, []string{fileutilPkg}) })

	for _, want := range []string{logPkg, nanoidPkg, "internal/storelog"} {
		if !errored(rec, want) {
			t.Errorf("AssertDepsWithin did not report %s as outside the allowlist; reported %v", want, rec.Errors)
		}
	}
	if len(rec.Fatals) != 0 {
		t.Errorf("AssertDepsWithin fatalled on an allowlist whose entry is present: %v", rec.Fatals)
	}
}

func TestAssertDepsWithin_FatalsOnAnEmptyDepSet(t *testing.T) {
	defer stubListDeps(t, nil, nil)()
	rec := &harnesstest.Recorder{}

	rec.Run(func() { AssertDepsWithin(rec, hooksPkg, []string{fileutilPkg}) })

	if len(rec.Fatals) != 1 {
		t.Fatalf("AssertDepsWithin reported %d fatals on an empty dependency set, want 1 — the guard would pass over nothing: %v", len(rec.Fatals), rec.Fatals)
	}
}

func TestAssertDepsWithin_FatalsWhenTheSetDoesNotHoldThePackageItself(t *testing.T) {
	defer stubListDeps(t, []dep{{Path: "strings"}}, nil)()
	rec := &harnesstest.Recorder{}

	rec.Run(func() { AssertDepsWithin(rec, hooksPkg, nil) })

	if len(rec.Fatals) != 1 {
		t.Fatalf("AssertDepsWithin reported %d fatals over a set that does not hold the package under test, want 1 — it was asserting about something else: %v", len(rec.Fatals), rec.Fatals)
	}
}

func TestAssertDepsWithin_FatalsWhenNoAllowlistedInternalDepIsPresent(t *testing.T) {
	rec := &harnesstest.Recorder{}

	rec.Run(func() { AssertDepsWithin(rec, nanoidPkg, []string{fileutilPkg}) })

	if len(rec.Fatals) != 1 {
		t.Fatalf("AssertDepsWithin reported %d fatals when its allowlist named no dependency the package actually has, want 1; errors %v", len(rec.Fatals), rec.Errors)
	}
	if !strings.Contains(rec.Fatals[0], fileutilPkg) {
		t.Errorf("fatal message %q does not name the allowlist it could not see", rec.Fatals[0])
	}
}

func TestAssertDepsWithin_LeavesAnotherModuleAloneByDefault(t *testing.T) {
	rec := &harnesstest.Recorder{}

	rec.Run(func() { AssertDepsWithin(rec, themePkg, []string{logPkg}) })

	if rec.Failed() {
		t.Errorf("AssertDepsWithin faulted a dependency outside the package's own module: %s", rec.Report())
	}
}

func TestAssertDepsWithin_ForbiddingThirdPartyReportsAnotherModule(t *testing.T) {
	rec := &harnesstest.Recorder{}

	rec.Run(func() { AssertDepsWithin(rec, themePkg, []string{logPkg}, ForbiddingThirdParty()) })

	if !errored(rec, "lipgloss") {
		t.Errorf("ForbiddingThirdParty did not report the third-party dependency; reported %v", rec.Errors)
	}
}

func TestAssertDepsWithin_PassesForAStdlibOnlyPackageWithAnEmptyAllowlist(t *testing.T) {
	rec := &harnesstest.Recorder{}

	rec.Run(func() { AssertDepsWithin(rec, nanoidPkg, nil, ForbiddingThirdParty()) })

	if rec.Failed() {
		t.Errorf("AssertDepsWithin faulted a stdlib-only package: %s", rec.Report())
	}
}

func TestPackageDeps_ResolvesAPackageRelativeToTheGivenWorkingDirectory(t *testing.T) {
	deps := PackageDeps(t, ".", InDir(nanoidPkgDir))

	if !slices.Contains(deps, nanoidPkg) {
		t.Errorf("PackageDeps(\".\", InDir(%q)) resolved %v, want the set to hold %s", nanoidPkgDir, deps, nanoidPkg)
	}
	if own := PackageDeps(t, "."); slices.Contains(own, nanoidPkg) {
		t.Errorf("PackageDeps(\".\") without a directory resolved %s — the control case is not distinguishing", nanoidPkg)
	}
}

// stubListDeps swaps the enumeration seam for the duration of a case, so the
// shapes go list cannot be made to produce on demand — an empty set among
// them — are still exercised. The returned func restores the real one.
func stubListDeps(t *testing.T, deps []dep, err error) func() {
	t.Helper()
	prior := listDeps
	listDeps = func(depsConfig, string) ([]dep, error) { return deps, err }
	return func() { listDeps = prior }
}

// errored reports whether any of the recorded complaints carries substring.
func errored(rec *harnesstest.Recorder, substring string) bool {
	return slices.ContainsFunc(rec.Errors, func(msg string) bool { return strings.Contains(msg, substring) })
}

func TestAssertDepsWithin_WithBuildTags(t *testing.T) {
	t.Run("it leaves a dependency behind the integration tag out of the default reading", func(t *testing.T) {
		dir := stageTaggedFixtureModule(t)
		rec := &harnesstest.Recorder{}

		rec.Run(func() { AssertDepsWithin(rec, fixtureLanePkg, nil, InDir(dir)) })

		if rec.Failed() {
			t.Fatalf("the default reading faulted the fixture, so the tagged reading proves nothing: %s", rec.Report())
		}
	})

	t.Run("it reports a forbidden dependency reachable only from an integration-tagged file", func(t *testing.T) {
		dir := stageTaggedFixtureModule(t)
		rec := &harnesstest.Recorder{}

		rec.Run(func() {
			AssertDepsWithin(rec, fixtureLanePkg, nil, InDir(dir), WithBuildTags(IntegrationTag))
		})

		if !errored(rec, fixtureForbiddenPkg) {
			t.Errorf("AssertDepsWithin did not report %s, which the fixture reaches only from its integration-tagged file; reported %v", fixtureForbiddenPkg, rec.Errors)
		}
		if !errored(rec, fixtureLanePkg+" under -tags "+IntegrationTag) {
			t.Errorf("the finding does not name the configuration it was resolved under, so a reader running the command it prints sees none of it; reported %v", rec.Errors)
		}
	})

	t.Run("it reports that dependency when the guard runs over every lane", func(t *testing.T) {
		dir := stageTaggedFixtureModule(t)
		rec := &harnesstest.Recorder{}

		rec.Run(func() {
			for _, lane := range Lanes() {
				AssertDepsWithin(rec, fixtureLanePkg, nil, InDir(dir), lane)
			}
		})

		if !errored(rec, fixtureForbiddenPkg) {
			t.Errorf("the lanes a guard runs over do not reach the integration one; reported %v", rec.Errors)
		}
	})

	t.Run("it reports a dependency reachable only from a file gated against the tag when the guard runs over every lane", func(t *testing.T) {
		dir := stageTaggedFixtureModule(t)
		tagged, everyLane := &harnesstest.Recorder{}, &harnesstest.Recorder{}

		tagged.Run(func() {
			AssertDepsWithin(tagged, fixtureDefaultLanePkg, nil, InDir(dir), WithBuildTags(IntegrationTag))
		})
		everyLane.Run(func() {
			for _, lane := range Lanes() {
				AssertDepsWithin(everyLane, fixtureDefaultLanePkg, nil, InDir(dir), lane)
			}
		})

		if tagged.Failed() {
			t.Fatalf("the integration reading faulted the fixture, so it does not distinguish the default reading: %s", tagged.Report())
		}
		if !errored(everyLane, fixtureForbiddenPkg) {
			t.Errorf("the lanes a guard runs over do not reach the default one, where a dependency gated against the tag is the only reading that holds it; reported %v", everyLane.Errors)
		}
	})
}

// stageTaggedFixtureModule writes a module holding both shapes a single-lane
// dependency reading is blind to — one package reaching a third only from an
// integration-tagged source, one reaching it only from a source gated against
// that tag — and returns its directory.
func stageTaggedFixtureModule(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("stage fixture directory for %s: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("stage fixture source %s: %v", rel, err)
		}
	}

	write("go.mod", "module "+fixtureModule+"\n\ngo 1.24\n")
	write("lane/lane.go", "package lane\n")
	write("defaultlane/defaultlane.go", "package defaultlane\n")
	write("defaultlane/defaultlane_unit.go", "//go:build !"+IntegrationTag+"\n\npackage defaultlane\n\nimport _ \""+fixtureForbiddenPkg+"\"\n")
	write("lane/lane_integration.go", "//go:build "+IntegrationTag+"\n\npackage lane\n\nimport _ \""+fixtureForbiddenPkg+"\"\n")
	write("forbidden/forbidden.go", "package forbidden\n")
	return dir
}
