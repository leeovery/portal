package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/tui/theme"
)

// The §6.2 proactive unsupported/NULL terminal banner: a filter-line analogue that
// REPLACES the standard `Sessions ··· N` section header when detection has resolved
// the host terminal to an unsupported resolution (a NULL remote/mosh identity OR a
// non-NULL recognised-but-undriven identity like com.apple.Terminal). Named
// identity: amber `⚠ unsupported terminal` + a dim `— <name> · <bundleID>` identity
// (copy-paste key) on the left, a right-anchored blue `see docs` hint. NULL
// identity: the honest `⚠ no host-local terminal` line, no identity, no `see docs`.
// NO `▌` left-bar (it is a section-header analogue, not a §11 notice band). These
// tests pin the colour roles, the exact copy, the right-alignment, the NULL branch,
// the single-row height, the NO_COLOR carve-out, and the applySectionHeader
// precedence (below multi-select, above the standard header).
//
// No t.Parallel() — the package-level mock convention and shared canvas helpers
// make parallelism unsafe across this package's tests.

// unsupportedResolvedModel drives a warm-direct Sessions model through the 6.1
// detection path with the given host identity resolved through the production
// native→unsupported resolver, landing on PageSessions with detection cached. A
// com.apple.Terminal identity resolves unsupported (non-NULL undriven); a NULL
// identity resolves unsupported (remote/mosh); a ghostty identity resolves native.
func unsupportedResolvedModel(t *testing.T, identity spawn.Identity) Model {
	t.Helper()
	return warmResolvedModel(t, &fakeDetector{identity: identity}, nativeResolve())
}

// TestUnsupportedHeader_NamedIdentityAmberDimSeeDocs asserts the named-identity
// render: the `⚠ unsupported terminal` cluster in accent.orange, the dim
// `— <name> · <bundleID>` identity in text.detail (the exact em-dash / middot
// separators from the delivered frame), and the right-anchored `see docs` hint in
// accent.blue — on both the dark and light canvas.
func TestUnsupportedHeader_NamedIdentityAmberDimSeeDocs(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode theme.Mode
	}{
		{"dark", theme.Dark},
		{"light", theme.Light},
	} {
		t.Run(tc.name, func(t *testing.T) {
			header := renderUnsupportedHeader("Apple Terminal", "com.apple.Terminal", sectionHeaderWidth, tc.mode, false)

			// The exact visible left cluster + separators from the delivered frame.
			const wantVisible = "⚠ unsupported terminal — Apple Terminal · com.apple.Terminal"
			if !strings.Contains(ansi.Strip(header), wantVisible) {
				t.Errorf("banner missing the exact copy %q:\n%s", wantVisible, ansi.Strip(header))
			}
			if !strings.Contains(ansi.Strip(header), "see docs") {
				t.Errorf("banner missing the %q hint:\n%s", "see docs", ansi.Strip(header))
			}
			// It is a section-header analogue, not a §11 notice band: NO `▌` left-bar.
			if strings.Contains(ansi.Strip(header), noticeBarGlyph) {
				t.Errorf("banner must not carry the %q notice-bar glyph:\n%s", noticeBarGlyph, ansi.Strip(header))
			}

			// The `⚠ unsupported terminal` label run is accent.orange (amber).
			amberRun := headerStyle(theme.MV.AccentOrange, tc.mode, false).Render(flashWarningGlyph + " " + "unsupported terminal")
			if !strings.Contains(header, amberRun) {
				t.Errorf("banner missing the accent.orange label run:\n%s", header)
			}
			// The identity run is dim text.detail.
			dimRun := headerStyle(theme.MV.TextDetail, tc.mode, false).Render(" — Apple Terminal · com.apple.Terminal")
			if !strings.Contains(header, dimRun) {
				t.Errorf("banner missing the text.detail identity run:\n%s", header)
			}
			// The `see docs` hint is accent.blue.
			blueRun := headerStyle(theme.MV.AccentBlue, tc.mode, false).Render("see docs")
			if !strings.Contains(header, blueRun) {
				t.Errorf("banner missing the accent.blue %q run:\n%s", "see docs", header)
			}
		})
	}
}

