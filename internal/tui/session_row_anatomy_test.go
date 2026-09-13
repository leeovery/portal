package tui

import (
	"bytes"
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/tmux"
)

func renderRow(d SessionDelegate, width int, items []list.Item, index, selIndex int) string {
	m := list.New(items, d, width, 10)
	m.Select(selIndex)
	var buf bytes.Buffer
	d.Render(&buf, m, index, items[index])
	return buf.String()
}

func visibleColOf(line, sub string) int {
	stripped := ansi.Strip(line)
	before, _, ok := strings.Cut(stripped, sub)
	if !ok {
		return -1
	}
	return ansi.StringWidth(before)
}

func selectionBgParams(t *testing.T, th theme.Theme) string {
	t.Helper()
	return sgrParams(t, lipgloss.NewStyle().Background(th.BgSelection.Color()))
}

func flatItems(specs ...tmux.Session) []list.Item {
	items := make([]list.Item, len(specs))
	for i, s := range specs {
		items[i] = SessionItem{Session: s}
	}
	return items
}

func TestSessionRow_FlexNameFixedTrailingSlots(t *testing.T) {
	const w = 80
	items := flatItems(tmux.Session{Name: "alpha", Windows: 3, Attached: true})
	out := renderRow(SessionDelegate{}, w, items, 0, 0)
	vis := ansi.Strip(out)

	if !strings.Contains(vis, "alpha") {
		t.Errorf("row missing name 'alpha': %q", vis)
	}
	if !strings.Contains(vis, "3 windows") {
		t.Errorf("row missing window count '3 windows': %q", vis)
	}
	if !strings.Contains(vis, "● attached") {
		t.Errorf("row missing attached marker '● attached': %q", vis)
	}
	nameCol := visibleColOf(out, "alpha")
	countCol := visibleColOf(out, "3 windows")
	if countCol <= nameCol {
		t.Errorf("count slot (col %d) must sit right of the name (col %d): %q", countCol, nameCol, vis)
	}
	if got := lipgloss.Width(out); got != w {
		t.Errorf("row width = %d, want exactly %d (trailing slots right-pinned to the list width)", got, w)
	}
}

func TestSessionRow_ColumnAlignsRegardlessOfNameLength(t *testing.T) {
	const w = 80
	items := flatItems(
		tmux.Session{Name: "a", Windows: 1, Attached: true},
		tmux.Session{Name: "a-much-longer-session-name-here", Windows: 5, Attached: true},
	)
	short := renderRow(SessionDelegate{}, w, items, 0, 0)
	long := renderRow(SessionDelegate{}, w, items, 1, 0)

	shortCount := visibleColOf(short, "window")
	longCount := visibleColOf(long, "window")
	if shortCount < 0 || longCount < 0 {
		t.Fatalf("a count column is missing: short=%q long=%q", ansi.Strip(short), ansi.Strip(long))
	}
	if shortCount != longCount {
		t.Errorf("window counts not column-aligned: short name count col %d, long name count col %d", shortCount, longCount)
	}

	shortBullet := visibleColOf(short, "●")
	longBullet := visibleColOf(long, "●")
	if shortBullet < 0 || longBullet < 0 {
		t.Fatalf("an attached bullet is missing: short=%q long=%q", ansi.Strip(short), ansi.Strip(long))
	}
	if shortBullet != longBullet {
		t.Errorf("attached bullets not column-aligned: short col %d, long col %d", shortBullet, longBullet)
	}
}

func TestSessionRow_EmptyAttachedSlotPreservesAlignment(t *testing.T) {
	const w = 80
	items := flatItems(
		tmux.Session{Name: "attached-one", Windows: 2, Attached: true},
		tmux.Session{Name: "detached-one", Windows: 2, Attached: false},
	)
	attached := renderRow(SessionDelegate{}, w, items, 0, 0)
	detached := renderRow(SessionDelegate{}, w, items, 1, 0)

	if strings.Contains(ansi.Strip(detached), "attached") {
		t.Errorf("unattached row must not render the attached marker: %q", ansi.Strip(detached))
	}
	if a, d := visibleColOf(attached, "window"), visibleColOf(detached, "window"); a != d {
		t.Errorf("count columns misaligned across attached/unattached: %d vs %d", a, d)
	}
	if a, d := lipgloss.Width(attached), lipgloss.Width(detached); a != d {
		t.Errorf("row widths differ across attached/unattached: %d vs %d (empty slot must match marker width)", a, d)
	}
}

