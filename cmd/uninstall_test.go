package cmd

import (
	"context"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/saver"
	"github.com/leeovery/portal/internal/state"
)

// removal is uninstall's second line, whatever it did.
const removal = "To remove Portal completely, uninstall the binary and delete that directory.\n"

// stopperFake is a saver whose daemon is pid when present, and whose reads
// and stops fail with readErr and stopErr.
type stopperFake struct {
	pid     int
	present bool
	readErr error
	stopErr error
	stops   []int
}

func (s *stopperFake) SaverPID(context.Context) (int, bool, error) {
	return s.pid, s.present, s.readErr
}

func (s *stopperFake) Stop(_ context.Context, pid int) error {
	s.stops = append(s.stops, pid)
	return s.stopErr
}

func TestUninstall(t *testing.T) {
	t.Run("it stops the daemon it read and says where the saved sessions and config are", func(t *testing.T) {
		stop := &stopperFake{pid: 4242, present: true}
		withUninstallDeps(t, UninstallDeps{Saver: stop})

		stdout, _, err := runRootCmd(t, "uninstall")

		if err != nil {
			t.Fatalf("uninstall: %v", err)
		}
		if want := "Portal's save daemon stopped. Saved sessions and config are untouched at ~/.config/portal/.\n" + removal; stdout.String() != want {
			t.Errorf("stdout = %q, want %q", stdout, want)
		}
		if !slices.Equal(stop.stops, []int{4242}) {
			t.Errorf("stops = %v, want the daemon read, 4242, once", stop.stops)
		}
	})

	t.Run("with no saver running it says so, stops nothing, and succeeds", func(t *testing.T) {
		stop := &stopperFake{}
		withUninstallDeps(t, UninstallDeps{Saver: stop})

		stdout, _, err := runRootCmd(t, "uninstall")

		if err != nil {
			t.Fatalf("uninstall: %v", err)
		}
		if want := "Portal's save daemon isn't running. Saved sessions and config are untouched at ~/.config/portal/.\n" + removal; stdout.String() != want {
			t.Errorf("stdout = %q, want %q", stdout, want)
		}
		if len(stop.stops) != 0 {
			t.Errorf("stops = %v, want none", stop.stops)
		}
	})

	t.Run("it names a config directory outside the home whole", func(t *testing.T) {
		withUninstallDeps(t, UninstallDeps{Saver: &stopperFake{pid: 4242, present: true}})
		t.Setenv("XDG_CONFIG_HOME", "/srv/config")

		stdout, _, err := runRootCmd(t, "uninstall")

		if want := "Portal's save daemon stopped. Saved sessions and config are untouched at /srv/config/portal/.\n" + removal; err != nil || stdout.String() != want {
			t.Errorf("uninstall = %q, %v; want %q", stdout, err, want)
		}
	})

	refused := errors.New("pid 4242 isn't confirmed as the save daemon, so nothing was sent to it")
	for name, fake := range map[string]*stopperFake{
		"a stop that fails":                  {pid: 4242, present: true, stopErr: refused},
		"a saver whose daemon can't be read": {readErr: refused},
	} {
		t.Run(name+" fails the command with its reason, printing nothing", func(t *testing.T) {
			withUninstallDeps(t, UninstallDeps{Saver: fake})

			stdout, _, err := runRootCmd(t, "uninstall")

			if !errors.Is(err, refused) {
				t.Fatalf("uninstall = %v, want the failure", err)
			}
			if want := "couldn't stop Portal's save daemon: " + refused.Error(); err.Error() != want {
				t.Errorf("error = %q, want %q", err, want)
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want nothing", stdout)
			}
		})
	}

	t.Run("a session backend that can't be bound fails the command the same way", func(t *testing.T) {
		t.Setenv("ZMX_DIR", "relative/zmx")

		stdout, _, err := runRootCmd(t, "uninstall")

		if err == nil || !strings.HasPrefix(err.Error(), "couldn't stop Portal's save daemon: ") {
			t.Errorf("uninstall = %v, want it to begin \"couldn't stop Portal's save daemon: \"", err)
		}
		if stdout.Len() != 0 {
			t.Errorf("stdout = %q, want nothing", stdout)
		}
	})

	t.Run("a session backend whose reach names no directory fails the command, reading nothing", func(t *testing.T) {
		fake := backendtest.New(backend.Session{Name: saver.SessionName, PID: 4242})
		withBackendDeps(t, BackendDeps{Backend: fake})

		_, _, err := runRootCmd(t, "uninstall")

		if err == nil || !strings.HasPrefix(err.Error(), "couldn't stop Portal's save daemon: ") {
			t.Errorf("uninstall = %v, want it to begin \"couldn't stop Portal's save daemon: \"", err)
		}
		if calls := fake.Calls(); len(calls) != 0 {
			t.Errorf("backend calls = %+v, want none", calls)
		}
	})

	t.Run("it keeps the latch, and writes nothing to the state directory", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("PORTAL_STATE_DIR", dir)
		if err := state.WriteLatch(dir, state.Latch{BootID: "4DAA6ADD-0334-49EC-9FC6-8323D691249E", Version: "0.13.0"}); err != nil {
			t.Fatal(err)
		}
		before := dirContents(t, dir)
		withUninstallDeps(t, UninstallDeps{Saver: &stopperFake{pid: 4242, present: true}})

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

	t.Run("unstaged, it reads the session backend's saver", func(t *testing.T) {
		fake := backendtest.New()
		fake.SetReachEnv([]string{"ZMX_DIR=/tmp/zx-uninstall"})
		withBackendDeps(t, BackendDeps{Backend: fake})

		stdout, _, err := runRootCmd(t, "uninstall")

		if err != nil {
			t.Fatalf("uninstall: %v", err)
		}
		if want := "Portal's save daemon isn't running. Saved sessions and config are untouched at ~/.config/portal/.\n" + removal; stdout.String() != want {
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
