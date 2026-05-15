package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// Externally-killed-session in-preview stability (Phase 4 task 4-6):
//
// Pin the contract from § Cross-cutting Seams > Externally-Killed Session
// During Preview and § Acceptance Criteria > Edge cases:
//
//   - Chrome stays anchored to the at-open structural enumeration regardless
//     of how `.bin` files vanish underneath.
//   - TmuxEnumerator.ListWindowsAndPanesInSession is called exactly once
//     (open-time only) — no live re-enumeration mid-preview.
//   - As .bin files are progressively cleaned by the daemon, ScrollbackReader
//     returns (nil, nil) for affected panes; the viewport renders the
//     placeholder for those panes and bytes for the unaffected ones.
//   - Cycle keys (Tab, ], [) continue to traverse every captured structural
//     entry — no skip, no panic — even when every pane has degraded to
//     placeholder.
//   - Esc still emits previewDismissedMsg cleanly from a fully-degraded
//     preview (the broader refresh path is owned by 4-5).
//
// Production code is unchanged by this file — these tests pin existing
// dispatcher behaviour as a regression boundary.

// killedSessionFixture returns the 2-window x 2-pane structural shape used
// across this file. All four (windowIdx, paneIdx) coordinates are exercised
// by the cycle sequence below.
func killedSessionFixture() []tmux.WindowGroup {
	return []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "first", PaneIndices: []int{0, 1}},
		{WindowIndex: 1, WindowName: "second", PaneIndices: []int{0, 1}},
	}
}

// progressivePlaceholderReader simulates the daemon progressively cleaning
// .bin files while preview is open. It runs three observable phases driven
// by the per-call counter:
//
//   - Phase "all bytes":         calls 1..bytesUntil      → (bytes, nil) for every paneKey.
//   - Phase "mixed":              calls bytesUntil+1..mixedUntil → (nil, nil) for any paneKey
//     in placeholderKeys; (bytes, nil) for the rest.
//   - Phase "all placeholder":    calls > mixedUntil       → (nil, nil) for every paneKey.
//
// The bytes value is unique per call so tests can pin the exact viewport
// content at any step. The phases approximate the daemon's clean-up sweep:
// some panes degrade first, then all of them.
type progressivePlaceholderReader struct {
	bytesUntil       int             // last call index that is "all bytes"
	mixedUntil       int             // last call index that is "mixed"
	placeholderKeys  map[string]bool // panes that go to (nil,nil) during mixed phase
	calls            []string        // every paneKey passed to Tail, in order
	bytesByCallIndex map[int][]byte  // bytes returned at call index N (1-based)
}

func newProgressivePlaceholderReader(bytesUntil, mixedUntil int, placeholderKeys map[string]bool) *progressivePlaceholderReader {
	return &progressivePlaceholderReader{
		bytesUntil:       bytesUntil,
		mixedUntil:       mixedUntil,
		placeholderKeys:  placeholderKeys,
		bytesByCallIndex: map[int][]byte{},
	}
}

func (r *progressivePlaceholderReader) Tail(paneKey string) ([]byte, error) {
	r.calls = append(r.calls, paneKey)
	idx := len(r.calls) // 1-based
	switch {
	case idx <= r.bytesUntil:
		b := []byte(fmt.Sprintf("bytes-%d-%s", idx, paneKey))
		r.bytesByCallIndex[idx] = b
		return b, nil
	case idx <= r.mixedUntil:
		if r.placeholderKeys[paneKey] {
			return nil, nil
		}
		b := []byte(fmt.Sprintf("bytes-%d-%s", idx, paneKey))
		r.bytesByCallIndex[idx] = b
		return b, nil
	default:
		return nil, nil
	}
}

