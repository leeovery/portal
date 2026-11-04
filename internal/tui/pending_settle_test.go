package tui

import (
	"errors"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// hydratingReaderStub answers each read with the next step's hydrating
// sessions, holding on the last step once the script runs out. With err set,
// the read numbered errStep fails with it instead.
type hydratingReaderStub struct {
	steps   [][]string
	err     error
	errStep int
	calls   int
	log     *[]string
}

func (r *hydratingReaderStub) ListHydratingSessions() (map[string]struct{}, error) {
	idx := r.calls
	r.calls++
	if r.log != nil {
		*r.log = append(*r.log, "hydrating")
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

var settleSessions = []Session{
	{Name: "alpha"},
	{Name: "bravo"},
	{Name: "charlie"},
}

const generousSettleBound = time.Minute

func coldSettleModel(lister SessionLister, pending PendingResumeReader, hydrating HydratingReader, bound time.Duration) Model {
	return Build(Deps{
		Lister:             lister,
		PendingReader:      pending,
		Hydrating:          hydrating,
		PendingSettleBound: bound,
		Bootstrapping:      true,
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

func TestPendingSettle_ColdRoute_GainsTheDotWithNoKeypressOnceTheHelperRaisesItsFlag(t *testing.T) {
	lister := &stepListerStub{steps: [][]Session{settleSessions}}
	pending := &pendingReaderStub{steps: [][]string{{}, {"alpha"}}}
	hydrating := &hydratingReaderStub{steps: [][]string{{}}}
	m := coldSettleModel(lister, pending, hydrating, generousSettleBound)

	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model, _ = model.Update(SessionsMsg{})
	model, _ = model.Update(LoadingMinElapsedMsg{})
	model, completeCmd := model.Update(BootstrapCompleteMsg{})
	final := drainBatchToModel(t, model.(Model), completeCmd)

	assertDots(t, final, map[string]bool{"alpha": true, "bravo": false, "charlie": false})
}

func TestPendingSettle_ColdRoute_EachSessionGainsItsDotAsItIsFlagged(t *testing.T) {
	lister := &stepListerStub{steps: [][]Session{settleSessions}}
	pending := &pendingReaderStub{steps: [][]string{{}, {"alpha"}, {"alpha", "bravo"}}}
	hydrating := &hydratingReaderStub{steps: [][]string{{"bravo"}, {}}}
	m := coldSettleModel(lister, pending, hydrating, generousSettleBound)

	m, next := driveToPostRestoreRefetch(t, m)
	assertDots(t, m, map[string]bool{"alpha": false, "bravo": false, "charlie": false})
	if next == nil {
		t.Fatal("the post-restore refetch scheduled no re-read")
	}

	m, next = settleStep(t, m)
	assertDots(t, m, map[string]bool{"alpha": true, "bravo": false, "charlie": false})
	if next == nil {
		t.Fatal("a reading with a session still hydrating scheduled no further re-read")
	}

	m, next = settleStep(t, m)
	assertDots(t, m, map[string]bool{"alpha": true, "bravo": true, "charlie": false})
	if next != nil {
		t.Error("a reading with no session hydrating scheduled a further re-read")
	}
}

func TestPendingSettle_ColdRoute_SettlesOnEveryDotWhenTheLastSessionStopsHydratingBeforeARead(t *testing.T) {
	tests := []struct {
		name      string
		pending   [][]string
		hydrating [][]string
		steps     int
	}{
		{
			name:      "between the refetch and the first re-read",
			pending:   [][]string{{}, {"alpha", "charlie"}},
			hydrating: [][]string{{}},
			steps:     1,
		},
		{
			name:      "between two re-reads",
			pending:   [][]string{{}, {"alpha"}, {"alpha", "charlie"}},
			hydrating: [][]string{{"charlie"}, {}},
			steps:     2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var log []string
			lister := &stepListerStub{steps: [][]Session{settleSessions}}
			pending := &pendingReaderStub{steps: tt.pending, log: &log}
			hydrating := &hydratingReaderStub{steps: tt.hydrating, log: &log}
			m := coldSettleModel(lister, pending, hydrating, generousSettleBound)

			m, _ = driveToPostRestoreRefetch(t, m)
			var next tea.Cmd
			for range tt.steps {
				m, next = settleStep(t, m)
			}

			if next != nil {
				t.Error("the picker kept re-reading after finding no session hydrating")
			}
			assertDots(t, m, map[string]bool{"alpha": true, "bravo": false, "charlie": true})
			want := []string{"pending"}
			for range tt.steps {
				want = append(want, "hydrating", "pending")
			}
			if !slices.Equal(log, want) {
				t.Errorf("reads = %v, want %v (each hydrating read ahead of its pending read)", log, want)
			}
		})
	}
}

func TestPendingSettle_ColdRoute_AFailedHydratingReadCountsAsSessionsStillHydrating(t *testing.T) {
	lister := &stepListerStub{steps: [][]Session{settleSessions}}
	pending := &pendingReaderStub{steps: [][]string{{}, {}, {"alpha"}}}
	hydrating := &hydratingReaderStub{steps: [][]string{{}}, err: errors.New("read hydrating flags: permission denied"), errStep: 0}
	m := coldSettleModel(lister, pending, hydrating, generousSettleBound)

	m, _ = driveToPostRestoreRefetch(t, m)

	m, next := settleStep(t, m)
	if next == nil {
		t.Fatal("a failed hydrating read ended the settle, want it to schedule a further re-read")
	}
	assertDots(t, m, map[string]bool{"alpha": false, "bravo": false, "charlie": false})

	m, next = settleStep(t, m)
	if next != nil {
		t.Error("a reading with no session hydrating scheduled a further re-read")
	}
	assertDots(t, m, map[string]bool{"alpha": true, "bravo": false, "charlie": false})
	if hydrating.calls != 2 || pending.calls != 3 {
		t.Errorf("reads: hydrating = %d, pending = %d, want 2 and 3", hydrating.calls, pending.calls)
	}
}

func TestPendingSettle_ColdRoute_TakesNoFurtherReadOnceNoSessionIsHydrating(t *testing.T) {
	lister := &stepListerStub{steps: [][]Session{settleSessions}}
	pending := &pendingReaderStub{steps: [][]string{{}, {"alpha"}}}
	hydrating := &hydratingReaderStub{steps: [][]string{{}}}
	m := coldSettleModel(lister, pending, hydrating, generousSettleBound)

	m, _ = driveToPostRestoreRefetch(t, m)
	m, next := settleStep(t, m)
	if next != nil {
		t.Fatal("a reading with no session hydrating scheduled a further re-read")
	}

	updated, stray := m.Update(pendingSettleTickMsg{})
	m = updated.(Model)
	if stray != nil {
		t.Error("a tick arriving after the settle ended scheduled a read")
	}
	if hydrating.calls != 1 || pending.calls != 2 {
		t.Errorf("reads after settling: hydrating = %d, pending = %d, want 1 and 2", hydrating.calls, pending.calls)
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
	if hydrating.calls != 1 || pending.calls != 3 {
		t.Errorf("reads after a kill: hydrating = %d, pending = %d, want 1 and 3", hydrating.calls, pending.calls)
	}
}

func TestPendingSettle_ColdRoute_StopsAtTheBoundWhenASessionNeverStopsHydrating(t *testing.T) {
	lister := &stepListerStub{steps: [][]Session{settleSessions}}
	pending := &pendingReaderStub{steps: [][]string{{}, {"bravo"}}}
	hydrating := &hydratingReaderStub{steps: [][]string{{"alpha"}}}
	m := coldSettleModel(lister, pending, hydrating, time.Nanosecond)

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
			lister := &stepListerStub{steps: [][]Session{settleSessions}}
			pending := &pendingReaderStub{steps: [][]string{{}, {"charlie"}}}
			hydrating := &hydratingReaderStub{steps: [][]string{{}}}
			m := coldSettleModel(lister, pending, hydrating, generousSettleBound)
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

func TestPendingSettle_WarmRoute_TakesNoHydratingRead(t *testing.T) {
	lister := &stepListerStub{steps: [][]Session{settleSessions}}
	pending := &pendingReaderStub{steps: [][]string{{"alpha"}}}
	hydrating := &hydratingReaderStub{steps: [][]string{{"alpha"}}}
	m := Build(Deps{Lister: lister, PendingReader: pending, Hydrating: hydrating, PendingSettleBound: generousSettleBound})
	m.sessionList.SetSize(80, 24)

	updated, next := m.Update(m.fetchSessionsCmd()())
	m = updated.(Model)
	for _, msg := range drainAll(next) {
		if _, ok := msg.(pendingSettleTickMsg); ok {
			t.Error("the warm route scheduled a settle re-read")
		}
	}

	if hydrating.calls != 0 {
		t.Errorf("warm route took %d hydrating reads, want 0", hydrating.calls)
	}
	if lister.calls != 1 || pending.calls != 1 {
		t.Errorf("warm route reads: sessions = %d, pending = %d, want 1 each", lister.calls, pending.calls)
	}
	assertDots(t, m, map[string]bool{"alpha": true, "bravo": false, "charlie": false})
}

func TestPendingSettle_ColdRoute_UnwiredHydratingSeamSchedulesNoReRead(t *testing.T) {
	lister := &stepListerStub{steps: [][]Session{settleSessions}}
	pending := &pendingReaderStub{steps: [][]string{{"alpha"}}}
	m := coldSettleModel(lister, pending, nil, generousSettleBound)

	_, next := driveToPostRestoreRefetch(t, m)

	for _, msg := range drainAll(next) {
		if _, ok := msg.(pendingSettleTickMsg); ok {
			t.Error("an unwired hydrating seam scheduled a settle re-read")
		}
	}
	if pending.calls != 1 {
		t.Errorf("pending reads = %d, want 1 (the refetch)", pending.calls)
	}
}
