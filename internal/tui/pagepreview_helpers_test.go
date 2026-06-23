package tui

import (
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// newPreviewModelForHelpers constructs a previewModel directly (bypassing the
// initial enumeration / read dance in NewPreviewModel) so the helpers can be
// exercised against curated groups without depending on the constructor's I/O
// path. previewModel is unexported and lives in this package, so this is the
// canonical way to drive it from in-package tests.
//
// enumerator and reader are intentionally left nil: the helpers under test
// (currentGroup, currentRawIndices, currentPaneKey, degenerate, composeChromeLine)
// must be pure — any helper that accidentally calls m.enumerator or m.reader
// would nil-panic in these tests, locking the purity contract from the
// outside.
func newPreviewModelForHelpers(session string, groups []tmux.WindowGroup, windowIdx, paneIdx int) previewModel {
	return previewModel{
		session:   session,
		groups:    groups,
		windowIdx: windowIdx,
		paneIdx:   paneIdx,
	}
}

// chromeLineForTest is the test-only shim that composes the §9.1 header
// compartment content (styled) for a model. Tests asserting on header content
// call this against a model; the helper feeds the model's structural fields into
// composePreviewHeaderRow at a fixed content width of 200, well above the tier-1
// threshold so the full header (marker + session + slash counters) renders for
// any reasonable session-name length. The session name is the §9.1 header
// content (the window name is no longer surfaced). The footer's nav hints live in
// a separate compartment now (composePreviewFooterRow); tests asserting on the
// footer use footerLineForTest. Tests that need to probe other header cascade
// tiers call composePreviewHeaderRow directly with a tier-specific width.
func chromeLineForTest(m previewModel) string {
	return composePreviewHeaderRow(200, m.windowIdx, len(m.groups), m.paneIdx, len(m.currentGroup().PaneIndices), m.session, m.mode, m.colourless)
}

// footerLineForTest composes the §9.1 footer compartment (styled) at a fixed wide
// content width of 200 so the full labelled nav hints render. Peer of
// chromeLineForTest for footer-content assertions.
func footerLineForTest(m previewModel) string {
	return composePreviewFooterRow(200, m.mode, m.colourless)
}

// firstLine returns the first '\n'-terminated line of s, or all of s if no
// newline is present. Used by frame/routing/cascade tests to extract the
// top border row from a rendered View() before asserting on its width.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// headerLine returns the raw (styled) line of the §9.1 joined-panel View that
// carries the `◉ preview` header marker — line 1, directly under the top border.
// Returns "" if no header line is found. Used by chrome tests that assert on the
// header compartment's styled content (the View's first line is now the top
// border, not the header).
func headerLine(view string) string {
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(stripANSI(line), "◉ preview") {
			return line
		}
	}
	return ""
}

// footerLine returns the raw (styled) line of the §9.1 joined-panel View that
// carries the footer nav hints (the `←→ window` group, or its compact `←→`
// glyph when labels are dropped). Returns "" if no footer line is found.
func footerLine(view string) string {
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(stripANSI(line), "←→") {
			return line
		}
	}
	return ""
}

// chromeLineAtModelWidth composes the §9.1 header row at the model's actual
// content width (m.innerWidth() = m.width − previewFrameOverhead), matching how
// View() renders the header at runtime. Use this when asserting against View()
// output, where the rendered cascade tier depends on the model's actual content
// width — chromeLineForTest's fixed width of 200 would compose a different tier
// and the comparison would fail.
func chromeLineAtModelWidth(m previewModel) string {
	return composePreviewHeaderRow(m.innerWidth(), m.windowIdx, len(m.groups), m.paneIdx, len(m.currentGroup().PaneIndices), m.session, m.mode, m.colourless)
}

// newFramePreviewModel constructs a single-window single-pane previewModel
// with the requested window name and ScrollbackReader payload, at the
// canonical 80x24 dimensions used across these frame tests.
func newFramePreviewModel(t *testing.T, windowName string, payload []byte) previewModel {
	t.Helper()
	return newFramePreviewModelAt(t, windowName, payload, 80, 24)
}

