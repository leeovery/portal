package tui

import (
	"bytes"
	"testing"

	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/tmux"
)

func preCanvasBg(th theme.Theme, colourless bool) lipgloss.Style {
	if colourless {
		return lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().Background(th.Canvas.Color())
}

func preTokenStyle(base lipgloss.Style, fg theme.Token, th theme.Theme, colourless bool) lipgloss.Style {
	if colourless {
		return base
	}
	return base.
		Foreground(fg.Color()).
		Background(th.Canvas.Color())
}

func TestSessionCanvasBg_DelegatesToHeaderCanvasBg(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		for _, colourless := range []bool{false, true} {
			d := SessionDelegate{Theme: th, Colourless: colourless}
			want := headerCanvasBg(th, colourless).Render("  ")
			if got := d.canvasBg().Render("  "); got != want {
				t.Errorf("canvasBg(th=%v col=%v) = %q, want headerCanvasBg %q",
					themeLabel(th), colourless, got, want)
			}
			pre := preCanvasBg(th, colourless).Render("  ")
			if want != pre {
				t.Errorf("headerCanvasBg(th=%v col=%v) = %q drifts from pre-refactor canvasBg %q",
					themeLabel(th), colourless, want, pre)
			}
		}
	}
}

func TestSessionTokenStyle_DelegatesToHeaderStyle(t *testing.T) {
	roles := []string{"text.muted", "text.subtle", "text.primary", "state.positive"}
	bases := map[string]lipgloss.Style{
		"empty": lipgloss.Style{},
		"bold":  lipgloss.NewStyle().Bold(true),
	}
	for baseName, base := range bases {
		for _, role := range roles {
			for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
				tok := tokenNamed(t, th, role)
				for _, colourless := range []bool{false, true} {
					d := SessionDelegate{Theme: th, Colourless: colourless}
					want := headerStyle(tok, th, colourless).Inherit(base).Render("probe")
					if got := d.tokenStyle(base, tok).Render("probe"); got != want {
						t.Errorf("tokenStyle(base=%s tok=%s th=%v col=%v) = %q, want headerStyle.Inherit %q",
							baseName, tok.Name, themeLabel(th), colourless, got, want)
					}
					pre := preTokenStyle(base, tok, th, colourless).Render("probe")
					if want != pre {
						t.Errorf("headerStyle(base=%s tok=%s th=%v col=%v) = %q drifts from pre-refactor tokenStyle %q",
							baseName, tok.Name, themeLabel(th), colourless, want, pre)
					}
				}
			}
		}
	}
}

func TestSessionDelegateRender_ByteIdenticalAcrossConsolidation(t *testing.T) {
	const w = 80
	modeNames := map[theme.Theme]string{testDarkTheme(t): "dark", testLightTheme(t): "light"}

	header := HeaderItem{Heading: "Portal", Count: 2, Key: "/p/portal"}
	sessions := []list.Item{
		SessionItem{Session: tmux.Session{Name: "dev", Windows: 3, Attached: true}, GroupKey: "/p/portal", GroupHeading: "Portal"},
		SessionItem{Session: tmux.Session{Name: "work", Windows: 1, Attached: false}, GroupKey: "/p/portal", GroupHeading: "Portal"},
	}

	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		for _, colourless := range []bool{false, true} {
			d := SessionDelegate{Theme: th, Colourless: colourless}
			modeName := modeNames[th]

			hm := list.New([]list.Item{header}, d, w, 10)
			var hb bytes.Buffer
			d.Render(&hb, hm, 0, header)
			assertSessionGolden(t, "header", modeName, colourless, hb.String())

			sm := list.New(sessions, d, w, 10)
			var sel, unsel bytes.Buffer
			d.Render(&sel, sm, 0, sessions[0])
			d.Render(&unsel, sm, 1, sessions[1])
			assertSessionGolden(t, "session-selected", modeName, colourless, sel.String())
			assertSessionGolden(t, "session-unselected", modeName, colourless, unsel.String())
		}
	}
}

