package tmux_test

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/tmux"
)

func TestWaitForSessions(t *testing.T) {
	t.Run("returns after min wait when sessions appear before min wait", func(t *testing.T) {
		cfg := tmux.WaitConfig{
			MinWait:      50 * time.Millisecond,
			MaxWait:      200 * time.Millisecond,
			PollInterval: 10 * time.Millisecond,
			HasSessions:  func() bool { return true }, // sessions immediately available
		}

		start := time.Now()
		tmux.WaitForSessions(cfg)
		elapsed := time.Since(start)

		if elapsed < cfg.MinWait {
			t.Errorf("returned in %v, want at least %v", elapsed, cfg.MinWait)
		}
		// Should return close to MinWait, not MaxWait
		if elapsed > cfg.MinWait+30*time.Millisecond {
			t.Errorf("returned in %v, want close to MinWait %v", elapsed, cfg.MinWait)
		}
	})

	t.Run("returns at max wait when no sessions ever appear", func(t *testing.T) {
		cfg := tmux.WaitConfig{
			MinWait:      50 * time.Millisecond,
			MaxWait:      200 * time.Millisecond,
			PollInterval: 10 * time.Millisecond,
			HasSessions:  func() bool { return false }, // sessions never appear
		}

		start := time.Now()
		tmux.WaitForSessions(cfg)
		elapsed := time.Since(start)

		if elapsed < cfg.MaxWait {
			t.Errorf("returned in %v, want at least %v", elapsed, cfg.MaxWait)
		}
		if elapsed > cfg.MaxWait+30*time.Millisecond {
			t.Errorf("returned in %v, want close to MaxWait %v", elapsed, cfg.MaxWait)
		}
	})

	t.Run("exits early when sessions appear between min and max", func(t *testing.T) {
		appearsAt := 100 * time.Millisecond
		start := time.Now()

		cfg := tmux.WaitConfig{
			MinWait:      50 * time.Millisecond,
			MaxWait:      200 * time.Millisecond,
			PollInterval: 10 * time.Millisecond,
			HasSessions: func() bool {
				return time.Since(start) >= appearsAt
			},
		}

		tmux.WaitForSessions(cfg)
		elapsed := time.Since(start)

		if elapsed < appearsAt {
			t.Errorf("returned in %v, want at least %v (session appearance time)", elapsed, appearsAt)
		}
		// Should exit shortly after sessions appear, not wait until MaxWait
		if elapsed > appearsAt+30*time.Millisecond {
			t.Errorf("returned in %v, want close to %v (session appearance time)", elapsed, appearsAt)
		}
	})

	t.Run("polls at the configured interval", func(t *testing.T) {
		var pollCount atomic.Int32

		cfg := tmux.WaitConfig{
			MinWait:      50 * time.Millisecond,
			MaxWait:      200 * time.Millisecond,
			PollInterval: 10 * time.Millisecond,
			HasSessions: func() bool {
				pollCount.Add(1)
				return false
			},
		}

		tmux.WaitForSessions(cfg)
		count := pollCount.Load()

		// With 200ms max and 10ms interval, expect roughly 20 polls.
		// Allow some tolerance for timing.
		expectedMin := int32(15)
		expectedMax := int32(25)
		if count < expectedMin || count > expectedMax {
			t.Errorf("poll count = %d, want between %d and %d", count, expectedMin, expectedMax)
		}
	})

	t.Run("sessions detected on first poll still waits for min wait", func(t *testing.T) {
		var pollCount atomic.Int32

		cfg := tmux.WaitConfig{
			MinWait:      50 * time.Millisecond,
			MaxWait:      200 * time.Millisecond,
			PollInterval: 10 * time.Millisecond,
			HasSessions: func() bool {
				pollCount.Add(1)
				return true // always returns true from the first call
			},
		}

		start := time.Now()
		tmux.WaitForSessions(cfg)
		elapsed := time.Since(start)

		if elapsed < cfg.MinWait {
			t.Errorf("returned in %v, want at least %v", elapsed, cfg.MinWait)
		}
		// Verify it polled (at least once)
		if pollCount.Load() < 1 {
			t.Error("expected at least 1 poll call")
		}
	})
}
