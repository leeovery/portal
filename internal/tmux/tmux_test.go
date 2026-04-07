package tmux_test

import (
	"errors"
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
		name    string
		output  string
		err     error
		want    []tmux.Session
		wantErr bool
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

func TestServerRunning(t *testing.T) {
	t.Run("returns true when tmux server is running", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		got := client.ServerRunning()

		if !got {
			t.Error("ServerRunning() = false, want true")
		}
	})

	t.Run("returns false when no tmux server is running", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("no server running on /tmp/tmux-501/default")}
		client := tmux.NewClient(mock)

		got := client.ServerRunning()

		if got {
			t.Error("ServerRunning() = true, want false")
		}
	})

	t.Run("calls tmux info to check server status", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		client.ServerRunning()

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		wantArgs := []string{"info"}
		if len(mock.Calls[0]) != len(wantArgs) {
			t.Fatalf("got %d args %v, want %d args %v", len(mock.Calls[0]), mock.Calls[0], len(wantArgs), wantArgs)
		}
		if mock.Calls[0][0] != "info" {
			t.Errorf("called with %q, want %q", mock.Calls[0][0], "info")
		}
	})
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

		err := client.NewSession("my-session", "/home/user/project", "")

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

	t.Run("includes shell-command when provided", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		shellCmd := "/bin/zsh -ic 'claude; exec /bin/zsh'"
		err := client.NewSession("my-session", "/home/user/project", shellCmd)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		wantArgs := []string{"new-session", "-d", "-s", "my-session", "-c", "/home/user/project", shellCmd}
		if len(mock.Calls[0]) != len(wantArgs) {
			t.Fatalf("got %d args %v, want %d args %v", len(mock.Calls[0]), mock.Calls[0], len(wantArgs), wantArgs)
		}
		for i, arg := range mock.Calls[0] {
			if arg != wantArgs[i] {
				t.Errorf("args[%d] = %q, want %q", i, arg, wantArgs[i])
			}
		}
	})

	t.Run("no shell-command argument when empty string", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		err := client.NewSession("my-session", "/home/user/project", "")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		// Should be exactly 6 args: new-session -d -s <name> -c <dir>
		if len(mock.Calls[0]) != 6 {
			t.Errorf("got %d args %v, want 6 args (no shell-command)", len(mock.Calls[0]), mock.Calls[0])
		}
	})

	t.Run("returns error when tmux command fails", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("tmux error")}
		client := tmux.NewClient(mock)

		err := client.NewSession("my-session", "/some/dir", "")

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

func TestStartServer(t *testing.T) {
	t.Run("starts tmux server successfully", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		err := client.StartServer()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		wantArgs := "new-session -d"
		gotArgs := strings.Join(mock.Calls[0], " ")
		if gotArgs != wantArgs {
			t.Errorf("called with %q, want %q", gotArgs, wantArgs)
		}
	})

	t.Run("returns error when start-server fails", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("tmux failed")}
		client := tmux.NewClient(mock)

		err := client.StartServer()

		if err == nil {
			t.Fatal("expected error, got nil")
		}

		wantMsg := "failed to start tmux server (bootstrap session)"
		if !strings.Contains(err.Error(), wantMsg) {
			t.Errorf("error %q does not contain %q", err.Error(), wantMsg)
		}

		// Verify the original error is wrapped
		wantWrapped := "tmux failed"
		if !strings.Contains(err.Error(), wantWrapped) {
			t.Errorf("error %q does not contain wrapped error %q", err.Error(), wantWrapped)
		}
	})

	t.Run("does not retry on failure", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("server start failed")}
		client := tmux.NewClient(mock)

		_ = client.StartServer()

		if len(mock.Calls) != 1 {
			t.Errorf("expected exactly 1 call (no retry), got %d", len(mock.Calls))
		}
	})
}

