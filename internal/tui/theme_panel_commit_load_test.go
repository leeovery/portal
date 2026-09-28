package tui

import (
	"errors"
	"io/fs"
	"os"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/themetest"
)

const conversionConstant = "aurora"

// Deliberately not any built-in's canvas, so a captured startup canvas hex is
// distinguishable from every palette a conversion could wrongly re-capture.
const conversionConstantValue = "#101010"

func newConversionThemesDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	themetest.Write(t, dir, conversionConstant+".theme", themetest.MonochromeLines(conversionConstantValue))
	return dir
}

func newConversionPanelModel(t *testing.T, dir string, keys theme.RawKeys) (Model, *fakeThemePersister, *logtest.Sink) {
	t.Helper()

	loader, sink := themeOpenTestLoader(t)
	m, persister := newLoadPanelModel(t, dir, keys, loader)
	if !m.themeState.nomination.IsConstant() {
		t.Fatalf("fixture: keys %+v resolved to an adaptive nomination; a conversion starts from a CONSTANT", keys)
	}
	return m, persister, sink
}

func newAdaptivePanelModel(t *testing.T, dir string, keys theme.RawKeys, reply tea.BackgroundColorMsg) (Model, *fakeThemePersister, *logtest.Sink) {
	t.Helper()

	loader, sink := themeOpenTestLoader(t)
	m, persister := newLoadPanelModel(t, dir, keys, loader)
	if m.themeState.nomination.IsConstant() {
		t.Fatalf("fixture: keys %+v resolved to a constant nomination; this fixture is the adaptive shape", keys)
	}
	m = deliverBackgroundReply(t, m, reply)
	if !m.modeResolved() {
		t.Fatal("fixture: the gate is still open after a reply; the panel must be driven against a resolved answer")
	}
	return m, persister, sink
}

func newLoadPanelModel(t *testing.T, dir string, keys theme.RawKeys, loader theme.Loader) (Model, *fakeThemePersister) {
	t.Helper()

	setting, _ := theme.ResolveSetting(keys)
	resolution, err := theme.NewSilentLoader().ResolveNomination(setting, dir)
	if err != nil {
		t.Fatalf("construction-time resolution of %+v: %v", setting, err)
	}
	persister := &fakeThemePersister{}
	m := Build(Deps{
		Lister:         fakeLister{},
		Theme:          resolution.Nomination,
		ThemeSource:    countingThemeSourceOver(loader, dir),
		ThemeKeys:      keys,
		ThemePersister: persister,
	})
	m.termWidth, m.termHeight = arrowTermW, arrowTermH
	m.applySessions(closePanelSessions(), nil)
	return m, persister
}

func deliverBackgroundReply(t *testing.T, m Model, reply tea.BackgroundColorMsg) Model {
	t.Helper()

	updated, cmd := m.Update(reply)
	if cmd != nil {
		t.Fatalf("the OSC 11 reply scheduled %T, want nothing", cmd)
	}
	return updated.(Model)
}

func openConversionPanel(t *testing.T, m Model) Model {
	t.Helper()

	m = pressThemeKey(t, m)
	if !m.themePanel.open {
		t.Fatal("fixture: the panel did not open")
	}
	requireCursorOn(t, m, conversionConstant)
	return m
}

func convertToSlot(t *testing.T, m Model, target string, press tea.KeyPressMsg) (Model, tea.Cmd) {
	t.Helper()

	m = arrowToThemeRow(t, m, target)
	m, _ = pressSlotKey(t, m, press)
	if got := m.themePanel.message; got.Kind != themeMessageConfirm {
		t.Fatalf("fixture: the slot key left the message %+v, want the confirm live over a constant", got)
	}
	return pressConfirmKey(t, m, confirmYes)
}

func requireLoadedLine(t *testing.T, sink *logtest.Sink, slug, slot string) {
	t.Helper()

	loaded := sink.Records().WithMessage("loaded").Only(t, "loaded record")
	if got := loaded.AttrString(t, "slug"); got != slug {
		t.Errorf("`theme: loaded` carries slug=%q, want %q", got, slug)
	}
	if got := loaded.AttrString(t, "slot"); got != slot {
		t.Errorf("`theme: loaded` carries slot=%q, want %q", got, slot)
	}
}

func requireNominationPair(t *testing.T, m Model, light, dark theme.Theme) {
	t.Helper()

	if m.themeState.nomination.IsConstant() {
		t.Fatalf("the nomination is still a CONSTANT after a conversion; the slot load joins the pair")
	}
	if got := m.themeState.nomination.Select(theme.MemberLight); got != light {
		t.Errorf("the nomination's light member is %s, want %s", themeLabel(got), themeLabel(light))
	}
	if got := m.themeState.nomination.Select(theme.MemberDark); got != dark {
		t.Errorf("the nomination's dark member is %s, want %s", themeLabel(got), themeLabel(dark))
	}
}

