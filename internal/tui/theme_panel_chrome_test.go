package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/tmux"
)

const (
	chromeContentW = 96
	chromeContentH = 26
)

func chromeSessionNames() []string {
	return []string{"alpha-one", "bravo-two", "charlie-three", "delta-four", "echo-five"}
}

func newChromePanelModel(t *testing.T) Model {
	t.Helper()
	rows := arrowValidRows(t, 6)
	m := Build(newArrowPanelDeps(t, rows, rows[0].Slug))

	sessions := make([]tmux.Session, 0, len(chromeSessionNames()))
	for _, name := range chromeSessionNames() {
		sessions = append(sessions, tmux.Session{Name: name, Windows: 1})
	}
	m = openPanelForTestWithSessions(t, m, chromeContentW, chromeContentH, sessions)

	if got := m.themePanel.width; got != themePanelPreferredWidth {
		t.Fatalf("fixture: the panel opened at %d cells, want the preferred %d", got, themePanelPreferredWidth)
	}
	return m
}

func chromeFrame(t *testing.T, m Model) []string {
	t.Helper()
	lines := strings.Split(ansi.Strip(m.View().Content), "\n")
	contentW, contentH := m.contentWidth(), m.contentHeight()
	leftPad, _, topPad, _ := gutterPadding(m.termWidth, m.termHeight, contentW, contentH)

	rows := make([]string, 0, contentH)
	for i := range contentH {
		cells := []rune(lines[topPad+i])
		if len(cells) != m.termWidth {
			t.Fatalf("frame row %d is %d cells, want the terminal's %d", topPad+i, len(cells), m.termWidth)
		}
		rows = append(rows, string(cells[leftPad:leftPad+contentW]))
	}
	return rows
}

func chromeSplit(m Model, row string) (page, panel string) {
	at := m.contentWidth() - m.themePanel.width
	cells := []rune(row)
	return string(cells[:at]), string(cells[at:])
}

func chromePageRow(m Model, rows []string, want string) int {
	for i, row := range rows {
		if page, _ := chromeSplit(m, row); strings.Contains(page, want) {
			return i
		}
	}
	return -1
}

func chromePanelRow(m Model, rows []string, want string) int {
	for i, row := range rows {
		if _, panel := chromeSplit(m, row); strings.Contains(panel, want) {
			return i
		}
	}
	return -1
}

func chromeRuleRow(t *testing.T, rows []string) int {
	t.Helper()
	at := -1
	for i, row := range rows {
		if !strings.Contains(row, headerRuleGlyph) {
			continue
		}
		if at >= 0 {
			t.Fatalf("content rows %d and %d both carry the rule glyph; the page's rule and the panel's are in two lanes", at, i)
		}
		at = i
	}
	if at < 0 {
		t.Fatal("no content row carries the rule glyph")
	}
	return at
}

func chromePanelFooterTop(m Model) int {
	return m.contentHeight() - themePanelFooterHeight(themePanelKeymap())
}

func chromeInkColumn(panel string) int {
	for i, r := range []rune(panel) {
		if i == 0 {
			continue
		}
		if r != ' ' {
			return i
		}
	}
	return -1
}

func TestPanelChrome_LabelSharesTheSectionHeaderRow(t *testing.T) {
	m := newChromePanelModel(t)
	rows := chromeFrame(t, m)

	section := chromePageRow(m, rows, sectionLabel)
	label := chromePanelRow(m, rows, themePanelHeaderLabel)
	if section < 0 {
		t.Fatalf("no content row carries the page's %q section header", sectionLabel)
	}
	if label < 0 {
		t.Fatalf("no content row carries the panel's %q label", themePanelHeaderLabel)
	}
	if label != section {
		t.Errorf("the panel's %q label renders on content row %d and the page's %q header on row %d — the two columns run different rhythms",
			themePanelHeaderLabel, label, sectionLabel, section)
	}
}

