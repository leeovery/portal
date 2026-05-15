package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Tests for the Sessions-page inline-flash tick-based auto-clear
// infrastructure (spec § Inline flash > Clear conditions, § Replacement
// on rapid successive bails). These cover flashTickMsg dispatch in
// Model.Update, the generation-guard discriminator, the
// flashAutoClearDuration constant, and flashTickCmd construction.
//
// No caller schedules a tick at this phase — tasks 2-5 and 2-6 wire that.
// These tests assert the primitives are present and the Update branch
// behaves correctly when a flashTickMsg lands.

func TestFlashTickMsg_ClearsFlashWhenGenMatches(t *testing.T) {
	var m Model
	m.setFlash("hello") // flashGen becomes 1
	if m.flashText != "hello" || m.flashGen != 1 {
		t.Fatalf("setup invariant: want text=%q gen=1, got text=%q gen=%d", "hello", m.flashText, m.flashGen)
	}

	updated, cmd := m.Update(flashTickMsg{Gen: 1})
	mm, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", updated)
	}
	if mm.flashText != "" {
		t.Fatalf("flashText after matching tick: want %q, got %q", "", mm.flashText)
	}
	if mm.flashGen != 1 {
		t.Fatalf("flashGen after matching tick: want 1 (preserved), got %d", mm.flashGen)
	}
	if cmd != nil {
		t.Fatalf("flashTickMsg handler returned non-nil cmd: %v", cmd)
	}
}

func TestFlashTickMsg_NoOpWhenGenStale(t *testing.T) {
	// Captured gen from a prior setFlash; a newer setFlash has bumped
	// the live gen so the in-flight tick is stale.
	var m Model
	m.setFlash("first")  // gen=1
	m.setFlash("second") // gen=2 — supersedes the prior flash

	updated, cmd := m.Update(flashTickMsg{Gen: 1})
	mm, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", updated)
	}
	if mm.flashText != "second" {
		t.Fatalf("flashText after stale tick: want %q (unchanged), got %q", "second", mm.flashText)
	}
	if mm.flashGen != 2 {
		t.Fatalf("flashGen after stale tick: want 2 (unchanged), got %d", mm.flashGen)
	}
	if cmd != nil {
		t.Fatalf("flashTickMsg handler returned non-nil cmd: %v", cmd)
	}
}

func TestFlashTickMsg_IdempotentAfterManualClear(t *testing.T) {
	// A tick that arrives after the flash was already cleared (e.g. by a
	// keystroke) is a no-op — must not panic, must not flip any state.
	var m Model
	m.setFlash("x") // gen=1
	m.clearFlash()  // text="", gen=1

	updated, cmd := m.Update(flashTickMsg{Gen: 1})
	mm, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", updated)
	}
	if mm.flashText != "" {
		t.Fatalf("flashText after tick on already-cleared: want %q, got %q", "", mm.flashText)
	}
	if mm.flashGen != 1 {
		t.Fatalf("flashGen after tick on already-cleared: want 1 (unchanged), got %d", mm.flashGen)
	}
	if cmd != nil {
		t.Fatalf("flashTickMsg handler returned non-nil cmd: %v", cmd)
	}
}

func TestFlashTickMsg_StaleTickAgainstFreshModelDropped(t *testing.T) {
	// Lifecycle invariant: a tick whose model has been replaced lands at a
	// fresh Model whose flashGen is zero — gen=1 from the prior model
	// mismatches and is silently dropped.
	var m Model // zero-value model, flashGen=0, flashText=""
	updated, cmd := m.Update(flashTickMsg{Gen: 1})
	mm, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", updated)
	}
	if mm.flashText != "" {
		t.Fatalf("flashText on fresh model: want %q, got %q", "", mm.flashText)
	}
	if mm.flashGen != 0 {
		t.Fatalf("flashGen on fresh model: want 0, got %d", mm.flashGen)
	}
	if cmd != nil {
		t.Fatalf("flashTickMsg handler returned non-nil cmd: %v", cmd)
	}
}

func TestFlashTickCmd_ReturnsNonNilCmd(t *testing.T) {
	cmd := flashTickCmd(7)
	if cmd == nil {
		t.Fatal("flashTickCmd returned nil tea.Cmd")
	}
}

func TestFlashTickCmd_InvokeProducesFlashTickMsgWithCapturedGen(t *testing.T) {
	// flashTickCmd(gen) must produce a tea.Cmd that, when invoked,
	// eventually emits a flashTickMsg carrying the captured gen. We use a
	// short-circuit by running the cmd in a goroutine with a generous
	// timeout — tea.Tick waits flashAutoClearDuration before firing, so
	// the test would otherwise sleep ~3s. To avoid that, we don't invoke
	// the underlying tick directly; we assert the gen capture by running
	// flashTickCmd on a goroutine and verifying it eventually emits the
	// expected message shape.
	const wantGen uint64 = 42
	cmd := flashTickCmd(wantGen)
	if cmd == nil {
		t.Fatal("flashTickCmd returned nil tea.Cmd")
	}

	type result struct {
		msg tea.Msg
	}
	ch := make(chan result, 1)
	go func() { ch <- result{msg: cmd()} }()

	select {
	case r := <-ch:
		ftm, ok := r.msg.(flashTickMsg)
		if !ok {
			t.Fatalf("flashTickCmd produced %T, want flashTickMsg", r.msg)
		}
		if ftm.Gen != wantGen {
			t.Fatalf("flashTickMsg.Gen: want %d, got %d", wantGen, ftm.Gen)
		}
	case <-time.After(flashAutoClearDuration + 2*time.Second):
		t.Fatalf("flashTickCmd did not fire within %v", flashAutoClearDuration+2*time.Second)
	}
}

func TestFlashAutoClearDuration_InSanityRange(t *testing.T) {
	// Spec § Inline flash > Clear conditions: "long enough to read, short
	// enough not to linger". ~3s is the noted default. Sanity-bound the
	// constant to [1s, 10s] so a refactor doesn't accidentally set it to
	// 0 (would clear instantly) or 30s (would linger).
	if flashAutoClearDuration < 1*time.Second {
		t.Errorf("flashAutoClearDuration too short: %v (want >= 1s)", flashAutoClearDuration)
	}
	if flashAutoClearDuration > 10*time.Second {
		t.Errorf("flashAutoClearDuration too long: %v (want <= 10s)", flashAutoClearDuration)
	}
}
