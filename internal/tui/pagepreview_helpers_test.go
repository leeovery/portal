package tui

import (
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// th is seeded rather than left zero: the chrome helpers render from it, and a
// zero palette would make every colour assertion pass against no colour at all.
func newPreviewModelForHelpers(t *testing.T, session string, groups []tmux.WindowGroup, windowIdx, paneIdx int) previewModel {
	t.Helper()
	return previewModel{
		session:   session,
		groups:    groups,
		windowIdx: windowIdx,
		paneIdx:   paneIdx,
		th:        testDarkTheme(t),
	}
}

func chromeLineForTest(m previewModel) string {
	return composePreviewHeaderRow(200, m.windowIdx, len(m.groups), m.paneIdx, len(m.currentGroup().Panes), m.session, m.th, m.colourless)
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
		if strings.Contains(stripANSI(line), "←→") {
			return line
		}
	}
	return ""
}

func chromeLineAtModelWidth(m previewModel) string {
	return composePreviewHeaderRow(m.innerWidth(), m.windowIdx, len(m.groups), m.paneIdx, len(m.currentGroup().Panes), m.session, m.th, m.colourless)
}

func newFramePreviewModel(t *testing.T, windowName string, payload []byte) previewModel {
	t.Helper()
	return newFramePreviewModelAt(t, windowName, payload, 80, 24)
}

func newFramePreviewModelAt(t *testing.T, windowName string, payload []byte, width, height int) previewModel {
	t.Helper()
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: windowName, Panes: []tmux.WindowPane{{Index: 0}}},
		},
	}
	reader := &recordingReader{bytes: payload}
	m, ok := NewPreviewModel("work", enum, reader, nil, width, height)
	if !ok {
		t.Fatalf("setup: expected ok=true from NewPreviewModel, got false")
	}
	return m
}

func TestPreviewModel_currentGroup_ReturnsCachedWindowGroupAtWindowIdx(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "alpha", Panes: []tmux.WindowPane{{Index: 0}, {Index: 1}}},
		{WindowIndex: 2, WindowName: "beta", Panes: []tmux.WindowPane{{Index: 0}}},
		{WindowIndex: 5, WindowName: "gamma", Panes: []tmux.WindowPane{{Index: 3}, {Index: 4}, {Index: 5}}},
	}
	m := newPreviewModelForHelpers(t, "work", groups, 1, 0)

	got := m.currentGroup()

	if got.WindowIndex != 2 {
		t.Errorf("currentGroup().WindowIndex = %d; want 2", got.WindowIndex)
	}
	if got.WindowName != "beta" {
		t.Errorf("currentGroup().WindowName = %q; want %q", got.WindowName, "beta")
	}
	if len(got.Panes) != 1 || got.Panes[0].Index != 0 {
		t.Errorf("currentGroup().Panes = %v; want one pane at index 0", got.Panes)
	}
}

func TestPreviewModel_currentRawIndices_ReturnsRawWindowIndexAndPaneIndicesNotOrdinals(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "alpha", Panes: []tmux.WindowPane{{Index: 0}, {Index: 1}}},
		{WindowIndex: 2, WindowName: "beta", Panes: []tmux.WindowPane{{Index: 4}, {Index: 7}}},
	}
	m := newPreviewModelForHelpers(t, "work", groups, 1, 1)

	rawWindow, rawPane := m.currentRawIndices()

	if rawWindow != 2 {
		t.Errorf("currentRawIndices() rawWindow = %d; want 2 (raw, not ordinal 1)", rawWindow)
	}
	if rawPane != 7 {
		t.Errorf("currentRawIndices() rawPane = %d; want 7 (raw, not ordinal 1)", rawPane)
	}
}

