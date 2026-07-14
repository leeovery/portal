package tui

// restore-host-terminal-windows-6-3 — N≥2 picker-burst dispatch + async spawn
// tea.Cmd + streaming message protocol.
//
// These white-box (package tui) tests drive the §5 multi-select Enter through the
// N≥2 arm and assert: the list-ordered net-N split (external = marked minus the
// trigger), the async burst opening the fake adapter once per external session in
// list order (never the trigger), By-Tag multi-tag de-dup, the cursor-but-unmarked
// exclusion, the streamed progress/complete message protocol, the detection gate
// (defer-while-in-flight and defer-then-dispatch-when-never-dispatched), and that
// the picker burst reuses the config-aware resolve seam.
//
// No t.Parallel: consistent with the rest of the tui test surface.

import (
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/spawntest"
	"github.com/leeovery/portal/internal/tmux"
)

// allPresent is a pre-flight has-session probe reporting every session present.
func allPresent(string) bool { return true }

// wireBurstSeams sets the §6 burst seams directly on a white-box model: a ghostty
// detector, a resolve seam returning the given adapter + resolution for any
// identity, the pre-flight probe, the ack channel, and the exe/PATH composition
// seams. It does NOT resolve detection — call resolveDetection for that.
func wireBurstSeams(m *Model, adapter spawn.Adapter, resolution spawn.Resolution, exists func(string) bool, ack spawn.AckChannelFull) {
	m.detector = &fakeDetector{identity: ghosttyIdentity()}
	m.resolve = func(spawn.Identity) (spawn.Adapter, spawn.Resolution) { return adapter, resolution }
	m.sessionExists = exists
	m.ackChannel = ack
	m.spawnExe = func() (string, error) { return "/abs/portal", nil }
	m.spawnGetenv = func(string) string { return "/usr/bin" }
}

// resolveDetection feeds a terminalDetectedMsg for id through Update so the model
// caches the identity + resolution (detectResolved true) via the wired resolve
// seam, mirroring the async Detect() completing.
func resolveDetection(t *testing.T, m Model, id spawn.Identity) Model {
	t.Helper()
	updated, _ := m.Update(terminalDetectedMsg{identity: id})
	rm := updated.(Model)
	if !rm.DetectResolved() {
		t.Fatal("precondition: terminalDetectedMsg must resolve detection")
	}
	return rm
}

// markRow selects the given list index and toggles it in the multi-select set
// (the mode must already be active).
func markRow(t *testing.T, m Model, index int) Model {
	t.Helper()
	m.sessionList.Select(index)
	return pressSession(t, m, pressM)
}

// spawnedSession extracts the target session from a composed attach argv (the
// element right after "attach"), so a test reads the open order from the fake
// adapter's recorded argv without depending on the argv's other fragments.
func spawnedSession(t *testing.T, argv []string) string {
	t.Helper()
	for i := 0; i+1 < len(argv); i++ {
		if argv[i] == "attach" {
			return argv[i+1]
		}
	}
	t.Fatalf("argv has no 'attach <session>' pair: %#v", argv)
	return ""
}

// countName counts occurrences of name in names.
func countName(names []string, name string) int {
	n := 0
	for _, s := range names {
		if s == name {
			n++
		}
	}
	return n
}

// sessionsFromNames builds the burst-suite's []tmux.Session from a names slice,
// stamping each with the canonical Windows: i+1 index convention. It is the single
// place that convention lives, so the four burst-model constructors cannot drift it.
func sessionsFromNames(names []string) []tmux.Session {
	sessions := make([]tmux.Session, len(names))
	for i, n := range names {
		sessions[i] = tmux.Session{Name: n, Windows: i + 1}
	}
	return sessions
}