func TestSessionRow_SelectedShowsVioletBarTintAndOnSelectionName(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		d := SessionDelegate{Theme: th}
		items := flatItems(tmux.Session{Name: "selected-row", Windows: 2, Attached: false})
		out := renderRow(d, 80, items, 0, 0)

		if !strings.Contains(ansi.Strip(out), "▌") {
			t.Errorf("[%v] selected row missing the ▌ selector bar: %q", themeLabel(th), ansi.Strip(out))
		}
		if seq := tokenFgSeq(t, th.AccentPrimary); !strings.Contains(out, seq) {
			t.Errorf("[%v] selected bar missing accent.primary fg %q", themeLabel(th), seq)
		}
		if params := selectionBgParams(t, th); !lineHasBgParams(out, params) {
			t.Errorf("[%v] selected row missing the bg.selection tint %q: %q", themeLabel(th), params, escSeq(out))
		}
		if seq := tokenFgSeq(t, th.TextOnSelection); !strings.Contains(out, seq) {
			t.Errorf("[%v] selected name missing text.on-selection fg %q", themeLabel(th), seq)
		}
	}
}

func TestSessionRow_UnselectedHasNoBarOrTint(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		d := SessionDelegate{Theme: th}
		items := flatItems(
			tmux.Session{Name: "row-zero", Windows: 1, Attached: false},
			tmux.Session{Name: "row-one", Windows: 1, Attached: false},
		)
		out := renderRow(d, 80, items, 1, 0)

		if strings.Contains(ansi.Strip(out), "▌") {
			t.Errorf("[%v] unselected row must not carry the ▌ bar: %q", themeLabel(th), ansi.Strip(out))
		}
		if params := selectionBgParams(t, th); lineHasBgParams(out, params) {
			t.Errorf("[%v] unselected row must not carry the bg.selection tint %q: %q", themeLabel(th), params, escSeq(out))
		}
		if params := wantCanvasBgParams(t, th); !lineHasBgParams(out, params) {
			t.Errorf("[%v] unselected row missing the canvas paint %q: %q", themeLabel(th), params, escSeq(out))
		}
	}
}

func TestSessionRow_AttachedKeepsStateGreenWhenSelected(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		d := SessionDelegate{Theme: th}
		items := flatItems(
			tmux.Session{Name: "attached-selected", Windows: 1, Attached: true},
			tmux.Session{Name: "attached-unselected", Windows: 1, Attached: true},
		)

		green := tokenFgSeq(t, th.StatePositive)
		onSelName := tokenFgSeq(t, th.TextOnSelection)

		sel := renderRow(d, 80, items, 0, 0)
		if !strings.Contains(sel, green) {
			t.Errorf("[%v] selected attached marker missing state.positive fg %q", themeLabel(th), green)
		}
		if th == testLightTheme(t) && green == onSelName {
			t.Fatalf("[light] test precondition broken: state.positive == text.on-selection")
		}

		uns := renderRow(d, 80, items, 1, 0)
		if !strings.Contains(uns, green) {
			t.Errorf("[%v] unselected attached marker missing state.positive fg %q", themeLabel(th), green)
		}
	}
}

func TestSessionRow_SelectedCountInTextStrong(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		d := SessionDelegate{Theme: th}
		items := flatItems(
			tmux.Session{Name: "row-zero", Windows: 4, Attached: false},
			tmux.Session{Name: "row-one", Windows: 4, Attached: false},
		)

		sel := renderRow(d, 80, items, 0, 0)
		uns := renderRow(d, 80, items, 1, 0)

		strong := tokenFgSeq(t, th.TextSecondary)
		detail := tokenFgSeq(t, th.TextMuted)

		if !strings.Contains(sel, strong) {
			t.Errorf("[%v] selected-row count missing text.secondary fg %q", themeLabel(th), strong)
		}
		if !strings.Contains(uns, detail) {
			t.Errorf("[%v] unselected-row count missing text.muted fg %q", themeLabel(th), detail)
		}
	}
}

