package capture_test

import (
	"image/color"
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/capture"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/themetest"
	"github.com/leeovery/portal/internal/tui"
)

const (
	truecolorForegroundIntroducer = "38;2;"
	truecolorBackgroundIntroducer = "48;2;"
)

const (
	syntheticRedA = 0x6E
	syntheticRedB = 0xD2
)

func syntheticPalettes(t *testing.T) (a, b theme.Theme) {
	t.Helper()
	return themetest.SyntheticPair(t, syntheticRedA, syntheticRedB)
}

type tokenForm struct {
	name string
	fg   string
	bg   string
}

func tokenForms(t *testing.T, th theme.Theme) []tokenForm {
	t.Helper()
	tokens := th.All()
	forms := make([]tokenForm, 0, len(tokens))
	for _, tok := range tokens {
		forms = append(forms, tokenForm{
			name: tok.Name,
			fg:   sgrParameterRun(t, lipgloss.NewStyle().Foreground(tok.Color())),
			bg:   sgrParameterRun(t, lipgloss.NewStyle().Background(tok.Color())),
		})
	}
	return forms
}

func sgrParameterRun(t *testing.T, style lipgloss.Style) string {
	t.Helper()
	probe := style.Render("x")
	start := strings.IndexByte(probe, '[')
	end := strings.IndexByte(probe, 'm')
	if start < 0 || end <= start {
		t.Fatalf("could not derive an SGR parameter run from %q", probe)
	}
	return probe[start+1 : end]
}

func carriesRun(frame, run string) bool {
	for offset := 0; offset < len(frame); {
		at := strings.Index(frame[offset:], run)
		if at < 0 {
			return false
		}
		end := offset + at + len(run)
		if end < len(frame) && (frame[end] == 'm' || frame[end] == ';') {
			return true
		}
		offset += at + 1
	}
	return false
}

func observedTokens(frame string, forms []tokenForm) map[string]bool {
	seen := make(map[string]bool, len(forms))
	for _, form := range forms {
		if carriesRun(frame, form.fg) || carriesRun(frame, form.bg) {
			seen[form.name] = true
		}
	}
	return seen
}

type colourReporter interface{ Colourless() bool }

// Clones first: slices.DeleteFunc compacts in place and zeroes the caller's
// tail, so filtering a slice the caller still holds would silently truncate it.
func excludeColourless[F colourReporter](all []F) []F {
	return slices.DeleteFunc(slices.Clone(all), func(fx F) bool { return fx.Colourless() })
}

func registryFixtures(t *testing.T) []*capture.Fixture {
	t.Helper()
	names := capture.FixtureNames()
	fixtures := make([]*capture.Fixture, 0, len(names))
	for _, name := range names {
		if capture.IsStandalone(name) {
			continue
		}
		fx, err := capture.FixtureByName(name)
		if err != nil {
			t.Fatalf("FixtureByName(%s): %v — every enumerated name outside %v must resolve; skipping one silently would read as coverage", name, err, capture.StandaloneNames())
		}
		fixtures = append(fixtures, fx)
	}
	return fixtures
}

func guardedFixtures(t *testing.T) []*capture.Fixture {
	t.Helper()
	fixtures := excludeColourless(registryFixtures(t))
	if len(fixtures) == 0 {
		t.Fatal("every enumerated fixture reported colourless; the guard would range over nothing and pass vacuously")
	}
	return fixtures
}

func swapModel(fx *capture.Fixture, a, b theme.Theme) tui.Model {
	m := fx.ModelAt(a, harnessWidth, harnessHeight)
	_ = m.View()
	m.ApplyTheme(b)
	return m
}

func sameColour(got, want color.Color) bool {
	if got == nil || want == nil {
		return got == nil && want == nil
	}
	gr, gg, gb, ga := got.RGBA()
	wr, wg, wb, wa := want.RGBA()
	return gr == wr && gg == wg && gb == wb && ga == wa
}

