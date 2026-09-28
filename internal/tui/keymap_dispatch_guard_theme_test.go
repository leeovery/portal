package tui

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/theme"
)

// A probe must assert the bound effect, never mere consumption: while the panel
// is open "swallowed" is true of every key, deleted dispatch arm or not.

func themePanelGuardRows(t *testing.T) []theme.Row {
	t.Helper()
	return append(arrowValidRows(t, 4), arrowInvalidRow(arrowSlug(4)))
}

func themePanelGuardModel(t *testing.T) (Model, *fakeThemePersister) {
	t.Helper()

	rows := themePanelGuardRows(t)
	persister := &fakeThemePersister{}
	deps := commitPairPanelDeps(t, rows)
	deps.ThemePersister = persister

	m := Build(deps)
	if m.colourless {
		t.Fatal("the guard seed must not be colourless — that blocks `t`, and every probe would assert a refusal")
	}
	m.termWidth, m.termHeight = arrowTermW, arrowPagingTermH
	m.applySessions(closePanelSessions(), nil)

	m = pressThemeKey(t, m)
	if !m.themePanel.open {
		t.Fatal("the guard seed's panel did not open; every probe would assert against a closed panel")
	}
	requireCursorOn(t, m, rows[1].Slug)
	requireArrowPanelPageSize(t, m, arrowPagingPerPage)
	return m, persister
}

// A value so the probes can also be driven against a deliberately broken stand-in.
type themePanelDispatch func(Model, tea.KeyPressMsg) (tea.Model, tea.Cmd)

var livePanelDispatch themePanelDispatch = Model.updateThemePanel

func dispatchPanelKey(t *testing.T, dispatch themePanelDispatch, m Model, press tea.KeyPressMsg) Model {
	t.Helper()

	updated, _ := dispatch(m, press)
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("the panel dispatch returned %T, want a Model", updated)
	}
	return next
}

func themePanelProbes(dispatch themePanelDispatch) map[string]dispatchProbe {
	return map[string]dispatchProbe{
		"↑↓": {press: arrowDown, honour: func(t *testing.T) bool {
			m, _ := themePanelGuardModel(t)
			before := m.themePanel.list.Index()
			return dispatchPanelKey(t, dispatch, m, arrowDown).themePanel.list.Index() != before
		}},
		"^↑/↓": {press: arrowPageDown, honour: func(t *testing.T) bool {
			m, _ := themePanelGuardModel(t)
			if len(m.themePanel.list.KeyMap.NextPage.Keys()) == 0 || len(m.themePanel.list.KeyMap.PrevPage.Keys()) == 0 {
				return false
			}
			before := m.themePanel.list.Paginator.Page
			return dispatchPanelKey(t, dispatch, m, arrowPageDown).themePanel.list.Paginator.Page != before
		}},
		"⏎": {press: commitEnter, honour: func(t *testing.T) bool {
			m, persister := themePanelGuardModel(t)
			slug := themePanelCursorRow(t, m).Slug
			after := dispatchPanelKey(t, dispatch, m, commitEnter)
			return slices.Equal(persister.constants, []string{slug}) && after.themePanel.open
		}},
		"d": {press: slotDarkPress, honour: func(t *testing.T) bool {
			return themePanelSlotHonoured(t, dispatch, slotDarkPress, theme.MemberDark)
		}},
		"l": {press: slotLightPress, honour: func(t *testing.T) bool {
			return themePanelSlotHonoured(t, dispatch, slotLightPress, theme.MemberLight)
		}},
		"esc": {press: keyEsc, honour: func(t *testing.T) bool {
			m, _ := themePanelGuardModel(t)
			after := dispatchPanelKey(t, dispatch, m, keyEsc)
			return !after.themePanel.open && !after.themePanel.confirming() && themePanelStateDropped(after)
		}},
	}
}

func themePanelSlotHonoured(t *testing.T, dispatch themePanelDispatch, press tea.KeyPressMsg, member theme.Member) bool {
	t.Helper()

	m, persister := themePanelGuardModel(t)
	slug := themePanelCursorRow(t, m).Slug
	after := dispatchPanelKey(t, dispatch, m, press)
	return slices.Equal(persister.slotCommits, []slotCommit{{slug: slug, member: member}}) &&
		after.themePanel.open && !after.themePanel.confirming()
}

func themePanelStateDropped(m Model) bool {
	return len(m.themePanel.enumeration.Entries) == 0 &&
		len(m.themePanel.union.Rows) == 0 &&
		len(m.themePanel.list.Items()) == 0 &&
		m.themePanel.width == 0
}

