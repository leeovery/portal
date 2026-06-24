package tui

import (
	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/tui/theme"
)

// The §7.1 two contextual filter footers. While a filter mode is active these
// REPLACE the §3.4 condensed Sessions footer (renderSessionsFooter):
//
//   - input-active (FilterState == Filtering):
//     `type to filter · ↵/↓ browse results · esc clear` + right-aligned `? help`
//   - list-active (FilterState == FilterApplied):
//     `↵ attach · ↑↓ navigate · esc clear filter` + right-aligned `? help`
//
// They reuse the §3.4 footer machinery (the 1px border.footer top rule, the dot
// separator, the canvas-painted flex spacer, the right-aligned `? help` anchor)
// so the rendered chrome stays byte-consistent with the standard footer — only
// the entries and the per-entry key colours differ. Per §7.1 each filter footer
// carries ONE accent.orange action word (`type` for input-active, `esc` for
// list-active), the nav glyphs in accent.blue, and the labels in text.detail.
//
// Every cell carries the owned canvas background (leaf .Background(canvas), §1)
// via the shared header helpers, so the spacer gap is canvas-painted with no
// terminal-bg island. Under the NO_COLOR carve-out (§2.5) every hue and the canvas
// drop; the glyphs stay structurally distinct on the terminal's native fg/bg.

// keyGlyph is one coloured run inside a filter-footer entry's key cluster: the
// glyph text and the token it renders in. A key may be ONE glyph (e.g. `esc`,
// `↑↓`) or several (e.g. `↵ / ↓`, where the arrows are accent.blue and the `/`
// separator is text.detail) — matching the §7.1 references where the
// "browse results" commit key reads as blue arrows around a quiet `/`.
type keyGlyph struct {
	Text string
	Tok  theme.Token
}

// filterFooterEntry is one entry in a contextual filter footer: a key cluster (one
// or more coloured glyphs) and the action label (text.detail). It mirrors the
// shape renderFooterEntry consumes but lets each entry pin its OWN per-glyph key
// colour (the accent.orange action word, the accent.blue nav glyphs, the quiet
// text.detail `/` separator), which the descriptor-driven sessionsKeymap path does
// not need.
//
// BrowseResults structurally tags the input-active footer's `browse results` entry
// (mirroring the sessionsKeymap descriptor's Core-flag membership model). The §7.3
// no-matches footer drops this entry — there are no results to browse — via the
// flag (dropBrowseResults), NOT by matching its display copy, so rewording the
// `browse results` label can never silently re-admit it.
type filterFooterEntry struct {
	Key           []keyGlyph
	Label         string
	BrowseResults bool
}

// filteringFooterEntries returns the §7.1 input-active footer entries:
// `type to filter` (the `type` action word in accent.orange) · `↵ / ↓ browse
// results` (the commit glyphs ↵ and ↓ in accent.blue, the `/` separator in
// text.detail) · `esc clear` (the dismiss key in text.detail — a plain key, not
// the action word). The exact per-glyph colours match the input-active reference.
func filteringFooterEntries() []filterFooterEntry {
	return []filterFooterEntry{
		{Key: []keyGlyph{{"type", theme.MV.AccentOrange}}, Label: "to filter"},
		{Key: []keyGlyph{
			{"↵", theme.MV.AccentBlue},
			{" / ", theme.MV.TextDetail},
			{"↓", theme.MV.AccentBlue},
		}, Label: "browse results", BrowseResults: true},
		{Key: []keyGlyph{{"esc", theme.MV.TextDetail}}, Label: "clear"},
	}
}

// dropBrowseResults returns the given entries with the structurally-tagged
// browse-results entry removed (BrowseResults flag), preserving order. Membership is
// decided by the flag — never the display copy — so rewording the `browse results`
// label cannot change what this drops. The §7.3 no-matches footer composes through
// this so its reduced entry set stays derived from (not a hand-kept copy of) the
// input-active footer.
func dropBrowseResults(src []filterFooterEntry) []filterFooterEntry {
	entries := make([]filterFooterEntry, 0, len(src))
	for _, e := range src {
		if e.BrowseResults {
			continue
		}
		entries = append(entries, e)
	}
	return entries
}

// filterAppliedFooterEntries returns the §7.1 list-active footer entries:
// `↵ attach` · `↑↓ navigate` (both glyphs accent.blue) · `esc clear filter` (the
// `esc` clear-filter key in accent.orange — the action word that exits the filter).
// The exact per-glyph colours match the list-active reference.
func filterAppliedFooterEntries() []filterFooterEntry {
	return []filterFooterEntry{
		{Key: []keyGlyph{{"↵", theme.MV.AccentBlue}}, Label: "attach"},
		{Key: []keyGlyph{{"↑↓", theme.MV.AccentBlue}}, Label: "navigate"},
		{Key: []keyGlyph{{"esc", theme.MV.AccentOrange}}, Label: "clear filter"},
	}
}

