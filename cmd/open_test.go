package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/leeovery/portal/internal/browser"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/resolver"
	"github.com/leeovery/portal/internal/session"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui"
	"github.com/spf13/cobra"
)

// testAliasLookup implements resolver.AliasLookup for testing.
type testAliasLookup struct {
	aliases map[string]string
}

func (t *testAliasLookup) Get(name string) (string, bool) {
	path, ok := t.aliases[name]
	return path, ok
}

// testZoxideQuerier implements resolver.ZoxideQuerier for testing.
type testZoxideQuerier struct {
	result string
	err    error
}

func (t *testZoxideQuerier) Query(terms string) (string, error) {
	return t.result, t.err
}

// testDirValidator implements resolver.DirValidator for testing.
type testDirValidator struct {
	existing map[string]bool
}

func (t *testDirValidator) Exists(path string) bool {
	return t.existing[path]
}

func TestOpenCommand_PathArgument_NonExistentPath(t *testing.T) {
	resetRootCmd()
	buf := new(bytes.Buffer)
	rootCmd.SetErr(buf)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"open", "/nonexistent/path/that/does/not/exist"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for non-existent path, got nil")
	}

	want := "Directory not found: /nonexistent/path/that/does/not/exist"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestOpenCommand_PathArgument_FileNotDirectory(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(filePath, []byte("content"), 0o644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	resetRootCmd()
	buf := new(bytes.Buffer)
	rootCmd.SetErr(buf)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"open", filePath})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for file path, got nil")
	}

	want := "not a directory: " + filePath
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestOpenCommand_PathArgument_SkipsTUI(t *testing.T) {
	// When a path argument is given, the TUI should not be launched.
	// We verify this by checking that IsPathArgument returns true for the arg,
	// and the command enters the path resolution branch.
	// A valid directory that exists will proceed to session creation, which
	// requires tmux -- so we test the path detection logic independently.
	if !resolver.IsPathArgument(".") {
		t.Error("expected IsPathArgument(\".\") to return true")
	}
	if !resolver.IsPathArgument("./subdir") {
		t.Error("expected IsPathArgument(\"./subdir\") to return true")
	}
	if !resolver.IsPathArgument("~/Code") {
		t.Error("expected IsPathArgument(\"~/Code\") to return true")
	}
	if resolver.IsPathArgument("myproject") {
		t.Error("expected IsPathArgument(\"myproject\") to return false")
	}
}

func TestOpenCommand_QueryResolution_AliasNotFound(t *testing.T) {
	// When a non-path query resolves to an alias that points to a non-existent directory,
	// the error message should indicate the directory was not found.
	openDeps = &OpenDeps{
		AliasLookup:  &testAliasLookup{aliases: map[string]string{"myapp": "/nonexistent/alias/path"}},
		Zoxide:       &testZoxideQuerier{err: resolver.ErrNoMatch},
		DirValidator: &testDirValidator{existing: map[string]bool{}},
	}
	t.Cleanup(func() { openDeps = nil })

	resetRootCmd()
	buf := new(bytes.Buffer)
	rootCmd.SetErr(buf)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"open", "myapp"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for non-existent alias path, got nil")
	}

	want := "Directory not found: /nonexistent/alias/path"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestOpenCommand_QueryResolution_ZoxideNotFound(t *testing.T) {
	// When a non-path query resolves via zoxide to a non-existent directory,
	// the error message should indicate the directory was not found.
	openDeps = &OpenDeps{
		AliasLookup:  &testAliasLookup{aliases: map[string]string{}},
		Zoxide:       &testZoxideQuerier{result: "/gone/zoxide/dir"},
		DirValidator: &testDirValidator{existing: map[string]bool{}},
	}
	t.Cleanup(func() { openDeps = nil })

	resetRootCmd()
	buf := new(bytes.Buffer)
	rootCmd.SetErr(buf)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"open", "myquery"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for non-existent zoxide path, got nil")
	}

	want := "Directory not found: /gone/zoxide/dir"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

// mockSwitchClient implements the SwitchClienter interface for testing.
type mockSwitchClient struct {
	switchedTo string
	err        error
}

func (m *mockSwitchClient) SwitchClient(name string) error {
	m.switchedTo = name
	return m.err
}

