package runnerreg_test

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/runnerreg"
)

func TestRegister(t *testing.T) {
	t.Run("it lists the runner: its pid and session in an entry, its socket beside it", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		at := time.Date(2026, 11, 3, 9, 30, 0, 0, time.UTC)
		g := register(t, r, "api-x7Kd9a", inputAt(at))

		data, err := os.ReadFile(g.Entry())
		if err != nil {
			t.Fatalf("read the entry: %v", err)
		}
		var e struct {
			PID     int    `json:"pid"`
			Session string `json:"session"`
		}
		if err := json.Unmarshal(data, &e); err != nil || e.PID != os.Getpid() || e.Session != "api-x7Kd9a" {
			t.Errorf("entry = %s, %v; want this pid and session api-x7Kd9a", data, err)
		}
		if stem(g.Socket()) != stem(g.Entry()) || filepath.Dir(g.Socket()) != r.Dir() {
			t.Errorf("socket %s, entry %s; want them side by side, named alike", g.Socket(), g.Entry())
		}
		runners := live(t, r)
		if len(runners) != 1 {
			t.Fatalf("Live = %+v, want the one runner", runners)
		}
		if rn := runners[0]; rn.PID != os.Getpid() || rn.Session != "api-x7Kd9a" || !rn.LastInput.Equal(at) {
			t.Errorf("Live = %+v; want this pid on api-x7Kd9a, last input at %v", rn, at)
		}
	})

	t.Run("only this user can reach it: the directory 0700, the socket 0600", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		if err := os.MkdirAll(r.Dir(), 0o755); err != nil {
			t.Fatal(err)
		}
		g := register(t, r, "api-x7Kd9a", &handler{})

		dir, err := os.Stat(r.Dir())
		if err != nil || dir.Mode().Perm() != 0o700 {
			t.Errorf("directory mode = %v, %v; want 0700, even made wider before", dir.Mode().Perm(), err)
		}
		socket, err := os.Stat(g.Socket())
		if err != nil || socket.Mode()&fs.ModeSocket == 0 || socket.Mode().Perm() != 0o600 {
			t.Errorf("socket mode = %v, %v; want a socket, 0600", socket.Mode(), err)
		}
		entry, err := os.Stat(g.Entry())
		if err != nil || entry.Mode().Perm() != 0o600 {
			t.Errorf("entry mode = %v, %v; want 0600", entry.Mode().Perm(), err)
		}
	})

	t.Run("SetSession moves the entry to the session switched to", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		g := register(t, r, "api-x7Kd9a", &handler{})

		if err := g.SetSession("blog-c3d4"); err != nil {
			t.Fatalf("SetSession: %v", err)
		}

		if runners := live(t, r); len(runners) != 1 || runners[0].Session != "blog-c3d4" {
			t.Errorf("Live = %+v, want the one runner on blog-c3d4", runners)
		}
	})

	t.Run("Close removes the entry and the socket, and may be called again", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		g := register(t, r, "api-x7Kd9a", &handler{})

		for range 2 {
			if err := g.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}
		}

		if names := files(t, r.Dir()); len(names) != 0 {
			t.Errorf("the registry holds %q after Close, want nothing", names)
		}
		if err := g.SetSession("blog-c3d4"); err == nil {
			t.Error("SetSession after Close = nil, want a refusal")
		}
	})
}

func TestRegister_SocketPathLength(t *testing.T) {
	// Every socket path is <dir>/<token>.sock, the token six characters.
	const pathBeyondDir = len("/abcdef.sock")
	stateDirWithSocketPath := func(t *testing.T, length int) string {
		t.Helper()
		dir := stateDir(t)
		pad := length - pathBeyondDir - len(dir) - len("/runners") - 1
		if pad < 1 {
			t.Fatalf("the state dir %s is too long to make a socket path of %d bytes", dir, length)
		}
		state := filepath.Join(dir, strings.Repeat("s", pad))
		if err := os.Mkdir(state, 0o700); err != nil {
			t.Fatal(err)
		}
		return state
	}

	t.Run("a path filling a socket address binds whole", func(t *testing.T) {
		r := runnerreg.New(stateDirWithSocketPath(t, runnerreg.MaxSocketPath()))
		g := register(t, r, "api-x7Kd9a", inputAt(time.Now()))

		if len(g.Socket()) != runnerreg.MaxSocketPath() {
			t.Fatalf("socket path is %d bytes, want %d; the case is unarmed", len(g.Socket()), runnerreg.MaxSocketPath())
		}
		if runners := live(t, r); len(runners) != 1 || runners[0].Socket() != g.Socket() {
			t.Errorf("Live = %+v, want the runner answering at %s", runners, g.Socket())
		}
	})

	t.Run("a path one byte longer is refused, never cut short, and makes nothing", func(t *testing.T) {
		state := stateDirWithSocketPath(t, runnerreg.MaxSocketPath()+1)
		r := runnerreg.New(state)

		g, err := r.Register("api-x7Kd9a", &handler{})

		if !errors.Is(err, runnerreg.ErrSocketPathTooLong) {
			t.Fatalf("Register = %v, %v; want ErrSocketPathTooLong", g, err)
		}
		for _, want := range []string{r.Dir(), strconv.Itoa(runnerreg.MaxSocketPath()+1) + " bytes", strconv.Itoa(runnerreg.MaxSocketPath())} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q does not say %q", err, want)
			}
		}
		if exists(t, r.Dir()) {
			t.Errorf("the refused registration made %s", r.Dir())
		}
	})
}

