package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/theme"
)

const (
	discardGoldenTitleRow  = destructiveTitleGlyph + " " + discardConfirmTitle
	discardGoldenFooterRow = discardKeyConfirm + " " + discardLabelConfirm + modalFooterGap +
		destructiveKeyCancel + " " + destructiveLabelCancel
	discardStackWidth = 30
)

// Pinned as literals rather than re-wrapped from the constant: an expectation
// built by the code under test's own wrap asserts nothing about where it broke.
var (
	discardConsequenceAtCard = []string{
		"Removes this pane's resume command permanently. The",
		"session and its scrollback are untouched.",
	}
	discardConsequenceAtStack = []string{
		"Removes this pane's resume",
		"command permanently. The",
		"session and its scrollback are",
		"untouched.",
	}
)

func discardScreenOf(r resumePartsRender, command, report string, w, h int) ResumeScreen {
	return ResumeScreen{
		Command:    command,
		Report:     report,
		Width:      w,
		Height:     h,
		Theme:      r.th,
		Colourless: r.colourless,
	}
}

func discardCardRows(t *testing.T, r resumePartsRender, command, report string) []string {
	t.Helper()
	out := RenderResumeDiscardConfirm(discardScreenOf(r, command, report, resumeRoomyWidth, resumeRoomyHeight))
	return nonEmptyRows(paneRows(t, out, resumeRoomyWidth, resumeRoomyHeight))
}

// The card's own content, lifted out of the frame: inset, pad and side glyphs
// dropped so a row reads as the text the compartment put there.
func discardCardContentRows(rows []string) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		runes := []rune(row)
		if len(runes) < 2*(1+panelRowInset) || string(runes[0]) != panelFrameSide {
			continue
		}
		inner := string(runes[1+panelRowInset : len(runes)-(1+panelRowInset)])
		out = append(out, strings.TrimRight(inner, " "))
	}
	return out
}

func discardGoldenCard(commandRows []string, report string, consequence []string) []string {
	body := append([]string{}, commandRows...)
	body = append(body, "")
	body = append(body, consequence...)
	if report != "" {
		body = append(body, report)
	}
	rows := append([]string{discardGoldenTitleRow, panelRuleGlyph}, body...)
	return resumeGoldenCard(append(rows, panelRuleGlyph, discardGoldenFooterRow)...)
}

func TestRenderResumeDiscardConfirm_Card(t *testing.T) {
	forEachResumePartsRender(t, func(t *testing.T, r resumePartsRender) {
		t.Run("it renders the discard title with the destructive glyph", func(t *testing.T) {
			rows := discardCardRows(t, r, resumeGoldenCommand, "")
			if len(rows) < 2 {
				t.Fatalf("the card renders %d rows, want at least 2: %q", len(rows), rows)
			}
			if got, want := rows[1], resumeGoldenContentRow("\u25b2 Discard resume?"); got != want {
				t.Errorf("the header row reads\n got: %q\nwant: %q", got, want)
			}
			out := RenderResumeDiscardConfirm(discardScreenOf(r, resumeGoldenCommand, "", resumeRoomyWidth, resumeRoomyHeight))
			assertEmphasisedToken(t, labelSegment(t, out, discardConfirmTitle), r.th.StateDestructive, true, r.colourless)
		})

		t.Run("it carries exactly the kill modal's compartments with nothing to report", func(t *testing.T) {
			rows := discardCardRows(t, r, resumeGoldenCommand, "")
			assertRowsEqual(t, rows, discardGoldenCard([]string{resumeGoldenCommand}, "", discardConsequenceAtCard))
		})

		t.Run("it renders the command in state.destructive over up to three rows", func(t *testing.T) {
			rows := discardCardRows(t, r, resumePartsLongCommand, "")
			assertRowsEqual(t, rows, discardGoldenCard([]string{
				"claude --resume 4f2c9a1e-7b33-4d01-9f6a-2c8e510db4a7",
				"--cwd /Users/leeovery/Code/portal/internal/tui --",
				"output-format stream-json",
			}, "", discardConsequenceAtCard))
			out := RenderResumeDiscardConfirm(discardScreenOf(r, resumeGoldenCommand, "", resumeRoomyWidth, resumeRoomyHeight))
			assertEmphasisedToken(t, labelSegment(t, out, resumeGoldenCommand), r.th.StateDestructive, true, r.colourless)
		})

		t.Run("it renders the consequence line verbatim", func(t *testing.T) {
			content := discardCardContentRows(discardCardRows(t, r, resumeGoldenCommand, ""))
			joined := strings.Join(content, " ")
			for _, want := range discardConsequenceAtCard {
				if !strings.Contains(joined, want) {
					t.Errorf("the card does not read %q:\n%q", want, content)
				}
			}
			const sentence = "Removes this pane's resume command permanently. The session and its scrollback are untouched."
			if discardConfirmConsequence != sentence {
				t.Errorf("the consequence constant reads\n got: %q\nwant: %q", discardConfirmConsequence, sentence)
			}
		})

		t.Run("it renders the y discard esc cancel footer", func(t *testing.T) {
			rows := discardCardRows(t, r, resumeGoldenCommand, "")
			if got, want := rows[len(rows)-2], resumeGoldenContentRow("y discard   esc cancel"); got != want {
				t.Errorf("the footer row reads\n got: %q\nwant: %q", got, want)
			}
		})

		t.Run("it carries the report row on the confirmation itself", func(t *testing.T) {
			rows := discardCardRows(t, r, resumeGoldenCommand, resumeGoldenReport)
			assertRowsEqual(t, rows, discardGoldenCard([]string{resumeGoldenCommand}, resumeGoldenReport, discardConsequenceAtCard))
			if quiet := discardCardRows(t, r, resumeGoldenCommand, ""); len(rows) != len(quiet)+1 {
				t.Errorf("the report adds %d rows, want exactly 1", len(rows)-len(quiet))
			}
		})

		t.Run("it renders the same card width whatever the command and the report", func(t *testing.T) {
			for _, tc := range []struct {
				name    string
				command string
				report  string
			}{
				{"one character", "v", ""},
				{"three lines", strings.Repeat(resumePartsLongCommand+" ", 4), ""},
				{"a long report", "vim", strings.Repeat(resumeGoldenReport+" ", 6)},
				{"an empty command", "", resumeGoldenReport},
			} {
				t.Run(tc.name, func(t *testing.T) {
					for i, row := range discardCardRows(t, r, tc.command, tc.report) {
						if got := lipgloss.Width(row); got != resumeCardWidth {
							t.Errorf("card row %d renders at width %d, want %d: %q", i, got, resumeCardWidth, row)
						}
					}
				})
			}
		})
	})
}

