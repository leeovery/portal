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
	errSecondFrame = errors.New("a second frame on one connection to a zmx daemon")
	// errStale marks a refused connect to a socket made before this boot.
	errStale = errors.New("socket predates this boot")
	// errNotAccepting marks a connect a daemon may be behind but did not
	// accept: refused from a socket made since boot, or a full backlog.
	errNotAccepting = fmt.Errorf("zmx daemon is not accepting connections: %w", backend.ErrTimeout)
)

// conn is one connection to a session's daemon. Its reads and writes fail
// once the context it was dialled with ends.
//
// A conn sends one frame, never two, and send refuses a second. A zmx 0.8.1
// daemon, which can share the directory, reads this protocol's 8-byte header
// as its own 8-byte header with the length in bytes 1-4, so every frame Portal
// sends reads to it as one still awaiting payload, and it waits. A second
// frame on the connection would complete the first, and the bytes after it
// would be read as frames of their own, Input among them: keystrokes into the
// session's shell.
type conn struct {
	nc   net.Conn
	r    *bufio.Reader
	stop func() bool
	sent bool
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
	if c.sent {
		return errSecondFrame
	}
	c.sent = true
	_, err := c.nc.Write(append(encodeHeader(t, len(payload)), payload...))
	return err
}

// askInfo sends Info and reads the daemon's record.
func (c *conn) askInfo() (info, error) {
	if err := c.send(tagInfo, nil); err != nil {
		return info{}, err
	}
	p, err := c.await(tagInfo)
	if err != nil {
		return info{}, err
	}
	return decodeInfo(p)
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
	if err != nil {
		return nil, b.dialFailure(entry, err)
	}
	return c, nil
}

// dialFailure marks a failed connect as a stale socket or a daemon not
// accepting. On Linux a connect to a unix socket whose listen backlog is full
// fails with EAGAIN, which proves a listener, so it is never stale; on macOS
// a full backlog refuses.
func (b *Backend) dialFailure(entry string, err error) error {
	switch {
	case errors.Is(err, syscall.EAGAIN):
		return fmt.Errorf("%w: %w", err, errNotAccepting)
	case errors.Is(err, syscall.ECONNREFUSED):
		return b.refused(entry, err)
	}
	return err
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
	return fmt.Errorf("%w: %w", err, errNotAccepting)
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
