package cmd

import (
	"go/ast"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/sourceguardtest"
	"github.com/leeovery/portal/internal/tui"
)

// Until the attach runner, the picker's multi-select refuses as the CLI burst
// does: openTUI builds no spawn seam, so nothing walks tmux's clients, sets a
// spawn ack option or opens a window, and it hands the picker the refusal.
func TestOpenTUIBuildsNoSpawnSeams(t *testing.T) {
	source := sourceguardtest.PackageSource(t, ".", "open.go")
	fn := findFuncDeclInFile(t, source.File, "openTUI")

	sawRefusal := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.CallExpr:
			switch fun := n.Fun.(type) {
			case *ast.Ident:
				if fun.Name == "buildProductionSpawnSeams" || fun.Name == "spawnDetector" {
					t.Errorf("open.go:%d openTUI calls %s", source.Position(n.Pos()).Line, fun.Name)
				}
			case *ast.SelectorExpr:
				if pkg, ok := fun.X.(*ast.Ident); ok && pkg.Name == "spawn" {
					t.Errorf("open.go:%d openTUI calls spawn.%s", source.Position(n.Pos()).Line, fun.Sel.Name)
				}
			}
		case *ast.KeyValueExpr:
			if key, ok := n.Key.(*ast.Ident); ok {
				switch key.Name {
				case "multiWindowRefusal":
					sawRefusal = true
				case "detector", "resolve", "sessionExists", "ackChannel", "spawnExe", "spawnGetenv", "spawnLogger":
					t.Errorf("open.go:%d openTUI wires the spawn seam %s", source.Position(n.Pos()).Line, key.Name)
				}
			}
		}
		return true
	})
	if !sawRefusal {
		t.Error("openTUI does not hand the picker the multi-window refusal")
	}
}

func TestBuildTUIModel_ThreadsTheMultiWindowRefusal(t *testing.T) {
	cfg := defaultTestTUIConfig()
	cfg.multiWindowRefusal = errSeveralSessions.Error()

	var model tea.Model = buildTUIModel(cfg, pickerLanding{}, nil)
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model, _ = model.Update(tui.SessionsMsg{Sessions: []tui.Session{{Name: "alpha"}, {Name: "bravo"}}})
	model, _ = model.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})

	if model.(tui.Model).MultiSelectActive() {
		t.Error("m entered multi-select; the picker must refuse it as the CLI burst does")
	}
}
