package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui/theme"
)

// Tests for the §11 shared notice-band primitive, the single-slot arbiter, and
// the F10 flash-driven viewport-height recompute (task 4-1).
//
// The band is a full-width single-line left-bar (▌) accent line that sits
// directly under the title separator, above the section header. The notice slot
// holds AT MOST ONE band: a transient flash wins over any persistent band while
// shown, and the persistent band returns once the flash clears.

// flattenNoticeBand reconstructs a (possibly wrapped, multi-line) notice band's
// MESSAGE text into a single line for substring matching: it strips ANSI, drops
// each line's leading `▌` left-bar + the bar/continuation-indent whitespace, and
// joins the per-line message fragments with a single space (mirroring how
// ansi.Wrap split the message on a word boundary). It lets the message-identity
// assertions stay wrap-agnostic — they care that the band carries the message,
// not whether it spans one line or three at a given width.
func flattenNoticeBand(band string) string {
	frags := make([]string, 0, 4)
	for _, line := range strings.Split(ansi.Strip(band), "\n") {
		// Drop the leading `▌` bar then trim the bar gap + continuation indent.
		body := strings.TrimPrefix(line, noticeBarGlyph)
		body = strings.TrimLeft(body, " ")
		// Trim the right-pad so fragments join cleanly.
		body = strings.TrimRight(body, " ")
		if body != "" {
			frags = append(frags, body)
		}
	}
	return strings.Join(frags, " ")
}

// viewHasNoticeMessage reports whether the model's rendered view carries the given
// notice message, tolerating the §11 wrap: it renders the message exactly as the
// view does (via the model's own band render at the model's width) and asserts each
// non-empty wrapped fragment is present in the view. A short message yields one
// fragment (the whole message); a wrapped message yields its per-line fragments,
// each of which must appear on its own line in the composed view.
func viewHasNoticeMessage(t *testing.T, m Model, role noticeBandRole, message string) bool {
	t.Helper()
	band := renderNoticeBand(role, message, noticeBandOnBandText(role), m.contentWidth(), m.canvasMode, m.colourless)
	view := ansi.Strip(m.View().Content)
	for _, line := range strings.Split(ansi.Strip(band), "\n") {
		frag := strings.TrimRight(strings.TrimLeft(strings.TrimPrefix(line, noticeBarGlyph), " "), " ")
		if frag == "" {
			continue
		}
		if !strings.Contains(view, frag) {
			return false
		}
	}
	return true
}

// noticeBandModel builds a Sessions-page model seeded with the given session
// names at 80x24 so the rendered list carries predictable substrings.
func noticeBandModel(names ...string) Model {
	sessions := make([]tmux.Session, 0, len(names))
	for _, n := range names {
		sessions = append(sessions, tmux.Session{Name: n, Windows: 1, Attached: false})
	}
	m := NewModelWithSessions(sessions)
	m.termWidth = 80
	m.termHeight = 24
	return m
}

// TestRenderNoticeBand_LeftBarInRoleColour asserts the primitive renders a
// single full-width line with a far-left ▌ in the role colour for each of the
// three §11 MV role variants, and the message in the on-band text token.
func TestRenderNoticeBand_LeftBarInRoleColour(t *testing.T) {
	const (
		w   = 60
		msg = "band-message-probe"
	)
	for _, tc := range []struct {
		name      string
		role      noticeBandRole
		barTok    theme.Token
		onBandTok theme.Token
	}{
		{"warning/orange", bandWarning, theme.MV.AccentOrange, theme.MV.TextOnWarning},
		{"success/green", bandSuccess, theme.MV.StateGreen, theme.MV.TextStrong},
		{"info/violet", bandInfo, theme.MV.AccentViolet, theme.MV.TextOnSelection},
	} {
		t.Run(tc.name, func(t *testing.T) {
			band := renderNoticeBand(tc.role, msg, tc.onBandTok, w, theme.Dark, false)

			// Single line.
			if h := lipgloss.Height(band); h != 1 {
				t.Errorf("band height = %d, want 1 (single line)", h)
			}
			// Full width.
			if got := lipgloss.Width(band); got != w {
				t.Errorf("band width = %d, want %d (full-width)", got, w)
			}
			// Far-left ▌ bar.
			stripped := ansi.Strip(band)
			if !strings.HasPrefix(stripped, noticeBarGlyph) {
				t.Errorf("band does not start with the %q left-bar: %q", noticeBarGlyph, stripped)
			}
			// Message text present.
			if !strings.Contains(stripped, msg) {
				t.Errorf("band missing the message %q: %q", msg, stripped)
			}
			// Bar colour = role token foreground.
			barSeq := tokenFgSeq(t, tc.barTok, theme.Dark)
			if !strings.Contains(band, barSeq) {
				t.Errorf("band missing the %s bar foreground sequence %q:\n%s", tc.name, barSeq, band)
			}
			// Message colour = on-band text token foreground.
			msgSeq := tokenFgSeq(t, tc.onBandTok, theme.Dark)
			if !strings.Contains(band, msgSeq) {
				t.Errorf("band missing the on-band text foreground sequence %q:\n%s", msgSeq, band)
			}
		})
	}
}