func requireNominationConstant(t *testing.T, m Model, want theme.Theme) {
	t.Helper()

	if !m.themeState.nomination.IsConstant() {
		t.Fatalf("the nomination is %+v, want the CONSTANT shape", m.themeState.nomination)
	}
	if got := m.themeState.nomination.Constant(); got != want {
		t.Errorf("the nomination's constant is %s, want %s", themeLabel(got), themeLabel(want))
	}
}

func requireNoFallbackLine(t *testing.T, sink *logtest.Sink) {
	t.Helper()

	if got := sink.Records().WithMessage("fallback applied"); len(got) != 0 {
		t.Errorf("the conversion emitted %d `theme: fallback applied` line(s), want none — the slot resolved\n%s", len(got), sink.Body())
	}
}

func requireSlotCanvas(t *testing.T, m Model, slot theme.Slot, want string) {
	t.Helper()

	got := m.themeState.nomination.Select(memberForSlot(slot))
	if got.Canvas.Value != want {
		t.Errorf("the nomination's %v member has canvas %q, want %q", slot, got.Canvas.Value, want)
	}
}

func TestCommitSlotLoad_LoadsTheOppositeSlot(t *testing.T) {
	for _, tc := range []struct {
		name  string
		press tea.KeyPressMsg
		slot  string
		slug  string
	}{
		{name: "l loads the dark slot", press: slotLightPress, slot: "dark", slug: theme.DefaultDarkSlug},
		{name: "d loads the light slot", press: slotDarkPress, slot: "light", slug: theme.DefaultLightSlug},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := newConversionThemesDir(t)
			m, _, sink := newConversionPanelModel(t, dir, theme.RawKeys{Theme: conversionConstant})
			m = openConversionPanel(t, m)

			m, _ = convertToSlot(t, m, "nord", tc.press)

			requireLoadedLine(t, sink, tc.slug, tc.slot)
			assigned := themetest.Builtin(t, "nord")
			loaded := themetest.Builtin(t, tc.slug)
			if tc.slot == "dark" {
				requireNominationPair(t, m, assigned, loaded)
			} else {
				requireNominationPair(t, m, loaded, assigned)
			}
		})
	}
}

func TestCommitSlotLoad_UntouchedSlotIsTheShippedDefault(t *testing.T) {
	dir := newConversionThemesDir(t)
	themetest.Write(t, dir, theme.DefaultLightSlug+".theme", themetest.MonochromeLines("#202020"))
	m, _, sink := newConversionPanelModel(t, dir, theme.RawKeys{Theme: conversionConstant})
	m = openConversionPanel(t, m)

	m, _ = convertToSlot(t, m, "nord", slotDarkPress)

	requireLoadedLine(t, sink, theme.DefaultLightSlug, "light")
	requireNoFallbackLine(t, sink)
	requireNominationPair(t, m, testLightTheme(t), themetest.Builtin(t, "nord"))
}

func TestCommitSlotLoad_StaleSlotFromEnumeration(t *testing.T) {
	t.Run("a stale slug resolves from the panel's parse", func(t *testing.T) {
		const opened, edited = "#202020", "#303030"
		dir := newConversionThemesDir(t)
		themetest.Write(t, dir, "ghostly.theme", themetest.MonochromeLines(opened))
		keys := theme.RawKeys{Theme: conversionConstant, Light: "ghostly"}
		m, _, sink := newConversionPanelModel(t, dir, keys)
		m = openConversionPanel(t, m)
		themetest.Write(t, dir, "ghostly.theme", themetest.MonochromeLines(edited))

		m, _ = convertToSlot(t, m, "nord", slotDarkPress)

		requireLoadedLine(t, sink, "ghostly", "light")
		requireNoFallbackLine(t, sink)
		requireSlotCanvas(t, m, theme.SlotLight, opened)
	})

	t.Run("a slug the enumeration has no entry for falls through to the embedded set", func(t *testing.T) {
		dir := newConversionThemesDir(t)
		keys := theme.RawKeys{Theme: conversionConstant, Light: "nord"}
		m, _, sink := newConversionPanelModel(t, dir, keys)
		m = openConversionPanel(t, m)

		m, _ = convertToSlot(t, m, theme.DefaultDarkSlug, slotDarkPress)

		requireLoadedLine(t, sink, "nord", "light")
		requireNoFallbackLine(t, sink)
		requireNominationPair(t, m, themetest.Builtin(t, "nord"), testDarkTheme(t))
	})
}

