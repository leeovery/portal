package capture

import (
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/tui"
	"github.com/leeovery/portal/internal/xdg"
)

// press is keys pressed together, at a moment of a play.
type press struct {
	at   time.Duration
	keys []tea.KeyPressMsg
}

// walk is what a scenario's play presses, and when it ends: the keys its tape
// is timed for, restated from the cues' comments in scenarios.go, so every
// screen a recording shows is played here.
type walk struct {
	presses []press
	ends    time.Duration
}

var (
	keyUp    = tea.KeyPressMsg{Code: tea.KeyUp}
	keyDown  = tea.KeyPressMsg{Code: tea.KeyDown}
	keySpace = tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	keyEsc   = tea.KeyPressMsg{Code: tea.KeyEscape}
)

// pressing is a press of keys at the given seconds into a play.
func pressing(seconds float64, keys ...tea.KeyPressMsg) press {
	return press{at: offset(seconds), keys: keys}
}

// typing is text typed from the given seconds, a key every 50ms, as a tape
// types at its default speed.
func typing(seconds float64, text string) []press {
	var presses []press
	for i, r := range text {
		presses = append(presses, press{at: offset(seconds) + time.Duration(i)*50*time.Millisecond, keys: []tea.KeyPressMsg{keyRune(r)}})
	}
	return presses
}

// walks are every scenario's walk, by its name.
var walks = map[string]walk{
	"portal-cold": {
		presses: []press{pressing(4.5, keyDown, keyDown), pressing(5.5, keySpace), pressing(8.5, keyDown)},
		ends:    offset(10.5),
	},
	"portal-tour": {
		presses: slices.Concat(
			[]press{pressing(1.5, keyRune('s')), pressing(3.5, keyRune('s')), pressing(5.5, keyRune('s')), pressing(7, keyRune('/'))},
			typing(7.25, "sync"),
			[]press{
				pressing(9.5, keyEsc), pressing(10.5, keyDown, keyDown), pressing(11.5, keySpace),
				pressing(13.5, keyDown), pressing(15, keySpace), pressing(16, keyRune('x')), pressing(18, keyRune('x')),
			},
		),
		ends: offset(19.5),
	},
	// tokyo-night's row sits fifth of the six, so the arrows go up to the
	// first, then down through every one to the last.
	"themes": {
		presses: []press{
			pressing(1, keyRune('t')),
			pressing(2, keyUp), pressing(2.6, keyUp), pressing(3.2, keyUp), pressing(3.8, keyUp),
			pressing(5, keyDown), pressing(6, keyDown), pressing(7, keyDown), pressing(8, keyDown), pressing(9, keyDown),
			pressing(10.5, keyEsc),
		},
		ends: offset(12),
	},
}

// walked plays the scenario's walk to its end, calling checkpoint every step
// from the start, with the play as it stands then, before any key pressed at
// that moment.
func walked(t *testing.T, s Scenario, step time.Duration, checkpoint func(at time.Duration, p *player)) {
	t.Helper()
	w, ok := walks[s.Name]
	if !ok {
		t.Fatalf("%s has no walk", s.Name)
	}
	p := playScenario(t, s, s.Size)
	presses := w.presses
	for at := time.Duration(0); at <= w.ends; at += step {
		for len(presses) > 0 && presses[0].at < at {
			p.press(presses[0].at, presses[0].keys...)
			presses = presses[1:]
		}
		p.until(at)
		checkpoint(at, p)
	}
}

// scenarioNamed is the named scenario, failing the test where there is none.
func scenarioNamed(t *testing.T, name string) Scenario {
	t.Helper()
	s, err := ScenarioNamed(name)
	if err != nil {
		t.Fatalf("ScenarioNamed(%s): %v", name, err)
	}
	return s
}

// everyScenario is every scenario, each as ScenarioNamed gives it.
func everyScenario(t *testing.T) []Scenario {
	t.Helper()
	var all []Scenario
	for _, name := range ScenarioNames() {
		all = append(all, scenarioNamed(t, name))
	}
	if len(all) == 0 {
		t.Fatal("there are no scenarios")
	}
	return all
}