func TestPanelChrome_ListsStartOnTheSameRow(t *testing.T) {
	m := newChromePanelModel(t)
	rows := chromeFrame(t, m)

	session := chromePageRow(m, rows, chromeSessionNames()[0])
	themeRow := chromePanelRow(m, rows, arrowSlug(0))
	if session < 0 {
		t.Fatalf("no content row carries the first session %q", chromeSessionNames()[0])
	}
	if themeRow < 0 {
		t.Fatalf("no content row carries the first theme %q", arrowSlug(0))
	}
	if themeRow != session {
		t.Errorf("the panel's first row renders on content row %d and the page's first session row on %d", themeRow, session)
	}
}

func TestPanelChrome_ListsStayInStep(t *testing.T) {
	m := newChromePanelModel(t)
	rows := chromeFrame(t, m)

	names := chromeSessionNames()
	for i, name := range names {
		session := chromePageRow(m, rows, name)
		themeRow := chromePanelRow(m, rows, arrowSlug(i))
		if session < 0 {
			t.Fatalf("no content row carries session %q", name)
		}
		if themeRow < 0 {
			t.Fatalf("no content row carries theme row %q", arrowSlug(i))
		}
		if themeRow != session {
			t.Errorf("list row %d: the theme %q is on content row %d and the session %q on row %d", i, arrowSlug(i), themeRow, name, session)
		}
	}
}

// The band composes between the header block and the list, so the page's
// section header — and every row under it — moves down by the slot's height. The
// panel is page-aligned against those rows, so it has to travel with them.
const chromeBandFlash = "banded fixture"

func newBandedChromePanelModel(t *testing.T) (Model, int) {
	t.Helper()
	m := newChromePanelModel(t)
	(&m).setFlash(chromeBandFlash)

	band := (&m).sessionBandHeight()
	if band == 0 {
		t.Fatal("fixture: no band slot is up, so this is the unbanded case again")
	}
	if got := m.themePanel.bandRows; got != band {
		t.Fatalf("fixture: the panel carries %d band rows, want the page's %d — the resync did not reach it", got, band)
	}
	return m, band
}

func TestPanelChrome_BandedLabelSharesTheSectionHeaderRow(t *testing.T) {
	m, band := newBandedChromePanelModel(t)
	rows := chromeFrame(t, m)

	if chromePageRow(m, rows, chromeBandFlash) < 0 {
		t.Fatalf("no content row carries the %q band", chromeBandFlash)
	}
	section := chromePageRow(m, rows, sectionLabel)
	label := chromePanelRow(m, rows, themePanelHeaderLabel)
	if section < 0 {
		t.Fatalf("no content row carries the page's %q section header", sectionLabel)
	}
	if label < 0 {
		t.Fatalf("no content row carries the panel's %q label", themePanelHeaderLabel)
	}
	if label != section {
		t.Errorf("under a %d-row band the panel's %q label renders on content row %d and the page's %q header on row %d — the band moved one column and not the other",
			band, themePanelHeaderLabel, label, sectionLabel, section)
	}
}

func TestPanelChrome_BandedListsStayInStep(t *testing.T) {
	m, band := newBandedChromePanelModel(t)
	rows := chromeFrame(t, m)

	for i, name := range chromeSessionNames() {
		session := chromePageRow(m, rows, name)
		themeRow := chromePanelRow(m, rows, arrowSlug(i))
		if session < 0 {
			t.Fatalf("no content row carries session %q", name)
		}
		if themeRow < 0 {
			t.Fatalf("no content row carries theme row %q", arrowSlug(i))
		}
		if themeRow != session {
			t.Errorf("under a %d-row band list row %d: the theme %q is on content row %d and the session %q on row %d",
				band, i, arrowSlug(i), themeRow, name, session)
		}
	}
}

func TestPanelChrome_RulesShareOneLane(t *testing.T) {
	m := newChromePanelModel(t)
	rows := chromeFrame(t, m)

	at := chromeRuleRow(t, rows)
	if want := strings.Repeat(headerRuleGlyph, m.contentWidth()); rows[at] != want {
		t.Errorf("the rule row = %q, want the glyph across all %d content columns — the panel's rule does not continue the page's to the frame edge",
			rows[at], m.contentWidth())
	}
}

