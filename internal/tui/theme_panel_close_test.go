package tui

import (
	"go/ast"
	"os"
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/themetest"
	"github.com/leeovery/portal/internal/tmux"
)

func newClosePanelModel(t *testing.T, dir string, keys theme.RawKeys) (Model, *countingThemeSource, *logtest.Sink) {
	t.Helper()

	loader, sink := themeOpenTestLoader(t)
	m, enumerator := newDirBackedPanelModelOver(t, dir, keys, theme.MemberDark, loader)
	return m, enumerator, sink
}

func closePanelForTest(t *testing.T, m Model) (Model, tea.Cmd) {
	t.Helper()
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	return updated.(Model), cmd
}

func closePanelSessions() []tmux.Session {
	return []tmux.Session{
		{Name: "alpha", Windows: 3, Attached: true},
		{Name: "bravo", Windows: 1},
		{Name: "charlie", Windows: 2},
	}
}

func newClosePanelStubModel(t *testing.T, rows []theme.Row) (Model, *fakeThemeSource) {
	t.Helper()
	deps := newArrowPanelDeps(t, rows, rows[0].Slug)
	stub, ok := deps.ThemeSource.(*fakeThemeSource)
	if !ok {
		t.Fatalf("the arrow deps' seam is %T, want the recording fake", deps.ThemeSource)
	}
	m := Build(deps)
	m.termWidth, m.termHeight = arrowTermW, arrowTermH
	m.applySessions(closePanelSessions(), nil)
	return m, stub
}

func newClosePanelLayoutModel(t *testing.T) Model {
	t.Helper()
	m, _ := newClosePanelStubModel(t, arrowValidRows(t, 4))
	projects := []project.Project{{Path: "/p/one", Name: "one"}, {Path: "/p/two", Name: "two"}}
	m.setProjects(projects)
	m.projectList.SetItems(ProjectsToListItems(projects))
	m.applySessionListSize(m.contentWidth(), m.contentHeight())
	m.applyProjectListSize(m.contentWidth(), m.contentHeight())
	return m
}

func TestPanelClose_DiscardsThePreview(t *testing.T) {
	rows := arrowValidRows(t, 6)
	m, stub := newClosePanelStubModel(t, rows)
	before := m.View().Content

	m = pressThemeKey(t, m)
	if !m.themePanel.open {
		t.Fatal("fixture: the panel did not open")
	}
	if len(stub.resolves) != 1 {
		t.Fatalf("the open ran %d resolutions, want exactly 1", len(stub.resolves))
	}
	for range 3 {
		m = pressPanelKey(t, m, arrowDown)
	}
	if m.themeState.active == rows[0].Theme {
		t.Fatal("fixture: three arrows previewed nothing, so the close below restores nothing")
	}
	if len(stub.resolves) != 1 {
		t.Errorf("three arrows ran %d resolutions in total, want the open's 1 — an arrow previews from the retained parse", len(stub.resolves))
	}

	m, cmd := closePanelForTest(t, m)

	if cmd != nil {
		t.Errorf("Esc scheduled %T; closing is ONE frame — no animation, no transition, no intermediate width", cmd)
	}
	if m.themePanel.open {
		t.Fatal("Esc left the panel open")
	}
	if len(stub.resolves) != 2 {
		t.Errorf("the close ran %d resolutions in total, want 2 — `Esc` RE-RESOLVES persisted state rather than restoring a snapshot", len(stub.resolves))
	}
	if m.themeState.active != rows[0].Theme {
		t.Errorf("the close left canvas %s, want the resolved persisted %s", m.themeState.active.Canvas.Value, rows[0].Theme.Canvas.Value)
	}
	if got := m.View().Content; got != before {
		t.Errorf("the post-close frame is not the pre-open frame\nbefore: %q\nafter:  %q", escSeq(before), escSeq(got))
	}
}

func TestPanelClose_ResolvesEditedValues(t *testing.T) {
	dir := t.TempDir()
	themetest.Write(t, dir, "sunset.theme", themetest.MonochromeLines("#101010"))
	m, _, _ := newClosePanelModel(t, dir, theme.RawKeys{Theme: "sunset"})
	if got := m.themeState.active.Canvas.Value; got != "#101010" {
		t.Fatalf("precondition: the launch rendered canvas %s, want the drop-in's #101010", got)
	}

	themetest.Write(t, dir, "sunset.theme", themetest.MonochromeLines("#202020"))
	m = pressThemeKey(t, m)
	if got := m.themeState.active.Canvas.Value; got != "#202020" {
		t.Fatalf("precondition: the open rendered canvas %s, want the edited #202020", got)
	}
	m = pressPanelKey(t, m, arrowDown)
	if m.themeState.active.Canvas.Value == "#202020" {
		t.Fatal("fixture: the arrow previewed nothing, so the close below restores nothing")
	}

	m = closeThemePanelForTest(t, m)

	if got := m.themeState.active.Canvas.Value; got != "#202020" {
		t.Errorf("the close rendered canvas %s, want the enumeration's edited #202020 — #101010 would be the palette held when the panel opened, which `Esc` must not restore", got)
	}
}

