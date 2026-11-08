package zmx_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
)

func TestKillByCLI(t *testing.T) {
	t.Run("it runs zmx kill on the name, bound on the directory, out of the caller's session", func(t *testing.T) {
		binary, log := fakeZmx(t, "")
		zmxDir := socketDir(t)
		b := newBackendRunning(t, binary, zmxDir,
			"PATH=/usr/bin:/bin", "ZMX_DIR=/Users/lee/.local/state/zmx", "ZMX_SESSION=live", "ZMX_SESSION_PREFIX=pre-",
			"TMUX=/tmp/tmux-501/default,1,0", "TMUX_PANE=%1")
		zmx.ShortenCeilings(b, zmx.CreateCeiling)

		if err := b.KillByCLI(t.Context(), "_portal-saver"); err != nil {
			t.Fatalf("KillByCLI = %v, want nil", err)
		}

		if got, want := strings.Split(strings.TrimSuffix(string(ran(t, log, "args")), "\x00"), "\x00"), []string{"kill", "_portal-saver"}; !slices.Equal(got, want) {
			t.Errorf("zmx args = %q, want %q", got, want)
		}
		env := strings.Split(string(ran(t, log, "env")), "\x00")
		for key, want := range map[string][]string{
			"ZMX_DIR":            {zmxDir},
			"ZMX_SESSION":        nil,
			"ZMX_SESSION_PREFIX": nil,
			"TMUX":               nil,
			"TMUX_PANE":          nil,
		} {
			if got := envValues(env, key); !slices.Equal(got, want) {
				t.Errorf("zmx env %s = %q, want %q", key, got, want)
			}
		}
	})

	t.Run("it refuses, running nothing, a name zmx kill reads as a flag or a prefix of many", func(t *testing.T) {
		for _, name := range []string{"-h", "--force", "work*", ""} {
			binary, log := fakeZmx(t, "")
			if err := newBackendRunning(t, binary, socketDir(t)).KillByCLI(t.Context(), name); !errors.Is(err, backend.ErrInvalidName) {
				t.Errorf("KillByCLI(%q) = %v, want ErrInvalidName", name, err)
			}
			notRun(t, log)
		}
	})

	t.Run("a run that fails is an error carrying what zmx printed", func(t *testing.T) {
		binary, _ := fakeZmx(t, "echo 'error: SessionNotFound' >&2; exit 1")

		b := newBackendRunning(t, binary, socketDir(t), "PATH=/usr/bin:/bin")
		zmx.ShortenCeilings(b, zmx.CreateCeiling)

		err := b.KillByCLI(t.Context(), "_portal-saver")

		if err == nil || !strings.Contains(err.Error(), "error: SessionNotFound") {
			t.Errorf("KillByCLI = %v, want zmx's failure with its output", err)
		}
	})

	t.Run("a run exiting 0 with the session's socket still there is an error carrying what zmx printed", func(t *testing.T) {
		dir := socketDir(t)
		serve(t, dir, "_portal-saver", silent)
		binary, _ := fakeZmx(t, "echo 'failed to kill session=_portal-saver: SessionUnresponsive' >&2")
		b := newBackendRunning(t, binary, dir, "PATH=/usr/bin:/bin")
		zmx.ShortenCeilings(b, zmx.CreateCeiling)

		err := b.KillByCLI(t.Context(), "_portal-saver")

		if err == nil || !strings.Contains(err.Error(), "failed to kill session=_portal-saver: SessionUnresponsive") {
			t.Errorf("KillByCLI = %v, want the socket left reported with zmx's output", err)
		}
	})

	t.Run("a run past its ceiling is a timeout", func(t *testing.T) {
		binary, _ := fakeZmx(t, "exec sleep 5")
		b := newBackendRunning(t, binary, socketDir(t), "PATH=/usr/bin:/bin")

		if err := b.KillByCLI(t.Context(), "_portal-saver"); !errors.Is(err, backend.ErrTimeout) {
			t.Errorf("KillByCLI = %v, want ErrTimeout", err)
		}
	})
}
