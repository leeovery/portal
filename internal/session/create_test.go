package session_test

import (
	"fmt"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/leeovery/portal/internal/session"
)

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
	upsertCount int
	upsertErr   error
}

func (m *mockProjectStore) Upsert(path, name string) error {
	m.upsertPath = path
	m.upsertName = name
	m.upsertCount++
	return m.upsertErr
}

// mockTmuxClient implements session.TmuxClient for testing.
type mockTmuxClient struct {
	existingSessions map[string]bool
	newSessionName   string
	newSessionDir    string
	newSessionErr    error
}

func (m *mockTmuxClient) HasSession(name string) bool {
	return m.existingSessions[name]
}

func (m *mockTmuxClient) NewSession(name, dir string) error {
	m.newSessionName = name
	m.newSessionDir = dir
	return m.newSessionErr
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

		_, err := creator.CreateFromDir(subDir)
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

		_, err := creator.CreateFromDir(dir)
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

		sessionName, err := creator.CreateFromDir(dir)
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

		_, err := creator.CreateFromDir(gitRoot)
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

		sessionName, err := creator.CreateFromDir(dir)
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

		_, err := creator.CreateFromDir("/nonexistent/path")
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

		_, err := creator.CreateFromDir(dir)
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

		_, err := creator.CreateFromDir(dir)
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

		_, err := creator.CreateFromDir(dir)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("returns the generated session name on success", func(t *testing.T) {
		dir := t.TempDir()
		gitResolver := &mockGitResolver{}
		store := &mockProjectStore{}
		tmuxClient := &mockTmuxClient{existingSessions: map[string]bool{}}
		gen := func() (string, error) { return "z9y8x7", nil }

		creator := session.NewSessionCreator(gitResolver, store, tmuxClient, gen)

		sessionName, err := creator.CreateFromDir(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		wantName := filepath.Base(dir) + "-z9y8x7"
		if sessionName != wantName {
			t.Errorf("session name = %q, want %q", sessionName, wantName)
		}
	})
}