func themeConfirmProbes(dispatch themePanelDispatch) map[string]dispatchProbe {
	return map[string]dispatchProbe{
		"y": {press: confirmYes, honour: func(t *testing.T) bool {
			return themeConfirmYesHonoured(t, dispatch, confirmYes)
		}},
		"n": {press: confirmNo, honour: func(t *testing.T) bool {
			return themeConfirmNoHonoured(t, dispatch, confirmNo)
		}},
	}
}

func themeConfirmYesHonoured(t *testing.T, dispatch themePanelDispatch, press tea.KeyPressMsg) bool {
	t.Helper()

	m, persister := themeConfirmGuardModel(t)
	after := dispatchPanelKey(t, dispatch, m, press)
	return slices.Equal(persister.slotCommits, []slotCommit{themeConfirmPending}) &&
		!after.themePanel.confirming() && after.themePanel.open
}

func themeConfirmNoHonoured(t *testing.T, dispatch themePanelDispatch, press tea.KeyPressMsg) bool {
	t.Helper()

	m, persister := themeConfirmGuardModel(t)
	after := dispatchPanelKey(t, dispatch, m, press)
	return len(persister.slugs) == 0 && !after.themePanel.confirming() && after.themePanel.open
}

var themeConfirmPending = slotCommit{slug: slotConfirmTarget(), member: theme.MemberLight}

func themeConfirmGuardModel(t *testing.T) (Model, *fakeThemePersister) {
	t.Helper()

	m, persister := newSlotConfirmModel(t)
	return raiseSlotConfirmForTest(t, m, slotLightPress, themeConfirmPending.member), persister
}

func TestThemePanelDescriptorDispatchParity(t *testing.T) {
	assertDescriptorDispatchParity(t, "theme panel", themePanelKeymap(), themePanelProbes(livePanelDispatch))
}

func TestThemeConfirmDescriptorDispatchParity(t *testing.T) {
	assertDescriptorDispatchParity(t, "theme confirm", themePanelConfirmKeymap(), themeConfirmProbes(livePanelDispatch))
}

func themePanelArmRemoved(presses ...tea.KeyPressMsg) themePanelDispatch {
	return func(m Model, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
		if slices.Contains(presses, msg) {
			return m, nil
		}
		return m.updateThemePanel(msg)
	}
}

func TestThemePanelParity_DetectsARemovedArm(t *testing.T) {
	t.Run("a removed dispatch arm", func(t *testing.T) {
		for _, tc := range []struct {
			key     string
			presses []tea.KeyPressMsg
		}{
			{key: "↑↓", presses: []tea.KeyPressMsg{arrowUp, arrowDown}},
			{key: "^↑/↓", presses: []tea.KeyPressMsg{arrowPageUp, arrowPageDown}},
			{key: "⏎", presses: []tea.KeyPressMsg{commitEnter}},
			{key: "d", presses: []tea.KeyPressMsg{slotDarkPress}},
			{key: "l", presses: []tea.KeyPressMsg{slotLightPress}},
			{key: "esc", presses: []tea.KeyPressMsg{keyEsc}},
		} {
			t.Run(tc.key, func(t *testing.T) {
				probes := themePanelProbes(themePanelArmRemoved(tc.presses...))

				if probes[tc.key].honour(t) {
					t.Errorf("the %q probe passes with that arm deleted; it is not tied to the dispatch at all", tc.key)
				}
				requireDriftNaming(t, descriptorDispatchDrift(t, themePanelKeymap(), probes), tc.key)
			})
		}
	})

	t.Run("an unprobed descriptor entry", func(t *testing.T) {
		seventh := append(themePanelKeymap(), keymapEntry{Key: "z", Action: "zoom", Core: true})

		requireDriftNaming(t, descriptorDispatchDrift(t, seventh, themePanelProbes(livePanelDispatch)), "z")
	})

	t.Run("an unadvertised probe", func(t *testing.T) {
		probes := themePanelProbes(livePanelDispatch)
		probes["z"] = probes["esc"]

		requireDriftNaming(t, descriptorDispatchDrift(t, themePanelKeymap(), probes), "z")
	})
}

func requireDriftNaming(t *testing.T, drift []string, key string) {
	t.Helper()

	if len(drift) != 1 {
		t.Fatalf("the parity check reported %d violations %v, want exactly the one naming %q", len(drift), drift, key)
	}
	if !strings.Contains(drift[0], strconv.Quote(key)) {
		t.Errorf("the parity check reported %q, want a violation naming %q", drift[0], key)
	}
}

