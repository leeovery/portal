package session_test

import (
	"fmt"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/leeovery/portal/internal/session"
)

func TestBuildShellCommand(t *testing.T) {
	tests := []struct {
		name    string
		command []string
		shell   string
		want    string
	}{
		{
			name:    "single word command uses SHELL env var",
			command: []string{"claude"},
			shell:   "/bin/zsh",
			want:    "/bin/zsh -ic 'claude; exec /bin/zsh'",
		},
		{
			name:    "multi-word command joined with spaces",
			command: []string{"claude", "--resume", "--model", "opus"},
			shell:   "/bin/zsh",
			want:    "/bin/zsh -ic 'claude --resume --model opus; exec /bin/zsh'",
		},
		{
			name:    "uses bash when SHELL is bash",
			command: []string{"vim"},
			shell:   "/bin/bash",
			want:    "/bin/bash -ic 'vim; exec /bin/bash'",
		},
		{
			name:    "single quotes in command are escaped",
			command: []string{"echo", "'hello'"},
			shell:   "/bin/zsh",
			want:    "/bin/zsh -ic 'echo '\\''hello'\\''; exec /bin/zsh'",
		},
		{
			name:    "special shell chars passed through",
			command: []string{"ls", "|", "grep", "foo", "&&", "echo", "done"},
			shell:   "/bin/zsh",
			want:    "/bin/zsh -ic 'ls | grep foo && echo done; exec /bin/zsh'",
		},
		{
			name:    "returns empty string for nil command",
			command: nil,
			shell:   "/bin/zsh",
			want:    "",
		},
		{
			name:    "returns empty string for empty command",
			command: []string{},
			shell:   "/bin/zsh",
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := session.BuildShellCommand(tt.command, tt.shell)
			if got != tt.want {
				t.Errorf("BuildShellCommand() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestShellFromEnv(t *testing.T) {
	t.Run("returns SHELL env var when set", func(t *testing.T) {
		t.Setenv("SHELL", "/bin/zsh")
		got := session.ShellFromEnv()
		if got != "/bin/zsh" {
			t.Errorf("ShellFromEnv() = %q, want %q", got, "/bin/zsh")
		}
	})

	t.Run("falls back to /bin/sh when SHELL not set", func(t *testing.T) {
		t.Setenv("SHELL", "")
		got := session.ShellFromEnv()
		if got != "/bin/sh" {
			t.Errorf("ShellFromEnv() = %q, want %q", got, "/bin/sh")
		}
	})
}

// mockGitResolver implements session.GitResolver for testing.
type mockGitResolver struct {
	resolvedDir string
	err         error
}

func (m *mockGitResolver) Resolve(dir string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	if m.resolvedDir != "" {
		return m.resolvedDir, nil
	}
	return dir, nil
}

// mockProjectStore implements session.ProjectStore for testing.
type mockProjectStore struct {
	upsertPath  string
	upsertName  string
	upsertVia   string
	upsertCount int
	upsertErr   error
}

func (m *mockProjectStore) Upsert(path, name, via string) error {
	m.upsertPath = path
	m.upsertName = name
	m.upsertVia = via
	m.upsertCount++
	return m.upsertErr
}

// setOptionCall records a single SetSessionOption invocation. Both @portal-dir
// and @portal-id stamp via SetSessionOption, so tests record ALL calls and
// assert each option independently.
type setOptionCall struct {
	Session string
	Name    string
	Value   string
}

// mockTmuxClient implements session.TmuxClient for testing.
type mockTmuxClient struct {
	existingSessions   map[string]bool
	newSessionName     string
	newSessionDir      string
	newSessionShellCmd string
	newSessionErr      error

	setOptionCalls []setOptionCall
	setOptionErr   error
}

// setOptionCallFor returns the recorded SetSessionOption call for the given
// option name, and whether one was made.
func (m *mockTmuxClient) setOptionCallFor(name string) (setOptionCall, bool) {
	for _, c := range m.setOptionCalls {
		if c.Name == name {
			return c, true
		}
	}
	return setOptionCall{}, false
}

func (m *mockTmuxClient) HasSession(name string) bool {
	return m.existingSessions[name]
}

func (m *mockTmuxClient) NewSession(name, dir, shellCommand string) error {
	m.newSessionName = name
	m.newSessionDir = dir
	m.newSessionShellCmd = shellCommand
	return m.newSessionErr
}

func (m *mockTmuxClient) SetSessionOption(session, name, value string) error {
	m.setOptionCalls = append(m.setOptionCalls, setOptionCall{
		Session: session,
		Name:    name,
		Value:   value,
	})
	return m.setOptionErr
}

func TestCreateFromDir(t *testing.T) {
	namePattern := regexp.MustCompile(`^[a-zA-Z0-9_-]+-[a-zA-Z0-9]{6}$`)

	t.Run("creates session with git-root-resolved directory", func(t *testing.T) {
		gitRoot := t.TempDir()
		subDir := filepath.Join(gitRoot, "subdir")

		gitResolver := &mockGitResolver{resolvedDir: gitRoot}
		store := &mockProjectStore{}
		tmuxClient := &mockTmuxClient{existingSessions: map[string]bool{}}
		gen := func() (string, error) { return "abc123", nil }

		creator := session.NewSessionCreator(gitResolver, store, tmuxClient, gen)

		_, err := creator.CreateFromDir(subDir, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if tmuxClient.newSessionDir != gitRoot {
			t.Errorf("tmux session dir = %q, want %q", tmuxClient.newSessionDir, gitRoot)
		}
	})

	t.Run("derives project name from basename of resolved directory", func(t *testing.T) {
		dir := t.TempDir() // e.g., /tmp/TestXxx/001
		// Resolve to this same dir so basename is used as project name
		gitResolver := &mockGitResolver{}
		store := &mockProjectStore{}
		tmuxClient := &mockTmuxClient{existingSessions: map[string]bool{}}
		gen := func() (string, error) { return "abc123", nil }

		creator := session.NewSessionCreator(gitResolver, store, tmuxClient, gen)

		_, err := creator.CreateFromDir(dir, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		wantName := filepath.Base(dir)
		if store.upsertName != wantName {
			t.Errorf("project name = %q, want %q", store.upsertName, wantName)
		}
	})

	t.Run("generates unique session name with nanoid suffix", func(t *testing.T) {
		dir := t.TempDir()
		gitResolver := &mockGitResolver{}
		store := &mockProjectStore{}
		tmuxClient := &mockTmuxClient{existingSessions: map[string]bool{}}
		gen := func() (string, error) { return "x7k2m9", nil }

		creator := session.NewSessionCreator(gitResolver, store, tmuxClient, gen)

		sessionName, err := creator.CreateFromDir(dir, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !namePattern.MatchString(sessionName) {
			t.Errorf("session name %q does not match pattern {project}-{nanoid}", sessionName)
		}

		wantSuffix := "x7k2m9"
		baseName := filepath.Base(dir)
		wantName := baseName + "-" + wantSuffix
		if sessionName != wantName {
			t.Errorf("session name = %q, want %q", sessionName, wantName)
		}
	})

	t.Run("upserts project in store with resolved path and derived name", func(t *testing.T) {
		gitRoot := t.TempDir()
		gitResolver := &mockGitResolver{resolvedDir: gitRoot}
		store := &mockProjectStore{}
		tmuxClient := &mockTmuxClient{existingSessions: map[string]bool{}}
		gen := func() (string, error) { return "abc123", nil }

		creator := session.NewSessionCreator(gitResolver, store, tmuxClient, gen)

		_, err := creator.CreateFromDir(gitRoot, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if store.upsertPath != gitRoot {
			t.Errorf("upsert path = %q, want %q", store.upsertPath, gitRoot)
		}

		wantName := filepath.Base(gitRoot)
		if store.upsertName != wantName {
			t.Errorf("upsert name = %q, want %q", store.upsertName, wantName)
		}

		// The session-creation pipeline is a code-driven mutation, so the
		// breadcrumb must record via=internal.
		if store.upsertVia != "internal" {
			t.Errorf("upsert via = %q, want %q", store.upsertVia, "internal")
		}
	})

	t.Run("handles tmux server not running by creating session normally", func(t *testing.T) {
		dir := t.TempDir()
		gitResolver := &mockGitResolver{}
		store := &mockProjectStore{}
		// HasSession returns false (no server), NewSession succeeds
		// (tmux new-session -A creates server if needed)
		tmuxClient := &mockTmuxClient{existingSessions: map[string]bool{}}
		gen := func() (string, error) { return "abc123", nil }

		creator := session.NewSessionCreator(gitResolver, store, tmuxClient, gen)

		sessionName, err := creator.CreateFromDir(dir, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if sessionName == "" {
			t.Error("expected non-empty session name")
		}

		if tmuxClient.newSessionName == "" {
			t.Error("expected NewSession to be called")
		}
	})

	t.Run("returns error for non-existent directory", func(t *testing.T) {
		gitResolver := &mockGitResolver{err: fmt.Errorf("directory does not exist: stat /nonexistent: no such file or directory")}
		store := &mockProjectStore{}
		tmuxClient := &mockTmuxClient{existingSessions: map[string]bool{}}
		gen := func() (string, error) { return "abc123", nil }

		creator := session.NewSessionCreator(gitResolver, store, tmuxClient, gen)

		_, err := creator.CreateFromDir("/nonexistent/path", nil)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("returns error when session name generation fails", func(t *testing.T) {
		dir := t.TempDir()
		gitResolver := &mockGitResolver{}
		store := &mockProjectStore{}
		tmuxClient := &mockTmuxClient{existingSessions: map[string]bool{}}
		gen := func() (string, error) { return "", fmt.Errorf("random source exhausted") }

		creator := session.NewSessionCreator(gitResolver, store, tmuxClient, gen)

		_, err := creator.CreateFromDir(dir, nil)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("returns error when project upsert fails", func(t *testing.T) {
		dir := t.TempDir()
		gitResolver := &mockGitResolver{}
		store := &mockProjectStore{upsertErr: fmt.Errorf("disk full")}
		tmuxClient := &mockTmuxClient{existingSessions: map[string]bool{}}
		gen := func() (string, error) { return "abc123", nil }

		creator := session.NewSessionCreator(gitResolver, store, tmuxClient, gen)

		_, err := creator.CreateFromDir(dir, nil)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("returns error when tmux NewSession fails", func(t *testing.T) {
		dir := t.TempDir()
		gitResolver := &mockGitResolver{}
		store := &mockProjectStore{}
		tmuxClient := &mockTmuxClient{
			existingSessions: map[string]bool{},
			newSessionErr:    fmt.Errorf("tmux error"),
		}
		gen := func() (string, error) { return "abc123", nil }

		creator := session.NewSessionCreator(gitResolver, store, tmuxClient, gen)

		_, err := creator.CreateFromDir(dir, nil)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("passes shell-command to tmux when command provided", func(t *testing.T) {
		t.Setenv("SHELL", "/bin/zsh")
		dir := t.TempDir()
		gitResolver := &mockGitResolver{}
		store := &mockProjectStore{}
		tmuxClient := &mockTmuxClient{existingSessions: map[string]bool{}}
		gen := func() (string, error) { return "abc123", nil }

		creator := session.NewSessionCreator(gitResolver, store, tmuxClient, gen)

		_, err := creator.CreateFromDir(dir, []string{"claude", "--resume"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := "/bin/zsh -ic 'claude --resume; exec /bin/zsh'"
		if tmuxClient.newSessionShellCmd != want {
			t.Errorf("shell command = %q, want %q", tmuxClient.newSessionShellCmd, want)
		}
	})

	t.Run("no shell-command passed to tmux when command is nil", func(t *testing.T) {
		dir := t.TempDir()
		gitResolver := &mockGitResolver{}
		store := &mockProjectStore{}
		tmuxClient := &mockTmuxClient{existingSessions: map[string]bool{}}
		gen := func() (string, error) { return "abc123", nil }

		creator := session.NewSessionCreator(gitResolver, store, tmuxClient, gen)

		_, err := creator.CreateFromDir(dir, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if tmuxClient.newSessionShellCmd != "" {
			t.Errorf("shell command = %q, want empty", tmuxClient.newSessionShellCmd)
		}
	})

	t.Run("uses shell resolved at construction time", func(t *testing.T) {
		t.Setenv("SHELL", "/usr/local/bin/fish")
		dir := t.TempDir()
		gitResolver := &mockGitResolver{}
		store := &mockProjectStore{}
		tmuxClient := &mockTmuxClient{existingSessions: map[string]bool{}}
		gen := func() (string, error) { return "abc123", nil }

		creator := session.NewSessionCreator(gitResolver, store, tmuxClient, gen)

		// Change SHELL after construction — should NOT affect the creator
		t.Setenv("SHELL", "/bin/bash")

		_, err := creator.CreateFromDir(dir, []string{"vim"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := "/usr/local/bin/fish -ic 'vim; exec /usr/local/bin/fish'"
		if tmuxClient.newSessionShellCmd != want {
			t.Errorf("shell command = %q, want %q", tmuxClient.newSessionShellCmd, want)
		}
	})

	t.Run("falls back to /bin/sh when SHELL not set", func(t *testing.T) {
		t.Setenv("SHELL", "")
		dir := t.TempDir()
		gitResolver := &mockGitResolver{}
		store := &mockProjectStore{}
		tmuxClient := &mockTmuxClient{existingSessions: map[string]bool{}}
		gen := func() (string, error) { return "abc123", nil }

		creator := session.NewSessionCreator(gitResolver, store, tmuxClient, gen)

		_, err := creator.CreateFromDir(dir, []string{"vim"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := "/bin/sh -ic 'vim; exec /bin/sh'"
		if tmuxClient.newSessionShellCmd != want {
			t.Errorf("shell command = %q, want %q", tmuxClient.newSessionShellCmd, want)
		}
	})

	t.Run("stamps @portal-dir with the resolved git root after creating a session", func(t *testing.T) {
		gitRoot := t.TempDir()
		subDir := filepath.Join(gitRoot, "subdir")

		gitResolver := &mockGitResolver{resolvedDir: gitRoot}
		store := &mockProjectStore{}
		tmuxClient := &mockTmuxClient{existingSessions: map[string]bool{}}
		gen := func() (string, error) { return "abc123", nil }

		creator := session.NewSessionCreator(gitResolver, store, tmuxClient, gen)

		sessionName, err := creator.CreateFromDir(subDir, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		dirCall, ok := tmuxClient.setOptionCallFor(session.PortalDirOption)
		if !ok {
			t.Fatal("expected SetSessionOption to be called for @portal-dir")
		}
		if dirCall.Session != sessionName {
			t.Errorf("stamp session = %q, want %q", dirCall.Session, sessionName)
		}
		if dirCall.Value != gitRoot {
			t.Errorf("stamp value = %q, want %q", dirCall.Value, gitRoot)
		}
	})

	t.Run("returns the session name even when SetSessionOption fails", func(t *testing.T) {
		dir := t.TempDir()
		gitResolver := &mockGitResolver{}
		store := &mockProjectStore{}
		tmuxClient := &mockTmuxClient{
			existingSessions: map[string]bool{},
			setOptionErr:     fmt.Errorf("set-option failed"),
		}
		gen := func() (string, error) { return "abc123", nil }

		creator := session.NewSessionCreator(gitResolver, store, tmuxClient, gen)

		sessionName, err := creator.CreateFromDir(dir, nil)
		if err != nil {
			t.Fatalf("SetSessionOption failure must not fail creation, got error: %v", err)
		}

		wantName := filepath.Base(dir) + "-abc123"
		if sessionName != wantName {
			t.Errorf("session name = %q, want %q", sessionName, wantName)
		}
	})

	t.Run("stamps using the prepared resolved dir, not a re-derived path", func(t *testing.T) {
		gitRoot := t.TempDir()
		subDir := filepath.Join(gitRoot, "a", "b", "c")

		// The resolver maps the deep subdir to the git root. The stamp value
		// must be that resolved root, never the input subdir.
		gitResolver := &mockGitResolver{resolvedDir: gitRoot}
		store := &mockProjectStore{}
		tmuxClient := &mockTmuxClient{existingSessions: map[string]bool{}}
		gen := func() (string, error) { return "abc123", nil }

		creator := session.NewSessionCreator(gitResolver, store, tmuxClient, gen)

		_, err := creator.CreateFromDir(subDir, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		dirCall, ok := tmuxClient.setOptionCallFor(session.PortalDirOption)
		if !ok {
			t.Fatal("expected SetSessionOption to be called for @portal-dir")
		}
		if dirCall.Value != gitRoot {
			t.Errorf("stamp value = %q, want resolved git root %q", dirCall.Value, gitRoot)
		}
		if dirCall.Value == subDir {
			t.Errorf("stamp value must not be the input subdir %q", subDir)
		}
	})

	t.Run("does not stamp at creation when NewSession fails", func(t *testing.T) {
		dir := t.TempDir()
		gitResolver := &mockGitResolver{}
		store := &mockProjectStore{}
		tmuxClient := &mockTmuxClient{
			existingSessions: map[string]bool{},
			newSessionErr:    fmt.Errorf("tmux error"),
		}
		gen := func() (string, error) { return "abc123", nil }

		creator := session.NewSessionCreator(gitResolver, store, tmuxClient, gen)

		_, err := creator.CreateFromDir(dir, nil)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if len(tmuxClient.setOptionCalls) != 0 {
			t.Error("SetSessionOption must not be called when NewSession fails")
		}
	})

	t.Run("stamps @portal-id with a fresh token after creating a session", func(t *testing.T) {
		dir := t.TempDir()
		gitResolver := &mockGitResolver{}
		store := &mockProjectStore{}
		tmuxClient := &mockTmuxClient{existingSessions: map[string]bool{}}
		gen := func() (string, error) { return "abc123", nil }

		creator := session.NewSessionCreator(gitResolver, store, tmuxClient, gen)

		sessionName, err := creator.CreateFromDir(dir, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		idCall, ok := tmuxClient.setOptionCallFor(session.PortalIDOption)
		if !ok {
			t.Fatal("expected SetSessionOption to be called for @portal-id")
		}
		if idCall.Session != sessionName {
			t.Errorf("stamp session = %q, want %q", idCall.Session, sessionName)
		}
		if idCall.Value != "abc123" {
			t.Errorf("stamp value = %q, want fresh token %q", idCall.Value, "abc123")
		}
	})

	t.Run("stamps both @portal-dir and @portal-id on a successful create", func(t *testing.T) {
		gitRoot := t.TempDir()
		gitResolver := &mockGitResolver{resolvedDir: gitRoot}
		store := &mockProjectStore{}
		tmuxClient := &mockTmuxClient{existingSessions: map[string]bool{}}
		gen := func() (string, error) { return "abc123", nil }

		creator := session.NewSessionCreator(gitResolver, store, tmuxClient, gen)

		if _, err := creator.CreateFromDir(gitRoot, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if _, ok := tmuxClient.setOptionCallFor(session.PortalDirOption); !ok {
			t.Error("expected SetSessionOption to be called for @portal-dir")
		}
		if _, ok := tmuxClient.setOptionCallFor(session.PortalIDOption); !ok {
			t.Error("expected SetSessionOption to be called for @portal-id")
		}
	})

	t.Run("returns the session name when the @portal-id stamp SetSessionOption fails", func(t *testing.T) {
		dir := t.TempDir()
		gitResolver := &mockGitResolver{}
		store := &mockProjectStore{}
		tmuxClient := &mockTmuxClient{
			existingSessions: map[string]bool{},
			setOptionErr:     fmt.Errorf("set-option failed"),
		}
		gen := func() (string, error) { return "abc123", nil }

		creator := session.NewSessionCreator(gitResolver, store, tmuxClient, gen)

		sessionName, err := creator.CreateFromDir(dir, nil)
		if err != nil {
			t.Fatalf("@portal-id stamp failure must not fail creation, got error: %v", err)
		}

		wantName := filepath.Base(dir) + "-abc123"
		if sessionName != wantName {
			t.Errorf("session name = %q, want %q", sessionName, wantName)
		}
	})

	t.Run("creates the session un-stamped when stamp-time token generation fails", func(t *testing.T) {
		dir := t.TempDir()
		gitResolver := &mockGitResolver{}
		store := &mockProjectStore{}
		tmuxClient := &mockTmuxClient{existingSessions: map[string]bool{}}
		// The generator succeeds for the session-name generation (first call in
		// PrepareSession) and fails on the stamp-time call (second).
		calls := 0
		gen := func() (string, error) {
			calls++
			if calls == 1 {
				return "abc123", nil
			}
			return "", fmt.Errorf("random source exhausted")
		}

		creator := session.NewSessionCreator(gitResolver, store, tmuxClient, gen)

		sessionName, err := creator.CreateFromDir(dir, nil)
		if err != nil {
			t.Fatalf("stamp-time token generation failure must not fail creation, got error: %v", err)
		}

		wantName := filepath.Base(dir) + "-abc123"
		if sessionName != wantName {
			t.Errorf("session name = %q, want %q", sessionName, wantName)
		}

		if _, ok := tmuxClient.setOptionCallFor(session.PortalIDOption); ok {
			t.Error("@portal-id must not be stamped when stamp-time token generation fails")
		}
	})

	t.Run("does not stamp @portal-id when NewSession fails", func(t *testing.T) {
		dir := t.TempDir()
		gitResolver := &mockGitResolver{}
		store := &mockProjectStore{}
		tmuxClient := &mockTmuxClient{
			existingSessions: map[string]bool{},
			newSessionErr:    fmt.Errorf("tmux error"),
		}
		gen := func() (string, error) { return "abc123", nil }

		creator := session.NewSessionCreator(gitResolver, store, tmuxClient, gen)

		_, err := creator.CreateFromDir(dir, nil)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if _, ok := tmuxClient.setOptionCallFor(session.PortalIDOption); ok {
			t.Error("@portal-id must not be stamped when NewSession fails")
		}
	})

	t.Run("returns the generated session name on success", func(t *testing.T) {
		dir := t.TempDir()
		gitResolver := &mockGitResolver{}
		store := &mockProjectStore{}
		tmuxClient := &mockTmuxClient{existingSessions: map[string]bool{}}
		gen := func() (string, error) { return "z9y8x7", nil }

		creator := session.NewSessionCreator(gitResolver, store, tmuxClient, gen)

		sessionName, err := creator.CreateFromDir(dir, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		wantName := filepath.Base(dir) + "-z9y8x7"
		if sessionName != wantName {
			t.Errorf("session name = %q, want %q", sessionName, wantName)
		}
	})
}
