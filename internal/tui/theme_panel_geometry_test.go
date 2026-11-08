package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/theme"
)

func TestPanelGeometry_WidthLadder(t *testing.T) {
	for _, tc := range []struct {
		name     string
		contentW int
		want     int
	}{
		{name: "far wider than the affordance", contentW: 200, want: themePanelPreferredWidth},
		{name: "exactly at the affordance", contentW: 2 * themePanelPreferredWidth, want: themePanelPreferredWidth},
		{name: "one column below the affordance", contentW: 2*themePanelPreferredWidth - 1, want: themePanelMinWidth},
		{name: "mid range", contentW: 40, want: themePanelMinWidth},
		{name: "exactly at the floor", contentW: themePanelMinWidth, want: themePanelMinWidth},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := themePanelWidthFor(tc.contentW)
			if got != tc.want {
				t.Errorf("themePanelWidthFor(%d) = %d, want %d", tc.contentW, got, tc.want)
			}
			if !ok {
				t.Errorf("themePanelWidthFor(%d) refused; the panel renders at every width down to %d", tc.contentW, themePanelMinWidth)
			}
		})
	}

	t.Run("it takes two widths and nothing between them", func(t *testing.T) {
		prev := themePanelPreferredWidth
		steps := 0
		for contentW := 200; contentW >= themePanelMinWidth; contentW-- {
			got, ok := themePanelWidthFor(contentW)
			if !ok {
				t.Fatalf("themePanelWidthFor(%d) refused above the floor", contentW)
			}
			if got != themePanelMinWidth && got != themePanelPreferredWidth {
				t.Fatalf("themePanelWidthFor(%d) = %d, want one of the ladder's two stages %d or %d", contentW, got, themePanelMinWidth, themePanelPreferredWidth)
			}
			if got > prev {
				t.Fatalf("narrowing the content region to %d WIDENED the panel from %d to %d", contentW, prev, got)
			}
			if got != prev {
				steps++
			}
			prev = got
		}
		if prev != themePanelMinWidth {
			t.Errorf("the bottom of the ladder is %d, want the minimum %d", prev, themePanelMinWidth)
		}
		if steps != 1 {
			t.Errorf("the panel changed width %d times across the range, want exactly 1 — the ladder is staged, not proportional", steps)
		}
	})
}

func TestPanelGeometry_TerminalEnds(t *testing.T) {
	t.Run("the min-width terminal steps the panel down, one column wider does not", func(t *testing.T) {
		term := ThemePanelMinWidthTerminal()

		if got, ok := themePanelWidthFor(insetRegion(term, Hinset)); !ok || got != themePanelMinWidth {
			t.Errorf("at %d columns the panel renders %d wide (ok=%v), want the stepped-down %d", term, got, ok, themePanelMinWidth)
		}
		if got, _ := themePanelWidthFor(insetRegion(term+1, Hinset)); got != themePanelPreferredWidth {
			t.Errorf("at %d columns the panel renders %d wide, want the preferred %d — the declared width is the WIDEST terminal that steps down", term+1, got, themePanelPreferredWidth)
		}
	})

	t.Run("the floor terminal opens the panel, one row shorter refuses", func(t *testing.T) {
		term := ThemePanelFloorTerminalHeight()
		contentW := insetRegion(ThemePanelMinWidthTerminal(), Hinset)

		if dim, ok := themePanelFloor(contentW, insetRegion(term, Vinset), false); !ok {
			t.Errorf("at %d rows the gate refuses on %v, want the panel to open", term, dim)
		}
		if dim, ok := themePanelFloor(contentW, insetRegion(term-1, Vinset), false); ok || dim != dimHeight {
			t.Errorf("at %d rows the gate reports (%v, ok=%v), want a height refusal — the declared height is the SHORTEST terminal that opens", term-1, dim, ok)
		}
	})
}

func TestPanelGeometry_WidthFloor(t *testing.T) {
	for contentW := themePanelMinWidth - 1; contentW >= 0; contentW-- {
		got, ok := themePanelWidthFor(contentW)
		if ok {
			t.Errorf("themePanelWidthFor(%d) accepted a content region below the %d-column minimum", contentW, themePanelMinWidth)
		}
		if got != themePanelMinWidth {
			t.Errorf("themePanelWidthFor(%d) = %d on the refusing path, want the defensively clamped %d", contentW, got, themePanelMinWidth)
		}
	}
}

// Written out rather than read from the production arithmetic: a floor asserted
// against whatever the header measures pins nothing.
const wantPanelHeaderRows = 2

