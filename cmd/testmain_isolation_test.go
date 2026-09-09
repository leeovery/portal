package cmd

// TestMain poisons the PORTAL_* config paths and TMUX package-wide so a test that
// forgets to isolate fails loudly, and HOME to a per-run temp directory — with
// XDG_CONFIG_HOME emptied so HOME is what decides — so one that resolves a default
// config path lands there instead of reading or mutating the developer's real
// config; subprocesses inherit the poison via os.Environ().

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/prefs"
)

// pinToolchainCaches re-exports the cache locations the Go toolchain would
// otherwise derive from HOME, so a subprocess `go build` keeps using the
// developer's caches once HOME is poisoned rather than re-downloading every
// module into the temp home and leaving a read-only cache there that the run's
// own cleanup cannot remove. The values come from the toolchain rather than
// being rebuilt from the pre-poison home, so an explicitly configured cache is
// honoured instead of overwritten with the default.
func pinToolchainCaches() error {
	names := []string{"GOMODCACHE", "GOCACHE"}

	out, err := exec.Command("go", append([]string{"env"}, names...)...).Output()
	if err != nil {
		return fmt.Errorf("go env %s: %w", strings.Join(names, " "), err)
	}

	values := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(values) != len(names) {
		return fmt.Errorf("go env %s: got %d values, want %d", strings.Join(names, " "), len(values), len(names))
	}

	for i, name := range names {
		value := strings.TrimSpace(values[i])
		if !filepath.IsAbs(value) {
			return fmt.Errorf("go env %s = %q is not an absolute path", name, value)
		}
		os.Setenv(name, value)
	}
	return nil
}

func TestMain(m *testing.M) {
	// Before the poison, while the toolchain still resolves against the real home.
	if err := pinToolchainCaches(); err != nil {
		fmt.Fprintf(os.Stderr, "cmd tests: pinning the toolchain caches ahead of the HOME poison: %v\n", err)
		os.Exit(1)
	}

	// A real directory rather than a /nonexistent path: a resolve against an absent
	// home succeeds and then fails at the write, so only a writable home can stand in
	// for the developer's for the migration and for what a resolve goes on to create.
	homeDir, err := os.MkdirTemp("", "portal-test-home")
	if err != nil {
		fmt.Fprintf(os.Stderr, "cmd tests: creating the package-wide temp HOME: %v\n", err)
		os.Exit(1)
	}
	os.Setenv("HOME", homeDir)
	// The home poison only decides a default resolve while XDG_CONFIG_HOME is
	// empty, so it is emptied here too: on a machine where the developer sets
	// it, a test resolving a non-overridden config path would read their real
	// config base instead of the temp home.
	os.Setenv("XDG_CONFIG_HOME", "")

	os.Setenv("PORTAL_STATE_DIR", "/nonexistent/portal-test-must-isolate-state")
	os.Setenv("PORTAL_HOOKS_FILE", "/nonexistent/portal-test-must-isolate-hooks.json")
	os.Setenv("PORTAL_PROJECTS_FILE", "/nonexistent/portal-test-must-isolate-projects.json")
	os.Setenv("PORTAL_ALIASES_FILE", "/nonexistent/portal-test-must-isolate-aliases")
	os.Setenv("PORTAL_THEMES_DIR", "/nonexistent/portal-test-must-isolate-themes")
	os.Setenv("PORTAL_PREFS_FILE", "/nonexistent/portal-test-must-isolate-prefs.json")
	os.Setenv("TMUX", "/nonexistent/portal-test-must-set-tmux-socket,0,0")
	// Neutralised package-wide: production dispatches this on its own goroutine, so
	// a test reaching loadPrefsStore would leave a write racing its own teardown.
	persistTranslation = func(*prefs.Store, string) {}

	code := m.Run()
	if err := os.RemoveAll(homeDir); err != nil {
		fmt.Fprintf(os.Stderr, "cmd tests: removing the package-wide temp HOME: %v\n", err)
	}
	os.Exit(code)
}
