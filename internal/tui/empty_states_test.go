package tui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/tui/theme"
)

// This file is the §11.1 empty-states gate (task 4-5). It pins the MV treatment of
// the genuinely-empty Sessions and Projects lists: a centred dim block glyph
// `▌ ▌ ▌` (text.faint) over the message (text.primary) over the hint (text.detail)
// on the owned canvas, with a FULLY-REPLACED footer drawn from the page's keymap
// descriptor (§12.1).
//
// The empty state renders ONLY when the underlying list has ZERO items AND no
// active filter — it is DISTINCT from the §7.3 over-filtered no-matches state
// (items exist, query filters to zero). Colour roles are pinned in exact
// mode-resolved SGR so a token swap is caught, not merely the glyph's presence.
// No t.Parallel() (the package-level mock convention + shared canvas helpers make
// parallelism unsafe across this package's tests).

// emptySessionsModel builds a Sessions model with ZERO sessions and no active
// filter (the §11.1 empty-sessions condition: Unfiltered state, empty list).
func emptySessionsModel(t *testing.T, mode theme.Mode) Model {
	t.Helper()
	appearance := prefs.AppearanceDark
	if mode == theme.Light {
		appearance = prefs.AppearanceLight
	}
	m := Build(Deps{Lister: fakeLister{}, Appearance: appearance})
	m.termWidth = filteringReskinWidth
	m.termHeight = filteringReskinHeight
	m.applySessions(nil)
	if m.sessionList.FilterState() != list.Unfiltered {
		t.Fatalf("precondition: filter state = %v, want Unfiltered (no active filter)", m.sessionList.FilterState())
	}
	if got := len(m.sessionList.VisibleItems()); got != 0 {
		t.Fatalf("precondition: %d visible items, want 0 (empty sessions)", got)
	}
	return m
}

// emptyProjectsModel builds a Projects-page model with ZERO projects and no active
// filter (the §11.1 empty-projects condition).
func emptyProjectsModel(t *testing.T, mode theme.Mode) Model {
	t.Helper()
	appearance := prefs.AppearanceDark
	if mode == theme.Light {
		appearance = prefs.AppearanceLight
	}
	m := Build(Deps{Lister: fakeLister{}, Appearance: appearance})
	m.termWidth = filteringReskinWidth
	m.termHeight = filteringReskinHeight
	m.applySessions(nil)
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

// TestEmptySessions_RendersGlyphMessageHint asserts §11.1: with zero sessions and
// no active filter the body shows the centred `▌ ▌ ▌` block glyph, the
// `No sessions yet` message, and the spec-exact hint.
func TestEmptySessions_RendersGlyphMessageHint(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		m := emptySessionsModel(t, mode)
		vis := ansi.Strip(m.View().Content)

		for _, want := range []string{
			emptySessionsGlyph,
			emptySessionsMessage,
			emptySessionsHint,
		} {
			if !strings.Contains(vis, want) {
				t.Errorf("[%v] empty-sessions body missing %q:\n%s", mode, want, vis)
			}
		}
		// The no-matches glyph/message must NOT appear (this is a distinct state).
		if strings.Contains(vis, noMatchesGlyph) || strings.Contains(vis, "No sessions match") {
			t.Errorf("[%v] empty-sessions state leaked the no-matches glyph/message:\n%s", mode, vis)
		}
	}
}

// TestEmptySessions_ReplacesFooterFromDescriptor asserts §11.1: the footer is FULLY
// REPLACED with `n new in cwd · x projects · / filter · ? help`, sourced from the
// Sessions keymap descriptor (§12.1). The standard condensed footer's keys that are
// NOT in this replaced set (navigate, attach, preview, switch view) must be absent.
func TestEmptySessions_ReplacesFooterFromDescriptor(t *testing.T) {
	m := emptySessionsModel(t, theme.Dark)
	vis := ansi.Strip(m.View().Content)

	for _, want := range []string{"n new in cwd", "x projects", "/ filter", "? help"} {
		if !strings.Contains(vis, want) {
			t.Errorf("empty-sessions footer missing replaced entry %q:\n%s", want, vis)
		}
	}
	// The standard footer is fully REPLACED, not just trimmed — its non-empty-state
	// Core keys must not appear.
	for _, banned := range []string{"navigate", "attach", "preview", "switch view"} {
		if strings.Contains(vis, banned) {
			t.Errorf("empty-sessions footer must FULLY replace the standard footer (found %q):\n%s", banned, vis)
		}
	}
}

// TestEmptySessions_FooterCopyFromDescriptor pins the replaced footer's copy against
// the Sessions keymap descriptor: the labels are READ from the descriptor, not
// hand-authored, so a label change in the descriptor flows through.
func TestEmptySessions_FooterCopyFromDescriptor(t *testing.T) {
	footer := renderEmptySessionsFooter(referenceFooterWidth, theme.Dark, false)
	keyRow := footerVisible(strings.Split(footer, "\n")[1])

	// The four entries' Action labels come straight off the descriptor.
	want := map[string]string{"n": "new in cwd", "x": "projects", "/": "filter", "?": "help"}
	for key, action := range want {
		entry := key + " " + action
		if !strings.Contains(keyRow, entry) {
			t.Errorf("empty-sessions footer missing descriptor entry %q:\n%s", entry, keyRow)
		}
	}
	// ? help is right-aligned (the trailing anchor).
	if !strings.HasSuffix(strings.TrimRight(keyRow, " "), "help") {
		t.Errorf("? help must be the trailing right-aligned entry:\n%s", keyRow)
	}
}

