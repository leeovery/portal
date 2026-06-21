package capture

import (
	"fmt"
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/tui/theme"
)

// ContrastValidationFixture is the registered fixture name for the §16.5
// lock-in/bail contrast-validation swatch (task 1-9).
const ContrastValidationFixture = "contrast-validation"

// NewContrastValidationModel builds the contrast-validation swatch tea.Model for
// the appearance the capture harness pins via --appearance. It mirrors the
// production WithCanvasMode pin path: a light/dark pin paints that owned canvas
// from frame one; AppearanceAuto falls back to the dark canvas (the §2.6
// no-answer fallback) since the swatch runs no OSC 11 detection. The result is a
// tea.Model so the capture tool drives it identically to the production model.
func NewContrastValidationModel(appearance prefs.Appearance) tea.Model {
	return newSwatchModel(modeFromAppearance(appearance))
}

// modeFromAppearance maps the pinned appearance preference to the resolved canvas
// mode the swatch paints. Light pins the #e1e2e7 canvas; dark and auto both
// resolve to the #0b0c14 canvas (auto via the §2.6 dark fallback, since the
// inert swatch runs no detection).
func modeFromAppearance(appearance prefs.Appearance) theme.Mode {
	if appearance == prefs.AppearanceLight {
		return theme.Light
	}
	return theme.Dark
}

// swatchModel is the contrast-validation swatch — a self-contained tea.Model the
// offline capture harness renders for the §16.5 lock-in/bail gate (task 1-9). It
// is NOT the real Sessions surface: the four light-tint surfaces (selection row,
// separator/footer borders, warning band, loading track) are built in LATER
// phases. This gate validates the colour TOKENS *before* those phases invest in
// the surfaces (anti-sunk-cost), so the swatch renders, for each of the four
// 1-9-pinned tints, a labelled band filled with the tint colour carrying its
// on-tint foreground text on the OWNED canvas for the resolved mode.
//
// The human captures this in both modes (vhs) and eyeballs each light tint
// against `#e1e2e7` — the wash-out risk the numeric floor alone cannot catch
// (§2.9 / §15.6). The §4.1 foreground-on-tint pairings are rendered explicitly so
// they are eyeballed on their tints, not just numerically verified.
type swatchModel struct {
	mode theme.Mode

	// width/height cache the terminal size from the first WindowSizeMsg so the
	// frame fills the screen on the painted canvas. A fixed fallback keeps a direct
	// render (tests) deterministic without a size message.
	width  int
	height int
}

// newSwatchModel builds the swatch for a resolved canvas mode. The mode pins the
// owned canvas (dark #0b0c14 / light #e1e2e7) and the resolved tint variants —
// mirroring the production WithCanvasMode pin path the capture harness drives via
// --appearance.
func newSwatchModel(mode theme.Mode) swatchModel {
	return swatchModel{mode: mode}
}

// canvasHex returns the hex of the owned canvas this swatch paints for its mode.
// Exposed for the harness/tests to assert the correct canvas is selected.
func (s swatchModel) canvasHex() string {
	return tokenHex(theme.MV.Canvas, s.mode)
}

// Init returns no command — the swatch is a static, deterministic render (no tmux,
// no detection, no timers).
func (s swatchModel) Init() tea.Cmd { return nil }

// Update caches the terminal size and quits on q / ctrl+c / esc so vhs can drive
// the capture, then exit cleanly.
func (s swatchModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.width = msg.Width
		s.height = msg.Height
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return s, tea.Quit
		}
	}
	return s, nil
}

// View paints the swatch on the owned canvas. It mirrors tui.Model.View: the
// frame is alt-screen and the screen background is set (OSC 11) to the owned
// canvas for the mode, so the light tints are eyeballed against the exact
// `#e1e2e7` surface (the wash-out risk) rather than an arbitrary terminal bg.
func (s swatchModel) View() tea.View {
	v := tea.NewView(s.fillCanvas(renderSwatch(s.mode)))
	v.AltScreen = true
	v.BackgroundColor = theme.MV.Canvas.ColorFor(s.mode)
	return v
}

