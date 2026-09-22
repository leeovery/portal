package tui

import (
	"fmt"
	"go/ast"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/sourceguardtest"
	"github.com/leeovery/portal/internal/theme"
)

const canvasFillFile = "canvas_fill.go"

func TestCanvasFill_PaintsThePerCellBackgroundInOnePlace(t *testing.T) {
	t.Run("it paints the per-cell background in one place", func(t *testing.T) {
		// The panel paints its own rows before the composite, so it reaches the
		// backfill without being a fill loop; nothing else may reach either helper.
		allowed := map[string]map[string]bool{
			"padLineToCanvasWidth":     {canvasFillFile: true},
			"backfillCanvasBackground": {canvasFillFile: true, "theme_panel_render.go": true},
		}
		sites := map[string][]string{}
		for _, source := range sourceguardtest.ParsePackageSources(t, ".", false) {
			file := filepath.Base(source.Path)
			sourceguardtest.ForEachFuncCall(source.File, func(_ string, call *ast.CallExpr) bool {
				callee := sourceguardtest.CalleeName(call)
				if allowed[callee] == nil {
					return true
				}
				sites[callee] = append(sites[callee], file+":"+strconv.Itoa(source.Position(call.Pos()).Line))
				return true
			})
		}

		for helper, files := range allowed {
			called := sites[helper]
			if len(called) == 0 {
				t.Fatalf("%s is called nowhere — the scan found no call sites at all", helper)
			}
			inKernel := 0
			for _, site := range called {
				file, _, _ := strings.Cut(site, ":")
				if !files[file] {
					t.Errorf("%s is called at %s; the per-cell background rule is painted in %s alone", helper, site, canvasFillFile)
				}
				if file == canvasFillFile {
					inKernel++
				}
			}
			if inKernel != 1 {
				t.Errorf("%s is called %d times in %s (%q), want exactly one — the kernel's single loop", helper, inKernel, canvasFillFile, called)
			}
		}
	})
}

// The fill as fillCanvas spelled it inline, before both callers were routed
// through the kernel. Frozen: it is what byte-identity is measured against.
func preConsolidationFillCanvas(m Model, view string) string {
	w, h := m.termDims()
	contentW := m.contentWidth()
	contentH := m.contentHeight()
	if m.colourless {
		content := m.overlayThemePanelOnContent(fillColourless(view, contentW, contentH), contentW, contentH)
		return insetColourless(content, w, h, contentW, contentH)
	}
	canvas := lipgloss.NewStyle().Background(m.themeState.active.Canvas.Color())
	canvasBg := canvasBgParams(m.themeState.active.Canvas.Color())
	parser := ansi.NewParser()

	lines := strings.Split(view, "\n")
	out := make([]string, 0, contentH)
	for _, line := range lines {
		if len(out) == contentH {
			break
		}
		line = backfillCanvasBackground(line, canvasBg, parser)
		out = append(out, padLineToCanvasWidth(line, contentW, canvas))
	}
	blank := canvas.Render(strings.Repeat(" ", contentW))
	for len(out) < contentH {
		out = append(out, blank)
	}
	content := m.overlayThemePanelOnContent(strings.Join(out, "\n"), contentW, contentH)
	return insetCanvasCanvas(strings.Split(content, "\n"), w, h, contentW, canvas)
}

func TestFillCanvas_ComposesTheFrameTheInlineFillComposed(t *testing.T) {
	const w, h = 90, 24

	t.Run("it composes the frame the inline fill composed", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			model func(t *testing.T) Model
		}{
			{"sessions without a band", func(t *testing.T) Model {
				return newCanvasTestModel(t, w, h, theme.MemberDark)
			}},
			{"sessions with a band", func(t *testing.T) Model {
				m := newCanvasTestModel(t, w, h, theme.MemberDark)
				m.flashText = "renamed"
				return m
			}},
			{"projects", func(t *testing.T) Model {
				m := newCanvasTestModel(t, w, h, theme.MemberDark)
				m.activePage = PageProjects
				return m
			}},
			{"a modal", func(t *testing.T) Model {
				m := newCanvasTestModel(t, w, h, theme.MemberDark)
				m.modal = modalHelp
				return m
			}},
			{"the theme panel open", func(t *testing.T) Model {
				m := themeOpenTestPopulatedModel(t, newOpenThemeSource(themeOpenTestUnion()))
				m.termWidth, m.termHeight = w, h
				m = pressPanelKey(t, m, tea.KeyPressMsg{Code: 't', Text: "t"})
				if !m.themePanel.open {
					t.Fatal("setup invariant: t did not open the theme panel")
				}
				return m
			}},
			{"colourless", func(t *testing.T) Model {
				return colourlessTestModel(t, w, h)
			}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				m := tc.model(t)
				view := m.viewString()
				if got, want := m.fillCanvas(view), preConsolidationFillCanvas(m, view); got != want {
					t.Errorf("the composed frame drifted from the inline fill's\n got: %q\nwant: %q",
						strings.ReplaceAll(got, "\x1b", "\\e"), strings.ReplaceAll(want, "\x1b", "\\e"))
				}
			})
		}
	})
}

