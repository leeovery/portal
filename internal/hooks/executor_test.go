package hooks_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/leeovery/portal/internal/hooks"
)

// mockPaneLister implements hooks.PaneLister for tests.
type mockPaneLister struct {
	panes map[string][]string
	err   error
}

func (m *mockPaneLister) ListPanes(sessionName string) ([]string, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.panes[sessionName], nil
}

// mockKeySender implements hooks.KeySender for tests.
type mockKeySender struct {
	sent    []keySend
	failFor map[string]bool
}

type keySend struct {
	paneID  string
	command string
}

func (m *mockKeySender) SendKeys(paneID string, command string) error {
	if m.failFor != nil && m.failFor[paneID] {
		return fmt.Errorf("send-keys failed for %s", paneID)
	}
	m.sent = append(m.sent, keySend{paneID: paneID, command: command})
	return nil
}

// mockOptionChecker implements hooks.OptionChecker for tests.
type mockOptionChecker struct {
	options map[string]string
	setLog  []optionSet
}

type optionSet struct {
	name  string
	value string
}

func (m *mockOptionChecker) GetServerOption(name string) (string, error) {
	if val, ok := m.options[name]; ok {
		return val, nil
	}
	return "", errors.New("option not found")
}

func (m *mockOptionChecker) SetServerOption(name, value string) error {
	m.setLog = append(m.setLog, optionSet{name: name, value: value})
	if m.options == nil {
		m.options = make(map[string]string)
	}
	m.options[name] = value
	return nil
}

// mockHookLoader implements hooks.HookLoader for tests.
type mockHookLoader struct {
	data map[string]map[string]string
	err  error
}

func (m *mockHookLoader) Load() (map[string]map[string]string, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.data, nil
}

