package tui

import (
	"errors"
	"go/ast"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/themetest"
)

var (
	slotDarkPress  = tea.KeyPressMsg{Code: 'd', Text: "d"}
	slotLightPress = tea.KeyPressMsg{Code: 'l', Text: "l"}
)

func pressSlotKey(t *testing.T, m Model, press tea.KeyPressMsg) (Model, tea.Cmd) {
	t.Helper()
	updated, cmd := m.Update(press)
	return updated.(Model), cmd
}

func requireSlotCommits(t *testing.T, p *fakeThemePersister, want ...slotCommit) {
	t.Helper()

	if len(p.slotCommits) != len(want) {
		t.Fatalf("the persister recorded slot commits %v, want %v", p.slotCommits, want)
	}
	for i, commit := range want {
		if p.slotCommits[i] != commit {
			t.Fatalf("slot commit %d recorded %v, want %v", i+1, p.slotCommits[i], commit)
		}
	}
	if len(p.constants) != 0 {
		t.Errorf("a slot key committed the constant(s) %v; `d`/`l` write a SLOT and clear the constant in the SAME write", p.constants)
	}
}

// The dark-slot committer the `d` key hands the selected-row wrapper.
func commitSelectedDarkSlot(m *Model) func(slug string) error {
	return func(slug string) error { return m.commitSlot(slug, theme.MemberDark) }
}

func requireNoSlotLoad(t *testing.T, m Model) {
	t.Helper()

	seam, ok := m.themeState.source.(*fakeThemeSource)
	if !ok {
		t.Fatalf("fixture: the seam is %T, want the recording fake — this is a statement about what the seam was ASKED", m.themeState.source)
	}
	if len(seam.slotLoads) != 0 {
		t.Errorf("a non-converting commit asked the seam for slot load(s) %v, want none — the load sits on the constant → adaptive transition and nowhere else", seam.slotLoads)
	}
}

func requirePairKeys(t *testing.T, m Model, light, dark string) {
	t.Helper()
	if got := m.themeState.keys; got != (theme.RawKeys{Light: light, Dark: dark}) {
		t.Errorf("themeKeys = %+v, want {Light:%s Dark:%s} — a slot commit clears the constant and leaves the OTHER slot alone", got, light, dark)
	}
}

// The dark slot is in force: the standing no-answer fallback.
func newSlotPairPanelModel(t *testing.T, dir, lightSlug, darkSlug string) (Model, *fakeThemePersister) {
	t.Helper()

	m, _, persister := newRecomputePanelModel(t, dir, theme.RawKeys{Light: lightSlug, Dark: darkSlug})
	requireCursorOn(t, m, darkSlug)
	return m, persister
}

func newSlotSplitPanelModel(t *testing.T, opened, reassembled []theme.Row) (Model, *fakeThemeSource, *fakeThemePersister) {
	t.Helper()

	light, dark := opened[0], opened[1]
	reassembly := themeRowsUnion(reassembled)
	enumerator := &fakeThemeSource{
		enumeration: theme.Enumeration{DirPath: fixtureThemesDir},
		union:       themeRowsUnion(opened),
		reassembled: &reassembly,
		resolution:  pairResolution(light, dark),
	}
	persister := &fakeThemePersister{}
	deps := stubPanelDeps(enumerator, theme.ConstantNomination(dark.Theme), theme.RawKeys{Light: light.Slug, Dark: dark.Slug})
	deps.ThemePersister = persister
	return openCommitPanel(t, deps, PageSessions, dark.Slug), enumerator, persister
}

func requireBothBadge(t *testing.T, m Model, label string) {
	t.Helper()

	requireBadge(t, m, label, theme.BadgeBoth)
	rendered := ansi.Strip(renderRecomputePanel(m))
	if got := strings.Count(rendered, "● both"); got != 1 {
		t.Errorf("the panel renders %d `● both` badges, want exactly 1 — both slots naming one slug is ONE row\n%s", got, rendered)
	}
	if strings.Contains(rendered, "● light") || strings.Contains(rendered, "● dark") {
		t.Errorf("the panel still renders a slot badge beside `● both`\n%s", rendered)
	}
}

