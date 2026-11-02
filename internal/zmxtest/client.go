package zmxtest

import (
	"io"
	"os/exec"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend/zmx"
)

// Client is a zmx attach client with no terminal, running the Backend's
// AttachCommand. It stays attached until zmx detaches it or the test ends.
type Client struct {
	done chan error
}

// Attach starts a client on the session, holding its stdin open: zmx detaches
// a client whose stdin reaches EOF.
func Attach(t *testing.T, b *zmx.Backend, name string) *Client {
	t.Helper()
	attach, err := b.AttachCommand(name)
	if err != nil {
		t.Fatalf("attach command for %s: %v", name, err)
	}
	cmd := exec.Command(attach.Argv[0], attach.Argv[1:]...)
	cmd.Env = attach.Env
	cmd.Stdout = io.Discard
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("zmx attach %s: %v", name, err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("zmx attach %s: %v", name, err)
	}
	c := &Client{done: make(chan error, 1)}
	go func() { c.done <- cmd.Wait() }()
	t.Cleanup(func() {
		_ = stdin.Close()
		if !c.Exited(settle) {
			_ = cmd.Process.Kill()
			t.Errorf("zmx attach %s outlived its stdin", name)
		}
	})
	return c
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
