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
	"strings"

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
	units   []checkUnit
	exports map[string]string
}

// checkUnit is one package the checker type-checks: its sources, and how its
// imports name the packages its build compiled for it.
type checkUnit struct {
	dir       string
	goFiles   []string
	importMap map[string]string
}

type listedPackage struct {
	ImportPath, Dir, Export, ForTest string
	GoFiles                          []string
	ImportMap                        map[string]string
	DepOnly                          bool
}

// NewTypeChecker resolves the package in dir under the current build, or the
// one WithBuildTags and ForGOOS name: the non-test files that build selects,
// and the export data of each package it imports, directly or not. A package
// go list cannot resolve or compile is fatal.
func NewTypeChecker(t harnesstest.TestingT, dir string, opts ...DepsOption) *TypeChecker {
	t.Helper()
	return newTypeChecker(t, dir, []string{"."}, false, opts)
}

// NewTestTypeChecker resolves the packages patterns name, from dir, as go test
// builds them under the build the options name: each package with its
// in-package test files, and its external test package where it has one, all
// checked by Packages. One go list serves every pattern, so a guard reading
// many packages pays for one.
func NewTestTypeChecker(t harnesstest.TestingT, dir string, patterns []string, opts ...DepsOption) *TypeChecker {
	t.Helper()
	return newTypeChecker(t, dir, patterns, true, opts)
}

func newTypeChecker(t harnesstest.TestingT, dir string, patterns []string, withTests bool, opts []DepsOption) *TypeChecker {
	t.Helper()

	cfg := newDepsConfig(opts)
	args := append([]string{"list", "-export", "-deps", "-json=ImportPath,Dir,GoFiles,Export,DepOnly,ForTest,ImportMap"}, buildFlags(cfg)...)
	if withTests {
		args = append(args, "-test")
	}
	cmd := exec.Command("go", append(args, patterns...)...)
	cmd.Dir = dir
	cmd.Env = buildEnv(cfg)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -export -deps in %s%s: %v", dir, cfg.lane(), listFailure(err))
		return nil
	}
	tc := &TypeChecker{exports: map[string]string{}}
	var own []listedPackage
	decoder := json.NewDecoder(bytes.NewReader(out))
	for decoder.More() {
		var pkg listedPackage
		if err := decoder.Decode(&pkg); err != nil {
			t.Fatalf("decode go list output for %s: %v", dir, err)
			return nil
		}
		tc.exports[pkg.ImportPath] = pkg.Export
		if !pkg.DepOnly {
			own = append(own, pkg)
		}
	}
	tc.units = checkUnits(own)
	if len(tc.units) == 0 {
		t.Fatalf("go list resolved no sources for the package in %s", dir)
		return nil
	}
	return tc
}

// checkUnits are the packages among own to type-check. A package its tests
// recompile is checked as they compile it, in place of the plain build, and a
// generated test main is no source of any package's.
func checkUnits(own []listedPackage) []checkUnit {
	recompiled := map[string]bool{}
	for _, pkg := range own {
		recompiled[pkg.ImportPath] = true
	}
	var units []checkUnit
	for _, pkg := range own {
		testMain := pkg.ForTest == "" && strings.HasSuffix(pkg.ImportPath, ".test")
		replaced := recompiled[pkg.ImportPath+" ["+pkg.ImportPath+".test]"]
		if testMain || replaced || len(pkg.GoFiles) == 0 {
			continue
		}
		units = append(units, checkUnit{dir: pkg.Dir, goFiles: pkg.GoFiles, importMap: pkg.ImportMap})
	}
	return units
}

// Package type-checks the package's own sources, for a checker NewTypeChecker
// made. A source that does not type-check is fatal.
func (tc *TypeChecker) Package(t harnesstest.TestingT) TypedPackage {
	t.Helper()

	if len(tc.units) != 1 {
		t.Fatalf("the checker holds %d packages; Packages checks each of them", len(tc.units))
		return TypedPackage{}
	}
	return tc.checkUnit(t, tc.units[0])
}

// Packages type-checks each package the checker holds: for one
// NewTestTypeChecker made, each package as its tests compile it and its
// external test package. A source that does not type-check is fatal.
func (tc *TypeChecker) Packages(t harnesstest.TestingT) []TypedPackage {
	t.Helper()

	pkgs := make([]TypedPackage, 0, len(tc.units))
	for _, unit := range tc.units {
		pkgs = append(pkgs, tc.checkUnit(t, unit))
	}
	return pkgs
}

func (tc *TypeChecker) checkUnit(t harnesstest.TestingT, unit checkUnit) TypedPackage {
	t.Helper()

	fset := token.NewFileSet()
	sources := make([]ParsedSource, 0, len(unit.goFiles))
	for _, name := range unit.goFiles {
		path := filepath.Join(unit.dir, name)
		file, err := parser.ParseFile(fset, path, nil, ParseMode)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
			return TypedPackage{}
		}
		sources = append(sources, ParsedSource{Path: path, Fset: fset, File: file})
	}
	return tc.check(t, fset, sources, unit.importMap)
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
	return tc.check(t, fset, sources, nil)
}

func (tc *TypeChecker) check(t harnesstest.TestingT, fset *token.FileSet, sources []ParsedSource, importMap map[string]string) TypedPackage {
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
		Types: map[ast.Expr]types.TypeAndValue{},
		Uses:  map[*ast.Ident]types.Object{},
		Defs:  map[*ast.Ident]types.Object{},
	}
	lookup := func(path string) (io.ReadCloser, error) {
		if mapped, ok := importMap[path]; ok {
			path = mapped
		}
		return tc.lookup(path)
	}
	config := types.Config{Importer: importer.ForCompiler(fset, "gc", lookup)}
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
