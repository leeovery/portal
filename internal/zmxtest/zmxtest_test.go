package zmxtest

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// procArgsOf builds a kern.procargs2 buffer as the kernel lays one out.
func procArgsOf(path string, argv, env []string) []byte {
	b := binary.NativeEndian.AppendUint32(nil, uint32(len(argv)))
	b = append(b, path...)
	b = append(b, 0, 0, 0, 0)
	for _, s := range append(slices.Clone(argv), env...) {
		b = append(append(b, s...), 0)
	}
	return append(b, 0, 'p', 't', 'r', '_', 'm', 'u', 'n', 'g', 'e', '=', 0)
}

func TestParseProcArgs(t *testing.T) {
	t.Run("it reads argv and the environment, and stops at the end of the environment", func(t *testing.T) {
		argv := []string{"/opt/zmx", "attach", "has space", ""}
		env := []string{"ZMX_DIR=/tmp/zx-1", "HOME=/h", "EMPTY="}

		got, err := parseProcArgs(procArgsOf("/opt/zmx", argv, env))

		if err != nil || !slices.Equal(got.argv, argv) || !slices.Equal(got.env, env) {
			t.Errorf("parseProcArgs = %+v, %v; want argv %q, env %q", got, err, argv, env)
		}
	})

	t.Run("it refuses a buffer cut short or with a bad count", func(t *testing.T) {
		whole := procArgsOf("/opt/zmx", []string{"zmx", "attach"}, nil)
		negative := binary.NativeEndian.AppendUint32(nil, 0xffffffff)
		for name, raw := range map[string][]byte{
			"empty":                  nil,
			"no executable path end": []byte{1, 0, 0, 0, '/', 'z'},
			"arguments cut short":    whole[:len(whole)-20],
			"negative count":         append(negative, "/x\x00a\x00"...),
		} {
			t.Run(name, func(t *testing.T) {
				if got, err := parseProcArgs(raw); !errors.Is(err, errProcArgs) {
					t.Errorf("parseProcArgs(%q) = %+v, %v; want errProcArgs", raw, got, err)
				}
			})
		}
	})
}

func FuzzParseProcArgs(f *testing.F) {
	f.Add(procArgsOf("/opt/zmx", []string{"zmx", "attach", "x"}, []string{"ZMX_DIR=/tmp/zx-1"}))
	f.Fuzz(func(t *testing.T, raw []byte) {
		got, err := parseProcArgs(raw)
		if err == nil && slices.ContainsFunc(got.env, func(kv string) bool { return kv == "" }) {
			t.Errorf("parseProcArgs(%q) returned an empty environment entry", raw)
		}
	})
}

// procTree fakes the process reader: each pid's ZMX_DIR, or "" for an
// environment the reader can't see, and its parent. A pid missing from either
// map fails that read.
type procTree struct {
	dirs    map[int]string
	parents map[int]int
}

func (p procTree) args(pid int) (procArgs, error) {
	dir, ok := p.dirs[pid]
	if !ok {
		return procArgs{}, errors.New("no such process")
	}
	if dir == "" {
		return procArgs{argv: []string{"/bin/sh"}}, nil
	}
	return procArgs{argv: []string{"zmx"}, env: []string{"HOME=/h", "ZMX_DIR=" + dir}}, nil
}

func (p procTree) parent(pid int) (int, error) {
	ppid, ok := p.parents[pid]
	if !ok {
		return 0, errors.New("no such process")
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
			if got := ownedThrough(10, dir, c.tree.args, c.tree.parent); got != c.want {
				t.Errorf("ownedThrough(10) over %+v = %v, want %v", c.tree, got, c.want)
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
	for value, ok := range map[string]bool{PoisonedDir: true, "": false, "/Users/lee/.local/state/zmx": false} {
		t.Run(value, func(t *testing.T) {
			err := checkPoisoned(func(string) string { return value })
			if (err == nil) != ok {
				t.Errorf("checkPoisoned with ZMX_DIR=%q = %v, want ok %v", value, err, ok)
			}
		})
	}
}

func TestSessionEnv(t *testing.T) {
	t.Run("it passes only PATH, TERM, USER, TMPDIR and the locale, with its own HOME and SHELL", func(t *testing.T) {
		environ := []string{
			"PATH=/usr/bin", "TERM=xterm-ghostty", "USER=lee", "TMPDIR=/var/t", "LANG=en_GB.UTF-8", "LC_ALL=C",
			"HOME=/Users/lee", "SHELL=/bin/zsh", "ZMX_DIR=/Users/lee/.local/state/zmx", "ZMX_SESSION=live",
			"ZMX_SESSION_PREFIX=pre-", "TMUX=/tmp/tmux", "SSH_AUTH_SOCK=/agent", "LCX=no",
		}

		got := sessionEnv(environ, "/tmp/home")

		want := []string{"PATH=/usr/bin", "TERM=xterm-ghostty", "USER=lee", "TMPDIR=/var/t", "LANG=en_GB.UTF-8", "LC_ALL=C", "HOME=/tmp/home", "SHELL=/bin/sh"}
		if !slices.Equal(got, want) {
			t.Errorf("sessionEnv = %q, want %q", got, want)
		}
	})
}
