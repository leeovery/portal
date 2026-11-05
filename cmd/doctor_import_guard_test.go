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

const saverhostPkgPath = "github.com/leeovery/portal/internal/saverhost"

// doctorSaverReads are the saverhost Host methods a doctor source may reach:
// the reads. Every other method of the host takes saver.lock, signals or
// creates, so a method it gains is one of those until it is named here.
var doctorSaverReads = []string{"Present", "SaverPID", "Prove"}

// doctorSignallers are cmd's ways to a process: the seam, the hosts that
// signal through it, and the full bootstrap, whose sweeps and saver ending
// signal.
var doctorSignallers = []string{
	"signalProcess",
	"newSaverHost", "saverHostSignalling", "newSaverDaemon", "saverDaemon", "endBySignal",
	"newProductionBootstrap", "owedBootstrap", "bootstrapFor", "runBootstrap",
}

// rawSignals are the package functions that send a signal: a doctor source
// calling one signals without cmd's seam.
var rawSignals = []struct{ pkgPath, name string }{
	{"syscall", "Kill"},
	{"golang.org/x/sys/unix", "Kill"},
}

// doctorSaverReaches reports each "<file>:<line>: <reach>" a doctor source
// makes past the saver's reads. The host doctor is given exposes the reads
// alone, which TestReadOnlySaverHostOnlyReads pins, so this is the way round
// it: a host doctor builds or holds, any method of it but a read, one of
// cmd's signallers, an EndDaemon, whatever it is called on, or a raw signal.
func doctorSaverReaches(pkg sourceguardtest.TypedPackage) []string {
	return reachesIn(pkg, doctorSaverReach)
}

func doctorSaverReach(pkg sourceguardtest.TypedPackage, ident *ast.Ident, _ string) string {
	obj := pkg.Object(ident)
	if obj == nil || obj.Pkg() == nil {
		return ""
	}
	if fn, ok := obj.(*types.Func); ok && isHostMethod(fn) && !slices.Contains(doctorSaverReads, fn.Name()) {
		return "saver method " + fn.Name()
	}
	if obj.Pkg().Name() == "cmd" && obj.Parent() == obj.Pkg().Scope() && slices.Contains(doctorSignallers, obj.Name()) {
		return obj.Name()
	}
	if fn, ok := obj.(*types.Func); ok && fn.Signature().Recv() != nil && fn.Name() == "EndDaemon" {
		return "EndDaemon"
	}
	return rawSignal(pkg, ident, obj)
}

// rawSignal names a signal ident sends without cmd's seam: a package function
// of rawSignals, or os.Process's Signal or Kill.
func rawSignal(pkg sourceguardtest.TypedPackage, ident *ast.Ident, obj types.Object) string {
	for _, raw := range rawSignals {
		if pkg.IsPackageFunc(ident, raw.pkgPath, raw.name) {
			return raw.pkgPath + "." + raw.name
		}
	}
	if fn, ok := obj.(*types.Func); ok && isProcessMethod(fn) && (fn.Name() == "Signal" || fn.Name() == "Kill") {
		return "(*os.Process)." + fn.Name()
	}
	return ""
}

// isProcessMethod reports whether fn is a method of os.Process.
func isProcessMethod(fn *types.Func) bool {
	return isMethodOf(fn, "os", "Process")
}

// isHostMethod reports whether fn is a method of saverhost's Host.
func isHostMethod(fn *types.Func) bool {
	return isMethodOf(fn, saverhostPkgPath, "Host")
}

// isMethodOf reports whether fn is a method of the type typeName in the
// package pkgPath, on a value or a pointer.
func isMethodOf(fn *types.Func, pkgPath, typeName string) bool {
	recv := fn.Signature().Recv()
	if recv == nil {
		return false
	}
	t := recv.Type()
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok || named.Obj().Pkg() == nil {
		return false
	}
	return named.Obj().Pkg().Path() == pkgPath && named.Obj().Name() == typeName
}

func runDoctorSaverReadsGuard(t harnesstest.TestingT, pkg sourceguardtest.TypedPackage) {
	t.Helper()
	if len(pkg.Sources) == 0 {
		t.Fatalf("no doctor*.go source in the cmd package; the guard is scanning nothing")
	}
	for _, found := range doctorSaverReaches(pkg) {
		t.Errorf("%s; doctor only reads the saver, and signals nothing", found)
	}
}