func TestThemeSwapGuard_EnumeratesRegistry(t *testing.T) {
	t.Run("every enumerated name outside the standalone set resolves to a fixture", func(t *testing.T) {
		resolved := make([]string, 0, len(capture.FixtureNames()))
		for _, fx := range registryFixtures(t) {
			resolved = append(resolved, fx.Name())
		}
		if len(resolved) == 0 {
			t.Fatal("the registry enumerated no build-backed fixtures; the guard would assert over nothing")
		}
		want := slices.DeleteFunc(capture.FixtureNames(), capture.IsStandalone)
		slices.Sort(resolved)
		if !slices.Equal(resolved, want) {
			t.Errorf("the guard covers %v, want every enumerated fixture %v", resolved, want)
		}
	})

	t.Run("it skips exactly the named standalone surfaces", func(t *testing.T) {
		skip := capture.StandaloneNames()
		if len(skip) == 0 {
			t.Fatal("the skip set is empty; the assertions below would pass vacuously")
		}
		enumerated := capture.FixtureNames()
		// The hazard the widening exists for: a standalone screen that is
		// rendered by name but not named here drops out of the skip, and every
		// enumerating guard fatals on a name it cannot resolve.
		for _, name := range capture.SurfaceNames() {
			if !slices.Contains(skip, name) {
				t.Errorf("surface %s is renderable by name but absent from the skip set %v; each enumerating guard reads through that set and would fatal on it", name, skip)
			}
		}
		for _, name := range skip {
			if name == capture.ContrastValidationFixture {
				continue
			}
			if _, ok := capture.SurfaceByName(name); !ok {
				t.Errorf("%s is skipped but resolves to no surface either; a skip names a screen with a guard of its own, and this one is rendered by nothing", name)
			}
		}
		for _, name := range skip {
			if !slices.Contains(enumerated, name) {
				t.Errorf("%s is skipped but not enumerated by FixtureNames(); a skip names a screen the guard gives up in exchange for a guard of its own, not a screen nothing reaches", name)
			}
			if _, err := capture.FixtureByName(name); err == nil {
				t.Errorf("FixtureByName(%s) resolved a fixture; it is skipped as a standalone tea.Model, so a resolvable one must go under the guard instead", name)
			}
		}

		covered := make([]string, 0, len(enumerated))
		for _, fx := range registryFixtures(t) {
			covered = append(covered, fx.Name())
		}
		covered = append(covered, skip...)
		slices.Sort(covered)
		if !slices.Equal(covered, enumerated) {
			t.Errorf("the resolved fixtures plus the skip set are %v, want exactly FixtureNames() %v — an enumerated name that is neither is a screen no guard covers", covered, enumerated)
		}
	})
}

func TestSyntheticThemes_AllValuesUnique(t *testing.T) {
	a, b := syntheticPalettes(t)
	tokens := append(a.All(), b.All()...)

	t.Run("all 38 values are unique", func(t *testing.T) {
		if len(tokens) != 38 {
			t.Fatalf("the pair carries %d values, want 38 (19 tokens × 2 palettes)", len(tokens))
		}
		seen := make(map[string]string, len(tokens))
		for i, tok := range tokens {
			palette := "A"
			if i >= len(tokens)/2 {
				palette = "B"
			}
			owner := palette + "." + tok.Name
			if prior, dup := seen[tok.Value]; dup {
				t.Errorf("%s repeats %s's value %s; a shared value renders identically either side of the swap and covers nothing", owner, prior, tok.Value)
			}
			seen[tok.Value] = owner
		}
	})

	t.Run("every component renders as three decimal digits", func(t *testing.T) {
		for _, tok := range tokens {
			for _, component := range rgbComponents(t, tok.Value) {
				if component < 100 || component > 255 {
					t.Errorf("token %s value %s has component %d outside the 100–255 three-digit range", tok.Name, tok.Value, component)
				}
			}
		}
	})
}

func rgbComponents(t *testing.T, value string) [3]int {
	t.Helper()
	if len(value) != 7 || value[0] != '#' {
		t.Fatalf("value %q is not #RRGGBB", value)
	}
	var components [3]int
	for i := range components {
		channel, err := strconv.ParseUint(value[1+2*i:3+2*i], 16, 8)
		if err != nil {
			t.Fatalf("parse channel %d of %q: %v", i, value, err)
		}
		components[i] = int(channel)
	}
	return components
}

func TestThemeSwapGuard_TokenFormsAreWellFormed(t *testing.T) {
	a, b := syntheticPalettes(t)
	for _, palette := range []struct {
		name  string
		theme theme.Theme
	}{{"A", a}, {"B", b}} {
		t.Run("palette "+palette.name, func(t *testing.T) {
			values := make(map[string]string, len(palette.theme.All()))
			for _, tok := range palette.theme.All() {
				values[tok.Name] = tok.Value
			}
			for _, form := range tokenForms(t, palette.theme) {
				want := rgbComponents(t, values[form.name])
				assertParameterRun(t, form.name, "foreground", form.fg, truecolorForegroundIntroducer, want)
				assertParameterRun(t, form.name, "background", form.bg, truecolorBackgroundIntroducer, want)
			}
		})
	}
}

