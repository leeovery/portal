package tui

import (
	"fmt"
	"go/ast"
	"maps"
	"slices"
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/theme"
)

// Literals, deliberately not re-derived from the production layout, so the floor
// the tests scan up from is an independent statement of the same rule.
const (
	themePanelTestBodyRows    = 1
	themePanelTestMessageRows = 1
)

type themePanelFixtureOpts struct {
	th          theme.Theme
	colourless  bool
	width       int
	rows        []theme.Row
	dirUnusable bool
	message     themePanelMessage
}

func newThemePanelFixture(o themePanelFixtureOpts) themePanel {
	items := make([]list.Item, 0, len(o.rows))
	for _, row := range o.rows {
		items = append(items, themeRowItem{Row: row})
	}
	delegate := themeRowDelegate{
		Theme:      o.th,
		Colourless: o.colourless,
		Width:      themePanelInnerWidth(o.width),
	}
	return themePanel{
		open:    true,
		list:    newThemePanelList(items, delegate),
		union:   themeRowsUnionDirUnusable(o.rows, o.dirUnusable),
		message: o.message,
		width:   o.width,
	}
}

func themePanelTestRows(n int) []theme.Row {
	rows := make([]theme.Row, 0, n)
	for i := range n {
		rows = append(rows, theme.Row{Slug: fmt.Sprintf("theme-%02d", i), Source: theme.SourceBuiltin})
	}
	return rows
}

func themePanelLines(block string) []string {
	return strings.Split(ansi.Strip(block), "\n")
}

func panelHeaderRowsOf(m Model) int {
	return themePanelHeaderRows(m.contentHeight(), m.themePanel.bandRows, m.themePanel.union.DirUnusable)
}

func themePanelContentPrefix() string {
	return panelFrameSide + strings.Repeat(" ", themePanelGutterWidth)
}

func themePanelColourParams(block string) map[string]bool {
	params := map[string]bool{}
	for _, chunk := range strings.Split(block, "\x1b[")[1:] {
		p, _, ok := strings.Cut(chunk, "m")
		if !ok {
			continue
		}
		if strings.Contains(p, "38;2;") || strings.Contains(p, "48;2;") {
			params[p] = true
		}
	}
	return params
}

func TestThemePanel_BlockGeometry(t *testing.T) {
	th := testDarkTheme(t)
	footerRows := themePanelFooterHeight(themePanelKeymap())

	for _, width := range []int{themePanelMinWidth, themePanelPreferredWidth} {
		for _, dirUnusable := range []bool{false, true} {
			for _, message := range []themePanelMessage{{}, messageTestFailed()} {
				floor := themePanelCompactHeaderRows + themePanelTestBodyRows + footerRows
				if dirUnusable {
					floor++
				}
				if message.Kind != themeMessageNone {
					floor += themePanelTestMessageRows
				}

				for height := floor; height <= floor+12; height++ {
					name := fmt.Sprintf("w=%d/dir=%v/msg=%v/h=%d", width, dirUnusable, message.Kind != themeMessageNone, height)
					t.Run(name, func(t *testing.T) {
						p := newThemePanelFixture(themePanelFixtureOpts{
							th:          th,
							width:       width,
							rows:        themePanelTestRows(12),
							dirUnusable: dirUnusable,
							message:     message,
						})
						block := renderThemePanel(p, height, th, false)

						if got := lipgloss.Height(block); got != height {
							t.Fatalf("block height = %d, want %d", got, height)
						}
						for i, line := range strings.Split(block, "\n") {
							if got := lipgloss.Width(line); got != width {
								t.Errorf("line %d width = %d, want %d: %q", i, got, width, ansi.Strip(line))
							}
						}
					})
				}
			}
		}
	}
}

