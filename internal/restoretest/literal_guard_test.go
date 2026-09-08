package restoretest_test

import (
	"go/ast"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/sourceguardtest"
)

// TestNoTestComposesAPaneArmingRestoreType is the standing guard over the one
// field whose omission is silent: both guarded types take an optional Exe,
// whose fallback — and what it costs a test driving a real restore —
// restoretest.StagedHydrateExe documents.
//
// The rule is therefore not "set Exe" but "never compose the struct": a rule
// about a field that may legitimately be absent cannot be checked, while a rule
// about the literal can. A set field proves nothing either, since a nil
// resolver takes the same fallback an absent one does. Each descriptor's
// constructors pin Exe, so routing through them makes the omission
// unrepresentable.
func TestNoTestComposesAPaneArmingRestoreType(t *testing.T) {
	scans := scanGuardTestFiles(t, restoreLiteralGuards)

	for i, guard := range restoreLiteralGuards {
		scan := scans[i]
		if len(scan.findings) == 0 {
			continue
		}
		t.Errorf("%d %s.%s literal(s) in %s (of %d scanned):\n  %s\n"+
			"  Exe is opt-in on the struct and a forgotten one is silent: the pane respawns into\n"+
			"  the test binary and the session vanishes with no error. Build it through %s.",
			len(scan.findings), restorePkg, guard.typeName, guard.fileSet, scan.scanned,
			strings.Join(scan.findings, "\n  "), guard.constructors)
	}
}

func TestRestoreLiteralGuard_FailsForAnUnpinnedExeLiteralOfEitherType(t *testing.T) {
	for _, tc := range []struct {
		name         string
		guard        literalGuard
		files        []guardFixtureFile
		wantScanned  int
		wantFindings int
	}{
		{
			name:  "it flags a test file composing a bare restore.Orchestrator literal",
			guard: orchestratorGuard,
			files: []guardFixtureFile{{name: "fixture_test.go", src: `package fixture

import "github.com/leeovery/portal/internal/restore"

func drive() {
	o := &restore.Orchestrator{StateDir: "/tmp"}
	_ = o
	_ = restore.Orchestrator{}
}
`}},
			wantScanned:  1,
			wantFindings: 2,
		},
		{
			name:  "it passes a test file routing through the orchestrator constructors",
			guard: orchestratorGuard,
			files: []guardFixtureFile{{name: "fixture_test.go", src: `package fixture

import "github.com/leeovery/portal/internal/restoretest"

func drive(t *testingT, client *client) {
	o := restoretest.NewRestoreOrchestrator(t, client, "/state", "/bin")
	p := restoretest.NewFakeExeOrchestrator(t, client, "/state", nil)
	_, _ = o, p
}
`}},
			wantScanned: 1,
		},
		{
			// The rule is scoped to tests: restoretest's own constructors
			// compose the struct, and production wiring composes it too.
			name:  "it ignores a production file composing the orchestrator",
			guard: orchestratorGuard,
			files: []guardFixtureFile{
				{name: "production.go", src: `package fixture

import "github.com/leeovery/portal/internal/restore"

var o = &restore.Orchestrator{}
`},
				{name: "keeps_scanning_test.go", src: "package fixture\n"},
			},
			wantScanned: 1,
		},
		{
			name:  "it flags an integration fixture composing a SessionRestorer literal",
			guard: sessionRestorerGuard,
			files: []guardFixtureFile{{name: "fixture_integration_test.go", src: `//go:build integration

package fixture

import "github.com/leeovery/portal/internal/restore"

func drive() {
	r := &restore.SessionRestorer{StateDir: "/state"}
	_ = r
	_ = restore.SessionRestorer{}
}
`}},
			wantScanned:  1,
			wantFindings: 2,
		},
		{
			// A set Exe is no defence: a nil resolver falls back to
			// os.Executable exactly as an absent field does, so the rule is
			// about the literal.
			name:  "it flags an integration fixture whose Exe is an explicit nil",
			guard: sessionRestorerGuard,
			files: []guardFixtureFile{{name: "fixture_integration_test.go", src: `//go:build integration

package fixture

import "github.com/leeovery/portal/internal/restore"

func drive() {
	_ = &restore.SessionRestorer{StateDir: "/state", Exe: nil}
}
`}},
			wantScanned:  1,
			wantFindings: 1,
		},
		{
			name:  "it ignores an integration fixture routing through the constructor",
			guard: sessionRestorerGuard,
			files: []guardFixtureFile{{name: "fixture_integration_test.go", src: `//go:build integration

package fixture

import "github.com/leeovery/portal/internal/restoretest"

func drive(t *testingT) {
	r := restoretest.NewSessionRestorer(t, nil, "/state", "/bin")
	_ = r
}
`}},
			wantScanned: 1,
		},
		{
			// The mock-driven literals in the unit lane arm no live pane, so an
			// unset Exe there is harmless and they are deliberately left alone.
			name:  "it ignores a unit-lane SessionRestorer literal",
			guard: sessionRestorerGuard,
			files: []guardFixtureFile{
				{name: "unit_test.go", src: `package fixture

import "github.com/leeovery/portal/internal/restore"

func drive() {
	_ = &restore.SessionRestorer{StateDir: "/state"}
}
`},
				{name: "keeps_scanning_integration_test.go", src: "//go:build integration\n\npackage fixture\n"},
			},
			wantScanned: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := writeGuardFixture(t, tc.files...)

			scans := scanGuardTestFiles(t, []literalGuard{tc.guard}, sourceguardtest.Rooted(root))

			if scans[0].scanned != tc.wantScanned {
				t.Fatalf("scanned %d files, want %d", scans[0].scanned, tc.wantScanned)
			}
			if len(scans[0].findings) != tc.wantFindings {
				t.Errorf("scan found %d literals, want %d: %v", len(scans[0].findings), tc.wantFindings, scans[0].findings)
			}
		})
	}
}