// The message row is counted although the slot is unreserved when empty: both of
// its contenders are non-suppressible, and a floor without it lands one row short.
func TestPanelGeometry_HeightFloorArithmetic(t *testing.T) {
	entries := themePanelKeymap()
	footer := themePanelFooterHeight(entries)
	if footer != lipgloss.Height(renderThemePanelFooter(entries, 0, testDarkTheme(t), false)) {
		t.Fatalf("fixture: themePanelFooterHeight(%d) is not the rendered footer's height", footer)
	}

	const listRow, messageRow = 1, 1
	want := wantPanelHeaderRows + footer + listRow + messageRow
	if got := themePanelMinHeight(false); got != want {
		t.Errorf("themePanelMinHeight = %d, want header(%d) + footer(%d) + %d list row + %d message row = %d",
			got, wantPanelHeaderRows, footer, listRow, messageRow, want)
	}
	if got, wantDir := themePanelMinHeight(true), want+1; got != wantDir {
		t.Errorf("themePanelMinHeight with an unusable directory = %d, want %d — the same composition plus the pinned row", got, wantDir)
	}
}

func TestPanelGeometry_ChromeRowsIsSharedByBothArithmetics(t *testing.T) {
	t.Run("the floor is the chrome sum plus its guaranteed list row", func(t *testing.T) {
		for _, dirUnusable := range []bool{false, true} {
			want := themePanelChromeRows(themePanelCompactHeaderRows, dirUnusable, themePanelFloorMessageRows) + themePanelMinBodyRows
			if got := themePanelMinHeight(dirUnusable); got != want {
				t.Errorf("themePanelMinHeight(dirUnusable=%v) = %d, want the chrome sum plus its %d list row = %d",
					dirUnusable, got, themePanelMinBodyRows, want)
			}
		}
	})

	t.Run("the body is the render height less the chrome sum", func(t *testing.T) {
		for _, tc := range []struct {
			name        string
			dirUnusable bool
			message     themePanelMessage
		}{
			{name: "an empty slot"},
			{name: "an unusable directory", dirUnusable: true},
			{name: "a raised message", message: themePanelMessage{Kind: themeMessageCommitFailed}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				p := newThemePanelFixture(themePanelFixtureOpts{
					th:          testDarkTheme(t),
					width:       themePanelPreferredWidth,
					rows:        themePanelTestRows(12),
					dirUnusable: tc.dirUnusable,
					message:     tc.message,
				})

				messageRows := themePanelMessageHeight(p.message)
				want := geometryContentH - themePanelChromeRows(
					themePanelHeaderRows(geometryContentH, p.bandRows, tc.dirUnusable),
					tc.dirUnusable, messageRows)
				if want <= themePanelMinBodyRows {
					t.Fatalf("fixture: a %d-row content region leaves a %d-row body, at or below the %d-row floor — the remainder is clamped, so the assertion proves nothing",
						geometryContentH, want, themePanelMinBodyRows)
				}

				if _, got := themePanelListSize(p, geometryContentH); got != want {
					t.Errorf("the list body at %d rows = %d, want the height less the chrome sum = %d",
						geometryContentH, got, want)
				}
			})
		}
	})
}

func TestPanelGeometry_DirRowRaisesTheFloor(t *testing.T) {
	usable := themePanelMinHeight(false)
	unusable := themePanelMinHeight(true)

	if got := unusable - usable; got != 1 {
		t.Errorf("an unusable themes directory raises the floor by %d row(s), want exactly 1", got)
	}
	if got := themePanelDirRowHeight(true); got != 1 {
		t.Fatalf("fixture: the directory row measures %d rows, so the floor delta says nothing", got)
	}
	if got := themePanelDirRowHeight(false); got != 0 {
		t.Fatalf("fixture: a usable directory measures %d rows, want 0", got)
	}
}

func TestPanelGeometry_FloorReportsWidthFirst(t *testing.T) {
	tallEnough := themePanelMinHeight(false)

	for _, tc := range []struct {
		name        string
		contentW    int
		contentH    int
		dirUnusable bool
		wantDim     themePanelDim
		wantOK      bool
	}{
		{name: "both dimensions clear", contentW: 80, contentH: tallEnough, wantDim: dimNone, wantOK: true},
		{name: "exactly at both floors", contentW: themePanelMinWidth, contentH: tallEnough, wantDim: dimNone, wantOK: true},
		{name: "too narrow only", contentW: themePanelMinWidth - 1, contentH: tallEnough, wantDim: dimWidth},
		{name: "too short only", contentW: 80, contentH: tallEnough - 1, wantDim: dimHeight},
		{name: "both fail", contentW: themePanelMinWidth - 1, contentH: 0, wantDim: dimWidth},
		{name: "an unusable directory takes the last row", contentW: 80, contentH: tallEnough, dirUnusable: true, wantDim: dimHeight},
		{name: "an unusable directory with its row to spare", contentW: 80, contentH: tallEnough + 1, dirUnusable: true, wantDim: dimNone, wantOK: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dim, ok := themePanelFloor(tc.contentW, tc.contentH, tc.dirUnusable)
			if ok != tc.wantOK {
				t.Fatalf("themePanelFloor(%d, %d, %v) ok = %v, want %v", tc.contentW, tc.contentH, tc.dirUnusable, ok, tc.wantOK)
			}
			if dim != tc.wantDim {
				t.Errorf("themePanelFloor(%d, %d, %v) reported %v, want %v", tc.contentW, tc.contentH, tc.dirUnusable, dim, tc.wantDim)
			}
		})
	}

	t.Run("the resize path decides by this predicate alone", func(t *testing.T) {
		for _, region := range [][2]int{
			{geometryWideW, geometryContentH},
			{themePanelMinWidth, tallEnough},
			{themePanelMinWidth - 1, tallEnough},
			{geometryWideW, tallEnough - 1},
			{themePanelMinWidth - 1, 1},
			{60, tallEnough},
		} {
			contentW, contentH := region[0], region[1]
			t.Run(fmt.Sprintf("%dx%d", contentW, contentH), func(t *testing.T) {
				m, cmd := resizeForTestCmd(t, newGeometryPanelModel(t, geometryWideW, geometryContentH), contentW, contentH)
				if got := m.contentWidth(); got != contentW {
					t.Fatalf("fixture: the resized content region is %d columns, want %d", got, contentW)
				}
				if got := m.contentHeight(); got != contentH {
					t.Fatalf("fixture: the resized content region is %d rows, want %d", got, contentH)
				}

				dim, ok := themePanelFloor(contentW, contentH, false)
				if m.themePanel.open != ok {
					t.Fatalf("the panel is open=%v after the resize, want %v — the predicate said (%v, ok=%v)", m.themePanel.open, ok, dim, ok)
				}
				if ok {
					if got := m.flashText; got != "" {
						t.Errorf("a resize above the floor raised %q, want no flash", got)
					}
					return
				}
				if got, want := m.flashText, themePanelForcedCloseFlash(dim); got != want {
					t.Errorf("the forced close raised %q, want the %v copy %q", got, dim, want)
				}
				// Presence only: evaluating the auto-clear tick it carries blocks for
				// flashAutoClearDuration, once per region.
				if cmd == nil {
					t.Error("the forced close returned no command, so its flash never auto-clears")
				}
			})
		}
	})
}

