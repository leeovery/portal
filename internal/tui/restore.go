package tui

import (
	"io"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/tui/theme"
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
// CANVAS-ECHO GUARD. The §2.6 OSC 11 query is async and non-gating, so it RACES
// the canvas set: on some terminals (timing-dependent — a heavier first render
// widens the window) the OSC 11 reply reflects the background AFTER our canvas
// set landed, and OriginalBackground() comes back as the canvas colour itself.
// Setting that back would re-paint the canvas AFTER Bubble Tea's OSC 111 reset,
// leaving the owned canvas stuck (the no-tags-signpost capture exposed this in
// Ghostty). So when the captured "original" resolves to the canvas we set, skip
// the set-back and let the OSC 111 reset stand (it restores correctly). Terminals
// that ignore OSC 111 report a genuine original — not the canvas — so they still
// get the set-back.
//
// Both program-launch sites (cmd/capturetool and cmd/open.go) call this with the
// program's output writer after p.Run() returns, so they restore identically.
func RestoreTerminalBackground(w io.Writer, m Model) {
	original := m.OriginalBackground()
	if original == "" {
		return
	}
	if sameHexColour(original, canvasHexFor(m.canvasMode)) {
		return
	}
	_, _ = io.WriteString(w, ansi.SetBackgroundColor(original))
}

// canvasHexFor returns the owned-canvas hex the model painted for the resolved
// appearance — the exact value RestoreTerminalBackground must NOT echo back. It
// mirrors theme.Token.ColorFor's mode branch for the canvas token.
func canvasHexFor(mode theme.Mode) string {
	if mode == theme.Light {
		return theme.MV.Canvas.Light
	}
	return theme.MV.Canvas.Dark
}

// sameHexColour reports whether two OSC-11-style colour strings denote the same
// RGB triple, tolerant of case, a leading '#', surrounding space, and a trailing
// alpha pair. A non-hex value (e.g. an `rgb:` reply) returns false, so the caller
// falls back to emitting the set-back unchanged — never worse than before.
func sameHexColour(a, b string) bool {
	na, oka := normaliseHex6(a)
	nb, okb := normaliseHex6(b)
	return oka && okb && na == nb
}

// normaliseHex6 reduces a colour string to its lower-case 6-hex-digit RGB body,
// stripping surrounding space, a leading '#', and any trailing alpha. It reports
// false for anything that is not at least six leading hex digits.
func normaliseHex6(s string) (string, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "#")
	if len(s) < 6 {
		return "", false
	}
	s = s[:6]
	for i := 0; i < len(s); i++ {
		if !isHexDigit(s[i]) {
			return "", false
		}
	}
	return s, true
}

// isHexDigit reports whether c is a lower-case hex digit (0-9, a-f).
func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')
}
