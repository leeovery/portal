package tui

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/theme"
)

const (
	resumePartsNarrowWidth = 24
	resumePartsLongCommand = "claude --resume 4f2c9a1e-7b33-4d01-9f6a-2c8e510db4a7 --cwd /Users/leeovery/Code/portal/internal/tui --output-format stream-json"
)

type resumePartsRender struct {
	label      string
	th         theme.Theme
	colourless bool
}

func forEachResumePartsRender(t *testing.T, fn func(t *testing.T, r resumePartsRender)) {
	t.Helper()
	for _, r := range []resumePartsRender{
		{"dark", testDarkTheme(t), false},
		{"light", testLightTheme(t), false},
		{"dark colourless", testDarkTheme(t), true},
		{"light colourless", testLightTheme(t), true},
	} {
		t.Run(r.label, func(t *testing.T) {
			fn(t, r)
		})
	}
}

func assertRowsAtWidth(t *testing.T, rows []string, want int) {
	t.Helper()
	for i, row := range rows {
		if got := lipgloss.Width(row); got != want {
			t.Errorf("row %d renders at width %d, want %d: %q", i, got, want, ansi.Strip(row))
		}
		if strings.Contains(row, "\n") {
			t.Errorf("row %d carries a newline: %q", i, ansi.Strip(row))
		}
	}
}

func assertRowsRead(t *testing.T, rows []string, want []string) {
	t.Helper()
	got := make([]string, 0, len(rows))
	for _, row := range rows {
		got = append(got, rowText(row))
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the rows read\n got: %q\nwant: %q", got, want)
	}
}

// The alternative to pinning exact rows, for text whose expected rows would be
// unreadable as literals: whatever the rows broke at, the visible text runs
// through the command in order and loses nothing before the … .
func assertRowsCarryCommand(t *testing.T, rows []string, command string) {
	t.Helper()
	var shown strings.Builder
	for _, row := range rows {
		shown.WriteString(rowText(row))
	}
	got := spacelessText(strings.TrimSuffix(shown.String(), "…"))
	want := spacelessText(sanitiseCommandText(command))
	if !strings.HasPrefix(want, got) {
		t.Errorf("the rows read %q, want a prefix of %q", got, want)
	}
}

func spacelessText(s string) string {
	return strings.ReplaceAll(s, " ", "")
}

func assertNoControlCharacters(t *testing.T, row string) {
	t.Helper()
	text := ansi.Strip(row)
	for _, ch := range text {
		if ch < 0x20 || ch == 0x7f {
			t.Errorf("the row carries control character %q: %q", ch, text)
		}
	}
}

// ansi.Strip hides an escape sequence from the assertion above, so the raw row
// is what says the sanitiser ran: the sequence must not reach a terminal.
func assertNoEscapeFromText(t *testing.T, row string) {
	t.Helper()
	if strings.Contains(row, "\x1b[31m") {
		t.Errorf("the row carries an escape sequence out of its own text: %q", row)
	}
}

func rowText(row string) string {
	return strings.TrimRight(ansi.Strip(row), " ")
}

// Every word differs, so a wrap that drops or reorders one breaks the prefix
// assertRowsCarryCommand makes; a repeated word would hide both.
func commandOfLength(n int) string {
	var b strings.Builder
	for i := 0; b.Len() < n; i++ {
		fmt.Fprintf(&b, "w%d ", i)
	}
	return b.String()[:n]
}

