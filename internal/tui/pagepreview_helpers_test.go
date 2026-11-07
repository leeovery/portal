package tui

import (
	"strings"
	"testing"
)

func chromeLineForTest(m previewModel) string {
	return composePreviewHeaderRow(200, m.session, m.th, m.colourless)
}

func footerLineForTest(m previewModel) string {
	return composePreviewFooterRow(200, m.th, m.colourless)
}

func firstLine(s string) string {
	if before, _, ok := strings.Cut(s, "\n"); ok {
		return before
	}
	return s
}

func headerLine(view string) string {
	for line := range strings.SplitSeq(view, "\n") {
		if strings.Contains(stripANSI(line), "◉ preview") {
			return line
		}
	}
	return ""
}

func footerLine(view string) string {
	for line := range strings.SplitSeq(view, "\n") {
		if strings.Contains(stripANSI(line), "↑↓") {
			return line
		}
	}
	return ""
}

func newFramePreviewModel(t *testing.T, vt string) previewModel {
	t.Helper()
	return newTestPreview(t, vt, 80, 24)
}
