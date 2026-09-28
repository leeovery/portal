package tui

import (
	"go/ast"
	"maps"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/sourceguardtest"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/themetest"
)

// Uppercase always carries ModShift on both terminal paths; the `*Upper` pair is
// the defensive `Mod == 0` shape. Caps and num lock ride on the base rune with a
// lock modifier, and bubbletea gives modifier combos an empty Key.Text.
var (
	confirmYes         = tea.KeyPressMsg{Code: 'y', Text: "y"}
	confirmYesUpper    = tea.KeyPressMsg{Code: 'Y', Text: "Y"}
	confirmYesShift    = tea.KeyPressMsg{Code: 'y', Text: "Y", Mod: tea.ModShift}
	confirmYesCapsLock = tea.KeyPressMsg{Code: 'y', Text: "Y", Mod: tea.ModCapsLock}
	confirmYesNumLock  = tea.KeyPressMsg{Code: 'y', Text: "y", Mod: tea.ModNumLock}
	confirmYesCtrl     = tea.KeyPressMsg{Code: 'y', Mod: tea.ModCtrl}
	confirmNo          = tea.KeyPressMsg{Code: 'n', Text: "n"}
	confirmNoUpper     = tea.KeyPressMsg{Code: 'N', Text: "N"}
	confirmNoShift     = tea.KeyPressMsg{Code: 'n', Text: "N", Mod: tea.ModShift}
	confirmNoAlt       = tea.KeyPressMsg{Code: 'n', Mod: tea.ModAlt}
	confirmEsc         = tea.KeyPressMsg{Code: tea.KeyEscape}
	confirmCtrlC       = tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
)

func slotConfirmRows(t *testing.T) []theme.Row {
	t.Helper()
	return arrowValidRows(t, 6)
}

// The cursor is arrowed off the persisted row, so the pending slug and the
// constant being cleared are distinguishable strings.
func newSlotConfirmModel(t *testing.T) (Model, *fakeThemePersister) {
	t.Helper()
	return newSlotConfirmModelAt(t, arrowTermH)
}

func newSlotConfirmModelAt(t *testing.T, termH int) (Model, *fakeThemePersister) {
	t.Helper()

	rows := slotConfirmRows(t)
	m := newArrowPanelModelAt(t, rows, rows[0].Slug, termH)
	persister := &fakeThemePersister{}
	WithThemePersister(persister)(&m)
	// The `k` control needs a killer to reach the confirm modal with the panel
	// closed; a nil one makes the key a no-op for an unrelated reason.
	m.sessionKiller = keymapParityKiller{}

	m = arrowToThemeRow(t, m, slotConfirmTarget())
	if m.themeState.keys.Theme == slotConfirmTarget() {
		t.Fatalf("fixture: the constant and the cursor both name %q, so nothing distinguishes them", m.themeState.keys.Theme)
	}
	if got := m.themePanel.message; got.Kind != themeMessageNone {
		t.Fatalf("fixture: the panel opened with the message %+v, want an empty slot", got)
	}
	return m, persister
}

func slotConfirmConstant() string { return arrowSlug(0) }
func slotConfirmTarget() string   { return arrowSlug(2) }

// The leading phrase plus as much of the slug as any width leaves — the slug is
// what the ladder truncates, and the floor guarantees three characters. Composed
// here rather than through the production composer, which would hold vacuously.
func slotConfirmCopy(slug string) string {
	runes := []rune(slug)
	return "clear constant " + string(runes[:min(themeRowLabelFloor-1, len(runes))])
}

const slotConfirmAnswerKeys = "?  y / n"

// For equality comparisons only — this one does go through the production
// composer, whose empty output fails against a rendered row rather than passing.
func slotConfirmLine(m Model, slug string) string {
	return themePanelConfirmText(slug, themePanelInnerWidth(m.themePanel.width))
}

func raiseSlotConfirmForTest(t *testing.T, m Model, press tea.KeyPressMsg, member theme.Member) Model {
	t.Helper()
	return raiseSlotConfirmForTestAt(t, m, press, themeSlotConfirm{slug: slotConfirmTarget(), member: member})
}

func raiseSlotConfirmForTestAt(t *testing.T, m Model, press tea.KeyPressMsg, pending themeSlotConfirm) Model {
	t.Helper()

	raised, cmd := pressSlotKey(t, m, press)
	if cmd != nil {
		t.Fatalf("the raise scheduled %T; the question is asked on this keypress", cmd)
	}
	requireConfirmLive(t, raised, pending)
	return raised
}

func pressConfirmKey(t *testing.T, m Model, press tea.KeyPressMsg) (Model, tea.Cmd) {
	t.Helper()
	updated, cmd := m.Update(press)
	return updated.(Model), cmd
}

func requireConfirmLive(t *testing.T, m Model, pending themeSlotConfirm) {
	t.Helper()

	want := themePanelMessage{Kind: themeMessageConfirm, Slug: m.themeState.keys.Theme}
	if got := m.themePanel.message; got != want {
		t.Errorf("the message slot holds %+v, want %+v — the confirm names the CONSTANT being cleared", got, want)
	}
	if got := m.themePanel.pending; got != pending {
		t.Errorf("the panel holds the pending assignment %+v, want %+v", got, pending)
	}
	if !m.themePanel.open {
		t.Error("the confirm closed the panel; it is inline, not a modal, and `Esc` is the only close")
	}
}

