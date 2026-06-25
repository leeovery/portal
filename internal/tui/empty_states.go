package tui

import (
	"strings"

	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/tui/theme"
)

// §11.1 empty states. When a list is GENUINELY empty — zero items AND no active
// filter — the body renders a centred empty state in place of the (empty)
// bubbles/list body: a dim block glyph (text.faint), a message (text.primary), and
// a hint (text.detail), and the footer is FULLY REPLACED by the keys relevant with
// no items, drawn from the page's keymap descriptor (§12.1).
//
// This state is DISTINCT from the §7.3 over-filtered no-matches state (items exist,
// an active query filters to zero): the empty-state predicates REQUIRE the
// Unfiltered state, so a model with an active filter never enters the empty state
// (the no-matches surface or the live filter input owns that case). The two paths
// share the SAME centred-empty-state body helper (renderEmptyStateBody) for layout,
// but stay separate surfaces with separate copy. All colours flow from §2.9 role
// tokens; the glyph/message/hint are leaf-painted on the owned canvas — no literal
// hex.

// emptySessionsGlyph is the §11.1 dim block glyph centred above the empty-sessions
// message: three `▌` block glyphs spaced apart (`▌ ▌ ▌`), rendered in text.faint.
const emptySessionsGlyph = "▌ ▌ ▌"

// emptySessionsMessage is the §11.1 spec-exact empty-sessions message (text.primary).
const emptySessionsMessage = "No sessions yet"

// emptySessionsHint is the §11.1 spec-exact empty-sessions hint (text.detail). Note
// the `·` middot separator and `x for projects` (NOT `/` and NOT `p`).
const emptySessionsHint = "Press n to start one in the current directory · x for projects"

// emptyProjectsGlyph mirrors the empty-sessions glyph (§11.1 "mirrors it").
const emptyProjectsGlyph = "▌ ▌ ▌"

// emptyProjectsMessage is the §11.1 spec-exact empty-projects message (text.primary).
const emptyProjectsMessage = "No projects yet"

// emptyProjectsHint is the §11.1 open-a-directory hint (text.detail) — a sensible
// mirror of the empty-sessions hint (the spec mocks no exact string for projects).
const emptyProjectsHint = "Press n to start one in the current directory · x for sessions"

// renderEmptyStateBody is the SHARED §11.1 / §7.3 centred-empty-state renderer: a
// dim glyph (text.faint) over a message (text.primary, bold) over a hint
// (text.detail), centred both ways via lipgloss.Place into a width×height block.
// Every run carries the owned canvas Background (headerStyle), and the surrounding
// placed gap is canvas-painted (headerCanvasBg) so no terminal-bg island bleeds
// through. Under the NO_COLOR carve-out (§2.5) the hues and the canvas drop, leaving
// the glyph/message/hint legible on the terminal's native fg/bg. Both the §11.1
// empty states and the §7.3 no-matches state route through here, so the centring +
// sizing + token treatment can never drift between them.
func renderEmptyStateBody(glyph, message, hint string, width, height int, mode theme.Mode, colourless bool) string {
	w := headerWidthOrFallback(width)
	g := headerStyle(theme.MV.TextFaint, mode, colourless).Render(glyph)
	msg := headerStyle(theme.MV.TextPrimary, mode, colourless).Bold(true).Render(message)
	h := headerStyle(theme.MV.TextDetail, mode, colourless).Render(hint)
	stack := lipgloss.JoinVertical(lipgloss.Center, g, "", msg, h)
	return lipgloss.Place(
		w, height,
		lipgloss.Center, lipgloss.Center,
		stack,
		lipgloss.WithWhitespaceStyle(headerCanvasBg(mode, colourless)),
	)
}

// sessionListEmpty reports whether the §11.1 empty-sessions state is active: the
// session list has ZERO visible items AND no active filter (FilterState is
// Unfiltered). The Unfiltered requirement is what keeps this DISTINCT from the §7.3
// no-matches state (which requires an active non-empty query) and from the live
// filter input — a model with an active filter never satisfies this predicate.
func (m Model) sessionListEmpty() bool {
	if m.sessionList.FilterState() != list.Unfiltered {
		return false
	}
	return len(m.sessionList.VisibleItems()) == 0
}

