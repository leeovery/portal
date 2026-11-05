package capture

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/tui"
)

// shows is what a screen shows: what it says, what it no longer says, the
// golden frame whose text it is, and the built-in it's drawn in, each where
// it's checked.
type shows struct {
	says, not    []string
	frame, theme string
}

// checkpoint is what a tape's screen shows, a moment into its play.
type checkpoint struct {
	at float64
	shows
}

// holding is what a tape is held to: what its prelude starts; what it shows at
// moments of its play; and what it shows as its last Show ends, which is a
// still's art.
type holding struct {
	plays       launch
	checkpoints []checkpoint
	ends        shows
}

// launch is what a tape's prelude starts capturetool on: a scenario or a
// fixture, by its name, in a theme.
type launch struct {
	kind, name, theme string
}

// Shown as a tape's prelude types it.
func (l launch) String() string {
	return fmt.Sprintf("--theme %s --%s %s", l.theme, l.kind, l.name)
}

// scenarioIn and fixtureIn are the named scenario or fixture launched in the
// frames' theme, as every demo is drawn.
func scenarioIn(name string) launch {
	return launch{kind: "scenario", name: name, theme: FramesThemeSlug}
}

func fixtureIn(name string) launch {
	return launch{kind: "fixture", name: name, theme: FramesThemeSlug}
}

// held are the demo tapes, by name, as the README's art has them: each tape
// plays its scenario or fixture, and shows what each checkpoint says, at its
// moment. A tape's keys are read from the tape itself; these are the screens
// they lead to.
var held = map[string]holding{
	"portal-cold": {
		plays: scenarioIn("portal-cold"),
		checkpoints: []checkpoint{
			{at: 0, shows: shows{says: []string{"Checking zmx", "Starting the saver", "Restoring sessions"}, not: []string{"Sessions 12"}}},
			{at: 3.5, shows: shows{says: []string{"Sessions 12", "▌ atelier-web-m4TzpD"}, not: []string{"Restoring sessions"}}},
			{at: 5, shows: shows{says: []string{"Sessions 12", "▌ kiln-relayx-explore"}}},
			{at: 6.5, shows: shows{says: []string{"◉ preview kiln-relayx-explore", "Apply complete! Resources: 4 added, 2 changed, 0 destroyed."}}},
		},
		ends: shows{says: []string{"◉ preview loom-p84cXa", "48 passed in 2.31s"}, not: []string{"kiln-relayx-explore"}},
	},
	"portal-tour": {
		plays: scenarioIn("portal-tour"),
		checkpoints: []checkpoint{
			{at: 1, shows: shows{says: []string{"Sessions 8", "▌ atelier-web-m4TzpD"}, not: []string{"— by"}}},
			{at: 2.5, shows: shows{says: []string{"Sessions 8 — by project", "orchard-pipeline ··· 2"}}},
			// By tag, its keys faded, as the by-tag fixture draws it.
			{at: 5.25, shows: shows{frame: "sessions-by-tag"}},
			{at: 6.5, shows: shows{says: []string{"Sessions 8", "▌ atelier-web-m4TzpD"}, not: []string{"— by"}}},
			{at: 8.4, shows: shows{says: []string{"/ sync", "lumen-sync-engine"}, not: []string{"kiln-aws-migration"}}},
			{at: 10, shows: shows{says: []string{"Sessions 8", "▌ atelier-web-m4TzpD"}, not: []string{"/ sync"}}},
			{at: 11, shows: shows{says: []string{"▌ kiln-relayx-explore"}}},
			{at: 12.5, shows: shows{says: []string{"◉ preview kiln-relayx-explore", "Apply complete! Resources: 4 added, 2 changed, 0 destroyed."}}},
			{at: 14.25, shows: shows{says: []string{"◉ preview lumen webhooks and watchers", "Tests: 42 passed, 42 total"}}},
			{at: 15.5, shows: shows{says: []string{"Sessions 8", "▌ lumen webhooks and watchers"}, not: []string{"◉ preview"}}},
			{at: 17, shows: shows{says: []string{"Projects 5", "▌ orchard-pipeline"}}},
		},
		ends: shows{says: []string{"Sessions 8", "▌ lumen webhooks and watchers"}, not: []string{"Projects 5"}},
	},
	"themes": {
		plays: scenarioIn("themes"),
		checkpoints: []checkpoint{
			{at: 0.5, shows: shows{says: []string{"Sessions 8"}, not: []string{"Themes"}, theme: "tokyo-night"}},
			{at: 1.5, shows: shows{says: []string{"│ Themes", "▌ tokyo-night "}, theme: "tokyo-night"}},
			{at: 2.4, shows: shows{says: []string{"▌ nord-wave"}, theme: "nord-wave"}},
			{at: 4.8, shows: shows{says: []string{"▌ gitlab-dark-grey"}, theme: "gitlab-dark-grey"}},
			{at: 9, shows: shows{says: []string{"▌ tokyo-night-day"}, theme: "tokyo-night-day"}},
		},
		ends: shows{says: []string{"Sessions 8"}, not: []string{"Themes"}, theme: "tokyo-night"},
	},
	"loading":         {plays: scenarioIn("portal-cold"), ends: shows{frame: "loading-screen"}},
	"sessions-by-tag": {plays: fixtureIn("sessions-by-tag")},
	"preview":         {plays: fixtureIn("preview-screen")},
	"help-modal":      {plays: fixtureIn("sessions-help")},
	"projects":        {plays: fixtureIn("projects")},
	"edit-project":    {plays: fixtureIn("projects-edit")},
	"filtering":       {plays: fixtureIn("sessions-filtering")},
	"kill-confirm":    {plays: fixtureIn("sessions-kill-confirm")},
	"resume-panel":    {plays: fixtureIn("resume-panel-waiting")},
	"theme-panel":     {plays: fixtureIn("theme-panel-invalid-row")},
}

