package tui

import (
	"slices"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/theme"
)

const (
	themePanelHeaderLabel = "Themes"

	// Short enough to fit the minimum panel width untruncated.
	themePanelDirUnreadable = flashWarningGlyph + " dir unreadable"

	// `● both` is deliberately no wider than `● light`, so the collapsed form
	// cannot steal truncation budget from the label.
	themePanelBadgeConstant = "●"
	themePanelBadgeLight    = "● light"
	themePanelBadgeDark     = "● dark"
	themePanelBadgeBoth     = "● both"
)

const (
	// The closed/entry pairs are worded differently on purpose — do not unify.
	themePanelNarrowClosedFlash = "terminal too narrow — theme picker closed"
	themePanelShortClosedFlash  = "terminal too short — theme picker closed"

	themePanelNoColorFlash     = "theme picker needs colour — NO_COLOR is set"
	themePanelNarrowEntryFlash = "terminal too narrow for the theme picker"
	themePanelShortEntryFlash  = "terminal too short for the theme picker"

	// No `⚠` here: the notice band's warning role prepends the glyph, so one in
	// the copy would render two.
	themeNotSavedFlash = "theme not saved — see portal.log"
)

// dimNone is unreachable here (callers reach this only on a refusal).
func themePanelForcedCloseFlash(dim themePanelDim) string {
	if dim == dimHeight {
		return themePanelShortClosedFlash
	}
	return themePanelNarrowClosedFlash
}

func themePanelEntryFlash(dim themePanelDim) string {
	if dim == dimHeight {
		return themePanelShortEntryFlash
	}
	return themePanelNarrowEntryFlash
}

// Composited over the page rather than replacing it: modals blank the page to
// the canvas, so a modal picker would preview nothing.
type themePanel struct {
	// Set only once the list exists: the restyle path keys off it and would
	// otherwise run against a zero list.Model mid-arm.
	open bool

	list list.Model

	// Retained for the panel's lifetime so previews need no fresh I/O.
	enumeration theme.Enumeration

	union theme.Union

	// Keyed by theme.Row.BadgeKey. Re-derived only at open and after a landed
	// commit, so the `●` cannot move on a write that did not land.
	badges map[string]theme.Badge

	message themePanelMessage

	pending themeSlotConfirm

	// Outer width, border column included.
	width int

	// The page's notice-band slot in rows, so the page-aligned header travels
	// with the section header the band displaces. Stamped from the model at
	// every size-apply and again on the way into the render, never held across
	// a frame: a band can be raised or cleared while the panel is open.
	bandRows int
}

// pinArrowOnlyNav is load-bearing: the v2 DefaultKeyMap binds `l` and `d` to
// NextPage, colliding with the panel's commit keys.
func newThemePanelList(items []list.Item, delegate list.ItemDelegate) list.Model {
	l := list.New(items, delegate, 0, 0)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(false)
	pinArrowOnlyNav(&l.KeyMap)
	return l
}

// The floor is read with dirUnusable false — the flag does not exist until the
// enumeration runs; openThemePanel re-checks with the real one.
func (m Model) themePanelEntry() (blockedFlash string, ok bool) {
	if m.colourless {
		return themePanelNoColorFlash, false
	}
	if dim, fits := themePanelFloor(m.contentWidth(), m.contentHeight(), false); !fits {
		return themePanelEntryFlash(dim), false
	}
	return "", true
}

func (m Model) handleThemePanelKey() (tea.Model, tea.Cmd) {
	flash, ok := m.themePanelEntry()
	if !ok {
		return m.blockThemePanel(flash)
	}
	return m.openThemePanel()
}

// The flash is the whole of a refused entry's output, and it is raised through
// setThemeFlash so it carries the theme precedence tier.
func (m Model) blockThemePanel(flash string) (tea.Model, tea.Cmd) {
	(&m).setThemeFlash(flash)
	return m, flashTickCmd(m.flashGen)
}

