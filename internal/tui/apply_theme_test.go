package tui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/tmux"
)

const swapProbeSessions = 12

func newSwapProbeModel(t *testing.T, before theme.Theme, mode prefs.SessionListMode, reader *fakeStamper) Model {
	t.Helper()
	const w, h = 120, 40

	projects := []project.Project{{Path: reader.path, Name: "Portal", Tags: []string{"work"}}}
	sessions := make([]tmux.Session, 0, swapProbeSessions)
	for i := range swapProbeSessions {
		sessions = append(sessions, tmux.Session{Name: nameN(i), Windows: 1})
	}

	m := Build(Deps{
		Lister:      fakeLister{},
		Theme:       theme.ConstantNomination(before),
		InitialMode: mode,
		DirReader:   reader,
		DirRunner:   &fakeDirRunner{gitRoot: reader.path},
	})
	m.termWidth = w
	m.termHeight = h
	m.applySessionListSize(m.contentWidth(), m.contentHeight())
	m.setProjects(projects)
	m.projectList.SetItems(ProjectsToListItems(projects))
	m.applyProjectListSize(m.contentWidth(), m.contentHeight())
	m.applySessions(sessions)

	m.activePage = PageSessions
	_ = m.viewSessionList()
	m.activePage = PageProjects
	_ = m.viewProjectList()
	m.activePage = PageSessions

	// Re-arm the lazy pass: the pre-render derived each session's dir, and a warm
	// cache would make "zero reads" true for the wrong reason.
	m.derivedDirs = nil
	return m
}

type swapProbeMode struct {
	name         string
	mode         prefs.SessionListMode
	rebuildReads bool
}

func swapProbeGroupingModes() []swapProbeMode {
	return []swapProbeMode{
		{"flat", prefs.ModeFlat, false},
		{"by-project", prefs.ModeByProject, true},
		{"by-tag", prefs.ModeByTag, true},
	}
}

func TestApplyTheme_RestylesWithoutRebuild(t *testing.T) {
	before, after := probeThemeBefore(t), probeThemeAfter(t)

	for _, tc := range swapProbeGroupingModes() {
		t.Run(tc.name, func(t *testing.T) {
			reader := &fakeStamper{path: t.TempDir()}
			m := newSwapProbeModel(t, before, tc.mode, reader)

			reader.reads = nil
			m.ApplyTheme(after)

			if len(reader.reads) != 0 {
				t.Errorf("ApplyTheme performed %d lazy pane read(s) %v — a swap is the O(1) restyle, never the rebuild's dir-resolution pass", len(reader.reads), reader.reads)
			}

			frame := m.viewSessionList()
			assertRepointed(t, tc.name+" sessions frame after ApplyTheme",
				frame, tokenBgSeq(t, after.Canvas), tokenBgSeq(t, before.Canvas))

			if len(reader.reads) != 0 {
				t.Errorf("rendering after ApplyTheme performed %d lazy pane read(s) %v — the render path pays no reads either", len(reader.reads), reader.reads)
			}

			if !tc.rebuildReads {
				return
			}
			m.rebuildSessionList()
			if len(reader.reads) == 0 {
				t.Fatal("positive control: rebuildSessionList performed no pane reads, so the counting DirReader proves nothing about ApplyTheme")
			}
		})
	}
}

type countingStores struct {
	projectStore   *countingProjectStore
	projectEditor  *countingProjectEditor
	aliasEditor    *countingAliasEditor
	modePersister  *countingModePersister
	themePersister *countingThemePersister
	scrollback     *countingScrollbackReader
	lister         *countingLister
}

func (c countingStores) calls() int {
	return c.projectStore.calls + c.projectEditor.calls + c.aliasEditor.calls +
		c.modePersister.calls + c.themePersister.calls + c.scrollback.calls + c.lister.calls
}

func (c countingStores) reset() {
	c.projectStore.calls = 0
	c.projectEditor.calls = 0
	c.aliasEditor.calls = 0
	c.modePersister.calls = 0
	c.themePersister.calls = 0
	c.scrollback.calls = 0
	c.lister.calls = 0
}

func (c countingStores) exercise() {
	_, _ = c.projectStore.List()
	_ = c.projectEditor.AddTag("/x", "y")
	_, _ = c.aliasEditor.Load()
	_ = c.modePersister.Save(prefs.ModeFlat)
	_ = c.themePersister.CommitTheme("nord")
	_, _ = c.scrollback.Tail("pane")
	_, _ = c.lister.ListSessions()
}

type countingProjectStore struct{ calls int }