func assertParameterRun(t *testing.T, token, role, run, introducer string, want [3]int) {
	t.Helper()
	channels, opened := strings.CutPrefix(run, introducer)
	if !opened {
		t.Errorf("token %s: derived %s run %q does not open with %q, so the guard scans for a form no render can contain", token, role, run, introducer)
		return
	}
	fields := strings.Split(channels, ";")
	if len(fields) != len(want) {
		t.Errorf("token %s: derived %s run %q carries %d channel(s) after %q, want %d", token, role, run, len(fields), introducer, len(want))
		return
	}
	for i, field := range fields {
		got, err := strconv.Atoi(field)
		if err != nil {
			t.Errorf("token %s: derived %s run %q has non-decimal channel %d (%q): %v", token, role, run, i, field, err)
			continue
		}
		if got != want[i] {
			t.Errorf("token %s: derived %s run %q has channel %d = %d, want %d — the run does not round-trip to the token's own value", token, role, run, i, got, want[i])
		}
	}
}

func TestThemeSwapGuard_RenderIsTruecolor(t *testing.T) {
	a, b := syntheticPalettes(t)
	aForms := tokenForms(t, a)
	for _, fx := range guardedFixtures(t) {
		t.Run(fx.Name(), func(t *testing.T) {
			before, _ := fx.RenderSwapRender(a, b, harnessWidth, harnessHeight)
			carried := slices.ContainsFunc(aForms, func(form tokenForm) bool {
				return carriesRun(before, form.fg) || carriesRun(before, form.bg)
			})
			if !carried {
				t.Error("the A-frame carries none of the derived theme-A runs, so either it is not a truecolor render or the derived forms do not match what it renders — either way there is nothing for the guard to diff")
			}
		})
	}
}

func TestThemeSwapGuard_NoStaleValueSurvives(t *testing.T) {
	a, b := syntheticPalettes(t)
	aForms := tokenForms(t, a)

	for _, fx := range guardedFixtures(t) {
		t.Run(fx.Name(), func(t *testing.T) {
			_, after := fx.RenderSwapRender(a, b, harnessWidth, harnessHeight)
			for _, form := range aForms {
				if carriesRun(after, form.fg) {
					t.Errorf("token %s: theme A's foreground run %q survives the swap on fixture %s — that site was never re-pointed onto the new theme", form.name, form.fg, fx.Name())
				}
				if carriesRun(after, form.bg) {
					t.Errorf("token %s: theme A's background run %q survives the swap on fixture %s — that site was never re-pointed onto the new theme", form.name, form.bg, fx.Name())
				}
			}
		})
	}
}

func TestThemeSwapGuard_EveryBValuePresentInUnion(t *testing.T) {
	a, b := syntheticPalettes(t)
	aForms, bForms := tokenForms(t, a), tokenForms(t, b)

	seenUnderA := make(map[string][]string, len(aForms))
	seenUnderB := make(map[string][]string, len(bForms))
	for _, fx := range guardedFixtures(t) {
		before, after := fx.RenderSwapRender(a, b, harnessWidth, harnessHeight)
		underA, underB := observedTokens(before, aForms), observedTokens(after, bForms)

		if len(underA) == 0 {
			t.Errorf("fixture %s: no theme-A token observed at all, so the swap has nothing to be diffed against on this screen", fx.Name())
		}
		namesA, namesB := slices.Sorted(maps.Keys(underA)), slices.Sorted(maps.Keys(underB))
		if !slices.Equal(namesA, namesB) {
			t.Errorf("fixture %s: renders %v under A but %v under B — a site on this screen stopped painting a token it painted before the swap, or started painting one it did not", fx.Name(), namesA, namesB)
		}

		for name := range underA {
			seenUnderA[name] = append(seenUnderA[name], fx.Name())
		}
		for name := range underB {
			seenUnderB[name] = append(seenUnderB[name], fx.Name())
		}
	}

	for _, name := range theme.TokenNames() {
		underA, underB := seenUnderA[name], seenUnderB[name]
		if len(underA) > 0 && len(underB) == 0 {
			slices.Sort(underA)
			t.Errorf("token %s renders under theme A (on %v) but no theme-B value for it appears on any fixture after the swap — that site renders nothing rather than merely stale", name, underA)
		}
		if len(underA) == 0 && len(underB) > 0 {
			slices.Sort(underB)
			t.Errorf("token %s renders under theme B (on %v) but never under theme A — the two renders disagree on which sites exist, so the diff is not comparing like for like", name, underB)
		}
	}
}

type swappableFixture interface {
	Name() string
	RenderSwapRender(a, b theme.Theme, w, h int) (before, after string)
}

type swappedFrame struct {
	fixture string
	frame   string
}