// `bubbles/list` renders a hard minimum of three rows whatever height it is
// given, while the floor budgets the body one — so the assembly's clamp drops
// the overshoot off the BOTTOM, off the footer.
func TestThemePanel_FooterSurvivesAtTheFloor(t *testing.T) {
	th := testDarkTheme(t)
	inner := themePanelInnerWidth(themePanelPreferredWidth)

	for _, dirUnusable := range []bool{false, true} {
		for _, message := range []themePanelMessage{{}, messageTestFailed()} {
			wantFooter := themePanelLines(renderThemePanelFooter(themePanelKeymap(), inner, th, false))
			floor := themePanelCompactHeaderRows + themePanelTestBodyRows + len(wantFooter)
			if dirUnusable {
				floor++
			}
			if message.Kind != themeMessageNone {
				floor += themePanelTestMessageRows
			}

			name := fmt.Sprintf("dir=%v/msg=%v/h=%d", dirUnusable, message.Kind != themeMessageNone, floor)
			t.Run(name, func(t *testing.T) {
				p := newThemePanelFixture(themePanelFixtureOpts{
					th:          th,
					width:       themePanelPreferredWidth,
					rows:        themePanelTestRows(12),
					dirUnusable: dirUnusable,
					message:     message,
				})
				lines := themePanelLines(renderThemePanel(p, floor, th, false))

				footer := lines[len(lines)-len(wantFooter):]
				for i, want := range wantFooter {
					if got, wantRow := footer[i], themePanelContentPrefix()+want; got != wantRow {
						t.Errorf("footer row %d = %q, want %q", i, got, wantRow)
					}
				}
				if message.Kind == themeMessageNone {
					return
				}
				slot := lines[len(lines)-len(wantFooter)-1]
				want := themePanelContentPrefix() + themePanelLines(renderThemePanelMessage(message, th, false))[0]
				if got := strings.TrimRight(slot, " "); got != want {
					t.Errorf("message slot = %q, want %q", got, want)
				}
			})
		}
	}
}

func TestThemePanel_LeftBorderOnly(t *testing.T) {
	th := testDarkTheme(t)
	p := newThemePanelFixture(themePanelFixtureOpts{
		th:    th,
		width: themePanelPreferredWidth,
		rows:  themePanelTestRows(6),
	})
	const height = 16
	shape := themePanelHeaderShapeFor(height, p.bandRows, p.union.DirUnusable)
	block := renderThemePanel(p, height, th, false)

	for i, line := range themePanelLines(block) {
		runes := []rune(line)
		if len(runes) == 0 {
			t.Fatalf("line %d is empty", i)
		}
		want := panelFrameSide
		switch {
		case i < shape.ruleRow:
			want = " "
		case i == shape.ruleRow:
			want = headerRuleGlyph
		}
		if got := string(runes[0]); got != want {
			t.Fatalf("line %d opens with %q, want %q (the border starts on row %d)", i, got, want, shape.borderFrom())
		}
		if strings.Contains(string(runes[1:]), panelFrameSide) {
			t.Errorf("line %d carries a second %q (a right edge): %q", i, panelFrameSide, line)
		}
	}

	for _, glyph := range []string{
		panelFrameTopLeft, panelFrameTopRight,
		panelFrameBottomLeft, panelFrameBottomRight,
		panelFrameTeeLeft, panelFrameTeeRight,
		panelRuleGlyph,
	} {
		if strings.Contains(ansi.Strip(block), glyph) {
			t.Errorf("block carries the inset-frame glyph %q — the panel draws a left border only", glyph)
		}
	}

	// Read from below the header rule: the rule paints in the same token, so the
	// first border-painted run in the whole block would be the rule's.
	bordered := strings.Join(strings.Split(block, "\n")[shape.borderFrom():], "\n")
	if got := themeRowRunAfter(t, bordered, tokenFgSeq(t, th.Border)); !strings.HasPrefix(got, panelFrameSide) {
		t.Errorf("the border run does not paint %q, got %q", panelFrameSide, got)
	}
}

