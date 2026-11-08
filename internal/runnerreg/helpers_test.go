package runnerreg_test

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/bootid"
	"github.com/leeovery/portal/internal/runnerreg"
)

// stateDir is a state directory of the test's own, short enough for the
// registry's sockets: t.TempDir sits under $TMPDIR, which on macOS leaves
// too little of a socket address's 104 bytes.
func stateDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "rr-")
	if err != nil {
		t.Fatalf("make a state dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// handler is a Handler recording what it is asked and each request's
// deadline, which accepts every request unless refuse is set.
type handler struct {
	mu        sync.Mutex
	refuse    error
	switches  []string
	detaches  int
	confirms  []string
	takes     []takeCall
	deadlines []time.Time
}

// takeCall is a take the handler was asked: the session, and where it went.
type takeCall struct{ session, by string }

func (h *handler) Switch(ctx context.Context, session string) error {
	return h.answer(ctx, func() { h.switches = append(h.switches, session) })
}

func (h *handler) DetachToPicker(ctx context.Context) error {
	return h.answer(ctx, func() { h.detaches++ })
}

func (h *handler) ConfirmSession(ctx context.Context, session string) error {
	return h.answer(ctx, func() { h.confirms = append(h.confirms, session) })
}

func (h *handler) Take(ctx context.Context, session, by string) error {
	return h.answer(ctx, func() { h.takes = append(h.takes, takeCall{session, by}) })
}

// askedConfirms is each session the handler was asked whether it shows.
func (h *handler) askedConfirms() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.confirms)
}

// askedTakes is each take the handler was asked.
func (h *handler) askedTakes() []takeCall {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.takes)
}

// askedDeadlines is the deadline of each request the handler took.
func (h *handler) askedDeadlines() []time.Time {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.deadlines)
}

func (h *handler) answer(ctx context.Context, record func()) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.refuse != nil {
		return h.refuse
	}
	deadline, _ := ctx.Deadline()
	h.deadlines = append(h.deadlines, deadline)
	record()
	return nil
}

// asked is what the handler has been asked: each switch's session, and how
// many detaches to the picker.
func (h *handler) asked() ([]string, int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.switches), h.detaches
}

// register registers a runner under token on session, unregistered at the
// end of the test.
func register(t *testing.T, r runnerreg.Registry, token, session string, h runnerreg.Handler) *runnerreg.Registration {
	t.Helper()
	g, err := r.Register(token, session, h)
	if err != nil {
		t.Fatalf("Register %s on %s: %v", token, session, err)
	}
	t.Cleanup(func() { _ = g.Close() })
	return g
}

// writeEntry writes the entry named token by hand.
func writeEntry(t *testing.T, r runnerreg.Registry, token, body string) {
	t.Helper()
	if err := os.MkdirAll(r.Dir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(r.EntryPath(token), []byte(body), 0o600); err != nil {
		t.Fatalf("write entry %s: %v", token, err)
	}
}

// thisBoot is the current boot's id, which the registry reads unless a test
// stands in another; empty where it can't be read, and the registry then
// judges by pid alone.
var thisBoot, _ = bootid.Read()

// entryFor is an entry naming pid on session, written in this boot.
func entryFor(pid int, session string) string {
	return entryIn(pid, session, thisBoot)
}

// entryIn is an entry naming pid on session, written in boot, or naming no
// boot when it is empty.
func entryIn(pid int, session, boot string) string {
	body := `{"pid":` + strconv.Itoa(pid) + `,"session":"` + session + `"`
	if boot != "" {
		body += `,"boot_id":"` + boot + `"`
	}
	return body + "}"
}

// refusingSocket leaves a socket file nothing listens on, as a runner that
// died leaves one.
func refusingSocket(t *testing.T, path string) {
	t.Helper()
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatalf("bind %s: %v", path, err)
	}
	ln.SetUnlinkOnClose(false)
	_ = ln.Close()
}

// listen serves path, handing each connection to handle, until the test
// ends.
func listen(t *testing.T, path string, handle func(net.Conn)) {
	t.Helper()
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("bind %s: %v", path, err)
	}
	var mu sync.Mutex
	var held []net.Conn
	var served sync.WaitGroup
	served.Go(func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			held = append(held, conn)
			mu.Unlock()
			served.Go(func() { handle(conn) })
		}
	})
	t.Cleanup(func() {
		_ = ln.Close()
		mu.Lock()
		for _, c := range held {
			_ = c.Close()
		}
		mu.Unlock()
		served.Wait()
	})
}

// silentSocket listens on path, taking connections and never answering.
func silentSocket(t *testing.T, path string) {
	t.Helper()
	listen(t, path, func(conn net.Conn) { _, _ = io.Copy(io.Discard, conn) })
}

// barrier opens once n have arrived at it.
type barrier struct {
	mu      sync.Mutex
	waiting int
	open    chan struct{}
}

func newBarrier(n int) *barrier {
	return &barrier{waiting: n, open: make(chan struct{})}
}

func (b *barrier) arrive() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.waiting--; b.waiting == 0 {
		close(b.open)
	}
}

// arrival is a request a socket read, and when it had read it whole.
type arrival struct {
	line string
	at   time.Time
}

// gatedSocket listens on path, reads each request whole, sends it on
// arrivals, and waits at b, accepting the request only once b opens.
func gatedSocket(t *testing.T, path string, b *barrier, arrivals chan<- arrival) {
	t.Helper()
	listen(t, path, func(conn net.Conn) {
		defer func() { _ = conn.Close() }()
		line, _ := io.ReadAll(conn)
		arrivals <- arrival{line: string(line), at: time.Now()}
		b.arrive()
		select {
		case <-b.open:
			_, _ = conn.Write([]byte(ourAnswer(`"ok":true`) + "\n"))
		case <-t.Context().Done():
		}
	})
}

// answeringSocket listens on path, reads each request whole, and answers it
// with answer. Each request read is sent on requests, when it is set.
func answeringSocket(t *testing.T, path, answer string, requests chan<- string) {
	t.Helper()
	listen(t, path, func(conn net.Conn) {
		defer func() { _ = conn.Close() }()
		line, _ := io.ReadAll(conn)
		if requests != nil {
			requests <- string(line)
		}
		_, _ = conn.Write([]byte(answer + "\n"))
	})
}

// hangingUpSocket listens on path, and closes each connection unanswered.
func hangingUpSocket(t *testing.T, path string) {
	t.Helper()
	listen(t, path, func(conn net.Conn) {
		_, _ = io.ReadAll(conn)
		_ = conn.Close()
	})
}

// deadPID is the pid of a process of the test's own that has exited and been
// reaped.
func deadPID(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Fatalf("run true: %v", err)
	}
	return cmd.Process.Pid
}

// otherPID is the pid of a running process of the test's own that is no
// runner, standing in for one that took a dead runner's pid.
func otherPID(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start sleep: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	return cmd.Process.Pid
}

func exists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Lstat(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("stat %s: %v", path, err)
	}
	return err == nil
}

// files lists the names in dir.
func files(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("read %s: %v", dir, err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// ourAnswer is an answer from this process, which registers the runners the
// tests ask.
func ourAnswer(body string) string {
	return `{"version":1,` + body + `,"pid":` + strconv.Itoa(os.Getpid()) + `}`
}
