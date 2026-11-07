package runnerreg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/nanoid"
	"github.com/leeovery/portal/internal/runner"
)

// ErrSocketPathTooLong refuses a control socket whose path a unix socket's
// address can't hold, rather than cut it short.
var ErrSocketPathTooLong = errors.New("the socket's path is too long")

// maxSocketPath is the longest path a unix socket's address holds: sun_path
// holds it and the NUL that ends it, 104 bytes on macOS.
var maxSocketPath = len(syscall.RawSockaddrUnix{}.Path) - 1

func checkSocketPath(path string) error {
	if len(path) > maxSocketPath {
		return fmt.Errorf("%w: %s is %d bytes, over the %d a unix socket's address holds", ErrSocketPathTooLong, path, len(path), maxSocketPath)
	}
	return nil
}

// acceptBackoffMin and acceptBackoffMax bound the wait before serve takes a
// connection again after its socket fails to take one.
const (
	acceptBackoffMin = 5 * time.Millisecond
	acceptBackoffMax = time.Second
)

// Handler carries out the requests sent to a registered runner, each by ctx's
// deadline, the caller's.
type Handler interface {
	// Switch asks the runner to end its attach and attach session next. It
	// returns once the runner has accepted, or with why it won't.
	Switch(ctx context.Context, session string) error
	// DetachToPicker asks the runner to end its attach for the picker. It
	// returns once the runner has accepted, or with why it won't.
	DetachToPicker(ctx context.Context) error
	// ConfirmSession asks the runner whether its attach under way shows
	// session, sure of it or confirmed so, ending nothing. It returns nil if
	// so, or why not.
	ConfirmSession(ctx context.Context, session string) error
}

// Registration is this process's entry in the registry, its socket served
// until Close.
type Registration struct {
	registry Registry
	token    string
	handler  Handler
	listener net.Listener
	ctx      context.Context
	cancel   context.CancelFunc
	served   sync.WaitGroup

	mu      sync.Mutex
	session string
	closed  bool
}

// Register lists this process under token as a runner attached to session,
// serving the requests sent to its socket through h until Close, once it has
// swept the registry of runners scan proves stale. The registry's
// directory is made 0700 and the socket 0600. A token that could not have
// been minted for a runner is refused, and a socket path too long for a unix
// socket's address with ErrSocketPathTooLong, making nothing.
func (r Registry) Register(token, session string, h Handler) (*Registration, error) {
	if !nanoid.IsIDShaped(token) {
		return nil, fmt.Errorf("register runner: token %q is no runner's", token)
	}
	r.sweep()
	g := &Registration{registry: r, token: token, handler: h, session: session}
	if err := g.listen(); err != nil {
		return nil, fmt.Errorf("register runner: %w", err)
	}
	if err := g.write(session); err != nil {
		_ = g.listener.Close()
		return nil, fmt.Errorf("register runner: %w", err)
	}
	g.start()
	logger.Debug("listed", "session", session, "path", r.entryPath(token))
	return g, nil
}

// start serves the socket until Close.
func (g *Registration) start() {
	g.ctx, g.cancel = context.WithCancel(context.Background())
	g.served.Go(g.serve)
}

// SetSession moves the entry to session, as the runner switches to it.
func (g *Registration) SetSession(session string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return fmt.Errorf("move runner to session %q: it is no longer registered", session)
	}
	if err := g.write(session); err != nil {
		return fmt.Errorf("move runner to session %q: %w", session, err)
	}
	g.session = session
	logger.Debug("listed", "session", session, "path", g.registry.entryPath(g.token))
	return nil
}

// Close removes the entry, so no one finds the runner, then stops serving its
// socket and removes it, once each request being served is answered.
func (g *Registration) Close() error {
	g.mu.Lock()
	if g.closed {
		g.mu.Unlock()
		return nil
	}
	g.closed = true
	session := g.session
	g.mu.Unlock()

	path := g.registry.entryPath(g.token)
	err := os.Remove(path)
	g.cancel()
	_ = g.listener.Close()
	g.served.Wait()
	logger.Debug("unlisted", "session", session, "path", path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("unregister runner: %w", err)
	}
	return nil
}

// listen binds the socket, which only this user can reach, making the
// registry's directory first.
func (g *Registration) listen() error {
	path := g.registry.socketPath(g.token)
	if err := checkSocketPath(path); err != nil {
		return err
	}
	if err := os.MkdirAll(g.registry.dir, 0o700); err != nil {
		return err
	}
	// MkdirAll leaves a directory that already exists as it is.
	if err := os.Chmod(g.registry.dir, 0o700); err != nil {
		return err
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = ln.Close()
		return err
	}
	g.listener = ln
	return nil
}