// newFramePreviewModelAt is the explicit-dimensions variant of
// newFramePreviewModel. Used by tests that need a wider terminal to
// trigger cascade tier 1 (full verbose chrome with " · win: {name}"),
// or other non-default sizes. Session is fixed to "work" — the only
// value any in-package preview test currently needs.
func newFramePreviewModelAt(t *testing.T, windowName string, payload []byte, width, height int) previewModel {
	t.Helper()
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: windowName, PaneIndices: []int{0}},
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
		{WindowIndex: 0, WindowName: "alpha", PaneIndices: []int{0, 1}},
		{WindowIndex: 2, WindowName: "beta", PaneIndices: []int{0}},
		{WindowIndex: 5, WindowName: "gamma", PaneIndices: []int{3, 4, 5}},
	}
	m := newPreviewModelForHelpers("work", groups, 1, 0)

	got := m.currentGroup()

	if got.WindowIndex != 2 {
		t.Errorf("currentGroup().WindowIndex = %d; want 2", got.WindowIndex)
	}
	if got.WindowName != "beta" {
		t.Errorf("currentGroup().WindowName = %q; want %q", got.WindowName, "beta")
	}
	if len(got.PaneIndices) != 1 || got.PaneIndices[0] != 0 {
		t.Errorf("currentGroup().PaneIndices = %v; want [0]", got.PaneIndices)
	}
}

func TestPreviewModel_currentRawIndices_ReturnsRawWindowIndexAndPaneIndicesNotOrdinals(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "alpha", PaneIndices: []int{0, 1}},
		{WindowIndex: 2, WindowName: "beta", PaneIndices: []int{4, 7}},
	}
	m := newPreviewModelForHelpers("work", groups, 1, 1)

	rawWindow, rawPane := m.currentRawIndices()

	if rawWindow != 2 {
		t.Errorf("currentRawIndices() rawWindow = %d; want 2 (raw, not ordinal 1)", rawWindow)
	}
	if rawPane != 7 {
		t.Errorf("currentRawIndices() rawPane = %d; want 7 (raw, not ordinal 1)", rawPane)
	}
}

func TestPreviewModel_currentRawIndices_HandlesNonContiguousWindowIndexAndBaseIndex1(t *testing.T) {
	// Non-contiguous tmux window_index (0,2,5), with pane-base-index 1.
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "first", PaneIndices: []int{1, 2}},
		{WindowIndex: 2, WindowName: "second", PaneIndices: []int{1}},
		{WindowIndex: 5, WindowName: "third", PaneIndices: []int{1, 2, 3}},
	}

	// Last window (ordinal 2 → raw 5), first pane in that window (ordinal 0 → raw 1).
	m := newPreviewModelForHelpers("work", groups, 2, 0)
	rawWindow, rawPane := m.currentRawIndices()
	if rawWindow != 5 {
		t.Errorf("currentRawIndices() rawWindow = %d; want 5 (raw), not 2 (ordinal)", rawWindow)
	}
	if rawPane != 1 {
		t.Errorf("currentRawIndices() rawPane = %d; want 1 (raw under base-index 1)", rawPane)
	}

	// Middle window (ordinal 1 → raw 2), only pane (ordinal 0 → raw 1).
	m2 := newPreviewModelForHelpers("work", groups, 1, 0)
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
		{WindowIndex: 0, WindowName: "alpha", PaneIndices: []int{0, 1}},
		{WindowIndex: 2, WindowName: "beta", PaneIndices: []int{4, 7}},
		{WindowIndex: 5, WindowName: "gamma", PaneIndices: []int{1, 2, 3}},
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
			m := newPreviewModelForHelpers(tc.session, groups, tc.windowIdx, tc.paneIdx)
			rawW := groups[tc.windowIdx].WindowIndex
			rawP := groups[tc.windowIdx].PaneIndices[tc.paneIdx]
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
				{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
			},
			want: true,
		},
		{
			name: "1x2 single window two panes",
			groups: []tmux.WindowGroup{
				{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0, 1}},
			},
			want: false,
		},
		{
			name: "2x1 two windows one pane each",
			groups: []tmux.WindowGroup{
				{WindowIndex: 0, WindowName: "a", PaneIndices: []int{0}},
				{WindowIndex: 1, WindowName: "b", PaneIndices: []int{0}},
			},
			want: false,
		},
		{
			name: "2x2 two windows two panes each",
			groups: []tmux.WindowGroup{
				{WindowIndex: 0, WindowName: "a", PaneIndices: []int{0, 1}},
				{WindowIndex: 1, WindowName: "b", PaneIndices: []int{0, 1}},
			},
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newPreviewModelForHelpers("work", tc.groups, 0, 0)
			got := m.degenerate()
			if got != tc.want {
				t.Errorf("degenerate() = %v; want %v", got, tc.want)
			}
		})
	}
}