// CONTENT widths, which is what the ladder is a function of: a change to the page
// gutter must move the fixtures rather than the band they sit in.
const (
	geometryWideW         = 96
	geometryDegradedW     = 52
	geometryDegradedPanel = themePanelMinWidth
	geometryContentH      = 26
)

// 24 cells: inside a badged row's label budget at the preferred width and beyond
// it at the stepped-down one, so a stale delegate shows a missing ellipsis.
const geometryLabel = "aurora-midnight-drifting"

func geometryTerm(contentW, contentH int) (termW, termH int) {
	return contentW + 2*Hinset, contentH + 2*Vinset
}

func geometryRows(t *testing.T) []theme.Row {
	t.Helper()
	rows := []theme.Row{arrowValidRow(t, geometryLabel, 0)}
	for i := 1; i < 5; i++ {
		rows = append(rows, arrowValidRow(t, arrowSlug(i), i))
	}
	return rows
}

func newGeometryPanelModel(t *testing.T, contentW, contentH int) Model {
	t.Helper()
	m := Build(newArrowPanelDeps(t, geometryRows(t), geometryLabel))
	return openPanelForTest(t, m, contentW, contentH)
}

func resizeForTest(t *testing.T, m Model, contentW, contentH int) Model {
	t.Helper()
	m, _ = resizeForTestCmd(t, m, contentW, contentH)
	return m
}

// A resize is handled in a pre-step of Update, so a command raised there reaches
// the runtime only by being folded onto whichever arm fires — hence the return.
func resizeForTestCmd(t *testing.T, m Model, contentW, contentH int) (Model, tea.Cmd) {
	t.Helper()
	termW, termH := geometryTerm(contentW, contentH)
	updated, cmd := m.Update(tea.WindowSizeMsg{Width: termW, Height: termH})
	return updated.(Model), cmd
}

func requireRenderedPanelWidth(t *testing.T, m Model, want int) {
	t.Helper()
	if got := m.themePanel.width; got != want {
		t.Fatalf("the panel's width is %d, want %d", got, want)
	}

	contentW, contentH := m.contentWidth(), m.contentHeight()
	panelLines := strings.Split(ansi.Strip(renderThemePanel(m.themePanel, contentH, m.themeState.active, m.colourless)), "\n")
	if len(panelLines) != contentH {
		t.Fatalf("the panel rendered %d rows, want the content height %d", len(panelLines), contentH)
	}
	for i, line := range panelLines {
		if got := lipgloss.Width(line); got != want {
			t.Errorf("panel row %d is %d cells, want %d: %q", i, got, want, line)
		}
	}

	frame := strings.Split(ansi.Strip(m.View().Content), "\n")
	_, _, topPad, _ := gutterPadding(m.termWidth, m.termHeight, contentW, contentH)
	left := (m.termWidth-contentW)/2 + contentW - want
	for j, wantLine := range panelLines {
		row := []rune(frame[topPad+j])
		if len(row) != m.termWidth {
			t.Fatalf("frame row %d is %d cells, want %d", topPad+j, len(row), m.termWidth)
		}
		if got := string(row[left : left+want]); got != wantLine {
			t.Errorf("frame row %d under the panel = %q, want the panel row %q", topPad+j, got, wantLine)
		}
	}
}

