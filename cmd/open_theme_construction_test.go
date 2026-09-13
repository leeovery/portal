package cmd

import (
	"fmt"
	"image/color"
	"os"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/commandertest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/themetest"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui"
)

// Deterministic OSC 11 answers: near-black and near-white, so the gate's
// classification is unambiguous either way.
var (
	darkBackgroundReply  = tea.BackgroundColorMsg{Color: color.RGBA{R: 0x0b, G: 0x0c, B: 0x14, A: 0xff}}
	lightBackgroundReply = tea.BackgroundColorMsg{Color: color.RGBA{R: 0xe1, G: 0xe2, B: 0xe7, A: 0xff}}
	otherBackgroundReply = tea.BackgroundColorMsg{Color: color.RGBA{R: 0x12, G: 0x34, B: 0x56, A: 0xff}}
)

// A built-in that is neither shipped default: asserting against a default would
// pass identically if the nomination were ignored and the fallback rendered.
const nordSlug = "nord"

func themeNominationForTest(t *testing.T) theme.Nomination {
	t.Helper()

	load, err := loadPrefsStore()
	if err != nil {
		t.Fatalf("load prefs store: %v", err)
	}
	resolution, _, err := themeResolution(load.Keys, newThemeLoader())
	if err != nil {
		t.Fatalf("resolve the persisted theme setting: %v", err)
	}
	return resolution.Nomination
}

func modelForNomination(n theme.Nomination) tui.Model {
	cfg := defaultTestTUIConfig()
	cfg.theme = n
	return buildTUIModel(cfg, pickerLanding{}, nil)
}

func assertPaintedCanvas(t *testing.T, m tui.Model, want color.Color) {
	t.Helper()
	if got := m.View().BackgroundColor; got != want {
		t.Errorf("painted canvas = %v, want %v", got, want)
	}
}

func TestConstruction_PersistedConstantSkipsTheGate(t *testing.T) {
	setPrefsFile(t, `{"theme":"`+nordSlug+`"}`)
	nord := themetest.Builtin(t, nordSlug)

	nomination := themeNominationForTest(t)
	assertConstant(t, nomination, nord)

	m := modelForNomination(nomination)
	assertPaintedCanvas(t, m, nord.Canvas.Color())

	after, _ := m.Update(lightBackgroundReply)
	assertPaintedCanvas(t, after.(tui.Model), nord.Canvas.Color())
}

// `theme_dark` alone is deliberate: an unset `theme_light` is not a partial pair
// but a slot holding the shipped default, which resolves to a real palette.
func TestConstruction_PersistedPairSelectsByGate(t *testing.T) {
	setPrefsFile(t, `{"theme_dark":"`+nordSlug+`"}`)
	nord := themetest.Builtin(t, nordSlug)
	day := themetest.Builtin(t, theme.DefaultLightSlug)

	nomination := themeNominationForTest(t)
	assertPair(t, nomination, day, nord)

	// No member is active until the gate resolves — never paint-then-flip.
	assertPaintedCanvas(t, modelForNomination(nomination), nil)

	for _, tc := range []struct {
		name    string
		resolve func(*testing.T, tui.Model) tui.Model
		want    theme.Theme
	}{
		{
			name:    "a dark reply selects the nominated dark member",
			resolve: func(t *testing.T, m tui.Model) tui.Model { return update(t, m, darkBackgroundReply) },
			want:    nord,
		},
		{
			name:    "a light reply selects the shipped light default",
			resolve: func(t *testing.T, m tui.Model) tui.Model { return update(t, m, lightBackgroundReply) },
			want:    day,
		},
		{
			name:    "no answer at all selects the dark member",
			resolve: resolveByTimeout,
			want:    nord,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resolved := tc.resolve(t, modelForNomination(nomination))

			assertPaintedCanvas(t, resolved, tc.want.Canvas.Color())
		})
	}
}