func requireConfirmResolved(t *testing.T, m Model) {
	t.Helper()

	requireConfirmGone(t, m)
	if got := m.themePanel.message; got.Kind != themeMessageNone {
		t.Errorf("the resolved confirm left the message %+v, want the slot empty", got)
	}
}

func requireConfirmGone(t *testing.T, m Model) {
	t.Helper()

	if m.themePanel.confirming() {
		t.Errorf("the confirm is still live, holding %+v", m.themePanel.message)
	}
	if got := m.themePanel.pending; got != (themeSlotConfirm{}) {
		t.Errorf("the resolved confirm left the pending assignment %+v, want the zero value", got)
	}
	if !m.themePanel.open {
		t.Error("resolving the confirm closed the panel; only `Esc` with NO confirm live closes it")
	}
}

func slotConfirmPanelText(m Model) string {
	return ansi.Strip(renderRecomputePanel(m))
}

func slotConfirmPanelCopy(m Model) string {
	return themePanelFooterCopy(slotConfirmPanelText(m))
}

func requireConfirmFooter(t *testing.T, m Model) {
	t.Helper()

	footer := slotConfirmPanelCopy(m)
	for _, want := range []string{"y confirm", "n cancel"} {
		if !strings.Contains(footer, want) {
			t.Errorf("the panel footer does not read %q while the confirm is live:\n%s", want, slotConfirmPanelText(m))
		}
	}
	for _, gone := range themePanelFooterPinnedRows() {
		if strings.Contains(footer, gone) {
			t.Errorf("the panel footer still advertises %q while the confirm is live; none of the standing keys would act\n%s", gone, slotConfirmPanelText(m))
		}
	}
}

func requireStandingFooter(t *testing.T, m Model) {
	t.Helper()

	footer := slotConfirmPanelCopy(m)
	for _, want := range themePanelFooterPinnedRows() {
		if !strings.Contains(footer, want) {
			t.Errorf("the panel footer does not read %q once the confirm resolved:\n%s", want, slotConfirmPanelText(m))
		}
	}
	for _, gone := range []string{"y confirm", "n cancel"} {
		if strings.Contains(footer, gone) {
			t.Errorf("the panel footer still advertises the confirm's %q after it resolved\n%s", gone, slotConfirmPanelText(m))
		}
	}
}

func TestSlotConfirm_RaisedByDAndLOverAConstant(t *testing.T) {
	for _, tc := range []struct {
		name   string
		press  tea.KeyPressMsg
		member theme.Member
	}{
		{name: "d", press: slotDarkPress, member: theme.MemberDark},
		{name: "l", press: slotLightPress, member: theme.MemberLight},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, persister := newSlotConfirmModel(t)
			previewed := m.themeState.active
			index := m.themePanel.list.Index()

			m, cmd := pressSlotKey(t, m, tc.press)

			if len(persister.slugs) != 0 {
				t.Errorf("%q wrote %v while raising the confirm; nothing is written until `y`", tc.name, persister.slugs)
			}
			requireConfirmLive(t, m, themeSlotConfirm{slug: slotConfirmTarget(), member: tc.member})
			requireConstantKeys(t, m, slotConfirmConstant())
			requireConfirmFooter(t, m)
			for _, want := range []string{slotConfirmCopy(slotConfirmConstant()), slotConfirmAnswerKeys} {
				if !strings.Contains(slotConfirmPanelText(m), want) {
					t.Errorf("the message slot does not read %q:\n%s", want, slotConfirmPanelText(m))
				}
			}
			if got := m.themePanel.list.Index(); got != index {
				t.Errorf("raising the confirm moved the cursor to row %d, want it left on %d", got, index)
			}
			if m.themeState.active != previewed {
				t.Errorf("raising the confirm rendered canvas %s, want the previewed %s left alone", m.themeState.active.Canvas.Value, previewed.Canvas.Value)
			}
			if cmd != nil {
				t.Errorf("%q scheduled %T; the question is asked on this keypress", tc.name, cmd)
			}
		})
	}
}