// Re-read per open, so a drop-in edit shows without relaunching. The floor is
// re-checked with the real DirUnusable — the warning row raises it by one.
func (m Model) openThemePanel() (tea.Model, tea.Cmd) {
	if m.themeState.source == nil {
		return m, nil
	}

	enumeration, union := m.themeState.source.Open(m.themeState.keys)
	if dim, fits := themePanelFloor(m.contentWidth(), m.contentHeight(), union.DirUnusable); !fits {
		return m.blockThemePanel(themePanelEntryFlash(dim))
	}
	(&m).armThemePanel(enumeration, union)
	return m, nil
}

// Order matters: the delegate's budget comes from width, the list from the
// resolution's palette and badges, the restyle path keys off open, and applying
// styles re-sizes the list — so the cursor anchors last.
func (m *Model) armThemePanel(enumeration theme.Enumeration, union theme.Union) {
	width, _ := themePanelWidthFor(m.contentWidth())
	m.themePanel = themePanel{
		enumeration: enumeration,
		union:       union,
		width:       width,
	}
	cursor := m.applyThemePanelResolution(enumeration)
	m.themePanel.list = newThemePanelList(m.themePanel.rowItems(), m.themeRowDelegate())
	m.themePanel.open = true
	m.applyThemePanelListStyles()
	m.anchorThemePanelCursor(cursor)
	m.anchorThemePanelCursor(m.themeState.initialCursor)
	m.seedThemePanelMessage()
}

// Runs after the cursor seed: the confirm records the slug under the cursor,
// and raising a message re-sizes the list.
func (m *Model) seedThemePanelMessage() {
	switch {
	case m.themeState.initialConfirm:
		if slug, ok := committableThemeSlug(m.themePanel.list); ok {
			m.raiseSlotConfirm(slug, theme.MemberLight)
		}
	case m.themeState.initialCommitFailed:
		m.reportCommitFailure()
	}
}

// Resolves against the retained parse, never the filesystem — a fresh read could
// disagree with the rows on screen. A broken builtin or a resolution naming no
// slot degrades (nothing applied or written) rather than quitting Portal.
func (m *Model) applyInForceTheme(e theme.Enumeration) (theme.Resolution, theme.SlotResolution, bool) {
	resolution, err := m.themeState.source.Resolve(e, m.themeState.keys)
	if err != nil {
		return theme.Resolution{}, theme.SlotResolution{}, false
	}
	inForce, ok := inForceSlot(resolution, m.themeState.inForceMode())
	if !ok {
		return theme.Resolution{}, theme.SlotResolution{}, false
	}

	if inForce.Theme != m.themeState.active {
		m.ApplyTheme(inForce.Theme)
	}
	return resolution, inForce, true
}

// The open's parse supersedes construction's, so a mid-session edit lands here.
// The empty return is the degrade path; the anchor treats it as a no-op.
func (m *Model) applyThemePanelResolution(e theme.Enumeration) string {
	resolution, inForce, ok := m.applyInForceTheme(e)
	if !ok {
		return ""
	}

	m.themePanel.badges = theme.Badges(resolution.Slots)
	return inForce.Resolved
}

// Routes through ResolveSetting so the gate cannot disagree with the seam's tiebreak.
func (m Model) themeSetting() theme.Setting {
	setting, _ := theme.ResolveSetting(m.themeState.keys)
	return setting
}

// The false return is a resolution naming no slot; callers degrade on it rather
// than selecting a zero Theme.
func inForceSlot(r theme.Resolution, mode theme.Member) (theme.SlotResolution, bool) {
	want := mode.Slot()
	for _, slot := range r.Slots {
		if slot.Slot == theme.SlotConstant || slot.Slot == want {
			return slot, true
		}
	}
	return theme.SlotResolution{}, false
}

