package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui/theme"
)

// Tests for the §11.2 inline-flash band reskin (task 4-2): the full MV warning /
// success styling routed through the task-4-1 notice-band primitive + single-slot
// arbiter. The warning flash is an accent.orange left-bar + ⚠ glyph + message on a
// bg.warning tint in text.on-warning; the success flash swaps to a state.green
// left-bar + ✓ glyph (glyph-distinct from ⚠, never colour-only — §2.2). The
// reskin must not perturb the auto-clear lifecycle or the F10 height recompute.

// reskinFlashModel builds a Sessions-page model seeded with the given session
// names at 80x24 so the rendered band carries predictable substrings.
func reskinFlashModel(names ...string) Model {
	sessions := make([]tmux.Session, 0, len(names))
	for _, n := range names {
		sessions = append(sessions, tmux.Session{Name: n, Windows: 1, Attached: false})
	}
	m := NewModelWithSessions(sessions)
	m.termWidth = 80
	m.termHeight = 24
	return m
}

// flashBandLine returns the single rendered band line carrying substr, or fails.
func flashBandLine(t *testing.T, m Model, substr string) string {
	t.Helper()
	lines := strings.Split(m.View().Content, "\n")
	idx := lineIndexContaining(lines, substr)
	if idx < 0 {
		t.Fatalf("band line containing %q not found in render:\n%s", substr, strings.Join(lines, "\n"))
	}
	return lines[idx]
}

// TestWarningFlash_OrangeBarWarningGlyphOnWarningTint asserts the §11.2 warning
// flash: an accent.orange ▌ left-bar, a ⚠ warning glyph, and the message in
// text.on-warning on a bg.warning tint.
func TestWarningFlash_OrangeBarWarningGlyphOnWarningTint(t *testing.T) {
	m := reskinFlashModel("alpha-row")
	const msg = "folio-Jiz4el closed externally — list updated"
	m.setFlash(msg)

	line := flashBandLine(t, m, msg)
	stripped := ansi.Strip(line)

	// Left-bar present at the far left, then the warning glyph, then the message.
	if !strings.HasPrefix(strings.TrimLeft(stripped, " "), noticeBarGlyph) {
		t.Errorf("warning flash must start with the %q left-bar: %q", noticeBarGlyph, stripped)
	}
	if !strings.Contains(stripped, flashWarningGlyph) {
		t.Errorf("warning flash must carry the %q glyph: %q", flashWarningGlyph, stripped)
	}
	if !strings.Contains(stripped, msg) {
		t.Errorf("warning flash must carry the message %q: %q", msg, stripped)
	}

	// accent.orange left-bar foreground.
	if barSeq := tokenFgSeq(t, theme.MV.AccentOrange, theme.Dark); !strings.Contains(line, barSeq) {
		t.Errorf("warning flash missing accent.orange bar foreground %q:\n%s", barSeq, line)
	}
	// text.on-warning message foreground.
	if msgSeq := tokenFgSeq(t, theme.MV.TextOnWarning, theme.Dark); !strings.Contains(line, msgSeq) {
		t.Errorf("warning flash missing text.on-warning message foreground %q:\n%s", msgSeq, line)
	}
	// bg.warning tint behind the band.
	if tintSeq := bgSeq(t, theme.MV.BgWarning, theme.Dark); !strings.Contains(line, tintSeq) {
		t.Errorf("warning flash missing bg.warning tint background %q:\n%s", tintSeq, line)
	}
}