// heldTape is the named tape and what it's held to.
func heldTape(t *testing.T, name string) (tape, holding) {
	t.Helper()
	h, ok := held[name]
	if !ok {
		t.Fatalf("demo/%s.tape is held to nothing", name)
	}
	return readTape(t, name), h
}

// preludeLine is the one line a tape types to the shell: capturetool, in UTC,
// on a scenario or fixture, in a theme, and Enter.
var preludeLine = regexp.MustCompile(`^TZ=UTC demo/out/capturetool --theme ([a-z0-9-]+) --(scenario|fixture) ([a-z0-9-]+)\r$`)

// launched is what the tape's prelude starts.
func (tp tape) launched(t *testing.T) launch {
	t.Helper()
	m := preludeLine.FindStringSubmatch(tp.prelude)
	if m == nil {
		t.Fatalf("demo/%s.tape types %q to the shell, want capturetool alone, in UTC, on a scenario or fixture, in a theme", tp.name, tp.prelude)
	}
	return launch{theme: m[1], kind: m[2], name: m[3]}
}

// grid is the terminal the tape records, in cells, as its width and height in
// pixels hold them: cells of 10 × 22 px, inside its padding and margins.
func (tp tape) grid(t *testing.T) Size {
	t.Helper()
	w, wErr := strconv.Atoi(tp.settings["Width"])
	h, hErr := strconv.Atoi(tp.settings["Height"])
	if wErr != nil || hErr != nil {
		t.Fatalf("demo/%s.tape sets its width and height as %q × %q, want pixels", tp.name, tp.settings["Width"], tp.settings["Height"])
	}
	return Size{Width: (w - 57) / 10, Height: (h - 42) / 22}
}

// stillsListed are the tapes demo/record.sh makes stills of.
func stillsListed(t *testing.T) []string {
	t.Helper()
	src, err := os.ReadFile(filepath.Join(demoDir, "record.sh"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^stills="([^"]*)"$`).FindSubmatch(src)
	if m == nil {
		t.Fatal(`demo/record.sh sets no stills="…"`)
	}
	return strings.Fields(string(m[1]))
}

// lastShownEnd is when the tape's last recorded span ends.
func (tp tape) lastShownEnd(t *testing.T) time.Duration {
	t.Helper()
	last, ok := tp.lastShown()
	if !ok {
		t.Fatalf("demo/%s.tape shows nothing", tp.name)
	}
	return last.to
}

