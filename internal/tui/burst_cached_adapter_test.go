package tui

import (
	"testing"

	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/spawntest"
)

func wireTOCTOUResolveSeams(m *Model, detectAdapter spawn.Adapter, ack spawn.AckChannelFull) *int {
	calls := new(int)
	m.detector = &fakeDetector{identity: ghosttyIdentity()}
	m.resolve = func(spawn.Identity) (spawn.Adapter, spawn.Resolution) {
		*calls++
		if *calls == 1 {
			return detectAdapter, spawn.ResolutionNative
		}
		return nil, spawn.ResolutionUnsupported
	}
	m.sessionExists = allPresent
	m.ackChannel = ack
	m.spawnExe = func() (string, error) { return "/abs/portal", nil }
	m.spawnEnv = func() (spawn.WindowEnvironment, error) {
		return spawn.WindowEnvironment{Set: []string{"PATH=/usr/bin"}}, nil
	}
	return calls
}

func TestBurstDispatch_UsesCachedAdapter_AlreadyResolved(t *testing.T) {
	sessions := []Session{
		{Name: "alpha"},
		{Name: "bravo"},
	}
	ack := &spawntest.FakeAckChannel{}
	detectAdapter := &spawntest.FakeAdapter{Ack: ack}
	m := NewModelWithSessions(sessions)
	calls := wireTOCTOUResolveSeams(&m, detectAdapter, ack)
	m = resolveDetection(t, m, ghosttyIdentity())
	if *calls != 1 {
		t.Fatalf("precondition: detection must resolve exactly once, got %d", *calls)
	}
	if m.DetectUnsupported() {
		t.Fatal("precondition: the detection-time resolution must be supported (native)")
	}

	m = markTwo(t, m)
	m, cmd := pressEnter(t, m)

	if !m.BurstPending() {
		t.Fatal("a supported N≥2 Enter must dispatch the burst")
	}
	if *calls != 1 {
		t.Errorf("m.resolve called %d times, want 1 (dispatchBurst must read the cached adapter, not re-resolve)", *calls)
	}

	m = drainBatchToModel(t, m, cmd)
	if len(detectAdapter.Calls) != 1 {
		t.Fatalf("the CACHED detection-time adapter must open the one external window; OpenWindow calls = %d, want 1", len(detectAdapter.Calls))
	}
}

func TestBurstDispatch_UsesCachedAdapter_DeferredEntry(t *testing.T) {
	sessions := []Session{
		{Name: "alpha"},
		{Name: "bravo"},
	}
	ack := &spawntest.FakeAckChannel{}
	detectAdapter := &spawntest.FakeAdapter{Ack: ack}
	m := NewModelWithSessions(sessions)
	calls := wireTOCTOUResolveSeams(&m, detectAdapter, ack)
	m.detectDispatched = true

	m = markTwo(t, m)
	m, deferredCmd := pressEnter(t, m)
	if m.BurstPending() {
		t.Fatal("N≥2 Enter while detection is in flight must DEFER, not dispatch")
	}
	if *calls != 0 {
		t.Fatalf("no resolve may run while the deferred Enter awaits detection, got %d", *calls)
	}
	if deferredCmd != nil {
		t.Error("detection already in flight → no new detection cmd (nil)")
	}

	updated, cmd := m.Update(terminalDetectedMsg{identity: ghosttyIdentity()})
	m = updated.(Model)
	if !m.BurstPending() {
		t.Fatal("the terminalDetectedMsg must resolve the deferred burst (supported → dispatch)")
	}
	if *calls != 1 {
		t.Errorf("m.resolve called %d times across the deferred path, want 1 (one detection resolve, no dispatch re-resolve)", *calls)
	}

	m = drainBatchToModel(t, m, cmd)
	if len(detectAdapter.Calls) != 1 {
		t.Fatalf("the cached detection-time adapter must open the one external window; OpenWindow calls = %d, want 1", len(detectAdapter.Calls))
	}
}

func TestBurstDispatch_NilCachedAdapter_RoutesToUnsupportedNoOp(t *testing.T) {
	sessions := []Session{
		{Name: "alpha"},
		{Name: "bravo"},
	}
	ack := &spawntest.FakeAckChannel{}
	m := NewModelWithSessions(sessions)
	m.detector = &fakeDetector{identity: ghosttyIdentity()}
	m.resolve = func(spawn.Identity) (spawn.Adapter, spawn.Resolution) {
		return nil, spawn.ResolutionNative
	}
	m.sessionExists = allPresent
	m.ackChannel = ack
	m.spawnExe = func() (string, error) { return "/abs/portal", nil }
	m.spawnEnv = func() (spawn.WindowEnvironment, error) {
		return spawn.WindowEnvironment{Set: []string{"PATH=/usr/bin"}}, nil
	}

	m = resolveDetection(t, m, ghosttyIdentity())
	if m.DetectUnsupported() {
		t.Fatal("precondition: a native resolution is supported, so decideBurst must fall through to dispatchBurst")
	}

	m = markTwo(t, m)
	m, cmd := pressEnter(t, m)

	if m.BurstPending() {
		t.Error("a nil cached adapter must NOT enter burst-pending (routes to the unsupported no-op)")
	}
	if m.burstPipe != nil {
		t.Error("a nil cached adapter must construct NO burst pipe")
	}
	if isQuitCmd(cmd) {
		t.Error("the nil-adapter no-op must NOT tea.Quit")
	}
	if m.Selected() != "" {
		t.Errorf("the nil-adapter no-op must NOT self-attach; Selected() = %q", m.Selected())
	}
	if !m.MultiSelectActive() {
		t.Error("the nil-adapter no-op must stay in multi-select mode")
	}
	if m.SelectedSessionCount() != 2 {
		t.Errorf("the selection must be INTACT after the no-op; count = %d, want 2", m.SelectedSessionCount())
	}
	want := unsupportedFlashText(m.DetectedIdentity())
	if m.flashText != want {
		t.Errorf("flashText = %q, want %q (the nil-adapter guard mirrors decideBurst's unsupported no-op flash)", m.flashText, want)
	}
}
