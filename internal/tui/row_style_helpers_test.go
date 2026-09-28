package tui

import (
	"bytes"
	"testing"

	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/tmux"
)

func preRowBg(th theme.Theme, selected, colourless bool) lipgloss.Style {
	if colourless {
		return lipgloss.NewStyle()
	}
	if selected {
		return lipgloss.NewStyle().Background(th.BgSelection.Color())
	}
	return lipgloss.NewStyle().Background(th.Canvas.Color())
}

func preRowToken(base lipgloss.Style, fg theme.Token, th theme.Theme, selected, colourless bool) lipgloss.Style {
	if colourless {
		return base
	}
	styled := base.Foreground(fg.Color())
	if selected {
		return styled.Background(th.BgSelection.Color())
	}
	return styled.Background(th.Canvas.Color())
}

func preLeftBar(th theme.Theme, selected, colourless bool) string {
	bg := preRowBg(th, selected, colourless)
	if selected {
		return preRowToken(lipgloss.Style{}, th.AccentPrimary, th, true, colourless).Render(selectorBar) +
			bg.Render(padTo("", leftBarColumnWidth-lipgloss.Width(selectorBar)))
	}
	return bg.Render(padTo("", leftBarColumnWidth))
}

func TestRowBgStyle_MatchesPreRefactorGolden(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		for _, selected := range []bool{false, true} {
			for _, colourless := range []bool{false, true} {
				want := preRowBg(th, selected, colourless).Render("  ")
				got := rowBgStyle(th, selected, colourless).Render("  ")
				if got != want {
					t.Errorf("rowBgStyle(th=%v sel=%v col=%v) = %q, want %q", themeLabel(th), selected, colourless, got, want)
				}
			}
		}
	}
}

func TestRowTokenStyle_MatchesPreRefactorGolden(t *testing.T) {
	bases := map[string]lipgloss.Style{
		"zero": lipgloss.Style{},
		"bold": lipgloss.NewStyle().Bold(true),
	}
	roles := []string{"text.primary", "accent.primary", "state.positive"}
	for baseName, base := range bases {
		for _, role := range roles {
			for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
				tok := tokenNamed(t, th, role)
				for _, selected := range []bool{false, true} {
					for _, colourless := range []bool{false, true} {
						want := preRowToken(base, tok, th, selected, colourless).Render("name")
						got := rowTokenStyle(base, tok, th, selected, colourless).Render("name")
						if got != want {
							t.Errorf("rowTokenStyle(base=%s tok=%s th=%v sel=%v col=%v) = %q, want %q",
								baseName, tok.Name, themeLabel(th), selected, colourless, got, want)
						}
					}
				}
			}
		}
	}
}

func TestRenderLeftBarColumn_MatchesPreRefactorGolden(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		for _, selected := range []bool{false, true} {
			for _, colourless := range []bool{false, true} {
				bg := rowBgStyle(th, selected, colourless)
				selectorStyle := rowTokenStyle(lipgloss.Style{}, th.AccentPrimary, th, true, colourless)
				want := preLeftBar(th, selected, colourless)
				got := renderLeftBarColumn(bg, selectorStyle, selected)
				if got != want {
					t.Errorf("renderLeftBarColumn(th=%v sel=%v col=%v) = %q, want %q", themeLabel(th), selected, colourless, got, want)
				}
			}
		}
	}
}

func preGlyphColumn(glyph string, glyphStyle, bg lipgloss.Style) string {
	return glyphStyle.Render(glyph) +
		bg.Render(padTo("", leftBarColumnWidth-lipgloss.Width(glyph)))
}

func TestRenderLeftBarGlyphColumn_MatchesPreRefactorGolden(t *testing.T) {
	glyphs := []struct {
		name  string
		glyph string
		role  string
	}{
		{"marker", multiSelectMarker, "accent.primary"},
		{"gone", flashWarningGlyph, "state.destructive"},
		{"selector", selectorBar, "accent.primary"},
	}
	for _, g := range glyphs {
		for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
			for _, selected := range []bool{false, true} {
				for _, colourless := range []bool{false, true} {
					bg := rowBgStyle(th, selected, colourless)
					glyphStyle := rowTokenStyle(lipgloss.Style{}, tokenNamed(t, th, g.role), th, selected, colourless)
					want := preGlyphColumn(g.glyph, glyphStyle, bg)
					got := renderLeftBarGlyphColumn(g.glyph, glyphStyle, bg)
					if got != want {
						t.Errorf("renderLeftBarGlyphColumn(%s th=%v sel=%v col=%v) = %q, want %q",
							g.name, themeLabel(th), selected, colourless, got, want)
					}
				}
			}
		}
	}
}

