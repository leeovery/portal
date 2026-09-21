package tui

import (
	"reflect"
	"strings"
	"testing"
	"unicode"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/theme"
)

const (
	resumeGoldenCommand    = "claude --resume 45604077-a9d0-41bd-9692-b97f63f237fc"
	resumeGoldenReportHead = "the pane is frozen"
	resumeGoldenReport     = resumeGoldenReportHead + " and will not lift"
	resumeCardWidth        = resumeCardContentWidth + 2*panelRowInset + 2
	resumeRoomyWidth       = 80
	resumeRoomyHeight      = 24
)

func resumeScreenOf(r resumePartsRender, command, report string, w, h int) ResumeScreen {
	return ResumeScreen{
		Command:    command,
		Report:     report,
		Width:      w,
		Height:     h,
		Theme:      r.th,
		Colourless: r.colourless,
	}
}

// The card as its own rows, lifted off the pane it was centred on.
func resumeCardRows(t *testing.T, r resumePartsRender, command, report string) []string {
	t.Helper()
	out := RenderResumePanel(resumeScreenOf(r, command, report, resumeRoomyWidth, resumeRoomyHeight))
	return nonEmptyRows(paneRows(t, out, resumeRoomyWidth, resumeRoomyHeight))
}

func resumeGoldenFrameRow(left, right string) string {
	return left + strings.Repeat(panelRuleGlyph, resumeCardWidth-2) + right
}

func resumeGoldenContentRow(content string) string {
	inset := strings.Repeat(" ", panelRowInset)
	pad := strings.Repeat(" ", resumeCardContentWidth-lipgloss.Width(content))
	return panelFrameSide + inset + content + pad + inset + panelFrameSide
}

func resumeGoldenCard(rows ...string) []string {
	card := []string{resumeGoldenFrameRow(panelFrameTopLeft, panelFrameTopRight)}
	for _, row := range rows {
		if row == panelRuleGlyph {
			card = append(card, resumeGoldenFrameRow(panelFrameTeeLeft, panelFrameTeeRight))
			continue
		}
		card = append(card, resumeGoldenContentRow(row))
	}
	return append(card, resumeGoldenFrameRow(panelFrameBottomLeft, panelFrameBottomRight))
}

func resumeGoldenHeader() string {
	gap := resumeCardContentWidth - lipgloss.Width(resumePanelTitle) - lipgloss.Width(resumePausedBadge)
	return resumePanelTitle + strings.Repeat(" ", gap) + resumePausedBadge
}

func resumeGoldenFooter() string {
	return resumeKeyResume + " " + resumeLabelResume + modalFooterGap + resumeKeyDiscard + " " + resumeLabelDiscard
}

func assertRowsEqual(t *testing.T, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the screen reads\n got: %q\nwant: %q", got, want)
	}
}

