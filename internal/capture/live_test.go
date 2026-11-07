package capture

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/tui"
)

// keyShiftTab is shift+tab, whose keycap is the widest the footer names.
var keyShiftTab = tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}

// lastRow is the screen's last row, the bottom gutter, as text alone, its
// trailing blanks kept.
func lastRow(p *player) string {
	rows := p.rows()
	return ansi.Strip(rows[len(rows)-1])
}

func TestKeysAreShownAsTheyrePressedTillTheyveHadTheirTime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := playScenario(t, scenarioNamed(t, "portal-tour"), harnessSize)

		p.press(0, keyDown, keyDown)
		p.press(typedFor-time.Millisecond, keyRune('s'))
		if last := lastRow(p); !strings.HasSuffix(last, " ↓   ↓   s   ") {
			t.Errorf("the last row reads %q, want the keys pressed, in turn, at its right", last)
		}
		p.until(2*typedFor - 2*time.Millisecond)
		if last := lastRow(p); !strings.HasSuffix(last, " s   ") {
			t.Errorf("as s has its time, the last row reads %q, want it still shown", last)
		}
		p.until(2*typedFor - time.Millisecond)
		if last := lastRow(p); strings.TrimSpace(last) != "" {
			t.Errorf("once the keys have had their time, the last row reads %q, want them gone", last)
		}
	})
}

func TestTheLatestKeysPressedAreShown(t *testing.T) {
	var keys typed
	at := time.Date(2026, time.November, 5, 9, 0, 0, 0, time.UTC)
	for _, key := range []string{"1", "2", "3", "4", "5"} {
		keys = keys.pressed(key, at)
		at = at.Add(typedFor / 2)
	}
	if got := keys.shown(at); !slices.Equal(got, []string{"2", "3", "4", "5"}) {
		t.Errorf("having pressed five keys in turn, %q are shown, want the last %d", got, mostTyped)
	}
	if got := keys.pressed("6", at.Add(typedFor)).shown(at.Add(typedFor)); !slices.Equal(got, []string{"6"}) {
		t.Errorf("a key pressed once the others have had their time shows %q, want it alone", got)
	}
}

func TestKeysAreNamedAsTheKeyLineNamesThem(t *testing.T) {
	tests := []struct {
		key  tea.KeyPressMsg
		want string
	}{
		{key: keyEnter(), want: "⏎"},
		{key: keySpace, want: "␣"},
		{key: tea.KeyPressMsg{Code: tea.KeySpace}, want: "␣"},
		{key: keyUp, want: "↑"},
		{key: keyDown, want: "↓"},
		{key: tea.KeyPressMsg{Code: tea.KeyLeft}, want: "←"},
		{key: tea.KeyPressMsg{Code: tea.KeyRight}, want: "→"},
		{key: keyEsc, want: "esc"},
		{key: tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModCtrl}, want: "^↑"},
		{key: keyPageDown(), want: "^↓"},
		{key: keyShiftTab, want: "shift-tab"},
		{key: tea.KeyPressMsg{Code: tea.KeyTab}, want: "tab"},
		{key: tea.KeyPressMsg{Code: tea.KeyHome}, want: "Home"},
		{key: tea.KeyPressMsg{Code: tea.KeyEnd}, want: "End"},
		{key: keyRune('s'), want: "s"},
		{key: keyRune('/'), want: "/"},
		{key: keyRune('?'), want: "?"},
	}
	for _, tt := range tests {
		if got := keyName(tt.key); got != tt.want {
			t.Errorf("keyName(%v) = %q, want %q", tt.key, got, tt.want)
		}
	}
}

// The keycaps are drawn in the bottom gutter row alone, their right edge at the
// gutter's inset, every other cell of the screen the picker's own.
func TestKeycapsSitInTheBottomGutterRightAligned(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := playScenario(t, scenarioNamed(t, "portal-tour"), harnessSize)
		p.press(0, keyRune('s'))

		rows := p.rows()
		picker := strings.Split(p.scenarioModel().Picker().View().Content, "\n")
		if len(rows) != harnessSize.Height || len(picker) != len(rows) {
			t.Fatalf("the screen has %d rows, the picker's %d, want %d", len(rows), len(picker), harnessSize.Height)
		}
		last := len(rows) - 1
		if !slices.Equal(rows[:last], picker[:last]) {
			t.Error("the keycaps drew above the bottom gutter row, want every row above it the picker's")
		}
		if want := strings.Repeat(" ", harnessSize.Width-tui.Hinset-len(" s ")) + " s " + strings.Repeat(" ", tui.Hinset); ansi.Strip(rows[last]) != want {
			t.Errorf("the gutter row reads %q, want the keycap ending at the inset, %d cells from the right", ansi.Strip(rows[last]), tui.Hinset)
		}
		drawn, err := parse(rows[last])
		if err != nil {
			t.Fatalf("parse the gutter row: %v", err)
		}
		own, err := parse(picker[last])
		if err != nil {
			t.Fatalf("parse the picker's gutter row: %v", err)
		}
		end := harnessSize.Width - tui.Hinset
		start := end - len(" s ")
		if !slices.Equal(drawn[0][:start], own[0][:start]) || !slices.Equal(drawn[0][end:], own[0][end:]) {
			t.Errorf("the gutter row is drawn %v, want the picker's own cells either side of the keycap, %v", drawn[0], own[0])
		}
	})
}