func TestPanelSlotCommit_DarkWritesTheDarkSlot(t *testing.T) {
	rows := arrowValidRows(t, 4)
	light, target := rows[0].Slug, rows[2].Slug
	m, persister := newCommitPairPanelModel(t, rows)

	m = arrowToThemeRow(t, m, target)
	m, cmd := pressSlotKey(t, m, slotDarkPress)

	requireSlotCommits(t, persister, slotCommit{slug: target, member: theme.MemberDark})
	requirePairKeys(t, m, light, target)
	requireNoSlotLoad(t, m)
	if !m.themePanel.open {
		t.Error("`d` closed the panel; `Esc` is the ONLY way out")
	}
	if cmd != nil {
		t.Errorf("`d` scheduled %T; the write lands on this keypress", cmd)
	}
}

// Asserted separately from `d`: a transposed slot argument is invisible from
// either key alone.
func TestPanelSlotCommit_LightWritesTheLightSlot(t *testing.T) {
	rows := arrowValidRows(t, 4)
	dark, target := rows[1].Slug, rows[2].Slug
	m, persister := newCommitPairPanelModel(t, rows)

	m = arrowToThemeRow(t, m, target)
	m, cmd := pressSlotKey(t, m, slotLightPress)

	requireSlotCommits(t, persister, slotCommit{slug: target, member: theme.MemberLight})
	requirePairKeys(t, m, target, dark)
	requireNoSlotLoad(t, m)
	if !m.themePanel.open {
		t.Error("`l` closed the panel; `Esc` is the ONLY way out")
	}
	if cmd != nil {
		t.Errorf("`l` scheduled %T; the write lands on this keypress", cmd)
	}
}

// The other slot names a slug that resolves to nothing on purpose: a commit
// that re-derived the pair, or dropped the unresolvable slug, shows up here.
func TestPanelSlotCommit_OtherSlotSurvives(t *testing.T) {
	dir := t.TempDir()
	themetest.Write(t, dir, "aurora.theme", themetest.MonochromeLines("#101010"))
	m, persister := newSlotPairPanelModel(t, dir, "ghost", "aurora")
	requireRowLabels(t, m, "aurora", "ghost", "gitlab-dark-grey", "horizon", "nord", "nord-wave", theme.DefaultDarkSlug, theme.DefaultLightSlug)
	requireBadge(t, m, "ghost", theme.BadgeLight)

	m = arrowToThemeRow(t, m, "nord")
	m, _ = pressSlotKey(t, m, slotDarkPress)

	requireSlotCommits(t, persister, slotCommit{slug: "nord", member: theme.MemberDark})
	requirePairKeys(t, m, "ghost", "nord")
	requireRowLabels(t, m, "aurora", "ghost", "gitlab-dark-grey", "horizon", "nord", "nord-wave", theme.DefaultDarkSlug, theme.DefaultLightSlug)
	requireBadge(t, m, "ghost", theme.BadgeLight)
	requireBadge(t, m, "nord", theme.BadgeDark)
	requireBadge(t, m, "aurora", theme.BadgeNone)
}

func TestPanelSlotCommit_EmptyOtherSlotStaysEmpty(t *testing.T) {
	dir := t.TempDir()
	// Sorted last so `↓` reaches it, and neither shipped default so the committed
	// slug is distinguishable from both.
	themetest.Write(t, dir, "zephyr.theme", themetest.MonochromeLines("#101010"))
	m, _, persister := newRecomputePanelModel(t, dir, theme.RawKeys{})
	requireRowLabels(t, m, "gitlab-dark-grey", "horizon", "nord", "nord-wave", theme.DefaultDarkSlug, theme.DefaultLightSlug, "zephyr")
	requireCursorOn(t, m, theme.DefaultDarkSlug)
	requireBadge(t, m, theme.DefaultLightSlug, theme.BadgeLight)

	m = arrowToThemeRow(t, m, "zephyr")
	m, _ = pressSlotKey(t, m, slotDarkPress)

	requireSlotCommits(t, persister, slotCommit{slug: "zephyr", member: theme.MemberDark})
	requirePairKeys(t, m, "", "zephyr")
	requireBadge(t, m, "zephyr", theme.BadgeDark)
	requireBadge(t, m, theme.DefaultLightSlug, theme.BadgeLight)
	requireBadge(t, m, theme.DefaultDarkSlug, theme.BadgeNone)
}

