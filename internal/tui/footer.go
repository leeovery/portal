package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/tui/theme"
)

// The §3.4 condensed Sessions footer: a SINGLE row of the Core keymap keys on
// the left (↑↓ navigate · ⏎ attach · / filter · ␣ preview · s switch
// view · x projects) with a right-aligned `? help` hint pinned to the content
// width, over a 1px border.footer top rule. It replaces the former manual
// three-column keymap footer for Sessions; the help-only keys
// (n/r/k/q/Ctrl+↑/↓) live in the ? help modal (Phase 3), not the footer.
//
// The footer is the FIRST consumer of the §12.1 keymap descriptor (task 2-1,
// sessionsKeymap): the entries are filtered to Core and rendered FROM the
// descriptor, so the footer and the ? help modal never drift from a second
// hand-authored binding list.
//
// Every cell carries the owned canvas background (leaf .Background(canvas), §1),
// mirroring header.go / section_header.go, so the right-aligned spacer gap is
// canvas-painted with no terminal-bg island. Under the NO_COLOR carve-out (§2.5)
// every hue and the canvas drop; the glyphs stay structurally distinct on the
// terminal's native fg/bg.

const (
	// footerRuleGlyph is the 1px footer top-rule glyph, drawn in border.footer.
	// It uses the UPPER one-eighth block so the rule lands at the TOP edge of its
	// cell, opening breathing room BELOW it before the key row (the header rule uses
	// the LOWER block ▁ to sit low; the footer rule mirrors it high so the border is
	// not crowded against the keybindings). NOTE: border.footer (1px, this rule) is a
	// DISTINCT token from border.separator (2px, the header rule) — not to be conflated.
	footerRuleGlyph = "▔"
	// footerKeyLabelGap is the single space between a key glyph and its label
	// (e.g. "↑↓ navigate"). Canvas-painted so the gap is not a terminal-bg island.
	// Used by renderFilterCluster (whose multi-glyph key cluster is a distinct shape
	// from the single-key renderKeyHint helper). renderKeyHint paints the same single
	// canvas space inline.
	footerKeyLabelGap = " "
	// footerEntrySeparator is the " · " dot separator between footer entries in the
	// left cluster (the §3.4 dot-separated condensed row). Rendered in text.detail
	// so it reads as quiet chrome between the brighter key glyphs.
	footerEntrySeparator = " · "
)

// footerEllipsis is the overflow marker the narrow-degrade path appends to the
// left cluster when one or more lower-priority entries are dropped (§2.7).
const footerEllipsis = "…"

// renderSessionsFooter renders the §3.4 condensed Sessions footer for the given
// content width and resolved canvas mode (and the NO_COLOR carve-out). It is the
// single render entry point so the composed-view render (viewSessionList) and the
// height-budget computation (applySessionListSize) resolve the footer against the
// SAME width/mode and agree on its height exactly.
//
// The footer is two rows: the 1px border.footer top rule, then the condensed key
// row (Core keys left, right-aligned ? help). Below the width at which the full
// left cluster + a spacer + the ? help no longer fit, lower-priority Core entries
// drop (with an ellipsis marker) so the row truncates gracefully on ONE line — it
// never wraps to a second line (which would steal a list row) and the ? help right
// anchor survives as long as possible (§2.7).
func renderSessionsFooter(width int, mode theme.Mode, colourless bool) string {
	return renderCondensedFooter(sessionsKeymap(), width, mode, colourless)
}

// renderProjectsFooter renders the §6.3 condensed Projects footer
// (`⏎ new session` · `x sessions` · `e edit` · `/ filter`, right-aligned
// `? help`) through the SAME condensed-footer machinery as the Sessions footer,
// driven by the projectsKeymap descriptor — replacing the former three-column
// keymap footer for the Projects page. Same two-row shape (the shared 1px
// border.footer top rule + one key row), so it is height-neutral against the
// Sessions footer's reserved budget.
func renderProjectsFooter(width int, mode theme.Mode, colourless bool) string {
	return renderCondensedFooter(projectsKeymap(), width, mode, colourless)
}

