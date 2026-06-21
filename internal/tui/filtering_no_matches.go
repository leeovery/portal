package tui

import (
	"fmt"

	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/tui/theme"
)

// §7.3 over-filtered no-matches state. When an ACTIVE non-empty filter query
// matches zero sessions, the Sessions body renders a centred empty state in place
// of the (empty) bubbles/list body: a dim null-set glyph (text.faint), the
// query-interpolated `No sessions match "<query>"` message (text.primary), and the
// `⌫ to widen the search · esc to clear the filter` hint (text.detail). The footer
// stays in the input-active form, reduced (no `browse results` entry — there are no
// results to browse, §7.3).
//
// This state is DISTINCT from the §11.1 empty-sessions state (no sessions exist at
// all, no active query): the two paths are NOT merged. sessionListNoMatches REQUIRES
// an active non-empty query, so a model with zero sessions and no filter (the §11.1
// condition) never enters this state. All colours flow from §2.9 role tokens; the
// glyph/message/hint are leaf-painted on the owned canvas — no literal hex.

// noMatchesGlyph is the dim null-set glyph centred above the message (the §7.3
// reference shows ∅ in text.faint). A single rune so it centres cleanly.
const noMatchesGlyph = "∅"

// noMatchesHint is the §7.3 widen/clear hint rendered beneath the message in
// text.detail. The widen key is the ⌫ backspace GLYPH (per the reference), not the
// literal word "backspace" — backspace deletes a query char (= widen the search),
// Esc clears the filter. The hint only documents the engine's existing behaviour;
// it changes nothing.
const noMatchesHint = "⌫ to widen the search · esc to clear the filter"

// formatNoMatchesMessage returns the §7.3 message wording with the query
// interpolated. Literal straight double-quote bytes wrap the query — never %q —
// mirroring formatSessionGoneFlash, so the query renders byte-exact regardless of
// its content (spaces, dashes, unicode, embedded quotes).
func formatNoMatchesMessage(query string) string {
	return fmt.Sprintf(`No sessions match "%s"`, query)
}

// sessionListNoMatches reports whether the §7.3 over-filtered no-matches state is
// active: an ACTIVE filter (FilterState is Filtering OR FilterApplied) with a
// non-empty query AND zero visible items. The active-non-empty-query requirement is
// what keeps this DISTINCT from the §11.1 empty-sessions state (no sessions + no
// active query never satisfies this predicate).
func (m Model) sessionListNoMatches() bool {
	st := m.sessionList.FilterState()
	if st != list.Filtering && st != list.FilterApplied {
		return false
	}
	if m.sessionList.FilterValue() == "" {
		return false
	}
	return len(m.sessionList.VisibleItems()) == 0
}

// renderNoMatchesBody renders the §7.3 centred empty state into a width×height
// block: the null-set glyph (text.faint) over the query-interpolated message
// (text.primary) over the widen/clear hint (text.detail), centred both ways via
// lipgloss.Place. Every run carries the owned canvas Background (headerStyle), and
// the surrounding placed gap is canvas-painted (headerCanvasBg) so no terminal-bg
// island bleeds through. Under the NO_COLOR carve-out the hues and the canvas drop.
func renderNoMatchesBody(query string, width, height int, mode theme.Mode, colourless bool) string {
	w := headerWidthOrFallback(width)
	glyph := headerStyle(theme.MV.TextFaint, mode, colourless).Render(noMatchesGlyph)
	message := headerStyle(theme.MV.TextPrimary, mode, colourless).Bold(true).Render(formatNoMatchesMessage(query))
	hint := headerStyle(theme.MV.TextDetail, mode, colourless).Render(noMatchesHint)
	stack := lipgloss.JoinVertical(lipgloss.Center, glyph, "", message, hint)
	return lipgloss.Place(
		w, height,
		lipgloss.Center, lipgloss.Center,
		stack,
		lipgloss.WithWhitespaceStyle(headerCanvasBg(mode, colourless)),
	)
}

// noMatchesFooterEntries returns the §7.3 footer entries: the input-active footer
// (task 2-8) WITHOUT the `browse results` entry — there are no results to browse, so
// the no-matches footer reads `type to filter · esc clear`. It REUSES the
// input-active footer machinery (filteringFooterEntries) and drops the
// browse-results entry, so the per-glyph colours (the accent.orange `type` action
// word, the text.detail `esc` key + labels) stay byte-consistent with the
// input-active footer.
func noMatchesFooterEntries() []filterFooterEntry {
	src := filteringFooterEntries()
	entries := make([]filterFooterEntry, 0, len(src))
	for _, e := range src {
		if e.Label == "browse results" {
			continue
		}
		entries = append(entries, e)
	}
	return entries
}

// renderNoMatchesFooter renders the §7.3 reduced input-active footer for the given
// content width and resolved canvas mode (and the NO_COLOR carve-out).
func renderNoMatchesFooter(width int, mode theme.Mode, colourless bool) string {
	return renderFilterFooter(noMatchesFooterEntries(), width, mode, colourless)
}