func TestPanelClose_ResolvesToFallback(t *testing.T) {
	dir := t.TempDir()
	themetest.Write(t, dir, "sunset.theme", themetest.MonochromeLines("#101010"))
	m, _, _ := newClosePanelModel(t, dir, theme.RawKeys{Theme: "sunset"})
	if got := m.themeState.active.Canvas.Value; got != "#101010" {
		t.Fatalf("precondition: the launch rendered canvas %s, want the drop-in's #101010", got)
	}

	themetest.Write(t, dir, "sunset.theme", themetest.MonochromeLines("not-a-colour"))
	m = pressThemeKey(t, m)
	fallback := testDarkTheme(t)
	if m.themeState.active != fallback {
		t.Fatalf("precondition: the open rendered canvas %s, want the fallback's %s", m.themeState.active.Canvas.Value, fallback.Canvas.Value)
	}
	m = pressPanelKey(t, m, arrowUp)
	if m.themeState.active == fallback {
		t.Fatal("fixture: the arrow previewed nothing, so the close below restores nothing")
	}

	m = closeThemePanelForTest(t, m)

	if m.themeState.active != fallback {
		t.Errorf("the close rendered canvas %s, want the fallback's %s — the persisted `sunset` no longer resolves, and Portal shows what the config NOW says", m.themeState.active.Canvas.Value, fallback.Canvas.Value)
	}
}

func TestPanelClose_ReadsNothing(t *testing.T) {
	dir := t.TempDir()
	themetest.Write(t, dir, "aurora.theme", themetest.MonochromeLines("#101010"))
	m, enumerator, _ := newClosePanelModel(t, dir, theme.RawKeys{Theme: "aurora"})

	m = pressThemeKey(t, m)
	if got := m.themeState.active.Canvas.Value; got != "#101010" {
		t.Fatalf("precondition: the open rendered canvas %s, want the drop-in's #101010", got)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatalf("remove the themes directory: %v", err)
	}
	m = pressPanelKey(t, m, arrowDown)
	if m.themeState.active.Canvas.Value == "#101010" {
		t.Fatal("fixture: the arrow previewed nothing, so the close below restores nothing")
	}

	m = closeThemePanelForTest(t, m)

	if got := m.themeState.active.Canvas.Value; got != "#101010" {
		t.Errorf("with the themes directory gone the close rendered canvas %s, want the RETAINED parse's #101010 — the close resolves against the panel's enumeration, never the filesystem", got)
	}
	if enumerator.opens != 1 {
		t.Errorf("the close ran %d enumerations in total, want the single one the open performed", enumerator.opens)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("the themes directory exists again after the close (err %v); nothing on this path touches it", err)
	}
}

