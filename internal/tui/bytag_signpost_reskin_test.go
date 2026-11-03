package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
)

func signpostModel(t *testing.T) Model {
	t.Helper()
	dir := t.TempDir()
	projects := []project.Project{{Path: dir, Name: "Portal"}}
	sessions := []Session{
		{Name: "portal-abc", Dir: dir},
		{Name: "portal-def", Dir: dir},
	}
	m := newRebuildTestModel(t, prefs.ModeByTag, sessions, projects)
	m.termWidth = 80
	m.termHeight = 24
	m.rebuildSessionList()
	if !m.byTagSignpost {
		t.Fatalf("setup invariant: byTagSignpost = false, want true (zero tags anywhere in By Tag mode)")
	}
	return m
}

func TestSignpostReskin_VioletInfoBand(t *testing.T) {
	m := signpostModel(t)

	band := m.renderActiveNoticeBand()
	if band == "" {
		t.Fatal("renderActiveNoticeBand returned empty; the signpost band must own the slot")
	}

	stripped := ansi.Strip(band)
	if !strings.HasPrefix(stripped, noticeBarGlyph) {
		t.Errorf("signpost band does not start with the %q left-bar: %q", noticeBarGlyph, stripped)
	}
	if flat := flattenNoticeBand(band); !strings.Contains(flat, byTagSignpostText) {
		t.Errorf("signpost band missing the message %q: %q", byTagSignpostText, flat)
	}
	violetSeq := tokenFgSeq(t, m.themeState.active.AccentPrimary)
	if !strings.Contains(band, violetSeq) {
		t.Errorf("signpost band missing the accent.primary bar foreground sequence %q:\n%s", violetSeq, band)
	}
	onSelectionSeq := tokenFgSeq(t, m.themeState.active.TextOnSelection)
	if !strings.Contains(band, onSelectionSeq) {
		t.Errorf("signpost band missing the text.on-selection message foreground sequence %q:\n%s", onSelectionSeq, band)
	}
	selectionBgSeq := tokenBgSeq(t, m.themeState.active.BgSelection)
	if !strings.Contains(band, selectionBgSeq) {
		t.Errorf("signpost band missing the bg.selection info-band tint %q (must not be flat):\n%s", selectionBgSeq, band)
	}
	warnBgSeq := tokenBgSeq(t, m.themeState.active.BgAttention)
	if strings.Contains(band, warnBgSeq) {
		t.Errorf("signpost band carries the bg.attention flash tint %q (info band is not a flash):\n%s", warnBgSeq, band)
	}
}

func TestInfoBands_ShareSameTint(t *testing.T) {
	if got := bandInfo.tintToken(testDarkTheme(t)).Name; got != testDarkTheme(t).BgSelection.Name {
		t.Errorf("bandInfo tint token = %q, want bg.selection (shared info-band tint)", got)
	}
	if got := bandCommand.tintToken(testDarkTheme(t)).Name; got != testDarkTheme(t).BgSelection.Name {
		t.Errorf("bandCommand tint token = %q, want bg.selection (shared info-band tint)", got)
	}
	if bandInfo.tintToken(testDarkTheme(t)).Name != bandCommand.tintToken(testDarkTheme(t)).Name {
		t.Errorf("info bands diverge: bandInfo tint %q != bandCommand tint %q (must share one info-band tint)",
			bandInfo.tintToken(testDarkTheme(t)).Name, bandCommand.tintToken(testDarkTheme(t)).Name)
	}
}

func TestSignpostReskin_SpecExactWording(t *testing.T) {
	const want = "No tags yet — add tags in a project's editor: press x for projects, then e to edit"
	if byTagSignpostText != want {
		t.Errorf("byTagSignpostText = %q, want the spec-exact wording %q", byTagSignpostText, want)
	}

	m := signpostModel(t)
	if !viewHasNoticeMessage(t, m, bandInfo, want) {
		t.Errorf("rendered view does not contain the spec-exact signpost wording %q:\n%s", want, m.View().Content)
	}
}

func TestSignpostReskin_OnlyByTagZeroTags(t *testing.T) {
	dir := t.TempDir()
	sessions := []Session{{Name: "portal-abc", Dir: dir}}

	cases := []struct {
		name     string
		mode     prefs.SessionListMode
		projects []project.Project
		want     bool
	}{
		{"Flat with zero tags", prefs.ModeFlat, []project.Project{{Path: dir, Name: "Portal"}}, false},
		{"By-Project with zero tags", prefs.ModeByProject, []project.Project{{Path: dir, Name: "Portal"}}, false},
		{"By-Tag with zero tags", prefs.ModeByTag, []project.Project{{Path: dir, Name: "Portal"}}, true},
		{"By-Tag with a tag present", prefs.ModeByTag, []project.Project{{Path: dir, Name: "Portal", Tags: []string{"work"}}}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := newRebuildTestModel(t, c.mode, sessions, c.projects)
			m.termWidth = 80
			m.termHeight = 24
			m.rebuildSessionList()

			role, _, ok := m.activeNoticeBand()
			gotSignpost := ok && role == bandInfo
			if gotSignpost != c.want {
				t.Errorf("signpost band present = %v, want %v", gotSignpost, c.want)
			}
			if got := viewHasNoticeMessage(t, m, bandInfo, byTagSignpostText); got != c.want {
				t.Errorf("signpost text rendered = %v, want %v:\n%s", got, c.want, m.View().Content)
			}
		})
	}
}

