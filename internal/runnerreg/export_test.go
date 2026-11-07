package runnerreg

import (
	"context"
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

// SetSelf has a take judge pid this process's own for the rest of t.
func SetSelf(t testing.TB, pid int) {
	t.Helper()
	old := selfPID
	selfPID = pid
	t.Cleanup(func() { selfPID = old })
}

// TakePath is where the record of session's last take lives, and
// TakeLockPath the file of the lock its takes hold.
func (r Registry) TakePath(session string) string     { return r.mustTakeFile(session, takeRecordExt) }
func (r Registry) TakeLockPath(session string) string { return r.mustTakeFile(session, takeLockExt) }

func (r Registry) mustTakeFile(session, ext string) string {
	path, err := r.takeFile(session, ext)
	if err != nil {
		panic(err)
	}
	return path
}

// SetBootID has the registry read the current boot's id through read for the
// rest of t.
func SetBootID(t testing.TB, read func() (string, error)) {
	t.Helper()
	old := bootID
	bootID = read
	t.Cleanup(func() { bootID = old })
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

// AnswerClosed answers the request conn carries as a registration answers
// one whose Close began before the answer did.
func AnswerClosed(conn net.Conn, h Handler) {
	g := &Registration{handler: h}
	g.ctx, g.cancel = context.WithCancel(context.Background())
	g.cancel()
	g.answer(conn)
}

// ServeThrough serves a registration of token on session through ln, as
// Register does once it has bound the socket, until Close.
func (r Registry) ServeThrough(token, session string, h Handler, ln net.Listener) *Registration {
	g := &Registration{registry: r, token: token, handler: h, session: session, listener: ln}
	g.start()
	return g
}
