package tui

import (
	"errors"
	"go/ast"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/sourceguardtest"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/themetest"
)

var commitEnter = tea.KeyPressMsg{Code: tea.KeyEnter}

var errThemeCommitFailed = errors.New("prefs.json: no such file or directory")

func commitPanelProjects() []project.Project {
	return []project.Project{{Path: "/p/one", Name: "one"}, {Path: "/p/two", Name: "two"}}
}

// Takes the whole seam set rather than wiring a persister of its own: the nil
// persister is a state under test.
func openCommitPanel(t *testing.T, deps Deps, p page, cursorSlug string) Model {
	t.Helper()

	m := Build(deps)
	projects := commitPanelProjects()
	m.setProjects(projects)
	m.projectList.SetItems(ProjectsToListItems(projects))
	m.projectList.Select(0)
	m.activePage = p

	m = openPanelForTest(t, m, arrowTermW-2*Hinset, arrowTermH-2*Vinset)
	requireCursorOn(t, m, cursorSlug)
	return m
}

func newCommitPanelModelAt(t *testing.T, rows []theme.Row, cursorSlug string, p page) (Model, *fakeThemePersister) {
	t.Helper()

	persister := &fakeThemePersister{}
	deps := newArrowPanelDeps(t, rows, cursorSlug)
	deps.ThemePersister = persister
	return openCommitPanel(t, deps, p, cursorSlug), persister
}

func newCommitPanelModel(t *testing.T, rows []theme.Row, cursorSlug string) (Model, *fakeThemePersister) {
	t.Helper()
	return newCommitPanelModelAt(t, rows, cursorSlug, PageSessions)
}

// The dark slot is in force (the standing no-answer fallback), so the cursor
// opens on its row.
func commitPairPanelDeps(t *testing.T, rows []theme.Row) Deps {
	t.Helper()

	light, dark := rows[0], rows[1]
	source := &fakeThemeSource{
		union:      themeRowsUnion(rows),
		resolution: pairResolution(light, dark),
	}
	return stubPanelDeps(source, theme.ConstantNomination(dark.Theme), theme.RawKeys{Light: light.Slug, Dark: dark.Slug})
}

func newCommitPairPanelModel(t *testing.T, rows []theme.Row) (Model, *fakeThemePersister) {
	t.Helper()

	persister := &fakeThemePersister{}
	deps := commitPairPanelDeps(t, rows)
	deps.ThemePersister = persister
	return openCommitPanel(t, deps, PageSessions, rows[1].Slug), persister
}

func pressCommitKey(t *testing.T, m Model) (Model, tea.Cmd) {
	t.Helper()
	updated, cmd := m.Update(commitEnter)
	return updated.(Model), cmd
}

func requireCommitted(t *testing.T, p *fakeThemePersister, want ...string) {
	t.Helper()

	if len(p.constants) != len(want) {
		t.Fatalf("the persister recorded constants %v, want %v", p.constants, want)
	}
	for i, slug := range want {
		if p.constants[i] != slug {
			t.Fatalf("commit %d recorded %q, want %q", i+1, p.constants[i], slug)
		}
	}
	if len(p.slots) != 0 {
		t.Errorf("`Enter` committed slot(s) %v; it writes the CONSTANT and clears both slots", p.slots)
	}
}

func requireConstantKeys(t *testing.T, m Model, slug string) {
	t.Helper()
	if got := m.themeState.keys; got != (theme.RawKeys{Theme: slug}) {
		t.Errorf("themeKeys = %+v, want {Theme:%s} — a constant clears both slots", got, slug)
	}
}

// The walk is bounded by the row count: `↓` reverses at the end, so one pass
// visits every reachable row.
func arrowToThemeRow(t *testing.T, m Model, label string) Model {
	t.Helper()

	for range len(m.themePanel.list.Items()) {
		if themePanelCursorRow(t, m).Label() == label {
			return m
		}
		m = pressPanelKey(t, m, arrowDown)
	}
	t.Fatalf("arrowing never reached the %q row; rows: %v", label, themePanelRowLabels(m))
	return m
}