// TestEmptySessions_FooterTokenColours asserts §3.4 / §11.1: key glyphs render in
// accent.blue, labels in text.detail, the ? glyph in accent.violet, over a 1px
// border.footer top rule — every colour via its §2.9 token.
func TestEmptySessions_FooterTokenColours(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		footer := renderEmptySessionsFooter(referenceFooterWidth, mode, false)

		if seq := tokenFgSeq(t, theme.MV.AccentBlue, mode); !strings.Contains(footer, seq) {
			t.Errorf("[%v] empty-sessions footer missing accent.blue key-glyph role %q", mode, seq)
		}
		if seq := tokenFgSeq(t, theme.MV.TextDetail, mode); !strings.Contains(footer, seq) {
			t.Errorf("[%v] empty-sessions footer missing text.detail label role %q", mode, seq)
		}
		if seq := tokenFgSeq(t, theme.MV.AccentViolet, mode); !strings.Contains(footer, seq) {
			t.Errorf("[%v] empty-sessions footer missing accent.violet ? glyph role %q", mode, seq)
		}
		if seq := tokenFgSeq(t, theme.MV.BorderFooter, mode); !strings.Contains(footer, seq) {
			t.Errorf("[%v] empty-sessions footer missing border.footer rule role %q", mode, seq)
		}
	}
}

// TestEmptyProjects_RendersGlyphMessageHint asserts §11.1: the empty-projects state
// mirrors the sessions pattern — the centred block glyph, `No projects yet`, and the
// open-a-directory hint, with the projects-relevant replaced footer.
func TestEmptyProjects_RendersGlyphMessageHint(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		m := emptyProjectsModel(t, mode)
		vis := ansi.Strip(m.View().Content)

		for _, want := range []string{
			emptyProjectsGlyph,
			emptyProjectsMessage,
			emptyProjectsHint,
		} {
			if !strings.Contains(vis, want) {
				t.Errorf("[%v] empty-projects body missing %q:\n%s", mode, want, vis)
			}
		}
	}
}

// TestEmptyProjects_ReplacesFooterFromProjectsDescriptor asserts §11.1: the
// empty-projects footer is replaced with the projects-relevant keys (n / x / / / ?)
// drawn from the Projects keymap descriptor — the Projects `x` is `sessions`, not
// `projects`, so the descriptor wiring (not a hard-coded copy) is exercised.
func TestEmptyProjects_ReplacesFooterFromProjectsDescriptor(t *testing.T) {
	m := emptyProjectsModel(t, theme.Dark)
	vis := ansi.Strip(m.View().Content)

	for _, want := range []string{"n new in cwd", "x sessions", "/ filter", "? help"} {
		if !strings.Contains(vis, want) {
			t.Errorf("empty-projects footer missing replaced entry %q:\n%s", want, vis)
		}
	}
	// Projects `x` is "sessions" (not "projects") — pins descriptor sourcing.
	if strings.Contains(vis, "x projects") {
		t.Errorf("empty-projects footer must read `x sessions` (Projects descriptor), not `x projects`:\n%s", vis)
	}
	for _, banned := range []string{"navigate", "new session", "edit"} {
		if strings.Contains(vis, banned) {
			t.Errorf("empty-projects footer must FULLY replace the standard footer (found %q):\n%s", banned, vis)
		}
	}
}

// TestEmptyStates_OnlyRenderWithZeroItems asserts §11.1 / §7.3: the empty state
// renders ONLY when the underlying list has zero items — with sessions present the
// state never renders.
func TestEmptyStates_OnlyRenderWithZeroItems(t *testing.T) {
	// Sessions present -> no empty state.
	m := filteringTestModel(t, theme.Dark)
	if m.sessionListEmpty() {
		t.Errorf("sessionListEmpty()=true with sessions present, want false")
	}
	vis := ansi.Strip(m.View().Content)
	if strings.Contains(vis, emptySessionsMessage) {
		t.Errorf("empty-sessions message rendered while sessions exist:\n%s", vis)
	}

	// Zero sessions, no filter -> empty state.
	empty := emptySessionsModel(t, theme.Dark)
	if !empty.sessionListEmpty() {
		t.Errorf("sessionListEmpty()=false with zero sessions and no filter, want true")
	}
}