func TestDoctorOnlyReadsTheSaver(t *testing.T) {
	checker := sourceguardtest.NewTypeChecker(t, ".")

	t.Run("it finds doctor reaching the saver's reads alone", func(t *testing.T) {
		runDoctorSaverReadsGuard(t, doctorSources(checker.Package(t)))
	})

	t.Run("it fails when it scans nothing", func(t *testing.T) {
		rec := &harnesstest.Recorder{}
		rec.Run(func() { runDoctorSaverReadsGuard(rec, sourceguardtest.TypedPackage{}) })
		if !rec.Failed() {
			t.Error("the guard passed over no sources")
		}
	})

	for _, tc := range []struct {
		name, body string
		want       []string
	}{
		{"a stop through the host", `import (
	"context"

	"github.com/leeovery/portal/internal/saverhost"
)

func checkSaver(ctx context.Context, h *saverhost.Host) { _, _ = h.Stop(ctx) }`,
			[]string{"doctor.go:9: saver method Stop"}},
		{"the host's ensure as a method expression", `import "github.com/leeovery/portal/internal/saverhost"

var ensurer = (*saverhost.Host).Ensurer`,
			[]string{"doctor.go:5: saver method Ensurer"}},
		{"a signal sent", `import "syscall"

var signalProcess = syscall.Kill

func checkDaemon() { _ = signalProcess(1, syscall.SIGTERM) }`,
			[]string{"doctor.go:5: signalProcess", "doctor.go:5: syscall.Kill", "doctor.go:7: signalProcess"}},
		{"a daemon ended", `import (
	"context"
	"syscall"

	"github.com/leeovery/portal/internal/backend/zmx"
)

func endIt(ctx context.Context, b *zmx.Backend, signal func(int, syscall.Signal) error) {
	_ = b.EndDaemon(ctx, "x", signal)
}`,
			[]string{"doctor.go:11: EndDaemon"}},
		{"the full bootstrap run", `import "context"

func owedBootstrap(context.Context) (any, error) { return nil, nil }

func bootstrapFor() error { return nil }

func runBootstrap() error { return nil }

func checkIt(ctx context.Context) { _, _ = owedBootstrap(ctx); _ = bootstrapFor(); _ = runBootstrap() }`,
			[]string{
				"doctor.go:5: owedBootstrap", "doctor.go:7: bootstrapFor", "doctor.go:9: runBootstrap",
				"doctor.go:11: owedBootstrap", "doctor.go:11: bootstrapFor", "doctor.go:11: runBootstrap",
			}},
		{"a raw signal through syscall or unix", `import (
	"syscall"

	"golang.org/x/sys/unix"
)

func killIt() { _ = syscall.Kill(1, syscall.SIGTERM); _ = unix.Kill(1, unix.SIGTERM) }`,
			[]string{"doctor.go:9: syscall.Kill", "doctor.go:9: golang.org/x/sys/unix.Kill"}},
		{"a raw signal through os.Process", `import (
	"os"
	"syscall"
)

func killIt(p *os.Process) { _ = p.Signal(syscall.SIGTERM); _ = p.Kill() }`,
			[]string{"doctor.go:8: (*os.Process).Signal", "doctor.go:8: (*os.Process).Kill"}},
		{"the reads, through the host and doctorSaver", `import (
	"context"

	"github.com/leeovery/portal/internal/saverhost"
)

type doctorSaver interface {
	Present(ctx context.Context) (bool, error)
	SaverPID(ctx context.Context) (int, bool, error)
	Prove(pid int) error
}

var _ doctorSaver = (*saverhost.Host)(nil)

func checkSaver(ctx context.Context, s doctorSaver, h *saverhost.Host) {
	_, _ = s.Present(ctx)
	_, _, _ = s.SaverPID(ctx)
	_ = s.Prove(1)
	_, _ = h.Present(ctx)
}`,
			nil},
	} {
		t.Run("it judges "+tc.name, func(t *testing.T) {
			src := "package cmd\n\n" + tc.body + "\n"
			if got := doctorSaverReaches(checker.Sources(t, map[string]string{"doctor.go": src})); !slices.Equal(got, tc.want) {
				t.Errorf("doctorSaverReaches = %q, want %q", got, tc.want)
			}
		})
	}
}
