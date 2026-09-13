package tui

import (
	"os"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/themetest"
	"github.com/leeovery/portal/internal/tmux"
)

var (
	arrowUp       = tea.KeyPressMsg{Code: tea.KeyUp}
	arrowDown     = tea.KeyPressMsg{Code: tea.KeyDown}
	arrowPageUp   = tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModCtrl}
	arrowPageDown = tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModCtrl}
)

const (
	arrowTermW = 100
	arrowTermH = 28
)

const (
	arrowPagingTermH   = 15
	arrowPagingPerPage = 2
)

var arrowPaletteReds = []uint8{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF}

func arrowPalette(t *testing.T, i int) theme.Theme {
	t.Helper()
	return themetest.SyntheticPalette(t, arrowPaletteReds[i%len(arrowPaletteReds)])
}

func arrowValidRow(t *testing.T, slug string, palette int) theme.Row {
	t.Helper()
	return theme.Row{Slug: slug, Source: theme.SourceBuiltin, Theme: arrowPalette(t, palette)}
}

func arrowInvalidRow(slug string) theme.Row {
	return theme.Row{
		Slug:      slug,
		Filename:  slug + ".theme",
		Source:    theme.SourceFile,
		Rejection: &theme.Rejection{Reason: theme.ReasonBadColour},
	}
}

func arrowValidRows(t *testing.T, n int) []theme.Row {
	t.Helper()
	rows := make([]theme.Row, 0, n)
	for i := range n {
		rows = append(rows, arrowValidRow(t, arrowSlug(i), i))
	}
	return rows
}

func arrowSlug(i int) string {
	return "theme-" + string(rune('a'+i/10)) + string(rune('0'+i%10))
}

func arrowRowBySlug(t *testing.T, rows []theme.Row, slug string) theme.Row {
	t.Helper()
	for _, row := range rows {
		if row.Slug == slug {
			return row
		}
	}
	t.Fatalf("fixture has no row %q", slug)
	return theme.Row{}
}

func newArrowPanelDeps(t *testing.T, rows []theme.Row, cursorSlug string) Deps {
	t.Helper()
	target := arrowRowBySlug(t, rows, cursorSlug)
	source := &fakeThemeSource{
		union:      themeRowsUnion(rows),
		resolution: constantResolution(cursorSlug, target.Theme),
	}
	return stubPanelDeps(source, theme.ConstantNomination(target.Theme), theme.RawKeys{Theme: cursorSlug})
}

func arrowRejectedCount(rows []theme.Row) int {
	n := 0
	for _, row := range rows {
		if !row.Selectable() {
			n++
		}
	}
	return n
}

func newArrowPanelModel(t *testing.T, rows []theme.Row, cursorSlug string) Model {
	t.Helper()
	return newArrowPanelModelAt(t, rows, cursorSlug, arrowTermH)
}

func newArrowPanelModelAt(t *testing.T, rows []theme.Row, cursorSlug string, termH int) Model {
	t.Helper()
	m := Build(newArrowPanelDeps(t, rows, cursorSlug))
	m = openPanelForTest(t, m, arrowTermW-2*Hinset, termH-2*Vinset)
	requireCursorOn(t, m, cursorSlug)
	return m
}

func requireArrowPanelPageSize(t *testing.T, m Model, want int) {
	t.Helper()
	if got := m.themePanel.list.Paginator.PerPage; got != want {
		t.Fatalf("fixture: the panel's list paginates %d row(s) per page, want %d", got, want)
	}
}

func arrowPanelIndex(m Model) int { return m.themePanel.list.Index() }

// `bubbles/list`'s Update has a value receiver, so driving it directly observes
// without mutating the model.
func requireArrowUnskippedLandingAt(t *testing.T, m Model, press tea.KeyPressMsg, want int) {
	t.Helper()
	raw, _ := m.themePanel.list.Update(press)
	if got := raw.Index(); got != want {
		t.Fatalf("fixture: unskipped, %v lands the list at index %d, want the unselectable %d", press, got, want)
	}
	if item, ok := raw.SelectedItem().(themeRowItem); !ok || item.Row.Selectable() {
		t.Fatalf("fixture: unskipped, %v lands on a SELECTABLE row, so there is no skip to exercise", press)
	}
}