func TestRenderResumePanel_Card(t *testing.T) {
	forEachResumePartsRender(t, func(t *testing.T, r resumePartsRender) {
		t.Run("it renders the title and the PAUSED badge in the header slot", func(t *testing.T) {
			rows := resumeCardRows(t, r, resumeGoldenCommand, "")
			if len(rows) < 2 {
				t.Fatalf("the card renders %d rows, want at least 2: %q", len(rows), rows)
			}
			if got, want := rows[1], resumeGoldenContentRow(resumeGoldenHeader()); got != want {
				t.Errorf("the header row reads\n got: %q\nwant: %q", got, want)
			}
		})

		t.Run("it carries exactly three parts with nothing to report", func(t *testing.T) {
			rows := resumeCardRows(t, r, resumeGoldenCommand, "")
			assertRowsEqual(t, rows, resumeGoldenCard(
				resumeGoldenHeader(),
				panelRuleGlyph,
				resumeCommandLabel,
				resumeGoldenCommand,
				panelRuleGlyph,
				resumeGoldenFooter(),
			))
		})

		t.Run("it carries the report row between the command and the key hints", func(t *testing.T) {
			rows := resumeCardRows(t, r, resumeGoldenCommand, resumeGoldenReport)
			assertRowsEqual(t, rows, resumeGoldenCard(
				resumeGoldenHeader(),
				panelRuleGlyph,
				resumeCommandLabel,
				resumeGoldenCommand,
				resumeGoldenReport,
				panelRuleGlyph,
				resumeGoldenFooter(),
			))
			if quiet := resumeCardRows(t, r, resumeGoldenCommand, ""); len(rows) != len(quiet)+1 {
				t.Errorf("the report adds %d rows, want exactly 1", len(rows)-len(quiet))
			}
		})

		t.Run("it renders the specified copy verbatim", func(t *testing.T) {
			card := strings.Join(resumeCardRows(t, r, resumeGoldenCommand, ""), "\n")
			for _, want := range []string{"Resume session", "● PAUSED", "ON RESUME", "⏎ resume", "d discard"} {
				if !strings.Contains(card, want) {
					t.Errorf("the card does not read %q:\n%s", want, card)
				}
			}
		})

		t.Run("it renders the title bolded on the card and on the plain stack", func(t *testing.T) {
			for _, tc := range []struct {
				name string
				w, h int
			}{
				{"the card", resumeRoomyWidth, resumeRoomyHeight},
				{"the plain stack", 30, 10},
			} {
				t.Run(tc.name, func(t *testing.T) {
					out := RenderResumePanel(resumeScreenOf(r, resumeGoldenCommand, "", tc.w, tc.h))
					assertEmphasisedToken(t, labelSegment(t, out, resumePanelTitle), r.th.TextPrimary, true, r.colourless)
				})
			}
		})

		t.Run("it renders the same card width whatever the command the report or the badge", func(t *testing.T) {
			for _, tc := range []struct {
				name    string
				command string
				report  string
			}{
				{"one character", "v", ""},
				{"three lines", strings.Repeat(resumePartsLongCommand+" ", 4), ""},
				{"a long report", "vim", strings.Repeat(resumeGoldenReport+" ", 6)},
				{"an empty report", resumeGoldenCommand, ""},
				{"an empty command", "", ""},
			} {
				t.Run(tc.name, func(t *testing.T) {
					for i, row := range resumeCardRows(t, r, tc.command, tc.report) {
						if got := lipgloss.Width(row); got != resumeCardWidth {
							t.Errorf("card row %d renders at width %d, want %d: %q", i, got, resumeCardWidth, row)
						}
					}
				})
			}
		})
	})
}

func TestRenderResumePanel_CardTokens(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		t.Run(themeLabel(th), func(t *testing.T) {
			r := resumePartsRender{themeLabel(th), th, false}
			out := RenderResumePanel(resumeScreenOf(r, resumeGoldenCommand, "", resumeRoomyWidth, resumeRoomyHeight))

			t.Run("it renders the badge in accent.attention", func(t *testing.T) {
				seg := labelSegment(t, out, resumePausedBadge)
				if seq := tokenFgSeq(t, th.AccentAttention); !strings.Contains(seg, seq) {
					t.Errorf("the badge does not render accent.attention %q: %q", seq, seg)
				}
				if seq := tokenFgSeq(t, th.TextPrimary); !strings.Contains(seg, seq) {
					t.Errorf("the title does not render text.primary %q: %q", seq, seg)
				}
			})

			t.Run("it renders the ON RESUME label in accent.primary over the command", func(t *testing.T) {
				label := labelSegment(t, out, resumeCommandLabel)
				if seq := tokenFgSeq(t, th.AccentPrimary); !strings.Contains(label, seq) {
					t.Errorf("the ON RESUME label does not render accent.primary %q: %q", seq, label)
				}
				command := labelSegment(t, out, resumeGoldenCommand)
				if seq := tokenFgSeq(t, th.TextPrimary); !strings.Contains(command, seq) {
					t.Errorf("the command does not render text.primary %q: %q", seq, command)
				}
				lines := strings.Split(out, "\n")
				if labelIdx, commandIdx := indexOfLine(lines, resumeCommandLabel), indexOfLine(lines, resumeGoldenCommand); commandIdx != labelIdx+1 {
					t.Errorf("the command sits at row %d, want directly under the label at row %d", commandIdx, labelIdx)
				}
			})

			t.Run("it renders the resume and discard key hints", func(t *testing.T) {
				footer := labelSegment(t, out, resumeLabelDiscard)
				if seq := tokenFgSeq(t, th.AccentKey); !strings.Contains(footer, seq) {
					t.Errorf("the key glyphs do not render accent.key %q: %q", seq, footer)
				}
				if seq := tokenFgSeq(t, th.TextMuted); !strings.Contains(footer, seq) {
					t.Errorf("the hint labels do not render text.muted %q: %q", seq, footer)
				}
			})
		})
	}
}

