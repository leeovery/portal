package cmd

import (
	"log/slog"
	"time"

	"github.com/leeovery/portal/internal/tmux"
)

// altScreenOption is pinned on a waiting pane so the panel sits on an
// alternate screen whatever the install sets; alternateOnFormat is the format
// variable tmux answers 1 or 0 for whether a pane is showing one.
const (
	altScreenOption   = "alternate-screen"
	alternateOnFormat = "alternate_on"
)

// A leave is confirmed within altScreenLeaveAttempts reads spaced
// altScreenLeavePoll apart. The parked chain's backstop polls on the same
// pair.
const (
	altScreenLeavePoll     = 50 * time.Millisecond
	altScreenLeaveAttempts = 20
)

// altScreenPin reaches the pane's pin through tmux: AlternateOn reads the
// pane's #{alternate_on}, Unpin unsets its pane-level alternate-screen.
type altScreenPin struct {
	AlternateOn func() (string, error)
	Unpin       func() error
	Pause       func(time.Duration)
}

// releaseAltScreenPin hands the pane back to its own alternate-screen setting
// once tmux reports it off the alternate screen. The order is load-bearing: an
// unset tmux processes before the leave bytes makes it ignore the leave,
// stranding the panel over the transcript, so a leave never confirmed keeps the
// pin. Neither failure holds back the hand-over.
func releaseAltScreenPin(logger *slog.Logger, paneKey string, pin altScreenPin) {
	if !awaitPrimaryScreen(pin) {
		logger.Warn("alternate screen leave unconfirmed", "pane_key", paneKey)
		return
	}
	if err := pin.Unpin(); err != nil {
		logger.Warn("unset alternate-screen pin failed", "pane_key", paneKey, "error", err)
	}
}

func awaitPrimaryScreen(pin altScreenPin) bool {
	for attempt := range altScreenLeaveAttempts {
		if attempt > 0 {
			pin.Pause(altScreenLeavePoll)
		}
		if on, err := pin.AlternateOn(); err == nil && on == "0" {
			return true
		}
	}
	return false
}

func paneAltScreenPin(client *tmux.Client, pane tmux.Target) altScreenPin {
	return altScreenPin{
		AlternateOn: func() (string, error) { return client.ReadPaneOption(pane, alternateOnFormat) },
		Unpin:       func() error { return client.UnsetPaneOption(pane, altScreenOption) },
		Pause:       time.Sleep,
	}
}
