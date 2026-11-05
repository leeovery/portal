package zmxtest

import (
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/harnesstest"
	"golang.org/x/sys/unix"
)

// Client is a zmx attach client with no terminal. It stays attached until zmx
// detaches it or the test ends.
type Client struct {
	stdin io.WriteCloser
	// unread is the read end of the client's stdin, held to ask whether it
	// has read all that was typed to it.
	unread *os.File
	done   chan error
}

// Attach starts a client on the session running the Backend's AttachCommand.
// It fails the test, starting nothing, for a directory New didn't make in this
// process.
func Attach(t *testing.T, b *zmx.Backend, name string) *Client {
	t.Helper()
	if err := made.refuse(b.Dir()); err != nil {
		t.Fatal(err)
	}
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
	unread, stdin, err := os.Pipe()
	if err != nil {
		t.Fatalf("%q: %v", attach.Argv, err)
	}
	cmd.Stdin = unread
	if err := cmd.Start(); err != nil {
		_ = unread.Close()
		_ = stdin.Close()
		t.Fatalf("%q: %v", attach.Argv, err)
	}
	c := &Client{stdin: stdin, unread: unread, done: make(chan error, 1)}
	go func() { c.done <- cmd.Wait() }()
	t.Cleanup(func() {
		_ = stdin.Close()
		if !c.Exited(settle) {
			_ = cmd.Process.Kill()
			t.Errorf("%q outlived its stdin", attach.Argv)
		}
		_ = unread.Close()
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

// AwaitRead waits for the client to read everything typed to it, so what is
// typed next reaches it in a read of its own.
func (c *Client) AwaitRead(t *testing.T) {
	t.Helper()
	fds := []unix.PollFd{{Fd: int32(c.unread.Fd()), Events: unix.POLLIN}}
	if !harnesstest.PollUntil(t, settle, 5*time.Millisecond, func() bool {
		n, err := unix.Poll(fds, 0)
		return err == nil && n == 0
	}) {
		t.Fatal("the client never read everything typed to it")
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
