package tui

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/themetest"
)

func newRecomputePanelModel(t *testing.T, dir string, keys theme.RawKeys) (Model, *countingThemeSource, *fakeThemePersister) {
	t.Helper()

	m, enumerator, _ := newClosePanelModel(t, dir, keys)
	persister := &fakeThemePersister{}
	WithThemePersister(persister)(&m)
	m.termWidth, m.termHeight = arrowTermW, arrowTermH
	m.applySessions(closePanelSessions(), nil)

	m = pressThemeKey(t, m)
	if !m.themePanel.open {
		t.Fatal("fixture: the panel did not open")
	}
	return m, enumerator, persister
}

func requireRowLabels(t *testing.T, m Model, want ...string) {
	t.Helper()

	got := themePanelRowLabels(m)
	if len(got) != len(want) {
		t.Fatalf("the panel lists %v, want %v", got, want)
	}
	for i, label := range want {
		if got[i] != label {
			t.Fatalf("panel row %d is %q, want %q — the full list is %v, want %v", i, got[i], label, got, want)
		}
	}
}

func TestPanelRecompute_RowDisappearsOnConstantCommit(t *testing.T) {
	dir := t.TempDir()
	themetest.Write(t, dir, "sunset.theme", themetest.MonochromeLines("#101010"))
	m, _, persister := newRecomputePanelModel(t, dir, theme.RawKeys{Light: "ghost", Dark: "sunset"})

	requireRowLabels(t, m, "ghost", "nord", "sunset", theme.DefaultDarkSlug, theme.DefaultLightSlug)
	requireCursorOn(t, m, "sunset")

	m, _ = pressCommitKey(t, m)

	requireCommitted(t, persister, "sunset")
	requireConstantKeys(t, m, "sunset")
	requireRowLabels(t, m, "nord", "sunset", theme.DefaultDarkSlug, theme.DefaultLightSlug)
	requireCursorOn(t, m, "sunset")
}

// Calls the commit directly because the key is gated behind the confirm while a
// constant is set.
func commitSlotForTest(t *testing.T, m Model, slug string, member theme.Member) Model {
	t.Helper()

	if err := (&m).commitSlot(slug, member); err != nil {
		t.Fatalf("commitSlot(%s, %v): %v", slug, member, err)
	}
	if m.themeState.keys.Theme != "" {
		t.Fatalf("the slot commit left the constant %q set; it is cleared in the SAME write", m.themeState.keys.Theme)
	}
	return m
}

func TestPanelRecompute_RowAppearsForNewlyLiveSlot(t *testing.T) {
	dir := t.TempDir()
	themetest.Write(t, dir, "sunset.theme", themetest.MonochromeLines("#101010"))
	keys := theme.RawKeys{Theme: "sunset", Light: "ghost", Dark: "sunset"}
	m, _, _ := newRecomputePanelModel(t, dir, keys)

	requireRowLabels(t, m, "nord", "sunset", theme.DefaultDarkSlug, theme.DefaultLightSlug)
	requireCursorOn(t, m, "sunset")

	m = commitSlotForTest(t, m, keys.Dark, theme.MemberDark)

	requireRowLabels(t, m, "ghost", "nord", "sunset", theme.DefaultDarkSlug, theme.DefaultLightSlug)
	ghost := themePanelRowFor(t, m, "ghost")
	if ghost.Row.Selectable() {
		t.Error("the minted `ghost` row is selectable; a slug with no file and no built-in is unselectable with its reason")
	}
	if got := ghost.Row.Rejection.Reason; got != theme.ReasonNotFound {
		t.Errorf("the minted `ghost` row carries reason %q, want %q — the slug resolves to nothing", got, theme.ReasonNotFound)
	}
}

