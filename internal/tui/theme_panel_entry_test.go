package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/theme"
)

// Verbatim, not the production constants: a test asserting a constant against
// itself pins nothing.
const (
	wantNoColorEntryFlash = "theme picker needs colour — NO_COLOR is set"
	wantNarrowEntryFlash  = "terminal too narrow for the theme picker"
	wantShortEntryFlash   = "terminal too short for the theme picker"
)

const (
	entryContentW = 96
	entryContentH = 26
)

func entryRows(t *testing.T) []theme.Row {
	t.Helper()
	return arrowValidRows(t, 4)
}

// The recorded open count discriminates the two refusal shapes: a proactive
// block reads nothing, the post-read re-evaluation has already enumerated.
func newEntryThemeSource(t *testing.T, dirUnusable bool) *fakeThemeSource {
	t.Helper()
	rows := entryRows(t)
	return &fakeThemeSource{
		enumeration: theme.Enumeration{DirPath: fixtureThemesDir},
		union:       themeRowsUnionDirUnusable(rows, dirUnusable),
		resolution:  constantResolution(rows[0].Slug, rows[0].Theme),
	}
}

type entryModelOpts struct {
	page       page
	colourless bool
	contentW   int
	contentH   int
}

// The dimensions are assigned directly rather than through a tea.WindowSizeMsg,
// so the gate is asked about the region the fixture declares.
func newEntryModel(t *testing.T, e ThemeSource, o entryModelOpts) (Model, *fakeThemeSource) {
	t.Helper()

	rec, _ := e.(*fakeThemeSource)
	rows := entryRows(t)
	deps := Deps{
		Lister:        fakeLister{},
		Killer:        keymapParityKiller{},
		Renamer:       keymapParityRenamer{},
		Creator:       sessionsGuardCreator{},
		ProjectStore:  projectsParityStore{},
		ProjectEditor: stubTagsProjectEditor{},
		AliasEditor:   stubTagsAliasEditor{},
		Enumerator:    keymapParityEnumerator{},
		Reader:        keymapParityReader{},
		ThemeSource:   e,
		Theme:         theme.ConstantNomination(rows[0].Theme),
		ThemeKeys:     theme.RawKeys{Theme: rows[0].Slug},
		NoColor:       o.colourless,
	}
	m := Build(deps)
	m.termWidth, m.termHeight = geometryTerm(o.contentW, o.contentH)
	m.applySessions(closePanelSessions(), nil)
	projects := []project.Project{{Path: "/p/one", Name: "one"}, {Path: "/p/two", Name: "two"}}
	m.setProjects(projects)
	m.projectList.SetItems(ProjectsToListItems(projects))
	m.projectList.Select(0)
	m.applySessionListSize(m.contentWidth(), m.contentHeight())
	m.applyProjectListSize(m.contentWidth(), m.contentHeight())
	m.activePage = o.page

	if got := m.contentWidth(); got != o.contentW {
		t.Fatalf("fixture: the content region is %d columns wide, want %d", got, o.contentW)
	}
	if got := m.contentHeight(); got != o.contentH {
		t.Fatalf("fixture: the content region is %d rows tall, want %d", got, o.contentH)
	}
	if m.themePanel.open {
		t.Fatal("fixture: the panel is already open")
	}
	return m, rec
}

func newUnblockedEntryModel(t *testing.T, p page) (Model, *fakeThemeSource) {
	t.Helper()
	return newEntryModel(t, newEntryThemeSource(t, false), entryModelOpts{
		page:     p,
		contentW: entryContentW,
		contentH: entryContentH,
	})
}

var entryPages = []struct {
	name string
	page page
}{
	{name: "sessions", page: PageSessions},
	{name: "projects", page: PageProjects},
}

func requireBlocked(t *testing.T, m Model, cmd tea.Cmd, wantFlash string) {
	t.Helper()
	if m.themePanel.open {
		t.Fatal("t opened the panel where the entry gate must refuse")
	}
	if got := m.flashText; got != wantFlash {
		t.Errorf("the blocked t raised %q, want %q", got, wantFlash)
	}
	if cmd == nil {
		t.Error("the blocked t scheduled no auto-clear tick; a blocked entry inherits the standard flash lifecycle")
	}
}