func TestResumeCommandRows_Geometry(t *testing.T) {
	forEachResumePartsRender(t, func(t *testing.T, r resumePartsRender) {
		t.Run("it renders a short command as one row at the pinned width", func(t *testing.T) {
			rows := resumeCommandRows("vim", resumeCardContentWidth, r.th.TextPrimary, false, r.th, r.colourless)
			assertRowsAtWidth(t, rows, resumeCardContentWidth)
			assertRowsRead(t, rows, []string{"vim"})
		})

		t.Run("it wraps a long command over at most three rows", func(t *testing.T) {
			rows := resumeCommandRows(resumePartsLongCommand, resumeCardContentWidth, r.th.TextPrimary, false, r.th, r.colourless)
			assertRowsAtWidth(t, rows, resumeCardContentWidth)
			assertRowsRead(t, rows, []string{
				"claude --resume 4f2c9a1e-7b33-4d01-9f6a-2c8e510db4a7",
				"--cwd /Users/leeovery/Code/portal/internal/tui --",
				"output-format stream-json",
			})
		})

		t.Run("it wraps a command that fills three rows without marking it cut", func(t *testing.T) {
			const command = "claude --resume 4f2c9a1e-7b33-4d01 --cwd /Users/leeovery/Code/portal/internal/tui --output stream"
			rows := resumeCommandRows(command, resumeCardContentWidth, r.th.TextPrimary, false, r.th, r.colourless)
			assertRowsAtWidth(t, rows, resumeCardContentWidth)
			assertRowsRead(t, rows, []string{
				"claude --resume 4f2c9a1e-7b33-4d01 --cwd",
				"/Users/leeovery/Code/portal/internal/tui --output",
				"stream",
			})
		})

		t.Run("it marks the third row with … when the command runs past it", func(t *testing.T) {
			rows := resumeCommandRows(strings.Repeat(resumePartsLongCommand+" ", 10), resumeCardContentWidth, r.th.TextPrimary, false, r.th, r.colourless)
			assertRowsAtWidth(t, rows, resumeCardContentWidth)
			assertRowsRead(t, rows, []string{
				"claude --resume 4f2c9a1e-7b33-4d01-9f6a-2c8e510db4a7",
				"--cwd /Users/leeovery/Code/portal/internal/tui --",
				"output-format stream-json claude --resume 4f2c9a1e-…",
			})
		})

		t.Run("it keeps the wrap's trailing space out of the joined third row", func(t *testing.T) {
			for _, tc := range []struct {
				name    string
				command string
				width   int
				want    []string
			}{
				{
					"the card's pinned width",
					"--permission-mode run /Users/leeovery/Code/portal/internal/tui 4f2c9a1e-7b33-4d01-9f6a-2c8e510db4a7 --permission-mode stream-json",
					resumeCardContentWidth,
					[]string{
						"--permission-mode run",
						"/Users/leeovery/Code/portal/internal/tui 4f2c9a1e-",
						"7b33-4d01-9f6a-2c8e510db4a7 --permission-mode strea…",
					},
				},
				{
					"a narrower pane's",
					"/Users/leeovery/Code/portal/internal/tui npm src/main.go npm 4f2c9a1e-7b33-4d01",
					resumePartsNarrowWidth,
					[]string{
						"/Users/leeovery/Code/por",
						"tal/internal/tui npm",
						"src/main.go npm 4f2c9a1…",
					},
				},
			} {
				t.Run(tc.name, func(t *testing.T) {
					rows := resumeCommandRows(tc.command, tc.width, r.th.TextPrimary, false, r.th, r.colourless)
					assertRowsAtWidth(t, rows, tc.width)
					assertRowsRead(t, rows, tc.want)
				})
			}
		})

		t.Run("it breaks a single unbroken token at the inner width", func(t *testing.T) {
			rows := resumeCommandRows(strings.Repeat("a", 500), resumeCardContentWidth, r.th.TextPrimary, false, r.th, r.colourless)
			assertRowsAtWidth(t, rows, resumeCardContentWidth)
			assertRowsRead(t, rows, []string{
				strings.Repeat("a", 52),
				strings.Repeat("a", 52),
				strings.Repeat("a", 51) + "…",
			})
		})

		t.Run("it keeps the pinned width for double-width and combining runes", func(t *testing.T) {
			const (
				wideWord = "再開するセッション"
				markWord = "résumé"
			)
			for _, tc := range []struct {
				name    string
				command string
				want    []string
			}{
				{
					"double width", strings.Repeat(wideWord+" ", 12),
					[]string{
						wideWord + " " + wideWord,
						wideWord + " " + wideWord,
						wideWord + " " + wideWord + " 再開するセッ…",
					},
				},
				{
					"combining marks", strings.Repeat(markWord+" ", 40),
					[]string{
						strings.TrimSuffix(strings.Repeat(markWord+" ", 7), " "),
						strings.TrimSuffix(strings.Repeat(markWord+" ", 7), " "),
						strings.Repeat(markWord+" ", 7) + "ré…",
					},
				},
			} {
				t.Run(tc.name, func(t *testing.T) {
					rows := resumeCommandRows(tc.command, resumeCardContentWidth, r.th.TextPrimary, false, r.th, r.colourless)
					assertRowsAtWidth(t, rows, resumeCardContentWidth)
					assertRowsRead(t, rows, tc.want)
				})
			}
		})

		t.Run("it neutralises control characters without changing the geometry", func(t *testing.T) {
			const (
				prefix = "claude --resume "
				suffix = " --cwd /Users/leeovery/Code/portal"
			)
			for _, tc := range []struct {
				name    string
				control string
				spaced  string
				want    []string
			}{
				{"newline", "\n", " ", []string{"claude --resume   --cwd /Users/leeovery/Code/portal"}},
				{"tab", "\t", " ", []string{"claude --resume   --cwd /Users/leeovery/Code/portal"}},
				{
					"escape sequence", "\x1b[31m", " [31m",
					[]string{"claude --resume  [31m --cwd", "/Users/leeovery/Code/portal"},
				},
				{"bell", "\x07", " ", []string{"claude --resume   --cwd /Users/leeovery/Code/portal"}},
				{"delete", "\x7f", " ", []string{"claude --resume   --cwd /Users/leeovery/Code/portal"}},
				{"unit separator", "\x1f", " ", []string{"claude --resume   --cwd /Users/leeovery/Code/portal"}},
			} {
				t.Run(tc.name, func(t *testing.T) {
					rows := resumeCommandRows(prefix+tc.control+suffix, resumeCardContentWidth, r.th.TextPrimary, false, r.th, r.colourless)
					spaced := resumeCommandRows(prefix+tc.spaced+suffix, resumeCardContentWidth, r.th.TextPrimary, false, r.th, r.colourless)
					if !reflect.DeepEqual(rows, spaced) {
						t.Errorf("the control character changes the render\n got: %q\nwant: %q", rows, spaced)
					}
					assertRowsAtWidth(t, rows, resumeCardContentWidth)
					assertRowsRead(t, rows, tc.want)
					for _, row := range rows {
						assertNoControlCharacters(t, row)
						assertNoEscapeFromText(t, row)
					}
				})
			}
		})

		t.Run("it carries the command's text across the rows it wraps to", func(t *testing.T) {
			const command = "claude --resume 4f2c9a1e --cwd /Users/leeovery/Code/portal"
			for _, tc := range []struct {
				name  string
				width int
				want  []string
			}{
				{
					"the card's pinned width",
					resumeCardContentWidth,
					[]string{"claude --resume 4f2c9a1e --cwd", "/Users/leeovery/Code/portal"},
				},
				{
					"a narrower pane's",
					resumePartsNarrowWidth,
					[]string{"claude --resume 4f2c9a1e", "--cwd", "/Users/leeovery/Code/po…"},
				},
			} {
				t.Run(tc.name, func(t *testing.T) {
					rows := resumeCommandRows(command, tc.width, r.th.TextPrimary, false, r.th, r.colourless)
					assertRowsAtWidth(t, rows, tc.width)
					assertRowsRead(t, rows, tc.want)
				})
			}
		})

		t.Run("it carries what it can when the pane is too narrow to wrap into", func(t *testing.T) {
			for _, tc := range []struct {
				name    string
				command string
				want    map[int][]string
			}{
				{
					"an ordinary command", "claude --resume abc",
					map[int][]string{
						1: {"c", "l", "…"},
						2: {"cl", "au", "d…"},
						3: {"cla", "ude", "es…"},
						4: {"clau", "de -", "-re…"},
					},
				},
				{
					"a double-width command", "再開するセッション",
					map[int][]string{
						1: {"", "", "…"},
						2: {"再", "開", "…"},
						3: {"再", "開", "す…"},
						4: {"再開", "する", "セ…"},
					},
				},
			} {
				t.Run(tc.name, func(t *testing.T) {
					for width := 1; width <= 4; width++ {
						t.Run(fmt.Sprintf("at width %d", width), func(t *testing.T) {
							rows := resumeCommandRows(tc.command, width, r.th.TextPrimary, false, r.th, r.colourless)
							assertRowsAtWidth(t, rows, width)
							assertRowsRead(t, rows, tc.want[width])
						})
					}
				})
			}
		})

		t.Run("it holds the width when the command opens on whitespace", func(t *testing.T) {
			for _, tc := range []struct {
				name    string
				leading string
				wide    []string
				narrow  []string
			}{
				{
					"a space", " ",
					[]string{strings.Repeat("a", 52), strings.Repeat("a", 8)},
					[]string{strings.Repeat("a", 24), strings.Repeat("a", 24), strings.Repeat("a", 12)},
				},
				{
					"a tab", "\t",
					[]string{strings.Repeat("a", 52), strings.Repeat("a", 8)},
					[]string{strings.Repeat("a", 24), strings.Repeat("a", 24), strings.Repeat("a", 12)},
				},
				{
					"an escape sequence", "\x1b[31m",
					[]string{"[31m" + strings.Repeat("a", 48), strings.Repeat("a", 12)},
					[]string{"[31m" + strings.Repeat("a", 20), strings.Repeat("a", 24), strings.Repeat("a", 16)},
				},
			} {
				t.Run(tc.name, func(t *testing.T) {
					for _, wc := range []struct {
						width int
						want  []string
					}{
						{resumeCardContentWidth, tc.wide},
						{resumePartsNarrowWidth, tc.narrow},
					} {
						t.Run(fmt.Sprintf("at width %d", wc.width), func(t *testing.T) {
							rows := resumeCommandRows(tc.leading+strings.Repeat("a", 60), wc.width, r.th.TextPrimary, false, r.th, r.colourless)
							assertRowsAtWidth(t, rows, wc.width)
							assertRowsRead(t, rows, wc.want)
						})
					}
				})
			}
		})

		t.Run("it renders the same width whatever the command's length", func(t *testing.T) {
			for _, length := range []int{1, 40, 52, 53, 500} {
				t.Run(fmt.Sprintf("%d characters", length), func(t *testing.T) {
					command := commandOfLength(length)
					rows := resumeCommandRows(command, resumeCardContentWidth, r.th.TextPrimary, false, r.th, r.colourless)
					if len(rows) < 1 || len(rows) > 3 {
						t.Fatalf("a %d-character command renders %d rows, want 1 to 3", length, len(rows))
					}
					assertRowsAtWidth(t, rows, resumeCardContentWidth)
					assertRowsCarryCommand(t, rows, command)
				})
			}
		})

		t.Run("it renders an empty command as one empty row", func(t *testing.T) {
			rows := resumeCommandRows("", resumeCardContentWidth, r.th.TextPrimary, false, r.th, r.colourless)
			assertRowsAtWidth(t, rows, resumeCardContentWidth)
			assertRowsRead(t, rows, []string{""})
		})
	})
}

