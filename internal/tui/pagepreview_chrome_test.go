package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/backendtest"
)

func stripANSI(s string) string {
	return ansi.Strip(s)
}

// th is seeded rather than left zero: the chrome helpers render from it, and a
// zero palette would make every colour assertion pass against no colour at all.
func newPreviewModelForHelpers(t *testing.T, session string) previewModel {
	t.Helper()
	return previewModel{session: session, th: testDarkTheme(t)}
}

func TestPreviewChromeLine_IsTheMarkerThenTheSessionAlone(t *testing.T) {
	m := newPreviewModelForHelpers(t, "work")

	if got := stripANSI(chromeLineForTest(m)); got != "◉ preview work" {
		t.Errorf("header = %q, want %q: no window or pane counters", got, "◉ preview work")
	}
}

func TestPreviewChromeLine_RendersTheSessionNameVerbatim(t *testing.T) {
	for _, name := range []string{"orchard webhooks and watchers", "weird|name with spaces", "#W:literal"} {
		m := newPreviewModelForHelpers(t, name)

		if got := stripANSI(chromeLineForTest(m)); got != "◉ preview "+name {
			t.Errorf("header = %q, want the session %q verbatim", got, name)
		}
	}
}

func TestPreviewFooter_OrdersSessionAttachBackLeftToRight(t *testing.T) {
	m := newPreviewModelForHelpers(t, "work")

	if got, want := stripANSI(footerLineForTest(m)), "↑↓ session  ⏎ attach  ␣ back"; got != want {
		t.Errorf("footer = %q, want %q", got, want)
	}
}

func TestPreviewFooter_HintsNoWindowOrPaneCycling(t *testing.T) {
	m := newPreviewModelForHelpers(t, "work")

	got := stripANSI(footerLineForTest(m))
	for _, gone := range []string{"←", "→", "⇥", "window", "pane"} {
		if strings.Contains(got, gone) {
			t.Errorf("footer = %q carries %q; window and pane cycling are gone", got, gone)
		}
	}
}

func TestPreviewChromeLine_ProducesNoIOWhenInvoked(t *testing.T) {
	history := historyFake(t, "content", "work")
	m, ok := NewPreviewModel("work", newPreviewHistory(history), 80, 24)
	if !ok {
		t.Fatalf("setup: NewPreviewModel ok=false")
	}
	before := len(history.CallsTo(backendtest.OpHistory))

	_ = chromeLineForTest(m)
	_ = footerLineForTest(m)
	_ = m.View()

	if after := len(history.CallsTo(backendtest.OpHistory)); after != before {
		t.Errorf("drawing the preview read the history: calls %d → %d", before, after)
	}
}
