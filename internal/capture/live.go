package capture

import (
	"image/color"
	"iter"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/tui"
)

// Model is the scenario as Bubble Tea plays it, full screen in a terminal of
// whatever size, in real time from its first frame: its world built, then its
// clock started, reading the scenario's start as it starts; its bootstrap
// reaching the loading page as each cue comes, and its restore landing at its
// moment; the picker's own timers running; and each key shown as it's pressed,
// so a recording shows what was typed.
func (s Scenario) Model() tea.Model {
	return s.begun(time.Now)
}

// begun is the scenario played on clock: its world built first, then its start
// read, so every cue counts from the scenario's first frame.
func (s Scenario) begun(clock func() time.Time) ScenarioModel {
	fx := s.world()
	began := clock()
	elapsed := func() time.Duration { return clock().Sub(began) }
	return ScenarioModel{
		picker:   tui.Build(s.deps(fx, elapsed)),
		now:      clock,
		palettes: append([]theme.Theme{s.theme}, builtinPalettes()...),
		coloured: !s.colourless,
	}
}

// ScenarioModel is the picker playing a scenario, and the keys pressed lately,
// drawn as keycaps in the bottom gutter row, so a recording shows what was
// typed.
type ScenarioModel struct {
	picker tui.Model
	now    func() time.Time
	// palettes are the themes a screen can be drawn in, the scenario's first,
	// then the built-ins the theme panel previews; coloured is whether it's
	// drawn in colour.
	palettes []theme.Theme
	coloured bool
	typed    typed
}

// Picker is the picker as the scenario left it, for what its program's end
// owes the terminal.
func (m ScenarioModel) Picker() tui.Model {
	return m.picker
}

// typedFor is how long a key pressed is shown, from when the last was.
const typedFor = 1500 * time.Millisecond

// mostTyped is how many of the keys pressed lately are shown.
const mostTyped = 4

// typed are the keys pressed lately, as they're shown: the last few, while each
// was pressed within typedFor of the next, and when the last was.
type typed struct {
	keys []string
	last time.Time
}

// typedMsg wakes the model to redraw once the keys typed have had their time.
type typedMsg struct{}

func (m ScenarioModel) Init() tea.Cmd {
	return m.picker.Init()
}

// Update takes in a message as the picker does, noting a key pressed to show.
func (m ScenarioModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var lapse tea.Cmd
	switch msg := msg.(type) {
	case typedMsg:
		return m, nil
	case tea.KeyPressMsg:
		m.typed = m.typed.pressed(keyName(msg), m.now())
		lapse = tea.Tick(typedFor, func(time.Time) tea.Msg { return typedMsg{} })
	}
	next, cmd := m.picker.Update(msg)
	m.picker = next.(tui.Model)
	return m, tea.Batch(cmd, lapse)
}

// View draws the picker, and in its bottom gutter row the keys pressed lately,
// while they're shown. A terminal too short for the gutter shows none, as the
// row is the page's own.
func (m ScenarioModel) View() tea.View {
	v := m.picker.View()
	keys := m.typed.shown(m.now())
	rows := strings.Split(v.Content, "\n")
	if len(keys) == 0 || len(rows) <= 2*tui.Vinset {
		return v
	}
	last := len(rows) - 1
	rows[last] = m.keycaps(rows[last], keys, v.BackgroundColor)
	v.SetContent(strings.Join(rows, "\n"))
	return v
}

// pressed are the keys typed once key is pressed at now: those pressed within
// typedFor of it before, the last few, then it.
func (t typed) pressed(key string, now time.Time) typed {
	keys := []string{key}
	if now.Sub(t.last) < typedFor {
		keys = append(slices.Clone(t.keys), key)
	}
	return typed{keys: keys[max(len(keys)-mostTyped, 0):], last: now}
}

// shown are the keys typed as they're shown at now: none once they've had
// their time.
func (t typed) shown(now time.Time) []string {
	if now.Sub(t.last) >= typedFor {
		return nil
	}
	return t.keys
}

