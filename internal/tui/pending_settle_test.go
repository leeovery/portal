package tui

import (
	"errors"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/tmux"
)

// markerReaderStub answers each read with the next step's marker keys, holding
// on the last step once the script runs out. With err set, the read numbered
// errStep fails with it instead.
type markerReaderStub struct {
	steps   [][]string
	err     error
	errStep int
	calls   int
	log     *[]string
}

func (r *markerReaderStub) ListSkeletonMarkers() (map[string]struct{}, error) {
	idx := r.calls
	r.calls++
	if r.log != nil {
		*r.log = append(*r.log, "markers")
	}
	if r.err != nil && idx == r.errStep {
		return nil, r.err
	}
	set := map[string]struct{}{}
	for _, k := range r.steps[min(idx, len(r.steps)-1)] {
		set[k] = struct{}{}
	}
	return set, nil
}

var settleSessions = []tmux.Session{
	{Name: "alpha", Windows: 1},
	{Name: "bravo", Windows: 1},
	{Name: "charlie", Windows: 1},
}

const generousSettleBound = time.Minute

func coldSettleModel(lister SessionLister, pending PendingResumeReader, markers SkeletonMarkerReader, bound time.Duration) Model {
	return Build(Deps{
		Lister:             lister,
		PendingReader:      pending,
		SkeletonMarkers:    markers,
		PendingSettleBound: bound,
		ServerStarted:      true,
		ProgressReceiver:   func() tea.Msg { return nil },
	})
}

// driveToPostRestoreRefetch dismisses the loading gate and applies the
// post-restore refetch, returning whatever the refetch's handling scheduled
// without running it.
func driveToPostRestoreRefetch(t *testing.T, m Model) (Model, tea.Cmd) {
	t.Helper()
	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model, _ = model.Update(SessionsMsg{})
	model, _ = model.Update(LoadingMinElapsedMsg{})
	model, completeCmd := model.Update(BootstrapCompleteMsg{})
	cur := model.(Model)
	if cur.ActivePage() != PageSessions {
		t.Fatalf("setup invariant: expected PageSessions after min+complete, got %v", cur.ActivePage())
	}
	var refetch SessionsMsg
	found := false
	for _, msg := range drainAll(completeCmd) {
		if sm, ok := msg.(SessionsMsg); ok {
			refetch = sm
			found = true
		}
	}
	if !found {
		t.Fatal("setup invariant: the cold route issued no post-restore refetch")
	}
	updated, next := cur.Update(refetch)
	return updated.(Model), next
}

// settleStep lets one scheduled re-read land: the tick fires, its read runs,
// and the reading is applied. It returns what the reading scheduled next.
func settleStep(t *testing.T, m Model) (Model, tea.Cmd) {
	t.Helper()
	updated, readCmd := m.Update(pendingSettleTickMsg{})
	if readCmd == nil {
		t.Fatal("a settle tick scheduled no read")
	}
	updated, next := updated.(Model).Update(readCmd())
	return updated.(Model), next
}

func TestPendingSettle_ColdRoute_GainsTheDotWithNoKeypressOnceTheHelperMarksThePane(t *testing.T) {
	lister := &stepListerStub{steps: [][]tmux.Session{settleSessions}}
	pending := &pendingReaderStub{steps: [][]string{{}, {"alpha"}}}
	markers := &markerReaderStub{steps: [][]string{{}}}
	m := coldSettleModel(lister, pending, markers, generousSettleBound)

	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model, _ = model.Update(SessionsMsg{})
	model, _ = model.Update(LoadingMinElapsedMsg{})
	model, completeCmd := model.Update(BootstrapCompleteMsg{})
	final := drainBatchToModel(t, model.(Model), completeCmd)

	assertDots(t, final, map[string]bool{"alpha": true, "bravo": false, "charlie": false})
}

func TestPendingSettle_ColdRoute_EachPaneGainsItsDotAsItIsMarked(t *testing.T) {
	lister := &stepListerStub{steps: [][]tmux.Session{settleSessions}}
	pending := &pendingReaderStub{steps: [][]string{{}, {"alpha"}, {"alpha", "bravo"}}}
	markers := &markerReaderStub{steps: [][]string{{"bravo:0.0"}, {}}}
	m := coldSettleModel(lister, pending, markers, generousSettleBound)

	m, next := driveToPostRestoreRefetch(t, m)
	assertDots(t, m, map[string]bool{"alpha": false, "bravo": false, "charlie": false})
	if next == nil {
		t.Fatal("the post-restore refetch scheduled no re-read")
	}

	m, next = settleStep(t, m)
	assertDots(t, m, map[string]bool{"alpha": true, "bravo": false, "charlie": false})
	if next == nil {
		t.Fatal("a reading with a marker remaining scheduled no further re-read")
	}

	m, next = settleStep(t, m)
	assertDots(t, m, map[string]bool{"alpha": true, "bravo": true, "charlie": false})
	if next != nil {
		t.Error("a reading with no marker remaining scheduled a further re-read")
	}
}

