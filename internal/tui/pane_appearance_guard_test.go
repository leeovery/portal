package tui_test

import (
	"go/ast"
	"path/filepath"
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/sourceguardtest"
)

const (
	backgroundSetCall  = "SetBackgroundColor"
	backgroundSetOwner = "restore.go"
	paneAppearanceFile = "pane_appearance.go"
	detectTimeoutConst = "appearanceDetectTimeout"
	probeBuilderName   = "newPaneAppearanceProbe"
)

var durationUnits = []string{"Nanosecond", "Microsecond", "Millisecond", "Second", "Minute", "Hour"}

// The picker owns a whole terminal's canvas and sets its background back on exit;
// a pane owns one region of someone else's terminal and must never move the
// terminal's default background at all.
func TestBackgroundSet_ConfinedToRestore(t *testing.T) {
	var callers []string
	for _, source := range sourceguardtest.ParsePackageSources(t, ".", false) {
		name := filepath.Base(source.Path)
		ast.Inspect(source.File, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if ok && isAnsiCall(call, backgroundSetCall) && !slices.Contains(callers, name) {
				callers = append(callers, name)
			}
			return true
		})
	}

	t.Run("it calls SetBackgroundColor from restore.go alone", func(t *testing.T) {
		for _, name := range callers {
			if name != backgroundSetOwner {
				t.Errorf("%s calls ansi.%s; only %s may write an OSC 11 set, so no pane-draw path can move the terminal's default background",
					name, backgroundSetCall, backgroundSetOwner)
			}
		}
	})

	t.Run("it finds the call site it polices", func(t *testing.T) {
		if !slices.Contains(callers, backgroundSetOwner) {
			t.Errorf("no non-test file calls ansi.%s; the guard above would pass over a package it has stopped reading", backgroundSetCall)
		}
	})
}

// The drop's ordering against the appearance query is held by the one function
// that builds a probe; a second builder call would resolve a pane's palette with
// the drop somewhere else, leaving the terminal's own reply in the pane's input
// queue for the discard confirmation to read as a keypress.
func TestPaneAppearance_ReachesTheProbeFromOnePlace(t *testing.T) {
	var callers []string
	for _, source := range sourceguardtest.ParsePackageSources(t, ".", false) {
		ast.Inspect(source.File, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok && sourceguardtest.CalleeName(call) == probeBuilderName {
				callers = append(callers, source.Position(call.Pos()).String())
			}
			return true
		})
	}

	t.Run("it reaches the probe from one place alone", func(t *testing.T) {
		if len(callers) != 1 {
			t.Errorf("%s is called from %d places (%v), want exactly 1 — every pane draw must resolve its palette through the seam that runs the input drop after the query", probeBuilderName, len(callers), callers)
		}
	})
}

func TestPaneAppearance_TakesThePickerTimeout(t *testing.T) {
	file := sourceguardtest.PackageSource(t, ".", paneAppearanceFile).File

	t.Run("it names the picker's detect timeout", func(t *testing.T) {
		named := false
		ast.Inspect(file, func(n ast.Node) bool {
			if ident, ok := n.(*ast.Ident); ok && ident.Name == detectTimeoutConst {
				named = true
			}
			return true
		})
		if !named {
			t.Errorf("%s never names %s; the pane draw and the picker must wait a silent terminal out for the same duration", paneAppearanceFile, detectTimeoutConst)
		}
	})

	t.Run("it declares no duration of its own", func(t *testing.T) {
		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || pkg.Name != "time" || !slices.Contains(durationUnits, sel.Sel.Name) {
				return true
			}
			t.Errorf("%s spells out time.%s; a second copy of the detect timeout would drift from the picker's silently",
				paneAppearanceFile, sel.Sel.Name)
			return true
		})
	})
}

// The pane draw's appearance probe reads the stdin it already holds. A device
// opened through syscall or x/sys/unix is as much an open as one through os.
func TestPaneAppearance_OpensNothing(t *testing.T) {
	openers := []string{"os", "syscall", "unix"}
	opens := []string{"Open", "OpenFile", "Openat", "OpenInRoot", "Create"}
	for _, source := range sourceguardtest.ParsePackageSources(t, ".", false) {
		ast.Inspect(source.File, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if ok && slices.Contains(openers, pkg.Name) && slices.Contains(opens, sel.Sel.Name) {
				t.Errorf("%s calls %s.%s; internal/tui opens no file or device — the pane's terminal is read through the stdin it already holds",
					source.Position(call.Pos()), pkg.Name, sel.Sel.Name)
			}
			return true
		})
	}
}

func isAnsiCall(call *ast.CallExpr, name string) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != name {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "ansi"
}