// TestSuccessFlash_GreenBarSuccessGlyph asserts the §11.2 success variant: a
// state.green ▌ left-bar and a ✓ success glyph with the message.
func TestSuccessFlash_GreenBarSuccessGlyph(t *testing.T) {
	m := reskinFlashModel("alpha-row")
	const msg = "session restored"
	m.setSuccessFlash(msg)

	line := flashBandLine(t, m, msg)
	stripped := ansi.Strip(line)

	if !strings.HasPrefix(strings.TrimLeft(stripped, " "), noticeBarGlyph) {
		t.Errorf("success flash must start with the %q left-bar: %q", noticeBarGlyph, stripped)
	}
	if !strings.Contains(stripped, flashSuccessGlyph) {
		t.Errorf("success flash must carry the %q glyph: %q", flashSuccessGlyph, stripped)
	}
	if !strings.Contains(stripped, msg) {
		t.Errorf("success flash must carry the message %q: %q", msg, stripped)
	}

	// state.green left-bar foreground.
	if barSeq := tokenFgSeq(t, theme.MV.StateGreen, theme.Dark); !strings.Contains(line, barSeq) {
		t.Errorf("success flash missing state.green bar foreground %q:\n%s", barSeq, line)
	}
}

// TestFlash_WarningVsSuccessGlyphDistinct asserts the two variants are
// distinguishable by GLYPH alone (§2.2 — never colour-only): the warning band
// carries ⚠ and not ✓, the success band carries ✓ and not ⚠.
func TestFlash_WarningVsSuccessGlyphDistinct(t *testing.T) {
	if flashWarningGlyph == flashSuccessGlyph {
		t.Fatalf("warning and success glyphs must differ: both = %q", flashWarningGlyph)
	}

	warnM := reskinFlashModel("alpha-row")
	warnM.setFlash("a warning")
	warn := ansi.Strip(flashBandLine(t, warnM, "a warning"))
	if !strings.Contains(warn, flashWarningGlyph) || strings.Contains(warn, flashSuccessGlyph) {
		t.Errorf("warning band must carry only %q (not %q): %q", flashWarningGlyph, flashSuccessGlyph, warn)
	}

	okM := reskinFlashModel("alpha-row")
	okM.setSuccessFlash("a success")
	ok := ansi.Strip(flashBandLine(t, okM, "a success"))
	if !strings.Contains(ok, flashSuccessGlyph) || strings.Contains(ok, flashWarningGlyph) {
		t.Errorf("success band must carry only %q (not %q): %q", flashSuccessGlyph, flashWarningGlyph, ok)
	}
}

// TestFlashKind_DefaultsToWarning asserts setFlash defaults the kind to warning so
// the externally-killed bail (which calls setFlash) stays a warning band.
func TestFlashKind_DefaultsToWarning(t *testing.T) {
	var m Model
	m.setFlash("default kind")
	if m.flashKind != flashWarning {
		t.Errorf("setFlash kind = %v, want flashWarning (default)", m.flashKind)
	}
	// A success flash followed by a plain setFlash must revert to warning.
	m.setSuccessFlash("ok")
	if m.flashKind != flashSuccess {
		t.Fatalf("setup invariant: setSuccessFlash kind = %v, want flashSuccess", m.flashKind)
	}
	m.setFlash("back to warning")
	if m.flashKind != flashWarning {
		t.Errorf("setFlash after setSuccessFlash kind = %v, want flashWarning (resets to default)", m.flashKind)
	}
}

// TestActiveNoticeBand_FlashKindSelectsRole asserts the arbiter maps the flash
// kind to the band role: warning → bandWarning, success → bandSuccess.
func TestActiveNoticeBand_FlashKindSelectsRole(t *testing.T) {
	var m Model
	m.setFlash("warn")
	if role, _, ok := m.activeNoticeBand(); !ok || role != bandWarning {
		t.Errorf("warning flash role = %v ok=%v, want bandWarning true", role, ok)
	}
	m.setSuccessFlash("done")
	if role, _, ok := m.activeNoticeBand(); !ok || role != bandSuccess {
		t.Errorf("success flash role = %v ok=%v, want bandSuccess true", role, ok)
	}
}

