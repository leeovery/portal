package xdg_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/portal/internal/xdg"
)

// cwd moves the test into a directory of its own and returns it, as the
// working directory reports it.
func cwd(t *testing.T) string {
	t.Helper()
	t.Chdir(t.TempDir())
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// A path from the environment means what it means where its user gave it:
// every process Portal starts runs in another directory, the saver's daemon in
// the state directory among them.
func TestRelativePathsResolveAgainstTheWorkingDirectory(t *testing.T) {
	t.Run("a per-file variable", func(t *testing.T) {
		dir := cwd(t)

		got, err := xdg.ConfigFilePath(xdg.EnvSlice([]string{"PORTAL_HOOKS_FILE=rel/hooks.json"}), xdg.HooksFile)

		if err != nil || got.Path != filepath.Join(dir, "rel/hooks.json") || !got.Overridden {
			t.Errorf("ConfigFilePath = %+v, %v; want %s, overridden", got, err, filepath.Join(dir, "rel/hooks.json"))
		}
	})

	t.Run("a per-directory variable", func(t *testing.T) {
		dir := cwd(t)

		got, err := xdg.ConfigDirPath(xdg.EnvSlice([]string{"PORTAL_STATE_DIR=state"}), xdg.StateDir)

		if want := filepath.Join(dir, "state"); err != nil || got != want {
			t.Errorf("ConfigDirPath = %q, %v; want %q", got, err, want)
		}
	})

	t.Run("the config base", func(t *testing.T) {
		dir := cwd(t)
		lookup := xdg.EnvSlice([]string{"XDG_CONFIG_HOME=cfg"})

		base, err := xdg.ConfigBaseFrom(lookup)
		if want := filepath.Join(dir, "cfg"); err != nil || base != want {
			t.Errorf("ConfigBaseFrom = %q, %v; want %q", base, err, want)
		}
		got, err := xdg.ConfigFilePath(lookup, xdg.PrefsFile)
		if want := filepath.Join(dir, "cfg", "portal", "prefs.json"); err != nil || got.Path != want {
			t.Errorf("ConfigFilePath = %+v, %v; want %q", got, err, want)
		}
	})

	t.Run("an absolute path is kept as written", func(t *testing.T) {
		cwd(t)
		const written = "/srv//portal/../portal/state/"

		got, err := xdg.ConfigDirPath(xdg.EnvSlice([]string{"PORTAL_STATE_DIR=" + written}), xdg.StateDir)

		if err != nil || got != written {
			t.Errorf("ConfigDirPath = %q, %v; want %q", got, err, written)
		}
	})
}

// So a process Portal starts inherits the one meaning: AbsoluteEnv rewrites
// a relative path in the environment, and only that.
func TestAbsoluteEnv(t *testing.T) {
	dir := cwd(t)
	for _, name := range xdg.Variables() {
		t.Setenv(name, "")
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PORTAL_STATE_DIR", "state")
	t.Setenv("PORTAL_HOOKS_FILE", "../hooks.json")
	t.Setenv("XDG_CONFIG_HOME", "/srv/cfg/")
	t.Setenv("PORTAL_PREFS_FILE", "")

	if err := xdg.AbsoluteEnv(); err != nil {
		t.Fatalf("AbsoluteEnv: %v", err)
	}

	for name, want := range map[string]string{
		"PORTAL_STATE_DIR":  filepath.Join(dir, "state"),
		"PORTAL_HOOKS_FILE": filepath.Join(filepath.Dir(dir), "hooks.json"),
		"XDG_CONFIG_HOME":   "/srv/cfg/",
		"PORTAL_PREFS_FILE": "",
	} {
		if got := os.Getenv(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	if _, set := os.LookupEnv("PORTAL_PROJECTS_FILE"); set {
		t.Error("PORTAL_PROJECTS_FILE is set, want an unset variable left unset")
	}
}
