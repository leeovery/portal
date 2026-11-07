package zmxtest

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/proctable"
	"github.com/leeovery/portal/internal/proctest"
)

// procTree fakes the process reader: each pid's ZMX_DIR, or "" for an
// environment the reader can't see, and its parent. A pid missing from either
// map fails that read. reusedAfterArgs stands in for a pid that exits once
// its args are read and is taken by a process with the parent it names.
// parentReusedAfterRead stands in for a child whose parent exits once the
// child's parent is read: the child is reparented to pid 1, and the parent's
// pid is taken by a process whose ZMX_DIR it names.
type procTree struct {
	dirs                  map[int]string
	parents               map[int]int
	reusedAfterArgs       map[int]int
	parentReusedAfterRead map[int]string
}

func (p *procTree) List() ([]proctable.Process, error) {
	var procs []proctable.Process
	for pid := range p.dirs {
		procs = append(procs, proctable.Process{PID: pid, PPID: p.parents[pid]})
	}
	return procs, nil
}

func (p *procTree) Args(pid int) (proctable.Args, error) {
	dir, ok := p.dirs[pid]
	if ppid, reused := p.reusedAfterArgs[pid]; reused {
		p.dirs[pid], p.parents[pid] = "", ppid
	}
	if !ok {
		return proctable.Args{}, errors.New("no such process")
	}
	if dir == "" {
		return proctable.Args{Argv: []string{"/bin/sh"}}, nil
	}
	return proctable.Args{Argv: []string{"zmx"}, Env: []string{"HOME=/h", "ZMX_DIR=" + dir}}, nil
}

func (p *procTree) Parent(pid int) (int, error) {
	ppid, ok := p.parents[pid]
	if !ok {
		return 0, errors.New("no such process")
	}
	if dir, reused := p.parentReusedAfterRead[pid]; reused {
		delete(p.parentReusedAfterRead, pid)
		p.dirs[ppid], p.parents[pid] = dir, 1
	}
	return ppid, nil
}

func TestOwnedThrough(t *testing.T) {
	const dir = "/tmp/zx-1"
	for name, c := range map[string]struct {
		tree procTree
		want bool
	}{
		"its own environment proves it": {
			procTree{dirs: map[int]string{10: dir}}, true,
		},
		"a parent's environment proves it, through hidden ones": {
			procTree{dirs: map[int]string{10: "", 11: "", 12: dir}, parents: map[int]int{10: 11, 11: 12}}, true,
		},
		"another directory's environment does not": {
			procTree{dirs: map[int]string{10: "", 12: "/tmp/zx-2"}, parents: map[int]int{10: 12}}, false,
		},
		"a chain ending at pid 1 does not, whatever pid 1 holds": {
			procTree{dirs: map[int]string{1: dir, 10: ""}, parents: map[int]int{10: 1}}, false,
		},
		"a chain ending at pid 0 does not": {
			procTree{dirs: map[int]string{10: ""}, parents: map[int]int{10: 0}}, false,
		},
		"a cycle does not": {
			procTree{dirs: map[int]string{10: "", 11: ""}, parents: map[int]int{10: 11, 11: 10}}, false,
		},
		"a failed environment read does not, even under a proven parent": {
			procTree{dirs: map[int]string{12: dir}, parents: map[int]int{10: 12}}, false,
		},
		"a failed parent read does not": {
			procTree{dirs: map[int]string{10: ""}}, false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := ownedThrough(10, dir, &c.tree); got != c.want {
				t.Errorf("ownedThrough(10) over %+v = %v, want %v", c.tree, got, c.want)
			}
		})
	}
}

// A proof reads each parent after the args it vouches for: a pid whose holder
// exits once its args are read, and is taken by a stranger, is never proved
// through the parent its last holder had, as a listing taken first would.
func TestOwnership_ReadsEachParentAfterItsArgs(t *testing.T) {
	const dir = "/tmp/zx-1"
	world := func(reused bool) *procTree {
		tree := &procTree{dirs: map[int]string{10: "", 20: dir}, parents: map[int]int{10: 20, 20: 1}}
		if reused {
			tree.reusedAfterArgs = map[int]int{10: 30}
		}
		return tree
	}
	use := func(t *testing.T, r proctest.Reader) {
		old := processes
		processes = r
		t.Cleanup(func() { processes = old })
	}

	use(t, world(false))
	if !ownedBy(10, dir) {
		t.Fatal("ownedBy(10) = false while its parent is the directory's daemon, want it owned")
	}

	use(t, world(true))
	if ownedBy(10, dir) {
		t.Error("ownedBy(10) = true for a pid a stranger took, want it not owned")
	}

	use(t, world(true))
	mine, err := owned(dir)
	if err != nil || slices.Contains(mine, 10) || !slices.Contains(mine, 20) {
		t.Errorf("owned = %v, %v; want the daemon 20 and never the pid a stranger took", mine, err)
	}
}

