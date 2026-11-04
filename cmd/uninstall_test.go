package cmd

import (
	"context"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/saver"
	"github.com/leeovery/portal/internal/state"
)

// stopperFake is a saver's stop that reports stopped and err.
type stopperFake struct {
	stopped bool
	err     error
	calls   int
}

func (s *stopperFake) Stop(context.Context) (bool, error) {
	s.calls++
	return s.stopped, s.err
}

func TestUninstall(t *testing.T) {
	t.Run("it stops the saver and says where the saved sessions and config are", func(t *testing.T) {
		stop := &stopperFake{stopped: true}
		withUninstallDeps(t, UninstallDeps{Saver: stop})

		stdout, _, err := runRootCmd(t, "uninstall")

		if err != nil {
			t.Fatalf("uninstall: %v", err)
		}
		if want := "Portal's save daemon stopped. Saved sessions and config are untouched at ~/.config/portal/.\n"; stdout.String() != want {
			t.Errorf("stdout = %q, want %q", stdout, want)
		}
		if stop.calls != 1 {
			t.Errorf("stops = %d, want 1", stop.calls)
		}
	})

	t.Run("with no saver running it says so, and succeeds", func(t *testing.T) {
		withUninstallDeps(t, UninstallDeps{Saver: &stopperFake{}})

		stdout, _, err := runRootCmd(t, "uninstall")

		if err != nil {
			t.Fatalf("uninstall: %v", err)
		}
		if want := "Portal's save daemon isn't running. Saved sessions and config are untouched at ~/.config/portal/.\n"; stdout.String() != want {
			t.Errorf("stdout = %q, want %q", stdout, want)
		}
	})

	t.Run("it names a config directory outside the home whole", func(t *testing.T) {
		withUninstallDeps(t, UninstallDeps{Saver: &stopperFake{stopped: true}})
		t.Setenv("XDG_CONFIG_HOME", "/srv/config")

		stdout, _, err := runRootCmd(t, "uninstall")

		if want := "Portal's save daemon stopped. Saved sessions and config are untouched at /srv/config/portal/.\n"; err != nil || stdout.String() != want {
			t.Errorf("uninstall = %q, %v; want %q", stdout, err, want)
		}
	})

	t.Run("a stop that fails fails the command with its reason, printing nothing", func(t *testing.T) {
		refused := errors.New("the saver session's process is not sent SIGTERM: pid 4242 is no portal state daemon")
		withUninstallDeps(t, UninstallDeps{Saver: &stopperFake{err: refused}})

		stdout, _, err := runRootCmd(t, "uninstall")

		if !errors.Is(err, refused) {
			t.Fatalf("uninstall = %v, want the stop's failure", err)
		}
		if want := "couldn't stop Portal's save daemon: " + refused.Error(); err.Error() != want {
			t.Errorf("error = %q, want %q", err, want)
		}
		if stdout.Len() != 0 {
			t.Errorf("stdout = %q, want nothing", stdout)
		}
	})

	t.Run("it keeps the latch, and writes nothing to the state directory", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("PORTAL_STATE_DIR", dir)
		if err := state.WriteLatch(dir, state.Latch{BootID: "4DAA6ADD-0334-49EC-9FC6-8323D691249E", Version: "0.13.0"}); err != nil {
			t.Fatal(err)
		}
		before := dirContents(t, dir)
		withUninstallDeps(t, UninstallDeps{Saver: &stopperFake{stopped: true}})

		if _, _, err := runRootCmd(t, "uninstall"); err != nil {
			t.Fatalf("uninstall: %v", err)
		}

		if after := dirContents(t, dir); !maps.Equal(before, after) {
			t.Errorf("state directory = %v, want it as it was, %v", after, before)
		}
	})

	t.Run("it runs no bootstrap", func(t *testing.T) {
		withBootstrapDeps(t, BootstrapDeps{Orchestrator: panicRunner{}})
		withUninstallDeps(t, UninstallDeps{Saver: &stopperFake{}})

		if _, _, err := runRootCmd(t, "uninstall"); err != nil {
			t.Fatalf("uninstall: %v", err)
		}
	})

	t.Run("unstaged, it stops the session backend's saver", func(t *testing.T) {
		fake := backendtest.New()
		withBackendDeps(t, BackendDeps{Backend: fake})

		stdout, _, err := runRootCmd(t, "uninstall")

		if err != nil {
			t.Fatalf("uninstall: %v", err)
		}
		if want := "Portal's save daemon isn't running. Saved sessions and config are untouched at ~/.config/portal/.\n"; stdout.String() != want {
			t.Errorf("stdout = %q, want %q", stdout, want)
		}
		calls := fake.Calls()
		if len(calls) != 1 || calls[0].Op != backendtest.OpSession || calls[0].Name != saver.SessionName {
			t.Errorf("backend calls = %+v, want one read of %s", calls, saver.SessionName)
		}
	})
}

func TestUninstall_IsBootstrapExempt(t *testing.T) {
	if !bootstrapExempt["uninstall"] {
		t.Fatal("uninstall must be registered in bootstrapExempt")
	}
}

// dirContents is every file under dir and its bytes.
func dirContents(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		files[path] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}
