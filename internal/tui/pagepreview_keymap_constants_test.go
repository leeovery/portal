package tui

import (
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/tmux"
)

func TestPreviewFooterCanonicalByteContent(t *testing.T) {
	const want = "←→ window  ⇥ pane  ⏎ attach  ␣ back"
	got := stripANSI(composePreviewFooterRow(200, testDarkTheme(t), false))
	if got != want {
		t.Errorf("preview footer = %q, want %q", got, want)
	}
}

func TestPreviewFooterNoMiddots(t *testing.T) {
	got := stripANSI(composePreviewFooterRow(200, testDarkTheme(t), false))
	if strings.ContainsRune(got, '·') {
		t.Errorf("preview footer contains a middot U+00B7; want space-separated only: %q", got)
	}
}

func TestPreviewFooterCompactGlyphsOnly(t *testing.T) {
	got := stripANSI(composePreviewFooterRow(20, testDarkTheme(t), false))
	const want = "←→  ⇥  ⏎  ␣"
	if got != want {
		t.Errorf("compact preview footer = %q, want %q", got, want)
	}
	for _, label := range []string{"window", "pane", "attach", "back"} {
		if strings.Contains(got, label) {
			t.Errorf("compact preview footer must drop labels; found %q in %q", label, got)
		}
	}
}

func TestPreviewMarkerExactByteContent(t *testing.T) {
	want := "◉ preview"
	if previewMarker != want {
		t.Errorf("previewMarker = %q, want %q", previewMarker, want)
	}
}

func TestPreviewBorderResolvesTheModeAccentToken(t *testing.T) {
	th := testDarkTheme(t)
	m := newPeekPreviewModel(t, "work", []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "editor", Panes: []tmux.WindowPane{{Index: 0}}},
	}, []byte("hello\n"), 80, 24)
	m.th = th
	frame := m.View()

	if seq := tokenFgSeq(t, th.AccentMode); !strings.Contains(frame, seq) {
		t.Errorf("preview frame does not draw its border from accent.mode (SGR %q)", seq)
	}
	if th.AccentMode.Value == "#7B95BD" {
		t.Errorf("accent.mode = %q; the retired explicit border hex must not survive", th.AccentMode.Value)
	}
}