// Only valid where the content region holds the message on one line: the band
// wraps below that.
func requireFlashBandVisible(t *testing.T, m Model, wantFlash string) {
	t.Helper()
	if got := ansi.Strip(m.View().Content); !strings.Contains(got, wantFlash) {
		t.Errorf("the frame carries no %q band:\n%s", wantFlash, got)
	}
}

func pressThemeKeyCmd(t *testing.T, m Model) (Model, tea.Cmd) {
	t.Helper()
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	return updated.(Model), cmd
}

func requireSilentRefusal(t *testing.T, m Model, rec *fakeThemeSource, where string) {
	t.Helper()
	if m.themePanel.open {
		t.Errorf("t opened the panel on %s, where it is not bound", where)
	}
	if rec.opens != 0 {
		t.Errorf("t ran %d enumerations on %s, want 0", rec.opens, where)
	}
	if got := m.flashText; got != "" {
		t.Errorf("t raised %q on %s; the refusal is SILENT where the key is not bound at all", got, where)
	}
}

// The entry gate blocks `t` under NO_COLOR, but the colourless render path is
// real: the same armThemePanel, with only the gate skipped.
func armPanelUnderNoColorForTest(t *testing.T, m Model) Model {
	t.Helper()
	if !m.colourless {
		t.Fatal("fixture: the model is not colourless, so `t` is not blocked and this bypass says nothing")
	}
	enumeration, union := m.themeState.source.Open(m.themeState.keys)
	(&m).armThemePanel(enumeration, union)
	if !m.themePanel.open {
		t.Fatal("fixture: arming the panel directly left it closed")
	}
	return m
}

func TestPanelEntry_OpensOnSessionsAndProjects(t *testing.T) {
	for _, tc := range entryPages {
		t.Run(tc.name, func(t *testing.T) {
			m, rec := newUnblockedEntryModel(t, tc.page)

			m, _ = pressThemeKeyCmd(t, m)

			if !m.themePanel.open {
				t.Fatalf("t did not open the panel on %s", tc.name)
			}
			if rec.opens != 1 {
				t.Errorf("the keypress ran %d enumerations, want exactly 1", rec.opens)
			}
			if got := m.flashText; got != "" {
				t.Errorf("an unblocked t raised %q, want no flash", got)
			}
			if m.activePage != tc.page {
				t.Errorf("opening the panel moved the active page to %d, want %d", m.activePage, tc.page)
			}
		})
	}
}

func TestPanelEntry_NoColorBlocked(t *testing.T) {
	for _, tc := range entryPages {
		t.Run(tc.name, func(t *testing.T) {
			m, rec := newEntryModel(t, newEntryThemeSource(t, false), entryModelOpts{
				page:       tc.page,
				colourless: true,
				contentW:   entryContentW,
				contentH:   entryContentH,
			})

			m, cmd := pressThemeKeyCmd(t, m)

			requireBlocked(t, m, cmd, wantNoColorEntryFlash)
			requireFlashBandVisible(t, m, wantNoColorEntryFlash)
			if rec.opens != 0 {
				t.Errorf("the blocked t ran %d enumerations, want 0 — the NO_COLOR block is proactive", rec.opens)
			}
		})
	}
}

func TestPanelEntry_FloorBlocked(t *testing.T) {
	floor := themePanelMinHeight(themePanelKeymap(), false)

	for _, tc := range []struct {
		name      string
		contentW  int
		contentH  int
		wantFlash string
	}{
		{name: "too narrow", contentW: themePanelMinWidth - 1, contentH: entryContentH, wantFlash: wantNarrowEntryFlash},
		{name: "too short", contentW: entryContentW, contentH: floor - 1, wantFlash: wantShortEntryFlash},
		{name: "both fail", contentW: themePanelMinWidth - 1, contentH: floor - 1, wantFlash: wantNarrowEntryFlash},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, rec := newEntryModel(t, newEntryThemeSource(t, false), entryModelOpts{
				page:     PageSessions,
				contentW: tc.contentW,
				contentH: tc.contentH,
			})

			m, cmd := pressThemeKeyCmd(t, m)

			requireBlocked(t, m, cmd, tc.wantFlash)
			if rec.opens != 0 {
				t.Errorf("the blocked t ran %d enumerations, want 0 — the floor is read before the directory is", rec.opens)
			}
		})
	}
}

