package cmd

import (
	"fmt"
	"go/ast"
	"go/types"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/sourceguardtest"
)

// doctorReadOnlyStoreMethods are the (*hooks.Store) methods a doctor source
// may reach: the reads. Every other method writes hooks.json, so a method the
// store gains is a write until it is named here.
var doctorReadOnlyStoreMethods = []string{"Load", "LoadStrict", "List", "LookupOnResume"}

// doctorHookRepair is the one doctor function that may run the sweep: the
// --fix repair. The diagnosis counts through hooksweep.Stale, which removes
// nothing. doctorFix is the one function that may run the repair, so the
// read-only path can't reach it through a call of its own.
const (
	doctorHookRepair = "pruneDoctorStaleHooks"
	doctorFix        = "runDoctorFix"
)

// doctorSources are the sources named doctor*.go.
func doctorSources(pkg sourceguardtest.TypedPackage) sourceguardtest.TypedPackage {
	pkg.Sources = slices.DeleteFunc(slices.Clone(pkg.Sources), func(s sourceguardtest.ParsedSource) bool {
		return !strings.HasPrefix(filepath.Base(s.Path), "doctor")
	})
	return pkg
}

// doctorHookWrites reports each "<file>:<line>: <reach>" a doctor source makes
// for a write to hooks.json: a (*hooks.Store) method other than a read, in any
// form, anywhere, and hooksweep.Run outside the repair. Each is judged by the
// object an identifier names, so an alias or a method value is caught. The
// read-only diagnosis must leave hooks.json as it found it.
func doctorHookWrites(pkg sourceguardtest.TypedPackage) []string {
	return reachesIn(pkg, doctorHookWrite)
}

// doctorRepairReaches reports each "<file>:<line>: <reach>" a source makes for
// the repair outside --fix, in any form: a call, or the function as a value.
func doctorRepairReaches(pkg sourceguardtest.TypedPackage) []string {
	return reachesIn(pkg, func(pkg sourceguardtest.TypedPackage, ident *ast.Ident, enclosing string) string {
		fn, ok := pkg.Info.Uses[ident].(*types.Func)
		if !ok || fn.Signature().Recv() != nil || fn.Name() != doctorHookRepair || fn.Pkg() == nil || fn.Pkg().Name() != "cmd" || enclosing == doctorFix {
			return ""
		}
		return doctorHookRepair + " outside " + doctorFix
	})
}

// reachesIn reports, as "<file>:<line>: <reach>", each identifier in pkg that
// reach judges a reach, handing it the function declaration enclosing it.
func reachesIn(pkg sourceguardtest.TypedPackage, reach func(pkg sourceguardtest.TypedPackage, ident *ast.Ident, enclosing string) string) []string {
	var found []string
	for _, source := range pkg.Sources {
		name := filepath.Base(source.Path)
		for _, decl := range source.File.Decls {
			enclosing := ""
			if fn, ok := decl.(*ast.FuncDecl); ok {
				enclosing = fn.Name.Name
			}
			ast.Inspect(decl, func(n ast.Node) bool {
				ident, ok := n.(*ast.Ident)
				if !ok {
					return true
				}
				if r := reach(pkg, ident, enclosing); r != "" {
					found = append(found, fmt.Sprintf("%s:%d: %s", name, source.Position(ident.Pos()).Line, r))
				}
				return true
			})
		}
	}
	return found
}

func doctorHookWrite(pkg sourceguardtest.TypedPackage, ident *ast.Ident, enclosing string) string {
	if pkg.IsPackageFunc(ident, hooksweepPkgPath, "Run") && enclosing != doctorHookRepair {
		return "hooksweep.Run outside " + doctorHookRepair
	}
	fn, ok := pkg.Object(ident).(*types.Func)
	if !ok || !isHooksStoreMethod(fn) || slices.Contains(doctorReadOnlyStoreMethods, fn.Name()) {
		return ""
	}
	return "(*hooks.Store)." + fn.Name()
}

func isHooksStoreMethod(fn *types.Func) bool {
	recv := fn.Signature().Recv()
	if recv == nil {
		return false
	}
	ptr, ok := recv.Type().(*types.Pointer)
	if !ok {
		return false
	}
	named, ok := ptr.Elem().(*types.Named)
	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == hooksPkgPath && named.Obj().Name() == "Store"
}

func runDoctorHookWritesGuard(t harnesstest.TestingT, pkg sourceguardtest.TypedPackage) {
	t.Helper()
	if len(pkg.Sources) == 0 {
		t.Fatalf("no doctor*.go source in the cmd package; the guard is scanning nothing")
	}
	for _, found := range doctorHookWrites(pkg) {
		t.Errorf("%s; doctor writes hooks.json only from its --fix repair", found)
	}
}

