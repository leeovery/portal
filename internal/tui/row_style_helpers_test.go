package tui

import (
	"bytes"
	"testing"

	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui/theme"
)

// Task 6-3 consolidation gate. rowBgStyle / rowTokenStyle / renderLeftBarColumn
// are the shared free functions the Session and Project delegates now route
// through. These tests pin the free functions' output AND prove the two
// delegates render byte-identically to the PRE-refactor render — the row
// goldens below were CAPTURED from the original inline-logic source (a
// throwaway generator run against the pre-refactor delegate, see the task
// notes), so a drift in either delegate's style composition, selector glyph,
// column width, or pad arithmetic is caught byte-for-byte.
//
// No t.Parallel() — the shared canvas helpers make parallelism unsafe.

// preRowBg reproduces the ORIGINAL SessionDelegate.rowBg / ProjectDelegate.rowBg
// inline logic verbatim — the golden the refactor must preserve.
func preRowBg(mode theme.Mode, selected, colourless bool) lipgloss.Style {
	if colourless {
		return lipgloss.NewStyle()
	}
	if selected {
		return lipgloss.NewStyle().Background(theme.MV.BgSelection.ColorFor(mode))
	}
	return lipgloss.NewStyle().Background(theme.MV.Canvas.ColorFor(mode))
}

// preRowToken reproduces the ORIGINAL rowToken inline logic verbatim — the
// golden the refactor must preserve.
func preRowToken(base lipgloss.Style, fg theme.Token, mode theme.Mode, selected, colourless bool) lipgloss.Style {
	if colourless {
		return base
	}
	styled := base.Foreground(fg.ColorFor(mode))
	if selected {
		return styled.Background(theme.MV.BgSelection.ColorFor(mode))
	}
	return styled.Background(theme.MV.Canvas.ColorFor(mode))
}

// preLeftBar reproduces the ORIGINAL §3.3 left-bar column block verbatim (the
// duplicated renderSessionRow / renderRowLine logic) — the golden the
// renderLeftBarColumn helper must preserve.
func preLeftBar(mode theme.Mode, selected, colourless bool) string {
	bg := preRowBg(mode, selected, colourless)
	if selected {
		return preRowToken(lipgloss.Style{}, theme.MV.AccentViolet, mode, true, colourless).Render(selectorBar) +
			bg.Render(padTo("", leftBarColumnWidth-lipgloss.Width(selectorBar)))
	}
	return bg.Render(padTo("", leftBarColumnWidth))
}

// TestRowBgStyle_MatchesPreRefactorGolden asserts rowBgStyle renders a probe
// string byte-identically to the original inline rowBg logic across
// selected/unselected × both modes × colourless true/false.
func TestRowBgStyle_MatchesPreRefactorGolden(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		for _, selected := range []bool{false, true} {
			for _, colourless := range []bool{false, true} {
				want := preRowBg(mode, selected, colourless).Render("  ")
				got := rowBgStyle(mode, selected, colourless).Render("  ")
				if got != want {
					t.Errorf("rowBgStyle(mode=%v sel=%v col=%v) = %q, want %q", mode, selected, colourless, got, want)
				}
			}
		}
	}
}

// TestRowTokenStyle_MatchesPreRefactorGolden asserts rowTokenStyle renders a
// probe string byte-identically to the original inline rowToken logic across a
// representative base/token × selected/unselected × both modes × colourless
// true/false.
func TestRowTokenStyle_MatchesPreRefactorGolden(t *testing.T) {
	bases := map[string]lipgloss.Style{
		"zero": lipgloss.Style{},
		"bold": lipgloss.NewStyle().Bold(true),
	}
	tokens := []theme.Token{theme.MV.TextPrimary, theme.MV.AccentViolet, theme.MV.StateGreen}
	for baseName, base := range bases {
		for _, tok := range tokens {
			for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
				for _, selected := range []bool{false, true} {
					for _, colourless := range []bool{false, true} {
						want := preRowToken(base, tok, mode, selected, colourless).Render("name")
						got := rowTokenStyle(base, tok, mode, selected, colourless).Render("name")
						if got != want {
							t.Errorf("rowTokenStyle(base=%s tok=%s mode=%v sel=%v col=%v) = %q, want %q",
								baseName, tok.Name, mode, selected, colourless, got, want)
						}
					}
				}
			}
		}
	}
}

// TestRenderLeftBarColumn_MatchesPreRefactorGolden asserts renderLeftBarColumn
// reproduces the original §3.3 left-bar block byte-for-byte for the selected
// (selector bar + padding) and unselected (full-width padding) cases, across
// both modes and colourless true/false. The bg + selectorStyle are passed the
// same way both call sites obtain them.
func TestRenderLeftBarColumn_MatchesPreRefactorGolden(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		for _, selected := range []bool{false, true} {
			for _, colourless := range []bool{false, true} {
				bg := rowBgStyle(mode, selected, colourless)
				selectorStyle := rowTokenStyle(lipgloss.Style{}, theme.MV.AccentViolet, mode, true, colourless)
				want := preLeftBar(mode, selected, colourless)
				got := renderLeftBarColumn(bg, selectorStyle, selected)
				if got != want {
					t.Errorf("renderLeftBarColumn(mode=%v sel=%v col=%v) = %q, want %q", mode, selected, colourless, got, want)
				}
			}
		}
	}
}

