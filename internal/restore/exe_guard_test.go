package restore_test

import (
	"go/ast"
	"go/token"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/sourceguardtest"
)

func TestNoTestRestoresWithItsOwnBinary(t *testing.T) {
	for _, finding := range scanExeGuard(t) {
		t.Errorf("%s: a restored session would run the test binary as the whole suite; build the exe with portalbintest.Build, and construct restorers in production only in cmd", finding)
	}
}

func TestExeGuard_Rule(t *testing.T) {
	const constructs = `package a_test

import "github.com/leeovery/portal/internal/restore"

func build(exe string) { _, _ = restore.New(restore.Config{Exe: exe}) }
`
	const selfExe = `package a_test

import "os"

func selfExe() string {
	exe, _ := os.Executable()
	return exe
}
`
	const constructsAliased = `package b_test

import (
	"os"

	r "github.com/leeovery/portal/internal/restore"
)

func build() {
	exe, _ := os.Executable()
	_, _ = r.New(r.Config{Exe: exe})
}
`
	const dotImports = `package c_test

import (
	"os"

	. "github.com/leeovery/portal/internal/restore"
)

var self = os.Executable

func build(exe string) { _, _ = New(Config{Exe: exe}) }
`
	const insideRestore = `package restore

import "os"

func build() {
	exe, _ := os.Executable()
	_, _ = New(Config{Exe: exe})
}
`
	const unrelated = `package d_test

import (
	"errors"
	"os"
)

func run() error {
	if _, err := os.Executable(); err != nil {
		return errors.New("no exe")
	}
	return nil
}
`
	const wires = `package wire

import "github.com/leeovery/portal/internal/restore"

var build = restore.New
`
	const argsZero = `package a_test

import "os"

func exe() string { return os.Args[0] }
`
	const refusal = `package restore_test

import (
	"os"

	"github.com/leeovery/portal/internal/restore"
)

func TestNew_RefusesTheRunningTestBinary() {
	self, _ := os.Executable()
	_, _ = restore.New(restore.Config{Exe: os.Args[0]})
	_, _ = restore.New(restore.Config{Exe: self})
}

func elsewhere() { _, _ = restore.New(restore.Config{Exe: os.Args[0]}) }
`
	const helperHandsOver = `package portaltest

import "os"

func PortalExe() string {
	exe, _ := os.Executable()
	return exe
}
`
	const procCheck = `package zmxtest

import "os"

func checkProcessReader() bool { return len(os.Args) > 0 }

func argv0() string { return os.Args[0] }
`
	const productionSelf = `package spawn

import "os"

func self() (string, error) { return os.Executable() }
`
	const refusalNamedElsewhere = `package a_test

import "os"

func TestNew_RefusesTheRunningTestBinary() { _ = os.Args[0] }
`
	for _, tc := range []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{
			name:  "it reports os.Executable in any test file of a package whose tests construct a restorer",
			files: map[string]string{"internal/a/a_test.go": constructs, "internal/a/helpers_test.go": selfExe},
			want:  []string{"internal/a/helpers_test.go:6:12: reaches os.Executable in a package whose tests construct a restorer"},
		},
		{
			name:  "it reports os.Args in any test file of a package whose tests construct a restorer",
			files: map[string]string{"internal/a/a_test.go": constructs, "internal/a/args_test.go": argsZero},
			want:  []string{"internal/a/args_test.go:5:28: reaches os.Args in a package whose tests construct a restorer"},
		},
		{
			name:  "it allows the running binary in restore's own test of New's refusal alone",
			files: map[string]string{"internal/restore/refusal_test.go": refusal},
			want:  []string{"internal/restore/refusal_test.go:15:59: reaches os.Args in a package whose tests construct a restorer"},
		},
		{
			name:  "it allows nothing in a test of the refusal test's name outside restore",
			files: map[string]string{"internal/a/a_test.go": constructs, "internal/a/refusal_test.go": refusalNamedElsewhere},
			want:  []string{"internal/a/refusal_test.go:5:50: reaches os.Args in a package whose tests construct a restorer"},
		},
		{
			name:  "it reports a helper in a test-only package that hands the running binary over",
			files: map[string]string{"internal/a/a_test.go": constructs, "internal/portaltest/exe.go": helperHandsOver},
			want:  []string{"internal/portaltest/exe.go:6:12: reaches os.Executable in a test-only package"},
		},
		{
			name:  "it allows zmxtest's check of the process reader alone",
			files: map[string]string{"internal/a/a_test.go": constructs, "internal/zmxtest/proc.go": procCheck},
			want:  []string{"internal/zmxtest/proc.go:7:30: reaches os.Args in a test-only package"},
		},
		{
			name:  "it leaves the running binary to a production package",
			files: map[string]string{"internal/a/a_test.go": constructs, "internal/spawn/self.go": productionSelf},
		},
		{
			name:  "it reports one beside a construction under any import name",
			files: map[string]string{"internal/b/b_test.go": constructsAliased},
			want:  []string{"internal/b/b_test.go:10:12: reaches os.Executable in a package whose tests construct a restorer"},
		},
		{
			name:  "it reports one beside an unqualified construction through a dot import",
			files: map[string]string{"internal/c/c_test.go": dotImports},
			want:  []string{"internal/c/c_test.go:9:12: reaches os.Executable in a package whose tests construct a restorer"},
		},
		{
			name:  "it reports one beside an unqualified construction in restore's own tests",
			files: map[string]string{"internal/restore/x_test.go": insideRestore},
			want:  []string{"internal/restore/x_test.go:6:12: reaches os.Executable in a package whose tests construct a restorer"},
		},
		{
			name:  "it leaves os.Executable alone in a package whose tests construct none",
			files: map[string]string{"internal/a/a_test.go": constructs, "internal/d/d_test.go": unrelated},
		},
		{
			name:  "it allows a production construction in cmd",
			files: map[string]string{"internal/a/a_test.go": constructs, "cmd/wire.go": wires},
		},
		{
			name: "it reports a production construction anywhere else, test-only packages and cmd's own included",
			files: map[string]string{
				"internal/a/a_test.go":        constructs,
				"internal/portaltest/wire.go": wires,
				"cmd/bootstrap/wire.go":       wires,
			},
			want: []string{
				"cmd/bootstrap/wire.go:5:13: constructs a restorer outside cmd",
				"internal/portaltest/wire.go:5:13: constructs a restorer outside cmd",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := scanExeGuard(t, sourceguardtest.Rooted(writeTree(t, scannable(tc.files))))

			slices.Sort(got)
			if !slices.Equal(got, tc.want) {
				t.Errorf("findings = %q\nwant %q", got, tc.want)
			}
		})
	}
}

