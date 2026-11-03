package tui

import (
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/spawntest"
)

func allPresent(string) bool { return true }

func wireBurstSeams(m *Model, adapter spawn.Adapter, resolution spawn.Resolution, exists func(string) bool, ack spawn.AckChannelFull) {
	m.detector = &fakeDetector{identity: ghosttyIdentity()}
	m.resolve = func(spawn.Identity) (spawn.Adapter, spawn.Resolution) { return adapter, resolution }
	m.sessionExists = exists
	m.ackChannel = ack
	m.spawnExe = func() (string, error) { return "/abs/portal", nil }
	m.spawnGetenv = func(string) string { return "/usr/bin" }
}

func resolveDetection(t *testing.T, m Model, id spawn.Identity) Model {
	t.Helper()
	updated, _ := m.Update(terminalDetectedMsg{identity: id})
	rm := updated.(Model)
	if !rm.DetectResolved() {
		t.Fatal("precondition: terminalDetectedMsg must resolve detection")
	}
	return rm
}

func markRow(t *testing.T, m Model, index int) Model {
	t.Helper()
	m.sessionList.Select(index)
	return pressSession(t, m, pressM)
}

func spawnedSession(t *testing.T, argv []string) string {
	t.Helper()
	for i := 0; i+1 < len(argv); i++ {
		if argv[i] == "--session" {
			return argv[i+1]
		}
	}
	t.Fatalf("argv has no '--session <name>' pair: %#v", argv)
	return ""
}

func countName(names []string, name string) int {
	n := 0
	for _, s := range names {
		if s == name {
			n++
		}
	}
	return n
}

func sessionsFromNames(names []string) []Session {
	sessions := make([]Session, len(names))
	for i, n := range names {
		sessions[i] = Session{Name: n}
	}
	return sessions
}

func markedSupportedBurstModel(t *testing.T, names []string) (Model, *spawntest.FakeAdapter, *spawntest.FakeAckChannel) {
	t.Helper()
	ack := &spawntest.FakeAckChannel{}
	adapter := &spawntest.FakeAdapter{Ack: ack}
	m := NewModelWithSessions(sessionsFromNames(names))
	wireBurstSeams(&m, adapter, spawn.ResolutionNative, allPresent, ack)
	m = resolveDetection(t, m, ghosttyIdentity())
	m = enterMultiSelectEmpty(t, m)
	for i := range names {
		m = markRow(t, m, i)
	}
	return m, adapter, ack
}

func TestBurstDispatch_OpensExternalInListOrder(t *testing.T) {
	sessions := []Session{
		{Name: "alpha"},
		{Name: "bravo"},
		{Name: "charlie"},
	}
	ack := &spawntest.FakeAckChannel{}
	adapter := &spawntest.FakeAdapter{Ack: ack}
	m := NewModelWithSessions(sessions)
	wireBurstSeams(&m, adapter, spawn.ResolutionNative, allPresent, ack)
	m = resolveDetection(t, m, ghosttyIdentity())

	m = enterMultiSelectEmpty(t, m)
	m = markRow(t, m, 0)
	m = markRow(t, m, 1)
	m = markRow(t, m, 2)
	if m.SelectedSessionCount() != 3 {
		t.Fatalf("precondition: 3 marked, got %d", m.SelectedSessionCount())
	}

	m, cmd := pressEnter(t, m)

	if !m.BurstPending() {
		t.Fatal("N≥2 Enter on a resolved-supported terminal must enter burst-pending")
	}
	if m.BurstTotal() != 3 {
		t.Errorf("BurstTotal() = %d, want 3 (N incl. the self-attach target)", m.BurstTotal())
	}
	if got := m.BurstTrigger(); got != "charlie" {
		t.Errorf("BurstTrigger() = %q, want charlie (list-order last)", got)
	}
	if got := m.BurstExternal(); !slices.Equal(got, []string{"alpha", "bravo"}) {
		t.Errorf("BurstExternal() = %v, want [alpha bravo] (net-N: marked minus trigger)", got)
	}

	mBefore, term := driveBurstToTerminal(t, m, cmd)

	if len(adapter.Calls) != 2 {
		t.Fatalf("OpenWindow called %d times, want 2 (N-1 external, never the trigger)", len(adapter.Calls))
	}
	for i, want := range []string{"alpha", "bravo"} {
		if got := spawnedSession(t, adapter.Calls[i]); got != want {
			t.Errorf("OpenWindow[%d] session = %q, want %q (list order)", i, got, want)
		}
	}
	for _, call := range adapter.Calls {
		if spawnedSession(t, call) == "charlie" {
			t.Error("the trigger (charlie) must NEVER be opened as an external window")
		}
	}
	if mBefore.BurstTotal() != 3 {
		t.Errorf("BurstTotal() = %d at the terminal event, want 3 (N must stay N across the burst, not be overwritten by the N-1 external count)", mBefore.BurstTotal())
	}
	if len(ack.Cleaned) != 1 {
		t.Errorf("the ack channel must self-clean the batch exactly once before the terminal event, got %d Clean calls", len(ack.Cleaned))
	}

	updated, _ := mBefore.Update(term)
	m = updated.(Model)
	if m.BurstPending() {
		t.Error("burst must clear pending once the terminal spawnCompleteMsg lands")
	}
}

