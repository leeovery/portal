package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/spawn"
)

func TestMultiSelectBlockedFlashText(t *testing.T) {
	tests := []struct {
		name string
		id   spawn.Identity
		want string
	}{
		{
			name: "NULL identity (remote/mosh)",
			id:   spawn.Identity{},
			want: "multi-select isn't available over a remote connection",
		},
		{
			name: "unsure NULL identity (detection couldn't tell)",
			id:   spawn.Identity{Unsure: true},
			want: "multi-select isn't available: can't tell which terminal Portal is running in",
		},
		{
			name: "named undriven identity",
			id:   appleTerminalIdentity(),
			want: "multi-select isn't available on this terminal",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := multiSelectBlockedFlashText(tc.id); got != tc.want {
				t.Errorf("multiSelectBlockedFlashText() = %q, want %q", got, tc.want)
			}
			if strings.Contains(multiSelectBlockedFlashText(tc.id), flashWarningGlyph) {
				t.Errorf("the block flash text must NOT embed the ⚠ glyph (the warning band prepends it): %q", multiSelectBlockedFlashText(tc.id))
			}
			if strings.Contains(multiSelectBlockedFlashText(tc.id), "nothing opened") {
				t.Errorf("a pre-emptive block attempts nothing → NO `— nothing opened` suffix: %q", multiSelectBlockedFlashText(tc.id))
			}
		})
	}
}

func TestMultiSelectEntryBlock_NamedUnsupported(t *testing.T) {
	m := unsupportedResolvedModel(t, appleTerminalIdentity())
	if !m.DetectUnsupported() {
		t.Fatal("precondition: com.apple.Terminal must resolve unsupported")
	}

	m = pressSession(t, m, pressM)

	if m.MultiSelectActive() {
		t.Error("m on a resolved-unsupported terminal must NOT enter multi-select")
	}
	if got := m.SelectedSessionCount(); got != 0 {
		t.Errorf("blocked m must mark nothing; SelectedSessionCount = %d, want 0", got)
	}
	const want = "multi-select isn't available on this terminal"
	if m.flashText != want {
		t.Errorf("flashText = %q, want %q (named block flash)", m.flashText, want)
	}
}

func TestMultiSelectEntryBlock_NullRemote(t *testing.T) {
	m := unsupportedResolvedModel(t, spawn.Identity{})
	if !m.DetectUnsupported() {
		t.Fatal("precondition: a NULL identity must resolve unsupported")
	}

	m = pressSession(t, m, pressM)

	if m.MultiSelectActive() {
		t.Error("m on a resolved NULL/remote terminal must NOT enter multi-select")
	}
	if got := m.SelectedSessionCount(); got != 0 {
		t.Errorf("blocked m must mark nothing; SelectedSessionCount = %d, want 0", got)
	}
	const want = "multi-select isn't available over a remote connection"
	if m.flashText != want {
		t.Errorf("flashText = %q, want %q (NULL/remote block flash)", m.flashText, want)
	}
}

func TestMultiSelectEntryBlock_FlashClearsOnNextActionableKey(t *testing.T) {
	m := unsupportedResolvedModel(t, appleTerminalIdentity())
	m = pressSession(t, m, pressM)
	if m.flashText == "" {
		t.Fatal("precondition: blocked m must set the flash before the clear")
	}

	m = pressSession(t, m, tea.KeyPressMsg{Code: tea.KeyDown})

	if m.flashText != "" {
		t.Errorf("the block flash must clear on the next actionable key; flashText = %q, want empty", m.flashText)
	}
}

func TestMultiSelectEntryBlock_NamedTwoRowCoRender(t *testing.T) {
	m := unsupportedResolvedModel(t, appleTerminalIdentity())
	m = pressSession(t, m, pressM)

	if !m.unsupportedBannerActive() {
		t.Fatal("a blocked m on a named terminal must keep the persistent banner active")
	}

	first := ansi.Strip(bannerFirstLine(m))
	if !strings.Contains(first, flashWarningGlyph) {
		t.Errorf("banner row must carry the %q glyph:\n%s", flashWarningGlyph, first)
	}
	if !strings.Contains(first, "unsupported terminal") {
		t.Errorf("banner row must carry %q:\n%s", "unsupported terminal", first)
	}

	band := ansi.Strip(m.renderActiveNoticeBand())
	if !strings.Contains(band, flashWarningGlyph) {
		t.Errorf("notice band must carry the %q glyph:\n%s", flashWarningGlyph, band)
	}
	if !strings.Contains(band, "multi-select isn't available on this terminal") {
		t.Errorf("notice band must carry the block flash string:\n%s", band)
	}
	for _, forbidden := range []string{"unsupported terminal", "Apple Terminal", "com.apple.Terminal", "see docs"} {
		if strings.Contains(band, forbidden) {
			t.Errorf("notice band must NOT repeat %q (the banner already supplies it):\n%s", forbidden, band)
		}
	}
}

func TestMultiSelectEntryBlock_RepeatedMReBlocks(t *testing.T) {
	m := unsupportedResolvedModel(t, appleTerminalIdentity())
	m = pressSession(t, m, pressM)
	m = pressSession(t, m, pressM)

	if m.MultiSelectActive() {
		t.Error("a repeated m on an unsupported terminal must keep the mode closed")
	}
	const want = "multi-select isn't available on this terminal"
	if m.flashText != want {
		t.Errorf("flashText = %q, want %q (clear-then-reflash on the second press)", m.flashText, want)
	}
}

func TestMultiSelectEntryBlock_InFlightStillEnters(t *testing.T) {
	m, _ := dispatchWarmDetection(t, &fakeDetector{identity: appleTerminalIdentity()}, nativeResolve())
	if !m.DetectDispatched() {
		t.Fatal("precondition: detection must be dispatched (in flight)")
	}
	if m.DetectResolved() {
		t.Fatal("precondition: detection must still be in flight (not resolved)")
	}
	if m.DetectUnsupported() {
		t.Fatal("precondition: DetectUnsupported must be false while detection is in flight")
	}

	m = pressSession(t, m, pressM)

	if !m.MultiSelectActive() {
		t.Error("m during the async in-flight window must still enter multi-select (A1 backstop path)")
	}
}

func TestMultiSelectEntryBlock_WithInitialMultiSelectNotGated(t *testing.T) {
	id := appleTerminalIdentity()
	m := New(fakeLister{},
		WithProjectStore(stubProjectStore{}),
		WithInitialMultiSelect([]string{"alpha"}),
		WithInitialDetection(&id),
	)
	if !m.DetectUnsupported() {
		t.Fatal("precondition: the seeded Apple Terminal identity must resolve unsupported")
	}
	if !m.multiSelectMode {
		t.Error("WithInitialMultiSelect must open the mode regardless of detection state (construction seam not gated)")
	}
}

func TestMultiSelectEntryBlock_SupportedEntersNoFlash(t *testing.T) {
	m := unsupportedResolvedModel(t, ghosttyIdentity())
	if m.DetectUnsupported() {
		t.Fatal("precondition: ghostty must resolve native (supported)")
	}

	m = pressSession(t, m, pressM)

	if !m.MultiSelectActive() {
		t.Error("m on a supported terminal must enter multi-select")
	}
	if m.flashText != "" {
		t.Errorf("a supported entry must set NO flash; flashText = %q", m.flashText)
	}
}