func TestExeGuard_FatalsWhenItScansNothing(t *testing.T) {
	const constructs = "package a_test\n\nimport \"github.com/leeovery/portal/internal/restore\"\n\nvar _, _ = restore.New(restore.Config{})\n"
	for name, files := range map[string]map[string]string{
		"an empty tree":                                 {},
		"a tree where no test constructs one":           scannable(map[string]string{"internal/a/a.go": "package a\n", "internal/a/a_test.go": "package a\n"}),
		"a tree where restore's production lacks New":   {"internal/restore/restore.go": "package restore\n\nfunc Old() {}\n", "internal/a/a_test.go": constructs},
		"a tree where only restore's tests declare New": {"internal/restore/new_test.go": newDeclared, "internal/a/a_test.go": constructs},
		"a tree with no test-only package's own source": {"internal/restore/restore.go": newDeclared, "internal/a/a_test.go": constructs, "internal/portaltest/x_test.go": "package portaltest\n"},
	} {
		t.Run(name, func(t *testing.T) {
			root := writeTree(t, files)
			recorder := &harnesstest.Recorder{}
			recorder.Run(func() { scanExeGuard(recorder, sourceguardtest.Rooted(root)) })

			if len(recorder.Fatals) != 1 {
				t.Errorf("fatals = %q, want one", recorder.Fatals)
			}
		})
	}
}

