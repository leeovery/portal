//go:build integration

package zmxtest

import (
	"os"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/harnesstest"
)

func mustCreate(t *testing.T, b *zmx.Backend, name string) {
	t.Helper()
	mustCreateRunning(t, b, name, "sleep", "1000")
}

func mustCreateRunning(t *testing.T, b *zmx.Backend, name string, command ...string) {
	t.Helper()
	if _, err := b.Create(t.Context(), name, t.TempDir(), command); err != nil {
		t.Fatalf("Create(%s): %v", name, err)
	}
}

func alive(pid int) bool { return syscall.Kill(pid, 0) == nil }

// Each case drives teardown directly while the session's daemon is out of
// zmx's reach; New's own cleanup then finds nothing left.
func TestTeardown(t *testing.T) {
	t.Run("it fails, within its bounds, on a daemon that will not die, and kills it", func(t *testing.T) {
		b := New(t)
		mustCreate(t, b, "wedged")
		daemon := Daemon(t, b, "wedged")
		if err := syscall.Kill(daemon, syscall.SIGSTOP); err != nil {
			t.Fatalf("stop daemon %d: %v", daemon, err)
		}
		rec := &harnesstest.Recorder{}
		start := time.Now()

		teardown(t.Context(), rec, b)

		if took := time.Since(start); took > 20*time.Second {
			t.Errorf("teardown took %s", took)
		}
		if !rec.Failed() || !strings.Contains(rec.Report(), "survived their kill") {
			t.Errorf("teardown reported %s, want the surviving daemon", rec.Report())
		}
		if alive(daemon) {
			_ = syscall.Kill(daemon, syscall.SIGKILL)
			t.Errorf("daemon %d survived teardown", daemon)
		}
	})

	t.Run("it finds and kills the processes of a directory zmx cannot list", func(t *testing.T) {
		b := New(t)
		mustCreate(t, b, "hidden")
		daemon := Daemon(t, b, "hidden")
		if err := os.Chmod(b.Dir(), 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(b.Dir(), 0o750) })
		rec := &harnesstest.Recorder{}

		teardown(t.Context(), rec, b)

		if !rec.Failed() {
			t.Error("teardown reported nothing for a directory it could not list")
		}
		if alive(daemon) {
			_ = syscall.Kill(daemon, syscall.SIGKILL)
			t.Errorf("daemon %d survived teardown", daemon)
		}
	})

	t.Run("it passes quietly over a session whose daemon has died", func(t *testing.T) {
		b := New(t)
		mustCreate(t, b, "dead")
		daemon := Daemon(t, b, "dead")
		if err := syscall.Kill(daemon, syscall.SIGKILL); err != nil {
			t.Fatal(err)
		}
		if !harnesstest.PollUntil(t, settle, 20*time.Millisecond, func() bool { return !alive(daemon) }) {
			t.Fatalf("daemon %d survived SIGKILL", daemon)
		}
		rec := &harnesstest.Recorder{}

		teardown(t.Context(), rec, b)

		if rec.Failed() {
			t.Errorf("teardown reported %s for a session that cannot answer and has no process left", rec.Report())
		}
	})

	t.Run("it passes quietly when every kill lands", func(t *testing.T) {
		b := New(t)
		mustCreate(t, b, "plain")
		daemon := Daemon(t, b, "plain")
		rec := &harnesstest.Recorder{}

		teardown(t.Context(), rec, b)

		if rec.Failed() {
			t.Errorf("teardown reported %s", rec.Report())
		}
		if alive(daemon) {
			t.Errorf("daemon %d survived teardown", daemon)
		}
	})
}

func TestOwned(t *testing.T) {
	t.Run("it finds the daemon, client and command of its own directory, and nothing of another's", func(t *testing.T) {
		a, b := New(t), New(t)
		for _, be := range []*zmx.Backend{a, b} {
			mustCreate(t, be, "same-name")
		}
		Attach(t, a, "same-name")
		sessions, err := a.List(t.Context())
		if err != nil || len(sessions) != 1 {
			t.Fatalf("List = %+v, %v", sessions, err)
		}
		daemon, command := Daemon(t, a, "same-name"), sessions[0].PID

		var got []int
		if !harnesstest.PollUntil(t, settle, 20*time.Millisecond, func() bool {
			got, err = owned(a.Dir())
			return err == nil && len(got) == 3
		}) {
			t.Fatalf("owned(%s) = %v, %v; want the daemon, the command and the attach client", a.Dir(), got, err)
		}
		if !slices.Contains(got, daemon) || !slices.Contains(got, command) {
			t.Errorf("owned(%s) = %v, want daemon %d and command %d among them", a.Dir(), got, daemon, command)
		}
		if other := Daemon(t, b, "same-name"); slices.Contains(got, other) {
			t.Errorf("owned(%s) = %v takes in %d, the daemon of %s", a.Dir(), got, other, b.Dir())
		}
	})

	t.Run("it finds a platform binary, whose environment macOS hides, through its daemon", func(t *testing.T) {
		b := New(t)
		mustCreateRunning(t, b, "platform", "/bin/sleep", "1000")
		sessions, err := b.List(t.Context())
		if err != nil || len(sessions) != 1 {
			t.Fatalf("List = %+v, %v", sessions, err)
		}
		daemon, command := Daemon(t, b, "platform"), sessions[0].PID

		var got []int
		if !harnesstest.PollUntil(t, settle, 20*time.Millisecond, func() bool {
			got, err = owned(b.Dir())
			return err == nil && len(got) == 2
		}) {
			t.Fatalf("owned(%s) = %v, %v; want the daemon and the command", b.Dir(), got, err)
		}
		if !slices.Contains(got, daemon) || !slices.Contains(got, command) {
			t.Errorf("owned(%s) = %v, want daemon %d and command %d", b.Dir(), got, daemon, command)
		}
	})
}

func TestStopDaemon(t *testing.T) {
	t.Run("it stops the daemon and continues it at cleanup, so teardown can kill it", func(t *testing.T) {
		b := New(t)
		mustCreate(t, b, "paused")
		var daemon int
		t.Run("inner", func(t *testing.T) {
			daemon = StopDaemon(t, b, "paused")
			sessions, err := b.List(t.Context())
			if err != nil || len(sessions) != 1 || sessions[0].Answered {
				t.Errorf("List of a stopped daemon = %+v, %v; want it unanswered", sessions, err)
			}
		})

		if !harnesstest.PollUntil(t, settle, 20*time.Millisecond, func() bool {
			sessions, err := b.List(t.Context())
			return err == nil && len(sessions) == 1 && sessions[0].Answered
		}) {
			t.Errorf("daemon %d still stopped after its test's cleanup", daemon)
		}
	})
}
