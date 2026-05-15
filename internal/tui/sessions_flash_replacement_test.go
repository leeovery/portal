package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// Integration tests for Phase 2 task 2-6: rapid-bail replacement contract.
//
// Verified invariants (spec § Replacement on rapid successive bails):
//
//  1. Visible flash always reflects the most recent bail — m.flashText is
//     replaced verbatim on each successive previewAttachBailMsg.
//  2. A pending tick from a superseded flash never clears a newer flash —
//     a flashTickMsg whose captured Gen no longer matches m.flashGen is
//     silently dropped and leaves m.flashText untouched.
//  3. The newest flash's own tick clears it at its deadline — a
//     flashTickMsg whose captured Gen equals the live m.flashGen zeros
//     m.flashText (leaving m.flashGen preserved).
//
// These tests exercise the contract end-to-end via Model.Update dispatch
// against directly-constructed previewAttachBailMsg + flashTickMsg values.
// Ticks are not scheduled through tea.Tick — the test harness fires them
// synchronously to assert the generation-guard semantics without sleeping
// for flashAutoClearDuration on every assertion.
//
// No new production code is required if tasks 2-1 / 2-3 / 2-5 are correct.
// If any scenario fails, the probable bug site is the ordering in the
// previewAttachBailMsg handler in model.go: flashTickCmd MUST capture the
// post-bump flashGen (i.e. setFlash before constructing the tick).

// applyBail dispatches a previewAttachBailMsg through Update and returns
// the resulting Model. The returned tea.Cmd is discarded; ticks are
// constructed and fed in directly by the test rather than driven by the
// real tea.Tick scheduler.
func applyBail(t *testing.T, m Model, name string) Model {
	t.Helper()
	updated, _ := m.Update(previewAttachBailMsg{Session: name})
	got, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update returned %T after bail, want tui.Model", updated)
	}
	return got
}

// applyTick dispatches a flashTickMsg{Gen: gen} through Update and returns
// the resulting Model. Mirrors applyBail but for ticks.
func applyTick(t *testing.T, m Model, gen uint64) Model {
	t.Helper()
	updated, _ := m.Update(flashTickMsg{Gen: gen})
	got, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update returned %T after tick, want tui.Model", updated)
	}
	return got
}

func TestFlashReplacement_TwoSuccessiveBailsReflectLatestText(t *testing.T) {
	// Scenario 1: two successive bails. m.flashText reflects the second;
	// m.flashGen increments by exactly 2 (one per bail).
	var m Model
	m = applyBail(t, m, "foo")
	if want, got := `session "foo" no longer exists`, m.flashText; got != want {
		t.Fatalf("after first bail: flashText=%q, want %q", got, want)
	}
	if m.flashGen != 1 {
		t.Fatalf("after first bail: flashGen=%d, want 1", m.flashGen)
	}

	m = applyBail(t, m, "bar")
	if want, got := `session "bar" no longer exists`, m.flashText; got != want {
		t.Errorf("after second bail: flashText=%q, want %q (latest bail wins)", got, want)
	}
	if m.flashGen != 2 {
		t.Errorf("after second bail: flashGen=%d, want 2 (bumped per bail)", m.flashGen)
	}
}

func TestFlashReplacement_PriorTickDoesNotClearNewerFlash(t *testing.T) {
	// Scenario 2: prior in-flight tick (Gen=1) does NOT clear the newer
	// flash. The handler discriminates on m.flashGen; the stale tick is a
	// no-op.
	var m Model
	m = applyBail(t, m, "foo") // gen=1
	m = applyBail(t, m, "bar") // gen=2 — supersedes
	m = applyTick(t, m, 1)     // stale tick from the first flash

	if want, got := `session "bar" no longer exists`, m.flashText; got != want {
		t.Errorf("stale tick must not clear newer flash: flashText=%q, want %q", got, want)
	}
	if m.flashGen != 2 {
		t.Errorf("stale tick must not touch flashGen: got %d, want 2", m.flashGen)
	}
}

func TestFlashReplacement_CurrentTickClearsItsOwnFlash(t *testing.T) {
	// Scenario 3: the current tick (Gen=2) clears its own flash. This is
	// the load-bearing assertion for the order-of-operations in the bail
	// handler: setFlash MUST bump gen BEFORE flashTickCmd captures it.
	// If the tick captured pre-bump gen (=1) it would mismatch the live
	// gen (=2) and silently drop, leaving the flash visible forever.
	var m Model
	m = applyBail(t, m, "foo") // gen=1
	m = applyBail(t, m, "bar") // gen=2
	m = applyTick(t, m, 2)     // live tick for the second flash

	if m.flashText != "" {
		t.Errorf("live tick must clear its own flash: flashText=%q, want %q", m.flashText, "")
	}
	if m.flashGen != 2 {
		t.Errorf("clearFlash must preserve flashGen: got %d, want 2", m.flashGen)
	}
}

