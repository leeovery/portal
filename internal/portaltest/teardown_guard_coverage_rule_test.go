package portaltest_test

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/sourceguardtest"
)

// The rule reads source, so each case below is a whole miniature fixture file.
const (
	fixtureFullyCovered = `package x

import "testing"

func TestX(t *testing.T) {
	env, stateDir := portaltest.IsolateStateForTest(t)
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	portaltest.RegisterStateDirTeardownGuard(t, stateDir)
	ts := zmxtest.New(t)
	_ = env
	_ = ts
}
`

	fixtureNoTeardownGuard = `package x

import "testing"

func TestX(t *testing.T) {
	env, stateDir := portaltest.IsolateStateForTest(t)
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	ts := zmxtest.New(t)
	_ = env
	_ = ts
}
`

	fixtureHandRolledStateDir = `package x

import "testing"

func TestX(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	portaltest.RegisterStateDirTeardownGuard(t, stateDir)
	ts := zmxtest.New(t)
	_ = ts
}
`

	fixtureSharedArrangeStateDir = `package x

import "testing"

func TestX(t *testing.T) {
	env, stateDir := portaltest.IsolateStateForTest(t)
	ts := zmxtest.New(t)
	_ = env
	_ = stateDir
	_ = ts
}
`

	fixtureNoServer = `package x

import "testing"

func TestX(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", stateDir)
}
`

	fixtureServerBeforeIsolate = `package x

import "testing"

func TestX(t *testing.T) {
	ts := zmxtest.New(t)
	env, stateDir := portaltest.IsolateStateForTest(t)
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	portaltest.RegisterStateDirTeardownGuard(t, stateDir)
	_ = env
	_ = ts
}
`

	fixtureGuardAfterServer = `package x

import "testing"

func TestX(t *testing.T) {
	env, stateDir := portaltest.IsolateStateForTest(t)
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	ts := zmxtest.New(t)
	portaltest.RegisterStateDirTeardownGuard(t, stateDir)
	_ = env
	_ = ts
}
`

	fixtureThroughLocalArrange = `package x

import "testing"

func TestX(t *testing.T) {
	env, stateDir := newIntegrationStateDir(t)
	ts := zmxtest.New(t)
	_ = env
	_ = stateDir
	_ = ts
}
`

	fixtureLocalArrange = `package x

import "testing"

func newIntegrationStateDir(t *testing.T) (env []string, stateDir string) {
	env, stateDir = portaltest.IsolateStateForTest(t)
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	portaltest.RegisterStateDirTeardownGuard(t, stateDir)
	return env, stateDir
}
`

	fixtureLocalArrangeWithoutGuard = `package x

import "testing"

func newIntegrationStateDir(t *testing.T) (env []string, stateDir string) {
	env, stateDir = portaltest.IsolateStateForTest(t)
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	return env, stateDir
}
`
)

// scanFixture parses one miniature fixture file into the records the rule is
// audited over. Several calls compose a package, which is what a fixture
// reaching its arrange through another file needs.
func scanFixture(t *testing.T, file, src string) []scannedFunc {
	t.Helper()
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, "fixture.go", src, sourceguardtest.ParseMode)
	if err != nil {
		t.Fatalf("parse fixture source: %v", err)
	}
	pkg, calls := fixtureCallsIn(fset, parsed)
	funcs := make([]scannedFunc, 0, len(calls))
	for name, scopes := range calls {
		funcs = append(funcs, scannedFunc{Pkg: pkg, File: file, Name: name, Scopes: scopes})
	}
	return funcs
}

func TestCoverageRuleFailsServerFixtureWithoutTeardownGuard(t *testing.T) {
	scanned, defects := auditFixtureCoverage(scanFixture(t, "no_guard_test.go", fixtureNoTeardownGuard))

	if scanned != 1 {
		t.Fatalf("scanned = %d, want 1", scanned)
	}
	if len(defects) != 1 || !strings.Contains(defects[0], "no_guard_test.go TestX") {
		t.Fatalf("defects = %v, want one naming no_guard_test.go TestX", defects)
	}
	if !strings.Contains(defects[0], guardCall) {
		t.Errorf("defect %q does not name the missing %s", defects[0], guardCall)
	}
}

func TestCoverageRuleFailsHandRolledStateDirWithoutIsolation(t *testing.T) {
	scanned, defects := auditFixtureCoverage(scanFixture(t, "hand_rolled_test.go", fixtureHandRolledStateDir))

	if scanned != 1 {
		t.Fatalf("scanned = %d, want 1; a hand-rolled state dir must not be invisible", scanned)
	}
	if len(defects) != 1 || !strings.Contains(defects[0], isolateCall) {
		t.Fatalf("defects = %v, want one naming the missing %s", defects, isolateCall)
	}
}

