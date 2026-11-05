package zmx

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// zmx 0.8.1's ipc.Header is a packed struct of tag u8 and len u32: eight
// bytes on the wire, the tag in byte 0 and the length in bytes 1-4. Its Info
// record is 552 bytes, as this protocol's is. Portal speaks that framing only
// to recognise a daemon of that zmx, which it refuses, never to drive one.
var olderInfoRequest = []byte{byte(tagInfo), 0, 0, 0, 0, 0, 0, 0}

func olderLen(header []byte) uint32 { return binary.NativeEndian.Uint32(header[1:5]) }

// OlderZmx reports whether the daemon of the live session name answers zmx
// 0.8.1's framing: Info framed as 0.8.1 frames it, answered with 0.8.1's
// Info record, within the probe ceiling, on a connection of its own. A daemon
// of this protocol reads that request as a frame of protocol 0, which it
// discards, answering nothing, so asking it is safe.
func (b *Backend) OlderZmx(ctx context.Context, name string) (bool, error) {
	ctx, cancel := b.bound(ctx, b.limits.probe, "")
	defer cancel()
	c, s, err := b.open(ctx, name)
	if err != nil {
		return false, fmt.Errorf("ask session %q in zmx 0.8.1's framing: %w", name, err)
	}
	defer func() { _ = c.Close() }()
	older, err := c.answersOlder()
	if err != nil {
		return false, fmt.Errorf("ask session %q in zmx 0.8.1's framing: %w", name, b.noAnswer(ctx, s, err))
	}
	return older, nil
}

// answersOlder sends Info in zmx 0.8.1's framing and reports whether the
// daemon answers as 0.8.1 does, with an Info frame of its record's size.
// Frames of other tags, read in that framing, are the session's broadcast
// output, skipped.
func (c *conn) answersOlder() (bool, error) {
	if c.sent {
		return false, errSecondFrame
	}
	c.sent = true
	if _, err := c.nc.Write(olderInfoRequest); err != nil {
		return false, err
	}
	header := make([]byte, headerSize)
	for {
		if _, err := io.ReadFull(c.r, header); err != nil {
			return false, err
		}
		n := olderLen(header)
		switch {
		case tag(header[0]) == tagInfo:
			return n == infoSize, nil
		case n > maxPayload:
			return false, fmt.Errorf("%w: %d-byte payload", errFrame, n)
		}
		if _, err := c.r.Discard(int(n)); err != nil {
			return false, err
		}
	}
}

// KillByCLI ends the session name with zmx's own kill, run in the Backend's
// directory within the kill ceiling. zmx speaks whatever protocol its own
// daemons do, so it ends a session of a zmx Portal can't speak to, and it,
// not Portal, signals the session's processes. It refuses a name Create
// refuses, as zmx kill would read some of those as its flags or as a prefix
// of many names.
func (b *Backend) KillByCLI(ctx context.Context, name string) error {
	if err := b.ValidateName(name); err != nil {
		return fmt.Errorf("zmx kill %q: %w", name, err)
	}
	ctx, cancel := b.bound(ctx, b.limits.kill, "; the kill may still land")
	defer cancel()
	cmd := exec.CommandContext(ctx, b.binary, "kill", name)
	cmd.Env = b.env(envSession, envPrefix)
	cmd.WaitDelay = pipeGrace
	out, err := cmd.CombinedOutput()
	switch {
	case err == nil:
		return b.killed(name, out)
	case ctx.Err() != nil:
		return fmt.Errorf("zmx kill %q: %w", name, context.Cause(ctx))
	}
	if msg := strings.TrimSpace(string(out)); msg != "" {
		return fmt.Errorf("zmx kill %q: %s: %w", name, msg, err)
	}
	return fmt.Errorf("zmx kill %q: %w", name, err)
}

// killed confirms a zmx kill that exited 0 took name's socket with it. zmx
// kill exits 0 though a session's kill fails, printing why, and a kill that
// lands waits for the daemon to hang up, which unlinks the socket first.
func (b *Backend) killed(name string, out []byte) error {
	_, held, err := b.held(name)
	switch {
	case err != nil:
		return fmt.Errorf("zmx kill %q: look for its socket after the kill: %w", name, err)
	case held:
		return fmt.Errorf("zmx kill %q exited 0, its socket still there: %s", name, strings.TrimSpace(string(out)))
	}
	return nil
}
