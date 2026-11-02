package zmx

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"syscall"
	"time"

	"github.com/leeovery/portal/internal/backend"
)

var (
	errUnsupported = errors.New("zmx daemon does not support the request")
	// errStale marks a refused connect to a socket made before this boot.
	errStale = errors.New("socket predates this boot")
	// errRefused marks a refused connect to a socket made since boot.
	errRefused = fmt.Errorf("zmx daemon is not accepting connections: %w", backend.ErrTimeout)
)

// conn is one connection to a session's daemon. Its reads and writes fail
// once the context it was dialled with ends.
type conn struct {
	nc   net.Conn
	r    *bufio.Reader
	stop func() bool
}

func dial(ctx context.Context, path string) (*conn, error) {
	var d net.Dialer
	nc, err := d.DialContext(ctx, "unix", path)
	if err != nil {
		return nil, err
	}
	// A deadline in the past fails a blocked read or write at once.
	stop := context.AfterFunc(ctx, func() { _ = nc.SetDeadline(time.Unix(1, 0)) })
	return &conn{nc: nc, r: bufio.NewReader(nc), stop: stop}, nil
}

func (c *conn) Close() error {
	c.stop()
	return c.nc.Close()
}

func (c *conn) send(t tag, payload []byte) error {
	_, err := c.nc.Write(append(encodeHeader(t, len(payload)), payload...))
	return err
}

// header reads the next frame's header. On a header of another protocol
// version it returns the header with the error, so its version can be told.
func (c *conn) header() (header, error) {
	b := make([]byte, headerSize)
	if _, err := io.ReadFull(c.r, b); err != nil {
		return header{}, err
	}
	return decodeHeader(b)
}

// await reads frames until one tagged want and returns its payload. The
// daemon broadcasts the session's output to every connection, so frames of
// other tags are skipped.
func (c *conn) await(want tag) ([]byte, error) {
	for {
		h, err := c.header()
		if err != nil {
			return nil, err
		}
		switch h.tag {
		case want:
			p := make([]byte, h.len)
			if _, err := io.ReadFull(c.r, p); err != nil {
				return nil, err
			}
			return p, nil
		case tagUnsupported:
			return nil, errUnsupported
		}
		if _, err := c.r.Discard(int(h.len)); err != nil {
			return nil, err
		}
	}
}

// hangupAfter sends a request the daemon answers only by closing the
// connection, and reads, discarding, until it does.
func (c *conn) hangupAfter(t tag) error {
	if err := c.send(t, nil); err != nil {
		return err
	}
	_, err := io.Copy(io.Discard, c.r)
	return err
}

// connect dials the socket of a directory entry.
func (b *Backend) connect(ctx context.Context, entry string) (*conn, error) {
	c, err := dial(ctx, b.path(entry))
	if errors.Is(err, syscall.ECONNREFUSED) {
		return nil, b.refused(entry, err)
	}
	return c, err
}

// refused tells a stale socket from a session not answering. A daemon whose
// listen backlog is full refuses as a dead one does, so a refusal alone is no
// proof the session has gone: only a socket made before this boot, whose
// daemon cannot be running now, is stale. Without the boot time, none is.
func (b *Backend) refused(entry string, err error) error {
	info, statErr := os.Stat(b.path(entry))
	boot, bootErr := b.booted()
	if statErr == nil && bootErr == nil && info.ModTime().Before(boot) {
		return fmt.Errorf("%w: %w", err, errStale)
	}
	return fmt.Errorf("%w: %w", err, errRefused)
}

// gone reports whether a connect found no session: the socket is missing, or
// stale.
func gone(err error) bool {
	return errors.Is(err, fs.ErrNotExist) || errors.Is(err, errStale)
}

// failure is err, or the context's cause when the context has ended, since
// an ended context is why the I/O failed.
func failure(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return context.Cause(ctx)
	}
	return err
}
