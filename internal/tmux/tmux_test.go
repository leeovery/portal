package tmux_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/tmux"
)

// MockCommander implements Commander for testing.
type MockCommander struct {
	Output string
	Err    error
	// Calls records all invocations as joined arg strings.
	Calls [][]string
	// RunFunc, when set, is called instead of returning Output/Err.
	RunFunc func(args ...string) (string, error)
}

// Run returns the configured output and error, or delegates to RunFunc.
func (m *MockCommander) Run(args ...string) (string, error) {
	m.Calls = append(m.Calls, args)
	if m.RunFunc != nil {
		return m.RunFunc(args...)
	}
	return m.Output, m.Err
}

func TestListSessions(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		err      error
		want     []tmux.Session
		wantErr  bool
	}{
		{
			name:   "parses multiple sessions correctly",
			output: "dev|3|1\nwork|5|0\nmisc|1|0",
			want: []tmux.Session{
				{Name: "dev", Windows: 3, Attached: true},
				{Name: "work", Windows: 5, Attached: false},
				{Name: "misc", Windows: 1, Attached: false},
			},
		},
		{
			name:   "parses single session",
			output: "main|2|0",
			want: []tmux.Session{
				{Name: "main", Windows: 2, Attached: false},
			},
		},
		{
			name:   "returns empty slice when tmux server is not running",
			output: "",
			err:    fmt.Errorf("exit status 1"),
			want:   []tmux.Session{},
		},
		{
			name:   "returns empty slice when output is empty",
			output: "",
			want:   []tmux.Session{},
		},
		{
			name:   "attached is true when session_attached > 0",
			output: "session1|2|3",
			want: []tmux.Session{
				{Name: "session1", Windows: 2, Attached: true},
			},
		},
		{
			name:   "attached is false when session_attached is 0",
			output: "session1|2|0",
			want: []tmux.Session{
				{Name: "session1", Windows: 2, Attached: false},
			},
		},
		{
			name:   "handles session name with special characters",
			output: "my-project.v2|4|1",
			want: []tmux.Session{
				{Name: "my-project.v2", Windows: 4, Attached: true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockCommander{Output: tt.output, Err: tt.err}
			client := tmux.NewClient(mock)

			got, err := client.ListSessions()

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(got) != len(tt.want) {
				t.Fatalf("got %d sessions, want %d", len(got), len(tt.want))
			}

			for i, session := range got {
				if session.Name != tt.want[i].Name {
					t.Errorf("session[%d].Name = %q, want %q", i, session.Name, tt.want[i].Name)
				}
				if session.Windows != tt.want[i].Windows {
					t.Errorf("session[%d].Windows = %d, want %d", i, session.Windows, tt.want[i].Windows)
				}
				if session.Attached != tt.want[i].Attached {
					t.Errorf("session[%d].Attached = %v, want %v", i, session.Attached, tt.want[i].Attached)
				}
			}
		})
	}
}

func TestHasSession(t *testing.T) {
	t.Run("returns true when session exists", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		got := client.HasSession("my-session")

		if !got {
			t.Error("HasSession() = false, want true")
		}

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		wantArgs := "has-session -t my-session"
		gotArgs := strings.Join(mock.Calls[0], " ")
		if gotArgs != wantArgs {
			t.Errorf("called with %q, want %q", gotArgs, wantArgs)
		}
	})

	t.Run("returns false when session does not exist", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("exit status 1")}
		client := tmux.NewClient(mock)

		got := client.HasSession("nonexistent")

		if got {
			t.Error("HasSession() = true, want false")
		}
	})

	t.Run("returns false when no tmux server running", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("no server running on /tmp/tmux-501/default")}
		client := tmux.NewClient(mock)

		got := client.HasSession("any-session")

		if got {
			t.Error("HasSession() = true, want false")
		}
	})
}

func TestNewSession(t *testing.T) {
	t.Run("creates session with name and directory", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		err := client.NewSession("my-session", "/home/user/project")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		wantArgs := "new-session -d -s my-session -c /home/user/project"
		gotArgs := strings.Join(mock.Calls[0], " ")
		if gotArgs != wantArgs {
			t.Errorf("called with %q, want %q", gotArgs, wantArgs)
		}
	})

	t.Run("returns error when tmux command fails", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("tmux error")}
		client := tmux.NewClient(mock)

		err := client.NewSession("my-session", "/some/dir")

		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestCurrentSessionName(t *testing.T) {
	t.Run("returns session name from tmux output", func(t *testing.T) {
		mock := &MockCommander{Output: "my-project-x7k2m9"}
		client := tmux.NewClient(mock)

		got, err := client.CurrentSessionName()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "my-project-x7k2m9" {
			t.Errorf("CurrentSessionName() = %q, want %q", got, "my-project-x7k2m9")
		}

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		wantArgs := "display-message -p #{session_name}"
		gotArgs := strings.Join(mock.Calls[0], " ")
		if gotArgs != wantArgs {
			t.Errorf("called with %q, want %q", gotArgs, wantArgs)
		}
	})

	t.Run("returns error when tmux command fails", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("no server running")}
		client := tmux.NewClient(mock)

		_, err := client.CurrentSessionName()

		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestKillSession(t *testing.T) {
	t.Run("runs kill-session with session name", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		err := client.KillSession("my-session")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		wantArgs := "kill-session -t my-session"
		gotArgs := strings.Join(mock.Calls[0], " ")
		if gotArgs != wantArgs {
			t.Errorf("called with %q, want %q", gotArgs, wantArgs)
		}
	})

	t.Run("returns error when tmux command fails", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("session not found")}
		client := tmux.NewClient(mock)

		err := client.KillSession("nonexistent")

		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestSwitchClient(t *testing.T) {
	t.Run("runs switch-client with session name", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		err := client.SwitchClient("my-session")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		wantArgs := "switch-client -t my-session"
		gotArgs := strings.Join(mock.Calls[0], " ")
		if gotArgs != wantArgs {
			t.Errorf("called with %q, want %q", gotArgs, wantArgs)
		}
	})

	t.Run("returns error when tmux command fails", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("session not found")}
		client := tmux.NewClient(mock)

		err := client.SwitchClient("nonexistent")

		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}
