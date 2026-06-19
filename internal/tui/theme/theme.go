// Package theme is the single centralised colour-token layer for the TUI. It
// declares the closed Modern Vivid (MV) vocabulary — the ~20 named semantic
// role tokens of specification.md §2.9 — with their pinned DARK hexes. Every
// renderer references a token here; no raw hex / ANSI-index colour literal
// survives at a call site (the rule that makes §2.8 theme-readiness work).
//
// Each token carries BOTH a Light and a Dark variant so the light-variant task
// (1-4) can fill the Light values without re-pointing any call site. Resolution
// currently defaults to the DARK variant — mirroring the dark-default the 1-2
// AdaptiveColor migration produced in the absence of OSC 11 light/dark
// detection. Detection (§2.6) lands in 1-7; until then ColorFor(Dark) is what
// every Color() call resolves to.
package theme

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// Mode is the resolved light/dark appearance the canvas is painted for. Until
// OSC 11 detection lands (1-7), the resolver defaults to Dark.
type Mode int

const (
	// Dark is the default appearance — MV is dark-first and the §2.6 no-answer
	// fallback resolves to the dark canvas. The zero value is Dark on purpose so
	// an unconfigured resolver paints the dark canvas it was tuned for.
	Dark Mode = iota
	// Light is the mode-matched near-white canvas appearance. Its variants are
	// filled by task 1-4 and wired to detection by task 1-7.
	Light
)

// Token is one semantic role colour in the closed MV vocabulary. It carries a
// Light and a Dark hex (or ANSI-index) variant; Dark is pinned to §2.9 now,
// Light is a placeholder filled by task 1-4. Name is the §2.9 token name, used
// only by All() consumers (tests, future theme tooling) — never at a render
// call site.
type Token struct {
	Name  string
	Light string
	Dark  string
}

// ColorFor resolves the token to a lipgloss colour for the given mode. The two
// variants resolve independently (§2.9): Light is measured only against the
// light canvas, Dark only against the dark canvas. The value flows through
// lipgloss.Color, so NO_COLOR suppression and palette downsample still apply.
func (t Token) ColorFor(m Mode) color.Color {
	if m == Light {
		return lipgloss.Color(t.Light)
	}
	return lipgloss.Color(t.Dark)
}

// Color resolves the token to its current default appearance. Until light/dark
// detection lands (1-7) this is always the DARK variant — the parity-preserving
// dark-default the 1-2 AdaptiveColor migration established. Renderers call this;
// when detection ships, this single resolver flips and every call site follows
// with no edit.
func (t Token) Color() color.Color {
	return t.ColorFor(Dark)
}

// Theme is the closed set of named MV role tokens. One built-in instance (MV)
// is exported; a user-overridable theme system is deferred to its own
// initiative (§2.8 / §16).
type Theme struct {
	// Text ramp (bright → faint).
	TextPrimary     Token // names, wordmark, active labels, modal titles, chip text
	TextStrong      Token // selected-row meta, help actions, banner/signpost
	TextMutedBright Token // done-tick labels, selected-row path
	TextDetail      Token // paths, counts, footer labels, subtitles, group headings
	TextDim         Token // group ··· N counts, pending loading steps
	TextFaint       Token // decorative only — inactive dots, + add, mode indicator, hints
	TextOnSelection Token // name on the selected row

	// Accents.
	AccentViolet Token // cursor, selector bar, active dot, ? key, focused field label, mode bar, loading bar
	AccentBlue   Token // footer / modal key-hint glyphs
	AccentCyan   Token // Sessions header, Preview chrome, active tick
	StateGreen   Token // ● attached, Sessions count, Projects label, ✓ done, success flash
	StateRed     Token // kill/delete emphasis, ▲
	AccentOrange Token // filter query / / / type, warning flash ⚠

	// Surfaces (tints / borders).
	Canvas          Token // owned mode-matched canvas (painted on every cell)
	BgSelection     Token // selected-row tint
	BgWarning       Token // warning-flash band
	BgTrack         Token // loading-bar empty track
	BorderSeparator Token // title rule (2px)
	BorderFooter    Token // footer rule (1px)
	TextOnWarning   Token // warning-flash message
}

// All returns every token in the theme in a stable order. Used by the closed
// vocabulary count guard and future theme tooling — never at a render call
// site. The slice length is the canonical ~20-token count of §2.9.
func (t Theme) All() []Token {
	return []Token{
		t.TextPrimary,
		t.TextStrong,
		t.TextMutedBright,
		t.TextDetail,
		t.TextDim,
		t.TextFaint,
		t.TextOnSelection,
		t.AccentViolet,
		t.AccentBlue,
		t.AccentCyan,
		t.StateGreen,
		t.StateRed,
		t.AccentOrange,
		t.Canvas,
		t.BgSelection,
		t.BgWarning,
		t.BgTrack,
		t.BorderSeparator,
		t.BorderFooter,
		t.TextOnWarning,
	}
}

// MV is the built-in Modern Vivid theme. DARK variants are pinned exactly to the
// §2.9 token table (the authoritative hex source). Light variants are empty
// placeholders — task 1-4 fills them and 1-7 wires the resolver; neither edits
// any call site, because every renderer already references these tokens.
var MV = Theme{
	// Text ramp.
	TextPrimary:     Token{Name: "text.primary", Dark: "#C0CAF5"},
	TextStrong:      Token{Name: "text.strong", Dark: "#A9B1D6"},
	TextMutedBright: Token{Name: "text.muted-bright", Dark: "#828BB8"},
	TextDetail:      Token{Name: "text.detail", Dark: "#737AA2"},
	TextDim:         Token{Name: "text.dim", Dark: "#535C86"},
	TextFaint:       Token{Name: "text.faint", Dark: "#3B4261"},
	TextOnSelection: Token{Name: "text.on-selection", Dark: "#FFFFFF"},

	// Accents.
	AccentViolet: Token{Name: "accent.violet", Dark: "#BB9AF7"},
	AccentBlue:   Token{Name: "accent.blue", Dark: "#7AA2F7"},
	AccentCyan:   Token{Name: "accent.cyan", Dark: "#7DCFFF"},
	StateGreen:   Token{Name: "state.green", Dark: "#9ECE6A"},
	StateRed:     Token{Name: "state.red", Dark: "#F7768E"},
	AccentOrange: Token{Name: "accent.orange", Dark: "#FF9E64"},

	// Surfaces.
	Canvas:          Token{Name: "canvas", Dark: "#0b0c14"},
	BgSelection:     Token{Name: "bg.selection", Dark: "#28243a"},
	BgWarning:       Token{Name: "bg.warning", Dark: "#241B10"},
	BgTrack:         Token{Name: "bg.track", Dark: "#26283A"},
	BorderSeparator: Token{Name: "border.separator", Dark: "#292E42"},
	BorderFooter:    Token{Name: "border.footer", Dark: "#20232E"},
	TextOnWarning:   Token{Name: "text.on-warning", Dark: "#E8C9A0"},
}