// Structurally unreachable, so the cursor is placed directly.
func TestSlotConfirm_UnselectableRowAsksNothing(t *testing.T) {
	requireNothingAsked := func(t *testing.T, m Model, p *fakeThemePersister, rows []theme.Row) {
		t.Helper()

		if got := m.themePanel.message; got.Kind != themeMessageNone {
			t.Errorf("`d` raised the message %+v on a row it cannot commit; an empty slug is not a setting to ask about", got)
		}
		if got := m.themePanel.pending; got != (themeSlotConfirm{}) {
			t.Errorf("`d` recorded the pending assignment %+v, want the zero value — a `y` would go on to write it", got)
		}
		requireSlotCommits(t, p)
		requireConstantKeys(t, m, rows[0].Slug)
		if !m.themePanel.open {
			t.Error("`d` on a row it cannot commit closed the panel")
		}
	}

	t.Run("an unselectable row", func(t *testing.T) {
		rows := []theme.Row{arrowValidRow(t, arrowSlug(0), 0), arrowInvalidRow(arrowSlug(1))}
		m, persister := newCommitPanelModel(t, rows, rows[0].Slug)
		m.themePanel.list.Select(1)
		if themePanelCursorRow(t, m).Selectable() {
			t.Fatal("fixture: the cursor is on a selectable row, so there is no refusal to exercise")
		}

		m, cmd := pressSlotKey(t, m, slotDarkPress)

		requireNothingAsked(t, m, persister, rows)
		if cmd != nil {
			t.Errorf("`d` on an unselectable row scheduled %T, want nothing", cmd)
		}
	})

	t.Run("a selectable row carrying no slug", func(t *testing.T) {
		rows := []theme.Row{arrowValidRow(t, arrowSlug(0), 0), {Source: theme.SourceBuiltin, Theme: arrowPalette(t, 1)}}
		m, persister := newCommitPanelModel(t, rows, rows[0].Slug)
		m.themePanel.list.Select(1)
		row := themePanelCursorRow(t, m)
		if !row.Selectable() || row.Slug != "" {
			t.Fatalf("fixture: the cursor is on %+v, want a SELECTABLE row with no slug", row)
		}

		m, _ = pressSlotKey(t, m, slotDarkPress)

		requireNothingAsked(t, m, persister, rows)
	})

	t.Run("a selectable row does ask", func(t *testing.T) {
		rows := []theme.Row{arrowValidRow(t, arrowSlug(0), 0), arrowInvalidRow(arrowSlug(1))}
		m, persister := newCommitPanelModel(t, rows, rows[0].Slug)

		m, _ = pressSlotKey(t, m, slotDarkPress)

		requireConfirmLive(t, m, themeSlotConfirm{slug: rows[0].Slug, member: theme.MemberDark})
		requireSlotCommits(t, persister)
	})
}

func TestSlotConfirm_ConfirmsOnEitherCase(t *testing.T) {
	for _, tc := range []struct {
		name  string
		press tea.KeyPressMsg
	}{
		{name: "y", press: confirmYes},
		{name: "Y", press: confirmYesUpper},
		{name: "shift+y", press: confirmYesShift},
		{name: "capslock+y", press: confirmYesCapsLock},
		{name: "numlock+y", press: confirmYesNumLock},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, persister := newSlotConfirmModel(t)
			m = raiseSlotConfirmForTest(t, m, slotLightPress, theme.MemberLight)

			m, cmd := pressConfirmKey(t, m, tc.press)

			requireSlotCommits(t, persister, slotCommit{slug: slotConfirmTarget(), member: theme.MemberLight})
			requirePairKeys(t, m, slotConfirmTarget(), "")
			requireConfirmResolved(t, m)
			requireStandingFooter(t, m)
			if cmd != nil {
				t.Errorf("the answer scheduled %T; the write lands on this keypress", cmd)
			}
		})
	}
}

func TestSlotConfirm_CancelsOnThreeInputs(t *testing.T) {
	for _, tc := range []struct {
		name  string
		press tea.KeyPressMsg
	}{
		{name: "n", press: confirmNo},
		{name: "N", press: confirmNoUpper},
		{name: "shift+n", press: confirmNoShift},
		{name: "esc", press: confirmEsc},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, persister := newSlotConfirmModel(t)
			keys := m.themeState.keys
			m = raiseSlotConfirmForTest(t, m, slotDarkPress, theme.MemberDark)

			m, cmd := pressConfirmKey(t, m, tc.press)

			if len(persister.slugs) != 0 {
				t.Errorf("%q wrote %v; a cancel writes nothing", tc.name, persister.slugs)
			}
			if m.themeState.keys != keys {
				t.Errorf("%q left keys %+v, want the untouched %+v", tc.name, m.themeState.keys, keys)
			}
			requireConfirmResolved(t, m)
			requireStandingFooter(t, m)
			if cmd != nil {
				t.Errorf("%q scheduled %T, want nothing", tc.name, cmd)
			}
		})
	}
}