func requireArrowCursorAt(t *testing.T, m Model, want int) {
	t.Helper()
	if got := arrowPanelIndex(m); got != want {
		t.Fatalf("the panel cursor is at index %d, want %d; rows: %v", got, want, themePanelRowLabels(m))
	}
}

func TestPanelArrow_NavigationBindings(t *testing.T) {
	rows := arrowValidRows(t, 6)
	m := newArrowPanelModelAt(t, rows, arrowSlug(0), arrowPagingTermH)
	requireArrowPanelPageSize(t, m, arrowPagingPerPage)
	requireArrowCursorAt(t, m, 0)

	m = pressPanelKey(t, m, arrowDown)
	requireArrowCursorAt(t, m, 1)

	m = pressPanelKey(t, m, arrowUp)
	requireArrowCursorAt(t, m, 0)

	m = pressPanelKey(t, m, arrowPageDown)
	if got := arrowPanelIndex(m); got <= 1 {
		t.Fatalf("Ctrl+↓ moved the panel cursor from index 0 to %d — it steps one row rather than paging, so it does nothing `↓` does not", got)
	}
	requireArrowCursorAt(t, m, arrowPagingPerPage)
	if got := m.themePanel.list.Paginator.Page; got != 1 {
		t.Errorf("Ctrl+↓ left the paginator on page %d, want 1 — it pages rather than stepping", got)
	}

	m = pressPanelKey(t, m, arrowPageUp)
	requireArrowCursorAt(t, m, 0)

	if !m.themePanel.open {
		t.Error("navigating closed the panel; arrows leave it open")
	}
}

// `bubbles/list` v2's DefaultKeyMap binds `←`/`→`/`d` to pages, and `d` is the
// dark-slot commit key — one reaching the list would page instead of committing.
func TestPanelArrow_ArrowOnlyNavigation(t *testing.T) {
	rows := arrowValidRows(t, 6)
	m := newArrowPanelModelAt(t, rows, arrowSlug(2), arrowPagingTermH)
	requireArrowPanelPageSize(t, m, arrowPagingPerPage)

	km := m.themePanel.list.KeyMap
	for _, tc := range []struct {
		name    string
		binding key.Binding
		want    []string
	}{
		{name: "CursorUp", binding: km.CursorUp, want: []string{"up"}},
		{name: "CursorDown", binding: km.CursorDown, want: []string{"down"}},
		{name: "PrevPage", binding: km.PrevPage, want: []string{"ctrl+up"}},
		{name: "NextPage", binding: km.NextPage, want: []string{"ctrl+down"}},
		{name: "GoToStart", binding: km.GoToStart, want: nil},
		{name: "GoToEnd", binding: km.GoToEnd, want: nil},
	} {
		if got := tc.binding.Keys(); strings.Join(got, ",") != strings.Join(tc.want, ",") {
			t.Errorf("the panel list's %s binds %v, want %v — the panel is arrow-only", tc.name, got, tc.want)
		}
	}

	start := arrowPanelIndex(m)
	for _, press := range []tea.KeyPressMsg{
		{Code: 'j', Text: "j"},
		{Code: 'k', Text: "k"},
		{Code: 'h', Text: "h"},
		{Code: 'l', Text: "l"},
		{Code: 'g', Text: "g"},
		{Code: 'G', Text: "G"},
		{Code: 'd', Text: "d"},
		{Code: tea.KeyLeft},
		{Code: tea.KeyRight},
		{Code: tea.KeyPgUp},
		{Code: tea.KeyPgDown},
		{Code: tea.KeyHome},
		{Code: tea.KeyEnd},
		{Code: ' ', Text: " "},
	} {
		moved := pressPanelKey(t, m, press)
		if got := arrowPanelIndex(moved); got != start {
			t.Errorf("%v moved the panel cursor to index %d, want it left at %d — only arrows navigate", press, got, start)
		}
		if !moved.themePanel.open {
			t.Errorf("%v closed the panel", press)
		}
	}
}

func arrowFrameCells(t *testing.T, m Model) [][]bgState {
	t.Helper()
	lines := strings.Split(m.View().Content, "\n")
	cells := make([][]bgState, 0, len(lines))
	for _, line := range lines {
		cells = append(cells, scanCellBackgrounds(line))
	}
	return cells
}

