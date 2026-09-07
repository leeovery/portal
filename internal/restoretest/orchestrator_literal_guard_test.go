package restoretest_test

import (
	"go/ast"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/sourceguardtest"
)

// The type no test may compose itself, and the constructors that compose it
// with Exe pinned.
const (
	orchestratorType = "Orchestrator"

	stagedConstructor = "restoretest.NewRestoreOrchestrator"
	fakeConstructor   = "restoretest.NewFakeExeOrchestrator"
)

// TestNoTestComposesABareRestoreOrchestrator is the standing guard over the one
// field whose omission is silent. restore.Orchestrator.Exe is optional and falls
// back to os.Executable(); under `go test` that resolves to the test binary, so a
// pane armed by an orchestrator with no Exe respawns into the suite itself, which
// stops flag parsing at the leading `state` positional, re-runs its own tests
// inside the tmux pane and exits 0. The symptom is a session that quietly
// disappeared — no error, no failure, nothing in the log.
//
// The rule is therefore not "set Exe" but "never compose the struct": a rule
// about a field that may legitimately be absent cannot be checked, while a rule
// about the literal can. Both constructors pin Exe, so routing through either
// makes the omission unrepresentable.
func TestNoTestComposesABareRestoreOrchestrator(t *testing.T) {
	scanned, findings := scanTestOrchestratorLiterals(t)
	if len(findings) == 0 {
		return
	}
	t.Fatalf("%d bare %s.%s literal(s) in test files (of %d scanned):\n  %s\n"+
		"  Exe is opt-in on the struct and a forgotten one is silent: the pane respawns into\n"+
		"  the test binary and the session vanishes with no error. Build it through %s\n"+
		"  (a staged binary) or %s (no live pane to arm).",
		len(findings), restorePkg, orchestratorType, scanned,
		strings.Join(findings, "\n  "), stagedConstructor, fakeConstructor)
}

func TestOrchestratorLiteralGuard_FlagsATestComposingTheStructItself(t *testing.T) {
	t.Run("it fails a test file composing a bare restore.Orchestrator literal", func(t *testing.T) {
		root := writeGuardFixture(t, "fixture_test.go", `package fixture

import "github.com/leeovery/portal/internal/restore"

func drive() {
	o := &restore.Orchestrator{StateDir: "/tmp"}
	_ = o
	_ = restore.Orchestrator{}
}
`)

		scanned, findings := scanTestOrchestratorLiterals(t, sourceguardtest.Rooted(root))
		if scanned != 1 {
			t.Fatalf("scanned %d files, want 1", scanned)
		}
		if len(findings) != 2 {
			t.Fatalf("scan found %d literals, want 2: %v", len(findings), findings)
		}
	})

	t.Run("it passes a test file routing through the constructors", func(t *testing.T) {
		root := writeGuardFixture(t, "fixture_test.go", `package fixture

import "github.com/leeovery/portal/internal/restoretest"

func drive(t *testingT, client *client) {
	o := restoretest.NewRestoreOrchestrator(t, client, "/state", "/bin")
	p := restoretest.NewFakeExeOrchestrator(t, client, "/state", nil)
	_, _ = o, p
}
`)

		_, findings := scanTestOrchestratorLiterals(t, sourceguardtest.Rooted(root))
		if len(findings) != 0 {
			t.Errorf("scan flagged %v, want nothing", findings)
		}
	})

	// The rule is scoped to tests: restoretest's own constructors compose the
	// struct, and production wiring composes it too.
	t.Run("it ignores a production file composing the struct", func(t *testing.T) {
		root := writeGuardFixture(t, "production.go", `package fixture

import "github.com/leeovery/portal/internal/restore"

var o = &restore.Orchestrator{}
`)
		writeGuardFile(t, root, "keeps_scanning_test.go", "package fixture\n")

		scanned, findings := scanTestOrchestratorLiterals(t, sourceguardtest.Rooted(root))
		if scanned != 1 || len(findings) != 0 {
			t.Errorf("scan of a production literal = %d scanned / %v, want 1 scanned / nothing", scanned, findings)
		}
	})
}

// A guard that stopped finding test sources would otherwise report a clean tree
// forever.
func TestOrchestratorLiteralGuard_FatalsWhenItEnumeratesNoTestFiles(t *testing.T) {
	t.Run("it fatals when the orchestrator guard scans no files", func(t *testing.T) {
		emptyTree := &harnesstest.Recorder{}
		emptyTree.Run(func() { scanTestOrchestratorLiterals(emptyTree, sourceguardtest.Rooted(t.TempDir())) })
		if len(emptyTree.Fatals) != 1 {
			t.Fatalf("scan of a directory holding no sources reported %d fatals, want 1: %v", len(emptyTree.Fatals), emptyTree.Fatals)
		}

		root := writeGuardFixture(t, "production.go", "package fixture\n")
		noTests := &harnesstest.Recorder{}
		noTests.Run(func() { scanTestOrchestratorLiterals(noTests, sourceguardtest.Rooted(root)) })
		if len(noTests.Fatals) != 1 {
			t.Fatalf("scan of a tree holding no _test.go reported %d fatals, want 1: %v", len(noTests.Fatals), noTests.Fatals)
		}
		if !strings.Contains(noTests.Fatals[0], "stopped looking") {
			t.Errorf("fatal message %q does not say the guard would pass having stopped looking", noTests.Fatals[0])
		}
	})
}

// scanTestOrchestratorLiterals reports every composite literal of the
// orchestrator type in a _test.go the scan reaches, as "<file>:<line>:<column>".
// It reads the AST rather than the text, so a mention of the type inside a
// string — this guard's own fixtures — is not a finding. Every lane is policed:
// the integration-tagged files are most of the subject, and an unpinned literal
// is as silent in one lane as the other.
func scanTestOrchestratorLiterals(t harnesstest.TestingT, opts ...sourceguardtest.ScanOption) (scanned int, findings []string) {
	t.Helper()

	scanned, findings = scanGuardTestFiles(t, everyTestFile, orchestratorLiteralsIn, opts...)
	if scanned == 0 {
		t.Fatalf("no _test.go was enumerated, so the guard would pass by having stopped looking")
		return 0, nil
	}
	return scanned, findings
}

func everyTestFile(*ast.File) bool { return true }

func orchestratorLiteralsIn(source sourceguardtest.ParsedSource) []string {
	var findings []string
	ast.Inspect(source.File, func(n ast.Node) bool {
		lit, isLit := n.(*ast.CompositeLit)
		if !isLit || !isRestorePkgType(lit.Type, orchestratorType) {
			return true
		}
		findings = append(findings, source.Position(lit.Pos()).String())
		return true
	})
	return findings
}
