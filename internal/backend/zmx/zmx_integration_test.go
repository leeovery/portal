//go:build integration

package zmx_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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
		_, err := zmx.ExecRunner{}.Run(b.AttachCommand(tooLong))
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
}