func TestScenarioNamesNameEveryScenarioOnceSorted(t *testing.T) {
	names := ScenarioNames()
	if want := []string{"portal-cold", "portal-tour", "themes"}; !slices.Equal(names, want) {
		t.Errorf("ScenarioNames() = %q, want %q", names, want)
	}
	if !slices.IsSorted(names) || len(slices.Compact(slices.Clone(names))) != len(names) {
		t.Errorf("ScenarioNames() = %q, want each once, sorted", names)
	}
	for _, name := range names {
		if s := scenarioNamed(t, name); s.Name != name {
			t.Errorf("ScenarioNamed(%s) is %s", name, s.Name)
		}
	}
}

func TestAnEmptyOrUnknownScenarioNameListsTheScenarios(t *testing.T) {
	tests := map[string]string{
		"":        "name a scenario (available: portal-cold, portal-tour, themes)",
		"tour":    `unknown scenario "tour" (available: portal-cold, portal-tour, themes)`,
		"loading": `unknown scenario "loading" (available: portal-cold, portal-tour, themes)`,
	}
	for name, want := range tests {
		if _, err := ScenarioNamed(name); err == nil || err.Error() != want {
			t.Errorf("ScenarioNamed(%q) error = %v, want %s", name, err, want)
		}
	}
}

func TestNoScenarioSharesAFixturesName(t *testing.T) {
	for _, name := range ScenarioNames() {
		if slices.Contains(FixtureNames(), name) {
			t.Errorf("the scenario %s shares a fixture's name, so capturetool's names would be ambiguous", name)
		}
	}
}

// Each scenario is made for the frames' terminal, so a checkpoint of one can be
// held to a golden frame, and is drawn in the frames' theme unless told
// otherwise.
func TestEachScenarioIsMadeForTheFramesSize(t *testing.T) {
	framesTheme := framesTheme(t)
	for _, s := range everyScenario(t) {
		if s.Size != harnessSize {
			t.Errorf("%s is made for %s, want the frames' %s", s.Name, s.Size, harnessSize)
		}
		if s.theme != framesTheme {
			t.Errorf("%s is drawn in another theme than the frames' %s", s.Name, FramesThemeSlug)
		}
	}
}

func TestEveryScenarioHasAWalk(t *testing.T) {
	for _, name := range ScenarioNames() {
		if _, ok := walks[name]; !ok {
			t.Errorf("%s has no walk, so no test plays it", name)
		}
	}
	for name := range walks {
		if !slices.Contains(ScenarioNames(), name) {
			t.Errorf("the walk of %s plays no scenario", name)
		}
	}
}

func TestEveryScenarioPlaysTheSameWayTwice(t *testing.T) {
	const step = 250 * time.Millisecond
	for _, s := range everyScenario(t) {
		t.Run(s.Name, func(t *testing.T) {
			var plays [2][]string
			for i := range plays {
				synctest.Test(t, func(t *testing.T) {
					walked(t, s, step, func(_ time.Duration, p *player) {
						plays[i] = append(plays[i], p.screen())
					})
				})
			}
			if len(plays[0]) == 0 || len(plays[0]) != len(plays[1]) {
				t.Fatalf("played %d screens, then %d, want the same, more than none", len(plays[0]), len(plays[1]))
			}
			for i := range plays[0] {
				if plays[0][i] != plays[1][i] {
					t.Fatalf("%s into it, the screen played\n%s\nthen\n%s\nwant the same each time", time.Duration(i)*step, plays[0][i], plays[1][i])
				}
			}
			if slices.Equal(slices.Compact(slices.Clone(plays[0])), plays[0][:1]) {
				t.Error("every screen it played is the same, want the walk to move it")
			}
		})
	}
}

// A scenario's bootstrap reaches the loading page in the order it happens,
// ending in the complete, which the receiver needs to end, at the moment the
// restore lands.
func TestEveryScenariosBootEndsInItsCompleteAsItsRestoreLands(t *testing.T) {
	for _, s := range everyScenario(t) {
		if len(s.boot) == 0 {
			if s.restoredAt != 0 {
				t.Errorf("%s restores at %s with no bootstrap, want its sessions listed from the start", s.Name, s.restoredAt)
			}
			continue
		}
		if !slices.IsSortedFunc(s.boot, func(a, b bootCue) int { return int(a.at - b.at) }) {
			t.Errorf("%s's bootstrap is out of order", s.Name)
		}
		last := s.boot[len(s.boot)-1]
		if _, ok := last.msg.(tui.BootstrapCompleteMsg); !ok || last.at != s.restoredAt {
			t.Errorf("%s's bootstrap ends %T at %s, want its complete as its restore lands, at %s", s.Name, last.msg, last.at, s.restoredAt)
		}
	}
}