// By identity, never index — the commit recompute can insert rows above the
// cursor. The target is the resolved slug, not the requested one: under a
// fallback the fallback's row is the painted, selectable one.
func (m *Model) anchorThemePanelCursor(identity string) {
	if identity == "" {
		return
	}
	m.themePanel.list.Select(themePanelRowIndex(m.themePanel.union.Rows, identity))
}

// The Selectable filter picks the built-in out of a reserved-name collision
// (both rows share an identity) and keeps a seed naming an unselectable row
// from parking the cursor where the arrows cannot return.
func themePanelRowIndex(rows []theme.Row, identity string) int {
	identified := func(row theme.Row) bool { return row.Identity() == identity && row.Selectable() }
	if at := slices.IndexFunc(rows, identified); at >= 0 {
		return at
	}
	return max(slices.IndexFunc(rows, theme.Row.Selectable), 0)
}

// Deliberately not a restore of a snapshot taken at open: a file broken
// mid-session must not come back, and Esc after a commit must resolve the newly
// persisted state. The resolution reads the retained enumeration, so discard last.
func (m *Model) closeThemePanel() tea.Cmd {
	m.applyInForceTheme(m.themePanel.enumeration)
	m.themePanel = themePanel{}
	return m.reportOutstandingCommitFailure()
}

// Raising discharges the flag, else every later close re-fires the report.
func (m *Model) reportOutstandingCommitFailure() tea.Cmd {
	if !m.themeState.commitFailed {
		return nil
	}
	m.setThemeFlash(themeNotSavedFlash)
	m.themeState.commitFailed = false
	return flashTickCmd(m.flashGen)
}

// Degrading must re-run applyThemePanelListStyles: a stale PerPage makes page
// keys move a different distance than the screen scrolls, and the delegate holds
// its width as a field. A due commit-failure report wins over the geometry flash,
// which otherwise carries the auto-clear tick the report would have brought.
func (m *Model) resizeThemePanel() tea.Cmd {
	if !m.themePanel.open {
		return nil
	}
	if dim, ok := themePanelFloor(m.contentWidth(), m.contentHeight(), m.themePanel.union.DirUnusable); !ok {
		// Order matters: the read precedes the close, which discharges the flag as it
		// reports. Reading it afterwards always yields false.
		willReport := m.themeState.commitFailed
		cmd := m.closeThemePanel()
		if !willReport {
			// The generation is read after the raise bumps it, so the tick the
			// guard admits is this flash's.
			m.setThemeFlash(themePanelForcedCloseFlash(dim))
			cmd = tea.Batch(cmd, flashTickCmd(m.flashGen))
		}
		return cmd
	}
	m.themePanel.width, _ = themePanelWidthFor(m.contentWidth())
	m.applyThemePanelListStyles()
	return nil
}

// BadgeKey, never Slug: a reserved-name row's slug equals the built-in's, so a
// Slug lookup would paint `●` on both.
func (p themePanel) rowItems() []list.Item {
	items := make([]list.Item, 0, len(p.union.Rows))
	for _, row := range p.union.Rows {
		items = append(items, themeRowItem{Row: row, Badge: p.badges[row.BadgeKey()]})
	}
	return items
}

// Sized to the real remainder, not themePanelMinBodyRows: `bubbles/list` derives
// PerPage from the height it is given, so a floor-sized list pins a one-row page.
func (m *Model) applyThemePanelListStyles() {
	m.themePanel.bandRows = m.pageBandHeight()
	width, rows := themePanelListSize(m.themePanel, m.contentHeight())
	sizeList(&m.themePanel.list, width, rows)
	m.applyThemePanelCanvasMode()
}

// Cannot be skipped: `bubbles/list` reads its pagination-dot strings out of the
// styles once, so restyling without re-feeding the paginator leaves hardcoded
// greys. The open guard keeps armThemePanel's mid-arm ApplyTheme off a zero list.
func (m *Model) applyThemePanelCanvasMode() {
	if !m.themePanel.open {
		return
	}
	applyListCanvasMode(&m.themePanel.list, m.themeRowDelegate(), m.themeState.active, m.colourless)
}

