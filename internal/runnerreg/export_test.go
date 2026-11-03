package runnerreg

import (
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

// Socket is the path the runner was found at.
func (rn Runner) Socket() string { return rn.socket }
