package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/mattn/go-runewidth"
)

const previewMarker = "◉ preview"

// Below this cell budget a truncated session name reads as garbage, so the
// header cascade drops the counters segment instead.
const minSessionNameCells = 8

// Rows (borders + header + dividers + footer) and columns (side borders +
// insets) happen to reduce by the same constant.
const previewFrameOverhead = 6

// Equal to previewFrameOverhead; named separately so the height arithmetic reads by intent.
const previewChromeRowOverhead = previewFrameOverhead

// Rendered when the reader returns (nil, nil), which collapses ENOENT,
// zero-byte .bin, and a file holding only an unterminated partial line.
const previewPlaceholder = "(no saved content)"

// Rendered when the reader returns (nil, err) — uniform across errno types. No
// per-pane error is cached; refocusing retries the read.
const previewReadError = "(unable to read scrollback)"

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

// An unterminated SGR sequence in the scrollback would bleed into the right
// border. Terminals collapse duplicate resets, so re-applying is harmless.
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

// Display is 1-based; callers pass ordinals, never raw tmux indices.
func previewCounters(windowIdx, windowCount, paneIdx, paneCount int) string {
	return fmt.Sprintf("Window %d/%d · Pane %d/%d", windowIdx+1, windowCount, paneIdx+1, paneCount)
}

// counters is "" when the width cascade drops that segment.
type previewHeaderSegments struct {
	marker   string
	session  string
	counters string
}

// The returned segments always fit within width.
func selectPreviewHeaderTier(width int, session string, windowIdx, windowCount, paneIdx, paneCount int) previewHeaderSegments {
	counters := previewCounters(windowIdx, windowCount, paneIdx, paneCount)
	markerW := lipgloss.Width(previewMarker)
	gapW := lipgloss.Width(chromeSegSpace)

	fullW := markerW + gapW + lipgloss.Width(session) + gapW + lipgloss.Width(counters)
	if fullW <= width {
		return previewHeaderSegments{marker: previewMarker, session: session, counters: counters}
	}

	fixedT2 := markerW + gapW + gapW + lipgloss.Width(counters)
	if sessBudget := width - fixedT2; sessBudget >= minSessionNameCells {
		return previewHeaderSegments{marker: previewMarker, session: truncateToCells(session, sessBudget), counters: counters}
	}

	if markerW+gapW+lipgloss.Width(session) <= width {
		return previewHeaderSegments{marker: previewMarker, session: session}
	}

	// max keeps the budget non-negative where even the marker overflows.
	sessBudget := max(0, width-markerW-gapW)
	return previewHeaderSegments{marker: previewMarker, session: truncateToCells(session, sessBudget)}
}

func composePreviewHeaderRow(contentWidth, windowIdx, windowCount, paneIdx, paneCount int, session string, th theme.Theme, colourless bool) string {
	// Degenerate width: the marker alone exceeds the body — clip it so the panel
	// still fills the terminal width exactly and never overflows.
	if contentWidth < lipgloss.Width(previewMarker) {
		clipped := truncateToCells(previewMarker, contentWidth)
		return headerStyle(th.AccentMode, th, colourless).Render(clipped)
	}

	segs := selectPreviewHeaderTier(contentWidth, session, windowIdx, windowCount, paneIdx, paneCount)
	gap := headerCanvasBg(th, colourless).Render(chromeSegSpace)

	row := headerStyle(th.AccentMode, th, colourless).Render(segs.marker)
	// A session truncated to empty drops its leading gap too, so the
	// marker-alone row stays exactly marker-wide.
	if segs.session != "" {
		sessionSeg := headerStyle(th.TextPrimary, th, colourless).Render(segs.session)
		row = lipgloss.JoinHorizontal(lipgloss.Top, row, gap, sessionSeg)
	}
	if segs.counters != "" {
		counters := headerStyle(th.TextMuted, th, colourless).Render(segs.counters)
		row = lipgloss.JoinHorizontal(lipgloss.Top, row, gap, counters)
	}
	return row
}

