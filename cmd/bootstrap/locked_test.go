package bootstrap

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/commitlock"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/restore"
	"github.com/leeovery/portal/internal/saverhost"
	"github.com/leeovery/portal/internal/state"
)

// countingRunner counts its runs and answers each with warnings.
type countingRunner struct {
	runs     atomic.Int32
	warnings []Warning
}

func (r *countingRunner) Run(context.Context) ([]Warning, error) {
	r.runs.Add(1)
	return r.warnings, nil
}

// runnerFunc is a Runner of its own.
type runnerFunc func(context.Context) ([]Warning, error)

func (f runnerFunc) Run(ctx context.Context) ([]Warning, error) { return f(ctx) }

// lockIn is a Locked's Lock on bootstrap.lock in dir, waiting up to wait.
func lockIn(dir string, wait time.Duration) func(context.Context) (func(), error) {
	return func(ctx context.Context) (func(), error) { return state.AcquireBootstrapLock(ctx, dir, wait) }
}

// holdLock takes bootstrap.lock in dir as another process would, through an
// open file description of the test's own, until the release it returns runs
// or the test ends.
func holdLock(t *testing.T, dir string) func() {
	t.Helper()
	release, err := state.AcquireBootstrapLock(t.Context(), dir, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	return release
}

func TestLocked(t *testing.T) {
	t.Run("it runs the bootstrap owed once it holds the lock, then releases it", func(t *testing.T) {
		dir := t.TempDir()
		boot := &countingRunner{warnings: []Warning{SaverDownWarning()}}

		warnings, err := Locked{Lock: lockIn(dir, time.Second), Owed: func(context.Context) bool { return true }, Bootstrap: boot}.Run(t.Context())

		if err != nil || boot.runs.Load() != 1 || !equalWarnings(warnings, boot.warnings) {
			t.Errorf("Run = %v, %v after %d runs; want the bootstrap's warnings from one run", warnings, err, boot.runs.Load())
		}
		if _, err := state.AcquireBootstrapLock(t.Context(), dir, 0); err != nil {
			t.Errorf("the lock after the run: %v, want it released", err)
		}
	})

	t.Run("a second bootstrap waits for the first, then skips once none is owed", func(t *testing.T) {
		dir := t.TempDir()
		release := holdLock(t, dir)
		var owed atomic.Bool
		owed.Store(true)
		boot := &countingRunner{}
		sink := &logtest.Sink{}
		done := make(chan error, 1)
		go func() {
			_, err := Locked{Lock: lockIn(dir, 10*time.Second), Owed: func(context.Context) bool { return owed.Load() }, Bootstrap: boot, Logger: slog.New(sink)}.Run(t.Context())
			done <- err
		}()

		select {
		case err := <-done:
			t.Fatalf("Run returned %v while the first bootstrap held the lock", err)
		case <-time.After(200 * time.Millisecond):
		}
		owed.Store(false)
		release()

		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("Run = %v, want the skip", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("the second bootstrap never took the released lock")
		}
		if n := boot.runs.Load(); n != 0 {
			t.Errorf("the second bootstrap ran %d times, want it skipped", n)
		}
		sink.Records().WithMessage("bootstrap skipped").AtExactLevel(slog.LevelInfo).Only(t, "the skip")
	})

	t.Run("a lock held past its bound is a fatal saying so, and nothing runs", func(t *testing.T) {
		dir := t.TempDir()
		holdLock(t, dir)
		boot := &countingRunner{}

		_, err := Locked{Lock: lockIn(dir, 150*time.Millisecond), Owed: func(context.Context) bool { return true }, Bootstrap: boot}.Run(t.Context())

		fatal, ok := errors.AsType[*FatalError](err)
		if !ok || !errors.Is(err, state.ErrBootstrapLockHeld) {
			t.Fatalf("Run = %v, want a fatal wrapping the held lock", err)
		}
		want := "Portal gave up waiting for another Portal to finish starting: bootstrap.lock is held by another process for 150ms: " + state.BootstrapLock(dir)
		if fatal.UserMessage != want || fatal.Step != 0 {
			t.Errorf("fatal = %q at step %d, want %q at no step", fatal.UserMessage, fatal.Step, want)
		}
		if n := boot.runs.Load(); n != 0 {
			t.Errorf("the bootstrap ran %d times, want none", n)
		}
	})

	t.Run("a wait past its notice is told to the progress emitter once, before the bootstrap runs", func(t *testing.T) {
		dir := t.TempDir()
		release := holdLock(t, dir)
		var events []StepEvent
		ran := 0
		boot := runnerFunc(func(context.Context) ([]Warning, error) {
			ran = len(events)
			return nil, nil
		})
		ctx := WithProgressEmitter(t.Context(), func(ev StepEvent) { events = append(events, ev) })
		time.AfterFunc(300*time.Millisecond, release)

		_, err := Locked{Lock: lockIn(dir, 5*time.Second), Owed: func(context.Context) bool { return true }, Bootstrap: boot, Notice: 50 * time.Millisecond}.Run(ctx)

		if err != nil {
			t.Fatalf("Run = %v", err)
		}
		if want := []StepEvent{{Waiting: true}}; !slices.Equal(events, want) || ran != 1 {
			t.Errorf("events = %+v, %d before the bootstrap ran; want %+v, before it", events, ran, want)
		}
	})

	t.Run("a wait within its notice is told nothing", func(t *testing.T) {
		var events []StepEvent
		ctx := WithProgressEmitter(t.Context(), func(ev StepEvent) { events = append(events, ev) })

		_, err := Locked{Lock: lockIn(t.TempDir(), time.Second), Owed: func(context.Context) bool { return true }, Bootstrap: &countingRunner{}, Notice: time.Second}.Run(ctx)

		if err != nil || len(events) != 0 {
			t.Errorf("Run = %v, events = %+v; want none", err, events)
		}
	})

	t.Run("a lock that can't be taken at all is a fatal too", func(t *testing.T) {
		boot := &countingRunner{}
		failing := func(context.Context) (func(), error) { return nil, errBoom }

		_, err := Locked{Lock: failing, Owed: func(context.Context) bool { return true }, Bootstrap: boot}.Run(t.Context())

		if fatal, ok := errors.AsType[*FatalError](err); !ok || !strings.HasPrefix(fatal.UserMessage, "Portal failed to take bootstrap.lock: ") {
			t.Errorf("Run = %v, want a fatal naming the lock", err)
		}
		if n := boot.runs.Load(); n != 0 {
			t.Errorf("the bootstrap ran %d times, want none", n)
		}
	})
}

// decision 78: x <name> brings a kept session back under bootstrap.lock, so
// it never races a bootstrap's restore; it takes the lock as a bootstrap does.
func TestLocked_Hold(t *testing.T) {
	t.Run("it holds the lock until its release, reading nothing but the lock", func(t *testing.T) {
		dir := t.TempDir()

		release, err := Locked{Lock: lockIn(dir, time.Second)}.Hold(t.Context())

		if err != nil {
			t.Fatalf("Hold = %v", err)
		}
		if _, err := state.AcquireBootstrapLock(t.Context(), dir, 0); !errors.Is(err, state.ErrBootstrapLockHeld) {
			t.Errorf("the lock while held = %v, want it held", err)
		}
		release()
		if _, err := state.AcquireBootstrapLock(t.Context(), dir, 0); err != nil {
			t.Errorf("the lock after its release = %v, want it free", err)
		}
	})

	t.Run("a wait past its notice is told to the progress emitter once", func(t *testing.T) {
		dir := t.TempDir()
		time.AfterFunc(300*time.Millisecond, holdLock(t, dir))
		var events []StepEvent
		ctx := WithProgressEmitter(t.Context(), func(ev StepEvent) { events = append(events, ev) })

		release, err := Locked{Lock: lockIn(dir, 5*time.Second), Notice: 50 * time.Millisecond}.Hold(ctx)

		if err != nil {
			t.Fatalf("Hold = %v", err)
		}
		release()
		if want := []StepEvent{{Waiting: true}}; !slices.Equal(events, want) {
			t.Errorf("events = %+v, want %+v", events, want)
		}
	})

	t.Run("a lock held past its bound is the fatal a bootstrap's wait gives", func(t *testing.T) {
		dir := t.TempDir()
		holdLock(t, dir)
		sink := &logtest.Sink{}

		release, err := Locked{Lock: lockIn(dir, 100*time.Millisecond), Logger: slog.New(sink)}.Hold(t.Context())

		fatal, ok := errors.AsType[*FatalError](err)
		if !ok || release != nil || !strings.HasPrefix(fatal.UserMessage, "Portal gave up waiting for another Portal to finish starting: ") {
			t.Errorf("Hold = %v, want the held lock's fatal and no release", err)
		}
		sink.Records().WithMessage("bootstrap.lock unavailable").AtExactLevel(slog.LevelError).Only(t, "the held lock")
	})
}

func TestLockWait(t *testing.T) {
	t.Run("it outlasts the saver step's own wait and run", func(t *testing.T) {
		if bound, saver := LockWait(0), 2*saverhost.LockWait; bound <= saver {
			t.Errorf("LockWait(0) = %s, want past the saver step's %s", bound, saver)
		}
	})

	// After its Create, a session's restore runs the longer of the Create's
	// refusal of an older zmx's session, a read and a zmx kill, and the second
	// ask of a Create that read back no creation or failed otherwise: a read,
	// backend.RecheckAfter's wait and a read again.
	refusal := saverhost.ReadCeiling + saverhost.KillCeiling + saverhost.RunGrace
	askAgain := 2*saverhost.ReadCeiling + backend.RecheckAfter
	each := saverhost.CreateCeiling + saverhost.RunGrace + max(refusal, askAgain) + commitlock.Bound()

	t.Run("it grows by a session's longest restore for each saved entry", func(t *testing.T) {
		if got := LockWait(20) - LockWait(0); got != 20*each {
			t.Errorf("LockWait(20) - LockWait(0) = %s, want 20 × %s", got, each)
		}
	})

	t.Run("it holds restore's retries, and the lookup and try that start as their budget ends, as one term", func(t *testing.T) {
		rest := checkRun + sweepRun + saverRun + commitlock.Bound() + restoreRun + lockSlack
		if got, want := LockWait(0)-rest, restore.RetryBudget+saverhost.ReadCeiling+each; got != want {
			t.Errorf("LockWait(0) less the steps before restore's sessions = %s, want %s", got, want)
		}
	})
}