func TestSessionRow_OverLongNameTruncatesWithoutPushingSlots(t *testing.T) {
	const w = 40
	longName := "this-is-a-really-very-long-session-name-that-overflows"
	items := flatItems(tmux.Session{Name: longName, Windows: 7, Attached: true})
	out := renderRow(SessionDelegate{}, w, items, 0, 0)
	vis := ansi.Strip(out)

	if strings.Contains(vis, longName) {
		t.Errorf("over-long name should be truncated, but the full name rendered: %q", vis)
	}
	if !strings.Contains(vis, "…") {
		t.Errorf("truncated name should carry the ellipsis glyph: %q", vis)
	}
	if !strings.Contains(vis, "7 windows") {
		t.Errorf("window-count slot pushed off-row by the long name: %q", vis)
	}
	if !strings.Contains(vis, "● attached") {
		t.Errorf("attached slot pushed off-row by the long name: %q", vis)
	}
	if got := lipgloss.Width(out); got != w {
		t.Errorf("truncated row width = %d, want exactly %d (no overflow, slots right-pinned)", got, w)
	}
}

func TestSessionRow_NeverOverflowsAtNarrowWidths(t *testing.T) {
	for _, w := range []int{1, 5, 10, 20, 25, 26, 29, 40, 80} {
		for _, sess := range []tmux.Session{
			{Name: "x", Windows: 1, Attached: false},
			{Name: "agentic-workflows-code-based-that-is-quite-long", Windows: 12, Attached: true},
		} {
			items := flatItems(sess)
			for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
				out := renderRow(SessionDelegate{Theme: th}, w, items, 0, 0)
				if got := lipgloss.Width(out); got > w {
					t.Errorf("[w=%d %v %q] row width = %d, overflows the list width %d", w, themeLabel(th), sess.Name, got, w)
				}
			}
		}
	}
}

func TestSessionRow_FlatIsNameOnly(t *testing.T) {
	items := flatItems(tmux.Session{
		Name:     "flat-name",
		Windows:  2,
		Attached: false,
		Dir:      "/home/user/code/some-project",
	})
	out := renderRow(SessionDelegate{}, 80, items, 0, 0)
	vis := ansi.Strip(out)

	if !strings.Contains(vis, "flat-name") {
		t.Errorf("flat row missing the name: %q", vis)
	}
	if strings.Contains(vis, "/home/user") || strings.Contains(vis, "some-project") {
		t.Errorf("flat row leaked the directory/path column: %q", vis)
	}
}

func TestSessionRow_NoRawAnsiColourLiterals(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		d := SessionDelegate{Theme: th}
		items := flatItems(tmux.Session{Name: "alpha", Windows: 3, Attached: true})
		out := renderRow(d, 80, items, 0, 0)

		for _, banned := range []string{"38;5;212", "38;5;76", "48;5;212", "48;5;76"} {
			if strings.Contains(out, banned) {
				t.Errorf("[%v] delegate emitted a legacy ANSI-256 colour sequence %q: %q", themeLabel(th), banned, escSeq(out))
			}
		}
		if strings.Contains(out, "38;2;119;119;119") {
			t.Errorf("[%v] delegate emitted the legacy #777777 grey: %q", themeLabel(th), escSeq(out))
		}
	}
}

func TestSessionRow_HeightStaysOne(t *testing.T) {
	d := SessionDelegate{}
	if d.Height() != 1 {
		t.Fatalf("Height() = %d, want 1", d.Height())
	}
	items := flatItems(tmux.Session{Name: "alpha", Windows: 3, Attached: true})
	out := renderRow(d, 80, items, 0, 0)
	if strings.Contains(out, "\n") {
		t.Errorf("session row emitted more than one line: %q", out)
	}
}

func lineHasBgParams(line, params string) bool {
	for _, c := range scanCellBackgrounds(line) {
		if c.set && c.params == params {
			return true
		}
	}
	return false
}

func escSeq(s string) string { return strings.ReplaceAll(s, "\x1b", "\\e") }