// A guard that stopped finding its own sources would otherwise report a clean
// tree forever.
func TestRestoreLiteralGuard_FatalsWhenADescriptorsFileSetEnumeratesNothing(t *testing.T) {
	for _, tc := range []struct {
		name  string
		guard literalGuard
	}{
		{name: "the orchestrator descriptor", guard: orchestratorGuard},
		{name: "the session-restorer descriptor", guard: sessionRestorerGuard},
	} {
		t.Run(tc.name, func(t *testing.T) {
			emptyTree := &harnesstest.Recorder{}
			emptyTree.Run(func() {
				scanGuardTestFiles(emptyTree, []literalGuard{tc.guard}, sourceguardtest.Rooted(t.TempDir()))
			})
			if len(emptyTree.Fatals) != 1 {
				t.Fatalf("scan of a directory holding no sources reported %d fatals, want 1: %v", len(emptyTree.Fatals), emptyTree.Fatals)
			}
			if !strings.Contains(emptyTree.Fatals[0], "stopped looking") {
				t.Errorf("fatal message %q does not say the guard would pass having stopped looking", emptyTree.Fatals[0])
			}
		})
	}

	// Only a descriptor that narrows the walk can see a non-empty tree hold none
	// of its own files — an empty tree is refused by the walk itself, one message
	// upstream of the descriptors.
	t.Run("it keeps a descriptor's own scanned-nothing wording", func(t *testing.T) {
		root := writeGuardFixture(t, guardFixtureFile{name: "unit_test.go", src: "package fixture\n"})

		emptyFileSet := &harnesstest.Recorder{}
		emptyFileSet.Run(func() {
			scanGuardTestFiles(emptyFileSet, []literalGuard{sessionRestorerGuard}, sourceguardtest.Rooted(root))
		})

		if len(emptyFileSet.Fatals) != 1 {
			t.Fatalf("scan of a tree holding no integration-tagged _test.go reported %d fatals, want 1: %v",
				len(emptyFileSet.Fatals), emptyFileSet.Fatals)
		}
		if emptyFileSet.Fatals[0] != sessionRestorerGuard.fatalWording {
			t.Errorf("fatal message %q, want the descriptor's own wording %q", emptyFileSet.Fatals[0], sessionRestorerGuard.fatalWording)
		}
	})

	t.Run("it gives each descriptor its own wording", func(t *testing.T) {
		for _, guard := range restoreLiteralGuards {
			if !strings.Contains(guard.fatalWording, "stopped looking") {
				t.Errorf("the %s descriptor fatals with %q, which does not say the guard would pass having stopped looking",
					guard.typeName, guard.fatalWording)
			}
		}
		if orchestratorGuard.fatalWording == sessionRestorerGuard.fatalWording {
			t.Errorf("both descriptors fatal with %q, so the message does not name the file set that ran dry", orchestratorGuard.fatalWording)
		}
	})
}

// The pair is driven from one walk because parsing the tree per descriptor
// doubles the package's unit-lane cost for the same answer. Sharing is
// observable in the parses themselves: a second walk would hand the second
// descriptor its own *ast.Files.
func TestRestoreLiteralGuard_WalksTheRepoOnceForBothDescriptors(t *testing.T) {
	root := writeGuardFixture(t,
		guardFixtureFile{name: "one_test.go", src: "package fixture\n"},
		guardFixtureFile{name: "two_test.go", src: "package fixture\n"},
	)
	var offeredToFirst, offeredToSecond []*ast.File
	record := func(offered *[]*ast.File) func(*ast.File) bool {
		return func(file *ast.File) bool {
			*offered = append(*offered, file)
			return true
		}
	}
	guards := []literalGuard{
		{typeName: orchestratorType, fileSet: "test files", include: record(&offeredToFirst), fatalWording: "scanned nothing"},
		{typeName: sessionRestorerType, fileSet: "test files", include: record(&offeredToSecond), fatalWording: "scanned nothing"},
	}

	scanGuardTestFiles(t, guards, sourceguardtest.Rooted(root))

	if len(offeredToFirst) != 2 {
		t.Fatalf("the first descriptor was offered %d files, want 2", len(offeredToFirst))
	}
	if !slices.Equal(offeredToFirst, offeredToSecond) {
		t.Errorf("the descriptors were offered different parses of the same tree, so the walk ran twice")
	}
}
