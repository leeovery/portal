package tui

import (
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/theme"
	"github.com/mattn/go-runewidth"
)

const previewMarker = "◉ preview"

// Rows (borders + header + dividers + footer) and columns (side borders +
// insets) happen to reduce by the same constant.
const previewFrameOverhead = 6

// Equal to previewFrameOverhead; named separately so the height arithmetic reads by intent.
const previewChromeRowOverhead = previewFrameOverhead

// truncateToCells clips s to a display-cell budget (per runewidth, so CJK and
// emoji count 2), appending "…" only when truncation occurred. Runes are never
// partially consumed, so output is always valid UTF-8; budget ≤ 0 returns "".
func truncateToCells(s string, budget int) string {
	if budget <= 0 {
		return ""
	}
	if runewidth.StringWidth(s) <= budget {
		return s
	}
	var b []byte
	used := 0
	for _, r := range s {
		w := runewidth.RuneWidth(r)
		if used+w > budget-1 {
			break
		}
		b = append(b, string(r)...)
		used += w
	}
	return string(b) + "…"
}

// An unterminated SGR sequence in a row the viewport clipped would bleed into
// the right border. Terminals collapse duplicate resets, so re-applying is
// harmless.
func injectSGRResets(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if len(line) > 0 {
			lines[i] = line + "\x1b[0m"
		}
	}
	return strings.Join(lines, "\n")
}

const chromeSegSpace = " "

const previewFooterGap = "  "

// The marker, then the text the session shows, truncated to whatever width is
// left.
func composePreviewHeaderRow(contentWidth int, session string, th theme.Theme, colourless bool) string {
	// Degenerate width: the marker alone exceeds the body — clip it so the panel
	// still fills the terminal width exactly and never overflows.
	if contentWidth < lipgloss.Width(previewMarker) {
		clipped := truncateToCells(previewMarker, contentWidth)
		return headerStyle(th.AccentMode, th, colourless).Render(clipped)
	}

	row := headerStyle(th.AccentMode, th, colourless).Render(previewMarker)
	sessionBudget := contentWidth - lipgloss.Width(previewMarker) - lipgloss.Width(chromeSegSpace)
	// A session truncated to empty drops its leading gap too, so the
	// marker-alone row stays exactly marker-wide.
	if name := truncateToCells(session, sessionBudget); name != "" {
		gap := headerCanvasBg(th, colourless).Render(chromeSegSpace)
		sessionSeg := headerStyle(th.TextPrimary, th, colourless).Render(name)
		row = lipgloss.JoinHorizontal(lipgloss.Top, row, gap, sessionSeg)
	}
	return row
}

// Descriptor order is the footer's left-to-right order.
func previewFooterGroups() []footerHintGroup {
	var groups []footerHintGroup
	for _, e := range previewKeymap() {
		if !e.Core {
			continue
		}
		groups = append(groups, footerHintGroup{key: e.Key, label: e.Action})
	}
	return groups
}

// Degrades so the footer never overflows the content width.
func composePreviewFooterRow(contentWidth int, th theme.Theme, colourless bool) string {
	groups := previewFooterGroups()

	full := previewFooterFromGroups(groups, true, th, colourless)
	if lipgloss.Width(full) <= contentWidth {
		return full
	}
	compact := previewFooterFromGroups(groups, false, th, colourless)
	if lipgloss.Width(compact) <= contentWidth {
		return compact
	}
	for n := len(groups) - 1; n >= 1; n-- {
		trimmed := previewFooterFromGroups(groups[:n], false, th, colourless)
		if lipgloss.Width(trimmed) <= contentWidth {
			return trimmed
		}
	}
	clipped := truncateToCells(groups[0].key, contentWidth)
	return headerStyle(th.AccentKey, th, colourless).Render(clipped)
}

func previewFooterFromGroups(groups []footerHintGroup, labelled bool, th theme.Theme, colourless bool) string {
	gap := headerCanvasBg(th, colourless).Render(previewFooterGap)
	rendered := make([]string, 0, len(groups))
	for _, g := range groups {
		if labelled {
			rendered = append(rendered, renderBlueKeyHint(g.key, g.label, th, colourless))
		} else {
			rendered = append(rendered, headerStyle(th.AccentKey, th, colourless).Render(g.key))
		}
	}
	return strings.Join(rendered, gap)
}

// Construct only via NewPreviewModel: the zero value is reserved for "between
// opens".
type previewModel struct {
	// session is the name history is read and stepping keyed by; shown is
	// the text the header names it by.
	session  string
	shown    string
	viewport viewport.Model
	width    int
	height   int
	// th is assigned by the parent after construction; a zero Theme renders
	// through lipgloss.Color("")'s no-colour sentinel, so it is not optional.
	th         theme.Theme
	colourless bool
	helpOpen   bool
}

// NewPreviewModel opens a preview of session showing body, which the
// session's read replaces once it lands.
func NewPreviewModel(session Session, body string, width, height int) previewModel {
	// Mirrors innerWidth/innerHeight, which aren't usable yet — m.width and
	// m.height are unassigned.
	m := previewModel{
		session:  session.Name,
		shown:    session.DisplayName(),
		viewport: newPreviewViewport(max(0, width-previewFrameOverhead), max(0, height-previewFrameOverhead)),
		width:    width,
		height:   height,
		// A zero Theme is silently colourless with no compile error.
		th: defaultDarkTheme(),
	}
	m.setBody(body)
	return m
}

// setBody shows body anchored at its bottom, where the session's cursor is.
func (m *previewModel) setBody(body string) {
	m.viewport.SetContent(body)
	// viewport.SetContent auto-jumps to the bottom only when the previous YOffset
	// overshoots, so the explicit jump is what anchors.
	m.viewport.GotoBottom()
}