// write writes the entry naming this process on session, in the current
// boot. A write lands once its rename does.
func (g *Registration) write(session string) error {
	data, err := json.Marshal(entry{PID: os.Getpid(), Session: session, BootID: currentBoot()})
	if err != nil {
		return err
	}
	path := g.registry.entryPath(g.token)
	return fileutil.AtomicWrite0600(path, data, fileutil.ReportUnsynced(func(err error) {
		logger.Warn("unsynced", "op", "unsynced", "path", path, "error", err, "error_class", fileutil.ClassifyWriteError(err))
	}))
}

// serve answers each connection to the socket until Close. A connection the
// socket fails to take, as with no descriptors free, is tried again after a
// backoff that doubles from acceptBackoffMin to acceptBackoffMax, and logged
// once for each run of failures; the entry stays listed throughout.
func (g *Registration) serve() {
	var backoff time.Duration
	for {
		conn, err := g.listener.Accept()
		switch {
		case err == nil:
			backoff = 0
			g.served.Go(func() { g.answer(conn) })
			continue
		case errors.Is(err, net.ErrClosed):
			return
		case backoff == 0:
			logger.Warn("serve failed", "session", g.currentSession(), "error", err)
		}
		backoff = min(max(2*backoff, acceptBackoffMin), acceptBackoffMax)
		select {
		case <-g.ctx.Done():
			return
		case <-time.After(backoff):
		}
	}
}

// answer answers the one request a connection carries, which must be read
// within the bound a caller gives one. Close cuts it short.
func (g *Registration) answer(nc net.Conn) {
	conn := &cuttable{Conn: nc}
	defer func() { _ = conn.Close() }()
	stop := context.AfterFunc(g.ctx, conn.cut)
	defer stop()
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	q, err := readRequest(conn)
	if err == nil {
		err = g.carryOut(q)
	}
	g.logRequest(q, err)
	a := answer{Version: Version, OK: err == nil, PID: os.Getpid()}
	if err != nil {
		a.Refusal = err.Error()
	}
	_ = conn.SetWriteDeadline(time.Now().Add(timeout))
	_ = writeMessage(conn, a)
}

// cuttable is a connection that cut ends at once: its deadlines pass, and no
// deadline set before or after the cut moves them back. Close's cut may run
// before the deadlines answer sets, as it does on a context already ended.
type cuttable struct {
	net.Conn
	mu    sync.Mutex
	isCut bool
}

func (c *cuttable) cut() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.isCut = true
	_ = c.Conn.SetDeadline(time.Unix(1, 0))
}

func (c *cuttable) SetDeadline(t time.Time) error {
	return c.set(c.Conn.SetDeadline, t)
}

func (c *cuttable) SetReadDeadline(t time.Time) error {
	return c.set(c.Conn.SetReadDeadline, t)
}

func (c *cuttable) SetWriteDeadline(t time.Time) error {
	return c.set(c.Conn.SetWriteDeadline, t)
}

func (c *cuttable) set(deadline func(time.Time) error, t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.isCut {
		return nil
	}
	return deadline(t)
}

// carryOut has the handler carry out q by its deadline, so a request the
// runner takes is one its caller is still waiting to hear of.
func (g *Registration) carryOut(q request) error {
	if !time.Now().Before(q.Deadline) {
		return runner.ErrPastDeadline
	}
	ctx, cancel := context.WithDeadline(g.ctx, q.Deadline)
	defer cancel()
	switch q.Request {
	case reqSwitch:
		return g.handler.Switch(ctx, q.Session)
	case reqConfirmSession:
		return g.handler.ConfirmSession(ctx, q.Session)
	}
	return g.handler.DetachToPicker(ctx)
}

func (g *Registration) logRequest(q request, refusal error) {
	result := "accepted"
	if refusal != nil {
		result = "refused"
	}
	attrs := []any{"session", g.currentSession(), "result", result}
	if q.Request != "" {
		attrs = append(attrs, "detail", q.Request)
	}
	if q.Session != "" {
		attrs = append(attrs, "target", q.Session)
	}
	if refusal != nil {
		attrs = append(attrs, "error", refusal)
	}
	logger.Debug("request", attrs...)
}

func (g *Registration) currentSession() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.session
}
