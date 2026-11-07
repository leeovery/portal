package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// A trackpad's momentum steps arrive about this far apart.
const momentumGap = 16 * time.Millisecond

type wheelClock struct{ at time.Time }

func (c *wheelClock) now() time.Time { return c.at }

func stageWheelClock(m *Model) *wheelClock {
	clock := &wheelClock{at: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
	m.wheel.now = clock.now
	return clock
}

// swipe sends n finger-down steps, each momentumGap after the one before.
func swipe(t *testing.T, m Model, clock *wheelClock, n int) Model {
	t.Helper()
	for range n {
		clock.at = clock.at.Add(momentumGap)
		m, _ = pointerUpdate(t, m, fingerDown)
	}
	return m
}

// applyKey presses a key and feeds back the message its command emits, as the
// runtime does, so a close that lands through a message has landed.
func applyKey(t *testing.T, m Model, key tea.KeyPressMsg) Model {
	t.Helper()
	m, cmd := pointerUpdate(t, m, key)
	if cmd == nil {
		return m
	}
	msg := cmd()
	if msg == nil {
		return m
	}
	m, _ = pointerUpdate(t, m, msg)
	return m
}

func requireSessionIndex(t *testing.T, m Model, want int, why string) {
	t.Helper()
	if got := m.sessionList.Index(); got != want {
		t.Fatalf("%s: the session cursor is at %d, want %d", why, got, want)
	}
}

func gesturePreviewModel(t *testing.T) (Model, *wheelClock) {
	t.Helper()
	sessions := pointerSessions("alpha", "bravo", "charlie")
	lines := make([]string, 0, 200)
	for i := range 200 {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	m := modelWithSeams(t, sessions, historyFor(t, strings.Join(lines, "\n"), sessions))
	clock := stageWheelClock(&m)
	m = openPreviewOn(t, m)
	if m.activePage != pagePreview {
		t.Fatal("fixture: Space did not open the preview")
	}
	return m, clock
}

// scrolledThenClosed scrolls the preview, closes it with key, and checks the
// scroll reached the preview and the close reached the list.
func scrolledThenClosed(t *testing.T, key tea.KeyPressMsg) (Model, *wheelClock) {
	t.Helper()
	m, clock := gesturePreviewModel(t)
	before := m.preview.viewport.YOffset()
	m = swipe(t, m, clock, 5)
	if m.preview.viewport.YOffset() >= before {
		t.Fatal("fixture: the swipe did not scroll the preview")
	}
	m = applyKey(t, m, key)
	if m.activePage != PageSessions {
		t.Fatalf("fixture: the close key left page %v, want the sessions list", m.activePage)
	}
	requireSessionIndex(t, m, 0, "fixture: after the close")
	return m, clock
}

var previewClosers = []struct {
	name string
	key  tea.KeyPressMsg
}{
	{"space", keySpaceMsg()},
	{"esc", tea.KeyPressMsg{Code: tea.KeyEscape}},
}

func TestWheelMomentumAfterThePreviewClosesNeverReachesTheList(t *testing.T) {
	for _, closer := range previewClosers {
		t.Run(closer.name+": a burst at once is dropped", func(t *testing.T) {
			m, clock := scrolledThenClosed(t, closer.key)
			m = swipe(t, m, clock, 10)
			requireSessionIndex(t, m, 0, "momentum after the preview closed")
		})

		t.Run(closer.name+": a burst after a quiet gap moves the list", func(t *testing.T) {
			m, clock := scrolledThenClosed(t, closer.key)
			clock.at = clock.at.Add(wheelSettle)
			m, _ = pointerUpdate(t, m, fingerDown)
			requireSessionIndex(t, m, 1, "a fresh swipe after the quiet")
		})

		t.Run(closer.name+": a stream outlasting the settle stays dropped until it stops", func(t *testing.T) {
			m, clock := scrolledThenClosed(t, closer.key)
			steps := int(3*wheelSettle/momentumGap) + 1
			m = swipe(t, m, clock, steps)
			requireSessionIndex(t, m, 0, "a momentum stream running past the settle")

			clock.at = clock.at.Add(wheelSettle - time.Millisecond)
			m, _ = pointerUpdate(t, m, fingerDown)
			requireSessionIndex(t, m, 0, "a step just inside the settle")

			clock.at = clock.at.Add(wheelSettle)
			m, _ = pointerUpdate(t, m, fingerDown)
			requireSessionIndex(t, m, 1, "the first step after the stream stopped")
		})

		t.Run(closer.name+": a key right after the close acts at once", func(t *testing.T) {
			m, clock := scrolledThenClosed(t, closer.key)
			m, _ = pointerUpdate(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
			requireSessionIndex(t, m, 1, "Down right after the close")
			m = swipe(t, m, clock, 3)
			m, _ = pointerUpdate(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
			requireSessionIndex(t, m, 2, "Down amid the dropped momentum")
		})
	}
}

func TestWheelMomentumAfterThePreviewOpensNeverScrollsIt(t *testing.T) {
	m, clock := gesturePreviewModel(t)
	m = applyKey(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	clock.at = clock.at.Add(wheelSettle)
	m = swipe(t, m, clock, 1)
	requireSessionIndex(t, m, 1, "fixture: the swipe on the list")

	m = openPreviewOn(t, m)
	before := m.preview.viewport.YOffset()
	m = swipe(t, m, clock, 10)
	if got := m.preview.viewport.YOffset(); got != before {
		t.Fatalf("momentum from the list scrolled the preview from %d to %d", before, got)
	}
}

func TestWheelMomentumAfterAModalClosesNeverReachesTheList(t *testing.T) {
	open := func(t *testing.T) (Model, *wheelClock) {
		t.Helper()
		m := pointerSessionsModel(t, pointerTermH, pointerSessions("alpha", "bravo", "charlie"))
		clock := stageWheelClock(&m)
		m = applyKey(t, m, tea.KeyPressMsg{Code: '?', Text: "?"})
		if m.modal != modalHelp {
			t.Fatal("fixture: ? did not open the help modal")
		}
		m = swipe(t, m, clock, 5)
		m = applyKey(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
		if m.modal != modalNone {
			t.Fatal("fixture: Esc did not close the help modal")
		}
		return m, clock
	}

	t.Run("a burst at once is dropped", func(t *testing.T) {
		m, clock := open(t)
		m = swipe(t, m, clock, 10)
		requireSessionIndex(t, m, 0, "momentum after the modal closed")
	})

	t.Run("a burst after a quiet gap moves the list", func(t *testing.T) {
		m, clock := open(t)
		clock.at = clock.at.Add(wheelSettle)
		m, _ = pointerUpdate(t, m, fingerDown)
		requireSessionIndex(t, m, 1, "a fresh swipe after the quiet")
	})

	t.Run("a key right after the close acts at once", func(t *testing.T) {
		m, _ := open(t)
		m, _ = pointerUpdate(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
		requireSessionIndex(t, m, 1, "Down right after the close")
	})
}

func TestWheelMomentumAfterTheThemePanelClosesNeverReachesTheList(t *testing.T) {
	rows := arrowValidRows(t, 4)
	open := func(t *testing.T) (Model, *wheelClock) {
		t.Helper()
		m := newArrowPanelModel(t, rows, arrowSlug(0))
		clock := stageWheelClock(&m)
		m = swipe(t, m, clock, 2)
		if arrowPanelIndex(m) == 0 {
			t.Fatal("fixture: the swipe did not move the panel cursor")
		}
		m = closeThemePanelForTest(t, m)
		if m.themePanel.open {
			t.Fatal("fixture: Esc did not close the theme panel")
		}
		requireSessionIndex(t, m, 0, "fixture: after the close")
		return m, clock
	}

	t.Run("a burst at once is dropped", func(t *testing.T) {
		m, clock := open(t)
		m = swipe(t, m, clock, 10)
		requireSessionIndex(t, m, 0, "momentum after the theme panel closed")
	})

	t.Run("a burst after a quiet gap moves the list", func(t *testing.T) {
		m, clock := open(t)
		clock.at = clock.at.Add(wheelSettle)
		m, _ = pointerUpdate(t, m, fingerDown)
		requireSessionIndex(t, m, 1, "a fresh swipe after the quiet")
	})

	t.Run("a key right after the close acts at once", func(t *testing.T) {
		m, _ := open(t)
		m, _ = pointerUpdate(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
		requireSessionIndex(t, m, 1, "Down right after the close")
	})
}

func TestWheelMomentumAfterThePanelOpensNeverMovesItsCursor(t *testing.T) {
	m := newArrowPanelModel(t, arrowValidRows(t, 4), arrowSlug(0))
	clock := stageWheelClock(&m)
	m = closeThemePanelForTest(t, m)
	m = swipe(t, m, clock, 1)
	requireSessionIndex(t, m, 1, "fixture: the swipe on the list")

	m = pressThemeKey(t, m)
	if !m.themePanel.open {
		t.Fatal("fixture: t did not open the theme panel")
	}
	before := arrowPanelIndex(m)
	m = swipe(t, m, clock, 10)
	if got := arrowPanelIndex(m); got != before {
		t.Fatalf("momentum from the list moved the panel cursor from %d to %d", before, got)
	}

	clock.at = clock.at.Add(wheelSettle)
	m = swipe(t, m, clock, 1)
	if arrowPanelIndex(m) == before {
		t.Fatal("a fresh swipe after the quiet did not move the panel cursor")
	}
}