// The height is exactly the non-directory floor — where the pre-read assumption
// (DirUnusable false, since the enumeration has not run) and the real one differ.
func TestPanelEntry_UsableDirectoryOpensAtTheNonDirFloor(t *testing.T) {
	entries := themePanelKeymap()
	floor := themePanelMinHeight(entries, false)
	if themePanelMinHeight(entries, true) != floor+1 {
		t.Fatalf("fixture: the directory-inclusive floor is %d and the plain one %d; the two must differ by exactly one row or this height discriminates nothing",
			themePanelMinHeight(entries, true), floor)
	}

	m, rec := newEntryModel(t, newEntryThemeSource(t, false), entryModelOpts{
		page:     PageSessions,
		contentW: entryContentW,
		contentH: floor,
	})

	m, _ = pressThemeKeyCmd(t, m)

	if !m.themePanel.open {
		t.Fatalf("t refused at exactly the %d-row floor with a USABLE directory — the gate reserved the `%s` row speculatively", floor, themePanelDirUnreadable)
	}
	if rec.opens != 1 {
		t.Errorf("the keypress ran %d enumerations, want exactly 1", rec.opens)
	}
	if got := m.flashText; got != "" {
		t.Errorf("the open raised %q, want no flash", got)
	}
}

func TestPanelEntry_UnusableDirectoryBlocksOnTheReEvaluation(t *testing.T) {
	entries := themePanelKeymap()
	floor := themePanelMinHeight(entries, false)

	t.Run("at the non-directory floor it discards the enumeration and refuses", func(t *testing.T) {
		m, rec := newEntryModel(t, newEntryThemeSource(t, true), entryModelOpts{
			page:     PageSessions,
			contentW: entryContentW,
			contentH: floor,
		})

		m, cmd := pressThemeKeyCmd(t, m)

		requireBlocked(t, m, cmd, wantShortEntryFlash)
		requireFlashBandVisible(t, m, wantShortEntryFlash)
		if rec.opens != 1 {
			t.Errorf("the re-evaluation ran %d enumerations, want exactly 1 — the read happens, then the floor refuses", rec.opens)
		}
		if got := m.themePanel; got.width != 0 || len(got.union.Rows) != 0 || len(got.enumeration.Entries) != 0 || got.badges != nil {
			t.Errorf("the refused open retained panel state %+v, want the zero value — the enumeration is discarded", got)
		}
	})

	t.Run("one row higher it opens with a list row beneath the warning", func(t *testing.T) {
		height := themePanelMinHeight(entries, true)
		m, _ := newEntryModel(t, newEntryThemeSource(t, true), entryModelOpts{
			page:     PageSessions,
			contentW: entryContentW,
			contentH: height,
		})

		m, _ = pressThemeKeyCmd(t, m)

		if !m.themePanel.open {
			t.Fatalf("t refused at the %d-row directory-inclusive floor, which the entry gate admits", height)
		}
		lines := themePanelLines(renderThemePanel(m.themePanel, m.contentHeight(), m.themeState.active, m.colourless))
		if len(lines) != height {
			t.Fatalf("the panel rendered %d rows at the %d-row floor", len(lines), height)
		}
		header := panelHeaderRowsOf(m)
		if got, want := strings.TrimRight(lines[header], " "), themePanelContentPrefix()+themePanelDirUnreadable; got != want {
			t.Fatalf("the row under the header = %q, want the pinned %q", got, want)
		}
		if got, want := lines[header+1], entryRows(t)[0].Label(); !strings.Contains(got, want) {
			t.Errorf("the row beneath the warning = %q, want the list row %q — rows must sit BENEATH it", got, want)
		}
	})
}

