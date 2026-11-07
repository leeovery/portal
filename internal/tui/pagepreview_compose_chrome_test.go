package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

const testSessionName = "nvim-editor"

func chl(t *testing.T, w int, name string) string {
	t.Helper()
	return stripANSI(composePreviewHeaderRow(w, name, testDarkTheme(t), false))
}

func TestComposePreviewHeaderRow_NoEmbeddedNewlines(t *testing.T) {
	for _, w := range []int{200, 80, 60, 40, 25, 15, 10, 4, 1, 0} {
		got := composePreviewHeaderRow(w, testSessionName, testDarkTheme(t), false)
		if n := strings.Count(got, "\n"); n != 0 {
			t.Errorf("composePreviewHeaderRow(width=%d) returned %d embedded newline(s); want 0; got=%q", w, n, got)
		}
	}
}

func TestComposePreviewHeaderRow_FitsWithinContentWidth(t *testing.T) {
	for _, w := range []int{200, 80, 60, 40, 25, 18, 13, 12, 11, 8, 4, 2, 1} {
		got := composePreviewHeaderRow(w, testSessionName, testDarkTheme(t), false)
		if width := lipgloss.Width(got); width > w {
			t.Errorf("content width %d: header width = %d, want <= %d; got=%q", w, width, w, stripANSI(got))
		}
	}
}

func TestComposePreviewHeaderRow_FullSessionWhenItFits(t *testing.T) {
	fullW := lipgloss.Width(previewMarker) + 1 + lipgloss.Width(testSessionName)
	for _, w := range []int{200, fullW} {
		if got := chl(t, w, testSessionName); got != previewMarker+" "+testSessionName {
			t.Errorf("width %d: header = %q, want the marker and the full session", w, got)
		}
	}
}

func TestComposePreviewHeaderRow_TruncatesTheSessionWhenItDoesNot(t *testing.T) {
	w := lipgloss.Width(previewMarker) + 1 + lipgloss.Width(testSessionName) - 1
	got := chl(t, w, testSessionName)
	if !strings.HasPrefix(got, previewMarker+" ") || !strings.HasSuffix(got, "…") {
		t.Errorf("width %d: header = %q, want the marker and the session truncated with an ellipsis", w, got)
	}
}

func TestComposePreviewHeaderRow_MarkerAloneWhenNoSessionCellFits(t *testing.T) {
	w := lipgloss.Width(previewMarker) + 1
	if got := chl(t, w, testSessionName); got != previewMarker {
		t.Errorf("width %d: header = %q, want the marker alone, without its gap", w, got)
	}
}

func TestComposePreviewHeaderRow_AlwaysCarriesMarker(t *testing.T) {
	for _, w := range []int{200, 60, 30, 18, 13, 11, 9} {
		got := chl(t, w, testSessionName)
		if !strings.Contains(got, previewMarker) {
			t.Errorf("width %d: header dropped the marker; got=%q", w, got)
		}
	}
}
