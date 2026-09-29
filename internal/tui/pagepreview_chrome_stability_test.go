package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/tmux"
)

// second is a deliberately distinct shape, so any post-open re-enumeration is
// observable in the render rather than silently identical.
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

func newChromeStabilityFixture() *chromeStabilityEnumerator {
	return &chromeStabilityEnumerator{
		first: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "first-window", Panes: []tmux.WindowPane{{Index: 0}, {Index: 1}, {Index: 2}}},
			{WindowIndex: 1, WindowName: "second-window", Panes: []tmux.WindowPane{{Index: 0}, {Index: 1}, {Index: 2}}},
		},
		second: []tmux.WindowGroup{
			{WindowIndex: 9, WindowName: "REENUMERATED", Panes: []tmux.WindowPane{{Index: 42}}},
		},
	}
}

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

	for i, mm := range models {
		line := stripANSI(chromeLineForTest(mm))
		if !strings.Contains(line, "/2 ·") {
			t.Errorf("step %d: header lost open-time window total (expected 'Window x/2'), got %q", i, line)
		}
		if !strings.Contains(line, "/3") {
			t.Errorf("step %d: header lost open-time pane total (expected 'Pane y/3'), got %q", i, line)
		}
		if strings.Contains(line, "/9") || strings.Contains(line, "42") {
			t.Errorf("step %d: header leaked post-open re-enumerated indices: %q", i, line)
		}
	}

	wantFocusedWindow := []string{
		"Window 2/2",
		"Window 1/2",
		"Window 2/2",
		"Window 1/2",
		"Window 1/2",
		"Window 1/2",
		"Window 1/2",
	}
	for i, want := range wantFocusedWindow {
		if !strings.Contains(stripANSI(chromeLineForTest(models[i])), want) {
			t.Errorf("step %d: expected focused window ordinal %q in header, got %q", i, want, stripANSI(chromeLineForTest(models[i])))
		}
	}
}

func TestPreviewChromeStability_ChromeLineNeverReflectsPostOpenEnumeratorStateChanges(t *testing.T) {
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
		if !strings.Contains(line, "/2 ·") || !strings.Contains(line, "/3") {
			t.Errorf("step %d: header lost open-time totals (leaked re-enumerated shape?): %q", i, line)
		}
	}
}

func TestPreviewChromeStability_TailCallsPerCycleEqualOnePlusSeven(t *testing.T) {
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
