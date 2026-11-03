package zmx

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"sync"

	"github.com/leeovery/portal/internal/backend"
)

// maxProbes caps the probes in flight, each holding a socket open, so a
// directory of many sessions cannot exhaust Portal's file descriptors.
const maxProbes = 64

// List probes every session at once, under one probe ceiling for the whole
// listing, so sessions that do not answer cost one ceiling between them, not
// one each, however many wait for a slot. A session not answered by then is
// listed with only its name. The listing fails with the caller's cause only
// when the caller's context ends before every probe is done. A stale socket is
// left out, and left in place: zmx replaces it, and Portal never deletes a
// socket.
func (b *Backend) List(ctx context.Context) ([]backend.Session, error) {
	names, err := b.sockets()
	if err != nil {
		return nil, fmt.Errorf("list zmx sessions: %w", err)
	}
	// Without a hostname only a cwd with no host, or localhost, reads as
	// local; every other field is unaffected.
	hostname, _ := b.hostname()
	probeCtx, cancel := b.bound(ctx, b.limits.probe, "")
	defer cancel()
	probes := make([]probed, len(names))
	slots := make(chan struct{}, maxProbes)
	var wg sync.WaitGroup
	for i, name := range names {
		wg.Go(func() {
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
				probes[i] = b.probe(ctx, probeCtx, name, hostname)
			case <-probeCtx.Done():
				probes[i] = unanswered(ctx, name, context.Cause(probeCtx))
			}
		})
	}
	wg.Wait()
	sessions := []backend.Session{}
	for _, p := range probes {
		if p.cut {
			return nil, fmt.Errorf("list zmx sessions: %w", context.Cause(ctx))
		}
		if !p.gone {
			sessions = append(sessions, p.session)
		}
	}
	return sessions, nil
}

// sockets names the directory's sessions as zmx finds them: every entry whose
// stat, following symlinks, is a unix socket with a path short enough to
// connect to, in name order. zmx makes the directory with its first session,
// so a missing one holds none.
func (b *Backend) sockets() ([]string, error) {
	entries, err := os.ReadDir(b.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		name := e.Name()
		if len(name) > b.MaxNameLen() {
			continue
		}
		if info, err := os.Stat(b.path(name)); err == nil && info.Mode().Type() == fs.ModeSocket {
			names = append(names, name)
		}
	}
	return names, nil
}

// probed is one socket's answer: its session, or gone when the socket is
// missing or stale, or cut when the caller's context ended before the answer.
type probed struct {
	session backend.Session
	gone    bool
	cut     bool
}

// probe keeps a session that fails to answer, for any reason but a missing or
// stale socket, with only its name known. ctx carries the listing's ceiling
// beneath the caller's context.
func (b *Backend) probe(caller, ctx context.Context, name, hostname string) probed {
	i, err := b.readInfo(ctx, name)
	switch {
	case gone(err):
		return probed{gone: true}
	case err != nil:
		return unanswered(caller, name, err)
	}
	return probed{session: sessionOf(name, i, hostname)}
}

// sessionOf is the session that answered i, on the machine named hostname.
func sessionOf(name string, i info, hostname string) backend.Session {
	s := backend.Session{
		Name:     name,
		Answered: true,
		Clients:  int(min(i.clients, math.MaxInt)),
		PID:      int(i.pid),
		Created:  i.createdAt(),
		Command:  i.cmd,
	}
	if !i.cwdTruncated() {
		s.Dir = localDir(i.cwd, hostname)
	}
	return s
}

// unanswered is a session that gave no answer, failing with err: cut short
// when err is the end of the caller's context.
func unanswered(caller context.Context, name string, err error) probed {
	return probed{session: backend.Session{Name: name}, cut: callerEnded(caller, err)}
}

// readInfo asks the daemon behind name's socket for its Info record. The
// daemon counts clients without the asker.
func (b *Backend) readInfo(ctx context.Context, name string) (info, error) {
	c, err := b.connect(ctx, name)
	if err != nil {
		return info{}, failure(ctx, err)
	}
	defer func() { _ = c.Close() }()
	i, err := c.askInfo()
	if err != nil {
		return info{}, failure(ctx, err)
	}
	return i, nil
}