func TestThemePanel_HeaderIsMeasuredAndCountless(t *testing.T) {
	th := testDarkTheme(t)
	rows := themePanelTestRows(7)
	p := newThemePanelFixture(themePanelFixtureOpts{
		th:    th,
		width: themePanelPreferredWidth,
		rows:  rows,
	})
	const height = 16
	shape := themePanelHeaderShapeFor(height, p.bandRows, p.union.DirUnusable)
	block := renderThemePanel(p, height, th, false)
	lines := themePanelLines(block)

	rule, label := shape.ruleRow, shape.labelRow
	if got, want := lines[rule], strings.Repeat(headerRuleGlyph, themePanelPreferredWidth); got != want {
		t.Errorf("header rule row = %q, want the glyph across all %d panel columns", got, themePanelPreferredWidth)
	}
	if got, want := strings.TrimRight(lines[label], " "), themePanelContentPrefix()+themePanelHeaderLabel; got != want {
		t.Errorf("header label row = %q, want %q", got, want)
	}
	for i := range shape.rows {
		if i == rule || i == label {
			continue
		}
		if got := strings.TrimSpace(lines[i]); got != "" && got != panelFrameSide {
			t.Errorf("header row %d carries %q, want nothing — the region's other rows are blank by decision", i, lines[i])
		}
	}
	if strings.ContainsAny(lines[rule]+lines[label], "0123456789") {
		t.Errorf("the header carries a count: %q / %q", lines[rule], lines[label])
	}
	if !strings.Contains(lines[shape.rows], rows[0].Label()) {
		t.Errorf("row %d is not the first list row (%q): %q", shape.rows, rows[0].Label(), lines[shape.rows])
	}

	if got := themeRowRunAfter(t, block, tokenFgSeq(t, th.AccentMode)); got != themePanelHeaderLabel {
		t.Errorf("accent.mode paints %q, want %q", got, themePanelHeaderLabel)
	}
}

func TestThemePanel_DirUnreadableIsPinnedChrome(t *testing.T) {
	th := testDarkTheme(t)
	rows := themePanelTestRows(24)
	p := newThemePanelFixture(themePanelFixtureOpts{
		th:          th,
		width:       themePanelPreferredWidth,
		rows:        rows,
		dirUnusable: true,
	})

	const height = 14
	header := themePanelHeaderRows(height, p.bandRows, p.union.DirUnusable)
	firstPage := themePanelLines(renderThemePanel(p, height, th, false))
	if got, want := strings.TrimRight(firstPage[header], " "), themePanelContentPrefix()+themePanelDirUnreadable; got != want {
		t.Fatalf("page 1 directory row = %q, want %q", got, want)
	}

	p.list.Select(len(rows) - 1)
	lastPage := themePanelLines(renderThemePanel(p, height, th, false))
	if got, want := strings.TrimRight(lastPage[header], " "), themePanelContentPrefix()+themePanelDirUnreadable; got != want {
		t.Errorf("last page directory row = %q, want %q — the row is paginating, so it is a list item", got, want)
	}
	if strings.Join(firstPage, "\n") == strings.Join(lastPage, "\n") {
		t.Fatal("the list did not page: the page-2 assertion proves nothing")
	}

	for i, item := range p.list.Items() {
		row, ok := item.(themeRowItem)
		if !ok {
			t.Fatalf("list item %d is not a themeRowItem: %T", i, item)
		}
		if row.Row.Label() == themePanelDirUnreadable {
			t.Errorf("the directory warning is a list item at index %d", i)
		}
	}

	block := renderThemePanel(p, height, th, false)
	if got := themeRowRunAfter(t, block, tokenFgSeq(t, th.AccentAttention)); got != themePanelDirUnreadable {
		t.Errorf("accent.attention paints %q, want %q", got, themePanelDirUnreadable)
	}
}

func TestThemePanel_RowsRenderBeneathDirRow(t *testing.T) {
	th := testDarkTheme(t)
	builtin := theme.Row{Slug: "tokyo-night", Source: theme.SourceBuiltin}
	persisted := theme.Row{
		Slug:      "zzz-gone",
		Source:    theme.SourcePersisted,
		Rejection: &theme.Rejection{Reason: theme.ReasonUnreadable},
	}
	p := newThemePanelFixture(themePanelFixtureOpts{
		th:          th,
		width:       themePanelPreferredWidth,
		rows:        []theme.Row{builtin, persisted},
		dirUnusable: true,
	})
	p.list.SetDelegate(themeRowDelegate{Theme: th, Width: themePanelInnerWidth(themePanelPreferredWidth)})
	p.list.Items()[1] = themeRowItem{Row: persisted, Badge: theme.BadgeDark}

	height := themePanelMinHeight(p.union.DirUnusable) + 10
	lines := themePanelLines(renderThemePanel(p, height, th, false))
	below := strings.Join(lines[themePanelHeaderRows(height, p.bandRows, p.union.DirUnusable)+1:], "\n")

	for _, want := range []string{builtin.Label(), persisted.Label(), themePanelBadgeText(theme.BadgeDark)} {
		if !strings.Contains(below, want) {
			t.Errorf("nothing beneath the directory row carries %q:\n%s", want, below)
		}
	}
}