// markedSupportedBurstModel builds a resolved-supported multi-select model with
// every named session marked: a DEFAULT confirm-all FakeAdapter + FakeAckChannel,
// the §6 burst seams wired, detection resolved to a ghostty identity, multi-select
// entered, and every row marked top-to-bottom. It is the shared
// wire→resolve→enter→mark-all prefix the confirm-all burst-model constructors layer
// their distinct tail onto (force burstPending / precondition check / …). A
// constructor needing a non-default adapter (e.g. an all-false Confirm) wires its own
// seams and only reuses sessionsFromNames.
func markedSupportedBurstModel(t *testing.T, names []string) (Model, *spawntest.FakeAdapter, *spawntest.FakeAckChannel) {
	t.Helper()
	ack := &spawntest.FakeAckChannel{}
	adapter := &spawntest.FakeAdapter{Ack: ack}
	m := NewModelWithSessions(sessionsFromNames(names))
	wireBurstSeams(&m, adapter, spawn.ResolutionNative, allPresent, ack)
	m = resolveDetection(t, m, ghosttyIdentity())
	m = pressSession(t, m, pressM)
	for i := range names {
		m = markRow(t, m, i)
	}
	return m, adapter, ack
}

// TestBurstDispatch_OpensExternalInListOrder is the core dispatch assertion: N=3
// marked on a resolved-supported terminal enters burst-pending and opens the N-1
// external sessions in list order (top-to-bottom), never the trigger, streaming to
// completion and self-cleaning the batch markers.
func TestBurstDispatch_OpensExternalInListOrder(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 1},
		{Name: "bravo", Windows: 2},
		{Name: "charlie", Windows: 3},
	}
	ack := &spawntest.FakeAckChannel{}
	adapter := &spawntest.FakeAdapter{Ack: ack}
	m := NewModelWithSessions(sessions)
	wireBurstSeams(&m, adapter, spawn.ResolutionNative, allPresent, ack)
	m = resolveDetection(t, m, ghosttyIdentity())

	// Enter multi-select and mark all three, top-to-bottom.
	m = pressSession(t, m, pressM)
	m = markRow(t, m, 0)
	m = markRow(t, m, 1)
	m = markRow(t, m, 2)
	if m.SelectedSessionCount() != 3 {
		t.Fatalf("precondition: 3 marked, got %d", m.SelectedSessionCount())
	}

	m, cmd := pressEnter(t, m)

	// Dispatch-time assertions (before draining the async pipe).
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

	// Drive the async pipe to the terminal event (NOT past it: the §6-4 full-success
	// arm self-attaches + resets the burst lifecycle state, so the post-reset model
	// can no longer witness the mid-burst counters).
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
	// Regression (§6-3 review fix): burstTotal is N (recorded once at dispatch) and
	// must stay N for the WHOLE burst — the streamed progress events carry the
	// external count (N-1) and must NOT overwrite it. Sampled AT the terminal event,
	// BEFORE the §6-4 full-success reset, so a progress-driven overwrite would
	// surface here as N-1.
	if mBefore.BurstTotal() != 3 {
		t.Errorf("BurstTotal() = %d at the terminal event, want 3 (N must stay N across the burst, not be overwritten by the N-1 external count)", mBefore.BurstTotal())
	}
	if len(ack.Cleaned) != 1 {
		t.Errorf("the ack channel must self-clean the batch exactly once before the terminal event, got %d Clean calls", len(ack.Cleaned))
	}

	// Applying the terminal full-success message self-attaches to the trigger and
	// clears burst-pending.
	updated, _ := mBefore.Update(term)
	m = updated.(Model)
	if m.BurstPending() {
		t.Error("burst must clear pending once the terminal spawnCompleteMsg lands")
	}
}

