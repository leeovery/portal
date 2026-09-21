package capture_test

import (
	"maps"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/capture"
	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/theme"
)

// The exchange for the fixture guard's skip: a surface is no *Fixture, so it
// cannot go under the swap-and-diff completeness guard — this is the equivalent
// guard of its own, so admitting the skip does not shrink the coverage.
func TestResumeSurfaceSwapGuard_EveryTokenFollowsThePalette(t *testing.T) {
	a, b := syntheticPalettes(t)
	aForms, bForms := tokenForms(t, a), tokenForms(t, b)

	names := capture.SurfaceNames()
	if len(names) == 0 {
		t.Fatal("no surface is registered; the guard would range over nothing and pass vacuously")
	}

	t.Run("it paints the same token set under either palette", func(t *testing.T) {
		for _, name := range names {
			t.Run(name, func(t *testing.T) {
				assertPaletteDiff(t, name, surfacePaletteFrame(t, name, a), surfacePaletteFrame(t, name, b), aForms, bForms)
			})
		}
	})
}

// A surface takes its palette as an argument rather than through a live swap,
// so the two frames are two draws of the same surface rather than one model
// re-themed.
func surfacePaletteFrame(t *testing.T, name string, th theme.Theme) string {
	t.Helper()
	sf, ok := capture.SurfaceByName(name)
	if !ok {
		t.Fatalf("SurfaceByName(%s): not registered", name)
	}
	var m tea.Model = sf.WithTheme(th, false)
	m, _ = m.Update(sf.PinRenderSize(tea.WindowSizeMsg{Width: 200, Height: 60}))
	return m.View().Content
}

func assertPaletteDiff(t harnesstest.TestingT, surface, underAFrame, underBFrame string, aForms, bForms []tokenForm) {
	t.Helper()
	for _, form := range aForms {
		if carriesRun(underBFrame, form.fg) {
			t.Errorf("token %s: theme A's foreground run %q survives on surface %s under palette B — that site was never re-pointed onto the palette it was handed", form.name, form.fg, surface)
		}
		if carriesRun(underBFrame, form.bg) {
			t.Errorf("token %s: theme A's background run %q survives on surface %s under palette B — that site was never re-pointed onto the palette it was handed", form.name, form.bg, surface)
		}
	}

	underA, underB := observedTokens(underAFrame, aForms), observedTokens(underBFrame, bForms)
	if len(underA) == 0 {
		t.Errorf("surface %s: no theme-A token observed at all, so there is nothing on this screen for the diff to compare", surface)
	}
	namesA, namesB := slices.Sorted(maps.Keys(underA)), slices.Sorted(maps.Keys(underB))
	if !slices.Equal(namesA, namesB) {
		t.Errorf("surface %s: paints %v under A but %v under B — a site on this screen stopped painting a token it painted under the other palette, or started painting one it did not", surface, namesA, namesB)
	}
}

// The guard's own failing paths, driven over hand-built frames: a guard that
// cannot report is indistinguishable from a screen that is correct.
func TestResumeSurfaceSwapGuard_ReportsWhatItProtects(t *testing.T) {
	a, b := syntheticPalettes(t)
	aForms, bForms := tokenForms(t, a), tokenForms(t, b)
	token := aForms[0]

	for _, tc := range []struct {
		name         string
		underA       string
		underB       string
		wantReported string
	}{
		{
			name:         "a token painted under A is not painted under B",
			underA:       frameCarrying(token.fg),
			underB:       frameCarrying(bForms[1].fg),
			wantReported: "paints",
		},
		{
			name:         "the surface paints no token at all",
			underA:       "plain text, no escapes",
			underB:       "plain text, no escapes",
			wantReported: "no theme-A token observed",
		},
		{
			name:         "a theme-A run survives under palette B",
			underA:       frameCarrying(token.fg),
			underB:       frameCarrying(token.fg),
			wantReported: "survives",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := &harnesstest.Recorder{}
			rec.Run(func() {
				assertPaletteDiff(rec, "probe-surface", tc.underA, tc.underB, aForms, bForms)
			})
			if !rec.Failed() {
				t.Fatalf("the guard reported nothing when %s", tc.name)
			}
			if report := rec.Report(); !strings.Contains(report, tc.wantReported) {
				t.Errorf("the guard's report does not name %q, so it failed for some other reason than %s:\n%s", tc.wantReported, tc.name, report)
			}
		})
	}
}
