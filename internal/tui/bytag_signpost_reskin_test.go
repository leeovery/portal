package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui/theme"
)

// Tests for task 4-3: the "No tags yet" signpost reskin. The By-Tag zero-tags
// signpost now routes through the §11 single-slot notice-band primitive as a
// persistent violet INFO band — an accent.violet `▌` left-bar + the message in
// text.on-selection on the SAME bg.selection tint as the §11.4 command-pending
// banner (the two are one info-message element) — owning the slot while m.byTagSignpost
// holds, yielding it to a transient flash (§11.2) for the flash's duration, then
// returning. The gate (By-Tag + zero tags anywhere), the flat-items arm, and the
// §5.4 zero-pane-reads invariant are preserved byte-for-byte.

// signpostModel builds a Sessions-page model in By-Tag mode with zero-tag
// projects (so the signpost shows over the flat list) at 80x24 so the rendered
// view carries predictable substrings.
func signpostModel(t *testing.T) Model {
	t.Helper()
	dir := t.TempDir()
	projects := []project.Project{{Path: dir, Name: "Portal"}}
	sessions := []tmux.Session{
		{Name: "portal-abc", Dir: dir},
		{Name: "portal-def", Dir: dir},
	}
	m := newRebuildTestModel(prefs.ModeByTag, sessions, projects)
	m.termWidth = 80
	m.termHeight = 24
	m.rebuildSessionList()
	if !m.byTagSignpost {
		t.Fatalf("setup invariant: byTagSignpost = false, want true (zero tags anywhere in By Tag mode)")
	}
	return m
}

// TestSignpostReskin_VioletInfoBand asserts the signpost renders as the §11.3
// persistent INFO band: an accent.violet `▌` left-bar + the message in
// text.on-selection, on the bg.selection info-band tint (the SAME tint as the §11.4
// command-pending banner — NOT the bg.warning flash tint).
func TestSignpostReskin_VioletInfoBand(t *testing.T) {
	m := signpostModel(t)

	band := m.renderActiveNoticeBand()
	if band == "" {
		t.Fatal("renderActiveNoticeBand returned empty; the signpost band must own the slot")
	}

	// Far-left ▌ bar.
	stripped := ansi.Strip(band)
	if !strings.HasPrefix(stripped, noticeBarGlyph) {
		t.Errorf("signpost band does not start with the %q left-bar: %q", noticeBarGlyph, stripped)
	}
	// Spec-exact message text present (wrap-agnostic: the message may span multiple
	// lines at the signpost width, so reconstruct it before matching).
	if flat := flattenNoticeBand(band); !strings.Contains(flat, byTagSignpostText) {
		t.Errorf("signpost band missing the message %q: %q", byTagSignpostText, flat)
	}
	// Bar colour = accent.violet (§2.9).
	violetSeq := tokenFgSeq(t, theme.MV.AccentViolet, m.canvasMode)
	if !strings.Contains(band, violetSeq) {
		t.Errorf("signpost band missing the accent.violet bar foreground sequence %q:\n%s", violetSeq, band)
	}
	// Message colour = text.on-selection (§2.9): the bright white co-tuned for the
	// bg.selection tint the info band sits on (the same token the selected
	// session-row name uses on that surface).
	onSelectionSeq := tokenFgSeq(t, theme.MV.TextOnSelection, m.canvasMode)
	if !strings.Contains(band, onSelectionSeq) {
		t.Errorf("signpost band missing the text.on-selection message foreground sequence %q:\n%s", onSelectionSeq, band)
	}
	// Tint = bg.selection (§2.9): the info band sits on the SAME subtle tint as the
	// §11.4 command-pending banner — it must NOT regress to a flat/Canvas band.
	selectionBgSeq := tokenBgSeq(t, theme.MV.BgSelection, m.canvasMode)
	if !strings.Contains(band, selectionBgSeq) {
		t.Errorf("signpost band missing the bg.selection info-band tint %q (must not be flat):\n%s", selectionBgSeq, band)
	}
	// NOT the bg.warning flash tint (§11.3 info band is NOT a flash): the bg.warning
	// background colour sequence must be ABSENT.
	warnBgSeq := tokenBgSeq(t, theme.MV.BgWarning, m.canvasMode)
	if strings.Contains(band, warnBgSeq) {
		t.Errorf("signpost band carries the bg.warning flash tint %q (info band is not a flash):\n%s", warnBgSeq, band)
	}
}

// TestInfoBands_ShareSameTint is the consolidation regression guard: the §11.3
// no-tags signpost (bandInfo) and the §11.4 command-pending banner (bandCommand)
// are one info-message element, so they MUST resolve the SAME tint token —
// bg.selection. If either drifts (e.g. bandInfo regresses to Canvas, or a future
// edit retints the command band) this fails before the visual divergence ships.
func TestInfoBands_ShareSameTint(t *testing.T) {
	if got := bandInfo.tintToken().Name; got != theme.MV.BgSelection.Name {
		t.Errorf("bandInfo tint token = %q, want bg.selection (shared info-band tint)", got)
	}
	if got := bandCommand.tintToken().Name; got != theme.MV.BgSelection.Name {
		t.Errorf("bandCommand tint token = %q, want bg.selection (shared info-band tint)", got)
	}
	if bandInfo.tintToken().Name != bandCommand.tintToken().Name {
		t.Errorf("info bands diverge: bandInfo tint %q != bandCommand tint %q (must share one info-band tint)",
			bandInfo.tintToken().Name, bandCommand.tintToken().Name)
	}
}