func TestCommitSlotLoad_UnresolvableTakesTheModeMatchedFallback(t *testing.T) {
	for _, tc := range []struct {
		name     string
		keys     theme.RawKeys
		press    tea.KeyPressMsg
		slot     theme.Slot
		slotAttr string
		slug     string
	}{
		{
			name:     "an unresolvable light slot falls to the light default",
			keys:     theme.RawKeys{Theme: conversionConstant, Light: "nope"},
			press:    slotDarkPress,
			slot:     theme.SlotLight,
			slotAttr: "light",
			slug:     theme.DefaultLightSlug,
		},
		{
			name:     "an unresolvable dark slot falls to the dark default",
			keys:     theme.RawKeys{Theme: conversionConstant, Dark: "nope"},
			press:    slotLightPress,
			slot:     theme.SlotDark,
			slotAttr: "dark",
			slug:     theme.DefaultDarkSlug,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := newConversionThemesDir(t)
			m, _, sink := newConversionPanelModel(t, dir, tc.keys)
			m = openConversionPanel(t, m)

			m, _ = convertToSlot(t, m, "nord", tc.press)

			requireLoadedLine(t, sink, tc.slug, tc.slotAttr)
			requireSlotCanvas(t, m, tc.slot, themetest.Builtin(t, tc.slug).Canvas.Value)
		})
	}
}

func TestCommitSlotLoad_NoDirectoryRead(t *testing.T) {
	t.Run("a stale slot still resolves from the retained parse", func(t *testing.T) {
		const value = "#202020"
		dir := newConversionThemesDir(t)
		themetest.Write(t, dir, "ghostly.theme", themetest.MonochromeLines(value))
		keys := theme.RawKeys{Theme: conversionConstant, Light: "ghostly"}
		m, _, sink := newConversionPanelModel(t, dir, keys)
		m = openConversionPanel(t, m)
		removeThemesDirForTest(t, dir)

		m, _ = convertToSlot(t, m, "nord", slotDarkPress)

		requireLoadedLine(t, sink, "ghostly", "light")
		requireNoFallbackLine(t, sink)
		requireSlotCanvas(t, m, theme.SlotLight, value)
	})

	t.Run("an untouched slot still resolves the shipped default", func(t *testing.T) {
		dir := newConversionThemesDir(t)
		m, _, sink := newConversionPanelModel(t, dir, theme.RawKeys{Theme: conversionConstant})
		m = openConversionPanel(t, m)
		removeThemesDirForTest(t, dir)

		m, _ = convertToSlot(t, m, "nord", slotDarkPress)

		requireLoadedLine(t, sink, theme.DefaultLightSlug, "light")
		requireNoFallbackLine(t, sink)
		requireNominationPair(t, m, testLightTheme(t), themetest.Builtin(t, "nord"))
	})
}

