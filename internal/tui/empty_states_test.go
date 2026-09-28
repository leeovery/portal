package tui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/theme"
)

func emptySessionsModel(t *testing.T, th theme.Theme) Model {
	t.Helper()
	m := Build(Deps{Lister: fakeLister{}, Theme: theme.ConstantNomination(th)})
	m.termWidth = filteringReskinWidth
	m.termHeight = filteringReskinHeight
	m.applySessions(nil, nil)
	if m.sessionList.FilterState() != list.Unfiltered {
		t.Fatalf("precondition: filter state = %v, want Unfiltered (no active filter)", m.sessionList.FilterState())
	}
	if got := len(m.sessionList.VisibleItems()); got != 0 {
		t.Fatalf("precondition: %d visible items, want 0 (empty sessions)", got)
	}
	return m
}

func emptyProjectsModel(t *testing.T, th theme.Theme) Model {
	t.Helper()
	m := Build(Deps{Lister: fakeLister{}, Theme: theme.ConstantNomination(th)})
	m.termWidth = filteringReskinWidth
	m.termHeight = filteringReskinHeight
	m.applySessions(nil, nil)
	model, _ := m.Update(ProjectsLoadedMsg{Projects: nil})
	m = model.(Model)
	m.activePage = PageProjects
	if m.projectList.FilterState() != list.Unfiltered {
		t.Fatalf("precondition: project filter state = %v, want Unfiltered", m.projectList.FilterState())
	}
	if got := len(m.projectList.VisibleItems()); got != 0 {
		t.Fatalf("precondition: %d visible project items, want 0 (empty projects)", got)
	}
	return m
}

func TestEmptySessions_RendersGlyphMessageHint(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		m := emptySessionsModel(t, th)
		vis := ansi.Strip(m.View().Content)

		for _, want := range []string{
			emptySessionsGlyph,
			emptySessionsMessage,
			emptySessionsHint,
		} {
			if !strings.Contains(vis, want) {
				t.Errorf("[%v] empty-sessions body missing %q:\n%s", themeLabel(th), want, vis)
			}
		}
		if strings.Contains(vis, noMatchesGlyph) || strings.Contains(vis, "No sessions match") {
			t.Errorf("[%v] empty-sessions state leaked the no-matches glyph/message:\n%s", themeLabel(th), vis)
		}
	}
}

func TestEmptySessions_ReplacesFooterFromDescriptor(t *testing.T) {
	m := emptySessionsModel(t, testDarkTheme(t))
	vis := ansi.Strip(m.View().Content)

	for _, want := range []string{"n new in cwd", "x projects", "/ filter", "? help"} {
		if !strings.Contains(vis, want) {
			t.Errorf("empty-sessions footer missing replaced entry %q:\n%s", want, vis)
		}
	}
	for _, banned := range []string{"navigate", "attach", "preview", "switch view"} {
		if strings.Contains(vis, banned) {
			t.Errorf("empty-sessions footer must FULLY replace the standard footer (found %q):\n%s", banned, vis)
		}
	}
}

func TestEmptySessions_FooterCopyFromDescriptor(t *testing.T) {
	footer := renderEmptySessionsFooter(referenceFooterWidth, testDarkTheme(t), false)
	keyRow := footerVisible(strings.Split(footer, "\n")[1])

	want := map[string]string{"n": "new in cwd", "x": "projects", "/": "filter", "?": "help"}
	for key, action := range want {
		entry := key + " " + action
		if !strings.Contains(keyRow, entry) {
			t.Errorf("empty-sessions footer missing descriptor entry %q:\n%s", entry, keyRow)
		}
	}
	if !strings.HasSuffix(strings.TrimRight(keyRow, " "), "help") {
		t.Errorf("? help must be the trailing right-aligned entry:\n%s", keyRow)
	}
}

func TestEmptySessions_FooterTokenColours(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		footer := renderEmptySessionsFooter(referenceFooterWidth, th, false)

		if seq := tokenFgSeq(t, th.AccentKey); !strings.Contains(footer, seq) {
			t.Errorf("[%v] empty-sessions footer missing accent.key key-glyph role %q", themeLabel(th), seq)
		}
		if seq := tokenFgSeq(t, th.TextMuted); !strings.Contains(footer, seq) {
			t.Errorf("[%v] empty-sessions footer missing text.muted label role %q", themeLabel(th), seq)
		}
		if seq := tokenFgSeq(t, th.AccentPrimary); !strings.Contains(footer, seq) {
			t.Errorf("[%v] empty-sessions footer missing accent.primary ? glyph role %q", themeLabel(th), seq)
		}
		if seq := tokenFgSeq(t, th.Border); !strings.Contains(footer, seq) {
			t.Errorf("[%v] empty-sessions footer missing border rule role %q", themeLabel(th), seq)
		}
	}
}

func TestEmptyProjects_RendersGlyphMessageHint(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		m := emptyProjectsModel(t, th)
		vis := ansi.Strip(m.View().Content)

		for _, want := range []string{
			emptyProjectsGlyph,
			emptyProjectsMessage,
			emptyProjectsHint,
		} {
			if !strings.Contains(vis, want) {
				t.Errorf("[%v] empty-projects body missing %q:\n%s", themeLabel(th), want, vis)
			}
		}
	}
}

