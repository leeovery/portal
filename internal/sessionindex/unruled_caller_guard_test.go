package sessionindex_test

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

const (
	modulePath          = "github.com/leeovery/portal"
	sessionindexPkgPath = modulePath + "/internal/sessionindex"
)

// unruledCaller is a place a change the claim rule never judges may be made
// from: a package-level function of a package, or anywhere in the package when
// fn is empty.
type unruledCaller struct{ pkg, fn string }

// Remove and Restored change an entry with no claim rule consulted: Remove
// drops one, and Restored adopts whatever creation an entry records. Anywhere
// else, either would stand for a replace the rule never judged. Remove's
// callers are kill and the saver's drop of a session that ended. Restored's
// is restore.
var unruledCallers = map[string][]unruledCaller{
	"Remove": {
		{pkg: modulePath + "/cmd", fn: "forgetSession"},
		{pkg: modulePath + "/internal/saver", fn: "drop"},
	},
	"Restored": {{pkg: modulePath + "/internal/restore"}},
}

// guardLane is a build the guard scans.
type guardLane struct {
	name string
	opts []sourceguardtest.DepsOption
}

// guardLanes are the host's build and linux's, each with and without the
// integration tag, so no file behind a platform or a tag escapes the guard.
var guardLanes = []guardLane{
	{name: "the host build"},
	{name: "the host build with the integration tag", opts: []sourceguardtest.DepsOption{sourceguardtest.WithBuildTags(sourceguardtest.IntegrationTag)}},
	{name: "linux", opts: []sourceguardtest.DepsOption{sourceguardtest.ForGOOS("linux")}},
	{name: "linux with the integration tag", opts: []sourceguardtest.DepsOption{sourceguardtest.ForGOOS("linux"), sourceguardtest.WithBuildTags(sourceguardtest.IntegrationTag)}},
}

func TestUnruledChanges_ComeOnlyFromTheirNamedCallers(t *testing.T) {
	methods := unruledMethods(t)
	for _, lane := range guardLanes {
		t.Run(lane.name, func(t *testing.T) {
			var scanned []scannedPackage
			for _, listed := range sessionindexDependents(t, lane) {
				typed := sourceguardtest.NewTypeChecker(t, listed.Dir, lane.opts...).Package(t)
				scanned = append(scanned, scannedPackage{path: listed.ImportPath, typed: typed})
			}
			runUnruledCallerGuard(t, methods, scanned)
		})
	}
}

func TestUnruledCallerGuard_Lanes(t *testing.T) {
	var names []string
	for _, lane := range guardLanes {
		names = append(names, lane.name)
	}
	want := []string{"the host build", "the host build with the integration tag", "linux", "linux with the integration tag"}
	if !slices.Equal(names, want) {
		t.Errorf("lanes = %v, want %v", names, want)
	}
}

