package tui

import (
	"bytes"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui/theme"
)

// This file is the §5.1 grouped-reskin gate (task 2-9). It pins the RENDER-ONLY
// reskin of the group HeaderItem and the grouped-row indent: the heading text in
// text.detail with the `··· N` count in text.dim (two separately-styled runs),
// and grouped session rows nested one indent level further than flat (cursor at
// col 2, name at col 4) while flat rows sit flush at col 2. The grouping
// MACHINERY (items / order / catch-alls / cursor-skip / signpost path) is
// asserted unchanged.
//
// Colour roles are pinned with exact mode-resolved SGR (the §2.9 token core),
// like the row / footer / header tests — a token swap is caught, not merely the
// glyph's presence. No t.Parallel() (the package-level mock convention and the
// shared canvas helpers make parallelism unsafe across this package's tests).

// renderHeaderRow renders a single HeaderItem through the production delegate at
// the given width and returns the styled string the delegate emitted.
func renderHeaderRow(d SessionDelegate, width int, h HeaderItem) string {
	m := list.New([]list.Item{h}, d, width, 10)
	var buf bytes.Buffer
	d.Render(&buf, m, 0, h)
	return buf.String()
}

// TestGroupHeading_TextDetailHeadingWithTextDimCount asserts §5.1: the heading
// label renders in text.detail and the `··· N` count in text.dim (dimmer) — two
// separately-styled runs, not one faint run. Pinned in exact mode-resolved SGR so
// a token swap (or a regression back to a single faint run) is caught.
func TestGroupHeading_TextDetailHeadingWithTextDimCount(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		d := SessionDelegate{Mode: mode}
		out := renderHeaderRow(d, 80, HeaderItem{Heading: "Portal", Count: 2, Key: "/p/portal"})

		detail := tokenFgSeq(t, theme.MV.TextDetail, mode)
		dim := tokenFgSeq(t, theme.MV.TextDim, mode)

		if !strings.Contains(out, detail) {
			t.Errorf("[%v] heading missing text.detail fg %q: %q", mode, detail, escSeq(out))
		}
		if !strings.Contains(out, dim) {
			t.Errorf("[%v] count missing text.dim fg %q: %q", mode, dim, escSeq(out))
		}
		// The two runs must be DISTINCT — a precondition that the heading and the
		// count are not painted with one shared style.
		if detail == dim {
			t.Fatalf("[%v] test precondition broken: text.detail == text.dim", mode)
		}

		// The visible text keeps the "Heading ··· N" shape.
		vis := ansi.Strip(out)
		if want := "Portal " + groupSeparator + " 2"; !strings.Contains(vis, want) {
			t.Errorf("[%v] heading text = %q, want it to contain %q", mode, vis, want)
		}
	}
}

// TestGroupHeading_HeadingRunCarriesDetailCountRunCarriesDim asserts the SPLIT is
// per-run: the text.detail SGR sits before the heading word and the text.dim SGR
// sits before the count digits — so the heading word is detail and the count is
// dim, not the reverse and not one run spanning both.
func TestGroupHeading_HeadingRunCarriesDetailCountRunCarriesDim(t *testing.T) {
	d := SessionDelegate{Mode: theme.Dark}
	out := renderHeaderRow(d, 80, HeaderItem{Heading: "Portal", Count: 7, Key: "/p/portal"})

	detail := tokenFgSeq(t, theme.MV.TextDetail, theme.Dark)
	dim := tokenFgSeq(t, theme.MV.TextDim, theme.Dark)

	// In the raw (un-stripped) output the heading glyphs must appear under a
	// text.detail SGR and the count digit under a text.dim SGR.
	detailIdx := strings.Index(out, detail)
	dimIdx := strings.Index(out, dim)
	if detailIdx < 0 || dimIdx < 0 {
		t.Fatalf("missing a run: detailIdx=%d dimIdx=%d in %q", detailIdx, dimIdx, escSeq(out))
	}
	// The detail (heading) run comes before the dim (count) run — heading first,
	// count last in the "Heading ··· N" shape.
	if detailIdx > dimIdx {
		t.Errorf("text.detail run (idx %d) should precede the text.dim run (idx %d): %q", detailIdx, dimIdx, escSeq(out))
	}
	// The count digit "7" must be under the dim run, not the detail run.
	dimRun := out[dimIdx:]
	if !strings.Contains(dimRun, "7") {
		t.Errorf("count digit '7' not under the text.dim run: %q", escSeq(dimRun))
	}
}