// Descriptor order is the footer's left-to-right order.
func previewFooterGroups() []footerHintGroup {
	groups := make([]footerHintGroup, 0, 4)
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
// opens", and currentGroup would index an empty slice.
type previewModel struct {
	session    string
	enumerator TmuxEnumerator
	reader     ScrollbackReader
	attacher   PreviewAttacher
	groups     []tmux.WindowGroup
	windowIdx  int
	paneIdx    int
	viewport   viewport.Model
	width      int
	height     int
	// th is assigned by the parent after construction; a zero Theme renders
	// through lipgloss.Color("")'s no-colour sentinel, so it is not optional.
	th         theme.Theme
	colourless bool
	helpOpen   bool
}

// NewPreviewModel returns ok=false on an enumeration error or an empty result,
// so the caller falls through to the silent no-open path.
func NewPreviewModel(session string, enumerator TmuxEnumerator, reader ScrollbackReader, attacher PreviewAttacher, width, height int) (previewModel, bool) {
	groups, err := enumerator.ListWindowsAndPanesInSession(session)
	if err != nil {
		return previewModel{}, false
	}
	if len(groups) == 0 || len(groups[0].PaneIndices) == 0 {
		return previewModel{}, false
	}

	// Mirrors innerWidth/innerHeight, which aren't usable yet — m.width and
	// m.height are unassigned.
	innerW := max(0, width-previewFrameOverhead)
	innerH := max(0, height-previewFrameOverhead)
	m := previewModel{
		session:    session,
		enumerator: enumerator,
		reader:     reader,
		attacher:   attacher,
		groups:     groups,
		windowIdx:  0,
		paneIdx:    0,
		viewport:   viewport.New(viewport.WithWidth(innerW), viewport.WithHeight(innerH)),
		width:      width,
		height:     height,
		// A zero Theme is silently colourless with no compile error.
		th: defaultDarkTheme(),
	}

	// viewport.SetContent auto-jumps to the bottom only when the previous YOffset
	// overshoots, so on a fresh viewport the helper's explicit jump is what anchors.
	m.viewport = m.readFocusedPaneIntoViewport()

	return m, true
}

func (m previewModel) currentGroup() tmux.WindowGroup {
	return m.groups[m.windowIdx]
}

// Raw tmux indices, not 0-based ordinals — under non-contiguous window_index or
// pane-base-index 1 these are what compose the daemon's canonical pane key.
func (m previewModel) currentRawIndices() (windowIndex, paneIndex int) {
	g := m.currentGroup()
	return g.WindowIndex, g.PaneIndices[m.paneIdx]
}

// Byte-identical to the key the daemon writer uses, so the tail read addresses
// the same `.bin` file tmux wrote.
func (m previewModel) currentPaneKey() string {
	rawWindow, rawPane := m.currentRawIndices()
	return state.SanitizePaneKey(m.session, rawWindow, rawPane)
}

func (m previewModel) degenerate() bool {
	return len(m.groups) == 1 && len(m.groups[0].PaneIndices) == 1
}

func (m previewModel) innerWidth() int {
	return max(0, m.width-previewFrameOverhead)
}

func (m previewModel) innerHeight() int {
	return max(0, m.height-previewChromeRowOverhead)
}

// The viewport is returned rather than mutated in place so every method on
// previewModel can stay a value receiver.
func (m previewModel) readFocusedPaneIntoViewport() viewport.Model {
	vp := m.viewport
	bytes, err := m.reader.Tail(m.currentPaneKey())
	switch {
	case bytes == nil && err == nil:
		vp.SetContent(previewPlaceholder)
	// Ordered after (nil, nil): the reader never returns bytes alongside an error,
	// and this arm routes any such drift to the error string rather than the bytes.
	case err != nil:
		vp.SetContent(previewReadError)
	default:
		vp.SetContent(string(bytes))
	}
	vp.GotoBottom()
	return vp
}

type previewDismissedMsg struct{}

// PreserveName is the session highlighted when preview opened, used to re-anchor
// the cursor by name. Err is surfaced rather than swallowed so the handler can
// keep the pre-refresh list intact.
type previewSessionsRefreshedMsg struct {
	Sessions     []tmux.Session
	Pending      map[string]struct{}
	Err          error
	PreserveName string
}

// Preview-owned keys are intercepted before the viewport, which binds plain ←/→
// for horizontal scroll and would swallow Tab. Everything else delegates, so its
// native keymap and resize clamping survive; only a window/pane cycle re-reads.
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
	// Attaching is unconditional — a pane with no saved scrollback still attaches.
	case keyIsCode(msg, tea.KeyEnter):
		if m.attacher == nil {
			return true, m, nil
		}
		windowIndex, paneIndex := m.currentRawIndices()
		return true, m, m.attacher.Run(m.session, windowIndex, paneIndex)
	// Tab, not Ctrl+←/→, because macOS Mission Control hijacks the latter.
	case keyIsCode(msg, tea.KeyTab):
		return true, m.cyclePane(+1), nil
	case keyIsCode(msg, tea.KeyLeft):
		return true, m.cycleWindow(-1), nil
	case keyIsCode(msg, tea.KeyRight):
		return true, m.cycleWindow(+1), nil
	}
	return false, previewModel{}, nil
}

// Per-window pane focus is not retained, so paneIdx resets to 0.
func (m previewModel) cycleWindow(delta int) previewModel {
	if len(m.groups) <= 1 {
		return m
	}
	m.windowIdx = (m.windowIdx + delta + len(m.groups)) % len(m.groups)
	m.paneIdx = 0
	m.viewport = m.readFocusedPaneIntoViewport()
	return m
}

func (m previewModel) cyclePane(delta int) previewModel {
	paneCount := len(m.currentGroup().PaneIndices)
	if paneCount <= 1 {
		return m
	}
	m.paneIdx = (m.paneIdx + delta + paneCount) % paneCount
	m.viewport = m.readFocusedPaneIntoViewport()
	return m
}

// The body is the captured ANSI, never themed. Chrome is recomposed every tick
// rather than cached, so resize and navigation need no cache invalidation.
func (m previewModel) View() string {
	contentWidth := m.innerWidth()

	header := composePreviewHeaderRow(
		contentWidth,
		m.windowIdx, len(m.groups),
		m.paneIdx, len(m.currentGroup().PaneIndices),
		m.session,
		m.th, m.colourless,
	)
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