func TestPanelRecompute_ReSortsThroughTheComparator(t *testing.T) {
	dir := t.TempDir()
	themetest.Write(t, dir, "sunset.theme", themetest.MonochromeLines("#101010"))
	keys := theme.RawKeys{Theme: "sunset", Light: "prism", Dark: "sunset"}
	m, _, _ := newRecomputePanelModel(t, dir, keys)

	requireRowLabels(t, m, "nord", "sunset", theme.DefaultDarkSlug, theme.DefaultLightSlug)

	m = commitSlotForTest(t, m, keys.Dark, theme.MemberDark)

	requireRowLabels(t, m, "nord", "prism", "sunset", theme.DefaultDarkSlug, theme.DefaultLightSlug)
}

func renderRecomputePanel(m Model) string {
	return renderThemePanel(m.themePanel, m.contentHeight(), m.themeState.active, m.colourless)
}

// The bare `●` is counted as glyph occurrences minus the slot forms, because
// `● light` and `● dark` both contain the glyph.
func requireBadgeText(t *testing.T, m Model, wantBare, wantLight, wantDark int) {
	t.Helper()

	rendered := ansi.Strip(renderRecomputePanel(m))
	light := strings.Count(rendered, "● light")
	dark := strings.Count(rendered, "● dark")
	bare := strings.Count(rendered, "●") - light - dark
	if bare != wantBare || light != wantLight || dark != wantDark {
		t.Errorf("the panel renders %d bare `●`, %d `● light` and %d `● dark`; want %d/%d/%d\n%s",
			bare, light, dark, wantBare, wantLight, wantDark, rendered)
	}
}

func TestPanelRecompute_VirginInstallBadgeCollapse(t *testing.T) {
	dir := t.TempDir()
	themetest.Write(t, dir, "sunset.theme", themetest.MonochromeLines("#101010"))
	m, _, persister := newRecomputePanelModel(t, dir, theme.RawKeys{})

	requireCursorOn(t, m, theme.DefaultDarkSlug)
	requireBadge(t, m, theme.DefaultDarkSlug, theme.BadgeDark)
	requireBadge(t, m, theme.DefaultLightSlug, theme.BadgeLight)
	requireBadgeText(t, m, 0, 1, 1)

	m, _ = pressCommitKey(t, m)

	requireCommitted(t, persister, theme.DefaultDarkSlug)
	requireConstantKeys(t, m, theme.DefaultDarkSlug)
	requireBadge(t, m, theme.DefaultDarkSlug, theme.BadgeConstant)
	requireBadge(t, m, theme.DefaultLightSlug, theme.BadgeNone)
	requireBadge(t, m, "nord", theme.BadgeNone)
	requireBadge(t, m, "sunset", theme.BadgeNone)
	requireBadgeText(t, m, 1, 0, 0)
}