func TestPanelEntry_SameFloorAsResize(t *testing.T) {
	floor := themePanelMinHeight(themePanelKeymap(), false)

	for _, region := range [][2]int{
		{entryContentW, entryContentH},
		{themePanelMinWidth, floor},
		{themePanelMinWidth - 1, entryContentH},
		{entryContentW, floor - 1},
		{themePanelMinWidth - 1, floor - 1},
		{60, entryContentH},
	} {
		contentW, contentH := region[0], region[1]
		t.Run(fmt.Sprintf("%dx%d", contentW, contentH), func(t *testing.T) {
			dim, ok := themePanelFloor(contentW, contentH, false)

			entered, _ := pressThemeKeyCmd(t, mustEntryModel(t, contentW, contentH))
			if entered.themePanel.open != ok {
				t.Fatalf("t left the panel open=%v, want %v — the predicate said (%v, ok=%v)", entered.themePanel.open, ok, dim, ok)
			}

			opened, _ := pressThemeKeyCmd(t, mustEntryModel(t, entryContentW, entryContentH))
			if !opened.themePanel.open {
				t.Fatal("fixture: the panel did not open at the unblocked region")
			}
			resized, resizeCmd := resizeForTestCmd(t, opened, contentW, contentH)
			if resized.themePanel.open != ok {
				t.Fatalf("the resize left the panel open=%v, want %v — one predicate, two callers", resized.themePanel.open, ok)
			}

			if ok {
				if got := entered.flashText; got != "" {
					t.Errorf("an admitted entry raised %q, want no flash", got)
				}
				if got := resized.flashText; got != "" {
					t.Errorf("an admitted resize raised %q, want no flash", got)
				}
				return
			}
			if got, want := entered.flashText, wantEntryFlash(dim); got != want {
				t.Errorf("the blocked entry raised %q, want the %v copy %q", got, dim, want)
			}
			if got, want := resized.flashText, themePanelForcedCloseFlash(dim); got != want {
				t.Errorf("the forced close raised %q, want the %v copy %q", got, dim, want)
			}
			// Presence only: evaluating the auto-clear tick it carries blocks for
			// flashAutoClearDuration, once per region.
			if resizeCmd == nil {
				t.Error("the forced close returned no command, so its flash never auto-clears — a blocked entry schedules one")
			}
		})
	}
}

func wantEntryFlash(dim themePanelDim) string {
	if dim == dimHeight {
		return wantShortEntryFlash
	}
	return wantNarrowEntryFlash
}

func mustEntryModel(t *testing.T, contentW, contentH int) Model {
	t.Helper()
	m, _ := newEntryModel(t, newEntryThemeSource(t, false), entryModelOpts{
		page:     PageSessions,
		contentW: contentW,
		contentH: contentH,
	})
	return m
}

func TestPanelEntry_SilentOnPreviewAndLoading(t *testing.T) {
	t.Run("preview", func(t *testing.T) {
		m, rec := newUnblockedEntryModel(t, PageSessions)
		m = pressPanelKey(t, m, tea.KeyPressMsg{Code: tea.KeySpace})
		if m.activePage != pagePreview {
			t.Fatalf("fixture: Space landed page %d, want the preview page", m.activePage)
		}

		m, cmd := pressThemeKeyCmd(t, m)

		requireSilentRefusal(t, m, rec, "the preview page")
		if cmd != nil {
			t.Error("t on the preview page returned a command; it is not bound there at all")
		}
		if m.activePage != pagePreview {
			t.Errorf("t moved the preview page to %d", m.activePage)
		}
	})

	t.Run("loading", func(t *testing.T) {
		m, rec := newUnblockedEntryModel(t, PageLoading)

		m, cmd := pressThemeKeyCmd(t, m)

		requireSilentRefusal(t, m, rec, "the loading page")
		if cmd != nil {
			t.Error("t on the loading page returned a command; the page is inert by design")
		}
		if m.activePage != PageLoading {
			t.Errorf("t moved the loading page to %d", m.activePage)
		}
	})
}