func TestSlotConfirm_EscCancelsNotCloses(t *testing.T) {
	dir := t.TempDir()
	themetest.Write(t, dir, "aurora.theme", themetest.MonochromeLines("#101010"))
	m, _, persister := newRecomputePanelModel(t, dir, theme.RawKeys{Theme: "aurora"})
	requireCursorOn(t, m, "aurora")

	m = arrowToThemeRow(t, m, "nord")
	previewed := m.themeState.active
	if previewed == themePanelRowFor(t, m, "aurora").Row.Theme {
		t.Fatal("fixture: the previewed row paints the persisted theme, so a close would be invisible")
	}
	m, _ = pressSlotKey(t, m, slotDarkPress)
	requireConfirmLive(t, m, themeSlotConfirm{slug: "nord", member: theme.MemberDark})

	m, cmd := pressConfirmKey(t, m, confirmEsc)

	if !m.themePanel.open {
		t.Fatal("`Esc` closed the panel while the confirm was live; the innermost thing resolves first")
	}
	if got := m.themePanel.enumeration; len(got.Entries) == 0 || got.DirPath != dir {
		t.Errorf("the cancel discarded the enumeration (%d entries from %q), want the retained read of %q — that is the close's own effect", len(got.Entries), got.DirPath, dir)
	}
	if len(m.themePanel.union.Rows) == 0 || m.themePanel.badges == nil || len(m.themePanel.list.Items()) == 0 || m.themePanel.width == 0 {
		t.Errorf("the cancel discarded panel state %+v, want everything the close would have dropped left in place", m.themePanel)
	}
	if m.themeState.active != previewed {
		t.Errorf("the cancel rendered canvas %s, want the previewed %s — the close is what discards a preview", m.themeState.active.Canvas.Value, previewed.Canvas.Value)
	}
	if len(persister.slugs) != 0 {
		t.Errorf("the cancel wrote %v, want nothing", persister.slugs)
	}
	if cmd != nil {
		t.Errorf("the cancel scheduled %T, want nothing", cmd)
	}

	m = closeThemePanelForTest(t, m)
	if m.themePanel.open {
		t.Fatal("control: `Esc` with no confirm live did not close the panel")
	}
	if got := m.themePanel.enumeration; len(got.Entries) != 0 {
		t.Errorf("control: the close retained %d enumeration entries, want the zero value", len(got.Entries))
	}
	if m.themeState.active == previewed {
		t.Errorf("control: the close kept the previewed canvas %s, so the cancel's preview assertion says nothing", m.themeState.active.Canvas.Value)
	}
}

func TestSlotConfirm_CtrlCQuits(t *testing.T) {
	m, persister := newSlotConfirmModel(t)
	m = raiseSlotConfirmForTest(t, m, slotDarkPress, theme.MemberDark)

	quit, cmd := pressConfirmKey(t, m, confirmCtrlC)

	if !isQuitCmd(cmd) {
		t.Errorf("`Ctrl-C` scheduled %T while the confirm was live, want tea.Quit", cmd)
	}
	if len(persister.slugs) != 0 {
		t.Errorf("`Ctrl-C` wrote %v; it is not a confirm", persister.slugs)
	}
	if quit.themeState.keys != m.themeState.keys {
		t.Errorf("`Ctrl-C` left keys %+v, want the untouched %+v", quit.themeState.keys, m.themeState.keys)
	}
}

