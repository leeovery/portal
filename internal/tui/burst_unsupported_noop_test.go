package tui

// restore-host-terminal-windows-6-9 — N≥2 Enter on a resolved-unsupported / NULL
// terminal: the atomic no-op + re-asserted banner flash.
//
// These white-box (package tui) tests drive the §5 multi-select N≥2 Enter through
// decideBurst's unsupported arm (DetectUnsupported() true) and assert the ATOMIC
// no-op: no burst pipe, no adapter resolve/call, no self-attach (Selected()==""),
// no tea.Quit, still in multi-select mode with the selection INTACT, and the
// re-asserted warning flash naming the identity (named) or the honest no-host-local
// line (NULL). A supported (ghostty) identity still dispatches the burst (unchanged).
//
// No t.Parallel: consistent with the rest of the tui test surface.

import (
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/spawntest"
	"github.com/leeovery/portal/internal/tmux"
)

// wireUnsupportedBurstSeams wires the §6 burst seams so the resolve seam maps ANY
// identity to the given fake adapter + ResolutionUnsupported. Using the fake adapter
// (rather than the production native adapter nativeResolve() would return) keeps the
// "no adapter call" assertion meaningful AND guarantees a regression that wrongly
// dispatched would record a fake Call rather than opening a real host window.
func wireUnsupportedBurstSeams(m *Model, adapter spawn.Adapter, ack spawn.AckChannelFull) {
	m.detector = &fakeDetector{identity: appleTerminalIdentity()}
	m.resolve = func(spawn.Identity) (spawn.Adapter, spawn.Resolution) {
		return adapter, spawn.ResolutionUnsupported
	}
	m.sessionExists = allPresent
	m.ackChannel = ack
	m.spawnExe = func() (string, error) { return "/abs/portal", nil }
	m.spawnGetenv = func(string) string { return "/usr/bin" }
}

// markTwo enters multi-select and marks the first two rows top-to-bottom.
func markTwo(t *testing.T, m Model) Model {
	t.Helper()
	m = enterMultiSelectEmpty(t, m)
	m = markRow(t, m, 0)
	m = markRow(t, m, 1)
	if m.SelectedSessionCount() != 2 {
		t.Fatalf("precondition: 2 marked, got %d", m.SelectedSessionCount())
	}
	return m
}

// assertAtomicNoOp asserts the §6-9 no-op invariants shared by every unsupported
// resolution: nothing pending, no pipe, no adapter call, no self-attach, still in
// multi-select mode, selection intact (both marked names kept — no prune).
func assertAtomicNoOp(t *testing.T, m Model, adapter *spawntest.FakeAdapter) {
	t.Helper()
	if m.BurstPending() {
		t.Error("unsupported N≥2 Enter must NOT enter burst-pending (atomic no-op)")
	}
	if m.burstPipe != nil {
		t.Error("unsupported N≥2 Enter must construct NO burst pipe")
	}
	if len(adapter.Calls) != 0 {
		t.Errorf("unsupported N≥2 Enter must call NO adapter method; OpenWindow calls = %d", len(adapter.Calls))
	}
	if m.Selected() != "" {
		t.Errorf("unsupported N≥2 Enter must NOT self-attach; Selected() = %q, want empty", m.Selected())
	}
	if !m.MultiSelectActive() {
		t.Error("unsupported N≥2 Enter must stay in multi-select mode")
	}
	if m.SelectedSessionCount() != 2 {
		t.Errorf("the selection must be INTACT after the no-op (no prune); count = %d, want 2", m.SelectedSessionCount())
	}
	for _, name := range []string{"alpha", "bravo"} {
		if !m.IsSessionSelected(name) {
			t.Errorf("marked session %q must remain marked after the no-op (nothing was gone, only unsupported)", name)
		}
	}
}

