package tui

import (
	"go/ast"
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/sourceguardtest"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/themetest"
)

// A real loader: a stub seam answering with a fixed union would make every row
// and every `●` a statement about the fixture.
func TestSlotOverConstant_ClearsTheConstantAndSetsTheSlot(t *testing.T) {
	dir := t.TempDir()
	themetest.Write(t, dir, "aurora.theme", themetest.MonochromeLines("#101010"))
	m, _, persister := newRecomputePanelModel(t, dir, theme.RawKeys{Theme: "aurora"})
	requireBadge(t, m, "aurora", theme.BadgeConstant)
	requireBadgeText(t, m, 1, 0, 0)

	m = arrowToThemeRow(t, m, "nord")
	m, cmd := pressSlotKey(t, m, slotLightPress)

	requireSlotCommits(t, persister, slotCommit{slug: "nord", member: theme.MemberLight})
	if len(persister.slugs) != 1 {
		t.Errorf("`l` made %d seam call(s) (%v), want the single atomic slot write", len(persister.slugs), persister.slugs)
	}
	requirePairKeys(t, m, "nord", "")
	if got := m.themePanel.message; got.Kind != themeMessageNone {
		t.Errorf("`l` over a constant left the message %+v, want the slot empty — nothing is asked", got)
	}
	if cmd != nil {
		t.Errorf("`l` over a constant scheduled %T, want nothing", cmd)
	}
	requireBadge(t, m, "nord", theme.BadgeLight)
	requireBadge(t, m, "aurora", theme.BadgeNone)
	requireBadge(t, m, theme.DefaultDarkSlug, theme.BadgeDark)
	requireBadgeText(t, m, 0, 1, 1)

	if got := themePanelSeamCallers(t, "loadNewlyLiveSlot"); !slices.Equal(got, []string{"commitSlotOverConstant"}) {
		t.Errorf("the newly-live-slot seam is called from %v, want exactly [commitSlotOverConstant] — the slot load has one route in", got)
	}
}

func TestSlotOverConstant_NilPersisterIsInert(t *testing.T) {
	t.Run("the keypress writes and moves nothing", func(t *testing.T) {
		rows := arrowValidRows(t, 4)
		persisted, target := rows[0].Slug, rows[2].Slug

		m := openCommitPanel(t, newArrowPanelDeps(t, rows, persisted), PageSessions, persisted)
		if m.themeState.persister != nil {
			t.Fatalf("fixture: the model holds persister %#v, want none", m.themeState.persister)
		}
		m = arrowToThemeRow(t, m, target)
		before := m.View().Content

		after, cmd := pressSlotKey(t, m, slotDarkPress)

		requireConstantKeys(t, after, persisted)
		if cmd != nil {
			t.Errorf("`d` over a nil persister scheduled %T, want nothing", cmd)
		}
		if got := after.View().Content; got != before {
			t.Errorf("`d` over a nil persister changed the frame\nbefore: %q\nafter:  %q", escSeq(before), escSeq(got))
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
			t.Errorf("`d` over a nil persister emitted %d `theme: loaded` line(s), want none — no write landed for a load to follow\n%s", len(got), sink.Body())
		}
		if got := sink.Records().WithMessage("fallback applied"); len(got) != 0 {
			t.Errorf("`d` over a nil persister emitted %d `theme: fallback applied` line(s), want none — a load run here would collapse the un-mirrored constant's empty slots onto the shipped defaults and report a load for a write that never happened\n%s", len(got), sink.Body())
		}
		if m.themeState.nomination != nomination || !m.themeState.nomination.IsConstant() {
			t.Errorf("`d` over a nil persister left the nomination %+v, want the untouched constant", m.themeState.nomination)
		}
		requireConstantKeys(t, m, conversionConstant)
	})
}

// Structurally unreachable, so the cursor is placed directly.
func TestSlotOverConstant_UncommittableRowWritesNothing(t *testing.T) {
	for _, tc := range []struct {
		name string
		row  func(t *testing.T) theme.Row
	}{
		{name: "an unselectable row", row: func(*testing.T) theme.Row { return arrowInvalidRow(arrowSlug(1)) }},
		{name: "a selectable row carrying no slug", row: func(t *testing.T) theme.Row {
			return theme.Row{Source: theme.SourceBuiltin, Theme: arrowPalette(t, 1)}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows := []theme.Row{arrowValidRow(t, arrowSlug(0), 0), tc.row(t)}
			m, persister := newCommitPanelModel(t, rows, rows[0].Slug)
			m.themePanel.list.Select(1)
			if _, ok := committableThemeSlug(m.themePanel.list); ok {
				t.Fatalf("fixture: the cursor is on %+v, a row `d` can commit", themePanelCursorRow(t, m))
			}

			m, cmd := pressSlotKey(t, m, slotDarkPress)

			requireSlotCommits(t, persister)
			requireConstantKeys(t, m, rows[0].Slug)
			if !m.themePanel.open {
				t.Error("`d` on a row it cannot commit closed the panel")
			}
			if cmd != nil {
				t.Errorf("`d` on a row it cannot commit scheduled %T, want nothing", cmd)
			}
		})
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