func TestExecuteHooks(t *testing.T) {
	t.Run("executes hook when persistent entry exists and marker absent", func(t *testing.T) {
		loader := &mockHookLoader{
			data: map[string]map[string]string{
				"%3": {"on-resume": "claude --resume abc123"},
			},
		}
		lister := &mockPaneLister{
			panes: map[string][]string{"my-session": {"%3"}},
		}
		sender := &mockKeySender{}
		checker := &mockOptionChecker{options: map[string]string{}}

		hooks.ExecuteHooks("my-session", lister, loader, sender, checker)

		if len(sender.sent) != 1 {
			t.Fatalf("expected 1 send-keys call, got %d", len(sender.sent))
		}
		if sender.sent[0].paneID != "%3" {
			t.Errorf("paneID = %q, want %%3", sender.sent[0].paneID)
		}
		if sender.sent[0].command != "claude --resume abc123" {
			t.Errorf("command = %q, want %q", sender.sent[0].command, "claude --resume abc123")
		}
	})

	t.Run("skips pane when volatile marker present", func(t *testing.T) {
		loader := &mockHookLoader{
			data: map[string]map[string]string{
				"%3": {"on-resume": "claude --resume abc123"},
			},
		}
		lister := &mockPaneLister{
			panes: map[string][]string{"my-session": {"%3"}},
		}
		sender := &mockKeySender{}
		checker := &mockOptionChecker{
			options: map[string]string{"@portal-active-%3": "1"},
		}

		hooks.ExecuteHooks("my-session", lister, loader, sender, checker)

		if len(sender.sent) != 0 {
			t.Errorf("expected 0 send-keys calls, got %d", len(sender.sent))
		}
	})

	t.Run("skips pane not in session", func(t *testing.T) {
		loader := &mockHookLoader{
			data: map[string]map[string]string{
				"%3": {"on-resume": "claude --resume abc123"},
				"%7": {"on-resume": "claude --resume def456"},
			},
		}
		lister := &mockPaneLister{
			panes: map[string][]string{"my-session": {"%3"}},
		}
		sender := &mockKeySender{}
		checker := &mockOptionChecker{options: map[string]string{}}

		hooks.ExecuteHooks("my-session", lister, loader, sender, checker)

		if len(sender.sent) != 1 {
			t.Fatalf("expected 1 send-keys call, got %d", len(sender.sent))
		}
		if sender.sent[0].paneID != "%3" {
			t.Errorf("paneID = %q, want %%3", sender.sent[0].paneID)
		}
	})

	t.Run("skips pane with no on-resume event", func(t *testing.T) {
		loader := &mockHookLoader{
			data: map[string]map[string]string{
				"%3": {"on-start": "echo hello"},
			},
		}
		lister := &mockPaneLister{
			panes: map[string][]string{"my-session": {"%3"}},
		}
		sender := &mockKeySender{}
		checker := &mockOptionChecker{options: map[string]string{}}

		hooks.ExecuteHooks("my-session", lister, loader, sender, checker)

		if len(sender.sent) != 0 {
			t.Errorf("expected 0 send-keys calls, got %d", len(sender.sent))
		}
	})

	t.Run("sets volatile marker after executing hook", func(t *testing.T) {
		loader := &mockHookLoader{
			data: map[string]map[string]string{
				"%3": {"on-resume": "claude --resume abc123"},
			},
		}
		lister := &mockPaneLister{
			panes: map[string][]string{"my-session": {"%3"}},
		}
		sender := &mockKeySender{}
		checker := &mockOptionChecker{options: map[string]string{}}

		hooks.ExecuteHooks("my-session", lister, loader, sender, checker)

		if len(checker.setLog) != 1 {
			t.Fatalf("expected 1 SetServerOption call, got %d", len(checker.setLog))
		}
		if checker.setLog[0].name != "@portal-active-%3" {
			t.Errorf("option name = %q, want %q", checker.setLog[0].name, "@portal-active-%3")
		}
		if checker.setLog[0].value != "1" {
			t.Errorf("option value = %q, want %q", checker.setLog[0].value, "1")
		}
	})

	t.Run("continues to next pane when SendKeys fails", func(t *testing.T) {
		loader := &mockHookLoader{
			data: map[string]map[string]string{
				"%3": {"on-resume": "claude --resume abc123"},
				"%7": {"on-resume": "claude --resume def456"},
			},
		}
		lister := &mockPaneLister{
			panes: map[string][]string{"my-session": {"%3", "%7"}},
		}
		sender := &mockKeySender{
			failFor: map[string]bool{"%3": true},
		}
		checker := &mockOptionChecker{options: map[string]string{}}

		hooks.ExecuteHooks("my-session", lister, loader, sender, checker)

		// %7 should still have been sent
		if len(sender.sent) != 1 {
			t.Fatalf("expected 1 send-keys call (for %%7), got %d", len(sender.sent))
		}
		if sender.sent[0].paneID != "%7" {
			t.Errorf("paneID = %q, want %%7", sender.sent[0].paneID)
		}
	})

	t.Run("silent return when hook store Load fails", func(t *testing.T) {
		loader := &mockHookLoader{
			err: errors.New("disk error"),
		}
		lister := &mockPaneLister{
			panes: map[string][]string{"my-session": {"%3"}},
		}
		sender := &mockKeySender{}
		checker := &mockOptionChecker{options: map[string]string{}}

		// Should not panic or call any other methods
		hooks.ExecuteHooks("my-session", lister, loader, sender, checker)

		if len(sender.sent) != 0 {
			t.Errorf("expected 0 send-keys calls, got %d", len(sender.sent))
		}
	})

	t.Run("silent return when ListPanes fails", func(t *testing.T) {
		loader := &mockHookLoader{
			data: map[string]map[string]string{
				"%3": {"on-resume": "claude --resume abc123"},
			},
		}
		lister := &mockPaneLister{
			err: errors.New("tmux error"),
		}
		sender := &mockKeySender{}
		checker := &mockOptionChecker{options: map[string]string{}}

		hooks.ExecuteHooks("my-session", lister, loader, sender, checker)

		if len(sender.sent) != 0 {
			t.Errorf("expected 0 send-keys calls, got %d", len(sender.sent))
		}
	})

	t.Run("no-op when hook store is empty", func(t *testing.T) {
		loader := &mockHookLoader{
			data: map[string]map[string]string{},
		}
		lister := &mockPaneLister{
			panes: map[string][]string{"my-session": {"%3"}},
		}
		sender := &mockKeySender{}
		checker := &mockOptionChecker{options: map[string]string{}}

		hooks.ExecuteHooks("my-session", lister, loader, sender, checker)

		if len(sender.sent) != 0 {
			t.Errorf("expected 0 send-keys calls, got %d", len(sender.sent))
		}
	})

	t.Run("no-op when session has no panes", func(t *testing.T) {
		loader := &mockHookLoader{
			data: map[string]map[string]string{
				"%3": {"on-resume": "claude --resume abc123"},
			},
		}
		lister := &mockPaneLister{
			panes: map[string][]string{"my-session": {}},
		}
		sender := &mockKeySender{}
		checker := &mockOptionChecker{options: map[string]string{}}

		hooks.ExecuteHooks("my-session", lister, loader, sender, checker)

		if len(sender.sent) != 0 {
			t.Errorf("expected 0 send-keys calls, got %d", len(sender.sent))
		}
	})

	t.Run("executes hooks for multiple qualifying panes", func(t *testing.T) {
		loader := &mockHookLoader{
			data: map[string]map[string]string{
				"%3": {"on-resume": "claude --resume abc123"},
				"%5": {"on-resume": "npm start"},
				"%7": {"on-resume": "claude --resume def456"},
			},
		}
		lister := &mockPaneLister{
			panes: map[string][]string{"my-session": {"%3", "%5", "%7"}},
		}
		sender := &mockKeySender{}
		checker := &mockOptionChecker{options: map[string]string{}}

		hooks.ExecuteHooks("my-session", lister, loader, sender, checker)

		if len(sender.sent) != 3 {
			t.Fatalf("expected 3 send-keys calls, got %d", len(sender.sent))
		}

		// Verify all three panes were sent commands
		sentPanes := make(map[string]string)
		for _, s := range sender.sent {
			sentPanes[s.paneID] = s.command
		}
		if sentPanes["%3"] != "claude --resume abc123" {
			t.Errorf("%%3 command = %q, want %q", sentPanes["%3"], "claude --resume abc123")
		}
		if sentPanes["%5"] != "npm start" {
			t.Errorf("%%5 command = %q, want %q", sentPanes["%5"], "npm start")
		}
		if sentPanes["%7"] != "claude --resume def456" {
			t.Errorf("%%7 command = %q, want %q", sentPanes["%7"], "claude --resume def456")
		}

		// Verify all three markers were set
		if len(checker.setLog) != 3 {
			t.Fatalf("expected 3 SetServerOption calls, got %d", len(checker.setLog))
		}
	})
}