func TestPanelClose_EnumerationDiscarded(t *testing.T) {
	dir := t.TempDir()
	themetest.Write(t, dir, "aurora.theme", themetest.MonochromeLines("#101010"))
	m, enumerator, _ := newClosePanelModel(t, dir, theme.RawKeys{Theme: "aurora"})

	m = pressThemeKey(t, m)
	if got := m.themePanel.enumeration; len(got.Entries) != 1 || got.DirPath != dir {
		t.Fatalf("precondition: the open retained %d entries read from %q, want the 1 seeded entry read from %q", len(got.Entries), got.DirPath, dir)
	}
	if len(m.themePanel.badges) == 0 {
		t.Fatal("precondition: the open populated no badges, so clearing them proves nothing")
	}
	if len(m.themePanel.list.Items()) == 0 {
		t.Fatal("precondition: the open listed no rows")
	}

	m = closeThemePanelForTest(t, m)

	if m.themePanel.open {
		t.Fatal("Esc left the panel open")
	}
	if got := m.themePanel.enumeration; len(got.Entries) != 0 || got.DirPath != "" {
		t.Errorf("close retained the enumeration %d entries from %q, want the zero value", len(got.Entries), got.DirPath)
	}
	if got := m.themePanel.union; len(got.Rows) != 0 || got.Count != 0 || got.Rejected != 0 || got.DirUnusable {
		t.Errorf("close retained the union %+v, want the zero value", got)
	}
	if got := m.themePanel.badges; got != nil {
		t.Errorf("close retained the badge table %v, want none", got)
	}
	if got := m.themePanel.message; got.Kind != themeMessageNone {
		t.Errorf("close retained the message %+v, want it cleared", got)
	}
	if got := m.themePanel.width; got != 0 {
		t.Errorf("close retained width %d, want 0", got)
	}
	if got := len(m.themePanel.list.Items()); got != 0 {
		t.Errorf("close retained %d list items, want 0", got)
	}
	if w, h := m.themePanel.list.Width(), m.themePanel.list.Height(); w != 0 || h != 0 {
		t.Errorf("close retained a list sized %dx%d, want the zero value", w, h)
	}
	// `bubbles/list` exports no delegate accessor, so the keymap stands in for
	// the delegate: a live list binds `up`, the zero value nothing.
	if got := m.themePanel.list.KeyMap.CursorUp.Keys(); len(got) != 0 {
		t.Errorf("close retained the list's keymap (CursorUp binds %v), so the list — and the delegate it carries — was not replaced by the zero value", got)
	}

	themetest.Write(t, dir, "sunset.theme", themetest.MonochromeLines("#202020"))
	m = pressThemeKey(t, m)

	themePanelRowFor(t, m, "sunset")
	if enumerator.opens != 2 {
		t.Errorf("two opens ran %d enumerations, want 2 — the discard is what makes the next open re-read", enumerator.opens)
	}
}

func TestPanelClose_WritesNothing(t *testing.T) {
	t.Run("no persister is reached", func(t *testing.T) {
		mode := &countingModePersister{}
		committer := &fakeThemePersister{}
		dir := t.TempDir()
		themetest.Write(t, dir, "sunset.theme", themetest.MonochromeLines("#101010"))
		m, _, _ := newClosePanelModel(t, dir, theme.RawKeys{Theme: "sunset"})
		WithModePersister(mode)(&m)
		WithThemePersister(committer)(&m)

		m = pressThemeKey(t, m)
		m = pressPanelKey(t, m, arrowDown)
		m, cmd := closePanelForTest(t, m)

		if mode.calls != 0 {
			t.Errorf("closing persisted %d preference(s); every write is an explicit keypress", mode.calls)
		}
		if len(committer.slugs) != 0 {
			t.Errorf("closing committed %v; nothing writes on close", committer.slugs)
		}
		if cmd != nil {
			t.Errorf("closing scheduled %T; a deferred write is the one shape the counters above cannot see", cmd)
		}

		m = pressPanelKey(t, m, tea.KeyPressMsg{Code: 's', Text: "s"})
		if mode.calls != 1 {
			t.Fatalf("positive control: `s` on the closed picker persisted %d time(s), want 1 — the counting persister proves nothing about the close", mode.calls)
		}
	})

	requireNoPrefsOrThemesWrite(t, panelReadOnlyPath{
		verb:          "closing",
		absentSubtest: "an absent prefs.json stays absent and the themes directory is untouched",
	}, func(t *testing.T, dir string, keys theme.RawKeys) {
		m, _, _ := newClosePanelModel(t, dir, keys)
		closeThemePanelForTest(t, pressPanelKey(t, pressThemeKey(t, m), arrowDown))
	})
}

func TestPanelClose_EventCadence(t *testing.T) {
	dir := t.TempDir()
	themetest.Write(t, dir, "sunset.theme", themetest.MonochromeLines("not-a-colour"))
	m, enumerator, sink := newClosePanelModel(t, dir, theme.RawKeys{Theme: "sunset"})

	const cycles = 10
	for i := range cycles {
		m = pressThemeKey(t, m)
		if !m.themePanel.open {
			t.Fatalf("cycle %d did not open the panel", i+1)
		}
		m = closeThemePanelForTest(t, m)
		if m.themePanel.open {
			t.Fatalf("cycle %d did not close the panel", i+1)
		}
	}

	if got := countThemeEvents(sink, "fallback applied"); got != 1 {
		t.Errorf("%d open/close cycles emitted %d `theme: fallback applied` records, want exactly 1 — the WARN dedups per process on slug+reason", cycles, got)
	}
	if got := countThemeEvents(sink, "loaded"); got != 0 {
		t.Errorf("%d open/close cycles emitted %d `theme: loaded` records, want 0 — its cadence is construction plus the one commit-time load", cycles, got)
	}

	if got := countThemeEvents(sink, "enumerated"); got != cycles {
		t.Fatalf("%d opens emitted %d `theme: enumerated` records, want %d — the sink is not recording this loader", cycles, got, cycles)
	}
	setting, _ := theme.ResolveSetting(theme.RawKeys{Theme: "sunset"})
	if _, err := enumerator.Loader.ResolveNomination(setting, dir); err != nil {
		t.Fatalf("positive-control by-name resolution: %v", err)
	}
	if countThemeEvents(sink, "loaded") == 0 {
		t.Fatal("positive control: a by-name resolution through the SAME loader emitted no `theme: loaded`, so the zero above says nothing about the panel path")
	}
}

