package zmx_test

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
)

// olderDaemon serves as a zmx 0.8.1 daemon frames its talk: each header a
// packed tag u8 and len u32, the length in bytes 1-4. It broadcasts output
// frames first, then answers each Info with a record of infoLen bytes and
// reads every other request's payload as 0.8.1 would, answering nothing.
func olderDaemon(infoLen uint32, output ...[]byte) func(*daemon, net.Conn) {
	header := func(t byte, n uint32) []byte {
		h := make([]byte, 8)
		h[0] = t
		binary.NativeEndian.PutUint32(h[1:5], n)
		return h
	}
	return func(d *daemon, c net.Conn) {
		for _, o := range output {
			_, _ = c.Write(append(header(1, uint32(len(o))), o...))
		}
		for {
			h := make([]byte, 8)
			if _, err := io.ReadFull(c, h); err != nil {
				return
			}
			payload := make([]byte, binary.NativeEndian.Uint32(h[1:5]))
			if _, err := io.ReadFull(c, payload); err != nil {
				return
			}
			d.mu.Lock()
			d.requests = append(d.requests, request{tag: h[0], version: h[1], payload: payload})
			d.mu.Unlock()
			if h[0] == 6 {
				_, _ = c.Write(append(header(6, infoLen), make([]byte, infoLen)...))
			}
		}
	}
}

func TestOlderZmx(t *testing.T) {
	t.Run("it recognises a zmx 0.8.1 daemon by its answer to Info in its own framing, past its output", func(t *testing.T) {
		dir := socketDir(t)
		d := serve(t, dir, "old", olderDaemon(552, []byte("prompt$ "), make([]byte, 300)))

		older, err := newBackend(t, dir).OlderZmx(t.Context(), "old")
		if err != nil || !older {
			t.Fatalf("OlderZmx = %v, %v; want true", older, err)
		}
		if reqs := d.received(); len(reqs) != 1 || reqs[0].tag != 6 || len(reqs[0].payload) != 0 {
			t.Errorf("the daemon received %+v, want one Info with no payload", reqs)
		}
	})

	t.Run("it does not take an Info record of another size for 0.8.1's", func(t *testing.T) {
		dir := socketDir(t)
		serve(t, dir, "odd", olderDaemon(560))

		if older, err := newBackend(t, dir).OlderZmx(t.Context(), "odd"); err != nil || older {
			t.Errorf("OlderZmx = %v, %v; want false", older, err)
		}
	})

	t.Run("it does not take an answer in this protocol's framing for 0.8.1's", func(t *testing.T) {
		dir := socketDir(t)
		d := serve(t, dir, "work", answering(infoFrame(t)))

		if older, err := newBackend(t, dir).OlderZmx(t.Context(), "work"); err != nil || older {
			t.Errorf("OlderZmx = %v, %v; want false", older, err)
		}
		if reqs := d.received(); len(reqs) != 1 || reqs[0].version != 0 {
			t.Errorf("the daemon received %+v, want one request of protocol 0", reqs)
		}
	})

	t.Run("a daemon that answers neither is not answering", func(t *testing.T) {
		dir := socketDir(t)
		serve(t, dir, "wedged", silent)

		if older, err := newBackend(t, dir).OlderZmx(t.Context(), "wedged"); older || !errors.Is(err, backend.ErrTimeout) {
			t.Errorf("OlderZmx = %v, %v; want ErrTimeout", older, err)
		}
	})

	t.Run("a session that isn't there is ErrNoSuchSession", func(t *testing.T) {
		if older, err := newBackend(t, socketDir(t)).OlderZmx(t.Context(), "gone"); older || !errors.Is(err, backend.ErrNoSuchSession) {
			t.Errorf("OlderZmx = %v, %v; want ErrNoSuchSession", older, err)
		}
	})

	t.Run("Portal's protocol gets no answer from a 0.8.1 daemon, which reads the request as one awaiting payload", func(t *testing.T) {
		dir := socketDir(t)
		serve(t, dir, "old", olderDaemon(552))

		if got, err := newBackend(t, dir).ProtocolVersion(t.Context(), "old"); !errors.Is(err, backend.ErrTimeout) {
			t.Errorf("ProtocolVersion = %d, %v; want ErrTimeout", got, err)
		}
	})
}

