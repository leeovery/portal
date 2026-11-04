package cmd

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/sourceguardtest"
)

// The \bCleanStale\b word boundary is deliberate: the marker-sweep method
// CleanStaleMarkers carries a trailing word char, so only the bare hooks
// method matches. The cycle itself now lives in internal/hooksweep, so the
// package name is what names it here: a bootstrap step has no business reaching
// that package at all, whichever of its entry points it reached for. Only
// production files are scanned — test files legitimately mention the name in
// prose.
func TestHooksCleanStale_NoBootstrapStepIsAnAutomaticCaller(t *testing.T) {
	forbidden := regexp.MustCompile(`\bCleanStale\b|\bhooksweep\b`)

	const bootstrapDir = "bootstrap"
	entries, err := os.ReadDir(bootstrapDir)
	if err != nil {
		t.Fatalf("read %s: %v", bootstrapDir, err)
	}

	scanned := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(bootstrapDir, name)
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		scanned++
		if loc := forbidden.FindIndex(src); loc != nil {
			line := 1 + strings.Count(string(src[:loc[0]]), "\n")
			t.Errorf("%s:%d references the hooks CleanStale path; only the daemon may sweep hooks "+
				"— a bootstrap step must not clean hooks",
				path, line)
		}
	}

	// The directory must actually contain production source, else a layout
	// change would leave this guard scanning nothing.
	if scanned == 0 {
		t.Fatalf("no production .go files scanned under %q; guard is vacuous", bootstrapDir)
	}
}

const (
	hooksPkgPath     = "github.com/leeovery/portal/internal/hooks"
	hooksweepPkgPath = "github.com/leeovery/portal/internal/hooksweep"
	modulePath       = "github.com/leeovery/portal"
)

// hookSweepCallers are the production sources, by module path, that may reach
// the hook sweep: the daemon, whose idle tick runs it, and doctor, whose
// diagnosis counts what it would remove and whose --fix runs it.
var hookSweepCallers = []string{"cmd/state_daemon.go", "cmd/doctor.go"}

// hookSweepOwners are the packages the guard does not judge: hooksweep, which
// is the sweep, and hooks, which declares the rule and the clean it runs.
var hookSweepOwners = []string{"internal/hooks", "internal/hooksweep"}

// judgedBySweepGuard reports whether the production source at the module path
// file is one the guard judges: neither a caller nor in an owning package.
func judgedBySweepGuard(file string) bool {
	return !slices.Contains(hookSweepCallers, file) && !slices.Contains(hookSweepOwners, path.Dir(file))
}

// hookSweepImports reports each "<file>:<line>: <import>" a judged source
// makes of the sweep's package.
func hookSweepImports(sources []sourceguardtest.ParsedSource) []string {
	var found []string
	for _, source := range sources {
		file := filepath.ToSlash(source.Path)
		if !judgedBySweepGuard(file) {
			continue
		}
		for _, spec := range source.File.Imports {
			if p, err := strconv.Unquote(spec.Path.Value); err == nil && p == hooksweepPkgPath {
				found = append(found, fmt.Sprintf("%s:%d: %s", file, source.Position(spec.Pos()).Line, hooksweepPkgPath))
			}
		}
	}
	return found
}

// hookSweepReferences reports each "<file>:<line>: <reach>" a judged source
// in pkg makes for the hook sweep: hooks.StaleKeys, the
// (*hooks.Store).CleanStale method in any form (a call, a method value or
// expression, or promoted through an embedded store), or an interface's
// CleanStale taking a func(hooks.Snapshot), the shape of a seam over it. Each
// is judged by the object an identifier names, not by how the source spelled
// it. Removing a hook loses the user's resume command, and a hook goes with
// its session's entry; the sweep is the backstop for one left behind, and
// nothing else may judge a key stale. Reflection, and a caller's helper
// called from another file, are outside its bound, accepted.
func hookSweepReferences(pkg sourceguardtest.TypedPackage) []string {
	var found []string
	for _, source := range pkg.Sources {
		file := filepath.ToSlash(source.Path)
		if !judgedBySweepGuard(file) {
			continue
		}
		ast.Inspect(source.File, func(n ast.Node) bool {
			if ident, ok := n.(*ast.Ident); ok {
				if reach := hookSweepReach(pkg.Object(ident)); reach != "" {
					found = append(found, fmt.Sprintf("%s:%d: %s", file, source.Position(ident.Pos()).Line, reach))
				}
			}
			return true
		})
	}
	return found
}