// A re-raise of the same slot key is state-identical by construction, so the
// OTHER slot key is where a re-raise would be observable.
func TestSlotConfirm_SwallowsEverythingElse(t *testing.T) {
	cursorMoved := func(before, after Model) bool {
		return after.themePanel.list.Index() != before.themePanel.list.Index()
	}
	openControl := func(effect func(before, after Model) bool) func(*testing.T, Model, tea.KeyPressMsg) bool {
		return func(t *testing.T, base Model, press tea.KeyPressMsg) bool {
			t.Helper()
			return effect(base, pressPanelKey(t, base, press))
		}
	}
	closedControl := func(effect func(before, after Model) bool) func(*testing.T, Model, tea.KeyPressMsg) bool {
		return func(t *testing.T, base Model, press tea.KeyPressMsg) bool {
			t.Helper()
			closed := closeThemePanelForTest(t, base)
			return effect(closed, pressPanelKey(t, closed, press))
		}
	}
	// The control for a modified answer letter is the UNMODIFIED letter, not the
	// same key elsewhere.
	answerControl := func(answer tea.KeyPressMsg, effect func(before, after Model) bool) func(*testing.T, Model, tea.KeyPressMsg) bool {
		return func(t *testing.T, base Model, _ tea.KeyPressMsg) bool {
			t.Helper()
			raised := raiseSlotConfirmForTest(t, base, slotLightPress, theme.MemberLight)
			answered, _ := pressConfirmKey(t, raised, answer)
			return effect(raised, answered)
		}
	}

	for _, tc := range []struct {
		name     string
		press    tea.KeyPressMsg
		control  func(*testing.T, Model, tea.KeyPressMsg) bool
		controlS string
	}{
		{name: "↑", press: arrowUp, control: openControl(cursorMoved), controlS: "move the panel cursor"},
		{name: "↓", press: arrowDown, control: openControl(cursorMoved), controlS: "move the panel cursor"},
		{name: "ctrl+↑", press: arrowPageUp, control: openControl(cursorMoved), controlS: "page the panel cursor"},
		{name: "ctrl+↓", press: arrowPageDown, control: openControl(cursorMoved), controlS: "page the panel cursor"},
		{
			name:  "enter",
			press: commitEnter,
			control: openControl(func(before, after Model) bool {
				return after.themeState.keys.Theme != before.themeState.keys.Theme
			}),
			controlS: "commit the cursor's slug as the constant",
		},
		{
			name:  "d",
			press: slotDarkPress,
			control: openControl(func(_, after Model) bool {
				return after.themePanel.pending == themeSlotConfirm{slug: slotConfirmTarget(), member: theme.MemberDark}
			}),
			controlS: "raise the dark-slot confirm",
		},
		{
			name:  "l",
			press: slotLightPress,
			control: openControl(func(_, after Model) bool {
				return after.themePanel.pending == themeSlotConfirm{slug: slotConfirmTarget(), member: theme.MemberLight}
			}),
			controlS: "raise the light-slot confirm",
		},
		{
			name:  "ctrl+y",
			press: confirmYesCtrl,
			control: answerControl(confirmYes, func(before, after Model) bool {
				return after.themeState.keys != before.themeState.keys
			}),
			controlS: "resolve the confirm as a `y` when the letter carries no ctrl",
		},
		{
			name:  "alt+n",
			press: confirmNoAlt,
			control: answerControl(confirmNo, func(before, after Model) bool {
				return !after.themePanel.confirming() && after.themeState.keys == before.themeState.keys
			}),
			controlS: "resolve the confirm as an `n` when the letter carries no alt",
		},
		{
			name:  "t",
			press: tea.KeyPressMsg{Code: 't', Text: "t"},
			control: closedControl(func(_, after Model) bool {
				return after.themePanel.open
			}),
			controlS: "open the theme panel",
		},
		{
			name:  "?",
			press: tea.KeyPressMsg{Code: '?', Text: "?"},
			control: closedControl(func(_, after Model) bool {
				return after.modal == modalHelp
			}),
			controlS: "open the help modal",
		},
		{
			name:  "k",
			press: tea.KeyPressMsg{Code: 'k', Text: "k"},
			control: closedControl(func(_, after Model) bool {
				return after.modal == modalKillConfirm
			}),
			controlS: "open the kill confirm modal",
		},
		{
			name:  "x",
			press: tea.KeyPressMsg{Code: 'x', Text: "x"},
			control: closedControl(func(_, after Model) bool {
				return after.activePage == PageProjects
			}),
			controlS: "switch to the Projects page",
		},
		{
			name:  "m",
			press: tea.KeyPressMsg{Code: 'm', Text: "m"},
			control: closedControl(func(_, after Model) bool {
				return after.MultiSelectActive()
			}),
			controlS: "enter multi-select",
		},
		{
			name:  "/",
			press: tea.KeyPressMsg{Code: '/', Text: "/"},
			control: closedControl(func(_, after Model) bool {
				return after.sessionList.SettingFilter()
			}),
			controlS: "focus the filter",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The paging height gives `Ctrl+↑`/`Ctrl+↓` somewhere to move; at the
			// standard height they are inert for an unrelated reason.
			base, _ := newSlotConfirmModelAt(t, arrowPagingTermH)
			requireArrowPanelPageSize(t, base, arrowPagingPerPage)

			if !tc.control(t, base, tc.press) {
				t.Fatalf("precondition: %v does not %s with no confirm live, so a swallow proves nothing", tc.press, tc.controlS)
			}

			live, persister := newSlotConfirmModelAt(t, arrowPagingTermH)
			live = raiseSlotConfirmForTest(t, live, slotLightPress, theme.MemberLight)
			frame := live.View().Content

			got, cmd := pressConfirmKey(t, live, tc.press)

			requireConfirmLive(t, got, themeSlotConfirm{slug: slotConfirmTarget(), member: theme.MemberLight})
			if len(persister.slugs) != 0 {
				t.Errorf("%v wrote %v while the confirm was live; only `y` writes", tc.press, persister.slugs)
			}
			if got.themeState.keys != live.themeState.keys {
				t.Errorf("%v left keys %+v, want the untouched %+v", tc.press, got.themeState.keys, live.themeState.keys)
			}
			if index, want := got.themePanel.list.Index(), live.themePanel.list.Index(); index != want {
				t.Errorf("%v moved the cursor to row %d, want it left on %d — an arrow mid-question would re-theme the screen behind the answer", tc.press, index, want)
			}
			if got.themeState.active != live.themeState.active {
				t.Errorf("%v rendered canvas %s, want the previewed %s left alone", tc.press, got.themeState.active.Canvas.Value, live.themeState.active.Canvas.Value)
			}
			if got.activePage != live.activePage || got.modal != modalNone || got.MultiSelectActive() || got.sessionList.SettingFilter() {
				t.Errorf("%v reached the page beneath the panel (page %d, modal %d, multi-select %v, filtering %v)",
					tc.press, got.activePage, got.modal, got.MultiSelectActive(), got.sessionList.SettingFilter())
			}
			if cmd != nil {
				t.Errorf("%v scheduled %T while the confirm was live, want nothing", tc.press, cmd)
			}
			if after := got.View().Content; after != frame {
				t.Errorf("%v changed the composed frame\nbefore: %q\nafter:  %q", tc.press, escSeq(frame), escSeq(after))
			}
		})
	}
}