func TestEnsureServer(t *testing.T) {
	t.Run("returns false when server is already running", func(t *testing.T) {
		mock := &MockCommander{
			RunFunc: func(args ...string) (string, error) {
				if args[0] == "info" {
					return "", nil // server is running
				}
				t.Fatalf("unexpected command: %v", args)
				return "", nil
			},
		}
		client := tmux.NewClient(mock)

		started, err := client.EnsureServer()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if started {
			t.Error("EnsureServer() started = true, want false")
		}
	})

	t.Run("starts server and returns true when server is not running", func(t *testing.T) {
		mock := &MockCommander{
			RunFunc: func(args ...string) (string, error) {
				if args[0] == "info" {
					return "", fmt.Errorf("no server running")
				}
				if args[0] == "new-session" {
					return "", nil
				}
				t.Fatalf("unexpected command: %v", args)
				return "", nil
			},
		}
		client := tmux.NewClient(mock)

		started, err := client.EnsureServer()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !started {
			t.Error("EnsureServer() started = false, want true")
		}
	})

	t.Run("returns true and error when start-server fails", func(t *testing.T) {
		mock := &MockCommander{
			RunFunc: func(args ...string) (string, error) {
				if args[0] == "info" {
					return "", fmt.Errorf("no server running")
				}
				if args[0] == "new-session" {
					return "", fmt.Errorf("start failed")
				}
				t.Fatalf("unexpected command: %v", args)
				return "", nil
			},
		}
		client := tmux.NewClient(mock)

		started, err := client.EnsureServer()

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !started {
			t.Error("EnsureServer() started = false, want true")
		}
	})

	t.Run("does not call start-server when server is running", func(t *testing.T) {
		mock := &MockCommander{
			RunFunc: func(args ...string) (string, error) {
				if args[0] == "info" {
					return "", nil // server is running
				}
				return "", nil
			},
		}
		client := tmux.NewClient(mock)

		_, _ = client.EnsureServer()

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		if mock.Calls[0][0] != "info" {
			t.Errorf("expected call to %q, got %q", "info", mock.Calls[0][0])
		}
	})
}

func TestRenameSession(t *testing.T) {
	t.Run("runs rename-session with old and new name", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		err := client.RenameSession("old-name", "new-name")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		wantArgs := "rename-session -t old-name new-name"
		gotArgs := strings.Join(mock.Calls[0], " ")
		if gotArgs != wantArgs {
			t.Errorf("called with %q, want %q", gotArgs, wantArgs)
		}
	})

	t.Run("returns error when tmux command fails", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("session not found")}
		client := tmux.NewClient(mock)

		err := client.RenameSession("old-name", "new-name")

		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestSetServerOption(t *testing.T) {
	t.Run("runs set-option -s with name and value", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		err := client.SetServerOption("@portal-active-%3", "1")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		wantArgs := "set-option -s @portal-active-%3 1"
		gotArgs := strings.Join(mock.Calls[0], " ")
		if gotArgs != wantArgs {
			t.Errorf("called with %q, want %q", gotArgs, wantArgs)
		}
	})

	t.Run("returns error when tmux command fails", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("tmux error")}
		client := tmux.NewClient(mock)

		err := client.SetServerOption("@portal-active-%3", "1")

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to set server option") {
			t.Errorf("error %q does not contain expected message", err.Error())
		}
		if !strings.Contains(err.Error(), "@portal-active-%3") {
			t.Errorf("error %q does not contain option name", err.Error())
		}
	})
}

func TestGetServerOption(t *testing.T) {
	t.Run("returns value when option exists", func(t *testing.T) {
		mock := &MockCommander{Output: "1"}
		client := tmux.NewClient(mock)

		got, err := client.GetServerOption("@portal-active-%3")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "1" {
			t.Errorf("GetServerOption() = %q, want %q", got, "1")
		}

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		wantArgs := "show-option -sv @portal-active-%3"
		gotArgs := strings.Join(mock.Calls[0], " ")
		if gotArgs != wantArgs {
			t.Errorf("called with %q, want %q", gotArgs, wantArgs)
		}
	})

	t.Run("returns ErrOptionNotFound when option does not exist", func(t *testing.T) {
		mock := &MockCommander{Err: errors.New("unknown option: @portal-active-%3")}
		client := tmux.NewClient(mock)

		got, err := client.GetServerOption("@portal-active-%3")

		if got != "" {
			t.Errorf("GetServerOption() = %q, want empty string", got)
		}
		if !errors.Is(err, tmux.ErrOptionNotFound) {
			t.Errorf("GetServerOption() error = %v, want ErrOptionNotFound", err)
		}
	})
}

