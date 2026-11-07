package tui

import (
	"github.com/leeovery/portal/internal/theme"
)

const (
	killTitle       = "Kill session?"
	killConsequence = "Ends the session. Can't be undone."

	killKeyConfirm   = "y"
	killLabelConfirm = "kill"

	killKeyDetach   = "d"
	killLabelDetach = "detach"
)

func renderKillModalContent(name string, fromInside bool, th theme.Theme, colourless bool) string {
	return renderDestructiveConfirm(killModalSpec(name, fromInside), th, colourless)
}

// fromInside is the prompt the kill key asked for from inside the session,
// whose cancel goes back into it: d's choice is added, keeping it and staying
// in the picker.
func killModalSpec(name string, fromInside bool) destructiveConfirmSpec {
	spec := destructiveConfirmSpec{
		title:        killTitle,
		targetName:   name,
		consequence:  killConsequence,
		confirmKey:   killKeyConfirm,
		confirmLabel: killLabelConfirm,
	}
	if fromInside {
		spec.keep = footerHintGroup{key: killKeyDetach, label: killLabelDetach}
	}
	return spec
}
