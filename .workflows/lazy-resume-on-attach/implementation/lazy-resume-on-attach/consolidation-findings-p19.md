# Consolidation Findings: lazy-resume-on-attach (Phase 19)

## Findings

None survived the bar.

## Comment Corrections

- internal/tui/pane_appearance.go:90-91: the phase's draw now turns echo off before `ResolvePaneTheme` (`cmd/state_resume_draw.go:45`), and that draw is the probe's only production caller (`cmd/state_resume_draw.go:100`). So the probe's raw-mode restore puts back an echo-off tty, and a reply arriving after it is queued as input, not echoed. The comment's stated consequence is false.
  OLD: // Armed before the query is written: a query with no bounded read behind it
	// leaves the terminal's reply to be echoed across the pane once raw mode ends.
  NEW: // Armed before the query is written: a query with no bounded read behind it
	// leaves the terminal's reply on the pane's input for whatever reads it next.
