package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/themetest"
)

const fixtureThemesDir = "/fixture/themes"

// A zero resolution names no slot, which the open's degrade policy reads as
// "leave all three as they were": such cases assert the cadence, not the result.
func newOpenThemeSource(union theme.Union) *fakeThemeSource {
	return &fakeThemeSource{
		enumeration: theme.Enumeration{DirPath: fixtureThemesDir},
		union:       union,
	}
}

// The production adapter embedded rather than restated, so counting is the only
// behaviour added: a re-implementation could drift and still compile.
type countingThemeSource struct {
	theme.DirThemeSource
	opens int
}

func (e *countingThemeSource) Open(keys theme.RawKeys) (theme.Enumeration, theme.Union) {
	e.opens++
	return e.DirThemeSource.Open(keys)
}

func countingThemeSourceOver(loader theme.Loader, dir string) *countingThemeSource {
	return &countingThemeSource{DirThemeSource: theme.DirThemeSource{Loader: loader, Dir: dir}}
}

func themeOpenTestUnion() theme.Union {
	rows := []theme.Row{
		{Slug: "nord", Source: theme.SourcePersisted, Rejection: &theme.Rejection{Reason: theme.ReasonNotFound}},
		{Slug: theme.DefaultDarkSlug, Source: theme.SourceBuiltin},
	}
	return themeRowsUnion(rows)
}

func themeOpenTestModel(t *testing.T, enumerator ThemeSource, keys theme.RawKeys) Model {
	t.Helper()
	return Build(Deps{
		Lister:      fakeLister{},
		ThemeSource: enumerator,
		ThemeKeys:   keys,
	})
}

// `bubbles/list` will not focus a filter input over an empty list, and a swallowed
// row action has to have a row to act on.
func themeOpenTestPopulatedModel(t *testing.T, enumerator ThemeSource) Model {
	t.Helper()
	m := NewModelWithSessions([]Session{{Name: "alpha"}, {Name: "bravo"}})
	m.sessionKiller = keymapParityKiller{}
	m.projectList.SetItems(ProjectsToListItems([]project.Project{{Path: "/p/one", Name: "one"}}))
	m.projectList.Select(0)
	WithThemeSource(enumerator)(&m)
	return m
}

func pressThemeKey(t *testing.T, m Model) Model {
	t.Helper()
	return pressPanelKey(t, m, tea.KeyPressMsg{Code: 't', Text: "t"})
}

func pressPanelKey(t *testing.T, m Model, msg tea.KeyPressMsg) Model {
	t.Helper()
	updated, _ := m.Update(msg)
	return updated.(Model)
}

