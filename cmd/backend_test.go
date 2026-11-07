package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/zmxtest"
	"github.com/spf13/cobra"
)

func TestWithBackendDeps_RefusesANilBackend(t *testing.T) {
	rec := &harnesstest.Recorder{}
	rec.Run(func() { requireStagedBackend(rec, BackendDeps{}) })

	if len(rec.Fatals) != 1 || !strings.Contains(rec.Fatals[0], "Backend is nil") {
		t.Errorf("fatals = %q, want one naming the nil Backend", rec.Fatals)
	}
}

// pathWithoutZmx leaves PATH holding nothing at all.
func pathWithoutZmx(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
}

// pathWithFakeZmx leaves PATH holding only a zmx that is never run, and
// returns its path.
func pathWithFakeZmx(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	binary := filepath.Join(dir, zmxProgram)
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write fake zmx: %v", err)
	}
	t.Setenv("PATH", dir)
	return binary
}

func TestAttachingBackend(t *testing.T) {
	t.Run("it is fatal when zmx is not on PATH", func(t *testing.T) {
		pathWithoutZmx(t)

		_, err := attachingBackend()
		if err == nil || err.Error() != "zmx not found on PATH; Portal needs zmx 1.0.1 or later" {
			t.Errorf("error = %v, want the zmx-missing line", err)
		}
	})

	t.Run("it binds the zmx dir the environment names", func(t *testing.T) {
		pathWithFakeZmx(t)
		dir := t.TempDir()
		t.Setenv("ZMX_DIR", dir)

		b, err := attachingBackend()
		if err != nil {
			t.Fatalf("attachingBackend: %v", err)
		}
		if got := b.(*zmx.Backend).Dir(); got != dir {
			t.Errorf("Dir() = %q, want %q", got, dir)
		}
	})

	t.Run("it hands back the staged backend without looking for zmx", func(t *testing.T) {
		pathWithoutZmx(t)
		fake := backendtest.New()
		withBackendDeps(t, BackendDeps{Backend: fake})

		b, err := attachingBackend()
		if err != nil || b != fake {
			t.Errorf("attachingBackend() = %v, %v; want the staged fake", b, err)
		}
	})
}

func TestSessionBackend(t *testing.T) {
	t.Run("it binds with no zmx on PATH", func(t *testing.T) {
		pathWithoutZmx(t)
		dir := t.TempDir()
		t.Setenv("ZMX_DIR", dir)

		b, err := sessionBackend()
		if err != nil {
			t.Fatalf("sessionBackend: %v", err)
		}
		if got := b.(*zmx.Backend).Dir(); got != dir {
			t.Errorf("Dir() = %q, want %q", got, dir)
		}
	})

	t.Run("it binds the package-wide poisoned zmx dir when a test stages no backend", func(t *testing.T) {
		b, err := sessionBackend()
		if err != nil {
			t.Fatalf("sessionBackend: %v", err)
		}
		if got := b.(*zmx.Backend).Dir(); got != zmxtest.PoisonedDir {
			t.Errorf("Dir() = %q, want the poison %q", got, zmxtest.PoisonedDir)
		}
		for _, name := range []string{"ZMX_SESSION", "ZMX_SESSION_PREFIX"} {
			if value, set := os.LookupEnv(name); set {
				t.Errorf("%s = %q, want it unset package-wide", name, value)
			}
		}
	})
}

func TestBackend_UnusableZmxDirIsFatalEverywhere(t *testing.T) {
	for _, dir := range []string{"relative/zmx", ""} {
		t.Run("it refuses ZMX_DIR="+dir, func(t *testing.T) {
			pathWithFakeZmx(t)
			t.Setenv("ZMX_DIR", dir)

			for name, bind := range map[string]func() (backend.Backend, error){
				"sessionBackend":   sessionBackend,
				"attachingBackend": attachingBackend,
			} {
				if _, err := bind(); err == nil || !strings.Contains(err.Error(), "not absolute") {
					t.Errorf("%s() error = %v, want the not-absolute refusal", name, err)
				}
			}
		})
	}

	t.Run("it stops list", func(t *testing.T) {
		t.Setenv("ZMX_DIR", "relative/zmx")

		if _, _, err := runRootCmd(t, "list"); err == nil || !strings.Contains(err.Error(), "not absolute") {
			t.Errorf("list error = %v, want the not-absolute refusal", err)
		}
	})

	t.Run("it stops open before the picker", func(t *testing.T) {
		pathWithFakeZmx(t)
		t.Setenv("ZMX_DIR", "relative/zmx")
		reached := stubPicker(t)

		if _, _, err := runRootCmd(t, "open"); err == nil || !strings.Contains(err.Error(), "not absolute") {
			t.Errorf("open error = %v, want the not-absolute refusal", err)
		}
		if *reached {
			t.Error("the picker opened on an unusable zmx dir")
		}
	})
}

func TestOpen_ZmxMissingIsFatal(t *testing.T) {
	for _, args := range [][]string{{"open"}, {"open", "dotfiles"}} {
		t.Run("it stops "+strings.Join(args, " ")+" before anything runs", func(t *testing.T) {
			resetBootstrapOnce(t)
			pathWithoutZmx(t)
			runner := &recordingRunner{}
			withBootstrapDeps(t, BootstrapDeps{Orchestrator: runner})
			reached := stubPicker(t)

			_, _, err := runRootCmd(t, args...)
			if !errors.Is(err, errZmxMissing) {
				t.Errorf("error = %v, want %v", err, errZmxMissing)
			}
			if *reached || runner.calls != 0 {
				t.Errorf("picker reached = %v, bootstrap runs = %d; want neither", *reached, runner.calls)
			}
		})
	}
}

// stubPicker stands in for the picker and reports whether it was reached.
func stubPicker(t *testing.T) *bool {
	t.Helper()
	reached := new(bool)
	withFuncSeam(t, &openTUIFunc, func(*cobra.Command, pickerLanding, []string) error {
		*reached = true
		return nil
	})
	return reached
}

func TestUserSessions(t *testing.T) {
	t.Run("it drops Portal's own sessions and keeps ones that do not answer", func(t *testing.T) {
		fake := backendtest.New(
			backend.Session{Name: "_portal-saver"},
			backend.Session{Name: "api-x7Kd9a"},
			backend.Session{Name: "web-Qm3pLz"},
		)
		fake.StopAnswering("web-Qm3pLz")

		sessions, err := userSessions(t.Context(), fake)
		if err != nil {
			t.Fatalf("userSessions: %v", err)
		}
		names := make([]string, 0, len(sessions))
		for _, s := range sessions {
			names = append(names, s.Name)
		}
		if want := []string{"api-x7Kd9a", "web-Qm3pLz"}; !slices.Equal(names, want) {
			t.Errorf("names = %v, want %v", names, want)
		}
	})

	t.Run("it returns the listing's error", func(t *testing.T) {
		fake := backendtest.New(backend.Session{Name: "api-x7Kd9a"})
		sentinel := errors.New("unreadable")
		fake.Fail(backendtest.OpList, sentinel)

		if _, err := userSessions(t.Context(), fake); !errors.Is(err, sentinel) {
			t.Errorf("error = %v, want %v", err, sentinel)
		}
	})
}