func removeThemesDirForTest(t *testing.T, dir string) {
	t.Helper()

	if err := os.RemoveAll(dir); err != nil {
		t.Fatalf("remove themes dir %q: %v", dir, err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("themes dir %q still stats as %v; the no-read assertion would be vacuous", dir, err)
	}
}

func TestCommitSlotLoad_EmitsLoadedOncePerConversion(t *testing.T) {
	dir := newConversionThemesDir(t)
	m, _, sink := newConversionPanelModel(t, dir, theme.RawKeys{Theme: conversionConstant})
	m = openConversionPanel(t, m)

	m, _ = convertToSlot(t, m, "nord", slotDarkPress)
	requireLoadedLine(t, sink, theme.DefaultLightSlug, "light")

	m, _ = pressCommitKey(t, m)
	requireConstantKeys(t, m, "nord")
	m, _ = convertToSlot(t, m, "nord", slotDarkPress)
	requireNominationPair(t, m, testLightTheme(t), themetest.Builtin(t, "nord"))

	loaded := sink.Records().WithMessage("loaded")
	if len(loaded) != 2 {
		t.Fatalf("two conversions emitted %d `theme: loaded` lines, want 2 — the event is NOT deduplicated\n%s", len(loaded), sink.Body())
	}
	for i, record := range loaded {
		if got := record.AttrString(t, "slug"); got != theme.DefaultLightSlug {
			t.Errorf("`theme: loaded` %d carries slug=%q, want %q", i+1, got, theme.DefaultLightSlug)
		}
		if got := record.AttrString(t, "slot"); got != "light" {
			t.Errorf("`theme: loaded` %d carries slot=%q, want %q", i+1, got, "light")
		}
	}
}

func TestCommitSlotLoad_LoadedNamesTheFallbackSlug(t *testing.T) {
	dir := newConversionThemesDir(t)
	keys := theme.RawKeys{Theme: conversionConstant, Light: "nope"}
	m, _, sink := newConversionPanelModel(t, dir, keys)
	m = openConversionPanel(t, m)

	m, _ = convertToSlot(t, m, "nord", slotDarkPress)

	requireLoadedLine(t, sink, theme.DefaultLightSlug, "light")
	applied := sink.Records().WithMessage("fallback applied").Only(t, "fallback applied record")
	if got := applied.AttrString(t, "slug"); got != "nope" {
		t.Errorf("`theme: fallback applied` carries slug=%q, want the NOMINATION that failed (%q)", got, "nope")
	}
	if got := applied.AttrString(t, "slot"); got != "light" {
		t.Errorf("`theme: fallback applied` carries slot=%q, want %q", got, "light")
	}
	if got := applied.AttrString(t, "reason"); got != string(theme.ReasonNotFound) {
		t.Errorf("`theme: fallback applied` carries reason=%q, want %q", got, theme.ReasonNotFound)
	}
	loadedRec := sink.Records().WithMessage("loaded").Only(t, "`theme: loaded` record")
	if loaded := loadedRec.AttrString(t, "slug"); loaded == applied.AttrString(t, "slug") {
		t.Errorf("both lines name %q; the pair is only useful because one names the theme that BROKE and the other the palette that RENDERED", loaded)
	}
	requireSlotCanvas(t, m, theme.SlotLight, testLightTheme(t).Canvas.Value)
}

func TestCommit_NominationTracksThePersistedSetting(t *testing.T) {
	t.Run("Enter over a constant persists the constant it commits", func(t *testing.T) {
		dir := newConversionThemesDir(t)
		m, _, _ := newConversionPanelModel(t, dir, theme.RawKeys{Theme: conversionConstant})
		m = openConversionPanel(t, m)

		m = arrowToThemeRow(t, m, "nord")
		m, _ = pressCommitKey(t, m)

		requireConstantKeys(t, m, "nord")
		requireNominationConstant(t, m, themetest.Builtin(t, "nord"))
	})

	t.Run("a slot commit over a pair persists the slot it writes", func(t *testing.T) {
		dir := newConversionThemesDir(t)
		keys := theme.RawKeys{Light: conversionConstant, Dark: "nord"}
		m, _, _ := newAdaptivePanelModel(t, dir, keys, lightBg)
		m = pressThemeKey(t, m)

		m = arrowToThemeRow(t, m, theme.DefaultDarkSlug)
		m, _ = pressSlotKey(t, m, slotLightPress)

		requireNominationPair(t, m, themetest.Builtin(t, theme.DefaultDarkSlug), themetest.Builtin(t, "nord"))
	})
}

func TestCommitSlotLoad_NonConvertingCommitIsSilent(t *testing.T) {
	adaptive := theme.RawKeys{Light: conversionConstant, Dark: "nord"}

	for _, tc := range []struct {
		name string
		run  func(*testing.T, Model) (Model, tea.Cmd)
		want func(*testing.T, Model, theme.Theme)
	}{
		{
			name: "d over a pair",
			run:  func(t *testing.T, m Model) (Model, tea.Cmd) { return pressSlotKey(t, m, slotDarkPress) },
			want: func(t *testing.T, m Model, previewed theme.Theme) {
				requireNominationPair(t, m, previewed, previewed)
			},
		},
		{
			name: "l over a pair",
			run:  func(t *testing.T, m Model) (Model, tea.Cmd) { return pressSlotKey(t, m, slotLightPress) },
			want: func(t *testing.T, m Model, previewed theme.Theme) {
				requireNominationPair(t, m, previewed, themetest.Builtin(t, "nord"))
			},
		},
		{
			name: "Enter over a pair",
			run:  func(t *testing.T, m Model) (Model, tea.Cmd) { return pressCommitKey(t, m) },
			want: func(t *testing.T, m Model, previewed theme.Theme) {
				requireNominationConstant(t, m, previewed)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := newConversionThemesDir(t)
			m, persister, sink := newAdaptivePanelModel(t, dir, adaptive, lightBg)
			m = pressThemeKey(t, m)
			if !m.themePanel.open {
				t.Fatal("fixture: the panel did not open")
			}
			previewed, mode := m.themeState.active, m.themeState.inForceMode()

			m, _ = tc.run(t, m)

			if len(persister.slugs) != 1 {
				t.Fatalf("fixture: the keypress wrote %v, want exactly one commit so the silence is not vacuous", persister.slugs)
			}
			if got := sink.Records().WithMessage("loaded"); len(got) != 0 {
				t.Errorf("a non-converting commit emitted %d `theme: loaded` line(s), want none\n%s", len(got), sink.Body())
			}
			tc.want(t, m, previewed)
			if m.themeState.inForceMode() != mode {
				t.Errorf("a non-converting commit moved the light/dark answer to %v, want the classified %v — an adaptive launch already has one", m.themeState.inForceMode(), mode)
			}
		})
	}

	t.Run("Enter over a constant", func(t *testing.T) {
		dir := newConversionThemesDir(t)
		m, persister, sink := newConversionPanelModel(t, dir, theme.RawKeys{Theme: conversionConstant})
		m = openConversionPanel(t, m)
		mode := m.themeState.inForceMode()

		m = arrowToThemeRow(t, m, "nord")
		m, _ = pressCommitKey(t, m)

		requireCommitted(t, persister, "nord")
		requireConstantKeys(t, m, "nord")
		if got := sink.Records().WithMessage("loaded"); len(got) != 0 {
			t.Errorf("`Enter` emitted %d `theme: loaded` line(s), want none\n%s", len(got), sink.Body())
		}
		requireNominationConstant(t, m, themetest.Builtin(t, "nord"))
		if m.themeState.inForceMode() != mode {
			t.Errorf("`Enter` moved the light/dark answer to %v, want the untouched %v", m.themeState.inForceMode(), mode)
		}
	})
}

func TestCommitSlotLoad_FailedCommitLoadsNothing(t *testing.T) {
	dir := newConversionThemesDir(t)
	m, persister, sink := newConversionPanelModel(t, dir, theme.RawKeys{Theme: conversionConstant})
	m = openConversionPanel(t, m)
	// A reply under a constant leaves the pinned gate unresolved, so the captured
	// mode stays the dark fallback while the retained reply says light. Without it
	// the answer assertion below cannot fail on a hoisted adoptRetainedReply.
	m = deliverBackgroundReply(t, m, lightBg)
	nomination, mode, keys := m.themeState.nomination, m.themeState.inForceMode(), m.themeState.keys
	persister.err = errThemeCommitFailed

	m, _ = convertToSlot(t, m, "nord", slotDarkPress)

	requireSlotCommits(t, persister, slotCommit{slug: "nord", member: theme.MemberDark})
	if got := sink.Records().WithMessage("loaded"); len(got) != 0 {
		t.Errorf("a failed write emitted %d `theme: loaded` line(s), want none — nothing became live\n%s", len(got), sink.Body())
	}
	if got := sink.Records().WithMessage("fallback applied"); len(got) != 0 {
		t.Errorf("a failed write emitted %d `theme: fallback applied` line(s), want none\n%s", len(got), sink.Body())
	}
	if m.themeState.nomination != nomination {
		t.Error("a failed write moved the nomination; a failed commit leaves the constant standing in memory")
	}
	if !m.themeState.nomination.IsConstant() {
		t.Error("a failed write left an adaptive nomination; the constant is still what is persisted")
	}
	if m.themeState.inForceMode() != mode {
		t.Errorf("a failed write moved the light/dark answer to %v, want the untouched %v", m.themeState.inForceMode(), mode)
	}
	if m.themeState.keys != keys {
		t.Errorf("a failed write left keys %+v, want the untouched %+v", m.themeState.keys, keys)
	}

	landed, _, landedSink := newConversionPanelModel(t, dir, theme.RawKeys{Theme: conversionConstant})
	landed = openConversionPanel(t, landed)
	landed, _ = convertToSlot(t, landed, "nord", slotDarkPress)
	requireLoadedLine(t, landedSink, theme.DefaultLightSlug, "light")
	if landed.themeState.nomination.IsConstant() {
		t.Fatal("positive control: a landed write left the nomination a constant")
	}
}

func TestCommitSlotLoad_ActiveThemeUnchanged(t *testing.T) {
	dir := newConversionThemesDir(t)
	m, _, _ := newConversionPanelModel(t, dir, theme.RawKeys{Theme: conversionConstant})
	m = openConversionPanel(t, m)
	m = arrowToThemeRow(t, m, "nord")
	previewed := m.themeState.active

	m, _ = pressSlotKey(t, m, slotDarkPress)
	m, _ = pressConfirmKey(t, m, confirmYes)

	if m.themeState.active != previewed {
		t.Errorf("the conversion rendered %s, want the previewed %s — a commit is a WRITE, not a navigation", themeLabel(m.themeState.active), themeLabel(previewed))
	}
	requireNominationPair(t, m, testLightTheme(t), previewed)

	frame := m.View().Content
	if seq := canvasSeq(t, previewed); !strings.Contains(frame, seq) {
		t.Errorf("the composed frame no longer paints the previewed canvas %q", seq)
	}
	if seq := canvasSeq(t, testLightTheme(t)); strings.Contains(frame, seq) {
		t.Errorf("the composed frame paints the newly-LOADED slot's canvas %q; the load joins the nomination, never the screen", seq)
	}
}

func TestCommitSlotLoad_SharesTheResolverBody(t *testing.T) {
	dir := newConversionThemesDir(t)
	themetest.Write(t, dir, "ghostly.theme", themetest.MonochromeLines("#202020"))
	loader, _ := themeOpenTestLoader(t)
	enumerator := countingThemeSourceOver(loader, dir)
	enumeration, _ := enumerator.Open(theme.RawKeys{})

	for _, slug := range []string{"nord", "ghostly", conversionConstant, "nope", "../escape"} {
		t.Run(slug, func(t *testing.T) {
			keys := theme.RawKeys{Light: slug, Dark: theme.DefaultDarkSlug}
			one, err := loader.ResolveSlot(enumeration, theme.SlotLight, theme.SlugForSlot(keys, theme.SlotLight))
			if err != nil {
				t.Fatalf("ResolveSlot(%q) returned %v", slug, err)
			}
			whole, err := enumerator.Resolve(enumeration, keys)
			if err != nil {
				t.Fatalf("Resolve(%q) returned %v", slug, err)
			}
			badge := whole.Slots[0]
			if badge.Slot != theme.SlotLight {
				t.Fatalf("the badge path's first slot is %v, want the light one", badge.Slot)
			}
			if one != badge {
				t.Errorf("ResolveSlot returned %+v and the badge path %+v for %q; the two share one rule body", one, badge, slug)
			}
		})
	}
}

func TestCommitSlotLoad_DiscardSilencesLoaded(t *testing.T) {
	control := func(t *testing.T) {
		t.Helper()
		dir := newConversionThemesDir(t)
		m, _, sink := newConversionPanelModel(t, dir, theme.RawKeys{Theme: conversionConstant})
		m = openConversionPanel(t, m)
		m, _ = convertToSlot(t, m, "nord", slotDarkPress)
		requireLoadedLine(t, sink, theme.DefaultLightSlug, "light")
		if m.themeState.nomination.IsConstant() {
			t.Fatal("control: the conversion did not complete the pair")
		}
	}
	control(t)

	sink := logtest.Install(t)
	dir := newConversionThemesDir(t)
	m, _ := newLoadPanelModel(t, dir, theme.RawKeys{Theme: conversionConstant}, theme.NewSilentLoader())
	m = openConversionPanel(t, m)

	m, _ = convertToSlot(t, m, "nord", slotDarkPress)

	if m.themeState.nomination.IsConstant() {
		t.Fatal("the discard-backed conversion did not complete the pair, so the silence is vacuous")
	}
	if lines := sink.Lines(); len(lines) != 0 {
		t.Errorf("a discard-backed loader emitted %d record(s) on a conversion, want none:\n%s", len(lines), sink.Body())
	}
}

func TestCommitSlotLoad_ConversionUsesTheRetainedAnswer(t *testing.T) {
	for _, tc := range []struct {
		name    string
		reply   tea.BackgroundColorMsg
		want    theme.Member
		inForce string
	}{
		{name: "a light terminal", reply: lightBg, want: theme.MemberLight, inForce: theme.DefaultLightSlug},
		{name: "a dark terminal", reply: darkBg, want: theme.MemberDark, inForce: "nord"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := newConversionThemesDir(t)
			m, _, _ := newConversionPanelModel(t, dir, theme.RawKeys{Theme: conversionConstant})
			m = deliverBackgroundReply(t, m, tc.reply)
			if m.themeState.inForceMode() != theme.MemberDark {
				t.Fatalf("fixture: the constant launch starts on %v, want the standing dark fallback so the answer is the conversion's", m.themeState.inForceMode())
			}
			m = openConversionPanel(t, m)

			m, _ = convertToSlot(t, m, "nord", slotDarkPress)

			if m.themeState.inForceMode() != tc.want {
				t.Errorf("the conversion left the light/dark answer %v, want %v — it classifies the reply already in hand", m.themeState.inForceMode(), tc.want)
			}

			closed := closeThemePanelForTest(t, m)
			want := themetest.Builtin(t, tc.inForce)
			if closed.themeState.active != want {
				t.Errorf("the close landed on %s, want %s — the in-force member is the one the answer names", themeLabel(closed.themeState.active), themeLabel(want))
			}
		})
	}

	t.Run("a reply that lands with the panel already open", func(t *testing.T) {
		dir := newConversionThemesDir(t)
		m, _, _ := newConversionPanelModel(t, dir, theme.RawKeys{Theme: conversionConstant})
		m = openConversionPanel(t, m)
		if m.themeState.reply.arrived {
			t.Fatal("fixture: a reply arrived before the open, so the late-arrival order is not being driven")
		}

		m = deliverBackgroundReply(t, m, lightBg)

		if !m.themeState.reply.arrived {
			t.Fatal("the reply was not retained with the panel open; the arm must sit ahead of every panel route")
		}
		if m.themeState.inForceMode() != theme.MemberDark {
			t.Fatalf("fixture: the reply moved the answer to %v before any conversion, want the constant's standing dark fallback", m.themeState.inForceMode())
		}

		m, _ = convertToSlot(t, m, "nord", slotDarkPress)

		if m.themeState.inForceMode() != theme.MemberLight {
			t.Errorf("the conversion left the light/dark answer %v, want light — a reply retained with the panel open classifies exactly as an early one", m.themeState.inForceMode())
		}
		closed := closeThemePanelForTest(t, m)
		if want := testLightTheme(t); closed.themeState.active != want {
			t.Errorf("the close landed on %s, want %s", themeLabel(closed.themeState.active), themeLabel(want))
		}
	})
}