// A proof checks the chain it walked still stands once a parent's args prove
// the directory: a child whose parent exits, and whose parent's pid a test's
// own zmx then takes, is never proved through that zmx.
func TestOwnership_ChecksTheChainStandsAfterTheProof(t *testing.T) {
	const dir = "/tmp/zx-1"
	for name, c := range map[string]struct {
		tree procTree
		want bool
	}{
		"a parent that stood through the proof proves its child": {
			procTree{dirs: map[int]string{10: "", 20: dir}, parents: map[int]int{10: 20, 20: 1}}, true,
		},
		"a parent whose pid a test's zmx took after the child's parent was read does not": {
			procTree{
				dirs:                  map[int]string{10: "", 20: ""},
				parents:               map[int]int{10: 20, 20: 1},
				parentReusedAfterRead: map[int]string{10: dir},
			}, false,
		},
		"nor does one taken further up the chain": {
			procTree{
				dirs:                  map[int]string{10: "", 20: "", 30: ""},
				parents:               map[int]int{10: 20, 20: 30, 30: 1},
				parentReusedAfterRead: map[int]string{20: dir},
			}, false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := ownedThrough(10, dir, &c.tree); got != c.want {
				t.Errorf("ownedThrough(10) = %v, want %v", got, c.want)
			}
		})
	}
}

func TestProcessReader(t *testing.T) {
	t.Run("it reads this process's own argv", func(t *testing.T) {
		if runtime.GOOS != "darwin" {
			t.Skip("zmxtest reads processes only on darwin")
		}
		if err := checkProcessReader(); err != nil {
			t.Error(err)
		}
	})
}

func TestPinnedBinary(t *testing.T) {
	t.Run("it finds the build of the pinned commit", func(t *testing.T) {
		repo := t.TempDir()
		pin(t, repo, "abc123\n")
		binary := filepath.Join(repo, ".cache", "zmx", "abc123", "zmx")
		write(t, binary, "", 0o755)

		if got, err := pinnedBinary(repo); err != nil || got != binary {
			t.Errorf("pinnedBinary = %q, %v; want %q", got, err, binary)
		}
	})

	t.Run("it names the command to run when the build is missing or not runnable", func(t *testing.T) {
		repo := t.TempDir()
		pin(t, repo, "abc123")
		if _, err := pinnedBinary(repo); err == nil || !strings.Contains(err.Error(), "scripts/zmx-build") {
			t.Errorf("pinnedBinary with no build = %v, want it to name scripts/zmx-build", err)
		}
		write(t, filepath.Join(repo, ".cache", "zmx", "abc123", "zmx"), "", 0o644)
		if _, err := pinnedBinary(repo); err == nil || !strings.Contains(err.Error(), "scripts/zmx-build") {
			t.Errorf("pinnedBinary with a build that cannot run = %v, want it to name scripts/zmx-build", err)
		}
	})

	t.Run("it reads the pin this repository holds", func(t *testing.T) {
		pinned, err := os.ReadFile(filepath.Join("..", "..", "scripts", "zmx-commit"))
		if err != nil {
			t.Fatal(err)
		}
		if commit := strings.TrimSpace(string(pinned)); len(commit) != 40 {
			t.Errorf("scripts/zmx-commit pins %q, want a full commit hash", commit)
		}
	})
}