func TestPanelGeometry_OpenUsesTheWidthLadder(t *testing.T) {
	t.Run("a wide terminal takes the preferred width", func(t *testing.T) {
		m := newGeometryPanelModel(t, geometryWideW, geometryContentH)
		requireRenderedPanelWidth(t, m, themePanelPreferredWidth)
	})

	t.Run("a terminal below the affordance opens already narrowed", func(t *testing.T) {
		want, ok := themePanelWidthFor(geometryDegradedW)
		if !ok || want != geometryDegradedPanel {
			t.Fatalf("fixture: a %d-column content region gives a %d-cell panel (ok=%v), want %d", geometryDegradedW, want, ok, geometryDegradedPanel)
		}
		if want >= themePanelPreferredWidth {
			t.Fatalf("fixture: %d is not below the preferred width, so the assertion below cannot fail", want)
		}

		m := newGeometryPanelModel(t, geometryDegradedW, geometryContentH)
		requireRenderedPanelWidth(t, m, want)
	})
}

func TestPanelGeometry_OpenAndResizeWidthsAgree(t *testing.T) {
	for _, contentW := range []int{geometryWideW, 60, 59, geometryDegradedW, 48, 40, themePanelMinWidth} {
		t.Run(fmt.Sprintf("contentW=%d", contentW), func(t *testing.T) {
			want, ok := themePanelWidthFor(contentW)
			if !ok {
				t.Fatalf("fixture: a %d-column content region is below the floor", contentW)
			}

			opened := newGeometryPanelModel(t, contentW, geometryContentH)
			resized := resizeForTest(t, newGeometryPanelModel(t, geometryWideW, geometryContentH), contentW, geometryContentH)

			if got := opened.themePanel.width; got != want {
				t.Errorf("the panel OPENED at %d cells, want the ladder's %d", got, want)
			}
			if got := resized.themePanel.width; got != want {
				t.Errorf("the panel RESIZED to %d cells, want the ladder's %d", got, want)
			}

			openW, openH := opened.themePanel.list.Width(), opened.themePanel.list.Height()
			resizeW, resizeH := resized.themePanel.list.Width(), resized.themePanel.list.Height()
			if openW != resizeW || openH != resizeH {
				t.Errorf("the opened panel's list is %dx%d and the resized one's is %dx%d; one ladder, one body arithmetic", openW, openH, resizeW, resizeH)
			}
		})
	}
}

// `bubbles/list` derives Paginator.PerPage from the height it is handed, so a
// stale list pages by the old amount while the drawn page is the new one.
func requirePanelListMatchesTheRenderCopy(t *testing.T, m Model) {
	t.Helper()
	wantW, wantH := themePanelListSize(m.themePanel, m.contentHeight())
	gotW, gotH := m.themePanel.list.Width(), m.themePanel.list.Height()
	if gotW != wantW || gotH != wantH {
		t.Errorf("the model's panel list is sized %dx%d, want the render copy's %dx%d", gotW, gotH, wantW, wantH)
	}

	if got, want := m.themePanel.list.Paginator.PerPage, themePanelDrawnPerPage(m); got != want {
		t.Errorf("the model's panel list paginates %d row(s) per page and the drawn page holds %d — Ctrl+↑/↓ moves by the stale amount", got, want)
	}
}

func themePanelDrawnPerPage(m Model) int {
	copied := m.themePanel
	copied.list.SetSize(themePanelListSize(copied, m.contentHeight()))
	return copied.list.Paginator.PerPage
}

func TestPanelGeometry_ResizeDegradesInPlace(t *testing.T) {
	t.Run("narrowing shrinks the panel and keeps it open", func(t *testing.T) {
		m := newGeometryPanelModel(t, geometryWideW, geometryContentH)
		requireRenderedPanelWidth(t, m, themePanelPreferredWidth)

		m = resizeForTest(t, m, geometryDegradedW, geometryContentH)

		if !m.themePanel.open {
			t.Fatal("a resize above the floor closed the panel; it degrades in place")
		}
		requireRenderedPanelWidth(t, m, geometryDegradedPanel)
		requirePanelListMatchesTheRenderCopy(t, m)
	})

	t.Run("widening restores the preferred width", func(t *testing.T) {
		m := newGeometryPanelModel(t, geometryDegradedW, geometryContentH)
		requireRenderedPanelWidth(t, m, geometryDegradedPanel)

		m = resizeForTest(t, m, geometryWideW, geometryContentH)

		if !m.themePanel.open {
			t.Fatal("a resize above the floor closed the panel")
		}
		requireRenderedPanelWidth(t, m, themePanelPreferredWidth)
		requirePanelListMatchesTheRenderCopy(t, m)
	})

	t.Run("a taller terminal re-derives the panel's page", func(t *testing.T) {
		shortH := themePanelMinHeight(false)
		m := newGeometryPanelModel(t, geometryWideW, shortH)
		short := m.themePanel.list.Paginator.PerPage
		requirePanelListMatchesTheRenderCopy(t, m)

		m = resizeForTest(t, m, geometryWideW, geometryContentH)

		if !m.themePanel.open {
			t.Fatal("a taller terminal closed the panel")
		}
		if drawn := themePanelDrawnPerPage(m); drawn == short {
			t.Fatalf("fixture: the drawn page is %d rows at both %d and %d content rows, so the assertion below proves nothing", drawn, shortH, geometryContentH)
		}
		requirePanelListMatchesTheRenderCopy(t, m)
		if got := lipgloss.Height(renderThemePanel(m.themePanel, m.contentHeight(), m.themeState.active, m.colourless)); got != geometryContentH {
			t.Errorf("the panel rendered %d rows, want the content height %d", got, geometryContentH)
		}
	})
}

