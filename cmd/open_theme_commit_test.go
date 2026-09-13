package cmd

import (
	"image/color"
	"maps"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/themetest"
	"github.com/leeovery/portal/internal/tui"
)

// Every observation is through the rendered frame or the file: internal/tui
// holds the panel's cursor unexported, so the fixtures locate it the way the
// user does, by the canvas the previewed row paints.

const (
	// Drop-ins rather than built-ins: a built-in resolves out of the embedded set
	// without the themes directory being read at all.
	roundTripStandingSlug = "aurora"
	roundTripChosenSlug   = "sunset"

	// The canvas is the one token saying which file was parsed, and the cursor
	// tracking reads it off the frame — so these must differ from each other
	// and from every built-in's.
	roundTripStandingCanvas = "#101010"
	roundTripChosenCanvas   = "#202020"

	roundTripDropIns = 2

	roundTripTermWidth  = 100
	roundTripTermHeight = 28

	// A literal because internal/tui keeps its own copy unexported; should the
	// two disagree the parse below finds no panel rows at all and says so.
	roundTripPanelBorder = "│"
)

var (
	themePanelOpenKey       = tea.KeyPressMsg{Code: 't', Text: "t"}
	themePanelConstantKey   = tea.KeyPressMsg{Code: tea.KeyEnter}
	themePanelDarkSlotKey   = tea.KeyPressMsg{Code: 'd', Text: "d"}
	themePanelCursorUpKey   = tea.KeyPressMsg{Code: tea.KeyUp}
	themePanelCursorDownKey = tea.KeyPressMsg{Code: tea.KeyDown}
	themePanelConfirmKey    = tea.KeyPressMsg{Code: 'y', Text: "y"}

	// Restated because internal/tui keeps these strings unexported.
	roundTripBadgeCopy = map[theme.Badge]string{
		theme.BadgeConstant: "●",
		theme.BadgeLight:    "● light",
		theme.BadgeDark:     "● dark",
		theme.BadgeBoth:     "● both",
	}
)

func seedRoundTripThemes(t *testing.T) {
	t.Helper()

	dir := useThemesDir(t)
	writeThemeFile(t, dir, roundTripStandingSlug, roundTripStandingCanvas)
	writeThemeFile(t, dir, roundTripChosenSlug, roundTripChosenCanvas)
}

// The persister is wired only where the store actually loaded: a typed-nil
// *prefs.Store wrapped in a persister boxes into a non-nil seam whose every
// write panics, so the slot is left empty instead.
func themeRoundTripConfig(t *testing.T) tuiConfig {
	t.Helper()

	logtest.Install(t)

	load, err := loadPrefsStore()
	if err != nil {
		t.Fatalf("load the prefs store: %v", err)
	}
	loader := newThemeLoader()
	resolution, keys, err := themeResolution(load.Keys, loader)
	if err != nil {
		t.Fatalf("resolve the persisted theme setting: %v", err)
	}

	cfg := defaultTestTUIConfig()
	cfg.theme = resolution.Nomination
	cfg.themeKeys = keys
	cfg.themeSource = newThemeSource(loader)
	if load.Store != nil {
		cfg.themePersister = newThemePersister(load.Store)
	}
	return cfg
}

// An adaptive nomination paints nothing until the gate resolves, so the reply is
// delivered before anything is read off the frame; under a constant the gate is
// skipped and the reply is inert.
func startRoundTripPicker(t *testing.T, cfg tuiConfig) tui.Model {
	t.Helper()

	m := buildTUIModel(cfg, pickerLanding{}, nil)
	m = update(t, m, tea.WindowSizeMsg{Width: roundTripTermWidth, Height: roundTripTermHeight})
	return update(t, m, darkBackgroundReply)
}

func openRoundTripPanel(t *testing.T, m tui.Model) tui.Model {
	t.Helper()

	m = update(t, m, themePanelOpenKey)
	if view := m.View().Content; !strings.Contains(view, themePanelHeaderCopy) {
		t.Fatalf("the frame after `t` carries no %q header, so the commit keys would land on no panel:\n%s", themePanelHeaderCopy, view)
	}
	return m
}

// The painted canvas is the cursor's position as seen from outside
// internal/tui. It parks at the top first: `↓` clamps rather than wrapping, so
// a walk from wherever the panel opened could only reach rows below it.
func arrowToPreviewedCanvas(t *testing.T, m tui.Model, canvas string) tui.Model {
	t.Helper()

	want := canvasColour(canvas)
	rows := len(theme.BuiltinSlugs()) + roundTripDropIns
	for range rows {
		m = update(t, m, themePanelCursorUpKey)
	}
	for range rows {
		if m.View().BackgroundColor == want {
			return m
		}
		m = update(t, m, themePanelCursorDownKey)
	}
	t.Fatalf("arrowing never previewed the theme whose canvas is %s, so the commit below would be about a row the cursor never reached", canvas)
	return m
}