func TestSlotConfirm_CancelIsInert(t *testing.T) {
	m, persister := newSlotConfirmModel(t)
	keys := m.themeState.keys
	index := m.themePanel.list.Index()
	previewed := m.themeState.active
	badges := maps.Clone(m.themePanel.badges)
	labels := themePanelRowLabels(m)
	frame := m.View().Content

	raised := raiseSlotConfirmForTest(t, m, slotLightPress, theme.MemberLight)
	if raised.View().Content == frame {
		t.Fatal("fixture: raising the confirm did not change the frame, so restoring it asserts nothing")
	}

	m, _ = pressConfirmKey(t, raised, confirmNo)

	if len(persister.slugs) != 0 {
		t.Errorf("the cancel wrote %v, want nothing", persister.slugs)
	}
	if m.themeState.keys != keys {
		t.Errorf("the cancel left keys %+v, want the untouched %+v", m.themeState.keys, keys)
	}
	if got := m.themePanel.list.Index(); got != index {
		t.Errorf("the cancel left the cursor on row %d, want %d", got, index)
	}
	if m.themeState.active != previewed {
		t.Errorf("the cancel rendered canvas %s, want the previewed %s", m.themeState.active.Canvas.Value, previewed.Canvas.Value)
	}
	if got := m.themePanel.badges; !maps.Equal(got, badges) {
		t.Errorf("the cancel left badges %v, want the untouched %v", got, badges)
	}
	if got := themePanelRowLabels(m); !slices.Equal(got, labels) {
		t.Errorf("the cancel left rows %v, want the untouched %v", got, labels)
	}
	if got := m.View().Content; got != frame {
		t.Errorf("the cancel did not restore the pre-raise frame\nbefore: %q\nafter:  %q", escSeq(frame), escSeq(got))
	}
}

// A real loader: a stub seam answering with a fixed union would make every row
// and every `●` a statement about the fixture.
func TestSlotConfirm_AtomicConstantClearPlusSlot(t *testing.T) {
	dir := t.TempDir()
	themetest.Write(t, dir, "aurora.theme", themetest.MonochromeLines("#101010"))
	m, _, persister := newRecomputePanelModel(t, dir, theme.RawKeys{Theme: "aurora"})
	requireBadge(t, m, "aurora", theme.BadgeConstant)
	requireBadgeText(t, m, 1, 0, 0)

	m = arrowToThemeRow(t, m, "nord")
	m, _ = pressSlotKey(t, m, slotLightPress)
	requireConfirmLive(t, m, themeSlotConfirm{slug: "nord", member: theme.MemberLight})

	m, _ = pressConfirmKey(t, m, confirmYes)

	requireSlotCommits(t, persister, slotCommit{slug: "nord", member: theme.MemberLight})
	if len(persister.slugs) != 1 {
		t.Errorf("the answer made %d seam call(s) (%v), want the single atomic slot write", len(persister.slugs), persister.slugs)
	}
	requirePairKeys(t, m, "nord", "")
	requireConfirmResolved(t, m)
	requireStandingFooter(t, m)
	requireBadge(t, m, "nord", theme.BadgeLight)
	requireBadge(t, m, "aurora", theme.BadgeNone)
	requireBadge(t, m, theme.DefaultDarkSlug, theme.BadgeDark)
	requireBadgeText(t, m, 0, 1, 1)

	if got := themePanelSeamCallers(t, "loadNewlyLiveSlot"); !slices.Equal(got, []string{"confirmSlotAssignment"}) {
		t.Errorf("the newly-live-slot seam is called from %v, want exactly [confirmSlotAssignment] — the slot load has one route in", got)
	}
}

func TestSlotConfirm_FailedCommitKeepsTheConstant(t *testing.T) {
	dir := t.TempDir()
	themetest.Write(t, dir, "aurora.theme", themetest.MonochromeLines("#101010"))
	m, _, persister := newRecomputePanelModel(t, dir, theme.RawKeys{Theme: "aurora"})
	keys := m.themeState.keys
	badges := maps.Clone(m.themePanel.badges)
	labels := themePanelRowLabels(m)

	m = arrowToThemeRow(t, m, "nord")
	previewed := m.themeState.active
	persister.err = errThemeCommitFailed
	m, _ = pressSlotKey(t, m, slotDarkPress)

	m, cmd := pressConfirmKey(t, m, confirmYes)

	requireSlotCommits(t, persister, slotCommit{slug: "nord", member: theme.MemberDark})
	if m.themeState.keys != keys {
		t.Errorf("a failed commit left keys %+v, want the untouched %+v — the constant is cleared in the WRITE, and this write did not land", m.themeState.keys, keys)
	}
	if got := m.themePanel.badges; !maps.Equal(got, badges) {
		t.Errorf("a failed commit left badges %v, want the untouched %v — a failed commit does not move the `●`", got, badges)
	}
	requireBadge(t, m, "aurora", theme.BadgeConstant)
	if got := themePanelRowLabels(m); !slices.Equal(got, labels) {
		t.Errorf("a failed commit left rows %v, want the untouched %v — only a SUCCESSFUL commit recomputes", got, labels)
	}
	requireConfirmGone(t, m)
	requireStandingFooter(t, m)
	if m.themeState.active != previewed {
		t.Errorf("a failed commit rendered canvas %s, want the previewed %s — a failed commit KEEPS the theme applied in memory", m.themeState.active.Canvas.Value, previewed.Canvas.Value)
	}
	if cmd != nil {
		t.Errorf("a failed commit scheduled %T, want nothing", cmd)
	}

	requireCommitFailedMessage(t, m)
	if !m.themeState.commitFailed {
		t.Error("a failed confirmed commit left no outstanding failure; the state runs until a commit SUCCEEDS")
	}

	if got := themePanelSeamCallers(t, "applyCommitResult"); !slices.Equal(got, []string{"commit"}) {
		t.Errorf("the commit-failure result handler is called from %v, want exactly [commit] — the failure semantics live in one place", got)
	}

	landed, _, _ := newRecomputePanelModel(t, dir, theme.RawKeys{Theme: "aurora"})
	landed = arrowToThemeRow(t, landed, "nord")
	landed, _ = pressSlotKey(t, landed, slotDarkPress)
	landed, _ = pressConfirmKey(t, landed, confirmYes)
	requirePairKeys(t, landed, "", "nord")
	requireBadge(t, landed, "aurora", theme.BadgeNone)
}

