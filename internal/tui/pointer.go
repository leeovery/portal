package tui

import (
	"time"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
)

// Pointer input: a mouse, a trackpad, or a touch terminal that turns a swipe
// into wheel steps and a tap into a click. A wheel step is an arrow press in the
// direction the finger moved, so every list scrolls through the navigation its
// keyboard already drives; a tap on a list row does what Enter on that row does.

// tapTarget is the row a press landed on. The tap completes only when the
// release lands on the same row of the same list, so a drag that ends elsewhere
// opens nothing. The key catches a refresh that re-filled the row in between.
type tapTarget struct {
	page  page
	index int
	key   string
}

func (m Model) updatePointer(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.MouseWheelMsg:
		return m.updateWheel(msg)
	case tea.MouseClickMsg:
		m.tapPressed = false
		if msg.Button != tea.MouseLeft {
			return m, nil
		}
		m.tapPress, m.tapPressed = m.tapTargetAt(msg.Y)
		return m, nil
	case tea.MouseReleaseMsg:
		press, pressed := m.tapPress, m.tapPressed
		m.tapPressed = false
		target, ok := m.tapTargetAt(msg.Y)
		if !pressed || !ok || target != press {
			return m, nil
		}
		return m.openTapped(target)
	}
	return m, nil
}

// A list steps against the wheel. The wheel reports content scrolling — a
// finger drawn up reads as a step down — but a list's rows hold still while its
// highlight moves, and the highlight should follow the finger. The preview's
// scrollback does move, so it takes the wheel as reported.
//
// Wrapping past either end is the arrow's: the session and project lists scroll
// infinitely, so a swipe off the bottom carries on from the top.
func (m Model) updateWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	if !m.wheel.admit(m.wheelSurface()) {
		return m, nil
	}
	var code rune
	switch msg.Button {
	case tea.MouseWheelUp:
		code = tea.KeyDown
	case tea.MouseWheelDown:
		code = tea.KeyUp
	default:
		return m, nil
	}

	switch {
	case m.themePanel.open:
		// The panel takes the step as the arrow it stands for.
	case m.activePage == pagePreview:
		// The viewport scrolls the captured scrollback itself.
		if m.preview.helpOpen {
			return m, nil
		}
		var cmd tea.Cmd
		m.preview, cmd = m.preview.Update(msg)
		return m, cmd
	case m.modal != modalNone || m.burstPending:
		return m, nil
	case m.filterFocused():
		// The filter input owns the keyboard as a modal does: bubbles/list takes
		// an arrow there as accepting the filter, which would hand the letters
		// typed next to the page's commands.
		return m, nil
	case m.activePage != PageSessions && m.activePage != PageProjects:
		return m, nil
	}
	return m.Update(tea.KeyPressMsg{Code: code})
}

// wheelSettle is how long the wheel must be quiet before its next step starts a
// fresh gesture. A trackpad goes on sending momentum steps for about a second
// after the fingers lift, and a slow render queues more; without the settle
// they would land on whatever surface a key brought up meanwhile.
const wheelSettle = 250 * time.Millisecond

// wheelSurface is what a wheel step would reach: each field one thing whose
// change swaps the surface beneath the pointer. previewGen tells one preview
// from the next, so stepping to another session or reopening one is a change.
type wheelSurface struct {
	page            page
	modal           modalState
	burstPending    bool
	filterFocused   bool
	themePanel      bool
	previewHelpOpen bool
	previewGen      uint64
}

func (m Model) wheelSurface() wheelSurface {
	s := wheelSurface{
		page:          m.activePage,
		modal:         m.modal,
		burstPending:  m.burstPending,
		filterFocused: m.filterFocused(),
		themePanel:    m.themePanel.open,
	}
	if m.activePage == pagePreview {
		s.previewHelpOpen = m.preview.helpOpen
		s.previewGen = m.previewGen
	}
	return s
}

func (m Model) filterFocused() bool {
	switch m.activePage {
	case PageSessions:
		return m.sessionList.SettingFilter()
	case PageProjects:
		return m.projectList.SettingFilter()
	}
	return false
}