func themePanelBodyRow(t *testing.T, m Model, offset int) string {
	t.Helper()
	lines := themePanelLines(renderThemePanel(m.themePanel, m.contentHeight(), m.themeState.active, m.colourless))
	at := panelHeaderRowsOf(m) + themePanelDirRowHeight(m.themePanel.union.DirUnusable) + offset
	if at >= len(lines) {
		t.Fatalf("the panel rendered %d rows, so body row %d does not exist", len(lines), offset)
	}
	return lines[at]
}

// No arrow is pressed between the resize and the assertion — an arrow would
// re-point the delegate and hide the defect.
func TestPanelGeometry_ResizeRepointsTheDelegate(t *testing.T) {
	m := newGeometryPanelModel(t, geometryWideW, geometryContentH)
	requireCursorOn(t, m, geometryLabel)
	if got := themePanelRowFor(t, m, geometryLabel).Badge; got != theme.BadgeConstant {
		t.Fatalf("fixture: the cursor row carries badge %v, want the constant `●` the right edge competes for", got)
	}

	before := themePanelBodyRow(t, m, 0)
	if !strings.Contains(before, geometryLabel) {
		t.Fatalf("fixture: the %d-cell label is not rendered in full at the preferred width: %q", lipgloss.Width(geometryLabel), before)
	}
	if strings.Contains(before, themeRowEllipsis) {
		t.Fatalf("fixture: the label is already truncated at the preferred width, so a truncation below proves nothing: %q", before)
	}

	m = resizeForTest(t, m, geometryDegradedW, geometryContentH)

	after := themePanelBodyRow(t, m, 0)
	if !strings.Contains(after, themeRowEllipsis) {
		t.Errorf("after narrowing with no arrow the cursor row = %q, want it truncated with %q — the delegate is still composing against the pre-resize budget", after, themeRowEllipsis)
	}
	if strings.Contains(after, geometryLabel) {
		t.Errorf("the cursor row still carries the full %d-cell label at a %d-cell panel: %q", lipgloss.Width(geometryLabel), m.themePanel.width, after)
	}
	if !strings.HasSuffix(after, themePanelBadgeText(theme.BadgeConstant)) {
		t.Errorf("the cursor row = %q, want it to end in the %q badge — the badge outranks the label and stays inside the inner edge", after, themePanelBadgeText(theme.BadgeConstant))
	}
	for i, line := range themePanelLines(renderThemePanel(m.themePanel, m.contentHeight(), m.themeState.active, m.colourless)) {
		if got := lipgloss.Width(line); got != m.themePanel.width {
			t.Errorf("panel row %d is %d cells, want the panel's %d: %q", i, got, m.themePanel.width, line)
		}
	}
}

func TestPanelGeometry_ResizeDoesNotReflowTheBase(t *testing.T) {
	m := newGeometryPanelModel(t, geometryWideW, geometryContentH)
	m = resizeForTest(t, m, geometryDegradedW, geometryContentH)
	if !m.themePanel.open {
		t.Fatal("fixture: the resize closed the panel")
	}

	control := closeThemePanelForTest(t, newGeometryPanelModel(t, geometryWideW, geometryContentH))
	control = resizeForTest(t, control, geometryDegradedW, geometryContentH)
	if control.themePanel.open {
		t.Fatal("fixture: the control still has a panel open")
	}

	wantW, wantH := control.SessionListSize()
	if gotW, gotH := m.SessionListSize(); gotW != wantW || gotH != wantH {
		t.Errorf("with the panel open the sessions list resized to %dx%d, want the unreduced %dx%d the same resize gives with no panel", gotW, gotH, wantW, wantH)
	}
	if wantW <= m.themePanel.width {
		t.Fatalf("fixture: the unreduced list width %d is not wider than the panel's %d, so a reduction would be invisible", wantW, m.themePanel.width)
	}

	contentW := m.contentWidth()
	footer := strings.Split(ansi.Strip(renderSessionsFooter(m.sessionsHelpKeymap(), contentW, m.themeState.active, m.colourless)), "\n")
	unreduced := []rune(footer[len(footer)-1])
	cut := contentW - m.themePanel.width
	if unreduced[cut-1] == ' ' || unreduced[cut] == ' ' {
		t.Fatalf("fixture: the panel's left border falls on a word boundary at col %d (%q) — the cut-mid-label case is not exercised",
			cut, string(unreduced[max(cut-8, 0):min(cut+8, len(unreduced))]))
	}

	frame := strings.Split(ansi.Strip(m.View().Content), "\n")
	_, _, topPad, _ := gutterPadding(m.termWidth, m.termHeight, contentW, m.contentHeight())
	covered := []rune(frame[topPad+m.contentHeight()-1])
	left := (m.termWidth - contentW) / 2
	if got, want := string(covered[left:left+cut]), string(unreduced[:cut]); got != want {
		t.Errorf("the covered footer reflowed to the reduced width: got %q, want the unreduced footer's first %d cells %q", got, cut, want)
	}
}

