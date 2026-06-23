package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/tmux"
)

// Chrome data is captured once at preview-open; cycle handlers must not re-enumerate.
//
// This file pins the invariant from § Multi-pane Rendering Shape > Chrome Floor
// (Chrome data source) and § Cross-cutting Seams > Externally-Killed Session
// During Preview: window/pane counts and names are captured at preview-open and
// reused mid-preview without re-querying tmux. ←/→ window + Tab pane cycle the captured
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

// driveCycleSequence drives →, →, ←, ←, Tab, Tab, Tab over the supplied model.
// Across a 2-window x 3-pane fixture this exercises forward window with wrap,
// backward window with wrap, and pane cycling within a window (1->2->0).
func driveCycleSequence(m previewModel) []previewModel {
	keys := []tea.KeyPressMsg{
		nextWindowKey,
		nextWindowKey,
		prevWindowKey,
		prevWindowKey,
		nextPaneKey,
		nextPaneKey,
		nextPaneKey,
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

	// After each keypress, the §9.1 chrome must reflect the open-time cached
	// shape: 2 windows total, 3 panes per window. The post-open re-enumerated
	// shape (1x1, raw index 9/42) must never surface — a re-enumeration would
	// collapse the totals to "/1" and leak the raw indices.
	for i, mm := range models {
		line := stripANSI(chromeLineForTest(mm))
		// Counters must reflect the open-time totals: 2 windows, 3 panes per
		// window. A re-enumeration to the 1x1 shape would surface "/1".
		if !strings.Contains(line, "/2 ·") {
			t.Errorf("step %d: header lost open-time window total (expected 'Window x/2'), got %q", i, line)
		}
		if !strings.Contains(line, "/3") {
			t.Errorf("step %d: header lost open-time pane total (expected 'Pane y/3'), got %q", i, line)
		}
		// The re-enumerated raw indices (9, 42) must never leak into the chrome.
		if strings.Contains(line, "/9") || strings.Contains(line, "42") {
			t.Errorf("step %d: header leaked post-open re-enumerated indices: %q", i, line)
		}
	}

	// Spot-check the per-step focused window ordinal across the sequence:
	//   start              : window 0 → Window 1/2
	//   →      -> window 1 → Window 2/2
	//   →      -> window 0 → Window 1/2 (wrap)
	//   ←      -> window 1 → Window 2/2 (wrap back)
	//   ←      -> window 0 → Window 1/2
	//   Tab     -> window 0, pane 1 → Window 1/2 · Pane 2/3
	//   Tab     -> window 0, pane 2 → Window 1/2 · Pane 3/3
	//   Tab     -> window 0, pane 0 → Window 1/2 · Pane 1/3 (wrap)
	wantFocusedWindow := []string{
		"Window 2/2", // after → #1
		"Window 1/2", // after → #2 (wrap)
		"Window 2/2", // after ← #1 (wrap back)
		"Window 1/2", // after ← #2
		"Window 1/2", // after Tab #1 (window unchanged)
		"Window 1/2", // after Tab #2
		"Window 1/2", // after Tab #3 (wrap)
	}
	for i, want := range wantFocusedWindow {
		if !strings.Contains(stripANSI(chromeLineForTest(models[i])), want) {
			t.Errorf("step %d: expected focused window ordinal %q in header, got %q", i, want, stripANSI(chromeLineForTest(models[i])))
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
		line := stripANSI(chromeLineForTest(mm))
		// A re-enumeration would collapse the open-time 2x3 totals to the 1x1
		// second-shape; the open-time totals (/2, /3) must survive every step.
		if !strings.Contains(line, "/2 ·") || !strings.Contains(line, "/3") {
			t.Errorf("step %d: header lost open-time totals (leaked re-enumerated shape?): %q", i, line)
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