func TestSlotConfirm_NilPersisterIsInert(t *testing.T) {
	t.Run("the answer writes and moves nothing", func(t *testing.T) {
		rows := arrowValidRows(t, 4)
		persisted, target := rows[0].Slug, rows[2].Slug

		m := openCommitPanel(t, newArrowPanelDeps(t, rows, persisted), PageSessions, persisted)
		if m.themeState.persister != nil {
			t.Fatalf("fixture: the model holds persister %#v, want none", m.themeState.persister)
		}
		m = arrowToThemeRow(t, m, target)
		before := m.View().Content

		raised := raiseSlotConfirmForTestAt(t, m, slotDarkPress, themeSlotConfirm{slug: target, member: theme.MemberDark})
		answered, cmd := pressConfirmKey(t, raised, confirmYes)

		requireConstantKeys(t, answered, persisted)
		requireConfirmResolved(t, answered)
		requireStandingFooter(t, answered)
		if cmd != nil {
			t.Errorf("`y` over a nil persister scheduled %T, want nothing", cmd)
		}
		if got := answered.View().Content; got != before {
			t.Errorf("`y` over a nil persister did not restore the pre-raise frame\nbefore: %q\nafter:  %q", escSeq(before), escSeq(got))
		}
	})

	t.Run("the newly-live-slot load is behind the persister", func(t *testing.T) {
		dir := newConversionThemesDir(t)
		loader, sink := themeOpenTestLoader(t)
		keys := theme.RawKeys{Theme: conversionConstant}
		setting, _ := theme.ResolveSetting(keys)
		resolution, err := theme.NewSilentLoader().ResolveNomination(setting, dir)
		if err != nil {
			t.Fatalf("construction-time resolution of %+v: %v", setting, err)
		}
		m := Build(Deps{
			Lister:      fakeLister{},
			Theme:       resolution.Nomination,
			ThemeSource: countingThemeSourceOver(loader, dir),
			ThemeKeys:   keys,
		})
		if m.themeState.persister != nil {
			t.Fatalf("fixture: the model holds persister %#v, want none", m.themeState.persister)
		}
		m.termWidth, m.termHeight = arrowTermW, arrowTermH
		m.applySessions(closePanelSessions(), nil)
		m = openConversionPanel(t, m)
		nomination := m.themeState.nomination

		m, _ = convertToSlot(t, m, "nord", slotDarkPress)

		if got := sink.Records().WithMessage("loaded"); len(got) != 0 {
			t.Errorf("`y` over a nil persister emitted %d `theme: loaded` line(s), want none — no write landed for a load to follow\n%s", len(got), sink.Body())
		}
		if got := sink.Records().WithMessage("fallback applied"); len(got) != 0 {
			t.Errorf("`y` over a nil persister emitted %d `theme: fallback applied` line(s), want none — a load run here would collapse the un-mirrored constant's empty slots onto the shipped defaults and report a load for a write that never happened\n%s", len(got), sink.Body())
		}
		if m.themeState.nomination != nomination || !m.themeState.nomination.IsConstant() {
			t.Errorf("`y` over a nil persister left the nomination %+v, want the untouched constant", m.themeState.nomination)
		}
		requireConstantKeys(t, m, conversionConstant)
	})
}

func TestSlotConfirm_ForcedCloseCancels(t *testing.T) {
	m := newGeometryPanelModel(t, geometryWideW, geometryContentH)
	persister := &fakeThemePersister{}
	WithThemePersister(persister)(&m)
	m, _ = pressSlotKey(t, m, slotDarkPress)
	if got := m.themePanel.message; got.Kind != themeMessageConfirm {
		t.Fatalf("fixture: `d` left the message %+v, want the confirm live over a constant", got)
	}
	keys := m.themeState.keys

	contentW, contentH := geometryBelowHeightFloor()
	m = resizeForTest(t, m, contentW, contentH)

	requireForcedClose(t, m, wantShortClosedFlash)
	if got := m.themePanel.pending; got != (themeSlotConfirm{}) {
		t.Errorf("the forced close retained the pending assignment %+v, want it cancelled with the panel", got)
	}
	if got := m.themePanel.message; got.Kind != themeMessageNone {
		t.Errorf("the forced close retained the message %+v, want the slot empty", got)
	}
	if len(persister.slugs) != 0 {
		t.Errorf("the forced close wrote %v; nothing has been written at that point", persister.slugs)
	}
	if m.themeState.keys != keys {
		t.Errorf("the forced close left keys %+v, want the untouched %+v", m.themeState.keys, keys)
	}
}