func TestPanelClose_DoesNotClearTheFilter(t *testing.T) {
	m := themeOpenTestPopulatedModel(t, newOpenThemeSource(themeOpenTestUnion()))

	m = pressPanelKey(t, m, tea.KeyPressMsg{Code: '/', Text: "/"})
	m = pressPanelKey(t, m, tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = pressPanelKey(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := m.sessionList.FilterState(); got != list.FilterApplied {
		t.Fatalf("precondition: the sessions filter state is %v, want FilterApplied", got)
	}

	m = pressThemeKey(t, m)
	if !m.themePanel.open {
		t.Fatal("precondition: the panel did not open over the applied filter")
	}
	m = closeThemePanelForTest(t, m)

	if m.themePanel.open {
		t.Fatal("Esc left the panel open")
	}
	if got := m.sessionList.FilterState(); got != list.FilterApplied {
		t.Errorf("the sessions filter state is %v after the close, want it left FilterApplied — Esc is consumed by the panel", got)
	}
	if got := m.sessionList.FilterValue(); got != "a" {
		t.Errorf("the sessions filter query = %q after the close, want the applied %q", got, "a")
	}

	m = closeThemePanelForTest(t, m)
	if got := m.sessionList.FilterState(); got == list.FilterApplied {
		t.Fatal("positive control: Esc on the closed picker left the filter applied, so the assertion above says nothing about the panel")
	}
}

func TestPanelClose_NestsOverMultiSelect(t *testing.T) {
	m := newClosePanelLayoutModel(t)

	m = pressPanelKey(t, m, tea.KeyPressMsg{Code: 'm', Text: "m"})
	if !m.MultiSelectActive() {
		t.Fatal("precondition: `m` did not enter multi-select")
	}
	marked := m.SelectedSessionCount()
	if marked == 0 {
		t.Fatal("precondition: entering multi-select marked nothing, so an intact set proves nothing")
	}

	m = pressThemeKey(t, m)
	if !m.themePanel.open {
		t.Fatal("precondition: `t` did not open the panel over multi-select")
	}
	m = closeThemePanelForTest(t, m)

	if m.themePanel.open {
		t.Fatal("Esc left the panel open")
	}
	if !m.MultiSelectActive() {
		t.Error("the close exited multi-select; Esc resolves innermost-first")
	}
	if got := m.SelectedSessionCount(); got != marked {
		t.Errorf("the marked set holds %d session(s) after the close, want the %d marked before", got, marked)
	}
	if !m.IsSessionSelected("alpha") {
		t.Error("the auto-marked `alpha` is no longer selected after the close")
	}
	if got := ansi.Strip(m.View().Content); !strings.Contains(got, "1 selected") {
		t.Errorf("the post-close frame carries no `1 selected` banner; it sits in the notice band and must survive the close")
	}

	m = closeThemePanelForTest(t, m)
	if m.MultiSelectActive() {
		t.Fatal("positive control: Esc on the closed picker stayed in multi-select, so the assertion above says nothing about the panel")
	}
}

func TestPanelClose_EscDoesNotQuit(t *testing.T) {
	for _, tc := range []struct {
		name string
		page page
	}{
		{name: "sessions", page: PageSessions},
		{name: "projects", page: PageProjects},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newClosePanelLayoutModel(t)
			m.activePage = tc.page

			_, control := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
			if !isQuitCmd(control) {
				t.Fatalf("precondition: Esc on the closed %s page does not quit, so the assertion below proves nothing", tc.name)
			}

			m = pressThemeKey(t, m)
			if !m.themePanel.open {
				t.Fatalf("precondition: `t` did not open the panel on %s", tc.name)
			}
			m, cmd := closePanelForTest(t, m)

			if isQuitCmd(cmd) {
				t.Errorf("Esc quit Portal from the open panel on %s; it closes the panel and nothing else", tc.name)
			}
			if m.themePanel.open {
				t.Errorf("Esc did not close the panel on %s", tc.name)
			}
			if m.activePage != tc.page {
				t.Errorf("the close moved the active page to %d, want it left on %d", m.activePage, tc.page)
			}
		})
	}
}