func arrowPanelColumn(m Model) int {
	left := (m.termWidth-m.contentWidth())/2 + m.contentWidth() - themePanelPreferredWidth
	// +1 clears the one-cell left border (the border token, not the canvas); the
	// cell it lands on is the panel's inner gutter — canvas on every row.
	return left + 1
}

func arrowMainColumn(m Model) int {
	return (m.termWidth - m.contentWidth()) / 2
}

func TestPanelArrow_PreviewsThroughApplyTheme(t *testing.T) {
	rows := arrowValidRows(t, 4)
	before, after := rows[0].Theme, rows[1].Theme
	m := newArrowPanelModel(t, rows, rows[0].Slug)

	if m.themeState.active != before {
		t.Fatalf("fixture: the open painted canvas %s, want the cursor row's %s", m.themeState.active.Canvas.Value, before.Canvas.Value)
	}
	beforeParams, afterParams := canvasBgParams(before.Canvas.Color()), canvasBgParams(after.Canvas.Color())
	if beforeParams == afterParams {
		t.Fatalf("fixture: the two rows paint identically (%s), so a diff proves nothing", beforeParams)
	}

	beforeCells := arrowFrameCells(t, m)
	m = pressPanelKey(t, m, arrowDown)
	afterCells := arrowFrameCells(t, m)

	if m.themeState.active != after {
		t.Fatalf("the arrow left the active theme at canvas %s, want the newly-selected row's %s", m.themeState.active.Canvas.Value, after.Canvas.Value)
	}

	row := arrowTermH / 2
	for _, tc := range []struct {
		name string
		col  int
	}{
		{name: "the main screen behind the panel", col: arrowMainColumn(m)},
		{name: "the panel's own chrome", col: arrowPanelColumn(m)},
	} {
		if got := beforeCells[row][tc.col].params; got != beforeParams {
			t.Fatalf("fixture: %s was painted %q before the arrow, want the pre-swap canvas %q", tc.name, got, beforeParams)
		}
		if got := afterCells[row][tc.col].params; got != afterParams {
			t.Errorf("%s is painted %q after the arrow, want the previewed canvas %q — every surface re-themes", tc.name, got, afterParams)
		}
	}
}

func TestPanelArrow_SkipsConsecutiveInvalidRows(t *testing.T) {
	rows := []theme.Row{
		arrowValidRow(t, "aaa", 0),
		arrowInvalidRow("bbb"),
		arrowInvalidRow("ccc"),
		arrowInvalidRow("ddd"),
		arrowValidRow(t, "eee", 1),
	}
	m := newArrowPanelModel(t, rows, "aaa")

	m = pressPanelKey(t, m, arrowDown)

	requireArrowCursorAt(t, m, 4)
	requireCursorOn(t, m, "eee")
	if row := themePanelCursorRow(t, m); !row.Selectable() {
		t.Errorf("the cursor rests on the unselectable %q", row.Label())
	}
	if want := rows[4].Theme; m.themeState.active != want {
		t.Errorf("the skip landed without previewing: canvas %s, want %s", m.themeState.active.Canvas.Value, want.Canvas.Value)
	}
}

func TestPanelArrow_SkipReversesAtTheBoundary(t *testing.T) {
	t.Run("upward into a leading invalid block", func(t *testing.T) {
		rows := []theme.Row{
			arrowInvalidRow("aaa"),
			arrowInvalidRow("bbb"),
			arrowValidRow(t, "ccc", 0),
			arrowValidRow(t, "ddd", 1),
		}
		m := newArrowPanelModel(t, rows, "ccc")
		requireArrowUnskippedLandingAt(t, m, arrowUp, 1)
		requireArrowMoves(t, m, arrowDown, 3)

		m = pressPanelKey(t, m, arrowUp)

		requireArrowCursorAt(t, m, 2)
		requireCursorOn(t, m, "ccc")
		if want := rows[2].Theme; m.themeState.active != want {
			t.Errorf("the reversal moved the preview to canvas %s, want the unchanged %s", m.themeState.active.Canvas.Value, want.Canvas.Value)
		}
	})

	t.Run("downward into a trailing invalid block", func(t *testing.T) {
		rows := []theme.Row{
			arrowValidRow(t, "aaa", 0),
			arrowValidRow(t, "bbb", 1),
			arrowInvalidRow("ccc"),
			arrowInvalidRow("ddd"),
		}
		m := newArrowPanelModel(t, rows, "bbb")
		requireArrowUnskippedLandingAt(t, m, arrowDown, 2)
		requireArrowMoves(t, m, arrowUp, 0)

		m = pressPanelKey(t, m, arrowDown)

		requireArrowCursorAt(t, m, 1)
		requireCursorOn(t, m, "bbb")
		if want := rows[1].Theme; m.themeState.active != want {
			t.Errorf("the reversal moved the preview to canvas %s, want the unchanged %s", m.themeState.active.Canvas.Value, want.Canvas.Value)
		}
	})
}

