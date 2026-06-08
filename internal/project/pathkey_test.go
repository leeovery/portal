package project

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/leeovery/portal/internal/resolver"
)

func TestCanonicalDirKey(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir() error = %v", err)
	}

	t.Run("it expands a leading tilde to the home directory", func(t *testing.T) {
		// EvalSymlinks the expected home so the comparison is symlink-stable
		// (the home dir itself may be a symlink on some platforms).
		want, err := filepath.EvalSymlinks(filepath.Join(home, "code", "portal"))
		if err != nil {
			// Path may not exist on disk; fall back to the Clean(Abs) contract.
			want = filepath.Clean(filepath.Join(home, "code", "portal"))
		}

		got := CanonicalDirKey("~/code/portal")
		if got != want {
			t.Errorf("CanonicalDirKey(%q) = %q, want %q", "~/code/portal", got, want)
		}
	})

	t.Run("it produces the same key for a path with and without a trailing slash", func(t *testing.T) {
		dir := t.TempDir()

		withSlash := CanonicalDirKey(dir + string(os.PathSeparator))
		withoutSlash := CanonicalDirKey(dir)
		if withSlash != withoutSlash {
			t.Errorf("trailing-slash key %q != no-slash key %q", withSlash, withoutSlash)
		}
	})

	t.Run("it resolves a symlinked path to the same key as its real target", func(t *testing.T) {
		base := t.TempDir()
		target := filepath.Join(base, "target")
		if err := os.Mkdir(target, 0o755); err != nil {
			t.Fatalf("Mkdir(%q) error = %v", target, err)
		}
		link := filepath.Join(base, "link")
		if err := os.Symlink(target, link); err != nil {
			t.Fatalf("Symlink(%q -> %q) error = %v", link, target, err)
		}

		linkKey := CanonicalDirKey(link)
		targetKey := CanonicalDirKey(target)
		if linkKey != targetKey {
			t.Errorf("symlink key %q != target key %q", linkKey, targetKey)
		}
	})

	t.Run("it resolves a relative path to absolute", func(t *testing.T) {
		got := CanonicalDirKey(".")
		if !filepath.IsAbs(got) {
			t.Errorf("CanonicalDirKey(%q) = %q, want an absolute path", ".", got)
		}
	})

	t.Run("it falls back to clean(abs) for a non-existent path", func(t *testing.T) {
		base := t.TempDir()
		missing := filepath.Join(base, "does", "not", "exist")

		// base exists and is canonical; EvalSymlinks will fail on the missing
		// suffix, so we expect Clean(Abs(missing)).
		abs, err := filepath.Abs(missing)
		if err != nil {
			t.Fatalf("Abs(%q) error = %v", missing, err)
		}
		want := filepath.Clean(abs)

		got := CanonicalDirKey(missing)
		if got != want {
			t.Errorf("CanonicalDirKey(%q) = %q, want %q", missing, got, want)
		}
	})
}

// TestCanonicalDirKey_MatchesResolveGitRoot is the spec's build-time
// lookup-key-matches-stored-path invariant: a directory resolved through
// ResolveGitRoot (the source of Project.Path at upsert time) must produce a
// CanonicalDirKey equal to the key derived from passing the same directory
// through the stamp/fallback path.
func TestCanonicalDirKey_MatchesResolveGitRoot(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repo := t.TempDir()
	if out, err := exec.Command("git", "-C", repo, "init").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}

	runner := &resolver.RealCommandRunner{}

	// Project.Path source: the git root of a sub-directory.
	subdir := filepath.Join(repo, "sub")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatalf("Mkdir(%q) error = %v", subdir, err)
	}
	gitRoot, err := resolver.ResolveGitRoot(subdir, runner)
	if err != nil {
		t.Fatalf("ResolveGitRoot(%q) error = %v", subdir, err)
	}

	storedKey := CanonicalDirKey(gitRoot)
	// Lookup-side key derived from the same repo directory (the stamp/fallback
	// path stamps the resolved git root).
	lookupKey := CanonicalDirKey(repo)

	if storedKey != lookupKey {
		t.Errorf("stored Project.Path key %q != lookup key %q", storedKey, lookupKey)
	}
}