// projectsFilterAppliedFooterEntries is the list-active footer for the PROJECTS
// page: identical to the Sessions list-active footer except the commit key reads
// `new session` (Enter on Projects creates a session — it does NOT attach), so the
// shared filterAppliedFooterEntries' "attach" copy is not leaked onto Projects.
func projectsFilterAppliedFooterEntries() []filterFooterEntry {
	return []filterFooterEntry{
		{Key: []keyGlyph{{"↵", theme.MV.AccentBlue}}, Label: "new session"},
		{Key: []keyGlyph{{"↑↓", theme.MV.AccentBlue}}, Label: "navigate"},
		{Key: []keyGlyph{{"esc", theme.MV.AccentOrange}}, Label: "clear filter"},
	}
}

// renderFilteringFooter renders the §7.1 input-active contextual footer for the
// given content width and resolved canvas mode (and the NO_COLOR carve-out). It is
// page-agnostic (`type to filter · ↵/↓ browse results · esc clear`), shared by
// Sessions and Projects.
func renderFilteringFooter(width int, mode theme.Mode, colourless bool) string {
	return renderFilterFooter(filteringFooterEntries(), width, mode, colourless)
}

// renderFilterAppliedFooter renders the §7.1 list-active contextual footer for the
// given content width and resolved canvas mode (and the NO_COLOR carve-out). This is
// the SESSIONS variant (`↵ attach`); Projects uses renderProjectsFilterAppliedFooter.
func renderFilterAppliedFooter(width int, mode theme.Mode, colourless bool) string {
	return renderFilterFooter(filterAppliedFooterEntries(), width, mode, colourless)
}

// renderProjectsFilterAppliedFooter renders the Projects list-active filter footer
// (`↵ new session` instead of `↵ attach`).
func renderProjectsFilterAppliedFooter(width int, mode theme.Mode, colourless bool) string {
	return renderFilterFooter(projectsFilterAppliedFooterEntries(), width, mode, colourless)
}

// renderFilterFooter is the shared two-row contextual-filter footer: the 1px
// border.footer top rule (the SAME rule as the §3.4 standard footer), then the
// entry row — the given entries as a dot-separated left cluster, a canvas-painted
// flex spacer, and the right-aligned `? help`. The row is always exactly width
// cells wide, mirroring footerKeyRow's right-anchor layout so the two filter
// footers and the standard footer agree on structure exactly.
func renderFilterFooter(entries []filterFooterEntry, width int, mode theme.Mode, colourless bool) string {
	w := headerWidthOrFallback(width)
	rule := footerTopRule(w, mode, colourless)
	row := filterFooterRow(entries, w, mode, colourless)
	return lipgloss.JoinVertical(lipgloss.Left, rule, row)
}

// filterFooterRow renders the single contextual-filter footer entry row: the left
// cluster of entries, a canvas flex spacer, then the right-aligned `? help`
// (sourced from the shared sessionsKeymap descriptor so its glyph/label/colour
// never drift from the standard footer). Always exactly w cells wide.
func filterFooterRow(entries []filterFooterEntry, w int, mode theme.Mode, colourless bool) string {
	left := renderFilterCluster(entries, mode, colourless)
	leftWidth := lipgloss.Width(left)

	// Reuse the standard footer's right-aligned ? help so the anchor is identical.
	_, helpEntry := splitFooterEntries(sessionsKeymap())
	rightSeg := ""
	rightWidth := 0
	if helpEntry != nil {
		rightSeg = renderFooterEntry(*helpEntry, theme.MV.AccentViolet, mode, colourless)
		rightWidth = lipgloss.Width(rightSeg)
	}

	// Hand the fit test, the narrow-degrade, and the flex-spacer join to the shared
	// assembler — the SAME right-anchor geometry as the standard footer (footerKeyRow).
	return assembleRightAnchoredRow(left, leftWidth, rightSeg, rightWidth, w, mode, colourless)
}

// renderFilterCluster renders the given filter-footer entries joined by the §3.4
// dot separator (text.detail) into a single left-cluster string. Each entry's key
// cluster renders via renderKeyGlyphs (per-glyph colours) and its label in
// text.detail, with a canvas-painted gap between — the SAME per-entry shape as
// renderFooterEntry.
func renderFilterCluster(entries []filterFooterEntry, mode theme.Mode, colourless bool) string {
	if len(entries) == 0 {
		return ""
	}
	segs := make([]string, 0, len(entries)*2-1)
	for i, e := range entries {
		if i > 0 {
			segs = append(segs, renderFooterDetail(footerEntrySeparator, mode, colourless))
		}
		key := renderKeyGlyphs(e.Key, mode, colourless)
		gap := headerCanvasBg(mode, colourless).Render(footerKeyLabelGap)
		label := headerStyle(theme.MV.TextDetail, mode, colourless).Render(e.Label)
		segs = append(segs, lipgloss.JoinHorizontal(lipgloss.Top, key, gap, label))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, segs...)
}

// renderKeyGlyphs renders a filter-footer entry's key cluster — one or more
// per-glyph coloured runs joined horizontally — each over the owned canvas. A
// single-glyph key renders one run; a composite (e.g. `↵ / ↓`) renders each glyph
// in its own token so the §7.1 references' mixed-colour key reads correctly.
func renderKeyGlyphs(glyphs []keyGlyph, mode theme.Mode, colourless bool) string {
	runs := make([]string, 0, len(glyphs))
	for _, g := range glyphs {
		runs = append(runs, headerStyle(g.Tok, mode, colourless).Render(g.Text))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, runs...)
}
