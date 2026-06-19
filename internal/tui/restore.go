package tui

import (
	"io"

	"github.com/charmbracelet/x/ansi"
)

// RestoreTerminalBackground restores the terminal's original background colour
// after the Bubble Tea program has returned, by explicitly SETTING the captured
// original back via OSC 11 (ansi.SetBackgroundColor emits ESC]11;<original>).
//
// Why set-back instead of relying on the reset: the owned canvas paint (§ owned
// canvas) sets the terminal's default background via OSC 11 so the canvas
// extends into the window gutter. Bubble Tea restores on exit via OSC 111
// (ResetBackgroundColor), but some terminals (mosh/Blink) IGNORE the reset — so
// the canvas colour sticks after Portal quits. Those same terminals DO honour
// the OSC 11 set, so writing the captured original back reliably restores them.
//
// It is best-effort and idempotent on the no-capture path: when the model never
// received an OSC 11 response (OriginalBackground() == ""), it writes nothing
// and lets Bubble Tea's own OSC 111 reset stand. Any write error is ignored —
// terminal restoration must never fail the caller's exit path.
//
// Both program-launch sites (cmd/capturetool and cmd/open.go) call this with the
// program's output writer after p.Run() returns, so they restore identically.
func RestoreTerminalBackground(w io.Writer, m Model) {
	original := m.OriginalBackground()
	if original == "" {
		return
	}
	_, _ = io.WriteString(w, ansi.SetBackgroundColor(original))
}
