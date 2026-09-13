package tui

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/sourceguardtest"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/tmux"
)

func renderHeaderRow(d SessionDelegate, width int, h HeaderItem) string {
	m := list.New([]list.Item{h}, d, width, 10)
	var buf bytes.Buffer
	d.Render(&buf, m, 0, h)
	return buf.String()
}

func TestGroupHeading_TextDetailHeadingWithTextDimCount(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		d := SessionDelegate{Theme: th}
		out := renderHeaderRow(d, 80, HeaderItem{Heading: "Portal", Count: 2, Key: "/p/portal"})

		detail := tokenFgSeq(t, th.TextMuted)
		dim := tokenFgSeq(t, th.TextSubtle)

		if !strings.Contains(out, detail) {
			t.Errorf("[%v] heading missing text.muted fg %q: %q", themeLabel(th), detail, escSeq(out))
		}
		if !strings.Contains(out, dim) {
			t.Errorf("[%v] count missing text.subtle fg %q: %q", themeLabel(th), dim, escSeq(out))
		}
		if detail == dim {
			t.Fatalf("[%v] test precondition broken: text.muted == text.subtle", themeLabel(th))
		}

		vis := ansi.Strip(out)
		if want := "Portal " + groupSeparator + " 2"; !strings.Contains(vis, want) {
			t.Errorf("[%v] heading text = %q, want it to contain %q", themeLabel(th), vis, want)
		}
	}
}

func TestGroupHeading_HeadingRunCarriesDetailCountRunCarriesDim(t *testing.T) {
	d := SessionDelegate{Theme: testDarkTheme(t)}
	out := renderHeaderRow(d, 80, HeaderItem{Heading: "Portal", Count: 7, Key: "/p/portal"})

	detail := tokenFgSeq(t, testDarkTheme(t).TextMuted)
	dim := tokenFgSeq(t, testDarkTheme(t).TextSubtle)

	detailIdx := strings.Index(out, detail)
	dimIdx := strings.Index(out, dim)
	if detailIdx < 0 || dimIdx < 0 {
		t.Fatalf("missing a run: detailIdx=%d dimIdx=%d in %q", detailIdx, dimIdx, escSeq(out))
	}
	if detailIdx > dimIdx {
		t.Errorf("text.muted run (idx %d) should precede the text.subtle run (idx %d): %q", detailIdx, dimIdx, escSeq(out))
	}
	dimRun := out[dimIdx:]
	if !strings.Contains(dimRun, "7") {
		t.Errorf("count digit '7' not under the text.subtle run: %q", escSeq(dimRun))
	}
}

func TestGroupHeading_NoFaintAttributeAtCallSite(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		d := SessionDelegate{Theme: th}
		out := renderHeaderRow(d, 80, HeaderItem{Heading: "Work", Count: 3, Key: "/p/work"})
		// Lipgloss emits attributes before colour markers, so only a leading
		// faint param discriminates: the truecolor markers contain ";2;" too.
		for _, faint := range []string{"[2;38", "[2;48", "[2m"} {
			if strings.Contains(out, faint) {
				t.Errorf("[%v] heading still emits a leading faint SGR param %q (want two token runs, no faint): %q", themeLabel(th), faint, escSeq(out))
			}
		}
	}
}

func TestGroupedRow_NestsOneLevelFurtherThanFlat(t *testing.T) {
	const w = 80

	flat := flatItems(tmux.Session{Name: "flatname", Windows: 1})
	flatOut := renderRow(SessionDelegate{}, w, flat, 0, 0)
	if got := visibleColOf(flatOut, "flatname"); got != 2 {
		t.Errorf("flat name at col %d, want 2 (flush after the 2-cell selector bar)", got)
	}

	grouped := []list.Item{
		SessionItem{Session: tmux.Session{Name: "grpname", Windows: 1}, GroupKey: "/p/portal", GroupHeading: "Portal"},
	}
	grpOut := renderRow(SessionDelegate{}, w, grouped, 0, 0)
	if got := visibleColOf(grpOut, "▌"); got != 2 {
		t.Errorf("grouped cursor/bar at col %d, want 2 (one indent level further than flat)", got)
	}
	if got := visibleColOf(grpOut, "grpname"); got != 4 {
		t.Errorf("grouped name at col %d, want 4 (one indent level further than flat)", got)
	}
}