func TestDeleteServerOption(t *testing.T) {
	t.Run("runs set-option -su with name", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		err := client.DeleteServerOption("@portal-active-%3")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		wantArgs := "set-option -su @portal-active-%3"
		gotArgs := strings.Join(mock.Calls[0], " ")
		if gotArgs != wantArgs {
			t.Errorf("called with %q, want %q", gotArgs, wantArgs)
		}
	})

	t.Run("succeeds when option does not exist", func(t *testing.T) {
		mock := &MockCommander{} // tmux set-option -su does not error for missing options
		client := tmux.NewClient(mock)

		err := client.DeleteServerOption("@nonexistent-option")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("returns error when tmux command fails", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("tmux error")}
		client := tmux.NewClient(mock)

		err := client.DeleteServerOption("@portal-active-%3")

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to delete server option") {
			t.Errorf("error %q does not contain expected message", err.Error())
		}
		if !strings.Contains(err.Error(), "@portal-active-%3") {
			t.Errorf("error %q does not contain option name", err.Error())
		}
	})
}

func TestListPanes(t *testing.T) {
	t.Run("returns structural keys for session with multiple panes", func(t *testing.T) {
		mock := &MockCommander{Output: "my-session:0.0\nmy-session:0.1\nmy-session:0.2"}
		client := tmux.NewClient(mock)

		got, err := client.ListPanes("my-session")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []string{"my-session:0.0", "my-session:0.1", "my-session:0.2"}
		if len(got) != len(want) {
			t.Fatalf("got %d panes, want %d", len(got), len(want))
		}
		for i, pane := range got {
			if pane != want[i] {
				t.Errorf("pane[%d] = %q, want %q", i, pane, want[i])
			}
		}

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		wantArgs := "list-panes -t my-session -F #{session_name}:#{window_index}.#{pane_index}"
		gotArgs := strings.Join(mock.Calls[0], " ")
		if gotArgs != wantArgs {
			t.Errorf("called with %q, want %q", gotArgs, wantArgs)
		}
	})

	t.Run("returns structural keys for multi-window multi-pane session", func(t *testing.T) {
		mock := &MockCommander{Output: "my-session:0.0\nmy-session:0.1\nmy-session:1.0\nmy-session:1.1"}
		client := tmux.NewClient(mock)

		got, err := client.ListPanes("my-session")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []string{"my-session:0.0", "my-session:0.1", "my-session:1.0", "my-session:1.1"}
		if len(got) != len(want) {
			t.Fatalf("got %d panes, want %d", len(got), len(want))
		}
		for i, pane := range got {
			if pane != want[i] {
				t.Errorf("pane[%d] = %q, want %q", i, pane, want[i])
			}
		}
	})

	t.Run("handles session names with colons", func(t *testing.T) {
		mock := &MockCommander{Output: "my:project:0.0\nmy:project:0.1"}
		client := tmux.NewClient(mock)

		got, err := client.ListPanes("my:project")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []string{"my:project:0.0", "my:project:0.1"}
		if len(got) != len(want) {
			t.Fatalf("got %d panes, want %d", len(got), len(want))
		}
		for i, pane := range got {
			if pane != want[i] {
				t.Errorf("pane[%d] = %q, want %q", i, pane, want[i])
			}
		}
	})

	t.Run("handles session names with dots", func(t *testing.T) {
		mock := &MockCommander{Output: "my.project:0.0"}
		client := tmux.NewClient(mock)

		got, err := client.ListPanes("my.project")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []string{"my.project:0.0"}
		if len(got) != len(want) {
			t.Fatalf("got %d panes, want %d", len(got), len(want))
		}
		if got[0] != want[0] {
			t.Errorf("pane[0] = %q, want %q", got[0], want[0])
		}
	})

	t.Run("returns empty slice when session has no panes", func(t *testing.T) {
		mock := &MockCommander{Output: ""}
		client := tmux.NewClient(mock)

		got, err := client.ListPanes("empty-session")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(got) != 0 {
			t.Fatalf("got %d panes, want 0", len(got))
		}
	})

	t.Run("returns error when session does not exist", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("can't find session: nonexistent")}
		client := tmux.NewClient(mock)

		_, err := client.ListPanes("nonexistent")

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to list panes") {
			t.Errorf("error %q does not contain expected message", err.Error())
		}
		if !strings.Contains(err.Error(), "nonexistent") {
			t.Errorf("error %q does not contain session name", err.Error())
		}
	})
}