func TestSessionRow_DirColumnSitsOneSpaceAfterTheName(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	for _, name := range []string{"a", "alpha-one", "a-considerably-longer-session-name"} {
		items := flatItems(tmux.Session{Name: name, Windows: 2, Dir: home + "/Code/portal"})
		out := renderRow(SessionDelegate{ShowDir: true}, 80, items, 0, 0)

		nameCol := visibleColOf(out, name)
		dirCol := visibleColOf(out, "~/Code/portal")
		if nameCol < 0 || dirCol < 0 {
			t.Fatalf("[%s] row missing the name or the directory: %q", name, ansi.Strip(out))
		}
		if want := nameCol + ansi.StringWidth(name) + 1; dirCol != want {
			t.Errorf("[%s] directory at col %d, want %d (one space after the name): %q", name, dirCol, want, ansi.Strip(out))
		}
	}
}

func TestSessionRow_DirColumnOffRendersTodaysRow(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		d := SessionDelegate{Theme: th}
		withDir := renderRow(d, 80, flatItems(tmux.Session{Name: "alpha", Windows: 2, Dir: home + "/Code/portal"}), 0, 0)
		without := renderRow(d, 80, flatItems(tmux.Session{Name: "alpha", Windows: 2}), 0, 0)

		if withDir != without {
			t.Errorf("[%v] with the column off a recorded directory changed the row:\n got %q\nwant %q", themeLabel(th), escSeq(withDir), escSeq(without))
		}
	}
}

func TestSessionRow_DirColumnRendersNoSeparatorWithoutARecordedDirectory(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		items := flatItems(tmux.Session{Name: "alpha", Windows: 2})
		on := renderRow(SessionDelegate{Theme: th, ShowDir: true}, 80, items, 0, 0)
		off := renderRow(SessionDelegate{Theme: th}, 80, items, 0, 0)

		if on != off {
			t.Errorf("[%v] a session with no recorded directory rendered a separator:\n got %q\nwant %q", themeLabel(th), escSeq(on), escSeq(off))
		}
	}
}

func TestSessionRow_DirColumnKeepsTheRowExactlyTheListWidth(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	tests := []struct {
		name  string
		dir   string
		width int
	}{
		{"whole directory", home + "/Code/portal", 80},
		{"no recorded directory", "", 80},
		{"left-truncated directory", home + "/Code/portal/internal/tui", 50},
		{"directory dropped below the floor", home + "/Code/portal/internal/tui", 34},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			items := flatItems(tmux.Session{Name: "alpha", Windows: 2, Attached: true, Dir: tc.dir})
			for _, sel := range []int{0, 1} {
				for _, colourless := range []bool{false, true} {
					d := SessionDelegate{Theme: testDarkTheme(t), ShowDir: true, Colourless: colourless}
					out := renderRow(d, tc.width, items, 0, sel)
					if got := lipgloss.Width(out); got != tc.width {
						t.Errorf("sel=%d colourless=%v row width = %d, want exactly %d: %q", sel, colourless, got, tc.width, ansi.Strip(out))
					}
				}
			}
		})
	}
}

func TestSessionRow_DirColumnKeepsTrailingSlotsAligned(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	items := flatItems(
		tmux.Session{Name: "a", Windows: 1, Attached: true, Dir: home + "/Code/portal"},
		tmux.Session{Name: "a-much-longer-session-name", Windows: 5, Attached: true, Dir: home + "/x"},
		tmux.Session{Name: "mid-name", Windows: 2, Attached: true},
	)
	d := SessionDelegate{ShowDir: true}

	var wantCount, wantBullet int
	for i := range items {
		out := renderRow(d, 80, items, i, 0)
		count := visibleColOf(out, "window")
		bullet := visibleColOf(out, "●")
		if count < 0 || bullet < 0 {
			t.Fatalf("row %d missing a trailing slot: %q", i, ansi.Strip(out))
		}
		if i == 0 {
			wantCount, wantBullet = count, bullet
			continue
		}
		if count != wantCount {
			t.Errorf("row %d count col %d, want %d", i, count, wantCount)
		}
		if bullet != wantBullet {
			t.Errorf("row %d attached bullet col %d, want %d", i, bullet, wantBullet)
		}
	}
}

func TestSessionRow_DirColumnLeftTruncatesTheDirectoryAndNeverTheName(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	items := flatItems(tmux.Session{Name: "alpha", Windows: 2, Dir: home + "/Code/portal/internal/tui"})
	out := renderRow(SessionDelegate{ShowDir: true}, 50, items, 0, 0)
	vis := ansi.Strip(out)

	if !strings.Contains(vis, "alpha") {
		t.Errorf("the name gave way to the directory: %q", vis)
	}
	if !strings.Contains(vis, "…/internal/tui") {
		t.Errorf("directory not left-truncated to its longest fitting tail: %q", vis)
	}
	if strings.Contains(vis, "~/Code") {
		t.Errorf("directory kept its head instead of its tail: %q", vis)
	}
}

