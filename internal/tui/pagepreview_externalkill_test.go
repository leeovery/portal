package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

func killedSessionFixture() []tmux.WindowGroup {
	return []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "first", Panes: []tmux.WindowPane{{Index: 0}, {Index: 1}}},
		{WindowIndex: 1, WindowName: "second", Panes: []tmux.WindowPane{{Index: 0}, {Index: 1}}},
	}
}

type progressivePlaceholderReader struct {
	bytesUntil       int
	mixedUntil       int
	placeholderKeys  map[string]bool
	calls            []string
	bytesByCallIndex map[int][]byte
}

func newProgressivePlaceholderReader(bytesUntil, mixedUntil int, placeholderKeys map[string]bool) *progressivePlaceholderReader {
	return &progressivePlaceholderReader{
		bytesUntil:       bytesUntil,
		mixedUntil:       mixedUntil,
		placeholderKeys:  placeholderKeys,
		bytesByCallIndex: map[int][]byte{},
	}
}

func (r *progressivePlaceholderReader) Tail(pane PaneScrollback) ([]byte, error) {
	paneKey := pane.PaneKey
	r.calls = append(r.calls, paneKey)
	idx := len(r.calls)
	switch {
	case idx <= r.bytesUntil:
		b := fmt.Appendf(nil, "bytes-%d-%s", idx, paneKey)
		r.bytesByCallIndex[idx] = b
		return b, nil
	case idx <= r.mixedUntil:
		if r.placeholderKeys[paneKey] {
			return nil, nil
		}
		b := fmt.Appendf(nil, "bytes-%d-%s", idx, paneKey)
		r.bytesByCallIndex[idx] = b
		return b, nil
	default:
		return nil, nil
	}
}

var killedSessionSequence = []tea.KeyPressMsg{
	nextPaneKey,
	nextWindowKey,
	nextPaneKey,
	nextPaneKey,
	prevWindowKey,
	nextWindowKey,
	nextPaneKey,
	nextPaneKey,
}

var killedSessionExpectedCoords = [][2]int{
	{0, 1},
	{1, 0},
	{1, 1},
	{1, 0},
	{0, 0},
	{1, 0},
	{1, 1},
	{1, 0},
}

func progressivePlaceholderFixture(t *testing.T) (*chromeStabilityEnumerator, *progressivePlaceholderReader, map[[2]int]string) {
	t.Helper()
	enum := &chromeStabilityEnumerator{
		first: killedSessionFixture(),
		second: []tmux.WindowGroup{
			{WindowIndex: 9, WindowName: "REENUMERATED", Panes: []tmux.WindowPane{{Index: 42}}},
		},
	}
	w0p0 := state.SanitizePaneKey("work", 0, 0)
	w0p1 := state.SanitizePaneKey("work", 0, 1)
	w1p0 := state.SanitizePaneKey("work", 1, 0)
	w1p1 := state.SanitizePaneKey("work", 1, 1)
	keysByCoord := map[[2]int]string{
		{0, 0}: w0p0,
		{0, 1}: w0p1,
		{1, 0}: w1p0,
		{1, 1}: w1p1,
	}
	reader := newProgressivePlaceholderReader(3, 6, map[string]bool{
		w1p1: true,
	})
	return enum, reader, keysByCoord
}

func driveKilledSessionSequence(m previewModel) []previewModel {
	out := make([]previewModel, 0, len(killedSessionSequence))
	for _, k := range killedSessionSequence {
		m, _ = m.Update(k)
		out = append(out, m)
	}
	return out
}

func TestPreviewExternalKill_ChromeStableWhenBinFilesDisappearMidPreview(t *testing.T) {
	enum, reader, _ := progressivePlaceholderFixture(t)

	m, ok := NewPreviewModel("work", enum, reader, nil, 80, 24)
	if !ok {
		t.Fatalf("expected ok=true on construction, got false")
	}

	models := driveKilledSessionSequence(m)

	initialChrome := stripANSI(chromeLineForTest(m))
	if !strings.Contains(initialChrome, "Window 1/2") || !strings.Contains(initialChrome, "Pane 1/2") {
		t.Errorf("initial chrome lost open-time totals: %q", initialChrome)
	}

	for i, mm := range models {
		chrome := stripANSI(chromeLineForTest(mm))
		if !strings.Contains(chrome, "/2 ") {
			t.Errorf("step %d: chromeLine() lost open-time totals (expected '/2'): %q", i, chrome)
		}
		if strings.Contains(chrome, "/9") || strings.Contains(chrome, "42") {
			t.Errorf("step %d: chromeLine() leaked post-open re-enumerated indices: %q", i, chrome)
		}
	}

	finalChrome := stripANSI(chromeLineForTest(models[len(models)-1]))
	if !strings.Contains(finalChrome, "Window 2/2") {
		t.Errorf("final chrome must show ordinal Window 2 (from windowIdx=1) with preserved '/2' total; got: %q", finalChrome)
	}
	if !strings.Contains(finalChrome, "Pane 1/2") {
		t.Errorf("final chrome must show ordinal Pane 1 (from paneIdx=0) with preserved '/2' total; got: %q", finalChrome)
	}
}