func TestThemePanelParity_ProbesAssertEffects(t *testing.T) {
	swallow := themePanelDispatch(func(m Model, _ tea.KeyPressMsg) (tea.Model, tea.Cmd) { return m, nil })

	for _, scope := range []struct {
		name   string
		probes map[string]dispatchProbe
	}{
		{name: "theme panel", probes: themePanelProbes(swallow)},
		{name: "theme confirm", probes: themeConfirmProbes(swallow)},
	} {
		for _, key := range slices.Sorted(maps.Keys(scope.probes)) {
			t.Run(scope.name+" "+key, func(t *testing.T) {
				if scope.probes[key].honour(t) {
					t.Errorf("the %s probe for %q passes against a dispatch that swallows every key and changes nothing; it asserts CONSUMPTION rather than the bound effect, so a deleted dispatch arm would still satisfy it", scope.name, key)
				}
			})
		}
	}
}

func TestThemePanelDispatch_EscMeansInnermostFirst(t *testing.T) {
	t.Run("with no confirm live it closes the panel", func(t *testing.T) {
		m, persister := themePanelGuardModel(t)
		if m.themePanel.confirming() {
			t.Fatal("fixture: the panel seed is already asking a question, so `Esc` would take the cancel")
		}

		after := dispatchPanelKey(t, livePanelDispatch, m, keyEsc)

		if after.themePanel.open {
			t.Error("`Esc` with no confirm live left the panel open; it is the ONLY way out")
		}
		if !themePanelStateDropped(after) {
			t.Errorf("the close retained panel state %+v, want the whole struct dropped so the next open re-reads", after.themePanel)
		}
		if len(persister.slugs) != 0 {
			t.Errorf("the close wrote %v; every write is an explicit commit key", persister.slugs)
		}
	})

	t.Run("with the confirm live it cancels and leaves the panel open", func(t *testing.T) {
		m, persister := themeConfirmGuardModel(t)

		after := dispatchPanelKey(t, livePanelDispatch, m, keyEsc)

		if !after.themePanel.open {
			t.Fatal("`Esc` closed the panel while the confirm was live; the innermost thing resolves first")
		}
		if after.themePanel.confirming() {
			t.Error("`Esc` left the confirm standing; it is one of the three inputs that resolve it")
		}
		if themePanelStateDropped(after) {
			t.Errorf("the cancel dropped the panel state %+v, want everything the CLOSE would have discarded left in place", after.themePanel)
		}
		if len(persister.slugs) != 0 {
			t.Errorf("the cancel wrote %v; nothing is written until `y`", persister.slugs)
		}
	})
}

func TestThemeConfirmDispatch_UppercaseReachesTheSameArm(t *testing.T) {
	for _, tc := range []struct {
		name     string
		press    tea.KeyPressMsg
		honoured func(*testing.T, themePanelDispatch, tea.KeyPressMsg) bool
	}{
		{name: "shift+y", press: confirmYesShift, honoured: themeConfirmYesHonoured},
		{name: "Y", press: confirmYesUpper, honoured: themeConfirmYesHonoured},
		{name: "shift+n", press: confirmNoShift, honoured: themeConfirmNoHonoured},
		{name: "N", press: confirmNoUpper, honoured: themeConfirmNoHonoured},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.honoured(t, livePanelDispatch, tc.press) {
				t.Errorf("%q does not reach the arm its lowercase form does; the descriptor carries ONE glyph for both cases", tc.name)
			}
		})
	}
}

func TestThemePanelKeymap_NoRightAlignedEntry(t *testing.T) {
	for _, scope := range themeKeymapScopes() {
		for _, e := range scope.entries {
			if e.RightAligned {
				t.Errorf("the %s scope's %q entry is RightAligned; the vertical footer has no right anchor, and the flag would exempt the key from the dispatch guard", scope.name, e.Key)
			}
		}
	}

	for _, page := range []struct {
		name    string
		entries []keymapEntry
	}{
		{name: "sessions", entries: sessionsKeymap()},
		{name: "projects", entries: projectsKeymap()},
	} {
		if got := countRightAligned(page.entries); got != 1 {
			t.Errorf("control: the %s descriptor carries %d RightAligned entries, want the single `?` help hint", page.name, got)
		}
	}
}

func countRightAligned(entries []keymapEntry) int {
	n := 0
	for _, e := range entries {
		if e.RightAligned {
			n++
		}
	}
	return n
}

