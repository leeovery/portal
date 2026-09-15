package tui

import (
	tea "charm.land/bubbletea/v2"
)

// WithSearchDecision carries the classification a session search defers to the
// point the live session list can answer for it. It is nil-tolerant: a nil fn
// leaves the seam inert and the loading page dismisses into the picker as it
// does for every other invocation.
func WithSearchDecision(fn func() (string, error)) Option {
	return func(m *Model) {
		m.searchDecide = fn
	}
}

// SearchAttached reports whether the search decision named a session to attach
// rather than opening the picker.
func (m Model) SearchAttached() bool {
	return m.searchAttached
}

// SearchError is the failed session-list read the search decision answered
// with, nil otherwise. It is not a bootstrap fatal and takes no error frame.
func (m Model) SearchError() error {
	return m.searchErr
}

// searchDecisionMsg is the classification the dispatched closure answered with:
// a session to attach, the empty string for the picker, or a failed read.
type searchDecisionMsg struct {
	name string
	err  error
}

// searchDecisionCmd runs the classification off the update goroutine. The
// closure is supplied by the caller and reads tmux, so its duration is
// unbounded from here: running it inline would freeze the loading page — and
// its Ctrl-C — for as long as the server takes to answer.
func (m Model) searchDecisionCmd() tea.Cmd {
	decide := m.searchDecide
	return func() tea.Msg {
		name, err := decide()
		return searchDecisionMsg{name: name, err: err}
	}
}

// applySearchDecision acts on the answered classification: tea.Quit without a
// picker frame for an attach or a read failure, and today's dismissal sequence
// for anything else.
func (m Model) applySearchDecision(msg searchDecisionMsg) (Model, tea.Cmd) {
	m.searchDecide = nil
	m.searchDecideInFlight = false
	if msg.err != nil {
		m.searchErr = msg.err
		return m, tea.Quit
	}
	if msg.name != "" {
		m.selected = msg.name
		m.searchAttached = true
		return m, tea.Quit
	}
	return m.completeLoadingDismissal()
}
