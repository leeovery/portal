package restoretest_test

import (
	"go/ast"
	"go/build/constraint"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/sourceguardtest"
)

const (
	sessionRestorerType = "SessionRestorer"

	sessionRestorerConstructor = "restoretest.NewSessionRestorer"
	integrationTag             = "integration"
)

// TestNoIntegrationFixtureComposesASessionRestorer is the session-layer half of
// the pane-arming guard. restore.SessionRestorer.Exe is optional and falls back
// to os.Executable(); under `go test` that resolves to the test binary, so a
// pane armed by a restorer with no Exe respawns into the suite itself, which
// stops flag parsing at the leading `state` positional, re-runs its own tests
// inside the tmux pane and exits 0. The symptom is a session that quietly
// disappeared — no error, no failure, nothing in the log.
//
// As with the orchestrator, the rule is "never compose the struct" rather than
// "set Exe": a set field proves nothing, since a nil resolver takes the same
// fallback an absent one does. It is scoped to integration-tagged files because
// only they drive a restorer against a live server — the unit lane composes the
// struct freely behind a mock commander, where an unset Exe arms nothing.
func TestNoIntegrationFixtureComposesASessionRestorer(t *testing.T) {
	scanned, findings := scanIntegrationSessionRestorerLiterals(t)
	if len(findings) == 0 {
		return
	}
	t.Fatalf("%d %s.%s literal(s) in integration-tagged test files (of %d scanned):\n  %s\n"+
		"  A forgotten Exe is silent: the pane respawns into the test binary and the session\n"+
		"  vanishes with no error. Build it through %s, which pins the staged binary.",
		len(findings), restorePkg, sessionRestorerType, scanned,
		strings.Join(findings, "\n  "), sessionRestorerConstructor)
}

func TestSessionRestorerLiteralGuard_FlagsAnIntegrationFixtureComposingTheStructItself(t *testing.T) {
	t.Run("it flags an integration fixture composing a SessionRestorer literal", func(t *testing.T) {
		root := writeGuardFixture(t, "fixture_integration_test.go", `//go:build integration

package fixture

import "github.com/leeovery/portal/internal/restore"

func drive() {
	r := &restore.SessionRestorer{StateDir: "/state"}
	_ = r
	_ = restore.SessionRestorer{}
}
`)

		scanned, findings := scanIntegrationSessionRestorerLiterals(t, sourceguardtest.Rooted(root))
		if scanned != 1 {
			t.Fatalf("scanned %d files, want 1", scanned)
		}
		if len(findings) != 2 {
			t.Fatalf("scan found %d literals, want 2: %v", len(findings), findings)
		}
	})

	// A set Exe is no defence: a nil resolver falls back to os.Executable
	// exactly as an absent field does, so the rule is about the literal.
	t.Run("it flags an integration fixture whose Exe is an explicit nil", func(t *testing.T) {
		root := writeGuardFixture(t, "fixture_integration_test.go", `//go:build integration

package fixture

import "github.com/leeovery/portal/internal/restore"

func drive() {
	_ = &restore.SessionRestorer{StateDir: "/state", Exe: nil}
}
`)

		_, findings := scanIntegrationSessionRestorerLiterals(t, sourceguardtest.Rooted(root))
		if len(findings) != 1 {
			t.Errorf("scan found %d literals, want 1: %v", len(findings), findings)
		}
	})

	t.Run("it ignores an integration fixture routing through the constructor", func(t *testing.T) {
		root := writeGuardFixture(t, "fixture_integration_test.go", `//go:build integration

package fixture

import "github.com/leeovery/portal/internal/restoretest"

func drive(t *testingT) {
	r := restoretest.NewSessionRestorer(t, nil, "/state", "/bin")
	_ = r
}
`)

		_, findings := scanIntegrationSessionRestorerLiterals(t, sourceguardtest.Rooted(root))
		if len(findings) != 0 {
			t.Errorf("scan flagged %v, want nothing", findings)
		}
	})

	// The mock-driven literals in the unit lane arm no live pane, so an unset
	// Exe there is harmless and they are deliberately left alone.
	t.Run("it ignores a unit-lane composite literal", func(t *testing.T) {
		root := writeGuardFixture(t, "unit_test.go", `package fixture

import "github.com/leeovery/portal/internal/restore"

func drive() {
	_ = &restore.SessionRestorer{StateDir: "/state"}
}
`)
		writeGuardFile(t, root, "keeps_scanning_integration_test.go", "//go:build integration\n\npackage fixture\n")

		scanned, findings := scanIntegrationSessionRestorerLiterals(t, sourceguardtest.Rooted(root))
		if scanned != 1 || len(findings) != 0 {
			t.Errorf("scan of a unit-lane literal = %d scanned / %v, want 1 scanned / nothing", scanned, findings)
		}
	})
}