func TestPanelEnter_CommitsTheCursorSlug(t *testing.T) {
	rows := arrowValidRows(t, 4)
	persisted, target := rows[0].Slug, rows[2].Slug
	m, persister := newCommitPanelModel(t, rows, persisted)

	m = arrowToThemeRow(t, m, target)
	if got := m.themeState.keys.Theme; got != persisted {
		t.Fatalf("fixture: the persisted constant is %q, want %q — the two must differ for this to assert anything", got, persisted)
	}

	m, _ = pressCommitKey(t, m)

	requireCommitted(t, persister, target)
	requireConstantKeys(t, m, target)
}

func TestPanelEnter_DoesNotClose(t *testing.T) {
	for _, tc := range entryPages {
		t.Run(tc.name, func(t *testing.T) {
			rows := arrowValidRows(t, 4)
			m, persister := newCommitPanelModelAt(t, rows, rows[0].Slug, tc.page)

			m, cmd := pressCommitKey(t, m)

			requireCommitted(t, persister, rows[0].Slug)
			if !m.themePanel.open {
				t.Error("`Enter` closed the panel; `Esc` is the ONLY way out")
			}
			if isQuitCmd(cmd) {
				t.Error("`Enter` quit Portal from the open panel")
			}
			if m.activePage != tc.page {
				t.Errorf("`Enter` moved the active page to %d, want it left on %d", m.activePage, tc.page)
			}
		})
	}
}

func TestPanelEnter_MutatesRawKeysToAConstant(t *testing.T) {
	rows := arrowValidRows(t, 4)
	m, persister := newCommitPairPanelModel(t, rows)
	before := m.themeState.keys
	if before.Light == "" || before.Dark == "" {
		t.Fatalf("fixture: the persisted keys are %+v, want both slots set", before)
	}

	m, _ = pressCommitKey(t, m)

	requireCommitted(t, persister, before.Dark)
	requireConstantKeys(t, m, before.Dark)
}

// The fixture arrows away first, so a commit that re-resolved from persisted
// state would visibly flip the frame back.
func TestPanelEnter_IsAWriteNotANavigation(t *testing.T) {
	t.Run("the frame is byte-identical across the keypress", func(t *testing.T) {
		rows := arrowValidRows(t, 4)
		persisted, target := rows[0].Slug, rows[2].Slug
		m, persister := newCommitPanelModel(t, rows, persisted)

		m = arrowToThemeRow(t, m, target)
		previewed := m.themeState.active
		if previewed == rows[0].Theme {
			t.Fatal("fixture: arrowing previewed nothing, so an unchanged frame says nothing about the commit")
		}
		before := m.View().Content

		m, _ = pressCommitKey(t, m)

		requireCommitted(t, persister, target)
		if m.themeState.active != previewed {
			t.Errorf("the commit rendered canvas %s, want the previewed %s left alone — a commit is a write, not a navigation", m.themeState.active.Canvas.Value, previewed.Canvas.Value)
		}
		if got := m.View().Content; got != before {
			t.Errorf("the commit changed the composed frame\nbefore: %q\nafter:  %q", escSeq(before), escSeq(got))
		}
	})

	t.Run("an arrow over the same fixture does change the frame", func(t *testing.T) {
		rows := arrowValidRows(t, 4)
		m, _ := newCommitPanelModel(t, rows, rows[0].Slug)
		before := m.View().Content

		if got := pressPanelKey(t, m, arrowDown).View().Content; got == before {
			t.Fatal("an arrow left the frame unchanged, so the byte comparison above proves nothing")
		}
	})

	t.Run("the commit path calls no ApplyTheme", func(t *testing.T) {
		if sites := applyThemeCallSitesIn(t, "theme_panel_commit.go"); len(sites) != 0 {
			t.Errorf("%v call Model.ApplyTheme; a commit is a WRITE, not a navigation — the frame must not move on this keypress", sites)
		}
	})

	t.Run("the scan reports the applies that are there", func(t *testing.T) {
		if sites := applyThemeCallSitesIn(t, "theme_panel.go"); len(sites) == 0 {
			t.Error("the scan found no ApplyTheme call in theme_panel.go, where the open, the close and the arrow-preview all make one; it would pass over the commit path whatever that file held")
		}
	})
}

func applyThemeCallSitesIn(t *testing.T, file string) []string {
	t.Helper()

	parsed, ok := parsePackageFilesByName(t)[file]
	if !ok {
		t.Fatalf("the package holds no %s, so scanning it proves nothing", file)
	}
	var sites []string
	sourceguardtest.ForEachFuncCall(parsed, func(funcName string, call *ast.CallExpr) bool {
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "ApplyTheme" {
			sites = append(sites, funcName)
		}
		return true
	})
	return sites
}

