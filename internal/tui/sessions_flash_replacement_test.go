package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func applyFlash(m Model, text string) Model {
	m.setFlash(text)
	return m
}

func applyTick(t *testing.T, m Model, gen uint64) Model {
	t.Helper()
	updated, _ := m.Update(flashTickMsg{Gen: gen})
	got, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update returned %T after tick, want tui.Model", updated)
	}
	return got
}

func TestFlashReplacement_TwoSuccessiveFlashesReflectLatestText(t *testing.T) {
	var m Model
	m = applyFlash(m, "foo")
	if want, got := "foo", m.flashText; got != want {
		t.Fatalf("after first flash: flashText=%q, want %q", got, want)
	}
	if m.flashGen != 1 {
		t.Fatalf("after first flash: flashGen=%d, want 1", m.flashGen)
	}

	m = applyFlash(m, "bar")
	if want, got := "bar", m.flashText; got != want {
		t.Errorf("after second flash: flashText=%q, want %q (latest flash wins)", got, want)
	}
	if m.flashGen != 2 {
		t.Errorf("after second flash: flashGen=%d, want 2 (bumped per flash)", m.flashGen)
	}
}

func TestFlashReplacement_PriorTickDoesNotClearNewerFlash(t *testing.T) {
	var m Model
	m = applyFlash(m, "foo")
	m = applyFlash(m, "bar")
	m = applyTick(t, m, 1)

	if want, got := "bar", m.flashText; got != want {
		t.Errorf("stale tick must not clear newer flash: flashText=%q, want %q", got, want)
	}
	if m.flashGen != 2 {
		t.Errorf("stale tick must not touch flashGen: got %d, want 2", m.flashGen)
	}
}

func TestFlashReplacement_CurrentTickClearsItsOwnFlash(t *testing.T) {
	var m Model
	m = applyFlash(m, "foo")
	m = applyFlash(m, "bar")
	m = applyTick(t, m, 2)

	if m.flashText != "" {
		t.Errorf("live tick must clear its own flash: flashText=%q, want %q", m.flashText, "")
	}
	if m.flashGen != 2 {
		t.Errorf("clearFlash must preserve flashGen: got %d, want 2", m.flashGen)
	}
}

func TestFlashReplacement_FiveSuccessiveFlashesOnlyLatestSurvives(t *testing.T) {
	var m Model
	names := []string{"a", "b", "c", "d", "e"}
	for _, n := range names {
		m = applyFlash(m, n)
	}
	if want, got := "e", m.flashText; got != want {
		t.Fatalf("after 5 flashes: flashText=%q, want %q (latest wins)", got, want)
	}
	if m.flashGen != 5 {
		t.Fatalf("after 5 flashes: flashGen=%d, want 5", m.flashGen)
	}

	for gen := uint64(1); gen <= 4; gen++ {
		m = applyTick(t, m, gen)
		if want, got := "e", m.flashText; got != want {
			t.Errorf("stale tick Gen=%d cleared the live flash: flashText=%q, want %q", gen, got, want)
		}
		if m.flashGen != 5 {
			t.Errorf("stale tick Gen=%d mutated flashGen: got %d, want 5", gen, m.flashGen)
		}
	}

	m = applyTick(t, m, 5)
	if m.flashText != "" {
		t.Errorf("live tick (Gen=5) must clear: flashText=%q, want %q", m.flashText, "")
	}
	if m.flashGen != 5 {
		t.Errorf("clearFlash must preserve flashGen: got %d, want 5", m.flashGen)
	}
}

func TestFlashReplacement_ManualClearBetweenFlashesLeavesStaleTicksAsNoOps(t *testing.T) {
	var m Model
	m = applyFlash(m, "foo")
	m.clearFlash()
	if m.flashText != "" {
		t.Fatalf("manual clear: flashText=%q, want %q", m.flashText, "")
	}
	if m.flashGen != 1 {
		t.Fatalf("manual clear must preserve flashGen: got %d, want 1", m.flashGen)
	}

	m = applyFlash(m, "bar")
	if want, got := "bar", m.flashText; got != want {
		t.Fatalf("post-manual-clear flash: flashText=%q, want %q", got, want)
	}
	if m.flashGen != 2 {
		t.Fatalf("post-manual-clear flash: flashGen=%d, want 2", m.flashGen)
	}

	m = applyTick(t, m, 1)
	if want, got := "bar", m.flashText; got != want {
		t.Errorf("stale tick (Gen=1) after manual clear + new flash must not clear: flashText=%q, want %q", got, want)
	}
	if m.flashGen != 2 {
		t.Errorf("stale tick must not touch flashGen: got %d, want 2", m.flashGen)
	}
}

func TestFlashReplacement_SetFlashBumpsGenByExactlyOnePerCall(t *testing.T) {
	var m Model
	if m.flashGen != 0 {
		t.Fatalf("zero-value flashGen: got %d, want 0", m.flashGen)
	}

	m.setFlash("x")
	if m.flashGen != 1 {
		t.Errorf("after setFlash #1: flashGen=%d, want 1", m.flashGen)
	}
	m.setFlash("y")
	if m.flashGen != 2 {
		t.Errorf("after setFlash #2: flashGen=%d, want 2", m.flashGen)
	}

	m = applyFlash(m, "foo")
	if m.flashGen != 3 {
		t.Errorf("after flash #1 post-setFlash: flashGen=%d, want 3", m.flashGen)
	}
	m = applyFlash(m, "bar")
	if m.flashGen != 4 {
		t.Errorf("after flash #2 post-setFlash: flashGen=%d, want 4", m.flashGen)
	}
}

func TestFlashReplacement_SameNameFlashesStillBumpGen(t *testing.T) {
	var m Model
	m = applyFlash(m, "foo")
	m = applyFlash(m, "foo")
	if m.flashGen != 2 {
		t.Fatalf("same-name flash #2: flashGen=%d, want 2", m.flashGen)
	}

	m = applyTick(t, m, 1)
	if want, got := "foo", m.flashText; got != want {
		t.Errorf("stale tick on same-name flash: flashText=%q, want %q", got, want)
	}
	if m.flashGen != 2 {
		t.Errorf("stale tick on same-name flash mutated flashGen: got %d, want 2", m.flashGen)
	}

	m = applyTick(t, m, 2)
	if m.flashText != "" {
		t.Errorf("live tick (Gen=2) must clear: flashText=%q, want %q", m.flashText, "")
	}
}

var _ tea.Msg = flashTickMsg{}
