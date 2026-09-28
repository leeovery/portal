package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/tmux"
)

// indicatorCombo is one of the four attached × pending states a row can be in.
type indicatorCombo struct {
	label    string
	attached bool
	pending  bool
}

var indicatorCombos = []indicatorCombo{
	{label: "neither"},
	{label: "attached", attached: true},
	{label: "pending", pending: true},
	{label: "both", attached: true, pending: true},
}

func comboRow(t *testing.T, d SessionDelegate, w int, c indicatorCombo) string {
	t.Helper()
	items := flatItems(tmux.Session{Name: "alpha", Windows: 3, Attached: c.attached})
	d.Pending = markedSet()
	if c.pending {
		d.Pending = markedSet("alpha")
	}
	return renderRow(d, w, items, 0, 0)
}

func TestSessionRow_RendersThePendingDotForASessionInThePendingSet(t *testing.T) {
	const w = 80
	items := flatItems(
		tmux.Session{Name: "alpha", Windows: 3},
		tmux.Session{Name: "bravo", Windows: 3},
	)
	margin := strings.Repeat(" ", rowRightMargin)
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		d := SessionDelegate{Theme: th, Pending: markedSet("alpha")}

		pending := renderRow(d, w, items, 0, 0)
		if vis := ansi.Strip(pending); !strings.HasSuffix(vis, "3 windows"+strings.Repeat(" ", 2+indicatorSlotWidth-1)+rowIndicatorGlyph+margin) {
			t.Errorf("[%v] pending row missing the dot hard right: %q", themeLabel(th), vis)
		}

		plain := renderRow(d, w, items, 1, 0)
		if vis := ansi.Strip(plain); !strings.HasSuffix(vis, "3 windows"+strings.Repeat(" ", 2+indicatorSlotWidth)+margin) {
			t.Errorf("[%v] row outside the pending set must leave the indicator cells blank: %q", themeLabel(th), vis)
		}
		if seq := tokenFgSeq(t, th.AccentAttention); strings.Contains(plain, seq) {
			t.Errorf("[%v] row outside the pending set emits the accent.attention fg %q", themeLabel(th), seq)
		}
	}
}

func TestSessionRow_PutsALonePendingDotAtTheSameColumnAsALoneAttachedDot(t *testing.T) {
	for _, w := range []int{40, 80, 120} {
		for _, colourless := range []bool{false, true} {
			d := SessionDelegate{Theme: testDarkTheme(t), Colourless: colourless}
			attachedGlyph, pendingGlyph := rowIndicatorGlyph, rowIndicatorGlyph
			if colourless {
				attachedGlyph, pendingGlyph = attachedIndicatorLetter, pendingIndicatorLetter
			}
			attached := comboRow(t, d, w, indicatorCombos[1])
			pending := comboRow(t, d, w, indicatorCombos[2])

			want := w - rowRightMargin - 1
			if col := visibleColOf(attached, attachedGlyph); col != want {
				t.Errorf("[w=%d col=%v] lone attached indicator at col %d, want %d: %q", w, colourless, col, want, ansi.Strip(attached))
			}
			if col := visibleColOf(pending, pendingGlyph); col != want {
				t.Errorf("[w=%d col=%v] lone pending indicator at col %d, want %d: %q", w, colourless, col, want, ansi.Strip(pending))
			}
		}
	}
}

func TestSessionRow_RendersAttachedThenPendingWhenARowCarriesBoth(t *testing.T) {
	const w = 80
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		out := comboRow(t, SessionDelegate{Theme: th}, w, indicatorCombos[3])
		vis := ansi.Strip(out)

		pair := rowIndicatorGlyph + rowIndicatorGlyph
		if col, want := visibleColOf(out, pair), w-rowRightMargin-2; col != want {
			t.Fatalf("[%v] pair at col %d, want %d (attached pushed left one cell): %q", themeLabel(th), col, want, vis)
		}

		attachedSgr := sgrOpeningGlyph(out, rowIndicatorGlyph)
		_, afterAttached, _ := strings.Cut(out, rowIndicatorGlyph)
		pendingSgr := sgrOpeningGlyph(afterAttached, rowIndicatorGlyph)
		if want := tokenFgSeq(t, th.StatePositive); !strings.Contains(attachedSgr, want) {
			t.Errorf("[%v] first dot opened by %q, want state.positive %q", themeLabel(th), escSeq(attachedSgr), want)
		}
		if want := tokenFgSeq(t, th.AccentAttention); !strings.Contains(pendingSgr, want) {
			t.Errorf("[%v] second dot opened by %q, want accent.attention %q", themeLabel(th), escSeq(pendingSgr), want)
		}
	}
}

func TestSessionRow_RendersTheSameRowWidthAcrossAllFourIndicatorCombinations(t *testing.T) {
	for _, w := range []int{40, 60, 80, 120} {
		for _, colourless := range []bool{false, true} {
			d := SessionDelegate{Theme: testDarkTheme(t), Colourless: colourless}
			countCol := -1
			for _, c := range indicatorCombos {
				out := comboRow(t, d, w, c)
				if got := lipgloss.Width(out); got != w {
					t.Errorf("[w=%d col=%v %s] row width = %d, want %d", w, colourless, c.label, got, w)
				}
				col := visibleColOf(out, "3 windows")
				if countCol == -1 {
					countCol = col
				} else if col != countCol {
					t.Errorf("[w=%d col=%v %s] count at col %d, want %d (slot must not move with its contents)", w, colourless, c.label, col, countCol)
				}
			}
		}
	}
}

