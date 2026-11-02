//go:build integration

package zmxtest

import (
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/harnesstest"
)

// Each case drives teardown directly while the session's daemon is out of
// zmx's reach; New's own cleanup then finds nothing left.
func TestTeardown(t *testing.T) {
	t.Run("it fails, within its bounds, on a daemon that will not die, and kills it", func(t *testing.T) {
		b := New(t)
		if err := b.Create("wedged", t.TempDir(), []string{"sleep", "1000"}); err != nil {
			t.Fatalf("Create: %v", err)
		}
		daemon := Daemon(t, b, "wedged")
		if err := syscall.Kill(daemon, syscall.SIGSTOP); err != nil {
			t.Fatalf("stop daemon %d: %v", daemon, err)
		}
		rec := &harnesstest.Recorder{}
		start := time.Now()

		teardown(rec, b)

		if took := time.Since(start); took > 20*time.Second {
			t.Errorf("teardown took %s", took)
		}
		if !rec.Failed() || !strings.Contains(rec.Report(), "survived their kill") {
			t.Errorf("teardown reported %s, want the surviving daemon", rec.Report())
		}
		if syscall.Kill(daemon, 0) == nil {
			_ = syscall.Kill(daemon, syscall.SIGKILL)
			t.Errorf("daemon %d survived teardown", daemon)
		}
	})

	t.Run("it finds and kills the daemons of a directory zmx cannot list", func(t *testing.T) {
		b := New(t)
		if err := b.Create("hidden", t.TempDir(), []string{"sleep", "1000"}); err != nil {
			t.Fatalf("Create: %v", err)
		}
		daemon := Daemon(t, b, "hidden")
		if err := os.Chmod(b.Dir(), 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(b.Dir(), 0o750) })
		rec := &harnesstest.Recorder{}

		teardown(rec, b)

		if !rec.Failed() {
			t.Error("teardown reported nothing for a directory it could not list")
		}
		if syscall.Kill(daemon, 0) == nil {
			_ = syscall.Kill(daemon, syscall.SIGKILL)
			t.Errorf("daemon %d survived teardown", daemon)
		}
	})

	t.Run("it passes quietly when every kill lands", func(t *testing.T) {
		b := New(t)
		if err := b.Create("plain", t.TempDir(), []string{"sleep", "1000"}); err != nil {
			t.Fatalf("Create: %v", err)
		}
		daemon := Daemon(t, b, "plain")
		rec := &harnesstest.Recorder{}

		teardown(rec, b)

		if rec.Failed() {
			t.Errorf("teardown reported %s", rec.Report())
		}
		if syscall.Kill(daemon, 0) == nil {
			t.Errorf("daemon %d survived teardown", daemon)
		}
	})
}

func TestDaemons(t *testing.T) {
	t.Run("it finds only the daemons started for its own directory", func(t *testing.T) {
		a, b := New(t), New(t)
		for _, be := range []*zmx.Backend{a, b} {
			if err := be.Create("same-name", t.TempDir(), []string{"sleep", "1000"}); err != nil {
				t.Fatalf("Create: %v", err)
			}
		}

		got := daemons(a.Dir())
		if want := Daemon(t, a, "same-name"); len(got) != 1 || got[0] != want {
			t.Errorf("daemons(%s) = %v, want only %d", a.Dir(), got, want)
		}
	})
}