// Plain ↑/↓ belong to the preview's session step, so the viewport's line scroll
// is j/k alone (the wheel scrolls through its own path, not the keymap), and
// ^↑/↓ page as they do in the lists. Nothing pans sideways: the body is the
// session's own screen, clipped as a terminal of this width would clip it.
func newPreviewViewport(width, height int) viewport.Model {
	vp := viewport.New(viewport.WithWidth(width), viewport.WithHeight(height))
	vp.KeyMap.Up.SetKeys("k")
	vp.KeyMap.Down.SetKeys("j")
	vp.KeyMap.PageUp.SetKeys("pgup", "ctrl+up", "b")
	vp.KeyMap.PageDown.SetKeys("pgdown", "ctrl+down", "f")
	// A zero step stills the sideways keys and the horizontal and shifted wheel
	// alike, where clearing the keymap would leave the wheel panning.
	vp.SetHorizontalStep(0)
	return vp
}

func (m previewModel) innerWidth() int {
	return max(0, m.width-previewFrameOverhead)
}

func (m previewModel) innerHeight() int {
	return max(0, m.height-previewChromeRowOverhead)
}

type previewDismissedMsg struct{}

// Delta is -1 for the previous session in the list, +1 for the next. The parent
// owns the session list, so it resolves the step and rebuilds the preview.
type previewStepSessionMsg struct {
	delta int
}

// previewOpenMsg asks the parent to open the previewed session as Enter on
// the list does: record it and quit, so the connector runs after the TUI.
type previewOpenMsg struct {
	Session string
}

// PreserveName is the session highlighted when preview opened, used to re-anchor
// the cursor by name. Err is surfaced rather than swallowed so the handler can
// keep the pre-refresh list intact.
type previewSessionsRefreshedMsg struct {
	Sessions     []Session
	Pending      map[string]struct{}
	Err          error
	PreserveName string
}

// Preview-owned keys are intercepted before the viewport, whose own keymap
// would otherwise claim them. Everything else delegates, so its native keymap
// and resize clamping survive.
func (m previewModel) Update(msg tea.Msg) (previewModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.SetWidth(m.innerWidth())
		m.viewport.SetHeight(m.innerHeight())
		return m, nil
	case tea.KeyPressMsg:
		if handled, next, cmd := m.handlePreviewKey(msg); handled {
			return next, cmd
		}
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// Returns handled=false when the key is not preview-owned, so the caller delegates.
func (m previewModel) handlePreviewKey(msg tea.KeyPressMsg) (handled bool, next previewModel, cmd tea.Cmd) {
	// Handled first so no binding leaks while the overlay is up: Esc dismisses the
	// overlay without falling through to the preview-back path.
	if m.helpOpen {
		switch {
		case isRuneKey(msg, "?"), keyIsCode(msg, tea.KeyEscape):
			m.helpOpen = false
			return true, m, nil
		}
		return true, m, nil
	}

	switch {
	case isRuneKey(msg, "?"):
		m.helpOpen = true
		return true, m, nil
	case keyIsCode(msg, tea.KeyEscape), keyIsCode(msg, tea.KeySpace):
		return true, m, func() tea.Msg { return previewDismissedMsg{} }
	// viewport.DefaultKeyMap binds neither Home nor End.
	case keyIsCode(msg, tea.KeyHome):
		m.viewport.GotoTop()
		return true, m, nil
	case keyIsCode(msg, tea.KeyEnd):
		m.viewport.GotoBottom()
		return true, m, nil
	case keyIsCode(msg, tea.KeyEnter):
		session := m.session
		return true, m, func() tea.Msg { return previewOpenMsg{Session: session} }
	case keyIsCode(msg, tea.KeyUp):
		return true, m, func() tea.Msg { return previewStepSessionMsg{delta: -1} }
	case keyIsCode(msg, tea.KeyDown):
		return true, m, func() tea.Msg { return previewStepSessionMsg{delta: +1} }
	}
	return false, previewModel{}, nil
}

// The body is the session's own drawing, never themed. Chrome is recomposed every tick
// rather than cached, so resize and navigation need no cache invalidation.
func (m previewModel) View() string {
	contentWidth := m.innerWidth()

	header := composePreviewHeaderRow(contentWidth, m.shown, m.th, m.colourless)
	footer := composePreviewFooterRow(contentWidth, m.th, m.colourless)

	// Split per line so each becomes a compartment row.
	body := strings.Split(injectSGRResets(m.viewport.View()), "\n")

	preview := renderJoinedPanel(
		[][]string{{header}, body, {footer}},
		m.th.AccentMode,
		m.th, m.colourless,
	)

	if m.helpOpen {
		return overlayHelpOnPreview(preview, previewKeymap(), m.th, m.colourless)
	}
	return preview
}

// Unlike the other modals, this overlays rather than blanking the page beneath.
func overlayHelpOnPreview(preview string, entries []keymapEntry, th theme.Theme, colourless bool) string {
	panel := renderHelpModalContent(entries, th, colourless, nil)

	bgW := lipgloss.Width(preview)
	bgH := lipgloss.Height(preview)
	panelW := lipgloss.Width(panel)
	panelH := lipgloss.Height(panel)

	// Clamped so a panel larger than the preview lands at 0,0, not a negative offset.
	x := max(0, (bgW-panelW)/2)
	y := max(0, (bgH-panelH)/2)

	background := lipgloss.NewLayer(preview).X(0).Y(0).Z(0)
	foreground := lipgloss.NewLayer(panel).X(x).Y(y).Z(1)
	return lipgloss.NewCompositor(background, foreground).Render()
}