// TestEmptyStates_DistinctFromNoMatches asserts §11.1 / §7.3: the empty state (zero
// items, no query) is a DISTINCT surface from the no-matches state (items exist,
// active query filters to zero). The two predicates are mutually exclusive.
func TestEmptyStates_DistinctFromNoMatches(t *testing.T) {
	// No-matches: items exist, active query -> no-matches true, empty false.
	noMatch := enterNoMatches(t, theme.Dark, "zzqx")
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

	// Empty: zero items, no query -> empty true, no-matches false.
	empty := emptySessionsModel(t, theme.Dark)
	if !empty.sessionListEmpty() {
		t.Fatalf("precondition: expected empty-sessions state")
	}
	if empty.sessionListNoMatches() {
		t.Errorf("sessionListNoMatches()=true during the empty-sessions state; the two surfaces must be distinct")
	}
}

// TestEmptyStates_NotRenderedWhileFiltering asserts §11.1: the empty state does NOT
// render while a filter is active (Filtering/FilterApplied) — even with zero items
// the no-matches surface (or the live filter input) owns that state, not the empty
// state.
func TestEmptyStates_NotRenderedWhileFiltering(t *testing.T) {
	// Active filter with a non-empty query matching zero -> no-matches, not empty.
	m := enterNoMatches(t, theme.Dark, "zzqx")
	if m.sessionListEmpty() {
		t.Errorf("sessionListEmpty() must be false while a filter is active:\n")
	}
}

// TestEmptyStates_ColourRoles asserts §2.9: the glyph reads text.faint, the message
// text.primary, and the hint text.detail — pinned in exact mode-resolved SGR.
func TestEmptyStates_ColourRoles(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		body := renderEmptyStateBody(emptySessionsGlyph, emptySessionsMessage, emptySessionsHint, filteringReskinWidth, 20, mode, false)

		if seq := tokenFgSeq(t, theme.MV.TextFaint, mode); !strings.Contains(body, seq) {
			t.Errorf("[%v] empty-state glyph missing text.faint SGR %q:\n%s", mode, seq, escSeq(body))
		}
		if seq := tokenFgSeq(t, theme.MV.TextPrimary, mode); !strings.Contains(body, seq) {
			t.Errorf("[%v] empty-state message missing text.primary SGR %q:\n%s", mode, seq, escSeq(body))
		}
		if seq := tokenFgSeq(t, theme.MV.TextDetail, mode); !strings.Contains(body, seq) {
			t.Errorf("[%v] empty-state hint missing text.detail SGR %q:\n%s", mode, seq, escSeq(body))
		}
	}
}

// TestEmptyStates_ColourlessLegibleOnNativeBg asserts §2.5: under NO_COLOR the
// empty-state body and footer render colourless on the native bg — no canvas SGR,
// no foreground hue — while the glyph/message/hint/footer copy stay intact.
func TestEmptyStates_ColourlessLegibleOnNativeBg(t *testing.T) {
	body := renderEmptyStateBody(emptySessionsGlyph, emptySessionsMessage, emptySessionsHint, filteringReskinWidth, 20, theme.Dark, true)
	for _, want := range []string{emptySessionsGlyph, emptySessionsMessage, emptySessionsHint} {
		if !ansiContains(body, want) {
			t.Errorf("colourless empty-state body dropped %q:\n%s", want, ansi.Strip(body))
		}
	}
	if seq := canvasSeq(t, theme.Dark); strings.Contains(body, seq) {
		t.Errorf("colourless empty-state body still paints the canvas background %q", seq)
	}
	for _, tok := range []theme.Token{theme.MV.TextFaint, theme.MV.TextPrimary, theme.MV.TextDetail} {
		if seq := tokenFgSeq(t, tok, theme.Dark); strings.Contains(body, seq) {
			t.Errorf("colourless empty-state body still emits a foreground role %q", seq)
		}
	}

	footer := renderEmptySessionsFooter(referenceFooterWidth, theme.Dark, true)
	keyRow := footerVisible(strings.Split(footer, "\n")[1])
	for _, want := range []string{"n new in cwd", "x projects", "/ filter", "? help"} {
		if !strings.Contains(keyRow, want) {
			t.Errorf("colourless empty-sessions footer dropped %q:\n%s", want, keyRow)
		}
	}
	if seq := canvasSeq(t, theme.Dark); strings.Contains(footer, seq) {
		t.Errorf("colourless empty-sessions footer still paints the canvas background %q", seq)
	}
}

// TestEmptyStates_OneRowPerDelegateInvariant asserts §11.1 / §3.5: the empty-state
// body replaces the list body sized against the SAME height budget (zero delegate
// rows), so the composed view never exceeds termHeight — the one-row-per-delegate
// pagination invariant is unperturbed.
func TestEmptyStates_OneRowPerDelegateInvariant(t *testing.T) {
	m := emptySessionsModel(t, theme.Dark)
	view := m.View().Content
	if got := lipgloss.Height(view); got > m.termHeight {
		t.Errorf("empty-sessions composed view height = %d, exceeds termHeight = %d:\n%s", got, m.termHeight, ansi.Strip(view))
	}
}

// ansiContains reports whether the ANSI-stripped string contains want.
func ansiContains(s, want string) bool {
	return strings.Contains(ansi.Strip(s), want)
}