func requireArrowMoves(t *testing.T, m Model, press tea.KeyPressMsg, want int) {
	t.Helper()
	if got := arrowPanelIndex(pressPanelKey(t, m, press)); got != want {
		t.Fatalf("fixture: %v left the cursor at index %d, want %d — the arrow arm is not live here, so the reversal below proves nothing", press, got, want)
	}
}

func TestPanelArrow_SkipComposesWithPaging(t *testing.T) {
	rows := []theme.Row{
		arrowValidRow(t, "aaa", 0),
		arrowValidRow(t, "bbb", 1),
		arrowInvalidRow("ccc"),
		arrowValidRow(t, "ddd", 2),
	}
	m := newArrowPanelModelAt(t, rows, "aaa", arrowPagingTermH)
	requireArrowPanelPageSize(t, m, arrowPagingPerPage)
	requireArrowUnskippedLandingAt(t, m, arrowPageDown, 2)

	m = pressPanelKey(t, m, arrowPageDown)

	requireArrowCursorAt(t, m, 3)
	requireCursorOn(t, m, "ddd")
	if got := m.themePanel.list.Paginator.Page; got != 1 {
		t.Errorf("the skip left the paginator on page %d, want the page Ctrl+↓ moved to (1)", got)
	}
	if want := rows[3].Theme; m.themeState.active != want {
		t.Errorf("the paged landing did not preview: canvas %s, want %s", m.themeState.active.Canvas.Value, want.Canvas.Value)
	}
}

func TestPanelArrow_NoFileReadPerKeystroke(t *testing.T) {
	dir := t.TempDir()
	themetest.Write(t, dir, "aurora.theme", themetest.MonochromeLines("#101010"))
	themetest.Write(t, dir, "sunset.theme", themetest.MonochromeLines("#202020"))
	loader, _ := themeOpenTestLoader(t)
	enumerator := countingThemeSourceOver(loader, dir)

	m := New(fakeLister{},
		WithThemeSource(enumerator),
		WithThemeKeys(theme.RawKeys{Theme: "aurora"}),
		WithThemeNomination(theme.ConstantNomination(testDarkTheme(t))),
	)
	m = pressThemeKey(t, m)
	requireCursorOn(t, m, "aurora")
	if got := m.themeState.active.Canvas.Value; got != "#101010" {
		t.Fatalf("fixture: the open painted canvas %s, want the drop-in's #101010", got)
	}

	if err := os.RemoveAll(dir); err != nil {
		t.Fatalf("remove the themes directory: %v", err)
	}

	const maxPresses = 12
	landed := false
	for range maxPresses {
		m = pressPanelKey(t, m, arrowDown)
		if themePanelCursorRow(t, m).Label() == "sunset" {
			landed = true
			break
		}
	}
	if !landed {
		t.Fatalf("the cursor never reached the `sunset` row in %d presses; rows: %v", maxPresses, themePanelRowLabels(m))
	}
	if got := m.themeState.active.Canvas.Value; got != "#202020" {
		t.Errorf("after the directory was removed the preview painted canvas %s, want the retained parse's #202020", got)
	}
	if enumerator.opens != 1 {
		t.Errorf("arrowing ran %d enumerations, want the single one the open performed", enumerator.opens)
	}
}