// playedTape plays the tape's program as vhs does, inside the bubble: started
// at the tape's zero on its grid, each key pressed at its moment. It calls at
// at each moment asked, before the keys pressed then, and pressed after each
// key, saying whether the program had quit before it.
func playedTape(t *testing.T, tp tape, moments []time.Duration, at func(time.Duration, *player), pressed func(k tapeKey, quitBefore bool, p *player)) {
	t.Helper()
	p := startedTape(t, tp)
	keys := tp.keys
	slices.Sort(moments)
	press := func(k tapeKey) {
		p.until(k.at)
		quitBefore := p.quit
		p.press(k.at, k.key)
		if pressed != nil {
			pressed(k, quitBefore, p)
		}
	}
	for _, m := range moments {
		for len(keys) > 0 && keys[0].at < m {
			press(keys[0])
			keys = keys[1:]
		}
		p.until(m)
		at(m, p)
	}
	for _, k := range keys {
		press(k)
	}
}

// startedTape is the tape's program started as its prelude starts it, on the
// tape's grid: a scenario played, or a fixture run live, as capturetool runs
// each, a fixture's declared size substituted for the terminal's.
func startedTape(t *testing.T, tp tape) *player {
	t.Helper()
	l := tp.launched(t)
	th := themeNamed(t, l.theme)
	grid := tp.grid(t)
	if l.kind == "scenario" {
		return playScenario(t, scenarioNamed(t, l.name).InTheme(th), grid)
	}
	resize := tea.WindowSizeMsg{Width: grid.Width, Height: grid.Height}
	if sf, ok := SurfaceByName(l.name); ok {
		resize = sf.PinRenderSize(resize).(tea.WindowSizeMsg)
		return playing(t, sf.WithTheme(th, false), Size{Width: resize.Width, Height: resize.Height})
	}
	fx, err := FixtureByName(l.name)
	if err != nil {
		t.Fatal(err)
	}
	resize = fx.PinRenderSize(resize).(tea.WindowSizeMsg)
	return playing(t, tui.Build(fx.Deps(th)), Size{Width: resize.Width, Height: resize.Height})
}

// check fails the test where the play's screen doesn't show what's held.
func (want shows) check(t *testing.T, at time.Duration, p *player) {
	t.Helper()
	screen := p.screen()
	for _, says := range want.says {
		if !strings.Contains(screen, says) {
			t.Errorf("%s in, it shows\n%s\nwant %q", at, screen, says)
		}
	}
	for _, not := range want.not {
		if strings.Contains(screen, not) {
			t.Errorf("%s in, it shows\n%s\nwant no %q", at, screen, not)
		}
	}
	if want.frame != "" {
		if frame := frameText(t, want.frame); screen != frame {
			t.Errorf("%s in, it shows\n%s\nwant %s's frame\n%s", at, screen, want.frame, frame)
		}
	}
	if want.theme != "" {
		if shown := canvasSlug(t, p.model.View()); shown != want.theme {
			t.Errorf("%s in, it's drawn in %s, want %s", at, shown, want.theme)
		}
	}
}

// frameText is the text of the named drawing's golden frame.
func frameText(t *testing.T, name string) string {
	t.Helper()
	d, err := DrawingByName(name)
	if err != nil {
		t.Fatal(err)
	}
	frame, err := os.ReadFile(filepath.Join(framesDir, d.Stem()+textExt))
	if err != nil {
		t.Fatalf("read %s's frame: %v", name, err)
	}
	return string(frame)
}

// A tape plays its scenario or fixture, pressing its keys at its times, as
// it's recorded, and what it shows at each checkpoint, and as its last Show
// ends, is what its cues are timed for. A Wait+Screen waits for a screen the
// program draws by then, the first for the one it draws as it starts.
func TestEachTapePlaysItsScenarioAsItsCuesAreTimedFor(t *testing.T) {
	for _, name := range demoTapeNames(t) {
		t.Run(name, func(t *testing.T) {
			tp, h := heldTape(t, name)
			ends := tp.lastShownEnd(t)
			want := map[time.Duration]shows{ends: h.ends}
			for _, c := range h.checkpoints {
				want[offset(c.at)] = c.shows
			}
			waits := map[time.Duration][]*regexp.Regexp{}
			moments := slices.Collect(maps.Keys(want))
			for _, w := range tp.waits {
				waits[w.at] = append(waits[w.at], w.re)
				moments = append(moments, w.at)
			}
			slices.Sort(moments)
			moments = slices.Compact(moments)
			synctest.Test(t, func(t *testing.T) {
				playedTape(t, tp, moments, func(at time.Duration, p *player) {
					for _, re := range waits[at] {
						if screen := p.screen(); !re.MatchString(screen) {
							t.Errorf("%s in, it waits for /%s/, but the screen is\n%s", at, re, screen)
						}
					}
					want[at].check(t, at, p)
				}, nil)
			})
		})
	}
}

