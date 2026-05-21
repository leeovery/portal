package state_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/state"
)

func TestDir(t *testing.T) {
	t.Run("resolves to PORTAL_STATE_DIR when set", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("PORTAL_STATE_DIR", dir)
		// Even when XDG_CONFIG_HOME and HOME are set, PORTAL_STATE_DIR wins
		// and the result is used verbatim (no /portal/state suffix).
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "xdg"))
		t.Setenv("HOME", t.TempDir())

		got, err := state.Dir()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != dir {
			t.Errorf("Dir() = %q; want %q", got, dir)
		}
	})

	t.Run("falls back to XDG_CONFIG_HOME/portal/state when PORTAL_STATE_DIR is unset", func(t *testing.T) {
		xdg := t.TempDir()
		t.Setenv("PORTAL_STATE_DIR", "")
		t.Setenv("XDG_CONFIG_HOME", xdg)
		t.Setenv("HOME", t.TempDir())

		got, err := state.Dir()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := filepath.Join(xdg, "portal", "state")
		if got != want {
			t.Errorf("Dir() = %q; want %q", got, want)
		}
	})

	t.Run("falls back to ~/.config/portal/state when neither env var is set", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("PORTAL_STATE_DIR", "")
		t.Setenv("XDG_CONFIG_HOME", "")
		t.Setenv("HOME", home)

		got, err := state.Dir()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := filepath.Join(home, ".config", "portal", "state")
		if got != want {
			t.Errorf("Dir() = %q; want %q", got, want)
		}
	})
}

func TestEnsureDir(t *testing.T) {
	t.Run("creates state/ and state/scrollback/ with mode 0700", func(t *testing.T) {
		root := t.TempDir()
		stateRoot := filepath.Join(root, "state")
		t.Setenv("PORTAL_STATE_DIR", stateRoot)

		got, err := state.EnsureDir()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != stateRoot {
			t.Errorf("EnsureDir() = %q; want %q", got, stateRoot)
		}

		assertDirMode(t, stateRoot, 0o700)
		assertDirMode(t, filepath.Join(stateRoot, "scrollback"), 0o700)
	})

	t.Run("is a no-op when the state directory already exists", func(t *testing.T) {
		root := t.TempDir()
		stateRoot := filepath.Join(root, "state")
		t.Setenv("PORTAL_STATE_DIR", stateRoot)

		if _, err := state.EnsureDir(); err != nil {
			t.Fatalf("first EnsureDir failed: %v", err)
		}

		// Drop a sentinel file so we can confirm nothing was clobbered.
		sentinel := filepath.Join(stateRoot, "sentinel")
		if err := os.WriteFile(sentinel, []byte("keep"), 0o600); err != nil {
			t.Fatalf("failed to write sentinel: %v", err)
		}

		got, err := state.EnsureDir()
		if err != nil {
			t.Fatalf("second EnsureDir failed: %v", err)
		}
		if got != stateRoot {
			t.Errorf("EnsureDir() = %q; want %q", got, stateRoot)
		}

		if data, err := os.ReadFile(sentinel); err != nil {
			t.Fatalf("sentinel disappeared: %v", err)
		} else if string(data) != "keep" {
			t.Errorf("sentinel content = %q; want %q", string(data), "keep")
		}

		assertDirMode(t, stateRoot, 0o700)
		assertDirMode(t, filepath.Join(stateRoot, "scrollback"), 0o700)
	})
}

func TestAccessors(t *testing.T) {
	dir := "/tmp/portal-state"

	t.Run("returns documented top-level paths", func(t *testing.T) {
		cases := []struct {
			name string
			got  string
			want string
		}{
			{"sessions.json", state.SessionsJSON(dir), filepath.Join(dir, "sessions.json")},
			{"save.requested", state.SaveRequested(dir), filepath.Join(dir, "save.requested")},
			{"daemon.pid", state.DaemonPID(dir), filepath.Join(dir, "daemon.pid")},
			{"daemon.version", state.DaemonVersion(dir), filepath.Join(dir, "daemon.version")},
			{"daemon.lock", state.DaemonLock(dir), filepath.Join(dir, "daemon.lock")},
			{"portal.log", state.PortalLog(dir), filepath.Join(dir, "portal.log")},
			{"portal.log.old", state.PortalLogOld(dir), filepath.Join(dir, "portal.log.old")},
			{"scrollback dir", state.ScrollbackDir(dir), filepath.Join(dir, "scrollback")},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				if c.got != c.want {
					t.Errorf("%s = %q; want %q", c.name, c.got, c.want)
				}
			})
		}
	})

	t.Run("builds scrollback file path from paneKey", func(t *testing.T) {
		got := state.ScrollbackFile(dir, "work__0.1")
		want := filepath.Join(dir, "scrollback", "work__0.1.bin")
		if got != want {
			t.Errorf("ScrollbackFile = %q; want %q", got, want)
		}
	})

	t.Run("builds hydration FIFO path from paneKey", func(t *testing.T) {
		got := state.FIFOPath(dir, "work__0.1")
		want := filepath.Join(dir, "hydrate-work__0.1.fifo")
		if got != want {
			t.Errorf("FIFOPath = %q; want %q", got, want)
		}
	})
}