func TestEmptyProjects_ReplacesFooterFromProjectsDescriptor(t *testing.T) {
	m := emptyProjectsModel(t, testDarkTheme(t))
	vis := ansi.Strip(m.View().Content)

	for _, want := range []string{"n new in cwd", "x sessions", "/ filter", "? help"} {
		if !strings.Contains(vis, want) {
			t.Errorf("empty-projects footer missing replaced entry %q:\n%s", want, vis)
		}
	}
	if strings.Contains(vis, "x projects") {
		t.Errorf("empty-projects footer must read `x sessions` (Projects descriptor), not `x projects`:\n%s", vis)
	}
	for _, banned := range []string{"navigate", "new session", "edit"} {
		if strings.Contains(vis, banned) {
			t.Errorf("empty-projects footer must FULLY replace the standard footer (found %q):\n%s", banned, vis)
		}
	}
}

func TestEmptyStates_OnlyRenderWithZeroItems(t *testing.T) {
	m := filteringTestModel(t, testDarkTheme(t))
	if m.sessionListEmpty() {
		t.Errorf("sessionListEmpty()=true with sessions present, want false")
	}
	vis := ansi.Strip(m.View().Content)
	if strings.Contains(vis, emptySessionsMessage) {
		t.Errorf("empty-sessions message rendered while sessions exist:\n%s", vis)
	}

	empty := emptySessionsModel(t, testDarkTheme(t))
	if !empty.sessionListEmpty() {
		t.Errorf("sessionListEmpty()=false with zero sessions and no filter, want true")
	}
}

func TestEmptyStates_DistinctFromNoMatches(t *testing.T) {
	noMatch := enterNoMatches(t, testDarkTheme(t), "zzqx")
	if !noMatch.sessionListNoMatches() {
		t.Fatalf("precondition: expected no-matches state")
	}
	if noMatch.sessionListEmpty() {
		t.Errorf("sessionListEmpty()=true during the no-matches state; the two surfaces must be distinct")
	}
	vis := ansi.Strip(noMatch.View().Content)
	if strings.Contains(vis, emptySessionsMessage) {
		t.Errorf("empty-sessions message must NOT render in the no-matches state:\n%s", vis)
	}

	empty := emptySessionsModel(t, testDarkTheme(t))
	if !empty.sessionListEmpty() {
		t.Fatalf("precondition: expected empty-sessions state")
	}
	if empty.sessionListNoMatches() {
		t.Errorf("sessionListNoMatches()=true during the empty-sessions state; the two surfaces must be distinct")
	}
}

func TestEmptyStates_NotRenderedWhileFiltering(t *testing.T) {
	m := enterNoMatches(t, testDarkTheme(t), "zzqx")
	if m.sessionListEmpty() {
		t.Errorf("sessionListEmpty() must be false while a filter is active:\n")
	}
}

func TestEmptyStates_ColourRoles(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		body := renderEmptyStateBody(emptySessionsGlyph, emptySessionsMessage, emptySessionsHint, filteringReskinWidth, 20, th, false)

		if seq := tokenFgSeq(t, th.TextFaint); !strings.Contains(body, seq) {
			t.Errorf("[%v] empty-state glyph missing text.faint SGR %q:\n%s", themeLabel(th), seq, escSeq(body))
		}
		if seq := tokenFgSeq(t, th.TextPrimary); !strings.Contains(body, seq) {
			t.Errorf("[%v] empty-state message missing text.primary SGR %q:\n%s", themeLabel(th), seq, escSeq(body))
		}
		if seq := tokenFgSeq(t, th.TextMuted); !strings.Contains(body, seq) {
			t.Errorf("[%v] empty-state hint missing text.muted SGR %q:\n%s", themeLabel(th), seq, escSeq(body))
		}
	}
}

func TestEmptyStates_ColourlessLegibleOnNativeBg(t *testing.T) {
	body := renderEmptyStateBody(emptySessionsGlyph, emptySessionsMessage, emptySessionsHint, filteringReskinWidth, 20, testDarkTheme(t), true)
	for _, want := range []string{emptySessionsGlyph, emptySessionsMessage, emptySessionsHint} {
		if !ansiContains(body, want) {
			t.Errorf("colourless empty-state body dropped %q:\n%s", want, ansi.Strip(body))
		}
	}
	if seq := canvasSeq(t, testDarkTheme(t)); strings.Contains(body, seq) {
		t.Errorf("colourless empty-state body still paints the canvas background %q", seq)
	}
	for _, tok := range []theme.Token{testDarkTheme(t).TextFaint, testDarkTheme(t).TextPrimary, testDarkTheme(t).TextMuted} {
		if seq := tokenFgSeq(t, tok); strings.Contains(body, seq) {
			t.Errorf("colourless empty-state body still emits a foreground role %q", seq)
		}
	}

	footer := renderEmptySessionsFooter(referenceFooterWidth, testDarkTheme(t), true)
	keyRow := footerVisible(strings.Split(footer, "\n")[1])
	for _, want := range []string{"n new in cwd", "x projects", "/ filter", "? help"} {
		if !strings.Contains(keyRow, want) {
			t.Errorf("colourless empty-sessions footer dropped %q:\n%s", want, keyRow)
		}
	}
	if seq := canvasSeq(t, testDarkTheme(t)); strings.Contains(footer, seq) {
		t.Errorf("colourless empty-sessions footer still paints the canvas background %q", seq)
	}
}

func TestEmptyStates_OneRowPerDelegateInvariant(t *testing.T) {
	m := emptySessionsModel(t, testDarkTheme(t))
	view := m.View().Content
	if got := lipgloss.Height(view); got > m.termHeight {
		t.Errorf("empty-sessions composed view height = %d, exceeds termHeight = %d:\n%s", got, m.termHeight, ansi.Strip(view))
	}
}

func ansiContains(s, want string) bool {
	return strings.Contains(ansi.Strip(s), want)
}
