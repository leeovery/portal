package zmx

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
)

func TestDialFailure(t *testing.T) {
	t.Run("it reads EAGAIN, a full backlog on Linux, as a daemon not accepting, never as a stale socket", func(t *testing.T) {
		dir, err := os.MkdirTemp("/tmp", "zx-")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(dir) })
		l, err := net.Listen("unix", filepath.Join(dir, "old"))
		if err != nil {
			t.Fatal(err)
		}
		l.(*net.UnixListener).SetUnlinkOnClose(false)
		_ = l.Close()
		beforeBoot := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
		if err := os.Chtimes(filepath.Join(dir, "old"), beforeBoot, beforeBoot); err != nil {
			t.Fatal(err)
		}
		b, err := New(Config{Dir: dir})
		if err != nil {
			t.Fatal(err)
		}
		dialErr := func(errno syscall.Errno) error {
			return &net.OpError{Op: "dial", Net: "unix", Err: os.NewSyscallError("connect", errno)}
		}

		if err := b.dialFailure(t.Context(), "old", dialErr(syscall.EAGAIN)); !errors.Is(err, errNotAccepting) || !errors.Is(err, backend.ErrTimeout) || gone(err) {
			t.Errorf("dialFailure(EAGAIN) = %v, want a daemon not accepting, ErrTimeout, not gone", err)
		}
		if err := b.dialFailure(t.Context(), "old", dialErr(syscall.ECONNREFUSED)); !gone(err) {
			t.Errorf("dialFailure(ECONNREFUSED) = %v, want the socket from before boot stale", err)
		}
	})
}

func TestConnSendsOneFrame(t *testing.T) {
	t.Run("it refuses a second frame on the connection, writing nothing of it", func(t *testing.T) {
		client, server := net.Pipe()
		received := make(chan []byte)
		go func() {
			b, _ := io.ReadAll(server)
			received <- b
		}()
		c := &conn{nc: client, r: bufio.NewReader(client), stop: func() bool { return true }}

		if err := c.send(tagHistory, []byte{historyVT}); err != nil {
			t.Fatalf("first send: %v", err)
		}
		if err := c.send(tagKill, nil); !errors.Is(err, errSecondFrame) {
			t.Errorf("second send = %v, want errSecondFrame", err)
		}
		_ = c.Close()

		if got, want := <-received, append(encodeHeader(tagHistory, 1), historyVT); !bytes.Equal(got, want) {
			t.Errorf("the daemon received % x, want only the first frame % x", got, want)
		}
	})
}