func swappedFrames[F swappableFixture](fixtures []F, a, b theme.Theme) []swappedFrame {
	frames := make([]swappedFrame, 0, len(fixtures))
	for _, fx := range fixtures {
		_, after := fx.RenderSwapRender(a, b, harnessWidth, harnessHeight)
		frames = append(frames, swappedFrame{fixture: fx.Name(), frame: after})
	}
	return frames
}

func coveredTokens(frames []swappedFrame, forms []tokenForm) map[string][]string {
	loci := make(map[string][]string, len(forms))
	for _, f := range frames {
		for name := range observedTokens(f.frame, forms) {
			loci[name] = append(loci[name], f.fixture)
		}
	}
	return loci
}

func uncoveredTokens(loci map[string][]string) []string {
	var gaps []string
	for _, name := range theme.TokenNames() {
		if len(loci[name]) == 0 {
			gaps = append(gaps, name)
		}
	}
	return gaps
}

func TestThemeSwapGuard_EveryTokenExercisedByAFixture(t *testing.T) {
	a, b := syntheticPalettes(t)
	bForms := tokenForms(t, b)
	frames := swappedFrames(guardedFixtures(t), a, b)

	t.Run("every token in the vocabulary is rendered by some fixture", func(t *testing.T) {
		covered := coveredTokens(frames, bForms)
		for _, name := range theme.TokenNames() {
			t.Logf("%s ← %v", name, covered[name])
		}
		for _, name := range uncoveredTokens(covered) {
			t.Errorf("token %s renders on no included fixture, so it is absent from BOTH of assertion 2's unions, they balance, and nothing reports it — ADD A FIXTURE that renders %s. Do NOT exempt the token: an exemption is the permanent render-layer carve-out this guard exists to catch. Do NOT weaken this to the tokens observed under theme A: that is assertion 2, and its A/B balance is exactly what hides the gap", name, name)
		}
	})

	t.Run("a narrowed fixture set reports the tokens it drops", func(t *testing.T) {
		narrowed := frames[:1]
		if len(uncoveredTokens(coveredTokens(narrowed, bForms))) == 0 {
			t.Errorf("fixture %s alone renders all %d tokens, so narrowing to it demonstrates nothing about whether this assertion can report a gap; re-point the narrowing at a set that genuinely drops one", narrowed[0].fixture, len(theme.TokenNames()))
		}
	})
}

func TestThemeSwapGuard_ViewBackgroundColourFollowsSwap(t *testing.T) {
	a, b := syntheticPalettes(t)
	for _, fx := range guardedFixtures(t) {
		t.Run(fx.Name(), func(t *testing.T) {
			got := swapModel(fx, a, b).View().BackgroundColor
			if !sameColour(got, b.Canvas.Color()) {
				t.Errorf("View().BackgroundColor = %v after the swap, want theme B's canvas %s — the declarative frame background did not follow the swap", got, b.Canvas.Value)
			}
		})
	}
}

type stubFixture struct {
	name       string
	colourless bool
	after      string
}

func (f stubFixture) Colourless() bool { return f.colourless }
func (f stubFixture) Name() string     { return f.name }

func (f stubFixture) RenderSwapRender(_, _ theme.Theme, _, _ int) (before, after string) {
	return "", f.after
}

func TestThemeSwapGuard_ExcludesColourlessFixtures(t *testing.T) {
	coloured := stubFixture{name: "coloured"}
	colourless := stubFixture{name: "colourless", colourless: true}

	kept := excludeColourless([]stubFixture{coloured, colourless})
	if !slices.Equal(kept, []stubFixture{coloured}) {
		t.Errorf("excludeColourless kept %v, want only the coloured fixture — a colourless render carries no theme colours, so there is nothing to diff", kept)
	}
}

func foregroundOnlyForms(forms []tokenForm) []tokenForm {
	narrowed := make([]tokenForm, 0, len(forms))
	for _, form := range forms {
		narrowed = append(narrowed, tokenForm{name: form.name, fg: form.fg, bg: form.fg})
	}
	return narrowed
}

func backgroundOnlyForms(forms []tokenForm) []tokenForm {
	narrowed := make([]tokenForm, 0, len(forms))
	for _, form := range forms {
		narrowed = append(narrowed, tokenForm{name: form.name, fg: form.bg, bg: form.bg})
	}
	return narrowed
}