func TestPanelEntry_SwallowedWhileBurstPending(t *testing.T) {
	m, rec := newUnblockedEntryModel(t, PageSessions)
	m.burstPending = true

	m, cmd := pressThemeKeyCmd(t, m)

	requireSilentRefusal(t, m, rec, "a pending burst")
	if cmd != nil {
		t.Error("t returned a command while the burst input-lock was engaged")
	}
	if !m.burstPending {
		t.Error("t cleared the pending burst")
	}
}

func TestPanelEntry_ModalKeepsTheKey(t *testing.T) {
	for _, tc := range []struct {
		name  string
		page  page
		press tea.KeyPressMsg
		modal modalState
	}{
		{name: "sessions help", page: PageSessions, press: tea.KeyPressMsg{Code: '?', Text: "?"}, modal: modalHelp},
		{name: "sessions kill", page: PageSessions, press: tea.KeyPressMsg{Code: 'k', Text: "k"}, modal: modalKillConfirm},
		{name: "projects delete", page: PageProjects, press: tea.KeyPressMsg{Code: 'd', Text: "d"}, modal: modalDeleteProject},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, rec := newUnblockedEntryModel(t, tc.page)
			m = pressPanelKey(t, m, tc.press)
			if m.modal != tc.modal {
				t.Fatalf("fixture: %v opened modal %d, want %d", tc.press, m.modal, tc.modal)
			}

			m, cmd := pressThemeKeyCmd(t, m)

			requireSilentRefusal(t, m, rec, "an open modal")
			if cmd != nil {
				t.Error("t returned a command while a modal owned the keyboard")
			}
			if m.modal != tc.modal {
				t.Errorf("t closed the modal (now %d, want %d)", m.modal, tc.modal)
			}
		})
	}
}

// The tick is dispatched as a message; invoking the returned command would
// sleep for the real auto-clear duration.
func TestPanelEntry_BlockedFlashLifecycle(t *testing.T) {
	for _, tc := range entryPages {
		blocked := func(t *testing.T) Model {
			t.Helper()
			m, _ := newEntryModel(t, newEntryThemeSource(t, false), entryModelOpts{
				page:       tc.page,
				colourless: true,
				contentW:   entryContentW,
				contentH:   entryContentH,
			})
			m, cmd := pressThemeKeyCmd(t, m)
			requireBlocked(t, m, cmd, wantNoColorEntryFlash)
			return m
		}

		t.Run(tc.name+"/the tick clears it", func(t *testing.T) {
			m := blocked(t)

			updated, _ := m.Update(flashTickMsg{Gen: m.flashGen})

			if got := updated.(Model).flashText; got != "" {
				t.Errorf("the matching tick left the flash %q, want it cleared", got)
			}
		})

		t.Run(tc.name+"/the next actionable key clears it", func(t *testing.T) {
			m := blocked(t)

			m = pressPanelKey(t, m, tea.KeyPressMsg{Code: tea.KeyDown})

			if got := m.flashText; got != "" {
				t.Errorf("the next actionable key left the flash %q, want it cleared", got)
			}
		})
	}
}

func TestPanelEntry_PinnedCopy(t *testing.T) {
	for _, tc := range []struct {
		got, want string
	}{
		{got: themePanelNoColorFlash, want: wantNoColorEntryFlash},
		{got: themePanelNarrowEntryFlash, want: wantNarrowEntryFlash},
		{got: themePanelShortEntryFlash, want: wantShortEntryFlash},
	} {
		if tc.got != tc.want {
			t.Errorf("the pinned constant is %q, want %q", tc.got, tc.want)
		}
	}
}

func openEntryPanel(t *testing.T, p page) Model {
	t.Helper()
	m, _ := newUnblockedEntryModel(t, p)
	m, _ = pressThemeKeyCmd(t, m)
	if !m.themePanel.open {
		t.Fatal("fixture: the panel did not open")
	}
	return m
}