func TestPanelSlotCommit_DThenLYieldsBoth(t *testing.T) {
	dir := t.TempDir()
	themetest.Write(t, dir, "aurora.theme", themetest.MonochromeLines("#101010"))
	themetest.Write(t, dir, "sunset.theme", themetest.MonochromeLines("#202020"))
	m, persister := newSlotPairPanelModel(t, dir, "sunset", "aurora")
	requireBadge(t, m, "sunset", theme.BadgeLight)
	requireBadge(t, m, "aurora", theme.BadgeDark)

	m = arrowToThemeRow(t, m, "nord")
	m, _ = pressSlotKey(t, m, slotDarkPress)
	requireBadge(t, m, "nord", theme.BadgeDark)
	requireBadge(t, m, "sunset", theme.BadgeLight)

	m, _ = pressSlotKey(t, m, slotLightPress)

	requireSlotCommits(t, persister,
		slotCommit{slug: "nord", member: theme.MemberDark},
		slotCommit{slug: "nord", member: theme.MemberLight},
	)
	requirePairKeys(t, m, "nord", "nord")
	requireBothBadge(t, m, "nord")
	requireBadge(t, m, "aurora", theme.BadgeNone)
	requireBadge(t, m, "sunset", theme.BadgeNone)
	if !m.themePanel.open {
		t.Error("the pair flow closed the panel; `Esc` is the ONLY way out")
	}
}

// The memory half is driven through commitSlot directly, so the clear is the
// write's own and not the key's.
func TestPanelSlotCommit_ClearsTheConstantAtomically(t *testing.T) {
	t.Run("one call, not two", func(t *testing.T) {
		rows := arrowValidRows(t, 4)
		m, persister := newCommitPairPanelModel(t, rows)

		m, _ = pressSlotKey(t, m, slotDarkPress)

		requireSlotCommits(t, persister, slotCommit{slug: rows[1].Slug, member: theme.MemberDark})
		if len(persister.slugs) != 1 {
			t.Errorf("the keypress made %d seam call(s) (%v), want the single atomic slot write", len(persister.slugs), persister.slugs)
		}
		if m.themeState.keys.Theme != "" {
			t.Errorf("themeKeys.Theme = %q after a slot commit, want it cleared", m.themeState.keys.Theme)
		}
	})

	t.Run("the constant is cleared in memory", func(t *testing.T) {
		dir := t.TempDir()
		themetest.Write(t, dir, "aurora.theme", themetest.MonochromeLines("#101010"))
		// All three keys resolve as a constant: the shape in which there is a
		// constant to clear and the untouched slot is invisible until it is.
		keys := theme.RawKeys{Theme: "aurora", Light: "ghost", Dark: "aurora"}
		m, _, persister := newRecomputePanelModel(t, dir, keys)
		requireRowLabels(t, m, "aurora", "gitlab-dark-grey", "horizon", "nord", "nord-wave", theme.DefaultDarkSlug, theme.DefaultLightSlug)

		if err := (&m).commitSlot("nord", theme.MemberDark); err != nil {
			t.Fatalf("commitSlot(nord, dark): %v", err)
		}

		requireSlotCommits(t, persister, slotCommit{slug: "nord", member: theme.MemberDark})
		if len(persister.slugs) != 1 {
			t.Errorf("the commit made %d seam call(s) (%v), want one — the clear rides the SAME write", len(persister.slugs), persister.slugs)
		}
		requirePairKeys(t, m, "ghost", "nord")
		requireRowLabels(t, m, "aurora", "ghost", "gitlab-dark-grey", "horizon", "nord", "nord-wave", theme.DefaultDarkSlug, theme.DefaultLightSlug)
		requireBadge(t, m, "ghost", theme.BadgeLight)
		requireBadge(t, m, "nord", theme.BadgeDark)
	})
}