func TestSwitchConnector(t *testing.T) {
	t.Run("calls SwitchClient with session name", func(t *testing.T) {
		mock := &mockSwitchClient{}
		connector := &SwitchConnector{client: mock}

		err := connector.Connect("my-session")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if mock.switchedTo != "my-session" {
			t.Errorf("SwitchClient called with %q, want %q", mock.switchedTo, "my-session")
		}
	})

	t.Run("returns error when SwitchClient fails", func(t *testing.T) {
		mock := &mockSwitchClient{err: fmt.Errorf("session not found")}
		connector := &SwitchConnector{client: mock}

		err := connector.Connect("nonexistent")

		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

// mockSessionCreator implements the sessionCreatorIface for testing.
type mockSessionCreator struct {
	createdDir     string
	createdCommand []string
	sessionName    string
	err            error
}

func (m *mockSessionCreator) CreateFromDir(dir string, command []string) (string, error) {
	m.createdDir = dir
	m.createdCommand = command
	return m.sessionName, m.err
}

// mockQuickStarter implements the quickStarter interface for testing.
type mockQuickStarter struct {
	ranPath    string
	ranCommand []string
	result     *session.QuickStartResult
	err        error
}

func (m *mockQuickStarter) Run(path string, command []string) (*session.QuickStartResult, error) {
	m.ranPath = path
	m.ranCommand = command
	return m.result, m.err
}

// mockExecer implements the execer interface for testing.
type mockExecer struct {
	calledPath string
	calledArgs []string
	calledEnv  []string
	err        error
}

func (m *mockExecer) Exec(argv0 string, argv []string, envv []string) error {
	m.calledPath = argv0
	m.calledArgs = argv
	m.calledEnv = envv
	return m.err
}

func TestPathOpener(t *testing.T) {
	t.Run("inside tmux creates session detached then switches", func(t *testing.T) {
		creator := &mockSessionCreator{sessionName: "myproject-abc123"}
		switcher := &mockSwitchClient{}
		qs := &mockQuickStarter{}
		execer := &mockExecer{}

		opener := &PathOpener{
			insideTmux: true,
			creator:    creator,
			switcher:   switcher,
			qs:         qs,
			execer:     execer,
		}

		err := opener.Open("/home/user/project", nil)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify detached session creation
		if creator.createdDir != "/home/user/project" {
			t.Errorf("CreateFromDir called with %q, want %q", creator.createdDir, "/home/user/project")
		}

		// Verify switch-client called with correct session name
		if switcher.switchedTo != "myproject-abc123" {
			t.Errorf("SwitchClient called with %q, want %q", switcher.switchedTo, "myproject-abc123")
		}

		// Verify no exec happened
		if execer.calledPath != "" {
			t.Errorf("exec was called with %q, expected no exec inside tmux", execer.calledPath)
		}
	})

	t.Run("outside tmux creates session with exec handoff", func(t *testing.T) {
		creator := &mockSessionCreator{}
		switcher := &mockSwitchClient{}
		qs := &mockQuickStarter{
			result: &session.QuickStartResult{
				SessionName: "myproject-abc123",
				Dir:         "/home/user/project",
				ExecArgs:    []string{"tmux", "new-session", "-A", "-s", "myproject-abc123", "-c", "/home/user/project"},
			},
		}
		execer := &mockExecer{}

		opener := &PathOpener{
			insideTmux: false,
			creator:    creator,
			switcher:   switcher,
			qs:         qs,
			execer:     execer,
			tmuxPath:   "/usr/bin/tmux",
		}

		err := opener.Open("/home/user/project", nil)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify QuickStart was called
		if qs.ranPath != "/home/user/project" {
			t.Errorf("QuickStart.Run called with %q, want %q", qs.ranPath, "/home/user/project")
		}

		// Verify exec was called with correct args
		if execer.calledPath != "/usr/bin/tmux" {
			t.Errorf("exec path = %q, want %q", execer.calledPath, "/usr/bin/tmux")
		}
		wantArgs := []string{"tmux", "new-session", "-A", "-s", "myproject-abc123", "-c", "/home/user/project"}
		if len(execer.calledArgs) != len(wantArgs) {
			t.Fatalf("exec args = %v, want %v", execer.calledArgs, wantArgs)
		}
		for i, arg := range execer.calledArgs {
			if arg != wantArgs[i] {
				t.Errorf("exec args[%d] = %q, want %q", i, arg, wantArgs[i])
			}
		}

		// Verify CreateFromDir was NOT called (outside tmux uses QuickStart)
		if creator.createdDir != "" {
			t.Errorf("CreateFromDir should not be called outside tmux, but was called with %q", creator.createdDir)
		}
	})

	t.Run("inside tmux switch-client called with correct session name", func(t *testing.T) {
		creator := &mockSessionCreator{sessionName: "portal-z9y8x7"}
		switcher := &mockSwitchClient{}

		opener := &PathOpener{
			insideTmux: true,
			creator:    creator,
			switcher:   switcher,
			qs:         &mockQuickStarter{},
			execer:     &mockExecer{},
		}

		err := opener.Open("/some/dir", nil)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if switcher.switchedTo != "portal-z9y8x7" {
			t.Errorf("SwitchClient called with %q, want %q", switcher.switchedTo, "portal-z9y8x7")
		}
	})

	t.Run("inside tmux returns error when session creation fails", func(t *testing.T) {
		creator := &mockSessionCreator{err: fmt.Errorf("tmux error")}
		switcher := &mockSwitchClient{}

		opener := &PathOpener{
			insideTmux: true,
			creator:    creator,
			switcher:   switcher,
			qs:         &mockQuickStarter{},
			execer:     &mockExecer{},
		}

		err := opener.Open("/some/dir", nil)

		if err == nil {
			t.Fatal("expected error, got nil")
		}

		// Verify switch-client was NOT called
		if switcher.switchedTo != "" {
			t.Errorf("SwitchClient should not be called when creation fails, but was called with %q", switcher.switchedTo)
		}
	})

	t.Run("inside tmux returns error when switch-client fails", func(t *testing.T) {
		creator := &mockSessionCreator{sessionName: "myproject-abc123"}
		switcher := &mockSwitchClient{err: fmt.Errorf("switch failed")}

		opener := &PathOpener{
			insideTmux: true,
			creator:    creator,
			switcher:   switcher,
			qs:         &mockQuickStarter{},
			execer:     &mockExecer{},
		}

		err := opener.Open("/some/dir", nil)

		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("inside tmux passes command to session creator", func(t *testing.T) {
		creator := &mockSessionCreator{sessionName: "myproject-abc123"}
		switcher := &mockSwitchClient{}

		opener := &PathOpener{
			insideTmux: true,
			creator:    creator,
			switcher:   switcher,
			qs:         &mockQuickStarter{},
			execer:     &mockExecer{},
		}

		command := []string{"claude", "--resume"}
		err := opener.Open("/home/user/project", command)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(creator.createdCommand) != len(command) {
			t.Fatalf("command = %v, want %v", creator.createdCommand, command)
		}
		for i, arg := range creator.createdCommand {
			if arg != command[i] {
				t.Errorf("command[%d] = %q, want %q", i, arg, command[i])
			}
		}
	})

	t.Run("outside tmux passes command to quickstart", func(t *testing.T) {
		qs := &mockQuickStarter{
			result: &session.QuickStartResult{
				SessionName: "myproject-abc123",
				Dir:         "/home/user/project",
				ExecArgs:    []string{"tmux", "new-session", "-A", "-s", "myproject-abc123", "-c", "/home/user/project", "/bin/zsh -ic 'claude --resume; exec /bin/zsh'"},
			},
		}
		execer := &mockExecer{}

		opener := &PathOpener{
			insideTmux: false,
			creator:    &mockSessionCreator{},
			switcher:   &mockSwitchClient{},
			qs:         qs,
			execer:     execer,
			tmuxPath:   "/usr/bin/tmux",
		}

		command := []string{"claude", "--resume"}
		err := opener.Open("/home/user/project", command)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(qs.ranCommand) != len(command) {
			t.Fatalf("command = %v, want %v", qs.ranCommand, command)
		}
		for i, arg := range qs.ranCommand {
			if arg != command[i] {
				t.Errorf("command[%d] = %q, want %q", i, arg, command[i])
			}
		}
	})

	t.Run("outside tmux returns error when quickstart fails", func(t *testing.T) {
		qs := &mockQuickStarter{err: fmt.Errorf("git error")}

		opener := &PathOpener{
			insideTmux: false,
			creator:    &mockSessionCreator{},
			switcher:   &mockSwitchClient{},
			qs:         qs,
			execer:     &mockExecer{},
			tmuxPath:   "/usr/bin/tmux",
		}

		err := opener.Open("/some/dir", nil)

		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

// newTestOpenCmd creates a fresh cobra command with the -e/--exec flag for testing
// parseCommandArgs in isolation, avoiding state leaks between subtests.
func newTestOpenCmd() (*cobra.Command, *cobra.Command) {
	child := &cobra.Command{
		Use:  "open",
		Args: cobra.ArbitraryArgs,
	}
	child.Flags().StringP("exec", "e", "", "command to execute in the new session")

	root := &cobra.Command{Use: "portal", SilenceUsage: true, SilenceErrors: true}
	root.AddCommand(child)

	return root, child
}

func TestParseCommandArgs(t *testing.T) {
	tests := []struct {
		name         string
		args         []string // args to set on the root command (e.g. ["open", "-e", "claude"])
		wantCmd      []string
		wantDest     string
		wantErr      string
		wantUsageErr bool
	}{
		{
			name:     "no flags produces nil command",
			args:     []string{"open"},
			wantCmd:  nil,
			wantDest: "",
		},
		{
			name:     "destination only produces nil command",
			args:     []string{"open", "myproject"},
			wantCmd:  nil,
			wantDest: "myproject",
		},
		{
			name:     "parses -e flag into command slice",
			args:     []string{"open", "-e", "claude"},
			wantCmd:  []string{"claude"},
			wantDest: "",
		},
		{
			name:     "parses --exec flag into command slice",
			args:     []string{"open", "--exec", "claude"},
			wantCmd:  []string{"claude"},
			wantDest: "",
		},
		{
			name:     "destination parsed correctly with -e flag",
			args:     []string{"open", "-e", "claude", "myproject"},
			wantCmd:  []string{"claude"},
			wantDest: "myproject",
		},
		{
			name:     "parses -- args into command slice",
			args:     []string{"open", "--", "claude", "--resume"},
			wantCmd:  []string{"claude", "--resume"},
			wantDest: "",
		},
		{
			name:     "destination parsed correctly with -- syntax",
			args:     []string{"open", "myproject", "--", "claude", "--resume", "--model", "opus"},
			wantCmd:  []string{"claude", "--resume", "--model", "opus"},
			wantDest: "myproject",
		},
		{
			name:         "-e with empty string produces exit code 2",
			args:         []string{"open", "-e", ""},
			wantErr:      "-e/--exec value must not be empty",
			wantUsageErr: true,
		},
		{
			name:         "-- with no arguments produces exit code 2",
			args:         []string{"open", "--"},
			wantErr:      "no command specified after --",
			wantUsageErr: true,
		},
		{
			name:         "both -e and -- produces exit code 2",
			args:         []string{"open", "-e", "vim", "--", "claude", "--resume"},
			wantErr:      "cannot use both -e/--exec and -- to specify a command",
			wantUsageErr: true,
		},
		{
			name:         "-- with destination but no command args produces exit code 2",
			args:         []string{"open", "myproject", "--"},
			wantErr:      "no command specified after --",
			wantUsageErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, child := newTestOpenCmd()

			var gotCmd []string
			var gotDest string
			var gotErr error

			child.RunE = func(cmd *cobra.Command, args []string) error {
				c, d, err := parseCommandArgs(cmd, args)
				gotCmd = c
				gotDest = d
				gotErr = err
				return err
			}

			root.SetArgs(tt.args)
			err := root.Execute()

			if tt.wantErr != "" {
				if gotErr == nil && err == nil {
					t.Fatalf("expected error %q, got nil", tt.wantErr)
				}
				errMsg := ""
				if gotErr != nil {
					errMsg = gotErr.Error()
				} else {
					errMsg = err.Error()
				}
				if errMsg != tt.wantErr {
					t.Errorf("error = %q, want %q", errMsg, tt.wantErr)
				}
				if tt.wantUsageErr {
					checkErr := gotErr
					if checkErr == nil {
						checkErr = err
					}
					var usageErr *UsageError
					if !errors.As(checkErr, &usageErr) {
						t.Errorf("expected UsageError for exit code 2, got %T", checkErr)
					}
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotErr != nil {
				t.Fatalf("unexpected parse error: %v", gotErr)
			}

			// Check command slice
			if tt.wantCmd == nil {
				if gotCmd != nil {
					t.Errorf("command = %v, want nil", gotCmd)
				}
			} else {
				if len(gotCmd) != len(tt.wantCmd) {
					t.Fatalf("command = %v, want %v", gotCmd, tt.wantCmd)
				}
				for i, arg := range gotCmd {
					if arg != tt.wantCmd[i] {
						t.Errorf("command[%d] = %q, want %q", i, arg, tt.wantCmd[i])
					}
				}
			}

			// Check destination
			if gotDest != tt.wantDest {
				t.Errorf("destination = %q, want %q", gotDest, tt.wantDest)
			}
		})
	}
}

// stubSessionLister implements tui.SessionLister for cmd-level testing.
type stubSessionLister struct {
	sessions []tmux.Session
	err      error
}

func (s *stubSessionLister) ListSessions() ([]tmux.Session, error) {
	return s.sessions, s.err
}

// stubProjectStore implements tui.ProjectStore for cmd-level testing.
type stubProjectStore struct {
	projects []project.Project
}

func (s *stubProjectStore) List() ([]project.Project, error) { return s.projects, nil }
func (s *stubProjectStore) CleanStale() ([]project.Project, error) {
	return s.projects, nil
}
func (s *stubProjectStore) Remove(_ string) error { return nil }

// stubSessionKiller implements tui.SessionKiller for cmd-level testing.
type stubSessionKiller struct{}

func (s *stubSessionKiller) KillSession(_ string) error { return nil }

// stubSessionRenamer implements tui.SessionRenamer for cmd-level testing.
type stubSessionRenamer struct{}

func (s *stubSessionRenamer) RenameSession(_, _ string) error { return nil }

// stubTUISessionCreator implements tui.SessionCreator for cmd-level testing.
type stubTUISessionCreator struct{}

func (s *stubTUISessionCreator) CreateFromDir(_ string, _ []string) (string, error) {
	return "stub-session", nil
}

// stubDirLister implements tui.DirLister for cmd-level testing.
type stubDirLister struct{}

func (s *stubDirLister) ListDirectories(_ string, _ bool) ([]browser.DirEntry, error) {
	return nil, nil
}

// mockConnector implements SessionConnector for testing.
type mockConnector struct {
	connectedTo string
	err         error
}

func (m *mockConnector) Connect(name string) error {
	m.connectedTo = name
	return m.err
}

func TestBuildTUIModel(t *testing.T) {
	t.Run("no command and no filter creates default model", func(t *testing.T) {
		cfg := tuiConfig{
			lister:         &stubSessionLister{},
			killer:         &stubSessionKiller{},
			renamer:        &stubSessionRenamer{},
			projectStore:   &stubProjectStore{},
			sessionCreator: &stubTUISessionCreator{},
			dirLister:      &stubDirLister{},
			cwd:            "/home/user",
		}

		m := buildTUIModel(cfg, "", nil)

		if m.Selected() != "" {
			t.Errorf("Selected() = %q, want empty", m.Selected())
		}
		if m.InitialFilter() != "" {
			t.Errorf("InitialFilter() = %q, want empty", m.InitialFilter())
		}
		if m.CommandPending() {
			t.Error("CommandPending() = true, want false")
		}
		if m.InsideTmux() {
			t.Error("InsideTmux() = true, want false")
		}
		if m.ActivePage() != tui.PageSessions {
			t.Errorf("ActivePage() = %d, want PageSessions (0)", m.ActivePage())
		}
	})

	t.Run("command creates model in command-pending mode", func(t *testing.T) {
		cfg := tuiConfig{
			lister:         &stubSessionLister{},
			killer:         &stubSessionKiller{},
			renamer:        &stubSessionRenamer{},
			projectStore:   &stubProjectStore{},
			sessionCreator: &stubTUISessionCreator{},
			dirLister:      &stubDirLister{},
			cwd:            "/home/user",
		}

		m := buildTUIModel(cfg, "", []string{"claude"})

		if !m.CommandPending() {
			t.Error("CommandPending() = false, want true")
		}
		if m.ActivePage() != tui.PageProjects {
			t.Errorf("ActivePage() = %d, want PageProjects (1)", m.ActivePage())
		}
		wantCmd := []string{"claude"}
		gotCmd := m.Command()
		if len(gotCmd) != len(wantCmd) {
			t.Fatalf("Command() = %v, want %v", gotCmd, wantCmd)
		}
		for i, arg := range gotCmd {
			if arg != wantCmd[i] {
				t.Errorf("Command()[%d] = %q, want %q", i, arg, wantCmd[i])
			}
		}
	})

	t.Run("filter creates model with initial filter", func(t *testing.T) {
		cfg := tuiConfig{
			lister:         &stubSessionLister{},
			killer:         &stubSessionKiller{},
			renamer:        &stubSessionRenamer{},
			projectStore:   &stubProjectStore{},
			sessionCreator: &stubTUISessionCreator{},
			dirLister:      &stubDirLister{},
			cwd:            "/home/user",
		}

		m := buildTUIModel(cfg, "myapp", nil)

		if m.InitialFilter() != "myapp" {
			t.Errorf("InitialFilter() = %q, want %q", m.InitialFilter(), "myapp")
		}
		if m.CommandPending() {
			t.Error("CommandPending() = true, want false")
		}
	})

	t.Run("command and filter combines both", func(t *testing.T) {
		cfg := tuiConfig{
			lister:         &stubSessionLister{},
			killer:         &stubSessionKiller{},
			renamer:        &stubSessionRenamer{},
			projectStore:   &stubProjectStore{},
			sessionCreator: &stubTUISessionCreator{},
			dirLister:      &stubDirLister{},
			cwd:            "/home/user",
		}

		m := buildTUIModel(cfg, "myapp", []string{"claude"})

		if m.InitialFilter() != "myapp" {
			t.Errorf("InitialFilter() = %q, want %q", m.InitialFilter(), "myapp")
		}
		if !m.CommandPending() {
			t.Error("CommandPending() = false, want true")
		}
		if m.ActivePage() != tui.PageProjects {
			t.Errorf("ActivePage() = %d, want PageProjects (1)", m.ActivePage())
		}
	})

	t.Run("inside tmux detection passes session name to model", func(t *testing.T) {
		cfg := tuiConfig{
			lister:         &stubSessionLister{},
			killer:         &stubSessionKiller{},
			renamer:        &stubSessionRenamer{},
			projectStore:   &stubProjectStore{},
			sessionCreator: &stubTUISessionCreator{},
			dirLister:      &stubDirLister{},
			cwd:            "/home/user",
			insideTmux:     true,
			currentSession: "my-session",
		}

		m := buildTUIModel(cfg, "", nil)

		if !m.InsideTmux() {
			t.Error("InsideTmux() = false, want true")
		}
		if m.CurrentSession() != "my-session" {
			t.Errorf("CurrentSession() = %q, want %q", m.CurrentSession(), "my-session")
		}
		if m.SessionListTitle() != "Sessions (current: my-session)" {
			t.Errorf("SessionListTitle() = %q, want %q", m.SessionListTitle(), "Sessions (current: my-session)")
		}
	})

	t.Run("cwd wired correctly", func(t *testing.T) {
		cfg := tuiConfig{
			lister:         &stubSessionLister{},
			killer:         &stubSessionKiller{},
			renamer:        &stubSessionRenamer{},
			projectStore:   &stubProjectStore{},
			sessionCreator: &stubTUISessionCreator{},
			dirLister:      &stubDirLister{},
			cwd:            "/home/user/projects",
		}

		m := buildTUIModel(cfg, "", nil)

		if m.CWD() != "/home/user/projects" {
			t.Errorf("CWD() = %q, want %q", m.CWD(), "/home/user/projects")
		}
	})
}

func TestProcessTUIResult(t *testing.T) {
	t.Run("clean exit without selection returns nil", func(t *testing.T) {
		m := tui.New(&stubSessionLister{})
		// m.Selected() is "" by default
		connector := &mockConnector{}

		err := processTUIResult(m, connector)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if connector.connectedTo != "" {
			t.Errorf("connector should not be called on clean exit, but was called with %q", connector.connectedTo)
		}
	})

	t.Run("selected session name forwarded to connector", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "dev", Windows: 3},
		}
		m := tui.NewModelWithSessions(sessions)
		// Simulate user selecting a session via Update with Enter
		updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		m = updated.(tui.Model)
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m = updated.(tui.Model)

		connector := &mockConnector{}

		err := processTUIResult(m, connector)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if connector.connectedTo != "dev" {
			t.Errorf("connector called with %q, want %q", connector.connectedTo, "dev")
		}
	})
}

func TestBuildSessionConnector(t *testing.T) {
	t.Run("returns SwitchConnector when inside tmux", func(t *testing.T) {
		t.Setenv("TMUX", "/tmp/tmux-501/default,12345,0")

		connector := buildSessionConnector()

		if _, ok := connector.(*SwitchConnector); !ok {
			t.Errorf("expected *SwitchConnector, got %T", connector)
		}
	})

	t.Run("returns AttachConnector when outside tmux", func(t *testing.T) {
		t.Setenv("TMUX", "")

		connector := buildSessionConnector()

		if _, ok := connector.(*AttachConnector); !ok {
			t.Errorf("expected *AttachConnector, got %T", connector)
		}
	})
}