// TestFlashReskin_NoColor asserts the NO_COLOR carve-out (§2.5 / §2.2): the band
// drops the tint + bar colour but keeps the ▌ bar, its position, the warning/
// success glyph, and the message — state survives colourlessly through the glyph.
func TestFlashReskin_NoColor(t *testing.T) {
	for _, tc := range []struct {
		name  string
		set   func(*Model)
		glyph string
	}{
		{"warning", func(m *Model) { m.setFlash("nocolor warning") }, flashWarningGlyph},
		{"success", func(m *Model) { m.setSuccessFlash("nocolor success") }, flashSuccessGlyph},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sessions := []tmux.Session{{Name: "alpha-row", Windows: 1}}
			m := NewModelWithSessions(sessions)
			m.termWidth = 80
			m.termHeight = 24
			m.colourless = true
			m.applyCanvasMode()
			tc.set(&m)

			band := m.renderActiveNoticeBand()
			stripped := ansi.Strip(band)
			// Bar + glyph survive.
			if !strings.HasPrefix(strings.TrimLeft(stripped, " "), noticeBarGlyph) {
				t.Errorf("NO_COLOR %s flash must keep the %q bar: %q", tc.name, noticeBarGlyph, stripped)
			}
			if !strings.Contains(stripped, tc.glyph) {
				t.Errorf("NO_COLOR %s flash must keep the %q glyph: %q", tc.name, tc.glyph, stripped)
			}
			// No SGR colour sequences at all — tint + bar colour dropped.
			if band != stripped {
				t.Errorf("NO_COLOR %s flash must carry no SGR colour sequences; got raw %q", tc.name, band)
			}
		})
	}
}

// TestFlashReskin_RecomputesListHeight asserts the F10 height recompute still
// fires through the reskin: a flash appearing reserves TWO fewer list rows (the
// band PLUS its blank breathing row), a clear releases both (warning and success
// alike).
func TestFlashReskin_RecomputesListHeight(t *testing.T) {
	m := reskinFlashModel("alpha-row")
	_, base := m.SessionListSize()

	m.setSuccessFlash("appear")
	_, withBand := m.SessionListSize()
	if withBand != base-2 {
		t.Errorf("list height with success band = %d, want %d (band + blank, two rows consumed)", withBand, base-2)
	}

	m.clearFlash()
	_, cleared := m.SessionListSize()
	if cleared != base {
		t.Errorf("list height after clear = %d, want %d (both rows released)", cleared, base)
	}
}

// TestFlashReskin_AutoClearLifecyclePreserved asserts the reskin does not perturb
// the auto-clear lifecycle: an actionable keypress clears the band, a matching
// flashTickMsg (the short timeout) clears it, and a superseded tick is dropped by
// the generation guard — for the success kind too.
func TestFlashReskin_AutoClearLifecyclePreserved(t *testing.T) {
	t.Run("actionable key clears success flash", func(t *testing.T) {
		m := reskinFlashModel("alpha-row")
		m.setSuccessFlash("clear-me")
		updated, _ := m.updateSessionList(tea.KeyPressMsg{Code: tea.KeyDown})
		if mm := updated.(Model); mm.flashText != "" {
			t.Errorf("actionable key must clear the success flash: flashText = %q", mm.flashText)
		}
	})
	t.Run("matching tick clears success flash", func(t *testing.T) {
		m := reskinFlashModel("alpha-row")
		m.setSuccessFlash("timeout-me") // gen 1
		updated, _ := m.Update(flashTickMsg{Gen: 1})
		if mm := updated.(Model); mm.flashText != "" {
			t.Errorf("matching tick must clear the success flash: flashText = %q", mm.flashText)
		}
	})
	t.Run("generation guard drops superseded tick", func(t *testing.T) {
		m := reskinFlashModel("alpha-row")
		m.setFlash("first")         // gen 1
		m.setSuccessFlash("second") // gen 2 supersedes
		updated, _ := m.Update(flashTickMsg{Gen: 1})
		mm := updated.(Model)
		if mm.flashText != "second" {
			t.Errorf("superseded tick must not early-clear: flashText = %q, want %q", mm.flashText, "second")
		}
		if mm.flashKind != flashSuccess {
			t.Errorf("superseded tick must not perturb kind: flashKind = %v, want flashSuccess", mm.flashKind)
		}
	})
}