func TestPendingSettle_ColdRoute_SettlesOnEveryDotWhenTheLastMarkerClearsBeforeARead(t *testing.T) {
	tests := []struct {
		name    string
		pending [][]string
		markers [][]string
		steps   int
	}{
		{
			name:    "between the refetch and the first re-read",
			pending: [][]string{{}, {"alpha", "charlie"}},
			markers: [][]string{{}},
			steps:   1,
		},
		{
			name:    "between two re-reads",
			pending: [][]string{{}, {"alpha"}, {"alpha", "charlie"}},
			markers: [][]string{{"charlie:0.0"}, {}},
			steps:   2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var log []string
			lister := &stepListerStub{steps: [][]tmux.Session{settleSessions}}
			pending := &pendingReaderStub{steps: tt.pending, log: &log}
			markers := &markerReaderStub{steps: tt.markers, log: &log}
			m := coldSettleModel(lister, pending, markers, generousSettleBound)

			m, _ = driveToPostRestoreRefetch(t, m)
			var next tea.Cmd
			for range tt.steps {
				m, next = settleStep(t, m)
			}

			if next != nil {
				t.Error("the picker kept re-reading after finding no marker")
			}
			assertDots(t, m, map[string]bool{"alpha": true, "bravo": false, "charlie": true})
			want := []string{"pending"}
			for range tt.steps {
				want = append(want, "markers", "pending")
			}
			if !slices.Equal(log, want) {
				t.Errorf("reads = %v, want %v (each marker read ahead of its pending read)", log, want)
			}
		})
	}
}

func TestPendingSettle_ColdRoute_AFailedMarkerReadCountsAsMarkersRemaining(t *testing.T) {
	lister := &stepListerStub{steps: [][]tmux.Session{settleSessions}}
	pending := &pendingReaderStub{steps: [][]string{{}, {}, {"alpha"}}}
	markers := &markerReaderStub{steps: [][]string{{}}, err: errors.New("show-options: server busy"), errStep: 0}
	m := coldSettleModel(lister, pending, markers, generousSettleBound)

	m, _ = driveToPostRestoreRefetch(t, m)

	m, next := settleStep(t, m)
	if next == nil {
		t.Fatal("a failed marker read ended the settle, want it to schedule a further re-read")
	}
	assertDots(t, m, map[string]bool{"alpha": false, "bravo": false, "charlie": false})

	m, next = settleStep(t, m)
	if next != nil {
		t.Error("a reading with no marker remaining scheduled a further re-read")
	}
	assertDots(t, m, map[string]bool{"alpha": true, "bravo": false, "charlie": false})
	if markers.calls != 2 || pending.calls != 3 {
		t.Errorf("reads: markers = %d, pending = %d, want 2 and 3", markers.calls, pending.calls)
	}
}

func TestPendingSettle_ColdRoute_TakesNoFurtherReadOnceNoMarkerRemains(t *testing.T) {
	lister := &stepListerStub{steps: [][]tmux.Session{settleSessions}}
	pending := &pendingReaderStub{steps: [][]string{{}, {"alpha"}}}
	markers := &markerReaderStub{steps: [][]string{{}}}
	m := coldSettleModel(lister, pending, markers, generousSettleBound)

	m, _ = driveToPostRestoreRefetch(t, m)
	m, next := settleStep(t, m)
	if next != nil {
		t.Fatal("a reading with no marker remaining scheduled a further re-read")
	}

	updated, stray := m.Update(pendingSettleTickMsg{})
	m = updated.(Model)
	if stray != nil {
		t.Error("a tick arriving after the settle ended scheduled a read")
	}
	if markers.calls != 1 || pending.calls != 2 {
		t.Errorf("reads after settling: markers = %d, pending = %d, want 1 and 2", markers.calls, pending.calls)
	}

	m.sessionKiller = &killerStub{}
	m.sessionList.Select(0)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	updated, killCmd := updated.(Model).Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	_, afterKill := updated.(Model).Update(killCmd())
	if afterKill != nil {
		for _, msg := range drainAll(afterKill) {
			if _, ok := msg.(pendingSettleTickMsg); ok {
				t.Error("a kill's re-read restarted the settle")
			}
		}
	}
	if markers.calls != 1 || pending.calls != 3 {
		t.Errorf("reads after a kill: markers = %d, pending = %d, want 1 and 3", markers.calls, pending.calls)
	}
}

