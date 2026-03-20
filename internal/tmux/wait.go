package tmux

import "time"

// Default timing constants for WaitForSessions.
const (
	DefaultMinWait      = 1 * time.Second
	DefaultMaxWait      = 6 * time.Second
	DefaultPollInterval = 500 * time.Millisecond
)

// WaitConfig controls the polling behavior of WaitForSessions.
type WaitConfig struct {
	// MinWait is the minimum duration to wait before returning, even if sessions appear immediately.
	MinWait time.Duration
	// MaxWait is the maximum duration to wait before returning, even if no sessions appear.
	MaxWait time.Duration
	// PollInterval is how often HasSessions is called.
	PollInterval time.Duration
	// HasSessions reports whether any tmux sessions currently exist.
	HasSessions func() bool
}

// DefaultWaitConfig returns a WaitConfig with default timing that uses
// the given client's ListSessions to check for session presence.
func DefaultWaitConfig(client *Client) WaitConfig {
	return WaitConfig{
		MinWait:      DefaultMinWait,
		MaxWait:      DefaultMaxWait,
		PollInterval: DefaultPollInterval,
		HasSessions: func() bool {
			sessions, err := client.ListSessions()
			return err == nil && len(sessions) > 0
		},
	}
}

// WaitForSessions polls for tmux sessions to appear within the configured timing bounds.
// It always waits at least MinWait. After MinWait, it returns as soon as sessions are
// detected. It always returns by MaxWait regardless of session state.
func WaitForSessions(cfg WaitConfig) {
	sessionsFound := false
	ticker := time.NewTicker(cfg.PollInterval)
	defer ticker.Stop()

	deadline := time.NewTimer(cfg.MaxWait)
	defer deadline.Stop()

	minTimer := time.NewTimer(cfg.MinWait)
	defer minTimer.Stop()

	minElapsed := false

	for {
		select {
		case <-deadline.C:
			return
		case <-minTimer.C:
			minElapsed = true
			if sessionsFound {
				return
			}
		case <-ticker.C:
			if cfg.HasSessions() {
				sessionsFound = true
				if minElapsed {
					return
				}
			}
		}
	}
}