func TestPanelChrome_HeaderRegionIsEmpty(t *testing.T) {
	m := newChromePanelModel(t)
	rows := chromeFrame(t, m)
	at := chromeRuleRow(t, rows)

	if at == 0 {
		t.Fatal("fixture: the rule is on content row 0, so there is no region above it to assert about")
	}
	for i := range at {
		_, panel := chromeSplit(m, rows[i])
		if strings.TrimSpace(panel) != "" {
			t.Errorf("content row %d above the rule carries %q in the panel's columns, want nothing", i, panel)
		}
	}
}

func TestPanelChrome_BorderStartsBelowTheRule(t *testing.T) {
	m := newChromePanelModel(t)
	rows := chromeFrame(t, m)
	at := chromeRuleRow(t, rows)

	for i, row := range rows {
		_, panel := chromeSplit(m, row)
		got := string([]rune(panel)[0])
		want := panelFrameSide
		switch {
		case i < at:
			want = " "
		case i == at:
			want = headerRuleGlyph
		}
		if got != want {
			t.Errorf("content row %d opens the panel with %q, want %q (the rule is on row %d)", i, got, want, at)
		}
	}
}

func TestPanelChrome_InnerGutter(t *testing.T) {
	m := newChromePanelModel(t)
	rows := chromeFrame(t, m)
	at := chromeRuleRow(t, rows)

	const contentColumn = 2

	for i := at + 1; i < len(rows); i++ {
		_, panel := chromeSplit(m, rows[i])
		if got := []rune(panel)[1]; got != ' ' {
			t.Errorf("content row %d has %q one cell in from the border, want the gutter blank", i, string(got))
		}
	}

	t.Run("the label", func(t *testing.T) {
		_, panel := chromeSplit(m, rows[chromePanelRow(m, rows, themePanelHeaderLabel)])
		if got := chromeInkColumn(panel); got != contentColumn {
			t.Errorf("the %q label starts at panel column %d, want %d", themePanelHeaderLabel, got, contentColumn)
		}
	})

	t.Run("the cursor row", func(t *testing.T) {
		row := chromePanelRow(m, rows, arrowSlug(0))
		_, panel := chromeSplit(m, rows[row])
		cells := []rune(panel)
		if got := string(cells[contentColumn]); got != selectorBar {
			t.Errorf("the cursor row carries %q at the content column, want the %q left bar — the cursor column did not move with the gutter", got, selectorBar)
		}
	})

	t.Run("an unselected row", func(t *testing.T) {
		row := chromePanelRow(m, rows, arrowSlug(1))
		_, panel := chromeSplit(m, rows[row])
		if got, want := chromeInkColumn(panel), contentColumn+leftBarColumnWidth; got != want {
			t.Errorf("an unselected row's label starts at panel column %d, want %d (the gutter plus the %d-cell cursor column)", got, want, leftBarColumnWidth)
		}
	})

	t.Run("the key list", func(t *testing.T) {
		top := chromePanelFooterTop(m)
		for i := top; i < m.contentHeight(); i++ {
			_, panel := chromeSplit(m, rows[i])
			if got := chromeInkColumn(panel); got != contentColumn {
				t.Errorf("key list row %d starts at panel column %d, want %d: %q", i-top, got, contentColumn, panel)
			}
		}
	})
}

func TestPanelChrome_LadderEnds(t *testing.T) {
	const wantPreferred, wantMinimum = 30, 24

	if themePanelPreferredWidth != wantPreferred {
		t.Errorf("themePanelPreferredWidth = %d, want %d", themePanelPreferredWidth, wantPreferred)
	}
	if themePanelMinWidth != wantMinimum {
		t.Errorf("themePanelMinWidth = %d, want %d", themePanelMinWidth, wantMinimum)
	}
}

// Derived independently of the production arithmetic. The page-aligning blank
// rows are deliberately absent — charging for them would refuse a usable panel.
func chromeMeasuredFloor(t *testing.T, m Model) int {
	t.Helper()
	if got := chromeMeasuredAffordance(t, m); got <= wantPanelHeaderRows+themePanelFooterHeight(themePanelKeymap())+2 {
		t.Fatalf("fixture: the page-aligned header costs no more than the %d rows the panel draws, so the floor and the affordance are the same number", wantPanelHeaderRows)
	}
	const listRow, messageRow = 1, 1
	return wantPanelHeaderRows + themePanelFooterHeight(themePanelKeymap()) + listRow + messageRow
}

