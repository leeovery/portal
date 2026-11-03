package zmx_test

import (
	"encoding/binary"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/shellquote"
)

// hostname is the machine name the unit tests' Backends report.
const hostname = "test-host.local"

// ceiling is the unit tests' bound on every operation.
const ceiling = 200 * time.Millisecond

// socketDir makes a zmx directory for fake daemons. t.TempDir is too long a
// path for a socket on macOS, so it sits under /tmp beside zmxtest's.
func socketDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "zx-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// newBackend binds a Backend to dir with the given environment, a fixed
// hostname and short ceilings. Its binary runs nothing.
func newBackend(t *testing.T, dir string, environ ...string) *zmx.Backend {
	t.Helper()
	return newBackendRunning(t, "/nonexistent/zmx", dir, environ...)
}

func newBackendRunning(t *testing.T, binary, dir string, environ ...string) *zmx.Backend {
	t.Helper()
	b := mustNew(t, zmx.Config{
		Binary:   binary,
		Dir:      dir,
		Environ:  func() []string { return slices.Clone(environ) },
		Hostname: func() (string, error) { return hostname, nil },
	})
	zmx.ShortenCeilings(b, ceiling)
	return b
}

func mustNew(t *testing.T, cfg zmx.Config) *zmx.Backend {
	t.Helper()
	b, err := zmx.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// request is one message a fake daemon received.
type request struct {
	tag     byte
	version byte
	payload []byte
}

// daemon is a fake zmx daemon listening on a socket.
type daemon struct {
	path string

	mu       sync.Mutex
	requests []request
	conns    []net.Conn
}

// serve listens on dir/name as a zmx daemon would, handing each connection
// to handle. At cleanup it closes every connection and waits for handle.
func serve(t *testing.T, dir, name string, handle func(d *daemon, c net.Conn)) *daemon {
	t.Helper()
	d := &daemon{path: filepath.Join(dir, name)}
	l, err := net.Listen("unix", d.path)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	t.Cleanup(func() {
		_ = l.Close()
		d.mu.Lock()
		for _, c := range d.conns {
			_ = c.Close()
		}
		d.mu.Unlock()
		wg.Wait()
	})
	wg.Go(func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			d.mu.Lock()
			d.conns = append(d.conns, c)
			d.mu.Unlock()
			wg.Go(func() {
				defer func() { _ = c.Close() }()
				handle(d, c)
			})
		}
	})
	return d
}

// read reads one request and records it.
func (d *daemon) read(c net.Conn) (request, error) {
	h := make([]byte, 8)
	if _, err := io.ReadFull(c, h); err != nil {
		return request{}, err
	}
	r := request{tag: h[0], version: h[1], payload: make([]byte, binary.NativeEndian.Uint32(h[4:]))}
	if _, err := io.ReadFull(c, r.payload); err != nil {
		return request{}, err
	}
	d.mu.Lock()
	d.requests = append(d.requests, r)
	d.mu.Unlock()
	return r, nil
}

func (d *daemon) received() []request {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Clone(d.requests)
}

// answering reads a request, writes reply, and holds the connection until the
// client closes it.
func answering(reply ...[]byte) func(*daemon, net.Conn) {
	return func(d *daemon, c net.Conn) {
		if _, err := d.read(c); err != nil {
			return
		}
		for _, r := range reply {
			_, _ = c.Write(r)
		}
		_, _ = io.Copy(io.Discard, c)
	}
}

// answeringAfter reads a request, waits delay, writes reply, and holds the
// connection until the client closes it.
func answeringAfter(delay time.Duration, reply ...[]byte) func(*daemon, net.Conn) {
	return func(d *daemon, c net.Conn) {
		if _, err := d.read(c); err != nil {
			return
		}
		time.Sleep(delay)
		for _, r := range reply {
			_, _ = c.Write(r)
		}
		_, _ = io.Copy(io.Discard, c)
	}
}

