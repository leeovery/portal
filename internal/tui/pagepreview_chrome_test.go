package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/tmux"
)

// stripANSI returns s with ANSI escape sequences removed so chrome assertions
// key off plain content regardless of any lipgloss styling applied by
// chromeLine().
func stripANSI(s string) string {
	return ansi.Strip(s)
}

func TestPreviewChromeLine_Renders1BasedOrdinalsForZeroIndexedGroups(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "alpha", PaneIndices: []int{0, 1}},
		{WindowIndex: 1, WindowName: "beta", PaneIndices: []int{0}},
	}
	m := newPreviewModelForHelpers("work", groups, 0, 0)

	got := stripANSI(chromeLineForTest(m))

	if !strings.Contains(got, "Window 1 of 2") {
		t.Errorf("chromeLine() = %q; want substring %q", got, "Window 1 of 2")
	}
	if !strings.Contains(got, "Pane 1 of 2") {
		t.Errorf("chromeLine() = %q; want substring %q", got, "Pane 1 of 2")
	}
}

func TestPreviewChromeLine_RendersOneToNCountersWhenWindowIndexValuesAreNonContiguous(t *testing.T) {
	// Non-contiguous tmux window_index values (0, 2, 5). Chrome must show
	// 1..N as the user cycles, never the raw window_index value.
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "first", PaneIndices: []int{0}},
		{WindowIndex: 2, WindowName: "second", PaneIndices: []int{0}},
		{WindowIndex: 5, WindowName: "third", PaneIndices: []int{0}},
	}

	cases := []struct {
		windowIdx int
		want      string
	}{
		{0, "Window 1 of 3"},
		{1, "Window 2 of 3"},
		{2, "Window 3 of 3"},
	}
	for _, tc := range cases {
		m := newPreviewModelForHelpers("work", groups, tc.windowIdx, 0)
		got := stripANSI(chromeLineForTest(m))
		if !strings.Contains(got, tc.want) {
			t.Errorf("windowIdx=%d: chromeLine() = %q; want substring %q", tc.windowIdx, got, tc.want)
		}
		// Defensively ensure raw window_index (5) never leaks as a counter.
		if strings.Contains(got, "Window 5 of 3") {
			t.Errorf("windowIdx=%d: chromeLine() = %q; raw window_index 5 leaked into chrome", tc.windowIdx, got)
		}
	}
}

func TestPreviewChromeLine_RendersOneToNCountersWhenPaneIndicesStartAt1(t *testing.T) {
	// pane-base-index 1 — PaneIndices=[1,2]. Chrome must show 1..N ordinals.
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", PaneIndices: []int{1, 2}},
	}

	cases := []struct {
		paneIdx int
		want    string
	}{
		{0, "Pane 1 of 2"},
		{1, "Pane 2 of 2"},
	}
	for _, tc := range cases {
		m := newPreviewModelForHelpers("work", groups, 0, tc.paneIdx)
		got := stripANSI(chromeLineForTest(m))
		if !strings.Contains(got, tc.want) {
			t.Errorf("paneIdx=%d: chromeLine() = %q; want substring %q", tc.paneIdx, got, tc.want)
		}
	}
}

func TestPreviewChromeLine_IncludesWindowNameVerbatimIncludingSpaces(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "editor window", PaneIndices: []int{0}},
	}
	m := newPreviewModelForHelpers("work", groups, 0, 0)

	got := stripANSI(chromeLineForTest(m))

	if !strings.Contains(got, "editor window") {
		t.Errorf("chromeLine() = %q; want substring %q (verbatim, including space)", got, "editor window")
	}
}

func TestPreviewChromeLine_IncludesBracketAndTabAndEscAsVisibleHints(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
	}
	m := newPreviewModelForHelpers("work", groups, 0, 0)

	got := stripANSI(chromeLineForTest(m))

	// Keymap glyphs per spec § Keymap glyphs: ] / [ stay ASCII; ⇥ (Tab),
	// ⏎ (Enter), ⎋ (Esc) replace the verbose word tokens. The chrome line
	// must surface every keypress glyph as a visible hint.
	for _, token := range []string{"]", "[", "⇥", "⏎", "⎋"} {
		if !strings.Contains(got, token) {
			t.Errorf("chromeLine() = %q; want visible hint token %q", got, token)
		}
	}
}