// A real loader over a real themes directory: the stub seam answers with a
// fixed resolution whatever it is handed, which is what must not be faked here.
func TestPanelEnter_EscResolvesTheCommittedTheme(t *testing.T) {
	dir := t.TempDir()
	themetest.Write(t, dir, "aurora.theme", themetest.MonochromeLines("#101010"))
	themetest.Write(t, dir, "sunset.theme", themetest.MonochromeLines("#202020"))
	m, _, _ := newClosePanelModel(t, dir, theme.RawKeys{Theme: "aurora"})
	persister := &fakeThemePersister{}
	WithThemePersister(persister)(&m)

	m = pressThemeKey(t, m)
	if got := m.themeState.active.Canvas.Value; got != "#101010" {
		t.Fatalf("precondition: the open rendered canvas %s, want the persisted aurora's #101010", got)
	}
	m = arrowToThemeRow(t, m, "sunset")

	m, _ = pressCommitKey(t, m)
	requireCommitted(t, persister, "sunset")

	m = closeThemePanelForTest(t, m)

	if m.themePanel.open {
		t.Fatal("Esc left the panel open")
	}
	if got := m.themeState.active.Canvas.Value; got != "#202020" {
		t.Errorf("the close rendered canvas %s, want the newly committed sunset's #202020 — #101010 is what was persisted at OPEN, which a commit replaced", got)
	}
}

func TestPanelEnter_FailedWriteLeavesKeysAlone(t *testing.T) {
	rows := arrowValidRows(t, 4)
	persisted, target := rows[0].Slug, rows[2].Slug

	t.Run("the keypress mutates nothing", func(t *testing.T) {
		m, persister := newCommitPanelModel(t, rows, persisted)
		persister.err = errThemeCommitFailed
		m = arrowToThemeRow(t, m, target)
		previewed := m.themeState.active

		m, _ = pressCommitKey(t, m)

		requireCommitted(t, persister, target)
		requireConstantKeys(t, m, persisted)
		if !m.themePanel.open {
			t.Error("a failed commit closed the panel; `Esc` is the only way out")
		}
		if m.themeState.active != previewed {
			t.Errorf("a failed commit rendered canvas %s, want the previewed %s — a failed commit KEEPS the theme applied in memory", m.themeState.active.Canvas.Value, previewed.Canvas.Value)
		}
	})

	t.Run("the helper returns the error", func(t *testing.T) {
		m, persister := newCommitPanelModel(t, rows, persisted)
		persister.err = errThemeCommitFailed

		if err := (&m).commitSelected((&m).commitConstant); !errors.Is(err, errThemeCommitFailed) {
			t.Errorf("the selected-row constant commit returned %v, want the persister's error — the caller reads the outcome from it", err)
		}
	})
}

func TestPanelEnter_NilPersisterIsInert(t *testing.T) {
	rows := arrowValidRows(t, 4)
	persisted, target := rows[0].Slug, rows[2].Slug

	m := openCommitPanel(t, newArrowPanelDeps(t, rows, persisted), PageSessions, persisted)
	if m.themeState.persister != nil {
		t.Fatalf("fixture: the model holds persister %#v, want none", m.themeState.persister)
	}
	m = arrowToThemeRow(t, m, target)
	before := m.View().Content

	m, cmd := pressCommitKey(t, m)

	requireConstantKeys(t, m, persisted)
	if !m.themePanel.open {
		t.Error("`Enter` closed the panel over a nil persister")
	}
	if got := m.themePanel.message; got.Kind != themeMessageNone {
		t.Errorf("a nil persister raised the message %+v; it is the absence of a WRITER, not a failed write", got)
	}
	if cmd != nil {
		t.Errorf("`Enter` over a nil persister scheduled %T, want nothing", cmd)
	}
	if got := m.View().Content; got != before {
		t.Errorf("`Enter` over a nil persister changed the frame\nbefore: %q\nafter:  %q", escSeq(before), escSeq(got))
	}

	wired, persister := newCommitPanelModel(t, rows, persisted)
	wired, _ = pressCommitKey(t, arrowToThemeRow(t, wired, target))
	requireCommitted(t, persister, target)
	requireConstantKeys(t, wired, target)
}