func TestCommitSlotLoad_ConversionIssuesNoQuery(t *testing.T) {
	dir := newConversionThemesDir(t)
	m, _, _ := newConversionPanelModel(t, dir, theme.RawKeys{Theme: conversionConstant})
	m = deliverBackgroundReply(t, m, lightBg)
	m = openConversionPanel(t, m)
	gate, original, arrived := m.themeState.gate, m.originalBg, m.themeState.reply.arrived

	m, cmd := convertToSlot(t, m, "nord", slotDarkPress)

	if cmd != nil {
		t.Errorf("the conversion scheduled %T; it needs no new query and arms no new gate", cmd)
	}
	if m.themeState.gate != gate {
		t.Errorf("the conversion moved the gate to %+v, want the untouched %+v — the appearance gate resolves exactly once", m.themeState.gate, gate)
	}
	if m.originalBg != original || m.themeState.reply.arrived != arrived {
		t.Errorf("the conversion moved the retained reply to (%q, %v), want the untouched (%q, %v)", m.originalBg, m.themeState.reply.arrived, original, arrived)
	}
	if m.themeState.inForceMode() != theme.MemberLight {
		t.Fatalf("the conversion answered %v, want light", m.themeState.inForceMode())
	}
	if m.themeState.gate.appearance == m.themeState.inForceMode() {
		t.Error("the gate's appearance now equals the answer, so this fixture cannot tell a classified reply from the gate's standing dark fallback")
	}
}