// renderCommandPendingFooter renders the §11.4 command-pending Projects footer:
// `⏎ run here · n run in cwd · esc cancel` (left cluster) + the right-aligned
// `? help` anchor, over the shared 1px border.footer top rule. The left cluster
// entries are derived from the commandPendingKeymap() descriptor — the single binding
// source (§11.4) — mapped to MV chrome (key glyphs accent.blue, labels text.detail,
// the `enter` binding shown as its declarative HelpKey `⏎` glyph). It routes through
// the shared renderFilterFooter machinery so the `? help` anchor + the two-row
// structure stay byte-consistent with the standard / filter footers; only the entries
// differ.
func renderCommandPendingFooter(width int, mode theme.Mode, colourless bool) string {
	return renderFilterFooter(commandPendingFooterEntries(), width, mode, colourless)
}

// commandPendingFooterEntries maps the §11.4 descriptor (commandPendingKeymap) to the
// filter-footer entry shape: each entry's Action becomes the label and its glyph comes
// from helpKeyGlyph (the declarative HelpKey when set — `enter`'s `⏎` — else the terse
// Key), the SAME glyph resolution the descriptor-driven help path uses. This retires
// the former inline `enter→⏎` rewrite, folding the command-pending footer into the
// shared descriptor/entry vocabulary. Every key glyph is accent.blue per the MV footer
// convention (§3.4 / §8.4); labels render in text.detail.
func commandPendingFooterEntries() []filterFooterEntry {
	descriptor := commandPendingKeymap()
	entries := make([]filterFooterEntry, 0, len(descriptor))
	for _, e := range descriptor {
		entries = append(entries, filterFooterEntry{
			Key:   []keyGlyph{{helpKeyGlyph(e), theme.MV.AccentBlue}},
			Label: e.Action,
		})
	}
	return entries
}

// The §5 multi-select mode footer copy — the spec-exact entry glyphs + labels fixed
// by the delivered Paper frame (design/sessions-multi-select-active.png) and the spec
// (Multi-Select Mode → Mode affordance):
//
//	↑↓ navigate · m toggle · ␣ preview · ⏎ open · esc cancel
//
// Sourced once here as named constants (mirroring commandBandText) so the wording
// can't drift from a paraphrase. The glyphs are the codebase canon: nav ↑↓, preview ␣
// (U+2423) and open ⏎ (U+23CE) match the sessionsKeymap Key forms. Unlike the
// standard/filter footers this footer carries NO right-aligned `? help` anchor — the
// delivered frame has none.
const (
	multiSelectNavGlyph     = "↑↓"
	multiSelectNavLabel     = "navigate"
	multiSelectToggleGlyph  = "m"
	multiSelectToggleLabel  = "toggle"
	multiSelectPreviewGlyph = "␣" // U+2423, the sessionsKeymap preview glyph
	multiSelectPreviewLabel = "preview"
	multiSelectOpenGlyph    = "⏎" // U+23CE, the sessionsKeymap enter/attach glyph
	multiSelectOpenLabel    = "open"
	multiSelectCancelGlyph  = "esc"
	multiSelectCancelLabel  = "cancel"
)

// multiSelectFooterText is the spec-exact §5 mode-footer copy assembled from the
// per-entry constants above (separators the shared footerEntrySeparator ` · `). It is
// the single source of truth the copy-pin test asserts the render against.
const multiSelectFooterText = multiSelectNavGlyph + footerKeyLabelGap + multiSelectNavLabel +
	footerEntrySeparator + multiSelectToggleGlyph + footerKeyLabelGap + multiSelectToggleLabel +
	footerEntrySeparator + multiSelectPreviewGlyph + footerKeyLabelGap + multiSelectPreviewLabel +
	footerEntrySeparator + multiSelectOpenGlyph + footerKeyLabelGap + multiSelectOpenLabel +
	footerEntrySeparator + multiSelectCancelGlyph + footerKeyLabelGap + multiSelectCancelLabel

