package state_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/leeovery/portal/internal/state"
)

// unsafeNames are names that are not a single path element.
var unsafeNames = []string{"", ".", "..", "/", "a/b", "../escape", "nul\x00byte"}

// Each stem is a session's name, then the first 12 hex digits of the SHA-256
// of the name's bytes, worked out apart from the code.
const (
	apiStem       = "api-x7Kd9a-2b7b3c0a554a"
	neighbourStem = "api-x7Kd9a-2-69c2a1faefed"
	webStem       = "web-Qm3pLz-1f66150b103c"
	fooUpperHash  = "1cbec737f863"
	// The stems "" and "." would have, which name no session.
	emptyNameHash = "e3b0c44298fc"
	dotNameHash   = "cdb4ee2aea69"
)

// foldsCase reports whether the filesystem holding dir takes two names that
// differ only in case for one.
func foldsCase(t *testing.T, dir string) bool {
	t.Helper()
	probe := filepath.Join(dir, "CaseProbe")
	writeFile(t, probe, "")
	defer func() { _ = os.Remove(probe) }()
	_, err := os.Stat(filepath.Join(dir, "caseprobe"))
	return err == nil
}

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

	t.Run("a directory at its path is no raised flag", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.Mkdir(state.Restoring.Path(dir), 0o700); err != nil {
			t.Fatal(err)
		}

		if got, err := state.Restoring.Exists(dir); err != nil || got {
			t.Errorf("Exists = %v, %v; want false", got, err)
		}
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

func TestSessionFlag_Raised(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	writeFile(t, blocker, "")
	raised := t.TempDir()
	if err := state.Hydrating.Set(raised, "api-x7Kd9a"); err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct {
		dir  string
		want bool
	}{
		"a raised flag is raised":                   {raised, true},
		"a lowered flag is not":                     {t.TempDir(), false},
		"a flag that can't be looked for is raised": {blocker, true},
	} {
		t.Run(name, func(t *testing.T) {
			if got := state.Hydrating.Raised(c.dir, "api-x7Kd9a"); got != c.want {
				t.Errorf("Raised = %v, want %v", got, c.want)
			}
		})
	}
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

		t.Run(string(flag)+" is a file named by the session's name and its hash, made 0600 in a directory made 0700", func(t *testing.T) {
			withUmask(t, 0)
			dir := filepath.Join(t.TempDir(), "state")

			if err := flag.Set(dir, "api-x7Kd9a"); err != nil {
				t.Fatalf("Set: %v", err)
			}

			path, err := flag.Path(dir, "api-x7Kd9a")
			if want := filepath.Join(dir, string(flag), apiStem); err != nil || path != want {
				t.Errorf("Path = %q, %v; want %q", path, err, want)
			}
			assertMode(t, dir, 0o700)
			assertMode(t, filepath.Join(dir, string(flag)), 0o700)
			assertMode(t, path, 0o600)
		})

		t.Run(string(flag)+" lists none before its directory is made, and once it is only regular files named by a session's stem", func(t *testing.T) {
			dir := t.TempDir()
			if got, err := flag.List(dir); err != nil || len(got) != 0 {
				t.Errorf("List of no directory = %v, %v; want none", got, err)
			}

			if err := flag.Set(dir, "api-x7Kd9a"); err != nil {
				t.Fatal(err)
			}
			flagDir := filepath.Join(dir, string(flag))
			if err := os.Mkdir(filepath.Join(flagDir, webStem), 0o700); err != nil {
				t.Fatal(err)
			}
			for _, stray := range []string{".DS_Store", "api-x7Kd9a", "api-x7Kd9a-000000000000", "-" + emptyNameHash, ".-" + dotNameHash} {
				writeFile(t, filepath.Join(flagDir, stray), "")
			}
			if err := os.Symlink(filepath.Join(flagDir, apiStem), filepath.Join(flagDir, "Foo-"+fooUpperHash)); err != nil {
				t.Fatal(err)
			}
			if got, err := flag.List(dir); err != nil || !slices.Equal(got, []string{"api-x7Kd9a"}) {
				t.Errorf("List = %v, %v; want api-x7Kd9a alone", got, err)
			}
		})

		t.Run(string(flag)+" counts only a regular file as raised", func(t *testing.T) {
			dir := t.TempDir()
			if err := os.MkdirAll(filepath.Join(dir, string(flag), apiStem), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(dir, "elsewhere"), filepath.Join(dir, string(flag), webStem)); err != nil {
				t.Fatal(err)
			}
			writeFile(t, filepath.Join(dir, "elsewhere"), "")

			for _, name := range []string{"api-x7Kd9a", "web-Qm3pLz"} {
				if got, err := flag.Exists(dir, name); err != nil || got {
					t.Errorf("Exists(%s) = %v, %v; want false", name, got, err)
				}
			}
		})
	}

	t.Run("names one filesystem may fold together keep a flag each", func(t *testing.T) {
		dir := t.TempDir()
		upper, _ := state.Hydrating.Path(dir, "Foo")
		lower, _ := state.Hydrating.Path(dir, "foo")
		if strings.EqualFold(upper, lower) {
			t.Fatalf("Path(Foo) = %q and Path(foo) = %q fold to one name", upper, lower)
		}
		if err := state.Hydrating.Set(dir, "Foo"); err != nil {
			t.Fatal(err)
		}

		if got, err := state.Hydrating.Exists(dir, "foo"); err != nil || got {
			t.Errorf("Exists(foo) = %v, %v; want false, Foo's flag being Foo's", got, err)
		}
		if removed, err := state.Hydrating.Clear(dir, "foo"); err != nil || removed {
			t.Errorf("Clear(foo) = %v, %v; want nothing removed", removed, err)
		}
		if got, err := state.Hydrating.List(dir); err != nil || !slices.Equal(got, []string{"Foo"}) {
			t.Errorf("List = %v, %v; want Foo alone, still raised", got, err)
		}
		if !foldsCase(t, dir) {
			t.Log("this filesystem keeps case, so only the paths' comparison tested the rule")
		}
	})

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
