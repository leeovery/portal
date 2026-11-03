//go:build integration

package zmxtest_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/zmxtest"
)

func TestNew(t *testing.T) {
	t.Run("it binds the Backend to a fresh directory short enough for Portal's names", func(t *testing.T) {
		b := zmxtest.New(t)

		if !strings.HasPrefix(b.Dir(), "/tmp/zx-") {
			t.Errorf("Dir = %q, want one under /tmp/zx-", b.Dir())
		}
		if info, err := os.Stat(b.Dir()); err != nil || !info.IsDir() {
			t.Errorf("Dir %q is not a directory: %v", b.Dir(), err)
		}
		if b.MaxNameLen() < 70 {
			t.Errorf("MaxNameLen = %d, want room for at least the 70 bytes the live directory leaves", b.MaxNameLen())
		}
		if other := zmxtest.New(t); other.Dir() == b.Dir() {
			t.Errorf("two Backends share %q", b.Dir())
		}
	})

	t.Run("it runs sessions in /bin/sh with HOME in a temp dir and only the allowed environment", func(t *testing.T) {
		t.Setenv("PORTAL_SECRET_PROBE", "leaked")
		b := zmxtest.New(t)
		if _, err := b.Create(t.Context(), "env", t.TempDir(), []string{"sh", "-c", `echo "shell=$SHELL"; echo "probe=${PORTAL_SECRET_PROBE-unset}"; echo "home=$HOME"; sleep 1000`}); err != nil {
			t.Fatalf("Create: %v", err)
		}

		var got []byte
		if !harnesstest.PollUntil(t, 5*time.Second, 20*time.Millisecond, func() bool {
			got, _ = b.History(t.Context(), "env", 0)
			return bytes.Contains(got, []byte("shell=/bin/sh\r\n")) && bytes.Contains(got, []byte("home="))
		}) {
			t.Fatalf("history = %q, want the shell and home echoed", got)
		}
		home, _ := os.UserHomeDir()
		if bytes.Contains(got, []byte("home="+home+"\r\n")) {
			t.Errorf("session HOME is the developer's own: %q", got)
		}
		if !bytes.Contains(got, []byte("probe=unset")) {
			t.Errorf("history = %q, want the variable outside the allowlist unset", got)
		}
	})

	t.Run("it kills every process of the directory and removes it at cleanup", func(t *testing.T) {
		var dir string
		var daemon int
		t.Run("inner", func(t *testing.T) {
			b := zmxtest.New(t)
			dir = b.Dir()
			if _, err := b.Create(t.Context(), "left-running", t.TempDir(), []string{"sleep", "1000"}); err != nil {
				t.Fatalf("Create: %v", err)
			}
			daemon = zmxtest.Daemon(t, b, "left-running")
		})

		if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("Dir %q survived cleanup: %v", dir, err)
		}
		if err := syscall.Kill(daemon, 0); err == nil {
			t.Errorf("daemon %d survived cleanup", daemon)
		}
	})
}

func TestNewIgnoresAnInheritedSession(t *testing.T) {
	t.Run("it never hands zmx the caller's session or prefix, whatever TestMain left", func(t *testing.T) {
		t.Setenv("ZMX_SESSION", "live-session")
		t.Setenv("ZMX_SESSION_PREFIX", "pre-")
		b := zmxtest.New(t)

		if got := b.Current(); got != "" {
			t.Errorf("Current = %q, want none", got)
		}
		if _, err := b.Create(t.Context(), "fresh", t.TempDir(), []string{"sleep", "1000"}); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if exists, err := b.Exists(t.Context(), "fresh"); err != nil || !exists {
			t.Errorf("Exists(fresh) = %v, %v; want the session made under its own name", exists, err)
		}
		cmd, err := b.AttachCommand(t.Context(), "fresh")
		if err != nil {
			t.Fatalf("AttachCommand: %v", err)
		}
		for _, kv := range cmd.Env {
			if strings.HasPrefix(kv, "ZMX_SESSION=") || strings.HasPrefix(kv, "ZMX_SESSION_PREFIX=") {
				t.Errorf("attach command carries %s", kv)
			}
		}
	})
}

func TestPoisonedEnvironment(t *testing.T) {
	t.Run("it makes a zmx run that misses the Backend's directory fail", func(t *testing.T) {
		if got := os.Getenv("ZMX_DIR"); got != zmxtest.PoisonedDir {
			t.Fatalf("ZMX_DIR = %q, want the poison", got)
		}
		cmd := exec.Command(zmxtest.Binary(t), "list")
		cmd.Env = append(os.Environ(), "HOME="+t.TempDir())
		if out, err := cmd.CombinedOutput(); err == nil {
			t.Errorf("zmx list on the ambient environment succeeded: %s", out)
		}
	})

	t.Run("it leaves no inherited session or prefix", func(t *testing.T) {
		for _, key := range []string{"ZMX_SESSION", "ZMX_SESSION_PREFIX"} {
			if v, ok := os.LookupEnv(key); ok {
				t.Errorf("%s = %q inherited into the test", key, v)
			}
		}
	})
}

func TestAttach(t *testing.T) {
	t.Run("it holds a client attached until zmx detaches it", func(t *testing.T) {
		b := zmxtest.New(t)
		if _, err := b.Create(t.Context(), "held", t.TempDir(), []string{"sleep", "1000"}); err != nil {
			t.Fatalf("Create: %v", err)
		}

		c := zmxtest.Attach(t, b, "held")

		if !harnesstest.PollUntil(t, 5*time.Second, 20*time.Millisecond, func() bool { return clients(t, b, "held") == 1 }) {
			t.Fatalf("client never attached")
		}
		if c.Exited(200 * time.Millisecond) {
			t.Fatal("client exited while attached")
		}
		if err := b.Detach(t.Context(), "held"); err != nil {
			t.Fatalf("Detach: %v", err)
		}
		if !c.Exited(5 * time.Second) {
			t.Error("client still running after Detach")
		}
	})
}

func clients(t *testing.T, b *zmx.Backend, name string) int {
	t.Helper()
	sessions, err := b.List(t.Context())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, s := range sessions {
		if s.Name == name {
			return s.Clients
		}
	}
	return -1
}
