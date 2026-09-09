package logtest_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/sourceguardtest"
)

// setTestHandlerFunc is the swap the rule is expressed in, matched on the
// callee's own name so a package-qualified call and a call from inside the
// package that declares it read alike.
const setTestHandlerFunc = "SetTestHandler"

// logOwnerDir declares the handler and so calls it from its own suites with
// handlers of its own.
var logOwnerDir = filepath.Join("internal", "log")

// logOwnerPackage is the other half of that: only an in-package file cannot
// reach a Sink — logtest imports internal/log, so importing it back from
// `package log` is an import cycle — which puts those calls outside the rule
// rather than exempted from it. An external `package log_test` file in the same
// directory has no cycle to plead and is judged like anyone else's.
const logOwnerPackage = "log"

// sanctionedHandlerInstalls names every site allowed to swap the process-wide
// handler, keyed by file and enclosing function, against why a capture sink
// cannot serve there. Every other test takes its handler from logtest.Install,
// so one route to a captured record exists rather than two.
var sanctionedHandlerInstalls = map[string]string{
	handlerSite(filepath.Join("internal", "logtest", "install.go"), "Install"):                    "the route itself",
	handlerSite(filepath.Join("cmd", "logging_capture_test.go"), "initTestLogToStateDirAs"):       "a discard silencing the pre-Init window, whose records nothing reads back",
	handlerSite(filepath.Join("cmd", "open_test.go"), "TestExecMarker_VisibleAtWARN"):             "the production level gate, which a Sink admitting every level cannot model",
	handlerSite(filepath.Join("internal", "hooks", "store_test.go"), "TestSetEmitsOpAsJSONField"): "the JSON rendering of an emission, which a Sink does not produce",
}

// handlerInstall is one call swapping the process-wide handler: where it sits,
// and the declaration holding it, which is what the sanctioned list names.
type handlerInstall struct {
	File string
	Func string
	Line int
}

// TestInstallIsTheOnlyRouteToACaptureHandler polices the rule that a test
// wanting captured records takes its sink from logtest.Install(t) rather than
// pairing a fresh Sink with a handler swap by hand. The pairing is what makes
// the capture and its restore one act: a hand-written swap is free to forget the
// restore, leaking a sink into every sibling test, and it is free to install a
// sink that nothing ever reads.
//
// The three survivors are sanctioned by name rather than by argument shape: what
// makes each legitimate is the handler it installs instead of a sink, and that
// reason belongs written down beside the site.
func TestInstallIsTheOnlyRouteToACaptureHandler(t *testing.T) {
	_, sources := sourceguardtest.RepoSources(t, sourceguardtest.AllSources)

	installs := handlerInstallsUnderTheRule(sources)

	scanned, defects := auditHandlerInstalls(installs)
	defects = append(defects, unmatchedSanctions(installs)...)

	if msg := handlerGuardFailure(scanned, defects); msg != "" {
		t.Fatal(msg)
	}
}

// handlerInstallsUnderTheRule collects the swaps made by the sources the rule
// reaches, dropping only the owner's in-package files.
func handlerInstallsUnderTheRule(sources []sourceguardtest.ParsedSource) []handlerInstall {
	var installs []handlerInstall
	for _, source := range sources {
		if ownerInPackageSource(source) {
			continue
		}
		installs = append(installs, handlerInstallsIn(source)...)
	}
	return installs
}

// ownerInPackageSource reports whether a source is one of internal/log's own
// in-package files — the directory alone is not enough, since an external test
// package sitting in it can import logtest with no cycle.
func ownerInPackageSource(source sourceguardtest.ParsedSource) bool {
	return strings.HasPrefix(source.Path, logOwnerDir+string(filepath.Separator)) &&
		source.File.Name.Name == logOwnerPackage
}

// handlerInstallsIn records every handler swap one parsed file makes, attributed
// to the declaration holding it.
func handlerInstallsIn(source sourceguardtest.ParsedSource) []handlerInstall {
	var installs []handlerInstall
	sourceguardtest.ForEachFuncCall(source.File, func(funcName string, call *ast.CallExpr) bool {
		if sourceguardtest.CalleeName(call) == setTestHandlerFunc {
			installs = append(installs, handlerInstall{
				File: source.Path,
				Func: funcName,
				Line: source.Fset.Position(call.Pos()).Line,
			})
		}
		return true
	})
	return installs
}