func TestPanelRouting_KeyExclusive(t *testing.T) {
	for _, tc := range []struct {
		name   string
		page   page
		press  tea.KeyPressMsg
		effect func(Model, tea.Cmd) bool
		desc   string
	}{
		{
			name:   "k does not kill",
			page:   PageSessions,
			press:  tea.KeyPressMsg{Code: 'k', Text: "k"},
			effect: func(m Model, _ tea.Cmd) bool { return m.modal == modalKillConfirm },
			desc:   "the kill confirm modal",
		},
		{
			name:   "x does not switch page",
			page:   PageSessions,
			press:  tea.KeyPressMsg{Code: 'x', Text: "x"},
			effect: func(m Model, _ tea.Cmd) bool { return m.activePage == PageProjects },
			desc:   "the Projects page",
		},
		{
			name:   "m does not enter multi-select",
			page:   PageSessions,
			press:  tea.KeyPressMsg{Code: 'm', Text: "m"},
			effect: func(m Model, _ tea.Cmd) bool { return m.MultiSelectActive() },
			desc:   "multi-select mode",
		},
		{
			name:   "/ does not filter",
			page:   PageSessions,
			press:  tea.KeyPressMsg{Code: '/', Text: "/"},
			effect: func(m Model, _ tea.Cmd) bool { return m.sessionList.SettingFilter() },
			desc:   "the filter input",
		},
		{
			name:   "? opens no help",
			page:   PageSessions,
			press:  tea.KeyPressMsg{Code: '?', Text: "?"},
			effect: func(m Model, _ tea.Cmd) bool { return m.modal == modalHelp },
			desc:   "the help modal",
		},
		{
			name:   "s does not cycle the grouping",
			page:   PageSessions,
			press:  tea.KeyPressMsg{Code: 's', Text: "s"},
			effect: func(m Model, _ tea.Cmd) bool { return m.sessionListMode != prefs.ModeFlat },
			desc:   "the next grouping mode",
		},
		{
			name:   "e opens no editor",
			page:   PageProjects,
			press:  tea.KeyPressMsg{Code: 'e', Text: "e"},
			effect: func(m Model, _ tea.Cmd) bool { return m.modal == modalEditProject },
			desc:   "the project edit modal",
		},
		{
			name:   "n mints nothing",
			page:   PageSessions,
			press:  tea.KeyPressMsg{Code: 'n', Text: "n"},
			effect: func(_ Model, cmd tea.Cmd) bool { return cmd != nil },
			desc:   "the new-session-in-cwd command",
		},
		{
			name:   "r opens no rename",
			page:   PageSessions,
			press:  tea.KeyPressMsg{Code: 'r', Text: "r"},
			effect: func(m Model, _ tea.Cmd) bool { return m.modal == modalRename },
			desc:   "the rename modal",
		},
		{
			name:   "q does not quit",
			page:   PageSessions,
			press:  tea.KeyPressMsg{Code: 'q', Text: "q"},
			effect: func(_ Model, cmd tea.Cmd) bool { return isQuitCmd(cmd) },
			desc:   "the quit",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			closed, _ := newUnblockedEntryModel(t, tc.page)
			controlModel, controlCmd := closed.Update(tc.press)
			if !tc.effect(controlModel.(Model), controlCmd) {
				t.Fatalf("precondition: %v does not reach %s with the panel CLOSED, so a swallow proves nothing", tc.press, tc.desc)
			}

			updated, cmd := openEntryPanel(t, tc.page).Update(tc.press)
			m := updated.(Model)

			if tc.effect(m, cmd) {
				t.Errorf("%v reached %s while the panel was open — the panel is key-exclusive", tc.press, tc.desc)
			}
			if !m.themePanel.open {
				t.Errorf("%v closed the panel; only Esc closes it", tc.press)
			}
			if m.activePage != tc.page {
				t.Errorf("%v moved the active page to %d, want it left on %d", tc.press, m.activePage, tc.page)
			}
		})
	}
}