func TestLive_RemovesTheDead(t *testing.T) {
	const session = `"session":"api-x7Kd9a"`
	entryFor := func(pid int) string { return `{"pid":` + strconv.Itoa(pid) + `,` + session + `}` }

	t.Run("an entry whose pid is dead goes, with its socket", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		writeEntry(t, r, "dead01", entryFor(deadPID(t)))
		staleSocket(t, r.SocketPath("dead01"))

		if runners := live(t, r); len(runners) != 0 {
			t.Errorf("Live = %+v, want none", runners)
		}
		if names := files(t, r.Dir()); len(names) != 0 {
			t.Errorf("the registry holds %q, want the dead entry and its socket gone", names)
		}
	})

	t.Run("a pid another process holds now, its socket refusing, counts as dead", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		writeEntry(t, r, "reuse1", entryFor(otherPID(t)))
		staleSocket(t, r.SocketPath("reuse1"))

		if runners := live(t, r); len(runners) != 0 {
			t.Errorf("Live = %+v, want none", runners)
		}
		if names := files(t, r.Dir()); len(names) != 0 {
			t.Errorf("the registry holds %q, want the entry and its socket gone", names)
		}
	})

	t.Run("a socket answering as another pid counts as dead", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		g := register(t, r, "api-x7Kd9a", inputAt(time.Now()))
		writeEntry(t, r, stem(g.Entry()), entryFor(otherPID(t)))

		if runners := live(t, r); len(runners) != 0 {
			t.Errorf("Live = %+v, want none: the socket is not that pid's", runners)
		}
		if exists(t, g.Entry()) || exists(t, g.Socket()) {
			t.Error("the entry or its socket survived")
		}
	})

	t.Run("a socket that never answers counts as dead", func(t *testing.T) {
		runnerreg.SetTimeout(t, 100*time.Millisecond)
		r := runnerreg.New(stateDir(t))
		writeEntry(t, r, "silent", entryFor(os.Getpid()))
		silentSocket(t, r.SocketPath("silent"))

		if runners := live(t, r); len(runners) != 0 {
			t.Errorf("Live = %+v, want none", runners)
		}
		if exists(t, r.EntryPath("silent")) || exists(t, r.SocketPath("silent")) {
			t.Error("the entry or its socket survived")
		}
	})

	t.Run("a missing socket counts as dead", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		writeEntry(t, r, "nosock", entryFor(os.Getpid()))

		if runners := live(t, r); len(runners) != 0 {
			t.Errorf("Live = %+v, want none", runners)
		}
		if exists(t, r.EntryPath("nosock")) {
			t.Error("the entry survived")
		}
	})

	t.Run("the live are listed beside the dead, and kept", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		g := register(t, r, "api-x7Kd9a", inputAt(time.Now()))
		writeEntry(t, r, "dead01", entryFor(deadPID(t)))

		if runners := live(t, r); len(runners) != 1 || runners[0].Socket() != g.Socket() {
			t.Errorf("Live = %+v, want the registered runner alone", runners)
		}
		if !exists(t, g.Entry()) || !exists(t, g.Socket()) {
			t.Error("the live runner's entry or socket went")
		}
	})
}

