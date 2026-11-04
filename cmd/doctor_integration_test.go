//go:build integration

package cmd_test

import (
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/portaltest"
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
	env, _ := portaltest.IsolateStateForTest(t)
	b := zmxtest.New(t)
	binary := buildPortalBinary(t)
	zmxBinary := zmxtest.Binary(t)

	if _, err := b.Create(t.Context(), "api-x7Kd9a", t.TempDir(), []string{"sleep", "1000"}); err != nil {
		t.Fatalf("Create(api-x7Kd9a): %v", err)
	}

	env = append(env,
		"ZMX_DIR="+b.Dir(),
		"PATH="+filepath.Dir(zmxBinary)+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	doctor := func(t *testing.T) (string, int) {
		t.Helper()
		cmd := exec.Command(binary, "doctor")
		cmd.Env = env
		out, err := cmd.Output()
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

	t.Run("it passes every check with every session answering", func(t *testing.T) {
		out, code := doctor(t)
		if code != 0 {
			t.Errorf("exit code = %d, want 0:\n%s", code, out)
		}
		assertLines(t, out,
			"✓ zmx: "+zmxBinary,
			"✓ zmx dir: "+b.Dir(),
			"✓ sessions: 1 session answering",
			"8 checks passed",
		)
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
			"✗ sessions: 3 of 4 not answering: crashed-9 (refuses connections: its daemon has crashed; `zmx list` clears its socket), "+
				"future-2 (speaks protocol 2; Portal speaks 1), wedged-Lp4s (no answer)",
			"7 of 8 checks passed",
		)
	})
}
