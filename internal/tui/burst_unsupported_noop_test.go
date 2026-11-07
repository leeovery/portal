package tui

import (
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/spawntest"
)

func wireUnsupportedBurstSeams(m *Model, adapter spawn.Adapter, ack spawn.AckChannelFull) {
	m.detector = &fakeDetector{identity: appleTerminalIdentity()}
	m.resolve = func(spawn.Identity) (spawn.Adapter, spawn.Resolution) {
		return adapter, spawn.ResolutionUnsupported
	}
	m.sessionExists = allPresent
	m.ackChannel = ack
	m.spawnExe = func() (string, error) { return "/abs/portal", nil }
	m.spawnEnv = func() (spawn.WindowEnvironment, error) {
		return spawn.WindowEnvironment{Set: []string{"PATH=/usr/bin"}}, nil
	}
}

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

func TestUnsupportedFlashText(t *testing.T) {
	tests := []struct {
		name string
		id   spawn.Identity
		want string
	}{
		{
			name: "named undriven identity",
			id:   appleTerminalIdentity(),
			want: "can't open new windows in Apple Terminal · com.apple.Terminal — nothing opened",
		},
		{
			name: "NULL identity (remote/mosh)",
			id:   spawn.Identity{},
			want: "can't open new windows over a remote connection — nothing opened",
		},
		{
			name: "unsure NULL identity (detection couldn't tell)",
			id:   spawn.Identity{Unsure: true},
			want: "can't tell which terminal Portal is running in — nothing opened",
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

func TestBurstUnsupported_NonNullAtomicNoOp(t *testing.T) {
	sessions := []Session{
		{Name: "alpha"},
		{Name: "bravo"},
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
	const want = "can't open new windows in Apple Terminal · com.apple.Terminal — nothing opened"
	if m.flashText != want {
		t.Errorf("flashText = %q, want %q (named identity, ⚠ added by the warning band)", m.flashText, want)
	}
}

func TestBurstUnsupported_NullFlash(t *testing.T) {
	sessions := []Session{
		{Name: "alpha"},
		{Name: "bravo"},
	}
	ack := &spawntest.FakeAckChannel{}
	adapter := &spawntest.FakeAdapter{Ack: ack}
	m := NewModelWithSessions(sessions)
	wireUnsupportedBurstSeams(&m, adapter, ack)
	m = markTwo(t, m)
	m = resolveDetection(t, m, spawn.Identity{})
	if !m.DetectUnsupported() {
		t.Fatal("precondition: a NULL identity must resolve unsupported")
	}

	m, cmd := pressEnter(t, m)

	assertAtomicNoOp(t, m, adapter)
	if isQuitCmd(cmd) {
		t.Error("NULL N≥2 Enter must NOT tea.Quit")
	}
	const want = "can't open new windows over a remote connection — nothing opened"
	if m.flashText != want {
		t.Errorf("flashText = %q, want %q (NULL identity plain remote-connection line)", m.flashText, want)
	}
}

func TestBurstUnsupported_DeferredThenUnsupported(t *testing.T) {
	sessions := []Session{
		{Name: "alpha"},
		{Name: "bravo"},
	}
	ack := &spawntest.FakeAckChannel{}
	adapter := &spawntest.FakeAdapter{Ack: ack}
	m := NewModelWithSessions(sessions)
	wireUnsupportedBurstSeams(&m, adapter, ack)
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

	updated, cmd2 := m.Update(terminalDetectedMsg{identity: appleTerminalIdentity()})
	m = updated.(Model)

	assertAtomicNoOp(t, m, adapter)
	if isQuitCmd(cmd2) {
		t.Error("deferred unsupported resolution must NOT tea.Quit")
	}
	const want = "can't open new windows in Apple Terminal · com.apple.Terminal — nothing opened"
	if m.flashText != want {
		t.Errorf("flashText = %q, want %q (deferred → unsupported re-asserts the named flash)", m.flashText, want)
	}
}

func TestBurstUnsupported_SupportedStillDispatches(t *testing.T) {
	sessions := []Session{
		{Name: "alpha"},
		{Name: "bravo"},
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

	m = drainBatchToModel(t, m, cmd)
	if len(adapter.Calls) != 1 {
		t.Errorf("the supported burst must open the one external window; OpenWindow calls = %d, want 1", len(adapter.Calls))
	}
}