func TestTokenCoverage_MatchesBackgroundForm(t *testing.T) {
	a, b := syntheticPalettes(t)
	bForms := tokenForms(t, b)
	frames := swappedFrames(guardedFixtures(t), a, b)

	byForeground := coveredTokens(frames, foregroundOnlyForms(bForms))
	byBackground := coveredTokens(frames, backgroundOnlyForms(bForms))

	for _, tc := range []struct {
		token                 string
		exclusivelyBackground bool
	}{
		{token: "canvas", exclusivelyBackground: true},
		{token: "bg.selection", exclusivelyBackground: true},
		{token: "bg.attention", exclusivelyBackground: true},
		{token: "bg.subtle", exclusivelyBackground: false},
	} {
		t.Run(tc.token, func(t *testing.T) {
			if len(byBackground[tc.token]) == 0 {
				t.Errorf("no fixture renders %s as a background, so the background half of observedTokens' match finds it nowhere", tc.token)
			}
			foundByForeground := len(byForeground[tc.token]) > 0
			if tc.exclusivelyBackground && foundByForeground {
				t.Errorf("%s is now found by a FOREGROUND run too (on %v); it was measured background-only, so this row no longer evidences that observedTokens' OR is load-bearing — re-measure and re-record", tc.token, byForeground[tc.token])
			}
			if !tc.exclusivelyBackground && !foundByForeground {
				t.Errorf("%s is no longer found by a foreground run anywhere; it was measured as rendering BOTH ways (the loading bar's track paints it as foreground and background alike), so this row's exception is stale", tc.token)
			}
		})
	}
}

func TestTokenCoverage_MatchesForegroundForm(t *testing.T) {
	a, b := syntheticPalettes(t)
	bForms := tokenForms(t, b)
	frames := swappedFrames(guardedFixtures(t), a, b)

	const token = "border"
	byForeground := coveredTokens(frames, foregroundOnlyForms(bForms))
	byBackground := coveredTokens(frames, backgroundOnlyForms(bForms))

	if len(byForeground[token]) == 0 {
		t.Errorf("no fixture renders %s as a foreground, so the foreground half of observedTokens' match finds it nowhere", token)
	}
	if got := byBackground[token]; len(got) > 0 {
		t.Errorf("%s is now found by a BACKGROUND run too (on %v); it was measured foreground-only, so this test no longer evidences that observedTokens' OR is load-bearing — re-measure and re-record", token, got)
	}
}

func TestTokenCoverage_IgnoresExcludedFixtures(t *testing.T) {
	a, b := syntheticPalettes(t)
	forms := tokenForms(t, b)
	included, excluded := forms[0], forms[1]

	fixtures := []stubFixture{
		{name: "coloured", after: frameCarrying(included.fg)},
		{name: "colourless", colourless: true, after: frameCarrying(excluded.bg)},
	}
	loci := coveredTokens(swappedFrames(excludeColourless(fixtures), a, b), forms)

	if got := loci[included.name]; !slices.Equal(got, []string{"coloured"}) {
		t.Errorf("token %s has loci %v, want only the included fixture — the scan did not read the included frame", included.name, got)
	}
	if got := loci[excluded.name]; len(got) > 0 {
		t.Errorf("token %s is credited to %v, but its only carrier is COLOURLESS — a colourless render carries no theme colour, so covering it there covers it nowhere", excluded.name, got)
	}
	if gaps := uncoveredTokens(loci); !slices.Contains(gaps, excluded.name) {
		t.Errorf("uncoveredTokens reports %v, which omits %s — a token carried only by an excluded fixture must still be reported as needing one", gaps, excluded.name)
	}
}

func frameCarrying(run string) string {
	return "\x1b[" + run + "mx\x1b[0m"
}

func TestTokenCoverage_TransientStatesHaveALocus(t *testing.T) {
	a, b := syntheticPalettes(t)
	frames := swappedFrames(guardedFixtures(t), a, b)
	loci := coveredTokens(frames, tokenForms(t, b))

	for _, tc := range []struct{ token, fixture string }{
		{token: "bg.attention", fixture: "sessions-inline-flash"},
		{token: "text.on-attention", fixture: "sessions-inline-flash"},
		{token: "accent.mode", fixture: "preview-screen"},
		{token: "state.destructive", fixture: "loading-error"},
		{token: "text.on-selection", fixture: "sessions-flat"},
		{token: "text.subtle", fixture: "sessions-by-project"},
		{token: "bg.subtle", fixture: "loading-screen"},
	} {
		t.Run(tc.token, func(t *testing.T) {
			if !slices.Contains(loci[tc.token], tc.fixture) {
				t.Errorf("%s is not rendered by %s — its loci are %v. That fixture was this token's recorded locus, so either restore what rendered it there or record the fixture that carries it now", tc.token, tc.fixture, loci[tc.token])
			}
		})
	}
}
