package state_test

import (
	"os"
	"path/filepath"
	"testing"

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
