package tui

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/theme"
)

const (
	fakeCardWidth  = 24
	fakeCardHeight = 5
	fakeStackRows  = 3
)

// The ladder is tested against builders that answer with markers rather than
// either screen's content, so a change to what the screens stack cannot decide
// whether the ladder's threshold or its clamp still hold.
type fakePaneScreen struct {
	card        []string
	stack       []string
	cardCalls   int
	stackWidths []int
}

func newFakePaneScreen(cardWidth, cardHeight, stackRows int) *fakePaneScreen {
	return &fakePaneScreen{card: fakeCardRows(cardWidth, cardHeight), stack: fakeStackRowsOf(stackRows)}
}

func (f *fakePaneScreen) parts() paneScreenParts {
	return paneScreenParts{
		card: func() string {
			f.cardCalls++
			return strings.Join(f.card, "\n")
		},
		stack: func(width int) []string {
			f.stackWidths = append(f.stackWidths, width)
			return f.stack
		},
	}
}

func fakeCardRows(width, height int) []string {
	rows := make([]string, 0, height)
	for i := range height {
		label := fmt.Sprintf("card-%d", i)
		rows = append(rows, label+strings.Repeat("=", width-lipgloss.Width(label)))
	}
	return rows
}

func fakeStackRowsOf(n int) []string {
	rows := make([]string, 0, n)
	for i := range n {
		rows = append(rows, fmt.Sprintf("stack-%d", i))
	}
	return rows
}

// Fatal on the line count, so a size assertion never reads rows that are not there.
func paneRows(t *testing.T, out string, w, h int) []string {
	t.Helper()
	lines := strings.Split(out, "\n")
	if len(lines) != h {
		t.Fatalf("the render is %d lines, want %d: %q", len(lines), h, ansi.Strip(out))
	}
	rows := make([]string, 0, h)
	for i, line := range lines {
		if got := lipgloss.Width(line); got != w {
			t.Errorf("line %d renders at width %d, want %d: %q", i, got, w, ansi.Strip(line))
		}
		rows = append(rows, strings.TrimRight(ansi.Strip(line), " "))
	}
	return rows
}

func nonEmptyRows(rows []string) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		if trimmed := strings.TrimSpace(row); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func assertPaneRows(t *testing.T, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the pane reads\n got: %q\nwant: %q", got, want)
	}
}

func assertEveryCellIsCanvas(t *testing.T, out string, w int, canvasParams string) {
	t.Helper()
	for li, line := range strings.Split(out, "\n") {
		cells := scanCellBackgrounds(line)
		if len(cells) != w {
			t.Errorf("line %d paints %d cells, want %d: %q", li, len(cells), w, ansi.Strip(line))
		}
		for col, c := range cells {
			if !c.set {
				t.Fatalf("line %d col %d has no explicit background — the terminal shows through: %q",
					li, col, strings.ReplaceAll(line, "\x1b", "\\e"))
			}
			if c.params != canvasParams {
				t.Fatalf("line %d col %d carries background %q, want the canvas %q", li, col, c.params, canvasParams)
			}
		}
	}
}

func assertNoBackgroundPainted(t *testing.T, out string) {
	t.Helper()
	if strings.Contains(out, "\x1b") {
		t.Errorf("the colourless render carries an escape sequence: %q", strings.ReplaceAll(out, "\x1b", "\\e"))
	}
	for li, line := range strings.Split(out, "\n") {
		for col, c := range scanCellBackgrounds(line) {
			if c.set {
				t.Errorf("line %d col %d carries background %q, want none", li, col, c.params)
			}
		}
	}
}