// TestUnsupportedHeader_NullIdentityNoHostLocal asserts the NULL (remote/mosh)
// branch — detected via bundleID == "" — renders the honest
// `⚠ no host-local terminal` line in accent.orange, with NO identity string and NO
// `see docs` hint (matching CLI task 2-7's IsNull copy branch).
func TestUnsupportedHeader_NullIdentityNoHostLocal(t *testing.T) {
	header := renderUnsupportedHeader("", "", sectionHeaderWidth, theme.Dark, false)

	if !strings.Contains(ansi.Strip(header), "⚠ no host-local terminal") {
		t.Errorf("NULL banner must read %q:\n%s", "⚠ no host-local terminal", ansi.Strip(header))
	}
	if strings.Contains(header, "see docs") {
		t.Errorf("NULL banner must NOT show the %q hint:\n%s", "see docs", ansi.Strip(header))
	}
	if strings.Contains(header, "unsupported terminal") {
		t.Errorf("NULL banner must NOT use the named %q copy:\n%s", "unsupported terminal", ansi.Strip(header))
	}
	// The honest label is accent.orange.
	amberRun := headerStyle(theme.MV.AccentOrange, theme.Dark, false).Render(flashWarningGlyph + " " + "no host-local terminal")
	if !strings.Contains(header, amberRun) {
		t.Errorf("NULL banner missing the accent.orange label run:\n%s", header)
	}
}

// TestUnsupportedHeader_RightAlignedSeeDocs asserts the `see docs` hint is
// right-aligned (the left cluster and the hint are separated by a flex spacer to
// the content width) and the single rendered row is exactly the content width.
func TestUnsupportedHeader_RightAlignedSeeDocs(t *testing.T) {
	header := renderUnsupportedHeader("Apple Terminal", "com.apple.Terminal", sectionHeaderWidth, theme.Dark, false)

	labelIdx := strings.Index(header, "unsupported terminal")
	hintIdx := strings.LastIndex(header, "see docs")
	if labelIdx < 0 || hintIdx < 0 {
		t.Fatalf("banner missing a cluster: labelIdx=%d hintIdx=%d\n%s", labelIdx, hintIdx, header)
	}
	if hintIdx < labelIdx {
		t.Errorf("hint (idx %d) appears before the label (idx %d); must be right-aligned", hintIdx, labelIdx)
	}
	if got := lipgloss.Width(header); got != sectionHeaderWidth {
		t.Errorf("banner width = %d, want exactly %d (flex spacer to content width)", got, sectionHeaderWidth)
	}
}

// TestUnsupportedHeader_ExactlyOneRow asserts the banner is exactly one rendered
// row for both the named and NULL branches — it REPLACES the section-header row and
// must not perturb the one-row-per-delegate pagination budget (§3.5).
func TestUnsupportedHeader_ExactlyOneRow(t *testing.T) {
	for _, tc := range []struct {
		name     string
		bundleID string
	}{
		{"named", "com.apple.Terminal"},
		{"null", ""},
	} {
		header := renderUnsupportedHeader("Apple Terminal", tc.bundleID, sectionHeaderWidth, theme.Dark, false)
		if got := lipgloss.Height(header); got != 1 {
			t.Errorf("%s banner height = %d, want exactly 1 row:\n%s", tc.name, got, header)
		}
	}
}