func TestSlotConfirm_NotRaisedByEnter(t *testing.T) {
	t.Run("over a pair", func(t *testing.T) {
		rows := arrowValidRows(t, 4)
		m, persister := newCommitPairPanelModel(t, rows)

		m, _ = pressCommitKey(t, arrowToThemeRow(t, m, rows[2].Slug))

		requireCommitted(t, persister, rows[2].Slug)
		requireConfirmResolved(t, m)
		requireStandingFooter(t, m)
	})

	t.Run("over a constant", func(t *testing.T) {
		m, persister := newSlotConfirmModel(t)

		m, _ = pressCommitKey(t, m)

		requireCommitted(t, persister, slotConfirmTarget())
		requireConstantKeys(t, m, slotConfirmTarget())
		requireConfirmResolved(t, m)
		requireStandingFooter(t, m)
	})
}

func TestSlotConfirm_HandEditedFileNamesTheConstant(t *testing.T) {
	dir := t.TempDir()
	themetest.Write(t, dir, "aurora.theme", themetest.MonochromeLines("#101010"))
	keys := theme.RawKeys{Theme: "aurora", Light: "ghost", Dark: "aurora"}
	m, _, persister := newRecomputePanelModel(t, dir, keys)
	requireRowLabels(t, m, "aurora", "nord", theme.DefaultDarkSlug, theme.DefaultLightSlug)
	requireBadge(t, m, "aurora", theme.BadgeConstant)

	m = arrowToThemeRow(t, m, "nord")
	m, _ = pressSlotKey(t, m, slotDarkPress)

	requireConfirmLive(t, m, themeSlotConfirm{slug: "nord", member: theme.MemberDark})
	rendered := slotConfirmPanelText(m)
	for _, want := range []string{slotConfirmCopy("aurora"), slotConfirmAnswerKeys} {
		if !strings.Contains(rendered, want) {
			t.Errorf("the confirm does not read %q — it names the CONSTANT being cleared\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "ghost") {
		t.Errorf("the confirm mentions the stale light slug; the copy is not extended for it\n%s", rendered)
	}

	m, _ = pressConfirmKey(t, m, confirmYes)

	requireSlotCommits(t, persister, slotCommit{slug: "nord", member: theme.MemberDark})
	requirePairKeys(t, m, "ghost", "nord")
	requireRowLabels(t, m, "aurora", "ghost", "nord", theme.DefaultDarkSlug, theme.DefaultLightSlug)
	requireBadge(t, m, "ghost", theme.BadgeLight)
	requireBadge(t, m, "nord", theme.BadgeDark)
	requireBadge(t, m, "aurora", theme.BadgeNone)
	if got := slotConfirmPanelText(m); !strings.Contains(got, "● light") {
		t.Errorf("the panel does not render the stale slot's `● light` badge:\n%s", got)
	}
}

func TestSlotConfirm_ResizesTheListForTheSwappedLayout(t *testing.T) {
	m, _ := newSlotConfirmModelAt(t, arrowPagingTermH)
	requireArrowPanelPageSize(t, m, arrowPagingPerPage)
	requireThemePanelListSized(t, m, "on open")
	before := m.themePanel.list.Height()

	raised := raiseSlotConfirmForTest(t, m, slotDarkPress, theme.MemberDark)

	requireThemePanelListSized(t, raised, "with the confirm live")
	if got := raised.themePanel.list.Height(); got == before {
		t.Fatalf("fixture: the list body is %d rows either side of the raise, so the re-size asserts nothing", got)
	}

	cancelled, _ := pressConfirmKey(t, raised, confirmNo)

	requireThemePanelListSized(t, cancelled, "once the confirm resolved")
	if got := cancelled.themePanel.list.Height(); got != before {
		t.Errorf("the resolved confirm left the list body at %d rows, want the pre-raise %d", got, before)
	}
}

func requireThemePanelListSized(t *testing.T, m Model, when string) {
	t.Helper()

	width, rows := themePanelListSize(m.themePanel, m.contentHeight())
	if got := m.themePanel.list.Height(); got != rows {
		t.Errorf("%s the model's list is %d rows tall and the rendered frame sizes its copy to %d", when, got, rows)
	}
	if got := m.themePanel.list.Width(); got != width {
		t.Errorf("%s the model's list is %d cells wide and the rendered frame sizes its copy to %d", when, got, width)
	}
}

func themePanelSeamCallers(t *testing.T, name string) []string {
	t.Helper()

	var callers []string
	for _, file := range parsePackageFilesByName(t) {
		sourceguardtest.ForEachFuncCall(file, func(funcName string, call *ast.CallExpr) bool {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == name {
				callers = append(callers, funcName)
			}
			return true
		})
	}
	slices.Sort(callers)
	return slices.Compact(callers)
}
