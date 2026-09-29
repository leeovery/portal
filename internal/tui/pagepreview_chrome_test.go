package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/tmux"
)

func stripANSI(s string) string {
	return ansi.Strip(s)
}

func TestPreviewChromeLine_Renders1BasedOrdinalsForZeroIndexedGroups(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "alpha", Panes: []tmux.WindowPane{{Index: 0}, {Index: 1}}},
		{WindowIndex: 1, WindowName: "beta", Panes: []tmux.WindowPane{{Index: 0}}},
	}
	m := newPreviewModelForHelpers(t, "work", groups, 0, 0)

	got := stripANSI(chromeLineForTest(m))

	if !strings.Contains(got, "Window 1/2") {
		t.Errorf("chromeLine() = %q; want substring %q", got, "Window 1/2")
	}
	if !strings.Contains(got, "Pane 1/2") {
		t.Errorf("chromeLine() = %q; want substring %q", got, "Pane 1/2")
	}
}

func TestPreviewChromeLine_RendersOneToNCountersWhenWindowIndexValuesAreNonContiguous(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "first", Panes: []tmux.WindowPane{{Index: 0}}},
		{WindowIndex: 2, WindowName: "second", Panes: []tmux.WindowPane{{Index: 0}}},
		{WindowIndex: 5, WindowName: "third", Panes: []tmux.WindowPane{{Index: 0}}},
	}

	cases := []struct {
		windowIdx int
		want      string
	}{
		{0, "Window 1/3"},
		{1, "Window 2/3"},
		{2, "Window 3/3"},
	}
	for _, tc := range cases {
		m := newPreviewModelForHelpers(t, "work", groups, tc.windowIdx, 0)
		got := stripANSI(chromeLineForTest(m))
		if !strings.Contains(got, tc.want) {
			t.Errorf("windowIdx=%d: chromeLine() = %q; want substring %q", tc.windowIdx, got, tc.want)
		}
		if strings.Contains(got, "Window 5/3") {
			t.Errorf("windowIdx=%d: chromeLine() = %q; raw window_index 5 leaked into chrome", tc.windowIdx, got)
		}
	}
}

func TestPreviewChromeLine_RendersOneToNCountersWhenPaneIndicesStartAt1(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 1}, {Index: 2}}},
	}

	cases := []struct {
		paneIdx int
		want    string
	}{
		{0, "Pane 1/2"},
		{1, "Pane 2/2"},
	}
	for _, tc := range cases {
		m := newPreviewModelForHelpers(t, "work", groups, 0, tc.paneIdx)
		got := stripANSI(chromeLineForTest(m))
		if !strings.Contains(got, tc.want) {
			t.Errorf("paneIdx=%d: chromeLine() = %q; want substring %q", tc.paneIdx, got, tc.want)
		}
	}
}

func TestPreviewChromeLine_IncludesSessionNameVerbatimIncludingSpaces(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "editor window", Panes: []tmux.WindowPane{{Index: 0}}},
	}
	m := newPreviewModelForHelpers(t, "evvi webhooks and watchers", groups, 0, 0)

	got := stripANSI(chromeLineForTest(m))

	if !strings.Contains(got, "evvi webhooks and watchers") {
		t.Errorf("chromeLine() = %q; want session substring %q (verbatim, including spaces)", got, "evvi webhooks and watchers")
	}
}

func TestPreviewFooter_IncludesWindowPaneAttachBackAsVisibleHints(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}}},
	}
	m := newPreviewModelForHelpers(t, "work", groups, 0, 0)

	got := stripANSI(footerLineForTest(m))

	for _, token := range []string{"←", "→", "⇥", "⏎", "␣"} {
		if !strings.Contains(got, token) {
			t.Errorf("footer = %q; want visible hint token %q", got, token)
		}
	}
}

func TestPreviewFooter_OrdersWindowPaneAttachBackLeftToRight(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}}},
	}
	m := newPreviewModelForHelpers(t, "work", groups, 0, 0)

	got := stripANSI(footerLineForTest(m))

	const wantSegment = "←→ window  ⇥ pane  ⏎ attach  ␣ back"
	if !strings.Contains(got, wantSegment) {
		t.Errorf("footer = %q; want substring %q", got, wantSegment)
	}

	windowIdx := strings.Index(got, "←→ window")
	paneIdx := strings.Index(got, "⇥ pane")
	attachIdx := strings.Index(got, "⏎ attach")
	backIdx := strings.Index(got, "␣ back")
	if windowIdx < 0 || paneIdx < 0 || attachIdx < 0 || backIdx < 0 {
		t.Fatalf("footer = %q; missing one of the nav groups", got)
	}
	if windowIdx >= paneIdx || paneIdx >= attachIdx || attachIdx >= backIdx {
		t.Errorf("footer = %q; want order window (%d) < pane (%d) < attach (%d) < back (%d)", got, windowIdx, paneIdx, attachIdx, backIdx)
	}
}

