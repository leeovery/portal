package cmd

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/xdg"
)

type fakeTerminalDetector struct {
	id spawn.Identity
}

func (f fakeTerminalDetector) Detect() spawn.Identity {
	return f.id
}

func isolateTerminalsFile(t *testing.T) {
	t.Helper()
	t.Setenv("PORTAL_TERMINALS_FILE", filepath.Join(t.TempDir(), "terminals.json"))
}

func TestBuildProductionSpawnSeams(t *testing.T) {
	isolateTerminalsFile(t)
	t.Setenv("PORTAL_STATE_DIR", t.TempDir())
	sink := logtest.Install(t)

	fake := backendtest.New(backend.Session{Name: "api-x7Kd9a"})
	seams := buildProductionSpawnSeams(context.Background(), fake, stateRunners{})

	t.Run("Exists asks the backend, and reads a session it can't find as gone", func(t *testing.T) {
		if !seams.Exists("api-x7Kd9a") {
			t.Error("Exists(api-x7Kd9a) = false for a live session")
		}
		if seams.Exists("gone-c3d4") {
			t.Error("Exists(gone-c3d4) = true for a session the backend lacks")
		}
	})

	t.Run("Ack is the state directory's spawn-ack channel", func(t *testing.T) {
		if err := seams.Ack.(spawn.AckWriter).Write("b1", "t1"); err != nil {
			t.Fatalf("Write: %v", err)
		}
		if got, err := seams.Ack.Collect("b1"); err != nil || len(got) != 1 {
			t.Errorf("Collect(b1) = %v, %v; want the one ack", got, err)
		}
		if _, err := os.Stat(filepath.Join(os.Getenv("PORTAL_STATE_DIR"), "spawn-acks", "b1-t1")); err != nil {
			t.Errorf("the ack is not in the state directory's spawn-acks: %v", err)
		}
	})

	t.Run("Logger is the spawn-component logger", func(t *testing.T) {
		seams.Logger.Info("seam probe")
		rec := sink.Records().Matching("spawn", "seam probe").Only(t, "log record")
		if got := rec.AttrString(t, "component"); got != "spawn" {
			t.Errorf("Logger component attr = %q, want spawn", got)
		}
	})

	t.Run("Detector, Resolve, Exe, Env are wired", func(t *testing.T) {
		if seams.Detector == nil || seams.Resolve == nil || seams.Exe == nil || seams.Env == nil {
			t.Fatal("a seam is nil")
		}
	})

	t.Run("a backend with no process-table search finds no raw client, so detection refuses", func(t *testing.T) {
		fake.SetCurrent("api-x7Kd9a")
		fake.SetLeaderEnv("api-x7Kd9a", backend.ClientEnv{{Name: "DISPLAY", Unset: true}})
		t.Cleanup(func() { fake.SetCurrent("") })

		if id := seams.Detector.Detect(); !id.IsNull() {
			t.Errorf("Detect = %+v, want NULL", id)
		}
		rec := sink.Records().Matching("spawn", "detection could not tell the host terminal").Only(t, "detection record")
		want := `inside-session raw client walk: no one terminal leads the session: 0 of 0 zmx attach processes on session "api-x7Kd9a" can be its leader, want 1`
		if detail := rec.AttrString(t, "detail"); detail != want {
			t.Errorf("detail = %q, want no raw client found", detail)
		}
	})
}

// unsetForTest unsets name for the rest of t, restoring it afterwards.
func unsetForTest(t *testing.T, name string) {
	t.Helper()
	t.Setenv(name, "")
	if err := os.Unsetenv(name); err != nil {
		t.Fatalf("unset %s: %v", name, err)
	}
}