// prefs.json on disk is seeded to name a slug this instance has never held, so
// a re-read — of the file or of the persister's merged bytes — shows up as a row.
func TestPanelRecompute_ReadsNothing(t *testing.T) {
	t.Run("no directory read", func(t *testing.T) {
		dir := t.TempDir()
		themetest.Write(t, dir, "sunset.theme", themetest.MonochromeLines("#101010"))
		m, enumerator, persister := newRecomputePanelModel(t, dir, theme.RawKeys{Light: "ghost", Dark: "sunset"})
		requireRowLabels(t, m, "ghost", "nord", "sunset", theme.DefaultDarkSlug, theme.DefaultLightSlug)

		if err := os.RemoveAll(dir); err != nil {
			t.Fatalf("remove the themes directory: %v", err)
		}
		m, _ = pressCommitKey(t, m)

		requireCommitted(t, persister, "sunset")
		requireRowLabels(t, m, "nord", "sunset", theme.DefaultDarkSlug, theme.DefaultLightSlug)
		if enumerator.opens != 1 {
			t.Errorf("the commit ran %d enumerations in total, want the single one the open performed — enumeration is pinned to panel OPEN, and a commit changes prefs rather than the directory", enumerator.opens)
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("the themes directory exists again after the commit (err %v); nothing on this path touches it", err)
		}
	})

	t.Run("no prefs read", func(t *testing.T) {
		const onDisk = `{"theme":"sunset","theme_light":"phantom","theme_dark":"sunset"}`
		prefsFile := filepath.Join(t.TempDir(), "prefs.json")
		if err := os.WriteFile(prefsFile, []byte(onDisk), 0o644); err != nil {
			t.Fatalf("write prefs: %v", err)
		}
		t.Setenv("PORTAL_PREFS_FILE", prefsFile)
		dir := t.TempDir()
		themetest.Write(t, dir, "sunset.theme", themetest.MonochromeLines("#101010"))
		keys := theme.RawKeys{Theme: "sunset", Light: "ghost", Dark: "sunset"}
		m, _, _ := newRecomputePanelModel(t, dir, keys)

		m = commitSlotForTest(t, m, keys.Dark, theme.MemberDark)

		requireRowLabels(t, m, "ghost", "nord", "sunset", theme.DefaultDarkSlug, theme.DefaultLightSlug)
		after, err := os.ReadFile(prefsFile)
		if err != nil {
			t.Fatalf("read back prefs: %v", err)
		}
		if string(after) != onDisk {
			t.Errorf("prefs.json =\n%s\nwant it byte-identical:\n%s", after, onDisk)
		}
	})
}

// The frame's colours are measured as a set rather than as bytes: a recompute
// legitimately moves rows, so a byte comparison cannot isolate the palette.
var sgrPattern = regexp.MustCompile("\x1b\\[[0-9;]*m")

func frameColours(frame string) []string {
	seqs := sgrPattern.FindAllString(frame, -1)
	slices.Sort(seqs)
	return slices.Compact(seqs)
}

func TestPanelRecompute_DoesNotApplyTheme(t *testing.T) {
	dir := t.TempDir()
	themetest.Write(t, dir, "aurora.theme", themetest.MonochromeLines("#101010"))
	themetest.Write(t, dir, "sunset.theme", themetest.MonochromeLines("#202020"))
	m, _, persister := newRecomputePanelModel(t, dir, theme.RawKeys{Light: "ghost", Dark: "aurora"})
	requireCursorOn(t, m, "aurora")

	m = arrowToThemeRow(t, m, "sunset")
	previewed := m.themeState.active
	if got := previewed.Canvas.Value; got != "#202020" {
		t.Fatalf("fixture: arrowing to `sunset` rendered canvas %s, want #202020 — the preview is what the commit must leave alone", got)
	}
	colours := frameColours(m.View().Content)

	m, _ = pressCommitKey(t, m)

	requireCommitted(t, persister, "sunset")
	requireRowLabels(t, m, "aurora", "nord", "sunset", theme.DefaultDarkSlug, theme.DefaultLightSlug)
	if m.themeState.active != previewed {
		t.Errorf("the recompute rendered canvas %s, want the previewed %s left alone — the re-resolution is for the badges, never for selecting a new active member", m.themeState.active.Canvas.Value, previewed.Canvas.Value)
	}
	if got := frameColours(m.View().Content); !slices.Equal(got, colours) {
		t.Errorf("the recompute changed the frame's colours\nbefore: %v\nafter:  %v", colours, got)
	}
}