// The tapes are held as their art: every one has a row, and every row a tape.
func TestEveryDemoTapeIsHeld(t *testing.T) {
	tapes := demoTapeNames(t)
	for _, name := range tapes {
		if _, ok := held[name]; !ok {
			t.Errorf("demo/%s.tape is held to nothing: give it a row in held", name)
		}
	}
	for name := range held {
		if !slices.Contains(tapes, name) {
			t.Errorf("held has a row for %s, which demo/ has no tape of", name)
		}
	}
}

// Each tape starts what its row says, and that scenario or fixture exists, in a
// built-in theme.
func TestEveryDemoTapeNamesAScenarioOrFixtureThatExists(t *testing.T) {
	exists := map[string][]string{"scenario": ScenarioNames(), "fixture": FixtureNames()}
	for _, name := range demoTapeNames(t) {
		tp, h := heldTape(t, name)
		l := tp.launched(t)
		if l != h.plays {
			t.Errorf("demo/%s.tape plays %s, want %s, as its row has it", name, l, h.plays)
		}
		if !slices.Contains(exists[l.kind], l.name) {
			t.Errorf("demo/%s.tape plays the %s %s, which there's none of", name, l.kind, l.name)
		}
		if !slices.Contains(theme.BuiltinSlugs(), l.theme) {
			t.Errorf("demo/%s.tape is drawn in %s, no built-in", name, l.theme)
		}
	}
}

// Each tape runs capturetool and nothing else: before its program draws, it
// types the one line that starts it, hidden, so no shell is recorded, and no
// portal, zmx, tmux or container is run.
func TestEveryDemoTapeRunsOnlyTheCaptureTool(t *testing.T) {
	for _, name := range demoTapeNames(t) {
		tp := readTape(t, name)
		tp.launched(t)
		if !tp.preludeHidden {
			t.Errorf("demo/%s.tape shows its shell before its program draws, want its prelude hidden", name)
		}
		for _, w := range tp.waits {
			if w.re.MatchString(tp.prelude) {
				t.Errorf("demo/%s.tape waits for /%s/, which the line it types matches, so the wait proves nothing drawn", name, w.re)
			}
		}
	}
}

// Every tape has Switchboard's header: the font and its spacing, the padding,
// no blinking cursor, 30 frames a second; a grid of whole cells the size its
// program plays at; a terminal theme of the one it's drawn in, the cursor the
// canvas, so it's never seen; and its frames written to its own directory in
// demo/out.
func TestEveryDemoTapeHasTheHeader(t *testing.T) {
	want := map[string]string{
		"Shell": `"bash"`, "FontFamily": `"JetBrains Mono"`, "FontSize": "17", "LetterSpacing": "0", "LineHeight": "1",
		"Padding": "16", "CursorBlink": "false", "Framerate": "30", "WaitTimeout": "2m",
	}
	for _, name := range demoTapeNames(t) {
		tp := readTape(t, name)
		for key, value := range want {
			if got := tp.settings[key]; got != value {
				t.Errorf("demo/%s.tape sets %s %q, want %q", name, key, got, value)
			}
		}
		if want := "demo/out/" + name + "/"; tp.output != want {
			t.Errorf("demo/%s.tape writes its frames to %q, want %q", name, tp.output, want)
		}
		if grid, size := tp.grid(t), playedSize(t, tp); grid != size {
			t.Errorf("demo/%s.tape records %s cells, want its program's %s", name, grid, size)
		}
		checkTerminalTheme(t, tp)
	}
}

// playedSize is the size the tape's program plays at: its scenario's, or its
// fixture's frame's.
func playedSize(t *testing.T, tp tape) Size {
	t.Helper()
	l := tp.launched(t)
	if l.kind == "scenario" {
		return scenarioNamed(t, l.name).Size
	}
	d, err := DrawingByName(l.name)
	if err != nil {
		t.Fatal(err)
	}
	return d.Size()
}

// ansiColours are the sixteen colours a terminal theme sets.
var ansiColours = []string{
	"black", "red", "green", "yellow", "blue", "magenta", "cyan", "white",
	"brightBlack", "brightRed", "brightGreen", "brightYellow", "brightBlue", "brightMagenta", "brightCyan", "brightWhite",
}

// hexColour is a colour as a terminal theme gives it.
var hexColour = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