// TestGroupHeading_NoFaintAttributeAtCallSite asserts the reskin replaced the
// single faint run: the delegate no longer emits the SGR faint attribute (CSI 2)
// for the heading. Lipgloss merges all of a style's params into ONE CSI per run
// and emits attributes (Faint=2) BEFORE the colour markers, so the old faint run
// rendered as "\x1b[2;38;2;...m" (a leading "2;" right after the "["). The
// truecolor markers themselves are "38;2;" / "48;2;", which legitimately contain
// ";2;" — so the discriminating signature of a surviving faint run is the LEADING
// "[2;38" / "[2;48" (faint then a colour marker) or a bare "[2m". The two-run
// token render carries no such leading faint param. (The source call-site no
// longer references Faint at all; this is the render-level cross-check.)
func TestGroupHeading_NoFaintAttributeAtCallSite(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		d := SessionDelegate{Mode: mode}
		out := renderHeaderRow(d, 80, HeaderItem{Heading: "Work", Count: 3, Key: "/p/work"})
		for _, faint := range []string{"[2;38", "[2;48", "[2m"} {
			if strings.Contains(out, faint) {
				t.Errorf("[%v] heading still emits a leading faint SGR param %q (want two token runs, no faint): %q", mode, faint, escSeq(out))
			}
		}
	}
}

