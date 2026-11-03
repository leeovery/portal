package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/backendtest"
)

func TestPreviewFooter_ByteIdenticalAcrossBodies(t *testing.T) {
	cases := []struct {
		name  string
		stage func(*backendtest.Fake)
	}{
		{name: "history", stage: func(*backendtest.Fake) {}},
		{name: "not answering", stage: func(f *backendtest.Fake) { f.StopAnswering("work") }},
		{name: "unavailable", stage: func(f *backendtest.Fake) { f.SetHistory("work", nil) }},
		{name: "failed read", stage: func(f *backendtest.Fake) { f.Fail(backendtest.OpHistory, errors.New("EIO synthetic")) }},
	}

	var first string
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			history := historyFake(t, "hello world", "work")
			tc.stage(history)
			m, ok := NewPreviewModel("work", newPreviewHistory(history), 80, 24)
			if !ok {
				t.Fatalf("expected ok=true on construction, got false")
			}
			got := stripANSI(footerLineForTest(m))

			if !strings.Contains(got, "⏎ attach  ␣ back") {
				t.Errorf("footer = %q; missing canonical attach/back segment", got)
			}

			if i == 0 {
				first = got
				return
			}
			if got != first {
				t.Errorf("footer under %q = %q; want byte-identical to first case %q", tc.name, got, first)
			}
		})
	}
}

func TestSessionsPageView_DoesNotContainPreviewChrome(t *testing.T) {
	sessions := []Session{
		{Name: "alpha"},
		{Name: "beta"},
	}
	m := NewModelWithSessions(sessions)

	got := stripANSI(m.View().Content)

	for _, forbidden := range []string{"◉ preview", "↑↓ session", "␣ back"} {
		if strings.Contains(got, forbidden) {
			t.Errorf("Sessions-page View() contains forbidden preview-chrome token %q; got %q", forbidden, got)
		}
	}
}