func TestUnruledCallerGuard_Rule(t *testing.T) {
	methods := unruledMethods(t)
	checker := sourceguardtest.NewTypeChecker(t, filepath.Join(sourceguardtest.ProjectRoot(t), "internal", "state"))
	const reaches = `package fixture

import "github.com/leeovery/portal/internal/sessionindex"

var drop = (*sessionindex.Index).Remove

func forgetSession(idx *sessionindex.Index) {
	func() { idx.Remove("a") }()
}

func elsewhere(idx *sessionindex.Index) {
	idx.Restored(sessionindex.Entry{Name: "a"}, at, 0)
	remove := idx.Remove
	_ = remove
}
`
	const embeds = `package fixture

import "github.com/leeovery/portal/internal/sessionindex"

type wrapped struct{ *sessionindex.Index }

func elsewhere(w wrapped) { w.Remove("a") }
`
	const viaInterface = `package fixture

import (
	"time"

	"github.com/leeovery/portal/internal/sessionindex"
)

type remover interface {
	Remove(name string) bool
	Other()
}

type restorer interface {
	Restored(sessionindex.Entry, time.Time, int) bool
}

func elsewhere(r remover, s restorer) {
	r.Remove("a")
	s.Restored(sessionindex.Entry{Name: "a"}, at, 0)
}
`
	const viaConstraint = `package fixture

import (
	"time"

	"github.com/leeovery/portal/internal/sessionindex"
)

func restore[T interface {
	Restored(sessionindex.Entry, time.Time, int) bool
}](x T) {
	x.Restored(sessionindex.Entry{Name: "a"}, at, 0)
}
`
	const savesDrop = `package fixture

import "github.com/leeovery/portal/internal/sessionindex"

func drop(idx *sessionindex.Index, names []string) {
	for _, name := range names {
		idx.Remove(name)
	}
}

func claim(idx *sessionindex.Index) {
	idx.Remove("a")
}
`
	const methodNamedAsAllowed = `package fixture

import "github.com/leeovery/portal/internal/sessionindex"

type killer struct{}

func (killer) forgetSession(idx *sessionindex.Index) { idx.Remove("a") }
`
	const unrelated = `package fixture

import "os"

type store struct{}

func (store) Remove(string) bool { return false }
func (store) Restored(string) bool { return false }

type remover interface{ Remove(string) error }

func elsewhere(s store, r remover) {
	_ = os.Remove("a")
	s.Remove("a")
	s.Restored("a")
	_ = r.Remove("a")
}
`
	for _, tc := range []struct {
		name, pkg, src string
		want           []string
	}{
		{
			name: "it reports a reach for either from a package not named, however it is spelled",
			pkg:  modulePath + "/internal/capture", src: reaches,
			want: []string{
				"fixture.go:5: Remove at package level",
				"fixture.go:8: Remove in forgetSession",
				"fixture.go:12: Restored in elsewhere",
				"fixture.go:13: Remove in elsewhere",
			},
		},
		{
			name: "it allows Remove from kill's forgetSession alone in cmd",
			pkg:  modulePath + "/cmd", src: reaches,
			want: []string{
				"fixture.go:5: Remove at package level",
				"fixture.go:12: Restored in elsewhere",
				"fixture.go:13: Remove in elsewhere",
			},
		},
		{
			name: "it allows Remove from the saver's drop alone in internal/saver",
			pkg:  modulePath + "/internal/saver", src: savesDrop,
			want: []string{"fixture.go:12: Remove in claim"},
		},
		{
			name: "it allows a method named forgetSession nothing, only the package-level func",
			pkg:  modulePath + "/cmd", src: methodNamedAsAllowed,
			want: []string{"fixture.go:7: Remove in (killer).forgetSession"},
		},
		{
			name: "it allows internal/state neither",
			pkg:  modulePath + "/internal/state", src: reaches,
			want: []string{
				"fixture.go:5: Remove at package level",
				"fixture.go:8: Remove in forgetSession",
				"fixture.go:12: Restored in elsewhere",
				"fixture.go:13: Remove in elsewhere",
			},
		},
		{
			name: "it allows Restored anywhere in internal/restore",
			pkg:  modulePath + "/internal/restore", src: reaches,
			want: []string{
				"fixture.go:5: Remove at package level",
				"fixture.go:8: Remove in forgetSession",
				"fixture.go:13: Remove in elsewhere",
			},
		},
		{
			name: "it reports a Remove promoted through an embedded index",
			pkg:  modulePath + "/cmd", src: embeds,
			want: []string{"fixture.go:7: Remove in elsewhere"},
		},
		{
			name: "it reports a call through an interface method an Index satisfies by name and signature",
			pkg:  modulePath + "/cmd", src: viaInterface,
			want: []string{
				"fixture.go:19: Remove through an interface in elsewhere",
				"fixture.go:20: Restored through an interface in elsewhere",
			},
		},
		{
			name: "it reports a call through a generic constraint",
			pkg:  modulePath + "/cmd", src: viaConstraint,
			want: []string{"fixture.go:12: Restored through an interface in restore"},
		},
		{
			name: "it allows an interface method through restore's package, as the method itself",
			pkg:  modulePath + "/internal/restore", src: viaConstraint,
		},
		{
			name: "it ignores methods of the same names on other types, and interface methods of other signatures",
			pkg:  modulePath + "/cmd", src: unrelated,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			typed := checker.Sources(t, map[string]string{"fixture.go": tc.src, "at.go": "package fixture\n\nimport \"time\"\n\nvar at time.Time\n"})

			if got := unruledReaches(methods, scannedPackage{path: tc.pkg, typed: typed}); !slices.Equal(got, tc.want) {
				t.Errorf("reaches = %v\nwant %v", got, tc.want)
			}
		})
	}
}

// A guard that scanned no package would report a clean tree forever.
func TestUnruledCallerGuard_FatalsWhenItScansNothing(t *testing.T) {
	recorder := &harnesstest.Recorder{}
	recorder.Run(func() { runUnruledCallerGuard(recorder, unruledMethods(t), nil) })

	if len(recorder.Fatals) != 1 {
		t.Fatalf("a scan of no packages reported %d fatals, want 1: %v", len(recorder.Fatals), recorder.Fatals)
	}
}

type scannedPackage struct {
	path  string
	typed sourceguardtest.TypedPackage
}

func runUnruledCallerGuard(t harnesstest.TestingT, methods map[string]string, scanned []scannedPackage) {
	t.Helper()
	if len(scanned) == 0 {
		t.Fatalf("no package depending on %s was scanned, so the guard would pass by having stopped looking", sessionindexPkgPath)
		return
	}
	for _, pkg := range scanned {
		for _, reach := range unruledReaches(methods, pkg) {
			t.Errorf("%s: %s; only its named callers may change an entry the claim rule never judges", pkg.path, reach)
		}
	}
}