func indexOfLine(lines []string, marker string) int {
	for i, line := range lines {
		if strings.Contains(ansi.Strip(line), marker) {
			return i
		}
	}
	return -1
}

func TestRenderResumePanel_PlainStack(t *testing.T) {
	forEachResumePartsRender(t, func(t *testing.T, r resumePartsRender) {
		const (
			narrowWidth = 24
			twoRowCmd   = "claude --resume 4f2c9a1e --cwd /tmp"
		)
		wantCommandRows := []string{"claude --resume 4f2c9a1e", "--cwd /tmp"}

		t.Run("it renders the plain stack below the card's size", func(t *testing.T) {
			out := RenderResumePanel(resumeScreenOf(r, twoRowCmd, resumeGoldenReport, narrowWidth, 10))
			want := append([]string{resumePanelTitle}, wantCommandRows...)
			want = append(want, ansi.Truncate(resumeGoldenReport, narrowWidth, resumeEllipsis), resumeGoldenFooter())
			assertRowsEqual(t, nonEmptyRows(paneRows(t, out, narrowWidth, 10)), want)
		})

		t.Run("it drops the ON RESUME label and the PAUSED badge from the plain stack", func(t *testing.T) {
			out := ansi.Strip(RenderResumePanel(resumeScreenOf(r, twoRowCmd, "", narrowWidth, 10)))
			for _, absent := range []string{resumeCommandLabel, resumePausedBadge, panelFrameTopLeft} {
				if strings.Contains(out, absent) {
					t.Errorf("the plain stack carries %q:\n%s", absent, out)
				}
			}
		})

		t.Run("it renders the title the command and the key hints in a four-row pane", func(t *testing.T) {
			out := RenderResumePanel(resumeScreenOf(r, twoRowCmd, "", narrowWidth, 4))
			want := append([]string{resumePanelTitle}, wantCommandRows...)
			assertRowsEqual(t, nonEmptyRows(paneRows(t, out, narrowWidth, 4)), append(want, resumeGoldenFooter()))
		})

		t.Run("it draws the title the command and the key hints at the smallest pane", func(t *testing.T) {
			for _, tc := range []struct {
				name string
				w, h int
				want []string
			}{
				{"three rows", narrowWidth, 3, []string{resumePanelTitle, wantCommandRows[0], resumeGoldenFooter()}},
				{"two rows", narrowWidth, 2, []string{resumePanelTitle, resumeGoldenFooter()}},
				{"one cell", 1, 1, []string{"R"}},
			} {
				t.Run(tc.name, func(t *testing.T) {
					out := RenderResumePanel(resumeScreenOf(r, twoRowCmd, "", tc.w, tc.h))
					assertRowsEqual(t, nonEmptyRows(paneRows(t, out, tc.w, tc.h)), tc.want)
				})
			}
		})
	})
}

func TestRenderResumePanel_Copy(t *testing.T) {
	const (
		shortCommand = "vim"
		shortReport  = "frozen"
		stackWidth   = 30
	)
	sizes := []struct {
		name string
		w, h int
	}{
		{"the card", resumeRoomyWidth, resumeRoomyHeight},
		{"the plain stack", stackWidth, 10},
	}

	forEachResumePartsRender(t, func(t *testing.T, r resumePartsRender) {
		t.Run("it carries no copy beyond its own constants and the caller's text", func(t *testing.T) {
			for _, size := range sizes {
				t.Run(size.name, func(t *testing.T) {
					left := ansi.Strip(RenderResumePanel(resumeScreenOf(r, shortCommand, shortReport, size.w, size.h)))
					for _, own := range []string{
						resumePanelTitle, resumePausedBadge, resumeCommandLabel,
						resumeLabelResume, resumeLabelDiscard, resumeKeyResume, resumeKeyDiscard,
						shortCommand, shortReport,
					} {
						left = strings.ReplaceAll(left, own, " ")
					}
					for _, ch := range left {
						if unicode.IsLetter(ch) || unicode.IsDigit(ch) {
							t.Fatalf("the screen carries copy of its own: %q in %q", ch, left)
						}
					}
				})
			}
		})
	})

	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		t.Run("it renders every state through glyphs and words under NO_COLOR/"+themeLabel(th), func(t *testing.T) {
			for _, size := range sizes {
				t.Run(size.name, func(t *testing.T) {
					screen := ResumeScreen{
						Command: resumeGoldenCommand, Report: resumeGoldenReport,
						Width: size.w, Height: size.h, Theme: th,
					}
					painted := RenderResumePanel(screen)
					screen.Colourless = true
					bare := RenderResumePanel(screen)

					if ansi.Strip(painted) != ansi.Strip(bare) {
						t.Errorf("the colourless render differs from the painted one's text\n got: %q\nwant: %q", ansi.Strip(bare), ansi.Strip(painted))
					}
					for _, tok := range []theme.Token{th.Canvas, th.TextPrimary, th.TextMuted, th.AccentPrimary, th.AccentAttention, th.AccentKey, th.Border} {
						if seq := tokenFgSeq(t, tok); strings.Contains(bare, seq) {
							t.Errorf("the colourless render paints the %s hue %q", tok.Name, seq)
						}
						if seq := tokenBgSeq(t, tok); strings.Contains(bare, seq) {
							t.Errorf("the colourless render paints the %s background %q", tok.Name, seq)
						}
					}
					stripped := ansi.Strip(bare)
					for _, glyph := range resumeStateGlyphsAt(size.name) {
						if !strings.Contains(stripped, glyph) {
							t.Errorf("the colourless render drops %q:\n%s", glyph, stripped)
						}
					}
				})
			}
		})
	}
}