// TestRenderNoticeBand_NoColor asserts the NO_COLOR carve-out (§2.5): the bar
// glyph, its position, and the message text survive, but the tint and bar colour
// are dropped (no SGR colour sequences in the rendered band).
func TestRenderNoticeBand_NoColor(t *testing.T) {
	const (
		w   = 60
		msg = "nocolor-band-probe"
	)
	band := renderNoticeBand(bandInfo, msg, theme.MV.TextOnSelection, w, theme.Dark, true)

	// Bar glyph + message survive.
	stripped := ansi.Strip(band)
	if !strings.HasPrefix(stripped, noticeBarGlyph) {
		t.Errorf("NO_COLOR band must keep the %q left-bar: %q", noticeBarGlyph, stripped)
	}
	if !strings.Contains(stripped, msg) {
		t.Errorf("NO_COLOR band must keep the message %q: %q", msg, stripped)
	}
	// No colour sequences at all — the bar colour and any tint are dropped.
	if band != stripped {
		t.Errorf("NO_COLOR band must carry no SGR colour sequences; got raw %q", band)
	}
}

// TestNoticeSlot_SingleBand_TransientFlashWins asserts the single-slot rule:
// with both a persistent band condition (By-Tag zero-tags signpost) AND a
// transient flash active, only the transient flash row renders.
func TestNoticeSlot_SingleBand_TransientFlashWins(t *testing.T) {
	dir := t.TempDir()
	projects := []project.Project{{Path: dir, Name: "Portal"}}
	sessions := []tmux.Session{{Name: "portal-abc", Dir: dir}}

	m := newRebuildTestModel(prefs.ModeByTag, sessions, projects)
	m.termWidth = 80
	m.termHeight = 24
	m.rebuildSessionList()
	if !m.byTagSignpost {
		t.Fatalf("setup invariant: byTagSignpost = false, want true (signposted By Tag)")
	}

	const flash = "__TRANSIENT_FLASH__"
	m.setFlash(flash)

	view := m.View().Content
	if !strings.Contains(view, flash) {
		t.Errorf("transient flash must render while active:\n%s", view)
	}
	if viewHasNoticeMessage(t, m, bandInfo, byTagSignpostText) {
		t.Errorf("persistent signpost must NOT render while the transient flash holds the slot:\n%s", view)
	}
}

// TestNoticeSlot_PersistentReturnsAfterFlashClear asserts the persistent band
// returns to the slot after the transient flash clears.
func TestNoticeSlot_PersistentReturnsAfterFlashClear(t *testing.T) {
	dir := t.TempDir()
	projects := []project.Project{{Path: dir, Name: "Portal"}}
	sessions := []tmux.Session{{Name: "portal-abc", Dir: dir}}

	m := newRebuildTestModel(prefs.ModeByTag, sessions, projects)
	m.termWidth = 80
	m.termHeight = 24
	m.rebuildSessionList()

	const flash = "__TRANSIENT_FLASH__"
	m.setFlash(flash)
	m.clearFlash()

	view := m.View().Content
	if strings.Contains(view, flash) {
		t.Errorf("flash must be gone after clear:\n%s", view)
	}
	if !viewHasNoticeMessage(t, m, bandInfo, byTagSignpostText) {
		t.Errorf("persistent signpost must return to the slot after the flash clears:\n%s", view)
	}
}