// killedSessionSequence is the cycle sequence pinned by the task body:
// Tab, ], Tab, Tab, [, ], Tab, Tab. Combined with the 2x2 fixture this is
// the resulting (windowIdx, paneIdx) trajectory:
//
//	open : (0, 0)
//	Tab  : (0, 1)
//	]    : (1, 0)
//	Tab  : (1, 1)
//	Tab  : (1, 0)   (intra-window wrap)
//	[    : (0, 0)   (window wrap back)
//	]    : (1, 0)
//	Tab  : (1, 1)
//	Tab  : (1, 0)   (intra-window wrap)
//
// 8 keypresses + 1 open-time read = 9 Tail calls.
var killedSessionSequence = []tea.KeyMsg{
	{Type: tea.KeyTab},                       // (0,1)
	{Type: tea.KeyRunes, Runes: []rune{']'}}, // (1,0)
	{Type: tea.KeyTab},                       // (1,1)
	{Type: tea.KeyTab},                       // (1,0)
	{Type: tea.KeyRunes, Runes: []rune{'['}}, // (0,0)
	{Type: tea.KeyRunes, Runes: []rune{']'}}, // (1,0)
	{Type: tea.KeyTab},                       // (1,1)
	{Type: tea.KeyTab},                       // (1,0)
}

// killedSessionExpectedCoords is the post-step (windowIdx, paneIdx)
// trajectory matching killedSessionSequence. Index i is the focus state
// AFTER applying killedSessionSequence[i].
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