func TestPanelRecompute_CursorAnchoredByIdentity(t *testing.T) {
	dir := t.TempDir()
	themetest.Write(t, dir, "sunset.theme", themetest.MonochromeLines("#101010"))
	keys := theme.RawKeys{Theme: "sunset", Light: "ghost", Dark: "sunset"}
	m, _, _ := newRecomputePanelModel(t, dir, keys)

	requireCursorOn(t, m, "sunset")
	previewed := m.themeState.active
	before := m.themePanel.list.Index()
	if before != 1 {
		t.Fatalf("fixture: the cursor opened at index %d, want 1 — the inserted row must land above it", before)
	}

	m = commitSlotForTest(t, m, keys.Dark, theme.MemberDark)

	requireRowLabels(t, m, "ghost", "nord", "sunset", theme.DefaultDarkSlug, theme.DefaultLightSlug)
	requireCursorOn(t, m, "sunset")
	if got := m.themePanel.list.Index(); got != before+1 {
		t.Errorf("the cursor sits at index %d, want %d — the inserted row pushed `sunset` down and the anchor followed the IDENTITY", got, before+1)
	}
	if m.themeState.active != previewed {
		t.Errorf("the recompute rendered canvas %s, want the previewed %s — the cursor's row is always what is painted behind the panel", m.themeState.active.Canvas.Value, previewed.Canvas.Value)
	}
}

func TestPanelRecompute_NoChangeCommitIsStable(t *testing.T) {
	dir := t.TempDir()
	themetest.Write(t, dir, "sunset.theme", themetest.MonochromeLines("#101010"))
	m, _, persister := newRecomputePanelModel(t, dir, theme.RawKeys{Theme: "sunset"})

	requireCursorOn(t, m, "sunset")
	rows := themePanelRowLabels(m)
	badges := maps.Clone(m.themePanel.badges)
	index := m.themePanel.list.Index()
	frame := m.View().Content

	m, _ = pressCommitKey(t, m)

	requireCommitted(t, persister, "sunset")
	requireConstantKeys(t, m, "sunset")
	requireRowLabels(t, m, rows...)
	if got := m.themePanel.badges; !maps.Equal(got, badges) {
		t.Errorf("the badge map is %v after an unchanged commit, want the identical %v", got, badges)
	}
	if got := m.themePanel.list.Index(); got != index {
		t.Errorf("the cursor sits at index %d after an unchanged commit, want the identical %d", got, index)
	}
	if got := m.View().Content; got != frame {
		t.Errorf("an unchanged commit changed the composed frame\nbefore: %q\nafter:  %q", escSeq(frame), escSeq(got))
	}
}

var errThemeResolveFatal = errors.New("theme: the embedded set cannot supply a fallback")

func themeRowsUnion(rows []theme.Row) theme.Union {
	return themeRowsUnionDirUnusable(rows, false)
}

func themeRowsUnionDirUnusable(rows []theme.Row, dirUnusable bool) theme.Union {
	return theme.Union{Rows: rows, DirUnusable: dirUnusable, Count: len(rows), Rejected: arrowRejectedCount(rows)}
}

func TestThemeRowsUnion_DerivesTalliesFromRows(t *testing.T) {
	rejected := func(slug string) theme.Row {
		return theme.Row{Slug: slug, Source: theme.SourceFile, Filename: slug + ".theme", Rejection: &theme.Rejection{Reason: theme.ReasonBadColour}}
	}
	valid := func(slug string) theme.Row {
		return theme.Row{Slug: slug, Source: theme.SourceBuiltin}
	}

	for _, tc := range []struct {
		name         string
		rows         []theme.Row
		wantCount    int
		wantRejected int
	}{
		{name: "no rows at all"},
		{name: "every row selectable", rows: []theme.Row{valid("a"), valid("b")}, wantCount: 2},
		{name: "one rejected row", rows: []theme.Row{valid("a"), rejected("b")}, wantCount: 2, wantRejected: 1},
		{name: "several rejected rows", rows: []theme.Row{rejected("a"), valid("b"), rejected("c"), rejected("d")}, wantCount: 4, wantRejected: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for name, got := range map[string]theme.Union{
				"themeRowsUnion":            themeRowsUnion(tc.rows),
				"themeRowsUnionDirUnusable": themeRowsUnionDirUnusable(tc.rows, false),
			} {
				if got.Count != tc.wantCount {
					t.Errorf("%s reports Count %d over %d rows, want %d", name, got.Count, len(tc.rows), tc.wantCount)
				}
				if got.Rejected != tc.wantRejected {
					t.Errorf("%s reports Rejected %d, want %d — the count of its own unselectable rows", name, got.Rejected, tc.wantRejected)
				}
				if got.DirUnusable {
					t.Errorf("%s marked the directory unusable unasked", name)
				}
			}
			if got := themeRowsUnionDirUnusable(tc.rows, true); !got.DirUnusable {
				t.Error("themeRowsUnionDirUnusable(rows, true) left the directory usable")
			}
		})
	}
}