// The badge and the label are card parts, so a stack carries neither.
func resumeStateGlyphsAt(size string) []string {
	hints := []string{
		resumeKeyResume + " " + resumeLabelResume,
		resumeKeyDiscard + " " + resumeLabelDiscard,
		resumeGoldenReportHead,
	}
	if size == "the plain stack" {
		return hints
	}
	return append(hints, resumePausedBadge, resumeCommandLabel)
}

// The bytes each header rendered before renderHeaderWithBadge took its badge
// text as a parameter.
func TestRenderHeaderWithBadge_ModalHeadersByteIdentical(t *testing.T) {
	t.Run("it leaves the rename and edit modal headers byte-identical", func(t *testing.T) {
		dark, light := testDarkTheme(t), testLightTheme(t)
		navigate := editModalModel(t, editFieldName, 0, 0)
		editing := editModalModel(t, editFieldName, 0, 0)
		editing.editMode = editModeEdit

		for _, tc := range []struct {
			name string
			got  string
			want string
		}{
			{
				"rename",
				renameModalHeaderRow(dark, false),
				"\x1b[1;38;2;192;202;245;48;2;11;12;20mRename session\x1b[m\x1b[48;2;11;12;20m                     \x1b[m\x1b[1;38;2;255;158;100;48;2;11;12;20m◉ EDIT MODE\x1b[m",
			},
			{
				"rename colourless",
				renameModalHeaderRow(dark, true),
				"\x1b[1mRename session\x1b[m                     \x1b[1m◉ EDIT MODE\x1b[m",
			},
			{
				"edit navigate",
				navigate.editModalHeaderRow(dark, false),
				"\x1b[1;38;2;192;202;245;48;2;11;12;20mEdit Project \x1b[m\x1b[38;2;115;122;162;48;2;11;12;20mflow-v1-api\x1b[m\x1b[48;2;11;12;20m                       \x1b[m\x1b[48;2;11;12;20m           \x1b[m",
			},
			{
				"edit navigate colourless",
				navigate.editModalHeaderRow(dark, true),
				"\x1b[1mEdit Project \x1b[mflow-v1-api                                  ",
			},
			{
				"edit editing",
				editing.editModalHeaderRow(light, false),
				"\x1b[1;38;2;46;60;100;48;2;225;226;231mEdit Project \x1b[m\x1b[38;2;88;96;147;48;2;225;226;231mflow-v1-api\x1b[m\x1b[48;2;225;226;231m                       \x1b[m\x1b[1;38;2;154;82;0;48;2;225;226;231m◉ EDIT MODE\x1b[m",
			},
			{
				"edit editing colourless",
				editing.editModalHeaderRow(light, true),
				"\x1b[1mEdit Project \x1b[mflow-v1-api                       \x1b[1m◉ EDIT MODE\x1b[m",
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				if tc.got != tc.want {
					t.Errorf("the header row renders\n got: %q\nwant: %q", tc.got, tc.want)
				}
			})
		}
	})
}