func TestPanelRouting_PanelOwnedKeysNeverReachThePage(t *testing.T) {
	t.Run("d opens no delete modal on Projects", func(t *testing.T) {
		press := tea.KeyPressMsg{Code: 'd', Text: "d"}

		closed, _ := newUnblockedEntryModel(t, PageProjects)
		control, _ := closed.Update(press)
		if control.(Model).modal != modalDeleteProject {
			t.Fatal("precondition: `d` opens no delete modal with the panel CLOSED, so the absence below proves nothing")
		}

		m := pressPanelKey(t, openEntryPanel(t, PageProjects), press)

		if m.modal != modalNone {
			t.Errorf("`d` opened modal %d while the panel was open; it is the panel's key, not the page's", m.modal)
		}
		if m.activePage != PageProjects {
			t.Errorf("`d` moved the active page to %d", m.activePage)
		}
	})

	t.Run("l reaches no page binding", func(t *testing.T) {
		press := tea.KeyPressMsg{Code: 'l', Text: "l"}

		for _, tc := range entryPages {
			t.Run(tc.name, func(t *testing.T) {
				before := openEntryPanel(t, tc.page)
				updated, cmd := before.Update(press)
				m := updated.(Model)

				if m.modal != modalNone {
					t.Errorf("`l` opened modal %d on %s", m.modal, tc.name)
				}
				if m.activePage != tc.page {
					t.Errorf("`l` moved the active page to %d on %s", m.activePage, tc.name)
				}
				if cmd != nil {
					t.Errorf("`l` produced a page command on %s", tc.name)
				}
				if got, want := m.sessionList.Index(), before.sessionList.Index(); got != want {
					t.Errorf("`l` moved the sessions cursor %d → %d on %s", want, got, tc.name)
				}
				if got, want := m.projectList.Index(), before.projectList.Index(); got != want {
					t.Errorf("`l` moved the projects cursor %d → %d on %s", want, got, tc.name)
				}
				if m.sessionList.SettingFilter() || m.projectList.SettingFilter() {
					t.Errorf("`l` focused a filter input on %s", tc.name)
				}
			})
		}
	})
}

func TestPanelRouting_CtrlCQuits(t *testing.T) {
	for _, tc := range entryPages {
		t.Run(tc.name, func(t *testing.T) {
			_, cmd := openEntryPanel(t, tc.page).Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})

			if !isQuitCmd(cmd) {
				t.Errorf("Ctrl-C did not quit from the open panel on %s — it must never be swallowed", tc.name)
			}
		})
	}
}

func TestPanelRouting_NestsOverMultiSelect(t *testing.T) {
	m, _ := newUnblockedEntryModel(t, PageSessions)
	m = pressPanelKey(t, m, tea.KeyPressMsg{Code: 'm', Text: "m"})
	if !m.MultiSelectActive() {
		t.Fatal("precondition: `m` did not enter multi-select")
	}
	marked := m.SelectedSessionCount()
	if marked == 0 {
		t.Fatal("precondition: entering multi-select marked nothing, so an intact set proves nothing")
	}
	first := closePanelSessions()[0].Name
	if !m.IsSessionSelected(first) {
		t.Fatalf("precondition: entering multi-select did not mark %q", first)
	}

	m, _ = pressThemeKeyCmd(t, m)

	if !m.themePanel.open {
		t.Fatal("`t` did not open the panel over multi-select")
	}
	if !m.MultiSelectActive() {
		t.Error("opening the panel exited multi-select; the panel NESTS over the mode")
	}
	if got := m.SelectedSessionCount(); got != marked {
		t.Errorf("the marked set holds %d session(s) with the panel open, want the %d marked before", got, marked)
	}
	if !m.IsSessionSelected(first) {
		t.Errorf("%q is no longer marked with the panel open", first)
	}
	if got := ansi.Strip(m.View().Content); !strings.Contains(got, fmt.Sprintf("%d selected", marked)) {
		t.Errorf("the panelled frame carries no `%d selected` banner; it sits in the notice band and stays visible behind the panel:\n%s", marked, got)
	}

	m = closeThemePanelForTest(t, m)

	if m.themePanel.open {
		t.Fatal("Esc left the panel open")
	}
	if !m.MultiSelectActive() {
		t.Error("the close exited multi-select; Esc resolves innermost-first")
	}
	if got := m.SelectedSessionCount(); got != marked {
		t.Errorf("the marked set holds %d session(s) after the close, want %d", got, marked)
	}
}