func TestBurstDispatch_MultiTagDedup(t *testing.T) {
	dir := t.TempDir()
	dir2 := t.TempDir()
	projects := []project.Project{
		{Path: dir, Name: "Portal", Tags: []string{"infra", "work"}},
		{Path: dir2, Name: "Other", Tags: []string{"work"}},
	}
	sessions := []Session{
		{Name: "portal-abc", Dir: dir},
		{Name: "other-xyz", Dir: dir2},
	}
	m := newRebuildTestModel(t, prefs.ModeByTag, sessions, projects)
	m.rebuildSessionList()

	ack := &spawntest.FakeAckChannel{}
	adapter := &spawntest.FakeAdapter{Ack: ack}
	wireBurstSeams(&m, adapter, spawn.ResolutionNative, allPresent, ack)
	m = resolveDetection(t, m, ghosttyIdentity())

	portalRows := 0
	for _, it := range m.sessionList.Items() {
		if si, ok := it.(SessionItem); ok && si.Session.Name == "portal-abc" {
			portalRows++
		}
	}
	if portalRows != 2 {
		t.Fatalf("precondition: portal-abc must span 2 By-Tag rows, got %d", portalRows)
	}

	m = enterMultiSelectEmpty(t, m)
	rows := sessionRowIndices(m.sessionList.Items())
	for _, idx := range rows {
		if si, _ := m.sessionList.Items()[idx].(SessionItem); si.Session.Name == "portal-abc" {
			m = markRow(t, m, idx)
			break
		}
	}
	for _, idx := range rows {
		if si, _ := m.sessionList.Items()[idx].(SessionItem); si.Session.Name == "other-xyz" {
			m = markRow(t, m, idx)
			break
		}
	}
	if m.SelectedSessionCount() != 2 {
		t.Fatalf("precondition: exactly 2 sessions marked (keyed by name), got %d", m.SelectedSessionCount())
	}

	m, cmd := pressEnter(t, m)

	if m.BurstTotal() != 2 {
		t.Errorf("BurstTotal() = %d, want 2 (a multi-tag session de-dupes to one)", m.BurstTotal())
	}
	openOrder := append(slices.Clone(m.BurstExternal()), m.BurstTrigger())
	if got := countName(openOrder, "portal-abc"); got != 1 {
		t.Errorf("portal-abc appears %d times in the open order, want 1 (de-duped at its first list position)", got)
	}
	if got := countName(openOrder, "other-xyz"); got != 1 {
		t.Errorf("other-xyz appears %d times in the open order, want 1", got)
	}

	m = drainBatchToModel(t, m, cmd)

	if len(adapter.Calls) != 1 {
		t.Fatalf("OpenWindow called %d times, want 1 (2 marked → 1 external, net-N)", len(adapter.Calls))
	}
	if got := spawnedSession(t, adapter.Calls[0]); got == m.BurstTrigger() {
		t.Errorf("the opened external window %q must NOT be the self-attach trigger %q", got, m.BurstTrigger())
	}
}

