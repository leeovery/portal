package zmx_test

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"testing"

	"github.com/leeovery/portal/internal/backend"
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