func TestResumePanelParts_CallerChoices(t *testing.T) {
	forEachResumePartsRender(t, func(t *testing.T, r resumePartsRender) {
		t.Run("it renders both blocks at the width its caller chose", func(t *testing.T) {
			command := resumePartsLongCommand + " --permission-mode acceptEdits"
			for _, tc := range []struct {
				name       string
				width      int
				wantRows   []string
				wantReport string
			}{
				{
					"the card's pinned width", resumeCardContentWidth,
					[]string{
						"claude --resume 4f2c9a1e-7b33-4d01-9f6a-2c8e510db4a7",
						"--cwd /Users/leeovery/Code/portal/internal/tui --",
						"output-format stream-json --permission-mode acceptE…",
					},
					"claude --resume 4f2c9a1e-7b33-4d01-9f6a-2c8e510db4a…",
				},
				{
					"a narrower pane's", resumePartsNarrowWidth,
					[]string{
						"claude --resume",
						"4f2c9a1e-7b33-4d01-9f6a-",
						"2c8e510db4a7 --cwd /Use…",
					},
					"claude --resume 4f2c9a1…",
				},
			} {
				t.Run(tc.name, func(t *testing.T) {
					rows := resumeCommandRows(command, tc.width, r.th.TextPrimary, false, r.th, r.colourless)
					assertRowsAtWidth(t, rows, tc.width)
					assertRowsRead(t, rows, tc.wantRows)

					report, ok := resumeReportRow(resumePartsLongCommand, tc.width, r.th, r.colourless)
					if !ok {
						t.Fatal("a non-empty report reports no row")
					}
					assertRowsAtWidth(t, []string{report}, tc.width)
					assertRowsRead(t, []string{report}, []string{tc.wantReport})
				})
			}
		})

		t.Run("it renders the command in the token and emphasis its caller chose", func(t *testing.T) {
			for _, tc := range []struct {
				name string
				tok  theme.Token
				bold bool
			}{
				{"text.primary unbolded", r.th.TextPrimary, false},
				{"state.destructive bolded", r.th.StateDestructive, true},
			} {
				t.Run(tc.name, func(t *testing.T) {
					rows := resumeCommandRows("vim", resumeCardContentWidth, tc.tok, tc.bold, r.th, r.colourless)
					assertEmphasisedToken(t, rows[0], tc.tok, tc.bold, r.colourless)
				})
			}
		})
	})
}