func TestListAllPanes(t *testing.T) {
	t.Run("returns structural keys across multiple sessions", func(t *testing.T) {
		mock := &MockCommander{Output: "dev-abc:0.0\ndev-abc:0.1\nwork-xyz:0.0\nwork-xyz:1.0"}
		client := tmux.NewClient(mock)

		got, err := client.ListAllPanes()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []string{"dev-abc:0.0", "dev-abc:0.1", "work-xyz:0.0", "work-xyz:1.0"}
		if len(got) != len(want) {
			t.Fatalf("got %d panes, want %d", len(got), len(want))
		}
		for i, pane := range got {
			if pane != want[i] {
				t.Errorf("pane[%d] = %q, want %q", i, pane, want[i])
			}
		}
	})

	t.Run("returns structural keys for multi-window multi-pane session", func(t *testing.T) {
		mock := &MockCommander{Output: "proj:0.0\nproj:0.1\nproj:1.0\nproj:1.1\nproj:2.0"}
		client := tmux.NewClient(mock)

		got, err := client.ListAllPanes()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []string{"proj:0.0", "proj:0.1", "proj:1.0", "proj:1.1", "proj:2.0"}
		if len(got) != len(want) {
			t.Fatalf("got %d panes, want %d", len(got), len(want))
		}
		for i, pane := range got {
			if pane != want[i] {
				t.Errorf("pane[%d] = %q, want %q", i, pane, want[i])
			}
		}
	})

	t.Run("handles session names with colons", func(t *testing.T) {
		mock := &MockCommander{Output: "my:project:0.0\nmy:project:0.1"}
		client := tmux.NewClient(mock)

		got, err := client.ListAllPanes()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []string{"my:project:0.0", "my:project:0.1"}
		if len(got) != len(want) {
			t.Fatalf("got %d panes, want %d", len(got), len(want))
		}
		for i, pane := range got {
			if pane != want[i] {
				t.Errorf("pane[%d] = %q, want %q", i, pane, want[i])
			}
		}
	})

	t.Run("handles session names with dots", func(t *testing.T) {
		mock := &MockCommander{Output: "my.project:0.0"}
		client := tmux.NewClient(mock)

		got, err := client.ListAllPanes()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []string{"my.project:0.0"}
		if len(got) != len(want) {
			t.Fatalf("got %d panes, want %d", len(got), len(want))
		}
		if got[0] != want[0] {
			t.Errorf("pane[0] = %q, want %q", got[0], want[0])
		}
	})

	t.Run("returns empty slice when no tmux server running", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("no server running on /tmp/tmux-501/default")}
		client := tmux.NewClient(mock)

		got, err := client.ListAllPanes()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("got %d panes, want 0", len(got))
		}
	})

	t.Run("returns empty slice when output is empty", func(t *testing.T) {
		mock := &MockCommander{Output: ""}
		client := tmux.NewClient(mock)

		got, err := client.ListAllPanes()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("got %d panes, want 0", len(got))
		}
	})

	t.Run("calls list-panes with -a flag and structural key format", func(t *testing.T) {
		mock := &MockCommander{Output: "sess:0.0"}
		client := tmux.NewClient(mock)

		_, _ = client.ListAllPanes()

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		wantArgs := "list-panes -a -F #{session_name}:#{window_index}.#{pane_index}"
		gotArgs := strings.Join(mock.Calls[0], " ")
		if gotArgs != wantArgs {
			t.Errorf("called with %q, want %q", gotArgs, wantArgs)
		}
	})
}

func TestResolveStructuralKey(t *testing.T) {
	t.Run("returns structural key for valid pane ID", func(t *testing.T) {
		mock := &MockCommander{Output: "my-project:0.1"}
		client := tmux.NewClient(mock)

		got, err := client.ResolveStructuralKey("%3")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "my-project:0.1" {
			t.Errorf("ResolveStructuralKey() = %q, want %q", got, "my-project:0.1")
		}

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		wantArgs := "display-message -p -t %3 #{session_name}:#{window_index}.#{pane_index}"
		gotArgs := strings.Join(mock.Calls[0], " ")
		if gotArgs != wantArgs {
			t.Errorf("called with %q, want %q", gotArgs, wantArgs)
		}
	})

	t.Run("returns error for invalid pane ID", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("can't find pane: %%99")}
		client := tmux.NewClient(mock)

		_, err := client.ResolveStructuralKey("%99")

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "%99") {
			t.Errorf("error %q does not contain pane ID", err.Error())
		}
	})

	t.Run("returns error when tmux command fails", func(t *testing.T) {
		mock := &MockCommander{Err: fmt.Errorf("no server running")}
		client := tmux.NewClient(mock)

		_, err := client.ResolveStructuralKey("%0")

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to resolve structural key") {
			t.Errorf("error %q does not contain expected message", err.Error())
		}
		if !strings.Contains(err.Error(), "%0") {
			t.Errorf("error %q does not contain pane ID", err.Error())
		}
	})
}