// The two unions differ deliberately: what the panel lists after a commit came
// from the reassembly, what it lists without one came from the open.
func newSplitPanelModel(t *testing.T, opened, reassembled []theme.Row, cursorSlug string) (Model, *fakeThemeSource, *fakeThemePersister) {
	t.Helper()

	target := arrowRowBySlug(t, opened, cursorSlug)
	reassembly := themeRowsUnion(reassembled)
	enumerator := &fakeThemeSource{
		enumeration: theme.Enumeration{DirPath: fixtureThemesDir},
		union:       themeRowsUnion(opened),
		reassembled: &reassembly,
		resolution:  constantResolution(cursorSlug, target.Theme),
	}
	persister := &fakeThemePersister{}
	deps := stubPanelDeps(enumerator, theme.ConstantNomination(target.Theme), theme.RawKeys{Theme: cursorSlug})
	deps.ThemePersister = persister
	return openCommitPanel(t, deps, PageSessions, cursorSlug), enumerator, persister
}

// The clamp lands on the first SELECTABLE row, which is why the reassembly
// leads with an unselectable one.
func TestPanelRecompute_CursorClampsOnMissingIdentity(t *testing.T) {
	opened := arrowValidRows(t, 4)
	reassembled := []theme.Row{
		arrowInvalidRow(arrowSlug(0)),
		arrowValidRow(t, arrowSlug(1), 1),
		arrowValidRow(t, arrowSlug(3), 3),
	}
	m, _, persister := newSplitPanelModel(t, opened, reassembled, arrowSlug(2))

	m, _ = pressCommitKey(t, m)

	requireCommitted(t, persister, arrowSlug(2))
	requireRowLabels(t, m, arrowSlug(0), arrowSlug(1), arrowSlug(3))
	requireCursorOn(t, m, arrowSlug(1))
	if row := themePanelCursorRow(t, m); !row.Selectable() {
		t.Errorf("the clamp landed the cursor on the unselectable %q; the invariant is that the cursor is always on a SELECTABLE row", row.Label())
	}
}

// The reassembly's order is deliberately not alphabetical, so a caller-side
// sort would show up.
func TestPanelRecompute_ResolveErrorKeepsBadges(t *testing.T) {
	opened := arrowValidRows(t, 4)
	reassembled := []theme.Row{
		arrowValidRow(t, arrowSlug(1), 1),
		arrowValidRow(t, arrowSlug(0), 0),
		arrowValidRow(t, arrowSlug(3), 3),
	}
	m, enumerator, persister := newSplitPanelModel(t, opened, reassembled, arrowSlug(0))
	badges := maps.Clone(m.themePanel.badges)
	if badges[arrowSlug(0)] != theme.BadgeConstant {
		t.Fatalf("fixture: the open left badges %v, want a constant `●` on %q", badges, arrowSlug(0))
	}
	// A ZERO Resolution alongside the fatal, as the loader answers: a populated
	// one would make the assertion below vacuous.
	enumerator.resolution = theme.Resolution{}
	enumerator.err = errThemeResolveFatal

	m, _ = pressCommitKey(t, m)

	requireCommitted(t, persister, arrowSlug(0))
	if got := m.themePanel.badges; !maps.Equal(got, badges) {
		t.Errorf("the failed re-resolution left badges %v, want the existing %v — an empty map would wipe every `●` off the panel", got, badges)
	}
	requireBadge(t, m, arrowSlug(0), theme.BadgeConstant)
	requireRowLabels(t, m, arrowSlug(1), arrowSlug(0), arrowSlug(3))
	requireCursorOn(t, m, arrowSlug(0))
}

