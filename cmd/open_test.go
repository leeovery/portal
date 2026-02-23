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