// checkTerminalTheme fails the test where the tape's terminal theme isn't the
// one its program is drawn in: named for it, its background and cursor the
// canvas, its foreground the primary text, its selection the selection's, and
// all sixteen colours set.
func checkTerminalTheme(t *testing.T, tp tape) {
	t.Helper()
	var set map[string]string
	if err := json.Unmarshal([]byte(tp.settings["Theme"]), &set); err != nil {
		t.Errorf("demo/%s.tape's Theme is no theme: %v", tp.name, err)
		return
	}
	l := tp.launched(t)
	th := themeNamed(t, l.theme)
	for key, want := range map[string]string{
		"name": l.theme, "background": th.Canvas.Value, "cursor": th.Canvas.Value,
		"foreground": th.TextPrimary.Value, "selection": th.BgSelection.Value,
	} {
		if !strings.EqualFold(set[key], want) {
			t.Errorf("demo/%s.tape's Theme has %s %q, want %s's %q", tp.name, key, set[key], l.theme, want)
		}
	}
	for _, key := range ansiColours {
		if !hexColour.MatchString(set[key]) {
			t.Errorf("demo/%s.tape's Theme has %s %q, want a colour", tp.name, key, set[key])
		}
	}
}

// record.sh makes stills only of tapes there are, and of every tape that runs
// a fixture, which plays nothing in time.
func TestRecordShListsOnlyTapesThatExist(t *testing.T) {
	tapes := demoTapeNames(t)
	stills := stillsListed(t)
	if len(stills) == 0 {
		t.Fatal("demo/record.sh lists no stills")
	}
	if len(slices.Compact(slices.Sorted(slices.Values(stills)))) != len(stills) {
		t.Errorf("demo/record.sh lists a still twice: %q", stills)
	}
	for _, name := range stills {
		if !slices.Contains(tapes, name) {
			t.Errorf("demo/record.sh makes a still of %s, which demo/ has no tape of", name)
		}
	}
	for _, name := range tapes {
		still := slices.Contains(stills, name)
		switch l := readTape(t, name).launched(t); {
		case l.kind == "fixture" && !still:
			t.Errorf("demo/%s.tape runs a fixture, which plays nothing in time, but record.sh makes a clip of it", name)
		case l.kind == "scenario" && still && held[name].ends.frame == "":
			t.Errorf("demo/%s.tape is a still of a scenario, but its art is held to no golden frame", name)
		}
	}
}

// A still of a fixture shows the fixture's golden frame as its last Show ends,
// whatever keys reach it, so its art is what the frames hold the code to.
func TestEachFixtureStillShowsItsFixturesFrame(t *testing.T) {
	var ran bool
	for _, name := range demoTapeNames(t) {
		tp := readTape(t, name)
		l := tp.launched(t)
		if l.kind != "fixture" {
			continue
		}
		ran = true
		t.Run(name, func(t *testing.T) {
			ends := tp.lastShownEnd(t)
			synctest.Test(t, func(t *testing.T) {
				playedTape(t, tp, []time.Duration{ends}, func(at time.Duration, p *player) {
					shows{frame: l.name}.check(t, at, p)
				}, nil)
			})
		})
	}
	if !ran {
		t.Fatal("no tape runs a fixture")
	}
}

// quarterSecond is how far a checkpoint sits from any key or cue: a recording
// starts its clock when vhs sees the program's first screen, a little after the
// program's own, and its keys land a little late, so a screen checked so close
// to a change could be recorded either side of it.
const quarterSecond = 250 * time.Millisecond

// tightWindow is the stretch a tape's art is shown in, between two cues closer
// than a quarter second either side of it, and why the art is that moment.
type tightWindow struct {
	from, to float64
	why      string
}

// tightCheckpoints are the tapes whose art sits closer than a quarter second
// to the cues either side of it, by design, each with the window between them.
var tightCheckpoints = map[string]tightWindow{
	"loading": {from: 1.95, to: 2.1, why: "8 of 12 restored, as the loading frame draws it, shows from the 8th restore to the 9th"},
}

