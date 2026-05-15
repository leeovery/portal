package tui

import "testing"

// Tests for the Sessions-page inline-flash state primitives (spec
// § Inline flash — feature-local infrastructure, § Replacement on rapid
// successive bails). These cover the pure state plumbing only: zero
// value, setFlash gen-bump + text set, clearFlash text-only zero,
// idempotence, and combined sequencing. No render, scheduling, or
// message-dispatch coverage — those land in later Phase 2 tasks.

func TestModel_FlashState_ZeroValue(t *testing.T) {
	var m Model
	if m.flashText != "" {
		t.Fatalf("zero-value flashText: want %q, got %q", "", m.flashText)
	}
	if m.flashGen != 0 {
		t.Fatalf("zero-value flashGen: want 0, got %d", m.flashGen)
	}
}

func TestModel_SetFlash_SetsTextAndBumpsGen(t *testing.T) {
	var m Model
	m.setFlash("hello")
	if m.flashText != "hello" {
		t.Fatalf("flashText after setFlash: want %q, got %q", "hello", m.flashText)
	}
	if m.flashGen != 1 {
		t.Fatalf("flashGen after first setFlash: want 1, got %d", m.flashGen)
	}
}

func TestModel_SetFlash_GenIncrementsMonotonically(t *testing.T) {
	var m Model
	m.setFlash("a")
	m.setFlash("b")
	m.setFlash("c")
	if m.flashGen != 3 {
		t.Fatalf("flashGen after three setFlash calls: want 3, got %d", m.flashGen)
	}
	if m.flashText != "c" {
		t.Fatalf("flashText after three setFlash calls: want %q, got %q", "c", m.flashText)
	}
}

func TestModel_ClearFlash_ZerosTextLeavesGen(t *testing.T) {
	var m Model
	m.setFlash("x")
	if m.flashGen != 1 || m.flashText != "x" {
		t.Fatalf("setup invariant: want gen=1 text=%q, got gen=%d text=%q", "x", m.flashGen, m.flashText)
	}
	m.clearFlash()
	if m.flashText != "" {
		t.Fatalf("flashText after clearFlash: want %q, got %q", "", m.flashText)
	}
	if m.flashGen != 1 {
		t.Fatalf("flashGen after clearFlash: want 1 (unchanged), got %d", m.flashGen)
	}
}

func TestModel_ClearFlash_IdempotentOnAlreadyCleared(t *testing.T) {
	var m Model
	m.clearFlash()
	if m.flashText != "" {
		t.Fatalf("flashText after clearFlash on zero value: want %q, got %q", "", m.flashText)
	}
	if m.flashGen != 0 {
		t.Fatalf("flashGen after clearFlash on zero value: want 0, got %d", m.flashGen)
	}

	m.setFlash("once")
	m.clearFlash()
	m.clearFlash()
	if m.flashText != "" {
		t.Fatalf("flashText after repeated clearFlash: want %q, got %q", "", m.flashText)
	}
	if m.flashGen != 1 {
		t.Fatalf("flashGen after repeated clearFlash: want 1, got %d", m.flashGen)
	}
}

func TestModel_FlashState_SetClearSet(t *testing.T) {
	var m Model
	m.setFlash("a")
	m.clearFlash()
	m.setFlash("b")
	if m.flashText != "b" {
		t.Fatalf("flashText after set→clear→set: want %q, got %q", "b", m.flashText)
	}
	if m.flashGen != 2 {
		t.Fatalf("flashGen after set→clear→set: want 2, got %d", m.flashGen)
	}
}

func TestModel_SetFlash_EmptyStringStillBumpsGen(t *testing.T) {
	// Per task edge cases: setFlash("") is a state primitive; the caller
	// decides what counts as a flash. Empty input still bumps gen.
	var m Model
	m.setFlash("")
	if m.flashText != "" {
		t.Fatalf("flashText after setFlash(\"\"): want %q, got %q", "", m.flashText)
	}
	if m.flashGen != 1 {
		t.Fatalf("flashGen after setFlash(\"\"): want 1, got %d", m.flashGen)
	}
}
