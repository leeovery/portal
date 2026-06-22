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
// §2.9 token table (the authoritative hex source). LIGHT variants are filled by
// task 1-4 (this task) and measured against the owned light canvas `#e1e2e7`
// (§2.3 / §2.9); 1-7 wires the resolver. Neither edits any call site, because
// every renderer already references these tokens.
//
// §2.9-erratum note (user-approved 2026-06-19). §2.9's published light *ratio*
// column was computed against pure white `#FFFFFF`, but §2.3 / §2.9 mandate
// measuring each light variant against the owned light canvas `#e1e2e7`. Six
// light foreground hexes were under-floor vs `#e1e2e7` under the original §2.9
// values; they are corrected below (darkened, hue-preserved) so each clears its
// floor vs `#e1e2e7`. Each correction carries an inline erratum comment recording
// original → corrected and the measured ratio vs `#e1e2e7`. The light surface
// tints are PINNED at the 1-9 in-terminal validation gate (the
// light-tint-on-light-canvas case is numeric-insufficient, so the human eyeball
// confirms each against `#e1e2e7`, §2.9 / §15.6); each carries an inline
// derivation comment (dark anchor + surface).
var MV = Theme{
	// Text ramp (light variants vs `#e1e2e7`).
	TextPrimary: Token{Name: "text.primary", Dark: "#C0CAF5", Light: "#2E3C64"},
	TextStrong:  Token{Name: "text.strong", Dark: "#A9B1D6", Light: "#3F4760"},
	// §2.9: light darkened #515A80 → #4C5478 so the selected-row path clears the 4.5
	// floor on bg.selection (#D0C6F0 = 4.57; it was 4.17) as well as the canvas (5.70).
	TextMutedBright: Token{Name: "text.muted-bright", Dark: "#828BB8", Light: "#4C5478"},
	// §2.9 erratum: light #5A6296 → #586093 (4.63 vs #e1e2e7).
	TextDetail: Token{Name: "text.detail", Dark: "#737AA2", Light: "#586093"},
	// §2.9 erratum: light #7C84AA → #767DA2 (3.11 vs #e1e2e7, 3:1 floor).
	TextDim:         Token{Name: "text.dim", Dark: "#535C86", Light: "#767DA2"},
	TextFaint:       Token{Name: "text.faint", Dark: "#3B4261", Light: "#AEB2C6"},
	TextOnSelection: Token{Name: "text.on-selection", Dark: "#FFFFFF", Light: "#1A1B2E"},

	// Accents (light variants vs `#e1e2e7`).
	AccentViolet: Token{Name: "accent.violet", Dark: "#BB9AF7", Light: "#8A3FD1"},
	// §2.9 erratum: light #2E5FD0 → #2D5CCA (4.64 vs #e1e2e7).
	AccentBlue: Token{Name: "accent.blue", Dark: "#7AA2F7", Light: "#2D5CCA"},
	// §2.9 erratum: light #0E7490 → #0D6C87 (4.62 vs #e1e2e7).
	AccentCyan: Token{Name: "accent.cyan", Dark: "#7DCFFF", Light: "#0D6C87"},
	// §2.9 erratum: light #4C7A1F → #456E1C; then darkened to #3B5E18 so the SINGLE
	// state.green token clears the floor BOTH on the canvas (#3B5E18 vs #e1e2e7 >
	// 4.64, raises the prior margin) AND on the bg.selection tint (#3B5E18 vs #D0C6F0
	// = 4.65). This folds the former light-only on-selection remedy into the global
	// token — the `● attached` marker uses state.green on the selected row too, no
	// per-context override (themeable as one green). Dark #9ECE6A clears everywhere.
	StateGreen: Token{Name: "state.green", Dark: "#9ECE6A", Light: "#3B5E18"},
	// §2.9 erratum: light #C32647 → #BD2545 (4.62 vs #e1e2e7).
	StateRed:     Token{Name: "state.red", Dark: "#F7768E", Light: "#BD2545"},
	AccentOrange: Token{Name: "accent.orange", Dark: "#FF9E64", Light: "#9A5200"},

	// Surfaces (light tints vs `#e1e2e7`).
	Canvas: Token{Name: "canvas", Dark: "#0b0c14", Light: "#e1e2e7"},
	// pinned — derivation: dark violet anchor #28243a lifted onto the light canvas
	// #e1e2e7; eyeball-confirmed at the 1-9 gate. Fill perceptible (1.25); carries
	// text.on-selection (10.5) / text.strong (5.71) / state.green (4.65 with the
	// darkened #3B5E18 light value, §4.1 `● attached` marker) above the 4.5 floor.
	BgSelection: Token{Name: "bg.selection", Dark: "#28243a", Light: "#D0C6F0"},
	// pinned — derivation: dark amber anchor #241B10 lifted onto the light canvas
	// #e1e2e7; eyeball-confirmed at the 1-9 gate. The 1-4 derived value held — fill
	// perceptible (1.11), on-warning leg 5.14 — so no more-contrast remedy needed.
	BgWarning: Token{Name: "bg.warning", Dark: "#241B10", Light: "#E8D6A8"},
	// pinned — derivation: dark grey anchor #26283A lifted onto the light canvas
	// #e1e2e7; eyeball-confirmed at the 1-9 gate. The 1-4 derived value held — fill
	// perceptible (1.14), reads as a distinct empty-track surface — no text leg
	// (the track carries no on-band text), so no remedy needed.
	BgTrack: Token{Name: "bg.track", Dark: "#26283A", Light: "#D2D4DE"},
	// pinned — derivation: dark separator/footer rules over the light canvas
	// #e1e2e7 (shared #C9CDDB); eyeball-confirmed at the 1-9 gate (perceptible 1.23).
	BorderSeparator: Token{Name: "border.separator", Dark: "#292E42", Light: "#C9CDDB"},
	BorderFooter:    Token{Name: "border.footer", Dark: "#20232E", Light: "#C9CDDB"},
	TextOnWarning:   Token{Name: "text.on-warning", Dark: "#E8C9A0", Light: "#7A4B12"},
}