// TestBurstDispatch_MultiTagDedup covers the By-Tag de-dup: a multi-tag session
// marked once spans several list rows but appears exactly once in the open order,
// so N marked names yield BurstTotal==N (not the row count).
func TestBurstDispatch_MultiTagDedup(t *testing.T) {
	dir := t.TempDir()
	dir2 := t.TempDir()
	projects := []project.Project{
		{Path: dir, Name: "Portal", Tags: []string{"infra", "work"}},
		{Path: dir2, Name: "Other", Tags: []string{"work"}},
	}
	sessions := []tmux.Session{
		{Name: "portal-abc", Dir: dir},
		{Name: "other-xyz", Dir: dir2},
	}
	m := newRebuildTestModel(prefs.ModeByTag, sessions, projects)
	m.rebuildSessionList()

	ack := &spawntest.FakeAckChannel{}
	adapter := &spawntest.FakeAdapter{Ack: ack}
	wireBurstSeams(&m, adapter, spawn.ResolutionNative, allPresent, ack)
	m = resolveDetection(t, m, ghosttyIdentity())

	// portal-abc (2 tags) spans 2 rows; other-xyz (1 tag) spans 1.
	portalRows := 0
	for _, it := range m.sessionList.Items() {
		if si, ok := it.(SessionItem); ok && si.Session.Name == "portal-abc" {
			portalRows++
		}
	}
	if portalRows != 2 {
		t.Fatalf("precondition: portal-abc must span 2 By-Tag rows, got %d", portalRows)
	}

	// Mark BOTH sessions (portal-abc via its first row).
	m = pressSession(t, m, pressM)
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
	// Each marked name appears exactly once across the open order (external + trigger).
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

// TestBurstDispatch_CursorUnmarkedNeverOpened covers the cursor-irrelevance rule:
// a highlighted-but-unmarked row is never in the open set, even when it sits under
// the cursor at Enter time.
func TestBurstDispatch_CursorUnmarkedNeverOpened(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 1},
		{Name: "bravo", Windows: 2},
		{Name: "charlie", Windows: 3},
	}
	ack := &spawntest.FakeAckChannel{}
	adapter := &spawntest.FakeAdapter{Ack: ack}
	m := NewModelWithSessions(sessions)
	wireBurstSeams(&m, adapter, spawn.ResolutionNative, allPresent, ack)
	m = resolveDetection(t, m, ghosttyIdentity())

	// Mark alpha (0) and charlie (2); leave the cursor on the UNMARKED bravo (1).
	m = pressSession(t, m, pressM)
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

// TestBurstDispatch_StreamsProgressThenComplete pins the message protocol: the
// goroutine streams one spawnProgressMsg per external window followed by a single
// terminal spawnCompleteMsg; the receiver is re-issued on progress (follow cmd
// non-nil) and stops on the terminal event (follow cmd nil).
func TestBurstDispatch_StreamsProgressThenComplete(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 1},
		{Name: "bravo", Windows: 2},
		{Name: "charlie", Windows: 3},
	}
	ack := &spawntest.FakeAckChannel{}
	adapter := &spawntest.FakeAdapter{Ack: ack}
	m := NewModelWithSessions(sessions)
	wireBurstSeams(&m, adapter, spawn.ResolutionNative, allPresent, ack)
	m = resolveDetection(t, m, ghosttyIdentity())

	m = pressSession(t, m, pressM)
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
			// §6-4: full success self-attaches — the terminal complete returns
			// tea.Quit, NOT a receiver re-issue, so the receive loop stops here (no
			// next spawnProgressMsg is pulled).
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

// TestBurstDispatch_DefersWhileDetectionInFlight covers the detection gate: an
// N≥2 Enter while detection is dispatched-but-unresolved DEFERS — no window opens
// and no burst dispatches — until the terminalDetectedMsg lands, which then
// branches to the burst (supported).
func TestBurstDispatch_DefersWhileDetectionInFlight(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 1},
		{Name: "bravo", Windows: 2},
	}
	ack := &spawntest.FakeAckChannel{}
	adapter := &spawntest.FakeAdapter{Ack: ack}
	m := NewModelWithSessions(sessions)
	wireBurstSeams(&m, adapter, spawn.ResolutionNative, allPresent, ack)
	// Detection dispatched but not yet resolved (in-flight).
	m.detectDispatched = true

	m = pressSession(t, m, pressM)
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

	// Detection resolves → the deferred burst branches and dispatches.
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

