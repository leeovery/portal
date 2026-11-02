//go:build integration

package zmx_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/zmxtest"
)

const settle = 5 * time.Second

func mustCreate(t *testing.T, b *zmx.Backend, name, dir string, command ...string) {
	t.Helper()
	if err := b.Create(name, dir, command); err != nil {
		t.Fatalf("Create(%q): %v", name, err)
	}
}

func session(t *testing.T, b *zmx.Backend, name string) backend.Session {
	t.Helper()
	sessions, err := b.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, s := range sessions {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("session %q not in %+v", name, sessions)
	return backend.Session{}
}

// awaitHistory polls until the session's history contains want.
func awaitHistory(t *testing.T, b *zmx.Backend, name, want string) []byte {
	t.Helper()
	var got []byte
	if !harnesstest.PollUntil(t, settle, 20*time.Millisecond, func() bool {
		var err error
		got, err = b.History(name)
		return err == nil && strings.Contains(string(got), want)
	}) {
		t.Fatalf("history of %q never contained %q; last %q", name, want, got)
	}
	return got
}

func realDir(t *testing.T, dir string) string {
	t.Helper()
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return real
}

func TestBackendAgainstZmx(t *testing.T) {
	t.Run("it creates a detached session that lists with no client", func(t *testing.T) {
		b := zmxtest.New(t)
		dir := t.TempDir()
		before := time.Now().Add(-time.Second)

		mustCreate(t, b, "work", dir, "sleep", "1000")

		s := session(t, b, "work")
		if s.Clients != 0 || s.Attached() {
			t.Errorf("Clients = %d right after Create, want 0", s.Clients)
		}
		if s.PID <= 0 {
			t.Errorf("PID = %d, want the session's process", s.PID)
		}
		if s.Created.Before(before) || s.Created.After(time.Now()) {
			t.Errorf("Created = %v, want about now", s.Created)
		}
		if s.Command != "sleep 1000" {
			t.Errorf("Command = %q, want %q", s.Command, "sleep 1000")
		}
		if want := realDir(t, dir); s.Dir != want {
			t.Errorf("Dir = %q, want %q", s.Dir, want)
		}
		if exists, err := b.Exists("work"); err != nil || !exists {
			t.Errorf("Exists(work) = %v, %v; want true", exists, err)
		}
		if exists, err := b.Exists("wor"); err != nil || exists {
			t.Errorf("Exists(wor) = %v, %v; want false", exists, err)
		}
	})

	t.Run("it runs the command in the directory, at 120x24", func(t *testing.T) {
		b := zmxtest.New(t)
		dir := filepath.Join(t.TempDir(), "my dir")
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}

		mustCreate(t, b, "where", dir, "sh", "-c", `echo "at=$(pwd -P) size=$(stty size)"; sleep 1000`)

		awaitHistory(t, b, "where", "at="+realDir(t, dir)+" size=24 120")
		if got := session(t, b, "where").Dir; got != realDir(t, dir) {
			t.Errorf("Dir = %q, want %q", got, realDir(t, dir))
		}
	})

	t.Run("it runs the user's shell when given no command", func(t *testing.T) {
		b := zmxtest.New(t)

		mustCreate(t, b, "shell", t.TempDir())

		s := session(t, b, "shell")
		if s.Command != "" || s.PID <= 0 {
			t.Errorf("session = %+v, want a running shell with no command", s)
		}
	})

	t.Run("it tells the session's processes their session name", func(t *testing.T) {
		b := zmxtest.New(t)

		mustCreate(t, b, "named", t.TempDir(), "sh", "-c", `echo "in=$ZMX_SESSION"; sleep 1000`)

		awaitHistory(t, b, "named", "in=named")
	})

	t.Run("it refuses to create over a live session", func(t *testing.T) {
		b := zmxtest.New(t)
		mustCreate(t, b, "taken", t.TempDir(), "sleep", "1000")

		if err := b.Create("taken", t.TempDir(), []string{"true"}); !errors.Is(err, backend.ErrSessionExists) {
			t.Errorf("Create over a live session = %v, want ErrSessionExists", err)
		}
		if got := session(t, b, "taken").Command; got != "sleep 1000" {
			t.Errorf("Command = %q, want the original sleep 1000", got)
		}
	})

	t.Run("it returns history with colour, verbatim", func(t *testing.T) {
		b := zmxtest.New(t)

		mustCreate(t, b, "colour", t.TempDir(), "sh", "-c", `printf '\033[31mred\033[0m plain\n'; sleep 1000`)

		got := awaitHistory(t, b, "colour", "plain")
		if !strings.Contains(string(got), "\x1b[") || !strings.Contains(string(got), "red") {
			t.Errorf("History = %q, want escape sequences around red", got)
		}
	})

	t.Run("it kills a session", func(t *testing.T) {
		b := zmxtest.New(t)
		mustCreate(t, b, "doomed", t.TempDir(), "sleep", "1000")
		mustCreate(t, b, "doomed-2", t.TempDir(), "sleep", "1000")

		if err := b.Kill("doomed"); err != nil {
			t.Fatalf("Kill: %v", err)
		}
		if exists, err := b.Exists("doomed"); err != nil || exists {
			t.Errorf("Exists(doomed) after Kill = %v, %v; want false", exists, err)
		}
		if exists, err := b.Exists("doomed-2"); err != nil || !exists {
			t.Errorf("Exists(doomed-2) after killing doomed = %v, %v; want true", exists, err)
		}
	})

	t.Run("it detaches every client of a session", func(t *testing.T) {
		b := zmxtest.New(t)
		mustCreate(t, b, "watched", t.TempDir(), "sleep", "1000")
		mustCreate(t, b, "other", t.TempDir(), "sleep", "1000")
		clients := []*zmxtest.Client{zmxtest.Attach(t, b, "watched"), zmxtest.Attach(t, b, "watched")}
		bystander := zmxtest.Attach(t, b, "other")
		if !harnesstest.PollUntil(t, settle, 20*time.Millisecond, func() bool { return session(t, b, "watched").Clients == 2 }) {
			t.Fatalf("clients never attached: %+v", session(t, b, "watched"))
		}

		if err := b.Detach("watched"); err != nil {
			t.Fatalf("Detach: %v", err)
		}

		for i, c := range clients {
			if !c.Exited(settle) {
				t.Errorf("client %d still attached after Detach", i)
			}
		}
		if bystander.Exited(100 * time.Millisecond) {
			t.Error("Detach detached another session's client")
		}
		if got := session(t, b, "watched").Clients; got != 0 {
			t.Errorf("Clients after Detach = %d, want 0", got)
		}
	})

	t.Run("it reports a missing session as ErrNoSuchSession", func(t *testing.T) {
		b := zmxtest.New(t)
		mustCreate(t, b, "here", t.TempDir(), "sleep", "1000")

		if err := b.Kill("gone"); !errors.Is(err, backend.ErrNoSuchSession) {
			t.Errorf("Kill = %v, want ErrNoSuchSession", err)
		}
		if err := b.Detach("gone"); !errors.Is(err, backend.ErrNoSuchSession) {
			t.Errorf("Detach = %v, want ErrNoSuchSession", err)
		}
		if _, err := b.History("gone"); !errors.Is(err, backend.ErrNoSuchSession) {
			t.Errorf("History = %v, want ErrNoSuchSession", err)
		}
	})

	t.Run("it lists nothing, without error, in an empty directory", func(t *testing.T) {
		got, err := zmxtest.New(t).List()
		if err != nil || got == nil || len(got) != 0 {
			t.Errorf("List = %#v, %v; want an empty list", got, err)
		}
	})

	t.Run("it holds the longest name ValidateName allows, where zmx refuses one byte more", func(t *testing.T) {
		b := zmxtest.New(t)
		longest := strings.Repeat("n", b.MaxNameLen())

		mustCreate(t, b, longest, t.TempDir(), "sleep", "1000")
		if got := session(t, b, longest).Name; got != longest {
			t.Errorf("listed name = %q, want %q", got, longest)
		}

		tooLong := longest + "n"
		if err := b.ValidateName(tooLong); !errors.Is(err, backend.ErrInvalidName) {
			t.Errorf("ValidateName(%d bytes) = %v, want ErrInvalidName", len(tooLong), err)
		}
		cmd, err := b.AttachCommand(longest)
		if err != nil {
			t.Fatalf("AttachCommand: %v", err)
		}
		cmd.Argv = []string{"zmx", "attach", tooLong}
		_, err = zmx.ExecRunner{}.Run(context.Background(), cmd)
		if ce, ok := errors.AsType[*zmx.CommandError](err); !ok || !strings.Contains(ce.Stderr, "too long") {
			t.Errorf("zmx attach of %d bytes = %v, want zmx's own too-long refusal", len(tooLong), err)
		}
	})

	t.Run("it lists names zmx holds that a line-oriented reader could split", func(t *testing.T) {
		b := zmxtest.New(t)
		names := []string{"a=b", "has space", "a.b:c", "_portal-saver", " lead", "ünï"}
		for _, name := range names {
			mustCreate(t, b, name, t.TempDir(), "sleep", "1000")
		}

		for _, name := range names {
			if exists, err := b.Exists(name); err != nil || !exists {
				t.Errorf("Exists(%q) = %v, %v; want true", name, exists, err)
			}
		}
		if got := session(t, b, "_portal-saver"); !got.PortalOwned() {
			t.Errorf("_portal-saver not Portal-owned: %+v", got)
		}
	})

	t.Run("it lists a session whose command holds a newline, and the sessions after it", func(t *testing.T) {
		b := zmxtest.New(t)
		mustCreate(t, b, "a-multi", t.TempDir(), "sh", "-c", "echo a\nsleep 1000")
		mustCreate(t, b, "b-tab", t.TempDir(), "sh", "-c", "echo a\tb; sleep 1000")
		mustCreate(t, b, "c-after", t.TempDir(), "sleep", "1000")

		sessions, err := b.List()
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		var got []string
		for _, s := range sessions {
			got = append(got, s.Name)
		}
		if strings.Join(got, ",") != "a-multi,b-tab,c-after" {
			t.Fatalf("List names = %v", got)
		}
		if want := "sh -c 'echo a\nsleep 1000'"; sessions[0].Command != want {
			t.Errorf("Command = %q, want %q", sessions[0].Command, want)
		}
		if want := "sh -c 'echo a"; sessions[1].Command != want {
			t.Errorf("Command = %q, want it cut at the tab: %q", sessions[1].Command, want)
		}
	})

	t.Run("it reports commands as DisplayCommand renders them", func(t *testing.T) {
		b := zmxtest.New(t)
		commands := map[string][]string{
			"quoted":    {"sh", "-c", `sleep 1000 # it's "$HOME" (x) [y] {z} | & ; < > ? * ~ ! \ ` + "`"},
			"empty-arg": {"sh", "-c", "sleep 1000", ""},
			"long":      {"sh", "-c", "sleep 1000", strings.Repeat("a", 200), strings.Repeat("b", 60), "c"},
		}
		for name, command := range commands {
			mustCreate(t, b, name, t.TempDir(), command...)
		}

		for name, command := range commands {
			if got, want := session(t, b, name).Command, zmx.DisplayCommand(command); got != want {
				t.Errorf("%s: zmx reports %q, DisplayCommand renders %q", name, got, want)
			}
		}
	})

	t.Run("it reports a session made even when its command exits at once", func(t *testing.T) {
		b := zmxtest.New(t)

		if err := b.Create("brief", t.TempDir(), []string{"true"}); err != nil {
			t.Errorf("Create of a command that exits at once = %v, want nil", err)
		}
	})

	t.Run("it returns a blank screen's history as something, never empty", func(t *testing.T) {
		b := zmxtest.New(t)
		mustCreate(t, b, "blank", t.TempDir(), "sleep", "1000")

		got, err := b.History("blank")
		if err != nil || len(got) == 0 {
			t.Errorf("History of a blank screen = %q, %v; want its VT state", got, err)
		}
	})

	t.Run("it never acts on a session held under the name in another case", func(t *testing.T) {
		b := zmxtest.New(t)
		mustCreate(t, b, "foo", t.TempDir(), "sleep", "1000")

		if err := b.Kill("FOO"); !errors.Is(err, backend.ErrNoSuchSession) {
			t.Errorf("Kill(FOO) = %v, want ErrNoSuchSession", err)
		}
		if err := b.Create("FOO", t.TempDir(), []string{"sleep", "1000"}); !errors.Is(err, backend.ErrSessionExists) {
			t.Errorf("Create(FOO) = %v, want ErrSessionExists", err)
		}
		if exists, err := b.Exists("foo"); err != nil || !exists {
			t.Errorf("Exists(foo) = %v, %v; want foo untouched", exists, err)
		}
	})

	t.Run("it gives up on a wedged session within its bounds", func(t *testing.T) {
		b := zmxtest.New(t)
		mustCreate(t, b, "wedged", t.TempDir(), "sleep", "1000")
		daemon := zmxtest.Daemon(t, b, "wedged")
		if err := syscall.Kill(daemon, syscall.SIGSTOP); err != nil {
			t.Fatalf("stop daemon %d: %v", daemon, err)
		}
		t.Cleanup(func() { _ = syscall.Kill(daemon, syscall.SIGCONT) })

		start := time.Now()
		err := b.Kill("wedged")
		if !errors.Is(err, backend.ErrTimeout) {
			t.Errorf("Kill of a wedged session = %v, want ErrTimeout", err)
		}
		if took := time.Since(start); took > 8*time.Second {
			t.Errorf("Kill of a wedged session took %s", took)
		}

		if got := session(t, b, "wedged"); !got.Unreachable {
			t.Errorf("wedged session listed as %+v, want unreachable", got)
		}
		if _, err := b.History("wedged"); !errors.Is(err, backend.ErrHistoryUnavailable) {
			t.Errorf("History of a wedged session = %v, want ErrHistoryUnavailable", err)
		}
	})

	t.Run("it refuses zmx's log directory as a name", func(t *testing.T) {
		b := zmxtest.New(t)

		if err := b.Create("logs", t.TempDir(), nil); !errors.Is(err, backend.ErrInvalidName) {
			t.Errorf("Create(logs) = %v, want ErrInvalidName", err)
		}
	})
}