func TestKillByCLI(t *testing.T) {
	t.Run("it runs zmx kill on the name, bound on the directory, out of the caller's session", func(t *testing.T) {
		binary, log := fakeZmx(t, "")
		zmxDir := socketDir(t)
		b := newBackendRunning(t, binary, zmxDir,
			"PATH=/usr/bin:/bin", "ZMX_DIR=/Users/lee/.local/state/zmx", "ZMX_SESSION=live", "ZMX_SESSION_PREFIX=pre-",
			"TMUX=/tmp/tmux-501/default,1,0", "TMUX_PANE=%1")
		zmx.ShortenCeilings(b, zmx.CreateCeiling)

		if err := b.KillByCLI(t.Context(), "_portal-saver"); err != nil {
			t.Fatalf("KillByCLI = %v, want nil", err)
		}

		if got, want := strings.Split(strings.TrimSuffix(string(ran(t, log, "args")), "\x00"), "\x00"), []string{"kill", "_portal-saver"}; !slices.Equal(got, want) {
			t.Errorf("zmx args = %q, want %q", got, want)
		}
		env := strings.Split(string(ran(t, log, "env")), "\x00")
		for key, want := range map[string][]string{
			"ZMX_DIR":            {zmxDir},
			"ZMX_SESSION":        nil,
			"ZMX_SESSION_PREFIX": nil,
			"TMUX":               nil,
			"TMUX_PANE":          nil,
		} {
			if got := envValues(env, key); !slices.Equal(got, want) {
				t.Errorf("zmx env %s = %q, want %q", key, got, want)
			}
		}
	})

	t.Run("it refuses, running nothing, a name zmx kill reads as a flag or a prefix of many", func(t *testing.T) {
		for _, name := range []string{"-h", "--force", "work*", ""} {
			binary, log := fakeZmx(t, "")
			if err := newBackendRunning(t, binary, socketDir(t)).KillByCLI(t.Context(), name); !errors.Is(err, backend.ErrInvalidName) {
				t.Errorf("KillByCLI(%q) = %v, want ErrInvalidName", name, err)
			}
			notRun(t, log)
		}
	})

	t.Run("a run that fails is an error carrying what zmx printed", func(t *testing.T) {
		binary, _ := fakeZmx(t, "echo 'error: SessionNotFound' >&2; exit 1")

		b := newBackendRunning(t, binary, socketDir(t), "PATH=/usr/bin:/bin")
		zmx.ShortenCeilings(b, zmx.CreateCeiling)

		err := b.KillByCLI(t.Context(), "_portal-saver")

		if err == nil || !strings.Contains(err.Error(), "error: SessionNotFound") {
			t.Errorf("KillByCLI = %v, want zmx's failure with its output", err)
		}
	})

	t.Run("a run exiting 0 with the session's socket still there is an error carrying what zmx printed", func(t *testing.T) {
		dir := socketDir(t)
		serve(t, dir, "_portal-saver", olderDaemon(552))
		binary, _ := fakeZmx(t, "echo 'failed to kill session=_portal-saver: SessionUnresponsive' >&2")
		b := newBackendRunning(t, binary, dir, "PATH=/usr/bin:/bin")
		zmx.ShortenCeilings(b, zmx.CreateCeiling)

		err := b.KillByCLI(t.Context(), "_portal-saver")

		if err == nil || !strings.Contains(err.Error(), "failed to kill session=_portal-saver: SessionUnresponsive") {
			t.Errorf("KillByCLI = %v, want the socket left reported with zmx's output", err)
		}
	})

	t.Run("a run past its ceiling is a timeout", func(t *testing.T) {
		binary, _ := fakeZmx(t, "exec sleep 5")
		b := newBackendRunning(t, binary, socketDir(t), "PATH=/usr/bin:/bin")

		if err := b.KillByCLI(t.Context(), "_portal-saver"); !errors.Is(err, backend.ErrTimeout) {
			t.Errorf("KillByCLI = %v, want ErrTimeout", err)
		}
	})
}
