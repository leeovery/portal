package tui

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/themetest"
	"github.com/leeovery/portal/internal/tmux"
)

// Dimensions are assigned directly, not through a tea.WindowSizeMsg: a fixture
// that resized on its way in could not tell the open ladder from the resize one.
func openPanelForTest(t *testing.T, m Model, contentW, contentH int) Model {
	t.Helper()
	return openPanelForTestWithSessions(t, m, contentW, contentH, closePanelSessions())
}

func openPanelForTestWithSessions(t *testing.T, m Model, contentW, contentH int, sessions []tmux.Session) Model {
	t.Helper()

	m.termWidth, m.termHeight = geometryTerm(contentW, contentH)
	m.applySessions(sessions, nil)
	m.applySessionListSize(m.contentWidth(), m.contentHeight())
	m.applyProjectListSize(m.contentWidth(), m.contentHeight())

	if got := m.contentWidth(); got != contentW {
		t.Fatalf("fixture: the content region is %d columns wide, want %d", got, contentW)
	}
	if got := m.contentHeight(); got != contentH {
		t.Fatalf("fixture: the content region is %d rows tall, want %d", got, contentH)
	}

	m = pressThemeKey(t, m)
	if !m.themePanel.open {
		t.Fatal("fixture: the panel did not open")
	}
	return m
}

// The light/dark answer is pinned rather than detected, so one fixture drives
// the in-force slot in both terminals without touching the async race.
func newDirBackedPanelModel(t *testing.T, dir string, keys theme.RawKeys, mode theme.Member) (Model, *countingThemeSource) {
	t.Helper()
	return newDirBackedPanelModelOver(t, dir, keys, mode, theme.NewSilentLoader())
}

// Construction resolves through a loader of its own, so a caller passing a
// sink-backed loader gets the panel's emissions alone, not a delta.
func newDirBackedPanelModelOver(t *testing.T, dir string, keys theme.RawKeys, mode theme.Member, panelLoader theme.Loader) (Model, *countingThemeSource) {
	t.Helper()

	setting, _ := theme.ResolveSetting(keys)
	resolution, err := theme.NewSilentLoader().ResolveNomination(setting, dir)
	if err != nil {
		t.Fatalf("construction-time resolution of %+v: %v", setting, err)
	}
	enumerator := countingThemeSourceOver(panelLoader, dir)
	m := New(fakeLister{},
		WithThemeSource(enumerator),
		WithThemeKeys(keys),
		WithThemeNomination(resolution.Nomination),
		WithCanvasMode(mode),
	)
	return m, enumerator
}