// TestNoticeSlot_NeverBothBandsSimultaneously asserts a persistent (violet info)
// band and a transient flash never render at the same time — exactly one notice
// row exists while both conditions hold.
func TestNoticeSlot_NeverBothBandsSimultaneously(t *testing.T) {
	dir := t.TempDir()
	projects := []project.Project{{Path: dir, Name: "Portal"}}
	sessions := []tmux.Session{{Name: "portal-abc", Dir: dir}}

	m := newRebuildTestModel(prefs.ModeByTag, sessions, projects)
	m.termWidth = 80
	m.termHeight = 24
	m.rebuildSessionList()

	const flash = "__TRANSIENT_FLASH__"
	m.setFlash(flash)

	// Exactly one notice row: the flash row is present, the signpost row is not.
	role, message, ok := m.activeNoticeBand()
	if !ok {
		t.Fatalf("activeNoticeBand reports no band while a flash is active")
	}
	if message != flash {
		t.Errorf("active band message = %q, want the transient flash %q", message, flash)
	}
	if role != bandWarning {
		t.Errorf("active band role = %v, want bandWarning (transient flash)", role)
	}
}

// TestNoticeBand_PlacedUnderSeparatorAboveSectionHeader asserts the band sits
// directly under the title separator, with ONE blank breathing row between the
// band and the section header (band → blank → section header), shifting the
// section header + list down by TWO rows.
func TestNoticeBand_PlacedUnderSeparatorAboveSectionHeader(t *testing.T) {
	m := noticeBandModel("alpha-row")
	const flash = "__PLACEMENT_FLASH__"

	beforeLines := strings.Split(m.View().Content, "\n")
	beforeSection := lineIndexContaining(beforeLines, "Sessions")
	beforeRow := lineIndexContaining(beforeLines, "alpha-row")
	if beforeSection < 0 || beforeRow < 0 {
		t.Fatalf("baseline render missing section header or session row")
	}

	m.setFlash(flash)
	afterLines := strings.Split(m.View().Content, "\n")

	ruleIdx := -1
	for i, l := range afterLines {
		if strings.Contains(ansi.Strip(l), strings.Repeat(headerRuleGlyph, 4)) {
			ruleIdx = i
			break
		}
	}
	bandIdx := lineIndexContaining(afterLines, flash)
	sectionIdx := lineIndexContaining(afterLines, "Sessions")
	rowIdx := lineIndexContaining(afterLines, "alpha-row")
	if ruleIdx < 0 || bandIdx < 0 || sectionIdx < 0 || rowIdx < 0 {
		t.Fatalf("flash render missing a landmark: rule=%d band=%d section=%d row=%d\n%s",
			ruleIdx, bandIdx, sectionIdx, rowIdx, strings.Join(afterLines, "\n"))
	}
	// Band directly under the separator rule, above the section header.
	if bandIdx <= ruleIdx {
		t.Errorf("band index %d must be > separator-rule index %d (band under the separator)", bandIdx, ruleIdx)
	}
	if sectionIdx <= bandIdx {
		t.Errorf("section header index %d must be > band index %d (band above the section header)", sectionIdx, bandIdx)
	}
	// One blank breathing row BETWEEN the band and the section header: the section
	// header is exactly two rows below the band (band → blank → section header), and
	// the intervening row carries neither the flash text nor "Sessions".
	if sectionIdx-bandIdx != 2 {
		t.Errorf("section header is %d rows below the band, want 2 (band → blank → section header)", sectionIdx-bandIdx)
	}
	blankIdx := bandIdx + 1
	blank := ansi.Strip(afterLines[blankIdx])
	if strings.TrimSpace(blank) != "" {
		t.Errorf("row between the band and section header must be blank, got %q", blank)
	}
	// Section header + list shifted down by TWO rows (band + blank).
	if sectionIdx-beforeSection != 2 {
		t.Errorf("section header shift = %d, want +2 (band + blank push it down two rows)", sectionIdx-beforeSection)
	}
	if rowIdx-beforeRow != 2 {
		t.Errorf("session row shift = %d, want +2 (band + blank push it down two rows)", rowIdx-beforeRow)
	}
}

// TestNoticeBand_RecomputesViewportHeight asserts the list viewport height is
// recomputed when the band appears (TWO rows consumed — the band PLUS its blank
// breathing row) and when it clears (both rows released) — the §11.2 F10 contract.
// The composed frame height stays constant (the slot is absorbed under the outer
// canvas fill), and the list reserves exactly two fewer rows while the band is
// active.
func TestNoticeBand_RecomputesViewportHeight(t *testing.T) {
	m := noticeBandModel("alpha-row")

	_, baseHeight := m.SessionListSize()

	m.setFlash("__HEIGHT_FLASH__")
	_, withBandHeight := m.SessionListSize()
	if withBandHeight != baseHeight-2 {
		t.Errorf("list height with band = %d, want %d (band + blank, two rows consumed)", withBandHeight, baseHeight-2)
	}

	m.clearFlash()
	_, clearedHeight := m.SessionListSize()
	if clearedHeight != baseHeight {
		t.Errorf("list height after clear = %d, want %d (both rows released)", clearedHeight, baseHeight)
	}
}