// TestUnsupportedFlashText pins the exact §6-9 flash copy at the pure-function
// level: the named-identity form mirrors the shared spawn.UnsupportedNoopMessage
// (the same copy the open burst's unsupported gate emits) without the `spawn:`
// prefix, the NULL form is the honest no-host-local line, and
// BOTH carry the `— nothing opened` RESPONSE suffix. No literal ⚠ (the band adds it).
func TestUnsupportedFlashText(t *testing.T) {
	tests := []struct {
		name string
		id   spawn.Identity
		want string
	}{
		{
			name: "named undriven identity",
			id:   appleTerminalIdentity(),
			want: "unsupported terminal — Apple Terminal · com.apple.Terminal — nothing opened",
		},
		{
			name: "NULL identity (remote/mosh or transient error)",
			id:   spawn.Identity{},
			want: "no host-local terminal — nothing opened",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := unsupportedFlashText(tc.id); got != tc.want {
				t.Errorf("unsupportedFlashText() = %q, want %q", got, tc.want)
			}
			if strings.Contains(unsupportedFlashText(tc.id), flashWarningGlyph) {
				t.Errorf("the flash text must NOT embed the ⚠ glyph (the warning band prepends it): %q", unsupportedFlashText(tc.id))
			}
		})
	}
}

// TestBurstUnsupported_NonNullAtomicNoOp is the core §6-9 assertion for a resolved-
// unsupported non-NULL undriven identity (Apple Terminal / com.apple.Terminal): the
// N≥2 Enter is an atomic no-op and re-asserts the named unsupported flash.
//
// Multi-select is entered during the async in-flight window (A1 — markTwo runs BEFORE
// detection resolves, so the proactive entry block is inert), then detection resolves
// unsupported and the Enter drives decideBurst's retained reactive unsupported arm.
func TestBurstUnsupported_NonNullAtomicNoOp(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 1},
		{Name: "bravo", Windows: 2},
	}
	ack := &spawntest.FakeAckChannel{}
	adapter := &spawntest.FakeAdapter{Ack: ack}
	m := NewModelWithSessions(sessions)
	wireUnsupportedBurstSeams(&m, adapter, ack)
	m = markTwo(t, m)
	m = resolveDetection(t, m, appleTerminalIdentity())
	if !m.DetectUnsupported() {
		t.Fatal("precondition: com.apple.Terminal must resolve unsupported")
	}

	m, cmd := pressEnter(t, m)

	assertAtomicNoOp(t, m, adapter)
	if isQuitCmd(cmd) {
		t.Error("unsupported N≥2 Enter must NOT tea.Quit")
	}
	const want = "unsupported terminal — Apple Terminal · com.apple.Terminal — nothing opened"
	if m.flashText != want {
		t.Errorf("flashText = %q, want %q (named identity, ⚠ added by the warning band)", m.flashText, want)
	}
}

// TestBurstUnsupported_NullFlash covers a resolved NULL identity (remote/mosh — and
// identically a transient detection error, which folds to the same Identity{}): the
// same atomic no-op with the honest no-host-local flash (no identity string).
//
// Multi-select is entered during the async in-flight window (A1 — markTwo runs BEFORE
// detection resolves, so the proactive entry block is inert), then detection resolves
// unsupported and the Enter drives decideBurst's retained reactive unsupported arm.
func TestBurstUnsupported_NullFlash(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 1},
		{Name: "bravo", Windows: 2},
	}
	ack := &spawntest.FakeAckChannel{}
	adapter := &spawntest.FakeAdapter{Ack: ack}
	m := NewModelWithSessions(sessions)
	wireUnsupportedBurstSeams(&m, adapter, ack)
	m = markTwo(t, m)
	// spawn.Identity{} is the NULL identity for BOTH a remote/mosh client AND a
	// transient detection error (Phase-1 folds the error to Identity{}), so this one
	// input pins the transient-error edge case too.
	m = resolveDetection(t, m, spawn.Identity{})
	if !m.DetectUnsupported() {
		t.Fatal("precondition: a NULL identity must resolve unsupported")
	}

	m, cmd := pressEnter(t, m)

	assertAtomicNoOp(t, m, adapter)
	if isQuitCmd(cmd) {
		t.Error("NULL N≥2 Enter must NOT tea.Quit")
	}
	const want = "no host-local terminal — nothing opened"
	if m.flashText != want {
		t.Errorf("flashText = %q, want %q (NULL identity honest line)", m.flashText, want)
	}
}

