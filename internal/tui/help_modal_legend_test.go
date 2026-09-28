package tui

import (
	"slices"
	"strings"
	"testing"
	"unicode"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/themetest"
)

// preLegendHelpPanel is the help panel's composition before the legend existed —
// one title compartment over one body compartment — kept as the reference the
// legend-less renders are compared against byte for byte.
func preLegendHelpPanel(entries []keymapEntry, th theme.Theme, colourless bool) string {
	bodyRows := helpModalBodyRows(entries, th, colourless)
	contentWidth := lipgloss.Width(helpModalHeader(0, th, colourless))
	for _, r := range bodyRows {
		contentWidth = max(contentWidth, lipgloss.Width(r))
	}
	title := helpModalHeader(contentWidth, th, colourless)
	return renderJoinedPanel([][]string{{title}, bodyRows}, th.Border, th, colourless)
}

func forEachThemeAndColourMode(t *testing.T, fn func(t *testing.T, th theme.Theme, colourless bool)) {
	t.Helper()
	for _, slug := range theme.BuiltinSlugs() {
		t.Run(slug, func(t *testing.T) {
			fn(t, themetest.Builtin(t, slug), false)
		})
	}
	t.Run("colourless", func(t *testing.T) {
		fn(t, testDarkTheme(t), true)
	})
}

func sessionsHelpPanel(t *testing.T, th theme.Theme, colourless bool) string {
	t.Helper()
	return renderHelpModalContent(sessionsKeymap(), th, colourless, sessionsIndicatorLegend(th, colourless))
}

// legendPanelRows returns the raw content lines of the panel's third
// compartment — everything between the second divider and the bottom edge.
func legendPanelRows(t *testing.T, panel string) []string {
	t.Helper()
	lines := strings.Split(panel, "\n")
	var dividers []int
	bottom := -1
	for i, raw := range lines {
		line := strings.TrimSpace(ansi.Strip(raw))
		if strings.HasPrefix(line, panelFrameTeeLeft) && strings.HasSuffix(line, panelFrameTeeRight) {
			dividers = append(dividers, i)
		}
		if bottom < 0 && strings.HasPrefix(line, "╰") {
			bottom = i
		}
	}
	if len(dividers) != 2 || bottom < dividers[len(dividers)-1] {
		t.Fatalf("want 2 dividers (title | keys | legend) above the bottom edge, got %d in:\n%s", len(dividers), ansi.Strip(panel))
	}
	return lines[dividers[1]+1 : bottom]
}

func innerText(raw string) string {
	line := strings.TrimSpace(ansi.Strip(raw))
	line = strings.TrimPrefix(line, "│")
	line = strings.TrimSuffix(line, "│")
	return strings.TrimSpace(line)
}

