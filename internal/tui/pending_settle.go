package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

const pendingSettleInterval = 100 * time.Millisecond

// HydratingReader names the sessions whose hydrating flag is raised: those
// restore has created whose scrollback replay hasn't finished.
type HydratingReader interface {
	ListHydratingSessions() (map[string]struct{}, error)
}

// WithPendingSettle has the cold route keep re-reading the pending set after
// the post-restore refetch while any restored session is still hydrating, for
// at most bound.
func WithPendingSettle(hydrating HydratingReader, bound time.Duration) Option {
	return func(m *Model) {
		m.pendingSettle = pendingSettle{hydrating: hydrating, bound: bound}
	}
}

type pendingSettle struct {
	hydrating HydratingReader
	bound     time.Duration
	active    bool
	deadline  time.Time
}

type pendingSettleTickMsg struct{}

type pendingSettleReadMsg struct {
	pending         map[string]struct{}
	hydratingRemain bool
}

func (m *Model) startPendingSettle() tea.Cmd {
	if m.pendingSettle.hydrating == nil || m.pendingReader == nil {
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

// Order matters: the hydrating read precedes the pending read. A helper raises
// its session's resume-pending flag before it lowers its hydrating flag, so a
// pending read taken after a hydrating read that found none is final. A failed
// hydrating read proves nothing is final, so it counts as sessions still
// hydrating.
func (m Model) pendingSettleReadCmd() tea.Cmd {
	hydrating := m.pendingSettle.hydrating
	return func() tea.Msg {
		set, err := hydrating.ListHydratingSessions()
		return pendingSettleReadMsg{
			pending:         m.readPendingSessions(),
			hydratingRemain: err != nil || len(set) > 0,
		}
	}
}

func (m *Model) applyPendingSettleRead(msg pendingSettleReadMsg) tea.Cmd {
	m.applyPendingSessions(msg.pending)
	if !msg.hydratingRemain || !time.Now().Before(m.pendingSettle.deadline) {
		m.pendingSettle.active = false
		return nil
	}
	return pendingSettleTickCmd()
}