// The derived-dir cache is emptied after the pre-render: a warm cache would make
// "zero reads" vacuous.
func newArrowRebuildProbeModel(t *testing.T, rows []theme.Row, reader *fakeStamper) Model {
	t.Helper()

	deps := newArrowPanelDeps(t, rows, rows[0].Slug)
	deps.InitialMode = prefs.ModeByProject
	deps.DirReader = reader
	deps.DirRunner = &fakeDirRunner{gitRoot: reader.path}

	projects := []project.Project{{Path: reader.path, Name: "Portal", Tags: []string{"work"}}}
	sessions := make([]tmux.Session, 0, swapProbeSessions)
	for i := range swapProbeSessions {
		sessions = append(sessions, tmux.Session{Name: nameN(i), Windows: 1})
	}

	m := Build(deps)
	m.termWidth, m.termHeight = arrowTermW, arrowTermH
	m.applySessionListSize(m.contentWidth(), m.contentHeight())
	m.setProjects(projects)
	m.projectList.SetItems(ProjectsToListItems(projects))
	m.applyProjectListSize(m.contentWidth(), m.contentHeight())
	m.applySessions(sessions)
	_ = m.viewSessionList()

	m.derivedDirs = nil
	return pressThemeKey(t, m)
}

func TestPanelArrow_DoesNotRebuildSessionList(t *testing.T) {
	rows := arrowValidRows(t, 4)
	reader := &fakeStamper{path: t.TempDir()}
	m := newArrowRebuildProbeModel(t, rows, reader)

	reader.reads = nil
	for range 10 {
		m = pressPanelKey(t, m, arrowDown)
		if arrowPanelIndex(m) == 0 {
			t.Fatal("fixture: `↓` did not move the panel cursor, so a read count of zero says nothing about the preview path")
		}
		m = pressPanelKey(t, m, arrowUp)
	}

	if len(reader.reads) != 0 {
		t.Errorf("twenty arrow presses performed %d lazy pane read(s) %v — a preview is the O(1) restyle, never the rebuild's dir-resolution pass", len(reader.reads), reader.reads)
	}
	if got := m.View().Content; !strings.Contains(got, tokenBgSeq(t, m.themeState.active.Canvas)) {
		t.Errorf("the post-arrow frame does not carry the previewed canvas, so the zero above is a swap that never happened")
	}
	if len(reader.reads) != 0 {
		t.Errorf("rendering after an arrow performed %d lazy pane read(s) %v — the render path pays no reads either", len(reader.reads), reader.reads)
	}

	m.rebuildSessionList()
	if len(reader.reads) == 0 {
		t.Fatal("positive control: rebuildSessionList performed no pane reads, so the counting DirReader proves nothing about the arrow path")
	}
}

func TestPanelArrow_StartupCanvasHexUnmoved(t *testing.T) {
	rows := arrowValidRows(t, 6)
	m := newArrowPanelModel(t, rows, rows[0].Slug)

	startup := m.themeState.startupCanvasHex
	if startup != rows[0].Theme.Canvas.Value {
		t.Fatalf("fixture: startupCanvasHex = %q, want the launch canvas %q", startup, rows[0].Theme.Canvas.Value)
	}

	m = pressPanelKey(t, m, arrowDown)
	if m.themeState.active == rows[0].Theme {
		t.Fatal("fixture: one arrow did not change the active theme, so the assertion below is vacuous")
	}
	if m.themeState.startupCanvasHex != startup {
		t.Errorf("after one arrow startupCanvasHex = %q, want %q unchanged — it is frozen at gate resolution", m.themeState.startupCanvasHex, startup)
	}

	for range 25 {
		m = pressPanelKey(t, m, arrowDown)
		m = pressPanelKey(t, m, arrowUp)
	}
	if m.themeState.startupCanvasHex != startup {
		t.Errorf("after fifty arrows startupCanvasHex = %q, want %q unchanged", m.themeState.startupCanvasHex, startup)
	}
}