// TestGroupedRow_NestsOneLevelFurtherThanFlat asserts §5.1: a grouped session row
// (GroupKey set) nests one indent level further than flat — the cursor/selector
// bar lands at col 2 and the name at col 4 — while a flat row sits flush (bar at
// col 0, name at col 2).
func TestGroupedRow_NestsOneLevelFurtherThanFlat(t *testing.T) {
	const w = 80

	// Flat: bar at col 0, name flush at col 2.
	flat := flatItems(tmux.Session{Name: "flatname", Windows: 1})
	flatOut := renderRow(SessionDelegate{}, w, flat, 0, 0)
	if got := visibleColOf(flatOut, "flatname"); got != 2 {
		t.Errorf("flat name at col %d, want 2 (flush after the 2-cell selector bar)", got)
	}

	// Grouped (selected, so the ▌ bar prints): cursor/bar at col 2, name at col 4.
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

// TestFlatRow_StaysFlushAtColTwo re-pins the negative of the grouped indent: a
// flat row (empty GroupKey) renders the name flush at col 2 and the selector bar
// at col 0 — the grouped indent is gated on GroupKey != "" and must not leak into
// the flat row.
func TestFlatRow_StaysFlushAtColTwo(t *testing.T) {
	const w = 80
	items := flatItems(tmux.Session{Name: "flatrow", Windows: 2, Attached: false})
	// Selected so the bar prints at col 0.
	out := renderRow(SessionDelegate{}, w, items, 0, 0)

	if got := visibleColOf(out, "▌"); got != 0 {
		t.Errorf("flat selector bar at col %d, want 0 (flush, no grouped indent)", got)
	}
	if got := visibleColOf(out, "flatrow"); got != 2 {
		t.Errorf("flat name at col %d, want 2 (flush after the 2-cell selector bar)", got)
	}
}

// TestGroupedRow_UnselectedAlsoIndents asserts the indent is the row's layout, not
// a selection artefact: an UNSELECTED grouped row also places its name at col 4
// (the two blank selector cells sit at cols 2-3).
func TestGroupedRow_UnselectedAlsoIndents(t *testing.T) {
	const w = 80
	grouped := []list.Item{
		SessionItem{Session: tmux.Session{Name: "row-zero", Windows: 1}, GroupKey: "/p/portal", GroupHeading: "Portal"},
		SessionItem{Session: tmux.Session{Name: "row-one", Windows: 1}, GroupKey: "/p/portal", GroupHeading: "Portal"},
	}
	// Render row 1 with the cursor on row 0 → row 1 is unselected (no ▌ bar).
	out := renderRow(SessionDelegate{}, w, grouped, 1, 0)
	if strings.Contains(ansi.Strip(out), "▌") {
		t.Fatalf("unselected grouped row must not carry the ▌ bar: %q", ansi.Strip(out))
	}
	if got := visibleColOf(out, "row-one"); got != 4 {
		t.Errorf("unselected grouped name at col %d, want 4 (indent is layout, not selection)", got)
	}
}

// TestGroupHeading_IndentsToColTwo asserts §5.1: a group header's heading text
// indents to col 2 (the title-box left edge / the flat-name column), not flush at
// col 0.
func TestGroupHeading_IndentsToColTwo(t *testing.T) {
	out := renderHeaderRow(SessionDelegate{}, 80, HeaderItem{Heading: "Portal", Count: 2, Key: "/p/portal"})
	if got := visibleColOf(out, "Portal"); got != 2 {
		t.Errorf("group heading at col %d, want 2 (aligned to the title-box left edge)", got)
	}
}

// TestCatchAllHeadings_UseSameHeadingStyle asserts §5.1 / the acceptance criterion:
// the catch-all (Unknown / Untagged) headings render with the SAME two-run
// heading style (text.detail heading + text.dim count) as a resolvable group's
// heading — they are HeaderItems too.
func TestCatchAllHeadings_UseSameHeadingStyle(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		d := SessionDelegate{Mode: mode}
		detail := tokenFgSeq(t, theme.MV.TextDetail, mode)
		dim := tokenFgSeq(t, theme.MV.TextDim, mode)

		for _, heading := range []string{"Unknown", "Untagged"} {
			out := renderHeaderRow(d, 80, HeaderItem{Heading: heading, Count: 3, Key: heading})
			if !strings.Contains(out, detail) {
				t.Errorf("[%v] catch-all %q heading missing text.detail fg %q: %q", mode, heading, detail, escSeq(out))
			}
			if !strings.Contains(out, dim) {
				t.Errorf("[%v] catch-all %q count missing text.dim fg %q: %q", mode, heading, dim, escSeq(out))
			}
			if got := visibleColOf(out, heading); got != 2 {
				t.Errorf("[%v] catch-all %q heading at col %d, want 2 (same indent as resolvable groups)", mode, heading, got)
			}
		}
	}
}