// projectListEmpty reports whether the §11.1 empty-projects state is active: the
// project list has ZERO visible items AND no active filter (FilterState is
// Unfiltered). Mirrors sessionListEmpty.
func (m Model) projectListEmpty() bool {
	if m.projectList.FilterState() != list.Unfiltered {
		return false
	}
	return len(m.projectList.VisibleItems()) == 0
}

// replaceListBodyWithEmptyState swaps the list BODY (every row below the title row)
// for a centred §11.1 empty state, preserving the title row (the first line)
// byte-for-byte. The body block is rendered at the SAME height the bubbles/list body
// occupies (Height()−1, the list height minus the title row), so the composed view
// height is unchanged and the §3.5 one-row-per-delegate pagination invariant is
// unaffected — the body is empty here anyway, this just paints guidance into the
// rows the empty list would otherwise leave blank. Mirrors
// replaceListBodyWithNoMatches.
func (m Model) replaceListBodyWithEmptyState(listView string, listHeight int, glyph, message, hint string) string {
	bodyHeight := max(listHeight-1, 1) // minus the title row
	body := renderEmptyStateBody(glyph, message, hint, m.contentWidth(), bodyHeight, m.canvasMode, m.colourless)
	idx := strings.IndexByte(listView, '\n')
	if idx < 0 {
		// Degenerate single-line listView (no body to replace): append the body.
		return listView + "\n" + body
	}
	return listView[:idx+1] + body
}

// emptyFooterKeys is the ordered set of key glyphs the §11.1 empty-state footer
// pins, in render order: the left cluster (n · x · /) then the right-aligned ? help.
// Both pages' empty-state footers select these SAME keys; the LABELS come from the
// per-page descriptor (so Sessions `x projects` vs Projects `x sessions` follow the
// descriptor, not a hard-coded copy).
var emptyFooterKeys = []string{"n", "x", "/", "?"}

// emptyFooterDescriptor selects the §11.1 empty-state footer entries from a page's
// keymap descriptor (the single source of truth, §12.1). It pulls the emptyFooterKeys
// entries BY KEY off the descriptor — so the labels are whatever the descriptor
// carries — and marks them Core (the right-aligned ? help keeps its RightAligned
// flag) so the shared renderCondensedFooter renders exactly this set, fully replacing
// the standard footer. Entries are returned in emptyFooterKeys order so the row reads
// `n … · x … · / …` with ? help pinned right.
func emptyFooterDescriptor(keymap []keymapEntry) []keymapEntry {
	byKey := make(map[string]keymapEntry, len(keymap))
	for _, e := range keymap {
		byKey[e.Key] = e
	}
	entries := make([]keymapEntry, 0, len(emptyFooterKeys))
	for _, k := range emptyFooterKeys {
		e, ok := byKey[k]
		if !ok {
			continue
		}
		// Force Core membership so renderCondensedFooter includes the entry (n is
		// help-only in the standard footer; the empty-state footer promotes it). The
		// descriptor's RightAligned flag is preserved so ? help stays the right anchor.
		e.Core = true
		entries = append(entries, e)
	}
	return entries
}

// renderEmptySessionsFooter renders the §11.1 empty-sessions footer:
// `n new in cwd · x projects · / filter · ? help` — the four relevant Sessions
// bindings selected from the Sessions keymap descriptor (§12.1) and rendered through
// the SAME condensed-footer machinery (key glyphs accent.blue, labels text.detail,
// the ? glyph accent.violet, over the 1px border.footer rule). It FULLY REPLACES the
// standard footer for the empty-sessions state.
func renderEmptySessionsFooter(width int, mode theme.Mode, colourless bool) string {
	return renderCondensedFooter(emptyFooterDescriptor(sessionsKeymap()), width, mode, colourless)
}

// renderEmptyProjectsFooter renders the §11.1 empty-projects footer:
// `n new in cwd · x sessions · / filter · ? help` — the projects-relevant bindings
// selected from the Projects keymap descriptor (§12.1). Because the labels come from
// the descriptor, the Projects `x` reads `sessions` (not `projects`), mirroring the
// pattern through the SAME machinery.
func renderEmptyProjectsFooter(width int, mode theme.Mode, colourless bool) string {
	return renderCondensedFooter(emptyFooterDescriptor(projectsKeymap()), width, mode, colourless)
}
