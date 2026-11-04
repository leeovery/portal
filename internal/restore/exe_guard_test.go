package restore_test

import (
	"go/ast"
	"go/token"
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
			got := scanExeGuard(t, sourceguardtest.Rooted(writeTree(t, tc.files)))

			slices.Sort(got)
			if !slices.Equal(got, tc.want) {
				t.Errorf("findings = %q\nwant %q", got, tc.want)
			}
		})
	}
}

func TestExeGuard_FatalsWhenItScansNothing(t *testing.T) {
	for name, files := range map[string]map[string]string{
		"an empty tree":                       {},
		"a tree where no test constructs one": {"internal/a/a.go": "package a\n", "internal/a/a_test.go": "package a\n"},
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
// os.Executable is the test binary: run with a leading state positional, it
// runs the whole suite again inside the session, with none of the test's
// isolation. New refuses an exe that isn't absolute, so the guard keeps
// os.Executable away from every construction:
//
//   - In tests, no package whose tests construct a restorer reaches
//     os.Executable in any of its test files, so no helper beside the
//     construction can hand it over.
//   - In production, only cmd constructs one, where the shipped binary's own
//     path is the intent.
const (
	restorePath    = "github.com/leeovery/portal/internal/restore"
	restoreDir     = "internal/restore"
	newRestorer    = "New"
	productionHome = "cmd"
)

// scanExeGuard reports each breach of the guard's rules in the tree the
// options name. A scan that finds no test constructing a restorer is fatal:
// restore's own tests construct one, so the guard has lost sight of New.
func scanExeGuard(t harnesstest.TestingT, opts ...sourceguardtest.ScanOption) []string {
	t.Helper()
	_, sources := sourceguardtest.RepoSources(t, sourceguardtest.AllSources, opts...)
	constructing := map[string]bool{}
	var findings []string
	for _, source := range sources {
		constructions := reaches(source, restorePath, newRestorer)
		if len(constructions) == 0 {
			continue
		}
		if isTest(source) {
			constructing[dirOf(source)] = true
		} else if dirOf(source) != productionHome {
			findings = append(findings, at(source, constructions, "constructs a restorer outside "+productionHome)...)
		}
	}
	if len(constructing) == 0 {
		t.Fatalf("no test constructs a restorer through %s.%s, so the guard would pass by having stopped looking", restorePath, newRestorer)
		return nil
	}
	for _, source := range sources {
		if isTest(source) && constructing[dirOf(source)] {
			findings = append(findings, at(source, reaches(source, "os", "Executable"), "reaches os.Executable in a package whose tests construct a restorer")...)
		}
	}
	return findings
}

// reaches is every place source reaches fn of the package at pkgPath: through
// the name it imports the package under, or, called unqualified, in the
// package itself or one that dot-imports it.
func reaches(source sourceguardtest.ParsedSource, pkgPath, fn string) []token.Pos {
	name := importedAs(source.File, pkgPath)
	unqualified := name == "." || (pkgPath == restorePath && inRestore(source))
	var found []token.Pos
	ast.Inspect(source.File, func(n ast.Node) bool {
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
