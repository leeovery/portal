package sourceguardtest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"

	"github.com/leeovery/portal/internal/harnesstest"
)

// TypedPackage is a package's sources with what their identifiers resolve to,
// for a rule a name alone cannot decide: a method, judged by the object it is
// rather than by how a source spelled the reach for it.
type TypedPackage struct {
	Sources []ParsedSource
	Info    *types.Info
}

// TypeChecker checks sources as if they sat in one package, importing what
// that package's build imports from the export data go list compiles for it.
type TypeChecker struct {
	dir     string
	goFiles []string
	exports map[string]string
}

// NewTypeChecker resolves the package in dir under the current build: the
// non-test files that build selects, and the export data of each package it
// imports, directly or not. A package go list cannot resolve or compile is
// fatal.
func NewTypeChecker(t harnesstest.TestingT, dir string) *TypeChecker {
	t.Helper()

	cmd := exec.Command("go", "list", "-export", "-deps", "-json=ImportPath,Dir,GoFiles,Export,DepOnly", ".")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -export -deps in %s: %v", dir, listFailure(err))
		return nil
	}
	tc := &TypeChecker{exports: map[string]string{}}
	decoder := json.NewDecoder(bytes.NewReader(out))
	for decoder.More() {
		var pkg struct {
			ImportPath, Dir, Export string
			GoFiles                 []string
			DepOnly                 bool
		}
		if err := decoder.Decode(&pkg); err != nil {
			t.Fatalf("decode go list output for %s: %v", dir, err)
			return nil
		}
		tc.exports[pkg.ImportPath] = pkg.Export
		if !pkg.DepOnly {
			tc.dir, tc.goFiles = pkg.Dir, pkg.GoFiles
		}
	}
	if len(tc.goFiles) == 0 {
		t.Fatalf("go list resolved no sources for the package in %s", dir)
		return nil
	}
	return tc
}

// Package type-checks the package's own sources. A source that does not
// type-check is fatal.
func (tc *TypeChecker) Package(t harnesstest.TestingT) TypedPackage {
	t.Helper()

	paths := make([]string, len(tc.goFiles))
	for i, name := range tc.goFiles {
		paths[i] = filepath.Join(tc.dir, name)
	}
	fset := token.NewFileSet()
	sources := make([]ParsedSource, 0, len(paths))
	for _, path := range paths {
		file, err := parser.ParseFile(fset, path, nil, ParseMode)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
			return TypedPackage{}
		}
		sources = append(sources, ParsedSource{Path: path, Fset: fset, File: file})
	}
	return tc.check(t, fset, sources)
}

// Sources type-checks src, a file name to its body, on its own, importing
// whatever the package's build can: a rule test's fixture, checked as the
// package's code would be.
func (tc *TypeChecker) Sources(t harnesstest.TestingT, src map[string]string) TypedPackage {
	t.Helper()

	fset := token.NewFileSet()
	sources := make([]ParsedSource, 0, len(src))
	for _, name := range slices.Sorted(maps.Keys(src)) {
		file, err := parser.ParseFile(fset, name, src[name], ParseMode)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
			return TypedPackage{}
		}
		sources = append(sources, ParsedSource{Path: name, Fset: fset, File: file})
	}
	return tc.check(t, fset, sources)
}

func (tc *TypeChecker) check(t harnesstest.TestingT, fset *token.FileSet, sources []ParsedSource) TypedPackage {
	t.Helper()

	if len(sources) == 0 {
		t.Fatalf("type-checked no sources, so a guard over them would pass by having stopped looking")
		return TypedPackage{}
	}
	files := make([]*ast.File, len(sources))
	for i, source := range sources {
		files[i] = source.File
	}
	info := &types.Info{
		Uses: map[*ast.Ident]types.Object{},
		Defs: map[*ast.Ident]types.Object{},
	}
	config := types.Config{Importer: importer.ForCompiler(fset, "gc", tc.lookup)}
	if _, err := config.Check(files[0].Name.Name, fset, files, info); err != nil {
		t.Fatalf("type-check %s: %v", files[0].Name.Name, err)
		return TypedPackage{}
	}
	return TypedPackage{Sources: sources, Info: info}
}

func (tc *TypeChecker) lookup(path string) (io.ReadCloser, error) {
	export, ok := tc.exports[path]
	if !ok || export == "" {
		return nil, fmt.Errorf("no export data for %s", path)
	}
	return os.Open(export)
}

// Object is what ident names, as a use or as a declaration; nil for an ident
// that names nothing, such as a package name.
func (p TypedPackage) Object(ident *ast.Ident) types.Object {
	if obj := p.Info.Uses[ident]; obj != nil {
		return obj
	}
	return p.Info.Defs[ident]
}

// IsPackageFunc reports whether ident names the function name of the package
// at pkgPath, under whatever name the source imported it.
func (p TypedPackage) IsPackageFunc(ident *ast.Ident, pkgPath, name string) bool {
	fn, ok := p.Info.Uses[ident].(*types.Func)
	return ok && fn.Signature().Recv() == nil && fn.Name() == name && fn.Pkg() != nil && fn.Pkg().Path() == pkgPath
}

func listFailure(err error) error {
	if exit, ok := errors.AsType[*exec.ExitError](err); ok {
		return fmt.Errorf("%w\n%s", err, exit.Stderr)
	}
	return err
}