func TestPreviewChromeLine_FullStringEqualityForCanonicalShape(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}, {Index: 1}}},
		{WindowIndex: 1, WindowName: "logs", Panes: []tmux.WindowPane{{Index: 0}}},
	}
	m := newPreviewModelForHelpers(t, "work", groups, 0, 0)

	header := stripANSI(chromeLineForTest(m))
	if want := "◉ preview work Window 1/2 · Pane 1/2"; !strings.Contains(header, want) {
		t.Errorf("header = %q; want substring %q", header, want)
	}
	footer := stripANSI(footerLineForTest(m))
	if want := "←→ window  ⇥ pane  ⏎ attach  ␣ back"; !strings.Contains(footer, want) {
		t.Errorf("footer = %q; want substring %q", footer, want)
	}
}

func TestPreviewChromeLine_DoesNotExposeRawTmuxIndices(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "first", Panes: []tmux.WindowPane{{Index: 0}}},
		{WindowIndex: 99, WindowName: "second", Panes: []tmux.WindowPane{{Index: 42}, {Index: 43}}},
	}
	m := newPreviewModelForHelpers(t, "work", groups, 1, 1)

	got := stripANSI(chromeLineForTest(m))

	if strings.Contains(got, "99") {
		t.Errorf("chromeLine() = %q; raw WindowIndex 99 leaked into chrome", got)
	}
	if strings.Contains(got, "42") || strings.Contains(got, "43") {
		t.Errorf("chromeLine() = %q; raw pane indices (42/43) leaked into chrome", got)
	}
}

func TestPreviewChromeLine_ProducesNoIOWhenInvoked(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}, {Index: 1}}},
		{WindowIndex: 1, WindowName: "other", Panes: []tmux.WindowPane{{Index: 0}}},
	}
	enum := &stubEnumerator{
		groups: groups,
	}
	reader := &recordingReader{bytes: []byte("content")}

	m := previewModel{
		session:    "work",
		enumerator: enum,
		reader:     reader,
		groups:     groups,
		windowIdx:  0,
		paneIdx:    0,
	}

	enumCallsBefore := enum.calls
	readerCallsBefore := len(reader.calls)

	_ = chromeLineForTest(m)

	if enum.calls != enumCallsBefore {
		t.Errorf("chromeLine() invoked enumerator: calls before=%d, after=%d", enumCallsBefore, enum.calls)
	}
	if len(reader.calls) != readerCallsBefore {
		t.Errorf("chromeLine() invoked reader: calls before=%d, after=%d", readerCallsBefore, len(reader.calls))
	}
}

func TestPreviewChromeLine_WordingDoesNotPromiseLiveness(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}}},
	}
	m := newPreviewModelForHelpers(t, "work", groups, 0, 0)

	got := strings.ToLower(stripANSI(chromeLineForTest(m)))

	for _, banned := range []string{"live", "now showing", "realtime", "current command"} {
		if strings.Contains(got, banned) {
			t.Errorf("chromeLine() = %q; must not contain liveness-implying token %q", got, banned)
		}
	}
}

func TestPreviewChromeLine_SingleWindowSinglePaneRendersOneOfOne(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}}},
	}
	m := newPreviewModelForHelpers(t, "work", groups, 0, 0)

	got := stripANSI(chromeLineForTest(m))

	if !strings.Contains(got, "Window 1/1") {
		t.Errorf("chromeLine() = %q; want substring %q for 1x1 case", got, "Window 1/1")
	}
	if !strings.Contains(got, "Pane 1/1") {
		t.Errorf("chromeLine() = %q; want substring %q for 1x1 case", got, "Pane 1/1")
	}
}

func TestPreviewChromeLine_SessionNameWithPipeRenderedVerbatim(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}}},
	}
	m := newPreviewModelForHelpers(t, "weird|name with spaces", groups, 0, 0)

	got := stripANSI(chromeLineForTest(m))

	if !strings.Contains(got, "weird|name with spaces") {
		t.Errorf("chromeLine() = %q; want substring %q (verbatim)", got, "weird|name with spaces")
	}
}

func TestPreviewChromeLine_DoesNotEmbedTmuxFormatCodePrefix(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}}},
		{WindowIndex: 1, WindowName: "logs", Panes: []tmux.WindowPane{{Index: 0}, {Index: 1}}},
	}
	for _, paneIdx := range []int{0} {
		m := newPreviewModelForHelpers(t, "work", groups, 0, paneIdx)
		got := stripANSI(chromeLineForTest(m))
		if strings.Contains(got, "#W:") {
			t.Errorf("chromeLine() = %q; must not contain raw tmux format-code label %q", got, "#W:")
		}
	}
}
