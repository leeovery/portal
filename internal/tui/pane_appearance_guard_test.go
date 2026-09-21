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
	terminalPathConst  = "paneTTYPath"
	terminalOpenerName = "openTTYPath"
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

// The construction guard exempts this file's one os.OpenFile; the exemption is
// only as narrow as the path that call is allowed to take.
func TestPaneAppearance_OpensNothingButThePanesTerminal(t *testing.T) {
	opens, openerParam := 0, terminalOpenerParam(t)

	for _, source := range sourceguardtest.ParsePackageSources(t, ".", false) {
		name := filepath.Base(source.Path)
		ast.Inspect(source.File, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			switch sourceguardtest.CalleeName(call) {
			case "OpenFile":
				opens++
				if arg, ok := call.Args[0].(*ast.Ident); !ok || arg.Name != openerParam {
					t.Errorf("%s opens a path %s does not carry; a config read would reach the TUI through this file's exemption from the construction guard",
						name, terminalOpenerName)
				}
			case terminalOpenerName:
				if arg, ok := call.Args[0].(*ast.Ident); !ok || arg.Name != terminalPathConst {
					t.Errorf("%s calls %s on something other than %s; the probe reads the pane's own terminal and nothing else",
						name, terminalOpenerName, terminalPathConst)
				}
			}
			return true
		})
	}

	if opens != 1 {
		t.Errorf("internal/tui makes %d OpenFile calls, want exactly 1 — the pane's own terminal", opens)
	}
}

// terminalOpenerParam is the parameter the opener takes its path from, so the
// assertion above reads the declaration rather than restating it.
func terminalOpenerParam(t *testing.T) string {
	t.Helper()
	file := sourceguardtest.PackageSource(t, ".", paneAppearanceFile).File
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || fn.Name.Name != terminalOpenerName {
			continue
		}
		if params := fn.Type.Params.List; len(params) == 1 && len(params[0].Names) == 1 {
			return params[0].Names[0].Name
		}
	}
	t.Fatalf("%s declares no single-parameter %s", paneAppearanceFile, terminalOpenerName)
	return ""
}

func isAnsiCall(call *ast.CallExpr, name string) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != name {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "ansi"
}
