package session_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/session"
)

func TestShellCommand(t *testing.T) {
	t.Run("it runs the joined words in the interactive shell, then re-execs it", func(t *testing.T) {
		got := session.ShellCommand([]string{"claude", "--resume", "|", "tee", "'log'"}, "/bin/zsh")
		want := []string{"/bin/zsh", "-ic", "claude --resume | tee 'log'\nexec '/bin/zsh'"}
		if !slices.Equal(got, want) {
			t.Errorf("ShellCommand = %q, want %q", got, want)
		}
	})

	t.Run("no command is no argv, the backend's own shell", func(t *testing.T) {
		for _, command := range [][]string{nil, {}} {
			if got := session.ShellCommand(command, "/bin/zsh"); got != nil {
				t.Errorf("ShellCommand(%q) = %q, want nil", command, got)
			}
		}
	})

	for name, dir := range map[string]string{
		"a space":        "My Apps",
		"a single quote": "it's",
		"nothing odd":    "bin",
	} {
		t.Run("a shell path holding "+name+" runs the command and re-execs that same shell", func(t *testing.T) {
			shell := stageFakeShell(t, filepath.Join(t.TempDir(), dir))
			if got, want := runShellCommand(t, []string{"echo", "cmd-ran"}, shell), fmt.Sprintf("flag=-ic\ncmd-ran\nreexec=%s\n", shell); got != want {
				t.Errorf("printed %q, want %q", got, want)
			}
		})
	}

	for name, command := range map[string][]string{
		"ends in &":            {"true &"},
		"ends in a comment":    {"echo cmd-ran # note"},
		"ends in a newline":    {"echo cmd-ran\n"},
		"spans lines":          {"echo one\necho two"},
		"is words ending in &": {"sleep", "0", "&"},
	} {
		t.Run("a command that "+name+" still reaches the re-exec", func(t *testing.T) {
			shell := stageFakeShell(t, t.TempDir())
			if got := runShellCommand(t, command, shell); !strings.HasSuffix(got, "reexec="+shell+"\n") {
				t.Errorf("printed %q, want it to end in the shell's re-exec", got)
			}
		})
	}
}

// runShellCommand runs ShellCommand's argv for command and returns what it
// printed.
func runShellCommand(t *testing.T, command []string, shell string) string {
	t.Helper()
	argv := session.ShellCommand(command, shell)
	var stdout, stderr bytes.Buffer
	run := exec.Command(argv[0], argv[1:]...)
	run.Stdout, run.Stderr = &stdout, &stderr
	if err := run.Run(); err != nil {
		t.Fatalf("running %q: %v (stderr %q)", argv, err, stderr.String())
	}
	return stdout.String()
}

// stageFakeShell writes a stand-in for the user's shell in dir. Given
// arguments it reports its flag and runs the script; given none, as the
// script's exec tail calls it, it reports the re-exec and stops.
func stageFakeShell(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create fake shell dir: %v", err)
	}
	path := filepath.Join(dir, "zsh")
	const body = `#!/bin/sh
if [ "$#" -eq 0 ]; then
	printf 'reexec=%s\n' "$0"
	exit 0
fi
printf 'flag=%s\n' "$1"
exec /bin/sh -c "$2"
`
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write fake shell: %v", err)
	}
	return path
}

func TestShellFromEnv(t *testing.T) {
	t.Run("returns SHELL env var when set", func(t *testing.T) {
		t.Setenv("SHELL", "/bin/zsh")
		if got := session.ShellFromEnv(); got != "/bin/zsh" {
			t.Errorf("ShellFromEnv() = %q, want %q", got, "/bin/zsh")
		}
	})

	t.Run("falls back to /bin/sh when SHELL not set", func(t *testing.T) {
		t.Setenv("SHELL", "")
		if got := session.ShellFromEnv(); got != "/bin/sh" {
			t.Errorf("ShellFromEnv() = %q, want %q", got, "/bin/sh")
		}
	})
}

type mockGitResolver struct {
	resolvedDir string
	err         error
}

func (m *mockGitResolver) Resolve(dir string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	if m.resolvedDir != "" {
		return m.resolvedDir, nil
	}
	return dir, nil
}

type mockProjectStore struct {
	upsertPath  string
	upsertName  string
	upsertVia   string
	upsertCount int
	upsertErr   error
}

func (m *mockProjectStore) Upsert(path, name, via string) error {
	m.upsertPath = path
	m.upsertName = name
	m.upsertVia = via
	m.upsertCount++
	return m.upsertErr
}

// ids hands out each id in turn, failing once they run out.
func ids(id ...string) session.IDGenerator {
	return func() (string, error) {
		if len(id) == 0 {
			return "", errors.New("ids exhausted")
		}
		next := id[0]
		id = id[1:]
		return next, nil
	}
}

