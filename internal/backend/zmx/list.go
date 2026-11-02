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

// errProbeCeiling ends the probes of a List that have not answered by its
// ceiling.
var errProbeCeiling = errors.New("zmx session did not answer within the listing's ceiling")

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
	probeCtx, cancel := context.WithTimeoutCause(ctx, b.limits.probe, errProbeCeiling)
	defer cancel()
	probes := make([]probed, len(names))
	slots := make(chan struct{}, maxProbes)
	var wg sync.WaitGroup
	for i, name := range names {
		wg.Go(func() {
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
				probes[i] = b.probe(probeCtx, name, hostname)
			case <-probeCtx.Done():
				probes[i] = unanswered(probeCtx, name)
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
// stale socket, with only its name known.
func (b *Backend) probe(ctx context.Context, name, hostname string) probed {
	i, err := b.readInfo(ctx, name)
	switch {
	case gone(err):
		return probed{gone: true}
	case err != nil:
		return unanswered(ctx, name)
	}
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
	return probed{session: s}
}

// unanswered is a session that gave no answer, cut short when its context
// ended for any reason but the listing's ceiling.
func unanswered(ctx context.Context, name string) probed {
	cut := ctx.Err() != nil && !errors.Is(context.Cause(ctx), errProbeCeiling)
	return probed{session: backend.Session{Name: name}, cut: cut}
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