var sessionRowGoldens = map[theme.Member]map[bool]struct{ sel, uns string }{
	theme.MemberDark: {
		false: {
			sel: "\x1b[48;2;40;36;58m\x1b[m\x1b[38;2;187;154;247;48;2;40;36;58m▌\x1b[m\x1b[48;2;40;36;58m \x1b[m\x1b[1;38;2;255;255;255;48;2;40;36;58malpha\x1b[m\x1b[48;2;40;36;58m                                                         \x1b[m\x1b[48;2;40;36;58m  \x1b[m\x1b[38;2;169;177;214;48;2;40;36;58m3 windows\x1b[m\x1b[48;2;40;36;58m  \x1b[m\x1b[48;2;40;36;58m\x1b[m\x1b[38;2;158;206;106;48;2;40;36;58m●\x1b[m\x1b[48;2;40;36;58m  \x1b[m",
			uns: "\x1b[48;2;11;12;20m\x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[1;38;2;192;202;245;48;2;11;12;20mbravo\x1b[m\x1b[48;2;11;12;20m                                                         \x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[38;2;115;122;162;48;2;11;12;20m1 window\x1b[m\x1b[48;2;11;12;20m   \x1b[m\x1b[48;2;11;12;20m \x1b[m\x1b[48;2;11;12;20m  \x1b[m",
		},
		true: {
			sel: "▌ \x1b[1malpha\x1b[m                                                           3 windows  A  ",
			uns: "  \x1b[1mbravo\x1b[m                                                           1 window      ",
		},
	},
	theme.MemberLight: {
		false: {
			sel: "\x1b[48;2;208;198;240m\x1b[m\x1b[38;2;138;63;209;48;2;208;198;240m▌\x1b[m\x1b[48;2;208;198;240m \x1b[m\x1b[1;38;2;26;27;46;48;2;208;198;240malpha\x1b[m\x1b[48;2;208;198;240m                                                         \x1b[m\x1b[48;2;208;198;240m  \x1b[m\x1b[38;2;63;71;96;48;2;208;198;240m3 windows\x1b[m\x1b[48;2;208;198;240m  \x1b[m\x1b[48;2;208;198;240m\x1b[m\x1b[38;2;59;94;24;48;2;208;198;240m●\x1b[m\x1b[48;2;208;198;240m  \x1b[m",
			uns: "\x1b[48;2;225;226;231m\x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[1;38;2;46;60;100;48;2;225;226;231mbravo\x1b[m\x1b[48;2;225;226;231m                                                         \x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[38;2;88;96;147;48;2;225;226;231m1 window\x1b[m\x1b[48;2;225;226;231m   \x1b[m\x1b[48;2;225;226;231m \x1b[m\x1b[48;2;225;226;231m  \x1b[m",
		},
		true: {
			sel: "▌ \x1b[1malpha\x1b[m                                                           3 windows  A  ",
			uns: "  \x1b[1mbravo\x1b[m                                                           1 window      ",
		},
	},
}