// Key-exclusive while open; only `Ctrl-C` passes. `Esc` is consumed here — on the
// page beneath it is the progressive-back key. `Enter` deliberately does not
// close: a dual-purpose exit would let a user who set both slots wipe the pair.
func (m Model) updateThemePanel(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	(&m).clearThemePanelCommitFailed()

	switch {
	case keyIsCtrlC(msg):
		return m, tea.Quit
	case m.themePanel.confirming():
		return m.updateSlotConfirm(msg)
	case keyIsCode(msg, tea.KeyEscape):
		cmd := (&m).closeThemePanel()
		return m, cmd
	case keyIsCode(msg, tea.KeyEnter):
		// The report is raised inside the commit; a failed write leaves the keys
		// untouched.
		_ = (&m).commitSelected((&m).commitConstant)
		return m, nil
	case isRuneKey(msg, "d"):
		return m.handleSlotCommitKey(theme.MemberDark)
	case isRuneKey(msg, "l"):
		return m.handleSlotCommitKey(theme.MemberLight)
	case themePanelNavKey(m.themePanel.list.KeyMap, msg):
		cmd := (&m).moveThemePanelCursor(msg)
		return m, cmd
	default:
		return m, nil
	}
}

// Matches the live KeyMap so routing and the list's dispatch share one binding set.
func themePanelNavKey(km list.KeyMap, msg tea.KeyPressMsg) bool {
	return key.Matches(msg, km.CursorUp, km.CursorDown, km.PrevPage, km.NextPage)
}

// OSC 11 needs no handling here: View assigns BackgroundColor declaratively
// and Bubble Tea diffs it — do not add suppression or a debounce. The canvas
// flash when previewing a light theme in a dark terminal is deliberate.
func (m *Model) moveThemePanelCursor(msg tea.KeyPressMsg) tea.Cmd {
	var cmd tea.Cmd
	m.themePanel.list, cmd = m.themePanel.list.Update(msg)
	m.skipUnselectableThemeRow(msg)
	m.previewSelectedThemeRow()
	return cmd
}

// Loops (broken drop-ins can be adjacent) and reverses at either boundary rather
// than falling off. The 2×rows bound keeps an all-invalid union from spinning.
func (m *Model) skipUnselectableThemeRow(msg tea.KeyPressMsg) {
	l := &m.themePanel.list
	rows := len(l.Items())
	upward := key.Matches(msg, l.KeyMap.CursorUp, l.KeyMap.PrevPage)

	for range 2 * rows {
		if row, ok := selectedThemeRow(*l); ok && row.Selectable() {
			return
		}
		switch index := l.Index(); {
		case upward && index == 0:
			upward = false
		case !upward && index == rows-1:
			upward = true
		}
		if upward {
			l.CursorUp()
			continue
		}
		l.CursorDown()
	}
}

// The Selectable check keeps a union with no selectable rows from painting a
// zero Theme, which renders silently colourless.
func (m *Model) previewSelectedThemeRow() {
	row, ok := selectedThemeRow(m.themePanel.list)
	if !ok || !row.Selectable() || row.Theme == m.themeState.active {
		return
	}
	m.ApplyTheme(row.Theme)
}

func selectedThemeRow(l list.Model) (theme.Row, bool) {
	item, ok := l.SelectedItem().(themeRowItem)
	if !ok {
		return theme.Row{}, false
	}
	return item.Row, true
}

// Re-invoked by the restyle path, so two sites cannot disagree about width or
// colourlessness.
func (m Model) themeRowDelegate() themeRowDelegate {
	return themeRowDelegate{
		Theme:      m.themeState.active,
		Colourless: m.colourless,
		Width:      themePanelInnerWidth(m.themePanel.width),
	}
}