// Assigning a slot is not destructive, so over a constant it asks nothing: the
// keypress clears the constant and assigns the slot in the one write.
func TestPanelSlotCommit_OverAConstantWritesAtOnce(t *testing.T) {
	for _, tc := range []struct {
		name   string
		press  tea.KeyPressMsg
		member theme.Member
	}{
		{name: "d", press: slotDarkPress, member: theme.MemberDark},
		{name: "l", press: slotLightPress, member: theme.MemberLight},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows := arrowValidRows(t, 4)
			persisted, target := rows[0].Slug, rows[2].Slug
			m, persister := newCommitPanelModel(t, rows, persisted)
			m = arrowToThemeRow(t, m, target)
			previewed := m.themeState.active

			m, cmd := pressSlotKey(t, m, tc.press)

			requireSlotCommits(t, persister, slotCommit{slug: target, member: tc.member})
			if len(persister.slugs) != 1 {
				t.Errorf("%q made %d seam call(s) (%v), want the single slot write that clears the constant with it", tc.name, len(persister.slugs), persister.slugs)
			}
			if tc.member == theme.MemberLight {
				requirePairKeys(t, m, target, "")
			} else {
				requirePairKeys(t, m, "", target)
			}
			if !m.themePanel.open {
				t.Errorf("%q closed the panel over a constant", tc.name)
			}
			if got := m.themePanel.message; got.Kind != themeMessageNone {
				t.Errorf("%q left the message %+v, want the slot empty — nothing is asked", tc.name, got)
			}
			if m.themeState.active != previewed {
				t.Errorf("%q rendered canvas %s, want the previewed %s left alone", tc.name, m.themeState.active.Canvas.Value, previewed.Canvas.Value)
			}
			if cmd != nil {
				t.Errorf("%q over a constant scheduled %T, want nothing", tc.name, cmd)
			}
		})
	}
}

func TestPanelSlotCommit_NonActiveSlotIsVisuallyInert(t *testing.T) {
	newModel := func(t *testing.T) (Model, *fakeThemePersister) {
		t.Helper()
		dir := t.TempDir()
		themetest.Write(t, dir, "aurora.theme", themetest.MonochromeLines("#101010"))
		themetest.Write(t, dir, "sunset.theme", themetest.MonochromeLines("#202020"))
		return newSlotPairPanelModel(t, dir, theme.DefaultLightSlug, "aurora")
	}

	t.Run("the frame is byte-identical across the keypress", func(t *testing.T) {
		m, persister := newModel(t)

		m = arrowToThemeRow(t, m, theme.DefaultLightSlug)
		previewed := m.themeState.active
		before := m.View().Content

		m, cmd := pressSlotKey(t, m, slotLightPress)

		requireSlotCommits(t, persister, slotCommit{slug: theme.DefaultLightSlug, member: theme.MemberLight})
		if m.themeState.active != previewed {
			t.Errorf("the commit rendered canvas %s, want the previewed %s left alone", m.themeState.active.Canvas.Value, previewed.Canvas.Value)
		}
		if got := m.View().Content; got != before {
			t.Errorf("the commit changed the composed frame\nbefore: %q\nafter:  %q", escSeq(before), escSeq(got))
		}
		if cmd != nil {
			t.Errorf("the commit scheduled %T, want nothing", cmd)
		}
	})

	// The panel legitimately re-renders here, so the palette is compared as a
	// set of SGR sequences rather than byte for byte.
	t.Run("a badge-moving commit leaves the palette alone", func(t *testing.T) {
		m, persister := newModel(t)

		m = arrowToThemeRow(t, m, "sunset")
		previewed := m.themeState.active
		if got := previewed.Canvas.Value; got != "#202020" {
			t.Fatalf("fixture: arrowing to `sunset` rendered canvas %s, want #202020", got)
		}
		colours := frameColours(m.View().Content)

		m, _ = pressSlotKey(t, m, slotLightPress)

		requireSlotCommits(t, persister, slotCommit{slug: "sunset", member: theme.MemberLight})
		requireBadge(t, m, "sunset", theme.BadgeLight)
		requireBadge(t, m, theme.DefaultLightSlug, theme.BadgeNone)
		if m.themeState.active != previewed {
			t.Errorf("the commit rendered canvas %s, want the previewed %s — the resolved-active theme is still the DARK slot", m.themeState.active.Canvas.Value, previewed.Canvas.Value)
		}
		if got := frameColours(m.View().Content); !slices.Equal(got, colours) {
			t.Errorf("the commit changed the frame's colours\nbefore: %v\nafter:  %v", colours, got)
		}
	})

	t.Run("Esc still resolves the dark slot", func(t *testing.T) {
		m, persister := newModel(t)

		m, _ = pressSlotKey(t, arrowToThemeRow(t, m, "sunset"), slotLightPress)
		requireSlotCommits(t, persister, slotCommit{slug: "sunset", member: theme.MemberLight})

		m = closeThemePanelForTest(t, m)

		if m.themePanel.open {
			t.Fatal("Esc left the panel open")
		}
		if got := m.themeState.active.Canvas.Value; got != "#101010" {
			t.Errorf("the close rendered canvas %s, want the DARK slot's aurora #101010 — a light-slot commit does not change what is in force in a dark terminal", got)
		}
	})
}