func closeThemePanelForTest(t *testing.T, m Model) Model {
	t.Helper()
	return pressPanelKey(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
}

func themeOpenTestLoader(t *testing.T) (theme.Loader, *logtest.Sink) {
	t.Helper()
	logger, sink := logtest.NewCaptureLogger(t)
	return theme.NewLoader(theme.NewEventLogger(logger)), sink
}

func countThemeEvents(sink *logtest.Sink, msg string) int {
	return len(sink.Records().WithMessage(msg))
}

func themePanelRowFor(t *testing.T, m Model, label string) themeRowItem {
	t.Helper()
	for _, item := range m.themePanel.list.Items() {
		row, ok := item.(themeRowItem)
		if !ok {
			t.Fatalf("panel item %#v is not a themeRowItem", item)
		}
		if row.Row.Label() == label {
			return row
		}
	}
	t.Fatalf("panel has no row labelled %q (rows: %v)", label, themePanelRowLabels(m))
	return themeRowItem{}
}

func themePanelRowLabels(m Model) []string {
	labels := make([]string, 0, len(m.themePanel.list.Items()))
	for _, item := range m.themePanel.list.Items() {
		if row, ok := item.(themeRowItem); ok {
			labels = append(labels, row.Row.Label())
		}
	}
	return labels
}

func TestThemePanelOpen_BoundOnBothPages(t *testing.T) {
	for _, tc := range []struct {
		name string
		page page
	}{
		{name: "sessions", page: PageSessions},
		{name: "projects", page: PageProjects},
	} {
		t.Run(tc.name, func(t *testing.T) {
			union := themeOpenTestUnion()
			enumerator := newOpenThemeSource(union)
			m := themeOpenTestModel(t, enumerator, theme.RawKeys{Theme: "nord"})
			m.activePage = tc.page

			m = pressThemeKey(t, m)

			if !m.themePanel.open {
				t.Fatalf("t on %s did not open the panel", tc.name)
			}
			if enumerator.opens != 1 {
				t.Errorf("the keypress ran %d enumerations, want exactly 1", enumerator.opens)
			}
			if got, want := len(m.themePanel.list.Items()), len(union.Rows); got != want {
				t.Errorf("panel list holds %d items, want %d — one per union row", got, want)
			}
			if m.themePanel.union.Count != union.Count {
				t.Errorf("panel union count = %d, want the enumerator's %d", m.themePanel.union.Count, union.Count)
			}
			if got := m.themePanel.enumeration.DirPath; got != fixtureThemesDir {
				t.Errorf("panel enumeration DirPath = %q, want the enumerator's %q — the open retains BOTH returned values", got, fixtureThemesDir)
			}
			if m.themePanel.width != themePanelPreferredWidth {
				t.Errorf("panel width = %d, want the preferred %d", m.themePanel.width, themePanelPreferredWidth)
			}
			if m.activePage != tc.page {
				t.Errorf("opening the panel changed the active page to %d", m.activePage)
			}
		})
	}
}

func TestThemePanelOpen_FilterCarveOut(t *testing.T) {
	t.Run("sessions", func(t *testing.T) {
		enumerator := newOpenThemeSource(themeOpenTestUnion())
		m := themeOpenTestPopulatedModel(t, enumerator)

		m = pressPanelKey(t, m, tea.KeyPressMsg{Code: '/', Text: "/"})
		if !m.sessionList.SettingFilter() {
			t.Fatal("precondition: the sessions filter input is not focused after /")
		}
		m = pressThemeKey(t, m)

		if m.themePanel.open {
			t.Error("t opened the panel while the / filter was focused")
		}
		if enumerator.opens != 0 {
			t.Errorf("a literal filter character ran %d enumerations, want 0", enumerator.opens)
		}
		if got := m.sessionList.FilterValue(); got != "t" {
			t.Errorf("sessions filter query = %q, want %q — t must reach the input as text", got, "t")
		}
	})

	t.Run("projects", func(t *testing.T) {
		enumerator := newOpenThemeSource(themeOpenTestUnion())
		m := themeOpenTestPopulatedModel(t, enumerator)
		m.activePage = PageProjects

		m = pressPanelKey(t, m, tea.KeyPressMsg{Code: '/', Text: "/"})
		if !m.projectList.SettingFilter() {
			t.Fatal("precondition: the projects filter input is not focused after /")
		}
		m = pressThemeKey(t, m)

		if m.themePanel.open {
			t.Error("t opened the panel while the / filter was focused")
		}
		if enumerator.opens != 0 {
			t.Errorf("a literal filter character ran %d enumerations, want 0", enumerator.opens)
		}
		if got := m.projectList.FilterValue(); got != "t" {
			t.Errorf("projects filter query = %q, want %q — t must reach the input as text", got, "t")
		}
	})
}

// The directory is populated, so a construction-time sweep would be visible as
// both a call and a log record.
func TestThemePanelOpen_NoEnumerationAtConstruction(t *testing.T) {
	dir := t.TempDir()
	themetest.Write(t, dir, "sunset.theme", themetest.MonochromeLines("#101010"))
	loader, sink := themeOpenTestLoader(t)
	enumerator := countingThemeSourceOver(loader, dir)

	m := themeOpenTestModel(t, enumerator, theme.RawKeys{})

	if enumerator.opens != 0 {
		t.Errorf("construction ran %d enumerations, want 0 — discovery is lazy", enumerator.opens)
	}
	if got := countThemeEvents(sink, "enumerated"); got != 0 {
		t.Errorf("construction emitted %d `theme: enumerated` records, want 0", got)
	}

	m = pressThemeKey(t, m)

	if enumerator.opens != 1 {
		t.Errorf("the keypress ran %d enumerations, want exactly 1", enumerator.opens)
	}
	if got := countThemeEvents(sink, "enumerated"); got != 1 {
		t.Errorf("the keypress emitted %d `theme: enumerated` records, want exactly 1", got)
	}
	themePanelRowFor(t, m, "sunset")
}

// `theme: enumerated` is a per-event INFO with no dedup, so three opens are
// three records.
func TestThemePanelOpen_ReEnumeratesPerOpen(t *testing.T) {
	dir := t.TempDir()
	themetest.Write(t, dir, "sunset.theme", themetest.MonochromeLines("#101010"))
	loader, sink := themeOpenTestLoader(t)
	enumerator := countingThemeSourceOver(loader, dir)
	m := themeOpenTestModel(t, enumerator, theme.RawKeys{})

	const opens = 3
	for i := range opens {
		m = pressThemeKey(t, m)
		if !m.themePanel.open {
			t.Fatalf("open %d did not open the panel", i+1)
		}
		m = closeThemePanelForTest(t, m)
	}

	if enumerator.opens != opens {
		t.Errorf("%d opens ran %d enumerations, want %d — the read is per open", opens, enumerator.opens, opens)
	}
	if got := countThemeEvents(sink, "enumerated"); got != opens {
		t.Errorf("%d opens emitted %d `theme: enumerated` records, want %d (per event, no dedup)", opens, got, opens)
	}
}

func TestThemePanelOpen_SeesAMidSessionEdit(t *testing.T) {
	dir := t.TempDir()
	themetest.Write(t, dir, "sunset.theme", themetest.MonochromeLines("not-a-colour"))
	loader, _ := themeOpenTestLoader(t)
	m := themeOpenTestModel(t, countingThemeSourceOver(loader, dir), theme.RawKeys{})

	m = pressThemeKey(t, m)
	if broken := themePanelRowFor(t, m, "sunset"); broken.Row.Selectable() {
		t.Fatalf("precondition: the bad-colour file came back selectable")
	}
	m = closeThemePanelForTest(t, m)

	themetest.Write(t, dir, "sunset.theme", themetest.MonochromeLines("#101010"))
	m = pressThemeKey(t, m)

	fixed := themePanelRowFor(t, m, "sunset")
	if !fixed.Row.Selectable() {
		t.Errorf("the repaired file is still rejected (%v) — the second open must re-read the directory", fixed.Row.Rejection)
	}
}

func TestThemePanelOpen_EnumerationDiscardedOnClose(t *testing.T) {
	dir := t.TempDir()
	// Seeded before the first open so the retained enumeration has an entry to
	// carry, and named other than `sunset` so the later write is a mutation.
	themetest.Write(t, dir, "aurora.theme", themetest.MonochromeLines("#101010"))
	loader, _ := themeOpenTestLoader(t)
	m := themeOpenTestModel(t, countingThemeSourceOver(loader, dir), theme.RawKeys{})

	m = pressThemeKey(t, m)
	if len(m.themePanel.union.Rows) == 0 {
		t.Fatal("precondition: the first open retained no rows")
	}
	if got := m.themePanel.enumeration; len(got.Entries) != 1 || got.DirPath != dir {
		t.Errorf("the open retained %d entries read from %q, want the 1 seeded entry read from %q", len(got.Entries), got.DirPath, dir)
	}
	m = closeThemePanelForTest(t, m)

	if m.themePanel.open {
		t.Fatal("Esc left the panel open")
	}
	if got := m.themePanel.enumeration; len(got.Entries) != 0 || got.DirPath != "" {
		t.Errorf("close retained the enumeration %+v, want the zero value", got)
	}
	if got := m.themePanel.union; len(got.Rows) != 0 || got.Count != 0 {
		t.Errorf("close retained the union %+v, want the zero value", got)
	}
	if got := len(m.themePanel.list.Items()); got != 0 {
		t.Errorf("close retained %d list items, want 0", got)
	}

	themetest.Write(t, dir, "sunset.theme", themetest.MonochromeLines("#101010"))
	m = pressThemeKey(t, m)
	themePanelRowFor(t, m, "sunset")
}

// The staged prefs file puts a different answer on disk at the path the config
// layer resolves to, so a re-read wired on by any route changes what this records.
func TestThemePanelOpen_UsesConstructionTimePrefsSnapshot(t *testing.T) {
	prefsFile := filepath.Join(t.TempDir(), "prefs.json")
	if err := os.WriteFile(prefsFile, []byte(`{"theme":"nord"}`), 0o644); err != nil {
		t.Fatalf("write prefs: %v", err)
	}
	t.Setenv("PORTAL_PREFS_FILE", prefsFile)

	construction := theme.RawKeys{Light: theme.DefaultLightSlug, Dark: "nord"}
	enumerator := newOpenThemeSource(themeOpenTestUnion())
	m := themeOpenTestModel(t, enumerator, construction)

	m = pressThemeKey(t, m)
	m = closeThemePanelForTest(t, m)

	if err := os.WriteFile(prefsFile, []byte(`{"theme":"tokyo-night-day"}`), 0o644); err != nil {
		t.Fatalf("rewrite prefs: %v", err)
	}
	m = pressThemeKey(t, m)

	if len(enumerator.keys) != 2 {
		t.Fatalf("the seam was asked %d times, want 2", len(enumerator.keys))
	}
	for i, got := range enumerator.keys {
		if got != construction {
			t.Errorf("open %d handed the seam %+v, want the construction-time snapshot %+v", i+1, got, construction)
		}
	}
	if len(enumerator.resolves) == 0 {
		t.Fatal("the seam was asked to resolve nothing across two opens")
	}
	for i, got := range enumerator.resolves {
		if got != construction {
			t.Errorf("resolution %d handed the seam %+v, want the construction-time snapshot %+v", i+1, got, construction)
		}
	}
	if m.themeState.keys != construction {
		t.Errorf("model keys = %+v, want the construction-time snapshot %+v", m.themeState.keys, construction)
	}
}

func TestThemePanelOpen_BadgesFromTheSeamsResolution(t *testing.T) {
	enumerator := newOpenThemeSource(themeOpenTestUnion())
	enumerator.resolution = theme.Resolution{
		Nomination: theme.ConstantNomination(testDarkTheme(t)),
		Slots: []theme.SlotResolution{{
			Slot:      theme.SlotConstant,
			Requested: "nord",
			Resolved:  theme.DefaultDarkSlug,
			FellBack:  true,
			Reason:    theme.ReasonNotFound,
			Theme:     testDarkTheme(t),
		}},
	}
	m := themeOpenTestModel(t, enumerator, theme.RawKeys{Theme: "nord"})

	m = pressThemeKey(t, m)

	if got := themePanelRowFor(t, m, "nord").Badge; got != theme.BadgeConstant {
		t.Errorf("the persisted row's badge = %v, want %v — the ● marks what is SET", got, theme.BadgeConstant)
	}
	if got := themePanelRowFor(t, m, theme.DefaultDarkSlug).Badge; got != theme.BadgeNone {
		t.Errorf("the fallback row's badge = %v, want %v — a fallback must never move the ●", got, theme.BadgeNone)
	}
}

func TestThemePanelOpen_NilSeamIsASilentNoOp(t *testing.T) {
	m := themeOpenTestModel(t, nil, theme.RawKeys{Theme: "nord"})

	m = pressThemeKey(t, m)

	if m.themePanel.open {
		t.Error("t opened the panel with no theme source wired")
	}
	if m.flashText != "" {
		t.Errorf("t raised %q with no seam wired; the nil-seam path must be silent", m.flashText)
	}
}

// `bubbles/list` reads its dot strings out of its styles once, so library
// defaults render identically before and after a swap — invisible to a
// swap-and-diff guard. The seed is the dark built-in, hence the light case.
func TestThemePanelOpen_ThemesThePaginationDots(t *testing.T) {
	rows := themePanelTestRows(20)
	union := themeRowsUnion(rows)

	forEachBuiltinTheme(t, func(t *testing.T, th theme.Theme) {
		m := themeOpenTestModel(t, newOpenThemeSource(union), theme.RawKeys{})
		m.themeState.active = th

		m = pressThemeKey(t, m)

		row := themePanelDotRow(t, renderThemePanel(m.themePanel, 16, th, false))
		for _, want := range []struct {
			role  string
			token theme.Token
		}{
			{role: "active dot accent.primary", token: th.AccentPrimary},
			{role: "inactive dot text.faint", token: th.TextFaint},
		} {
			if seq := tokenFgSeq(t, want.token); !strings.Contains(row, seq) {
				t.Errorf("the panel dot row is missing the %s sequence %q:\n%q", want.role, seq, row)
			}
		}
		for _, grey := range bubblesDefaultDotGreys {
			if strings.Contains(row, grey) {
				t.Errorf("the panel dot row still carries the bubbles/list default grey %q — a colour belonging to no theme:\n%q", grey, row)
			}
		}
	})

	t.Run("colourless", func(t *testing.T) {
		m := themeOpenTestModel(t, newOpenThemeSource(union), theme.RawKeys{})
		m.colourless = true

		m = armPanelUnderNoColorForTest(t, m)

		row := themePanelDotRow(t, renderThemePanel(m.themePanel, 16, testDarkTheme(t), true))
		for col, cell := range scanCellBackgrounds(row) {
			if cell.set {
				t.Fatalf("colourless dot row col %d carries a background SGR (%s): %q", col, cell.params, escSeq(row))
			}
		}
		if strings.Contains(row, "38;2;") {
			t.Errorf("colourless dot row emits a foreground hue: %q", escSeq(row))
		}
	})
}

// `bubbles/list`'s own paginator dot colours as truecolor SGR parameters — what
// a panel list left at the library defaults renders under every theme.
var bubblesDefaultDotGreys = []string{"38;2;151;151;151", "38;2;60;60;60"}

func themePanelDotRow(t *testing.T, block string) string {
	t.Helper()
	for line := range strings.SplitSeq(block, "\n") {
		visible := strings.TrimSpace(ansi.Strip(line))
		if !strings.Contains(visible, paginationDotGlyph) {
			continue
		}
		if strings.Trim(visible, paginationDotGlyph+panelFrameSide+" ") == "" {
			return line
		}
	}
	t.Fatalf("no pagination dot row in the rendered panel:\n%s", ansi.Strip(block))
	return ""
}

func TestThemePanelOpen_SwallowsPageKeys(t *testing.T) {
	newModel := func(t *testing.T) Model {
		t.Helper()
		return themeOpenTestPopulatedModel(t, newOpenThemeSource(themeOpenTestUnion()))
	}

	for _, tc := range []struct {
		name    string
		press   tea.KeyPressMsg
		effect  func(Model) bool
		effectS string
	}{
		{
			name:    "k does not kill",
			press:   tea.KeyPressMsg{Code: 'k', Text: "k"},
			effect:  func(m Model) bool { return m.modal == modalKillConfirm },
			effectS: "the kill confirm modal",
		},
		{
			name:    "x does not switch page",
			press:   tea.KeyPressMsg{Code: 'x', Text: "x"},
			effect:  func(m Model) bool { return m.activePage == PageProjects },
			effectS: "the Projects page",
		},
		{
			name:    "m does not enter multi-select",
			press:   tea.KeyPressMsg{Code: 'm', Text: "m"},
			effect:  func(m Model) bool { return m.MultiSelectActive() },
			effectS: "multi-select mode",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			control := pressPanelKey(t, newModel(t), tc.press)
			if !tc.effect(control) {
				t.Fatalf("precondition: %v does not reach %s with the panel CLOSED, so a swallow proves nothing", tc.press, tc.effectS)
			}

			m := pressThemeKey(t, newModel(t))
			if !m.themePanel.open {
				t.Fatal("precondition: the panel did not open")
			}
			m = pressPanelKey(t, m, tc.press)

			if tc.effect(m) {
				t.Errorf("%v reached %s while the panel was open — the panel is key-exclusive", tc.press, tc.effectS)
			}
			if !m.themePanel.open {
				t.Errorf("%v closed the panel; only Esc closes it", tc.press)
			}
		})
	}

	t.Run("Ctrl-C stays live", func(t *testing.T) {
		m := pressThemeKey(t, newModel(t))
		_, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
		if !isQuitCmd(cmd) {
			t.Error("Ctrl-C did not quit while the panel was open — it must never be swallowed")
		}
	})

	t.Run("Esc closes", func(t *testing.T) {
		m := closeThemePanelForTest(t, pressThemeKey(t, newModel(t)))
		if m.themePanel.open {
			t.Error("Esc did not close the panel")
		}
	})
}