// progressivePlaceholderFixture wires a 2x2 enumerator + a stateful reader
// with three phases tuned so that:
//   - reads 1..3 are all bytes (open + first Tab + first ]),
//   - reads 4..6 are mixed (w1p1 returns (nil,nil); others bytes),
//   - reads 7..9 are all placeholder.
//
// This pins observable bytes → mixed → all-placeholder progression across
// the 9-read sequence, matching the task body's progression contract.
func progressivePlaceholderFixture(t *testing.T) (*chromeStabilityEnumerator, *progressivePlaceholderReader, map[[2]int]string) {
	t.Helper()
	enum := &chromeStabilityEnumerator{
		first: killedSessionFixture(),
		// Distinct shape so any leak via re-enumeration is observable.
		second: []tmux.WindowGroup{
			{WindowIndex: 9, WindowName: "REENUMERATED", PaneIndices: []int{42}},
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
	// During the mixed phase only w1p1 is degraded.
	reader := newProgressivePlaceholderReader(3, 6, map[string]bool{
		w1p1: true,
	})
	return enum, reader, keysByCoord
}

// driveKilledSessionSequence runs killedSessionSequence over the supplied
// model and returns the post-step model snapshots. The returned slice is
// length 8 — one entry per keypress, matching killedSessionExpectedCoords.
func driveKilledSessionSequence(m previewModel) []previewModel {
	out := make([]previewModel, 0, len(killedSessionSequence))
	for _, k := range killedSessionSequence {
		m, _ = m.Update(k)
		out = append(out, m)
	}
	return out
}

func TestPreviewExternalKill_ChromeStableWhenBinFilesDisappearMidPreview(t *testing.T) {
	// Chrome counts and window names must stay anchored to the open-time
	// enumeration across every step of the cycle sequence — even as the
	// reader degrades from bytes → mixed → all placeholder. The only
	// post-open enumerator shape uses WindowName "REENUMERATED" / 1x1, so
	// any leak via mid-preview re-enumeration would be observable.
	enum, reader, _ := progressivePlaceholderFixture(t)

	m, ok := NewPreviewModel("work", enum, reader, nil, 80, 24)
	if !ok {
		t.Fatalf("expected ok=true on construction, got false")
	}

	models := driveKilledSessionSequence(m)

	// Initial chrome — open-time enumeration shape.
	initialChrome := stripANSI(m.chromeLine())
	if !strings.Contains(initialChrome, "Window 1 of 2") || !strings.Contains(initialChrome, "Pane 1 of 2") {
		t.Errorf("initial chrome lost open-time totals: %q", initialChrome)
	}

	// Per-step chrome — never reflect the post-open shape.
	for i, mm := range models {
		chrome := stripANSI(mm.chromeLine())
		if strings.Contains(chrome, "REENUMERATED") {
			t.Errorf("step %d: chromeLine() leaked post-open enumerator state: %q", i, chrome)
		}
		if !strings.Contains(chrome, "of 2") {
			// "Window M of 2" and "Pane X of 2" are both present.
			t.Errorf("step %d: chromeLine() lost open-time totals (expected 'of 2'): %q", i, chrome)
		}
		// Every focused window must be one of the open-time names.
		if !strings.Contains(chrome, "first") && !strings.Contains(chrome, "second") {
			t.Errorf("step %d: chromeLine() lost open-time window names: %q", i, chrome)
		}
	}

	// Final chrome shape preserved — same totals as initial. The cycle sequence
	// lands us on (windowIdx=1, paneIdx=0), so chrome should read
	// "Window 2 of 2" / "Pane 1 of 2" with the "of N" totals unchanged from
	// the at-open enumeration.
	finalChrome := stripANSI(models[len(models)-1].chromeLine())
	if !strings.Contains(finalChrome, "Window 2 of 2") {
		t.Errorf("final chrome must show ordinal Window 2 (from windowIdx=1) with preserved 'of 2' total; got: %q", finalChrome)
	}
	if !strings.Contains(finalChrome, "Pane 1 of 2") {
		t.Errorf("final chrome must show ordinal Pane 1 (from paneIdx=0) with preserved 'of 2' total; got: %q", finalChrome)
	}
}

func TestPreviewExternalKill_PlaceholdersAppearProgressivelyAsContentVanishes(t *testing.T) {
	// Per-step viewport content must match the reader's progression:
	//   reads 1..3 → bytes (every step in the all-bytes phase shows bytes),
	//   reads 4..6 → mixed (w1p1 returns placeholder; other panes show bytes),
	//   reads 7..9 → placeholder (every step shows placeholder).
	//
	// Index mapping (reads are 1-based; killedSessionSequence is 0-based):
	//   read 1: open-time          (not a step in models[]);   coord (0,0)
	//   read 2: step 0 (Tab)       coord (0,1)
	//   read 3: step 1 (])         coord (1,0)
	//   read 4: step 2 (Tab)       coord (1,1)   ← FIRST mixed-phase read; w1p1 is degraded → placeholder
	//   read 5: step 3 (Tab)       coord (1,0)
	//   read 6: step 4 ([)         coord (0,0)
	//   read 7: step 5 (])         coord (1,0)   ← FIRST all-placeholder read
	//   read 8: step 6 (Tab)       coord (1,1)
	//   read 9: step 7 (Tab)       coord (1,0)
	enum, reader, _ := progressivePlaceholderFixture(t)

	m, ok := NewPreviewModel("work", enum, reader, nil, 80, 24)
	if !ok {
		t.Fatalf("expected ok=true on construction, got false")
	}

	// Initial open is read 1 — phase "all bytes". Viewport renders bytes.
	if got := stripTrailingBlanks(m.viewport.View()); got != string(reader.bytesByCallIndex[1]) {
		t.Errorf("initial viewport = %q; want %q (read #1 bytes)", got, string(reader.bytesByCallIndex[1]))
	}

	models := driveKilledSessionSequence(m)

	// Per-step expectations: (readIndex, expectPlaceholder).
	type stepExp struct {
		readIndex         int
		expectPlaceholder bool
	}
	expectations := []stepExp{
		{readIndex: 2, expectPlaceholder: false}, // step 0: Tab → (0,1), all-bytes
		{readIndex: 3, expectPlaceholder: false}, // step 1: ]   → (1,0), all-bytes
		{readIndex: 4, expectPlaceholder: true},  // step 2: Tab → (1,1), mixed (degraded key)
		{readIndex: 5, expectPlaceholder: false}, // step 3: Tab → (1,0), mixed (other key, bytes)
		{readIndex: 6, expectPlaceholder: false}, // step 4: [   → (0,0), mixed (other key, bytes)
		{readIndex: 7, expectPlaceholder: true},  // step 5: ]   → (1,0), all-placeholder
		{readIndex: 8, expectPlaceholder: true},  // step 6: Tab → (1,1), all-placeholder
		{readIndex: 9, expectPlaceholder: true},  // step 7: Tab → (1,0), all-placeholder
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
	// TmuxEnumerator.ListWindowsAndPanesInSession is called exactly once —
	// at preview-open. No cycle handler must reach the enumerator, even
	// after the reader has fully degraded to (nil, nil) on every paneKey.
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
	// Across the full sequence every (windowIdx, paneIdx) coordinate from
	// the 2x2 fixture must be visited at least once — cycle keys must keep
	// driving structural traversal regardless of placeholder progression.
	enum, reader, _ := progressivePlaceholderFixture(t)

	m, ok := NewPreviewModel("work", enum, reader, nil, 80, 24)
	if !ok {
		t.Fatalf("expected ok=true on construction, got false")
	}

	visited := map[[2]int]int{}
	visited[[2]int{m.windowIdx, m.paneIdx}]++ // initial focus

	models := driveKilledSessionSequence(m)
	for i, mm := range models {
		visited[[2]int{mm.windowIdx, mm.paneIdx}]++
		// Every step's (windowIdx, paneIdx) must match the planned trajectory.
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

	// Tail call budget across the sequence is exactly 1 (open-time read for
	// pane (0,0)) + 8 (each cycle keypress triggers one synchronous read for
	// the newly-focused pane). No double-reads, no skipped reads.
	const wantReads = 1 + 8
	if len(reader.calls) != wantReads {
		t.Errorf("expected %d Tail calls (1 open + 8 cycles), got %d (calls=%v)",
			wantReads, len(reader.calls), reader.calls)
	}
}

func TestPreviewExternalKill_NoPanicWhenAllPanesReturnNilNilMidPreview(t *testing.T) {
	// Panic guard. Drives the full cycle sequence over a reader that
	// degrades to (nil, nil) for every paneKey by the final phase. Any
	// panic in the dispatcher, viewport, chrome line, or View() composition
	// would abort the goroutine and surface as a test failure via the
	// deferred recover.
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

	// Exercise View() at every step — the chrome+viewport composition path
	// is the most likely panic surface if the model has lost a structural
	// invariant under degradation.
	for i, mm := range models {
		// View() composes chromeLine() and viewport.View() internally; if
		// any of the three would panic, the contains-check below would
		// not be reached. The assertion doubles as the panic surface.
		if !strings.Contains(stripANSI(mm.View()), stripANSI(mm.chromeLine())) {
			t.Errorf("step %d: View() did not contain chromeLine() — composition broken", i)
		}
	}
}

func TestPreviewExternalKill_EscDismissesCleanlyFromFullyDegradedPreview(t *testing.T) {
	// After the full cycle sequence the reader has degraded to (nil, nil)
	// for every paneKey. Esc must still emit previewDismissedMsg cleanly —
	// no panic, no error frame, exactly the dismiss message. The broader
	// refresh path (sessions list re-fetch) is owned by 4-5; this test
	// pins only the dismiss-msg emission.
	enum, reader, _ := progressivePlaceholderFixture(t)

	m, ok := NewPreviewModel("work", enum, reader, nil, 80, 24)
	if !ok {
		t.Fatalf("expected ok=true on construction, got false")
	}

	models := driveKilledSessionSequence(m)
	final := models[len(models)-1]

	// Confirm the model is fully degraded — viewport on the final step
	// renders the placeholder (all-placeholder phase).
	if got := stripTrailingBlanks(final.viewport.View()); got != previewPlaceholder {
		t.Fatalf("test setup invariant: expected fully-degraded viewport before Esc; got %q", got)
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Esc on fully-degraded preview panicked: %v", r)
		}
	}()

	_, cmd := final.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatalf("expected non-nil tea.Cmd from Esc, got nil")
	}
	msg := cmd()
	if _, ok := msg.(previewDismissedMsg); !ok {
		t.Errorf("Esc cmd produced %T; want previewDismissedMsg", msg)
	}

	// Esc must not trigger any further enumeration or Tail call.
	if enum.callCount != 1 {
		t.Errorf("Esc triggered re-enumeration: callCount = %d; want 1", enum.callCount)
	}
	if len(reader.calls) != 1+len(killedSessionSequence) {
		t.Errorf("Esc triggered an extra Tail call: got %d total, want %d",
			len(reader.calls), 1+len(killedSessionSequence))
	}
}