func TestThemePanel_DirRowFitsMinimumWidthUntruncated(t *testing.T) {
	th := testDarkTheme(t)

	if got, want := themePanelDirUnreadable, flashWarningGlyph+" dir unreadable"; got != want {
		t.Fatalf("directory row copy = %q, want %q", got, want)
	}
	const wantCells = 16
	if got := lipgloss.Width(themePanelDirUnreadable); got != wantCells {
		t.Fatalf("directory row copy is %d cells, want %d", got, wantCells)
	}
	if inner := themePanelInnerWidth(themePanelMinWidth); wantCells > inner {
		t.Fatalf("the %d-cell copy does not fit the %d-cell minimum inner width", wantCells, inner)
	}

	p := newThemePanelFixture(themePanelFixtureOpts{
		th:          th,
		width:       themePanelMinWidth,
		rows:        themePanelTestRows(3),
		dirUnusable: true,
	})
	const height = 10
	lines := themePanelLines(renderThemePanel(p, height, th, false))
	row := strings.TrimRight(lines[themePanelHeaderRows(height, p.bandRows, p.union.DirUnusable)], " ")
	if want := themePanelContentPrefix() + themePanelDirUnreadable; row != want {
		t.Errorf("at the minimum width the directory row = %q, want %q", row, want)
	}
	if strings.Contains(row, themeRowEllipsis) {
		t.Errorf("the directory row was truncated at the minimum width: %q", row)
	}
}

func TestThemePanel_MessageSlotUnreservedWhenEmpty(t *testing.T) {
	th := testDarkTheme(t)

	if got := renderThemePanelMessage(themePanelMessage{}, th, false); got != "" {
		t.Errorf("an empty message rendered %q, want the empty string", got)
	}
	if got := themePanelMessageHeight(themePanelMessage{}); got != 0 {
		t.Errorf("an empty message reserved %d rows, want 0", got)
	}
	if got := themePanelMessageHeight(messageTestFailed()); got != themePanelTestMessageRows {
		t.Errorf("a live message reserved %d rows, want %d", got, themePanelTestMessageRows)
	}
}

func TestThemePanel_MessageSlotRecomputesListHeight(t *testing.T) {
	th := testDarkTheme(t)
	const height = 14
	opts := themePanelFixtureOpts{
		th:    th,
		width: themePanelPreferredWidth,
		rows:  themePanelTestRows(12),
	}

	empty := newThemePanelFixture(opts)
	opts.message = messageTestFailed()
	live := newThemePanelFixture(opts)

	_, emptyRows := themePanelListSize(empty, height)
	_, liveRows := themePanelListSize(live, height)
	if liveRows != emptyRows-1 {
		t.Errorf("list body with a message = %d rows, want %d (one fewer than %d)", liveRows, emptyRows-1, emptyRows)
	}

	block := renderThemePanel(live, height, th, false)
	if got := lipgloss.Height(block); got != height {
		t.Fatalf("block height with a message = %d, want %d", got, height)
	}
	lines := themePanelLines(block)
	slot := lines[height-themePanelFooterHeight(themePanelKeymap())-1]
	if !strings.Contains(slot, themePanelCommitFailedMessage) {
		t.Errorf("the row above the footer is not the message slot: %q", slot)
	}
}