// A guard that stopped finding integration sources would otherwise report a
// clean tree forever.
func TestSessionRestorerLiteralGuard_FatalsWhenItEnumeratesNoIntegrationTestFiles(t *testing.T) {
	t.Run("it fatals when the session-restorer guard scans no files", func(t *testing.T) {
		emptyTree := &harnesstest.Recorder{}
		emptyTree.Run(func() { scanIntegrationSessionRestorerLiterals(emptyTree, sourceguardtest.Rooted(t.TempDir())) })
		if len(emptyTree.Fatals) != 1 {
			t.Fatalf("scan of a directory holding no sources reported %d fatals, want 1: %v", len(emptyTree.Fatals), emptyTree.Fatals)
		}

		root := writeGuardFixture(t, "unit_test.go", "package fixture\n")
		noIntegrationTests := &harnesstest.Recorder{}
		noIntegrationTests.Run(func() { scanIntegrationSessionRestorerLiterals(noIntegrationTests, sourceguardtest.Rooted(root)) })
		if len(noIntegrationTests.Fatals) != 1 {
			t.Fatalf("scan of a tree holding no integration-tagged _test.go reported %d fatals, want 1: %v", len(noIntegrationTests.Fatals), noIntegrationTests.Fatals)
		}
		if !strings.Contains(noIntegrationTests.Fatals[0], "stopped looking") {
			t.Errorf("fatal message %q does not say the guard would pass having stopped looking", noIntegrationTests.Fatals[0])
		}
	})
}

// scanIntegrationSessionRestorerLiterals reports every composite literal of the
// session-restorer type in an integration-tagged _test.go the scan reaches, as
// "<file>:<line>:<column>". It reads the AST rather than the text, so a mention
// of the type inside a string — this guard's own fixtures — is not a finding.
func scanIntegrationSessionRestorerLiterals(t harnesstest.TestingT, opts ...sourceguardtest.ScanOption) (scanned int, findings []string) {
	t.Helper()

	scanned, findings = scanGuardTestFiles(t, isIntegrationTagged, sessionRestorerLiteralsIn, opts...)
	if scanned == 0 {
		t.Fatalf("no integration-tagged _test.go was enumerated, so the guard would pass by having stopped looking")
		return 0, nil
	}
	return scanned, findings
}

// isIntegrationTagged evaluates the file's //go:build line with `integration`
// as the only satisfied tag, so a file gated on some other tag is not mistaken
// for one the integration lane compiles.
func isIntegrationTagged(file *ast.File) bool {
	for _, group := range file.Comments {
		if group.Pos() > file.Package {
			break
		}
		for _, c := range group.List {
			expr, err := constraint.Parse(c.Text)
			if err != nil {
				continue
			}
			return expr.Eval(func(tag string) bool { return tag == integrationTag })
		}
	}
	return false
}

func sessionRestorerLiteralsIn(source sourceguardtest.ParsedSource) []string {
	var findings []string
	ast.Inspect(source.File, func(n ast.Node) bool {
		lit, isLit := n.(*ast.CompositeLit)
		if !isLit || !isRestorePkgType(lit.Type, sessionRestorerType) {
			return true
		}
		findings = append(findings, source.Position(lit.Pos()).String())
		return true
	})
	return findings
}