func TestWindowEnv(t *testing.T) {
	zmxDir := t.TempDir()
	driver, err := bindZmxAt("/nonexistent/zmx", zmxDir)
	if err != nil {
		t.Fatal(err)
	}
	// unsetAll unsets every variable the window environment carries but
	// PATH, so a case sets only what it is about.
	unsetAll := func(t *testing.T) {
		for _, name := range append(xdg.Variables(), log.Variables()...) {
			unsetForTest(t, name)
		}
	}

	t.Run("it carries PATH, the resolved state directory, the backend's directory and every path and log variable set", func(t *testing.T) {
		unsetAll(t)
		base := t.TempDir()
		t.Setenv("PATH", "/opt/homebrew/bin:/usr/bin")
		t.Setenv("XDG_CONFIG_HOME", base)
		t.Setenv("ZMX_DIR", "/a/dir/the/backend/is/not/bound/to")
		t.Setenv("ZMX_SESSION_PREFIX", "work-")
		t.Setenv("PORTAL_HOOKS_FILE", "/Users/me/hooks.json")
		t.Setenv("PORTAL_THEMES_DIR", "")
		t.Setenv("PORTAL_LOG_LEVEL", "debug")

		env, err := windowEnv(driver)()
		if err != nil {
			t.Fatalf("windowEnv: %v", err)
		}
		wantSet := []string{
			"PATH=/opt/homebrew/bin:/usr/bin",
			"PORTAL_STATE_DIR=" + filepath.Join(base, "portal", "state"),
			"ZMX_DIR=" + zmxDir,
			"XDG_CONFIG_HOME=" + base,
			"PORTAL_HOOKS_FILE=/Users/me/hooks.json",
			"PORTAL_THEMES_DIR=",
			"PORTAL_LOG_LEVEL=debug",
		}
		if !slices.Equal(env.Set, wantSet) {
			t.Errorf("window environment sets %q, want %q", env.Set, wantSet)
		}
	})

	t.Run("it unsets each path and log variable this process lacks", func(t *testing.T) {
		unsetAll(t)
		t.Setenv("PORTAL_HOOKS_FILE", "/Users/me/hooks.json")
		t.Setenv("PORTAL_STATE_DIR", "/Users/me/portal-state")

		env, err := windowEnv(driver)()
		if err != nil {
			t.Fatalf("windowEnv: %v", err)
		}
		want := slices.DeleteFunc(append(xdg.Variables(), log.Variables()...), func(name string) bool {
			return name == "PORTAL_HOOKS_FILE" || name == "PORTAL_STATE_DIR"
		})
		if !slices.Equal(env.Unset, want) {
			t.Errorf("window environment unsets %q, want %q", env.Unset, want)
		}
	})

	t.Run("it makes each carried path absolute against this process's directory", func(t *testing.T) {
		unsetAll(t)
		cwd, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		t.Setenv("PORTAL_STATE_DIR", "rel/state")
		t.Setenv("XDG_CONFIG_HOME", "rel/config")
		t.Setenv("PORTAL_PREFS_FILE", "rel/prefs.json")

		env, err := windowEnv(driver)()
		if err != nil {
			t.Fatalf("windowEnv: %v", err)
		}
		for _, want := range []string{
			"PORTAL_STATE_DIR=" + filepath.Join(cwd, "rel/state"),
			"XDG_CONFIG_HOME=" + filepath.Join(cwd, "rel/config"),
			"PORTAL_PREFS_FILE=" + filepath.Join(cwd, "rel/prefs.json"),
		} {
			if !slices.Contains(env.Set, want) {
				t.Errorf("window environment sets %q, want %s among them", env.Set, want)
			}
		}
	})

	t.Run("it carries a state directory set by its variable once, as set", func(t *testing.T) {
		unsetAll(t)
		t.Setenv("PORTAL_STATE_DIR", "/Users/me/portal-state")

		env, err := windowEnv(driver)()
		if err != nil {
			t.Fatalf("windowEnv: %v", err)
		}
		var assigned []string
		for _, kv := range env.Set {
			if strings.HasPrefix(kv, "PORTAL_STATE_DIR=") {
				assigned = append(assigned, kv)
			}
		}
		if !slices.Equal(assigned, []string{"PORTAL_STATE_DIR=/Users/me/portal-state"}) {
			t.Errorf("state directory assignments = %q, want it once as set", assigned)
		}
	})

	t.Run("it fails when the state directory won't resolve", func(t *testing.T) {
		unsetAll(t)
		t.Setenv("HOME", "")

		if env, err := windowEnv(driver)(); err == nil {
			t.Errorf("windowEnv = %+v, want an error with no state directory to resolve", env)
		}
	})

	t.Run("a backend that can't say how to reach its sessions adds nothing for them", func(t *testing.T) {
		t.Setenv("ZMX_DIR", "/elsewhere")

		env, err := windowEnv(backendtest.New())()
		if err != nil {
			t.Fatalf("windowEnv: %v", err)
		}
		for _, kv := range env.Set {
			if strings.HasPrefix(kv, "ZMX_") {
				t.Errorf("windowEnv carries %q for a backend with no reach", kv)
			}
		}
	})
}

func TestTheProductionAdaptersArePoisoned(t *testing.T) {
	isolateTerminalsFile(t)
	seams := buildProductionSpawnSeams(context.Background(), backendtest.New(), stateRunners{})
	ghostty := spawn.NewIdentity("com.mitchellh.ghostty", "Ghostty")
	for name, resolve := range map[string]spawn.AdapterResolver{
		"the production seams' resolver": seams.Resolve,
		"a resolver a test builds":       spawn.NewResolver(spawn.TerminalsConfig{}).Resolve,
	} {
		t.Run(name, func(t *testing.T) {
			adapter, resolution := resolve(ghostty)
			if adapter == nil || resolution != spawn.ResolutionNative {
				t.Fatalf("Resolve(Ghostty) = %v, %v; want its native adapter", adapter, resolution)
			}
			defer func() {
				if recover() == nil {
					t.Error("Ghostty's adapter asked for a window without panicking")
				}
			}()
			adapter.OpenWindow([]string{"/usr/bin/true"})
		})
	}
}
