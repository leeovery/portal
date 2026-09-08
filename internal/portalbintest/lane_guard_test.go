package portalbintest_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/sourceguardtest"
)

// buildHelperNames is the vocabulary the rule is expressed in: every helper that
// compiles the portal binary, matched on the callee's own name so a qualified
// call and a call from inside the declaring package read alike.
var buildHelperNames = []string{
	"BuildPortalBinary",
	"StagePortalBinary",
	"BuildPortalBinaryDir",
	"BuildPortalBinaryStable",
}

// helperRef is one reference to a build helper: which one, and where.
type helperRef struct {
	File   string
	Func   string
	Helper string
	Line   int
}

// scannedTestFile is one _test.go file, carrying whether the unit lane compiles
// it and the build-helper references it makes.
type scannedTestFile struct {
	Rel      string
	UnitLane bool
	Refs     []helperRef
}

// TestBuildHelpersStayInTheIntegrationLane polices the rule that a test which
// builds the portal binary carries //go:build integration, whichever helper it
// builds through. The lane is meant to be pure: `go test ./...` compiles no
// portal binary, so every test that does one belongs behind the tag alongside
// those that run one. The rule is awkward to verify by hand — a PATH shim on the
// go command does not intercept the build, because the toolchain directory is
// prepended for test binaries — which is the other half of why it is a guard.
func TestBuildHelpersStayInTheIntegrationLane(t *testing.T) {
	_, sources := sourceguardtest.RepoSources(t, sourceguardtest.TestSources)

	var files []scannedTestFile
	for _, source := range sources {
		files = append(files, scannedTestFile{
			Rel:      source.Path,
			UnitLane: compiledInUnitLane(source.File),
			Refs:     buildHelperRefsIn(source),
		})
	}

	if msg := laneGuardFailure(auditBuildHelperLane(files)); msg != "" {
		t.Fatal(msg)
	}
}

// buildHelperRefsIn records every build-helper call one parsed file makes.
func buildHelperRefsIn(source sourceguardtest.ParsedSource) []helperRef {
	var refs []helperRef
	sourceguardtest.ForEachFuncCall(source.File, func(funcName string, call *ast.CallExpr) bool {
		callee := sourceguardtest.CalleeName(call)
		if slices.Contains(buildHelperNames, callee) {
			refs = append(refs, helperRef{
				File:   source.Path,
				Func:   funcName,
				Helper: callee,
				Line:   source.Fset.Position(call.Pos()).Line,
			})
		}
		return true
	})
	return refs
}

// compiledInUnitLane reports whether the unit lane compiles the file — that is,
// whether its build constraint holds with the integration tag unset. A file
// stating no constraint the guard can read is in the lane, so where the tag is
// unreadable the guard judges rather than excuses.
func compiledInUnitLane(file *ast.File) bool {
	expr, stated := sourceguardtest.BuildConstraint(file)
	return !stated || sourceguardtest.SatisfiedWithout(expr, sourceguardtest.IntegrationTag)
}

// auditBuildHelperLane returns how many unit-lane test files it judged, how many
// build-helper references it saw in either lane, and the sorted descriptions of
// those made from the unit lane.
func auditBuildHelperLane(files []scannedTestFile) (scanned, referenced int, defects []string) {
	for _, file := range files {
		referenced += len(file.Refs)
		if !file.UnitLane {
			continue
		}
		scanned++
		for _, ref := range file.Refs {
			defects = append(defects, fmt.Sprintf("%s:%d %s calls %s", ref.File, ref.Line, ref.Func, ref.Helper))
		}
	}
	sort.Strings(defects)
	return scanned, referenced, defects
}

// laneGuardFailure renders the guard's failure, or "" when the tree passes.
func laneGuardFailure(scanned, referenced int, defects []string) string {
	switch {
	case scanned == 0:
		return "no untagged _test.go file was scanned; the guard has stopped looking rather than passed"
	case referenced == 0:
		return fmt.Sprintf("no test in either lane calls any of %v; the helper vocabulary has drifted off the tree, so this guard proves nothing", buildHelperNames)
	case len(defects) == 0:
		return ""
	}
	return fmt.Sprintf("%d unit-lane test file(s) build the portal binary:\n  %s\n"+
		"  a test that builds the binary carries //go:build %s, which keeps `go test ./...` free of portal builds",
		len(defects), strings.Join(defects, "\n  "), sourceguardtest.IntegrationTag)
}

// stageTestFile parses one miniature fixture file into the record the rule is
// audited over, under the path it is judged by.
func stageTestFile(t *testing.T, rel, src string) scannedTestFile {
	t.Helper()
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, filepath.Base(rel), src, sourceguardtest.ParseMode)
	if err != nil {
		t.Fatalf("parse fixture source: %v", err)
	}
	source := sourceguardtest.ParsedSource{Path: rel, Fset: fset, File: parsed}
	return scannedTestFile{
		Rel:      rel,
		UnitLane: compiledInUnitLane(parsed),
		Refs:     buildHelperRefsIn(source),
	}
}