// Driven on the row already carrying that slot's badge, where a "don't write
// what is already set" shortcut would be tempting.
func TestPanelSlotCommit_RepeatIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	themetest.Write(t, dir, "aurora.theme", themetest.MonochromeLines("#101010"))
	m, persister := newSlotPairPanelModel(t, dir, theme.DefaultLightSlug, "aurora")
	requireBadge(t, m, "aurora", theme.BadgeDark)

	m, _ = pressSlotKey(t, m, slotDarkPress)
	once := m.View().Content
	onceKeys := m.themeState.keys
	onceBadges := maps.Clone(m.themePanel.badges)

	m, cmd := pressSlotKey(t, m, slotDarkPress)

	repeat := slotCommit{slug: "aurora", member: theme.MemberDark}
	requireSlotCommits(t, persister, repeat, repeat)
	requirePairKeys(t, m, theme.DefaultLightSlug, "aurora")
	if m.themeState.keys != onceKeys {
		t.Errorf("the second commit left keys %+v, want the first's %+v", m.themeState.keys, onceKeys)
	}
	if got := m.themePanel.badges; !maps.Equal(got, onceBadges) {
		t.Errorf("the second commit left badges %v, want the first's %v", got, onceBadges)
	}
	if got := m.themePanel.message; got.Kind != themeMessageNone {
		t.Errorf("the repeat commit raised the message %+v; there is no retry affordance and no state to clear first", got)
	}
	if cmd != nil {
		t.Errorf("the repeat commit scheduled %T, want nothing", cmd)
	}
	if got := m.View().Content; got != once {
		t.Errorf("the repeat commit changed the frame\nonce:  %q\ntwice: %q", escSeq(once), escSeq(got))
	}

	// The keypress discards the error return (the report is raised inside the
	// commit), so the helper is called directly to observe it.
	if err := (&m).commitSelected(commitSelectedDarkSlot(&m)); err != nil {
		t.Errorf("a repeated commit returned %v, want nil — a commit is always re-attemptable", err)
	}
}

func TestPanelSlotCommit_TypedSlotOnly(t *testing.T) {
	members, conversions, prefsSlots := themeSlotUsagesInPackage(t)

	if len(conversions) != 0 {
		t.Errorf("%v convert a value to theme.Member; the half is the typed value threaded through the seam, never one minted here", conversions)
	}
	if len(prefsSlots) != 0 {
		t.Errorf("%v name a prefs slot; the persistence type is the persister's, and this layer carries the domain's light/dark type end to end", prefsSlots)
	}
	if want := []string{"MemberDark", "MemberLight"}; !slices.Equal(members, want) {
		t.Errorf("the package names theme members %v, want exactly %v — a third half must not be nameable", members, want)
	}

	method, ok := reflect.TypeFor[ThemePersister]().MethodByName("CommitThemeSlot")
	if !ok {
		t.Fatal("ThemePersister declares no CommitThemeSlot")
	}
	if got, want := method.Type.In(1), reflect.TypeFor[theme.Member](); got != want {
		t.Errorf("CommitThemeSlot takes a %s, want the typed %s", got, want)
	}
}

func themeSlotUsagesInPackage(t *testing.T) (members, conversions, prefsSlots []string) {
	t.Helper()

	for name, file := range parsePackageFilesByName(t) {
		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.CallExpr:
				if isPackageSelector(node.Fun, "theme", "Member") {
					conversions = append(conversions, name)
				}
			case *ast.SelectorExpr:
				if isPackageSelector(node, "theme", "") && isThemeMemberValue(node.Sel.Name) {
					members = append(members, node.Sel.Name)
				}
				if (isPackageSelector(node, "prefs", "") && strings.HasPrefix(node.Sel.Name, "Slot")) ||
					isPackageSelector(node, "prefs", "ThemeSlot") {
					prefsSlots = append(prefsSlots, name)
				}
			}
			return true
		})
	}
	for _, found := range []*[]string{&members, &conversions, &prefsSlots} {
		slices.Sort(*found)
		*found = slices.Compact(*found)
	}
	return members, conversions, prefsSlots
}

