//go:build darwin

package tui

import (
	"errors"
	"os"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// A pipe takes a read deadline whatever reader shape is bound to it, so the
// seam-driven tests cannot tell the production open from an os.Stdin-shaped one.
// Only a real terminal can.
const realTTYDeadline = 80 * time.Millisecond

func TestProductionReader_BoundsAReadAgainstARealTerminal(t *testing.T) {
	reader, err := openTTYPath(grantedPTYSlave(t))
	if err != nil {
		t.Fatalf("open the pty slave the way the probe opens the pane's terminal: %v", err)
	}
	t.Cleanup(func() { _ = reader.Close() })

	t.Run("it bounds a read against a real terminal", func(t *testing.T) {
		if err := reader.SetReadDeadline(time.Now().Add(realTTYDeadline)); err != nil {
			t.Fatalf("SetReadDeadline on a terminal: %v — a reader that refuses one sends every production probe down the dark branch", err)
		}

		start := time.Now()
		n, err := reader.Read(make([]byte, 16))
		elapsed := time.Since(start)

		if !errors.Is(err, os.ErrDeadlineExceeded) {
			t.Fatalf("read of a silent terminal returned (%d, %v), want %v", n, err, os.ErrDeadlineExceeded)
		}
		if elapsed < realTTYDeadline {
			t.Errorf("read returned after %v, want it to run the deadline of %v out", elapsed, realTTYDeadline)
		}
	})
}

// grantedPTYSlave is the slave path of a fresh pty, granted and unlocked through
// the host's own ioctls. The master is held open for the test's duration: closing
// it would make the slave read return EOF instead of running its deadline out.
func grantedPTYSlave(t *testing.T) string {
	t.Helper()
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("open /dev/ptmx: %v", err)
	}
	t.Cleanup(func() { _ = master.Close() })

	fd := int(master.Fd())
	if err := unix.IoctlSetInt(fd, unix.TIOCPTYGRANT, 0); err != nil {
		t.Fatalf("grant the pty: %v", err)
	}
	if err := unix.IoctlSetInt(fd, unix.TIOCPTYUNLK, 0); err != nil {
		t.Fatalf("unlock the pty: %v", err)
	}

	var name [128]byte
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), uintptr(unix.TIOCPTYGNAME), uintptr(unsafe.Pointer(&name[0]))); errno != 0 {
		t.Fatalf("read the pty slave's name: %v", errno)
	}
	end := 0
	for end < len(name) && name[end] != 0 {
		end++
	}
	return string(name[:end])
}