func TestThemePanelKeymap_NoHelpEntry(t *testing.T) {
	t.Run("neither scope advertises or probes ?", func(t *testing.T) {
		for _, scope := range themeKeymapScopes() {
			for _, e := range scope.entries {
				if e.Key == "?" || e.HelpKey == "?" {
					t.Errorf("the %s scope advertises a `?` entry; `?` does nothing inside the panel", scope.name)
				}
			}
			if _, probed := scope.probes["?"]; probed {
				t.Errorf("the %s scope carries a `?` probe; the key is swallowed, so there is no bound effect to assert", scope.name)
			}
		}
	})

	t.Run("the live dispatch swallows ?", func(t *testing.T) {
		m, persister := themePanelGuardModel(t)
		before := m.themePanel.list.Index()

		after := dispatchPanelKey(t, livePanelDispatch, m, tea.KeyPressMsg{Code: '?', Text: "?"})

		if !after.themePanel.open || after.modal != modalNone {
			t.Errorf("`?` inside the panel left open=%t modal=%v, want the panel standing with no modal", after.themePanel.open, after.modal)
		}
		if got := after.themePanel.list.Index(); got != before {
			t.Errorf("`?` moved the panel cursor to row %d, want it left on %d", got, before)
		}
		if len(persister.slugs) != 0 {
			t.Errorf("`?` wrote %v, want nothing", persister.slugs)
		}
	})

	t.Run("neither scope advertises the global quit", func(t *testing.T) {
		m, _ := themePanelGuardModel(t)
		if _, cmd := livePanelDispatch(m, confirmCtrlC); !isQuitCmd(cmd) {
			t.Fatal("control: `Ctrl-C` no longer quits inside the panel, so the descriptors' silence about it says nothing")
		}

		for _, scope := range themeKeymapScopes() {
			for _, e := range scope.entries {
				if slices.Contains(ctrlCGlyphs, e.Key) {
					t.Errorf("the %s scope advertises %q; `Ctrl-C` is the global quit that stays live everywhere, not a scope binding", scope.name, e.Key)
				}
			}
		}
	})
}

var ctrlCGlyphs = []string{"^c", "^C", "ctrl+c", "ctrl-c", "Ctrl-C"}

func themeKeymapScopes() []struct {
	name    string
	entries []keymapEntry
	probes  map[string]dispatchProbe
} {
	return []struct {
		name    string
		entries []keymapEntry
		probes  map[string]dispatchProbe
	}{
		{name: "theme panel", entries: themePanelKeymap(), probes: themePanelProbes(livePanelDispatch)},
		{name: "theme confirm", entries: themePanelConfirmKeymap(), probes: themeConfirmProbes(livePanelDispatch)},
	}
}

func TestKeymapGuard_PageDescriptorsUnchanged(t *testing.T) {
	for _, tc := range []struct {
		page string
		got  []keymapEntry
		want []string
	}{
		{page: "sessions", got: sessionsKeymap(), want: []string{
			"↑↓ core=false right=false destructive=false",
			"^↑/↓ core=false right=false destructive=false",
			"⏎ core=true right=false destructive=false",
			"/ core=true right=false destructive=false",
			"␣ core=true right=false destructive=false",
			"s core=true right=false destructive=false",
			"n core=false right=false destructive=false",
			"r core=false right=false destructive=false",
			"k core=false right=false destructive=true",
			"q core=false right=false destructive=false",
			"x core=true right=false destructive=false",
			"t core=true right=false destructive=false",
			"m core=true right=false destructive=false",
			"? core=true right=true destructive=false",
		}},
		{page: "projects", got: projectsKeymap(), want: []string{
			"↑↓ core=false right=false destructive=false",
			"^↑/↓ core=false right=false destructive=false",
			"⏎ core=true right=false destructive=false",
			"x core=true right=false destructive=false",
			"e core=true right=false destructive=false",
			"/ core=true right=false destructive=false",
			"t core=true right=false destructive=false",
			"d core=false right=false destructive=true",
			"n core=false right=false destructive=false",
			"q core=false right=false destructive=false",
			"esc core=false right=false destructive=false",
			"? core=true right=true destructive=false",
		}},
		{page: "preview", got: previewKeymap(), want: []string{
			"↑/↓ core=false right=false destructive=false",
			"^↑/↓ core=false right=false destructive=false",
			"Home/End core=false right=false destructive=false",
			"←→ core=true right=false destructive=false",
			"⇥ core=true right=false destructive=false",
			"⏎ core=true right=false destructive=false",
			"␣ core=true right=false destructive=false",
		}},
	} {
		t.Run(tc.page, func(t *testing.T) {
			if got := descriptorShape(tc.got); !slices.Equal(got, tc.want) {
				t.Errorf("the %s descriptor's keys/flags changed:\ngot  %v\nwant %v", tc.page, got, tc.want)
			}
		})
	}
}

func descriptorShape(entries []keymapEntry) []string {
	shapes := make([]string, 0, len(entries))
	for _, e := range entries {
		shapes = append(shapes, fmt.Sprintf("%s core=%t right=%t destructive=%t", e.Key, e.Core, e.RightAligned, e.Destructive))
	}
	return shapes
}