func chromeMeasuredAffordance(t *testing.T, m Model) int {
	t.Helper()
	rows := chromeFrame(t, m)
	first := chromePageRow(m, rows, chromeSessionNames()[0])
	if first < 0 {
		t.Fatalf("no content row carries the first session %q", chromeSessionNames()[0])
	}
	const listRow, messageRow = 1, 1
	return first + themePanelFooterHeight(themePanelKeymap()) + listRow + messageRow
}

func TestPanelChrome_FloorFollowsTheHeader(t *testing.T) {
	m := newChromePanelModel(t)
	entries := themePanelKeymap()

	want := chromeMeasuredFloor(t, m)
	if got := themePanelMinHeight(entries, false); got != want {
		t.Errorf("themePanelMinHeight = %d, want %d — the header draws a rule and a label, and the floor charges for those", got, want)
	}
	if got, wantDir := themePanelMinHeight(entries, true), want+1; got != wantDir {
		t.Errorf("the directory-inclusive floor = %d, want %d", got, wantDir)
	}

	affordance := chromeMeasuredAffordance(t, m)
	if got := themePanelHeaderRows(affordance, 0, false); got != affordance-themePanelFooterHeight(entries)-2 {
		t.Errorf("at %d rows the header costs %d, want the rows the page spends before its first session row", affordance, got)
	}
	if got := themePanelHeaderRows(affordance-1, 0, false); got != wantPanelHeaderRows {
		t.Errorf("one row below the page's rhythm the header costs %d, want the %d rows it draws", got, wantPanelHeaderRows)
	}

	th := testDarkTheme(t)
	p := newThemePanelFixture(themePanelFixtureOpts{
		th:    th,
		width: themePanelMinWidth,
		rows:  themePanelTestRows(8),
	})
	lines := themePanelLines(renderThemePanel(p, want, th, false))
	if len(lines) != want {
		t.Fatalf("the panel rendered %d rows at its floor of %d", len(lines), want)
	}
	wantFooter := themePanelLines(renderThemePanelFooter(entries, themePanelInnerWidth(themePanelMinWidth), th, false))
	for i, row := range wantFooter {
		if got := lines[len(lines)-len(wantFooter)+i]; !strings.HasSuffix(strings.TrimRight(got, " "), strings.TrimRight(row, " ")) {
			t.Errorf("footer row %d = %q, want it to carry %q — the floor overflowed and the assembly cut the footer", i, got, row)
		}
	}
}

func TestPanelChrome_EntryGateFollowsTheFloor(t *testing.T) {
	floor := chromeMeasuredFloor(t, newChromePanelModel(t))

	t.Run("it refuses at one row below the floor", func(t *testing.T) {
		m := newChromeGateModel(t, floor-1)
		m = pressThemeKey(t, m)
		if m.themePanel.open {
			t.Errorf("`t` opened the panel at %d content rows, one below the %d-row floor", floor-1, floor)
		}
		if got := m.flashText; got != themePanelShortEntryFlash {
			t.Errorf("the refusal raised %q, want %q", got, themePanelShortEntryFlash)
		}
	})

	t.Run("it admits at the floor", func(t *testing.T) {
		m := newChromeGateModel(t, floor)
		m = pressThemeKey(t, m)
		if !m.themePanel.open {
			t.Errorf("`t` refused at exactly the %d-row floor, which the entry gate admits", floor)
		}
	})
}

func newChromeGateModel(t *testing.T, contentH int) Model {
	t.Helper()
	rows := arrowValidRows(t, 6)
	m := Build(newArrowPanelDeps(t, rows, rows[0].Slug))
	m.termWidth, m.termHeight = geometryTerm(chromeContentW, contentH)
	m.applySessions([]tmux.Session{{Name: chromeSessionNames()[0], Windows: 1}}, nil)
	m.applySessionListSize(m.contentWidth(), m.contentHeight())
	if got := m.contentHeight(); got != contentH {
		t.Fatalf("fixture: the content region is %d rows tall, want %d", got, contentH)
	}
	return m
}

func chromeWords(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
