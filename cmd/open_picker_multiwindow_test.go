package cmd

import (
	"go/ast"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/sourceguardtest"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/spawntest"
	"github.com/leeovery/portal/internal/tui"
)

// pickerSpawnSeams are the tuiConfig fields multi-select bursts through.
var pickerSpawnSeams = []string{"detector", "resolve", "sessionExists", "ackChannel", "spawnExe", "spawnGetenv", "spawnLogger"}

// runPicker wires every spawn seam from the buildProductionSpawnSeams bundle
// the CLI burst and doctor read, rather than hand-building one: the compiler
// cannot catch a seam added or swapped on only one side.
func TestRunPickerWiresTheSharedSpawnSeams(t *testing.T) {
	source := sourceguardtest.PackageSource(t, ".", "open.go")
	fn := findFuncDeclInFile(t, source.File, "runPicker")

	sawBundle := false
	wired := map[string]bool{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.CallExpr:
			switch fun := n.Fun.(type) {
			case *ast.Ident:
				switch fun.Name {
				case "buildProductionSpawnSeams":
					sawBundle = true
				case "buildResolver":
					t.Errorf("open.go:%d runPicker calls buildResolver directly", source.Position(n.Pos()).Line)
				}
			case *ast.SelectorExpr:
				if pkg, ok := fun.X.(*ast.Ident); ok && pkg.Name == "spawn" {
					t.Errorf("open.go:%d runPicker calls spawn.%s directly", source.Position(n.Pos()).Line, fun.Sel.Name)
				}
			}
		case *ast.KeyValueExpr:
			if key, ok := n.Key.(*ast.Ident); ok {
				wired[key.Name] = true
			}
		}
		return true
	})
	if !sawBundle {
		t.Error("runPicker does not call buildProductionSpawnSeams")
	}
	for _, seam := range pickerSpawnSeams {
		if !wired[seam] {
			t.Errorf("runPicker leaves the spawn seam %s unwired", seam)
		}
	}
}

func TestBuildTUIModel_MultiSelectEntersWithTheSpawnSeams(t *testing.T) {
	cfg := defaultTestTUIConfig()
	cfg.detector = fakeTerminalDetector{id: ghosttyIdentity()}
	cfg.resolve = func(spawn.Identity) (spawn.Adapter, spawn.Resolution) {
		return &spawntest.FakeAdapter{}, spawn.ResolutionNative
	}
	cfg.sessionExists = func(string) bool { return true }
	cfg.ackChannel = &spawntest.FakeAckChannel{}

	var model tea.Model = buildTUIModel(cfg, pickerLanding{}, nil)
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model, _ = model.Update(tui.SessionsMsg{Sessions: []tui.Session{{Name: "alpha"}, {Name: "bravo"}}})
	model, _ = model.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})

	if !model.(tui.Model).MultiSelectActive() {
		t.Error("m did not enter multi-select")
	}
}