func (c *countingProjectStore) List() ([]project.Project, error) {
	c.calls++
	return nil, nil
}

func (c *countingProjectStore) CleanStale() ([]project.Project, error) {
	c.calls++
	return nil, nil
}

func (c *countingProjectStore) Remove(string, string) error {
	c.calls++
	return nil
}

type countingProjectEditor struct{ calls int }

func (c *countingProjectEditor) Rename(string, string, string) error { c.calls++; return nil }
func (c *countingProjectEditor) AddTag(string, string) error         { c.calls++; return nil }
func (c *countingProjectEditor) RemoveTag(string, string) error      { c.calls++; return nil }

type countingAliasEditor struct{ calls int }

func (c *countingAliasEditor) Load() (map[string]string, error) { c.calls++; return nil, nil }
func (c *countingAliasEditor) SetAndSave(string, string, string) error {
	c.calls++
	return nil
}

func (c *countingAliasEditor) DeleteAndSave(string, string) (bool, error) {
	c.calls++
	return true, nil
}

type countingModePersister struct{ calls int }

func (c *countingModePersister) Save(prefs.SessionListMode) error { c.calls++; return nil }

type countingThemePersister struct{ calls int }

func (c *countingThemePersister) CommitTheme(string) error { c.calls++; return nil }

func (c *countingThemePersister) CommitThemeSlot(string, theme.Member) error {
	c.calls++
	return nil
}

type countingScrollbackReader struct{ calls int }

func (c *countingScrollbackReader) Tail(string) ([]byte, error) { c.calls++; return nil, nil }

type countingLister struct{ calls int }

func (c *countingLister) ListSessions() ([]tmux.Session, error) { c.calls++; return nil, nil }

func newCountingStores() countingStores {
	return countingStores{
		projectStore:   &countingProjectStore{},
		projectEditor:  &countingProjectEditor{},
		aliasEditor:    &countingAliasEditor{},
		modePersister:  &countingModePersister{},
		themePersister: &countingThemePersister{},
		scrollback:     &countingScrollbackReader{},
		lister:         &countingLister{},
	}
}

func TestApplyTheme_PerformsNoFileRead(t *testing.T) {
	before, after := probeThemeBefore(t), probeThemeAfter(t)

	t.Run("no seam reaches disk across fifty swaps", func(t *testing.T) {
		stores := newCountingStores()

		stores.exercise()
		if stores.calls() != 7 {
			t.Fatalf("positive control: exercising the seven seams recorded %d calls, want 7 — the counters do not count", stores.calls())
		}

		m := Build(Deps{
			Lister:         stores.lister,
			Theme:          theme.ConstantNomination(before),
			ProjectStore:   stores.projectStore,
			ProjectEditor:  stores.projectEditor,
			AliasEditor:    stores.aliasEditor,
			ModePersister:  stores.modePersister,
			ThemePersister: stores.themePersister,
			Reader:         stores.scrollback,
		})
		m.termWidth, m.termHeight = 120, 40
		m.applySessionListSize(m.contentWidth(), m.contentHeight())
		_ = m.viewSessionList()

		stores.reset()
		for range 50 {
			m.ApplyTheme(after)
			m.ApplyTheme(before)
		}

		if got := stores.calls(); got != 0 {
			t.Errorf("fifty swaps made %d file-touching seam call(s) — a swap reads nothing (project store %d, project editor %d, alias editor %d, mode persister %d, theme persister %d, scrollback %d, lister %d)",
				got, stores.projectStore.calls, stores.projectEditor.calls, stores.aliasEditor.calls,
				stores.modePersister.calls, stores.themePersister.calls, stores.scrollback.calls, stores.lister.calls)
		}
	})

	t.Run("the model holds no theme loader", func(t *testing.T) {
		for _, path := range loaderFields(reflect.TypeFor[Model](), "Model") {
			t.Errorf("%s is a theme.Loader — a swap takes a LOADED palette; a loader on the model is the shape through which one would grow a file read", path)
		}
	})
}

func loaderFields(tp reflect.Type, path string) []string {
	var found []string
	for f := range tp.Fields() {
		at := path + "." + f.Name
		switch {
		case f.Type == reflect.TypeFor[theme.Loader]():
			found = append(found, at)
		case f.Type.Kind() == reflect.Struct && f.Type.PkgPath() == tp.PkgPath():
			found = append(found, loaderFields(f.Type, at)...)
		}
	}
	return found
}