// The reply is still consumed — restore.go needs the original background for
// the exit-time set-back — but the selection must not flip with it.
func TestConstruction_LateReplyNeverReThemes(t *testing.T) {
	setPrefsFile(t, `{"theme_dark":"`+nordSlug+`"}`)
	nord := themetest.Builtin(t, nordSlug)

	resolved := resolveByTimeout(t, modelForNomination(themeNominationForTest(t)))
	assertPaintedCanvas(t, resolved, nord.Canvas.Color())

	after := update(t, resolved, lightBackgroundReply)

	assertPaintedCanvas(t, after, nord.Canvas.Color())
	if got, want := after.OriginalBackground(), "#e1e2e7"; got != want {
		t.Errorf("OriginalBackground() = %q after a late reply, want %q — the reply is still consumed for restore-on-exit", got, want)
	}
}

// Both slot values are unusable, so a read of either would raise a
// `theme: fallback applied` WARN — one `loaded` line and no warning is what
// "the slots are never read" looks like from the log.
func TestConstruction_ConstantWinsOverStaleSlots(t *testing.T) {
	setPrefsFile(t, `{"theme":"`+nordSlug+`","theme_light":"../evil","theme_dark":"no-such-theme"}`)
	nord := themetest.Builtin(t, nordSlug)
	sink := logtest.Install(t)

	nomination := themeNominationForTest(t)

	assertConstant(t, nomination, nord)
	assertPaintedCanvas(t, modelForNomination(nomination), nord.Canvas.Color())
	assertThemeEvents(t, sink, "INFO loaded slug="+nordSlug)
}

// Persisting the fallback would turn a transient failure into a destructive one:
// fixing the theme file would no longer restore the theme, the name having
// already been overwritten.
func TestConstruction_UnloadableNominationFallsBackWithoutWriting(t *testing.T) {
	const content = `{"session_list_mode":"by-tag","theme":"no-such-theme"}`
	setPrefsFile(t, content)
	sink := logtest.Install(t)

	nomination := themeNominationForTest(t)

	assertConstant(t, nomination, themetest.Builtin(t, theme.DefaultDarkSlug))
	assertThemeEvents(t, sink,
		"WARN fallback applied slug=no-such-theme reason=not found",
		"INFO loaded slug="+theme.DefaultDarkSlug,
	)

	after, err := os.ReadFile(os.Getenv("PORTAL_PREFS_FILE"))
	if err != nil {
		t.Fatalf("read prefs file back: %v", err)
	}
	if string(after) != content {
		t.Errorf("prefs.json changed:\n got %s\nwant %s\n(falling back never overwrites the persisted name)", after, content)
	}
}

func TestConstruction_EmitsLoadedPerNomination(t *testing.T) {
	t.Run("a constant emits one line carrying no slot", func(t *testing.T) {
		setPrefsFile(t, `{"theme":"`+nordSlug+`"}`)
		sink := logtest.Install(t)

		themeNominationForTest(t)

		assertThemeEvents(t, sink, "INFO loaded slug="+nordSlug)
	})

	t.Run("a pair emits one line per slot, light then dark", func(t *testing.T) {
		setPrefsFile(t, `{"theme_dark":"`+nordSlug+`"}`)
		sink := logtest.Install(t)

		themeNominationForTest(t)

		assertThemeEvents(t, sink,
			"INFO loaded slug="+theme.DefaultLightSlug+" slot=light",
			"INFO loaded slug="+nordSlug+" slot=dark",
		)
	})
}