func TestRenderPaneScreen_SizeLadder(t *testing.T) {
	th := testDarkTheme(t)

	t.Run("it renders the card when the pane fits it", func(t *testing.T) {
		f := newFakePaneScreen(fakeCardWidth, fakeCardHeight, fakeStackRows)
		out := renderPaneScreen(f.parts(), fakeCardWidth, fakeCardHeight, th, false)
		assertPaneRows(t, paneRows(t, out, fakeCardWidth, fakeCardHeight), f.card)
		if f.stackWidths != nil {
			t.Errorf("the stack builder was called at %v, want not called", f.stackWidths)
		}
		if f.cardCalls != 1 {
			t.Errorf("the card builder was called %d times, want 1", f.cardCalls)
		}
	})

	t.Run("it centres the card on a pane larger than it", func(t *testing.T) {
		const w, h = 60, 11
		leftPad := strings.Repeat(" ", (w-fakeCardWidth)/2)
		topPad := (h - fakeCardHeight) / 2

		f := newFakePaneScreen(fakeCardWidth, fakeCardHeight, fakeStackRows)
		out := renderPaneScreen(f.parts(), w, h, th, false)

		want := make([]string, 0, h)
		for range topPad {
			want = append(want, "")
		}
		for _, row := range f.card {
			want = append(want, leftPad+row)
		}
		for len(want) < h {
			want = append(want, "")
		}
		assertPaneRows(t, paneRows(t, out, w, h), want)
	})

	t.Run("it renders the plain stack when the pane is one column short", func(t *testing.T) {
		f := newFakePaneScreen(fakeCardWidth, fakeCardHeight, fakeStackRows)
		const w = fakeCardWidth - 1
		out := renderPaneScreen(f.parts(), w, fakeCardHeight, th, false)
		assertPaneRows(t, paneRows(t, out, w, fakeCardHeight), append(f.stack, "", ""))
	})

	t.Run("it renders the plain stack when the pane is one row short", func(t *testing.T) {
		f := newFakePaneScreen(fakeCardWidth, fakeCardHeight, fakeStackRows)
		const h = fakeCardHeight - 1
		out := renderPaneScreen(f.parts(), fakeCardWidth, h, th, false)
		assertPaneRows(t, paneRows(t, out, fakeCardWidth, h), append(f.stack, ""))
	})

	t.Run("it measures the threshold off the card it just built", func(t *testing.T) {
		fits := newFakePaneScreen(fakeCardWidth, fakeCardHeight, fakeStackRows)
		wider := newFakePaneScreen(fakeCardWidth+1, fakeCardHeight, fakeStackRows)

		atCard := renderPaneScreen(fits.parts(), fakeCardWidth, fakeCardHeight, th, false)
		assertPaneRows(t, nonEmptyRows(paneRows(t, atCard, fakeCardWidth, fakeCardHeight)), fits.card)

		atWider := renderPaneScreen(wider.parts(), fakeCardWidth, fakeCardHeight, th, false)
		assertPaneRows(t, nonEmptyRows(paneRows(t, atWider, fakeCardWidth, fakeCardHeight)), wider.stack)
	})

	t.Run("it calls the stack builder at the pane's width", func(t *testing.T) {
		const w = 40
		f := newFakePaneScreen(w+1, fakeCardHeight, fakeStackRows)
		out := renderPaneScreen(f.parts(), w, fakeCardHeight, th, false)
		if !reflect.DeepEqual(f.stackWidths, []int{w}) {
			t.Errorf("the stack builder was called at %v, want [%d]", f.stackWidths, w)
		}
		assertPaneRows(t, nonEmptyRows(paneRows(t, out, w, fakeCardHeight)), f.stack)
	})

	t.Run("it renders at the fallback size for a non-positive dimension", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			w, h  int
			wantW int
			wantH int
			want  []string
		}{
			{"0x0", 0, 0, fallbackTermWidth, fallbackTermHeight, nil},
			{"-1x10", -1, 10, fallbackTermWidth, 10, nil},
			{"10x-1", 10, -1, 10, fallbackTermHeight, []string{"stack-0", "stack-1", "stack-2"}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				f := newFakePaneScreen(fakeCardWidth, fakeCardHeight, fakeStackRows)
				want := tc.want
				if want == nil {
					want = f.card
				}
				out := renderPaneScreen(f.parts(), tc.w, tc.h, th, false)
				assertPaneRows(t, nonEmptyRows(paneRows(t, out, tc.wantW, tc.wantH)), want)
			})
		}
	})
}

func paneSizeCases(f *fakePaneScreen) []struct {
	name string
	w, h int
	want []string
} {
	return []struct {
		name string
		w, h int
		want []string
	}{
		{"1x1", 1, 1, []string{"s"}},
		{"one column short", fakeCardWidth - 1, fakeCardHeight, f.stack},
		{"one row short", fakeCardWidth, fakeCardHeight - 1, f.stack},
		{"exactly fits", fakeCardWidth, fakeCardHeight, f.card},
		{"larger than the card", 80, 24, f.card},
	}
}

