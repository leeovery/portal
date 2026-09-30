//go:build darwin

package cmd

import (
	"errors"
	"io"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// chainPTY is a pty whose slave is in the state the resume chain hands a draw:
// canonical processing on and signal generation off. The master's output is
// drained for the test's life, so echo never fills it and stalls the writes.
func chainPTY(t *testing.T) (master *os.File, fd int) {
	t.Helper()
	master, slave := openPTY(t)
	fd = descriptorOf(t, slave)
	modes, err := unix.IoctlGetTermios(fd, ioctlReadTermios)
	if err != nil {
		t.Fatalf("read the slave's modes: %v", err)
	}
	modes.Lflag |= unix.ICANON
	modes.Lflag &^= unix.ISIG
	if err := unix.IoctlSetTermios(fd, ioctlWriteTermios, modes); err != nil {
		t.Fatalf("set the slave's modes: %v", err)
	}
	go func() { _, _ = io.Copy(io.Discard, master) }()
	return master, fd
}

// queuedOn reads what the slave still holds, raw so an unterminated line is
// readable too, and puts the slave's modes back afterwards.
func queuedOn(t *testing.T, fd int) []byte {
	t.Helper()
	prior, err := unix.IoctlGetTermios(fd, ioctlReadTermios)
	if err != nil {
		t.Fatalf("read the slave's modes: %v", err)
	}
	raw := *prior
	raw.Lflag &^= unix.ICANON
	raw.Cc[unix.VMIN] = 0
	raw.Cc[unix.VTIME] = 0
	if err := unix.IoctlSetTermios(fd, ioctlWriteTermios, &raw); err != nil {
		t.Fatalf("make the slave readable: %v", err)
	}
	defer func() { _ = unix.IoctlSetTermios(fd, ioctlWriteTermios, prior) }()

	var queued []byte
	buf := make([]byte, 4096)
	for {
		n, err := unix.Read(fd, buf)
		if n <= 0 || err != nil {
			return queued
		}
		queued = append(queued, buf[:n]...)
	}
}

// feed writes each chunk into the master, pausing between them, and reports
// through done once the last one is written.
func feed(t *testing.T, master *os.File, pause time.Duration, chunks ...string) *atomic.Bool {
	t.Helper()
	var done atomic.Bool
	stop := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		defer done.Store(true)
		for _, chunk := range chunks {
			select {
			case <-stop:
				return
			case <-time.After(pause):
			}
			if _, err := master.Write([]byte(chunk)); err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() {
		close(stop)
		<-finished
	})
	return &done
}

func repeated(chunk string, n int) []string {
	chunks := make([]string, n)
	for i := range chunks {
		chunks[i] = chunk
	}
	return chunks
}

type ttyModes struct {
	canonical, signals bool
	vmin, vtime        uint8
}

func modesOf(t *testing.T, fd int) ttyModes {
	t.Helper()
	m, err := unix.IoctlGetTermios(fd, ioctlReadTermios)
	if err != nil {
		t.Fatalf("read the slave's modes: %v", err)
	}
	return ttyModes{
		canonical: m.Lflag&unix.ICANON != 0,
		signals:   m.Lflag&unix.ISIG != 0,
		vmin:      m.Cc[unix.VMIN],
		vtime:     m.Cc[unix.VTIME],
	}
}

func TestTTYDrain_RealPTY(t *testing.T) {
	t.Run("it discards input queued before it runs", func(t *testing.T) {
		master, fd := chainPTY(t)
		if _, err := master.Write([]byte("d\ncd ~/dev && yarn\nyes")); err != nil {
			t.Fatalf("write the burst into the master: %v", err)
		}

		if err := resumeInputDrain.drain(fd); err != nil {
			t.Fatalf("drain() error = %v", err)
		}
		if got := queuedOn(t, fd); len(got) != 0 {
			t.Errorf("the slave still holds %q after the drain, want nothing", got)
		}
	})

	t.Run("it drains a multi-line burst that keeps arriving until it goes quiet", func(t *testing.T) {
		master, fd := chainPTY(t)
		done := feed(t, master, 10*time.Millisecond, repeated("echo 0123456789 >> burst-ran\n", 20)...)

		if err := resumeInputDrain.drain(fd); err != nil {
			t.Fatalf("drain() error = %v", err)
		}
		if !done.Load() {
			t.Error("the drain returned while the burst was still arriving")
		}
		if got := queuedOn(t, fd); len(got) != 0 {
			t.Errorf("the slave still holds %q after the drain, want nothing", got)
		}
	})

	t.Run("it drains an unterminated line still arriving rather than reading it as quiet", func(t *testing.T) {
		master, fd := chainPTY(t)
		done := feed(t, master, 20*time.Millisecond, "d", "y", "e", "s", "y", "e", "s", "y", "e", "s", "y", "e", "s")

		if err := resumeInputDrain.drain(fd); err != nil {
			t.Fatalf("drain() error = %v", err)
		}
		if !done.Load() {
			t.Error("the drain returned while the unterminated line was still arriving")
		}
		if got := queuedOn(t, fd); len(got) != 0 {
			t.Errorf("the slave still holds %q after the drain, want nothing", got)
		}
	})

	t.Run("it drains a background reply landing after the drain began", func(t *testing.T) {
		master, fd := chainPTY(t)
		done := feed(t, master, 20*time.Millisecond, "\x1b]11;rgb:0d0d/1111/1717\x07")

		if err := resumeInputDrain.drain(fd); err != nil {
			t.Fatalf("drain() error = %v", err)
		}
		if !done.Load() {
			t.Fatal("the drain returned before the reply landed")
		}
		if got := queuedOn(t, fd); len(got) != 0 {
			t.Errorf("the slave still holds %q after the drain, want nothing", got)
		}
	})

	t.Run("it returns once the input has been quiet for the settle window", func(t *testing.T) {
		_, fd := chainPTY(t)

		start := time.Now()
		if err := resumeInputDrain.drain(fd); err != nil {
			t.Fatalf("drain() error = %v", err)
		}
		if elapsed := time.Since(start); elapsed < resumeEscapeFollow || elapsed >= resumeInputDrain.bound {
			t.Errorf("a quiet drain took %v, want at least %v and under %v", elapsed, resumeEscapeFollow, resumeInputDrain.bound)
		}
	})

	t.Run("it reports input still arriving once its bound has elapsed", func(t *testing.T) {
		master, fd := chainPTY(t)
		feed(t, master, 10*time.Millisecond, repeated("x\n", 300)...)

		start := time.Now()
		err := resumeInputDrain.drain(fd)
		elapsed := time.Since(start)

		if !errors.Is(err, errInputKeptArriving) {
			t.Fatalf("drain() error = %v, want errInputKeptArriving", err)
		}
		if err.Error() != "input kept arriving" {
			t.Errorf("drain() error = %q, want %q", err, "input kept arriving")
		}
		if elapsed < time.Second || elapsed > time.Second+resumeEscapeFollow+200*time.Millisecond {
			t.Errorf("the drain gave up after %v, want once one second had elapsed", elapsed)
		}
	})

	t.Run("it leaves the tty as it found it", func(t *testing.T) {
		cases := []struct {
			name   string
			arrive func(t *testing.T, master *os.File)
		}{
			{"quiet", func(*testing.T, *os.File) {}},
			{"a burst that ends", func(t *testing.T, master *os.File) {
				feed(t, master, 10*time.Millisecond, repeated("abc", 10)...)
			}},
			{"input that keeps arriving", func(t *testing.T, master *os.File) {
				feed(t, master, 10*time.Millisecond, repeated("x", 300)...)
			}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				master, fd := chainPTY(t)
				before := modesOf(t, fd)
				tc.arrive(t, master)

				_ = resumeInputDrain.drain(fd)

				after := modesOf(t, fd)
				if after != before {
					t.Errorf("modes after the drain = %+v, want those it found %+v", after, before)
				}
				if !after.canonical || after.signals {
					t.Errorf("modes after the drain = %+v, want canonical on and signals off", after)
				}
			})
		}
	})
}