// The directory's `nord.theme` carries a different canvas from the embedded
// Nord's, so which palette loaded says which file was read; the mode-0111
// directory makes any ReadDir fail loudly.
func TestConstruction_ReadBudget(t *testing.T) {
	const (
		shadowCanvas = "#111213"
		aloneCanvas  = "#141516"
		lightCanvas  = "#171819"
		darkCanvas   = "#1A1B1C"
	)
	seedUnlistableThemesDir(t, map[string]string{
		nordSlug:     shadowCanvas,
		"drop-alone": aloneCanvas,
		"drop-light": lightCanvas,
		"drop-dark":  darkCanvas,
	})

	t.Run("a built-in constant reads the directory zero times", func(t *testing.T) {
		setPrefsFile(t, `{"theme":"`+nordSlug+`"}`)
		sink := logtest.Install(t)

		nomination := themeNominationForTest(t)

		assertConstant(t, nomination, themetest.Builtin(t, nordSlug))
		assertThemeEvents(t, sink, "INFO loaded slug="+nordSlug)
	})

	t.Run("a drop-in constant reads exactly its own file", func(t *testing.T) {
		setPrefsFile(t, `{"theme":"drop-alone"}`)
		sink := logtest.Install(t)

		nomination := themeNominationForTest(t)

		assertCanvasValue(t, nomination.Constant(), aloneCanvas)
		assertThemeEvents(t, sink, "INFO loaded slug=drop-alone")
	})

	t.Run("a drop-in pair reads exactly two files", func(t *testing.T) {
		setPrefsFile(t, `{"theme_light":"drop-light","theme_dark":"drop-dark"}`)
		sink := logtest.Install(t)

		nomination := themeNominationForTest(t)

		assertCanvasValue(t, nomination.Select(theme.MemberLight), lightCanvas)
		assertCanvasValue(t, nomination.Select(theme.MemberDark), darkCanvas)
		assertThemeEvents(t, sink,
			"INFO loaded slug=drop-light slot=light",
			"INFO loaded slug=drop-dark slot=dark",
		)
	})
}

func TestConstruction_PathFailuresDegradeNotBlock(t *testing.T) {
	shippedLight := themetest.Builtin(t, theme.DefaultLightSlug)
	shippedDark := themetest.Builtin(t, theme.DefaultDarkSlug)

	t.Run("a prefs store that could not be built opens on the shipped pair", func(t *testing.T) {
		// Zero keys are openTUI's own degradation: a prefs path-resolution
		// failure leaves a zero prefsLoad — no store, no keys.
		resolution, _, err := themeResolution(prefs.ThemeKeys{}, newThemeLoader())
		if err != nil {
			t.Fatalf("a prefs load that failed must not fail construction: %v", err)
		}

		assertPair(t, resolution.Nomination, shippedLight, shippedDark)
	})

	t.Run("a themes directory that cannot be resolved opens on the shipped pair", func(t *testing.T) {
		setPrefsFile(t, "")
		unresolvableThemesDir(t)

		nomination := themeNominationForTest(t)

		assertPair(t, nomination, shippedLight, shippedDark)
	})

	t.Run("a drop-in nomination falls back when there is no directory to look in", func(t *testing.T) {
		setPrefsFile(t, `{"theme":"a-drop-in"}`)
		unresolvableThemesDir(t)
		sink := logtest.Install(t)

		nomination := themeNominationForTest(t)

		assertConstant(t, nomination, shippedDark)
		assertThemeEvents(t, sink,
			"WARN fallback applied slug=a-drop-in reason=not found",
			"INFO loaded slug="+theme.DefaultDarkSlug,
		)
	})
}

// Both themes are still loaded — a commit made in that session must have
// something to persist against. Real content on the first frame is the
// observable difference between a skipped gate and a pending one.
func TestConstruction_NoColorLoadsBothSelectsDark(t *testing.T) {
	setPrefsFile(t, `{"theme_dark":"`+nordSlug+`"}`)
	sink := logtest.Install(t)

	nomination := themeNominationForTest(t)

	assertPair(t, nomination, themetest.Builtin(t, theme.DefaultLightSlug), themetest.Builtin(t, nordSlug))
	assertThemeEvents(t, sink,
		"INFO loaded slug="+theme.DefaultLightSlug+" slot=light",
		"INFO loaded slug="+nordSlug+" slot=dark",
	)

	cfg := defaultTestTUIConfig()
	cfg.theme = nomination
	cfg.noColor = true
	m := buildTUIModel(cfg, pickerLanding{}, nil)

	assertPaintedCanvas(t, m, nil)
	if content := m.View().Content; !strings.Contains(content, "Sessions") {
		t.Errorf("the first NO_COLOR frame painted no real content; want the gate skipped rather than pending")
	}
}