// Every route from this package to disk runs through one of the counted seams:
// internal/tui resolves no config path and reads no PORTAL_* env var.
func requireCommitDoesNoOtherIO(
	t *testing.T,
	keys theme.RawKeys,
	subject string,
	press func(*testing.T, Model) (Model, tea.Cmd),
	assertCommitted func(*testing.T, *fakeThemePersister),
) {
	t.Helper()

	configDir := t.TempDir()
	t.Setenv("PORTAL_PREFS_FILE", filepath.Join(configDir, "prefs.json"))
	dir := t.TempDir()
	themetest.Write(t, dir, "sunset.theme", themetest.MonochromeLines("#101010"))

	stores := newCountingStores()
	persister := &fakeThemePersister{}
	loader := theme.NewSilentLoader()
	enumerator := countingThemeSourceOver(loader, dir)
	setting, _ := theme.ResolveSetting(keys)
	resolution, err := loader.ResolveNomination(setting, dir)
	if err != nil {
		t.Fatalf("construction-time resolution of %+v: %v", setting, err)
	}

	m := Build(Deps{
		Lister:         stores.lister,
		Theme:          resolution.Nomination,
		ProjectStore:   stores.projectStore,
		ProjectEditor:  stores.projectEditor,
		AliasEditor:    stores.aliasEditor,
		ModePersister:  stores.modePersister,
		Reader:         stores.scrollback,
		ThemeSource:    enumerator,
		ThemeKeys:      keys,
		ThemePersister: persister,
	})
	m.termWidth, m.termHeight = arrowTermW, arrowTermH
	m.applySessions(closePanelSessions(), nil)
	m = pressThemeKey(t, m)
	if !m.themePanel.open {
		t.Fatal("fixture: the panel did not open")
	}
	stores.reset()
	opens := enumerator.opens

	_, cmd := press(t, m)

	assertCommitted(t, persister)
	if got := stores.calls(); got != 0 {
		t.Errorf("%s made %d file-touching seam call(s) — the prefs write is the only one (project store %d, project editor %d, alias editor %d, mode persister %d, theme persister %d, scrollback %d, lister %d)",
			subject, got, stores.projectStore.calls, stores.projectEditor.calls, stores.aliasEditor.calls,
			stores.modePersister.calls, stores.themePersister.calls, stores.scrollback.calls, stores.lister.calls)
	}
	if enumerator.opens != opens {
		t.Errorf("%s ran %d enumerations in total, want the open's %d — a commit re-derives from the retained parse and never re-reads the directory", subject, enumerator.opens, opens)
	}
	if cmd != nil {
		t.Errorf("%s scheduled %T; a deferred write is the one shape the counters above cannot see", subject, cmd)
	}
	if entries, err := os.ReadDir(configDir); err != nil || len(entries) != 0 {
		t.Errorf("%s left %d entries in the config directory (err %v), want none — the model writes through the seam and touches no path of its own", subject, len(entries), err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	if len(entries) != 1 || entries[0].Name() != "sunset.theme" {
		t.Errorf("the themes directory holds %d entries after %s, want only the seeded drop-in", len(entries), subject)
	}
}

type panelReadOnlyPath struct {
	verb          string
	absentSubtest string
}

// act stages nothing of its own: it receives the staged themes directory and
// the keys naming the theme in it, and differs only in model and keypresses.
func requireNoPrefsOrThemesWrite(t *testing.T, readOnly panelReadOnlyPath, act func(t *testing.T, dir string, keys theme.RawKeys)) {
	t.Helper()

	const persisted = `{"session_list_mode":"by-project","theme":"sunset"}`
	keys := theme.RawKeys{Theme: "sunset"}

	t.Run("a present prefs.json survives byte for byte", func(t *testing.T) {
		prefsFile := filepath.Join(t.TempDir(), "prefs.json")
		if err := os.WriteFile(prefsFile, []byte(persisted), 0o644); err != nil {
			t.Fatalf("write prefs: %v", err)
		}
		t.Setenv("PORTAL_PREFS_FILE", prefsFile)
		dir := t.TempDir()
		// Deliberately invalid, so the path runs over the rejection fallback — the
		// case where a write would overwrite the name the user set.
		themetest.Write(t, dir, "sunset.theme", themetest.MonochromeLines("not-a-colour"))

		act(t, dir, keys)

		after, err := os.ReadFile(prefsFile)
		if err != nil {
			t.Fatalf("read back prefs: %v", err)
		}
		if string(after) != persisted {
			t.Errorf("prefs.json after %s =\n%s\nwant it byte-identical:\n%s", readOnly.verb, after, persisted)
		}
	})

	t.Run(readOnly.absentSubtest, func(t *testing.T) {
		configDir := t.TempDir()
		t.Setenv("PORTAL_PREFS_FILE", filepath.Join(configDir, "prefs.json"))
		dir := t.TempDir()
		themetest.Write(t, dir, "sunset.theme", themetest.MonochromeLines("#101010"))

		act(t, dir, keys)

		if entries, err := os.ReadDir(configDir); err != nil || len(entries) != 0 {
			t.Errorf("%s left %d entries in the config directory (err %v), want none", readOnly.verb, len(entries), err)
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read %s: %v", dir, err)
		}
		if len(entries) != 1 || entries[0].Name() != "sunset.theme" {
			t.Errorf("the themes directory holds %d entries after %s, want only the seeded drop-in", len(entries), readOnly.verb)
		}
	})
}

// Requested and Resolved come from one argument deliberately: badges key on the
// former and the cursor on the latter, so a transposed shape still resolves.
func constantResolution(slug string, th theme.Theme) theme.Resolution {
	return theme.Resolution{
		Nomination: theme.ConstantNomination(th),
		Slots: []theme.SlotResolution{{
			Slot:      theme.SlotConstant,
			Requested: slug,
			Resolved:  slug,
			Theme:     th,
		}},
	}
}

func pairResolution(light, dark theme.Row) theme.Resolution {
	return theme.Resolution{
		Nomination: theme.AdaptivePair(light.Theme, dark.Theme),
		Slots: []theme.SlotResolution{
			{Slot: theme.SlotLight, Requested: light.Slug, Resolved: light.Slug, Theme: light.Theme},
			{Slot: theme.SlotDark, Requested: dark.Slug, Resolved: dark.Slug, Theme: dark.Theme},
		},
	}
}

func stubPanelDeps(source *fakeThemeSource, nomination theme.Nomination, keys theme.RawKeys) Deps {
	return Deps{
		Lister:      fakeLister{},
		Theme:       nomination,
		ThemeSource: source,
		ThemeKeys:   keys,
	}
}

func TestConstantResolution_IsTheConstantStubShape(t *testing.T) {
	th := testDarkTheme(t)
	got := constantResolution("aurora", th)

	if !got.Nomination.IsConstant() {
		t.Errorf("the nomination is %+v, want the CONSTANT shape", got.Nomination)
	}
	if palette := got.Nomination.Constant(); palette != th {
		t.Errorf("the nomination carries canvas %s, want the given palette's %s", palette.Canvas.Value, th.Canvas.Value)
	}
	want := []theme.SlotResolution{{
		Slot:      theme.SlotConstant,
		Requested: "aurora",
		Resolved:  "aurora",
		Theme:     th,
	}}
	if !reflect.DeepEqual(got.Slots, want) {
		t.Errorf("the slots are %+v, want the single constant record %+v", got.Slots, want)
	}
}

func TestPairResolution_IsTheAdaptiveStubShape(t *testing.T) {
	light := theme.Row{Slug: "day", Source: theme.SourceBuiltin, Theme: testLightTheme(t)}
	dark := theme.Row{Slug: "night", Source: theme.SourceBuiltin, Theme: testDarkTheme(t)}
	got := pairResolution(light, dark)

	if got.Nomination.IsConstant() {
		t.Errorf("the nomination is %+v, want the ADAPTIVE shape", got.Nomination)
	}
	if palette := got.Nomination.Select(theme.MemberLight); palette != light.Theme {
		t.Errorf("the nomination's light member carries canvas %s, want %s", palette.Canvas.Value, light.Theme.Canvas.Value)
	}
	if palette := got.Nomination.Select(theme.MemberDark); palette != dark.Theme {
		t.Errorf("the nomination's dark member carries canvas %s, want %s", palette.Canvas.Value, dark.Theme.Canvas.Value)
	}
	want := []theme.SlotResolution{
		{Slot: theme.SlotLight, Requested: "day", Resolved: "day", Theme: light.Theme},
		{Slot: theme.SlotDark, Requested: "night", Resolved: "night", Theme: dark.Theme},
	}
	if !reflect.DeepEqual(got.Slots, want) {
		t.Errorf("the slots are %+v, want the light-then-dark pair %+v", got.Slots, want)
	}
}

// Returns the bare parameter run, not the whole `ESC[…m`: a style carrying
// foreground, background and attributes emits them as one sequence, so only the
// `38;2;…` / `48;2;…` core lands in composed output.
func sgrParams(t *testing.T, style lipgloss.Style) string {
	t.Helper()
	probe := style.Render("x")
	start := strings.IndexByte(probe, '[')
	end := strings.IndexByte(probe, 'm')
	if start < 0 || end <= start {
		t.Fatalf("could not derive an SGR parameter run from %q", probe)
	}
	return probe[start+1 : end]
}

func tokenFgSeq(t *testing.T, tok theme.Token) string {
	t.Helper()
	return sgrParams(t, lipgloss.NewStyle().Foreground(tok.Color()))
}

func tokenBgSeq(t *testing.T, tok theme.Token) string {
	t.Helper()
	return sgrParams(t, lipgloss.NewStyle().Background(tok.Color()))
}

// Functions, not package-level vars: the test side must not grow the shape the
// production guard against package-level theme state exists to prevent.
func testDarkTheme(t *testing.T) theme.Theme {
	t.Helper()
	return themetest.DefaultDark(t)
}

func testLightTheme(t *testing.T) theme.Theme {
	t.Helper()
	return themetest.DefaultLight(t)
}

type builtinThemeCase struct {
	name string
	th   theme.Theme
}

func builtinThemeCases(t *testing.T) []builtinThemeCase {
	t.Helper()
	return []builtinThemeCase{
		{"dark", testDarkTheme(t)},
		{"light", testLightTheme(t)},
	}
}

func forEachBuiltinTheme(t *testing.T, fn func(t *testing.T, th theme.Theme)) {
	t.Helper()
	for _, tc := range builtinThemeCases(t) {
		t.Run(tc.name, func(t *testing.T) {
			fn(t, tc.th)
		})
	}
}

func forEachCanvasMode(t *testing.T, fn func(t *testing.T, m theme.Member)) {
	t.Helper()
	for _, tc := range []struct {
		name string
		mode theme.Member
	}{
		{"dark", theme.MemberDark},
		{"light", theme.MemberLight},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fn(t, tc.mode)
		})
	}
}

