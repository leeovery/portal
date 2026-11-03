package cmd

import (
	"go/ast"
	"testing"

	"github.com/leeovery/portal/internal/sourceguardtest"
)

// resolveDoctorDeps must source its Detector/Resolve from the shared
// buildProductionSpawnSeams bundle rather than hand-rebuilding them: the
// compiler cannot catch a seam added or swapped on only one side. It builds
// the bundle over the read-only registry, since doctor's read-only path heals
// nothing, and detection would otherwise remove a dead runner's entry.
func TestResolveDoctorDepsUsesSharedSpawnSeams(t *testing.T) {
	source := sourceguardtest.PackageSource(t, ".", "doctor.go")

	fn := findFuncDeclInFile(t, source.File, "resolveDoctorDeps")

	sawBundle := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fun := call.Fun.(type) {
		case *ast.Ident:
			switch fun.Name {
			case "buildResolver":
				pos := source.Position(call.Pos())
				t.Errorf("doctor.go:%d resolveDoctorDeps calls buildResolver() directly; route the host-terminal Resolve seam through buildProductionSpawnSeams instead", pos.Line)
			case "buildProductionSpawnSeams":
				sawBundle = true
				if !readOnlyRunnersArg(call) {
					pos := source.Position(call.Pos())
					t.Errorf("doctor.go:%d resolveDoctorDeps builds the spawn seams over a registry other than readOnlyRunners{}", pos.Line)
				}
			}
		case *ast.SelectorExpr:
			if pkg, ok := fun.X.(*ast.Ident); ok && pkg.Name == "spawn" && fun.Sel.Name == "NewDetector" {
				pos := source.Position(call.Pos())
				t.Errorf("doctor.go:%d resolveDoctorDeps calls spawn.NewDetector directly; route the host-terminal Detector seam through buildProductionSpawnSeams instead", pos.Line)
			}
		}
		return true
	})

	if !sawBundle {
		t.Error("resolveDoctorDeps does not call buildProductionSpawnSeams; its Detector/Resolve seams must originate from the shared bundle")
	}
}

// readOnlyRunnersArg reports whether call's last argument is readOnlyRunners{}.
func readOnlyRunnersArg(call *ast.CallExpr) bool {
	if len(call.Args) == 0 {
		return false
	}
	lit, ok := call.Args[len(call.Args)-1].(*ast.CompositeLit)
	if !ok {
		return false
	}
	typ, ok := lit.Type.(*ast.Ident)
	return ok && typ.Name == "readOnlyRunners"
}

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