// Verbatim, not the production constants: a test asserting a constant against
// itself pins nothing.
const (
	wantNarrowClosedFlash = "terminal too narrow — theme picker closed"
	wantShortClosedFlash  = "terminal too short — theme picker closed"
)

func geometryBelowWidthFloor() (contentW, contentH int) {
	return themePanelMinWidth - 1, geometryContentH
}

func geometryBelowHeightFloor() (contentW, contentH int) {
	return geometryWideW, themePanelMinHeight(false) - 1
}

func requireForcedClose(t *testing.T, m Model, wantFlash string) {
	t.Helper()
	if m.themePanel.open {
		t.Fatal("a resize below the floor left the panel open")
	}
	if got := m.themePanel; got.width != 0 || len(got.union.Rows) != 0 || len(got.enumeration.Entries) != 0 || got.badges != nil {
		t.Errorf("the forced close retained panel state %+v, want the zero value — it takes the `Esc` path exactly", got)
	}
	if got := m.flashText; got != wantFlash {
		t.Errorf("the forced close raised %q, want %q", got, wantFlash)
	}
}

func TestPanelGeometry_ResizeBelowWidthFloorClosesWithFlash(t *testing.T) {
	m := newGeometryPanelModel(t, geometryWideW, geometryContentH)
	contentW, contentH := geometryBelowWidthFloor()
	if dim, ok := themePanelFloor(contentW, contentH, false); ok || dim != dimWidth {
		t.Fatalf("fixture: a %dx%d content region reports (%v, ok=%v), want (dimWidth, false)", contentW, contentH, dim, ok)
	}

	m = resizeForTest(t, m, contentW, contentH)

	requireForcedClose(t, m, wantNarrowClosedFlash)
	if got := themePanelNarrowClosedFlash; got != wantNarrowClosedFlash {
		t.Errorf("the pinned constant is %q, want %q", got, wantNarrowClosedFlash)
	}
}

// The terminal is left wide, so the copy is the height dimension's rather than
// the width-first answer a terminal failing both would get.
func TestPanelGeometry_ResizeBelowHeightFloorClosesWithFlash(t *testing.T) {
	m := newGeometryPanelModel(t, geometryWideW, geometryContentH)
	contentW, contentH := geometryBelowHeightFloor()
	if dim, ok := themePanelFloor(contentW, contentH, false); ok || dim != dimHeight {
		t.Fatalf("fixture: a %dx%d content region reports (%v, ok=%v), want (dimHeight, false)", contentW, contentH, dim, ok)
	}

	m = resizeForTest(t, m, contentW, contentH)

	requireForcedClose(t, m, wantShortClosedFlash)
	if got := themePanelShortClosedFlash; got != wantShortClosedFlash {
		t.Errorf("the pinned constant is %q, want %q", got, wantShortClosedFlash)
	}
	if got := ansi.Strip(m.View().Content); !strings.Contains(got, wantShortClosedFlash) {
		t.Errorf("the post-close frame carries no %q band:\n%s", wantShortClosedFlash, got)
	}
}

// `bubbles/list` derives its page when SetSize runs and that derivation is
// path-dependent, so the band is driven through the control, not cleared here.
func TestPanelGeometry_ForcedCloseIsTheEscPath(t *testing.T) {
	contentW, contentH := geometryBelowHeightFloor()
	rows := geometryRows(t)

	previewed := func() Model {
		m := newGeometryPanelModel(t, geometryWideW, geometryContentH)
		m = pressPanelKey(t, m, arrowDown)
		if m.themeState.active == rows[0].Theme {
			t.Fatal("fixture: the arrow previewed nothing, so the two closes have nothing to discard")
		}
		return m
	}

	forced := resizeForTest(t, previewed(), contentW, contentH)
	viaEsc := resizeForTest(t, closeThemePanelForTest(t, previewed()), contentW, contentH)

	if forced.themePanel.open {
		t.Fatal("the forced close left the panel open")
	}
	if forced.themeState.active != rows[0].Theme {
		t.Errorf("the forced close rendered canvas %s, want the resolved persisted %s", forced.themeState.active.Canvas.Value, rows[0].Theme.Canvas.Value)
	}
	if forced.themeState.active != viaEsc.themeState.active {
		t.Errorf("the forced close rendered canvas %s and `Esc` rendered %s", forced.themeState.active.Canvas.Value, viaEsc.themeState.active.Canvas.Value)
	}
	if got := len(forced.themePanel.union.Rows); got != 0 {
		t.Errorf("the forced close retained %d union row(s), want the enumeration discarded", got)
	}

	if got := forced.flashText; got == "" {
		t.Fatal("fixture: the forced close raised no flash, so the control's band history matches nothing")
	}
	(&viaEsc).setFlash(forced.flashText)
	(&forced).clearFlash()
	(&viaEsc).clearFlash()
	if got, want := forced.View().Content, viaEsc.View().Content; got != want {
		t.Errorf("the forced close's frame is not `Esc`'s\nforced: %q\nesc:    %q", escSeq(got), escSeq(want))
	}
}

