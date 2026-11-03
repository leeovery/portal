package runnerreg_test

import (
	"bufio"
	"context"
	"errors"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

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

// handler is a Handler recording what it is asked, which accepts every
// request unless refuse is set.
type handler struct {
	mu        sync.Mutex
	lastInput time.Time
	refuse    error
	switches  []string
	detaches  int
}

func (h *handler) LastInput() time.Time {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.lastInput
}

func (h *handler) Switch(_ context.Context, session string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.refuse != nil {
		return h.refuse
	}
	h.switches = append(h.switches, session)
	return nil
}

func (h *handler) DetachToPicker(context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.refuse != nil {
		return h.refuse
	}
	h.detaches++
	return nil
}

// asked is what the handler has been asked: each switch's session, and how
// many detaches to the picker.
func (h *handler) asked() ([]string, int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.switches), h.detaches
}

// inputAt is a handler whose runner last forwarded input at at.
func inputAt(at time.Time) *handler { return &handler{lastInput: at} }

// register registers a runner on session, unregistered at the end of the
// test.
func register(t *testing.T, r runnerreg.Registry, session string, h runnerreg.Handler) *runnerreg.Registration {
	t.Helper()
	g, err := r.Register(session, h)
	if err != nil {
		t.Fatalf("Register %s: %v", session, err)
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

// staleSocket leaves a socket file nothing listens on, as a runner that died
// leaves one.
func staleSocket(t *testing.T, path string) {
	t.Helper()
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatalf("bind %s: %v", path, err)
	}
	ln.SetUnlinkOnClose(false)
	_ = ln.Close()
}

// silentSocket listens on path, taking connections and never answering.
func silentSocket(t *testing.T, path string) {
	t.Helper()
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("bind %s: %v", path, err)
	}
	var held []net.Conn
	var mu sync.Mutex
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			held = append(held, conn)
			mu.Unlock()
		}
	}()
	t.Cleanup(func() {
		_ = ln.Close()
		<-done
		mu.Lock()
		defer mu.Unlock()
		for _, c := range held {
			_ = c.Close()
		}
	})
}

// answeringSocket listens on path and answers each request with answer.
func answeringSocket(t *testing.T, path, answer string) {
	t.Helper()
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("bind %s: %v", path, err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_, _ = bufio.NewReader(conn).ReadBytes('\n')
			_, _ = conn.Write([]byte(answer + "\n"))
			_ = conn.Close()
		}
	}()
	t.Cleanup(func() {
		_ = ln.Close()
		<-done
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

// live is the registry's live runners, failing the test on an error.
func live(t *testing.T, r runnerreg.Registry) []runnerreg.Runner {
	t.Helper()
	runners, err := r.Live(t.Context())
	if err != nil {
		t.Fatalf("Live: %v", err)
	}
	return runners
}

// stem is a file name without its extension.
func stem(path string) string {
	base := filepath.Base(path)
	return base[:len(base)-len(filepath.Ext(base))]
}
