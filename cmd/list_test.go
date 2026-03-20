package cmd

// Tests in this file mutate package-level state (bootstrapDeps, listDeps) and MUST NOT use t.Parallel.

import (
	"bytes"
	"testing"

	"github.com/leeovery/portal/internal/tmux"
)

// mockSessionLister implements SessionLister for testing.
type mockSessionLister struct {
	sessions []tmux.Session
	err      error
}

func (m *mockSessionLister) ListSessions() ([]tmux.Session, error) {
	return m.sessions, m.err
}

func TestListCommand(t *testing.T) {
	bootstrapDeps = &BootstrapDeps{Bootstrapper: &mockServerBootstrapper{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	t.Run("TTY output includes name status and window count", func(t *testing.T) {
		lister := &mockSessionLister{
			sessions: []tmux.Session{
				{Name: "flowx-dev", Windows: 3, Attached: true},
				{Name: "claude-lab", Windows: 1, Attached: false},
			},
		}
		listDeps = &ListDeps{
			Lister: lister,
			IsTTY:  func() bool { return true },
		}
		t.Cleanup(func() { listDeps = nil })

		resetRootCmd()
		buf := new(bytes.Buffer)
		rootCmd.SetOut(buf)
		rootCmd.SetArgs([]string{"list"})

		err := rootCmd.Execute()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := "flowx-dev    attached    3 windows\nclaude-lab    detached    1 window\n"
		if buf.String() != want {
			t.Errorf("output = %q, want %q", buf.String(), want)
		}
	})

	t.Run("piped output shows names only", func(t *testing.T) {
		lister := &mockSessionLister{
			sessions: []tmux.Session{
				{Name: "flowx-dev", Windows: 3, Attached: true},
				{Name: "claude-lab", Windows: 1, Attached: false},
			},
		}
		listDeps = &ListDeps{
			Lister: lister,
			IsTTY:  func() bool { return false },
		}
		t.Cleanup(func() { listDeps = nil })

		resetRootCmd()
		buf := new(bytes.Buffer)
		rootCmd.SetOut(buf)
		rootCmd.SetArgs([]string{"list"})

		err := rootCmd.Execute()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := "flowx-dev\nclaude-lab\n"
		if buf.String() != want {
			t.Errorf("output = %q, want %q", buf.String(), want)
		}
	})

	t.Run("short flag forces names only even on TTY", func(t *testing.T) {
		lister := &mockSessionLister{
			sessions: []tmux.Session{
				{Name: "flowx-dev", Windows: 3, Attached: true},
				{Name: "claude-lab", Windows: 1, Attached: false},
			},
		}
		listDeps = &ListDeps{
			Lister: lister,
			IsTTY:  func() bool { return true },
		}
		t.Cleanup(func() { listDeps = nil })

		resetRootCmd()
		buf := new(bytes.Buffer)
		rootCmd.SetOut(buf)
		rootCmd.SetArgs([]string{"list", "--short"})

		err := rootCmd.Execute()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := "flowx-dev\nclaude-lab\n"
		if buf.String() != want {
			t.Errorf("output = %q, want %q", buf.String(), want)
		}
	})

	t.Run("long flag forces full details even when piped", func(t *testing.T) {
		lister := &mockSessionLister{
			sessions: []tmux.Session{
				{Name: "flowx-dev", Windows: 3, Attached: true},
				{Name: "claude-lab", Windows: 1, Attached: false},
			},
		}
		listDeps = &ListDeps{
			Lister: lister,
			IsTTY:  func() bool { return false },
		}
		t.Cleanup(func() { listDeps = nil })

		resetRootCmd()
		buf := new(bytes.Buffer)
		rootCmd.SetOut(buf)
		rootCmd.SetArgs([]string{"list", "--long"})

		err := rootCmd.Execute()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := "flowx-dev    attached    3 windows\nclaude-lab    detached    1 window\n"
		if buf.String() != want {
			t.Errorf("output = %q, want %q", buf.String(), want)
		}
	})

	t.Run("no sessions produces empty output", func(t *testing.T) {
		lister := &mockSessionLister{
			sessions: []tmux.Session{},
		}
		listDeps = &ListDeps{
			Lister: lister,
			IsTTY:  func() bool { return true },
		}
		t.Cleanup(func() { listDeps = nil })

		resetRootCmd()
		buf := new(bytes.Buffer)
		rootCmd.SetOut(buf)
		rootCmd.SetArgs([]string{"list"})

		err := rootCmd.Execute()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if buf.String() != "" {
			t.Errorf("output = %q, want empty string", buf.String())
		}
	})

	t.Run("exit code is 0 with no sessions", func(t *testing.T) {
		lister := &mockSessionLister{
			sessions: []tmux.Session{},
		}
		listDeps = &ListDeps{
			Lister: lister,
			IsTTY:  func() bool { return true },
		}
		t.Cleanup(func() { listDeps = nil })

		resetRootCmd()
		buf := new(bytes.Buffer)
		rootCmd.SetOut(buf)
		rootCmd.SetArgs([]string{"list"})

		err := rootCmd.Execute()

		if err != nil {
			t.Fatalf("expected nil error (exit 0), got: %v", err)
		}
	})

	t.Run("all sessions listed inside tmux including current", func(t *testing.T) {
		// Inside tmux: all sessions should be listed, no exclusions
		lister := &mockSessionLister{
			sessions: []tmux.Session{
				{Name: "current-session", Windows: 2, Attached: true},
				{Name: "other-session", Windows: 1, Attached: false},
			},
		}
		listDeps = &ListDeps{
			Lister: lister,
			IsTTY:  func() bool { return true },
		}
		t.Cleanup(func() { listDeps = nil })

		t.Setenv("TMUX", "/tmp/tmux-501/default,12345,0")

		resetRootCmd()
		buf := new(bytes.Buffer)
		rootCmd.SetOut(buf)
		rootCmd.SetArgs([]string{"list"})

		err := rootCmd.Execute()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := "current-session    attached    2 windows\nother-session    detached    1 window\n"
		if buf.String() != want {
			t.Errorf("output = %q, want %q", buf.String(), want)
		}
	})

	t.Run("window count pluralisation correct", func(t *testing.T) {
		lister := &mockSessionLister{
			sessions: []tmux.Session{
				{Name: "one-win", Windows: 1, Attached: false},
				{Name: "two-win", Windows: 2, Attached: false},
				{Name: "many-win", Windows: 10, Attached: true},
			},
		}
		listDeps = &ListDeps{
			Lister: lister,
			IsTTY:  func() bool { return true },
		}
		t.Cleanup(func() { listDeps = nil })

		resetRootCmd()
		buf := new(bytes.Buffer)
		rootCmd.SetOut(buf)
		rootCmd.SetArgs([]string{"list"})

		err := rootCmd.Execute()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := "one-win    detached    1 window\ntwo-win    detached    2 windows\nmany-win    attached    10 windows\n"
		if buf.String() != want {
			t.Errorf("output = %q, want %q", buf.String(), want)
		}
	})

	t.Run("short and long flags are mutually exclusive", func(t *testing.T) {
		lister := &mockSessionLister{
			sessions: []tmux.Session{
				{Name: "test", Windows: 1, Attached: false},
			},
		}
		listDeps = &ListDeps{
			Lister: lister,
			IsTTY:  func() bool { return true },
		}
		t.Cleanup(func() { listDeps = nil })

		resetRootCmd()
		buf := new(bytes.Buffer)
		rootCmd.SetOut(buf)
		rootCmd.SetErr(buf)
		rootCmd.SetArgs([]string{"list", "--short", "--long"})

		err := rootCmd.Execute()

		if err == nil {
			t.Fatal("expected error for mutually exclusive flags, got nil")
		}
	})
}