// preGlyphColumn reproduces the ORIGINAL glyph-column block verbatim — the
// byte-identical shape shared by renderLeftBarColumn's selected branch (▌),
// renderMarkedLeftBarColumn (●), and renderGoneLeftBarColumn (⚠). It is the
// golden the extracted renderLeftBarGlyphColumn helper must preserve.
func preGlyphColumn(glyph string, glyphStyle, bg lipgloss.Style) string {
	return glyphStyle.Render(glyph) +
		bg.Render(padTo("", leftBarColumnWidth-lipgloss.Width(glyph)))
}

// TestRenderLeftBarGlyphColumn_MatchesPreRefactorGolden asserts the extracted
// renderLeftBarGlyphColumn helper reproduces the original glyph-column block
// byte-for-byte for each of the three glyphs that fold into it — the ● marker,
// the ⚠ gone flag, and the ▌ selector — across both modes, selected/unselected,
// and colourless true/false, for representative role tokens. The fixed 2-cell
// leftBarColumnWidth geometry (glyph at col 0 + correct pad) is what keeps the
// name's left edge fixed regardless of which glyph occupies col 0.
func TestRenderLeftBarGlyphColumn_MatchesPreRefactorGolden(t *testing.T) {
	glyphs := []struct {
		name  string
		glyph string
		tok   theme.Token
	}{
		{"marker", multiSelectMarker, theme.MV.AccentViolet},
		{"gone", flashWarningGlyph, theme.MV.StateRed},
		{"selector", selectorBar, theme.MV.AccentViolet},
	}
	for _, g := range glyphs {
		for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
			for _, selected := range []bool{false, true} {
				for _, colourless := range []bool{false, true} {
					bg := rowBgStyle(mode, selected, colourless)
					glyphStyle := rowTokenStyle(lipgloss.Style{}, g.tok, mode, selected, colourless)
					want := preGlyphColumn(g.glyph, glyphStyle, bg)
					got := renderLeftBarGlyphColumn(g.glyph, glyphStyle, bg)
					if got != want {
						t.Errorf("renderLeftBarGlyphColumn(%s mode=%v sel=%v col=%v) = %q, want %q",
							g.name, mode, selected, colourless, got, want)
					}
				}
			}
		}
	}
}

// sessionRowGoldens / projectRowGoldens are the EXACT bytes the PRE-refactor
// delegates emitted for a selected (cursor on row 0) and an unselected (cursor
// on row 0, render row 1) row at width 80, captured from a throwaway generator
// run against the original inline-logic source. Keyed by [mode][colourless].
// The post-refactor render MUST reproduce these byte-for-byte.
//
// Dark and Light coincide under colourless (no colour SGR is emitted), which is
// itself a property worth pinning.
var sessionRowGoldens = map[theme.Mode]map[bool]struct{ sel, uns string }{
	theme.Dark: {
		false: {
			sel: "\x1b[48;2;40;36;58m\x1b[m\x1b[38;2;187;154;247;48;2;40;36;58m▌\x1b[m\x1b[48;2;40;36;58m \x1b[m\x1b[1;38;2;255;255;255;48;2;40;36;58malpha\x1b[m\x1b[48;2;40;36;58m                                                \x1b[m\x1b[48;2;40;36;58m  \x1b[m\x1b[38;2;169;177;214;48;2;40;36;58m3 windows\x1b[m\x1b[48;2;40;36;58m  \x1b[m\x1b[38;2;158;206;106;48;2;40;36;58m● attached\x1b[m\x1b[48;2;40;36;58m\x1b[m\x1b[48;2;40;36;58m  \x1b[m",
			uns: "\x1b[48;2;11;12;20m\x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[1;38;2;192;202;245;48;2;11;12;20mbravo\x1b[m\x1b[48;2;11;12;20m                                                \x1b[m\x1b[48;2;11;12;20m  \x1b[m\x1b[38;2;115;122;162;48;2;11;12;20m1 window\x1b[m\x1b[48;2;11;12;20m   \x1b[m\x1b[48;2;11;12;20m          \x1b[m\x1b[48;2;11;12;20m  \x1b[m",
		},
		true: {
			sel: "▌ \x1b[1malpha\x1b[m                                                  3 windows  ● attached  ",
			uns: "  \x1b[1mbravo\x1b[m                                                  1 window               ",
		},
	},
	theme.Light: {
		false: {
			sel: "\x1b[48;2;208;198;240m\x1b[m\x1b[38;2;138;63;209;48;2;208;198;240m▌\x1b[m\x1b[48;2;208;198;240m \x1b[m\x1b[1;38;2;26;27;46;48;2;208;198;240malpha\x1b[m\x1b[48;2;208;198;240m                                                \x1b[m\x1b[48;2;208;198;240m  \x1b[m\x1b[38;2;63;71;96;48;2;208;198;240m3 windows\x1b[m\x1b[48;2;208;198;240m  \x1b[m\x1b[38;2;59;94;24;48;2;208;198;240m● attached\x1b[m\x1b[48;2;208;198;240m\x1b[m\x1b[48;2;208;198;240m  \x1b[m",
			uns: "\x1b[48;2;225;226;231m\x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[1;38;2;46;60;100;48;2;225;226;231mbravo\x1b[m\x1b[48;2;225;226;231m                                                \x1b[m\x1b[48;2;225;226;231m  \x1b[m\x1b[38;2;88;96;147;48;2;225;226;231m1 window\x1b[m\x1b[48;2;225;226;231m   \x1b[m\x1b[48;2;225;226;231m          \x1b[m\x1b[48;2;225;226;231m  \x1b[m",
		},
		true: {
			sel: "▌ \x1b[1malpha\x1b[m                                                  3 windows  ● attached  ",
			uns: "  \x1b[1mbravo\x1b[m                                                  1 window               ",
		},
	},
}