func TestSessionRow_RendersOneDotForASessionHoldingSeveralWaitingPanes(t *testing.T) {
	// The set is keyed on the session: however many of its panes wait, the
	// session contributes one member and the row one dot.
	items := flatItems(tmux.Session{Name: "alpha", Windows: 3})
	d := SessionDelegate{Theme: testDarkTheme(t), Pending: markedSet("alpha", "alpha", "alpha")}
	vis := ansi.Strip(renderRow(d, 80, items, 0, 0))
	if n := strings.Count(vis, rowIndicatorGlyph); n != 1 {
		t.Errorf("row carries %d dots, want exactly 1: %q", n, vis)
	}
}

func TestSessionRow_ShowsNeitherIndicatorOnAGoneRow(t *testing.T) {
	const w = 60
	items := flatItems(tmux.Session{Name: "alpha", Windows: 2, Attached: true})
	for _, colourless := range []bool{false, true} {
		base := SessionDelegate{Theme: testDarkTheme(t), Colourless: colourless, MultiSelect: true, GoneFlagged: markedSet("alpha")}
		want := renderRow(base, w, items, 0, 0)

		withPending := base
		withPending.Pending = markedSet("alpha")
		got := renderRow(withPending, w, items, 0, 0)
		if got != want {
			t.Errorf("[col=%v] pending set changed the gone row\n got: %q\nwant: %q", colourless, got, want)
		}
		vis := ansi.Strip(got)
		if !strings.HasSuffix(vis, "2 windows  "+goneBadge) {
			t.Errorf("[col=%v] gone badge must own the trailing region: %q", colourless, vis)
		}
		if strings.Contains(vis, rowIndicatorGlyph) || strings.Contains(vis, pendingIndicatorLetter+" ") {
			t.Errorf("[col=%v] gone row carries an indicator: %q", colourless, vis)
		}
	}
}

func TestSessionRow_RendersAPAndAPUnderNoColor(t *testing.T) {
	const w = 80
	margin := strings.Repeat(" ", rowRightMargin)
	wantCluster := map[string]string{
		"neither":  "",
		"attached": attachedIndicatorLetter,
		"pending":  pendingIndicatorLetter,
		"both":     attachedIndicatorLetter + pendingIndicatorLetter,
	}
	for _, c := range indicatorCombos {
		t.Run(c.label, func(t *testing.T) {
			vis := ansi.Strip(comboRow(t, SessionDelegate{Theme: testDarkTheme(t), Colourless: true}, w, c))
			cluster := wantCluster[c.label]
			want := "3 windows" + strings.Repeat(" ", 2+indicatorSlotWidth-len(cluster)) + cluster + margin
			if !strings.HasSuffix(vis, want) {
				t.Errorf("colourless %s row = %q, want suffix %q", c.label, vis, want)
			}
			if strings.Contains(vis, rowIndicatorGlyph) {
				t.Errorf("colourless %s row renders a dot: %q", c.label, vis)
			}
		})
	}
}

func TestSessionRow_MarksNothingForANilPendingSet(t *testing.T) {
	items := flatItems(
		tmux.Session{Name: "alpha", Windows: 3, Attached: true},
		tmux.Session{Name: "bravo", Windows: 3},
	)
	for _, colourless := range []bool{false, true} {
		nilSet := SessionDelegate{Theme: testDarkTheme(t), Colourless: colourless}
		emptySet := nilSet
		emptySet.Pending = markedSet()
		for i := range items {
			for _, sel := range []int{0, 1} {
				if got, want := renderRow(nilSet, 80, items, i, sel), renderRow(emptySet, 80, items, i, sel); got != want {
					t.Errorf("[col=%v row=%d sel=%d] nil set renders differently from an empty one\n got: %q\nwant: %q", colourless, i, sel, got, want)
				}
			}
		}
	}
}

func TestSessionRow_RendersThePendingDotInTheAttentionToken(t *testing.T) {
	items := flatItems(
		tmux.Session{Name: "pending-selected", Windows: 1},
		tmux.Session{Name: "pending-unselected", Windows: 1},
	)
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		d := SessionDelegate{Theme: th, Pending: markedSet("pending-selected", "pending-unselected")}
		attention := tokenFgSeq(t, th.AccentAttention)
		for i, wantBg := range []string{selectionBgParams(t, th), wantCanvasBgParams(t, th)} {
			sgr := sgrOpeningGlyph(renderRow(d, 80, items, i, 0), rowIndicatorGlyph)
			if !strings.Contains(sgr, attention) || !strings.Contains(sgr, wantBg) {
				t.Errorf("[%v row=%d] pending dot opened by %q, want accent.attention fg %q over bg %q", themeLabel(th), i, escSeq(sgr), attention, wantBg)
			}
		}
	}
}
