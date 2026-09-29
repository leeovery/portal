//go:build darwin

package tui

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/leeovery/portal/internal/theme"
	"golang.org/x/sys/unix"
)

// A pipe honours any bound, so the seam-driven tests cannot tell a read macOS
// refuses to bound from one it honours. Only a process whose controlling
// terminal is a real pty can, and it must read the pty the way a pane's process
// does: as its own stdin.
const paneProbeChildEnv = "PORTAL_TUI_PANE_PROBE_CHILD"

// The child's first ExtraFiles descriptor, which carries its answer back so
// nothing but the probe itself writes to the pty.
const paneProbeResultFD = 3

const (
	childLightCanvas = "light-member"
	childDarkCanvas  = "dark-member"
)

// Long enough, once the probe's process has exited, for any echo of a reply
// it left unread to reach the master.
const ptyDrainWindow = 200 * time.Millisecond

const paneProbeChildBudget = 10 * time.Second

func TestProductionPaneProbe_RealTerminal(t *testing.T) {
	if os.Getenv(paneProbeChildEnv) == "1" {
		runPaneProbeChild()
		return
	}

	cases := []struct {
		name  string
		reply string
		want  string
	}{
		{"it resolves the light member for a terminal answering with a light background", lightReply, childLightCanvas},
		{"it resolves the dark member for a terminal answering with a dark background", darkReply, childDarkCanvas},
		{"it resolves dark for a terminal that never answers", "", childDarkCanvas},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			run := probeOnPTY(t, tc.reply)

			if run.resolved != tc.want {
				t.Errorf("the probe resolved %q, want %q", run.resolved, tc.want)
			}
			if run.transcript != backgroundQuery {
				t.Errorf("the terminal received %q, want exactly one background-colour query %q and nothing of the reply echoed back", run.transcript, backgroundQuery)
			}
		})
	}
}

// runPaneProbeChild resolves a pane's palette through the production probe and
// exits before the test framework can write its verdict onto the pty.
func runPaneProbeChild() {
	pair := theme.AdaptivePair(
		theme.Theme{Canvas: theme.Token{Name: "canvas", Value: childLightCanvas}},
		theme.Theme{Canvas: theme.Token{Name: "canvas", Value: childDarkCanvas}},
	)
	resolved, _ := ResolvePaneTheme(pair, false, nil)
	result := os.NewFile(paneProbeResultFD, "result")
	_, _ = result.WriteString(resolved.Canvas.Value)
	_ = result.Close()
	os.Exit(0)
}

type paneProbeRun struct {
	resolved   string
	transcript string
}

// probeOnPTY runs the production probe in a child whose controlling terminal is
// a fresh pty, answering its query with reply (or never, for an empty reply),
// and returns what the child resolved alongside everything the terminal saw.
func probeOnPTY(t *testing.T, reply string) paneProbeRun {
	t.Helper()
	master, slave := openProbePTY(t)
	results, resultWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	t.Cleanup(func() { _ = results.Close() })

	child := exec.Command(os.Args[0], "-test.run=^TestProductionPaneProbe_RealTerminal$")
	child.Env = append(os.Environ(), paneProbeChildEnv+"=1")
	child.Stdin, child.Stdout, child.Stderr = slave, slave, slave
	child.ExtraFiles = []*os.File{resultWriter}
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := child.Start(); err != nil {
		t.Fatalf("start the probe's process: %v", err)
	}
	_ = resultWriter.Close()
	exited := make(chan struct{})
	go func() {
		_ = child.Wait()
		close(exited)
	}()
	t.Cleanup(func() {
		_ = child.Process.Kill()
		<-exited
	})

	transcript := answerQuery(t, readChunks(master), master, reply, exited)
	resolved, err := io.ReadAll(results)
	if err != nil {
		t.Fatalf("read the probe's answer: %v", err)
	}
	return paneProbeRun{resolved: string(resolved), transcript: transcript}
}

// answerQuery collects what the terminal receives until the child has exited
// and the drain window has passed, writing reply to the terminal the moment the
// query has arrived.
func answerQuery(t *testing.T, chunks <-chan []byte, master *os.File, reply string, exited <-chan struct{}) string {
	t.Helper()
	var seen bytes.Buffer
	answered := reply == ""
	budget := time.After(paneProbeChildBudget)
	var drained <-chan time.Time
	for {
		select {
		case chunk := <-chunks:
			seen.Write(chunk)
			if !answered && bytes.Contains(seen.Bytes(), []byte(backgroundQuery)) {
				answered = true
				if _, err := master.WriteString(reply); err != nil {
					t.Fatalf("answer the query: %v", err)
				}
			}
		case <-exited:
			exited = nil
			drained = time.After(ptyDrainWindow)
		case <-drained:
			return seen.String()
		case <-budget:
			t.Fatalf("the probe's process did not exit within %v; the terminal saw %q", paneProbeChildBudget, seen.String())
		}
	}
}

// The master is read for the pty's whole life: a session leader exiting with
// output still queued waits for it to be read.
func readChunks(master *os.File) <-chan []byte {
	chunks := make(chan []byte, 64)
	go func() {
		buf := make([]byte, 256)
		for {
			n, err := master.Read(buf)
			if n > 0 {
				chunks <- bytes.Clone(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()
	return chunks
}

// openProbePTY is a fresh pty granted and unlocked through the host's own
// ioctls, with both ends closed at the test's end. The slave is opened without
// becoming this process's controlling terminal, so only the child takes it.
func openProbePTY(t *testing.T) (master, slave *os.File) {
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
