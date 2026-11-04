package state_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"syscall"
	"testing"

	"github.com/leeovery/portal/internal/state"
)

// unsafeNames are names that are not a single path element.
var unsafeNames = []string{"", ".", "..", "/", "a/b", "../escape", "nul\x00byte"}

// withUmask runs the test under mask, so a mode the code asks for is not
// mistaken for one the default umask happened to give.
func withUmask(t *testing.T, mask int) {
	t.Helper()
	prev := syscall.Umask(mask)
	t.Cleanup(func() { syscall.Umask(prev) })
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := fi.Mode().Perm(); got != want {
		t.Errorf("%s mode = %o, want %o", path, got, want)
	}
}

func assertAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("stat %s = %v, want it absent", path, err)
	}
}

func TestFlag(t *testing.T) {
	t.Run("it round-trips: set, exists, clear", func(t *testing.T) {
		dir := t.TempDir()

		if got, err := state.Restoring.Exists(dir); err != nil || got {
			t.Fatalf("Exists before Set = %v, %v; want false", got, err)
		}
		if err := state.Restoring.Set(dir); err != nil {
			t.Fatalf("Set: %v", err)
		}
		if err := state.Restoring.Set(dir); err != nil {
			t.Fatalf("Set of a raised flag: %v", err)
		}
		if got, err := state.Restoring.Exists(dir); err != nil || !got {
			t.Errorf("Exists after Set = %v, %v; want true", got, err)
		}
		if removed, err := state.Restoring.Clear(dir); err != nil || !removed {
			t.Errorf("Clear = %v, %v; want it removed", removed, err)
		}
		if removed, err := state.Restoring.Clear(dir); err != nil || removed {
			t.Errorf("Clear of a lowered flag = %v, %v; want nothing removed, no error", removed, err)
		}
		if got, err := state.Restoring.Exists(dir); err != nil || got {
			t.Errorf("Exists after Clear = %v, %v; want false", got, err)
		}
	})

	t.Run("it is the restoring file in the state directory, made 0600 in a directory made 0700", func(t *testing.T) {
		withUmask(t, 0)
		dir := filepath.Join(t.TempDir(), "state")

		if err := state.Restoring.Set(dir); err != nil {
			t.Fatalf("Set: %v", err)
		}

		if got, want := state.Restoring.Path(dir), filepath.Join(dir, "restoring"); got != want {
			t.Errorf("Path = %q, want %q", got, want)
		}
		assertMode(t, dir, 0o700)
		assertMode(t, state.Restoring.Path(dir), 0o600)
	})

	t.Run("a flag that can't be looked for is an error, not a lowered flag", func(t *testing.T) {
		blocker := filepath.Join(t.TempDir(), "file")
		if err := os.WriteFile(blocker, nil, 0o600); err != nil {
			t.Fatal(err)
		}

		if got, err := state.Restoring.Exists(blocker); err == nil {
			t.Errorf("Exists under a file = %v, nil; want an error", got)
		}
	})
}