// Member-prefixed VALUES only: a signature names the type without naming a half.
func isThemeMemberValue(name string) bool {
	return strings.HasPrefix(name, "Member") && name != "Member"
}

func isPackageSelector(expr ast.Expr, pkg, sel string) bool {
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	ident, ok := selector.X.(*ast.Ident)
	return ok && ident.Name == pkg && (sel == "" || selector.Sel.Name == sel)
}

// Both subtests commit a slug the target slot does not already hold: committing
// one it holds leaves the keys byte-identical either way and asserts nothing.
func TestPanelSlotCommit_FailedWriteLeavesKeysAlone(t *testing.T) {
	opened := arrowValidRows(t, 4)
	reassembled := arrowValidRows(t, 2)

	t.Run("the keypress mutates nothing", func(t *testing.T) {
		m, enumerator, persister := newSlotSplitPanelModel(t, opened, reassembled)
		m = arrowToThemeRow(t, m, opened[2].Slug)
		keys := m.themeState.keys
		badges := maps.Clone(m.themePanel.badges)
		previewed := m.themeState.active
		persister.err = errThemeCommitFailed

		m, cmd := pressSlotKey(t, m, slotDarkPress)

		requireSlotCommits(t, persister, slotCommit{slug: opened[2].Slug, member: theme.MemberDark})
		if m.themeState.keys != keys {
			t.Errorf("a failed slot commit left keys %+v, want the untouched %+v", m.themeState.keys, keys)
		}
		requireRowLabels(t, m, arrowSlug(0), arrowSlug(1), arrowSlug(2), arrowSlug(3))
		if got := m.themePanel.badges; !maps.Equal(got, badges) {
			t.Errorf("the failed commit left badges %v, want the untouched %v — a failed commit does not move the `●`", got, badges)
		}
		if enumerator.reassembles != 0 {
			t.Errorf("the failed commit ran %d reassemblies, want 0 — only a SUCCESSFUL commit recomputes", enumerator.reassembles)
		}
		if !m.themePanel.open {
			t.Error("a failed slot commit closed the panel; `Esc` is the only way out")
		}
		if m.themeState.active != previewed {
			t.Errorf("a failed commit rendered canvas %s, want the previewed %s — a failed commit KEEPS the theme applied in memory", m.themeState.active.Canvas.Value, previewed.Canvas.Value)
		}
		if got := m.themePanel.message; got.Kind != themeMessageCommitFailed {
			t.Errorf("a failed slot commit left the message %+v, want the failed-commit line reported in the slot", got)
		}
		if cmd != nil {
			t.Errorf("a failed slot commit scheduled %T, want nothing", cmd)
		}
	})

	t.Run("the helper returns the error and leaves the constant set", func(t *testing.T) {
		dir := t.TempDir()
		themetest.Write(t, dir, "aurora.theme", themetest.MonochromeLines("#101010"))
		keys := theme.RawKeys{Theme: "aurora", Light: "ghost", Dark: "aurora"}
		m, _, persister := newRecomputePanelModel(t, dir, keys)
		requireBadge(t, m, "aurora", theme.BadgeConstant)
		badges := maps.Clone(m.themePanel.badges)
		m = arrowToThemeRow(t, m, "nord")
		persister.err = errThemeCommitFailed

		if err := (&m).commitSelected(commitSelectedDarkSlot(&m)); !errors.Is(err, errThemeCommitFailed) {
			t.Errorf("the selected-row slot commit returned %v, want the persister's error — the caller reads the outcome from it", err)
		}

		requireSlotCommits(t, persister, slotCommit{slug: "nord", member: theme.MemberDark})
		if m.themeState.keys != keys {
			t.Errorf("a failed slot commit left keys %+v, want the untouched %+v — the constant is cleared in the WRITE, and this write did not land", m.themeState.keys, keys)
		}
		if got := m.themePanel.badges; !maps.Equal(got, badges) {
			t.Errorf("the failed commit left badges %v, want the untouched %v — the constant is still set, so the panel still marks it", got, badges)
		}
		requireBadge(t, m, "aurora", theme.BadgeConstant)
	})

	t.Run("the same fixture recomputes when the write lands", func(t *testing.T) {
		m, enumerator, _ := newSlotSplitPanelModel(t, opened, reassembled)

		m, _ = pressSlotKey(t, m, slotDarkPress)

		requireRowLabels(t, m, arrowSlug(0), arrowSlug(1))
		if enumerator.reassembles != 1 {
			t.Fatalf("a successful slot commit ran %d reassemblies, want 1", enumerator.reassembles)
		}
	})
}

