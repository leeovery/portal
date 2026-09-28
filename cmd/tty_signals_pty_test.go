//go:build darwin

package cmd

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/leeovery/portal/internal/themetest"
	"github.com/leeovery/portal/internal/tui"
	"golang.org/x/sys/unix"
)

const (
	ttyInterruptKey = "\x03"
	ttyQuitKey      = "\x1c"
	ttySuspendKey   = "\x1a"
)

// paneProcess is a short-lived process whose controlling terminal is the pty,
// so the line discipline's kill keys reach it as they reach a pane's own
// process. Its process group has no parent in its session, and the kernel
// sends no suspend to such a group, so a suspend is observed on the input
// queue rather than on the process.
type paneProcess struct {
	pid    int
	exited chan struct{}
}

// The terminal side is drained for the process's life: a session leader
// exiting with echoed kill keys still queued waits for them to be read.
func foregroundOn(t *testing.T, master, slave *os.File) paneProcess {
	t.Helper()
	go func() { _, _ = io.Copy(io.Discard, master) }()
	child := exec.Command("/bin/sleep", "5")
	child.Stdin, child.Stdout, child.Stderr = slave, slave, slave
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := child.Start(); err != nil {
		t.Fatalf("start the foreground process: %v", err)
	}
	p := paneProcess{pid: child.Process.Pid, exited: make(chan struct{})}
	go func() {
		_ = child.Wait()
		close(p.exited)
	}()
	t.Cleanup(func() {
		_ = child.Process.Kill()
		if !p.endedWithin(5 * time.Second) {
			t.Errorf("the foreground process %d outlived its kill", p.pid)
		}
	})
	return p
}

func (p paneProcess) endedWithin(d time.Duration) bool {
	select {
	case <-p.exited:
		return true
	case <-time.After(d):
		return false
	}
}

func localModes(t *testing.T, fd int) *unix.Termios {
	t.Helper()
	modes, err := unix.IoctlGetTermios(fd, unix.TIOCGETA)
	if err != nil {
		t.Fatalf("read the pty's modes: %v", err)
	}
	return modes
}

func TestTTYSignals_RealPTY(t *testing.T) {
	keys := []struct{ name, key string }{
		{"Ctrl-C", ttyInterruptKey},
		{"Ctrl-\\", ttyQuitKey},
		{"Ctrl-Z", ttySuspendKey},
	}
	for _, k := range keys {
		t.Run("with signal generation cleared, "+k.name+" ends nothing", func(t *testing.T) {
			master, slave := openPTY(t)
			child := foregroundOn(t, master, slave)
			if err := clearTTYSignals(descriptorOf(t, slave)); err != nil {
				t.Fatalf("clearTTYSignals() error = %v", err)
			}

			if _, err := master.Write([]byte(k.key)); err != nil {
				t.Fatalf("press %s: %v", k.name, err)
			}

			if child.endedWithin(300 * time.Millisecond) {
				t.Fatalf("the foreground process ended on %s with signal generation cleared", k.name)
			}
		})

		t.Run("with signal generation cleared, "+k.name+" reaches the reader as a byte", func(t *testing.T) {
			master, slave := openPTY(t)
			if err := clearTTYSignals(descriptorOf(t, slave)); err != nil {
				t.Fatalf("clearTTYSignals() error = %v", err)
			}

			// A line terminator releases the canonical line the key sits in.
			if _, err := master.Write([]byte(k.key + "\n")); err != nil {
				t.Fatalf("press %s: %v", k.name, err)
			}

			if got, want := string(readWithin(t, slave)), k.key+"\n"; got != want {
				t.Errorf("the reader saw %q, want %q", got, want)
			}
		})
	}

	t.Run("with signal generation turned back on, Ctrl-C interrupts the pane's process", func(t *testing.T) {
		master, slave := openPTY(t)
		child := foregroundOn(t, master, slave)
		fd := descriptorOf(t, slave)
		if err := clearTTYSignals(fd); err != nil {
			t.Fatalf("clearTTYSignals() error = %v", err)
		}
		if err := setTTYSignals(fd); err != nil {
			t.Fatalf("setTTYSignals() error = %v", err)
		}

		if _, err := master.Write([]byte(ttyInterruptKey)); err != nil {
			t.Fatalf("press Ctrl-C: %v", err)
		}

		if !child.endedWithin(2 * time.Second) {
			t.Fatal("Ctrl-C did not end the foreground process once signal generation was back on")
		}
	})

	t.Run("a raw-mode round trip puts signal generation back off", func(t *testing.T) {
		_, slave := openPTY(t)
		fd := descriptorOf(t, slave)
		if err := clearTTYSignals(fd); err != nil {
			t.Fatalf("clearTTYSignals() error = %v", err)
		}

		prior, err := term.MakeRaw(uintptr(fd))
		if err != nil {
			t.Fatalf("make the pty raw: %v", err)
		}
		if err := term.Restore(uintptr(fd), prior); err != nil {
			t.Fatalf("restore the pty: %v", err)
		}

		if localModes(t, fd).Lflag&unix.ISIG != 0 {
			t.Error("signal generation is on after a raw-mode round trip, want it left as it was found")
		}
	})

	t.Run("clearing signal generation leaves output processing on", func(t *testing.T) {
		_, slave := openPTY(t)
		fd := descriptorOf(t, slave)
		if err := clearTTYSignals(fd); err != nil {
			t.Fatalf("clearTTYSignals() error = %v", err)
		}

		if localModes(t, fd).Oflag&unix.OPOST == 0 {
			t.Error("output processing is off after clearing signal generation")
		}
	})
}

