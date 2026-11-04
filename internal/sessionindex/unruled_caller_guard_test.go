package sessionindex_test

import (
	"fmt"
	"go/ast"
	"go/types"
	"os/exec"
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
// from: a function of a package, or anywhere in it when fn is empty.
type unruledCaller struct{ pkg, fn string }

// Remove and Restored change an entry with no claim rule consulted: Remove
// drops one, and Restored adopts whatever creation an entry records. Anywhere
// else, either would stand for a replace the rule never judged. Remove's
// callers are kill and the saver's drop of a session that ended, which lives
// beside the commit cycle in internal/state; Restored's is restore.
var unruledCallers = map[string][]unruledCaller{
	"Remove":   {{pkg: modulePath + "/cmd", fn: "forgetSession"}, {pkg: modulePath + "/internal/state"}},
	"Restored": {{pkg: modulePath + "/internal/restore"}},
}

func TestUnruledChanges_ComeOnlyFromTheirNamedCallers(t *testing.T) {
	var scanned []scannedPackage
	for _, listed := range sessionindexDependents(t) {
		scanned = append(scanned, scannedPackage{path: listed.path, typed: sourceguardtest.NewTypeChecker(t, listed.dir).Package(t)})
	}
	runUnruledCallerGuard(t, scanned)
}

func TestUnruledCallerGuard_Rule(t *testing.T) {
	checker := sourceguardtest.NewTypeChecker(t, filepath.Join(sourceguardtest.ProjectRoot(t), "internal", "state"))
	const reaches = `package fixture

import "github.com/leeovery/portal/internal/sessionindex"

var drop = (*sessionindex.Index).Remove

func forgetSession(idx *sessionindex.Index) {
	func() { idx.Remove("a") }()
}

func elsewhere(idx *sessionindex.Index) {
	idx.Restored("a", at, 0)
	remove := idx.Remove
	_ = remove
}
`
	const embeds = `package fixture

import "github.com/leeovery/portal/internal/sessionindex"

type wrapped struct{ *sessionindex.Index }

func elsewhere(w wrapped) { w.Remove("a") }
`
	const unrelated = `package fixture

import "os"

type store struct{}

func (store) Remove(string) bool { return false }
func (store) Restored(string) bool { return false }

func elsewhere(s store) {
	_ = os.Remove("a")
	s.Remove("a")
	s.Restored("a")
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
			name: "it allows Remove anywhere in internal/state, and Restored nowhere there",
			pkg:  modulePath + "/internal/state", src: reaches,
			want: []string{"fixture.go:12: Restored in elsewhere"},
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
			name: "it ignores methods of the same names on other types",
			pkg:  modulePath + "/cmd", src: unrelated,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			typed := checker.Sources(t, map[string]string{"fixture.go": tc.src, "at.go": "package fixture\n\nimport \"time\"\n\nvar at time.Time\n"})

			if got := unruledReaches(scannedPackage{path: tc.pkg, typed: typed}); !slices.Equal(got, tc.want) {
				t.Errorf("reaches = %v\nwant %v", got, tc.want)
			}
		})
	}
}

// A guard that scanned no package would report a clean tree forever.
func TestUnruledCallerGuard_FatalsWhenItScansNothing(t *testing.T) {
	recorder := &harnesstest.Recorder{}
	recorder.Run(func() { runUnruledCallerGuard(recorder, nil) })

	if len(recorder.Fatals) != 1 {
		t.Fatalf("a scan of no packages reported %d fatals, want 1: %v", len(recorder.Fatals), recorder.Fatals)
	}
}

type scannedPackage struct {
	path  string
	typed sourceguardtest.TypedPackage
}

func runUnruledCallerGuard(t harnesstest.TestingT, scanned []scannedPackage) {
	t.Helper()
	if len(scanned) == 0 {
		t.Fatalf("no package depending on %s was scanned, so the guard would pass by having stopped looking", sessionindexPkgPath)
		return
	}
	for _, pkg := range scanned {
		for _, reach := range unruledReaches(pkg) {
			t.Errorf("%s: %s; only its named callers may change an entry the claim rule never judges", pkg.path, reach)
		}
	}
}

// unruledReaches reports each "<file>:<line>: <method> in <func>" where pkg's
// sources reach (*sessionindex.Index).Remove or Restored from outside the
// callers named for it: called, taken as a value or a method expression, or
// promoted through an embedded index, each judged by the object an identifier
// names rather than by how the source spelled it.
func unruledReaches(pkg scannedPackage) []string {
	var found []string
	for _, source := range pkg.typed.Sources {
		for _, decl := range source.File.Decls {
			fn := ""
			if fd, ok := decl.(*ast.FuncDecl); ok {
				fn = fd.Name.Name
			}
			ast.Inspect(decl, func(n ast.Node) bool {
				ident, ok := n.(*ast.Ident)
				if !ok {
					return true
				}
				method := unruledMethod(pkg.typed.Object(ident))
				if method == "" || callerAllowed(method, pkg.path, fn) {
					return true
				}
				where := "at package level"
				if fn != "" {
					where = "in " + fn
				}
				found = append(found, fmt.Sprintf("%s:%d: %s %s", filepath.Base(source.Path), source.Position(ident.Pos()).Line, method, where))
				return true
			})
		}
	}
	return found
}

// unruledMethod names the Index method obj is, of those unruledCallers bounds,
// empty for any other object.
func unruledMethod(obj types.Object) string {
	fn, ok := obj.(*types.Func)
	if !ok {
		return ""
	}
	for method := range unruledCallers {
		if fn.FullName() == "(*"+sessionindexPkgPath+".Index)."+method {
			return method
		}
	}
	return ""
}

func callerAllowed(method, pkg, fn string) bool {
	return slices.ContainsFunc(unruledCallers[method], func(c unruledCaller) bool {
		return c.pkg == pkg && (c.fn == "" || c.fn == fn)
	})
}

type listedPackage struct{ path, dir string }

// sessionindexDependents lists every package of the module whose build
// depends on sessionindex, directly or not: a method is reached through any
// value of the type, imported or handed over.
func sessionindexDependents(t *testing.T) []listedPackage {
	t.Helper()
	cmd := exec.Command("go", "list", "-f", "{{.ImportPath}}\t{{.Dir}}\t{{join .Deps \" \"}}", "./...")
	cmd.Dir = sourceguardtest.ProjectRoot(t)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list the module's packages: %v", err)
	}
	var dependents []listedPackage
	for line := range strings.Lines(string(out)) {
		fields := strings.SplitN(strings.TrimSuffix(line, "\n"), "\t", 3)
		if len(fields) == 3 && slices.Contains(strings.Fields(fields[2]), sessionindexPkgPath) {
			dependents = append(dependents, listedPackage{path: fields[0], dir: fields[1]})
		}
	}
	return dependents
}