// auditHandlerInstalls returns how many swaps it judged and the sorted
// descriptions of those no sanction covers.
func auditHandlerInstalls(installs []handlerInstall) (scanned int, defects []string) {
	for _, install := range installs {
		scanned++
		if _, sanctioned := sanctionedHandlerInstalls[handlerSite(install.File, install.Func)]; sanctioned {
			continue
		}
		defects = append(defects, fmt.Sprintf("%s:%d %s swaps the process-wide handler by hand — take a sink from logtest.Install(t), which pairs the capture with its restore",
			install.File, install.Line, install.Func))
	}
	sort.Strings(defects)
	return scanned, defects
}

// unmatchedSanctions reports each sanctioned site making no handler swap at all.
// Such an entry describes a site that has moved or gone, so leaving it standing
// would carry a permission over a name a later test could take.
func unmatchedSanctions(installs []handlerInstall) []string {
	made := make(map[string]bool, len(installs))
	for _, install := range installs {
		made[handlerSite(install.File, install.Func)] = true
	}

	var unmatched []string
	for site, reason := range sanctionedHandlerInstalls {
		if !made[site] {
			unmatched = append(unmatched, fmt.Sprintf("%s swaps no handler, so its sanction (%s) names a site that is gone", site, reason))
		}
	}
	sort.Strings(unmatched)
	return unmatched
}

// handlerGuardFailure renders the guard's failure, or "" when the tree passes.
func handlerGuardFailure(scanned int, defects []string) string {
	if scanned == 0 {
		return fmt.Sprintf("no call to %s anywhere outside %s; the guard has stopped looking rather than passed", setTestHandlerFunc, logOwnerDir)
	}
	if len(defects) == 0 {
		return ""
	}
	sort.Strings(defects)
	return fmt.Sprintf("%d of %d handler swaps are unaccounted for:\n  %s", len(defects), scanned, strings.Join(defects, "\n  "))
}

// handlerSite keys one declaration of one file, which is the grain the sanctions
// are written at.
func handlerSite(file, funcName string) string {
	return file + " " + funcName
}

// stageHandlerInstalls parses one miniature fixture file under the path the rule
// judges it by, returning the swaps it makes.
func stageHandlerInstalls(t *testing.T, rel, src string) []handlerInstall {
	t.Helper()
	return handlerInstallsIn(stageSource(t, rel, src))
}

// stageSource parses one miniature fixture file under the path the rule judges
// it by. Nothing is written to disk, so a fixture is never a dependency of the
// package that stages it.
func stageSource(t *testing.T, rel, src string) sourceguardtest.ParsedSource {
	t.Helper()
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, filepath.Base(rel), src, sourceguardtest.ParseMode)
	if err != nil {
		t.Fatalf("parse fixture source: %v", err)
	}
	return sourceguardtest.ParsedSource{Path: rel, Fset: fset, File: parsed}
}

// The owner directory holds files of both kinds, and only the package clause
// separates them: an in-package file has an import cycle to plead, an external
// test package has none.
const (
	fixtureOwnerInPackageHandler = `package log

import "testing"

func TestHandlerSwap(t *testing.T) {
	SetTestHandler(t, &recordingHandler{})
}
`

	fixtureOwnerExternalTestSink = `package log_test

import "testing"

func TestRogue(t *testing.T) {
	sink := &logtest.Sink{}
	log.SetTestHandler(t, sink)
	_ = sink
}
`
)

// TestHandlerRuleJudgesAnExternalTestPackageInTheOwnerDirectory pins the half of
// the skip the directory prefix alone would give away: a `package log_test` file
// can import logtest, so a hand-paired Sink there is the very pairing the rule
// exists to refuse.
func TestHandlerRuleJudgesAnExternalTestPackageInTheOwnerDirectory(t *testing.T) {
	rel := filepath.Join("internal", "log", "discard_guard_test.go")

	installs := handlerInstallsUnderTheRule([]sourceguardtest.ParsedSource{stageSource(t, rel, fixtureOwnerExternalTestSink)})

	scanned, defects := auditHandlerInstalls(installs)
	if scanned != 1 {
		t.Fatalf("scanned = %d, want 1 — an external test package in %s is under the rule", scanned, logOwnerDir)
	}
	if len(defects) != 1 || !strings.Contains(defects[0], rel) {
		t.Fatalf("defects = %v, want one naming %s", defects, rel)
	}
}

// TestHandlerRuleSkipsTheOwnersInPackageFiles is the other half: those calls are
// outside the rule because a Sink cannot reach them at all.
func TestHandlerRuleSkipsTheOwnersInPackageFiles(t *testing.T) {
	rel := filepath.Join("internal", "log", "log_test.go")

	installs := handlerInstallsUnderTheRule([]sourceguardtest.ParsedSource{stageSource(t, rel, fixtureOwnerInPackageHandler)})

	if len(installs) != 0 {
		t.Fatalf("installs = %v, want none — %s's own in-package suites are outside the rule", installs, logOwnerDir)
	}
}