func fixedID(id string) session.IDGenerator {
	return func() (string, error) { return id, nil }
}

func newCreator(t *testing.T, root string, b session.Backend, gen session.IDGenerator) (*session.SessionCreator, *mockProjectStore) {
	t.Helper()
	t.Setenv("SHELL", "/bin/zsh")
	store := &mockProjectStore{}
	return session.NewSessionCreator(&mockGitResolver{resolvedDir: root}, store, b, gen), store
}

func TestSessionCreator_Create(t *testing.T) {
	ctx := context.Background()

	t.Run("it creates the session at the git root, named for the project", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "portal")
		fake := backendtest.New()
		creator, _ := newCreator(t, root, fake, fixedID("abc123"))

		got, err := creator.Create(ctx, filepath.Join(root, "sub"), nil)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if got.Name != "portal-abc123" || got.Dir != root {
			t.Errorf("Create = %+v, want portal-abc123 at %s", got, root)
		}
		calls := fake.CallsTo(backendtest.OpCreate)
		if len(calls) != 1 || calls[0].Name != "portal-abc123" || calls[0].Dir != root || calls[0].Command != nil {
			t.Errorf("Create calls = %+v, want one for portal-abc123 at %s with no command", calls, root)
		}
	})

	t.Run("it hands a command to the backend as the shell's argv", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "portal")
		fake := backendtest.New()
		creator, _ := newCreator(t, root, fake, fixedID("abc123"))

		if _, err := creator.Create(ctx, root, []string{"claude", "--resume"}); err != nil {
			t.Fatalf("Create: %v", err)
		}
		want := session.ShellCommand([]string{"claude", "--resume"}, "/bin/zsh")
		if calls := fake.CallsTo(backendtest.OpCreate); len(calls) != 1 || !slices.Equal(calls[0].Command, want) {
			t.Errorf("Create calls = %+v, want command %q", calls, want)
		}
	})

	t.Run("it registers the project at the git root", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "portal")
		creator, store := newCreator(t, root, backendtest.New(), fixedID("abc123"))

		if _, err := creator.Create(ctx, root, nil); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if store.upsertCount != 1 || store.upsertPath != root || store.upsertName != "portal" || store.upsertVia != "internal" {
			t.Errorf("upsert = %d× (%q, %q, %q), want once (%q, portal, internal)", store.upsertCount, store.upsertPath, store.upsertName, store.upsertVia, root)
		}
	})

	t.Run("it draws a fresh nanoid for a name the backend holds", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "portal")
		fake := backendtest.New(backend.Session{Name: "portal-taken1"})
		creator, _ := newCreator(t, root, fake, ids("taken1", "fresh1"))

		got, err := creator.Create(ctx, root, nil)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if got.Name != "portal-fresh1" {
			t.Errorf("name = %q, want portal-fresh1", got.Name)
		}
	})

	t.Run("it draws a fresh nanoid when Create refuses a name held under another case", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "portal")
		fake := backendtest.New(backend.Session{Name: "PORTAL-TAKEN1"})
		creator, _ := newCreator(t, root, fake, ids("taken1", "fresh1"))

		got, err := creator.Create(ctx, root, nil)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if got.Name != "portal-fresh1" {
			t.Errorf("name = %q, want portal-fresh1", got.Name)
		}
		var refused []string
		for _, c := range fake.CallsTo(backendtest.OpCreate) {
			refused = append(refused, c.Name)
		}
		if !slices.Equal(refused, []string{"portal-taken1", "portal-fresh1"}) {
			t.Errorf("Create was asked for %q, want the refused name then the fresh one", refused)
		}
	})

	t.Run("it draws a fresh nanoid for a name held by a session not answering", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "portal")
		fake := backendtest.New(backend.Session{Name: "portal-taken1"})
		fake.StopAnswering("portal-taken1")
		creator, _ := newCreator(t, root, fake, ids("taken1", "fresh1"))

		got, err := creator.Create(ctx, root, nil)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if got.Name != "portal-fresh1" {
			t.Errorf("name = %q, want portal-fresh1", got.Name)
		}
	})

	t.Run("it gives up after a bounded number of held names", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "portal")
		fake := backendtest.New(backend.Session{Name: "portal-taken1"})
		creator, _ := newCreator(t, root, fake, fixedID("taken1"))

		if _, err := creator.Create(ctx, root, nil); err == nil || !strings.Contains(err.Error(), "unique session name") {
			t.Errorf("Create = %v, want the bounded-retry refusal", err)
		}
		if n := len(fake.CallsTo(backendtest.OpExists)); n != 10 {
			t.Errorf("Exists was asked %d times, want 10", n)
		}
	})

	t.Run("it shortens a long project name to the backend's limit", func(t *testing.T) {
		long := strings.Repeat("p", 80)
		root := filepath.Join(t.TempDir(), long)
		fake := backendtest.New()
		fake.Validate = maxLen(20)
		creator, _ := newCreator(t, root, fake, fixedID("abc123"))

		got, err := creator.Create(ctx, root, nil)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if want := strings.Repeat("p", 13) + "-abc123"; got.Name != want {
			t.Errorf("name = %q, want %q", got.Name, want)
		}
	})

	t.Run("a project that sanitises to nothing takes the fallback word", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "--")
		creator, _ := newCreator(t, root, backendtest.New(), fixedID("abc123"))

		got, err := creator.Create(ctx, root, nil)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if want := session.FallbackProject + "-abc123"; got.Name != want {
			t.Errorf("name = %q, want %q", got.Name, want)
		}
	})

	t.Run("it returns the created time the backend reports for the session", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "portal")
		fake := backendtest.New()
		at := time.Date(2026, 11, 3, 9, 30, 0, 0, time.UTC)
		fake.Now = func() time.Time { return at }
		creator, _ := newCreator(t, root, fake, fixedID("abc123"))

		got, err := creator.Create(ctx, root, nil)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if !got.At.Equal(at) {
			t.Errorf("At = %v, want %v", got.At, at)
		}
		if n := len(fake.CallsTo(backendtest.OpList)); n != 0 {
			t.Errorf("List was called %d times; the created time comes from Create", n)
		}
	})

	t.Run("it runs commands in the shell $SHELL named at construction", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "portal")
		fake := backendtest.New()
		creator, _ := newCreator(t, root, fake, fixedID("abc123"))
		t.Setenv("SHELL", "/bin/bash")

		if _, err := creator.Create(ctx, root, []string{"claude"}); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if calls := fake.CallsTo(backendtest.OpCreate); len(calls) != 1 || calls[0].Command[0] != "/bin/zsh" {
			t.Errorf("Create calls = %+v, want the command under /bin/zsh, $SHELL when the creator was made", calls)
		}
	})

	t.Run("a failed create is returned", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "portal")
		fake := backendtest.New()
		boom := errors.New("zmx attach: boom")
		fake.Fail(backendtest.OpCreate, boom)
		creator, _ := newCreator(t, root, fake, fixedID("abc123"))

		if _, err := creator.Create(ctx, root, nil); !errors.Is(err, boom) {
			t.Errorf("Create = %v, want %v", err, boom)
		}
	})

	t.Run("a failed existence check is returned", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "portal")
		fake := backendtest.New()
		boom := errors.New("unreadable zmx dir")
		fake.Fail(backendtest.OpExists, boom)
		creator, _ := newCreator(t, root, fake, fixedID("abc123"))

		if _, err := creator.Create(ctx, root, nil); !errors.Is(err, boom) {
			t.Errorf("Create = %v, want %v", err, boom)
		}
		if n := len(fake.CallsTo(backendtest.OpCreate)); n != 0 {
			t.Errorf("Create was called %d times, want none", n)
		}
	})

	t.Run("a failed id draw is returned", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "portal")
		creator, _ := newCreator(t, root, backendtest.New(), ids())

		if _, err := creator.Create(ctx, root, nil); err == nil || !strings.Contains(err.Error(), "ids exhausted") {
			t.Errorf("Create = %v, want the generator's error", err)
		}
	})

	t.Run("a directory that does not resolve creates nothing", func(t *testing.T) {
		fake := backendtest.New()
		t.Setenv("SHELL", "/bin/zsh")
		creator := session.NewSessionCreator(&mockGitResolver{err: errors.New("no such dir")}, &mockProjectStore{}, fake, fixedID("abc123"))

		if _, err := creator.Create(ctx, "/gone", nil); err == nil {
			t.Fatal("Create = nil, want the resolve error")
		}
		if n := len(fake.Calls()); n != 0 {
			t.Errorf("backend calls = %d, want none", n)
		}
	})

	t.Run("a failed project upsert creates nothing", func(t *testing.T) {
		fake := backendtest.New()
		store := &mockProjectStore{upsertErr: errors.New("disk full")}
		creator := session.NewSessionCreator(&mockGitResolver{}, store, fake, fixedID("abc123"))

		if _, err := creator.Create(ctx, t.TempDir(), nil); err == nil {
			t.Fatal("Create = nil, want the upsert error")
		}
		if n := len(fake.Calls()); n != 0 {
			t.Errorf("backend calls = %d, want none", n)
		}
	})
}

// maxLen is a name rule refusing names longer than n bytes, as zmx refuses
// one whose socket path would not fit.
func maxLen(n int) func(string) error {
	return func(name string) error {
		if len(name) > n {
			return fmt.Errorf("%w: %q is longer than %d", backend.ErrInvalidName, name, n)
		}
		return nil
	}
}