func TestSessionRow_DirColumnDropsTheDirectoryBelowTheFloor(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	items := flatItems(tmux.Session{Name: "alpha", Windows: 2, Dir: home + "/Code/portal/internal/tui"})
	out := renderRow(SessionDelegate{ShowDir: true}, 34, items, 0, 0)
	vis := ansi.Strip(out)

	if !strings.Contains(vis, "alpha") {
		t.Errorf("row lost its name below the directory floor: %q", vis)
	}
	if strings.ContainsAny(vis, "/~") {
		t.Errorf("row kept a fragment of the directory below the floor: %q", vis)
	}
}

func TestSessionRow_DirColumnLeavesAnOverLongNameTruncatedAsItIsWithTheColumnOff(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	sess := tmux.Session{
		Name:     "this-is-a-really-very-long-session-name-that-overflows",
		Windows:  7,
		Attached: true,
		Dir:      home + "/Code/portal",
	}
	on := renderRow(SessionDelegate{ShowDir: true}, 60, flatItems(sess), 0, 0)
	off := renderRow(SessionDelegate{}, 60, flatItems(sess), 0, 0)

	if on != off {
		t.Errorf("an over-long name rendered differently with the column on:\n got %q\nwant %q", escSeq(on), escSeq(off))
	}
	if !strings.Contains(ansi.Strip(on), "…") {
		t.Errorf("over-long name lost its ellipsis: %q", ansi.Strip(on))
	}
}

func TestSessionRow_DirColumnTakesTheWindowCountsToken(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	const dirText = "~/Code/portal"

	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		items := flatItems(
			tmux.Session{Name: "row-zero", Windows: 2, Dir: home + "/Code/portal"},
			tmux.Session{Name: "row-one", Windows: 2, Dir: home + "/Code/portal"},
		)
		d := SessionDelegate{Theme: th, ShowDir: true}

		sel := renderRow(d, 80, items, 0, 0)
		wantSel := lipgloss.NewStyle().
			Foreground(th.TextSecondary.Color()).
			Background(th.BgSelection.Color()).
			Render(dirText)
		if !strings.Contains(sel, wantSel) {
			t.Errorf("[%v] selected row's directory missing text.secondary over bg.selection %q: %q", themeLabel(th), escSeq(wantSel), escSeq(sel))
		}

		uns := renderRow(d, 80, items, 1, 0)
		wantUns := lipgloss.NewStyle().
			Foreground(th.TextMuted.Color()).
			Background(th.Canvas.Color()).
			Render(dirText)
		if !strings.Contains(uns, wantUns) {
			t.Errorf("[%v] unselected row's directory missing text.muted over canvas %q: %q", themeLabel(th), escSeq(wantUns), escSeq(uns))
		}
	}
}

func TestSessionRow_DirColumnSeparatingSpaceCarriesTheSelectionTint(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		items := flatItems(tmux.Session{Name: "alpha", Windows: 2, Dir: home + "/Code/portal"})
		out := renderRow(SessionDelegate{Theme: th, ShowDir: true}, 80, items, 0, 0)

		cells := scanCellBackgrounds(out)
		start := visibleColOf(out, "~/Code/portal") - 1
		end := start + 1 + ansi.StringWidth("~/Code/portal")
		if start < 0 || end > len(cells) {
			t.Fatalf("[%v] directory region [%d,%d) outside the %d scanned cells: %q", themeLabel(th), start, end, len(cells), ansi.Strip(out))
		}
		want := selectionBgParams(t, th)
		for i := start; i < end; i++ {
			if !cells[i].set || cells[i].params != want {
				t.Errorf("[%v] cell %d of the directory region is not the selection tint: got %+v, want %q", themeLabel(th), i, cells[i], want)
			}
		}
	}
}

