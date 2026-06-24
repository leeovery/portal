package tui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"charm.land/lipgloss/v2"
)

// Task spectrum-tui-design-8-6 consolidation gate. The §8.1/§13.5 cleared-canvas
// modal centring — lipgloss.Place(width, height, Center, Center, panel) — had
// accreted as a verbatim copy inside every render*ModalOnClearedCanvas wrapper
// (help / kill / delete / rename / edit, plus the now-unused generic legacy
// wrapper), one copy per modal across the 3-4…3-9 reskin tasks. These tests prove
// the post-consolidation render routes through the single placeModalOnClearedCanvas
// helper with ZERO output drift, and that the centring line now lives in exactly
// one place.
//
// No t.Parallel() — the tui package injects shared mutable state; parallelism is
// unsafe across this package's tests.

// prePlaceModalOnClearedCanvas reproduces the ORIGINAL inline placement logic
// verbatim — the golden the consolidation must preserve byte-for-byte.
func prePlaceModalOnClearedCanvas(panel string, width, height int) string {
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, panel)
}

// TestPlaceModalOnClearedCanvas_ByteIdenticalToInline asserts the extracted
// placeModalOnClearedCanvas helper produces output byte-identical to the original
// inline lipgloss.Place call across a representative spread of panels and content
// region sizes (including the 80×24 fallback dims the modals are actually placed
// into, and a degenerate zero-size region).
func TestPlaceModalOnClearedCanvas_ByteIdenticalToInline(t *testing.T) {
	panels := []string{
		"",
		"x",
		"single line panel",
		"line one\nline two\nline three",
		"╭───────╮\n│ panel │\n╰───────╯",
	}
	dims := []struct{ w, h int }{
		{80, 24},
		{120, 40},
		{40, 12},
		{1, 1},
		{0, 0},
	}
	for _, panel := range panels {
		for _, d := range dims {
			want := prePlaceModalOnClearedCanvas(panel, d.w, d.h)
			if got := placeModalOnClearedCanvas(panel, d.w, d.h); got != want {
				t.Errorf("placeModalOnClearedCanvas(%q, %d, %d) drift\n got: %q\nwant: %q",
					panel, d.w, d.h, got, want)
			}
		}
	}
}

// TestModalCentringAppearsInExactlyOnePlace is the consolidation guard: it walks
// modal.go's AST and asserts the cleared-canvas centring expression —
// lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, panel) — appears
// in exactly ONE function body (placeModalOnClearedCanvas). No per-modal wrapper
// may re-implement the centring line. The signature shape pinned: Place's first
// two args are the bare idents `width`/`height` and the centre args are
// lipgloss.Center twice — the exact form every wrapper had copied.
func TestModalCentringAppearsInExactlyOnePlace(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "modal.go", nil, 0)
	if err != nil {
		t.Fatalf("parse modal.go: %v", err)
	}

	var hosts []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if isClearedCanvasPlaceCall(call) {
				hosts = append(hosts, fn.Name.Name)
			}
			return true
		})
	}

	if len(hosts) != 1 {
		t.Fatalf("cleared-canvas centring lipgloss.Place(width, height, Center, Center, panel) must appear in exactly one function; found in %v", hosts)
	}
	if hosts[0] != "placeModalOnClearedCanvas" {
		t.Errorf("cleared-canvas centring lives in %q, want placeModalOnClearedCanvas", hosts[0])
	}
}

// isClearedCanvasPlaceCall reports whether call is exactly
// lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, <panel>) — the
// verbatim modal-centring shape the consolidation collapses to one place.
func isClearedCanvasPlaceCall(call *ast.CallExpr) bool {
	if !isSelector(call.Fun, "lipgloss", "Place") {
		return false
	}
	if len(call.Args) != 5 {
		return false
	}
	return isIdent(call.Args[0], "width") &&
		isIdent(call.Args[1], "height") &&
		isSelector(call.Args[2], "lipgloss", "Center") &&
		isSelector(call.Args[3], "lipgloss", "Center")
}

func isSelector(expr ast.Expr, pkg, name string) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != name {
		return false
	}
	return isIdent(sel.X, pkg)
}

func isIdent(expr ast.Expr, name string) bool {
	id, ok := expr.(*ast.Ident)
	return ok && id.Name == name
}