// footerKeyNames are the keys Portal's footer names otherwise than Bubble Tea
// does.
var footerKeyNames = map[string]string{
	"enter":     "⏎",
	"space":     "␣",
	"up":        "↑",
	"down":      "↓",
	"left":      "←",
	"right":     "→",
	"ctrl+up":   "^↑",
	"ctrl+down": "^↓",
	"shift+tab": "shift-tab",
	"home":      "Home",
	"end":       "End",
}

// keyName is a key as Portal's footer names it.
func keyName(key tea.KeyPressMsg) string {
	name := key.String()
	if footer, ok := footerKeyNames[name]; ok {
		return footer
	}
	return name
}

// keycaps draws the latest of keys that fit as keycaps over row, the screen's
// bottom gutter row, a cell apart, their right edge at the gutter's inset: in
// the theme the screen is drawn in, as its canvas says, or where it paints
// none, in reverse. The row keeps its width, and keeps every cell the caps
// don't cover; where not one key fits between the insets, it's left as it is.
func (m ScenarioModel) keycaps(row string, keys []string, canvas color.Color) string {
	capStyle, gapStyle := m.capStyles(canvas)
	width := ansi.StringWidth(row)
	room, end := width-2*tui.Hinset, width-tui.Hinset
	for i := range keys {
		caps := drawCaps(keys[i:], capStyle, gapStyle)
		if capsWidth := ansi.StringWidth(caps); capsWidth <= room {
			return ansi.Truncate(row, end-capsWidth, "") + ansi.ResetStyle + caps + ansi.ResetStyle + ansi.Cut(row, end, width)
		}
	}
	return row
}

// drawCaps draws each key as a keycap, a gap between each.
func drawCaps(keys []string, capStyle, gapStyle lipgloss.Style) string {
	caps := make([]string, len(keys))
	for i, key := range keys {
		caps[i] = capStyle.Render(" " + key + " ")
	}
	return strings.Join(caps, gapStyle.Render(" "))
}

// capStyles are the styles of a keycap and of the gaps between them: bold, the
// canvas on accent.key of the theme shown, the gaps the canvas; or where no
// theme is shown, bold in reverse, the gaps unstyled.
func (m ScenarioModel) capStyles(canvas color.Color) (capStyle, gapStyle lipgloss.Style) {
	th, ok := m.showing(canvas)
	if !ok {
		return lipgloss.NewStyle().Bold(true).Reverse(true), lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().Bold(true).Foreground(th.Canvas.Color()).Background(th.AccentKey.Color()),
		lipgloss.NewStyle().Background(th.Canvas.Color())
}

// showing is the theme the screen is drawn in, as its canvas says: the
// scenario's, or the built-in the theme panel previews, whose canvas it is;
// reporting false for a screen that paints none.
func (m ScenarioModel) showing(canvas color.Color) (theme.Theme, bool) {
	if canvas == nil || !m.coloured {
		return theme.Theme{}, false
	}
	for _, th := range m.palettes {
		if sameColour(th.Canvas.Color(), canvas) {
			return th, true
		}
	}
	return theme.Theme{}, false
}

// sameColour reports whether two colours are the same red, green and blue.
func sameColour(a, b color.Color) bool {
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	return ar == br && ag == bg && ab == bb
}

// builtinPalettes are the built-in themes, in their slugs' order.
func builtinPalettes() []theme.Theme {
	var palettes []theme.Theme
	for _, th := range builtins() {
		palettes = append(palettes, th)
	}
	return palettes
}

// builtins yields each built-in theme by its slug, in the slugs' order, loaded
// through the silent loader, as capturetool renders a theme rather than using
// one.
func builtins() iter.Seq2[string, theme.Theme] {
	return func(yield func(string, theme.Theme) bool) {
		loader := theme.NewSilentLoader()
		for _, slug := range theme.BuiltinSlugs() {
			result, rejection, found := loader.LoadBuiltin(slug)
			if found && rejection == nil && !yield(slug, result.Theme) {
				return
			}
		}
	}
}