func TestThemePanel_ListIsConstructedWithPanelChromeDisabled(t *testing.T) {
	l := newThemePanelList(nil, themeRowDelegate{})

	if l.FilteringEnabled() {
		t.Error("the panel's list has filtering enabled")
	}
	if l.ShowTitle() {
		t.Error("the panel's list shows a title")
	}
	if l.ShowStatusBar() {
		t.Error("the panel's list shows a status bar")
	}
	if l.ShowHelp() {
		t.Error("the panel's list shows its own help")
	}

	th := testDarkTheme(t)
	const height = 15
	p := newThemePanelFixture(themePanelFixtureOpts{
		th:    th,
		width: themePanelPreferredWidth,
		rows:  themePanelTestRows(12),
	})
	wantW, wantH := themePanelInnerWidth(themePanelPreferredWidth),
		height-themePanelHeaderRows(height, p.bandRows, p.union.DirUnusable)-themePanelFooterHeight(themePanelKeymap())
	gotW, gotH := themePanelListSize(p, height)
	if gotW != wantW || gotH != wantH {
		t.Fatalf("themePanelListSize = (%d, %d), want (%d, %d)", gotW, gotH, wantW, wantH)
	}

	p.list.SetSize(3, 3)
	if got := lipgloss.Height(renderThemePanel(p, height, th, false)); got != height {
		t.Errorf("block height after a stale list size = %d, want %d", got, height)
	}
}

func TestThemePanel_EveryChromeSurfaceIsATokenLookup(t *testing.T) {
	dark, light := testDarkTheme(t), testLightTheme(t)
	rows := themePanelTestRows(6)

	// Derived so the list draws no paginator: this fixture skips the production
	// restyle, and library-default dots would read as a surface surviving the swap.
	pageAligned := themePanelFloorFor(themePanelPageAlignedHeaderShape(0).rows, true)
	height := pageAligned - themePanelMinBodyRows + len(rows) + 2

	render := func(th theme.Theme) string {
		p := newThemePanelFixture(themePanelFixtureOpts{
			th:          th,
			width:       themePanelPreferredWidth,
			rows:        rows,
			dirUnusable: true,
			message:     messageTestFailed(),
		})
		return renderThemePanel(p, height, th, false)
	}

	darkBlock, lightBlock := render(dark), render(light)
	if strings.Contains(ansi.Strip(darkBlock), paginationDotGlyph) {
		t.Fatalf("fixture: the list paginated at height %d, so its un-restyled dot greys would read as a surviving surface:\n%s", height, ansi.Strip(darkBlock))
	}
	if ansi.Strip(darkBlock) != ansi.Strip(lightBlock) {
		t.Fatalf("the two renders differ in TEXT, so a colour diff proves nothing:\n%s\n---\n%s",
			ansi.Strip(darkBlock), ansi.Strip(lightBlock))
	}

	darkParams, lightParams := themePanelColourParams(darkBlock), themePanelColourParams(lightBlock)
	if len(darkParams) == 0 || len(lightParams) == 0 {
		t.Fatalf("a render carried no colour at all: dark=%d light=%d", len(darkParams), len(lightParams))
	}
	for p := range darkParams {
		if lightParams[p] {
			t.Errorf("SGR params %q survive the theme swap — a chrome surface is not a token lookup", p)
		}
	}
}

func themePanelOverlayBase(contentW, contentH int) []string {
	base := make([]string, contentH)
	for i := range base {
		row := fmt.Sprintf("row-%02d:", i) + strings.Repeat("abcdefghij", 1+contentW/10)
		base[i] = row[:contentW]
	}
	return base
}

// The paint makes the overlay's two failure modes visible: a flattened base and
// a recoloured panel both survive a text-only comparison untouched.
func themePanelPaintedOverlayBase(contentW, contentH int, th theme.Theme) []string {
	rows := themePanelOverlayBase(contentW, contentH)
	for i, row := range rows {
		rows[i] = headerCanvasBg(th, false).Render(row)
	}
	return rows
}

