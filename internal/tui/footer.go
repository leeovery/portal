package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/tui/theme"
)

// The §3.4 condensed Sessions footer: a SINGLE row of the Core keymap keys on
// the left (↑/↓ navigate · enter attach · / filter · space preview · s switch
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
	// (e.g. "↑/↓ navigate"). Canvas-painted so the gap is not a terminal-bg island.
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

	// No room for the ? help beside the (possibly truncated) left cluster, or no
	// right entry: pad the left cluster to width and return (the ? help would
	// overflow). The fitLeftCluster contract guarantees leftWidth ≤ w.
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