// TestUnsupportedHeader_NarrowDegradeDropsHint asserts the §2.7 narrow degrade:
// below the width at which the left cluster + a spacer + the hint fit, the right
// `see docs` hint drops and the row never overflows — matching the standard
// section header's degrade exactly (both route through the shared right-anchor
// core). The identity-bearing left cluster is never truncated (like the standard
// header's own left cluster), so the degrade width must still admit the cluster.
func TestUnsupportedHeader_NarrowDegradeDropsHint(t *testing.T) {
	// Wide: hint present.
	wide := renderUnsupportedHeader("Apple Terminal", "com.apple.Terminal", sectionHeaderWidth, theme.Dark, false)
	if !strings.Contains(wide, "see docs") {
		t.Fatalf("wide banner missing the hint:\n%s", wide)
	}
	clusterWidth := lipgloss.Width("⚠ unsupported terminal — Apple Terminal · com.apple.Terminal")

	// Narrow: admits the left cluster but not the cluster + a spacer + `see docs`.
	narrow := clusterWidth + 4
	header := renderUnsupportedHeader("Apple Terminal", "com.apple.Terminal", narrow, theme.Dark, false)
	if strings.Contains(header, "see docs") {
		t.Errorf("narrow banner at width %d still shows the %q hint (degrade failed):\n%s", narrow, "see docs", header)
	}
	// The identity cluster survives the degrade.
	if !strings.Contains(ansi.Strip(header), "unsupported terminal — Apple Terminal · com.apple.Terminal") {
		t.Errorf("narrow banner dropped the identity cluster:\n%s", ansi.Strip(header))
	}
	for i, line := range strings.Split(header, "\n") {
		if lw := lipgloss.Width(line); lw > narrow {
			t.Errorf("narrow banner line %d width = %d (overflow, want <= %d)", i, lw, narrow)
		}
	}
}

// TestUnsupportedHeader_ColourlessGlyphBacked asserts the NO_COLOR carve-out
// (§2.5): a colourless banner carries no canvas background SGR and no foreground
// hue — the `⚠`, the label, the identity, and `see docs` survive on the terminal's
// native fg/bg (glyph-backed, never colour-only).
func TestUnsupportedHeader_ColourlessGlyphBacked(t *testing.T) {
	header := renderUnsupportedHeader("Apple Terminal", "com.apple.Terminal", sectionHeaderWidth, theme.Dark, true)

	stripped := ansi.Strip(header)
	for _, want := range []string{"⚠", "unsupported terminal", "Apple Terminal", "com.apple.Terminal", "see docs"} {
		if !strings.Contains(stripped, want) {
			t.Errorf("colourless banner dropped %q:\n%s", want, stripped)
		}
	}
	if seq := canvasSeq(t, theme.Dark); strings.Contains(header, seq) {
		t.Errorf("colourless banner still paints the canvas background sequence %q", seq)
	}
	for _, tok := range []theme.Token{theme.MV.AccentOrange, theme.MV.TextDetail, theme.MV.AccentBlue} {
		if seq := tokenFgSeq(t, tok, theme.Dark); strings.Contains(header, seq) {
			t.Errorf("colourless banner still emits a foreground role sequence %q", seq)
		}
	}
}

// TestUnsupportedHeader_PaintsCanvasNoEdgeBleed asserts the banner cells carry the
// owned canvas background (leaf .Background(canvas)) so the right-aligned spacer
// gap is canvas-painted, not a terminal-bg island.
func TestUnsupportedHeader_PaintsCanvasNoEdgeBleed(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		header := renderUnsupportedHeader("Apple Terminal", "com.apple.Terminal", sectionHeaderWidth, mode, false)
		if seq := canvasSeq(t, mode); !strings.Contains(header, seq) {
			t.Errorf("banner does not paint the canvas background sequence %q:\n%s", seq, header)
		}
	}
}

// TestApplySectionHeader_UnsupportedShowsBanner asserts that on a resolved-
// unsupported non-NULL undriven terminal (com.apple.Terminal), outside multi-select
// mode, the section-header row swaps to the unsupported banner naming the identity
// with a blue `see docs`, and the standard `Sessions` header is NOT shown. Driven
// through the 6.1 detection path.
func TestApplySectionHeader_UnsupportedShowsBanner(t *testing.T) {
	m := unsupportedResolvedModel(t, appleTerminalIdentity())
	if !m.DetectUnsupported() {
		t.Fatalf("precondition: com.apple.Terminal must resolve unsupported")
	}

	first := ansi.Strip(bannerFirstLine(m))
	for _, want := range []string{"unsupported terminal", "Apple Terminal", "com.apple.Terminal", "see docs"} {
		if !strings.Contains(first, want) {
			t.Errorf("unsupported section-header row missing %q:\n%s", want, first)
		}
	}
	if strings.Contains(first, "Sessions") {
		t.Errorf("unsupported section-header row must NOT show the standard %q header:\n%s", "Sessions", first)
	}
	// The label cluster is painted amber (accent.orange).
	if seq := tokenFgSeq(t, theme.MV.AccentOrange, m.canvasMode); !strings.Contains(bannerFirstLine(m), seq) {
		t.Errorf("unsupported banner missing the accent.orange fg sequence %q:\n%s", seq, bannerFirstLine(m))
	}
}

