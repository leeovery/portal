package tui

import (
	"github.com/leeovery/portal/internal/theme"
)

const (
	killTitle       = "Kill session?"
	killConsequence = "Ends the session. Can't be undone."

	killKeyConfirm   = "y"
	killLabelConfirm = "kill"
)

func renderKillModalContent(name string, th theme.Theme, colourless bool) string {
	spec := destructiveConfirmSpec{
		title:        killTitle,
		targetName:   name,
		consequence:  killConsequence,
		confirmKey:   killKeyConfirm,
		confirmLabel: killLabelConfirm,
	}
	return renderDestructiveConfirm(spec, th, colourless)
}