func TestPanelGeometry_ForcedCloseWritesNothing(t *testing.T) {
	mode := &countingModePersister{}
	committer := &fakeThemePersister{}
	m := newGeometryPanelModel(t, geometryWideW, geometryContentH)
	WithModePersister(mode)(&m)
	WithThemePersister(committer)(&m)
	m = pressPanelKey(t, m, arrowDown)

	contentW, contentH := geometryBelowWidthFloor()
	m = resizeForTest(t, m, contentW, contentH)
	if m.themePanel.open {
		t.Fatal("fixture: the resize did not force-close the panel")
	}

	if mode.calls != 0 {
		t.Errorf("the forced close persisted %d preference(s); every write is an explicit keypress", mode.calls)
	}
	if len(committer.slugs) != 0 {
		t.Errorf("the forced close committed %v; nothing writes on a close", committer.slugs)
	}

	m = pressPanelKey(t, m, tea.KeyPressMsg{Code: 's', Text: "s"})
	if mode.calls != 1 {
		t.Fatalf("positive control: `s` on the closed picker persisted %d time(s), want 1", mode.calls)
	}
}

func TestPanelGeometry_RendersAtTheFloor(t *testing.T) {
	th := testDarkTheme(t)
	rows := themePanelTestRows(12)
	inner := themePanelInnerWidth(themePanelMinWidth)

	for _, dirUnusable := range []bool{false, true} {
		for _, message := range []themePanelMessage{{}, {Kind: themeMessageCommitFailed}} {
			floor := themePanelMinHeight(dirUnusable)
			wantFooter := themePanelLines(renderThemePanelFooter(themePanelKeymap(), inner, th, false))
			name := fmt.Sprintf("dir=%v/msg=%v/h=%d", dirUnusable, message.Kind != themeMessageNone, floor)
			t.Run(name, func(t *testing.T) {
				p := newThemePanelFixture(themePanelFixtureOpts{
					th:          th,
					width:       themePanelMinWidth,
					rows:        rows,
					dirUnusable: dirUnusable,
					message:     message,
				})
				lines := themePanelLines(renderThemePanel(p, floor, th, false))

				if len(lines) != floor {
					t.Fatalf("the panel rendered %d rows at its floor of %d", len(lines), floor)
				}
				if floor >= panelPageAlignedAffordance(dirUnusable) {
					t.Fatalf("fixture: the floor of %d affords the page-aligned header, so the floor is carrying rows the panel draws nothing in", floor)
				}
				if got, want := lines[0], strings.Repeat(headerRuleGlyph, themePanelMinWidth); got != want {
					t.Errorf("the header rule row = %q, want the glyph across all %d panel columns", got, themePanelMinWidth)
				}
				if got, want := strings.TrimRight(lines[1], " "), themePanelContentPrefix()+themePanelHeaderLabel; got != want {
					t.Errorf("the header label row = %q, want %q directly beneath the rule", got, want)
				}
				at := wantPanelHeaderRows
				if dirUnusable {
					if got, want := strings.TrimRight(lines[at], " "), themePanelContentPrefix()+themePanelDirUnreadable; got != want {
						t.Errorf("the directory row = %q, want %q", got, want)
					}
					at++
				}
				if got := lines[at]; !strings.Contains(got, rows[0].Label()) {
					t.Errorf("the single list row = %q, want it carrying %q", got, rows[0].Label())
				}
				footer := lines[len(lines)-len(wantFooter):]
				for i, want := range wantFooter {
					if got, wantRow := footer[i], themePanelContentPrefix()+want; got != wantRow {
						t.Errorf("footer row %d = %q, want %q — the floor overflowed and the assembly cut the footer", i, got, wantRow)
					}
				}
				if message.Kind == themeMessageNone {
					return
				}
				slot := strings.TrimRight(lines[len(lines)-len(wantFooter)-1], " ")
				if slot != themePanelContentPrefix()+themePanelCommitFailedMessage {
					t.Errorf("the row above the footer is not the message slot: %q", slot)
				}
			})
		}
	}
}

