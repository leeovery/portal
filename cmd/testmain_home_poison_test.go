package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/xdg"
)

// ambientHomeAtInit is the developer's own HOME, captured during package
// initialisation — which runs before TestMain replaces it with the per-run
// temp home, so it is the one reading of the real home the poison cannot hide.
var ambientHomeAtInit = os.Getenv("HOME")

// requirePoisonedHome refuses to hand back a home the package-wide poison did
// not replace, because resolving a default config path runs the Application
// Support migration against whatever HOME names.
func requirePoisonedHome(t *testing.T) string {
	t.Helper()

	home := os.Getenv("HOME")
	if home == "" || home == ambientHomeAtInit {
		t.Fatalf("HOME = %q is not poisoned package-wide; resolving a default config path would move the developer's real config files", home)
	}

	info, err := os.Stat(home)
	if err != nil {
		t.Fatalf("poisoned HOME %q: %v", home, err)
	}
	if !info.IsDir() {
		t.Fatalf("poisoned HOME %q is not a directory", home)
	}

	return home
}

func TestPackageWideHomePoison(t *testing.T) {
	t.Run("it resolves a default config path under the package-wide poisoned HOME when a subtest pins none", func(t *testing.T) {
		home := requirePoisonedHome(t)

		got, err := configFilePath(xdg.ConfigFileID{EnvVar: "TEST_HOME_POISON_UNSET", Filename: "projects.json"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := filepath.Join(home, ".config", "portal", "projects.json")
		if got != want {
			t.Errorf("configFilePath() = %q, want %q", got, want)
		}
	})

	t.Run("it runs the Application Support migration against the poisoned home rather than the developer's", func(t *testing.T) {
		home := requirePoisonedHome(t)
		t.Setenv("XDG_CONFIG_HOME", "")

		const filename = "home-poison-migration.json"
		oldDir := filepath.Join(home, "Library", "Application Support", "portal")
		if err := os.MkdirAll(oldDir, 0o755); err != nil {
			t.Fatalf("staging old config dir: %v", err)
		}
		oldPath := filepath.Join(oldDir, filename)
		body := []byte(`{"migrated":true}`)
		if err := os.WriteFile(oldPath, body, 0o644); err != nil {
			t.Fatalf("staging old config file: %v", err)
		}

		got, err := configFilePath(xdg.ConfigFileID{EnvVar: "TEST_HOME_POISON_MIGRATE", Filename: filename})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := filepath.Join(home, ".config", "portal", filename)
		if got != want {
			t.Fatalf("configFilePath() = %q, want %q", got, want)
		}
		migrated, err := os.ReadFile(want)
		if err != nil {
			t.Fatalf("reading migrated file: %v", err)
		}
		if string(migrated) != string(body) {
			t.Errorf("migrated body = %q, want %q", migrated, body)
		}
		if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
			t.Errorf("old path %q still present after migration (stat error: %v)", oldPath, err)
		}
	})

	// The Go toolchain derives its caches from HOME, so without this a subprocess
	// `go build` would re-download every module into the temp home and leave a
	// read-only cache there that the run's own cleanup cannot remove.
	t.Run("it keeps the toolchain's caches outside the poisoned HOME", func(t *testing.T) {
		home := requirePoisonedHome(t)

		for _, name := range []string{"GOMODCACHE", "GOCACHE"} {
			value := os.Getenv(name)
			if value == "" {
				t.Errorf("%s is unset; a subprocess build would resolve it under the poisoned HOME", name)
				continue
			}
			if !filepath.IsAbs(value) {
				t.Errorf("%s = %q is not an absolute path", name, value)
				continue
			}
			if rel, err := filepath.Rel(home, value); err == nil && !strings.HasPrefix(rel, "..") {
				t.Errorf("%s = %q lies under the poisoned HOME %q", name, value, home)
			}
		}
	})

	t.Run("it still honours a subtest's own HOME pin over the package-wide poison", func(t *testing.T) {
		poisoned := requirePoisonedHome(t)
		homeDir := t.TempDir()
		t.Setenv("HOME", homeDir)
		t.Setenv("XDG_CONFIG_HOME", "")

		got, err := configFilePath(xdg.ConfigFileID{EnvVar: "TEST_HOME_POISON_PINNED", Filename: "projects.json"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := filepath.Join(homeDir, ".config", "portal", "projects.json")
		if got != want {
			t.Errorf("configFilePath() = %q, want %q", got, want)
		}
		if got == filepath.Join(poisoned, ".config", "portal", "projects.json") {
			t.Errorf("configFilePath() resolved under the poisoned home %q despite the subtest's own pin", poisoned)
		}
	})
}
