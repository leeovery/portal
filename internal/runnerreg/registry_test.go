package runnerreg_test

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/runnerreg"
)

func TestRegister(t *testing.T) {
	t.Run("it lists the runner under its token: its pid and session in an entry, its socket beside it", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		g := register(t, r, "tok123", "api-x7Kd9a", &handler{})

		if g.Entry() != r.EntryPath("tok123") || g.Socket() != r.SocketPath("tok123") || filepath.Dir(g.Socket()) != r.Dir() {
			t.Errorf("entry %s, socket %s; want tok123.json and tok123.sock in %s", g.Entry(), g.Socket(), r.Dir())
		}
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
		if err := r.Switch(t.Context(), "tok123", "api-x7Kd9a", "blog-c3d4"); err != nil {
			t.Errorf("a switch through the socket = %v, want it served", err)
		}
	})

	t.Run("only this user can reach it: the directory 0700, the socket 0600", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		if err := os.MkdirAll(r.Dir(), 0o755); err != nil {
			t.Fatal(err)
		}
		g := register(t, r, "tok123", "api-x7Kd9a", &handler{})

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
		g := register(t, r, "tok123", "api-x7Kd9a", &handler{})

		if err := g.SetSession("blog-c3d4"); err != nil {
			t.Fatalf("SetSession: %v", err)
		}

		if data, err := os.ReadFile(g.Entry()); err != nil || !strings.Contains(string(data), `"session":"blog-c3d4"`) {
			t.Errorf("entry = %s, %v; want it on blog-c3d4", data, err)
		}
	})

	t.Run("Close removes the entry and the socket, and may be called again", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		g := register(t, r, "tok123", "api-x7Kd9a", &handler{})

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

	t.Run("it first sweeps entries whose pids aren't running, on any session, and orphan sockets that refuse", func(t *testing.T) {
		sink := logtest.Install(t)
		r := runnerreg.New(stateDir(t))
		writeEntry(t, r, "dead01", entryFor(deadPID(t), "api-x7Kd9a"))
		refusingSocket(t, r.SocketPath("dead01"))
		writeEntry(t, r, "dead02", entryFor(deadPID(t), "blog-c3d4"))
		writeEntry(t, r, "live01", entryFor(otherPID(t), "blog-c3d4"))
		refusingSocket(t, r.SocketPath("live01"))
		writeEntry(t, r, "odd001", "not json")
		refusingSocket(t, r.SocketPath("orphan"))
		silentSocket(t, r.SocketPath("listen")) // a runner yet to write its entry

		register(t, r, "tok123", "docs-e5f6", &handler{})

		want := []string{"listen.sock", "live01.json", "live01.sock", "odd001.json", "tok123.json", "tok123.sock"}
		if got := files(t, r.Dir()); !slices.Equal(got, want) {
			t.Errorf("the registry holds %q, want %q", got, want)
		}
		var swept []string
		for _, rec := range sink.Records().Matching("attach", "stale runner") {
			swept = append(swept, filepath.Base(rec.AttrString(t, "path")))
		}
		slices.Sort(swept)
		if want := []string{"dead01.json", "dead02.json", "orphan.sock"}; !slices.Equal(swept, want) {
			t.Errorf("stale runner records name %q, want %q", swept, want)
		}
	})

	t.Run("a token no runner could have is refused, making nothing", func(t *testing.T) {
		for _, token := range []string{"", "../tok", "a/b", "tok.12", "tók123"} {
			r := runnerreg.New(stateDir(t))

			if g, err := r.Register(token, "api-x7Kd9a", &handler{}); err == nil {
				_ = g.Close()
				t.Errorf("Register(%q) = nil, want a refusal", token)
			}
			if exists(t, r.Dir()) {
				t.Errorf("Register(%q) made %s", token, r.Dir())
			}
		}
	})
}

