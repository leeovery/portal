//go:build integration

package cmd_test

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/cmd"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/zmxtest"
)

// serveProtocol2 puts a daemon in dir under name that answers every request
// with a frame header of protocol 2, as a zmx newer than Portal's would.
func serveProtocol2(t *testing.T, dir, name string) {
	t.Helper()
	ln, err := net.Listen("unix", filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("listen as %s: %v", name, err)
	}
	done := make(chan struct{})
	t.Cleanup(func() {
		_ = ln.Close()
		<-done
	})
	go func() {
		defer close(done)
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			request := make([]byte, 8)
			if _, err := io.ReadFull(c, request); err == nil {
				_, _ = c.Write([]byte{request[0], 2, 0, 0, 0, 0, 0, 0})
			}
			_ = c.Close()
		}
	}()
}

// leaveRefusingSocket leaves a socket in dir under name that refuses every
// connection, as a crashed zmx daemon's does.
func leaveRefusingSocket(t *testing.T, dir, name string) {
	t.Helper()
	ln, err := net.Listen("unix", filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("listen as %s: %v", name, err)
	}
	ln.(*net.UnixListener).SetUnlinkOnClose(false)
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestPortalDoctorAgainstZmx(t *testing.T) {
	binary := buildPortalBinary(t)
	d := portaltest.StartSaverDaemon(t, binary)
	b := d.Backend
	zmxBinary := zmxtest.Binary(t)

	if _, err := b.Create(t.Context(), "api-x7Kd9a", t.TempDir(), []string{"sleep", "1000"}); err != nil {
		t.Fatalf("Create(api-x7Kd9a): %v", err)
	}
	if err := state.ResumePending.Set(d.StateDir, "api-x7Kd9a"); err != nil {
		t.Fatal(err)
	}

	env := append(slices.Clone(d.Env),
		"ZMX_DIR="+b.Dir(),
		"PATH="+filepath.Dir(zmxBinary)+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	doctor := func(t *testing.T) (string, int) {
		t.Helper()
		run := exec.Command(binary, "doctor")
		run.Env = env
		out, err := run.Output()
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			return string(out), exitErr.ExitCode()
		}
		if err != nil {
			t.Fatalf("portal doctor: %v\n%s", err, out)
		}
		return string(out), 0
	}
	assertLines := func(t *testing.T, out string, lines ...string) {
		t.Helper()
		for _, line := range lines {
			if !strings.Contains(out, "\n  "+line+"\n") {
				t.Errorf("report missing %q:\n%s", line, out)
			}
		}
	}

	t.Run("it passes every check with every session answering and the saver's daemon proved", func(t *testing.T) {
		out, code := doctor(t)
		if code != 0 {
			t.Errorf("exit code = %d, want 0:\n%s", code, out)
		}
		assertLines(t, out,
			"✓ zmx: "+zmxBinary,
			"✓ zmx dir: "+b.Dir(),
			"✓ sessions: 2 sessions answering",
			"✓ saver: _portal-saver up",
			"  pending resumes: 1 session waiting to resume",
			"10 checks passed",
		)
		if want := fmt.Sprintf("\n  ✓ daemon: running (pid %d, version ", d.PID); !strings.Contains(out, want) {
			t.Errorf("report missing %q:\n%s", want, out)
		}
		if d.Signal(0) != nil {
			t.Error("the saver's daemon is gone after doctor")
		}
	})

	t.Run("it names a refusing socket, a protocol-2 daemon and a silent one, and exits non-zero", func(t *testing.T) {
		if _, err := b.Create(t.Context(), "wedged-Lp4s", t.TempDir(), []string{"sleep", "1000"}); err != nil {
			t.Fatalf("Create(wedged-Lp4s): %v", err)
		}
		zmxtest.StopDaemon(t, b, "wedged-Lp4s")
		serveProtocol2(t, b.Dir(), "future-2")
		leaveRefusingSocket(t, b.Dir(), "crashed-9")

		out, code := doctor(t)
		if code != 1 {
			t.Errorf("exit code = %d, want 1:\n%s", code, out)
		}
		assertLines(t, out,
			"✗ sessions: 3 of 5 not answering: crashed-9 (refuses connections: its daemon has crashed; `zmx list` clears its socket), "+
				"future-2 (speaks protocol 2; Portal speaks 1), wedged-Lp4s (no answer)",
			"9 of 10 checks passed",
		)
	})
}

// The host doctor reads the saver through reads a real saver as the
// lifecycle's host does: its session, the process it runs, proved its daemon.
func TestDoctorsSaverHostReadsTheSaverAgainstZmx(t *testing.T) {
	binary := buildPortalBinary(t)
	d := portaltest.StartSaverDaemon(t, binary)
	host, err := cmd.ReadOnlySaverHost(d.Backend, d.StateDir)
	if err != nil {
		t.Fatal(err)
	}

	if present, err := host.Present(t.Context()); err != nil || !present {
		t.Errorf("Present = %v, %v; want the saver found", present, err)
	}
	if pid, present, err := host.SaverPID(t.Context()); err != nil || !present || pid != d.PID {
		t.Errorf("SaverPID = %d, %v, %v; want the daemon's %d", pid, present, err, d.PID)
	}
	if err := host.Prove(d.PID); err != nil {
		t.Errorf("Prove(%d) = %v; want the saver's daemon proved", d.PID, err)
	}
	if d.Signal(0) != nil {
		t.Error("the daemon is gone")
	}
}