// TestNoticeBand_FrameHeightConstant asserts the composed frame height does not
// grow when the band appears — the band is absorbed by the list shrinking one
// row underneath the outer canvas fill (the §3.5 / §4.1 one-row-per-delegate
// pagination invariant: the frame re-pads to exactly termH).
func TestNoticeBand_FrameHeightConstant(t *testing.T) {
	m := noticeBandModel("alpha-row")

	baseline := strings.Split(m.View().Content, "\n")
	m.setFlash("__CONSTANT_HEIGHT_FLASH__")
	withBand := strings.Split(m.View().Content, "\n")

	if len(withBand) != len(baseline) {
		t.Errorf("band activation must not change the frame height: baseline=%d withBand=%d",
			len(baseline), len(withBand))
	}
}

// TestNoticeBand_FlashGenerationGuardPreserved asserts the flash auto-clear
// generation guard still drops a superseded flashTickMsg — the arbiter changes
// WHICH row renders, never the flash lifecycle.
func TestNoticeBand_FlashGenerationGuardPreserved(t *testing.T) {
	m := noticeBandModel("alpha-row")
	m.setFlash("first")  // gen 1
	m.setFlash("second") // gen 2 — supersedes

	updated, _ := m.Update(flashTickMsg{Gen: 1})
	mm := updated.(Model)
	if mm.flashText != "second" {
		t.Errorf("superseded tick must not early-clear: flashText = %q, want %q", mm.flashText, "second")
	}
}

// TestNoticeBand_ActionableKeyClearsFlash asserts an actionable keypress clears
// an active flash (lifecycle parity through the arbiter).
func TestNoticeBand_ActionableKeyClearsFlash(t *testing.T) {
	m := noticeBandModel("alpha-row")
	m.setFlash("clear-me")

	updated, _ := m.updateSessionList(tea.KeyPressMsg{Code: tea.KeyDown})
	mm := updated.(Model)
	if mm.flashText != "" {
		t.Errorf("actionable key must clear the active flash: flashText = %q, want empty", mm.flashText)
	}
}

// TestNoticeBand_TimeoutClearsFlash asserts a matching flashTickMsg (the short
// timeout) clears the active flash (lifecycle parity through the arbiter).
func TestNoticeBand_TimeoutClearsFlash(t *testing.T) {
	m := noticeBandModel("alpha-row")
	m.setFlash("timeout-me") // gen 1

	updated, _ := m.Update(flashTickMsg{Gen: 1})
	mm := updated.(Model)
	if mm.flashText != "" {
		t.Errorf("matching tick (timeout) must clear the flash: flashText = %q, want empty", mm.flashText)
	}
}

// longBandMessage is a message guaranteed to exceed any narrow band width so the
// wrap tests (below) reliably exercise the multi-line path. The signpost wording is
// the longest real band message, so it is the canonical overflow probe.
const longBandMessage = byTagSignpostText

// TestNoticeBand_WrapsLongMessage asserts a message longer than the band's
// available content width wraps to MULTIPLE lines, each line clamped to the band
// width (no right-edge overflow — the §11 narrow-terminal fix).
func TestNoticeBand_WrapsLongMessage(t *testing.T) {
	const w = 30
	band := renderNoticeBand(bandInfo, longBandMessage, theme.MV.TextOnSelection, w, theme.Dark, false)

	lines := strings.Split(band, "\n")
	if len(lines) < 2 {
		t.Fatalf("long message did not wrap: got %d line(s), want >1\n%s", len(lines), band)
	}
	for i, l := range lines {
		if got := lipgloss.Width(l); got != w {
			t.Errorf("wrapped line %d width = %d, want %d (clamped to band width, no overflow)", i, got, w)
		}
	}
	// The full message survives across the wrapped lines.
	if flat := flattenNoticeBand(band); !strings.Contains(flat, longBandMessage) {
		t.Errorf("wrapped band dropped the message: flat=%q want contains %q", flat, longBandMessage)
	}
}