// wheelGesture holds a wheel gesture to the surface it began on. Every step,
// dropped or not, extends the gesture, so a momentum stream that outlives its
// surface is dropped until it stops; the first step after wheelSettle of quiet
// begins a new gesture on whatever surface is up then. Once a step is dropped
// the gesture is lost, so a stream that leaves its surface and comes back to it
// still reaches nothing. A model starts lost, as of its construction: a
// stream already running as the picker opens, such as one scrolling the
// session a detach just left, reaches nothing until it has gone quiet.
type wheelGesture struct {
	owner wheelSurface
	lost  bool
	last  time.Time
	// nil reads the wall clock; tests stage their own.
	now func() time.Time
}

func (g *wheelGesture) clock() time.Time {
	if g.now != nil {
		return g.now()
	}
	return time.Now()
}

func (g *wheelGesture) startLost() {
	g.lost, g.last = true, g.clock()
}

func (g *wheelGesture) admit(surface wheelSurface) bool {
	at := g.clock()
	if at.Sub(g.last) >= wheelSettle {
		g.owner, g.lost = surface, false
	}
	g.last = at
	if g.owner != surface {
		g.lost = true
	}
	return !g.lost
}

// The theme panel, a modal and the burst input-lock each own the input, so a
// tap reaches no row beneath them.
func (m Model) tapTargetAt(y int) (tapTarget, bool) {
	if m.themePanel.open || m.modal != modalNone || m.burstPending {
		return tapTarget{}, false
	}
	switch m.activePage {
	case PageSessions:
		index, ok := m.listRowAt(m.sessionList, (&m).sessionBandHeight(), SessionDelegate{}, y)
		if !ok {
			return tapTarget{}, false
		}
		// A group heading is drawn as a row but is never selectable.
		si, ok := m.sessionList.VisibleItems()[index].(SessionItem)
		if !ok {
			return tapTarget{}, false
		}
		return tapTarget{page: PageSessions, index: index, key: si.Session.Name}, true
	case PageProjects:
		index, ok := m.listRowAt(m.projectList, m.projectBandHeight(), ProjectDelegate{}, y)
		if !ok {
			return tapTarget{}, false
		}
		pi, ok := m.projectList.VisibleItems()[index].(ProjectItem)
		if !ok {
			return tapTarget{}, false
		}
		return tapTarget{page: PageProjects, index: index, key: pi.Project.Path}, true
	}
	return tapTarget{}, false
}

// Maps a terminal row to the visible item drawn on it. It walks the page's
// composition top-down — the gutter, the header block, the notice band, then the
// list's title bar ahead of the current page's rows — measured off the same
// renderers the height budgets use, so a band raised or cleared moves the rows
// and this mapping together.
func (m Model) listRowAt(l list.Model, bandRows int, d list.ItemDelegate, y int) (int, bool) {
	w, h := m.termDims()
	contentW := m.contentWidth()
	_, _, topPad, _ := gutterPadding(w, h, contentW, m.contentHeight())
	row := y - topPad - m.headerHeight(contentW) - bandRows - sectionHeaderBlockRows()
	if row < 0 {
		return 0, false
	}
	stride := d.Height() + d.Spacing()
	slot := row / stride
	if row%stride >= d.Height() {
		return 0, false
	}
	if slot >= l.Paginator.ItemsOnPage(len(l.VisibleItems())) {
		return 0, false
	}
	return l.Paginator.Page*l.Paginator.PerPage + slot, true
}

// A tap in multi-select marks the row, as m does: Enter there commits the whole
// marked set, which a tap on one row must not. The handlers are called directly
// rather than through a synthesised key, so a tap opens its row even while the
// filter input holds the keyboard.
func (m Model) openTapped(target tapTarget) (tea.Model, tea.Cmd) {
	if m.flashText != "" {
		m.clearFlash()
	}
	switch target.page {
	case PageSessions:
		if m.abortBannerText != "" {
			(&m).clearAbortBanner()
		}
		m.sessionList.Select(target.index)
		if m.multiSelectMode {
			return m.handleMultiSelectToggle()
		}
		return m.handleSessionListEnter()
	case PageProjects:
		m.projectList.Select(target.index)
		return m.handleProjectEnter()
	}
	return m, nil
}