// newDeclared is restore's production source as far as the guard needs it:
// the declaration of New.
const newDeclared = "package restore\n\nfunc New(Config) (*Restorer, error) { return nil, nil }\n"

// scannable is files with what every scan must find beside them: restore's
// declaration of New, and a test-only package's own source.
func scannable(files map[string]string) map[string]string {
	tree := maps.Clone(files)
	tree["internal/restore/restore.go"] = newDeclared
	tree["internal/portaltest/portaltest.go"] = "package portaltest\n"
	return tree
}

// writeTree writes files, each a slash path to its source, under a fresh root.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, src := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// The successor of restoretest's literal guard. A restored session runs the
// exe its restorer was built with as `<exe> state hydrate …`, and under go test
// the running binary is the test binary: run with a leading state positional,
// it runs the whole suite again inside the session, with none of the test's
// isolation. New refuses an exe that isn't absolute or isn't named portal, and
// the guard keeps the running binary's path away from every construction it
// can see:
//
//   - In tests, no package whose tests construct a restorer reaches
//     os.Executable or os.Args in any of its test files, so no helper beside
//     the construction can hand either over.
//   - No test-only package reaches either in its own sources, so no helper
//     there can.
//   - In production, only cmd constructs one, where the shipped binary's own
//     path is the intent.
//
// The functions allowed name the exceptions.
const (
	modulePath     = "github.com/leeovery/portal"
	restorePath    = modulePath + "/internal/restore"
	restoreDir     = "internal/restore"
	newRestorer    = "New"
	productionHome = "cmd"
)

// runningBinary is each name of package os a test reaches the running
// binary's path through.
var runningBinary = []string{"Executable", "Args"}

// allowed is the one function of each directory that may reach the running
// binary, for a reason no restorer meets it by.
var allowed = map[string]string{
	// It hands New the running binary, to prove New refuses it.
	restoreDir: "TestNew_RefusesTheRunningTestBinary",
	// It checks the process table reads this process's argv as os.Args.
	"internal/zmxtest": "checkProcessReader",
}

// scanExeGuard reports each breach of the guard's rules in the tree the
// options name. A scan that finds no production source of restore declaring
// New, no test constructing a restorer, or no test-only package's own source
// is fatal: the guard has lost sight of what it holds.
func scanExeGuard(t harnesstest.TestingT, opts ...sourceguardtest.ScanOption) []string {
	t.Helper()
	_, sources := sourceguardtest.RepoSources(t, sourceguardtest.AllSources, opts...)
	s := surveyOf(sources)
	switch {
	case !s.declared:
		t.Fatalf("no production source of %s declares %s, so the guard would pass by having stopped looking", restoreDir, newRestorer)
		return nil
	case len(s.constructing) == 0:
		t.Fatalf("no test constructs a restorer through %s.%s, so the guard would pass by having stopped looking", restorePath, newRestorer)
		return nil
	case s.helpers == 0:
		t.Fatalf("no source of a test-only package %v was scanned, so the guard would pass by having stopped looking", sourceguardtest.TestOnlyPackages)
		return nil
	}
	findings := s.findings
	for _, source := range sources {
		switch {
		case isTest(source) && s.constructing[dirOf(source)]:
			findings = append(findings, runningBinaryReaches(source, "in a package whose tests construct a restorer")...)
		case !isTest(source) && isTestOnly(source):
			findings = append(findings, runningBinaryReaches(source, "in a test-only package")...)
		}
	}
	return findings
}

// survey is what a pass over the sources finds: whether restore's production
// declares New, the directories whose tests construct a restorer, how many
// sources of test-only packages there are, and each production construction
// outside cmd.
type survey struct {
	declared     bool
	constructing map[string]bool
	helpers      int
	findings     []string
}