// Read through restore.go's canvas-echo guard, the one surface consuming the
// retained hex. Each member's canvas is replayed as the reply, so a hex captured
// from the wrong member would emit a set-back in one case and swallow a real one
// in the other.
func TestConstruction_StartupCanvasHexFromSelectedMember(t *testing.T) {
	setPrefsFile(t, `{"theme_dark":"`+nordSlug+`"}`)
	nomination := themeNominationForTest(t)
	nord := themetest.Builtin(t, nordSlug)
	day := themetest.Builtin(t, theme.DefaultLightSlug)

	for _, tc := range []struct {
		name     string
		reported theme.Theme
	}{
		{"the dark reply selects the dark member", nord},
		{"the light reply selects the light member", day},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := update(t, modelForNomination(nomination), backgroundReplyFor(tc.reported))

			if got := restoreWrite(m); got != "" {
				t.Errorf("exit-time restore wrote %q, want nothing: the terminal reported the very canvas the gate selected, so the echo guard must suppress the set-back", got)
			}
		})
	}

	t.Run("a background that is neither member's canvas is still set back", func(t *testing.T) {
		m := update(t, modelForNomination(nomination), otherBackgroundReply)

		if got := restoreWrite(m); got == "" {
			t.Errorf("exit-time restore wrote nothing; want the captured original set back (it is not the canvas the gate selected)")
		}
	})
}

func TestOpenTUI_FatalBeforeModelConstruction(t *testing.T) {
	setPrefsFile(t, `{"theme":"no-such-theme"}`)

	withOpenDeps(t, OpenDeps{ThemeLoader: brokenBuiltinLoader(theme.DefaultDarkSlug)})

	bootstrapWarnings.Drain()
	staged := bootstrap.Warning{Lines: []string{"staged: must survive the fatal"}}
	bootstrapWarnings.Add(staged)
	t.Cleanup(func() { bootstrapWarnings.Drain() })

	// The pre-construction tripwire below is only armed while tmux.InsideTmux()
	// is true, so the fixture sets TMUX itself rather than inheriting whatever
	// TestMain's package-wide poison happens to supply.
	t.Setenv("TMUX", "/nonexistent/portal-test-must-set-tmux-socket,0,0")
	if !tmux.InsideTmux() {
		t.Fatal("tmux.InsideTmux() is false after setting TMUX — the no-calls tripwire below would be unarmed")
	}

	commander := commandertest.Quiet()
	err := openTUI(cmdWithClient(tmux.NewClient(commander)), pickerLanding{}, nil, false)

	if err == nil {
		t.Fatal("openTUI returned nil; want the broken-built-in fatal")
	}
	if want := theme.BrokenBuiltinError(theme.DefaultDarkSlug).Error(); err.Error() != want {
		t.Errorf("openTUI error = %q, want %q", err.Error(), want)
	}
	if len(commander.Calls()) != 0 {
		t.Errorf("tmux commander saw %v, want no calls at all — the current-session read is the last statement before construction, so any call means the fatal returned past it", commander.Calls())
	}
	if pending := bootstrapWarnings.Drain(); len(pending) != 1 || !slices.Equal(pending[0].Lines, staged.Lines) {
		t.Errorf("bootstrap warnings after the fatal = %+v, want the seeded one still pending — a drained sink means a model was constructed on the fatal path", pending)
	}
}

// Production carries a nil BuiltinSource and reads the real embedded set.
func brokenBuiltinLoader(missing string) *theme.Loader {
	loader := theme.NewLoader(theme.NewEventLogger(nil))
	loader.BuiltinSource = func(slug string) ([]byte, bool) {
		if slug == missing {
			return nil, false
		}
		return theme.BuiltinBytes(slug)
	}
	return &loader
}

func assertPair(t *testing.T, n theme.Nomination, wantLight, wantDark theme.Theme) {
	t.Helper()
	if n.IsConstant() {
		t.Fatalf("nomination is constant; want the adaptive pair the persisted slots nominate")
	}
	if got := n.Select(theme.MemberLight); got != wantLight {
		t.Errorf("light member = %s, want %s", canvasOf(got), canvasOf(wantLight))
	}
	if got := n.Select(theme.MemberDark); got != wantDark {
		t.Errorf("dark member = %s, want %s", canvasOf(got), canvasOf(wantDark))
	}
}