// TestBurstUnsupported_DeferredThenUnsupported covers the in-flight-at-Enter path:
// an N≥2 Enter pressed while detection is in flight DEFERS, and the deferred-Enter
// resolution (the terminalDetectedMsg arm) lands the SAME atomic no-op + flash when
// it resolves to an unsupported identity.
func TestBurstUnsupported_DeferredThenUnsupported(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 1},
		{Name: "bravo", Windows: 2},
	}
	ack := &spawntest.FakeAckChannel{}
	adapter := &spawntest.FakeAdapter{Ack: ack}
	m := NewModelWithSessions(sessions)
	wireUnsupportedBurstSeams(&m, adapter, ack)
	// Detection dispatched but not resolved (in-flight).
	m.detectDispatched = true

	m = markTwo(t, m)
	m, cmd := pressEnter(t, m)

	if m.BurstPending() {
		t.Fatal("N≥2 Enter while detection is in flight must DEFER, not act")
	}
	if m.flashText != "" {
		t.Errorf("no flash may render while the deferred Enter awaits detection; flashText = %q", m.flashText)
	}
	if len(adapter.Calls) != 0 {
		t.Fatalf("no adapter call while detection is in flight, got %d", len(adapter.Calls))
	}
	if cmd != nil {
		t.Error("detection already in flight → no new detection cmd (nil)")
	}

	// Detection resolves to the unsupported Apple Terminal identity → the deferred
	// Enter resolution takes the atomic no-op + flash.
	updated, cmd2 := m.Update(terminalDetectedMsg{identity: appleTerminalIdentity()})
	m = updated.(Model)

	assertAtomicNoOp(t, m, adapter)
	if isQuitCmd(cmd2) {
		t.Error("deferred unsupported resolution must NOT tea.Quit")
	}
	const want = "unsupported terminal — Apple Terminal · com.apple.Terminal — nothing opened"
	if m.flashText != want {
		t.Errorf("flashText = %q, want %q (deferred → unsupported re-asserts the named flash)", m.flashText, want)
	}
}

// TestBurstUnsupported_SupportedStillDispatches is the guard that the unsupported arm
// does not regress the supported path: a resolved-supported (ghostty → native)
// identity still dispatches the async burst and sets NO flash.
func TestBurstUnsupported_SupportedStillDispatches(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 1},
		{Name: "bravo", Windows: 2},
	}
	ack := &spawntest.FakeAckChannel{}
	adapter := &spawntest.FakeAdapter{Ack: ack}
	m := NewModelWithSessions(sessions)
	wireBurstSeams(&m, adapter, spawn.ResolutionNative, allPresent, ack)
	m = resolveDetection(t, m, ghosttyIdentity())
	if m.DetectUnsupported() {
		t.Fatal("precondition: ghostty must resolve native (supported)")
	}

	m = markTwo(t, m)
	m, cmd := pressEnter(t, m)

	if !m.BurstPending() {
		t.Error("a supported N≥2 Enter must still dispatch the burst (unchanged)")
	}
	if m.flashText != "" {
		t.Errorf("a supported dispatch must set NO flash; flashText = %q", m.flashText)
	}

	// Drain the goroutine to the terminal outcome so it does not leak past the test.
	m = drainBatchToModel(t, m, cmd)
	if len(adapter.Calls) != 1 {
		t.Errorf("the supported burst must open the one external window; OpenWindow calls = %d, want 1", len(adapter.Calls))
	}
}
