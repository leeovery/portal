package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/portal/internal/resolver"
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
	createdDir  string
	sessionName string
	err         error
}

func (m *mockSessionCreator) CreateFromDir(dir string) (string, error) {
	m.createdDir = dir
	return m.sessionName, m.err
}

// mockQuickStarter implements the quickStarter interface for testing.
type mockQuickStarter struct {
	ranPath string
	result  *quickStartResult
	err     error
}

func (m *mockQuickStarter) Run(path string) (*quickStartResult, error) {
	m.ranPath = path
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

		err := opener.Open("/home/user/project")

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
			result: &quickStartResult{
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

		err := opener.Open("/home/user/project")

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

		err := opener.Open("/some/dir")

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

		err := opener.Open("/some/dir")

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

		err := opener.Open("/some/dir")

		if err == nil {
			t.Fatal("expected error, got nil")
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

		err := opener.Open("/some/dir")

		if err == nil {
			t.Fatal("expected error, got nil")
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