func update(t *testing.T, m tui.Model, msg tea.Msg) tui.Model {
	t.Helper()
	updated, _ := m.Update(msg)
	model, ok := updated.(tui.Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", updated)
	}
	return model
}

// Init batches the gate's deadline tick alongside the OSC 11 query, so draining
// Init's commands is the no-answer path without this package naming
// internal/tui's unexported deadline message.
func resolveByTimeout(t *testing.T, m tui.Model) tui.Model {
	t.Helper()
	for _, msg := range drainCmd(t, m.Init()) {
		m = update(t, m, msg)
	}
	return m
}

func drainCmd(t *testing.T, cmd tea.Cmd) []tea.Msg {
	t.Helper()
	if cmd == nil {
		return nil
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{msg}
	}
	var msgs []tea.Msg
	for _, c := range batch {
		msgs = append(msgs, drainCmd(t, c)...)
	}
	return msgs
}

func assertThemeEvents(t *testing.T, sink *logtest.Sink, want ...string) {
	t.Helper()
	got := themeEvents(t, sink)
	if !slices.Equal(got, want) {
		t.Errorf("theme events =\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

// The canvas is the one token identifying which file was parsed: the fixtures
// differ from each other in it and in nothing else.
func assertCanvasValue(t *testing.T, th theme.Theme, want string) {
	t.Helper()
	if got := th.Canvas.Value; got != want {
		t.Errorf("loaded canvas = %q, want %q", got, want)
	}
}

func backgroundReplyFor(th theme.Theme) tea.BackgroundColorMsg {
	return tea.BackgroundColorMsg{Color: th.Canvas.Color()}
}

func restoreWrite(m tui.Model) string {
	var out strings.Builder
	tui.RestoreTerminalBackground(&out, m)
	return out.String()
}

// Mode 0111 is searchable but not listable: a by-name read still succeeds while
// any ReadDir fails, so an implementation enumerating at construction fails here
// rather than quietly paying for 50 parses on the cold path. The mode is
// restored before t.TempDir's own removal, cleanups running
// last-registered-first.
func seedUnlistableThemesDir(t *testing.T, canvases map[string]string) {
	t.Helper()

	if os.Geteuid() == 0 {
		t.Skip("running as root: mode bits do not deny, so an unlistable directory cannot be staged")
	}

	dir := useThemesDir(t)
	for slug, canvas := range canvases {
		writeThemeFile(t, dir, slug, canvas)
	}
	for i := len(canvases); i < 50; i++ {
		writeThemeFile(t, dir, fmt.Sprintf("filler-%02d", i), fmt.Sprintf("#%06X", 0x300000+i))
	}

	if err := os.Chmod(dir, 0o111); err != nil {
		t.Fatalf("make themes dir unlistable: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	if _, err := os.ReadDir(dir); err == nil {
		t.Fatal("themes dir is still listable — the no-ReadDir tripwire would be vacuous")
	}
}

func writeThemeFile(t *testing.T, dir, slug, canvas string) {
	t.Helper()

	themetest.WriteWithCanvas(t, dir, slug+".theme", canvas)
}

func unresolvableThemesDir(t *testing.T) {
	t.Helper()

	t.Setenv("PORTAL_THEMES_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "")
	if got, err := themesDirPath(); err == nil {
		t.Fatalf("themesDirPath() = %q with no env var, no XDG_CONFIG_HOME and no HOME; this subtest would be vacuous", got)
	}
}

func themeEvents(t *testing.T, sink *logtest.Sink) []string {
	t.Helper()
	var events []string
	for _, rec := range sink.Records() {
		if !rec.HasAttr("component") || rec.AttrString(t, "component") != "theme" {
			continue
		}
		parts := []string{rec.Level.String(), rec.Msg}
		for _, key := range rec.Keys {
			if key == "component" {
				continue
			}
			parts = append(parts, key+"="+rec.AttrOrEmpty(key))
		}
		events = append(events, strings.Join(parts, " "))
	}
	return events
}
