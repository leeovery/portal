package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/leeovery/portal/internal/tmux"
)

// Chrome data is captured once at preview-open; cycle handlers must not re-enumerate.
//
// This file pins the invariant from § Multi-pane Rendering Shape > Chrome Floor
// (Chrome data source) and § Cross-cutting Seams > Externally-Killed Session
// During Preview: window/pane counts and names are captured at preview-open and
// reused mid-preview without re-querying tmux. ] / [ / Tab cycle the captured
// shape; if a future refactor introduces a re-enumeration call into any cycle
// handler, the assertions below fail with a clear "called N times, expected 1"
// message.

// chromeStabilityEnumerator is a recording TmuxEnumerator whose first call
// returns a known-good 2-window x 3-pane shape and whose subsequent calls
// return a deliberately distinct shape (different WindowName / PaneIndices /
// counts). If a cycle handler accidentally re-enumerates, the chromeLine()
// assertions detect it via the distinct WindowName values; if the model later
// switches to error-on-re-enumerate behaviour, secondErr can be flipped on.
type chromeStabilityEnumerator struct {
	first     []tmux.WindowGroup
	second    []tmux.WindowGroup
	secondErr error
	calls     []string
	callCount int
}

func (e *chromeStabilityEnumerator) ListWindowsAndPanesInSession(session string) ([]tmux.WindowGroup, error) {
	e.callCount++
	e.calls = append(e.calls, session)
	if e.callCount == 1 {
		return e.first, nil
	}
	if e.secondErr != nil {
		return nil, e.secondErr
	}
	return e.second, nil
}

// newChromeStabilityFixture returns the canonical 2-window x 3-pane shape
// used across the subtests below. Window names are chosen so a re-enumeration
// would observably change chromeLine() output.
func newChromeStabilityFixture() *chromeStabilityEnumerator {
	return &chromeStabilityEnumerator{
		first: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "first-window", PaneIndices: []int{0, 1, 2}},
			{WindowIndex: 1, WindowName: "second-window", PaneIndices: []int{0, 1, 2}},
		},
		// Distinct shape: different counts (1 window, 1 pane), different names,
		// different raw indices. Any leak of these into chromeLine() proves a
		// cycle handler re-enumerated.
		second: []tmux.WindowGroup{
			{WindowIndex: 9, WindowName: "REENUMERATED", PaneIndices: []int{42}},
		},
	}
}

// driveCycleSequence drives ], ], [, [, Tab, Tab, Tab over the supplied model.
// Across a 2-window x 3-pane fixture this exercises forward window with wrap,
// backward window with wrap, and pane cycling within a window (1->2->0).
func driveCycleSequence(m previewModel) []previewModel {
	keys := []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune{']'}},
		{Type: tea.KeyRunes, Runes: []rune{']'}},
		{Type: tea.KeyRunes, Runes: []rune{'['}},
		{Type: tea.KeyRunes, Runes: []rune{'['}},
		{Type: tea.KeyTab},
		{Type: tea.KeyTab},
		{Type: tea.KeyTab},
	}
	out := make([]previewModel, 0, len(keys))
	for _, k := range keys {
		m, _ = m.Update(k)
		out = append(out, m)
	}
	return out
}

func TestPreviewChromeStability_FullCycleSequenceProducesExactlyOneEnumerationCall(t *testing.T) {
	enum := newChromeStabilityFixture()
	reader := &recordingReader{bytes: []byte("content")}

	m, ok := NewPreviewModel("work", enum, reader, nil, 80, 24)
	if !ok {
		t.Fatalf("expected ok=true on construction, got false")
	}

	_ = driveCycleSequence(m)

	if enum.callCount != 1 {
		t.Errorf("expected ListWindowsAndPanesInSession called exactly 1 time (open-time only), got %d", enum.callCount)
	}
}