var projectRowGoldens = map[theme.Member]map[bool]struct{ sel, uns string }{
	theme.MemberDark: {
		false: {
			sel: "\x1b[38;2;187;154;247;48;2;40;36;58m▌\x1b[m\x1b[48;2;40;36;58m \x1b[m\x1b[1;38;2;255;255;255;48;2;40;36;58mportal\x1b[m\x1b[48;2;40;36;58m                                                                        \x1b[m\n\x1b[38;2;187;154;247;48;2;40;36;58m▌\x1b[m\x1b[48;2;40;36;58m \x1b[m\x1b[38;2;130;139;184;48;2;40;36;58m/home/user/code/portal\x1b[m\x1b[48;2;40;36;58m                                                        \x1b[m",
			uns: "\x1b[48;2;11;12;20m  \x1b[m\x1b[1;38;2;192;202;245;48;2;11;12;20mother\x1b[m\x1b[48;2;11;12;20m                                                                         \x1b[m\n\x1b[48;2;11;12;20m  \x1b[m\x1b[38;2;115;122;162;48;2;11;12;20m/home/user/code/other\x1b[m\x1b[48;2;11;12;20m                                                         \x1b[m",
		},
		true: {
			sel: "▌ \x1b[1mportal\x1b[m                                                                        \n▌ /home/user/code/portal                                                        ",
			uns: "  \x1b[1mother\x1b[m                                                                         \n  /home/user/code/other                                                         ",
		},
	},
	theme.MemberLight: {
		false: {
			sel: "\x1b[38;2;138;63;209;48;2;208;198;240m▌\x1b[m\x1b[48;2;208;198;240m \x1b[m\x1b[1;38;2;26;27;46;48;2;208;198;240mportal\x1b[m\x1b[48;2;208;198;240m                                                                        \x1b[m\n\x1b[38;2;138;63;209;48;2;208;198;240m▌\x1b[m\x1b[48;2;208;198;240m \x1b[m\x1b[38;2;76;84;120;48;2;208;198;240m/home/user/code/portal\x1b[m\x1b[48;2;208;198;240m                                                        \x1b[m",
			uns: "\x1b[48;2;225;226;231m  \x1b[m\x1b[1;38;2;46;60;100;48;2;225;226;231mother\x1b[m\x1b[48;2;225;226;231m                                                                         \x1b[m\n\x1b[48;2;225;226;231m  \x1b[m\x1b[38;2;88;96;147;48;2;225;226;231m/home/user/code/other\x1b[m\x1b[48;2;225;226;231m                                                         \x1b[m",
		},
		true: {
			sel: "▌ \x1b[1mportal\x1b[m                                                                        \n▌ /home/user/code/portal                                                        ",
			uns: "  \x1b[1mother\x1b[m                                                                         \n  /home/user/code/other                                                         ",
		},
	},
}

func renderSessionRowSnapshot(d SessionDelegate, width int, items []list.Item, index, selIndex int) string {
	m := list.New(items, d, width, 10)
	m.Select(selIndex)
	var buf bytes.Buffer
	d.Render(&buf, m, index, items[index])
	return buf.String()
}

func renderProjectRowSnapshot(d ProjectDelegate, width int, items []list.Item, index, selIndex int) string {
	m := list.New(items, d, width, 10)
	m.Select(selIndex)
	var buf bytes.Buffer
	d.Render(&buf, m, index, items[index])
	return buf.String()
}

func TestRenderSessionRow_ByteIdenticalAcrossRefactor(t *testing.T) {
	const w = 80
	items := flatItems(
		tmux.Session{Name: "alpha", Windows: 3, Attached: true},
		tmux.Session{Name: "bravo", Windows: 1, Attached: false},
	)
	for _, appearance := range []theme.Member{theme.MemberDark, theme.MemberLight} {
		for _, colourless := range []bool{false, true} {
			d := SessionDelegate{Theme: themeForAppearance(t, appearance), Colourless: colourless}
			golden := sessionRowGoldens[appearance][colourless]

			if got := renderSessionRowSnapshot(d, w, items, 0, 0); got != golden.sel {
				t.Errorf("[%v col=%v] selected session row drifted from pre-refactor golden\n got: %q\nwant: %q",
					appearance, colourless, got, golden.sel)
			}
			if got := renderSessionRowSnapshot(d, w, items, 1, 0); got != golden.uns {
				t.Errorf("[%v col=%v] unselected session row drifted from pre-refactor golden\n got: %q\nwant: %q",
					appearance, colourless, got, golden.uns)
			}
		}
	}
}

func TestRenderRowLine_ByteIdenticalAcrossRefactor(t *testing.T) {
	const w = 80
	items := projectItems(
		project.Project{Name: "portal", Path: "/home/user/code/portal"},
		project.Project{Name: "other", Path: "/home/user/code/other"},
	)
	for _, appearance := range []theme.Member{theme.MemberDark, theme.MemberLight} {
		for _, colourless := range []bool{false, true} {
			d := ProjectDelegate{Theme: themeForAppearance(t, appearance), Colourless: colourless}
			golden := projectRowGoldens[appearance][colourless]

			if got := renderProjectRowSnapshot(d, w, items, 0, 0); got != golden.sel {
				t.Errorf("[%v col=%v] selected project row drifted from pre-refactor golden\n got: %q\nwant: %q",
					appearance, colourless, got, golden.sel)
			}
			if got := renderProjectRowSnapshot(d, w, items, 1, 0); got != golden.uns {
				t.Errorf("[%v col=%v] unselected project row drifted from pre-refactor golden\n got: %q\nwant: %q",
					appearance, colourless, got, golden.uns)
			}
		}
	}
}
