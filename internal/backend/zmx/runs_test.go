package zmx_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/shellquote"
)

func TestRuns(t *testing.T) {
	t.Run("it runs zmx version bound on the directory, out of the caller's session", func(t *testing.T) {
		binary, log := fakeZmx(t, `printf 'zmx\t\t1.0.1\n'`)
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

// A release older than the first to speak Portal's protocol is refused by
// its version line alone; every other passes, the protocol its daemons speak
// judging it, and so does a version line Runs can't read.
func TestRuns_JudgesTheVersionLine(t *testing.T) {
	runs := func(t *testing.T, out string) error {
		t.Helper()
		binary, _ := fakeZmx(t, "printf '%s' "+shellquote.Single(out))
		return newBackendRunning(t, binary, socketDir(t), "PATH=/usr/bin:/bin").Runs(t.Context())
	}

	for out, release := range map[string]string{
		"zmx\t\t0.9.2\nghostty_vt\tghostty-1.3.0\n": "0.9.2",
		"zmx 0.4.12\n":     "0.4.12",
		"zmx\t\t0.9.2-dev": "0.9.2-dev",
	} {
		t.Run("it refuses the release "+release+", naming it", func(t *testing.T) {
			err := runs(t, out)

			older, ok := errors.AsType[*zmx.OlderReleaseError](err)
			if !ok || older.Release != release {
				t.Fatalf("Runs = %v, want an *OlderReleaseError of %s", err, release)
			}
			if want := "zmx " + release + " is older than 1.0.0, the first release that speaks protocol 1"; err.Error() != want {
				t.Errorf("Runs = %q, want %q", err, want)
			}
		})
	}

	for name, out := range map[string]string{
		"the first release":                          "zmx\t\t1.0.0\n",
		"a later patch":                              "zmx\t\t1.0.1\nghostty_vt\tghostty-1.4.1-dev\n",
		"a later major":                              "zmx 10.0.0\n",
		"a pre-release of the first release":         "zmx 1.0.0-rc.1\n",
		"a build of a later release":                 "zmx 1.2.0+abc123\n",
		"a release that isn't three numbers":         "zmx 0.9\n",
		"a release that isn't numbers":               "zmx dev\n",
		"a release with a leading zero":              "zmx 00.9.2\n",
		"a line naming no zmx":                       "0.9.2\n",
		"a line naming another program":              "ZMX 0.9.2\n",
		"a line with more than the release":          "zmx 0.9.2 (dev)\n",
		"no output":                                  "",
		"an older release on a line after the first": "version\nzmx 0.9.2\n",
	} {
		t.Run("it passes "+name, func(t *testing.T) {
			if err := runs(t, out); err != nil {
				t.Errorf("Runs = %v, want nil", err)
			}
		})
	}
}
