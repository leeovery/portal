package capture

import (
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/tui"
)

// Scenario is a stretch of the picker's life, scripted to play in real time, as
// capturetool's --scenario plays it for the README's demos: a fixture's world;
// the bootstrap a cold start streams to the loading page, each step at its
// offset from the start; and the moment its restore lands, from which the
// world's sessions are listed; on a terminal of a size, in a theme. It plays the
// same way every time: what it shows is set by its cues and the keys pressed,
// and by nothing else.
type Scenario struct {
	// Name is what capturetool's --scenario calls it.
	Name string
	// Size is the terminal it's made for.
	Size Size
	// world is the fixture it plays over: its sessions, their pending set, the
	// projects and the mode the picker opens in.
	world func() *Fixture
	// boot is the bootstrap streamed to the loading page, ending in its
	// complete; none opens the picker straight on its sessions.
	boot []bootCue
	// restoredAt is when the restore lands: the world's sessions and pending
	// set are listed from then on, and none before.
	restoredAt time.Duration
	// histories are the sessions' scrollback, by name, as the preview reads it.
	histories map[string]string
	// themes, where set, is the theme panel's source and the keys it opens on,
	// for the palette the scenario is drawn in; the world's otherwise.
	themes func(palette theme.Theme) (tui.ThemeSource, theme.RawKeys)
	// theme is what it's drawn in: the frames' theme unless InTheme says
	// otherwise.
	theme theme.Theme
	// colourless draws it without colour, as NO_COLOR asks.
	colourless bool
}

// InTheme is the scenario drawn in t.
func (s Scenario) InTheme(t theme.Theme) Scenario {
	s.theme = t
	return s
}

// WithoutColour is the scenario drawn without colour, as NO_COLOR asks.
func (s Scenario) WithoutColour() Scenario {
	s.colourless = true
	return s
}

// ScenarioNames lists every scenario's name, sorted, building no world.
func ScenarioNames() []string {
	var names []string
	for _, s := range scenarios() {
		names = append(names, s.Name)
	}
	slices.Sort(names)
	return names
}

// ScenarioNamed returns the scenario with the given name, drawn in the frames'
// theme. An empty or unknown name is an error that lists the scenarios.
func ScenarioNamed(name string) (Scenario, error) {
	if name == "" {
		return Scenario{}, fmt.Errorf("name a scenario (available: %s)", strings.Join(ScenarioNames(), ", "))
	}
	all := scenarios()
	i := slices.IndexFunc(all, func(s Scenario) bool { return s.Name == name })
	if i < 0 {
		return Scenario{}, fmt.Errorf("unknown scenario %q (available: %s)", name, strings.Join(ScenarioNames(), ", "))
	}
	th, err := FramesTheme()
	if err != nil {
		return Scenario{}, err
	}
	return all[i].InTheme(th), nil
}

// deps are the seams the picker plays the scenario over, the world's fixture's
// but for what the scenario times, elapsed saying how long it has played: its
// listings either side of the restore, its bootstrap, its scrollback and any
// theme source of its own. The world's other seams stay as a fixture leaves
// them, so nothing a scenario does reaches a terminal, a session or a file.
func (s Scenario) deps(fx *Fixture, elapsed func() time.Duration) tui.Deps {
	deps := fx.Deps(s.theme)
	deps.Lister = timedLister{elapsed: elapsed, restoredAt: s.restoredAt, sessions: fx.Lister.sessions}
	if deps.PendingReader != nil {
		deps.PendingReader = timedPendingReader{elapsed: elapsed, restoredAt: s.restoredAt, sessions: fx.pendingSessions}
	}
	deps.History = fakeHistory{content: fx.history, histories: s.histories}
	if len(s.boot) > 0 {
		deps.ProgressReceiver = (&bootReceiver{elapsed: elapsed, cues: slices.Clone(s.boot)}).receive
	}
	if s.themes != nil {
		deps.ThemeSource, deps.ThemeKeys = s.themes(s.theme)
	}
	deps.NoColor = deps.NoColor || s.colourless
	return deps
}

// bootCue is an event of the bootstrap reaching the loading page at an offset
// from the scenario's start.
type bootCue struct {
	at  time.Duration
	msg tea.Msg
}

// offset is a cue's offset, given in seconds from the scenario's start.
func offset(seconds float64) time.Duration {
	return time.Duration(seconds * float64(time.Second))
}

// The bootstrap's steps, numbered as cmd/bootstrap numbers them and the loading
// page keys its labels by: checking zmx; removing the latch, the sweep and
// ensuring the saver, which it shows as starting the saver; the restore; and
// writing the latch.
const (
	checkZmxStep   = 1
	firstSaverStep = 2
	lastSaverStep  = 4
	restoreStep    = 5
	latchStep      = 6
)

// stepDone is the bootstrap's step given done, the given seconds in.
func stepDone(seconds float64, step int) bootCue {
	return bootCue{at: offset(seconds), msg: tui.BootstrapProgressMsg{Index: step}}
}

// checks is zmx checked, the given seconds in.
func checks(seconds float64) bootCue {
	return stepDone(seconds, checkZmxStep)
}

// saverStep is one of the steps the loading page shows as starting the saver
// done, the given seconds in. A step outside them panics, as a scenario that
// can't be.
func saverStep(seconds float64, step int) bootCue {
	if step < firstSaverStep || step > lastSaverStep {
		panic(fmt.Sprintf("capture: step %d is none of the saver's, %d to %d", step, firstSaverStep, lastSaverStep))
	}
	return stepDone(seconds, step)
}

// restores counts m sessions restored, the first the given seconds in and each
// next every seconds after.
func restores(from, every float64, m int) []bootCue {
	cues := make([]bootCue, m)
	for i := range cues {
		cues[i] = bootCue{
			at:  offset(from) + time.Duration(i)*offset(every),
			msg: tui.BootstrapProgressMsg{Index: restoreStep, RestoreN: i + 1, RestoreM: m},
		}
	}
	return cues
}

// restoreDone is the restore step done, the given seconds in.
func restoreDone(seconds float64) bootCue {
	return stepDone(seconds, restoreStep)
}

// latchWritten is the bootstrap's last step done, the given seconds in.
func latchWritten(seconds float64) bootCue {
	return stepDone(seconds, latchStep)
}

// completes is the bootstrap complete, the given seconds in, with no warning.
func completes(seconds float64) bootCue {
	return bootCue{at: offset(seconds), msg: tui.BootstrapCompleteMsg{}}
}
