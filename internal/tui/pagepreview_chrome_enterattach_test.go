package tui

import (
	"errors"
	"strings"
	"syscall"
	"testing"

	"github.com/leeovery/portal/internal/tmux"
)

// TestPreviewFooter_ByteIdenticalAcrossViewportStates pins spec § Discoverability
// > Token wording is unconditional: the §9.1 footer must produce a byte-identical
// string regardless of whether the viewport rendered real bytes, the (nil, nil)
// "(no saved content)" placeholder, or the (nil, err) error string. The footer is
// a pure function of the descriptor and must not branch on viewport content state.
func TestPreviewFooter_ByteIdenticalAcrossViewportStates(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0, 1}},
		{WindowIndex: 1, WindowName: "logs", PaneIndices: []int{0}},
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

// TestSessionsPageView_DoesNotContainPreviewChrome pins spec § Discoverability
// > Sessions-page help bar: the preview chrome must not propagate to or
// duplicate on the Sessions page. The §9.1 preview footer renders glyph-keyed,
// space-separated nav-hint tokens (`←→ window  ⇥ pane  ⏎ attach  ␣ back`)
// plus the `◉ preview` marker that are preview-specific and must never appear
// on the Sessions page.
//
// Note: the §3.4 condensed Sessions footer independently advertises `enter
// attach` as one of its Core keys (text-keyed: "enter attach"), which is
// unrelated to and distinct from the preview footer's glyph-keyed `⏎ attach`
// token. This guard targets the preview chrome's own tokens, not the Sessions
// footer's legitimate `enter attach` entry.
func TestSessionsPageView_DoesNotContainPreviewChrome(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "alpha"},
		{Name: "beta"},
	}
	m := NewModelWithSessions(sessions)

	got := stripANSI(m.View().Content)

	// The preview chrome's preview-specific glyph-keyed tokens must never appear
	// on the Sessions page.
	for _, forbidden := range []string{"◉ preview", "←→ window", "⇥ pane", "⏎ attach", "␣ back"} {
		if strings.Contains(got, forbidden) {
			t.Errorf("Sessions-page View() contains forbidden preview-chrome token %q; got %q", forbidden, got)
		}
	}
}