func canvasColour(canvas string) color.Color {
	return theme.Token{Value: canvas}.Color()
}

// The badges are a projection of the keys, not the keys: a stale slot beside a
// constant moves no badge, so a constant commit is bound by the commit that
// follows it rather than by itself.
func assertBadgesMatchPersistedKeys(t *testing.T, m tui.Model) {
	t.Helper()

	want := badgesForPersistedKeys(t)
	got := panelBadgesOnFrame(t, m)
	if !maps.Equal(got, want) {
		t.Errorf("the panel marks %v, want %v — the badges and prefs.json disagree about what is set\n%s",
			got, want, ansi.Strip(m.View().Content))
	}
}

func badgesForPersistedKeys(t *testing.T) map[string]string {
	t.Helper()

	store, err := loadPrefsStoreNoMigrate()
	if err != nil {
		t.Fatalf("resolve the prefs store: %v", err)
	}
	keys, err := store.LoadThemeKeys()
	if err != nil {
		t.Fatalf("read the theme keys back off disk: %v", err)
	}
	resolution, _, err := themeResolution(keys, newThemeLoader())
	if err != nil {
		t.Fatalf("resolve the persisted theme setting %+v: %v", keys, err)
	}

	badges := make(map[string]string)
	for slug, badge := range theme.Badges(resolution.Slots) {
		marker, ok := roundTripBadgeCopy[badge]
		if !ok {
			t.Fatalf("prefs.json's keys derive badge %d for %q, which the panel has no copy for", badge, slug)
		}
		badges[slug] = marker
	}
	return badges
}

// Only the union's own slugs are looked up: the footer rows read `d set as
// dark` and `l set as light`, which a scan for the badge vocabulary would match.
// Each slug must be present, so a badged row paginated off the frame fails
// rather than reading as an absent marker.
func panelBadgesOnFrame(t *testing.T, m tui.Model) map[string]string {
	t.Helper()

	frame := ansi.Strip(m.View().Content)
	trailing := panelRowTrailing(t, frame)

	badges := make(map[string]string)
	for _, slug := range roundTripUnionSlugs() {
		row, listed := trailing[slug]
		if !listed {
			t.Fatalf("the frame carries no %q row, so a badge on it could not be seen:\n%s", slug, frame)
		}
		if row != "" {
			badges[slug] = row
		}
	}
	return badges
}

// A list row composes as `[cursor column][label][pad][badge]`, leaving the
// label as the key and the badge as the value.
func panelRowTrailing(t *testing.T, frame string) map[string]string {
	t.Helper()

	trailing := make(map[string]string)
	for line := range strings.SplitSeq(frame, "\n") {
		_, panel, onPanel := strings.Cut(line, roundTripPanelBorder)
		if !onPanel {
			continue
		}
		fields := strings.Fields(panel)
		if len(fields) > 0 && fields[0] == "▌" {
			fields = fields[1:]
		}
		if len(fields) == 0 {
			continue
		}
		trailing[fields[0]] = strings.Join(fields[1:], " ")
	}
	if len(trailing) == 0 {
		t.Fatalf("no panel rows parsed out of the frame; the slide-over did not render:\n%s", frame)
	}
	return trailing
}

func roundTripUnionSlugs() []string {
	return append(theme.BuiltinSlugs(), roundTripStandingSlug, roundTripChosenSlug)
}

// The cursor is arrowed away from the row the panel opened on, so "the cursor's
// slug" and "the persisted slug" are distinguishable strings.
func TestThemePanelCommit_EnterRoundTripsAConstantToPrefs(t *testing.T) {
	seedRoundTripThemes(t)
	path := setPrefsFile(t, `{"session_list_mode":"by-tag","theme_dark":"`+roundTripStandingSlug+`"}`)

	m := startRoundTripPicker(t, themeRoundTripConfig(t))
	m = openRoundTripPanel(t, m)
	assertPaintedCanvas(t, m, canvasColour(roundTripStandingCanvas))
	m = arrowToPreviewedCanvas(t, m, roundTripChosenCanvas)

	m = update(t, m, themePanelConstantKey)

	assertPrefsOnDisk(t, path, prefsOnDisk{SessionListMode: "by-tag", Theme: roundTripChosenSlug})
	assertBadgesMatchPersistedKeys(t, m)

	nomination := themeNominationForTest(t)
	if !nomination.IsConstant() {
		t.Fatalf("the relaunch resolved an adaptive pair; want the constant `Enter` committed")
	}
	assertCanvasValue(t, nomination.Constant(), roundTripChosenCanvas)
}

