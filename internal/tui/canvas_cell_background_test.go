package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/tui/theme"
)

// bgState classifies the active SGR background after a (possibly empty) run of
// SGR parameters: whether any explicit background colour is set at all, and — if
// it is — its raw parameter form (so the test can tell the canvas bg apart from
// a content bg like the selected-row tint or the violet title box).
type bgState struct {
	set    bool   // an explicit background SGR is active
	params string // the raw "48;..." / "4x" / "10x" form of the active bg
}

// applySGR folds one SGR sequence's parameters into the running background
// state. It honours only the background-relevant codes: 0 / empty (full reset →
// default), 49 (background-default), 48;... (truecolor / 256 bg), 40-47 and
// 100-107 (named bg). Foreground colour runs (38;2;r;g;b / 38;5;n) are skipped
// whole — mirroring production's sgrBackgroundActive — so a fg channel value
// can't be misread as a bg code; other foreground/attribute codes leave bg state.
// A line is processed left to right; the final state after each sequence is what
// the following text run renders on.
func applySGR(st bgState, params []string) bgState {
	if len(params) == 0 {
		return bgState{} // bare ESC[m == full reset == default background
	}
	for i := 0; i < len(params); i++ {
		p := params[i]
		switch p {
		case "", "0", "49":
			st = bgState{} // full reset / background reset to default
		case "48":
			// Truecolor (48;2;r;g;b) or 256 (48;5;n). Consume the whole run.
			run := []string{p}
			if i+1 < len(params) {
				switch params[i+1] {
				case "2":
					run = append(run, params[i+1:min(i+5, len(params))]...)
					i = min(i+4, len(params)-1)
				case "5":
					run = append(run, params[i+1:min(i+3, len(params))]...)
					i = min(i+2, len(params)-1)
				}
			}
			st = bgState{set: true, params: strings.Join(run, ";")}
		case "38":
			// Foreground colour — skip its channel params (mirroring 48) so a fg
			// channel value can't be misread as a bg code. Leaves bg state.
			if i+1 < len(params) {
				switch params[i+1] {
				case "2":
					i = min(i+4, len(params)-1)
				case "5":
					i = min(i+2, len(params)-1)
				}
			}
		default:
			if isNamedBg(p) {
				st = bgState{set: true, params: p}
			}
			// any other code (foreground, attrs) leaves bg untouched
		}
	}
	return st
}

func isNamedBg(p string) bool {
	switch p {
	case "40", "41", "42", "43", "44", "45", "46", "47",
		"100", "101", "102", "103", "104", "105", "106", "107":
		return true
	}
	return false
}

// wantCanvasBgParams returns the raw background-parameter form (e.g.
// "48;2;11;12;20") that the mode's canvas background renders as, derived from
// lipgloss so the test pins the SAME bytes production paints.
func wantCanvasBgParams(t *testing.T, m theme.Mode) string {
	t.Helper()
	seq := canvasSeq(t, m) // e.g. "\x1b[48;2;11;12;20m"
	inner := strings.TrimSuffix(strings.TrimPrefix(seq, "\x1b["), "m")
	if inner == "" {
		t.Fatalf("could not derive canvas bg params from %q", seq)
	}
	return inner
}

// scanCellBackgrounds walks a single composed line and returns, for each visible
// cell column it produced, whether that cell carried an explicit background and
// (if so) its raw params. It uses ansi.DecodeSequence to step the line so it is
// agnostic to how sub-renderers emitted their SGR. Returns the slice of per-cell
// states up to the printable width of the line.
func scanCellBackgrounds(line string) []bgState {
	var cells []bgState
	st := bgState{}
	p := ansi.NewParser()
	b := []byte(line)
	state := byte(0)
	for len(b) > 0 {
		seq, width, n, newState := ansi.DecodeSequence(b, state, p)
		if n == 0 {
			break
		}
		if ansi.HasCsiPrefix(seq) && len(seq) > 0 && seq[len(seq)-1] == 'm' {
			st = applySGR(st, sgrParams(string(seq)))
		} else if width > 0 {
			// A printable rune (or wide cluster). Record its cells.
			for i := 0; i < width; i++ {
				cells = append(cells, st)
			}
		}
		b = b[n:]
		state = newState
	}
	return cells
}

// sgrParams extracts the ";"-separated parameter list from a CSI ...m sequence.
func sgrParams(seq string) []string {
	inner := strings.TrimSuffix(strings.TrimPrefix(seq, "\x1b["), "m")
	if inner == "" {
		return nil
	}
	return strings.Split(inner, ";")
}

// TestCanvasCellBackground_EveryInGridCellIsCanvas is the terminal-independent
// gate for the mid-line-bleed bug. It parses the composed Sessions View()
// cell-by-cell in both canvas modes and asserts every visible in-grid cell
// carries an EXPLICIT background — either the canvas itself OR a content
// background (selected-row tint, violet title box). NO cell may fall back to the
// terminal default (a bare 0-reset / 49 region). This is the assertion mosh
// fails without the backfill: the footer inter-column gaps and the title-row gap
// were rendering on the terminal default.
func TestCanvasCellBackground_EveryInGridCellIsCanvas(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode theme.Mode
	}{
		{"dark", theme.Dark},
		{"light", theme.Light},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const w, h = 90, 24
			m := newCanvasTestModel(t, w, h, tc.mode)
			canvasParams := wantCanvasBgParams(t, tc.mode)

			view := m.View().Content
			lines := strings.Split(view, "\n")

			sawCanvas := false
			for li, line := range lines {
				cells := scanCellBackgrounds(line)
				// Every cell up to the grid width must carry an explicit bg.
				for col, c := range cells {
					if col >= w {
						break
					}
					if !c.set {
						t.Fatalf("mode %s line %d col %d: cell has NO explicit background (falls back to terminal default) — mid-line bleed. line=%q",
							tc.name, li, col, strings.ReplaceAll(line, "\x1b", "\\e"))
					}
					if c.params == canvasParams {
						sawCanvas = true
					}
				}
			}
			if !sawCanvas {
				t.Errorf("mode %s: no cell carried the canvas background %q — the canvas paint is absent", tc.name, canvasParams)
			}
		})
	}
}

// TestCanvasCellBackground_TitleAndFooterGaps targets the exact regression sites
// from the mosh/Blink report: the gap directly right of the "Sessions" title box
// (line 0) and the footer inter-column gaps (the keymap rows). It asserts those
// specific rows contain NO unpainted default-background cell anywhere up to the
// grid width.
func TestCanvasCellBackground_TitleAndFooterGaps(t *testing.T) {
	const w, h = 90, 24
	m := newCanvasTestModel(t, w, h, theme.Dark)

	view := m.View().Content
	lines := strings.Split(view, "\n")

	// Title row is line 0; footer rows are the trailing keymap rows. Spot-check
	// the title row plus every line that contains a footer keybinding label.
	check := func(li int, label string) {
		if li < 0 || li >= len(lines) {
			t.Fatalf("expected a line for %s", label)
		}
		for col, c := range scanCellBackgrounds(lines[li]) {
			if col >= w {
				break
			}
			if !c.set {
				t.Fatalf("%s (line %d) col %d unpainted (terminal-default cell): %q",
					label, li, col, strings.ReplaceAll(lines[li], "\x1b", "\\e"))
			}
		}
	}

	check(0, "title row")
	for li, line := range lines {
		if strings.Contains(ansi.Strip(line), "switch view") ||
			strings.Contains(ansi.Strip(line), "go to start") ||
			strings.Contains(ansi.Strip(line), "go to end") {
			check(li, "footer row")
		}
	}
}