func TestTheColdScenarioMatchesTheLoadingFrameAtEightOfTwelve(t *testing.T) {
	want, err := os.ReadFile(filepath.Join(framesDir, "loading-screen-120x40.txt"))
	if err != nil {
		t.Fatalf("read the loading frame: %v", err)
	}
	synctest.Test(t, func(t *testing.T) {
		p := playScenario(t, scenarioNamed(t, "portal-cold"), harnessSize)
		p.until(offset(2))
		if got := p.screen(); got != string(want) {
			t.Errorf("at 8 of 12, the screen is\n%s\nwant the loading frame\n%s", got, want)
		}
	})
}

func TestTheColdScenarioRestoresOntoTheSessionsPage(t *testing.T) {
	fx := coldWorld()
	synctest.Test(t, func(t *testing.T) {
		p := playScenario(t, scenarioNamed(t, "portal-cold"), harnessSize)
		p.until(offset(coldCompletes) - time.Millisecond)
		if screen := p.screen(); !strings.Contains(screen, tui.LabelRestoringSessions) || strings.Contains(screen, "Sessions 12") {
			t.Errorf("as the restore lands, the screen is\n%s\nwant the loading page still", screen)
		}
		p.until(offset(coldCompletes))
		screen := p.screen()
		if !strings.Contains(screen, "Sessions 12") {
			t.Fatalf("restored, the screen is\n%s\nwant the sessions page, its twelve sessions listed", screen)
		}
		for _, s := range fx.Lister.sessions {
			row := rowNaming(screen, s.Name)
			waiting := slices.Contains(fx.pendingSessions, s.Name)
			if row == "" || strings.HasSuffix(row, "●") != waiting {
				t.Errorf("restored, %s's row reads %q, want it listed, its dot shown iff it waits to resume", s.Name, row)
			}
		}
	})
}

// rowNaming is the screen's row naming the session, or empty where none does.
func rowNaming(screen, name string) string {
	for row := range strings.Lines(screen) {
		if strings.Contains(row, " "+name) {
			return strings.TrimSuffix(row, "\n")
		}
	}
	return ""
}

// The themes scenario previews each built-in as the panel's cursor reaches it,
// in its own palette, and Esc goes back to the theme it's drawn in.
func TestTheThemesScenarioPreviewsEachBuiltIn(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := scenarioNamed(t, "themes")
		var shown []string
		walked(t, s, 250*time.Millisecond, func(_ time.Duration, p *player) {
			slug := canvasSlug(t, p.model.View())
			if len(shown) == 0 || shown[len(shown)-1] != slug {
				shown = append(shown, slug)
			}
		})
		want := []string{"tokyo-night", "nord-wave", "nord", "horizon", "gitlab-dark-grey", "horizon", "nord", "nord-wave", "tokyo-night", "tokyo-night-day", "tokyo-night"}
		if !slices.Equal(shown, want) {
			t.Errorf("the screen is drawn in %q in turn, want %q", shown, want)
		}
	})
}

// canvasSlug is the built-in whose canvas the view paints.
func canvasSlug(t *testing.T, v tea.View) string {
	t.Helper()
	for slug, th := range builtins() {
		if v.BackgroundColor != nil && sameColour(th.Canvas.Color(), v.BackgroundColor) {
			return slug
		}
	}
	t.Fatalf("the view paints %v, no built-in's canvas", v.BackgroundColor)
	return ""
}