// unruledMethods gives each Index method unruledCallers bounds its signature's
// shape, read from the package's own declaration of it.
func unruledMethods(t harnesstest.TestingT) map[string]string {
	t.Helper()
	pkg := sourceguardtest.NewTypeChecker(t, filepath.Join(sourceguardtest.ProjectRoot(t), "internal", "sessionindex")).Package(t)
	shapes := map[string]string{}
	for _, source := range pkg.Sources {
		for _, decl := range source.File.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Recv == nil || unruledCallers[fd.Name.Name] == nil || receiverName(fd) != "*Index" {
				continue
			}
			if fn, ok := pkg.Object(fd.Name).(*types.Func); ok {
				shapes[fd.Name.Name] = shape(fn.Signature(), fn.Pkg())
			}
		}
	}
	if len(shapes) != len(unruledCallers) {
		t.Fatalf("found the Index methods %v, want every one of %d the guard bounds", shapes, len(unruledCallers))
	}
	return shapes
}

// unruledReaches reports each "<file>:<line>: <method> in <func>" where pkg's
// sources reach (*sessionindex.Index).Remove or Restored from outside the
// callers named for it: called, taken as a value or a method expression,
// promoted through an embedded index, or reached through an interface or a
// constraint whose method of the name has the Index method's signature. Each
// is judged by the object an identifier uses rather than by how the source
// spelled it.
func unruledReaches(methods map[string]string, pkg scannedPackage) []string {
	var found []string
	for _, source := range pkg.typed.Sources {
		for _, decl := range source.File.Decls {
			fn := funcName(decl)
			ast.Inspect(decl, func(n ast.Node) bool {
				ident, ok := n.(*ast.Ident)
				if !ok {
					return true
				}
				method, how := unruledMethod(methods, pkg.typed.Info.Uses[ident])
				if method == "" || callerAllowed(method, pkg.path, fn) {
					return true
				}
				where := "at package level"
				if fn != "" {
					where = "in " + fn
				}
				found = append(found, fmt.Sprintf("%s:%d: %s%s %s", filepath.Base(source.Path), source.Position(ident.Pos()).Line, method, how, where))
				return true
			})
		}
	}
	return found
}

// funcName names the function decl declares, a method by its receiver, so no
// method answers to a package-level function's name; empty for any other
// declaration.
func funcName(decl ast.Decl) string {
	fd, ok := decl.(*ast.FuncDecl)
	switch {
	case !ok:
		return ""
	case fd.Recv == nil:
		return fd.Name.Name
	}
	return "(" + receiverName(fd) + ")." + fd.Name.Name
}

func receiverName(fd *ast.FuncDecl) string {
	if fd.Recv == nil || len(fd.Recv.List) == 0 {
		return ""
	}
	return types.ExprString(fd.Recv.List[0].Type)
}

// unruledMethod names the Index method of methods obj is or stands for, with
// how it does, empty for any other object.
func unruledMethod(methods map[string]string, obj types.Object) (method, how string) {
	fn, ok := obj.(*types.Func)
	if !ok {
		return "", ""
	}
	if _, bounded := methods[fn.Name()]; bounded && fn.FullName() == "(*"+sessionindexPkgPath+".Index)."+fn.Name() {
		return fn.Name(), ""
	}
	if want, bounded := methods[fn.Name()]; bounded && isInterfaceMethod(fn) && shape(fn.Signature(), nil) == want {
		return fn.Name(), " through an interface"
	}
	return "", ""
}

func isInterfaceMethod(fn *types.Func) bool {
	recv := fn.Signature().Recv()
	return recv != nil && types.IsInterface(recv.Type())
}

// shape renders sig's parameter and result types, packages by import path and
// without names, so a method and an interface's can be compared across
// type-checks. A type-check of sessionindex's own sources knows it by its name
// alone, so own, when given, is that package, rendered by its import path.
func shape(sig *types.Signature, own *types.Package) string {
	qualifier := func(p *types.Package) string {
		if p == own {
			return sessionindexPkgPath
		}
		return p.Path()
	}
	render := func(tuple *types.Tuple) string {
		var rendered []string
		for v := range tuple.Variables() {
			rendered = append(rendered, types.TypeString(v.Type(), qualifier))
		}
		return "(" + strings.Join(rendered, ", ") + ")"
	}
	variadic := ""
	if sig.Variadic() {
		variadic = "..."
	}
	return render(sig.Params()) + variadic + render(sig.Results())
}

func callerAllowed(method, pkg, fn string) bool {
	return slices.ContainsFunc(unruledCallers[method], func(c unruledCaller) bool {
		return c.pkg == pkg && (c.fn == "" || c.fn == fn)
	})
}

// sessionindexDependents lists every package of the module whose build in
// lane depends on sessionindex, directly or not: a method is reached through
// any value of the type, imported or handed over.
func sessionindexDependents(t *testing.T, lane guardLane) []sourceguardtest.ListedPackage {
	t.Helper()
	listed := sourceguardtest.ModulePackages(t, sourceguardtest.ProjectRoot(t), lane.opts...)
	return slices.DeleteFunc(listed, func(pkg sourceguardtest.ListedPackage) bool {
		return !slices.Contains(pkg.Deps, sessionindexPkgPath)
	})
}
