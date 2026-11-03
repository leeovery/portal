package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func reskinFlashModel(names ...string) Model {
	sessions := make([]Session, 0, len(names))
	for _, n := range names {
		sessions = append(sessions, Session{Name: n, Attached: false})
	}
	m := NewModelWithSessions(sessions)
	m.termWidth = 80
	m.termHeight = 24
	return m
}

func flashBandLine(t *testing.T, m Model, substr string) string {
	t.Helper()
	lines := strings.Split(m.View().Content, "\n")
	idx := lineIndexContaining(lines, substr)
	if idx < 0 {
		t.Fatalf("band line containing %q not found in render:\n%s", substr, strings.Join(lines, "\n"))
	}
	return lines[idx]
}

func TestWarningFlash_OrangeBarWarningGlyphOnWarningTint(t *testing.T) {
	m := reskinFlashModel("alpha-row")
	const msg = "folio-Jiz4el closed externally — list updated"
	m.setFlash(msg)

	line := flashBandLine(t, m, msg)
	stripped := ansi.Strip(line)

	if !strings.HasPrefix(strings.TrimLeft(stripped, " "), noticeBarGlyph) {
		t.Errorf("warning flash must start with the %q left-bar: %q", noticeBarGlyph, stripped)
	}
	if !strings.Contains(stripped, flashWarningGlyph) {
		t.Errorf("warning flash must carry the %q glyph: %q", flashWarningGlyph, stripped)
	}
	if !strings.Contains(stripped, msg) {
		t.Errorf("warning flash must carry the message %q: %q", msg, stripped)
	}

	if barSeq := tokenFgSeq(t, testDarkTheme(t).AccentAttention); !strings.Contains(line, barSeq) {
		t.Errorf("warning flash missing accent.attention bar foreground %q:\n%s", barSeq, line)
	}
	if msgSeq := tokenFgSeq(t, testDarkTheme(t).TextOnAttention); !strings.Contains(line, msgSeq) {
		t.Errorf("warning flash missing text.on-attention message foreground %q:\n%s", msgSeq, line)
	}
	if tintSeq := tokenBgSeq(t, testDarkTheme(t).BgAttention); !strings.Contains(line, tintSeq) {
		t.Errorf("warning flash missing bg.attention tint background %q:\n%s", tintSeq, line)
	}
}

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

	if barSeq := tokenFgSeq(t, testDarkTheme(t).StatePositive); !strings.Contains(line, barSeq) {
		t.Errorf("success flash missing state.positive bar foreground %q:\n%s", barSeq, line)
	}
}

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

func TestFlashKind_DefaultsToWarning(t *testing.T) {
	var m Model
	m.setFlash("default kind")
	if m.flashKind != flashWarning {
		t.Errorf("setFlash kind = %v, want flashWarning (default)", m.flashKind)
	}
	m.setSuccessFlash("ok")
	if m.flashKind != flashSuccess {
		t.Fatalf("setup invariant: setSuccessFlash kind = %v, want flashSuccess", m.flashKind)
	}
	m.setFlash("back to warning")
	if m.flashKind != flashWarning {
		t.Errorf("setFlash after setSuccessFlash kind = %v, want flashWarning (resets to default)", m.flashKind)
	}
}

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
			sessions := []Session{{Name: "alpha-row"}}
			m := NewModelWithSessions(sessions)
			m.termWidth = 80
			m.termHeight = 24
			m.colourless = true
			m.applyCanvasMode()
			tc.set(&m)

			band := m.renderActiveNoticeBand()
			stripped := ansi.Strip(band)
			if !strings.HasPrefix(strings.TrimLeft(stripped, " "), noticeBarGlyph) {
				t.Errorf("NO_COLOR %s flash must keep the %q bar: %q", tc.name, noticeBarGlyph, stripped)
			}
			if !strings.Contains(stripped, tc.glyph) {
				t.Errorf("NO_COLOR %s flash must keep the %q glyph: %q", tc.name, tc.glyph, stripped)
			}
			if band != stripped {
				t.Errorf("NO_COLOR %s flash must carry no SGR colour sequences; got raw %q", tc.name, band)
			}
		})
	}
}

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
		m.setSuccessFlash("timeout-me")
		updated, _ := m.Update(flashTickMsg{Gen: 1})
		if mm := updated.(Model); mm.flashText != "" {
			t.Errorf("matching tick must clear the success flash: flashText = %q", mm.flashText)
		}
	})
	t.Run("generation guard drops superseded tick", func(t *testing.T) {
		m := reskinFlashModel("alpha-row")
		m.setFlash("first")
		m.setSuccessFlash("second")
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

type reskinStubLister struct{ sessions []Session }

func (l reskinStubLister) ListSessions() ([]Session, error) { return l.sessions, nil }

func TestBuild_InitialFlashSeedsWarningFlash(t *testing.T) {
	lister := reskinStubLister{sessions: []Session{{Name: "dev"}}}
	const msg = "folio-Jiz4el closed externally — list updated"
	m := Build(Deps{
		Lister:  lister,
		Capture: CaptureSeeds{Flash: msg},
	})
	if m.flashText != msg {
		t.Errorf("Build InitialFlash flashText = %q, want %q", m.flashText, msg)
	}
	if m.flashKind != flashWarning {
		t.Errorf("Build InitialFlash flashKind = %v, want flashWarning", m.flashKind)
	}
	none := Build(Deps{Lister: lister})
	if none.flashText != "" {
		t.Errorf("empty InitialFlash must leave flashText empty, got %q", none.flashText)
	}
}

func TestBuild_InitialFlash_RendersWarningBand(t *testing.T) {
	const msg = "folio-Jiz4el closed externally — list updated"
	sessions := []Session{
		{Name: "fab-flowx-explore", Attached: true},
		{Name: "agentic-workflows-codify"},
	}
	m := Build(Deps{
		Lister:  reskinStubLister{sessions: sessions},
		Capture: CaptureSeeds{Flash: msg},
	})
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
	if tintSeq := tokenBgSeq(t, testDarkTheme(t).BgAttention); !strings.Contains(band, tintSeq) {
		t.Errorf("seeded band missing the bg.attention tint %q:\n%s", tintSeq, band)
	}
}

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