func newSwapFrameModel(t *testing.T, before theme.Theme, colourless bool) Model {
	t.Helper()
	const w, h = 120, 40

	sessions := make([]tmux.Session, 0, swapProbeSessions)
	projects := make([]project.Project, 0, swapProbeSessions)
	for i := range swapProbeSessions {
		sessions = append(sessions, tmux.Session{Name: nameN(i), Windows: 1, Dir: "/Users/leeovery/Code/" + nameN(i)})
		projects = append(projects, project.Project{Name: nameN(i), Path: "/Users/leeovery/Code/" + nameN(i)})
	}

	m := Build(Deps{
		Lister:  fakeLister{},
		Theme:   theme.ConstantNomination(before),
		NoColor: colourless,
	})
	m.termWidth = w
	m.termHeight = h
	m.applySessionListSize(m.contentWidth(), m.contentHeight())
	m.setProjects(projects)
	m.projectList.SetItems(ProjectsToListItems(projects))
	m.applyProjectListSize(m.contentWidth(), m.contentHeight())
	m.applySessions(sessions)

	frame := m.View().Content
	if !strings.Contains(frame, nameN(0)) {
		t.Fatalf("probe setup: the pre-swap frame does not render the session rows, so a frame comparison would compare nothing: %q", escSeq(frame))
	}
	return m
}

func TestApplyTheme_DoesNotMoveStartupCanvasHex(t *testing.T) {
	before, after := probeThemeBefore(t), probeThemeAfter(t)
	m := newSwapFrameModel(t, before, false)

	startup := m.themeState.startupCanvasHex
	if startup != before.Canvas.Value {
		t.Fatalf("probe setup: startupCanvasHex = %q, want the pre-swap canvas %q", startup, before.Canvas.Value)
	}

	m.ApplyTheme(after)
	if m.themeState.startupCanvasHex != startup {
		t.Errorf("after one swap startupCanvasHex = %q, want %q unchanged — it is frozen at gate resolution, never re-derived from the active theme", m.themeState.startupCanvasHex, startup)
	}

	for range 50 {
		m.ApplyTheme(before)
		m.ApplyTheme(after)
	}
	if m.themeState.startupCanvasHex != startup {
		t.Errorf("after fifty swaps startupCanvasHex = %q, want %q unchanged", m.themeState.startupCanvasHex, startup)
	}
}

func TestApplyTheme_SameThemeIsANoOp(t *testing.T) {
	before := probeThemeBefore(t)
	m := newSwapFrameModel(t, before, false)

	frameBefore := m.View().Content
	m.ApplyTheme(before)
	frameAfter := m.View().Content

	if frameAfter != frameBefore {
		t.Errorf("swapping to the ACTIVE theme changed the frame; a same-theme swap is a no-op\nbefore: %q\nafter:  %q", escSeq(frameBefore), escSeq(frameAfter))
	}
}

func TestApplyTheme_RepeatedSwapsAreIdempotent(t *testing.T) {
	before, after := probeThemeBefore(t), probeThemeAfter(t)
	m := newSwapFrameModel(t, before, false)

	m.ApplyTheme(after)
	once := m.View().Content
	m.ApplyTheme(after)
	twice := m.View().Content

	if twice != once {
		t.Errorf("A→B→B does not render what A→B renders; the swap accumulates state\nonce:  %q\ntwice: %q", escSeq(once), escSeq(twice))
	}
}

func TestApplyTheme_ColourlessStaysColourless(t *testing.T) {
	const (
		fgTruecolor = "38;2;"
		bgTruecolor = "48;2;"
	)
	before, after := probeThemeBefore(t), probeThemeAfter(t)

	colourless := newSwapFrameModel(t, before, true)
	colourless.ApplyTheme(after)
	frame := colourless.View().Content

	if strings.Contains(frame, fgTruecolor) {
		t.Errorf("post-swap colourless frame carries a %q foreground sequence — NO_COLOR imposes no hue, and a swap may not reintroduce one: %q", fgTruecolor, escSeq(frame))
	}
	if strings.Contains(frame, bgTruecolor) {
		t.Errorf("post-swap colourless frame carries a %q background sequence — NO_COLOR paints no canvas, and a swap may not reintroduce one: %q", bgTruecolor, escSeq(frame))
	}

	coloured := newSwapFrameModel(t, before, false)
	coloured.ApplyTheme(after)
	control := coloured.View().Content
	if !strings.Contains(control, fgTruecolor) || !strings.Contains(control, bgTruecolor) {
		t.Fatal("positive control: the COLOURED post-swap frame carries no truecolor SGR, so the colourless assertions prove nothing")
	}
}
