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

// ErrRefusing marks a session whose socket refuses connections while a
// process may still hold it: on macOS a live daemon whose listen backlog is
// full refuses as a dead one's socket does. It wraps backend.ErrTimeout: the
// session is held but not answering.
var ErrRefusing = fmt.Errorf("zmx session refuses connections: %w", errNotAccepting)

var (
	errUnsupported = errors.New("zmx daemon does not support the request")
	errSecondFrame = errors.New("a second frame on one connection to a zmx daemon")
	// errStale marks a refused connect to a socket no daemon is behind.
	errStale = errors.New("no daemon holds the socket")
	// errNotAccepting marks a connect a daemon may be behind but did not
	// accept: refused while a process may hold the socket (ErrRefusing), or a
	// full backlog.
	errNotAccepting = fmt.Errorf("zmx daemon is not accepting connections: %w", backend.ErrTimeout)
	errNoAnswer     = fmt.Errorf("zmx daemon gave no well-formed answer: %w", backend.ErrTimeout)
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

// answerVersion reads frames until the daemon's answer to Info, Info or
// Unsupported, and returns the protocol version its header carries. Frames of
// other tags are the session's broadcast output, skipped as await skips them,
// but only in this protocol: the length of a frame in any other cannot be
// trusted, and a zmx 0.8.1 daemon's output frame carries its length in bytes
// 1-4, so its byte 1 is no version at all.
func (c *conn) answerVersion() (uint8, error) {
	for {
		h, err := c.header()
		answer := h.tag == tagInfo || h.tag == tagUnsupported
		switch {
		case errors.Is(err, errProtocolVersion) && answer:
			return h.version, nil
		case errors.Is(err, errProtocolVersion):
			return 0, fmt.Errorf("%w: a tag %d frame of protocol %d before the answer", errFrame, h.tag, h.version)
		case err != nil:
			return 0, err
		case answer:
			return h.version, nil
		}
		if _, err := c.r.Discard(int(h.len)); err != nil {
			return 0, err
		}
	}
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
		return nil, b.dialFailure(ctx, entry, err)
	}
	return c, nil
}

// dialFailure marks a failed connect as a stale socket or a daemon not
// accepting. On Linux a connect to a unix socket whose listen backlog is full
// fails with EAGAIN, which proves a listener, so it is never stale; on macOS
// a full backlog refuses.
func (b *Backend) dialFailure(ctx context.Context, entry string, err error) error {
	switch {
	case errors.Is(err, syscall.EAGAIN):
		return fmt.Errorf("%w: %w", err, errNotAccepting)
	case errors.Is(err, syscall.ECONNREFUSED):
		return b.refused(ctx, entry, err)
	}
	return err
}

// refused tells a stale socket from a session not answering. On Linux a
// refusal proves no listener, a full backlog failing with EAGAIN instead, so
// the socket is stale. On macOS a daemon whose listen backlog is full refuses
// as a dead one does, so a refusal alone is no proof the session has gone. A
// socket made before this boot, whose daemon cannot be running now, is stale;
// one made since is stale only once the process table proves no process holds
// it, a read paid for on a refusal alone.
func (b *Backend) refused(ctx context.Context, entry string, err error) error {
	if refusalProvesNoListener {
		return fmt.Errorf("%w: %w", err, errStale)
	}
	info, statErr := os.Stat(b.path(entry))
	boot, bootErr := b.booted()
	if statErr == nil && bootErr == nil && info.ModTime().Before(boot) {
		return fmt.Errorf("%w: %w", err, errStale)
	}
	if held := b.unheld(ctx, entry); held != nil {
		return fmt.Errorf("%w: %w: %w", err, ErrRefusing, held)
	}
	return fmt.Errorf("%w: %w", err, errStale)
}

// gone reports whether a connect found no session: the socket is missing, or
// stale.
func gone(err error) bool {
	return errors.Is(err, fs.ErrNotExist) || errors.Is(err, errStale)
}

// failure is err, or the context's cause when the context's end is what
// failed the I/O: a dial on an ended context, or a read or write cut by the
// deadline dial sets once the context ends. An I/O that failed on its own
// keeps its error, even if the context ended after.
func failure(ctx context.Context, err error) error {
	if endedIO(err) && ctx.Err() != nil {
		return context.Cause(ctx)
	}
	return err
}

func endedIO(err error) bool {
	return errors.Is(err, os.ErrDeadlineExceeded) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// noAnswer explains a request on the socket s that got no well-formed
// answer: the context's cause when its end failed the I/O; an answer in
// another protocol version as itself; ErrNoSuchSession when the socket has
// gone since; and otherwise a daemon that hung up or answered garbled, which
// is not answering.
func (b *Backend) noAnswer(ctx context.Context, s socket, err error) error {
	switch {
	case endedIO(err) && ctx.Err() != nil:
		return context.Cause(ctx)
	case errors.Is(err, errProtocolVersion):
		return err
	case !b.stillHeld(s):
		return fmt.Errorf("%w: %w", err, backend.ErrNoSuchSession)
	}
	return fmt.Errorf("%w: %w", err, errNoAnswer)
}

// callerEnded reports whether err, as failure reports it, is the end of the
// caller's own context, not of a ceiling beneath it. Each must match the
// other: a ceiling's cause wraps ErrTimeout, which a caller's cause may be.
func callerEnded(caller context.Context, err error) bool {
	cause := context.Cause(caller)
	return cause != nil && errors.Is(err, cause) && errors.Is(cause, err)
}