func TestSignpostReskin_ZeroPaneReads(t *testing.T) {
	dir := t.TempDir()
	projects := []project.Project{{Path: dir, Name: "Portal"}}
	sessions := []Session{
		{Name: "portal-a", Dir: ""},
		{Name: "portal-b", Dir: ""},
	}

	stamper := &fakeStamper{path: dir}
	m := newRebuildTestModel(t, prefs.ModeByTag, sessions, projects)
	m.termWidth = 80
	m.termHeight = 24
	m.dirReader = stamper
	m.dirRunner = &fakeDirRunner{gitRoot: dir}

	m.rebuildSessionList()
	_ = m.View().Content

	if !m.byTagSignpost {
		t.Fatalf("setup invariant: byTagSignpost = false, want true (zero tags anywhere)")
	}
	if len(stamper.reads) != 0 {
		t.Errorf("signpost path performed %d pane reads (reads=%v), want 0", len(stamper.reads), stamper.reads)
	}
	if len(stamper.setCalls) != 0 {
		t.Errorf("signpost path performed %d stamp writes, want 0", len(stamper.setCalls))
	}
}

func TestSignpostReskin_GroupingMachineryUntouched(t *testing.T) {
	dir := t.TempDir()
	projects := []project.Project{{Path: dir, Name: "Portal"}}
	sessions := []Session{
		{Name: "portal-abc", Dir: dir},
		{Name: "portal-def", Dir: dir},
	}

	m := newRebuildTestModel(t, prefs.ModeByTag, sessions, projects)
	m.rebuildSessionList()

	if anyTagsExist(projects) {
		t.Fatalf("test invariant: anyTagsExist = true, want false (no project carries a tag)")
	}
	got := m.sessionList.Items()
	want := ToListItems(sessions)
	if len(got) != len(want) {
		t.Fatalf("len(items) = %d, want %d (plain flat slice)", len(got), len(want))
	}
	for i := range want {
		gi := asSessionItem(t, got[i])
		if gi != asSessionItem(t, want[i]) {
			t.Errorf("item %d = %+v, want flat %+v", i, gi, want[i])
		}
		if gi.GroupKey != "" || gi.GroupHeading != "" || gi.CatchAll {
			t.Errorf("item %d carries group metadata (key=%q heading=%q catchAll=%v), want flat",
				i, gi.GroupKey, gi.GroupHeading, gi.CatchAll)
		}
	}
}

func TestSignpostReskin_YieldsToFlashThenReturns(t *testing.T) {
	m := signpostModel(t)

	if !viewHasNoticeMessage(t, m, bandInfo, byTagSignpostText) {
		t.Fatalf("setup invariant: signpost not rendered before the flash:\n%s", m.View().Content)
	}

	const flash = "__TRANSIENT_FLASH__"
	m.setFlash(flash)

	view := m.View().Content
	if !strings.Contains(view, flash) {
		t.Errorf("transient flash must render while active:\n%s", view)
	}
	if viewHasNoticeMessage(t, m, bandInfo, byTagSignpostText) {
		t.Errorf("signpost must yield the slot while the flash holds it:\n%s", view)
	}

	m.clearFlash()

	view = m.View().Content
	if strings.Contains(view, flash) {
		t.Errorf("flash must be gone after clear:\n%s", view)
	}
	if !viewHasNoticeMessage(t, m, bandInfo, byTagSignpostText) {
		t.Errorf("signpost must return to the slot after the flash clears:\n%s", view)
	}
}

func TestSignpostReskin_NoColorKeepsBarAndPosition(t *testing.T) {
	band := renderNoticeBand(bandInfo, byTagSignpostText, testDarkTheme(t).TextOnSelection, 60, testDarkTheme(t), true)

	stripped := ansi.Strip(band)
	if !strings.HasPrefix(stripped, noticeBarGlyph) {
		t.Errorf("NO_COLOR signpost band must keep the far-left %q bar: %q", noticeBarGlyph, stripped)
	}
	if flat := flattenNoticeBand(band); !strings.Contains(flat, byTagSignpostText) {
		t.Errorf("NO_COLOR signpost band must keep the message %q: %q", byTagSignpostText, flat)
	}
	if band != stripped {
		t.Errorf("NO_COLOR signpost band must carry no SGR colour sequences; got raw %q", band)
	}
}