// changesOf are the moments the tape's screen changes but for what its
// program does of itself, by what changes it: each key it presses; and where
// it plays a scenario, each of its cues, its restore, and each run of keys'
// keycaps fading.
func changesOf(t *testing.T, tp tape) map[time.Duration]string {
	t.Helper()
	changes := map[time.Duration]string{}
	for _, k := range tp.keys {
		changes[k.at] = "the key " + keyName(k.key)
	}
	l := tp.launched(t)
	if l.kind != "scenario" {
		return changes
	}
	s := scenarioNamed(t, l.name)
	for _, c := range s.boot {
		changes[c.at] = fmt.Sprintf("the cue %T", c.msg)
	}
	changes[s.restoredAt] = "the restore"
	for i, k := range tp.keys {
		if fades := k.at + typedFor; i == len(tp.keys)-1 || tp.keys[i+1].at >= fades {
			changes[fades] = "the keycaps fading after " + keyName(k.key)
		}
	}
	return changes
}

func TestNoCheckpointSitsWithinAQuarterSecondOfAKeyOrCue(t *testing.T) {
	for _, name := range demoTapeNames(t) {
		tp, h := heldTape(t, name)
		moments := []time.Duration{tp.lastShownEnd(t)}
		for _, c := range h.checkpoints {
			moments = append(moments, offset(c.at))
		}
		changes := changesOf(t, tp)
		window, tight := tightCheckpoints[name]
		from, to := offset(window.from), offset(window.to)
		_, fromChange := changes[from]
		_, toChange := changes[to]
		if tight && (!fromChange || !toChange) {
			t.Errorf("demo/%s.tape's window, %s to %s, isn't between two of its changes", name, from, to)
		}
		var near, outside []string
		for _, m := range moments {
			for at, what := range changes {
				if math.Abs(float64(m-at)) >= float64(quarterSecond) {
					continue
				}
				said := fmt.Sprintf("%s is checked within a quarter second of %s, at %s", m, what, at)
				if tight && (at <= from || at >= to) {
					outside = append(outside, said)
				} else {
					near = append(near, said)
				}
			}
		}
		slices.Sort(near)
		if len(near) > 0 {
			t.Errorf("demo/%s.tape: %s", name, strings.Join(near, "; "))
		}
		if tight && len(outside) == 0 {
			t.Errorf("demo/%s.tape is checked a quarter second from every key and cue: delete it from tightCheckpoints", name)
		}
	}
}

// A tight checkpoint's art is the moment its last Show ends, inside its window,
// and the screen is the same throughout the window, so the art is the same
// wherever in it a recording lands.
func TestATightCheckpointShowsTheSameThroughItsWindow(t *testing.T) {
	for name, window := range tightCheckpoints {
		t.Run(name, func(t *testing.T) {
			tp := readTape(t, name)
			from, to, ends := offset(window.from), offset(window.to), tp.lastShownEnd(t)
			if ends < from || ends >= to {
				t.Fatalf("its art is at %s, outside its window, %s to %s", ends, from, to)
			}
			synctest.Test(t, func(t *testing.T) {
				var screens []string
				playedTape(t, tp, []time.Duration{from, ends, to - time.Millisecond}, func(_ time.Duration, p *player) {
					screens = append(screens, p.screen())
				}, nil)
				if len(slices.Compact(slices.Clone(screens))) != 1 {
					t.Errorf("through its window, from %s to %s, its screen changes:\n%s", from, to, strings.Join(screens, "\n"))
				}
			})
		})
	}
}

// A tape quits its program once it's done recording, with keys pressed while
// hidden, so no frame of it shows a quit, and nothing it types after reaches
// the shell.
func TestEachTapeQuitsWhileHidden(t *testing.T) {
	for _, name := range demoTapeNames(t) {
		t.Run(name, func(t *testing.T) {
			tp := readTape(t, name)
			tail := tp.tail()
			if len(tail) == 0 {
				t.Fatal("it presses no key once it's done recording, so its program never quits")
			}
			synctest.Test(t, func(t *testing.T) {
				var quitAt *tapeKey
				playedTape(t, tp, nil, nil, func(k tapeKey, quitBefore bool, p *player) {
					switch {
					case quitBefore && quitAt == nil:
						t.Errorf("its program quits by itself before %s, at %s", keyName(k.key), k.at)
					case quitBefore:
						t.Errorf("it presses %s at %s, after its program quit at %s, so the shell gets it", keyName(k.key), k.at, quitAt.at)
					case p.quit:
						quitAt = &k
					}
				})
				switch {
				case quitAt == nil:
					t.Error("its program never quits")
				case !quitAt.hidden || !slices.Contains(tail, *quitAt):
					t.Errorf("its program quits at %s, on %s, while it's recording", quitAt.at, keyName(quitAt.key))
				}
			})
		})
	}
}