func assertEmphasisedToken(t *testing.T, row string, tok theme.Token, bold, colourless bool) {
	t.Helper()
	fg := tokenFgSeq(t, tok)
	switch {
	case colourless && bold:
		if !strings.Contains(row, "\x1b[1m") {
			t.Errorf("the colourless row carries no bold attribute: %q", row)
		}
	case colourless:
		if strings.Contains(row, "\x1b") {
			t.Errorf("the colourless row carries an escape sequence: %q", row)
		}
	case bold:
		if !strings.Contains(row, "\x1b[1;"+fg) {
			t.Errorf("the row does not render %s bolded: %q", tok.Name, row)
		}
	default:
		if !strings.Contains(row, "\x1b["+fg) {
			t.Errorf("the row does not render %s: %q", tok.Name, row)
		}
	}
}

func TestResumeReportRow(t *testing.T) {
	forEachResumePartsRender(t, func(t *testing.T, r resumePartsRender) {
		t.Run("it renders a report as one truncated row", func(t *testing.T) {
			for _, tc := range []struct {
				name   string
				report string
				want   string
			}{
				{"shorter than the width", "the freeze will not lift", "the freeze will not lift"},
				{
					"longer than the width", strings.Repeat("the freeze will not lift ", 10),
					"the freeze will not lift the freeze will not lift t…",
				},
			} {
				t.Run(tc.name, func(t *testing.T) {
					row, ok := resumeReportRow(tc.report, resumeCardContentWidth, r.th, r.colourless)
					if !ok {
						t.Fatal("a non-empty report reports no row")
					}
					assertRowsAtWidth(t, []string{row}, resumeCardContentWidth)
					assertRowsRead(t, []string{row}, []string{tc.want})
					if !r.colourless && !strings.Contains(row, tokenFgSeq(t, r.th.AccentAttention)) {
						t.Errorf("the row does not render accent.attention: %q", row)
					}
				})
			}
		})

		t.Run("it carries what it can when the pane is too narrow to report into", func(t *testing.T) {
			for _, tc := range []struct {
				name   string
				report string
				want   map[int]string
			}{
				{
					"an ordinary report", "the freeze will not lift",
					map[int]string{1: "…", 2: "t…", 3: "th…", 4: "the…"},
				},
				{
					"a double-width report", "再開するセッション",
					map[int]string{1: "…", 2: "…", 3: "再…", 4: "再…"},
				},
			} {
				t.Run(tc.name, func(t *testing.T) {
					for width := 1; width <= 4; width++ {
						t.Run(fmt.Sprintf("at width %d", width), func(t *testing.T) {
							row, ok := resumeReportRow(tc.report, width, r.th, r.colourless)
							if !ok {
								t.Fatal("a non-empty report reports no row")
							}
							assertRowsAtWidth(t, []string{row}, width)
							assertRowsRead(t, []string{row}, []string{tc.want[width]})
						})
					}
				})
			}
		})

		t.Run("it reports no row at all for an empty report", func(t *testing.T) {
			row, ok := resumeReportRow("", resumeCardContentWidth, r.th, r.colourless)
			if ok || row != "" {
				t.Errorf("an empty report reports (%q, %t), want (\"\", false)", row, ok)
			}
		})

		t.Run("it neutralises control characters in a report", func(t *testing.T) {
			const (
				prefix = "frozen"
				suffix = "still"
			)
			for _, tc := range []struct {
				name    string
				control string
				spaced  string
				want    string
			}{
				{"newline", "\n", " ", "frozen still"},
				{"tab", "\t", " ", "frozen still"},
				{"escape sequence", "\x1b[31m", " [31m", "frozen [31mstill"},
				{"bell", "\x07", " ", "frozen still"},
				{"delete", "\x7f", " ", "frozen still"},
				{"unit separator", "\x1f", " ", "frozen still"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					row, ok := resumeReportRow(prefix+tc.control+suffix, resumeCardContentWidth, r.th, r.colourless)
					if !ok {
						t.Fatal("a non-empty report reports no row")
					}
					spaced, _ := resumeReportRow(prefix+tc.spaced+suffix, resumeCardContentWidth, r.th, r.colourless)
					if row != spaced {
						t.Errorf("the control character changes the render\n got: %q\nwant: %q", row, spaced)
					}
					assertRowsAtWidth(t, []string{row}, resumeCardContentWidth)
					assertRowsRead(t, []string{row}, []string{tc.want})
					assertNoControlCharacters(t, row)
					assertNoEscapeFromText(t, row)
				})
			}
		})
	})
}
