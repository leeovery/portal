package tui

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/tmux"
)

func flattenNoticeBand(band string) string {
	frags := make([]string, 0, 4)
	for line := range strings.SplitSeq(ansi.Strip(band), "\n") {
		body := strings.TrimPrefix(line, noticeBarGlyph)
		body = strings.TrimLeft(body, " ")
		body = strings.TrimRight(body, " ")
		if body != "" {
			frags = append(frags, body)
		}
	}
	return strings.Join(frags, " ")
}

func viewHasNoticeMessage(t *testing.T, m Model, role noticeBandRole, message string) bool {
	t.Helper()
	band := renderNoticeBand(role, message, noticeBandOnBandText(role, m.themeState.active), m.contentWidth(), m.themeState.active, m.colourless)
	view := ansi.Strip(m.View().Content)
	for line := range strings.SplitSeq(ansi.Strip(band), "\n") {
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
		{"warning/orange", bandWarning, testDarkTheme(t).AccentAttention, testDarkTheme(t).TextOnAttention},
		{"success/green", bandSuccess, testDarkTheme(t).StatePositive, testDarkTheme(t).TextSecondary},
		{"info/violet", bandInfo, testDarkTheme(t).AccentPrimary, testDarkTheme(t).TextOnSelection},
	} {
		t.Run(tc.name, func(t *testing.T) {
			band := renderNoticeBand(tc.role, msg, tc.onBandTok, w, testDarkTheme(t), false)

			if h := lipgloss.Height(band); h != 1 {
				t.Errorf("band height = %d, want 1 (single line)", h)
			}
			if got := lipgloss.Width(band); got != w {
				t.Errorf("band width = %d, want %d (full-width)", got, w)
			}
			stripped := ansi.Strip(band)
			if !strings.HasPrefix(stripped, noticeBarGlyph) {
				t.Errorf("band does not start with the %q left-bar: %q", noticeBarGlyph, stripped)
			}
			if !strings.Contains(stripped, msg) {
				t.Errorf("band missing the message %q: %q", msg, stripped)
			}
			barSeq := tokenFgSeq(t, tc.barTok)
			if !strings.Contains(band, barSeq) {
				t.Errorf("band missing the %s bar foreground sequence %q:\n%s", tc.name, barSeq, band)
			}
			msgSeq := tokenFgSeq(t, tc.onBandTok)
			if !strings.Contains(band, msgSeq) {
				t.Errorf("band missing the on-band text foreground sequence %q:\n%s", msgSeq, band)
			}
		})
	}
}

func TestRenderNoticeBand_NoColor(t *testing.T) {
	const (
		w   = 60
		msg = "nocolor-band-probe"
	)
	band := renderNoticeBand(bandInfo, msg, testDarkTheme(t).TextOnSelection, w, testDarkTheme(t), true)

	stripped := ansi.Strip(band)
	if !strings.HasPrefix(stripped, noticeBarGlyph) {
		t.Errorf("NO_COLOR band must keep the %q left-bar: %q", noticeBarGlyph, stripped)
	}
	if !strings.Contains(stripped, msg) {
		t.Errorf("NO_COLOR band must keep the message %q: %q", msg, stripped)
	}
	if band != stripped {
		t.Errorf("NO_COLOR band must carry no SGR colour sequences; got raw %q", band)
	}
}