func TestCommitSlotLoad_ConversionWithNoReplyIsDark(t *testing.T) {
	dir := newConversionThemesDir(t)
	m, _, _ := newConversionPanelModel(t, dir, theme.RawKeys{Theme: conversionConstant})
	if m.themeState.reply.arrived {
		t.Fatal("fixture: a reply has already arrived, so the no-reply case is not being driven")
	}
	m = openConversionPanel(t, m)

	m, _ = convertToSlot(t, m, "nord", slotDarkPress)

	if m.themeState.inForceMode() != theme.MemberDark {
		t.Errorf("a conversion with no reply answered %v, want dark (the no-answer fallback)", m.themeState.inForceMode())
	}
	active, nomination := m.themeState.active, m.themeState.nomination

	late := deliverBackgroundReply(t, m, lightBg)

	if late.themeState.inForceMode() != theme.MemberDark {
		t.Errorf("a late reply moved the answer to %v; the gate resolves exactly once", late.themeState.inForceMode())
	}
	if late.themeState.active != active {
		t.Errorf("a late reply re-themed to %s, want the untouched %s", themeLabel(late.themeState.active), themeLabel(active))
	}
	if late.themeState.nomination != nomination {
		t.Error("a late reply moved the nomination")
	}
}

func TestCommitSlotLoad_ConversionDoesNotMoveStartupCanvasHex(t *testing.T) {
	for _, tc := range []struct {
		name    string
		reply   tea.BackgroundColorMsg
		deliver bool
	}{
		{name: "a light terminal", reply: lightBg, deliver: true},
		{name: "a dark terminal", reply: darkBg, deliver: true},
		{name: "no reply at all", deliver: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := newConversionThemesDir(t)
			m, _, _ := newConversionPanelModel(t, dir, theme.RawKeys{Theme: conversionConstant})
			if tc.deliver {
				m = deliverBackgroundReply(t, m, tc.reply)
			}
			anchor := m.themeState.startupCanvasHex
			if anchor != conversionConstantValue {
				t.Fatalf("fixture: the startup canvas hex is %q, want the constant's %q", anchor, conversionConstantValue)
			}
			m = openConversionPanel(t, m)

			m, _ = convertToSlot(t, m, "nord", slotDarkPress)

			if m.themeState.startupCanvasHex != anchor {
				t.Errorf("the conversion moved the startup canvas hex to %q, want the byte-identical %q — the startup-canvas anchor is frozen at gate resolution", m.themeState.startupCanvasHex, anchor)
			}
			if m.themeState.startupCanvasHex == m.themeState.active.Canvas.Value {
				t.Error("the anchor now equals the PREVIEWED canvas, so this fixture cannot detect a re-capture from the active theme")
			}
			if m.themeState.startupCanvasHex == testLightTheme(t).Canvas.Value {
				t.Error("the anchor now equals the newly-LOADED slot's canvas, so this fixture cannot detect a re-capture from the load")
			}
		})
	}

	t.Run("the classification does not route through syncResolvedMode", func(t *testing.T) {
		callers := themePanelSeamCallers(t, "syncResolvedMode")
		if !slices.Contains(callers, "New") {
			t.Fatalf("syncResolvedMode is called from %v; the scan found no known caller, so its absence proves nothing", callers)
		}
		if want := []string{"New", "Update", "armAppearanceDetection"}; !slices.Equal(callers, want) {
			t.Errorf("syncResolvedMode is called from %v, want exactly %v — the startup-canvas anchor is captured there, so the conversion records its answer directly", callers, want)
		}
	})
}