func TestPanelArrow_WritesNothing(t *testing.T) {
	t.Run("arrowing persists no preference", func(t *testing.T) {
		persister := &countingModePersister{}
		themePersister := &countingThemePersister{}
		dir := t.TempDir()
		themetest.Write(t, dir, "sunset.theme", themetest.MonochromeLines("#101010"))
		m := themeCursorModel(t, dir, theme.RawKeys{Theme: "sunset"}, theme.MemberDark)
		WithModePersister(persister)(&m)
		WithThemePersister(themePersister)(&m)

		m = pressThemeKey(t, m)
		launch := m.themeState.active
		for range 8 {
			m = pressPanelKey(t, m, arrowDown)
			if m.themeState.active == launch {
				t.Fatal("fixture: `↓` previewed nothing, so a zero write count says nothing about the preview path")
			}
			m = pressPanelKey(t, m, arrowUp)
		}

		if persister.calls != 0 {
			t.Errorf("sixteen arrows persisted %d preference(s); every write is an explicit commit keypress", persister.calls)
		}
		// The preview seam the panel would leak through: a persisted theme is a
		// commit, and the mode persister would never see it.
		if themePersister.calls != 0 {
			t.Errorf("sixteen arrows committed %d theme(s) through the theme persister; a preview persists nothing", themePersister.calls)
		}

		m = pressPanelKey(t, closeThemePanelForTest(t, m), tea.KeyPressMsg{Code: 's', Text: "s"})
		if persister.calls != 1 {
			t.Fatalf("positive control: `s` on the closed picker persisted %d time(s), want 1 — the counting persister proves nothing about the arrows", persister.calls)
		}
	})

	t.Run("the themes directory is untouched", func(t *testing.T) {
		dir := t.TempDir()
		themetest.Write(t, dir, "sunset.theme", themetest.MonochromeLines("#101010"))
		m := themeCursorModel(t, dir, theme.RawKeys{Theme: "sunset"}, theme.MemberDark)

		m = pressThemeKey(t, m)
		launch := m.themeState.active
		for range 8 {
			m = pressPanelKey(t, m, arrowDown)
		}
		if m.themeState.active == launch {
			t.Fatal("fixture: arrowing previewed nothing, so an untouched directory says nothing about the preview path")
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read %s: %v", dir, err)
		}
		if len(entries) != 1 || entries[0].Name() != "sunset.theme" {
			t.Errorf("the themes directory holds %d entries after arrowing, want only the seeded drop-in", len(entries))
		}
	})
}

// `bubbles/list` reads its pagination dot strings out of the styles once at
// construction, so the rendered dot row is diffed, not just the styles.
func TestPanelArrow_PanelListStylesRepointed(t *testing.T) {
	before, after := probeThemeBefore(t), probeThemeAfter(t)
	rows := make([]theme.Row, 0, 20)
	for i := range 20 {
		palette := before
		if i == 1 {
			palette = after
		}
		rows = append(rows, theme.Row{Slug: arrowSlug(i), Source: theme.SourceBuiltin, Theme: palette})
	}
	m := newArrowPanelModel(t, rows, arrowSlug(0))

	const height = 16
	dotsBefore := themePanelDotRow(t, renderThemePanel(m.themePanel, height, m.themeState.active, m.colourless))

	m = pressPanelKey(t, m, arrowDown)
	if m.themeState.active != after {
		t.Fatalf("fixture: the arrow left the active theme at canvas %s, want %s", m.themeState.active.Canvas.Value, after.Canvas.Value)
	}
	panel := renderThemePanel(m.themePanel, height, m.themeState.active, m.colourless)
	dotsAfter := themePanelDotRow(t, panel)

	if dotsAfter == dotsBefore {
		t.Fatal("the rendered dot row is byte-identical across the swap — the paginator is still printing the pre-swap dots")
	}
	assertRepointed(t, "the panel's rendered active dot",
		dotsAfter, tokenFgSeq(t, after.AccentPrimary), tokenFgSeq(t, before.AccentPrimary))
	assertRepointed(t, "the panel's rendered inactive dot",
		dotsAfter, tokenFgSeq(t, after.TextFaint), tokenFgSeq(t, before.TextFaint))

	l := &m.themePanel.list
	assertRepointed(t, "the panel list's Paginator.ActiveDot",
		l.Paginator.ActiveDot, tokenFgSeq(t, after.AccentPrimary), tokenFgSeq(t, before.AccentPrimary))
	assertRepointed(t, "the panel list's Paginator.InactiveDot",
		l.Paginator.InactiveDot, tokenFgSeq(t, after.TextFaint), tokenFgSeq(t, before.TextFaint))
	assertRepointed(t, "the panel list's Styles.HelpStyle",
		l.Styles.HelpStyle.Render("x"), tokenBgSeq(t, after.Canvas), tokenBgSeq(t, before.Canvas))
	assertRepointed(t, "the panel list's Styles.TitleBar",
		l.Styles.TitleBar.Render("x"), tokenBgSeq(t, after.Canvas), tokenBgSeq(t, before.Canvas))
	assertRepointed(t, "the panel list's Styles.NoItems",
		l.Styles.NoItems.Render("x"), tokenFgSeq(t, after.TextMuted), tokenFgSeq(t, before.TextMuted))
	if titleRun := l.Styles.Title.Render("x"); strings.ContainsRune(titleRun, '\x1b') {
		t.Errorf("the panel list's Styles.Title emits colour %q — nothing paints the title box", escSeq(titleRun))
	}

	assertRepointed(t, "the panel's row delegate (cursor-row label)",
		panel, tokenFgSeq(t, after.TextOnSelection), tokenFgSeq(t, before.TextOnSelection))
}