func TestBurstDispatch_CursorUnmarkedNeverOpened(t *testing.T) {
	sessions := []Session{
		{Name: "alpha"},
		{Name: "bravo"},
		{Name: "charlie"},
	}
	ack := &spawntest.FakeAckChannel{}
	adapter := &spawntest.FakeAdapter{Ack: ack}
	m := NewModelWithSessions(sessions)
	wireBurstSeams(&m, adapter, spawn.ResolutionNative, allPresent, ack)
	m = resolveDetection(t, m, ghosttyIdentity())

	m = enterMultiSelectEmpty(t, m)
	m = markRow(t, m, 0)
	m = markRow(t, m, 2)
	m.sessionList.Select(1)
	if si, ok := m.selectedSessionItem(); !ok || si.Session.Name != "bravo" {
		t.Fatalf("precondition: cursor must rest on the unmarked bravo row")
	}

	m, cmd := pressEnter(t, m)

	if got := m.BurstExternal(); !slices.Equal(got, []string{"alpha"}) {
		t.Errorf("BurstExternal() = %v, want [alpha] (bravo is unmarked, charlie is the trigger)", got)
	}
	if got := m.BurstTrigger(); got != "charlie" {
		t.Errorf("BurstTrigger() = %q, want charlie", got)
	}

	m = drainBatchToModel(t, m, cmd)

	if len(adapter.Calls) != 1 {
		t.Fatalf("OpenWindow called %d times, want 1", len(adapter.Calls))
	}
	for _, call := range adapter.Calls {
		if spawnedSession(t, call) == "bravo" {
			t.Error("the cursor-but-unmarked bravo row must NEVER be opened")
		}
	}
}

func TestBurstDispatch_StreamsProgressThenComplete(t *testing.T) {
	sessions := []Session{
		{Name: "alpha"},
		{Name: "bravo"},
		{Name: "charlie"},
	}
	ack := &spawntest.FakeAckChannel{}
	adapter := &spawntest.FakeAdapter{Ack: ack}
	m := NewModelWithSessions(sessions)
	wireBurstSeams(&m, adapter, spawn.ResolutionNative, allPresent, ack)
	m = resolveDetection(t, m, ghosttyIdentity())

	m = enterMultiSelectEmpty(t, m)
	m = markRow(t, m, 0)
	m = markRow(t, m, 1)
	m = markRow(t, m, 2)

	m, cmd := pressEnter(t, m)

	var seq []string
	for steps := 0; cmd != nil && steps < 20; steps++ {
		msg := cmd()
		var follow tea.Cmd
		switch msg.(type) {
		case spawnProgressMsg:
			seq = append(seq, "progress")
			updated, f := m.Update(msg)
			m = updated.(Model)
			follow = f
			if follow == nil {
				t.Error("the receiver must be RE-ISSUED after a progress event")
			}
		case spawnCompleteMsg:
			seq = append(seq, "complete")
			updated, f := m.Update(msg)
			m = updated.(Model)
			if !isQuitCmd(f) {
				t.Error("the terminal complete event must return tea.Quit (self-attach), not a receiver re-issue")
			}
			follow = nil
		default:
			t.Fatalf("unexpected burst message %T", msg)
		}
		cmd = follow
	}

	want := []string{"progress", "progress", "complete"}
	if !slices.Equal(seq, want) {
		t.Errorf("burst message stream = %v, want %v (one progress per external window + one terminal)", seq, want)
	}
}

func TestBurstDispatch_DefersWhileDetectionInFlight(t *testing.T) {
	sessions := []Session{
		{Name: "alpha"},
		{Name: "bravo"},
	}
	ack := &spawntest.FakeAckChannel{}
	adapter := &spawntest.FakeAdapter{Ack: ack}
	m := NewModelWithSessions(sessions)
	wireBurstSeams(&m, adapter, spawn.ResolutionNative, allPresent, ack)
	m.detectDispatched = true

	m = enterMultiSelectEmpty(t, m)
	m = markRow(t, m, 0)
	m = markRow(t, m, 1)

	m, cmd := pressEnter(t, m)

	if m.BurstPending() {
		t.Fatal("N≥2 Enter while detection is in flight must DEFER, not dispatch the burst")
	}
	if len(adapter.Calls) != 0 {
		t.Fatalf("no window may open while detection is in flight, got %d", len(adapter.Calls))
	}
	if cmd != nil {
		t.Error("no new detection dispatch when detection is already in flight (cmd must be nil)")
	}

	updated, cmd2 := m.Update(terminalDetectedMsg{identity: ghosttyIdentity()})
	m = updated.(Model)
	if !m.BurstPending() {
		t.Fatal("the terminalDetectedMsg must resolve the deferred burst (supported → dispatch)")
	}

	m = drainBatchToModel(t, m, cmd2)
	if len(adapter.Calls) != 1 {
		t.Fatalf("OpenWindow called %d times, want 1 (external = [alpha], trigger = bravo)", len(adapter.Calls))
	}
	if got := spawnedSession(t, adapter.Calls[0]); got != "alpha" {
		t.Errorf("deferred burst opened %q, want alpha", got)
	}
}

