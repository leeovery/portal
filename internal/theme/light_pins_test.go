package theme_test

import (
	"maps"
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/theme"
)

var themeIsLight = map[string]bool{
	"gitlab-dark-grey": false,
	"horizon":          false,
	"nord":             false,
	"nord-wave":        false,
	"tokyo-night":      false,
	"tokyo-night-day":  true,
}

var pinnedTokenNames = []string{
	"bg.selection",
	"bg.attention",
	"bg.subtle",
	"border",
}

var lightPins = map[string]map[string]string{
	"tokyo-night-day": {
		"bg.selection": "#D0C6F0",
		"bg.attention": "#E8D6A8",
		"bg.subtle":    "#D2D4DE",
		"border":       "#C9CDDB",
	},
}

func TestThemeAppearanceTableCoversEveryEmbeddedTheme(t *testing.T) {
	embedded := theme.BuiltinSlugs()
	if len(embedded) == 0 {
		t.Fatal("the embedded set is empty — the enrolment assertion would pass vacuously")
	}

	for _, slug := range embedded {
		if _, enrolled := themeIsLight[slug]; !enrolled {
			t.Errorf("built-in %q has no light/dark entry — enrol it in themeIsLight so the light-only pins know whether to run against it", slug)
		}
	}

	enrolled := slices.Sorted(maps.Keys(themeIsLight))
	if !slices.Equal(enrolled, embedded) {
		t.Errorf("the light/dark table enrols %v, want exactly the embedded set %v", enrolled, embedded)
	}
}

func TestThemeAppearanceTableHasNoStaleEntries(t *testing.T) {
	embedded := theme.BuiltinSlugs()

	for _, slug := range slices.Sorted(maps.Keys(themeIsLight)) {
		if !slices.Contains(embedded, slug) {
			t.Errorf("the light/dark table enrols %q, which is not an embedded built-in (embedded: %v) — drop the stale row", slug, embedded)
		}
	}
}

func TestLightSurfaceTintsPinned(t *testing.T) {
	themes := embeddedThemes(t)

	for _, slug := range lightThemeSlugs(t) {
		pins, ok := lightPins[slug]
		if !ok {
			t.Errorf("light built-in %q has no row in lightPins — its four un-derivable tints would carry no guard", slug)
			continue
		}

		th := themes[slug]
		for _, token := range pinnedTokenNames {
			t.Run(slug+"/"+token, func(t *testing.T) {
				want, pinned := pins[token]
				if !pinned {
					t.Fatalf("lightPins[%q] carries no pin for %q — all four eyeball-pinned tints are required", slug, token)
				}
				if got := tokenValue(t, th, token); got != want {
					t.Errorf("%s %s = %q, want the eyeball-pinned %q", slug, token, got, want)
				}
			})
		}
	}
}

func TestLightTintFillsArePerceptible(t *testing.T) {
	themes := embeddedThemes(t)

	for _, slug := range lightThemeSlugs(t) {
		th := themes[slug]
		for _, token := range pinnedTokenNames {
			t.Run(slug+"/"+token, func(t *testing.T) {
				assertAtLeast(t, slug+" "+token+" fill vs own canvas",
					tokenValue(t, th, token), th.Canvas.Value, floorFillPerceptible)
			})
		}
	}
}

func TestLightPins_AreExactlyFourTokens(t *testing.T) {
	const want = 4

	wantSet := slices.Sorted(slices.Values(pinnedTokenNames))

	if got := len(pinnedTokenNames); got != want {
		t.Errorf("the eyeball-pinned set is %d tokens %v, want exactly %d", got, pinnedTokenNames, want)
	}
	if got := len(slices.Compact(slices.Clone(wantSet))); got != len(pinnedTokenNames) {
		t.Errorf("the eyeball-pinned set %v repeats a token — the count is of DISTINCT tokens", pinnedTokenNames)
	}

	vocabulary := theme.TokenNames()
	for _, token := range pinnedTokenNames {
		if !slices.Contains(vocabulary, token) {
			t.Errorf("pinned token %q is not in the closed vocabulary %v", token, vocabulary)
		}
	}

	for _, slug := range slices.Sorted(maps.Keys(lightPins)) {
		if pinned := slices.Sorted(maps.Keys(lightPins[slug])); !slices.Equal(pinned, wantSet) {
			t.Errorf("lightPins[%q] pins %v, want exactly the four eyeball-pinned tokens %v", slug, pinned, wantSet)
		}
	}
}

func TestLightPins_SkipDarkThemes(t *testing.T) {
	dark := darkThemeSlugs(t)
	if len(dark) == 0 {
		t.Fatal("no embedded theme is enrolled dark — this test would pass vacuously")
	}

	light := lightThemeSlugs(t)
	for _, slug := range dark {
		if slices.Contains(light, slug) {
			t.Errorf("%q is enrolled dark yet appears in the light run set %v", slug, light)
		}
		if _, pinned := lightPins[slug]; pinned {
			t.Errorf("dark built-in %q carries a light pin row — the eyeball pins are light-only", slug)
		}
	}

	// Without this a row for a deleted theme survives silently, which is the
	// staleness the enrolment table exists to prevent.
	for slug := range lightPins {
		if !slices.Contains(light, slug) {
			t.Errorf("lightPins carries a row for %q, which is enrolled in no light theme %v", slug, light)
		}
	}
}

func lightThemeSlugs(t *testing.T) []string {
	t.Helper()
	return enrolledSlugs(t, true)
}

func darkThemeSlugs(t *testing.T) []string {
	t.Helper()
	return enrolledSlugs(t, false)
}

func enrolledSlugs(t *testing.T, isLight bool) []string {
	t.Helper()

	if len(themeIsLight) == 0 {
		t.Fatal("the light/dark table is empty — every assertion driven by it would pass vacuously")
	}

	var slugs []string
	for slug, light := range themeIsLight {
		if light == isLight {
			slugs = append(slugs, slug)
		}
	}
	slices.Sort(slugs)
	return slugs
}

func tokenValue(t *testing.T, th theme.Theme, name string) string {
	t.Helper()

	for _, token := range th.All() {
		if token.Name == name {
			return token.Value
		}
	}
	t.Fatalf("token %q is not in the theme's vocabulary %v", name, theme.TokenNames())
	return ""
}
