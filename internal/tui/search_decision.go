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

// resolveSearchDecision runs the search classification at the loading-to-picker
// gate and returns tea.Quit when the model must leave without a picker frame —
// a named session to attach, or a read failure to report. A nil return means
// the transition proceeds.
//
// The closure runs synchronously: deferring it through a tea.Cmd would let the
// transition it is meant to pre-empt fire first. Clearing the field before the
// call is the single-shot guard — Model is a value Bubble Tea copies per
// Update, and the copy cleared here is the one returned.
func (m *Model) resolveSearchDecision() tea.Cmd {
	if m.searchDecide == nil {
		return nil
	}
	decide := m.searchDecide
	m.searchDecide = nil

	name, err := decide()
	if err != nil {
		m.searchErr = err
		return tea.Quit
	}
	if name == "" {
		return nil
	}
	m.selected = name
	m.searchAttached = true
	return tea.Quit
}