func TestPreviewChromeLine_IncludesEnterAttachTokenBetweenTabAndEsc(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
	}
	m := newPreviewModelForHelpers("work", groups, 0, 0)

	got := stripANSI(chromeLineForTest(m))

	// Spec § Verbose form: action labels follow each glyph, separated by
	// middle dots. The "⏎ attach" token sits between "⇥ next pane" and
	// "⎋ back" in left-to-right order.
	const wantSegment = "· ⇥ next pane · ⏎ attach · ⎋ back"
	if !strings.Contains(got, wantSegment) {
		t.Errorf("chromeLine() = %q; want substring %q", got, wantSegment)
	}

	tabIdx := strings.Index(got, "⇥ next pane")
	enterIdx := strings.Index(got, "⏎ attach")
	escIdx := strings.Index(got, "⎋ back")
	if tabIdx < 0 || enterIdx < 0 || escIdx < 0 {
		t.Fatalf("chromeLine() = %q; missing one of: '⇥ next pane', '⏎ attach', '⎋ back'", got)
	}
	if tabIdx >= enterIdx || enterIdx >= escIdx {
		t.Errorf("chromeLine() = %q; want order ⇥ next pane (%d) < ⏎ attach (%d) < ⎋ back (%d)", got, tabIdx, enterIdx, escIdx)
	}
}

func TestPreviewChromeLine_FullStringEqualityForCanonicalShape(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0, 1}},
		{WindowIndex: 1, WindowName: "logs", PaneIndices: []int{0}},
	}
	m := newPreviewModelForHelpers("work", groups, 0, 0)

	got := stripANSI(chromeLineForTest(m))

	// Substring equality against the unstyled chrome content. The full
	// chromeLineForTest output includes lipgloss border corners ('╭', '╮')
	// and filler dashes around the chrome content; the canonical chrome
	// content itself is the substring asserted here.
	const wantContent = "Window 1 of 2 · Pane 1 of 2 · win: main ] next win · [ prev win · ⇥ next pane · ⏎ attach · ⎋ back"
	if !strings.Contains(got, wantContent) {
		t.Errorf("chromeLine() = %q; want substring %q", got, wantContent)
	}
}

func TestPreviewChromeLine_DoesNotExposeRawTmuxIndices(t *testing.T) {
	// Construct a session where the raw indices are large and would be
	// visually distinct from the ordinals. Chrome must not surface them.
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "first", PaneIndices: []int{0}},
		{WindowIndex: 99, WindowName: "second", PaneIndices: []int{42, 43}},
	}
	m := newPreviewModelForHelpers("work", groups, 1, 1)

	got := stripANSI(chromeLineForTest(m))

	if strings.Contains(got, "99") {
		t.Errorf("chromeLine() = %q; raw WindowIndex 99 leaked into chrome", got)
	}
	if strings.Contains(got, "42") || strings.Contains(got, "43") {
		t.Errorf("chromeLine() = %q; raw PaneIndices (42/43) leaked into chrome", got)
	}
}

func TestPreviewChromeLine_ProducesNoIOWhenInvoked(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0, 1}},
		{WindowIndex: 1, WindowName: "other", PaneIndices: []int{0}},
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
		{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
	}
	m := newPreviewModelForHelpers("work", groups, 0, 0)

	got := strings.ToLower(stripANSI(chromeLineForTest(m)))

	for _, banned := range []string{"live", "now showing", "realtime", "current command"} {
		if strings.Contains(got, banned) {
			t.Errorf("chromeLine() = %q; must not contain liveness-implying token %q", got, banned)
		}
	}
}

func TestPreviewChromeLine_SingleWindowSinglePaneRendersOneOfOne(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
	}
	m := newPreviewModelForHelpers("work", groups, 0, 0)

	got := stripANSI(chromeLineForTest(m))

	if !strings.Contains(got, "Window 1 of 1") {
		t.Errorf("chromeLine() = %q; want substring %q for 1x1 case", got, "Window 1 of 1")
	}
	if !strings.Contains(got, "Pane 1 of 1") {
		t.Errorf("chromeLine() = %q; want substring %q for 1x1 case", got, "Pane 1 of 1")
	}
}

func TestPreviewChromeLine_WindowNameWithPipeRenderedVerbatim(t *testing.T) {
	// Pipe handling is owned at enumeration time; chrome renders the name
	// verbatim regardless of contents. Documented as an edge case in the
	// task so tests pin it.
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "weird|name with spaces", PaneIndices: []int{0}},
	}
	m := newPreviewModelForHelpers("work", groups, 0, 0)

	got := stripANSI(chromeLineForTest(m))

	if !strings.Contains(got, "weird|name with spaces") {
		t.Errorf("chromeLine() = %q; want substring %q (verbatim)", got, "weird|name with spaces")
	}
}

// Regression guard for analysis-cycle-1 task 5-3: the original chrome format
// embedded the literal "#W:" tmux-format-code as a user-facing label. Pinning
// its absence here so a future revision cannot silently reintroduce it.
func TestPreviewChromeLine_DoesNotEmbedTmuxFormatCodePrefix(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
		{WindowIndex: 1, WindowName: "logs", PaneIndices: []int{0, 1}},
	}
	for _, paneIdx := range []int{0} {
		m := newPreviewModelForHelpers("work", groups, 0, paneIdx)
		got := stripANSI(chromeLineForTest(m))
		if strings.Contains(got, "#W:") {
			t.Errorf("chromeLine() = %q; must not contain raw tmux format-code label %q", got, "#W:")
		}
	}
}
