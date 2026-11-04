package zmxtest

import (
	"os"
	"path/filepath"
	"testing"
)

// fakeZmx writes an executable named name in a directory of its own running
// script, and returns its path.
func fakeZmx(t *testing.T, name, script string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestOlderBuild(t *testing.T) {
	// Only a run in the directory it is handed, as its home too, reports
	// itself, so no run reads the developer's.
	const inDir = `[ -n "$ZMX_DIR" ] && [ "$ZMX_DIR" = "$HOME" ] || exit 3; `

	t.Run("a zmx reporting 0.8.1 is one", func(t *testing.T) {
		binary := fakeZmx(t, "zmx", inDir+`printf 'zmx\t\t0.8.1\nghostty_vt\tghostty-1.3.2\n'`)

		if err := olderBuild(binary, t.TempDir()); err != nil {
			t.Errorf("olderBuild = %v, want nil", err)
		}
	})

	for name, binary := range map[string]func(t *testing.T) string{
		"a zmx reporting another version": func(t *testing.T) string { return fakeZmx(t, "zmx", inDir+`printf 'zmx\t\t0.9.0\n'`) },
		"a zmx reporting nothing":         func(t *testing.T) string { return fakeZmx(t, "zmx", inDir) },
		"a zmx that fails":                func(t *testing.T) string { return fakeZmx(t, "zmx", `printf 'zmx\t\t0.8.1\n'; exit 1`) },
		"a build not named zmx":           func(t *testing.T) string { return fakeZmx(t, "zmx-0.8.1", inDir+`printf 'zmx\t\t0.8.1\n'`) },
		"a relative path":                 func(*testing.T) string { return "zmx" },
		"no file":                         func(t *testing.T) string { return filepath.Join(t.TempDir(), "zmx") },
	} {
		t.Run(name+" is none", func(t *testing.T) {
			if err := olderBuild(binary(t), t.TempDir()); err == nil {
				t.Error("olderBuild = nil, want it refused")
			}
		})
	}
}