func TestRenderResumeDiscardConfirm_PlainStack(t *testing.T) {
	forEachResumePartsRender(t, func(t *testing.T, r resumePartsRender) {
		t.Run("it degrades to the plain stack below the card's size", func(t *testing.T) {
			const h = 12
			out := RenderResumeDiscardConfirm(discardScreenOf(r, "vim", resumeGoldenReport, discardStackWidth, h))
			want := append([]string{discardGoldenTitleRow, "vim"}, discardConsequenceAtStack...)
			want = append(want, ansi.Truncate(resumeGoldenReport, discardStackWidth, resumeEllipsis), discardGoldenFooterRow)
			assertRowsEqual(t, nonEmptyRows(paneRows(t, out, discardStackWidth, h)), want)
			if strings.Contains(ansi.Strip(out), panelFrameTopLeft) {
				t.Errorf("the plain stack carries the frame:\n%s", ansi.Strip(out))
			}
		})

		t.Run("it wraps the consequence to the pane in the plain stack", func(t *testing.T) {
			const h = 12
			rows := nonEmptyRows(paneRows(t, RenderResumeDiscardConfirm(
				discardScreenOf(r, "vim", "", discardStackWidth, h)), discardStackWidth, h))
			for i, row := range rows {
				if got := lipgloss.Width(row); got > discardStackWidth {
					t.Errorf("stack row %d is %d cells wide, want at most %d: %q", i, got, discardStackWidth, row)
				}
			}
			joined := strings.Join(rows, "\n")
			for _, want := range discardConsequenceAtStack {
				if !strings.Contains(joined, want) {
					t.Errorf("the stack does not re-wrap the consequence to the pane, missing %q:\n%s", want, joined)
				}
			}
			for word := range strings.FieldsSeq(discardConfirmConsequence) {
				if !strings.Contains(joined, word) {
					t.Errorf("the stack drops the word %q from the consequence:\n%s", word, joined)
				}
			}
		})

		t.Run("it builds the plain stack from the same compartments as the card", func(t *testing.T) {
			const h = 14
			card := discardCardContentRows(discardCardRows(t, r, resumeGoldenCommand, resumeGoldenReport))
			out := RenderResumeDiscardConfirm(discardScreenOf(r, resumeGoldenCommand, resumeGoldenReport, resumeCardContentWidth, h))
			stack := paneRows(t, out, resumeCardContentWidth, h)
			assertRowsEqual(t, stack[:len(card)], card)
		})
	})
}

func TestRenderResumeDiscardConfirm_NoColour(t *testing.T) {
	sizes := []struct {
		name string
		w, h int
	}{
		{"the card", resumeRoomyWidth, resumeRoomyHeight},
		{"the plain stack", discardStackWidth, 12},
	}
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		t.Run("it keeps the destructive signal under NO_COLOR/"+themeLabel(th), func(t *testing.T) {
			for _, size := range sizes {
				t.Run(size.name, func(t *testing.T) {
					screen := ResumeScreen{
						Command: "vim", Report: resumeGoldenReport,
						Width: size.w, Height: size.h, Theme: th,
					}
					painted := RenderResumeDiscardConfirm(screen)
					screen.Colourless = true
					bare := RenderResumeDiscardConfirm(screen)

					if ansi.Strip(painted) != ansi.Strip(bare) {
						t.Errorf("the colourless render differs from the painted one's text\n got: %q\nwant: %q", ansi.Strip(bare), ansi.Strip(painted))
					}
					for _, tok := range []theme.Token{th.Canvas, th.StateDestructive, th.TextMuted, th.AccentAttention, th.AccentKey, th.Border} {
						if seq := tokenFgSeq(t, tok); strings.Contains(bare, seq) {
							t.Errorf("the colourless render paints the %s hue %q", tok.Name, seq)
						}
						if seq := tokenBgSeq(t, tok); strings.Contains(bare, seq) {
							t.Errorf("the colourless render paints the %s background %q", tok.Name, seq)
						}
					}
					stripped := ansi.Strip(bare)
					for _, glyph := range []string{
						destructiveTitleGlyph, discardConfirmTitle, "vim",
						discardConsequenceAtStack[0], discardGoldenFooterRow,
					} {
						if !strings.Contains(stripped, glyph) {
							t.Errorf("the colourless render drops %q:\n%s", glyph, stripped)
						}
					}
					if !strings.Contains(bare, "\x1b[1m") {
						t.Errorf("the colourless render carries no bold attribute:\n%q", bare)
					}
				})
			}
		})
	}
}