// TestBurstDispatch_DetectionNeverDispatched_DefersThenResolves covers the 6-1
// edge: an N≥2 Enter when detection was NEVER dispatched must ALSO kick off
// detection (else it would defer forever), then resolve and dispatch the burst
// without hanging.
func TestBurstDispatch_DetectionNeverDispatched_DefersThenResolves(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 1},
		{Name: "bravo", Windows: 2},
	}
	ack := &spawntest.FakeAckChannel{}
	adapter := &spawntest.FakeAdapter{Ack: ack}
	m := NewModelWithSessions(sessions)
	wireBurstSeams(&m, adapter, spawn.ResolutionNative, allPresent, ack)
	// Detection NEVER dispatched: detectDispatched=false, detectResolved=false.
	if m.DetectDispatched() || m.DetectResolved() {
		t.Fatal("precondition: detection must be neither dispatched nor resolved")
	}

	m = pressSession(t, m, pressM)
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

	// Running the detection cmd yields the terminalDetectedMsg (it does not hang).
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

// TestBurstDispatch_SplitDerivesFromSplitNetN is the cross-caller drift guard for
// the net-N split: the picker's dispatched BurstExternal/BurstTrigger for a shared
// fixture are byte-identical to spawn.SplitNetN's output — the SAME single
// computation the CLI's runSpawn derives its split through — so the "net N, never
// N+1" split cannot diverge between the two callers.
func TestBurstDispatch_SplitDerivesFromSplitNetN(t *testing.T) {
	fixture := []string{"alpha", "bravo", "charlie"}
	m, _, _ := markedSupportedBurstModel(t, fixture)

	m, cmd := pressEnter(t, m)

	if !m.BurstPending() {
		t.Fatal("precondition: N≥2 Enter on a resolved-supported terminal must enter burst-pending")
	}

	// Recorded at dispatch (before any terminal reset): the picker's split must equal
	// the shared helper's output for the identical fixture.
	wantExternal, wantTrigger := spawn.SplitNetN(fixture)
	if got := m.BurstExternal(); !slices.Equal(got, wantExternal) {
		t.Errorf("BurstExternal() = %v, want %v (must derive from spawn.SplitNetN)", got, wantExternal)
	}
	if got := m.BurstTrigger(); got != wantTrigger {
		t.Errorf("BurstTrigger() = %q, want %q (must derive from spawn.SplitNetN)", got, wantTrigger)
	}

	// Drain the async pipe so the burst goroutine completes (no lingering goroutine).
	drainBatchToModel(t, m, cmd)
}

// TestBurstDispatch_ConfigResolveUsesConfigAdapter is the regression guard for the
// picker's config-aware resolve seam: an identity matching a config entry resolves
// to the config adapter + ResolutionConfig, and the picker burst opens its windows
// through THAT adapter.
func TestBurstDispatch_ConfigResolveUsesConfigAdapter(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 1},
		{Name: "bravo", Windows: 2},
	}
	ack := &spawntest.FakeAckChannel{}
	configAdapter := &spawntest.FakeAdapter{Ack: ack}
	m := NewModelWithSessions(sessions)
	// The injected resolve returns the config adapter + ResolutionConfig, mirroring a
	// terminals.json match in the picker.
	wireBurstSeams(&m, configAdapter, spawn.ResolutionConfig, allPresent, ack)
	m = resolveDetection(t, m, ghosttyIdentity())
	if m.DetectedResolution() != spawn.ResolutionConfig {
		t.Fatalf("precondition: resolution must cache as config, got %q", m.DetectedResolution())
	}

	m = pressSession(t, m, pressM)
	m = markRow(t, m, 0)
	m = markRow(t, m, 1)

	m, cmd := pressEnter(t, m)
	m = drainBatchToModel(t, m, cmd)

	if len(configAdapter.Calls) != 1 {
		t.Fatalf("the config-matched adapter must open the external window; Calls=%d, want 1 (config adapter used in the picker burst)", len(configAdapter.Calls))
	}
}