// TestApplySectionHeader_UnsupportedNullShowsStandardHeader asserts a resolved NULL
// (remote/mosh) identity now renders the standard `Sessions ··· N` header at the
// section-header row. The IsNull() discriminator on unsupportedBannerActive() makes
// the banner named-only, so a NULL client keeps its session count and grouping
// indicator — no banner, no `no host-local terminal` / `unsupported terminal` /
// `see docs`.
func TestApplySectionHeader_UnsupportedNullShowsStandardHeader(t *testing.T) {
	m := unsupportedResolvedModel(t, spawn.Identity{}) // NULL (remote/mosh)
	if !m.DetectUnsupported() {
		t.Fatalf("precondition: a NULL identity must resolve unsupported")
	}

	first := ansi.Strip(bannerFirstLine(m))
	if !strings.Contains(first, "Sessions") {
		t.Errorf("NULL section-header row must show the standard %q header:\n%s", "Sessions", first)
	}
	for _, absent := range []string{"no host-local terminal", "unsupported terminal", "see docs"} {
		if strings.Contains(first, absent) {
			t.Errorf("NULL section-header row must NOT show %q (banner is named-only):\n%s", absent, first)
		}
	}
}

// TestApplySectionHeader_InFlightShowsStandardHeader asserts that while detection
// is in-flight (detectDispatched && !detectResolved) the standard `Sessions ··· N`
// header shows — no banner.
func TestApplySectionHeader_InFlightShowsStandardHeader(t *testing.T) {
	// dispatchWarmDetection lands PageSessions and dispatches detection but does NOT
	// drain the async command, so detection is in-flight.
	m, _ := dispatchWarmDetection(t, &fakeDetector{identity: appleTerminalIdentity()}, nativeResolve())
	if !m.DetectDispatched() || m.DetectResolved() {
		t.Fatalf("precondition: in-flight must be dispatched && !resolved; dispatched=%v resolved=%v", m.DetectDispatched(), m.DetectResolved())
	}

	first := ansi.Strip(bannerFirstLine(m))
	if !strings.Contains(first, "Sessions") {
		t.Errorf("in-flight section-header row must show the standard %q header:\n%s", "Sessions", first)
	}
	if strings.Contains(first, "unsupported terminal") || strings.Contains(first, "no host-local terminal") {
		t.Errorf("in-flight section-header row must NOT show the unsupported banner:\n%s", first)
	}
}

// TestApplySectionHeader_SupportedShowsStandardHeader asserts a resolved SUPPORTED
// identity (ghostty → native) shows the standard header — being non-NULL is NOT
// sufficient to hide the banner; being SUPPORTED is.
func TestApplySectionHeader_SupportedShowsStandardHeader(t *testing.T) {
	m := unsupportedResolvedModel(t, ghosttyIdentity())
	if m.DetectUnsupported() {
		t.Fatalf("precondition: ghostty must resolve native (supported)")
	}

	first := ansi.Strip(bannerFirstLine(m))
	if !strings.Contains(first, "Sessions") {
		t.Errorf("supported section-header row must show the standard %q header:\n%s", "Sessions", first)
	}
	if strings.Contains(first, "unsupported terminal") {
		t.Errorf("supported section-header row must NOT show the unsupported banner:\n%s", first)
	}
}

