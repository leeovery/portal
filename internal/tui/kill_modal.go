package tui

import (
	"github.com/leeovery/portal/internal/theme"
)

const (
	killTitle       = "Kill session?"
	killConsequence = "Ends the session. Can't be undone."

	killKeyConfirm   = "y"
	killLabelConfirm = "kill"

	killKeyAttach   = "⏎"
	killLabelAttach = "attach"
)

// offersAttach adds Enter's choice, for the prompt the kill key asked for
// from inside the session: keep it and attach it again.
func renderKillModalContent(name string, offersAttach bool, th theme.Theme, colourless bool) string {
	spec := destructiveConfirmSpec{
		title:        killTitle,
		targetName:   name,
		consequence:  killConsequence,
		confirmKey:   killKeyConfirm,
		confirmLabel: killLabelConfirm,
	}
	if offersAttach {
		spec.keep = footerHintGroup{key: killKeyAttach, label: killLabelAttach}
	}
	return renderDestructiveConfirm(spec, th, colourless)
}