// TestCatchAllRow_IndentsLikeResolvableGroupRow asserts a catch-all session row
// (its GroupKey is stamped to the catch-all heading by orderedSessionItems, so
// GroupKey != "") indents one level further than flat, exactly like a resolvable
// group's row — Unknown / Untagged rows are nested children too.
func TestCatchAllRow_IndentsLikeResolvableGroupRow(t *testing.T) {
	const w = 80
	dir := t.TempDir()
	// One known project + one session whose dir is unknown → an Unknown catch-all
	// session row carrying GroupKey == "Unknown".
	projects := []project.Project{{Path: dir, Name: "Known"}}
	sessions := []tmux.Session{
		{Name: "known-1", Dir: dir},
		{Name: "orphan-1", Dir: "/nope/elsewhere"},
	}
	items := buildByProject(sessions, project.NewIndex(projects))

	// Find the orphan (catch-all) session item and assert it carries a GroupKey
	// and renders nested at col 4.
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

// TestGroupedRow_OneDelegateLine re-pins the §3.5 pagination invariant for the
// reskinned grouped rows: a HeaderItem and a grouped SessionItem each render
// EXACTLY one delegate line (no newline), so bubbles/list pagination stays exact.
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

// TestGroupedRow_NeverOverflowsAtNarrowWidths asserts the grouped indent does not
// break the §2.7 / §3.5 no-overflow guard: the indent is folded into the row's
// width budget, so a grouped row never exceeds the list width even at pathological
// narrow widths (the indent shrinks the flex name, it does not push the row wide).
func TestGroupedRow_NeverOverflowsAtNarrowWidths(t *testing.T) {
	for _, w := range []int{1, 5, 10, 20, 26, 30, 40, 80} {
		grouped := []list.Item{
			SessionItem{Session: tmux.Session{Name: "a-fairly-long-grouped-session-name", Windows: 9, Attached: true}, GroupKey: "/p/portal", GroupHeading: "Portal"},
		}
		for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
			out := renderRow(SessionDelegate{Mode: mode}, w, grouped, 0, 0)
			if got := lipgloss.Width(out); got > w {
				t.Errorf("[w=%d %v] grouped row width = %d, overflows the list width", w, mode, got)
			}
		}
	}
}

// TestSessionsTuiNoLipglossTree is the §14.1 build-constraint guard: the tui
// package must NEVER import charm.land/lipgloss/v2/tree — grouping stays pure
// Lipgloss styling in the existing delegate, not a tree widget. A source walk over
// every production .go file in the package fails if any imports the tree package.
func TestSessionsTuiNoLipglossTree(t *testing.T) {
	matches, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	for _, name := range matches {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, imp := range file.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if strings.Contains(path, "lipgloss") && strings.Contains(path, "tree") {
				t.Errorf("%s imports %q — grouping must stay pure Lipgloss in the delegate, not lipgloss/tree (§14.1)", name, path)
			}
		}
	}
}

// TestGroupingMachineryPreserved is the behaviour-parity gate: the grouping
// builders produce the SAME items, order, Pattern A/B materialisation, and
// catch-alls the pre-reskin task produced — only heading colour and row indent
// changed, never the machinery. Asserted against the builder output directly so a
// machinery regression (a reordering, a dropped catch-all, a lost Pattern B
// repeat) is caught independently of render.
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
		items := buildByProject(sessions, project.NewIndex(projects))

		// Walk the items collecting (headingOrName) so we can assert the exact
		// interleave: header, then its rows, with Unknown pinned last.
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
			{Path: other, Name: "Other"}, // untagged → Untagged catch-all
		}
		sessions := []tmux.Session{
			{Name: "portal-1", Dir: dir},
			{Name: "other-1", Dir: other},
		}
		items := buildByTag(sessions, project.NewIndex(projects))

		var shape []string
		for _, it := range items {
			switch v := it.(type) {
			case HeaderItem:
				shape = append(shape, "H:"+v.Heading)
			case SessionItem:
				shape = append(shape, "S:"+v.Session.Name)
			}
		}
		// portal-1 repeats under infra and work (Pattern B); other-1 in Untagged.
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

// TestNoTagsSignpostPathUnchanged asserts the §11.3 No-tags signpost path is
// behaviourally intact: with NO project tagged anywhere, the By-Tag mode degrades
// to the signpost over the flat list (not the grouped header path), and the
// rendered view still carries the signpost text — its render is OUT of scope for
// this task (Phase 4), so the path must be untouched.
func TestNoTagsSignpostPathUnchanged(t *testing.T) {
	dir := t.TempDir()
	projects := []project.Project{{Path: dir, Name: "Portal"}} // no tags anywhere
	sessions := []tmux.Session{{Name: "portal-1", Dir: dir}}

	m := Model{
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

	// With zero tags the By-Tag view must NOT have injected any group HeaderItem —
	// it degrades to the signpost over the flat list.
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
