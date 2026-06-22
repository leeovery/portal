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
		{"info/violet", bandInfo, theme.MV.AccentViolet, theme.MV.TextStrong},
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
	band := renderNoticeBand(bandInfo, msg, theme.MV.TextStrong, w, theme.Dark, true)

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
	if strings.Contains(view, byTagSignpostText) {
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
	if !strings.Contains(view, byTagSignpostText) {
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
// directly under the title separator, above the section header, and shifts the
// section header + list down by one row.
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
	// Section header + list shifted down by one row.
	if sectionIdx-beforeSection != 1 {
		t.Errorf("section header shift = %d, want +1 (band pushes it down one row)", sectionIdx-beforeSection)
	}
	if rowIdx-beforeRow != 1 {
		t.Errorf("session row shift = %d, want +1 (band pushes it down one row)", rowIdx-beforeRow)
	}
}

// TestNoticeBand_RecomputesViewportHeight asserts the list viewport height is
// recomputed when the band appears (one row consumed) and when it clears (one
// row released) — the §11.2 F10 contract. The composed frame height stays
// constant (the band is absorbed under the outer canvas fill), and the list
// reserves exactly one fewer row while the band is active.
func TestNoticeBand_RecomputesViewportHeight(t *testing.T) {
	m := noticeBandModel("alpha-row")

	_, baseHeight := m.SessionListSize()

	m.setFlash("__HEIGHT_FLASH__")
	_, withBandHeight := m.SessionListSize()
	if withBandHeight != baseHeight-1 {
		t.Errorf("list height with band = %d, want %d (one row consumed)", withBandHeight, baseHeight-1)
	}

	m.clearFlash()
	_, clearedHeight := m.SessionListSize()
	if clearedHeight != baseHeight {
		t.Errorf("list height after clear = %d, want %d (one row released)", clearedHeight, baseHeight)
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