// multiSelectFooterEntries returns the §5 multi-select mode footer entries in frame
// order. Every key glyph is accent.blue and every label text.detail — the standard MV
// footer colour convention (§3.4). It mirrors filteringFooterEntries' entry-list shape
// so the cluster renders through the shared renderFilterCluster machinery.
func multiSelectFooterEntries() []filterFooterEntry {
	return []filterFooterEntry{
		{Key: []keyGlyph{{multiSelectNavGlyph, theme.MV.AccentBlue}}, Label: multiSelectNavLabel},
		{Key: []keyGlyph{{multiSelectToggleGlyph, theme.MV.AccentBlue}}, Label: multiSelectToggleLabel},
		{Key: []keyGlyph{{multiSelectPreviewGlyph, theme.MV.AccentBlue}}, Label: multiSelectPreviewLabel},
		{Key: []keyGlyph{{multiSelectOpenGlyph, theme.MV.AccentBlue}}, Label: multiSelectOpenLabel},
		{Key: []keyGlyph{{multiSelectCancelGlyph, theme.MV.AccentBlue}}, Label: multiSelectCancelLabel},
	}
}

// renderMultiSelectFooter renders the §5 multi-select mode footer: the five spec-exact
// entries as a dot-separated left cluster over the shared 1px border.footer top rule,
// with NO right-aligned `? help` anchor (the delivered frame has none). It reuses
// renderFilterCluster (via fitFilterCluster) for the cluster body so the dot
// separators, canvas-painted gaps, and NO_COLOR carve-out match the other footers
// byte-for-byte, and hands the width-pad to assembleRightAnchoredRow with an EMPTY
// right segment (the §2.7 pad-to-width path — no anchor). At a narrow width
// fitFilterCluster drops trailing entries with an ellipsis so the row degrades on ONE
// line without wrapping. Two rows (rule + entry row), height-neutral against the
// reserved sessionFooterHeight budget. It does NOT route through
// renderFilterFooter/filterFooterRow (those hardcode the sessionsKeymap `? help`
// anchor).
func renderMultiSelectFooter(width int, mode theme.Mode, colourless bool) string {
	w := headerWidthOrFallback(width)
	rule := footerTopRule(w, mode, colourless)
	left, leftWidth := fitFilterCluster(multiSelectFooterEntries(), w, mode, colourless)
	row := assembleRightAnchoredRow(left, leftWidth, "", 0, w, mode, colourless)
	return lipgloss.JoinVertical(lipgloss.Left, rule, row)
}

// fitFilterCluster renders the given filter-footer entries as a dot-separated left
// cluster that fits within w cells, greedily including entries in order and, when the
// full cluster does not fit, dropping trailing entries and appending an ellipsis
// marker so the row degrades on ONE line without wrapping (§2.7). It mirrors
// fitLeftCluster (the keymap-descriptor footer's fitter) for the per-glyph
// filterFooterEntry cluster path — the multi-select footer has no right anchor, so the
// full width is the budget. Returns the rendered cluster and its exact rendered width
// (always ≤ w).
func fitFilterCluster(entries []filterFooterEntry, w int, mode theme.Mode, colourless bool) (string, int) {
	// Try the full cluster first (the common, wide-terminal case).
	if full := renderFilterCluster(entries, mode, colourless); lipgloss.Width(full) <= w {
		return full, lipgloss.Width(full)
	}

	// Narrow degrade (§2.7): include as many leading entries as fit, then append an
	// ellipsis marker. Find the largest prefix whose rendered width (with the ellipsis
	// appended) still fits w.
	ellipsis := renderFooterDetail(footerEllipsis, mode, colourless)
	sep := renderFooterDetail(footerEntrySeparator, mode, colourless)
	ellipsisWidth := lipgloss.Width(ellipsis)
	sepWidth := lipgloss.Width(sep)

	best := ""
	bestWidth := 0
	for n := 1; n <= len(entries); n++ {
		cluster := renderFilterCluster(entries[:n], mode, colourless)
		// Width of "<cluster> · …": the cluster, a separator, then the ellipsis.
		candidateWidth := lipgloss.Width(cluster) + sepWidth + ellipsisWidth
		if candidateWidth > w {
			break
		}
		best = lipgloss.JoinHorizontal(lipgloss.Top, cluster, sep, ellipsis)
		bestWidth = candidateWidth
	}
	if best != "" {
		return best, bestWidth
	}

	// Not even one entry + ellipsis fits: render just the ellipsis if it fits, else an
	// empty cluster (the row degrades to blank canvas at extreme narrowness, §2.7).
	if ellipsisWidth <= w {
		return ellipsis, ellipsisWidth
	}
	return "", 0
}