// fillCanvas pads the rendered swatch to the cached terminal size, backfilling
// every cell with the owned-canvas background so the whole frame (not just the
// band rows) reads on the canvas — the same invariant tui.Model enforces. A fixed
// 80x24 fallback keeps a sizeless render bounded.
func (s swatchModel) fillCanvas(view string) string {
	w, h := s.width, s.height
	if w == 0 {
		w = 80
	}
	if h == 0 {
		h = 24
	}
	canvasStyle := lipgloss.NewStyle().Background(theme.MV.Canvas.ColorFor(s.mode))
	blank := canvasStyle.Render(strings.Repeat(" ", w))
	lines := strings.Split(view, "\n")
	out := make([]string, 0, h)
	for _, line := range lines {
		if len(out) == h {
			break
		}
		gap := w - lipgloss.Width(line)
		if gap > 0 {
			line += canvasStyle.Render(strings.Repeat(" ", gap))
		}
		out = append(out, line)
	}
	for len(out) < h {
		out = append(out, blank)
	}
	return strings.Join(out, "\n")
}

// bandWidth is the fixed cell width of each swatch band — wide enough to carry
// the label + the on-tint sample text, narrow enough to leave the canvas visible
// on either side (so the tint reads AS a band against the canvas, not edge-to-edge
// fill).
const bandWidth = 56

// renderSwatch is the pure render of the contrast-validation swatch for a mode.
// It composes a title, the four tint bands (each with its on-tint foreground
// pairings labelled), and the borders rule on the owned canvas. Kept pure
// (mode-in, string-out) so it is unit-testable without a tea.Program.
func renderSwatch(mode theme.Mode) string {
	canvas := theme.MV.Canvas.ColorFor(mode)
	onCanvas := func(tok theme.Token) lipgloss.Style {
		return lipgloss.NewStyle().Foreground(tok.ColorFor(mode)).Background(canvas)
	}

	var b strings.Builder
	title := onCanvas(theme.MV.TextPrimary).Bold(true).
		Render(fmt.Sprintf("CONTRAST VALIDATION — %s canvas %s", modeName(mode), tokenHex(theme.MV.Canvas, mode)))
	b.WriteString(title)
	b.WriteString("\n\n")

	// bg.selection band: name in text.on-selection, "3 windows" count in
	// text.strong, "● attached" marker in state.green (the single token, darkened
	// to #3B5E18 light so it clears the floor on the bg.selection tint too) — ALL on
	// the bg.selection tint (the §4.1 selected-row foreground-on-tint pairings).
	b.WriteString(tintLabel(mode, "bg.selection", theme.MV.BgSelection))
	b.WriteString("\n")
	b.WriteString(selectionBand(mode))
	b.WriteString("\n")
	b.WriteString(pairCaption(mode, "fg-on-tint: text.on-selection · text.strong · state.green (● attached)"))
	b.WriteString("\n\n")

	// bg.warning band: "⚠ message" in text.on-warning ON the bg.warning tint.
	b.WriteString(tintLabel(mode, "bg.warning", theme.MV.BgWarning))
	b.WriteString("\n")
	b.WriteString(warningBand(mode))
	b.WriteString("\n")
	b.WriteString(pairCaption(mode, "fg-on-tint: text.on-warning (⚠ message)"))
	b.WriteString("\n\n")

	// bg.track band: the empty-track tint with a filled portion (accent.violet, the
	// loading-bar fill) so the bar reads against the empty track.
	b.WriteString(tintLabel(mode, "bg.track", theme.MV.BgTrack))
	b.WriteString("\n")
	b.WriteString(trackBand(mode))
	b.WriteString("\n\n")

	// borders: a separator/footer rule line on the canvas (border.separator and
	// border.footer share the same light #C9CDDB; both shown).
	b.WriteString(tintLabel(mode, "border.separator / border.footer", theme.MV.BorderSeparator))
	b.WriteString("\n")
	b.WriteString(borderRule(mode))

	return b.String()
}

// tintLabel renders the token name + its resolved hex above its band, in
// text.detail on the canvas, so the human reads exactly which token + hex the
// band below is.
func tintLabel(mode theme.Mode, name string, tok theme.Token) string {
	style := lipgloss.NewStyle().
		Foreground(theme.MV.TextDetail.ColorFor(mode)).
		Background(theme.MV.Canvas.ColorFor(mode))
	return style.Render(fmt.Sprintf("%s  %s", name, tokenHex(tok, mode)))
}