// hangingUp reads a request, writes reply, and closes the connection.
func hangingUp(reply ...[]byte) func(*daemon, net.Conn) {
	return func(d *daemon, c net.Conn) {
		if _, err := d.read(c); err != nil {
			return
		}
		for _, r := range reply {
			_, _ = c.Write(r)
		}
	}
}

// silent records requests and answers nothing until the client goes.
func silent(d *daemon, c net.Conn) {
	for {
		if _, err := d.read(c); err != nil {
			return
		}
	}
}

// ending reads a request, then unlinks its socket and closes, as a zmx daemon
// ends on Kill.
func ending(d *daemon, c net.Conn) {
	if _, err := d.read(c); err != nil {
		return
	}
	_ = os.Remove(d.path)
}

// refusing leaves a socket at dir/name that refuses every connection, as a
// SIGKILLed zmx daemon's does, and as a live one's does while its listen
// backlog is full.
func refusing(t *testing.T, dir, name string) {
	t.Helper()
	l, err := net.Listen("unix", filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	l.(*net.UnixListener).SetUnlinkOnClose(false)
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
}

// stale leaves a refusing socket at dir/name made, by its mtime, before this
// machine booted, so no running daemon can be behind it.
func stale(t *testing.T, dir, name string) {
	t.Helper()
	refusing(t, dir, name)
	backdate(t, filepath.Join(dir, name))
}

// backdate sets a file's times to before this machine booted.
func backdate(t *testing.T, path string) {
	t.Helper()
	beforeBoot := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(path, beforeBoot, beforeBoot); err != nil {
		t.Fatal(err)
	}
}

// frame is a message as zmx writes one.
func frame(tag, version byte, payload []byte) []byte {
	h := []byte{tag, version, 0, 0, 0, 0, 0, 0}
	binary.NativeEndian.PutUint32(h[4:], uint32(len(payload)))
	return append(h, payload...)
}

// infoFrame is the pinned build's Info answer for the session running
// fixtureScript, captured by TestInfoFixture.
func infoFrame(t *testing.T) []byte {
	t.Helper()
	f, err := os.ReadFile(filepath.Join("testdata", "info.frame"))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

const (
	// fixtureScript is what the session behind testdata/info.frame runs
	// under sh -c: it reports its cwd on another host by OSC 7.
	fixtureScript  = `printf '\033]7;file://example-host/srv/demo%%20dir\033\\'; sleep 1000`
	fixtureCommand = `sh -c 'printf '\''\033]7;file://example-host/srv/demo%%20dir\033\\'\''; sleep 1000'`
	fixtureCwd     = "file://example-host/srv/demo%20dir"
)

// withCwd is an Info frame with its cwd replaced.
func withCwd(f []byte, cwd string) []byte {
	f = slices.Clone(f)
	binary.NativeEndian.PutUint16(f[8+14:], uint16(len(cwd)))
	clear(f[8+272 : 8+528])
	copy(f[8+272:8+528], cwd)
	return f
}

// fakeZmx writes a stand-in for the zmx binary. It records its arguments,
// working directory, environment and stdin into the returned log directory,
// then runs body.
func fakeZmx(t *testing.T, body string) (binary, log string) {
	t.Helper()
	dir := t.TempDir()
	log = filepath.Join(dir, "log")
	if err := os.Mkdir(log, 0o700); err != nil {
		t.Fatal(err)
	}
	at := func(file string) string { return shellquote.Single(filepath.Join(log, file)) }
	script := "#!/bin/sh\n" +
		`for a in "$@"; do printf '%s\0' "$a"; done > ` + at("args") + "\n" +
		"pwd -P > " + at("cwd") + "\n" +
		"env -0 > " + at("env") + "\n" +
		"cat > " + at("stdin") + "\n" +
		body + "\n"
	binary = filepath.Join(dir, "zmx")
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return binary, log
}