// The base is painted in the OTHER built-in's canvas, so a survived base and a
// flattened one are distinguishable rather than the same bytes.
func TestThemePanel_OverlayDoesNotRelayoutTheBase(t *testing.T) {
	th, baseTh := testDarkTheme(t), testLightTheme(t)
	const contentW, contentH = 100, 20

	basePaint, panelPaint := canvasBgParams(baseTh.Canvas.Color()), canvasBgParams(th.Canvas.Color())
	if basePaint == panelPaint {
		t.Fatalf("base and panel paint identically (%s) — the fixture cannot tell a flatten from a survival", basePaint)
	}

	base := themePanelPaintedOverlayBase(contentW, contentH, baseTh)
	p := newThemePanelFixture(themePanelFixtureOpts{
		th:    th,
		width: themePanelPreferredWidth,
		rows:  themePanelTestRows(8),
	})
	panel := renderThemePanel(p, contentH, th, false)

	composed := overlayThemePanel(strings.Join(base, "\n"), panel, contentW)

	composedRows := strings.Split(composed, "\n")
	panelRows := strings.Split(panel, "\n")
	composedLines := strings.Split(ansi.Strip(composed), "\n")
	panelLines := strings.Split(ansi.Strip(panel), "\n")
	if len(composedLines) != contentH {
		t.Fatalf("composed height = %d, want %d", len(composedLines), contentH)
	}

	cut := contentW - themePanelPreferredWidth
	for i, line := range composedLines {
		cells := []rune(line)
		if len(cells) != contentW {
			t.Fatalf("composed line %d is %d cells, want %d: %q", i, len(cells), contentW, line)
		}
		if got, want := string(cells[:cut]), string([]rune(ansi.Strip(base[i]))[:cut]); got != want {
			t.Errorf("line %d left of the panel = %q, want %q", i, got, want)
		}
		if got, want := string(cells[cut:]), panelLines[i]; got != want {
			t.Errorf("line %d under the panel = %q, want the panel row %q", i, got, want)
		}

		composedCells := scanCellBackgrounds(composedRows[i])
		baseCells := scanCellBackgrounds(base[i])
		panelCells := scanCellBackgrounds(panelRows[i])
		if len(composedCells) != contentW {
			t.Fatalf("composed line %d has %d painted cells, want %d: %q", i, len(composedCells), contentW, escSeq(composedRows[i]))
		}
		for col := range cut {
			if baseCells[col].params != basePaint {
				t.Fatalf("base line %d col %d is not canvas-painted (%+v) — the fixture proves nothing", i, col, baseCells[col])
			}
			if got := composedCells[col]; got != baseCells[col] {
				t.Errorf("line %d col %d left of the panel is painted %+v, want the base's own %+v", i, col, got, baseCells[col])
			}
		}
		if len(panelCells) != themePanelPreferredWidth {
			t.Fatalf("panel line %d has %d painted cells, want %d: %q", i, len(panelCells), themePanelPreferredWidth, escSeq(panelRows[i]))
		}
		for col, want := range panelCells {
			if got := composedCells[cut+col]; got != want {
				t.Errorf("line %d col %d under the panel is painted %+v, want the panel's own %+v", i, cut+col, got, want)
			}
		}
	}
}

// Not a violation of the footer's "never truncate a label" rule: that governs
// how the footer lays ITSELF out, so a covered footer is cut, not re-laid-out.
func TestThemePanel_OverlayCutsMidLabel(t *testing.T) {
	th := testDarkTheme(t)
	// At this content width the panel's left border lands inside `x projects`; a
	// re-authored footer row or a move of the width ladder shifts that.
	const contentW, contentH = 86, 20

	footer := renderSessionsFooter(sessionsKeymap(), contentW, th, false)
	footerLines := strings.Split(ansi.Strip(footer), "\n")
	keyRow := footerLines[len(footerLines)-1]
	if !strings.Contains(keyRow, "? help") || !strings.Contains(keyRow, "x projects") {
		t.Fatalf("the un-overlaid footer is not the condensed Sessions row: %q", keyRow)
	}

	base := themePanelOverlayBase(contentW, contentH)
	base[contentH-1] = keyRow

	p := newThemePanelFixture(themePanelFixtureOpts{
		th:    th,
		width: themePanelPreferredWidth,
		rows:  themePanelTestRows(8),
	})
	panel := renderThemePanel(p, contentH, th, false)
	composed := overlayThemePanel(strings.Join(base, "\n"), panel, contentW)

	cut := contentW - themePanelPreferredWidth
	covered := []rune(strings.Split(ansi.Strip(composed), "\n")[contentH-1])

	full := []rune(keyRow)
	if full[cut-1] == ' ' || full[cut] == ' ' {
		t.Fatalf("the panel's left border falls on a word boundary at col %d (%q) — the case is not exercised",
			cut, string(full[max(cut-8, 0):min(cut+8, len(full))]))
	}
	if got, want := string(covered[:cut]), string(full[:cut]); got != want {
		t.Errorf("the covered footer reflowed: got %q, want the unreduced footer's first %d cells %q", got, cut, want)
	}
	if want := "x proj"; !strings.HasSuffix(string(covered[:cut]), want) {
		t.Errorf("the cut does not land mid-label: %q does not end in %q", string(covered[:cut]), want)
	}
	if strings.Contains(string(covered), "? help") {
		t.Error("the right-aligned `? help` survived the overlay — the panel is not covering the right column")
	}
}

