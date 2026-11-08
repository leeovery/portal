package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/theme"
)

// A trackpad's momentum steps arrive about this far apart.
const momentumGap = 16 * time.Millisecond

type wheelClock struct{ at time.Time }

func (c *wheelClock) now() time.Time { return c.at }

func stageWheelClock(m *Model) *wheelClock {
	clock := &wheelClock{at: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
	m.wheel.now = clock.now
	openedASettleAgo(m)
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
			// Not a multiple of the three rows, so momentum that got through
			// would show even on the wrapping list.
			m = swipe(t, m, clock, 4)
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

func requirePreviewOffset(t *testing.T, m Model, want int, why string) {
	t.Helper()
	if got := m.preview.viewport.YOffset(); got != want {
		t.Fatalf("%s: the preview is at offset %d, want %d", why, got, want)
	}
}

func TestWheelMomentumNeverReachesAnotherSessionsPreview(t *testing.T) {
	t.Run("stepped to with Down", func(t *testing.T) {
		m, clock := gesturePreviewModel(t)
		m = swipe(t, m, clock, 5)
		m = pressPreviewStep(t, m, tea.KeyDown)
		if m.preview.session != "bravo" {
			t.Fatalf("fixture: Down previewed %q, want bravo", m.preview.session)
		}
		tail := m.preview.viewport.YOffset()
		m = swipe(t, m, clock, 10)
		requirePreviewOffset(t, m, tail, "momentum from alpha's preview")
	})

	t.Run("reopened with Esc then Space", func(t *testing.T) {
		m, clock := gesturePreviewModel(t)
		m = swipe(t, m, clock, 5)
		m = applyKey(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
		m = openPreviewOn(t, m)
		tail := m.preview.viewport.YOffset()
		m = swipe(t, m, clock, 10)
		requirePreviewOffset(t, m, tail, "momentum from the preview before")
	})
}

func TestWheelGestureThatLeavesItsSurfaceNeverReachesItAgain(t *testing.T) {
	m, clock := gesturePreviewModel(t)
	m = applyKey(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	clock.at = clock.at.Add(wheelSettle)
	m = swipe(t, m, clock, 1)
	requireSessionIndex(t, m, 1, "fixture: the swipe onto bravo")

	m = openPreviewOn(t, m)
	m = swipe(t, m, clock, 2)
	m = applyKey(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	m = swipe(t, m, clock, 4)
	requireSessionIndex(t, m, 1, "momentum back on the list it began on")
}

func TestWheelGestureBegunOverThePreviewsHelpNeverScrollsThePreview(t *testing.T) {
	m, clock := gesturePreviewModel(t)
	help := tea.KeyPressMsg{Code: '?', Text: "?"}
	m = applyKey(t, m, help)
	if !m.preview.helpOpen {
		t.Fatal("fixture: ? did not open the preview's help")
	}
	tail := m.preview.viewport.YOffset()
	m = swipe(t, m, clock, 5)
	m = applyKey(t, m, help)
	if m.preview.helpOpen {
		t.Fatal("fixture: ? did not close the preview's help")
	}
	m = swipe(t, m, clock, 10)
	requirePreviewOffset(t, m, tail, "momentum begun over the help")
}

func TestWheelGestureBegunOverTheThemeConfirmNeverMovesThePanel(t *testing.T) {
	m, _ := newSlotConfirmModel(t)
	clock := stageWheelClock(&m)
	m = raiseSlotConfirmForTest(t, m, slotLightPress, theme.MemberLight)
	m = swipe(t, m, clock, 5)
	m, _ = pressConfirmKey(t, m, confirmNo)
	if !m.themePanel.open || m.themePanel.confirming() {
		t.Fatal("fixture: n did not resolve the confirm with the panel left open")
	}
	before := arrowPanelIndex(m)
	m = swipe(t, m, clock, 10)
	if got := arrowPanelIndex(m); got != before {
		t.Fatalf("momentum begun over the confirm moved the panel cursor from %d to %d", before, got)
	}
}

func TestWheelGestureBegunUnderTheBurstLockNeverMovesTheList(t *testing.T) {
	m := pointerSessionsModel(t, pointerTermH, pointerSessions("alpha", "bravo", "charlie"))
	clock := stageWheelClock(&m)
	m.burstPending = true
	m = swipe(t, m, clock, 5)
	m.burstPending = false
	m = swipe(t, m, clock, 10)
	requireSessionIndex(t, m, 0, "momentum begun under the burst lock")

	clock.at = clock.at.Add(wheelSettle)
	m = swipe(t, m, clock, 1)
	requireSessionIndex(t, m, 1, "a fresh swipe once the lock lifted")
}

// typeAmidSwipes types each letter with a wheel step before it, and fails if any
// letter ran as a command rather than reaching the page's filter.
func typeAmidSwipes(t *testing.T, m Model, clock *wheelClock, page filterPage, letters string) Model {
	t.Helper()
	for _, r := range letters {
		m = swipe(t, m, clock, 1)
		var cmd tea.Cmd
		m, cmd = pointerUpdate(t, m, tea.KeyPressMsg{Code: r, Text: string(r)})
		if isQuitCmd(cmd) {
			t.Fatalf("%q after a wheel step quit the picker", r)
		}
		if m.activePage != page.page || !page.list(m).SettingFilter() {
			t.Fatalf("%q after a wheel step left the filter input unfocused (page %v)", r, m.activePage)
		}
	}
	return m
}

// A filtering page, with letters each of which is one of its commands.
type filterPage struct {
	name    string
	page    page
	model   func(t *testing.T) Model
	list    func(Model) list.Model
	letters string
}

var filterPages = []filterPage{
	{
		name: "sessions",
		page: PageSessions,
		model: func(t *testing.T) Model {
			return pointerSessionsModel(t, pointerTermH, pointerSessions("alpha", "bravo", "charlie"))
		},
		list:    func(m Model) list.Model { return m.sessionList },
		letters: "anqs",
	},
	{
		name: "projects",
		page: PageProjects,
		model: func(t *testing.T) Model {
			projects := []project.Project{
				{Path: "/work/orion", Name: "Orion"},
				{Path: "/work/vega", Name: "Vega"},
				{Path: "/work/lyra", Name: "Lyra"},
			}
			return pointerProjectsModel(t, projects, &recordingCreator{})
		},
		list:    func(m Model) list.Model { return m.projectList },
		letters: "oxeq",
	},
}

func TestWheelStepsNeverReachTheListWhileItsFilterIsFocused(t *testing.T) {
	slash := tea.KeyPressMsg{Code: '/', Text: "/"}
	for _, fp := range filterPages {
		t.Run(fp.name+": momentum from the list while typing", func(t *testing.T) {
			m := fp.model(t)
			clock := stageWheelClock(&m)
			m = swipe(t, m, clock, 2)
			m, _ = pointerUpdate(t, m, slash)
			m = typeAmidSwipes(t, m, clock, fp, fp.letters)
			if got := fp.list(m).FilterValue(); got != fp.letters {
				t.Fatalf("the filter holds %q, want every letter typed", got)
			}
		})

		t.Run(fp.name+": a fresh swipe while typing", func(t *testing.T) {
			m := fp.model(t)
			clock := stageWheelClock(&m)
			m, _ = pointerUpdate(t, m, slash)
			clock.at = clock.at.Add(wheelSettle)
			m = typeAmidSwipes(t, m, clock, fp, fp.letters[:1])
			clock.at = clock.at.Add(wheelSettle)
			m = typeAmidSwipes(t, m, clock, fp, fp.letters[1:])
			if got := fp.list(m).FilterValue(); got != fp.letters {
				t.Fatalf("the filter holds %q, want every letter typed", got)
			}
		})

		t.Run(fp.name+": a swipe begun in the filter, which then closes", func(t *testing.T) {
			m := fp.model(t)
			clock := stageWheelClock(&m)
			m, _ = pointerUpdate(t, m, slash)
			m = swipe(t, m, clock, 3)
			m, _ = pointerUpdate(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
			if fp.list(m).SettingFilter() {
				t.Fatal("fixture: Esc left the filter focused")
			}
			m = swipe(t, m, clock, 4)
			if got := fp.list(m).Index(); got != 0 {
				t.Fatalf("momentum begun in the filter moved the cursor to %d", got)
			}
		})
	}
}

func TestWheelGestureBegunOnAnotherPageNeverMovesThisOne(t *testing.T) {
	t.Run("sessions to projects with x", func(t *testing.T) {
		m := filterPages[1].model(t)
		m.activePage = PageSessions
		clock := stageWheelClock(&m)
		m = swipe(t, m, clock, 2)
		m = applyKey(t, m, tea.KeyPressMsg{Code: 'x', Text: "x"})
		if m.activePage != PageProjects {
			t.Fatalf("fixture: x left page %v, want projects", m.activePage)
		}
		before := m.projectList.Index()
		m = swipe(t, m, clock, 4)
		if got := m.projectList.Index(); got != before {
			t.Fatalf("momentum from the sessions page moved the project cursor from %d to %d", before, got)
		}
	})

	t.Run("loading to sessions", func(t *testing.T) {
		m := pointerSessionsModel(t, pointerTermH, pointerSessions("alpha", "bravo", "charlie"))
		clock := stageWheelClock(&m)
		m.activePage = PageLoading
		m = swipe(t, m, clock, 5)
		m.activePage = PageSessions
		m = swipe(t, m, clock, 4)
		requireSessionIndex(t, m, 0, "momentum begun on the loading page")
	})
}

func TestWheelStreamRunningAsThePickerOpensNeverReachesIt(t *testing.T) {
	open := func(t *testing.T) (Model, *wheelClock) {
		t.Helper()
		clock := &wheelClock{at: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
		m := New(fakeLister{}, WithClock(clock.now))
		m.termWidth, m.termHeight = pointerTermW, pointerTermH
		m.applySessions(pointerSessions("alpha", "bravo", "charlie"), nil)
		m.resyncPageLayouts()
		if m.activePage != PageSessions {
			t.Fatalf("fixture: the picker opened on page %v, want sessions", m.activePage)
		}
		return m, clock
	}

	t.Run("a stream from the moment it opens is dropped", func(t *testing.T) {
		m, clock := open(t)
		m = swipe(t, m, clock, 4)
		requireSessionIndex(t, m, 0, "a stream running as the picker opened")
	})

	t.Run("a stream outlasting the settle stays dropped until it stops", func(t *testing.T) {
		m, clock := open(t)
		m = swipe(t, m, clock, int(3*wheelSettle/momentumGap)+1)
		requireSessionIndex(t, m, 0, "a stream running on past the settle")
		clock.at = clock.at.Add(wheelSettle)
		m, _ = pointerUpdate(t, m, fingerDown)
		requireSessionIndex(t, m, 1, "the first step after the stream stopped")
	})

	t.Run("a first swipe a settle after it opens moves the list", func(t *testing.T) {
		m, clock := open(t)
		clock.at = clock.at.Add(wheelSettle)
		m, _ = pointerUpdate(t, m, fingerDown)
		requireSessionIndex(t, m, 1, "a fresh swipe once the picker had been quiet")
	})

	t.Run("a key at once acts", func(t *testing.T) {
		m, _ := open(t)
		m, _ = pointerUpdate(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
		requireSessionIndex(t, m, 1, "Down as the picker opened")
	})
}
