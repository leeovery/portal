package state_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/xdg"
)

func TestDir(t *testing.T) {
	t.Run("resolves to PORTAL_STATE_DIR when set", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("PORTAL_STATE_DIR", dir)
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

// The state directory is not config content, but it takes its location by the
// same rule: the per-directory variable, else <config base>/portal/<name>. This
// pins that it reads that rule rather than re-authoring it, so a config-base
// layer added in one place reaches the state directory too.
func TestDirResolvesThroughTheSharedRule(t *testing.T) {
	t.Run("it resolves the state dir through the shared config base", func(t *testing.T) {
		cases := []struct {
			name     string
			stateEnv string
			xdgHome  string
		}{
			{name: "the per-directory variable", stateEnv: t.TempDir(), xdgHome: t.TempDir()},
			{name: "the config base", stateEnv: "", xdgHome: t.TempDir()},
			{name: "the home fallback", stateEnv: "", xdgHome: ""},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				home := t.TempDir()
				t.Setenv("HOME", home)
				t.Setenv("PORTAL_STATE_DIR", tc.stateEnv)
				t.Setenv("XDG_CONFIG_HOME", tc.xdgHome)

				got, err := state.Dir()
				if err != nil {
					t.Fatalf("state.Dir(): %v", err)
				}

				want, err := xdg.ConfigDirPath(xdg.OSEnv, xdg.StateDir)
				if err != nil {
					t.Fatalf("shared rule: %v", err)
				}
				if got != want {
					t.Errorf("state.Dir() = %q, want %q", got, want)
				}
			})
		}
	})

	t.Run("it resolves a path and creates nothing", func(t *testing.T) {
		base := t.TempDir()
		t.Setenv("HOME", t.TempDir())
		t.Setenv("PORTAL_STATE_DIR", "")
		t.Setenv("XDG_CONFIG_HOME", base)

		got, err := state.Dir()
		if err != nil {
			t.Fatalf("state.Dir(): %v", err)
		}

		if _, err := os.Stat(got); !os.IsNotExist(err) {
			t.Errorf("resolving created %q (stat err = %v)", got, err)
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
			{"daemon.pid", state.DaemonPID(dir), filepath.Join(dir, "daemon.pid")},
			{"daemon.version", state.DaemonVersion(dir), filepath.Join(dir, "daemon.version")},
			{"daemon.lock", state.DaemonLock(dir), filepath.Join(dir, "daemon.lock")},
			{"portal.log", state.PortalLog(dir), filepath.Join(dir, "portal.log")},
			{"scrollback dir", state.ScrollbackDir(dir), filepath.Join(dir, "scrollback")},
			{"the tmux version's migrated state, beside it", state.TmuxStateDir(dir), "/tmp/state-tmux"},
			{"the tmux version's migrated state, beside it given with a trailing separator", state.TmuxStateDir(dir + "/"), "/tmp/state-tmux"},
			{"a migration's staging dir, beside it", state.MigrationStagingDir(dir), "/tmp/state-migrating"},
			{"a migration's staging dir, beside it given with a trailing separator", state.MigrationStagingDir(dir + "/"), "/tmp/state-migrating"},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				if c.got != c.want {
					t.Errorf("%s = %q; want %q", c.name, c.got, c.want)
				}
			})
		}
	})
}

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