func TestSessionFlag(t *testing.T) {
	for _, flag := range []state.SessionFlag{state.Hydrating, state.ResumePending} {
		t.Run(string(flag)+" round-trips: set, exists, list, clear", func(t *testing.T) {
			dir := t.TempDir()

			for _, name := range []string{"web-Qm3pLz", "api-x7Kd9a", "api-x7Kd9a"} {
				if err := flag.Set(dir, name); err != nil {
					t.Fatalf("Set(%s): %v", name, err)
				}
			}
			for name, want := range map[string]bool{"api-x7Kd9a": true, "web-Qm3pLz": true, "api": false} {
				if got, err := flag.Exists(dir, name); err != nil || got != want {
					t.Errorf("Exists(%s) = %v, %v; want %v", name, got, err, want)
				}
			}
			if got, err := flag.List(dir); err != nil || !slices.Equal(got, []string{"api-x7Kd9a", "web-Qm3pLz"}) {
				t.Errorf("List = %v, %v; want both, in name order", got, err)
			}
			if removed, err := flag.Clear(dir, "api-x7Kd9a"); err != nil || !removed {
				t.Errorf("Clear = %v, %v; want it removed", removed, err)
			}
			if removed, err := flag.Clear(dir, "api-x7Kd9a"); err != nil || removed {
				t.Errorf("Clear of a lowered flag = %v, %v; want nothing removed, no error", removed, err)
			}
			if got, err := flag.List(dir); err != nil || !slices.Equal(got, []string{"web-Qm3pLz"}) {
				t.Errorf("List after Clear = %v, %v; want web-Qm3pLz alone", got, err)
			}
		})

		t.Run(string(flag)+" is a file named after the session, made 0600 in a directory made 0700", func(t *testing.T) {
			withUmask(t, 0)
			dir := filepath.Join(t.TempDir(), "state")

			if err := flag.Set(dir, "api-x7Kd9a"); err != nil {
				t.Fatalf("Set: %v", err)
			}

			path, err := flag.Path(dir, "api-x7Kd9a")
			if want := filepath.Join(dir, string(flag), "api-x7Kd9a"); err != nil || path != want {
				t.Errorf("Path = %q, %v; want %q", path, err, want)
			}
			assertMode(t, dir, 0o700)
			assertMode(t, filepath.Join(dir, string(flag)), 0o700)
			assertMode(t, path, 0o600)
		})

		t.Run(string(flag)+" lists none before its directory is made, and only regular files once it is", func(t *testing.T) {
			dir := t.TempDir()
			if got, err := flag.List(dir); err != nil || len(got) != 0 {
				t.Errorf("List of no directory = %v, %v; want none", got, err)
			}

			if err := flag.Set(dir, "api-x7Kd9a"); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(filepath.Join(dir, string(flag), "not-a-flag"), 0o700); err != nil {
				t.Fatal(err)
			}
			if got, err := flag.List(dir); err != nil || !slices.Equal(got, []string{"api-x7Kd9a"}) {
				t.Errorf("List = %v, %v; want the regular file alone", got, err)
			}
		})
	}

	t.Run("the two flags of one session are separate files", func(t *testing.T) {
		dir := t.TempDir()
		if err := state.Hydrating.Set(dir, "api-x7Kd9a"); err != nil {
			t.Fatal(err)
		}

		if got, err := state.ResumePending.Exists(dir, "api-x7Kd9a"); err != nil || got {
			t.Errorf("ResumePending.Exists = %v, %v; want false", got, err)
		}
	})

	t.Run("names that are a single path element are accepted", func(t *testing.T) {
		dir := t.TempDir()
		for _, name := range []string{".hidden", "a.b", "-dash", "with space", "...", "日本"} {
			if err := state.Hydrating.Set(dir, name); err != nil {
				t.Errorf("Set(%q): %v", name, err)
			}
		}
	})

	for _, name := range unsafeNames {
		t.Run("the name "+strconv.Quote(name)+" is refused by every call, writing nothing", func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "state")
			flag := state.Hydrating

			if _, err := flag.Path(dir, name); !errors.Is(err, state.ErrUnsafeName) {
				t.Errorf("Path = %v, want ErrUnsafeName", err)
			}
			if err := flag.Set(dir, name); !errors.Is(err, state.ErrUnsafeName) {
				t.Errorf("Set = %v, want ErrUnsafeName", err)
			}
			if _, err := flag.Exists(dir, name); !errors.Is(err, state.ErrUnsafeName) {
				t.Errorf("Exists = %v, want ErrUnsafeName", err)
			}
			if _, err := flag.Clear(dir, name); !errors.Is(err, state.ErrUnsafeName) {
				t.Errorf("Clear = %v, want ErrUnsafeName", err)
			}
			assertAbsent(t, dir)
		})
	}

	t.Run("clearing an unsafe name leaves the flag directory alone", func(t *testing.T) {
		dir := t.TempDir()
		flagDir := filepath.Join(dir, string(state.Hydrating))
		if err := os.Mkdir(flagDir, 0o700); err != nil {
			t.Fatal(err)
		}

		for _, name := range []string{"", "."} {
			_, _ = state.Hydrating.Clear(dir, name)
		}

		if _, err := os.Stat(flagDir); err != nil {
			t.Errorf("stat %s = %v, want the empty directory kept", flagDir, err)
		}
	})
}