func TestTTYSignals_ScreensRenderFromTheLeftEdge(t *testing.T) {
	sizes := []struct {
		name          string
		width, height int
	}{
		{"card", 100, 30},
		{"plain stack", 24, 6},
	}
	screens := []struct {
		name   string
		render func(tui.ResumeScreen) string
	}{
		{"panel", tui.RenderResumePanel},
		{"confirmation", tui.RenderResumeDiscardConfirm},
	}
	for _, size := range sizes {
		for _, screen := range screens {
			t.Run("the "+screen.name+" as a "+size.name+" starts every row at the pane's left edge", func(t *testing.T) {
				master, slave := openPTY(t)
				if err := clearTTYSignals(descriptorOf(t, slave)); err != nil {
					t.Fatalf("clearTTYSignals() error = %v", err)
				}
				painted := screen.render(tui.ResumeScreen{
					Command: "make deploy",
					Width:   size.width,
					Height:  size.height,
					Theme:   themetest.DefaultDark(t),
				})
				rows := strings.Count(painted, "\n")
				if rows == 0 {
					t.Fatalf("the %s rendered a single row; nothing to check", screen.name)
				}

				go func() { _, _ = slave.Write([]byte(painted)) }()
				arrived := readRows(t, master, rows)

				if bare := bytes.Count(arrived, []byte("\n")) - bytes.Count(arrived, []byte("\r\n")); bare != 0 {
					t.Errorf("%d of %d rows reached the terminal without a carriage return", bare, rows)
				}
			})
		}
	}
}

// readRows reads the terminal's side until it has seen rows line feeds. The
// master is a blocking descriptor, so the bound is kept outside the read.
func readRows(t *testing.T, master *os.File, rows int) []byte {
	t.Helper()
	type result struct {
		got []byte
		err error
	}
	done := make(chan result, 1)
	go func() {
		var got []byte
		buf := make([]byte, 4096)
		for bytes.Count(got, []byte("\n")) < rows {
			n, err := master.Read(buf)
			got = append(got, buf[:n]...)
			if err != nil {
				done <- result{got, err}
				return
			}
		}
		done <- result{got, nil}
	}()
	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("read the master after %d bytes: %v", len(r.got), r.err)
		}
		return r.got
	case <-time.After(2 * time.Second):
		t.Fatalf("the terminal saw fewer than %d rows within 2s", rows)
		return nil
	}
}