// The layer ORDER is deliberately not pinned: every panel cell already carries
// an explicit background, so the outer fill has nothing to do to one either way.
func TestThemePanel_ViewCompositesWhenOpen(t *testing.T) {
	const termW, termH = 90, 24

	m := newCanvasTestModel(t, termW, termH, theme.MemberDark)
	closed := m.View().Content

	m.themePanel = newThemePanelFixture(themePanelFixtureOpts{
		th:    m.themeState.active,
		width: themePanelPreferredWidth,
		rows:  themePanelTestRows(9),
	})
	opened := m.View().Content

	if opened == closed {
		t.Fatal("the composed frame is unchanged with the panel open")
	}
	if !strings.Contains(ansi.Strip(opened), themePanelHeaderLabel) {
		t.Errorf("the composed frame carries no %q header", themePanelHeaderLabel)
	}

	contentW, contentH := m.contentWidth(), m.contentHeight()
	panel := renderThemePanel(m.themePanel, contentH, m.themeState.active, m.colourless)
	panelLines := strings.Split(ansi.Strip(panel), "\n")
	_, _, topPad, _ := gutterPadding(termW, termH, contentW, contentH)
	left := (termW-contentW)/2 + contentW - themePanelPreferredWidth

	frame := strings.Split(ansi.Strip(opened), "\n")
	rawFrame, panelRows := strings.Split(opened, "\n"), strings.Split(panel, "\n")
	if len(frame) != termH {
		t.Fatalf("composed frame is %d rows, want %d", len(frame), termH)
	}
	for j, want := range panelLines {
		row := []rune(frame[topPad+j])
		if len(row) != termW {
			t.Fatalf("frame row %d is %d cells, want %d", topPad+j, len(row), termW)
		}
		if got := string(row[left : left+themePanelPreferredWidth]); got != want {
			t.Errorf("frame row %d under the panel = %q, want the panel row %q", topPad+j, got, want)
		}

		frameCells, panelCells := scanCellBackgrounds(rawFrame[topPad+j]), scanCellBackgrounds(panelRows[j])
		if len(frameCells) != termW {
			t.Fatalf("frame row %d has %d painted cells, want %d: %q", topPad+j, len(frameCells), termW, escSeq(rawFrame[topPad+j]))
		}
		if len(panelCells) != themePanelPreferredWidth {
			t.Fatalf("panel row %d has %d painted cells, want %d: %q", j, len(panelCells), themePanelPreferredWidth, escSeq(panelRows[j]))
		}
		for col, wantCell := range panelCells {
			if got := frameCells[left+col]; got != wantCell {
				t.Errorf("frame row %d col %d is painted %+v, want the panel's own %+v — the fill painted over a panel cell",
					topPad+j, left+col, got, wantCell)
			}
		}
	}

	m.themePanel.open = false
	if reclosed := m.View().Content; reclosed != closed {
		t.Error("the closed frame is not byte-identical to the pre-panel view")
	}
}

