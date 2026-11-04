package runnerreg

import (
	"net"
	"testing"
	"time"
)

// SetTimeout bounds each request at d for the rest of t, so a test of one
// that goes unanswered need not wait out the real bound.
func SetTimeout(t testing.TB, d time.Duration) {
	t.Helper()
	old := timeout
	timeout = d
	t.Cleanup(func() { timeout = old })
}

// SetRunning has the registry judge whether a pid is running by f for the
// rest of t, f given the real judgement to defer to.
func SetRunning(t testing.TB, f func(pid int, running func(int) bool) bool) {
	t.Helper()
	old := running
	running = func(pid int) bool { return f(pid, processRunning) }
	t.Cleanup(func() { running = old })
}

// Timeout and AnswerGrace are the bound of a request and how long past its
// deadline a caller waits for the answer.
func Timeout() time.Duration { return timeout }

const AnswerGrace = answerGrace

// MaxSocketPath is the longest socket path the registry binds.
func MaxSocketPath() int { return maxSocketPath }

// Entry is the path of the registration's entry.
func (g *Registration) Entry() string { return g.registry.entryPath(g.token) }

// Socket is the path of the registration's socket.
func (g *Registration) Socket() string { return g.registry.socketPath(g.token) }

// EntryPath and SocketPath are where the entry named token and its socket
// live.
func (r Registry) EntryPath(token string) string  { return r.entryPath(token) }
func (r Registry) SocketPath(token string) string { return r.socketPath(token) }

// ServeThrough serves a registration of token on session through ln, as
// Register does once it has bound the socket, until Close.
func (r Registry) ServeThrough(token, session string, h Handler, ln net.Listener) *Registration {
	g := &Registration{registry: r, token: token, handler: h, session: session, listener: ln}
	g.start()
	return g
}
