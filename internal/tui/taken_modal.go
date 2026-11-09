package tui

import (
	"strings"
	"unicode"

	"github.com/leeovery/portal/internal/theme"
)

// Taken is a session another terminal took from this one, which the picker
// opens on: its name, and where it went, empty when that isn't known.
type Taken struct {
	Session string
	By      string
}

const (
	takenTitleGlyph = "⇄"
	takenTitle      = "Open on another device"
	takenByLead     = "Taken by "
	takenByEnd      = "."

	takenKeyBack     = "⏎"
	takenLabelBack   = "take it back"
	takenKeyWatch    = "space"
	takenLabelWatch  = "watch"
	takenLabelPicker = "picker"

	takenOverSSH   = "SSH from "
	takenOnThisMac = " on this Mac"
	takenUnknown   = "another terminal on this Mac"

	takenEndedGlyph   = "✕"
	takenEndedTitle   = "Session ended"
	takenEndedByLead  = "Ended after it was taken by "
	takenEndedUnknown = "It has ended."
	takenKeyQuit      = "q"
	takenLabelQuit    = "quit"
)

// appNames are the names TERM_PROGRAM gives terminal apps that differ from
// the apps' own.
var appNames = map[string]string{"ghostty": "Ghostty"}

// WhereFrom is where a session goes as this process takes it, in the words
// the screen of the terminal it was taken from shows, read from this
// process's environment through lookup: over SSH, the client's address; else
// the terminal app TERM_PROGRAM names, on this Mac; else another terminal on
// this Mac.
func WhereFrom(lookup func(string) (string, bool)) string {
	if conn, ok := lookup("SSH_CONNECTION"); ok {
		if fields := strings.Fields(printableOf(conn)); len(fields) > 0 {
			return takenOverSSH + fields[0]
		}
	}
	if app, ok := lookup("TERM_PROGRAM"); ok {
		if app = strings.TrimSpace(printableOf(app)); app != "" {
			if name, known := appNames[app]; known {
				app = name
			}
			return app + takenOnThisMac
		}
	}
	return takenUnknown
}

// printableOf is s without the characters a terminal would act on rather
// than show.
func printableOf(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) {
			return r
		}
		return -1
	}, s)
}

// takenModalSpec is the kill prompt's confirmation, titled as the help modal
// is and in no destructive colour, since nothing is lost, for the session
// another terminal took, shown as name in the accent: then where it went,
// by, when that is known. Enter takes it back, Space watches it, and Esc
// goes to the picker. Once the session has ended, it says so instead.
func takenModalSpec(name, by string, ended bool, th theme.Theme, colourless bool) destructiveConfirmSpec {
	lines := wrappedLines(name, destructiveBodyWidth)
	rows := make([]string, 0, len(lines))
	for _, line := range lines {
		rows = append(rows, headerStyle(th.AccentPrimary, th, colourless).Bold(true).Render(line))
	}
	if ended {
		return takenEndedSpec(rows, by, th, colourless)
	}
	spec := destructiveConfirmSpec{
		header:       modalTitle(takenTitleGlyph, takenTitle, th, colourless),
		targetRows:   rows,
		confirmKey:   takenKeyBack,
		confirmLabel: takenLabelBack,
		keep:         footerHintGroup{key: takenKeyWatch, label: takenLabelWatch},
		cancelLabel:  takenLabelPicker,
	}
	if by != "" {
		spec.consequence = takenByLead + by + takenByEnd
	}
	return spec
}

// takenEndedSpec is the taken modal once its session has ended, the session
// shown as rows: titled so, then that it ended after it was taken by where it
// went, which is all Portal can tell of how it ended. Enter or Esc goes to the
// picker, the hint naming Enter alone, and q quits.
func takenEndedSpec(rows []string, by string, th theme.Theme, colourless bool) destructiveConfirmSpec {
	spec := destructiveConfirmSpec{
		header:      modalTitle(takenEndedGlyph, takenEndedTitle, th, colourless),
		targetRows:  rows,
		consequence: takenEndedUnknown,
		hints:       []footerHintGroup{{takenKeyBack, takenLabelPicker}, {takenKeyQuit, takenLabelQuit}},
	}
	if by != "" {
		spec.consequence = takenEndedByLead + by + takenByEnd
	}
	return spec
}

func renderTakenModalOnClearedCanvas(name, by string, ended bool, width, height int, th theme.Theme, colourless bool) string {
	return placeModal(destructiveConfirmLayout(takenModalSpec(name, by, ended, th, colourless), th, colourless), 0, width, height, th, colourless)
}