func TestPreviewChromeStability_ChromeLineAfterEachCycleReflectsOpenTimeCachedGroups(t *testing.T) {
	enum := newChromeStabilityFixture()
	reader := &recordingReader{bytes: []byte("content")}

	m, ok := NewPreviewModel("work", enum, reader, nil, 80, 24)
	if !ok {
		t.Fatalf("expected ok=true on construction, got false")
	}

	models := driveCycleSequence(m)

	// After each keypress, chromeLine() must reflect the open-time cached
	// shape: window names "first-window" / "second-window", 2 windows total,
	// 3 panes per window. The post-open shape ("REENUMERATED" / 1x1) must
	// never appear.
	for i, mm := range models {
		line := mm.chromeLine()
		if strings.Contains(line, "REENUMERATED") {
			t.Errorf("step %d: chromeLine() leaked post-open enumerator state: %q", i, line)
		}
		// Both window names from the open-time shape must remain reachable
		// across the cycle sequence — every step must show one of them.
		if !strings.Contains(line, "first-window") && !strings.Contains(line, "second-window") {
			t.Errorf("step %d: chromeLine() lost open-time window names, got %q", i, line)
		}
		// Counters must reflect the open-time totals: 2 windows, 3 panes per
		// window. A re-enumeration to the 1x1 shape would surface "of 1".
		if !strings.Contains(line, "of 2") {
			t.Errorf("step %d: chromeLine() lost open-time window total (expected 'of 2'), got %q", i, line)
		}
		if !strings.Contains(line, "of 3") {
			t.Errorf("step %d: chromeLine() lost open-time pane total (expected 'of 3'), got %q", i, line)
		}
	}

	// Spot-check the per-step focused window name across the sequence:
	//   start              : window 0 ("first-window")
	//   ]      -> window 1 ("second-window")
	//   ]      -> window 0 ("first-window") (wrap)
	//   [      -> window 1 ("second-window") (wrap back)
	//   [      -> window 0 ("first-window")
	//   Tab    -> window 0, pane 1 ("first-window")
	//   Tab    -> window 0, pane 2 ("first-window")
	//   Tab    -> window 0, pane 0 ("first-window") (wrap)
	wantFocusedName := []string{
		"second-window", // after ] #1
		"first-window",  // after ] #2 (wrap)
		"second-window", // after [ #1 (wrap back)
		"first-window",  // after [ #2
		"first-window",  // after Tab #1 (window unchanged)
		"first-window",  // after Tab #2
		"first-window",  // after Tab #3 (wrap)
	}
	for i, want := range wantFocusedName {
		if !strings.Contains(models[i].chromeLine(), want) {
			t.Errorf("step %d: expected focused window name %q in chromeLine(), got %q", i, want, models[i].chromeLine())
		}
	}
}

func TestPreviewChromeStability_ChromeLineNeverReflectsPostOpenEnumeratorStateChanges(t *testing.T) {
	// Same as above but with the second-call enumerator behaviour set to
	// error. If a cycle handler ever re-enumerates, the model's groups would
	// either flip to a degenerate shape (in the value-returning variant) or
	// the model would observe an error it has no handler for. This subtest
	// pins the stricter invariant: even with errSecond armed, no cycle
	// handler must reach the enumerator.
	enum := newChromeStabilityFixture()
	enum.secondErr = errors.New("session vanished")
	reader := &recordingReader{bytes: []byte("content")}

	m, ok := NewPreviewModel("work", enum, reader, nil, 80, 24)
	if !ok {
		t.Fatalf("expected ok=true on construction, got false")
	}

	models := driveCycleSequence(m)

	if enum.callCount != 1 {
		t.Errorf("expected ListWindowsAndPanesInSession called exactly 1 time even with armed second-call error, got %d", enum.callCount)
	}
	for i, mm := range models {
		line := mm.chromeLine()
		if strings.Contains(line, "REENUMERATED") {
			t.Errorf("step %d: chromeLine() leaked post-open enumerator state: %q", i, line)
		}
	}
}

func TestPreviewChromeStability_TailCallsPerCycleEqualOnePlusSeven(t *testing.T) {
	// Tail call budget across the sequence is exactly 1 (open-time read for
	// pane (0,0)) + 7 (each cycle keypress triggers one synchronous read for
	// the newly-focused pane). No double-reads, no skipped reads.
	enum := newChromeStabilityFixture()
	reader := &recordingReader{bytes: []byte("content")}

	m, ok := NewPreviewModel("work", enum, reader, nil, 80, 24)
	if !ok {
		t.Fatalf("expected ok=true on construction, got false")
	}

	_ = driveCycleSequence(m)

	const want = 1 + 7
	if len(reader.calls) != want {
		t.Errorf("expected %d Tail calls (1 open + 7 cycles), got %d", want, len(reader.calls))
	}
}
