package state_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/state"
)

func TestAcquireBootstrapLock(t *testing.T) {
	t.Run("it creates the state directory and takes the lock there", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "state")

		release, err := state.AcquireBootstrapLock(t.Context(), dir, time.Second)
		if err != nil {
			t.Fatalf("AcquireBootstrapLock: %v", err)
		}
		defer release()

		if _, err := state.AcquireBootstrapLock(t.Context(), dir, 0); !errors.Is(err, state.ErrBootstrapLockHeld) {
			t.Errorf("a second acquire = %v, want the lock held", err)
		}
	})

	t.Run("a second acquire waits out the holder, then takes the lock", func(t *testing.T) {
		dir := t.TempDir()
		release, err := state.AcquireBootstrapLock(t.Context(), dir, time.Second)
		if err != nil {
			t.Fatalf("AcquireBootstrapLock: %v", err)
		}
		got := make(chan error, 1)
		go func() {
			second, err := state.AcquireBootstrapLock(t.Context(), dir, 10*time.Second)
			if err == nil {
				second()
			}
			got <- err
		}()

		select {
		case err := <-got:
			t.Fatalf("the second acquire returned %v while the first held the lock", err)
		case <-time.After(200 * time.Millisecond):
		}
		release()
		select {
		case err := <-got:
			if err != nil {
				t.Errorf("the second acquire = %v, want the lock once released", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("the second acquire never took the released lock")
		}
	})

	t.Run("a lock held past the bound is the held error, naming the bound and the lock", func(t *testing.T) {
		dir := t.TempDir()
		release, err := state.AcquireBootstrapLock(t.Context(), dir, time.Second)
		if err != nil {
			t.Fatalf("AcquireBootstrapLock: %v", err)
		}
		defer release()

		_, err = state.AcquireBootstrapLock(t.Context(), dir, 150*time.Millisecond)

		if !errors.Is(err, state.ErrBootstrapLockHeld) {
			t.Fatalf("err = %v, want ErrBootstrapLockHeld", err)
		}
		for _, want := range []string{"150ms", state.BootstrapLock(dir)} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("err = %q, want it to name %q", err, want)
			}
		}
	})

	t.Run("a wait whose context ends fails with its cause", func(t *testing.T) {
		dir := t.TempDir()
		release, err := state.AcquireBootstrapLock(t.Context(), dir, time.Second)
		if err != nil {
			t.Fatalf("AcquireBootstrapLock: %v", err)
		}
		defer release()
		cause := errors.New("the command ended")
		ctx, cancel := context.WithCancelCause(t.Context())
		cancel(cause)

		if _, err := state.AcquireBootstrapLock(ctx, dir, time.Minute); !errors.Is(err, cause) {
			t.Errorf("err = %v, want the context's cause", err)
		}
	})
}