// TestNoticeBand_BarOnEveryWrappedLine asserts the `▌` left-bar renders on EVERY
// wrapped line (so the bar spans the band's full height), in the role colour, and
// — under the NO_COLOR carve-out — the bar glyph survives on every line.
func TestNoticeBand_BarOnEveryWrappedLine(t *testing.T) {
	const w = 30

	t.Run("coloured/role-bar-every-line", func(t *testing.T) {
		band := renderNoticeBand(bandWarning, longBandMessage, theme.MV.TextOnWarning, w, theme.Dark, false)
		lines := strings.Split(band, "\n")
		if len(lines) < 2 {
			t.Fatalf("setup: message did not wrap (%d lines)", len(lines))
		}
		barSeq := tokenFgSeq(t, theme.MV.AccentOrange, theme.Dark)
		for i, l := range lines {
			if !strings.HasPrefix(ansi.Strip(l), noticeBarGlyph) {
				t.Errorf("line %d does not start with the %q bar: %q", i, noticeBarGlyph, ansi.Strip(l))
			}
			if !strings.Contains(l, barSeq) {
				t.Errorf("line %d missing the role bar foreground sequence %q:\n%s", i, barSeq, l)
			}
		}
	})

	t.Run("nocolor/bar-glyph-every-line", func(t *testing.T) {
		band := renderNoticeBand(bandWarning, longBandMessage, theme.MV.TextOnWarning, w, theme.Dark, true)
		lines := strings.Split(band, "\n")
		if len(lines) < 2 {
			t.Fatalf("setup: message did not wrap (%d lines)", len(lines))
		}
		for i, l := range lines {
			if !strings.HasPrefix(l, noticeBarGlyph) {
				t.Errorf("NO_COLOR line %d does not start with the %q bar: %q", i, noticeBarGlyph, l)
			}
		}
		// No colour sequences at all under the carve-out.
		if band != ansi.Strip(band) {
			t.Errorf("NO_COLOR wrapped band must carry no SGR colour sequences; got raw %q", band)
		}
	})
}

// TestNoticeBand_ContinuationLinesAlignUnderMessage asserts continuation lines
// indent their text under line 1's message start — past the bar + gap [+ glyph +
// gap] — and the status glyph appears ONLY on line 1.
func TestNoticeBand_ContinuationLinesAlignUnderMessage(t *testing.T) {
	const w = 30
	// A warning flash carries the ⚠ glyph, so the message starts at column 4
	// (bar + gap + glyph + gap); continuation text must start at the same column.
	band := renderNoticeBand(bandWarning, longBandMessage, theme.MV.TextOnWarning, w, theme.Dark, true)
	lines := strings.Split(ansi.Strip(band), "\n")
	if len(lines) < 2 {
		t.Fatalf("setup: message did not wrap (%d lines)", len(lines))
	}

	// Line 1 carries the glyph; the message starts at the cell after `▌ ⚠ `.
	const msgStartCol = 4 // bar(1) + gap(1) + glyph(1) + gap(1)
	if got := []rune(lines[0]); string(got[:msgStartCol]) != "▌ "+flashWarningGlyph+" " {
		t.Errorf("line 1 prefix = %q, want %q", string(got[:msgStartCol]), "▌ "+flashWarningGlyph+" ")
	}

	for i := 1; i < len(lines); i++ {
		runes := []rune(lines[i])
		// Glyph must NOT appear on continuation lines.
		if strings.Contains(lines[i], flashWarningGlyph) {
			t.Errorf("continuation line %d carries the status glyph %q (glyph is line 1 only): %q", i, flashWarningGlyph, lines[i])
		}
		// The bar is present; the cells between the bar and the message-start column
		// are blank (the continuation indent), so the wrapped text lines up under
		// line 1's message.
		if runes[0] != []rune(noticeBarGlyph)[0] {
			t.Errorf("continuation line %d does not start with the bar: %q", i, lines[i])
		}
		for c := 1; c < msgStartCol; c++ {
			if runes[c] != ' ' {
				t.Errorf("continuation line %d indent cell %d = %q, want a space (text must align under line 1's message)", i, c, string(runes[c]))
			}
		}
		if runes[msgStartCol] == ' ' {
			t.Errorf("continuation line %d has no text at the message-start column %d: %q", i, msgStartCol, lines[i])
		}
	}
}

