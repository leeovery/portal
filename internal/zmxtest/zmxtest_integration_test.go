//go:build integration

package zmxtest_test

import (
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

	t.Run("it runs sessions in /bin/sh with HOME in a temp dir", func(t *testing.T) {
		b := zmxtest.New(t)
		if err := b.Create("env", t.TempDir(), []string{"sh", "-c", `echo "shell=$SHELL home=$HOME"; sleep 1000`}); err != nil {
			t.Fatalf("Create: %v", err)
		}

		var got []byte
		if !harnesstest.PollUntil(t, 5*time.Second, 20*time.Millisecond, func() bool {
			got, _ = b.History("env")
			return strings.Contains(string(got), "shell=/bin/sh home=")
		}) {
			t.Fatalf("history = %q, want the shell and home echoed", got)
		}
		home, _ := os.UserHomeDir()
		if strings.Contains(string(got), "home="+home) {
			t.Errorf("session HOME is the developer's own: %q", got)
		}
	})

	t.Run("it kills every daemon in the directory and removes it at cleanup", func(t *testing.T) {
		var dir string
		var daemon int
		t.Run("inner", func(t *testing.T) {
			b := zmxtest.New(t)
			dir = b.Dir()
			if err := b.Create("left-running", t.TempDir(), []string{"sleep", "1000"}); err != nil {
				t.Fatalf("Create: %v", err)
			}
			sessions, err := b.List()
			if err != nil || len(sessions) != 1 {
				t.Fatalf("List = %+v, %v", sessions, err)
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

		cmd, err := b.AttachCommand("fresh")
		if err != nil {
			t.Fatalf("AttachCommand: %v", err)
		}
		for _, kv := range cmd.Env {
			if strings.HasPrefix(kv, "ZMX_SESSION=") || strings.HasPrefix(kv, "ZMX_SESSION_PREFIX=") {
				t.Errorf("attach command carries %s", kv)
			}
		}
		if got := b.Current(); got != "" {
			t.Errorf("Current = %q, want none", got)
		}
		if err := b.Create("fresh", t.TempDir(), []string{"sleep", "1000"}); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if exists, err := b.Exists("fresh"); err != nil || !exists {
			t.Errorf("Exists(fresh) = %v, %v; want the session made under its own name", exists, err)
		}
	})
}

func TestPoisonedEnvironment(t *testing.T) {
	t.Run("it makes a zmx run that misses the Backend's directory fail", func(t *testing.T) {
		if _, err := exec.LookPath("zmx"); err != nil {
			t.Skip("zmx not available")
		}
		if got := os.Getenv("ZMX_DIR"); got != "/nonexistent/zmx-poison" {
			t.Fatalf("ZMX_DIR = %q, want the poison", got)
		}
		cmd := exec.Command("zmx", "list")
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
		if err := b.Create("held", t.TempDir(), []string{"sleep", "1000"}); err != nil {
			t.Fatalf("Create: %v", err)
		}

		c := zmxtest.Attach(t, b, "held")

		if !harnesstest.PollUntil(t, 5*time.Second, 20*time.Millisecond, func() bool { return clients(t, b, "held") == 1 }) {
			t.Fatalf("client never attached")
		}
		if c.Exited(200 * time.Millisecond) {
			t.Fatal("client exited while attached")
		}
		if err := b.Detach("held"); err != nil {
			t.Fatalf("Detach: %v", err)
		}
		if !c.Exited(5 * time.Second) {
			t.Error("client still running after Detach")
		}
	})
}

func clients(t *testing.T, b *zmx.Backend, name string) int {
	t.Helper()
	sessions, err := b.List()
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
