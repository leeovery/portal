package tui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func spaceWrappedLines(text string, width int) []string {
	return rowTexts(spaceWrappedRows(text, width))
}

func TestWrappedLines_ProseBreaks(t *testing.T) {
	t.Run("it breaks prose straight after a hyphen", func(t *testing.T) {
		if got, want := wrappedLines("gruvbox-material", 10), []string{"gruvbox-", "material"}; !reflect.DeepEqual(got, want) {
			t.Errorf("the rows read\n got: %q\nwant: %q", got, want)
		}
	})

	t.Run("it re-flows a hyphen run the wrap over-packs onto the next row", func(t *testing.T) {
		got := wrappedLines(resumePartsHyphenCommand, 12)
		want := []string{"a-b-c-d-e-f", "-- --port=30", "00 --resume"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("the rows read\n got: %q\nwant: %q", got, want)
		}
	})

	t.Run("it returns no row wider than the width asked for", func(t *testing.T) {
		forEachResumePartsCommand(t, func(t *testing.T, command string, width int) {
			for i, line := range wrappedLines(sanitiseCommandText(command), width) {
				if got := ansi.StringWidth(line); got > width {
					t.Errorf("row %d renders at width %d over a width of %d: %q", i, got, width, line)
				}
			}
		})
	})
}

func TestSpaceWrappedRows(t *testing.T) {
	t.Run("it breaks inside a word only where the word fills the row", func(t *testing.T) {
		forEachResumePartsCommand(t, func(t *testing.T, command string, width int) {
			rows := spaceWrappedRows(sanitiseCommandText(command), width)
			for i := 1; i < len(rows); i++ {
				if rows[i].gap != "" || rows[i-1].text == "" || rows[i].text == "" {
					continue
				}
				_, next := ansi.FirstGraphemeCluster(rows[i].text, ansi.GraphemeWidth)
				if ansi.StringWidth(rows[i-1].text)+next <= width {
					t.Errorf("at a width of %d row %d %q breaks from %q inside a word with room left on the row", width, i-1, rows[i-1].text, rows[i].text)
				}
			}
		})
	})

	t.Run("it reads the source back", func(t *testing.T) {
		forEachResumePartsCommand(t, func(t *testing.T, command string, width int) {
			text := sanitiseCommandText(command)
			assertRowsReadSource(t, spaceWrappedLines(text, width), text, width)
		})
	})

	t.Run("it hard-breaks a word wider than the row at the width", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			text  string
			width int
			want  []string
		}{
			{"a plain word", strings.Repeat("a", 25), 10, []string{"aaaaaaaaaa", "aaaaaaaaaa", "aaaaa"}},
			{"a hyphenated word", "4f2c9a1e-7b33-4d01-9f6a-2c8e510db4a7", 20, []string{"4f2c9a1e-7b33-4d01-9", "f6a-2c8e510db4a7"}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				if got := spaceWrappedLines(tc.text, tc.width); !reflect.DeepEqual(got, tc.want) {
					t.Errorf("the rows read\n got: %q\nwant: %q", got, tc.want)
				}
			})
		}
	})

	t.Run("it hands back a private-use character the text already holds", func(t *testing.T) {
		const text = "\ue000\ue001 --resume x"
		if got, want := spaceWrappedLines(text, 10), []string{"\ue000\ue001", "--resume x"}; !reflect.DeepEqual(got, want) {
			t.Errorf("the rows read\n got: %q\nwant: %q", got, want)
		}
	})
}

// ansi.Wrap over-packs a space and a word as wide as the row onto one line and
// ends on an empty one, which is no row of the text's.
func TestWrap_LeadingSpaceAndARowWideWord(t *testing.T) {
	word := strings.Repeat("a", 60)
	for name, wrap := range map[string]func(string, int) []string{
		"prose":   wrappedLines,
		"command": spaceWrappedLines,
	} {
		t.Run("the "+name+" path leaves no trailing empty row", func(t *testing.T) {
			if got, want := wrap(" "+word, 60), []string{word}; !reflect.DeepEqual(got, want) {
				t.Errorf("the rows read\n got: %q\nwant: %q", got, want)
			}
			if got, want := wrap(" "+word+" b", 60), []string{word, "b"}; !reflect.DeepEqual(got, want) {
				t.Errorf("with a word after, the rows read\n got: %q\nwant: %q", got, want)
			}
		})

		t.Run("the "+name+" path keeps the row a line break ends on", func(t *testing.T) {
			if got, want := wrap(word+"\n", 60), []string{word, ""}; !reflect.DeepEqual(got, want) {
				t.Errorf("the rows read\n got: %q\nwant: %q", got, want)
			}
			if got, want := wrap(" "+word+"\n", 60), []string{word, ""}; !reflect.DeepEqual(got, want) {
				t.Errorf("after the over-packed row, the rows read\n got: %q\nwant: %q", got, want)
			}
		})

		t.Run("the "+name+" path adds no row between an over-packed row and the next line", func(t *testing.T) {
			if got, want := wrap(" "+word+"\nb", 60), []string{word, "b"}; !reflect.DeepEqual(got, want) {
				t.Errorf("the rows read\n got: %q\nwant: %q", got, want)
			}
		})
	}

	t.Run("a capped wrap fills its rows from the text alone", func(t *testing.T) {
		if got, want := wrapCappedAtSpaces(" "+word+" b", 60, 2, "…"), []string{word, "b"}; !reflect.DeepEqual(got, want) {
			t.Errorf("the rows read\n got: %q\nwant: %q", got, want)
		}
		if got, want := wrapCapped(" "+word, 60, 1, "…"), []string{word}; !reflect.DeepEqual(got, want) {
			t.Errorf("the rows read\n got: %q\nwant: %q", got, want)
		}
	})
}

func TestWrap_KeepsEveryBlankLine(t *testing.T) {
	cases := []struct {
		text string
		want []string
	}{
		{"a\n\nb", []string{"a", "", "b"}},
		{"a\n\n\nb", []string{"a", "", "", "b"}},
		{"a\n\n\n\nb", []string{"a", "", "", "", "b"}},
		{"a\n\n", []string{"a", "", ""}},
		{"\n\na", []string{"", "", "a"}},
	}
	for name, wrap := range map[string]func(string, int) []string{
		"prose":   wrappedLines,
		"command": spaceWrappedLines,
	} {
		t.Run("the "+name+" path gives each blank line its row", func(t *testing.T) {
			for _, c := range cases {
				if got := wrap(c.text, 60); !reflect.DeepEqual(got, c.want) {
					t.Errorf("%q wraps to\n got: %q\nwant: %q", c.text, got, c.want)
				}
			}
		})
	}
}