// Identifies a theme by canvas — the token the built-ins can never share —
// without printing its whole palette into a failure message.
func themeLabel(th theme.Theme) string {
	switch v := th.Canvas.Value; v {
	case "":
		return "zero-theme"
	case builtinCanvasValue(theme.DefaultDarkSlug):
		return "dark"
	case builtinCanvasValue(theme.DefaultLightSlug):
		return "light"
	default:
		return v
	}
}

// Takes no *testing.T: themeLabel is evaluated inside an argument list, so a
// broken embedded file degrades to "", which themeLabel rules out before asking.
func builtinCanvasValue(slug string) string {
	loaded, rejection, found := theme.NewSilentLoader().LoadBuiltin(slug)
	if !found || rejection != nil {
		return ""
	}
	return loaded.Theme.Canvas.Value
}

// Resolves through All() so a role that stops existing fails loudly rather than
// yielding a zero token.
func tokenNamed(t *testing.T, th theme.Theme, name string) theme.Token {
	t.Helper()
	for _, tok := range th.All() {
		if tok.Name == name {
			return tok
		}
	}
	t.Fatalf("theme carries no token named %q", name)
	return theme.Token{}
}

func testConstantFor(t *testing.T, appearance theme.Member) theme.Nomination {
	t.Helper()
	return theme.ConstantNomination(themeForAppearance(t, appearance))
}

func appearanceForTheme(t *testing.T, th theme.Theme) theme.Member {
	t.Helper()
	switch th.Canvas.Value {
	case testLightTheme(t).Canvas.Value:
		return theme.MemberLight
	case testDarkTheme(t).Canvas.Value:
		return theme.MemberDark
	}
	t.Fatalf("theme with canvas %q is neither built-in", th.Canvas.Value)
	return theme.MemberDark
}