func TestThemePanelCommit_DarkKeyOverAConstantRoundTripsThePairToPrefs(t *testing.T) {
	seedRoundTripThemes(t)
	path := setPrefsFile(t, `{"theme":"`+roundTripStandingSlug+`"}`)

	m := startRoundTripPicker(t, themeRoundTripConfig(t))
	m = openRoundTripPanel(t, m)
	assertPaintedCanvas(t, m, canvasColour(roundTripStandingCanvas))
	m = arrowToPreviewedCanvas(t, m, roundTripChosenCanvas)

	// `d` alone only raises the question: a keypress that wrote here would make
	// the confirm decorative.
	m = update(t, m, themePanelDarkSlotKey)
	assertPrefsOnDisk(t, path, prefsOnDisk{Theme: roundTripStandingSlug})
	m = update(t, m, themePanelConfirmKey)

	assertPrefsOnDisk(t, path, prefsOnDisk{ThemeDark: roundTripChosenSlug})
	assertBadgesMatchPersistedKeys(t, m)

	nomination := themeNominationForTest(t)
	if nomination.IsConstant() {
		t.Fatalf("the relaunch resolved a constant; want the pair `d` converted it into")
	}
	assertCanvasValue(t, nomination.Select(theme.MemberDark), roundTripChosenCanvas)
	assertCanvasValue(t, nomination.Select(theme.MemberLight), themetest.Builtin(t, theme.DefaultLightSlug).Canvas.Value)
}

// A constant's clear is not visible on the frame it happens on — under a
// constant the slots are unread — so the sequence is a constant over a pair and
// then a slot over that constant, asserted after each.
func TestThemePanelCommit_ConsecutiveCommitsStayBoundToPrefs(t *testing.T) {
	seedRoundTripThemes(t)
	path := setPrefsFile(t, `{"theme_light":"`+roundTripStandingSlug+`","theme_dark":"`+nordSlug+`"}`)

	m := startRoundTripPicker(t, themeRoundTripConfig(t))
	m = openRoundTripPanel(t, m)
	assertPaintedCanvas(t, m, themetest.Builtin(t, nordSlug).Canvas.Color())
	m = arrowToPreviewedCanvas(t, m, roundTripChosenCanvas)

	m = update(t, m, themePanelConstantKey)
	assertPrefsOnDisk(t, path, prefsOnDisk{Theme: roundTripChosenSlug})
	assertBadgesMatchPersistedKeys(t, m)

	m = update(t, m, themePanelDarkSlotKey)
	m = update(t, m, themePanelConfirmKey)

	assertPrefsOnDisk(t, path, prefsOnDisk{ThemeDark: roundTripChosenSlug})
	assertBadgesMatchPersistedKeys(t, m)
}

// The setting is already adaptive, so the keypress commits outright.
func TestThemePanelCommit_DarkKeyRoundTripsOneSlotToPrefs(t *testing.T) {
	seedRoundTripThemes(t)
	path := setPrefsFile(t, `{"theme_light":"`+roundTripStandingSlug+`","theme_dark":"`+nordSlug+`"}`)

	m := startRoundTripPicker(t, themeRoundTripConfig(t))
	m = openRoundTripPanel(t, m)
	// A dark terminal renders the pair's dark member, so the panel opens on the
	// built-in the dark slot names rather than on either drop-in.
	assertPaintedCanvas(t, m, themetest.Builtin(t, nordSlug).Canvas.Color())
	m = arrowToPreviewedCanvas(t, m, roundTripChosenCanvas)

	m = update(t, m, themePanelDarkSlotKey)

	assertPrefsOnDisk(t, path, prefsOnDisk{ThemeLight: roundTripStandingSlug, ThemeDark: roundTripChosenSlug})
	assertBadgesMatchPersistedKeys(t, m)

	nomination := themeNominationForTest(t)
	if nomination.IsConstant() {
		t.Fatalf("the relaunch resolved a constant; want the pair `d` completed")
	}
	assertCanvasValue(t, nomination.Select(theme.MemberLight), roundTripStandingCanvas)
	assertCanvasValue(t, nomination.Select(theme.MemberDark), roundTripChosenCanvas)
}

func TestThemePanelCommit_NoPrefsStoreWritesNothing(t *testing.T) {
	seedRoundTripThemes(t)
	const seeded = `{"session_list_mode":"by-tag","theme_dark":"` + roundTripStandingSlug + `"}`
	path := setPrefsFile(t, seeded)

	cfg := themeRoundTripConfig(t)
	cfg.themePersister = nil

	m := startRoundTripPicker(t, cfg)
	m = openRoundTripPanel(t, m)
	assertPaintedCanvas(t, m, canvasColour(roundTripStandingCanvas))
	m = arrowToPreviewedCanvas(t, m, roundTripChosenCanvas)

	m = update(t, m, themePanelConstantKey)
	m = update(t, m, themePanelDarkSlotKey)

	assertPrefsUnchanged(t, path, []byte(seeded))
	if view := m.View().Content; !strings.Contains(view, themePanelHeaderCopy) {
		t.Errorf("a commit over the unwired seam closed the panel:\n%s", view)
	}
}