// A scenario's own theme source is the panel's, opened on the keys it gives, for
// the palette the scenario is drawn in.
func TestAScenariosOwnThemeSourceIsThePanels(t *testing.T) {
	nord := themeNamed(t, "nord")
	var asked theme.Theme
	source := previewingThemeSource{slug: "nord"}
	s := Scenario{Name: "own", Size: harnessSize, world: tourWorld, theme: nord, themes: func(palette theme.Theme) (tui.ThemeSource, theme.RawKeys) {
		asked = palette
		return source, theme.RawKeys{Theme: "nord"}
	}}
	deps := s.deps(s.world(), func() time.Duration { return 0 })
	if !reflect.DeepEqual(deps.ThemeSource, source) || deps.ThemeKeys != (theme.RawKeys{Theme: "nord"}) || asked != nord {
		t.Errorf("the panel's source is %#v on %+v, asked for %v, want the scenario's own on its keys, for its palette", deps.ThemeSource, deps.ThemeKeys, asked)
	}
	if deps := scenarioNamed(t, "portal-tour").deps(tourWorld(), func() time.Duration { return 0 }); deps.ThemeSource != nil {
		t.Errorf("a scenario with no theme source of its own opens the world's, none, not %#v", deps.ThemeSource)
	}
}

// The previewing source keeps every built-in's own palette on its row, and
// resolves to the palette the scenario is drawn in, which it marks.
func TestThePreviewingSourceKeepsEachBuiltInsOwnPalette(t *testing.T) {
	tokyo := themeNamed(t, FramesThemeSlug)
	source, keys := previewingThemes(tokyo)
	_, union := source.Open(keys)
	palettes := maps.Collect(builtins())
	if len(union.Rows) != len(palettes) {
		t.Fatalf("the panel lists %d rows, want the %d built-ins", len(union.Rows), len(palettes))
	}
	for _, row := range union.Rows {
		if row.Theme != palettes[row.Slug] {
			t.Errorf("the row of %s carries another palette than its own", row.Slug)
		}
	}
	resolution, err := source.Resolve(theme.Enumeration{}, keys)
	if err != nil || keys.Theme != FramesThemeSlug || len(resolution.Slots) != 1 || resolution.Slots[0].Theme != tokyo || resolution.Slots[0].Resolved != FramesThemeSlug {
		t.Errorf("it resolves to %+v (%v) on %+v, want %s, a constant, marked", resolution.Slots, err, keys, FramesThemeSlug)
	}
	if _, keys := previewingThemes(themeWithCanvas(tokyo, "#102030")); keys.Theme != "" {
		t.Errorf("a palette that is no built-in marks %q, want none", keys.Theme)
	}
}

// themeNamed is the built-in with the given slug.
func themeNamed(t *testing.T, slug string) theme.Theme {
	t.Helper()
	result, rejection, found := theme.NewSilentLoader().LoadBuiltin(slug)
	if !found || rejection != nil {
		t.Fatalf("the built-in %s doesn't load: %v", slug, rejection)
	}
	return result.Theme
}

// themeWithCanvas is th painted on another canvas.
func themeWithCanvas(th theme.Theme, canvas string) theme.Theme {
	th.Canvas.Value = canvas
	return th
}

// redirected are the environment's paths a write could reach: home and every
// path Portal's config and state resolve from.
func redirected() []string {
	return append([]string{"HOME", "XDG_STATE_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME"}, xdg.Variables()...)
}

// No scenario reaches a terminal, a session, a spawn or a file: each seam that
// would is left as a fixture leaves it, nil, and a scenario walked through, and
// through every key that writes elsewhere, writes nothing where home and the
// config resolve.
func TestNoScenarioWritesOrReachesTheSystem(t *testing.T) {
	for _, s := range everyScenario(t) {
		t.Run(s.Name, func(t *testing.T) {
			deps := s.deps(s.world(), func() time.Duration { return 0 })
			seams := map[string]any{
				"Detector": deps.Detector, "Resolve": deps.Resolve, "SpawnExe": deps.SpawnExe, "AckChannel": deps.AckChannel,
				"SpawnEnv": deps.SpawnEnv, "SessionExists": deps.SessionExists, "DirRunner": deps.DirRunner,
				"ModePersister": deps.ModePersister, "ThemePersister": deps.ThemePersister, "Hydrating": deps.Hydrating,
				"CreateFatal": deps.CreateFatal,
			}
			for name, seam := range seams {
				if v := reflect.ValueOf(seam); v.IsValid() && !v.IsZero() {
					t.Errorf("%s's %s is wired, want it nil", s.Name, name)
				}
			}

			root := t.TempDir()
			for _, name := range redirected() {
				t.Setenv(name, filepath.Join(root, name))
			}
			synctest.Test(t, func(t *testing.T) {
				walked(t, s, time.Second, func(at time.Duration, p *player) {
					if at == walks[s.Name].ends {
						p.press(at, keyEsc, keyRune('k'), keyRune('y'), keyRune('r'), keyRune('z'), keyEnter(), keyRune('s'),
							keyRune('t'), keyEnter(), keyRune('m'), keyRune('n'))
					}
				})
			})
			if entries, err := os.ReadDir(root); err != nil || len(entries) > 0 {
				t.Errorf("played, %s wrote %v (%v) where home and the config resolve, want nothing", s.Name, entries, err)
			}
		})
	}
}