func TestPreviewModel_currentRawIndices_HandlesNonContiguousWindowIndexAndBaseIndex1(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "first", Panes: []tmux.WindowPane{{Index: 1}, {Index: 2}}},
		{WindowIndex: 2, WindowName: "second", Panes: []tmux.WindowPane{{Index: 1}}},
		{WindowIndex: 5, WindowName: "third", Panes: []tmux.WindowPane{{Index: 1}, {Index: 2}, {Index: 3}}},
	}

	m := newPreviewModelForHelpers(t, "work", groups, 2, 0)
	rawWindow, rawPane := m.currentRawIndices()
	if rawWindow != 5 {
		t.Errorf("currentRawIndices() rawWindow = %d; want 5 (raw), not 2 (ordinal)", rawWindow)
	}
	if rawPane != 1 {
		t.Errorf("currentRawIndices() rawPane = %d; want 1 (raw under base-index 1)", rawPane)
	}

	m2 := newPreviewModelForHelpers(t, "work", groups, 1, 0)
	rawWindow2, rawPane2 := m2.currentRawIndices()
	if rawWindow2 != 2 {
		t.Errorf("currentRawIndices() rawWindow = %d; want 2 (raw)", rawWindow2)
	}
	if rawPane2 != 1 {
		t.Errorf("currentRawIndices() rawPane = %d; want 1 (raw)", rawPane2)
	}
}

func TestPreviewModel_currentPaneKey_MatchesSanitizePaneKeyOnRawIndicesForSameSession(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "alpha", Panes: []tmux.WindowPane{{Index: 0}, {Index: 1}}},
		{WindowIndex: 2, WindowName: "beta", Panes: []tmux.WindowPane{{Index: 4}, {Index: 7}}},
		{WindowIndex: 5, WindowName: "gamma", Panes: []tmux.WindowPane{{Index: 1}, {Index: 2}, {Index: 3}}},
	}

	cases := []struct {
		name      string
		session   string
		windowIdx int
		paneIdx   int
	}{
		{"first window first pane", "work", 0, 0},
		{"first window second pane", "work", 0, 1},
		{"second window second pane raw 7", "work", 1, 1},
		{"third window third pane raw 3", "work", 2, 2},
		{"unsafe session name", "foo/bar", 1, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newPreviewModelForHelpers(t, tc.session, groups, tc.windowIdx, tc.paneIdx)
			rawW := groups[tc.windowIdx].WindowIndex
			rawP := groups[tc.windowIdx].Panes[tc.paneIdx].Index
			want := state.SanitizePaneKey(tc.session, rawW, rawP)

			got := m.currentPaneKey()

			if got != want {
				t.Errorf("currentPaneKey() = %q; want %q (state.SanitizePaneKey(%q, %d, %d))", got, want, tc.session, rawW, rawP)
			}
		})
	}
}

func TestPreviewModel_degenerate_ReturnsTrueFor1x1AndFalseOtherwise(t *testing.T) {
	cases := []struct {
		name   string
		groups []tmux.WindowGroup
		want   bool
	}{
		{
			name: "1x1 single window single pane",
			groups: []tmux.WindowGroup{
				{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}}},
			},
			want: true,
		},
		{
			name: "1x2 single window two panes",
			groups: []tmux.WindowGroup{
				{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}, {Index: 1}}},
			},
			want: false,
		},
		{
			name: "2x1 two windows one pane each",
			groups: []tmux.WindowGroup{
				{WindowIndex: 0, WindowName: "a", Panes: []tmux.WindowPane{{Index: 0}}},
				{WindowIndex: 1, WindowName: "b", Panes: []tmux.WindowPane{{Index: 0}}},
			},
			want: false,
		},
		{
			name: "2x2 two windows two panes each",
			groups: []tmux.WindowGroup{
				{WindowIndex: 0, WindowName: "a", Panes: []tmux.WindowPane{{Index: 0}, {Index: 1}}},
				{WindowIndex: 1, WindowName: "b", Panes: []tmux.WindowPane{{Index: 0}, {Index: 1}}},
			},
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newPreviewModelForHelpers(t, "work", tc.groups, 0, 0)
			got := m.degenerate()
			if got != tc.want {
				t.Errorf("degenerate() = %v; want %v", got, tc.want)
			}
		})
	}
}