func surveyOf(sources []sourceguardtest.ParsedSource) survey {
	s := survey{constructing: map[string]bool{}}
	for _, source := range sources {
		s.declared = s.declared || !isTest(source) && inRestore(source) && declaresFunc(source.File, newRestorer)
		if !isTest(source) && isTestOnly(source) {
			s.helpers++
		}
		constructions := reaches(source, source.File, restorePath, newRestorer)
		switch {
		case len(constructions) == 0:
		case isTest(source):
			s.constructing[dirOf(source)] = true
		case dirOf(source) != productionHome:
			s.findings = append(s.findings, at(source, constructions, "constructs a restorer outside "+productionHome)...)
		}
	}
	return s
}

// runningBinaryReaches is every place source reaches the running binary's
// path, outside the function its directory is allowed, each found where.
func runningBinaryReaches(source sourceguardtest.ParsedSource, where string) []string {
	var findings []string
	for _, decl := range source.File.Decls {
		if fd, ok := decl.(*ast.FuncDecl); ok && fd.Recv == nil && fd.Name.Name == allowed[dirOf(source)] {
			continue
		}
		for _, name := range runningBinary {
			findings = append(findings, at(source, reaches(source, decl, "os", name), "reaches os."+name+" "+where)...)
		}
	}
	return findings
}

func isTestOnly(source sourceguardtest.ParsedSource) bool {
	return slices.Contains(sourceguardtest.TestOnlyPackages, modulePath+"/"+dirOf(source))
}

// declaresFunc reports whether file declares the package-level function name.
func declaresFunc(file *ast.File, name string) bool {
	return slices.ContainsFunc(file.Decls, func(decl ast.Decl) bool {
		fd, ok := decl.(*ast.FuncDecl)
		return ok && fd.Recv == nil && fd.Name.Name == name
	})
}

// reaches is every place within node, of source, that reaches fn of the
// package at pkgPath: through the name source imports the package under, or,
// called unqualified, in the package itself or one that dot-imports it.
func reaches(source sourceguardtest.ParsedSource, node ast.Node, pkgPath, fn string) []token.Pos {
	name := importedAs(source.File, pkgPath)
	unqualified := name == "." || (pkgPath == restorePath && inRestore(source))
	var found []token.Pos
	ast.Inspect(node, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.SelectorExpr:
			if x, ok := n.X.(*ast.Ident); ok && name != "" && x.Name == name && n.Sel.Name == fn {
				found = append(found, n.Pos())
			}
		case *ast.CallExpr:
			if id, ok := n.Fun.(*ast.Ident); ok && unqualified && id.Name == fn {
				found = append(found, id.Pos())
			}
		}
		return true
	})
	return found
}

// importedAs is the name file imports the package at pkgPath under, "." for a
// dot import, and "" when it imports it under no usable name.
func importedAs(file *ast.File, pkgPath string) string {
	for _, spec := range file.Imports {
		if imported, err := strconv.Unquote(spec.Path.Value); err != nil || imported != pkgPath {
			continue
		}
		switch {
		case spec.Name == nil:
			return path.Base(pkgPath)
		case spec.Name.Name == "_":
			return ""
		}
		return spec.Name.Name
	}
	return ""
}

// inRestore reports whether source is the restore package's own code, its
// internal tests included, which names New unqualified.
func inRestore(source sourceguardtest.ParsedSource) bool {
	return dirOf(source) == restoreDir && source.File.Name.Name == path.Base(restorePath)
}

func isTest(source sourceguardtest.ParsedSource) bool {
	return strings.HasSuffix(source.Path, "_test.go")
}

func dirOf(source sourceguardtest.ParsedSource) string {
	return filepath.ToSlash(filepath.Dir(source.Path))
}

func at(source sourceguardtest.ParsedSource, positions []token.Pos, what string) []string {
	findings := make([]string, 0, len(positions))
	for _, pos := range positions {
		findings = append(findings, source.Position(pos).String()+": "+what)
	}
	return findings
}
