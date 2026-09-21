package tmuxtest

import (
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// StampPaneToken writes Portal's pane-token option onto target using raw tmux,
// so a fixture never stages itself through the client call under test.
func (s *Socket) StampPaneToken(t *testing.T, target tmux.Target, token string) {
	t.Helper()
	s.Run(t, "set-option", "-p", "-t", string(target), state.PortalPaneIDOption, token)
}

// ReadPaneToken reads Portal's pane-token option back off target using raw
// tmux, reporting "" when the option is unset: an unset pane user-option makes
// `show-options -p -v` exit non-zero.
//
// The read is deliberately `show-options` and not `display-message -F`: on a
// target no pane answers to, `display-message` exits 0 having fallen back to
// the session's current pane (the `=` prefix does not change this), so it
// would hand a fixture another pane's token instead of failing.
func (s *Socket) ReadPaneToken(t *testing.T, target tmux.Target) string {
	t.Helper()
	out, err := s.TryRun("show-options", "-p", "-t", string(target), "-v", state.PortalPaneIDOption)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// MarkResumePending writes Portal's resume-pending pane option onto target
// using raw tmux, so a fixture never stages itself through the code under test.
func (s *Socket) MarkResumePending(t *testing.T, target tmux.Target) {
	t.Helper()
	s.Run(t, "set-option", "-p", "-t", string(target), state.ResumePendingOption, "1")
}

// ReadResumePending reads Portal's resume-pending pane option back off target
// using raw tmux, reporting "" when the option is unset, on the reasoning
// ReadPaneToken states.
func (s *Socket) ReadResumePending(t *testing.T, target tmux.Target) string {
	t.Helper()
	out, err := s.TryRun("show-options", "-p", "-t", string(target), "-v", state.ResumePendingOption)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}
