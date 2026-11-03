package zmxtest

import (
	"io"
	"os/exec"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
)

// Client is a zmx attach client with no terminal. It stays attached until zmx
// detaches it or the test ends.
type Client struct {
	stdin io.WriteCloser
	done  chan error
}

// Attach starts a client on the session running the Backend's AttachCommand.
func Attach(t *testing.T, b *zmx.Backend, name string) *Client {
	t.Helper()
	attach, err := b.AttachCommand(t.Context(), name)
	if err != nil {
		t.Fatalf("attach command for %s: %v", name, err)
	}
	return Run(t, attach)
}

// Run starts a client running attach, an attach command, holding its stdin
// open: zmx detaches a client whose stdin reaches EOF.
func Run(t *testing.T, attach backend.Command) *Client {
	t.Helper()
	cmd := exec.Command(attach.Argv[0], attach.Argv[1:]...)
	cmd.Env = attach.Env
	cmd.Stdout = io.Discard
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("%q: %v", attach.Argv, err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("%q: %v", attach.Argv, err)
	}
	c := &Client{stdin: stdin, done: make(chan error, 1)}
	go func() { c.done <- cmd.Wait() }()
	t.Cleanup(func() {
		_ = stdin.Close()
		if !c.Exited(settle) {
			_ = cmd.Process.Kill()
			t.Errorf("%q outlived its stdin", attach.Argv)
		}
	})
	return c
}

// Type sends input to the session through the client, as typing in its
// terminal would.
func (c *Client) Type(t *testing.T, input string) {
	t.Helper()
	if _, err := io.WriteString(c.stdin, input); err != nil {
		t.Fatalf("type %q: %v", input, err)
	}
}

// Exited reports whether the client exited within timeout.
func (c *Client) Exited(timeout time.Duration) bool {
	select {
	case err := <-c.done:
		c.done <- err
		return true
	case <-time.After(timeout):
		return false
	}
}