// renderCondensedFooter is the shared §3.4 / §6.3 condensed-footer renderer for a
// per-page keymap descriptor: the descriptor's Core entries form the dot-separated
// left cluster and the single right-aligned entry (the ? help hint) is pinned to
// the right, over the 1px border.footer top rule. It is the single render entry
// point so a page's composed-view render and its height-budget computation resolve
// the footer against the SAME width/mode and agree on its height exactly. Both the
// Sessions and Projects footers route through here so the two never drift.
func renderCondensedFooter(entries []keymapEntry, width int, mode theme.Mode, colourless bool) string {
	w := headerWidthOrFallback(width)
	rule := footerTopRule(w, mode, colourless)
	row := footerKeyRow(entries, w, mode, colourless)
	return lipgloss.JoinVertical(lipgloss.Left, rule, row)
}

// footerTopRule renders the full-width 1px border.footer top rule above the
// condensed footer row. Under the NO_COLOR carve-out the rule keeps its glyphs but
// drops the colour and the canvas, rendering on the terminal's native fg/bg.
// Mirrors headerSeparatorRule, swapping border.separator → border.footer.
func footerTopRule(w int, mode theme.Mode, colourless bool) string {
	rule := strings.Repeat(footerRuleGlyph, w)
	return headerStyle(theme.MV.BorderFooter, mode, colourless).Render(rule)
}

// footerKeyRow renders the single condensed key row for the given keymap
// descriptor: the Core keymap entries as a dot-separated left cluster, then a
// canvas-painted flex spacer, then the right-aligned ? help. The row is always
// exactly w cells wide. When the full left cluster does not fit beside the ? help,
// lower-priority Core entries are dropped (with an ellipsis) until it fits — the
// ? help anchor survives as long as possible (§2.7), and the row never wraps.
func footerKeyRow(entries []keymapEntry, w int, mode theme.Mode, colourless bool) string {
	core, right := splitFooterEntries(entries)

	// Render the right-aligned ? help hint first — it is the surviving anchor, so
	// the left cluster is fitted around the space it reserves. Its key glyph is the
	// only one in accent.violet (the rest are accent.blue).
	rightSeg := ""
	rightWidth := 0
	if right != nil {
		rightSeg = renderFooterEntry(*right, theme.MV.AccentViolet, mode, colourless)
		rightWidth = lipgloss.Width(rightSeg)
	}

	left, leftWidth := fitLeftCluster(core, w, rightWidth, mode, colourless)

	// The fitLeftCluster contract guarantees leftWidth ≤ w; the shared assembler
	// owns the fit test, the narrow-degrade, and the flex-spacer join.
	return assembleRightAnchoredRow(left, leftWidth, rightSeg, rightWidth, w, mode, colourless)
}

// assembleRightAnchoredRow lays out a right-anchored footer row of exactly w
// cells: a left cluster (already rendered, leftWidth cells) and a right anchor
// segment (rightSeg, rightWidth cells) pinned to the row's right edge with a
// canvas-painted flex spacer between them. When the right anchor does not fit
// beside the left cluster (leftWidth+1+rightWidth > w — at least one spacer cell)
// or there is no right anchor (rightSeg == ""), the anchor is dropped and the left
// cluster is padded to width via headerPadRight (the §2.7 narrow-degrade). It is
// the single owner of this right-anchor geometry, shared by the standard condensed
// footer (footerKeyRow) and the contextual filter footers (filterFooterRow) so a
// change to the degrade rule is made once. Callers render their OWN left cluster
// (the footer-specific fitLeftCluster ellipsis logic stays out of here).
func assembleRightAnchoredRow(left string, leftWidth int, rightSeg string, rightWidth, w int, mode theme.Mode, colourless bool) string {
	if rightSeg == "" || leftWidth+1+rightWidth > w {
		return headerPadRight(left, leftWidth, w, mode, colourless)
	}

	spacerWidth := w - leftWidth - rightWidth
	spacer := headerCanvasBg(mode, colourless).Render(strings.Repeat(" ", spacerWidth))
	return lipgloss.JoinHorizontal(lipgloss.Top, left, spacer, rightSeg)
}