// TestApplySectionHeader_MultiSelectStepsUnsupportedAside asserts entering multi-
// select mode replaces the unsupported banner with the violet `N selected` banner
// — the multi-select banner owns the section-header row and the unsupported banner
// steps aside.
func TestApplySectionHeader_MultiSelectStepsUnsupportedAside(t *testing.T) {
	m := unsupportedResolvedModel(t, appleTerminalIdentity())
	if !m.DetectUnsupported() {
		t.Fatalf("precondition: com.apple.Terminal must resolve unsupported")
	}
	m.multiSelectMode = true

	first := ansi.Strip(bannerFirstLine(m))
	if !strings.Contains(first, "selected") {
		t.Errorf("multi-select must own the section-header row with the %q banner:\n%s", "N selected", first)
	}
	if strings.Contains(first, "unsupported terminal") {
		t.Errorf("multi-select mode must step the unsupported banner aside:\n%s", first)
	}
	// The helper agrees: the unsupported banner is not active while in the mode.
	if m.unsupportedBannerActive() {
		t.Errorf("unsupportedBannerActive() must be false in multi-select mode")
	}
}

// TestActiveNoticeBand_SuppressesSignpostWhenUnsupported asserts the By-Tag
// "No tags yet" signpost does NOT own the notice slot while the unsupported banner
// owns the section-header row (the banner outranks the signpost per the §11
// precedence). Mirrors the multi-select suppression sibling by setting the
// resolved-unsupported detection cache directly on the By-Tag signpost model.
func TestActiveNoticeBand_SuppressesSignpostWhenUnsupported(t *testing.T) {
	m := signpostModel(t)
	if _, _, ok := m.activeNoticeBand(); !ok {
		t.Fatalf("precondition: the signpost must own the slot before the unsupported banner activates")
	}

	m.detectIdentity = appleTerminalIdentity()
	m.detectResolution = spawn.ResolutionUnsupported
	m.detectResolved = true
	if !m.unsupportedBannerActive() {
		t.Fatalf("precondition: the unsupported banner must be active")
	}

	if _, _, ok := m.activeNoticeBand(); ok {
		t.Errorf("the unsupported banner must suppress the By-Tag signpost notice band")
	}
}

// TestActiveNoticeBand_NullReturnsSignpost asserts a resolved-unsupported NULL
// (remote/mosh) identity does NOT suppress the By-Tag "No tags yet" signpost. With
// the banner gate now named-only (unsupportedBannerActive() false for NULL), no
// banner competes for the section-header row, so the signpost owns the notice slot
// again. Mirror of TestActiveNoticeBand_SuppressesSignpostWhenUnsupported (which
// uses a named identity and stays valid).
func TestActiveNoticeBand_NullReturnsSignpost(t *testing.T) {
	m := signpostModel(t)
	if _, _, ok := m.activeNoticeBand(); !ok {
		t.Fatalf("precondition: the signpost must own the slot before detection resolves")
	}

	m.detectIdentity = spawn.Identity{} // NULL (remote/mosh)
	m.detectResolution = spawn.ResolutionUnsupported
	m.detectResolved = true
	if m.unsupportedBannerActive() {
		t.Fatalf("unsupportedBannerActive() must be false for a resolved-unsupported NULL identity")
	}

	if _, _, ok := m.activeNoticeBand(); !ok {
		t.Errorf("a NULL client with no tags must show the By-Tag signpost (no banner competing for the slot)")
	}
}

// TestActiveNoticeBand_FlashOutranksUnsupported asserts a transient flash still
// owns the notice slot even when the unsupported banner is active — the flash arm
// stays FIRST, so a flash outranks both the (section-header) unsupported banner and
// the suppressed signpost.
func TestActiveNoticeBand_FlashOutranksUnsupported(t *testing.T) {
	m := signpostModel(t)
	m.detectIdentity = appleTerminalIdentity()
	m.detectResolution = spawn.ResolutionUnsupported
	m.detectResolved = true
	const flash = "session \"alpha\" no longer exists"
	m.setFlash(flash)

	role, message, ok := m.activeNoticeBand()
	if !ok {
		t.Fatalf("a transient flash must own the notice slot even when the unsupported banner is active")
	}
	if message != flash {
		t.Errorf("flash message = %q, want %q", message, flash)
	}
	if role != bandWarning {
		t.Errorf("default flash role = %v, want bandWarning", role)
	}
}
