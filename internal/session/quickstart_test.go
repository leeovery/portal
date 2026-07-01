package session_test

import (
	"fmt"
	"path/filepath"
	"reflect"
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

// wantExecArgs builds the expected tmux create-stamp-attach exec chain for a
// quick-started session: create the session detached, stamp @portal-dir then
// @portal-id while it is detached (before attach blocks the chain), then attach.
// ";" elements are literal tmux command separators. token is the value the
// generator yields on its stamp call; when non-empty the @portal-id step is
// interpolated between the @portal-dir stamp and attach-session.
func wantExecArgs(name, dir, shellCmd, token string) []string {
	args := []string{"tmux", "new-session", "-d", "-s", name, "-c", dir}
	if shellCmd != "" {
		args = append(args, shellCmd)
	}
	args = append(args,
		";", "set-option", "-t", name, session.PortalDirOption, dir,
	)
	if token != "" {
		args = append(args,
			";", "set-option", "-t", name, session.PortalIDOption, token,
		)
	}
	return append(args,
		";", "attach-session", "-t", name,
	)
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

		// Verify ExecArgs include resolved dir via -c flag (and the stamp/attach chain).
		wantSessionName := filepath.Base(gitRoot) + "-abc123"
		wantArgs := wantExecArgs(wantSessionName, gitRoot, "", "abc123")
		if !reflect.DeepEqual(result.ExecArgs, wantArgs) {
			t.Fatalf("result.ExecArgs = %v, want %v", result.ExecArgs, wantArgs)
		}
	})

	t.Run("stamps @portal-dir at creation via the exec chain", func(t *testing.T) {
		// Creating detached gives an in-server point to stamp @portal-dir
		// BEFORE attaching, so a quick-started session is anchored to its
		// origin directory and grouping stays stable after the pane cd's away.
		gitRoot := t.TempDir()
		gitResolver := &mockGitResolver{resolvedDir: gitRoot}
		store := &mockProjectStore{}
		checker := &mockSessionChecker{existingSessions: map[string]bool{}}
		gen := func() (string, error) { return "abc123", nil }

		qs := session.NewQuickStart(gitResolver, store, checker, gen)

		result, err := qs.Run(gitRoot, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		wantSessionName := filepath.Base(gitRoot) + "-abc123"
		// The chain must contain: set-option -t <name> @portal-dir <dir>.
		assertContainsSubseq(t, result.ExecArgs, []string{
			"set-option", "-t", wantSessionName, session.PortalDirOption, gitRoot,
		})
		// It must NOT attach before stamping (stamp-before-attach ordering).
		setIdx := indexOf(result.ExecArgs, "set-option")
		attachIdx := indexOf(result.ExecArgs, "attach-session")
		if setIdx < 0 || attachIdx < 0 || setIdx >= attachIdx {
			t.Errorf("set-option (%d) must precede attach-session (%d) in %v", setIdx, attachIdx, result.ExecArgs)
		}
		// And it must NOT attach directly via new-session -A (which would block
		// the stamp); detached create is required.
		if indexOf(result.ExecArgs, "-A") >= 0 {
			t.Errorf("ExecArgs must not use new-session -A: %v", result.ExecArgs)
		}
	})

	t.Run("interpolates the @portal-id token as a literal set-option step in the exec chain", func(t *testing.T) {
		// The stamp token is generated in Go inside Run (a second qs.gen call,
		// independent of the name suffix) and interpolated as a single literal
		// argv element — opaque alphanumeric needs no shell-escaping.
		gitRoot := t.TempDir()
		gitResolver := &mockGitResolver{resolvedDir: gitRoot}
		store := &mockProjectStore{}
		checker := &mockSessionChecker{existingSessions: map[string]bool{}}
		gen := func() (string, error) { return "abc123", nil }

		qs := session.NewQuickStart(gitResolver, store, checker, gen)

		result, err := qs.Run(gitRoot, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		wantSessionName := filepath.Base(gitRoot) + "-abc123"
		assertContainsSubseq(t, result.ExecArgs, []string{
			"set-option", "-t", wantSessionName, session.PortalIDOption, "abc123",
		})
	})

	t.Run("orders the @portal-id stamp before attach-session", func(t *testing.T) {
		// The stamp must land while the session is detached; attach-session
		// blocks the chain, so any step after it never runs.
		gitRoot := t.TempDir()
		gitResolver := &mockGitResolver{resolvedDir: gitRoot}
		store := &mockProjectStore{}
		checker := &mockSessionChecker{existingSessions: map[string]bool{}}
		gen := func() (string, error) { return "abc123", nil }

		qs := session.NewQuickStart(gitResolver, store, checker, gen)

		result, err := qs.Run(gitRoot, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		idIdx := indexOfSubseq(result.ExecArgs, []string{session.PortalIDOption})
		attachIdx := indexOf(result.ExecArgs, "attach-session")
		if idIdx < 0 || attachIdx < 0 || idIdx >= attachIdx {
			t.Errorf("@portal-id stamp (%d) must precede attach-session (%d) in %v", idIdx, attachIdx, result.ExecArgs)
		}
	})

	t.Run("orders the @portal-id stamp after the @portal-dir stamp", func(t *testing.T) {
		gitRoot := t.TempDir()
		gitResolver := &mockGitResolver{resolvedDir: gitRoot}
		store := &mockProjectStore{}
		checker := &mockSessionChecker{existingSessions: map[string]bool{}}
		gen := func() (string, error) { return "abc123", nil }

		qs := session.NewQuickStart(gitResolver, store, checker, gen)

		result, err := qs.Run(gitRoot, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		dirIdx := indexOfSubseq(result.ExecArgs, []string{session.PortalDirOption})
		idIdx := indexOfSubseq(result.ExecArgs, []string{session.PortalIDOption})
		if dirIdx < 0 || idIdx < 0 || dirIdx >= idIdx {
			t.Errorf("@portal-dir stamp (%d) must precede @portal-id stamp (%d) in %v", dirIdx, idIdx, result.ExecArgs)
		}
	})

	t.Run("omits the @portal-id step when stamp-time token generation fails", func(t *testing.T) {
		// qs.gen is called twice: first for the name suffix (must succeed), then
		// for the stamp token. When only the stamp call errors, the @portal-id
		// step is dropped and the rest of the chain is unchanged (session still
		// created, un-stamped -> name fallback). Run must not return an error.
		gitRoot := t.TempDir()
		gitResolver := &mockGitResolver{resolvedDir: gitRoot}
		store := &mockProjectStore{}
		checker := &mockSessionChecker{existingSessions: map[string]bool{}}

		calls := 0
		gen := func() (string, error) {
			calls++
			if calls == 1 {
				return "abc123", nil
			}
			return "", fmt.Errorf("stamp token generation failed")
		}

		qs := session.NewQuickStart(gitResolver, store, checker, gen)

		result, err := qs.Run(gitRoot, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if indexOfSubseq(result.ExecArgs, []string{session.PortalIDOption}) >= 0 {
			t.Errorf("ExecArgs must not contain a @portal-id step when stamp generation fails: %v", result.ExecArgs)
		}

		// Rest of chain (create -> @portal-dir -> attach) is unchanged: token "" -> no @portal-id step.
		wantSessionName := filepath.Base(gitRoot) + "-abc123"
		wantArgs := wantExecArgs(wantSessionName, gitRoot, "", "")
		if !reflect.DeepEqual(result.ExecArgs, wantArgs) {
			t.Fatalf("result.ExecArgs = %v, want %v", result.ExecArgs, wantArgs)
		}
	})

	t.Run("does not use new-session -A", func(t *testing.T) {
		// Detached-create + stamp-before-attach ordering must be preserved: a
		// new-session -A would attach immediately and block the stamp steps.
		gitRoot := t.TempDir()
		gitResolver := &mockGitResolver{resolvedDir: gitRoot}
		store := &mockProjectStore{}
		checker := &mockSessionChecker{existingSessions: map[string]bool{}}
		gen := func() (string, error) { return "abc123", nil }

		qs := session.NewQuickStart(gitResolver, store, checker, gen)

		result, err := qs.Run(gitRoot, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if indexOf(result.ExecArgs, "-A") >= 0 {
			t.Errorf("ExecArgs must not use new-session -A: %v", result.ExecArgs)
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

	t.Run("exec args create detached, stamp, then attach", func(t *testing.T) {
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

		wantArgs := wantExecArgs(wantSessionName, dir, "", "abc123")
		if !reflect.DeepEqual(result.ExecArgs, wantArgs) {
			t.Fatalf("result.ExecArgs = %v, want %v", result.ExecArgs, wantArgs)
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
		wantArgs := wantExecArgs(wantSessionName, dir, shellCmd, "abc123")
		if !reflect.DeepEqual(result.ExecArgs, wantArgs) {
			t.Fatalf("result.ExecArgs = %v, want %v", result.ExecArgs, wantArgs)
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
		wantArgs := wantExecArgs(wantSessionName, dir, shellCmd, "abc123")
		if !reflect.DeepEqual(result.ExecArgs, wantArgs) {
			t.Fatalf("result.ExecArgs = %v, want %v", result.ExecArgs, wantArgs)
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

		wantSessionName := filepath.Base(dir) + "-abc123"
		wantArgs := wantExecArgs(wantSessionName, dir, "", "abc123")
		if !reflect.DeepEqual(result.ExecArgs, wantArgs) {
			t.Fatalf("result.ExecArgs = %v, want %v", result.ExecArgs, wantArgs)
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

// indexOf returns the first index of v in s, or -1.
func indexOf(s []string, v string) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}

// indexOfSubseq returns the start index of the first occurrence of want as a
// contiguous subsequence of got, or -1.
func indexOfSubseq(got, want []string) int {
	for i := 0; i+len(want) <= len(got); i++ {
		if reflect.DeepEqual(got[i:i+len(want)], want) {
			return i
		}
	}
	return -1
}

// assertContainsSubseq fails the test unless want appears as a contiguous
// subsequence of got.
func assertContainsSubseq(t *testing.T, got, want []string) {
	t.Helper()
	for i := 0; i+len(want) <= len(got); i++ {
		if reflect.DeepEqual(got[i:i+len(want)], want) {
			return
		}
	}
	t.Errorf("ExecArgs %v does not contain subsequence %v", got, want)
}