func TestLive_LeavesWhatItCannotJudge(t *testing.T) {
	t.Run("an entry it can't read", func(t *testing.T) {
		for name, body := range map[string]string{
			"not json":       "not json",
			"no pid":         `{"session":"api-x7Kd9a"}`,
			"no session":     `{"pid":` + strconv.Itoa(os.Getpid()) + `}`,
			"a field to add": `{"pid":` + strconv.Itoa(os.Getpid()) + `,"session":"api-x7Kd9a","tty":"/dev/ttys001"}`,
		} {
			t.Run(name, func(t *testing.T) {
				r := runnerreg.New(stateDir(t))
				writeEntry(t, r, "odd001", body)
				staleSocket(t, r.SocketPath("odd001"))

				if runners := live(t, r); len(runners) != 0 {
					t.Errorf("Live = %+v, want none", runners)
				}
				if !exists(t, r.EntryPath("odd001")) || !exists(t, r.SocketPath("odd001")) {
					t.Error("an entry Live could not read was removed")
				}
			})
		}
	})

	t.Run("a runner that refuses, speaking another version", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		writeEntry(t, r, "newer1", `{"pid":`+strconv.Itoa(os.Getpid())+`,"session":"api-x7Kd9a"}`)
		answeringSocket(t, r.SocketPath("newer1"), `{"version":2,"ok":false,"refusal":"protocol version 1, where this runner speaks 2"}`)

		if runners := live(t, r); len(runners) != 0 {
			t.Errorf("Live = %+v, want none", runners)
		}
		if !exists(t, r.EntryPath("newer1")) {
			t.Error("a runner that answered was removed")
		}
	})

	t.Run("a runner whose socket's path is too long to reach from here", func(t *testing.T) {
		long := filepath.Join(stateDir(t), strings.Repeat("s", runnerreg.MaxSocketPath()))
		r := runnerreg.New(long)
		writeEntry(t, r, "long01", `{"pid":`+strconv.Itoa(os.Getpid())+`,"session":"api-x7Kd9a"}`)

		if runners := live(t, r); len(runners) != 0 {
			t.Errorf("Live = %+v, want none", runners)
		}
		if !exists(t, r.EntryPath("long01")) {
			t.Error("an entry whose socket Live could not reach was removed")
		}
	})

	t.Run("anything, once the caller's context has ended", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		writeEntry(t, r, "silent", `{"pid":`+strconv.Itoa(os.Getpid())+`,"session":"api-x7Kd9a"}`)
		silentSocket(t, r.SocketPath("silent"))
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		if _, err := r.Live(ctx); err != nil {
			t.Fatalf("Live: %v", err)
		}
		if !exists(t, r.EntryPath("silent")) {
			t.Error("an entry was removed for want of an answer the caller stopped waiting for")
		}
	})
}

func TestLive_AnUnmadeRegistryHoldsNone(t *testing.T) {
	runners, err := runnerreg.New(filepath.Join(stateDir(t), "never-made")).Live(t.Context())
	if err != nil || len(runners) != 0 {
		t.Errorf("Live = %+v, %v; want none and no error", runners, err)
	}
}

func TestLive_LogsTheStale(t *testing.T) {
	sink := logtest.Install(t)
	r := runnerreg.New(stateDir(t))
	pid := deadPID(t)
	writeEntry(t, r, "dead01", `{"pid":`+strconv.Itoa(pid)+`,"session":"api-x7Kd9a"}`)

	live(t, r)

	rec := sink.Records().Matching("attach", "stale runner").AtExactLevel(slog.LevelDebug).Only(t, "stale runner record")
	if got := rec.IntAttr(t, "target_pid"); got != int64(pid) {
		t.Errorf("target_pid = %d, want %d", got, pid)
	}
	if got := rec.AttrString(t, "reason"); got != "dead" {
		t.Errorf("reason = %q, want dead", got)
	}
	if got := rec.AttrString(t, "path"); got != r.EntryPath("dead01") {
		t.Errorf("path = %q, want %q", got, r.EntryPath("dead01"))
	}
}

func TestLatest(t *testing.T) {
	at := time.Date(2026, 11, 3, 9, 30, 0, 0, time.UTC)
	runner := func(pid int, input time.Time) runnerreg.Runner {
		return runnerreg.Runner{PID: pid, Session: "api-x7Kd9a", LastInput: input}
	}

	for _, tc := range []struct {
		name    string
		runners []runnerreg.Runner
		want    int
	}{
		{"none", nil, 0},
		{"none that has forwarded input", []runnerreg.Runner{runner(1, time.Time{}), runner(2, time.Time{})}, 0},
		{"the one that forwarded input last", []runnerreg.Runner{runner(1, at), runner(2, at.Add(time.Second)), runner(3, at.Add(-time.Second))}, 2},
		{"one that has forwarded input over one that hasn't", []runnerreg.Runner{runner(1, time.Time{}), runner(2, at)}, 2},
		{"the latest by a nanosecond", []runnerreg.Runner{runner(1, at.Add(time.Nanosecond)), runner(2, at)}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := runnerreg.Latest(tc.runners)

			if ok != (tc.want != 0) || got.PID != tc.want {
				t.Errorf("Latest = %+v, %v; want pid %d", got, ok, tc.want)
			}
		})
	}
}