// reskinStubLister is a minimal internal-package SessionLister for the Build test.
type reskinStubLister struct{ sessions []tmux.Session }

func (l reskinStubLister) ListSessions() ([]tmux.Session, error) { return l.sessions, nil }

// TestBuild_InitialFlashSeedsWarningFlash asserts the Build seam seeds a warning
// inline flash from Deps.InitialFlash (the capture-fixture entry point) — the
// flash renders as a warning band on the first frame with no key sequence.
func TestBuild_InitialFlashSeedsWarningFlash(t *testing.T) {
	lister := reskinStubLister{sessions: []tmux.Session{{Name: "dev", Windows: 2}}}
	const msg = "folio-Jiz4el closed externally — list updated"
	m := Build(Deps{
		Lister:       lister,
		InitialFlash: msg,
	})
	if m.flashText != msg {
		t.Errorf("Build InitialFlash flashText = %q, want %q", m.flashText, msg)
	}
	if m.flashKind != flashWarning {
		t.Errorf("Build InitialFlash flashKind = %v, want flashWarning", m.flashKind)
	}
	// An empty InitialFlash leaves no flash (the no-op path for every other fixture).
	none := Build(Deps{Lister: lister})
	if none.flashText != "" {
		t.Errorf("empty InitialFlash must leave flashText empty, got %q", none.flashText)
	}
}

// TestBuild_InitialFlash_RendersWarningBand mirrors the capture-harness path
// end-to-end: Build with a pinned dark appearance + Deps.InitialFlash, then drive
// a WindowSizeMsg + SessionsMsg the way the live program does. The seeded warning
// band must render with its ⚠ glyph and message above the `Sessions` section
// header — the exact frame the sessions-inline-flash fixture screenshots.
func TestBuild_InitialFlash_RendersWarningBand(t *testing.T) {
	const msg = "folio-Jiz4el closed externally — list updated"
	sessions := []tmux.Session{
		{Name: "fab-flowx-explore", Windows: 3, Attached: true},
		{Name: "agentic-workflows-codify", Windows: 1},
	}
	m := Build(Deps{
		Lister:       reskinStubLister{sessions: sessions},
		Appearance:   prefs.AppearanceDark,
		InitialFlash: msg,
	})
	// Drive the live program's first messages: size, then the session ingestion.
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	updated, _ = m.Update(SessionsMsg{Sessions: sessions})
	m = updated.(Model)

	lines := strings.Split(m.View().Content, "\n")
	bandIdx := lineIndexContaining(lines, msg)
	sectionIdx := lineIndexContaining(lines, "Sessions")
	if bandIdx < 0 || sectionIdx < 0 {
		t.Fatalf("missing landmark: band=%d section=%d\n%s", bandIdx, sectionIdx, strings.Join(lines, "\n"))
	}
	if bandIdx >= sectionIdx {
		t.Errorf("flash band index %d must be < section-header index %d (band above section header)", bandIdx, sectionIdx)
	}
	band := lines[bandIdx]
	if !strings.Contains(ansi.Strip(band), flashWarningGlyph) {
		t.Errorf("seeded band missing the ⚠ glyph: %q", ansi.Strip(band))
	}
	if tintSeq := bgSeq(t, theme.MV.BgWarning, theme.Dark); !strings.Contains(band, tintSeq) {
		t.Errorf("seeded band missing the bg.warning tint %q:\n%s", tintSeq, band)
	}
}

// TestFlashReskin_BandFullWidthSingleLine asserts the reskinned band is one
// full-width line (the tint fills the whole row like the section header above it).
func TestFlashReskin_BandFullWidthSingleLine(t *testing.T) {
	m := reskinFlashModel("alpha-row")
	m.setFlash("width probe")
	band := m.renderActiveNoticeBand()
	if h := lipgloss.Height(band); h != 1 {
		t.Errorf("reskinned band height = %d, want 1", h)
	}
	if got, want := lipgloss.Width(band), m.contentWidth(); got != want {
		t.Errorf("reskinned band width = %d, want %d (full content width)", got, want)
	}
}