func assertSessionGolden(t *testing.T, frame, th string, colourless bool, got string) {
	t.Helper()
	want, ok := sessionStyleGoldens[sessionStyleGoldenKey{frame, th, colourless}]
	if !ok {
		t.Fatalf("no golden for {%s %s col=%v}", frame, th, colourless)
	}
	if got != want {
		t.Errorf("[%s %s col=%v] delegate render drifted from pre-refactor golden\n got: %q\nwant: %q",
			frame, th, colourless, ansi.Strip(got), ansi.Strip(want))
	}
}

type sessionStyleGoldenKey struct {
	frame      string
	th         string
	colourless bool
}

var sessionStyleGoldens = map[sessionStyleGoldenKey]string{
	{"header", "dark", false}:              "\x1b[48;2;11;12;20m  \x1b[m\x1b[38;2;115;122;162;48;2;11;12;20mPortal \x1b[m\x1b[38;2;83;92;134;48;2;11;12;20m··· 2\x1b[m",
	{"session-selected", "dark", false}:    "\x1b[48;2;40;36;58m  \x1b[m\x1b[38;2;187;154;247;48;2;40;36;58m▌\x1b[m\x1b[48;2;40;36;58m \x1b[m\x1b[1;38;2;255;255;255;48;2;40;36;58mdev\x1b[m\x1b[48;2;40;36;58m                                                        \x1b[m\x1b[48;2;40;36;58m  \x1b[m\x1b[38;2;169;177;214;48;2;40;36;58m3 windows\x1b[m\x1b[48;2;40;36;58m  \x1b[m\x1b[48;2;40;36;58m \x1b[m\x1b[38;2;158;206;106;48;2;40;36;58m●\x1b[m\x1b[48;2;40;36;58m  \x1b[m",
	{"session-unselected", "dark", false}:  "\x1b[48;2;11;12;20m  \x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[1;38;2;192;202;245;48;2;11;12;20mwork\x1b[m\x1b[48;2;11;12;20m                                                       \x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[38;2;115;122;162;48;2;11;12;20m1 window\x1b[m\x1b[48;2;11;12;20m   \x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[48;2;11;12;20m  \x1b[m",
	{"header", "dark", true}:               "  Portal ··· 2",
	{"session-selected", "dark", true}:     "  ▌ \x1b[1mdev\x1b[m                                                          3 windows   A  ",
	{"session-unselected", "dark", true}:   "    \x1b[1mwork\x1b[m                                                         1 window       ",
	{"header", "light", false}:             "\x1b[48;2;225;226;231m  \x1b[m\x1b[38;2;88;96;147;48;2;225;226;231mPortal \x1b[m\x1b[38;2;118;125;162;48;2;225;226;231m··· 2\x1b[m",
	{"session-selected", "light", false}:   "\x1b[48;2;208;198;240m  \x1b[m\x1b[38;2;138;63;209;48;2;208;198;240m▌\x1b[m\x1b[48;2;208;198;240m \x1b[m\x1b[1;38;2;26;27;46;48;2;208;198;240mdev\x1b[m\x1b[48;2;208;198;240m                                                        \x1b[m\x1b[48;2;208;198;240m  \x1b[m\x1b[38;2;63;71;96;48;2;208;198;240m3 windows\x1b[m\x1b[48;2;208;198;240m  \x1b[m\x1b[48;2;208;198;240m \x1b[m\x1b[38;2;59;94;24;48;2;208;198;240m●\x1b[m\x1b[48;2;208;198;240m  \x1b[m",
	{"session-unselected", "light", false}: "\x1b[48;2;225;226;231m  \x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[1;38;2;46;60;100;48;2;225;226;231mwork\x1b[m\x1b[48;2;225;226;231m                                                       \x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[38;2;88;96;147;48;2;225;226;231m1 window\x1b[m\x1b[48;2;225;226;231m   \x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[48;2;225;226;231m  \x1b[m",
	{"header", "light", true}:              "  Portal ··· 2",
	{"session-selected", "light", true}:    "  ▌ \x1b[1mdev\x1b[m                                                          3 windows   A  ",
	{"session-unselected", "light", true}:  "    \x1b[1mwork\x1b[m                                                         1 window       ",
}
