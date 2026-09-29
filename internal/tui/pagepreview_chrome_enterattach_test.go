package tui

import (
	"errors"
	"strings"
	"syscall"
	"testing"

	"github.com/leeovery/portal/internal/tmux"
)

func TestPreviewFooter_ByteIdenticalAcrossViewportStates(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}, {Index: 1}}},
		{WindowIndex: 1, WindowName: "logs", Panes: []tmux.WindowPane{{Index: 0}}},
	}

	cases := []struct {
		name   string
		reader ScrollbackReader
	}{
		{name: "real bytes", reader: &recordingReader{bytes: []byte("hello world\n")}},
		{name: "(nil, nil) placeholder", reader: &nilNilReader{}},
		{name: "OS read error", reader: &nilErrReader{err: syscall.EACCES}},
		{name: "OS read error EIO", reader: &nilErrReader{err: errors.New("EIO synthetic")}},
	}

	var first string
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			enum := &stubEnumerator{groups: groups}
			m, ok := NewPreviewModel("work", enum, tc.reader, nil, 80, 24)
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
	sessions := []tmux.Session{
		{Name: "alpha"},
		{Name: "beta"},
	}
	m := NewModelWithSessions(sessions)

	got := stripANSI(m.View().Content)

	for _, forbidden := range []string{"◉ preview", "←→ window", "⇥ pane", "␣ back"} {
		if strings.Contains(got, forbidden) {
			t.Errorf("Sessions-page View() contains forbidden preview-chrome token %q; got %q", forbidden, got)
		}
	}
}