func TestPanelSlotCommit_NilPersisterIsInert(t *testing.T) {
	rows := arrowValidRows(t, 4)
	target := rows[2].Slug

	m := openCommitPanel(t, commitPairPanelDeps(t, rows), PageSessions, rows[1].Slug)
	if m.themeState.persister != nil {
		t.Fatalf("fixture: the model holds persister %#v, want none", m.themeState.persister)
	}
	m = arrowToThemeRow(t, m, target)
	keys := m.themeState.keys
	before := m.View().Content

	m, cmd := pressSlotKey(t, m, slotDarkPress)

	if m.themeState.keys != keys {
		t.Errorf("`d` over a nil persister left keys %+v, want the untouched %+v", m.themeState.keys, keys)
	}
	if !m.themePanel.open {
		t.Error("`d` closed the panel over a nil persister")
	}
	if got := m.themePanel.message; got.Kind != themeMessageNone {
		t.Errorf("a nil persister raised the message %+v; it is the absence of a WRITER, not a failed write", got)
	}
	if cmd != nil {
		t.Errorf("`d` over a nil persister scheduled %T, want nothing", cmd)
	}
	if got := m.View().Content; got != before {
		t.Errorf("`d` over a nil persister changed the frame\nbefore: %q\nafter:  %q", escSeq(before), escSeq(got))
	}

	wired, persister := newCommitPairPanelModel(t, rows)
	wired, _ = pressSlotKey(t, arrowToThemeRow(t, wired, target), slotDarkPress)
	requireSlotCommits(t, persister, slotCommit{slug: target, member: theme.MemberDark})
	requirePairKeys(t, wired, rows[0].Slug, target)
}

func TestPanelSlotCommit_EnterAfterSlotRaisesNoMessageAndSchedulesNothing(t *testing.T) {
	dir := t.TempDir()
	themetest.Write(t, dir, "aurora.theme", themetest.MonochromeLines("#101010"))
	m, persister := newSlotPairPanelModel(t, dir, theme.DefaultLightSlug, "aurora")

	m, _ = pressSlotKey(t, m, slotDarkPress)
	requireSlotCommits(t, persister, slotCommit{slug: "aurora", member: theme.MemberDark})
	requirePairKeys(t, m, theme.DefaultLightSlug, "aurora")

	m, cmd := pressCommitKey(t, m)

	requireConstantKeys(t, m, "aurora")
	if len(persister.constants) != 1 || persister.constants[0] != "aurora" {
		t.Errorf("`Enter` after a slot commit recorded constants %v, want exactly [aurora]", persister.constants)
	}
	requireBadge(t, m, "aurora", theme.BadgeConstant)
	requireBadge(t, m, theme.DefaultLightSlug, theme.BadgeNone)
	if got := m.themePanel.message; got.Kind != themeMessageNone {
		t.Errorf("`Enter` after a slot commit raised the message %+v, want the slot empty", got)
	}
	if cmd != nil {
		t.Errorf("`Enter` scheduled %T; the write lands on this keypress rather than awaiting an answer", cmd)
	}
	if !m.themePanel.open {
		t.Error("`Enter` closed the panel")
	}
}

func TestPanelSlotCommit_NoOtherIO(t *testing.T) {
	requireCommitDoesNoOtherIO(t,
		theme.RawKeys{Light: theme.DefaultLightSlug, Dark: "sunset"},
		"the slot commit",
		func(t *testing.T, m Model) (Model, tea.Cmd) { return pressSlotKey(t, m, slotDarkPress) },
		func(t *testing.T, p *fakeThemePersister) {
			requireSlotCommits(t, p, slotCommit{slug: "sunset", member: theme.MemberDark})
		},
	)
}