func TestThemePanel_DelegateHasASingleConstructionPoint(t *testing.T) {
	th := testLightTheme(t)
	m := Model{themeState: themeState{active: th}, colourless: true}
	m.themePanel.width = themePanelMinWidth

	got := m.themeRowDelegate()
	if got.Theme.Canvas != th.Canvas {
		t.Errorf("delegate theme = %s, want %s", themeLabel(got.Theme), themeLabel(th))
	}
	if !got.Colourless {
		t.Error("delegate did not take the model's colourless flag")
	}
	if want := themePanelInnerWidth(themePanelMinWidth); got.Width != want {
		t.Errorf("delegate width = %d, want the panel's inner width %d", got.Width, want)
	}

	sites := themeRowDelegateLiteralSites(t)
	if len(sites) != 1 {
		t.Fatalf("themeRowDelegate is constructed at %d production sites (%v), want exactly 1", len(sites), sites)
	}
	if sites[0] != "theme_panel.go" {
		t.Errorf("the single construction site is %s, want theme_panel.go", sites[0])
	}
}

// Test files are excluded: a test-side delegate cannot reach the render path.
func themeRowDelegateLiteralSites(t *testing.T) []string {
	t.Helper()
	files := parsePackageFilesByName(t)

	var sites []string
	for _, name := range slices.Sorted(maps.Keys(files)) {
		ast.Inspect(files[name], func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			if ident, isIdent := lit.Type.(*ast.Ident); isIdent && ident.Name == "themeRowDelegate" {
				sites = append(sites, name)
			}
			return true
		})
	}
	return sites
}

// `bubbles/list` pads short lines with unstyled spaces, and composited after the
// outer fill those cells are past its backfill — bare, they are terminal-bg
// islands inside the panel.
func TestThemePanel_BodyIsCanvas(t *testing.T) {
	th := testDarkTheme(t)
	p := newThemePanelFixture(themePanelFixtureOpts{
		th:          th,
		width:       themePanelPreferredWidth,
		rows:        themePanelTestRows(9),
		dirUnusable: true,
	})
	block := renderThemePanel(p, 20, th, false)
	canvasParams := canvasBgParams(th.Canvas.Color())

	sawCanvas := false
	for i, line := range strings.Split(block, "\n") {
		cells := scanCellBackgrounds(line)
		if len(cells) != themePanelPreferredWidth {
			t.Fatalf("line %d has %d painted cells, want %d: %q", i, len(cells), themePanelPreferredWidth, ansi.Strip(line))
		}
		for col, cell := range cells {
			if !cell.set {
				t.Fatalf("line %d col %d falls back to the terminal default — a bg island inside the panel: %q",
					i, col, escSeq(line))
			}
			if cell.params == canvasParams {
				sawCanvas = true
			}
		}
	}
	if !sawCanvas {
		t.Errorf("no cell carried the canvas background %q", canvasParams)
	}
}

// The panel is blocked under NO_COLOR outright, so this is defence, not the
// daily path.
func TestThemePanel_Colourless(t *testing.T) {
	th := testDarkTheme(t)
	p := newThemePanelFixture(themePanelFixtureOpts{
		th:          th,
		colourless:  true,
		width:       themePanelPreferredWidth,
		rows:        themePanelTestRows(5),
		dirUnusable: true,
		message:     messageTestFailed(),
	})
	const height = 14
	shape := themePanelHeaderShapeFor(height, p.bandRows, p.union.DirUnusable)
	block := renderThemePanel(p, height, th, true)

	for i, line := range strings.Split(block, "\n") {
		for col, cell := range scanCellBackgrounds(line) {
			if cell.set {
				t.Fatalf("colourless line %d col %d carries a background SGR (%s): %q",
					i, col, cell.params, escSeq(line))
			}
		}
	}

	lines := themePanelLines(block)
	label := lines[shape.labelRow]
	if !strings.HasPrefix(label, themePanelContentPrefix()+themePanelHeaderLabel) {
		t.Errorf("colourless header row = %q", label)
	}
	if !strings.Contains(lines[shape.rows], themePanelDirUnreadable) {
		t.Errorf("colourless directory row = %q", lines[shape.rows])
	}
}
