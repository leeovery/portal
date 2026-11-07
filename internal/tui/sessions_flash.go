package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// The zero value is flashWarning, so an unparameterised setFlash stays a warning.
type flashKind int

const (
	flashWarning flashKind = iota
	flashSuccess
)

// Carried by the flash so a theme signal's precedence tier is granted at set time
// rather than inferred from message text, which a copy edit would silently move a
// signal out of. What the tier does and does not arbitrate is on flashSlotClaim.
type flashOrigin int

const (
	flashOriginDefault flashOrigin = iota
	flashOriginTheme
)

const flashAutoClearDuration = 3 * time.Second

// Gen is the flashGen captured when the tick was scheduled, compared against the
// live value so a superseded flash's tick cannot early-clear the current one.
type flashTickMsg struct {
	Gen uint64
}

func flashTickCmd(gen uint64) tea.Cmd {
	return tea.Tick(flashAutoClearDuration, func(time.Time) tea.Msg {
		return flashTickMsg{Gen: gen}
	})
}

// The zero-zero shape (no Code, no Text) is non-actionable — a guard against
// library-emitted no-op key events; every real keystroke satisfies one condition.
func isActionableKey(msg tea.KeyPressMsg) bool {
	return msg.Code != 0 || msg.Text != ""
}