func TestCommitSlotLoad_RestoreStaysAnchoredAfterACommit(t *testing.T) {
	dir := newConversionThemesDir(t)
	m, _, _ := newConversionPanelModel(t, dir, theme.RawKeys{Theme: conversionConstant})
	m = deliverBackgroundReply(t, m, lightBg)
	m = openConversionPanel(t, m)

	m, _ = convertToSlot(t, m, nordSlug, slotDarkPress)

	if got := m.themeState.startupCanvasHex; got != conversionConstantValue {
		t.Fatalf("startupCanvasHex = %q after the commit, want the constant's %q", got, conversionConstantValue)
	}
	if got := m.themeState.active.Canvas.Value; got != nordCanvas {
		t.Fatalf("active canvas = %q after the commit, want the previewed %q — without the divergence the two originals below assert the same thing", got, nordCanvas)
	}

	t.Run("an echo of the startup canvas is skipped", func(t *testing.T) {
		assertSkipped(t, withCapturedOriginal(m, conversionConstantValue))
	})

	t.Run("the active theme's canvas is a genuine original and is set back", func(t *testing.T) {
		assertSetBack(t, withCapturedOriginal(m, nordCanvas))
	})
}

func TestCommitSlotLoad_AnswerIsIndependentOfTheLoad(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "the slot load lands"},
		{name: "the slot load returns the fatal", err: errThemeResolveFatal},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows := arrowValidRows(t, 4)
			persisted, target := rows[0].Slug, rows[2].Slug
			deps := newArrowPanelDeps(t, rows, persisted)
			deps.ThemePersister = &fakeThemePersister{}
			m := openCommitPanel(t, deps, PageSessions, persisted)
			seam, ok := m.themeState.source.(*fakeThemeSource)
			if !ok {
				t.Fatalf("fixture: the seam is %T, want the recording fake", m.themeState.source)
			}
			m = deliverBackgroundReply(t, m, lightBg)
			if m.themeState.inForceMode() != theme.MemberDark {
				t.Fatalf("fixture: the constant launch answers %v before the conversion, want the standing dark fallback", m.themeState.inForceMode())
			}
			m = arrowToThemeRow(t, m, target)
			seam.err = tc.err

			m, _ = pressSlotKey(t, m, slotDarkPress)
			m, _ = pressConfirmKey(t, m, confirmYes)

			if len(seam.slotLoads) != 1 {
				t.Fatalf("the conversion asked for %d slot load(s), want 1", len(seam.slotLoads))
			}
			if m.themeState.inForceMode() != theme.MemberLight {
				t.Errorf("the conversion left the light/dark answer %v, want light — the terminal's classification does not depend on the load", m.themeState.inForceMode())
			}
		})
	}
}

