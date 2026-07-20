// Tests in this file parse cmd/doctor.go's source (AST-only, comments ignored)
// and MUST NOT use t.Parallel.
package cmd

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// TestResolveDoctorDepsUsesSharedSpawnSeams enforces the single-construction-site
// invariant for doctor's host-terminal seams: resolveDoctorDeps must source its
// Detector/Resolve from the shared buildProductionSpawnSeams bundle
// (cmd/spawn_seams.go) — the same bundle the picker and the multi-target open
// burst read — and NEVER hand-rebuild them via an independent spawn.NewDetector /
// buildResolver construction. A by-hand third copy reintroduces exactly the drift
// obligation the bundle exists to abolish (the compiler cannot catch a seam added
// or swapped on only one side). The check is AST-based (comments are ignored) and
// scoped to the resolveDoctorDeps function body.
func TestResolveDoctorDepsUsesSharedSpawnSeams(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "doctor.go", nil, 0)
	if err != nil {
		t.Fatalf("parse doctor.go: %v", err)
	}

	fn := findFuncDeclInFile(t, file, "resolveDoctorDeps")

	sawBundle := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fun := call.Fun.(type) {
		case *ast.Ident:
			// Package-local calls: buildResolver() / buildProductionSpawnSeams(...).
			switch fun.Name {
			case "buildResolver":
				pos := fset.Position(call.Pos())
				t.Errorf("doctor.go:%d resolveDoctorDeps calls buildResolver() directly; route the host-terminal Resolve seam through buildProductionSpawnSeams instead", pos.Line)
			case "buildProductionSpawnSeams":
				sawBundle = true
			}
		case *ast.SelectorExpr:
			// Package-qualified calls: spawn.NewDetector(...).
			if pkg, ok := fun.X.(*ast.Ident); ok && pkg.Name == "spawn" && fun.Sel.Name == "NewDetector" {
				pos := fset.Position(call.Pos())
				t.Errorf("doctor.go:%d resolveDoctorDeps calls spawn.NewDetector directly; route the host-terminal Detector seam through buildProductionSpawnSeams instead", pos.Line)
			}
		}
		return true
	})

	if !sawBundle {
		t.Error("resolveDoctorDeps does not call buildProductionSpawnSeams; its Detector/Resolve seams must originate from the shared bundle")
	}
}

// findFuncDeclInFile returns the top-level (non-method) function declaration
// named name, failing the test when absent.
func findFuncDeclInFile(t *testing.T, file *ast.File, name string) *ast.FuncDecl {
	t.Helper()
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name.Name == name {
			return fn
		}
	}
	t.Fatalf("function %s not found in parsed file", name)
	return nil
}