func TestPreviewExternalKill_PlaceholdersAppearProgressivelyAsContentVanishes(t *testing.T) {
	enum, reader, _ := progressivePlaceholderFixture(t)

	m, ok := NewPreviewModel("work", enum, reader, nil, 80, 24)
	if !ok {
		t.Fatalf("expected ok=true on construction, got false")
	}

	if got := stripTrailingBlanks(m.viewport.View()); got != string(reader.bytesByCallIndex[1]) {
		t.Errorf("initial viewport = %q; want %q (read #1 bytes)", got, string(reader.bytesByCallIndex[1]))
	}

	models := driveKilledSessionSequence(m)

	type stepExp struct {
		readIndex         int
		expectPlaceholder bool
	}
	expectations := []stepExp{
		{readIndex: 2, expectPlaceholder: false},
		{readIndex: 3, expectPlaceholder: false},
		{readIndex: 4, expectPlaceholder: true},
		{readIndex: 5, expectPlaceholder: false},
		{readIndex: 6, expectPlaceholder: false},
		{readIndex: 7, expectPlaceholder: true},
		{readIndex: 8, expectPlaceholder: true},
		{readIndex: 9, expectPlaceholder: true},
	}

	if len(expectations) != len(models) {
		t.Fatalf("expectations length %d != models length %d (test bug)", len(expectations), len(models))
	}

	for i, exp := range expectations {
		got := stripTrailingBlanks(models[i].viewport.View())
		if exp.expectPlaceholder {
			if got != previewPlaceholder {
				t.Errorf("step %d (read #%d): viewport = %q; want placeholder %q",
					i, exp.readIndex, got, previewPlaceholder)
			}
			continue
		}
		want := string(reader.bytesByCallIndex[exp.readIndex])
		if want == "" {
			t.Fatalf("step %d (read #%d): missing recorded bytes — fixture wiring bug", i, exp.readIndex)
		}
		if got != want {
			t.Errorf("step %d (read #%d): viewport = %q; want bytes %q",
				i, exp.readIndex, got, want)
		}
	}
}

func TestPreviewExternalKill_NoLiveReEnumerationMidPreviewWhenSessionIsKilled(t *testing.T) {
	enum, reader, _ := progressivePlaceholderFixture(t)

	m, ok := NewPreviewModel("work", enum, reader, nil, 80, 24)
	if !ok {
		t.Fatalf("expected ok=true on construction, got false")
	}

	_ = driveKilledSessionSequence(m)

	if enum.callCount != 1 {
		t.Errorf("expected ListWindowsAndPanesInSession called exactly 1 time (open-time only), got %d",
			enum.callCount)
	}
}

func TestPreviewExternalKill_CycleKeysContinueToTraverseAfterContentVanishes(t *testing.T) {
	enum, reader, _ := progressivePlaceholderFixture(t)

	m, ok := NewPreviewModel("work", enum, reader, nil, 80, 24)
	if !ok {
		t.Fatalf("expected ok=true on construction, got false")
	}

	visited := map[[2]int]int{}
	visited[[2]int{m.windowIdx, m.paneIdx}]++

	models := driveKilledSessionSequence(m)
	for i, mm := range models {
		visited[[2]int{mm.windowIdx, mm.paneIdx}]++
		want := killedSessionExpectedCoords[i]
		if mm.windowIdx != want[0] || mm.paneIdx != want[1] {
			t.Errorf("step %d: focus = (%d, %d); want (%d, %d)",
				i, mm.windowIdx, mm.paneIdx, want[0], want[1])
		}
	}

	for _, coord := range [][2]int{{0, 0}, {0, 1}, {1, 0}, {1, 1}} {
		if visited[coord] == 0 {
			t.Errorf("traversal skipped pane (windowIdx=%d, paneIdx=%d) — cycle keys must traverse all captured panes",
				coord[0], coord[1])
		}
	}

	const wantReads = 1 + 8
	if len(reader.calls) != wantReads {
		t.Errorf("expected %d Tail calls (1 open + 8 cycles), got %d (calls=%v)",
			wantReads, len(reader.calls), reader.calls)
	}
}

func TestPreviewExternalKill_NoPanicWhenAllPanesReturnNilNilMidPreview(t *testing.T) {
	enum, reader, _ := progressivePlaceholderFixture(t)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("preview model panicked under progressive placeholder degradation: %v", r)
		}
	}()

	m, ok := NewPreviewModel("work", enum, reader, nil, 80, 24)
	if !ok {
		t.Fatalf("expected ok=true on construction, got false")
	}

	models := driveKilledSessionSequence(m)

	for i, mm := range models {
		want := stripANSI(chromeLineAtModelWidth(mm))
		if !strings.Contains(stripANSI(mm.View()), want) {
			t.Errorf("step %d: View() did not contain header line — composition broken", i)
		}
	}
}

func TestPreviewExternalKill_EscDismissesCleanlyFromFullyDegradedPreview(t *testing.T) {
	enum, reader, _ := progressivePlaceholderFixture(t)

	m, ok := NewPreviewModel("work", enum, reader, nil, 80, 24)
	if !ok {
		t.Fatalf("expected ok=true on construction, got false")
	}

	models := driveKilledSessionSequence(m)
	final := models[len(models)-1]

	if got := stripTrailingBlanks(final.viewport.View()); got != previewPlaceholder {
		t.Fatalf("test setup invariant: expected fully-degraded viewport before Esc; got %q", got)
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Esc on fully-degraded preview panicked: %v", r)
		}
	}()

	_, cmd := final.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd == nil {
		t.Fatalf("expected non-nil tea.Cmd from Esc, got nil")
	}
	msg := cmd()
	if _, ok := msg.(previewDismissedMsg); !ok {
		t.Errorf("Esc cmd produced %T; want previewDismissedMsg", msg)
	}

	if enum.callCount != 1 {
		t.Errorf("Esc triggered re-enumeration: callCount = %d; want 1", enum.callCount)
	}
	if len(reader.calls) != 1+len(killedSessionSequence) {
		t.Errorf("Esc triggered an extra Tail call: got %d total, want %d",
			len(reader.calls), 1+len(killedSessionSequence))
	}
}
