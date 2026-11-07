package portalbintest_test

import (
	"strings"
	"testing"
)

// The rule reads source, so each case below is a whole miniature test file.
const (
	fixtureUntaggedBuilder = `package cmd

import "testing"

func TestX(t *testing.T) {
	binDir := portalbintest.StagePortalBinary(t)
	_ = binDir
}
`

	fixtureIntegrationTaggedBuilder = `//go:build integration

package cmd

import "testing"

func TestX(t *testing.T) {
	binDir := portalbintest.StagePortalBinary(t)
	_ = binDir
}
`

	fixtureUntaggedNoBuilder = `package cmd

import "testing"

func TestX(t *testing.T) {
	root, err := portalbintest.ProjectRoot()
	_, _ = root, err
}
`
)

func TestLaneRuleFlagsABuildHelperReferencedFromAnUntaggedTest(t *testing.T) {
	files := []scannedTestFile{
		stageTestFile(t, "cmd/rogue_test.go", fixtureUntaggedBuilder),
		stageTestFile(t, "cmd/tagged_test.go", fixtureIntegrationTaggedBuilder),
	}

	scanned, referenced, defects := auditBuildHelperLane(files)

	if scanned != 1 {
		t.Fatalf("scanned = %d, want 1 — one of the two fixtures is in the unit lane", scanned)
	}
	if referenced != 2 {
		t.Fatalf("referenced = %d, want 2", referenced)
	}
	if len(defects) != 1 || !strings.Contains(defects[0], "cmd/rogue_test.go") {
		t.Fatalf("defects = %v, want one naming cmd/rogue_test.go", defects)
	}
	if !strings.Contains(defects[0], "StagePortalBinary") {
		t.Errorf("defect %q does not name the build helper it caught", defects[0])
	}
}

func TestLaneRuleAllowsABuildHelperReferencedFromAnIntegrationTaggedTest(t *testing.T) {
	files := []scannedTestFile{
		stageTestFile(t, "cmd/tagged_test.go", fixtureIntegrationTaggedBuilder),
		stageTestFile(t, "cmd/plain_test.go", fixtureUntaggedNoBuilder),
	}

	scanned, referenced, defects := auditBuildHelperLane(files)

	if scanned != 1 {
		t.Fatalf("scanned = %d, want 1", scanned)
	}
	if referenced != 1 {
		t.Fatalf("referenced = %d, want 1", referenced)
	}
	if len(defects) != 0 {
		t.Fatalf("defects = %v, want none — the integration tag is what the rule asks for", defects)
	}
	if msg := laneGuardFailure(scanned, referenced, defects); msg != "" {
		t.Fatalf("laneGuardFailure = %q, want empty", msg)
	}
}

func TestLaneRuleFailsWhenItScansNothing(t *testing.T) {
	msg := laneGuardFailure(0, 3, nil)

	if msg == "" {
		t.Fatal("laneGuardFailure = empty over zero unit-lane test files; the guard must fail rather than pass having found nothing")
	}
	if !strings.Contains(msg, "stopped looking") {
		t.Errorf("laneGuardFailure = %q; want it to say the guard has stopped looking", msg)
	}
}

func TestLaneRuleFailsWhenNoTestReferencesABuildHelperAtAll(t *testing.T) {
	scanned, referenced, defects := auditBuildHelperLane([]scannedTestFile{
		stageTestFile(t, "cmd/plain_test.go", fixtureUntaggedNoBuilder),
	})

	if referenced != 0 {
		t.Fatalf("referenced = %d, want 0", referenced)
	}
	msg := laneGuardFailure(scanned, referenced, defects)
	if msg == "" {
		t.Fatal("laneGuardFailure = empty when nothing references a build helper; the vocabulary has drifted off the tree")
	}
	if !strings.Contains(msg, "drifted") {
		t.Errorf("laneGuardFailure = %q; want it to say the helper vocabulary has drifted", msg)
	}
}