func TestRegister_SocketPathLength(t *testing.T) {
	// Every socket path is <dir>/<token>.sock, the token six characters.
	const pathBeyondDir = len("/tok123.sock")
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
		g := register(t, r, "tok123", "api-x7Kd9a", &handler{})

		if len(g.Socket()) != runnerreg.MaxSocketPath() {
			t.Fatalf("socket path is %d bytes, want %d; the case is unarmed", len(g.Socket()), runnerreg.MaxSocketPath())
		}
		if err := r.Switch(t.Context(), "tok123", "api-x7Kd9a", "blog-c3d4"); err != nil {
			t.Errorf("a switch through the socket = %v, want it served", err)
		}
	})

	t.Run("a path one byte longer is refused, never cut short, and makes nothing", func(t *testing.T) {
		state := stateDirWithSocketPath(t, runnerreg.MaxSocketPath()+1)
		r := runnerreg.New(state)

		g, err := r.Register("tok123", "api-x7Kd9a", &handler{})

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

// failingListener fails Accept as a process out of descriptors does, as many
// times as fails says, then takes connections from the listener it wraps.
type failingListener struct {
	net.Listener
	fails atomic.Int32
}

func (l *failingListener) Accept() (net.Conn, error) {
	if l.fails.Add(-1) >= 0 {
		return nil, &net.OpError{Op: "accept", Net: "unix", Err: os.NewSyscallError("accept", syscall.EMFILE)}
	}
	return l.Listener.Accept()
}

// servedThrough serves a registration of tok123 through a listener on its
// socket that fails fails times before it takes a connection.
func servedThrough(t *testing.T, fails int32) (*runnerreg.Registration, *failingListener) {
	t.Helper()
	r := runnerreg.New(stateDir(t))
	if err := os.MkdirAll(r.Dir(), 0o700); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("unix", r.SocketPath("tok123"))
	if err != nil {
		t.Fatal(err)
	}
	failing := &failingListener{Listener: ln}
	failing.fails.Store(fails)
	g := r.ServeThrough("tok123", "api-x7Kd9a", &handler{}, failing)
	t.Cleanup(func() { _ = g.Close() })
	return g, failing
}

func TestRegister_ServingThroughAcceptFailures(t *testing.T) {
	t.Run("a connection the socket fails to take is taken on a retry, with one WARN for each run of failures", func(t *testing.T) {
		sink := logtest.Install(t)
		g, failing := servedThrough(t, 3)
		request := `{"version":1,"request":"switch","session":"blog-c3d4",` + due() + `}` + "\n"

		for run := range 2 {
			if got := call(t, g.Socket(), request); !got.OK {
				t.Fatalf("run %d: answer = %+v, want the switch served once the socket takes the connection", run, got)
			}
			failing.fails.Store(2)
		}

		recs := sink.Records().Matching("attach", "serve failed").AtExactLevel(slog.LevelWarn)
		if len(recs) != 2 {
			t.Fatalf("serve failed records = %d, want one for each run of failures", len(recs))
		}
		if err := recs[0].ErrorAttr(t, "error"); !errors.Is(err, syscall.EMFILE) {
			t.Errorf("serve failed error = %v, want the socket's EMFILE", err)
		}
		if got := recs[0].AttrString(t, "session"); got != "api-x7Kd9a" {
			t.Errorf("serve failed session = %q, want api-x7Kd9a", got)
		}
	})

	t.Run("Close ends the retries of a socket that never takes a connection", func(t *testing.T) {
		sink := logtest.Install(t)
		g, _ := servedThrough(t, 1<<30)
		if !harnesstest.PollUntil(t, 5*time.Second, 5*time.Millisecond, func() bool {
			return len(sink.Records().Matching("attach", "serve failed")) > 0
		}) {
			t.Fatal("the socket never failed to take a connection")
		}

		closed := make(chan error, 1)
		go func() { closed <- g.Close() }()

		select {
		case err := <-closed:
			if err != nil {
				t.Errorf("Close = %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Close never returned: serve went on retrying")
		}
	})
}

// Close's cut wins over every deadline the answer sets, before it or after:
// a request being answered as Close begins never waits on a peer that is
// slow to send it or to read the answer.
func TestRegistration_CloseCutsAnAnswerShortWhateverItsDeadlines(t *testing.T) {
	runner, peer := net.Pipe()
	t.Cleanup(func() { _ = peer.Close() })
	answered := make(chan struct{})
	start := time.Now()

	go func() {
		defer close(answered)
		runnerreg.AnswerClosed(runner, &handler{})
	}()

	select {
	case <-answered:
		if took := time.Since(start); took > runnerreg.Timeout()/4 {
			t.Errorf("the answer took %s, want it cut short at once, not waiting out %s", took, runnerreg.Timeout())
		}
	case <-time.After(2 * runnerreg.Timeout()):
		t.Fatal("the answer never ended")
	}
}
