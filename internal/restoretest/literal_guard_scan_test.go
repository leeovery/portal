package restoretest_test

import (
	"go/ast"
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/sourceguardtest"
)

// restorePkg is the package qualifier a guarded type is written under at a call
// site.
const restorePkg = "restore"

// The types no test may compose by hand.
const (
	orchestratorType    = "Orchestrator"
	sessionRestorerType = "SessionRestorer"
)

// literalGuard describes one guarded restore type: the composite literals it
// refuses, the files it refuses them in, the constructors that compose the type
// with Exe pinned, and what it says when its own file set runs dry.
type literalGuard struct {
	typeName     string
	fileSet      string
	include      func(*ast.File) bool
	constructors string
	fatalWording string
}

var (
	orchestratorGuard = literalGuard{
		typeName: orchestratorType,
		fileSet:  "test files",
		include:  everyTestFile,
		constructors: "restoretest.NewRestoreOrchestrator (a staged binary) or " +
			"restoretest.NewFakeExeOrchestrator (no live pane to arm)",
		fatalWording: "no _test.go was enumerated, so the guard would pass by having stopped looking",
	}

	// The session-restorer half is scoped to integration-tagged files because
	// only they drive a restorer against a live server — the unit lane composes
	// the struct freely behind a mock commander, where an unset Exe arms
	// nothing.
	sessionRestorerGuard = literalGuard{
		typeName:     sessionRestorerType,
		fileSet:      "integration-tagged test files",
		include:      isIntegrationTagged,
		constructors: "restoretest.NewSessionRestorer, which pins the staged binary",
		fatalWording: "no integration-tagged _test.go was enumerated, so the guard would pass by having stopped looking",
	}

	restoreLiteralGuards = []literalGuard{orchestratorGuard, sessionRestorerGuard}
)

func everyTestFile(*ast.File) bool { return true }

// isIntegrationTagged evaluates the file's build constraint with `integration`
// as the only satisfied tag, so a file gated on some other tag is not mistaken
// for one the integration lane compiles.
func isIntegrationTagged(file *ast.File) bool {
	expr, stated := sourceguardtest.BuildConstraint(file)
	return stated && sourceguardtest.SatisfiedWith(expr, sourceguardtest.IntegrationTag)
}

// guardScan is one descriptor's verdict: how many of its own files the walk
// offered it, and every literal it refuses in them.
type guardScan struct {
	scanned  int
	findings []string
}

// scanGuardTestFiles takes one walk of every _test.go the scan reaches and
// answers each descriptor from it, in the order they were given. A descriptor
// whose own file set enumerated nothing is fatal in its own words, since a
// guard that has stopped finding sources reports a clean tree forever.
//
// Build tags are not honoured by the walk itself: a descriptor decides through
// its include which lane it polices, so the unit lane can police both.
func scanGuardTestFiles(
	t harnesstest.TestingT,
	guards []literalGuard,
	opts ...sourceguardtest.ScanOption,
) []guardScan {
	t.Helper()

	scans := make([]guardScan, len(guards))
	_, sources := sourceguardtest.RepoSources(t, sourceguardtest.TestSources, opts...)
	for _, source := range sources {
		for i, guard := range guards {
			if !guard.include(source.File) {
				continue
			}
			scans[i].scanned++
			scans[i].findings = append(scans[i].findings, guard.literalsIn(source)...)
		}
	}
	for i, guard := range guards {
		if scans[i].scanned == 0 {
			t.Fatalf("%s", guard.fatalWording)
			return nil
		}
	}
	return scans
}

// literalsIn reports every composite literal of the guarded type in one source,
// as "<file>:<line>:<column>". It reads the AST rather than the text, so a
// mention of the type inside a string — this guard's own fixtures — is not a
// finding.
func (g literalGuard) literalsIn(source sourceguardtest.ParsedSource) []string {
	var findings []string
	ast.Inspect(source.File, func(n ast.Node) bool {
		lit, isLit := n.(*ast.CompositeLit)
		if !isLit || !isRestorePkgType(lit.Type, g.typeName) {
			return true
		}
		findings = append(findings, source.Position(lit.Pos()).String())
		return true
	})
	return findings
}

// isRestorePkgType reports whether expr names the given type of the restore
// package, as written at a call site: restore.<typeName>.
func isRestorePkgType(expr ast.Expr, typeName string) bool {
	sel, isSel := expr.(*ast.SelectorExpr)
	if !isSel || sel.Sel.Name != typeName {
		return false
	}
	pkg, isIdent := sel.X.(*ast.Ident)
	return isIdent && pkg.Name == restorePkg
}

type guardFixtureFile struct {
	name string
	src  string
}

func writeGuardFixture(t *testing.T, files ...guardFixtureFile) string {
	t.Helper()
	root := t.TempDir()
	for _, file := range files {
		if err := os.WriteFile(filepath.Join(root, file.name), []byte(file.src), 0o600); err != nil {
			t.Fatalf("write fixture %s: %v", file.name, err)
		}
	}
	return root
}