// hookSweepReach names what obj reaches of the hook sweep, empty for nothing.
func hookSweepReach(obj types.Object) string {
	fn, ok := obj.(*types.Func)
	switch {
	case !ok:
		return ""
	case fn.FullName() == hooksPkgPath+".StaleKeys":
		return "hooks.StaleKeys"
	case fn.FullName() == "(*"+hooksPkgPath+".Store).CleanStale":
		return "(*hooks.Store).CleanStale"
	case fn.Name() == "CleanStale" && isInterfaceMethod(fn) && takesSnapshotFunc(fn.Signature()):
		return "a CleanStale seam taking func(hooks.Snapshot)"
	}
	return ""
}

func isInterfaceMethod(fn *types.Func) bool {
	recv := fn.Signature().Recv()
	return recv != nil && types.IsInterface(recv.Type())
}

// takesSnapshotFunc reports whether sig has a parameter that is a func taking
// a hooks.Snapshot first, as (*hooks.Store).CleanStale's enumerator is.
func takesSnapshotFunc(sig *types.Signature) bool {
	for param := range sig.Params().Variables() {
		fn, ok := param.Type().Underlying().(*types.Signature)
		if !ok || fn.Params().Len() == 0 {
			continue
		}
		if named, ok := fn.Params().At(0).Type().(*types.Named); ok && named.Obj().Pkg() != nil &&
			named.Obj().Pkg().Path() == hooksPkgPath && named.Obj().Name() == "Snapshot" {
			return true
		}
	}
	return false
}

// hooksImporters returns the module path of every package whose production
// sources import internal/hooks: only such a package can name the store's
// clean or the rule, so only they are type-checked.
func hooksImporters(t *testing.T) []string {
	t.Helper()
	out, err := exec.Command("go", "list", "-f", "{{.ImportPath}}\t{{join .Imports \" \"}}", modulePath+"/...").Output()
	if err != nil {
		t.Fatalf("go list the module's packages: %v", err)
	}
	var pkgs []string
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		importPath, imports, _ := strings.Cut(line, "\t")
		if slices.Contains(strings.Fields(imports), hooksPkgPath) {
			pkgs = append(pkgs, strings.TrimPrefix(strings.TrimPrefix(importPath, modulePath), "/"))
		}
	}
	return pkgs
}

// modulePackage type-checks the package at the module path dir, its sources
// named by module path.
func modulePackage(t *testing.T, root, dir string) sourceguardtest.TypedPackage {
	t.Helper()
	pkg := sourceguardtest.NewTypeChecker(t, filepath.Join(root, filepath.FromSlash(dir))).Package(t)
	for i, source := range pkg.Sources {
		rel, err := filepath.Rel(root, source.Path)
		if err != nil {
			t.Fatalf("relativise %s: %v", source.Path, err)
		}
		pkg.Sources[i].Path = filepath.ToSlash(rel)
	}
	return pkg
}