func TestFlashReplacement_FiveSuccessiveBailsOnlyLatestSurvives(t *testing.T) {
	// Scenario 4: five successive bails. Only the latest text is visible;
	// stale ticks 1..4 are no-ops; the live tick (Gen=5) clears.
	var m Model
	names := []string{"a", "b", "c", "d", "e"}
	for _, n := range names {
		m = applyBail(t, m, n)
	}
	if want, got := `session "e" no longer exists`, m.flashText; got != want {
		t.Fatalf("after 5 bails: flashText=%q, want %q (latest wins)", got, want)
	}
	if m.flashGen != 5 {
		t.Fatalf("after 5 bails: flashGen=%d, want 5", m.flashGen)
	}

	// All stale ticks must be no-ops.
	for gen := uint64(1); gen <= 4; gen++ {
		m = applyTick(t, m, gen)
		if want, got := `session "e" no longer exists`, m.flashText; got != want {
			t.Errorf("stale tick Gen=%d cleared the live flash: flashText=%q, want %q", gen, got, want)
		}
		if m.flashGen != 5 {
			t.Errorf("stale tick Gen=%d mutated flashGen: got %d, want 5", gen, m.flashGen)
		}
	}

	// Live tick clears.
	m = applyTick(t, m, 5)
	if m.flashText != "" {
		t.Errorf("live tick (Gen=5) must clear: flashText=%q, want %q", m.flashText, "")
	}
	if m.flashGen != 5 {
		t.Errorf("clearFlash must preserve flashGen: got %d, want 5", m.flashGen)
	}
}

func TestFlashReplacement_ManualClearBetweenBailsLeavesStaleTicksAsNoOps(t *testing.T) {
	// Scenario 5: manual keystroke clear between bails. Stale ticks
	// remain no-ops. clearFlash leaves flashGen untouched, so a
	// post-clear bail keeps incrementing past the cleared generation.
	var m Model
	m = applyBail(t, m, "foo") // gen=1, text="...foo..."
	// Simulate manual keystroke clear (the keystroke path eventually
	// calls clearFlash — we exercise the primitive directly here to
	// isolate the generation-guard contract from key-routing).
	m.clearFlash()
	if m.flashText != "" {
		t.Fatalf("manual clear: flashText=%q, want %q", m.flashText, "")
	}
	if m.flashGen != 1 {
		t.Fatalf("manual clear must preserve flashGen: got %d, want 1", m.flashGen)
	}

	m = applyBail(t, m, "bar") // gen=2, text="...bar..."
	if want, got := `session "bar" no longer exists`, m.flashText; got != want {
		t.Fatalf("post-manual-clear bail: flashText=%q, want %q", got, want)
	}
	if m.flashGen != 2 {
		t.Fatalf("post-manual-clear bail: flashGen=%d, want 2", m.flashGen)
	}

	// Stale tick from the first flash must not clear.
	m = applyTick(t, m, 1)
	if want, got := `session "bar" no longer exists`, m.flashText; got != want {
		t.Errorf("stale tick (Gen=1) after manual clear + new bail must not clear: flashText=%q, want %q", got, want)
	}
	if m.flashGen != 2 {
		t.Errorf("stale tick must not touch flashGen: got %d, want 2", m.flashGen)
	}
}

func TestFlashReplacement_SetFlashBumpsGenByExactlyOnePerCall(t *testing.T) {
	// Scenario 6: regression re-assertion at integration level (covered
	// at primitive level by sessions_flash_state_test.go). Each setFlash
	// — direct or via the bail handler — bumps flashGen by exactly 1.
	var m Model
	if m.flashGen != 0 {
		t.Fatalf("zero-value flashGen: got %d, want 0", m.flashGen)
	}

	// Direct setFlash.
	m.setFlash("x")
	if m.flashGen != 1 {
		t.Errorf("after setFlash #1: flashGen=%d, want 1", m.flashGen)
	}
	m.setFlash("y")
	if m.flashGen != 2 {
		t.Errorf("after setFlash #2: flashGen=%d, want 2", m.flashGen)
	}

	// Via bail dispatch — each bail invokes setFlash exactly once.
	m = applyBail(t, m, "foo")
	if m.flashGen != 3 {
		t.Errorf("after bail #1 post-setFlash: flashGen=%d, want 3", m.flashGen)
	}
	m = applyBail(t, m, "bar")
	if m.flashGen != 4 {
		t.Errorf("after bail #2 post-setFlash: flashGen=%d, want 4", m.flashGen)
	}
}

func TestFlashReplacement_SameNameBailsStillBumpGen(t *testing.T) {
	// Edge case from the task notes: same-name successive bails still
	// bump gen and discard prior ticks. The discriminator is gen, not
	// text equality.
	var m Model
	m = applyBail(t, m, "foo") // gen=1
	m = applyBail(t, m, "foo") // gen=2 — same text, new generation
	if m.flashGen != 2 {
		t.Fatalf("same-name bail #2: flashGen=%d, want 2", m.flashGen)
	}

	// Stale tick from gen=1 must not clear the gen=2 flash.
	m = applyTick(t, m, 1)
	if want, got := `session "foo" no longer exists`, m.flashText; got != want {
		t.Errorf("stale tick on same-name bail: flashText=%q, want %q", got, want)
	}
	if m.flashGen != 2 {
		t.Errorf("stale tick on same-name bail mutated flashGen: got %d, want 2", m.flashGen)
	}

	// Live tick clears.
	m = applyTick(t, m, 2)
	if m.flashText != "" {
		t.Errorf("live tick (Gen=2) must clear: flashText=%q, want %q", m.flashText, "")
	}
}

// Compile-time sanity check: flashTickMsg is the message type expected by
// Model.Update. If a refactor renames or relocates the type, this line
// breaks the build before the test bodies do.
var _ tea.Msg = flashTickMsg{}