func TestHelpModalIndicatorLegend(t *testing.T) {
	t.Run("it renders a legend row for each indicator on the Sessions help", func(t *testing.T) {
		m := helpModelSessions(t, theme.MemberDark)
		m.modal = modalHelp
		view := m.viewSessionList()

		legend := sessionsIndicatorLegend(m.themeState.active, m.colourless)
		if len(legend) != 2 {
			t.Fatalf("want 2 legend entries (attached, pending), got %d", len(legend))
		}
		rows := legendPanelRows(t, view)
		if len(rows) != len(legend) {
			t.Fatalf("want %d legend rows below the key rows, got %d:\n%s", len(legend), len(rows), ansi.Strip(strings.Join(rows, "\n")))
		}
		for i, e := range legend {
			glyph := ansi.Strip(e.indicator)
			want := glyph + spaces(helpKeyColumnWidth-lipgloss.Width(glyph)) + helpColumnGap + e.label
			if got := innerText(rows[i]); got != want {
				t.Errorf("legend row %d = %q, want %q", i, got, want)
			}
		}

		keysEnd := strings.Index(ansi.Strip(view), "Quit")
		legendStart := strings.Index(ansi.Strip(view), legend[0].label)
		if keysEnd < 0 || legendStart < keysEnd {
			t.Errorf("the legend must sit below the key rows; Quit at %d, legend at %d", keysEnd, legendStart)
		}
	})

	t.Run("it names no tool in either legend label", func(t *testing.T) {
		wantLabels := []string{"Session attached", "Resume pending"}
		forEachThemeAndColourMode(t, func(t *testing.T, th theme.Theme, colourless bool) {
			legend := sessionsIndicatorLegend(th, colourless)
			labels := make([]string, 0, len(legend))
			for _, e := range legend {
				labels = append(labels, e.label)
			}
			if !slices.Equal(labels, wantLabels) {
				t.Errorf("the legend labels read\n got: %q\nwant: %q", labels, wantLabels)
			}
			rows := legendPanelRows(t, sessionsHelpPanel(t, th, colourless))
			for i, e := range legend {
				rest := innerText(rows[i])
				rest = strings.Replace(rest, ansi.Strip(e.indicator), "", 1)
				rest = strings.Replace(rest, e.label, "", 1)
				if strings.IndexFunc(rest, unicode.IsLetter) >= 0 {
					t.Errorf("legend row %d carries text beyond its indicator and declared label: %q", i, rest)
				}
				if e.label == "" || strings.HasSuffix(e.label, ".") || !unicode.IsUpper([]rune(e.label)[0]) {
					t.Errorf("legend label %q must be sentence case with no trailing period", e.label)
				}
			}
		})
	})

	t.Run("it leaves the Projects help body byte-identical", func(t *testing.T) {
		forEachThemeAndColourMode(t, func(t *testing.T, th theme.Theme, colourless bool) {
			if got, want := renderHelpModalContent(projectsKeymap(), th, colourless, nil), preLegendHelpPanel(projectsKeymap(), th, colourless); got != want {
				t.Errorf("Projects help panel changed:\ngot:\n%s\nwant:\n%s", got, want)
			}
		})

		m := helpModelProjects(t, theme.MemberDark)
		m.modal = modalHelp
		want := placeModalOnClearedCanvas(preLegendHelpPanel(m.projectsHelpKeymap(), m.themeState.active, m.colourless), m.contentWidth(), m.contentHeight())
		if got := m.viewProjectList(); got != want {
			t.Errorf("Projects help view must carry no legend:\ngot:\n%s\nwant:\n%s", ansi.Strip(got), ansi.Strip(want))
		}
	})

	t.Run("it leaves the preview help body byte-identical", func(t *testing.T) {
		forEachThemeAndColourMode(t, func(t *testing.T, th theme.Theme, colourless bool) {
			if got, want := renderHelpModalContent(previewKeymap(), th, colourless, nil), preLegendHelpPanel(previewKeymap(), th, colourless); got != want {
				t.Errorf("preview help panel changed:\ngot:\n%s\nwant:\n%s", got, want)
			}
		})

		m := newPreviewHelpModel(t, testDarkTheme(t), false)
		m, _ = pressPreviewKey(t, m, keyQuestionMark())
		view := ansi.Strip(m.View())
		for _, e := range sessionsIndicatorLegend(m.th, m.colourless) {
			if strings.Contains(view, e.label) {
				t.Errorf("preview help must carry no legend; found %q in:\n%s", e.label, view)
			}
		}
		for line := range strings.SplitSeq(ansi.Strip(preLegendHelpPanel(previewKeymap(), m.th, m.colourless)), "\n") {
			if !strings.Contains(view, line) {
				t.Errorf("preview help must composite the unchanged panel; line %q missing from:\n%s", line, view)
			}
		}
	})

	t.Run("it names the letters under NO_COLOR", func(t *testing.T) {
		rows := legendPanelRows(t, sessionsHelpPanel(t, testDarkTheme(t), true))
		for i, letter := range []string{"A", "P"} {
			got := innerText(rows[i])
			if !strings.HasPrefix(got, letter+" ") {
				t.Errorf("colourless legend row %d = %q, want it to lead with %q", i, got, letter)
			}
			if strings.Contains(got, rowIndicatorGlyph) {
				t.Errorf("colourless legend row %d must not show the dot: %q", i, got)
			}
		}
	})

	t.Run("it renders the legend indicators through the row's own renderer", func(t *testing.T) {
		forEachThemeAndColourMode(t, func(t *testing.T, th theme.Theme, colourless bool) {
			d := SessionDelegate{Theme: th, Colourless: colourless}
			want := []string{
				d.indicatorCluster(rowIndicators{attached: true}, false),
				d.indicatorCluster(rowIndicators{pending: true}, false),
			}
			legend := sessionsIndicatorLegend(th, colourless)
			rows := legendPanelRows(t, sessionsHelpPanel(t, th, colourless))
			for i, w := range want {
				if legend[i].indicator != w {
					t.Errorf("legend entry %d indicator = %q, want the row's own %q", i, legend[i].indicator, w)
				}
				if !strings.Contains(rows[i], w) {
					t.Errorf("legend row %d must carry the row renderer's bytes %q; got %q", i, w, rows[i])
				}
			}
		})
	})

	t.Run("it adds no keymap entry and advertises no key", func(t *testing.T) {
		m := helpModelSessions(t, theme.MemberDark)
		legend := sessionsIndicatorLegend(m.themeState.active, m.colourless)
		footer := ansi.Strip(renderSessionsFooter(m.sessionsHelpKeymap(), m.contentWidth(), m.themeState.active, m.colourless))
		for _, e := range legend {
			for _, k := range sessionsKeymap() {
				if k.Action == e.label || k.HelpAction == e.label {
					t.Errorf("legend label %q must not be a keymap entry", e.label)
				}
			}
			if strings.Contains(footer, e.label) {
				t.Errorf("the footer must not advertise legend label %q: %q", e.label, footer)
			}
		}
	})

	t.Run("it fits the content region at the smallest height the help modal renders at", func(t *testing.T) {
		forEachCanvasMode(t, func(t *testing.T, mode theme.Member) {
			m := helpModelSessions(t, mode)
			m.modal = modalHelp
			panel := renderHelpModalContent(m.sessionsHelpKeymap(), m.themeState.active, m.colourless, sessionsIndicatorLegend(m.themeState.active, m.colourless))
			if h, limit := lipgloss.Height(panel), m.contentHeight(); h > limit {
				t.Errorf("Sessions help with legend is %d rows, taller than the %d-row content region", h, limit)
			}
			if h, limit := lipgloss.Height(m.viewSessionList()), m.contentHeight(); h != limit {
				t.Errorf("Sessions help view is %d rows, want exactly the %d-row content region", h, limit)
			}
		})
	})

	t.Run("it still skips the ? self-entry", func(t *testing.T) {
		panel := ansi.Strip(sessionsHelpPanel(t, testDarkTheme(t), false))
		for line := range strings.SplitSeq(panel, "\n") {
			if strings.Contains(line, "help") || strings.HasPrefix(innerText(line), "?  ") {
				t.Errorf("the ? self-entry must stay out of the Sessions help; found %q", line)
			}
		}
	})
}