func TestFlatRow_StaysFlushAtColTwo(t *testing.T) {
	const w = 80
	items := flatItems(tmux.Session{Name: "flatrow", Windows: 2, Attached: false})
	out := renderRow(SessionDelegate{}, w, items, 0, 0)

	if got := visibleColOf(out, "▌"); got != 0 {
		t.Errorf("flat selector bar at col %d, want 0 (flush, no grouped indent)", got)
	}
	if got := visibleColOf(out, "flatrow"); got != 2 {
		t.Errorf("flat name at col %d, want 2 (flush after the 2-cell selector bar)", got)
	}
}

func TestGroupedRow_UnselectedAlsoIndents(t *testing.T) {
	const w = 80
	grouped := []list.Item{
		SessionItem{Session: tmux.Session{Name: "row-zero", Windows: 1}, GroupKey: "/p/portal", GroupHeading: "Portal"},
		SessionItem{Session: tmux.Session{Name: "row-one", Windows: 1}, GroupKey: "/p/portal", GroupHeading: "Portal"},
	}
	out := renderRow(SessionDelegate{}, w, grouped, 1, 0)
	if strings.Contains(ansi.Strip(out), "▌") {
		t.Fatalf("unselected grouped row must not carry the ▌ bar: %q", ansi.Strip(out))
	}
	if got := visibleColOf(out, "row-one"); got != 4 {
		t.Errorf("unselected grouped name at col %d, want 4 (indent is layout, not selection)", got)
	}
}

func TestGroupHeading_IndentsToColTwo(t *testing.T) {
	out := renderHeaderRow(SessionDelegate{}, 80, HeaderItem{Heading: "Portal", Count: 2, Key: "/p/portal"})
	if got := visibleColOf(out, "Portal"); got != 2 {
		t.Errorf("group heading at col %d, want 2 (aligned to the title-box left edge)", got)
	}
}

func TestCatchAllHeadings_UseSameHeadingStyle(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		d := SessionDelegate{Theme: th}
		detail := tokenFgSeq(t, th.TextMuted)
		dim := tokenFgSeq(t, th.TextSubtle)

		for _, heading := range []string{"Unknown", "Untagged"} {
			out := renderHeaderRow(d, 80, HeaderItem{Heading: heading, Count: 3, Key: heading})
			if !strings.Contains(out, detail) {
				t.Errorf("[%v] catch-all %q heading missing text.muted fg %q: %q", themeLabel(th), heading, detail, escSeq(out))
			}
			if !strings.Contains(out, dim) {
				t.Errorf("[%v] catch-all %q count missing text.subtle fg %q: %q", themeLabel(th), heading, dim, escSeq(out))
			}
			if got := visibleColOf(out, heading); got != 2 {
				t.Errorf("[%v] catch-all %q heading at col %d, want 2 (same indent as resolvable groups)", themeLabel(th), heading, got)
			}
		}
	}
}

func TestCatchAllRow_IndentsLikeResolvableGroupRow(t *testing.T) {
	const w = 80
	dir := t.TempDir()
	projects := []project.Project{{Path: dir, Name: "Known"}}
	sessions := []tmux.Session{
		{Name: "known-1", Dir: dir},
		{Name: "orphan-1", Dir: "/nope/elsewhere"},
	}
	items := buildByProject(sessions, project.NewIndex(projects), nil)

	var idx = -1
	for i, it := range items {
		if si, ok := it.(SessionItem); ok && si.Session.Name == "orphan-1" {
			idx = i
			if si.GroupKey == "" {
				t.Fatalf("catch-all session row has empty GroupKey; the indent gate would skip it")
			}
		}
	}
	if idx < 0 {
		t.Fatalf("orphan-1 catch-all row not found in %v", items)
	}
	out := renderRow(SessionDelegate{}, w, items, idx, idx)
	if got := visibleColOf(out, "orphan-1"); got != 4 {
		t.Errorf("catch-all session name at col %d, want 4 (nested like a resolvable group row)", got)
	}
}

func TestGroupedRow_OneDelegateLine(t *testing.T) {
	d := SessionDelegate{}
	if d.Height() != 1 {
		t.Fatalf("Height() = %d, want 1", d.Height())
	}

	header := renderHeaderRow(d, 80, HeaderItem{Heading: "Portal", Count: 2, Key: "/p/portal"})
	if strings.Contains(header, "\n") {
		t.Errorf("header row emitted more than one line: %q", header)
	}

	grouped := []list.Item{
		SessionItem{Session: tmux.Session{Name: "grp-1", Windows: 1}, GroupKey: "/p/portal", GroupHeading: "Portal"},
	}
	row := renderRow(d, 80, grouped, 0, 0)
	if strings.Contains(row, "\n") {
		t.Errorf("grouped session row emitted more than one line: %q", row)
	}
}