var projectRowGoldens = map[theme.Mode]map[bool]struct{ sel, uns string }{
	theme.Dark: {
		false: {
			sel: "\x1b[38;2;187;154;247;48;2;40;36;58m▌\x1b[m\x1b[48;2;40;36;58m \x1b[m\x1b[1;38;2;255;255;255;48;2;40;36;58mportal\x1b[m\x1b[48;2;40;36;58m                                                                        \x1b[m\n\x1b[38;2;187;154;247;48;2;40;36;58m▌\x1b[m\x1b[48;2;40;36;58m \x1b[m\x1b[38;2;130;139;184;48;2;40;36;58m/home/user/code/portal\x1b[m\x1b[48;2;40;36;58m                                                        \x1b[m",
			uns: "\x1b[48;2;11;12;20m  \x1b[m\x1b[1;38;2;192;202;245;48;2;11;12;20mother\x1b[m\x1b[48;2;11;12;20m                                                                         \x1b[m\n\x1b[48;2;11;12;20m  \x1b[m\x1b[38;2;115;122;162;48;2;11;12;20m/home/user/code/other\x1b[m\x1b[48;2;11;12;20m                                                         \x1b[m",
		},
		true: {
			sel: "▌ \x1b[1mportal\x1b[m                                                                        \n▌ /home/user/code/portal                                                        ",
			uns: "  \x1b[1mother\x1b[m                                                                         \n  /home/user/code/other                                                         ",
		},
	},
	theme.Light: {
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

// TestRenderSessionRow_ByteIdenticalAcrossRefactor asserts the post-refactor
// renderSessionRow output is byte-for-byte identical to the PRE-refactor goldens
// for a selected and an unselected flat row, both modes, colourless on/off.
func TestRenderSessionRow_ByteIdenticalAcrossRefactor(t *testing.T) {
	const w = 80
	items := flatItems(
		tmux.Session{Name: "alpha", Windows: 3, Attached: true},
		tmux.Session{Name: "bravo", Windows: 1, Attached: false},
	)
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		for _, colourless := range []bool{false, true} {
			d := SessionDelegate{Mode: mode, Colourless: colourless}
			golden := sessionRowGoldens[mode][colourless]

			if got := renderSessionRowSnapshot(d, w, items, 0, 0); got != golden.sel {
				t.Errorf("[%v col=%v] selected session row drifted from pre-refactor golden\n got: %q\nwant: %q",
					mode, colourless, got, golden.sel)
			}
			if got := renderSessionRowSnapshot(d, w, items, 1, 0); got != golden.uns {
				t.Errorf("[%v col=%v] unselected session row drifted from pre-refactor golden\n got: %q\nwant: %q",
					mode, colourless, got, golden.uns)
			}
		}
	}
}

// TestRenderRowLine_ByteIdenticalAcrossRefactor asserts the post-refactor
// project Render (which composes two renderRowLine calls) output is
// byte-for-byte identical to the PRE-refactor goldens for a selected and an
// unselected row, both modes, colourless on/off.
func TestRenderRowLine_ByteIdenticalAcrossRefactor(t *testing.T) {
	const w = 80
	items := projectItems(
		project.Project{Name: "portal", Path: "/home/user/code/portal"},
		project.Project{Name: "other", Path: "/home/user/code/other"},
	)
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		for _, colourless := range []bool{false, true} {
			d := ProjectDelegate{Mode: mode, Colourless: colourless}
			golden := projectRowGoldens[mode][colourless]

			if got := renderProjectRowSnapshot(d, w, items, 0, 0); got != golden.sel {
				t.Errorf("[%v col=%v] selected project row drifted from pre-refactor golden\n got: %q\nwant: %q",
					mode, colourless, got, golden.sel)
			}
			if got := renderProjectRowSnapshot(d, w, items, 1, 0); got != golden.uns {
				t.Errorf("[%v col=%v] unselected project row drifted from pre-refactor golden\n got: %q\nwant: %q",
					mode, colourless, got, golden.uns)
			}
		}
	}
}
