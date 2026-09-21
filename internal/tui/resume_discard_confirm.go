package tui

const (
	discardConfirmTitle = "Discard resume?"
	// Stated verbatim by the specification. It names no tool and is not
	// paraphrased here: it is the only copy standing between a keypress and the
	// loss of a user-authored command.
	discardConfirmConsequence = "Removes this pane's resume command permanently. The session and its scrollback are untouched."

	discardKeyConfirm   = "y"
	discardLabelConfirm = "discard"
)

// RenderResumeDiscardConfirm draws the discard confirmation as a string of
// exactly Width by Height cells. It is the picker's kill modal retitled, built
// through the same destructive-confirm builder, and it degrades with the pane
// exactly as the waiting panel does.
func RenderResumeDiscardConfirm(s ResumeScreen) string {
	parts := paneScreenParts{
		card:  func() string { return discardConfirmCard(s) },
		stack: func(width int) []string { return discardConfirmStack(s, width) },
	}
	return renderPaneScreen(parts, s.Width, s.Height, s.Theme, s.Colourless)
}

func discardConfirmCard(s ResumeScreen) string {
	return renderDestructiveConfirm(discardConfirmSpec(s, resumeCardContentWidth), s.Theme, s.Colourless)
}

// The frame's own compartments, flattened, with the command, report and
// consequence built at the pane's width rather than the card's.
func discardConfirmStack(s ResumeScreen, width int) []string {
	compartments := destructiveConfirmCompartments(discardConfirmSpec(s, width), width, s.Theme, s.Colourless)
	rows := make([]string, 0, len(compartments))
	for _, compartment := range compartments {
		rows = append(rows, compartment...)
	}
	return rows
}

func discardConfirmSpec(s ResumeScreen, width int) destructiveConfirmSpec {
	spec := destructiveConfirmSpec{
		title:        discardConfirmTitle,
		targetRows:   resumeCommandRows(s.Command, width, s.Theme.StateDestructive, true, s.Theme, s.Colourless),
		consequence:  discardConfirmConsequence,
		confirmKey:   discardKeyConfirm,
		confirmLabel: discardLabelConfirm,
	}
	if row, ok := resumeReportRow(s.Report, width, s.Theme, s.Colourless); ok {
		spec.reportRows = []string{row}
	}
	return spec
}