func TestRenderPaneScreen_PaintsEveryCell(t *testing.T) {
	forEachBuiltinTheme(t, func(t *testing.T, th theme.Theme) {
		canvasParams := wantCanvasBgParams(t, th)
		for _, tc := range paneSizeCases(newFakePaneScreen(fakeCardWidth, fakeCardHeight, fakeStackRows)) {
			t.Run(tc.name, func(t *testing.T) {
				f := newFakePaneScreen(fakeCardWidth, fakeCardHeight, fakeStackRows)
				out := renderPaneScreen(f.parts(), tc.w, tc.h, th, false)
				assertPaneRows(t, nonEmptyRows(paneRows(t, out, tc.w, tc.h)), tc.want)
				assertEveryCellIsCanvas(t, out, tc.w, canvasParams)
			})
		}
	})
}

func TestRenderPaneScreen_Colourless(t *testing.T) {
	forEachBuiltinTheme(t, func(t *testing.T, th theme.Theme) {
		for _, tc := range paneSizeCases(newFakePaneScreen(fakeCardWidth, fakeCardHeight, fakeStackRows)) {
			t.Run("it keeps the layout and paints no canvas under NO_COLOR/"+tc.name, func(t *testing.T) {
				painted := newFakePaneScreen(fakeCardWidth, fakeCardHeight, fakeStackRows)
				bare := newFakePaneScreen(fakeCardWidth, fakeCardHeight, fakeStackRows)

				withCanvas := renderPaneScreen(painted.parts(), tc.w, tc.h, th, false)
				colourless := renderPaneScreen(bare.parts(), tc.w, tc.h, th, true)

				assertPaneRows(t, nonEmptyRows(paneRows(t, colourless, tc.w, tc.h)), tc.want)
				if ansi.Strip(withCanvas) != colourless {
					t.Errorf("the colourless render differs from the painted one's text\n got: %q\nwant: %q",
						colourless, ansi.Strip(withCanvas))
				}
				assertNoBackgroundPainted(t, colourless)
			})
		}
	})
}

func TestRenderPaneScreen_ClampsToThePane(t *testing.T) {
	th := testDarkTheme(t)
	const w = 20

	t.Run("it clamps content taller than the pane", func(t *testing.T) {
		f := newFakePaneScreen(w+8, fakeCardHeight, 20)
		out := renderPaneScreen(f.parts(), w, 5, th, false)
		paneRows(t, out, w, 5)
	})

	t.Run("it clamps a stack row wider than the pane", func(t *testing.T) {
		f := newFakePaneScreen(w+8, fakeCardHeight, fakeStackRows)
		f.stack[1] = strings.Repeat("x", w+30)
		out := renderPaneScreen(f.parts(), w, fakeCardHeight, th, false)
		assertPaneRows(t, nonEmptyRows(paneRows(t, out, w, fakeCardHeight)),
			[]string{"stack-0", strings.Repeat("x", w), "stack-2"})
	})

	t.Run("it truncates rather than re-flows the degraded stack", func(t *testing.T) {
		rows := []string{"a-b-c-d-e-f -- --port=3000 --resume", "resume  d  discard"}
		clamped := clampStackToPane(rows, 12, len(rows))
		assertPaneRows(t, clamped, []string{"a-b-c-d-e-f ", "resume  d  d"})
	})

	t.Run("it keeps the stack's first and last rows when it clamps", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			rows int
			h    int
			want []string
		}{
			{"twenty rows in five", 20, 5, []string{"stack-0", "stack-1", "stack-2", "stack-3", "stack-19"}},
			{"four rows in three", 4, 3, []string{"stack-0", "stack-1", "stack-3"}},
			{"two rows in one", 2, 1, []string{"stack-0"}},
			{"six rows in three", 6, 3, []string{"stack-0", "stack-1", "stack-5"}},
			{"six rows in two", 6, 2, []string{"stack-0", "stack-5"}},
			{"six rows in one", 6, 1, []string{"stack-0"}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				f := newFakePaneScreen(w+8, fakeCardHeight, tc.rows)
				out := renderPaneScreen(f.parts(), w, tc.h, th, false)
				assertPaneRows(t, paneRows(t, out, w, tc.h), tc.want)
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