func TestBurstDispatch_DetectionNeverDispatched_DefersThenResolves(t *testing.T) {
	sessions := []Session{
		{Name: "alpha"},
		{Name: "bravo"},
	}
	ack := &spawntest.FakeAckChannel{}
	adapter := &spawntest.FakeAdapter{Ack: ack}
	m := NewModelWithSessions(sessions)
	wireBurstSeams(&m, adapter, spawn.ResolutionNative, allPresent, ack)
	if m.DetectDispatched() || m.DetectResolved() {
		t.Fatal("precondition: detection must be neither dispatched nor resolved")
	}

	m = enterMultiSelectEmpty(t, m)
	m = markRow(t, m, 0)
	m = markRow(t, m, 1)

	m, cmd := pressEnter(t, m)

	if m.BurstPending() {
		t.Fatal("Enter with detection never dispatched must DEFER, not dispatch the burst")
	}
	if !m.DetectDispatched() {
		t.Fatal("Enter with detection never dispatched must ALSO dispatch detection so the defer can resolve")
	}
	if cmd == nil {
		t.Fatal("Enter with detection never dispatched must return the detection cmd")
	}

	msg := cmd()
	detMsg, ok := msg.(terminalDetectedMsg)
	if !ok {
		t.Fatalf("the detection cmd must produce a terminalDetectedMsg, got %T", msg)
	}
	updated, cmd2 := m.Update(detMsg)
	m = updated.(Model)
	if !m.BurstPending() {
		t.Fatal("resolving the newly-dispatched detection must dispatch the deferred burst (not hang)")
	}

	m = drainBatchToModel(t, m, cmd2)
	if len(adapter.Calls) != 1 {
		t.Fatalf("OpenWindow called %d times, want 1", len(adapter.Calls))
	}
}

func TestBurstDispatch_SplitDerivesFromSplitNetN(t *testing.T) {
	fixture := []string{"alpha", "bravo", "charlie"}
	m, _, _ := markedSupportedBurstModel(t, fixture)

	m, cmd := pressEnter(t, m)

	if !m.BurstPending() {
		t.Fatal("precondition: N≥2 Enter on a resolved-supported terminal must enter burst-pending")
	}

	wantExternal, wantTrigger := spawn.SplitNetN(fixture)
	if got := m.BurstExternal(); !slices.Equal(got, wantExternal) {
		t.Errorf("BurstExternal() = %v, want %v (must derive from spawn.SplitNetN)", got, wantExternal)
	}
	if got := m.BurstTrigger(); got != wantTrigger {
		t.Errorf("BurstTrigger() = %q, want %q (must derive from spawn.SplitNetN)", got, wantTrigger)
	}

	drainBatchToModel(t, m, cmd)
}

func TestBurstDispatch_ConfigResolveUsesConfigAdapter(t *testing.T) {
	sessions := []Session{
		{Name: "alpha"},
		{Name: "bravo"},
	}
	ack := &spawntest.FakeAckChannel{}
	configAdapter := &spawntest.FakeAdapter{Ack: ack}
	m := NewModelWithSessions(sessions)
	wireBurstSeams(&m, configAdapter, spawn.ResolutionConfig, allPresent, ack)
	m = resolveDetection(t, m, ghosttyIdentity())
	if m.DetectedResolution() != spawn.ResolutionConfig {
		t.Fatalf("precondition: resolution must cache as config, got %q", m.DetectedResolution())
	}

	m = enterMultiSelectEmpty(t, m)
	m = markRow(t, m, 0)
	m = markRow(t, m, 1)

	m, cmd := pressEnter(t, m)
	m = drainBatchToModel(t, m, cmd)

	if len(configAdapter.Calls) != 1 {
		t.Fatalf("the config-matched adapter must open the external window; Calls=%d, want 1 (config adapter used in the picker burst)", len(configAdapter.Calls))
	}
}