func TestPanelArrow_ColourlessStaysColourless(t *testing.T) {
	const (
		fgTruecolor = "38;2;"
		bgTruecolor = "48;2;"
	)
	rows := arrowValidRows(t, 4)

	build := func(colourless bool) Model {
		deps := newArrowPanelDeps(t, rows, rows[0].Slug)
		deps.NoColor = colourless
		m := Build(deps)
		m.termWidth, m.termHeight = arrowTermW, arrowTermH
		m.applySessions([]tmux.Session{{Name: "alpha", Windows: 1}, {Name: "bravo", Windows: 2}})
		if colourless {
			m = armPanelUnderNoColorForTest(t, m)
		} else {
			m = pressThemeKey(t, m)
		}
		m = pressPanelKey(t, m, arrowDown)
		if arrowPanelIndex(m) != 1 {
			t.Fatalf("fixture (colourless=%v): `↓` left the cursor at index %d, so the frame below is not a post-preview one", colourless, arrowPanelIndex(m))
		}
		return m
	}

	frame := build(true).View().Content
	if strings.Contains(frame, fgTruecolor) {
		t.Errorf("the post-preview colourless frame carries a %q foreground sequence: %q", fgTruecolor, escSeq(frame))
	}
	if strings.Contains(frame, bgTruecolor) {
		t.Errorf("the post-preview colourless frame carries a %q background sequence: %q", bgTruecolor, escSeq(frame))
	}

	control := build(false).View().Content
	if !strings.Contains(control, fgTruecolor) || !strings.Contains(control, bgTruecolor) {
		t.Fatal("positive control: the COLOURED post-preview frame carries no truecolor SGR, so the colourless assertions prove nothing")
	}
}

func TestPanelArrow_SameRowIsANoOp(t *testing.T) {
	rows := arrowValidRows(t, 3)
	m := newArrowPanelModel(t, rows, rows[0].Slug)
	first := m.View().Content

	t.Run("an arrow that cannot move renders an identical frame", func(t *testing.T) {
		blocked := pressPanelKey(t, m, arrowUp)
		if got := blocked.View().Content; got != first {
			t.Errorf("`↑` on the first row changed the frame\nbefore: %q\nafter:  %q", escSeq(first), escSeq(got))
		}
	})

	t.Run("landing back on the active row restores the identical frame", func(t *testing.T) {
		moved := pressPanelKey(t, m, arrowDown)
		if moved.View().Content == first {
			t.Fatal("fixture: `↓` did not change the frame, so the round trip proves nothing")
		}
		back := pressPanelKey(t, moved, arrowUp)
		if got := back.View().Content; got != first {
			t.Errorf("`↓` then `↑` did not restore the frame\nbefore: %q\nafter:  %q", escSeq(first), escSeq(got))
		}
	})

	t.Run("repeated swaps are idempotent per swap", func(t *testing.T) {
		once := pressPanelKey(t, m, arrowDown).View().Content
		twice := pressPanelKey(t, pressPanelKey(t, pressPanelKey(t, m, arrowDown), arrowUp), arrowDown).View().Content
		if twice != once {
			t.Errorf("A→B→A→B does not render what A→B renders; the preview accumulates state\nonce:  %q\ntwice: %q", escSeq(once), escSeq(twice))
		}
	})
}