func TestEnsureServerThenListSessions(t *testing.T) {
	t.Run("bootstrap session is queryable and server is running after EnsureServer starts server", func(t *testing.T) {
		infoCallCount := 0
		mock := &MockCommander{
			RunFunc: func(args ...string) (string, error) {
				switch args[0] {
				case "info":
					infoCallCount++
					if infoCallCount == 1 {
						return "", fmt.Errorf("no server running")
					}
					return "", nil
				case "new-session":
					return "", nil
				case "list-sessions":
					return "0|1|0", nil
				default:
					t.Fatalf("unexpected command: %v", args)
					return "", nil
				}
			},
		}
		client := tmux.NewClient(mock)

		// Step 1: EnsureServer should start the server
		started, err := client.EnsureServer()
		if err != nil {
			t.Fatalf("EnsureServer() unexpected error: %v", err)
		}
		if !started {
			t.Error("EnsureServer() started = false, want true")
		}

		// Step 2: ListSessions should return the bootstrap session
		sessions, err := client.ListSessions()
		if err != nil {
			t.Fatalf("ListSessions() unexpected error: %v", err)
		}
		if len(sessions) != 1 {
			t.Fatalf("ListSessions() returned %d sessions, want 1", len(sessions))
		}
		if sessions[0].Name != "0" {
			t.Errorf("session.Name = %q, want %q", sessions[0].Name, "0")
		}
		if sessions[0].Windows != 1 {
			t.Errorf("session.Windows = %d, want 1", sessions[0].Windows)
		}
		if sessions[0].Attached {
			t.Error("session.Attached = true, want false")
		}

		// Step 3: ServerRunning should return true
		if !client.ServerRunning() {
			t.Error("ServerRunning() = false, want true")
		}

		// Verify exactly 4 mock calls in correct order
		if len(mock.Calls) != 4 {
			t.Fatalf("expected 4 calls, got %d: %v", len(mock.Calls), mock.Calls)
		}

		wantCalls := [][]string{
			{"info"},
			{"new-session", "-d"},
			{"list-sessions", "-F", "#{session_name}|#{session_windows}|#{session_attached}"},
			{"info"},
		}
		for i, wantArgs := range wantCalls {
			gotArgs := strings.Join(mock.Calls[i], " ")
			wantJoined := strings.Join(wantArgs, " ")
			if gotArgs != wantJoined {
				t.Errorf("call[%d] = %q, want %q", i, gotArgs, wantJoined)
			}
		}
	})
}

func TestSendKeys(t *testing.T) {
	t.Run("sends command followed by Enter to pane", func(t *testing.T) {
		mock := &MockCommander{}
		client := tmux.NewClient(mock)

		err := client.SendKeys("%3", "claude --resume abc123")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(mock.Calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(mock.Calls))
		}
		wantArgs := []string{"send-keys", "-t", "%3", "claude --resume abc123", "Enter"}
		if len(mock.Calls[0]) != len(wantArgs) {
			t.Fatalf("got %d args %v, want %d args %v", len(mock.Calls[0]), mock.Calls[0], len(wantArgs), wantArgs)
		}
		for i, arg := range mock.Calls[0] {
			if arg != wantArgs[i] {
				t.Errorf("args[%d] = %q, want %q", i, arg, wantArgs[i])
			}
		}
	})

	t.Run("returns error when pane does not exist", func(t *testing.T) {
		mock := &MockCommander{Err: errors.New("can't find pane: %99")}
		client := tmux.NewClient(mock)

		err := client.SendKeys("%99", "some-command")

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to send keys") {
			t.Errorf("error %q does not contain expected message", err.Error())
		}
		if !strings.Contains(err.Error(), "%99") {
			t.Errorf("error %q does not contain pane ID", err.Error())
		}
	})
}