func TestPanelGeometry_RendersAcrossTheCompactBand(t *testing.T) {
	th := testDarkTheme(t)
	rows := themePanelTestRows(12)
	inner := themePanelInnerWidth(themePanelMinWidth)

	for _, dirUnusable := range []bool{false, true} {
		for _, message := range []themePanelMessage{{}, {Kind: themeMessageCommitFailed}} {
			floor := themePanelMinHeight(dirUnusable)
			wantFooter := themePanelLines(renderThemePanelFooter(themePanelKeymap(), inner, th, false))
			for height := floor; height < panelPageAlignedAffordance(dirUnusable); height++ {
				name := fmt.Sprintf("dir=%v/msg=%v/h=%d", dirUnusable, message.Kind, height)
				t.Run(name, func(t *testing.T) {
					p := newThemePanelFixture(themePanelFixtureOpts{
						th:          th,
						width:       themePanelMinWidth,
						rows:        rows,
						dirUnusable: dirUnusable,
						message:     message,
					})
					lines := themePanelLines(renderThemePanel(p, height, th, false))

					if len(lines) != height {
						t.Fatalf("the panel rendered %d rows at height %d", len(lines), height)
					}
					if got := strings.TrimRight(lines[1], " "); got != themePanelContentPrefix()+themePanelHeaderLabel {
						t.Errorf("row 1 = %q, want the label directly beneath the rule", got)
					}
					body := wantPanelHeaderRows + themePanelDirRowHeight(dirUnusable)
					if got := lines[body]; !strings.Contains(got, rows[0].Label()) {
						t.Errorf("row %d = %q, want the list row %q", body, got, rows[0].Label())
					}
					footer := lines[len(lines)-len(wantFooter):]
					for i, want := range wantFooter {
						if got, wantRow := footer[i], themePanelContentPrefix()+want; got != wantRow {
							t.Errorf("footer row %d = %q, want %q — the assembly overflowed and cut the footer", i, got, wantRow)
						}
					}
				})
			}
		}
	}
}

func panelPageAlignedAffordance(dirUnusable bool) int {
	const listRow, messageRow = 1, 1
	return panelPageHeaderRows() +
		themePanelDirRowHeight(dirUnusable) +
		themePanelFooterHeight(themePanelKeymap()) +
		listRow + messageRow
}

func panelPageHeaderRows() int {
	return lipgloss.Height(renderHeaderBlock(0, theme.Theme{}, true)) + sectionHeaderBlockRows()
}

func TestPanelGeometry_HeaderShapeFollowsTheHeight(t *testing.T) {
	th := testDarkTheme(t)
	rows := themePanelTestRows(12)
	p := newThemePanelFixture(themePanelFixtureOpts{
		th:    th,
		width: themePanelPreferredWidth,
		rows:  rows,
	})
	wantRule := strings.Repeat(headerRuleGlyph, themePanelPreferredWidth)
	wantLabel := themePanelContentPrefix() + themePanelHeaderLabel
	affordance := panelPageAlignedAffordance(false)

	t.Run("at the affordance and above it keeps the page's rhythm", func(t *testing.T) {
		pageRule := lipgloss.Height(headerBand(0, theme.Theme{}, true))
		pageLabel := lipgloss.Height(renderHeaderBlock(0, theme.Theme{}, true))
		for _, height := range []int{affordance, affordance + 1, affordance + 6} {
			t.Run(fmt.Sprintf("h=%d", height), func(t *testing.T) {
				lines := themePanelLines(renderThemePanel(p, height, th, false))
				if got := lines[pageRule]; got != wantRule {
					t.Errorf("row %d = %q, want the rule in the page's own rule lane", pageRule, got)
				}
				if got := strings.TrimRight(lines[pageLabel], " "); got != wantLabel {
					t.Errorf("row %d = %q, want the label on the page's section-header row %q", pageLabel, got, wantLabel)
				}
				for i := range panelPageHeaderRows() {
					if i == pageRule || i == pageLabel {
						continue
					}
					if got := strings.TrimSpace(lines[i]); got != "" && got != panelFrameSide {
						t.Errorf("header row %d carries %q, want the page-aligning blank", i, lines[i])
					}
				}
				if got := lines[panelPageHeaderRows()]; !strings.Contains(got, rows[0].Label()) {
					t.Errorf("row %d = %q, want the first list row %q", panelPageHeaderRows(), got, rows[0].Label())
				}
			})
		}
	})

	t.Run("one row below the affordance the blank rows go", func(t *testing.T) {
		height := affordance - 1
		lines := themePanelLines(renderThemePanel(p, height, th, false))
		if len(lines) != height {
			t.Fatalf("the panel rendered %d rows at height %d", len(lines), height)
		}
		if got := lines[0]; got != wantRule {
			t.Errorf("row 0 = %q, want the rule — the compact header opens with it", got)
		}
		if got := strings.TrimRight(lines[1], " "); got != wantLabel {
			t.Errorf("row 1 = %q, want %q directly beneath the rule", got, wantLabel)
		}
		if got := lines[2]; !strings.Contains(got, rows[0].Label()) {
			t.Errorf("row 2 = %q, want the first list row %q — the compact header costs %d rows",
				got, rows[0].Label(), wantPanelHeaderRows)
		}
	})
}

// Test-only: production selects copy through themePanelForcedCloseFlash, never
// by printing the value.
func (d themePanelDim) String() string {
	switch d {
	case dimWidth:
		return "dimWidth"
	case dimHeight:
		return "dimHeight"
	default:
		return fmt.Sprintf("dimNone(%d)", int(d))
	}
}
