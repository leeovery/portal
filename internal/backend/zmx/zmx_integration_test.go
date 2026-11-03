//go:build integration

package zmx_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"flag"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/zmxtest"
)

var update = flag.Bool("update", false, "rewrite testdata from the pinned zmx build")

const settle = 5 * time.Second

func mustCreate(t *testing.T, b *zmx.Backend, name, dir string, command ...string) {
	t.Helper()
	if _, err := b.Create(t.Context(), name, dir, command); err != nil {
		t.Fatalf("Create(%q): %v", name, err)
	}
}

func session(t *testing.T, b *zmx.Backend, name string) backend.Session {
	t.Helper()
	sessions, err := b.List(t.Context())
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

func listed(t *testing.T, b *zmx.Backend) []string {
	t.Helper()
	sessions, err := b.List(t.Context())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	names := []string{}
	for _, s := range sessions {
		names = append(names, s.Name)
	}
	return names
}

// awaitHistory polls until the session's whole history contains want.
func awaitHistory(t *testing.T, b *zmx.Backend, name, want string) []byte {
	t.Helper()
	var got []byte
	if !harnesstest.PollUntil(t, settle, 20*time.Millisecond, func() bool {
		var err error
		got, err = b.History(t.Context(), name, 0)
		return err == nil && bytes.Contains(got, []byte(want))
	}) {
		t.Fatalf("history of %q never contained %q; last %q", name, want, got)
	}
	return got
}

// rawInfo asks a session's daemon for Info without the driver, and returns
// the whole answering frame.
func rawInfo(t *testing.T, b *zmx.Backend, name string) []byte {
	t.Helper()
	c, err := net.Dial("unix", filepath.Join(b.Dir(), name))
	if err != nil {
		t.Fatalf("dial %s: %v", name, err)
	}
	defer func() { _ = c.Close() }()
	_ = c.SetDeadline(time.Now().Add(time.Second))
	if _, err := c.Write([]byte{6, 1, 0, 0, 0, 0, 0, 0}); err != nil {
		t.Fatalf("send Info to %s: %v", name, err)
	}
	for {
		h := make([]byte, 8)
		if _, err := io.ReadFull(c, h); err != nil {
			t.Fatalf("read from %s: %v", name, err)
		}
		p := make([]byte, binary.NativeEndian.Uint32(h[4:]))
		if _, err := io.ReadFull(c, p); err != nil {
			t.Fatalf("read from %s: %v", name, err)
		}
		if h[0] == 6 {
			return append(h, p...)
		}
	}
}

// crash SIGKILLs the daemon of a session, waits for its socket to refuse, and
// returns the socket's path.
func crash(t *testing.T, b *zmx.Backend, name string) string {
	t.Helper()
	if err := syscall.Kill(zmxtest.Daemon(t, b, name), syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(b.Dir(), name)
	if !harnesstest.PollUntil(t, settle, 20*time.Millisecond, func() bool {
		c, err := net.Dial("unix", socket)
		if err == nil {
			_ = c.Close()
		}
		return errors.Is(err, syscall.ECONNREFUSED)
	}) {
		t.Fatalf("the socket of %s's dead daemon never refused", name)
	}
	return socket
}

// fillBacklog connects to a stopped daemon's socket until it refuses, and
// returns what closes those connections.
func fillBacklog(t *testing.T, socket string) (drain func()) {
	t.Helper()
	var conns []net.Conn
	drain = func() {
		for _, c := range conns {
			_ = c.Close()
		}
		conns = nil
	}
	t.Cleanup(drain)
	for range 4096 {
		c, err := net.Dial("unix", socket)
		if errors.Is(err, syscall.ECONNREFUSED) {
			return drain
		}
		if err != nil {
			t.Fatalf("fill the backlog of %s: %v", socket, err)
		}
		conns = append(conns, c)
	}
	t.Fatalf("%s never refused a connection", socket)
	return drain
}

// assertNotAnswering asserts that a session whose socket refuses is held, and
// not answering: listed with only its name, every call on it ErrTimeout, and
// its name refused by Create.
func assertNotAnswering(t *testing.T, b *zmx.Backend, name string) {
	t.Helper()
	ctx := t.Context()
	if got := session(t, b, name); got != (backend.Session{Name: name}) {
		t.Errorf("%s listed as %+v, want only its name", name, got)
	}
	calls := map[string]error{
		"Kill":   b.Kill(ctx, name),
		"Detach": b.Detach(ctx, name),
	}
	_, calls["History"] = b.History(ctx, name, 0)
	_, calls["AttachCommand"] = b.AttachCommand(ctx, name)
	_, calls["Exists"] = b.Exists(ctx, name)
	for call, err := range calls {
		if !errors.Is(err, backend.ErrTimeout) || errors.Is(err, backend.ErrNoSuchSession) {
			t.Errorf("%s(%s) = %v, want ErrTimeout and not ErrNoSuchSession", call, name, err)
		}
	}
	_, err := b.Create(ctx, name, t.TempDir(), []string{"sleep", "1000"})
	if exists, ok := errors.AsType[*backend.SessionExistsError](err); !ok || exists.Held != name {
		t.Errorf("Create(%s) = %v, want a SessionExistsError held as %s", name, err, name)
	}
}

// winches counts the WINCH lines a session's trap has printed.
func winches(t *testing.T, b *zmx.Backend, name string) int {
	t.Helper()
	vt, err := b.History(t.Context(), name, 0)
	if err != nil {
		t.Fatalf("History(%s): %v", name, err)
	}
	return bytes.Count(vt, []byte("WINCH"))
}

// varying blanks the parts of an Info frame that differ run to run: pid and
// created_at.
func varying(frame []byte) []byte {
	f := bytes.Clone(frame)
	clear(f[8+8 : 8+12])
	clear(f[8+528 : 8+536])
	return f
}

func TestInfoFixture(t *testing.T) {
	t.Run("it is the pinned build's own answer to Info", func(t *testing.T) {
		b := zmxtest.New(t)
		mustCreate(t, b, "fixture", t.TempDir(), "sh", "-c", fixtureScript)
		var frame []byte
		if !harnesstest.PollUntil(t, settle, 20*time.Millisecond, func() bool {
			frame = rawInfo(t, b, "fixture")
			return bytes.HasPrefix(frame[8+272:], []byte("file://example-host/"))
		}) {
			t.Fatalf("the session's cwd never took its OSC 7: %q", frame)
		}
		path := filepath.Join("testdata", "info.frame")
		if *update {
			if err := os.WriteFile(path, frame, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(varying(frame), varying(want)) {
			t.Errorf("Info frame = %x, want %x, pid and created_at aside; run with -update if zmx's pin moved", frame, want)
		}
	})
}

// historyScript draws what the preview has to lay out: scrollback above the
// screen, colours, a tab and a background run to the edge, wide runes, a
// hyperlink, an erased line, modes and a cwd, and a last row filled to the
// right edge, which leaves the cursor's wrap pending.
const historyScript = `i=1; while [ $i -le 30 ]; do printf 'scrollback %d\n' $i; i=$((i+1)); done
printf '\033]7;file://example-host/srv/demo\033\\'
printf '\033[1;31mbold red\033[0m \033[38;2;10;200;30mtruecolour\033[0m\n'
printf 'gap\tafter-tab\n'
printf '\033[44mblue to the edge\033[K\033[0m\n'
printf 'wide 中文 end\n'
printf '\033]8;;https://example.com\033\\link\033]8;;\033\\ text\n'
printf 'erase-me\r\033[2Kerased\n'
printf '\033[?2004h\033[>4;2m'
printf 'last row %0111d' 0
sleep 1000`

func TestHistoryFixture(t *testing.T) {
	t.Run("it is the pinned build's own answer to History", func(t *testing.T) {
		b := zmxtest.New(t)
		mustCreate(t, b, "fixture", t.TempDir(), "sh", "-c", historyScript)
		vt := awaitHistory(t, b, "fixture", "last row "+strings.Repeat("0", 111))

		path := filepath.Join("testdata", "history.vt")
		if *update {
			if err := os.WriteFile(path, vt, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(vt, want) {
			t.Errorf("History = %q, want %q; run with -update if zmx's pin moved", vt, want)
		}
	})
}

func TestBackendAgainstZmx(t *testing.T) {
	t.Run("it creates a detached session that lists with its details", func(t *testing.T) {
		b := zmxtest.New(t)
		dir := t.TempDir()
		before := time.Now().Add(-time.Second)

		mustCreate(t, b, "work", dir, "sleep", "1000")

		s := session(t, b, "work")
		if !s.Answered || s.Clients != 0 || s.Attached() {
			t.Errorf("session = %+v, want answering with no client", s)
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
		if exists, err := b.Exists(t.Context(), "work"); err != nil || !exists {
			t.Errorf("Exists(work) = %v, %v; want true", exists, err)
		}
		if exists, err := b.Exists(t.Context(), "wor"); err != nil || exists {
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

		if s := session(t, b, "shell"); s.Command != "" || s.PID <= 0 {
			t.Errorf("session = %+v, want a running shell with no command", s)
		}
	})

	t.Run("it tells the session's processes their session name, and nothing of the caller's tmux", func(t *testing.T) {
		b := zmxtest.New(t)

		mustCreate(t, b, "named", t.TempDir(), "sh", "-c", `echo "in=$ZMX_SESSION tmux=${TMUX-unset}"; sleep 1000`)

		awaitHistory(t, b, "named", "in=named tmux=unset")
	})

	t.Run("it refuses to create over a live session, naming it, and leaves it untouched", func(t *testing.T) {
		b := zmxtest.New(t)
		mustCreate(t, b, "taken", t.TempDir(), "sleep", "1000")

		_, err := b.Create(t.Context(), "taken", t.TempDir(), []string{"true"})

		if exists, ok := errors.AsType[*backend.SessionExistsError](err); !ok || exists.Held != "taken" {
			t.Errorf("Create over a live session = %v, want a SessionExistsError held as taken", err)
		}
		if got := session(t, b, "taken"); got.Command != "sleep 1000" || got.Clients != 0 {
			t.Errorf("session = %+v, want the original sleep 1000 with no client joined", got)
		}
	})

	t.Run("it reports a session made even when its command exits at once", func(t *testing.T) {
		b := zmxtest.New(t)

		if _, err := b.Create(t.Context(), "brief", t.TempDir(), []string{"true"}); err != nil {
			t.Errorf("Create of a command that exits at once = %v, want nil", err)
		}
	})

	t.Run("it returns the whole history with colour, verbatim", func(t *testing.T) {
		b := zmxtest.New(t)

		mustCreate(t, b, "colour", t.TempDir(), "sh", "-c", `printf '\033[31mred\033[0m plain\n'; sleep 1000`)

		got := awaitHistory(t, b, "colour", "plain")
		if !bytes.Contains(got, []byte("\x1b[")) || !bytes.Contains(got, []byte("red")) {
			t.Errorf("History = %q, want escape sequences around red", got)
		}
	})

	t.Run("it bounds history to the screen and the rows above it", func(t *testing.T) {
		b := zmxtest.New(t)
		mustCreate(t, b, "long", t.TempDir(), "sh", "-c", `i=0; while [ $i -lt 100 ]; do echo row$i; i=$((i+1)); done; sleep 1000`)
		awaitHistory(t, b, "long", "row99")

		full, err := b.History(t.Context(), "long", 0)
		if err != nil || !bytes.Contains(full, []byte("row0")) {
			t.Errorf("History(0) = %q, %v; want every row from row0", full, err)
		}
		bounded, err := b.History(t.Context(), "long", 5)
		if err != nil {
			t.Fatalf("History(5): %v", err)
		}
		// The 24-row screen holds row77 to row99 above a blank line, so 5 rows
		// of scrollback reach back to row72.
		if !bytes.Contains(bounded, []byte("row72")) || bytes.Contains(bounded, []byte("row71")) {
			t.Errorf("History(5) = %q, want rows from row72 and none before", bounded)
		}
	})

	t.Run("it returns a blank screen's history as something, never empty", func(t *testing.T) {
		b := zmxtest.New(t)
		mustCreate(t, b, "blank", t.TempDir(), "sleep", "1000")

		for _, rows := range []int{0, 10} {
			if got, err := b.History(t.Context(), "blank", rows); err != nil || len(got) == 0 {
				t.Errorf("History(%d) of a blank screen = %q, %v; want its VT state", rows, got, err)
			}
		}
	})

	t.Run("it kills a session and frees its name", func(t *testing.T) {
		b := zmxtest.New(t)
		mustCreate(t, b, "doomed", t.TempDir(), "sleep", "1000")
		mustCreate(t, b, "doomed-2", t.TempDir(), "sleep", "1000")
		daemon := zmxtest.Daemon(t, b, "doomed")

		if err := b.Kill(t.Context(), "doomed"); err != nil {
			t.Fatalf("Kill: %v", err)
		}
		if _, err := os.Lstat(filepath.Join(b.Dir(), "doomed")); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("doomed's socket after Kill: %v, want it gone", err)
		}
		if got := listed(t, b); strings.Join(got, ",") != "doomed-2" {
			t.Errorf("List after killing doomed = %v, want [doomed-2]", got)
		}
		if !harnesstest.PollUntil(t, settle, 20*time.Millisecond, func() bool { return syscall.Kill(daemon, 0) != nil }) {
			t.Errorf("daemon %d outlived its kill", daemon)
		}
	})

	t.Run("it gives up on a stopped daemon's kill within its bounds, and the kill lands once it runs", func(t *testing.T) {
		b := zmxtest.New(t)
		zmx.ShortenCeilings(b, time.Second)
		mustCreate(t, b, "wedged", t.TempDir(), "sleep", "1000")
		daemon := zmxtest.StopDaemon(t, b, "wedged")

		start := time.Now()
		err := b.Kill(t.Context(), "wedged")
		if !errors.Is(err, backend.ErrTimeout) || !strings.Contains(err.Error(), "may still land") {
			t.Errorf("Kill of a stopped daemon = %v, want ErrTimeout saying the kill may still land", err)
		}
		if took := time.Since(start); took > 3*time.Second {
			t.Errorf("Kill of a stopped daemon took %s past its 1s ceiling", took)
		}
		if got := session(t, b, "wedged"); got.Answered {
			t.Errorf("stopped session listed as %+v, want it unanswered", got)
		}
		if _, err := b.History(t.Context(), "wedged", 0); !errors.Is(err, backend.ErrTimeout) {
			t.Errorf("History of a stopped daemon = %v, want ErrTimeout", err)
		}

		if err := syscall.Kill(daemon, syscall.SIGCONT); err != nil {
			t.Fatal(err)
		}
		if !harnesstest.PollUntil(t, settle, 20*time.Millisecond, func() bool { return syscall.Kill(daemon, 0) != nil }) {
			t.Errorf("daemon %d survived the kill it was sent while stopped", daemon)
		}
	})

	t.Run("it detaches every client of a session, and only that session's", func(t *testing.T) {
		b := zmxtest.New(t)
		mustCreate(t, b, "watched", t.TempDir(), "sleep", "1000")
		mustCreate(t, b, "other", t.TempDir(), "sleep", "1000")
		clients := []*zmxtest.Client{zmxtest.Attach(t, b, "watched"), zmxtest.Attach(t, b, "watched")}
		bystander := zmxtest.Attach(t, b, "other")
		if !harnesstest.PollUntil(t, settle, 20*time.Millisecond, func() bool { return session(t, b, "watched").Clients == 2 }) {
			t.Fatalf("clients never attached: %+v", session(t, b, "watched"))
		}

		if err := b.Detach(t.Context(), "watched"); err != nil {
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
		ctx := t.Context()

		if err := b.Kill(ctx, "gone"); !errors.Is(err, backend.ErrNoSuchSession) {
			t.Errorf("Kill = %v, want ErrNoSuchSession", err)
		}
		if err := b.Detach(ctx, "gone"); !errors.Is(err, backend.ErrNoSuchSession) {
			t.Errorf("Detach = %v, want ErrNoSuchSession", err)
		}
		if _, err := b.History(ctx, "gone", 0); !errors.Is(err, backend.ErrNoSuchSession) {
			t.Errorf("History = %v, want ErrNoSuchSession", err)
		}
		if _, err := b.AttachCommand(ctx, "gone"); !errors.Is(err, backend.ErrNoSuchSession) {
			t.Errorf("AttachCommand = %v, want ErrNoSuchSession", err)
		}
	})

	t.Run("it lists nothing, without error, in an empty directory", func(t *testing.T) {
		got, err := zmxtest.New(t).List(t.Context())
		if err != nil || got == nil || len(got) != 0 {
			t.Errorf("List = %#v, %v; want an empty list", got, err)
		}
	})

	t.Run("it holds the longest name ValidateName allows, where zmx refuses one byte more", func(t *testing.T) {
		b := zmxtest.New(t)
		longest := strings.Repeat("n", b.MaxNameLen())

		mustCreate(t, b, longest, t.TempDir(), "sleep", "1000")
		if got := session(t, b, longest); got.Name != longest || !got.Answered {
			t.Errorf("listed %+v, want %q answering", got, longest)
		}

		tooLong := longest + "n"
		if err := b.ValidateName(tooLong); !errors.Is(err, backend.ErrInvalidName) {
			t.Errorf("ValidateName(%d bytes) = %v, want ErrInvalidName", len(tooLong), err)
		}
		cmd := exec.Command(zmxtest.Binary(t), "attach", tooLong)
		cmd.Env = []string{"ZMX_DIR=" + b.Dir(), "HOME=" + t.TempDir()}
		if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "NameTooLong") {
			t.Errorf("zmx attach of %d bytes = %q, %v; want zmx's own too-long refusal", len(tooLong), out, err)
		}
	})

	t.Run("it lists names zmx holds that a line-oriented reader could split", func(t *testing.T) {
		b := zmxtest.New(t)
		names := []string{" lead", "_portal-saver", "a.b:c", "a=b", "has space", "ünï"}
		for _, name := range names {
			mustCreate(t, b, name, t.TempDir(), "sleep", "1000")
		}

		if got := listed(t, b); strings.Join(got, "|") != strings.Join(names, "|") {
			t.Errorf("List names = %q, want %q", got, names)
		}
		if got := session(t, b, "_portal-saver"); !got.PortalOwned() {
			t.Errorf("_portal-saver not Portal-owned: %+v", got)
		}
	})

	t.Run("it lists commands holding a newline, a tab or an indented name= whole, and no phantom session", func(t *testing.T) {
		b := zmxtest.New(t)
		commands := map[string][]string{
			"a-multi": {"sh", "-c", "echo a\nsleep 1000"},
			"b-tab":   {"sh", "-c", "echo a\tb; sleep 1000"},
			"c-forge": {"sh", "-c", "sleep 1000 #\n  name=forged\tpid=1\tclients=0\tcreated=1"},
		}
		for name, command := range commands {
			mustCreate(t, b, name, t.TempDir(), command...)
		}

		sessions, err := b.List(t.Context())
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(sessions) != len(commands) {
			t.Fatalf("List = %+v, want only %d sessions", sessions, len(commands))
		}
		for _, s := range sessions {
			if want := zmx.DisplayCommand(commands[s.Name]); s.Command != want {
				t.Errorf("%s: Command = %q, want %q", s.Name, s.Command, want)
			}
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

	t.Run("it never acts on a session held under the name in another case", func(t *testing.T) {
		b := zmxtest.New(t)
		mustCreate(t, b, "foo", t.TempDir(), "sleep", "1000")
		if _, err := os.Stat(filepath.Join(b.Dir(), "FOO")); err != nil {
			t.Skip("the filesystem does not fold case")
		}
		ctx := t.Context()

		if err := b.Kill(ctx, "FOO"); !errors.Is(err, backend.ErrNoSuchSession) {
			t.Errorf("Kill(FOO) = %v, want ErrNoSuchSession", err)
		}
		if err := b.Detach(ctx, "FOO"); !errors.Is(err, backend.ErrNoSuchSession) {
			t.Errorf("Detach(FOO) = %v, want ErrNoSuchSession", err)
		}
		if _, err := b.History(ctx, "FOO", 0); !errors.Is(err, backend.ErrNoSuchSession) {
			t.Errorf("History(FOO) = %v, want ErrNoSuchSession", err)
		}
		if exists, err := b.Exists(ctx, "FOO"); err != nil || exists {
			t.Errorf("Exists(FOO) = %v, %v; want false", exists, err)
		}
		_, err := b.Create(ctx, "FOO", t.TempDir(), []string{"sleep", "1000"})
		if exists, ok := errors.AsType[*backend.SessionExistsError](err); !ok || exists.Held != "foo" {
			t.Errorf("Create(FOO) = %v, want a SessionExistsError held as foo", err)
		}
		if got := session(t, b, "foo"); !got.Answered || got.Clients != 0 || got.Command != "sleep 1000" {
			t.Errorf("foo = %+v, want it untouched", got)
		}
	})

	t.Run("it holds an NFD name as given, and never acts on it under its NFC twin", func(t *testing.T) {
		b := zmxtest.New(t)
		nfd, nfc := "café", "café"
		mustCreate(t, b, nfd, t.TempDir(), "sleep", "1000")
		if _, err := os.Stat(filepath.Join(b.Dir(), nfc)); err != nil {
			t.Skip("the filesystem does not fold Unicode normalisation")
		}
		ctx := t.Context()

		if got := listed(t, b); len(got) != 1 || got[0] != nfd {
			t.Errorf("List names = %q, want only the NFD %q", got, nfd)
		}
		if exists, err := b.Exists(ctx, nfd); err != nil || !exists {
			t.Errorf("Exists(NFD) = %v, %v; want true", exists, err)
		}
		if err := b.Kill(ctx, nfc); !errors.Is(err, backend.ErrNoSuchSession) {
			t.Errorf("Kill(NFC) = %v, want ErrNoSuchSession", err)
		}
		_, err := b.Create(ctx, nfc, t.TempDir(), []string{"sleep", "1000"})
		if exists, ok := errors.AsType[*backend.SessionExistsError](err); !ok || exists.Held != nfd {
			t.Errorf("Create(NFC) = %v, want a SessionExistsError held as the NFD name", err)
		}
		if err := b.Kill(ctx, nfd); err != nil {
			t.Errorf("Kill(NFD) = %v, want nil", err)
		}
	})

	t.Run("it holds a SIGKILLed daemon's socket from this boot as not answering, and leaves it alone", func(t *testing.T) {
		b := zmxtest.New(t)
		mustCreate(t, b, "crashed", t.TempDir(), "sleep", "1000")
		mustCreate(t, b, "alive", t.TempDir(), "sleep", "1000")
		socket := crash(t, b, "crashed")
		before, err := os.Stat(socket)
		if err != nil {
			t.Fatal(err)
		}

		assertNotAnswering(t, b, "crashed")
		if got := session(t, b, "alive"); !got.Answered {
			t.Errorf("alive listed as %+v, want it answering", got)
		}
		after, err := os.Stat(socket)
		if err != nil || !os.SameFile(before, after) {
			t.Errorf("the refusing socket after Portal's calls: %v; want it left as it was", err)
		}
	})

	t.Run("it leaves out a SIGKILLed daemon's socket from before this boot, and zmx replaces it on Create", func(t *testing.T) {
		b := zmxtest.New(t)
		mustCreate(t, b, "crashed", t.TempDir(), "sleep", "1000")
		mustCreate(t, b, "alive", t.TempDir(), "sleep", "1000")
		socket := crash(t, b, "crashed")
		backdate(t, socket)
		ctx := t.Context()

		if got := listed(t, b); strings.Join(got, ",") != "alive" {
			t.Errorf("List = %v, want only alive", got)
		}
		if err := b.Kill(ctx, "crashed"); !errors.Is(err, backend.ErrNoSuchSession) {
			t.Errorf("Kill of a stale socket = %v, want ErrNoSuchSession", err)
		}
		if _, err := b.AttachCommand(ctx, "crashed"); !errors.Is(err, backend.ErrNoSuchSession) {
			t.Errorf("AttachCommand of a stale socket = %v, want ErrNoSuchSession", err)
		}
		if _, err := os.Lstat(socket); err != nil {
			t.Errorf("the stale socket is gone after Portal's calls: %v; only zmx removes it", err)
		}
		if _, err := b.Create(ctx, "crashed", t.TempDir(), []string{"sleep", "1000"}); err != nil {
			t.Errorf("Create over a stale socket = %v, want zmx to replace it", err)
		}
		if got := session(t, b, "crashed"); !got.Answered {
			t.Errorf("recreated session = %+v, want it answering", got)
		}
	})

	t.Run("it holds a live session refusing on a full backlog as not answering, and it answers once it drains", func(t *testing.T) {
		b := zmxtest.New(t)
		mustCreate(t, b, "busy", t.TempDir(), "sleep", "1000")
		mustCreate(t, b, "alive", t.TempDir(), "sleep", "1000")
		pid := session(t, b, "busy").PID
		daemon := zmxtest.StopDaemon(t, b, "busy")
		socket := filepath.Join(b.Dir(), "busy")
		before, err := os.Stat(socket)
		if err != nil {
			t.Fatal(err)
		}
		drain := fillBacklog(t, socket)

		assertNotAnswering(t, b, "busy")
		if got := session(t, b, "alive"); !got.Answered {
			t.Errorf("alive listed as %+v, want it answering", got)
		}
		after, err := os.Stat(socket)
		if err != nil || !os.SameFile(before, after) {
			t.Errorf("the busy socket after Portal's calls: %v; want it left as it was", err)
		}

		drain()
		if err := syscall.Kill(daemon, syscall.SIGCONT); err != nil {
			t.Fatal(err)
		}
		if !harnesstest.PollUntil(t, settle, 20*time.Millisecond, func() bool {
			got := session(t, b, "busy")
			return got.Answered && got.PID == pid
		}) {
			t.Errorf("busy after draining = %+v, want it answering as the session it was, pid %d", session(t, b, "busy"), pid)
		}
	})

	t.Run("it counts attached clients, not its own probe", func(t *testing.T) {
		b := zmxtest.New(t)
		mustCreate(t, b, "seen", t.TempDir(), "sleep", "1000")
		zmxtest.Attach(t, b, "seen")

		if !harnesstest.PollUntil(t, settle, 20*time.Millisecond, func() bool { return session(t, b, "seen").Clients == 1 }) {
			t.Errorf("Clients with one client attached = %d, want 1", session(t, b, "seen").Clients)
		}
	})

	t.Run("it probes without side effect on the session", func(t *testing.T) {
		b := zmxtest.New(t)
		mustCreate(t, b, "probed", t.TempDir(), "sh", "-c", `trap 'echo WINCH' WINCH; while :; do sleep 0.05; done`)
		mustCreate(t, b, "watched", t.TempDir(), "sh", "-c", `trap 'echo WINCH' WINCH; while :; do sleep 0.05; done`)
		zmxtest.Attach(t, b, "watched")
		if !harnesstest.PollUntil(t, settle, 20*time.Millisecond, func() bool { return session(t, b, "watched").Clients == 1 }) {
			t.Fatalf("client never attached: %+v", session(t, b, "watched"))
		}
		before := session(t, b, "probed")
		daemon := zmxtest.Daemon(t, b, "probed")
		resizes := map[string]int{}
		for _, name := range []string{"probed", "watched"} {
			resizes[name] = winches(t, b, name)
		}

		var wg sync.WaitGroup
		for range 20 {
			wg.Go(func() {
				for range 10 {
					if _, err := b.List(t.Context()); err != nil {
						t.Errorf("List: %v", err)
						return
					}
				}
			})
		}
		wg.Wait()

		after := session(t, b, "probed")
		if after.PID != before.PID || after.Clients != 0 || syscall.Kill(daemon, 0) != nil {
			t.Errorf("after 200 listings, probed = %+v and daemon alive %v; want %+v, alive", after, syscall.Kill(daemon, 0) == nil, before)
		}
		if got := session(t, b, "watched").Clients; got != 1 {
			t.Errorf("after 200 listings, watched has %d clients, want its one", got)
		}
		for name, before := range resizes {
			if got := winches(t, b, name); got != before {
				t.Errorf("%s saw %d resizes during the listings, want none", name, got-before)
			}
		}
	})

	t.Run("it keeps healthy calls fast among many stopped daemons", func(t *testing.T) {
		b := zmxtest.New(t)
		mustCreate(t, b, "healthy", t.TempDir(), "sleep", "1000")
		for _, name := range []string{"s1", "s2", "s3", "s4", "s5", "s6"} {
			mustCreate(t, b, name, t.TempDir(), "sleep", "1000")
			zmxtest.StopDaemon(t, b, name)
		}

		start := time.Now()
		if _, err := b.History(t.Context(), "healthy", 0); err != nil {
			t.Errorf("History(healthy): %v", err)
		}
		if exists, err := b.Exists(t.Context(), "healthy"); err != nil || !exists {
			t.Errorf("Exists(healthy) = %v, %v", exists, err)
		}
		if took := time.Since(start); took > 500*time.Millisecond {
			t.Errorf("calls on the healthy session took %s beside six stopped daemons", took)
		}

		start = time.Now()
		sessions, err := b.List(t.Context())
		if took := time.Since(start); took > 2500*time.Millisecond {
			t.Errorf("List took %s; six stopped daemons should cost one probe ceiling between them", took)
		}
		answered := 0
		for _, s := range sessions {
			if s.Answered {
				answered++
			}
		}
		if err != nil || len(sessions) != 7 || answered != 1 {
			t.Errorf("List = %+v, %v; want seven sessions, only healthy answering", sessions, err)
		}
	})

	t.Run("it reports the protocol version the daemon speaks", func(t *testing.T) {
		b := zmxtest.New(t)
		mustCreate(t, b, "versioned", t.TempDir(), "sleep", "1000")

		if got, err := b.ProtocolVersion(t.Context(), "versioned"); err != nil || got != zmx.ProtocolVersion {
			t.Errorf("ProtocolVersion = %d, %v; want %d", got, err, zmx.ProtocolVersion)
		}
	})

	t.Run("it refuses zmx's log directory as a name", func(t *testing.T) {
		b := zmxtest.New(t)

		if _, err := b.Create(t.Context(), "logs", t.TempDir(), nil); !errors.Is(err, backend.ErrInvalidName) {
			t.Errorf("Create(logs) = %v, want ErrInvalidName", err)
		}
	})
}