func TestPanelClose_PageLayoutUnchangedAcrossOpenAndClose(t *testing.T) {
	for _, tc := range []struct {
		name string
		page page
		size func(Model) (int, int)
	}{
		{name: "sessions", page: PageSessions, size: Model.SessionListSize},
		{name: "projects", page: PageProjects, size: Model.ProjectListSize},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newClosePanelLayoutModel(t)
			m.activePage = tc.page

			beforeW, beforeH := tc.size(m)
			if beforeW <= 0 || beforeH <= 0 {
				t.Fatalf("fixture: the %s list is sized %dx%d, so an unchanged size says nothing", tc.name, beforeW, beforeH)
			}

			m = pressThemeKey(t, m)
			if !m.themePanel.open {
				t.Fatalf("precondition: `t` did not open the panel on %s", tc.name)
			}
			duringW, duringH := tc.size(m)
			if duringW != beforeW || duringH != beforeH {
				t.Errorf("the open re-laid-out the %s list to %dx%d, want the unreduced %dx%d", tc.name, duringW, duringH, beforeW, beforeH)
			}

			m = closeThemePanelForTest(t, m)
			afterW, afterH := tc.size(m)
			if afterW != beforeW || afterH != beforeH {
				t.Errorf("the close re-laid-out the %s list to %dx%d, want the unreduced %dx%d — the page was never reduced, so there is nothing to reclaim", tc.name, afterW, afterH, beforeW, beforeH)
			}
		})
	}
}

func TestPanelClose_ForcedCloseUsesTheSameFunction(t *testing.T) {
	t.Run("the panel is discarded in exactly one place", func(t *testing.T) {
		sites := panelDiscardSites(t)
		if len(sites) != 1 || sites[0] != "closeThemePanel" {
			t.Errorf("the panel struct is zeroed in %v, want exactly [closeThemePanel] — the forced close and the page hooks must have one close to route through, not a second to drift from", sites)
		}
	})

	t.Run("a direct call is the whole close", func(t *testing.T) {
		rows := arrowValidRows(t, 4)
		open := func() Model {
			m, _ := newClosePanelStubModel(t, rows)
			m = pressThemeKey(t, m)
			if !m.themePanel.open {
				t.Fatal("fixture: the panel did not open")
			}
			m = pressPanelKey(t, m, arrowDown)
			if m.themeState.active == rows[0].Theme {
				t.Fatal("fixture: the arrow previewed nothing, so the two closes have nothing to undo")
			}
			return m
		}

		viaEsc := closeThemePanelForTest(t, open())
		direct := open()
		(&direct).closeThemePanel()

		if direct.themePanel.open {
			t.Fatal("the direct call left the panel open")
		}
		if direct.themeState.active != viaEsc.themeState.active {
			t.Errorf("the direct call rendered canvas %s and Esc rendered %s; the forced close takes the Esc path EXACTLY", direct.themeState.active.Canvas.Value, viaEsc.themeState.active.Canvas.Value)
		}
		if got, want := direct.View().Content, viaEsc.View().Content; got != want {
			t.Errorf("the direct call's frame is not Esc's\ndirect: %q\nesc:    %q", escSeq(got), escSeq(want))
		}
	})
}

func panelDiscardSites(t *testing.T) []string {
	t.Helper()
	var sites []string
	for _, file := range parsePackageFilesByName(t) {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				assign, ok := n.(*ast.AssignStmt)
				if !ok {
					return true
				}
				for i, lhs := range assign.Lhs {
					sel, ok := lhs.(*ast.SelectorExpr)
					if !ok || sel.Sel.Name != "themePanel" || i >= len(assign.Rhs) {
						continue
					}
					lit, ok := assign.Rhs[i].(*ast.CompositeLit)
					if !ok || len(lit.Elts) != 0 {
						continue
					}
					if ident, ok := lit.Type.(*ast.Ident); ok && ident.Name == "themePanel" {
						sites = append(sites, fn.Name.Name)
					}
				}
				return true
			})
		}
	}
	return sites
}
