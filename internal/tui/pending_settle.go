package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

const pendingSettleInterval = 100 * time.Millisecond

type SkeletonMarkerReader interface {
	ListSkeletonMarkers() (map[string]struct{}, error)
}

// WithPendingSettle has the cold route keep re-reading the pending set after
// the post-restore refetch while any restored pane still carries its skeleton
// marker, for at most bound.
func WithPendingSettle(markers SkeletonMarkerReader, bound time.Duration) Option {
	return func(m *Model) {
		m.pendingSettle = pendingSettle{markers: markers, bound: bound}
	}
}

type pendingSettle struct {
	markers  SkeletonMarkerReader
	bound    time.Duration
	active   bool
	deadline time.Time
}

type pendingSettleTickMsg struct{}

type pendingSettleReadMsg struct {
	pending       map[string]struct{}
	markersRemain bool
}

func (m *Model) startPendingSettle() tea.Cmd {
	if m.pendingSettle.markers == nil || m.pendingReader == nil {
		return nil
	}
	m.pendingSettle.active = true
	m.pendingSettle.deadline = time.Now().Add(m.pendingSettle.bound)
	return pendingSettleTickCmd()
}

func pendingSettleTickCmd() tea.Cmd {
	return tea.Tick(pendingSettleInterval, func(time.Time) tea.Msg {
		return pendingSettleTickMsg{}
	})
}

func (m Model) pendingSettleTick() tea.Cmd {
	if !m.pendingSettle.active {
		return nil
	}
	return m.pendingSettleReadCmd()
}

// Order matters: the marker read precedes the pending read. A helper marks its
// pane pending before it unsets the pane's skeleton marker, so a pending read
// taken after a marker read that found none is final. A failed marker read
// proves nothing is final, so it counts as markers remaining.
func (m Model) pendingSettleReadCmd() tea.Cmd {
	markers := m.pendingSettle.markers
	return func() tea.Msg {
		set, err := markers.ListSkeletonMarkers()
		return pendingSettleReadMsg{
			pending:       m.readPendingSessions(),
			markersRemain: err != nil || len(set) > 0,
		}
	}
}

func (m *Model) applyPendingSettleRead(msg pendingSettleReadMsg) tea.Cmd {
	m.applyPendingSessions(msg.pending)
	if !msg.markersRemain || !time.Now().Before(m.pendingSettle.deadline) {
		m.pendingSettle.active = false
		return nil
	}
	return pendingSettleTickCmd()
}