// TestNoticeBand_FlashTintSpansEveryWrappedLine asserts the flash tint (bg.warning)
// is painted on EVERY wrapped line and each line is padded to the full width — no
// terminal-bg island on any line (the §11.2 tint must span the multi-line band).
func TestNoticeBand_FlashTintSpansEveryWrappedLine(t *testing.T) {
	const w = 30
	band := renderNoticeBand(bandWarning, longBandMessage, theme.MV.TextOnWarning, w, theme.Dark, false)
	lines := strings.Split(band, "\n")
	if len(lines) < 2 {
		t.Fatalf("setup: message did not wrap (%d lines)", len(lines))
	}
	tintSeq := tokenBgSeq(t, theme.MV.BgWarning, theme.Dark)
	for i, l := range lines {
		if !strings.Contains(l, tintSeq) {
			t.Errorf("wrapped line %d missing the bg.warning tint %q (no tint island allowed):\n%s", i, tintSeq, l)
		}
		// Each line padded to the full width (the pad carries the tint to the edge).
		if got := lipgloss.Width(l); got != w {
			t.Errorf("wrapped line %d width = %d, want %d (padded to full width)", i, got, w)
		}
	}
}

// TestNoticeBand_ShortMessageSingleLine asserts a message that fits within the
// available content width renders as a SINGLE line (no regression from the wrap
// change).
func TestNoticeBand_ShortMessageSingleLine(t *testing.T) {
	const w = 60
	band := renderNoticeBand(bandWarning, "short notice", theme.MV.TextOnWarning, w, theme.Dark, false)
	if h := lipgloss.Height(band); h != 1 {
		t.Errorf("short message band height = %d, want 1 (single line)\n%s", h, band)
	}
	if got := lipgloss.Width(band); got != w {
		t.Errorf("short message band width = %d, want %d (full width)", got, w)
	}
}

// TestSessionBandHeight_TracksWrappedLineCount asserts the §11.2 F10 reserve tracks
// the WRAPPED band height: at a narrow width the signpost band wraps to multiple
// lines and sessionBandHeight reflects that count PLUS the one blank breathing row,
// so the list reserves the correct number of rows and the composed view stays within
// termH. Measured off the SAME renderSessionBandSlot block the view composes.
func TestSessionBandHeight_TracksWrappedLineCount(t *testing.T) {
	dir := t.TempDir()
	projects := []project.Project{{Path: dir, Name: "Portal"}}
	sessions := []tmux.Session{{Name: "portal-abc", Dir: dir}}

	// Narrow width so the long signpost wraps; tall enough that the list survives.
	m := newRebuildTestModel(prefs.ModeByTag, sessions, projects)
	m.termWidth = 40
	m.termHeight = 40
	m.rebuildSessionList()
	if !m.byTagSignpost {
		t.Fatalf("setup invariant: byTagSignpost = false, want true")
	}

	bandHeight := lipgloss.Height(m.renderActiveNoticeBand())
	if bandHeight < 2 {
		t.Fatalf("setup: signpost did not wrap at the narrow width (band height %d, want >1)", bandHeight)
	}
	// The slot is the wrapped band PLUS one blank breathing row.
	if got, want := m.sessionBandHeight(), bandHeight+1; got != want {
		t.Errorf("sessionBandHeight = %d, want %d (wrapped band height %d + 1 blank)", got, want, bandHeight)
	}
}

// TestNoticeBand_WrappedFrameHeightStaysTermH asserts the composed frame height
// stays exactly termH when the band WRAPS at a narrow width — the wrapped band +
// blank is absorbed by the list shrinking underneath the outer canvas fill, so the
// one-row-per-delegate pagination invariant still holds (the wrapped band does not
// push the frame past termH).
func TestNoticeBand_WrappedFrameHeightStaysTermH(t *testing.T) {
	dir := t.TempDir()
	projects := []project.Project{{Path: dir, Name: "Portal"}}
	sessions := []tmux.Session{{Name: "portal-abc", Dir: dir}}

	m := newRebuildTestModel(prefs.ModeByTag, sessions, projects)
	m.termWidth = 40
	m.termHeight = 40
	m.applySessionListSize(m.termWidth, m.termHeight)
	m.rebuildSessionList()
	if !m.byTagSignpost {
		t.Fatalf("setup invariant: byTagSignpost = false, want true")
	}
	if h := lipgloss.Height(m.renderActiveNoticeBand()); h < 2 {
		t.Fatalf("setup: signpost did not wrap at the narrow width (band height %d)", h)
	}

	lines := strings.Split(m.View().Content, "\n")
	if len(lines) != m.termHeight {
		t.Errorf("composed frame height = %d, want termHeight %d (wrapped band must not push the frame past termH)", len(lines), m.termHeight)
	}
}