func TestPanelRecompute_SkippedOnFailedCommit(t *testing.T) {
	opened := arrowValidRows(t, 4)
	reassembled := arrowValidRows(t, 2)

	m, enumerator, persister := newSplitPanelModel(t, opened, reassembled, arrowSlug(0))
	badges := maps.Clone(m.themePanel.badges)
	persister.err = errThemeCommitFailed

	m, _ = pressCommitKey(t, m)

	requireCommitted(t, persister, arrowSlug(0))
	requireRowLabels(t, m, arrowSlug(0), arrowSlug(1), arrowSlug(2), arrowSlug(3))
	if got := m.themePanel.badges; !maps.Equal(got, badges) {
		t.Errorf("the failed commit left badges %v, want the untouched %v — a failed commit does not move the `●`", got, badges)
	}
	if enumerator.reassembles != 0 {
		t.Errorf("the failed commit ran %d reassemblies, want 0 — only a SUCCESSFUL commit recomputes", enumerator.reassembles)
	}

	wired, control, _ := newSplitPanelModel(t, opened, reassembled, arrowSlug(0))
	wired, _ = pressCommitKey(t, wired)
	requireRowLabels(t, wired, arrowSlug(0), arrowSlug(1))
	if control.reassembles != 1 {
		t.Fatalf("positive control: a successful commit ran %d reassemblies, want 1", control.reassembles)
	}
}

// `bubbles/list` snapshots its dot strings at construction, so a rebuilt list
// prints the library greys. Title is the instance probe: production never
// touches it, so a value there survives an item swap but not a fresh list.Model.
func TestPanelRecompute_ItemsReplacedNotRebuilt(t *testing.T) {
	const (
		sentinel   = "recompute-instance-probe"
		dotsHeight = 16
	)
	opened := arrowValidRows(t, 20)
	m, _, persister := newSplitPanelModel(t, opened, opened[:19], arrowSlug(0))
	m.themePanel.list.Title = sentinel
	width, height := m.themePanel.list.Width(), m.themePanel.list.Height()
	if width <= 0 || height <= 0 {
		t.Fatalf("fixture: the panel list is sized %dx%d, so an unchanged size says nothing", width, height)
	}

	m, _ = pressCommitKey(t, m)

	requireCommitted(t, persister, arrowSlug(0))
	if got := len(m.themePanel.list.Items()); got != 19 {
		t.Fatalf("the panel lists %d rows after the recompute, want the reassembly's 19", got)
	}
	if got := m.themePanel.list.Title; got != sentinel {
		t.Errorf("the panel list's Title is %q, want the sentinel %q — the recompute replaced the list MODEL rather than its items", got, sentinel)
	}
	if w, h := m.themePanel.list.Width(), m.themePanel.list.Height(); w != width || h != height {
		t.Errorf("the panel list is sized %dx%d after the recompute, want the unchanged %dx%d", w, h, width, height)
	}
	if got := m.themePanel.list.KeyMap.CursorUp.Keys(); len(got) == 0 {
		t.Error("the panel list's keymap binds nothing after the recompute; the list was replaced by a fresh model")
	}

	dots := themePanelDotRow(t, renderThemePanel(m.themePanel, dotsHeight, m.themeState.active, m.colourless))
	for _, tc := range []struct {
		what  string
		token theme.Token
	}{
		{what: "active dot", token: m.themeState.active.AccentPrimary},
		{what: "inactive dot", token: m.themeState.active.TextFaint},
	} {
		if want := tokenFgSeq(t, tc.token); !strings.Contains(dots, want) {
			t.Errorf("the rendered %s does not carry the previewed theme's SGR %q — a rebuilt list prints `bubbles/list`'s own greys: %q", tc.what, want, escSeq(dots))
		}
	}
}
