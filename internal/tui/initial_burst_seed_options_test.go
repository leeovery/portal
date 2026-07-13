package tui

import (
	"testing"

	"github.com/leeovery/portal/internal/spawn"
)

// These white-box (package tui) tests pin the three capture-only seed seams that
// let the offline harness render the otherwise user/async-driven §6 picker-burst
// frames: the proactive unsupported-terminal banner (WithInitialDetection), the
// pre-flight abort banner + gone-row flags (WithInitialGoneFlagged), and the
// in-burst Opening band (WithInitialBurstOpening). Production never sets them —
// they mirror the §5 WithInitialMultiSelect / WithInitialCursor precedent.
//
// No t.Parallel() — the package-level mock convention makes parallelism unsafe.

// TestWithInitialDetection verifies the capture-only detection cache seed: the
// option marks detection resolved, caches the identity, AND resolves the
// resolution via the zero-config resolver so DetectUnsupported() is true for a
// non-NULL Apple Terminal identity (the proactive banner gate). A nil identity is
// a no-op.
func TestWithInitialDetection(t *testing.T) {
	t.Run("seeds a resolved unsupported cache for a non-NULL Apple Terminal", func(t *testing.T) {
		id := spawn.Identity{Name: "Apple Terminal", BundleID: "com.apple.Terminal"}
		m := New(fakeLister{}, WithInitialDetection(&id))

		if !m.DetectResolved() {
			t.Fatal("DetectResolved() = false, want true (the seed must mark detection resolved)")
		}
		if got := m.DetectedIdentity(); got != id {
			t.Errorf("DetectedIdentity() = %+v, want %+v", got, id)
		}
		if got := m.DetectedResolution(); got != spawn.ResolutionUnsupported {
			t.Errorf("DetectedResolution() = %q, want %q (resolved via ResolveAdapter)", got, spawn.ResolutionUnsupported)
		}
		if !m.DetectUnsupported() {
			t.Error("DetectUnsupported() = false, want true (the proactive banner must render for the non-NULL undriven terminal)")
		}
	})

	t.Run("nil identity is a no-op", func(t *testing.T) {
		m := New(fakeLister{}, WithInitialDetection(nil))
		if m.DetectResolved() {
			t.Error("DetectResolved() = true for a nil identity, want false (no-op)")
		}
		if m.DetectUnsupported() {
			t.Error("DetectUnsupported() = true for a nil identity, want false (no-op)")
		}
	})
}

// TestWithInitialGoneFlagged verifies the capture-only pre-flight abort seed: the
// option seeds the gone-row set and composes the abort banner text identically to
// handlePreflightAbort (spawn.QuoteJoin + spawn.GoneVerb), and refreshes the
// delegate so the red gone flag renders. An empty slice is a no-op.
func TestWithInitialGoneFlagged(t *testing.T) {
	t.Run("seeds the gone set + banner text over a multi-select model (single, is-verb)", func(t *testing.T) {
		m := New(fakeLister{},
			WithInitialMultiSelect([]string{"agentic-workflows-codify", "fab-flowx-explore", "designlab-web-r8suyU"}),
			WithInitialGoneFlagged([]string{"fab-flowx-explore"}),
		)

		if _, ok := m.goneFlagged["fab-flowx-explore"]; !ok {
			t.Error("goneFlagged missing fab-flowx-explore")
		}
		const want = "'fab-flowx-explore' is gone — nothing opened"
		if m.abortBannerText != want {
			t.Errorf("abortBannerText = %q, want %q", m.abortBannerText, want)
		}
		// The delegate must carry the gone flag so the row draws the red ⚠ / badge.
		if !isSelected(m.sessionDelegate().GoneFlagged, "fab-flowx-explore") {
			t.Error("sessionDelegate().GoneFlagged missing fab-flowx-explore")
		}
		// The mode stays active so survivors keep their ● and the multi-select footer shows.
		if !m.multiSelectMode {
			t.Error("multiSelectMode = false, want true (the abort seed keeps the mode)")
		}
	})

	t.Run("several gone names use the plural verb", func(t *testing.T) {
		m := New(fakeLister{}, WithInitialGoneFlagged([]string{"s2", "s4"}))
		const want = "'s2', 's4' are gone — nothing opened"
		if m.abortBannerText != want {
			t.Errorf("abortBannerText = %q, want %q", m.abortBannerText, want)
		}
	})

	t.Run("empty is a no-op", func(t *testing.T) {
		m := New(fakeLister{}, WithInitialGoneFlagged(nil))
		if m.goneFlagged != nil {
			t.Errorf("goneFlagged = %v for nil names, want nil (no-op)", m.goneFlagged)
		}
		if m.abortBannerText != "" {
			t.Errorf("abortBannerText = %q for nil names, want empty (no-op)", m.abortBannerText)
		}
	})
}

// TestWithInitialBurstOpening verifies the capture-only in-burst seed: a non-zero
// (done,total) marks a pending burst with the Opening n/N counters; the zero value
// is a no-op (not pending).
func TestWithInitialBurstOpening(t *testing.T) {
	t.Run("seeds a pending burst with the done/total counters", func(t *testing.T) {
		m := New(fakeLister{}, WithInitialBurstOpening(2, 3))
		if !m.BurstPending() {
			t.Fatal("BurstPending() = false, want true (the seed must mark the burst pending)")
		}
		if got := m.BurstDone(); got != 2 {
			t.Errorf("BurstDone() = %d, want 2", got)
		}
		if got := m.BurstTotal(); got != 3 {
			t.Errorf("BurstTotal() = %d, want 3", got)
		}
	})

	t.Run("zero value is a no-op (not pending)", func(t *testing.T) {
		m := New(fakeLister{}, WithInitialBurstOpening(0, 0))
		if m.BurstPending() {
			t.Error("BurstPending() = true for the zero value, want false (no-op)")
		}
	})
}