// runDoctorRepairCallerGuard judges every source in cmd, not doctor's alone:
// a call from anywhere else reaches the repair outside --fix as surely.
func runDoctorRepairCallerGuard(t harnesstest.TestingT, pkg sourceguardtest.TypedPackage) {
	t.Helper()
	if len(pkg.Sources) == 0 {
		t.Fatalf("no source in the cmd package; the guard is scanning nothing")
	}
	for _, found := range doctorRepairReaches(pkg) {
		t.Errorf("%s; only --fix runs the hook repair", found)
	}
}

func TestDoctorWritesHooksOnlyFromItsRepair(t *testing.T) {
	checker := sourceguardtest.NewTypeChecker(t, ".")

	t.Run("it finds no write to hooks.json outside the repair", func(t *testing.T) {
		runDoctorHookWritesGuard(t, doctorSources(checker.Package(t)))
	})

	t.Run("it fails when it scans nothing", func(t *testing.T) {
		rec := &harnesstest.Recorder{}
		rec.Run(func() { runDoctorHookWritesGuard(rec, sourceguardtest.TypedPackage{}) })
		if !rec.Failed() {
			t.Error("the guard passed over no sources")
		}
	})

	for _, tc := range []struct {
		name, body string
		want       []string
	}{
		{"the sweep run from the diagnosis", `import "github.com/leeovery/portal/internal/hooksweep"

func checkStaleHooks() { _, _ = hooksweep.Run(nil, hooksweep.Sources{}, nil) }`,
			[]string{"doctor.go:5: hooksweep.Run outside " + doctorHookRepair}},
		{"the sweep under an alias as a value", `import sweep "github.com/leeovery/portal/internal/hooksweep"

var run = sweep.Run`,
			[]string{"doctor.go:5: hooksweep.Run outside " + doctorHookRepair}},
		{"a store write", `import "github.com/leeovery/portal/internal/hooks"

func checkStaleHooks(s *hooks.Store) { _, _ = s.Remove("k", hooks.EventOnResume, hooks.ViaDoctor) }`,
			[]string{"doctor.go:5: (*hooks.Store).Remove"}},
		{"a store clean as a method expression", `import "github.com/leeovery/portal/internal/hooks"

var clean = (*hooks.Store).CleanStale`,
			[]string{"doctor.go:5: (*hooks.Store).CleanStale"}},
		{"the sweep from the repair, and the reads anywhere", `import (
	"context"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hooksweep"
)

func ` + doctorHookRepair + `(ctx context.Context, s *hooks.Store) { _, _ = hooksweep.Run(ctx, hooksweep.Sources{}, s) }

func checkStaleHooks(ctx context.Context, s *hooks.Store) {
	persisted, _ := s.Load(hooks.ViaDoctor)
	_, _ = hooksweep.Stale(ctx, hooksweep.Sources{}, persisted)
	_, _ = s.List(hooks.ViaDoctor)
}`,
			nil},
	} {
		t.Run("it judges "+tc.name, func(t *testing.T) {
			src := "package cmd\n\n" + tc.body + "\n"
			if got := doctorHookWrites(checker.Sources(t, map[string]string{"doctor.go": src})); !slices.Equal(got, tc.want) {
				t.Errorf("doctorHookWrites = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestOnlyDoctorFixRunsTheHookRepair(t *testing.T) {
	checker := sourceguardtest.NewTypeChecker(t, ".")

	t.Run("it finds the repair reached from --fix alone", func(t *testing.T) {
		runDoctorRepairCallerGuard(t, checker.Package(t))
	})

	t.Run("it fails when it scans nothing", func(t *testing.T) {
		rec := &harnesstest.Recorder{}
		rec.Run(func() { runDoctorRepairCallerGuard(rec, sourceguardtest.TypedPackage{}) })
		if !rec.Failed() {
			t.Error("the guard passed over no sources")
		}
	})

	const repair = "func " + doctorHookRepair + "() {}\n\n"
	for _, tc := range []struct {
		name, file, body string
		want             []string
	}{
		{"a call from the diagnosis", "doctor.go", repair + "func runDoctorDiagnosis() { " + doctorHookRepair + "() }",
			[]string{"doctor.go:5: " + doctorHookRepair + " outside " + doctorFix}},
		{"a call from outside doctor's files", "state_daemon.go", repair + "func tick() { " + doctorHookRepair + "() }",
			[]string{"state_daemon.go:5: " + doctorHookRepair + " outside " + doctorFix}},
		{"the repair as a value", "doctor.go", repair + "var prune = " + doctorHookRepair,
			[]string{"doctor.go:5: " + doctorHookRepair + " outside " + doctorFix}},
		{"a call from --fix", "doctor.go", repair + "func " + doctorFix + "() { " + doctorHookRepair + "() }", nil},
	} {
		t.Run("it judges "+tc.name, func(t *testing.T) {
			src := "package cmd\n\n" + tc.body + "\n"
			if got := doctorRepairReaches(checker.Sources(t, map[string]string{tc.file: src})); !slices.Equal(got, tc.want) {
				t.Errorf("doctorRepairReaches = %q, want %q", got, tc.want)
			}
		})
	}
}