func TestPaneKeyFromFIFOPath(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "absolute path with hydrate prefix and .fifo suffix",
			in:   "/var/lib/portal/state/hydrate-work__0.1.fifo",
			want: "work__0.1",
		},
		{
			name: "basename only with hydrate prefix and .fifo suffix",
			in:   "hydrate-foo__0.0.fifo",
			want: "foo__0.0",
		},
		{
			name: "round-trips state.FIFOPath",
			in:   state.FIFOPath("/tmp/portal-state", "myproj__1.2"),
			want: "myproj__1.2",
		},
		{
			name: "idempotent on input lacking prefix and suffix",
			in:   "already-bare",
			want: "already-bare",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := state.PaneKeyFromFIFOPath(c.in)
			if got != c.want {
				t.Errorf("PaneKeyFromFIFOPath(%q) = %q; want %q", c.in, got, c.want)
			}
		})
	}
}

func TestTouchSaveRequested(t *testing.T) {
	t.Run("creates save.requested with mtime near now", func(t *testing.T) {
		dir := t.TempDir()

		before := time.Now()
		if err := state.TouchSaveRequested(dir); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		after := time.Now()

		path := state.SaveRequested(dir)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat save.requested: %v", err)
		}
		if info.Size() != 0 {
			t.Errorf("save.requested size = %d; want 0", info.Size())
		}
		mtime := info.ModTime()
		// Tolerance accounts for filesystem mtime resolution (e.g. 1s on
		// some platforms). Bracket inclusive on both sides with a 2s slack.
		if mtime.Before(before.Add(-2*time.Second)) || mtime.After(after.Add(2*time.Second)) {
			t.Errorf("mtime = %v; want within [%v, %v] (±2s)", mtime, before, after)
		}
	})

	t.Run("bumps mtime on an already-present save.requested", func(t *testing.T) {
		dir := t.TempDir()
		path := state.SaveRequested(dir)

		// Pre-create save.requested with an old mtime.
		if err := os.WriteFile(path, []byte("stale"), 0o600); err != nil {
			t.Fatalf("seed save.requested: %v", err)
		}
		old := time.Now().Add(-1 * time.Hour)
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatalf("set old mtime: %v", err)
		}

		before := time.Now()
		if err := state.TouchSaveRequested(dir); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if info.Size() != 0 {
			t.Errorf("save.requested truncated size = %d; want 0", info.Size())
		}
		if !info.ModTime().After(old) {
			t.Errorf("mtime not bumped: got %v; want after %v", info.ModTime(), old)
		}
		// Sanity: mtime must be near `before`, not stale.
		if info.ModTime().Before(before.Add(-2 * time.Second)) {
			t.Errorf("mtime = %v; expected near %v", info.ModTime(), before)
		}
	})

	t.Run("returns wrapped error when parent directory does not exist", func(t *testing.T) {
		root := t.TempDir()
		missing := filepath.Join(root, "does-not-exist")

		err := state.TouchSaveRequested(missing)
		if err == nil {
			t.Fatal("expected error when parent dir is missing, got nil")
		}
		// File must not have been created.
		if _, statErr := os.Stat(state.SaveRequested(missing)); !os.IsNotExist(statErr) {
			t.Errorf("save.requested should not exist; stat err = %v", statErr)
		}
	})
}

// assertDirMode fails the test unless path is a directory whose permission
// bits exactly equal want.
func assertDirMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if !info.IsDir() {
		t.Fatalf("%s: expected directory", path)
	}
	if got := info.Mode().Perm(); got != want {
		t.Errorf("%s mode = %o; want %o", path, got, want)
	}
}