func TestCoverageRulePassesFixtureMakingAllThreeCalls(t *testing.T) {
	scanned, defects := auditFixtureCoverage(scanFixture(t, "covered_test.go", fixtureFullyCovered))

	if scanned != 1 {
		t.Fatalf("scanned = %d, want 1", scanned)
	}
	if len(defects) != 0 {
		t.Fatalf("defects = %v, want none", defects)
	}
	if msg := coverageFailure(scanned, defects); msg != "" {
		t.Fatalf("coverageFailure = %q, want empty", msg)
	}
}

func TestCoverageRuleFailsIsolatedFixtureTakingItsStateDirFromASharedArrange(t *testing.T) {
	funcs := scanFixture(t, "shared_arrange_test.go", fixtureSharedArrangeStateDir)
	if funcs[0].aggregate().NamesStateDir {
		t.Fatalf("fixture names %s; it must not, or it qualifies on the other arm", stateDirEnv)
	}

	scanned, defects := auditFixtureCoverage(funcs)

	if scanned != 1 {
		t.Fatalf("scanned = %d, want 1; a fixture that isolates and starts a server stays in reach "+
			"even when the state dir it uses is named in a shared arrange", scanned)
	}
	if len(defects) != 1 || !strings.Contains(defects[0], "shared_arrange_test.go TestX") {
		t.Fatalf("defects = %v, want one naming shared_arrange_test.go TestX", defects)
	}
}

func TestCoverageRuleFailsAFixtureStartingAServerBeforeIsolating(t *testing.T) {
	scanned, defects := auditFixtureCoverage(scanFixture(t, "server_first_test.go", fixtureServerBeforeIsolate))

	if scanned != 1 {
		t.Fatalf("scanned = %d, want 1", scanned)
	}
	want := "server_first_test.go TestX: starts a session server at line 6 before isolating at line 7"
	if len(defects) != 1 || defects[0] != want {
		t.Fatalf("defects = %v, want [%s]; a server started before the isolation must fail the rule", defects, want)
	}
}

func TestCoverageRuleFailsAFixtureRegisteringTheTeardownGuardAfterTheServer(t *testing.T) {
	scanned, defects := auditFixtureCoverage(scanFixture(t, "late_guard_test.go", fixtureGuardAfterServer))

	if scanned != 1 {
		t.Fatalf("scanned = %d, want 1", scanned)
	}
	want := "late_guard_test.go TestX: starts a session server at line 8 before registering the teardown guard at line 9"
	if len(defects) != 1 || defects[0] != want {
		t.Fatalf("defects = %v, want [%s]; LIFO cleanup makes a guard registered after the server useless", defects, want)
	}
}

func TestCoverageRuleCountsAFixtureIsolatingThroughAPackageLocalArrange(t *testing.T) {
	funcs := append(
		scanFixture(t, "through_arrange_test.go", fixtureThroughLocalArrange),
		scanFixture(t, "helpers_test.go", fixtureLocalArrange)...,
	)

	scanned, defects := auditFixtureCoverage(funcs)

	if scanned != 1 {
		t.Fatalf("scanned = %d, want 1; a fixture reaching both helpers through a same-package arrange "+
			"must be judged rather than skipped", scanned)
	}
	if len(defects) != 0 {
		t.Fatalf("defects = %v, want none; the arrange isolates and registers the guard before the server starts", defects)
	}
}

func TestCoverageRuleFailsAPackageLocalArrangeOmittingTheGuard(t *testing.T) {
	funcs := append(
		scanFixture(t, "through_arrange_test.go", fixtureThroughLocalArrange),
		scanFixture(t, "helpers_test.go", fixtureLocalArrangeWithoutGuard)...,
	)

	scanned, defects := auditFixtureCoverage(funcs)

	if scanned != 1 {
		t.Fatalf("scanned = %d, want 1", scanned)
	}
	if len(defects) != 1 || !strings.Contains(defects[0], "through_arrange_test.go TestX") {
		t.Fatalf("defects = %v, want one naming through_arrange_test.go TestX", defects)
	}
	if !strings.Contains(defects[0], guardCall) {
		t.Errorf("defect %q does not name the missing %s", defects[0], guardCall)
	}
}

func TestCoverageRuleFailsWhenNothingQualifies(t *testing.T) {
	scanned, defects := auditFixtureCoverage(scanFixture(t, "no_server_test.go", fixtureNoServer))

	if scanned != 0 {
		t.Fatalf("scanned = %d, want 0", scanned)
	}
	msg := coverageFailure(scanned, defects)
	if msg == "" {
		t.Fatal("coverageFailure = empty over zero qualifying files; the guard must fatal rather than pass")
	}
	if !strings.Contains(msg, "stopped looking") {
		t.Errorf("coverageFailure = %q; want it to say the guard has stopped looking", msg)
	}
}

// fixtureServerInASiblingClosure is the shape of a table-style fixture: one
// closure starts the server the subtests drive, another isolates before calling
// it. Lexically the server comes first; at run time it does not.
const fixtureServerInASiblingClosure = `package x

import "testing"

func TestX(t *testing.T) {
	invoke := func(t *testing.T) {
		ts := zmxtest.New(t)
		_ = ts
	}
	t.Run("case", func(t *testing.T) {
		env, stateDir := portaltest.IsolateStateForTest(t)
		t.Setenv("PORTAL_STATE_DIR", stateDir)
		portaltest.RegisterStateDirTeardownGuard(t, stateDir)
		invoke(t)
		_ = env
	})
}
`