func TestResolveBinary(t *testing.T) {
	env := func(binary string) func(string) string {
		return func(key string) string {
			if key == BinaryEnv {
				return binary
			}
			return ""
		}
	}
	noRepo := func() (string, error) { return "", errors.New("no repository") }

	t.Run("it drives the build BinaryEnv names over the pinned one", func(t *testing.T) {
		binary := filepath.Join(t.TempDir(), "zmx")
		write(t, binary, "", 0o755)

		if got, err := resolveBinary(env(binary), noRepo); err != nil || got != binary {
			t.Errorf("resolveBinary = %q, %v; want %q", got, err, binary)
		}
	})

	t.Run("it refuses a named build that is missing, not runnable, a directory, relative or not named zmx", func(t *testing.T) {
		dir := t.TempDir()
		notRunnable := filepath.Join(dir, "plain", "zmx")
		write(t, notRunnable, "", 0o644)
		misnamed := filepath.Join(dir, "zmx-main")
		write(t, misnamed, "", 0o755)
		directory := filepath.Join(dir, "d", "zmx")
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
		for name, binary := range map[string]string{
			"missing":      filepath.Join(dir, "absent", "zmx"),
			"not runnable": notRunnable,
			"directory":    directory,
			"relative":     filepath.Join(".cache", "zmx", "abc123", "zmx"),
			"misnamed":     misnamed,
		} {
			t.Run(name, func(t *testing.T) {
				got, err := resolveBinary(env(binary), noRepo)
				if err == nil || !strings.Contains(err.Error(), BinaryEnv) || !strings.Contains(err.Error(), binary) {
					t.Errorf("resolveBinary with %s=%q = %q, %v; want an error naming both", BinaryEnv, binary, got, err)
				}
			})
		}
	})

	t.Run("it drives the pinned build when BinaryEnv is unset", func(t *testing.T) {
		repo := t.TempDir()
		pin(t, repo, "abc123")
		binary := filepath.Join(repo, ".cache", "zmx", "abc123", "zmx")
		write(t, binary, "", 0o755)

		got, err := resolveBinary(env(""), func() (string, error) { return repo, nil })
		if err != nil || got != binary {
			t.Errorf("resolveBinary = %q, %v; want %q", got, err, binary)
		}
	})
}

func pin(t *testing.T, repo, commit string) {
	t.Helper()
	write(t, filepath.Join(repo, "scripts", "zmx-commit"), commit, 0o644)
}

func write(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func TestCheckPoisoned(t *testing.T) {
	poisoned := map[string]string{"ZMX_DIR": PoisonedDir, "TMUX_TMPDIR": PoisonedTmpdir}
	t.Run("both poisoned", func(t *testing.T) {
		if err := checkPoisoned(func(name string) string { return poisoned[name] }); err != nil {
			t.Errorf("checkPoisoned = %v, want ok", err)
		}
	})
	for name := range poisoned {
		for _, value := range []string{"", "/Users/lee/.local/state/zmx", "/private/tmp"} {
			t.Run(name+"="+value, func(t *testing.T) {
				err := checkPoisoned(func(n string) string {
					if n == name {
						return value
					}
					return poisoned[n]
				})
				if err == nil || !strings.Contains(err.Error(), name) {
					t.Errorf("checkPoisoned with %s=%q = %v, want a failure naming it", name, value, err)
				}
			})
		}
	}
}

func TestSessionEnv(t *testing.T) {
	t.Run("it passes only PATH, TERM, USER, TMPDIR, TMUX_TMPDIR and the locale, with its own HOME and SHELL", func(t *testing.T) {
		environ := []string{
			"PATH=/usr/bin", "TERM=xterm-ghostty", "USER=lee", "TMPDIR=/var/t", "TMUX_TMPDIR=" + PoisonedTmpdir, "LANG=en_GB.UTF-8", "LC_ALL=C",
			"HOME=/Users/lee", "SHELL=/bin/zsh", "ZMX_DIR=/Users/lee/.local/state/zmx", "ZMX_SESSION=live",
			"ZMX_SESSION_PREFIX=pre-", "TMUX=/tmp/tmux", "SSH_AUTH_SOCK=/agent", "LCX=no",
		}

		got := sessionEnv(environ, "/tmp/home")

		want := []string{"PATH=/usr/bin", "TERM=xterm-ghostty", "USER=lee", "TMPDIR=/var/t", "TMUX_TMPDIR=" + PoisonedTmpdir, "LANG=en_GB.UTF-8", "LC_ALL=C", "HOME=/tmp/home", "SHELL=/bin/sh"}
		if !slices.Equal(got, want) {
			t.Errorf("sessionEnv = %q, want %q", got, want)
		}
	})
}