// toolNames are the tools no demo may name: Portal's resume is generic.
var toolNames = []string{"claude", "fable", "brewed", "codex", "cursor", "aider"}

// personalNames are names of the developer's own, which no demo may show.
var personalNames = []string{"leeovery", "lee overy", "folio", "switchboard", "/users/"}

func TestNoScenarioNamesAToolOrAnythingPersonal(t *testing.T) {
	for _, s := range everyScenario(t) {
		fx := s.world()
		said := slices.Collect(maps.Values(s.histories))
		said = append(said, s.Name)
		for _, session := range fx.Lister.sessions {
			said = append(said, session.Name, session.Label)
			if !strings.HasPrefix(session.Dir, fixtureHome+"/") {
				t.Errorf("%s's session %s is in %s, want it under the fixtures' home, %s", s.Name, session.Name, session.Dir, fixtureHome)
			}
		}
		for _, p := range fx.projectStore.projects {
			said = append(said, p.Name, p.Path)
			said = append(said, p.Tags...)
		}
		for _, text := range said {
			for _, name := range slices.Concat(toolNames, personalNames) {
				if strings.Contains(strings.ToLower(text), name) {
					t.Errorf("%s says %q, which names %s", s.Name, text, name)
				}
			}
		}
	}
}

// Before its restore lands, a scenario lists nothing and nothing waits to
// resume; from then on, the world's sessions and those of them that wait.
func TestTheTimedFakesAnswerTheRestoredSessionsFromTheRestore(t *testing.T) {
	var now time.Duration
	elapsed := func() time.Duration { return now }
	sessions := []tui.Session{{Name: "kiln-aws-migration"}, {Name: "loom-p84cXa"}}
	lister := timedLister{elapsed: elapsed, restoredAt: time.Second, sessions: sessions}
	pending := timedPendingReader{elapsed: elapsed, restoredAt: time.Second, sessions: []string{"loom-p84cXa"}}

	now = time.Second - time.Nanosecond
	if listed, _ := lister.ListSessions(); len(listed) > 0 {
		t.Errorf("before the restore, it lists %v, want none", listed)
	}
	if waiting, _ := pending.ListPendingResumeSessions(); len(waiting) > 0 {
		t.Errorf("before the restore, %v wait to resume, want none", waiting)
	}
	now = time.Second
	if listed, _ := lister.ListSessions(); !slices.Equal(listed, sessions) {
		t.Errorf("restored, it lists %v, want %v", listed, sessions)
	}
	if waiting, _ := pending.ListPendingResumeSessions(); len(waiting) != 1 {
		t.Errorf("restored, %v wait to resume, want loom-p84cXa", waiting)
	}
}

// The receiver hands each event over at its moment, and once past the
// complete, answers nil at once rather than blocking, as the fixtures' does.
func TestTheBootReceiverEndsAtItsComplete(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		r := &bootReceiver{elapsed: func() time.Duration { return time.Since(start) }, cues: []bootCue{checks(0.25), completes(1)}}
		for _, want := range []struct {
			msg tea.Msg
			at  time.Duration
		}{
			{msg: tui.BootstrapProgressMsg{Index: checkZmxStep}, at: offset(0.25)},
			{msg: tui.BootstrapCompleteMsg{}, at: time.Second},
			{msg: nil, at: time.Second},
		} {
			if msg := r.receive(); !reflect.DeepEqual(msg, want.msg) || time.Since(start) != want.at {
				t.Errorf("received %#v at %s, want %#v at %s", msg, time.Since(start), want.msg, want.at)
			}
		}
	})
}
