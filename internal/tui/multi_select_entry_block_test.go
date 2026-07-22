package tui

// persistent-no-host-terminal-banner-2-2 — Proactive Multi-Select Entry Block +
// TUI-local blocked-entry flash helper (spec §3 / §5).
//
// These white-box (package tui) tests drive the `m` keypress through
// updateSessionList → handleMultiSelectToggle and assert the §3 proactive block:
// once detection has resolved unsupported (NULL or named), pressing `m` does NOT
// open multi-select — it sets a transient blocked-entry flash (§5 copy) and
// returns, instead of walking the user to a guaranteed dead-end at the N≥2 Enter.
// The retained reactive backstop (decideBurst) covers only the async in-flight
// window, so an in-flight `m` still enters (A1). WithInitialMultiSelect (the
// capture-harness construction seam) is deliberately NOT gated. A supported
// terminal is unaffected.
//
// No t.Parallel: consistent with the rest of the tui test surface (package-level
// mocks + shared canvas helpers).

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/spawn"
)

// TestMultiSelectBlockedFlashText pins the §5 blocked-entry copy at the
// pure-function level: NULL/remote vs named shape selected via id.IsNull()
// (mirroring unsupportedFlashText's branch), and — unlike the reactive no-op —
// NO ⚠ glyph in either string (the notice band prepends it) and NO
// `— nothing opened` suffix (a pre-emptive block attempts nothing).
func TestMultiSelectBlockedFlashText(t *testing.T) {
	tests := []struct {
		name string
		id   spawn.Identity
		want string
	}{
		{
			name: "NULL identity (remote/mosh or transient error)",
			id:   spawn.Identity{},
			want: "multi-select isn't available over a remote connection",
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

// TestMultiSelectEntryBlock_NamedUnsupported asserts the core §3 block on a
// resolved-unsupported NAMED terminal: `m` does NOT enter the mode, marks
// nothing, and sets the named block flash.
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

// TestMultiSelectEntryBlock_NullRemote asserts the §3 block on a resolved NULL /
// remote terminal (spawn.Identity{}): the mode stays closed and the remote block
// flash is set.
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

// TestMultiSelectEntryBlock_FlashClearsOnNextActionableKey asserts the block
// flash self-clears on the next actionable key (the authoritative §11 key-driven
// clear path at the top of updateSessionList).
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

// TestMultiSelectEntryBlock_NamedTwoRowCoRender asserts the §5 named two-row
// co-render after a blocked m: the persistent banner on the header row and the
// block flash on the notice-band row, BOTH carrying the ⚠ glyph, with the notice
// band NOT repeating the banner's `unsupported terminal` text, the identity
// string, or `see docs` (intent-only copy — the non-repetition constraint).
func TestMultiSelectEntryBlock_NamedTwoRowCoRender(t *testing.T) {
	m := unsupportedResolvedModel(t, appleTerminalIdentity())
	m = pressSession(t, m, pressM)

	if !m.unsupportedBannerActive() {
		t.Fatal("a blocked m on a named terminal must keep the persistent banner active")
	}

	// Row 1 — the section-header row carries the ⚠ unsupported-terminal banner.
	first := ansi.Strip(bannerFirstLine(m))
	if !strings.Contains(first, flashWarningGlyph) {
		t.Errorf("banner row must carry the %q glyph:\n%s", flashWarningGlyph, first)
	}
	if !strings.Contains(first, "unsupported terminal") {
		t.Errorf("banner row must carry %q:\n%s", "unsupported terminal", first)
	}

	// Row 2 — the notice band carries the ⚠ + the intent-only block flash.
	band := ansi.Strip(m.renderActiveNoticeBand())
	if !strings.Contains(band, flashWarningGlyph) {
		t.Errorf("notice band must carry the %q glyph:\n%s", flashWarningGlyph, band)
	}
	if !strings.Contains(band, "multi-select isn't available on this terminal") {
		t.Errorf("notice band must carry the block flash string:\n%s", band)
	}
	// Non-repetition constraint (§5): the block flash is intent-only — it must NOT
	// repeat what the co-rendered banner already supplies.
	for _, forbidden := range []string{"unsupported terminal", "Apple Terminal", "com.apple.Terminal", "see docs"} {
		if strings.Contains(band, forbidden) {
			t.Errorf("notice band must NOT repeat %q (the banner already supplies it):\n%s", forbidden, band)
		}
	}
}

// TestMultiSelectEntryBlock_RepeatedMReBlocks asserts a second m while the block
// flash shows clears then re-blocks + re-flashes (intentional §5 behaviour): the
// mode stays closed and the block flash is re-set.
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

// TestMultiSelectEntryBlock_InFlightStillEnters asserts the async in-flight
// window is NOT blocked (A1): while detection is dispatched but not resolved,
// DetectUnsupported() is false so pressing m still enters the mode — the retained
// reactive backstop (decideBurst) covers this path.
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

// TestMultiSelectEntryBlock_WithInitialMultiSelectNotGated asserts the
// construction seam is NOT gated: WithInitialMultiSelect combined with
// resolved-unsupported detection still opens the mode at construction (the
// capture harness sets multiSelectMode directly, not via a keypress).
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

// TestMultiSelectEntryBlock_SupportedEntersNoFlash guards the supported path: a
// resolved-supported (ghostty → native) terminal still enters the mode on m and
// sets no flash.
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