func TestCommitSlotLoad_BrokenBuiltinDegrades(t *testing.T) {
	rows := arrowValidRows(t, 4)
	persisted, target := rows[0].Slug, rows[2].Slug
	persister := &fakeThemePersister{}
	deps := newArrowPanelDeps(t, rows, persisted)
	deps.ThemePersister = persister
	m := openCommitPanel(t, deps, PageSessions, persisted)
	seam, ok := m.themeState.source.(*fakeThemeSource)
	if !ok {
		t.Fatalf("fixture: the seam is %T, want the recording fake", m.themeState.source)
	}
	m = arrowToThemeRow(t, m, target)
	nomination, mode, active := m.themeState.nomination, m.themeState.inForceMode(), m.themeState.active
	seam.err = errThemeResolveFatal

	m, _ = pressSlotKey(t, m, slotDarkPress)
	m, cmd := pressConfirmKey(t, m, confirmYes)

	if len(seam.slotLoads) != 1 {
		t.Fatalf("the conversion asked for %d slot load(s), want 1 — the degrade must be the seam's answer rather than a load that never ran", len(seam.slotLoads))
	}
	if m.themeState.nomination != nomination {
		t.Errorf("the fatal left the nomination %+v, want the untouched %+v — a degrade moves nothing", m.themeState.nomination, nomination)
	}
	if m.themeState.inForceMode() != mode {
		t.Errorf("the fatal left the light/dark answer %v, want the no-reply dark fallback %v — the answer is recorded from the retained reply before the load runs, and this fixture receives none", m.themeState.inForceMode(), mode)
	}
	if m.themeState.active != active {
		t.Errorf("the fatal rendered %s, want the untouched %s", themeLabel(m.themeState.active), themeLabel(active))
	}
	if cmd != nil {
		t.Errorf("the fatal scheduled %T; the panel degrades rather than quitting Portal mid-session", cmd)
	}
	if !m.themePanel.open {
		t.Error("the fatal closed the panel; `Esc` is the only way out")
	}
}