// TestSignpostReskin_SpecExactWording pins the signpost wording to the spec-exact
// string, sourced as a single constant (byTagSignpostText). The constant is the
// single source of truth — the View, the band, and this assertion all read it.
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

// TestSignpostReskin_OnlyByTagZeroTags asserts the signpost band owns the slot
// ONLY in By-Tag mode with zero tags anywhere — Flat, By-Project, and By-Tag with
// a tag present never show it.
func TestSignpostReskin_OnlyByTagZeroTags(t *testing.T) {
	dir := t.TempDir()
	sessions := []tmux.Session{{Name: "portal-abc", Dir: dir}}

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
			m := newRebuildTestModel(c.mode, sessions, c.projects)
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

// TestSignpostReskin_ZeroPaneReads asserts the §5.4 invariant: the signpost path
// performs ZERO pane reads — resolveSessionDirs (the lazy pane-read fallback) is
// never invoked on the signpost/flat arm. fakeStamper.ActivePaneCurrentPath is the
// per-session resolution counter; an empty-Dir session would cost one read if the
// path resolved.
func TestSignpostReskin_ZeroPaneReads(t *testing.T) {
	dir := t.TempDir()
	projects := []project.Project{{Path: dir, Name: "Portal"}}
	sessions := []tmux.Session{
		{Name: "portal-a", Dir: ""},
		{Name: "portal-b", Dir: ""},
	}

	stamper := &fakeStamper{path: dir}
	m := newRebuildTestModel(prefs.ModeByTag, sessions, projects)
	m.termWidth = 80
	m.termHeight = 24
	m.dirReader = stamper
	m.dirRunner = &fakeDirRunner{gitRoot: dir}

	m.rebuildSessionList()
	// Render the full view too — the band render path must not read panes either.
	_ = m.View().Content

	if !m.byTagSignpost {
		t.Fatalf("setup invariant: byTagSignpost = false, want true (zero tags anywhere)")
	}
	if len(stamper.reads) != 0 {
		t.Errorf("signpost path performed %d pane reads (reads=%v), want 0 (§5.4)", len(stamper.reads), stamper.reads)
	}
	if len(stamper.setCalls) != 0 {
		t.Errorf("signpost path performed %d stamp writes, want 0", len(stamper.setCalls))
	}
}

// TestSignpostReskin_GroupingMachineryUntouched asserts the reskin left the
// anyTagsExist gate and the ToListItems flat-items arm intact: the signposted list
// is byte-for-byte the flat slice (no group metadata, no headings).
func TestSignpostReskin_GroupingMachineryUntouched(t *testing.T) {
	dir := t.TempDir()
	projects := []project.Project{{Path: dir, Name: "Portal"}}
	sessions := []tmux.Session{
		{Name: "portal-abc", Dir: dir},
		{Name: "portal-def", Dir: dir},
	}

	m := newRebuildTestModel(prefs.ModeByTag, sessions, projects)
	m.rebuildSessionList()

	// The gate fired (zero tags anywhere).
	if anyTagsExist(projects) {
		t.Fatalf("test invariant: anyTagsExist = true, want false (no project carries a tag)")
	}
	// The flat-items arm built the list: byte-for-byte ToListItems(sessions).
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

// TestSignpostReskin_YieldsToFlashThenReturns asserts the §11 single-slot arbiter
// behaviour for the signpost: a transient flash takes the slot for its duration,
// hiding the signpost, then the signpost returns once the flash clears.
func TestSignpostReskin_YieldsToFlashThenReturns(t *testing.T) {
	m := signpostModel(t)

	// Signpost owns the slot initially.
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

// TestSignpostReskin_NoColorKeepsBarAndPosition asserts the NO_COLOR carve-out
// (§2.5): under colourless the signpost band drops the tint + bar colour but keeps
// the `▌` bar, its far-left position, and the message text — and carries no SGR
// colour sequences at all.
func TestSignpostReskin_NoColorKeepsBarAndPosition(t *testing.T) {
	band := renderNoticeBand(bandInfo, byTagSignpostText, theme.MV.TextOnSelection, 60, theme.Dark, true)

	stripped := ansi.Strip(band)
	if !strings.HasPrefix(stripped, noticeBarGlyph) {
		t.Errorf("NO_COLOR signpost band must keep the far-left %q bar: %q", noticeBarGlyph, stripped)
	}
	// Message survives (wrap-agnostic: reconstruct across wrapped lines before matching).
	if flat := flattenNoticeBand(band); !strings.Contains(flat, byTagSignpostText) {
		t.Errorf("NO_COLOR signpost band must keep the message %q: %q", byTagSignpostText, flat)
	}
	if band != stripped {
		t.Errorf("NO_COLOR signpost band must carry no SGR colour sequences; got raw %q", band)
	}
}
