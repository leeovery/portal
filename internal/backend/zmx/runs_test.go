package zmx_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
)

func TestRuns(t *testing.T) {
	t.Run("it runs zmx version bound on the directory, out of the caller's session", func(t *testing.T) {
		binary, log := fakeZmx(t, "echo 'zmx 0.8.1'")
		zmxDir := socketDir(t)
		b := newBackendRunning(t, binary, zmxDir,
			"PATH=/usr/bin:/bin", "ZMX_DIR=/Users/lee/.local/state/zmx", "ZMX_SESSION=live", "ZMX_SESSION_PREFIX=pre-",
			"TMUX=/tmp/tmux-501/default,1,0", "TMUX_PANE=%1")

		if err := b.Runs(t.Context()); err != nil {
			t.Fatalf("Runs = %v, want nil", err)
		}

		if got := strings.Split(strings.TrimSuffix(string(ran(t, log, "args")), "\x00"), "\x00"); !slices.Equal(got, []string{"version"}) {
			t.Errorf("zmx args = %q, want [version]", got)
		}
		env := strings.Split(string(ran(t, log, "env")), "\x00")
		for key, want := range map[string][]string{
			"ZMX_DIR":            {zmxDir},
			"ZMX_SESSION":        nil,
			"ZMX_SESSION_PREFIX": nil,
			"TMUX":               nil,
			"TMUX_PANE":          nil,
			"PATH":               {"/usr/bin:/bin"},
		} {
			if got := envValues(env, key); !slices.Equal(got, want) {
				t.Errorf("zmx env %s = %q, want %q", key, got, want)
			}
		}
	})

	t.Run("a run that fails is an error carrying what zmx printed", func(t *testing.T) {
		binary, _ := fakeZmx(t, "echo 'error: NotDir' >&2; exit 1")

		err := newBackendRunning(t, binary, socketDir(t), "PATH=/usr/bin:/bin").Runs(t.Context())

		if err == nil || !strings.Contains(err.Error(), "error: NotDir") {
			t.Errorf("Runs = %v, want zmx's failure with its output", err)
		}
	})

	t.Run("a binary that isn't there is an error", func(t *testing.T) {
		if err := newBackend(t, socketDir(t), "PATH=/usr/bin:/bin").Runs(t.Context()); err == nil {
			t.Error("Runs = nil for a binary that isn't there")
		}
	})

	t.Run("a run past its ceiling is a timeout", func(t *testing.T) {
		binary, _ := fakeZmx(t, "sleep 5")
		b := newBackendRunning(t, binary, socketDir(t), "PATH=/usr/bin:/bin")
		zmx.ShortenCreateCeiling(b, 300*time.Millisecond)

		if err := b.Runs(t.Context()); !errors.Is(err, backend.ErrTimeout) {
			t.Errorf("Runs = %v, want ErrTimeout", err)
		}
	})
}