// pairCaption renders the §4.1 foreground-on-tint token legend under a band, in
// text.detail on the canvas, so the human reads which token each on-tint
// foreground uses (the pairing each band proves clears its floor).
func pairCaption(mode theme.Mode, text string) string {
	return lipgloss.NewStyle().
		Foreground(theme.MV.TextDetail.ColorFor(mode)).
		Background(theme.MV.Canvas.ColorFor(mode)).
		Render(text)
}

// tokenHex resolves a token's hex string for a mode — the label text the human
// reads ("bg.selection  #D0C6F0"). It reads the exported Light/Dark fields so the
// label is the exact pinned hex.
func tokenHex(tok theme.Token, mode theme.Mode) string {
	if mode == theme.Light {
		return tok.Light
	}
	return tok.Dark
}

// selectionBand fills bandWidth cells with the bg.selection tint and lays the
// three §4.1 selected-row foregrounds over it: the name (text.on-selection), the
// window count (text.strong) and the "● attached" marker (state.green). Each run
// keeps the bg.selection tint as its background so every pairing is rendered ON
// the tint.
func selectionBand(mode theme.Mode) string {
	tint := theme.MV.BgSelection.ColorFor(mode)
	on := func(tok theme.Token) lipgloss.Style {
		return lipgloss.NewStyle().Foreground(tok.ColorFor(mode)).Background(tint)
	}
	name := on(theme.MV.TextOnSelection).Bold(true).Render("agentic-workflows-code-based")
	count := on(theme.MV.TextStrong).Render("3 windows")
	// The `● attached` marker on the bg.selection tint renders in the single
	// state.green token (light darkened to #3B5E18 = 4.65 vs #D0C6F0, dark #9ECE6A =
	// 8.19) — the former dedicated on-selection override was folded into the global
	// token, so the selected row uses the same green as every other state.green usage.
	attached := on(theme.MV.StateGreen).Render("● attached")
	gap := on(theme.MV.TextOnSelection).Render("  ")
	content := name + gap + count + gap + attached
	return padBand(content, bandWidth, tint)
}

// warningBand fills bandWidth cells with the bg.warning tint and lays the §4.1
// "⚠ message" in text.on-warning over it.
func warningBand(mode theme.Mode) string {
	tint := theme.MV.BgWarning.ColorFor(mode)
	msg := lipgloss.NewStyle().
		Foreground(theme.MV.TextOnWarning.ColorFor(mode)).
		Background(tint).
		Render("⚠ tmux state saver is not running — restore may be incomplete")
	return padBand(msg, bandWidth, tint)
}

// trackBand renders the loading-bar track: a filled head (accent.violet, the bar
// token §2.9) over the bg.track empty-track tint, so the human eyeballs the bar
// against the empty track and the empty track against the canvas.
func trackBand(mode theme.Mode) string {
	track := theme.MV.BgTrack.ColorFor(mode)
	const filled = 18
	bar := lipgloss.NewStyle().
		Background(theme.MV.AccentViolet.ColorFor(mode)).
		Render(strings.Repeat(" ", filled))
	empty := lipgloss.NewStyle().
		Background(track).
		Render(strings.Repeat(" ", bandWidth-filled))
	return bar + empty
}

// borderRule renders a full-bandWidth rule in border.separator over the canvas
// (the separator and footer rules share the same light #C9CDDB), so the rule's
// perceptibility against the canvas is eyeballed.
func borderRule(mode theme.Mode) string {
	return lipgloss.NewStyle().
		Foreground(theme.MV.BorderSeparator.ColorFor(mode)).
		Background(theme.MV.Canvas.ColorFor(mode)).
		Render(strings.Repeat("─", bandWidth))
}

// padBand right-pads content to width cells, keeping the tint background across
// the padding so the band is a continuous filled surface of exactly width cells.
func padBand(content string, width int, tint color.Color) string {
	gap := width - lipgloss.Width(content)
	if gap > 0 {
		content += lipgloss.NewStyle().Background(tint).Render(strings.Repeat(" ", gap))
	}
	return content
}

// modeName is the human label for the swatch title.
func modeName(mode theme.Mode) string {
	if mode == theme.Light {
		return "LIGHT"
	}
	return "DARK"
}