func TestSessionRow_DirColumnRendersTheSameSingleSpaceWhenColourless(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	items := flatItems(tmux.Session{Name: "alpha", Windows: 2, Attached: true, Dir: home + "/Code/portal"})
	coloured := renderRow(SessionDelegate{Theme: testDarkTheme(t), ShowDir: true}, 80, items, 0, 0)
	colourless := renderRow(SessionDelegate{Theme: testDarkTheme(t), ShowDir: true, Colourless: true}, 80, items, 0, 0)

	if got, want := ansi.Strip(colourless), ansi.Strip(coloured); got != want {
		t.Errorf("colourless row text differs from the coloured row's:\n got %q\nwant %q", got, want)
	}
}

func TestSessionRow_DirColumnNarrowsByAGroupedRowsIndent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	sess := tmux.Session{Name: "s", Windows: 2, Dir: home + "/Code/portal/internal/tui"}
	d := SessionDelegate{ShowDir: true}

	flat := renderRow(d, 50, []list.Item{SessionItem{Session: sess}}, 0, 0)
	grouped := renderRow(d, 50, []list.Item{SessionItem{Session: sess, GroupKey: "k"}}, 0, 0)

	if want := "…/portal/internal/tui"; !strings.Contains(ansi.Strip(flat), want) {
		t.Errorf("flat row directory = %q, want it to carry %q", ansi.Strip(flat), want)
	}
	if want := "…/internal/tui"; !strings.Contains(ansi.Strip(grouped), want) {
		t.Errorf("grouped row directory = %q, want it to carry %q", ansi.Strip(grouped), want)
	}
	if strings.Contains(ansi.Strip(grouped), "…/portal/") {
		t.Errorf("grouped row directory region not narrowed by the indent: %q", ansi.Strip(grouped))
	}
}

func TestSessionRow_DirColumnRendersWholeOnAnUnsizedList(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	items := flatItems(tmux.Session{Name: "alpha", Windows: 2, Dir: home + "/Code/portal/internal/tui"})
	out := renderRow(SessionDelegate{ShowDir: true}, 0, items, 0, 0)
	vis := ansi.Strip(out)

	if !strings.Contains(vis, "alpha ~/Code/portal/internal/tui") {
		t.Errorf("unsized row did not render name, one space and the whole abbreviated directory: %q", vis)
	}
}

func TestSessionRow_DirColumnNeverOverflowsAtNarrowWidths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	for _, w := range []int{1, 5, 10, 20, 25, 26, 29, 40, 80} {
		for _, sess := range []tmux.Session{
			{Name: "x", Windows: 1, Dir: home + "/Code/portal/internal/tui/deeply/nested"},
			{Name: "agentic-workflows-code-based-that-is-quite-long", Windows: 12, Attached: true, Dir: home + "/Code/portal/internal/tui/deeply/nested"},
		} {
			items := flatItems(sess)
			for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
				out := renderRow(SessionDelegate{Theme: th, ShowDir: true}, w, items, 0, 0)
				if got := lipgloss.Width(out); got > w {
					t.Errorf("[w=%d %v %q] row width = %d, overflows the list width %d", w, themeLabel(th), sess.Name, got, w)
				}
			}
		}
	}
}

// The assembled row is clamped to the list width, so a row whose directory
// overran its budget still measures exactly that width — it pays the overrun
// out of the right margin. An intact margin is what says nothing was cut.
func TestSessionRow_DirColumnNeverLeansOnThePathologicalWidthBackstop(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	margin := strings.Repeat(" ", rowRightMargin)
	narrowest := leftBarColumnWidth + nameGap + countSlotWidth + attachedSlotWidth + rowRightMargin + 1

	for _, dir := range []string{
		home + "/Code",
		home + "/Code/portal/internal/tui/deeply/nested",
		"/opt/homebrew/var/lib/some-service",
	} {
		for _, name := range []string{"a", "portal-a1b2", "a-considerably-longer-session-name"} {
			for _, attached := range []bool{false, true} {
				items := flatItems(tmux.Session{Name: name, Windows: 3, Attached: attached, Dir: dir})
				for w := narrowest; w <= 120; w++ {
					out := renderRow(SessionDelegate{ShowDir: true}, w, items, 0, 0)
					if vis := ansi.Strip(out); !strings.HasSuffix(vis, margin) {
						t.Errorf("[w=%d %q %q attached=%v] row lost its right margin to the width backstop: %q", w, name, dir, attached, vis)
					}
				}
			}
		}
	}
}