func TestNoticeSlot_SingleBand_TransientFlashWins(t *testing.T) {
	dir := t.TempDir()
	projects := []project.Project{{Path: dir, Name: "Portal"}}
	sessions := []tmux.Session{{Name: "portal-abc", Dir: dir}}

	m := newRebuildTestModel(t, prefs.ModeByTag, sessions, projects)
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

func TestNoticeSlot_PersistentReturnsAfterFlashClear(t *testing.T) {
	dir := t.TempDir()
	projects := []project.Project{{Path: dir, Name: "Portal"}}
	sessions := []tmux.Session{{Name: "portal-abc", Dir: dir}}

	m := newRebuildTestModel(t, prefs.ModeByTag, sessions, projects)
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

func TestNoticeSlot_NeverBothBandsSimultaneously(t *testing.T) {
	dir := t.TempDir()
	projects := []project.Project{{Path: dir, Name: "Portal"}}
	sessions := []tmux.Session{{Name: "portal-abc", Dir: dir}}

	m := newRebuildTestModel(t, prefs.ModeByTag, sessions, projects)
	m.termWidth = 80
	m.termHeight = 24
	m.rebuildSessionList()

	const flash = "__TRANSIENT_FLASH__"
	m.setFlash(flash)

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
	if bandIdx <= ruleIdx {
		t.Errorf("band index %d must be > separator-rule index %d (band under the separator)", bandIdx, ruleIdx)
	}
	if sectionIdx <= bandIdx {
		t.Errorf("section header index %d must be > band index %d (band above the section header)", sectionIdx, bandIdx)
	}
	if sectionIdx-bandIdx != 2 {
		t.Errorf("section header is %d rows below the band, want 2 (band → blank → section header)", sectionIdx-bandIdx)
	}
	blankIdx := bandIdx + 1
	blank := ansi.Strip(afterLines[blankIdx])
	if strings.TrimSpace(blank) != "" {
		t.Errorf("row between the band and section header must be blank, got %q", blank)
	}
	if sectionIdx-beforeSection != 2 {
		t.Errorf("section header shift = %d, want +2 (band + blank push it down two rows)", sectionIdx-beforeSection)
	}
	if rowIdx-beforeRow != 2 {
		t.Errorf("session row shift = %d, want +2 (band + blank push it down two rows)", rowIdx-beforeRow)
	}
}

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

func TestNoticeBand_FlashGenerationGuardPreserved(t *testing.T) {
	m := noticeBandModel("alpha-row")
	m.setFlash("first")
	m.setFlash("second")

	updated, _ := m.Update(flashTickMsg{Gen: 1})
	mm := updated.(Model)
	if mm.flashText != "second" {
		t.Errorf("superseded tick must not early-clear: flashText = %q, want %q", mm.flashText, "second")
	}
}

func TestNoticeBand_ActionableKeyClearsFlash(t *testing.T) {
	m := noticeBandModel("alpha-row")
	m.setFlash("clear-me")

	updated, _ := m.updateSessionList(tea.KeyPressMsg{Code: tea.KeyDown})
	mm := updated.(Model)
	if mm.flashText != "" {
		t.Errorf("actionable key must clear the active flash: flashText = %q, want empty", mm.flashText)
	}
}

func TestNoticeBand_TimeoutClearsFlash(t *testing.T) {
	m := noticeBandModel("alpha-row")
	m.setFlash("timeout-me")

	updated, _ := m.Update(flashTickMsg{Gen: 1})
	mm := updated.(Model)
	if mm.flashText != "" {
		t.Errorf("matching tick (timeout) must clear the flash: flashText = %q, want empty", mm.flashText)
	}
}

const longBandMessage = byTagSignpostText

func TestNoticeBand_WrapsLongMessage(t *testing.T) {
	const w = 30
	band := renderNoticeBand(bandInfo, longBandMessage, testDarkTheme(t).TextOnSelection, w, testDarkTheme(t), false)

	lines := strings.Split(band, "\n")
	if len(lines) < 2 {
		t.Fatalf("long message did not wrap: got %d line(s), want >1\n%s", len(lines), band)
	}
	for i, l := range lines {
		if got := lipgloss.Width(l); got != w {
			t.Errorf("wrapped line %d width = %d, want %d (clamped to band width, no overflow)", i, got, w)
		}
	}
	if flat := flattenNoticeBand(band); !strings.Contains(flat, longBandMessage) {
		t.Errorf("wrapped band dropped the message: flat=%q want contains %q", flat, longBandMessage)
	}
}

func TestNoticeBand_BarOnEveryWrappedLine(t *testing.T) {
	const w = 30

	t.Run("coloured/role-bar-every-line", func(t *testing.T) {
		band := renderNoticeBand(bandWarning, longBandMessage, testDarkTheme(t).TextOnAttention, w, testDarkTheme(t), false)
		lines := strings.Split(band, "\n")
		if len(lines) < 2 {
			t.Fatalf("setup: message did not wrap (%d lines)", len(lines))
		}
		barSeq := tokenFgSeq(t, testDarkTheme(t).AccentAttention)
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
		band := renderNoticeBand(bandWarning, longBandMessage, testDarkTheme(t).TextOnAttention, w, testDarkTheme(t), true)
		lines := strings.Split(band, "\n")
		if len(lines) < 2 {
			t.Fatalf("setup: message did not wrap (%d lines)", len(lines))
		}
		for i, l := range lines {
			if !strings.HasPrefix(l, noticeBarGlyph) {
				t.Errorf("NO_COLOR line %d does not start with the %q bar: %q", i, noticeBarGlyph, l)
			}
		}
		if band != ansi.Strip(band) {
			t.Errorf("NO_COLOR wrapped band must carry no SGR colour sequences; got raw %q", band)
		}
	})
}

func TestNoticeBand_ContinuationLinesAlignUnderMessage(t *testing.T) {
	const w = 30
	band := renderNoticeBand(bandWarning, longBandMessage, testDarkTheme(t).TextOnAttention, w, testDarkTheme(t), true)
	lines := strings.Split(ansi.Strip(band), "\n")
	if len(lines) < 2 {
		t.Fatalf("setup: message did not wrap (%d lines)", len(lines))
	}

	const msgStartCol = 4
	if got := []rune(lines[0]); string(got[:msgStartCol]) != "▌ "+flashWarningGlyph+" " {
		t.Errorf("line 1 prefix = %q, want %q", string(got[:msgStartCol]), "▌ "+flashWarningGlyph+" ")
	}

	for i := 1; i < len(lines); i++ {
		runes := []rune(lines[i])
		if strings.Contains(lines[i], flashWarningGlyph) {
			t.Errorf("continuation line %d carries the status glyph %q (glyph is line 1 only): %q", i, flashWarningGlyph, lines[i])
		}
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

func TestNoticeBand_FlashTintSpansEveryWrappedLine(t *testing.T) {
	const w = 30
	band := renderNoticeBand(bandWarning, longBandMessage, testDarkTheme(t).TextOnAttention, w, testDarkTheme(t), false)
	lines := strings.Split(band, "\n")
	if len(lines) < 2 {
		t.Fatalf("setup: message did not wrap (%d lines)", len(lines))
	}
	tintSeq := tokenBgSeq(t, testDarkTheme(t).BgAttention)
	for i, l := range lines {
		if !strings.Contains(l, tintSeq) {
			t.Errorf("wrapped line %d missing the bg.attention tint %q (no tint island allowed):\n%s", i, tintSeq, l)
		}
		if got := lipgloss.Width(l); got != w {
			t.Errorf("wrapped line %d width = %d, want %d (padded to full width)", i, got, w)
		}
	}
}

func TestNoticeBand_ShortMessageSingleLine(t *testing.T) {
	const w = 60
	band := renderNoticeBand(bandWarning, "short notice", testDarkTheme(t).TextOnAttention, w, testDarkTheme(t), false)
	if h := lipgloss.Height(band); h != 1 {
		t.Errorf("short message band height = %d, want 1 (single line)\n%s", h, band)
	}
	if got := lipgloss.Width(band); got != w {
		t.Errorf("short message band width = %d, want %d (full width)", got, w)
	}
}

func TestSessionBandHeight_TracksWrappedLineCount(t *testing.T) {
	dir := t.TempDir()
	projects := []project.Project{{Path: dir, Name: "Portal"}}
	sessions := []tmux.Session{{Name: "portal-abc", Dir: dir}}

	m := newRebuildTestModel(t, prefs.ModeByTag, sessions, projects)
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
	if got, want := m.sessionBandHeight(), bandHeight+1; got != want {
		t.Errorf("sessionBandHeight = %d, want %d (wrapped band height %d + 1 blank)", got, want, bandHeight)
	}
}

func TestNoticeBand_WrappedFrameHeightStaysTermH(t *testing.T) {
	dir := t.TempDir()
	projects := []project.Project{{Path: dir, Name: "Portal"}}
	sessions := []tmux.Session{{Name: "portal-abc", Dir: dir}}

	m := newRebuildTestModel(t, prefs.ModeByTag, sessions, projects)
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

// Neither reproduces at today's copy: they stand in for the first band message
// to quote a path or a bundle id.
const (
	bandPathMessage   = "couldn't read /Users/leeovery/Code/portal/internal/tui/testdata/vhs/reference/sessions-multi-select-active.png"
	bandBundleMessage = "can't open new windows in Warp · dev.warp.Warp-Stable-x86_64 — nothing opened"
)

func bandShippedCopy() []struct{ name, message string } {
	named := spawn.Identity{Name: "Warp", BundleID: "dev.warp.Warp-Stable"}
	return []struct{ name, message string }{
		{"the by-tag signpost", byTagSignpostText},
		{"the remote no-op", spawn.UnsupportedNoopMessage(spawn.Identity{})},
		{"the named no-op", spawn.UnsupportedNoopMessage(named)},
		{"the remote multi-select refusal", multiSelectBlockedRemoteFlash},
		{"the named multi-select refusal", multiSelectBlockedNamedFlash},
	}
}

func bandRows(t *testing.T, message string, width int) []string {
	t.Helper()
	return strings.Split(renderNoticeBand(bandWarning, message, testDarkTheme(t).TextOnAttention, width, testDarkTheme(t), true), "\n")
}

func TestNoticeBand_RowsFitTheBand(t *testing.T) {
	t.Run("it renders no band row wider than the band", func(t *testing.T) {
		for _, tc := range []struct{ name, message string }{
			{"a path", bandPathMessage},
			{"a bundle id", bandBundleMessage},
		} {
			t.Run(tc.name, func(t *testing.T) {
				for width := 10; width <= 120; width++ {
					for i, row := range bandRows(t, tc.message, width) {
						if got := lipgloss.Width(row); got != width {
							t.Errorf("at a band width of %d row %d renders at %d cells: %q", width, i, got, row)
						}
					}
				}
			})
		}
	})

	t.Run("it leaves today's shipped copy reading the same", func(t *testing.T) {
		for _, tc := range bandShippedCopy() {
			t.Run(tc.name, func(t *testing.T) {
				for width := 20; width <= 120; width++ {
					want := bareWrapRows(tc.message, noticeBandAvail(width))
					got := make([]string, 0, len(want))
					for _, row := range bandRows(t, tc.message, width) {
						got = append(got, bandRowBody(row))
					}
					if !slices.Equal(got, want) {
						t.Errorf("at a band width of %d the copy reads\n got: %q\nwant: %q", width, got, want)
					}
				}
			})
		}
	})
}

// The message text a rendered row carries, with the bar, the status glyph and
// the continuation indent taken off.
func bandRowBody(row string) string {
	body := strings.TrimLeft(strings.TrimPrefix(ansi.Strip(row), noticeBarGlyph), " ")
	return strings.Trim(strings.TrimPrefix(body, flashWarningGlyph), " ")
}

// The rows the band rendered before it took the shared wrap.
func bareWrapRows(message string, avail int) []string {
	rows := strings.Split(ansi.Wrap(message, avail, ""), "\n")
	for i, row := range rows {
		rows[i] = strings.Trim(row, " ")
	}
	return rows
}

func noticeBandAvail(width int) int {
	return max(width-(lipgloss.Width(noticeBarGlyph)+1+lipgloss.Width(flashWarningGlyph)+1), 1)
}

func TestNoticeBand_OverPackedMessageCostsAnotherRow(t *testing.T) {
	// The width the wrap over-packs this message at.
	const width = 19

	t.Run("it costs one more row for a message that over-packs", func(t *testing.T) {
		rows := bandRows(t, bandBundleMessage, width)
		bare := bareWrapRows(bandBundleMessage, noticeBandAvail(width))
		if want := len(bare) + 1; len(rows) != want {
			t.Errorf("the band renders %d rows, want %d — the overshoot re-flowed rather than running off the row:\n%q", len(rows), want, rows)
		}
		bodies := make([]string, 0, len(rows))
		for _, row := range rows {
			bodies = append(bodies, bandRowBody(row))
		}
		if got, want := spacelessText(strings.Join(bodies, "")), spacelessText(bandBundleMessage); got != want {
			t.Errorf("the band reads %q, want the whole message %q", got, want)
		}
	})

	t.Run("the page budget counts the row it costs", func(t *testing.T) {
		m := noticeBandModel("alpha-row")
		m.termWidth = termWidthForContent(t, m, width)
		m.termHeight = 24
		m.applySessionListSize(m.termWidth, m.termHeight)
		m.setFlash(bandBundleMessage)
		m.applySessionListSize(m.termWidth, m.termHeight)

		slot := lipgloss.Height(m.renderSessionBandSlot())
		if want := len(bandRows(t, bandBundleMessage, width)) + 1; slot != want {
			t.Fatalf("the band slot is %d rows, want the re-flowed band's %d plus its blank", slot, want)
		}
		if got := m.pageBandHeight(); got != slot {
			t.Errorf("pageBandHeight = %d, want the rendered slot's %d rows", got, slot)
		}
		if lines := strings.Split(m.View().Content, "\n"); len(lines) != m.termHeight {
			t.Errorf("the composed frame is %d rows, want termHeight %d", len(lines), m.termHeight)
		}
	})
}

func termWidthForContent(t *testing.T, m Model, content int) int {
	t.Helper()
	for w := content; w <= content+16; w++ {
		m.termWidth = w
		if m.contentWidth() == content {
			return w
		}
	}
	t.Fatalf("no terminal width renders a content region of %d cells", content)
	return 0
}