func TestFillPaneCanvas(t *testing.T) {
	forEachBuiltinTheme(t, func(t *testing.T, th theme.Theme) {
		canvasParams := wantCanvasBgParams(t, th)

		t.Run("it pads a short view with blank canvas rows", func(t *testing.T) {
			out := fillPaneCanvas("ab\ncd", 6, 4, th, false)
			assertPaneRows(t, paneRows(t, out, 6, 4), []string{"ab", "cd", "", ""})
			assertEveryCellIsCanvas(t, out, 6, canvasParams)
		})

		t.Run("it clamps a view taller than the pane", func(t *testing.T) {
			out := fillPaneCanvas("a\nb\nc\nd", 3, 2, th, false)
			assertPaneRows(t, paneRows(t, out, 3, 2), []string{"a", "b"})
			assertEveryCellIsCanvas(t, out, 3, canvasParams)
		})

		t.Run("it backfills a mid-line cell left on the terminal's own background", func(t *testing.T) {
			out := fillPaneCanvas("ab\x1b[0mcd", 6, 1, th, false)
			assertPaneRows(t, paneRows(t, out, 6, 1), []string{"abcd"})
			assertEveryCellIsCanvas(t, out, 6, canvasParams)
		})

		t.Run("it paints no background under NO_COLOR", func(t *testing.T) {
			out := fillPaneCanvas("ab\ncd", 6, 4, th, true)
			assertPaneRows(t, paneRows(t, out, 6, 4), []string{"ab", "cd", "", ""})
			assertNoBackgroundPainted(t, out)
		})
	})
}

func TestFillPaneCanvas_FillsBothCallersShapes(t *testing.T) {
	th := testDarkTheme(t)
	canvasParams := wantCanvasBgParams(t, th)

	t.Run("it fills both callers' shapes", func(t *testing.T) {
		for _, region := range []struct {
			name string
			w, h int
		}{
			{"the picker's content region", 86, 20},
			{"a restored pane", 20, 8},
		} {
			t.Run(region.name, func(t *testing.T) {
				for _, tc := range []struct {
					name string
					view string
					want []string
				}{
					{"a view shorter than the region", "ab\ncd", []string{"ab", "cd"}},
					{"a view taller than the region", strings.Join(numberedRows(region.h+3), "\n"), numberedRows(region.h)},
					{"a view narrower than the width", "ab", []string{"ab"}},
					{"a mid-line SGR reset", "ab\x1b[0mcd", []string{"abcd"}},
				} {
					t.Run(tc.name, func(t *testing.T) {
						out := fillPaneCanvas(tc.view, region.w, region.h, th, false)
						assertPaneRows(t, paneRows(t, out, region.w, region.h), padRowsToHeight(tc.want, region.h))
						assertEveryCellIsCanvas(t, out, region.w, canvasParams)
					})
				}

				t.Run("colourless", func(t *testing.T) {
					out := fillPaneCanvas("ab\ncd", region.w, region.h, th, true)
					assertPaneRows(t, paneRows(t, out, region.w, region.h), padRowsToHeight([]string{"ab", "cd"}, region.h))
					assertNoBackgroundPainted(t, out)
				})
			})
		}
	})
}

func numberedRows(n int) []string {
	rows := make([]string, 0, n)
	for i := range n {
		rows = append(rows, fmt.Sprintf("row-%d", i))
	}
	return rows
}

func padRowsToHeight(rows []string, h int) []string {
	padded := make([]string, 0, h)
	padded = append(padded, rows...)
	for len(padded) < h {
		padded = append(padded, "")
	}
	return padded
}