func TestPendingSettle_ColdRoute_StopsAtTheBoundWhenAMarkerNeverClears(t *testing.T) {
	lister := &stepListerStub{steps: [][]tmux.Session{settleSessions}}
	pending := &pendingReaderStub{steps: [][]string{{}, {"bravo"}}}
	markers := &markerReaderStub{steps: [][]string{{"alpha:0.0"}}}
	m := coldSettleModel(lister, pending, markers, time.Nanosecond)

	m, _ = driveToPostRestoreRefetch(t, m)
	time.Sleep(time.Millisecond)
	m, next := settleStep(t, m)

	if next != nil {
		t.Error("the picker kept re-reading past the bound")
	}
	assertDots(t, m, map[string]bool{"alpha": false, "bravo": true, "charlie": false})

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(Model)
	if m.ActivePage() != PageSessions || m.sessionList.Index() != 1 {
		t.Errorf("picker not usable after the bound: page = %v, cursor = %d", m.ActivePage(), m.sessionList.Index())
	}
	assertDots(t, m, map[string]bool{"alpha": false, "bravo": true, "charlie": false})
}

func TestPendingSettle_ColdRoute_ReReadChangesOnlyWhichRowsCarryTheDot(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, m Model) Model
	}{
		{
			name: "cursor moved",
			setup: func(t *testing.T, m Model) Model {
				updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
				updated, _ = updated.(Model).Update(tea.KeyPressMsg{Code: tea.KeyDown})
				return updated.(Model)
			},
		},
		{
			name: "filter typed",
			setup: func(t *testing.T, m Model) Model {
				var model tea.Model = m
				for _, r := range "/a" {
					model, _ = model.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
				}
				return model.(Model)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lister := &stepListerStub{steps: [][]tmux.Session{settleSessions}}
			pending := &pendingReaderStub{steps: [][]string{{}, {"charlie"}}}
			markers := &markerReaderStub{steps: [][]string{{}}}
			m := coldSettleModel(lister, pending, markers, generousSettleBound)
			m, _ = driveToPostRestoreRefetch(t, m)
			m = tt.setup(t, m)

			rows, index, filter, state := visibleSessionNames(m), m.sessionList.Index(), m.sessionList.FilterValue(), m.sessionList.FilterState()
			m, _ = settleStep(t, m)

			if got := visibleSessionNames(m); !slices.Equal(got, rows) {
				t.Errorf("rows = %v, want %v", got, rows)
			}
			if got := m.sessionList.Index(); got != index {
				t.Errorf("cursor = %d, want %d", got, index)
			}
			if got := m.sessionList.FilterValue(); got != filter {
				t.Errorf("filter = %q, want %q", got, filter)
			}
			if got := m.sessionList.FilterState(); got != state {
				t.Errorf("filter state = %v, want %v", got, state)
			}
			assertDots(t, m, map[string]bool{"alpha": false, "bravo": false, "charlie": true})
		})
	}
}

func TestPendingSettle_WarmRoute_TakesNoMarkerRead(t *testing.T) {
	lister := &stepListerStub{steps: [][]tmux.Session{settleSessions}}
	pending := &pendingReaderStub{steps: [][]string{{"alpha"}}}
	markers := &markerReaderStub{steps: [][]string{{"alpha:0.0"}}}
	m := Build(Deps{Lister: lister, PendingReader: pending, SkeletonMarkers: markers, PendingSettleBound: generousSettleBound})
	m.sessionList.SetSize(80, 24)

	updated, next := m.Update(m.fetchSessionsCmd()())
	m = updated.(Model)
	for _, msg := range drainAll(next) {
		if _, ok := msg.(pendingSettleTickMsg); ok {
			t.Error("the warm route scheduled a settle re-read")
		}
	}

	if markers.calls != 0 {
		t.Errorf("warm route took %d skeleton-marker reads, want 0", markers.calls)
	}
	if lister.calls != 1 || pending.calls != 1 {
		t.Errorf("warm route reads: sessions = %d, pending = %d, want 1 each", lister.calls, pending.calls)
	}
	assertDots(t, m, map[string]bool{"alpha": true, "bravo": false, "charlie": false})
}

func TestPendingSettle_ColdRoute_UnwiredMarkerSeamSchedulesNoReRead(t *testing.T) {
	lister := &stepListerStub{steps: [][]tmux.Session{settleSessions}}
	pending := &pendingReaderStub{steps: [][]string{{"alpha"}}}
	m := coldSettleModel(lister, pending, nil, generousSettleBound)

	_, next := driveToPostRestoreRefetch(t, m)

	for _, msg := range drainAll(next) {
		if _, ok := msg.(pendingSettleTickMsg); ok {
			t.Error("an unwired marker seam scheduled a settle re-read")
		}
	}
	if pending.calls != 1 {
		t.Errorf("pending reads = %d, want 1 (the refetch)", pending.calls)
	}
}