func TestGroupedRow_NeverOverflowsAtNarrowWidths(t *testing.T) {
	for _, w := range []int{1, 5, 10, 20, 26, 30, 40, 80} {
		grouped := []list.Item{
			SessionItem{Session: tmux.Session{Name: "a-fairly-long-grouped-session-name", Windows: 9, Attached: true}, GroupKey: "/p/portal", GroupHeading: "Portal"},
		}
		for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
			out := renderRow(SessionDelegate{Theme: th}, w, grouped, 0, 0)
			if got := lipgloss.Width(out); got > w {
				t.Errorf("[w=%d %v] grouped row width = %d, overflows the list width", w, themeLabel(th), got)
			}
		}
	}
}

func TestSessionsTuiNoLipglossTree(t *testing.T) {
	for _, source := range sourceguardtest.ParsePackageSources(t, ".", false) {
		name := filepath.Base(source.Path)
		for _, imp := range source.File.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if strings.Contains(path, "lipgloss") && strings.Contains(path, "tree") {
				t.Errorf("%s imports %q — grouping must stay pure Lipgloss in the delegate, not lipgloss/tree", name, path)
			}
		}
	}
}

func TestGroupingMachineryPreserved(t *testing.T) {
	t.Run("By Project: Pattern A one row per session with a pinned Unknown catch-all", func(t *testing.T) {
		dirA := t.TempDir()
		dirB := t.TempDir()
		projects := []project.Project{
			{Path: dirA, Name: "Alpha"},
			{Path: dirB, Name: "Bravo"},
		}
		sessions := []tmux.Session{
			{Name: "alpha-1", Dir: dirA},
			{Name: "bravo-1", Dir: dirB},
			{Name: "orphan-1", Dir: "/nowhere"},
		}
		items := buildByProject(sessions, project.NewIndex(projects), nil)

		var shape []string
		for _, it := range items {
			switch v := it.(type) {
			case HeaderItem:
				shape = append(shape, "H:"+v.Heading)
			case SessionItem:
				shape = append(shape, "S:"+v.Session.Name)
			}
		}
		want := []string{
			"H:Alpha", "S:alpha-1",
			"H:Bravo", "S:bravo-1",
			"H:Unknown", "S:orphan-1",
		}
		if strings.Join(shape, "|") != strings.Join(want, "|") {
			t.Errorf("By-Project shape = %v, want %v", shape, want)
		}
	})

	t.Run("By Tag: Pattern B repeats a multi-tag session under each tag, Untagged pinned last", func(t *testing.T) {
		dir := t.TempDir()
		other := t.TempDir()
		projects := []project.Project{
			{Path: dir, Name: "Portal", Tags: []string{"infra", "work"}},
			{Path: other, Name: "Other"},
		}
		sessions := []tmux.Session{
			{Name: "portal-1", Dir: dir},
			{Name: "other-1", Dir: other},
		}
		items := buildByTag(sessions, project.NewIndex(projects), nil)

		var shape []string
		for _, it := range items {
			switch v := it.(type) {
			case HeaderItem:
				shape = append(shape, "H:"+v.Heading)
			case SessionItem:
				shape = append(shape, "S:"+v.Session.Name)
			}
		}
		want := []string{
			"H:infra", "S:portal-1",
			"H:work", "S:portal-1",
			"H:Untagged", "S:other-1",
		}
		if strings.Join(shape, "|") != strings.Join(want, "|") {
			t.Errorf("By-Tag shape = %v, want %v", shape, want)
		}
	})
}

func TestNoTagsSignpostPathUnchanged(t *testing.T) {
	dir := t.TempDir()
	projects := []project.Project{{Path: dir, Name: "Portal"}}
	sessions := []tmux.Session{{Name: "portal-1", Dir: dir}}

	m := Model{
		themeState:      themeState{active: testDarkTheme(t)},
		sessions:        sessions,
		projects:        projects,
		projectIndex:    project.NewIndex(projects),
		sessionList:     newSessionList(nil),
		projectList:     newProjectList(),
		activePage:      PageSessions,
		sessionListMode: prefs.ModeByTag,
		termWidth:       100,
		termHeight:      30,
	}
	m.applySessionListSize(100, 30)
	m.rebuildSessionList()

	for _, it := range m.sessionList.Items() {
		if _, ok := it.(HeaderItem); ok {
			t.Fatalf("zero-tags By-Tag injected a group header; the signpost path must be taken instead")
		}
	}

	view := m.viewSessionList()
	if !strings.Contains(ansi.Strip(view), byTagSignpostText) {
		t.Errorf("zero-tags By-Tag view missing the No-tags signpost %q", byTagSignpostText)
	}
}