// However many keys, and however narrow or short the terminal, the keycaps
// never widen the gutter row, showing the latest keys that fit between its
// insets, or none where not one fits, or where the terminal is too short to
// have a gutter.
func TestKeycapsNeverWidenTheRow(t *testing.T) {
	keys := []tea.KeyPressMsg{keyShiftTab, keyShiftTab, keyShiftTab, keyShiftTab}
	tests := []struct {
		size  Size
		shown int
	}{
		{size: harnessSize, shown: 4},
		{size: Size{Width: 40, Height: 12}, shown: 3},
		{size: Size{Width: 14, Height: 12}, shown: 0},
		{size: Size{Width: 120, Height: 2}, shown: 0},
	}
	for _, tt := range tests {
		t.Run(tt.size.String(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				p := playScenario(t, scenarioNamed(t, "portal-tour"), tt.size)
				p.press(0, keys...)
				rows := p.rows()
				last := rows[len(rows)-1]
				if width := ansi.StringWidth(last); width != tt.size.Width {
					t.Errorf("the last row is %d cells wide, want the terminal's %d", width, tt.size.Width)
				}
				if shown := strings.Count(ansi.Strip(last), "shift-tab"); shown != tt.shown {
					t.Errorf("the last row reads %q, %d keys shown, want %d", ansi.Strip(last), shown, tt.shown)
				}
				picker := strings.Split(p.scenarioModel().Picker().View().Content, "\n")
				if tt.shown == 0 && last != picker[len(picker)-1] {
					t.Errorf("with no key shown, the last row reads %q, want the picker's own", last)
				}
			})
		})
	}
}

// sgr is the colour of a token as an SGR sets it, as a foreground (38) or a
// background (48).
func sgr(ground int, tok theme.Token) string {
	r, g, b, _ := tok.Color().RGBA()
	return fmt.Sprintf("%d;2;%d;%d;%d", ground, r>>8, g>>8, b>>8)
}

func TestKeycapsAreDrawnInTheThemeShown(t *testing.T) {
	tokyo := framesTheme(t)
	synctest.Test(t, func(t *testing.T) {
		p := playScenario(t, scenarioNamed(t, "portal-tour"), harnessSize)
		p.press(0, keyRune('s'))
		rows := p.rows()
		if last := rows[len(rows)-1]; !strings.Contains(last, sgr(48, tokyo.AccentKey)) || !strings.Contains(last, sgr(38, tokyo.Canvas)) {
			t.Errorf("the keycap is drawn %q, want it the canvas on tokyo-night's accent.key, the theme shown", last)
		}
	})
	synctest.Test(t, func(t *testing.T) {
		p := playScenario(t, scenarioNamed(t, "portal-tour").WithoutColour(), harnessSize)
		p.press(0, keyRune('s'))
		rows := p.rows()
		if last := rows[len(rows)-1]; !strings.Contains(last, "\x1b[1;7m s ") || strings.Contains(last, "2;") {
			t.Errorf("without colour, the keycap is drawn %q, want it bold in reverse, and no colour", last)
		}
	})
}

// As the theme panel previews a built-in, the keycaps are drawn in it, the
// theme whose canvas is on screen, not the theme the scenario started in.
func TestKeycapsFollowTheCanvasThePanelPreviews(t *testing.T) {
	horizon, tokyo := themeNamed(t, "horizon"), framesTheme(t)
	synctest.Test(t, func(t *testing.T) {
		p := playScenario(t, scenarioNamed(t, "themes"), harnessSize)
		p.press(offset(1), keyRune('t'))
		p.press(offset(2), keyUp, keyUp, keyUp)
		if slug := canvasSlug(t, p.model.View()); slug != "horizon" {
			t.Fatalf("three rows up the panel, the screen is drawn in %s, want horizon previewed", slug)
		}
		rows := p.rows()
		last := rows[len(rows)-1]
		if !strings.Contains(last, sgr(48, horizon.AccentKey)) || !strings.Contains(last, sgr(38, horizon.Canvas)) || strings.Contains(last, sgr(48, tokyo.AccentKey)) {
			t.Errorf("the keycaps are drawn %q, want them in horizon's accent.key and canvas, the theme previewed", last)
		}
	})
}

// A screen is shown in the theme whose canvas it paints, the scenario's or a
// built-in's; a screen painting none, or drawn without colour, in none.
func TestTheThemeShownIsTheOneWhoseCanvasIsOnScreen(t *testing.T) {
	m := scenarioNamed(t, "portal-tour").begun(time.Now)
	for slug, th := range builtins() {
		if shown, ok := m.showing(th.Canvas.Color()); !ok || shown != th {
			t.Errorf("on %s's canvas, the theme shown is %v (%t), want %s", slug, shown.Canvas, ok, slug)
		}
	}
	lake := themeWithCanvas(framesTheme(t), "#102030")
	own := scenarioNamed(t, "portal-tour").InTheme(lake).begun(time.Now)
	if shown, ok := own.showing(lake.Canvas.Color()); !ok || shown != lake {
		t.Errorf("on the scenario's own canvas, the theme shown is %v (%t), want the scenario's", shown.Canvas, ok)
	}
	if _, ok := m.showing(nil); ok {
		t.Error("a screen painting no canvas is shown in a theme, want none")
	}
	plain := scenarioNamed(t, "portal-tour").WithoutColour().begun(time.Now)
	if _, ok := plain.showing(framesTheme(t).Canvas.Color()); ok {
		t.Error("a scenario drawn without colour is shown in a theme, want none")
	}
}