// splitFooterEntries partitions a keymap descriptor into the ordered Core entries
// that form the footer's left cluster and the single right-aligned entry (the ?
// help hint) the footer pins to the right. Non-Core (help-only) entries are
// dropped — they live in the ? help modal, not the footer (§3.4 / §8.5). The
// right-aligned entry is excluded from the left cluster slice.
func splitFooterEntries(entries []keymapEntry) (core []keymapEntry, right *keymapEntry) {
	for i := range entries {
		e := entries[i]
		if !e.Core {
			continue
		}
		if e.RightAligned {
			r := e
			right = &r
			continue
		}
		core = append(core, e)
	}
	return core, right
}

// fitLeftCluster renders the ordered Core entries as a dot-separated left cluster
// that fits within w cells while leaving room for the reserved right anchor
// (rightWidth, plus one spacer cell). It greedily includes entries in priority
// order (descriptor order — navigate is highest priority, projects lowest) and, if
// the full cluster does not fit, drops trailing entries and appends an ellipsis
// marker so the row truncates on ONE line without wrapping (§2.7). Returns the
// rendered cluster and its exact rendered width (always ≤ w).
func fitLeftCluster(core []keymapEntry, w, rightWidth int, mode theme.Mode, colourless bool) (string, int) {
	// The budget the left cluster may occupy: the full width minus the reserved
	// right anchor and one spacer cell. When there is no right anchor the cluster
	// may use the full width.
	budget := w
	if rightWidth > 0 {
		budget = w - rightWidth - 1
	}
	if budget < 0 {
		budget = 0
	}

	// Try the full cluster first (the common, wide-terminal case).
	if full := renderFooterCluster(core, mode, colourless); lipgloss.Width(full) <= budget {
		return full, lipgloss.Width(full)
	}

	// Narrow degrade (§2.7): include as many leading entries as fit, then append an
	// ellipsis marker. Find the largest prefix whose rendered width (with the
	// ellipsis appended) still fits the budget.
	ellipsis := renderFooterDetail(footerEllipsis, mode, colourless)
	sep := renderFooterDetail(footerEntrySeparator, mode, colourless)
	ellipsisWidth := lipgloss.Width(ellipsis)
	sepWidth := lipgloss.Width(sep)

	best := ""
	bestWidth := 0
	for n := 1; n <= len(core); n++ {
		cluster := renderFooterCluster(core[:n], mode, colourless)
		// Width of "<cluster> · …": the cluster, a separator, then the ellipsis.
		candidateWidth := lipgloss.Width(cluster) + sepWidth + ellipsisWidth
		if candidateWidth > budget {
			break
		}
		best = lipgloss.JoinHorizontal(lipgloss.Top, cluster, sep, ellipsis)
		bestWidth = candidateWidth
	}
	if best != "" {
		return best, bestWidth
	}

	// Not even one entry + ellipsis fits: render just the ellipsis if it fits, else
	// an empty cluster (the row degrades to the ? help anchor alone, §2.7).
	if ellipsisWidth <= budget {
		return ellipsis, ellipsisWidth
	}
	return "", 0
}

// renderFooterCluster renders the given Core entries joined by the dot separator
// into a single left-cluster string. Each entry's key glyph is accent.blue, its
// label text.detail, and the separators text.detail.
func renderFooterCluster(entries []keymapEntry, mode theme.Mode, colourless bool) string {
	if len(entries) == 0 {
		return ""
	}
	segs := make([]string, 0, len(entries)*2-1)
	for i, e := range entries {
		if i > 0 {
			segs = append(segs, renderFooterDetail(footerEntrySeparator, mode, colourless))
		}
		segs = append(segs, renderFooterEntry(e, theme.MV.AccentBlue, mode, colourless))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, segs...)
}

// renderFooterEntry renders one keymap entry as "<key> <label>" with the key glyph
// in keyTok (accent.blue for left-cluster entries, accent.violet for the ? help
// hint) and the label in text.detail, with a single canvas-painted gap between
// them. It routes through the shared renderKeyHint helper (the single canvas space
// renderKeyHint paints matches footerKeyLabelGap, so the output is byte-identical).
func renderFooterEntry(e keymapEntry, keyTok theme.Token, mode theme.Mode, colourless bool) string {
	return renderKeyHint(e.Key, e.Action, keyTok, mode, colourless)
}

// renderFooterDetail renders a chrome run (a separator or the ellipsis marker) in
// text.detail over the owned canvas.
func renderFooterDetail(s string, mode theme.Mode, colourless bool) string {
	return headerStyle(theme.MV.TextDetail, mode, colourless).Render(s)
}