func TestCoverageRuleJudgesOrderWithinOneScopeOnly(t *testing.T) {
	scanned, defects := auditFixtureCoverage(scanFixture(t, "closures_test.go", fixtureServerInASiblingClosure))

	if scanned != 1 {
		t.Fatalf("scanned = %d, want 1", scanned)
	}
	if len(defects) != 0 {
		t.Fatalf("defects = %v, want none; the server and the isolation sit in sibling closures, "+
			"so their line order says nothing about the order they run in", defects)
	}
}

// fixtureServerThroughLocalHelper takes its server from a same-package helper
// and registers no guard: the mirror of routing the isolation through an
// arrange, and just as much a way out of the rule.
const fixtureServerThroughLocalHelper = `package x

import "testing"

func TestX(t *testing.T) {
	env, stateDir := portaltest.IsolateStateForTest(t)
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	ts := startServer(t)
	_ = env
	_ = ts
}
`

const fixtureLocalServerHelper = `package x

import "testing"

func startServer(t *testing.T) *zmx.Backend {
	return zmxtest.New(t)
}
`

func TestCoverageRuleFailsAFixtureWhoseServerStartsInAPackageLocalHelper(t *testing.T) {
	funcs := append(
		scanFixture(t, "through_helper_test.go", fixtureServerThroughLocalHelper),
		scanFixture(t, "helpers_test.go", fixtureLocalServerHelper)...,
	)

	scanned, defects := auditFixtureCoverage(funcs)

	if scanned != 1 {
		t.Fatalf("scanned = %d, want 1; taking the server from a same-package helper must not "+
			"put the fixture out of the rule's reach", scanned)
	}
	want := "through_helper_test.go TestX: starts a session server at line 8 without calling " + guardCall
	if len(defects) != 1 || defects[0] != want {
		t.Fatalf("defects = %v, want [%s]", defects, want)
	}
}

// fixtureArrangeInAnotherPackage declares the same arrange name in a second
// package, which the hop must not answer a package x caller with.
const fixtureArrangeInAnotherPackage = `package y

import "testing"

func newIntegrationStateDir(t *testing.T) (env []string, stateDir string) {
	env, stateDir = portaltest.IsolateStateForTest(t)
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	portaltest.RegisterStateDirTeardownGuard(t, stateDir)
	return env, stateDir
}
`

func TestCoverageRuleDoesNotFollowAnArrangeInAnotherPackage(t *testing.T) {
	funcs := append(
		scanFixture(t, "through_arrange_test.go", fixtureThroughLocalArrange),
		scanFixture(t, "other_package_helpers_test.go", fixtureArrangeInAnotherPackage)...,
	)

	scanned, defects := auditFixtureCoverage(funcs)

	if scanned != 0 {
		t.Fatalf("scanned = %d, want 0; the hop resolves within one package, so an identically "+
			"named arrange elsewhere must not stand in for the caller's own", scanned)
	}
	if len(defects) != 0 {
		t.Fatalf("defects = %v, want none over a fixture the rule cannot reach", defects)
	}
}

// zmxFixture is a fixture whose session server is a zmxtest directory, its
// calls in the order body gives them.
func zmxFixture(body string) string {
	return "package x\n\nimport \"testing\"\n\nfunc TestX(t *testing.T) {\n" + body + "}\n"
}

func TestCoverageRuleHoldsAZmxFixtureToTheSameRule(t *testing.T) {
	const (
		isolate = "\tenv, stateDir := portaltest.IsolateStateForTest(t)\n"
		guard   = "\tportaltest.RegisterStateDirTeardownGuard(t, stateDir)\n"
		server  = "\tb := zmxtest.New(t)\n\t_, _ = env, b\n"
	)
	for _, tc := range []struct {
		name, body, want string
	}{
		{name: "isolated and guarded before the server, it passes", body: isolate + guard + server},
		{name: "with no guard, it fails", body: isolate + server, want: "without calling " + guardCall},
		{name: "guarded after the server, it fails", body: isolate + server + guard, want: "starts a session server at line 7 before registering the teardown guard at line 9"},
		{name: "started before the isolation, it fails", body: server + isolate + guard, want: "starts a session server at line 6 before isolating at line 8"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scanned, defects := auditFixtureCoverage(scanFixture(t, "zmx_test.go", zmxFixture(tc.body)))

			if scanned != 1 {
				t.Fatalf("scanned = %d, want 1; a zmxtest.New fixture pairing a state dir is in the rule's reach", scanned)
			}
			switch {
			case tc.want == "" && len(defects) != 0:
				t.Fatalf("defects = %v, want none", defects)
			case tc.want != "" && (len(defects) != 1 || !strings.Contains(defects[0], tc.want)):
				t.Fatalf("defects = %v, want one saying %q", defects, tc.want)
			}
		})
	}
}