func TestOnlyTheDaemonAndDoctorReachTheHookSweep(t *testing.T) {
	root, sources := sourceguardtest.RepoSources(t, sourceguardtest.NonTestSources)

	t.Run("it finds no import of the sweep outside the daemon and doctor", func(t *testing.T) {
		for _, found := range hookSweepImports(sources) {
			t.Errorf("%s; only the daemon and doctor may reach the hook sweep", found)
		}
	})

	t.Run("it finds no reach for the sweep outside the daemon and doctor", func(t *testing.T) {
		importers := hooksImporters(t)
		if !slices.Contains(importers, "cmd") {
			t.Fatalf("the packages importing %s are %v, without cmd; the guard is not looking where the callers are", hooksPkgPath, importers)
		}
		for _, dir := range importers {
			if slices.Contains(hookSweepOwners, dir) {
				continue
			}
			for _, found := range hookSweepReferences(modulePackage(t, root, dir)) {
				t.Errorf("%s; only the daemon and doctor may judge a hook key stale", found)
			}
		}
	})

	// The guard is only as good as its callers' reach: were either to stop
	// reaching the sweep, an allowance would stand for a caller that no longer
	// exists.
	t.Run("it finds both callers importing the sweep", func(t *testing.T) {
		for _, caller := range hookSweepCallers {
			if !slices.ContainsFunc(sources, func(s sourceguardtest.ParsedSource) bool {
				return filepath.ToSlash(s.Path) == caller && slices.ContainsFunc(s.File.Imports, func(spec *ast.ImportSpec) bool {
					return spec.Path.Value == strconv.Quote(hooksweepPkgPath)
				})
			}) {
				t.Errorf("%s no longer imports %s; drop it from hookSweepCallers", caller, hooksweepPkgPath)
			}
		}
	})

	t.Run("it judges an import of the sweep", func(t *testing.T) {
		const src = "package x\n\nimport _ \"" + hooksweepPkgPath + "\"\n"
		for file, want := range map[string][]string{
			"cmd/hooks.go":              {"cmd/hooks.go:3: " + hooksweepPkgPath},
			"cmd/state_resume_draw.go":  {"cmd/state_resume_draw.go:3: " + hooksweepPkgPath},
			"internal/saver/saver.go":   {"internal/saver/saver.go:3: " + hooksweepPkgPath},
			"cmd/bootstrap/steps.go":    {"cmd/bootstrap/steps.go:3: " + hooksweepPkgPath},
			"cmd/state_daemon.go":       nil,
			"cmd/doctor.go":             nil,
			"internal/hooksweep/run.go": nil,
		} {
			fset := token.NewFileSet()
			parsed, err := parser.ParseFile(fset, file, src, sourceguardtest.ParseMode)
			if err != nil {
				t.Fatalf("parse fixture: %v", err)
			}
			got := hookSweepImports([]sourceguardtest.ParsedSource{{Path: file, Fset: fset, File: parsed}})
			if !slices.Equal(got, want) {
				t.Errorf("hookSweepImports(%s) = %q, want %q", file, got, want)
			}
		}
	})

	checker := sourceguardtest.NewTypeChecker(t, ".")
	for _, tc := range []struct {
		name, body string
		want       []string
	}{
		{"StaleKeys under an alias", `import h "github.com/leeovery/portal/internal/hooks"

var stale = h.StaleKeys`,
			[]string{"%s:5: hooks.StaleKeys"}},
		{"a call", `import "github.com/leeovery/portal/internal/hooks"

func prune(s *hooks.Store) { _, _ = s.CleanStale(nil, nil) }`,
			[]string{"%s:5: (*hooks.Store).CleanStale"}},
		{"a method value", `import "github.com/leeovery/portal/internal/hooks"

func prune(s *hooks.Store) { clean := s.CleanStale; _ = clean }`,
			[]string{"%s:5: (*hooks.Store).CleanStale"}},
		{"a method expression", `import "github.com/leeovery/portal/internal/hooks"

var clean = (*hooks.Store).CleanStale`,
			[]string{"%s:5: (*hooks.Store).CleanStale"}},
		{"a method promoted through an embedded store", `import "github.com/leeovery/portal/internal/hooks"

type store struct{ *hooks.Store }

func prune(s store) { _, _ = s.CleanStale(nil, nil) }`,
			[]string{"%s:7: (*hooks.Store).CleanStale"}},
		{"a seam typed as an interface", `import "github.com/leeovery/portal/internal/hooks"

type cleaner interface {
	CleanStale(func(hooks.Snapshot) ([]string, error), func() ([]string, error)) ([]string, error)
}

func prune(c cleaner) { _, _ = c.CleanStale(nil, nil) }`,
			[]string{"%s:6: a CleanStale seam taking func(hooks.Snapshot)", "%s:9: a CleanStale seam taking func(hooks.Snapshot)"}},
		{"a project store's CleanStale", `import "github.com/leeovery/portal/internal/project"

type cleaner interface{ CleanStale() ([]project.Project, error) }

func prune(s *project.Store, c cleaner) { _, _ = s.CleanStale(); _, _ = c.CleanStale() }`,
			nil},
	} {
		src := "package cmd\n\n" + tc.body + "\n"
		for _, file := range []string{"cmd/hooks.go", "internal/saver/saver.go"} {
			t.Run("it judges "+tc.name+" in "+file, func(t *testing.T) {
				var want []string
				for _, w := range tc.want {
					want = append(want, fmt.Sprintf(w, file))
				}
				if got := hookSweepReferences(checker.Sources(t, map[string]string{file: src})); !slices.Equal(got, want) {
					t.Errorf("hookSweepReferences = %q, want %q", got, want)
				}
			})
		}
		for _, file := range append(slices.Clone(hookSweepCallers), "internal/hooksweep/sweep.go", "internal/hooks/store.go") {
			t.Run("it lets "+file+" reach "+tc.name, func(t *testing.T) {
				if got := hookSweepReferences(checker.Sources(t, map[string]string{file: src})); len(got) != 0 {
					t.Errorf("hookSweepReferences = %q, want none for %s", got, file)
				}
			})
		}
	}
}
