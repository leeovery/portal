package session_test

import (
	"fmt"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/leeovery/portal/internal/session"
)

// mockSessionChecker implements session.SessionChecker for testing.
type mockSessionChecker struct {
	existingSessions map[string]bool
}

func (m *mockSessionChecker) HasSession(name string) bool {
	return m.existingSessions[name]
}

func TestQuickStart(t *testing.T) {
	namePattern := regexp.MustCompile(`^[a-zA-Z0-9_-]+-[a-zA-Z0-9]{6}$`)

	t.Run("creates session with git-root-resolved directory", func(t *testing.T) {
		gitRoot := t.TempDir()
		subDir := filepath.Join(gitRoot, "subdir")

		gitResolver := &mockGitResolver{resolvedDir: gitRoot}
		store := &mockProjectStore{}
		checker := &mockSessionChecker{existingSessions: map[string]bool{}}
		gen := func() (string, error) { return "abc123", nil }

		qs := session.NewQuickStart(gitResolver, store, checker, gen)

		result, err := qs.Run(subDir, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Dir != gitRoot {
			t.Errorf("result.Dir = %q, want %q", result.Dir, gitRoot)
		}

		// Verify ExecArgs include resolved dir via -c flag
		wantSessionName := filepath.Base(gitRoot) + "-abc123"
		wantArgs := []string{"tmux", "new-session", "-A", "-s", wantSessionName, "-c", gitRoot}
		if len(result.ExecArgs) != len(wantArgs) {
			t.Fatalf("result.ExecArgs = %v, want %v", result.ExecArgs, wantArgs)
		}
		for i, arg := range result.ExecArgs {
			if arg != wantArgs[i] {
				t.Errorf("result.ExecArgs[%d] = %q, want %q", i, arg, wantArgs[i])
			}
		}
	})

	t.Run("registers new project in store", func(t *testing.T) {
		gitRoot := t.TempDir()
		gitResolver := &mockGitResolver{resolvedDir: gitRoot}
		store := &mockProjectStore{}
		checker := &mockSessionChecker{existingSessions: map[string]bool{}}
		gen := func() (string, error) { return "abc123", nil }

		qs := session.NewQuickStart(gitResolver, store, checker, gen)

		_, err := qs.Run(gitRoot, nil)
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

	t.Run("updates last_used for existing project", func(t *testing.T) {
		gitRoot := t.TempDir()
		gitResolver := &mockGitResolver{resolvedDir: gitRoot}
		store := &mockProjectStore{}
		checker := &mockSessionChecker{existingSessions: map[string]bool{}}
		gen := func() (string, error) { return "abc123", nil }

		qs := session.NewQuickStart(gitResolver, store, checker, gen)

		// First call registers the project
		_, err := qs.Run(gitRoot, nil)
		if err != nil {
			t.Fatalf("unexpected error on first run: %v", err)
		}

		// Second call should also call Upsert (which updates last_used)
		_, err = qs.Run(gitRoot, nil)
		if err != nil {
			t.Fatalf("unexpected error on second run: %v", err)
		}

		// Verify Upsert was called twice (once per Run)
		if store.upsertCount != 2 {
			t.Errorf("upsert count = %d, want 2", store.upsertCount)
		}

		if store.upsertPath != gitRoot {
			t.Errorf("upsert path = %q, want %q", store.upsertPath, gitRoot)
		}
	})

	t.Run("exec args use new-session -A for atomic create-or-attach", func(t *testing.T) {
		dir := t.TempDir()
		gitResolver := &mockGitResolver{}
		store := &mockProjectStore{}
		checker := &mockSessionChecker{existingSessions: map[string]bool{}}
		gen := func() (string, error) { return "abc123", nil }

		qs := session.NewQuickStart(gitResolver, store, checker, gen)

		result, err := qs.Run(dir, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		wantSessionName := filepath.Base(dir) + "-abc123"

		if result.SessionName != wantSessionName {
			t.Errorf("result.SessionName = %q, want %q", result.SessionName, wantSessionName)
		}

		// Verify exec args use tmux new-session -A -s <name> -c <dir>
		wantArgs := []string{"tmux", "new-session", "-A", "-s", wantSessionName, "-c", dir}
		if len(result.ExecArgs) != len(wantArgs) {
			t.Fatalf("result.ExecArgs = %v, want %v", result.ExecArgs, wantArgs)
		}
		for i, arg := range result.ExecArgs {
			if arg != wantArgs[i] {
				t.Errorf("result.ExecArgs[%d] = %q, want %q", i, arg, wantArgs[i])
			}
		}
	})

	t.Run("session name follows project-nanoid format", func(t *testing.T) {
		dir := t.TempDir()
		gitResolver := &mockGitResolver{}
		store := &mockProjectStore{}
		checker := &mockSessionChecker{existingSessions: map[string]bool{}}
		gen := func() (string, error) { return "x7k2m9", nil }

		qs := session.NewQuickStart(gitResolver, store, checker, gen)

		result, err := qs.Run(dir, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !namePattern.MatchString(result.SessionName) {
			t.Errorf("session name %q does not match pattern {project}-{nanoid}", result.SessionName)
		}

		wantName := filepath.Base(dir) + "-x7k2m9"
		if result.SessionName != wantName {
			t.Errorf("session name = %q, want %q", result.SessionName, wantName)
		}
	})

	t.Run("project name derived from directory basename after git root resolution", func(t *testing.T) {
		gitRoot := "/tmp/myproject"
		subDir := "/tmp/myproject/src/pkg"

		gitResolver := &mockGitResolver{resolvedDir: gitRoot}
		store := &mockProjectStore{}
		checker := &mockSessionChecker{existingSessions: map[string]bool{}}
		gen := func() (string, error) { return "abc123", nil }

		qs := session.NewQuickStart(gitResolver, store, checker, gen)

		_, err := qs.Run(subDir, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if store.upsertName != "myproject" {
			t.Errorf("project name = %q, want %q", store.upsertName, "myproject")
		}
	})

	t.Run("exec args include shell-command when command provided", func(t *testing.T) {
		t.Setenv("SHELL", "/bin/zsh")
		dir := t.TempDir()
		gitResolver := &mockGitResolver{}
		store := &mockProjectStore{}
		checker := &mockSessionChecker{existingSessions: map[string]bool{}}
		gen := func() (string, error) { return "abc123", nil }

		qs := session.NewQuickStart(gitResolver, store, checker, gen)

		result, err := qs.Run(dir, []string{"claude", "--resume"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		wantSessionName := filepath.Base(dir) + "-abc123"
		shellCmd := "/bin/zsh -ic 'claude --resume; exec /bin/zsh'"
		wantArgs := []string{"tmux", "new-session", "-A", "-s", wantSessionName, "-c", dir, shellCmd}
		if len(result.ExecArgs) != len(wantArgs) {
			t.Fatalf("result.ExecArgs = %v, want %v", result.ExecArgs, wantArgs)
		}
		for i, arg := range result.ExecArgs {
			if arg != wantArgs[i] {
				t.Errorf("result.ExecArgs[%d] = %q, want %q", i, arg, wantArgs[i])
			}
		}
	})

	t.Run("uses shell resolved at construction time", func(t *testing.T) {
		t.Setenv("SHELL", "/usr/local/bin/fish")
		dir := t.TempDir()
		gitResolver := &mockGitResolver{}
		store := &mockProjectStore{}
		checker := &mockSessionChecker{existingSessions: map[string]bool{}}
		gen := func() (string, error) { return "abc123", nil }

		qs := session.NewQuickStart(gitResolver, store, checker, gen)

		// Change SHELL after construction — should NOT affect the QuickStart
		t.Setenv("SHELL", "/bin/bash")

		result, err := qs.Run(dir, []string{"vim"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		wantSessionName := filepath.Base(dir) + "-abc123"
		shellCmd := "/usr/local/bin/fish -ic 'vim; exec /usr/local/bin/fish'"
		wantArgs := []string{"tmux", "new-session", "-A", "-s", wantSessionName, "-c", dir, shellCmd}
		if len(result.ExecArgs) != len(wantArgs) {
			t.Fatalf("result.ExecArgs = %v, want %v", result.ExecArgs, wantArgs)
		}
		for i, arg := range result.ExecArgs {
			if arg != wantArgs[i] {
				t.Errorf("result.ExecArgs[%d] = %q, want %q", i, arg, wantArgs[i])
			}
		}
	})

	t.Run("no shell-command in exec args when command is nil", func(t *testing.T) {
		dir := t.TempDir()
		gitResolver := &mockGitResolver{}
		store := &mockProjectStore{}
		checker := &mockSessionChecker{existingSessions: map[string]bool{}}
		gen := func() (string, error) { return "abc123", nil }

		qs := session.NewQuickStart(gitResolver, store, checker, gen)

		result, err := qs.Run(dir, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should be exactly 7 args: tmux new-session -A -s <name> -c <dir>
		wantSessionName := filepath.Base(dir) + "-abc123"
		wantArgs := []string{"tmux", "new-session", "-A", "-s", wantSessionName, "-c", dir}
		if len(result.ExecArgs) != len(wantArgs) {
			t.Fatalf("result.ExecArgs = %v, want %v", result.ExecArgs, wantArgs)
		}
		for i, arg := range result.ExecArgs {
			if arg != wantArgs[i] {
				t.Errorf("result.ExecArgs[%d] = %q, want %q", i, arg, wantArgs[i])
			}
		}
	})

	t.Run("returns error when git resolution fails", func(t *testing.T) {
		gitResolver := &mockGitResolver{err: fmt.Errorf("git error")}
		store := &mockProjectStore{}
		checker := &mockSessionChecker{existingSessions: map[string]bool{}}
		gen := func() (string, error) { return "abc123", nil }

		qs := session.NewQuickStart(gitResolver, store, checker, gen)

		_, err := qs.Run("/some/path", nil)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}