func TestPanelEnter_RepeatCommitIsIdempotent(t *testing.T) {
	rows := arrowValidRows(t, 4)
	target := rows[2].Slug
	m, persister := newCommitPanelModel(t, rows, rows[0].Slug)
	m = arrowToThemeRow(t, m, target)

	m, _ = pressCommitKey(t, m)
	once := m.View().Content
	onceKeys := m.themeState.keys

	m, cmd := pressCommitKey(t, m)

	requireCommitted(t, persister, target, target)
	if m.themeState.keys != onceKeys {
		t.Errorf("the second commit left keys %+v, want the first's %+v", m.themeState.keys, onceKeys)
	}
	requireConstantKeys(t, m, target)
	if got := m.themePanel.message; got.Kind != themeMessageNone {
		t.Errorf("the repeat commit raised the message %+v; there is no retry affordance and no state to clear first", got)
	}
	if cmd != nil {
		t.Errorf("the repeat commit scheduled %T, want nothing", cmd)
	}
	if got := m.View().Content; got != once {
		t.Errorf("the repeat commit changed the frame\nonce:  %q\ntwice: %q", escSeq(once), escSeq(got))
	}
}

func TestPanelEnter_NoOtherIO(t *testing.T) {
	requireCommitDoesNoOtherIO(t,
		theme.RawKeys{Theme: "sunset"},
		"the commit",
		pressCommitKey,
		func(t *testing.T, p *fakeThemePersister) { requireCommitted(t, p, "sunset") },
	)
}

// Structurally unreachable — the arrows skip unselectable rows and the open-time
// anchor lands on a selectable one — so the cursor is placed directly.
func TestPanelEnter_UnselectableRowWritesNothing(t *testing.T) {
	t.Run("an unselectable row", func(t *testing.T) {
		rows := []theme.Row{arrowValidRow(t, arrowSlug(0), 0), arrowInvalidRow(arrowSlug(1))}
		m, persister := newCommitPanelModel(t, rows, rows[0].Slug)
		m.themePanel.list.Select(1)
		if themePanelCursorRow(t, m).Selectable() {
			t.Fatal("fixture: the cursor is on a selectable row, so there is no refusal to exercise")
		}

		m, cmd := pressCommitKey(t, m)

		requireCommitted(t, persister)
		requireConstantKeys(t, m, rows[0].Slug)
		if !m.themePanel.open {
			t.Error("`Enter` on an unselectable row closed the panel")
		}
		if cmd != nil {
			t.Errorf("`Enter` on an unselectable row scheduled %T, want nothing", cmd)
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

		m, _ = pressCommitKey(t, m)

		requireCommitted(t, persister)
		requireConstantKeys(t, m, rows[0].Slug)
	})

	t.Run("a selectable row does write", func(t *testing.T) {
		rows := []theme.Row{arrowValidRow(t, arrowSlug(0), 0), arrowInvalidRow(arrowSlug(1))}
		m, persister := newCommitPanelModel(t, rows, rows[0].Slug)

		m, _ = pressCommitKey(t, m)

		requireCommitted(t, persister, rows[0].Slug)
		requireConstantKeys(t, m, rows[0].Slug)
	})
}

func TestPanelEnter_RaisesNoMessageAndSchedulesNothing(t *testing.T) {
	for _, tc := range []struct {
		name  string
		build func(*testing.T, []theme.Row) (Model, *fakeThemePersister)
		want  func([]theme.Row) string
	}{
		{
			name:  "adaptive pair",
			build: newCommitPairPanelModel,
			want:  func(rows []theme.Row) string { return rows[1].Slug },
		},
		{
			name: "constant",
			build: func(t *testing.T, rows []theme.Row) (Model, *fakeThemePersister) {
				return newCommitPanelModel(t, rows, rows[0].Slug)
			},
			want: func(rows []theme.Row) string { return rows[0].Slug },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows := arrowValidRows(t, 4)
			m, persister := tc.build(t, rows)

			m, cmd := pressCommitKey(t, m)

			requireCommitted(t, persister, tc.want(rows))
			if got := m.themePanel.message; got.Kind != themeMessageNone {
				t.Errorf("`Enter` raised the message %+v, want the slot empty", got)
			}
			if cmd != nil {
				t.Errorf("`Enter` scheduled %T; the write lands on this keypress rather than awaiting an answer", cmd)
			}
		})
	}
}
