//go:build darwin

package cmd

import (
	"errors"
	"os"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/charmbracelet/x/term"
	"golang.org/x/sys/unix"
)

const ptyReadDeadline = 200 * time.Millisecond

func TestFlushTTYInput_RealPTY(t *testing.T) {
	t.Run("it discards bytes queued before the flush and keeps bytes queued after it", func(t *testing.T) {
		master, slave := openPTY(t)

		// Raw, so a byte is readable without a line terminator behind it.
		fd := descriptorOf(t, slave)
		prior, err := term.MakeRaw(uintptr(fd))
		if err != nil {
			t.Fatalf("make the slave raw: %v", err)
		}
		t.Cleanup(func() { _ = term.Restore(uintptr(fd), prior) })

		// A write to the master runs the line discipline before it returns, so
		// the burst is on the slave's input queue once it does.
		if _, err := master.Write([]byte("cd ~/dev && yarn")); err != nil {
			t.Fatalf("write the burst into the master: %v", err)
		}

		if err := flushTTYInput(fd); err != nil {
			t.Fatalf("flushTTYInput() error = %v", err)
		}

		if got := readWithin(t, slave); len(got) != 0 {
			t.Errorf("read %q from the slave after the flush, want nothing", got)
		}

		if _, err := master.Write([]byte("y")); err != nil {
			t.Fatalf("write the later key into the master: %v", err)
		}
		if got := readWithin(t, slave); string(got) != "y" {
			t.Errorf("read %q from the slave after a post-flush write, want %q", got, "y")
		}
	})
}

// openPTY is a fresh pty granted and unlocked through the host's own ioctls,
// with both ends closed at the test's end.
func openPTY(t *testing.T) (master, slave *os.File) {
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

	slave, err = os.OpenFile(string(name[:end]), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Fatalf("open the pty slave: %v", err)
	}
	t.Cleanup(func() { _ = slave.Close() })
	return master, slave
}

// descriptorOf reads f's descriptor without File.Fd, which would switch it to
// blocking mode and leave every read deadline on it unenforced.
func descriptorOf(t *testing.T, f *os.File) int {
	t.Helper()
	conn, err := f.SyscallConn()
	if err != nil {
		t.Fatalf("SyscallConn: %v", err)
	}
	var fd int
	if err := conn.Control(func(raw uintptr) { fd = int(raw) }); err != nil {
		t.Fatalf("read the descriptor: %v", err)
	}
	return fd
}

func readWithin(t *testing.T, slave *os.File) []byte {
	t.Helper()
	if err := slave.SetReadDeadline(time.Now().Add(ptyReadDeadline)); err != nil {
		t.Fatalf("SetReadDeadline on the slave: %v", err)
	}
	buf := make([]byte, 64)
	n, err := slave.Read(buf)
	if err != nil && !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("read the slave: %v", err)
	}
	return buf[:n]
}
