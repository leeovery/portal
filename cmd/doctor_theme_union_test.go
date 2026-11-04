package cmd

import (
	"bytes"
	"fmt"
	"go/ast"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/theme"
)

func unionAdvisoriesFor(t *testing.T, content, themesDir string) []themeAdvisory {
	t.Helper()

	return themeAdvisoryUnion(persistedThemeDeps(t, content, themesDir))
}

// Widening assembleThemeAdvisories to the renderer's line-only class stops this
// file compiling: a producer with no identity field could then defeat the dedup.
var _ func([]themeAdvisory, []themeAdvisory) []themeAdvisory = assembleThemeAdvisories

func requireAdvisoryLines(t *testing.T, got []themeAdvisory, want ...string) {
	t.Helper()

	lines := advisoryLines(got)
	if !slices.Equal(lines, want) {
		t.Fatalf("advisory lines =\n  %s\nwant\n  %s", strings.Join(lines, "\n  "), strings.Join(want, "\n  "))
	}
}

func TestThemeAdvisoryUnion_PersistedLineWins(t *testing.T) {
	requireDropInSlug(t, "nord-frost")

	cases := []struct{ name, prefs, want string }{
		{
			name:  "a slot names the invalid file",
			prefs: `{"theme_dark":"nord-frost"}`,
			want:  "⚠ theme nord-frost (dark) does not resolve: bad colour",
		},
		{
			name:  "a constant names the invalid file",
			prefs: `{"theme":"nord-frost"}`,
			want:  "⚠ theme nord-frost does not resolve: bad colour",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := themesDirWith(t, map[string][]byte{"nord-frost.theme": sourceBadColours(t, themeOverride{"canvas", "blue"})})

			const fileLine = "⚠ theme nord-frost: bad colour — canvas = blue"
			loader := theme.NewSilentLoader()
			requireAdvisoryLines(t, scanThemesDirectory(loader.OpenEnumeration(dir)), fileLine)

			got := requireOneAdvisory(t, unionAdvisoriesFor(t, tc.prefs, dir))
			if got.line != tc.want {
				t.Errorf("advisory line = %q; want %q — the file line %q is the one that must be dropped", got.line, tc.want, fileLine)
			}
			if !got.fromPrefs {
				t.Error("advisory fromPrefs = false; the surviving line is the persisted one")
			}
		})
	}

	t.Run("the report carries the one line", func(t *testing.T) {
		t.Setenv("PORTAL_THEMES_DIR", themesDirWith(t, map[string][]byte{
			"nord-frost.theme": sourceBadColours(t, themeOverride{"canvas", "blue"}),
		}))
		setPrefsFile(t, `{"theme_dark":"nord-frost"}`)

		outBuf, _, err := runDoctorWith(t, healthyDoctorDeps(t))
		if err != nil {
			t.Fatalf("Execute err = %v; a theme advisory must never drive the exit code", err)
		}

		out := outBuf.String()
		want := "" +
			"  ⚠ theme nord-frost (dark) does not resolve: bad colour\n" +
			"  8 checks passed · 1 advisory\n"
		if !strings.HasSuffix(out, want) {
			t.Errorf("report does not close with the single union line and its summary:\n%s", out)
		}
		if strings.Contains(out, "⚠ theme nord-frost: bad colour") {
			t.Errorf("report carries the dropped file line as well:\n%s", out)
		}
	})

	t.Run("the rank is the fromPrefs field, not the argument position", func(t *testing.T) {
		// Only a hand-built value can separate rank-by-field from
		// rank-by-argument-position: the real producer always sets fromPrefs.
		file := themeAdvisory{line: "⚠ theme nord-frost: bad colour — canvas = blue", slug: "nord-frost"}
		unranked := themeAdvisory{line: "⚠ theme nord-frost does not resolve: bad colour", slug: "nord-frost"}

		requireAdvisoryLines(t, assembleThemeAdvisories([]themeAdvisory{file}, []themeAdvisory{unranked}), file.line, unranked.line)
	})
}

func TestThemeAdvisoryUnion_OneParseBehindBothProducers(t *testing.T) {
	requireDropInSlug(t, "nord-frost")

	const want = "⚠ theme nord-frost does not resolve: bad colour"

	assemble := func(t *testing.T, dir string, disturb func(*testing.T, string)) []themeAdvisory {
		t.Helper()

		loader := theme.NewSilentLoader()
		enumeration := loader.OpenEnumeration(dir)
		disturb(t, dir)

		deps := persistedThemeDeps(t, `{"theme":"nord-frost"}`, dir)
		return assembleThemeAdvisories(scanThemesDirectory(enumeration), persistedThemeAdvisories(deps, loader, enumeration))
	}

	t.Run("a rewritten file cannot change one producer's answer", func(t *testing.T) {
		dir := themesDirWith(t, map[string][]byte{"nord-frost.theme": sourceBadColours(t, themeOverride{"canvas", "blue"})})

		requireAdvisoryLines(t, assemble(t, dir, func(t *testing.T, dir string) {
			path := filepath.Join(dir, "nord-frost.theme")
			if err := os.WriteFile(path, validThemeSource(t), 0o644); err != nil {
				t.Fatalf("rewrite %s: %v", path, err)
			}
		}), want)
	})

	t.Run("a directory that is gone changes nothing", func(t *testing.T) {
		dir := themesDirWith(t, map[string][]byte{"nord-frost.theme": sourceBadColours(t, themeOverride{"canvas", "blue"})})

		requireAdvisoryLines(t, assemble(t, dir, func(t *testing.T, dir string) {
			if err := os.RemoveAll(dir); err != nil {
				t.Fatalf("remove %s: %v", dir, err)
			}
		}), want)
	})
}

func TestThemeAdvisoryUnion_BadNameFileNeverCollides(t *testing.T) {
	requireDropInSlug(t, "nord-frost")
	dir := themesDirWith(t, map[string][]byte{"Nord.theme": validThemeSource(t)})

	got := unionAdvisoriesFor(t, `{"theme":"nord-frost"}`, dir)
	requireAdvisoryLines(t, got,
		"⚠ theme file Nord.theme: slug must be lowercase letters, digits and hyphens",
		"⚠ theme nord-frost does not resolve: not found",
	)
	if got[0].slug != "" {
		t.Errorf("file advisory slug = %q; the two lines stand together because a `bad name` row carries none", got[0].slug)
	}

	t.Run("a persisted line with no slug covers nothing", func(t *testing.T) {
		// Neither side of the guard is reachable from the producers, so only a
		// hand-built pair reaches it: without the guard the empty string would be
		// an ordinary key matching every bad-name row and the directory line.
		directory := themeAdvisory{line: "⚠ themes directory unreadable: /themes"}
		badName := themeAdvisory{line: "⚠ theme file Nord.theme: slug must be lowercase letters, digits and hyphens"}
		slugless := themeAdvisory{line: "⚠ theme  does not resolve: not found", fromPrefs: true}

		requireAdvisoryLines(t, assembleThemeAdvisories([]themeAdvisory{directory, badName}, []themeAdvisory{slugless}),
			directory.line, badName.line, slugless.line)
	})
}

func TestThemeAdvisoryUnion_ReservedNameResolvesToBuiltin(t *testing.T) {
	requireBuiltinSlug(t, "nord")
	dir := themesDirWith(t, map[string][]byte{"nord.theme": validThemeSource(t)})

	got := requireOneAdvisory(t, unionAdvisoriesFor(t, `{"theme":"nord"}`, dir))

	if want := reservedNameLine("nord.theme", "nord"); got.line != want {
		t.Errorf("advisory line = %q; want %q", got.line, want)
	}
	if got.fromPrefs {
		t.Error("advisory fromPrefs = true; the surviving line is the file's — the persisted slug resolved to the built-in")
	}
	if strings.Contains(got.line, "does not resolve") {
		t.Errorf("advisory line = %q; a slug that resolves to the built-in earns no persisted line at all", got.line)
	}
}

func TestThemeAdvisoryUnion_ValidPersistedFileIsSilent(t *testing.T) {
	requireDropInSlug(t, "nord-frost")
	dir := themesDirWith(t, map[string][]byte{
		"nord-frost.theme": validThemeSource(t),
		"notes.txt":        []byte("not a theme file\n"),
	})

	requireNoAdvisories(t, unionAdvisoriesFor(t, `{"theme":"nord-frost"}`, dir))
}

func TestThemeAdvisoryUnion_BothSlotsStayOneLine(t *testing.T) {
	requireDropInSlug(t, "nord-frost")
	dir := themesDirWith(t, map[string][]byte{"nord-frost.theme": sourceBadColours(t, themeOverride{"canvas", "blue"})})

	got := requireOneAdvisory(t, unionAdvisoriesFor(t, `{"theme_light":"nord-frost","theme_dark":"nord-frost"}`, dir))

	if want := "⚠ theme nord-frost (both) does not resolve: bad colour"; got.line != want {
		t.Errorf("advisory line = %q; want %q", got.line, want)
	}
}

func TestThemeAdvisoryUnion_DirectoryLineIsNeverDeduped(t *testing.T) {
	requireDropInSlug(t, "nord-frost")
	dir := filepath.Join(t.TempDir(), "themes")
	if err := os.WriteFile(dir, []byte("not a directory\n"), 0o644); err != nil {
		t.Fatalf("seed %s: %v", dir, err)
	}

	requireAdvisoryLines(t, unionAdvisoriesFor(t, `{"theme":"nord-frost"}`, dir),
		"⚠ themes directory unreadable: "+dir,
		"⚠ theme nord-frost does not resolve: unreadable",
	)
}

func TestThemeAdvisoryUnion_HandsTheRendererLinesOnly(t *testing.T) {
	requireDropInSlug(t, "nord-frost")
	dir := themesDirWith(t, map[string][]byte{"nord-frost.theme": sourceBadColours(t, themeOverride{"canvas", "blue"})})
	deps := persistedThemeDeps(t, `{"theme_dark":"nord-frost"}`, dir)

	themeBlock := collectThemeAdvisories(deps)
	if want := []advisory{{line: "⚠ theme nord-frost (dark) does not resolve: bad colour"}}; !slices.Equal(themeBlock, want) {
		t.Fatalf("collected block = %+v; want %+v", themeBlock, want)
	}

	elsewhere := advisory{line: "⚠ theme nord-frost: reported by another producer"}
	lines := renderedLines(t, mixedCatalog(), append(themeBlock, elsewhere))

	want := []string{
		"  " + themeBlock[0].line,
		"  " + elsewhere.line,
		"  1 of 2 checks passed · 2 advisories",
	}
	if got := lines[len(lines)-3:]; !slices.Equal(got, want) {
		t.Errorf("report closes with\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

// `Nord.theme` sorts above the lowercase names, so the resulting order is
// provably os.ReadDir's rather than any tidier one.
func everyRegionFixture(t *testing.T) (*DoctorDeps, []string) {
	t.Helper()

	requireDropInSlug(t, "b-colour")
	requireDropInSlug(t, "gone")

	dir := themesDirWith(t, map[string][]byte{
		"Nord.theme":      validThemeSource(t),
		"a-missing.theme": sourceMissingTokens(t, "text.primary"),
		"b-colour.theme":  sourceBadColours(t, themeOverride{"canvas", "blue"}),
		"c-syntax.theme":  sourceDuplicateKeyAt(t, 12, "text.primary"),
		"d-valid.theme":   validThemeSource(t),
	})
	return persistedThemeDeps(t, `{"theme_light":"b-colour","theme_dark":"gone"}`, dir), []string{
		"⚠ theme file Nord.theme: slug must be lowercase letters, digits and hyphens",
		"⚠ theme a-missing: missing tokens — missing text.primary",
		"⚠ theme c-syntax: bad syntax — line 12: duplicate key text.primary",
		"⚠ theme b-colour (light) does not resolve: bad colour",
		"⚠ theme gone (dark) does not resolve: not found",
	}
}

func TestThemeAdvisoryUnion_OrderIsDeterministic(t *testing.T) {
	t.Run("the regions appear in the pinned order", func(t *testing.T) {
		deps, want := everyRegionFixture(t)

		got := themeAdvisoryUnion(deps)
		requireAdvisoryLines(t, got, want...)

		// The region boundary is the producers', not a coincidence of how these
		// particular slugs happen to sort.
		for i, a := range got {
			if wantFromPrefs := i >= 3; a.fromPrefs != wantFromPrefs {
				t.Errorf("advisory[%d] (%q) fromPrefs = %t; want %t — every file line precedes every persisted line", i, a.line, a.fromPrefs, wantFromPrefs)
			}
		}
	})

	t.Run("repeat runs over an unchanged directory and prefs are byte-identical", func(t *testing.T) {
		deps, want := everyRegionFixture(t)

		var first string
		for run := range 10 {
			var buf bytes.Buffer
			renderDoctorReport(&buf, mixedCatalog(), collectThemeAdvisories(deps))

			if run == 0 {
				requireAdvisoryLines(t, themeAdvisoryUnion(deps), want...)
				first = buf.String()
				continue
			}
			if got := buf.String(); got != first {
				t.Fatalf("run %d rendered a different report\n got: %q\nwant: %q", run, got, first)
			}
		}
	})

	t.Run("file lines follow the enumeration, not the creation order", func(t *testing.T) {
		// Seeded in reverse-alphabetical order, so a scan that reported files as it
		// found them — or as a map yielded them — could not produce the order below.
		dir := t.TempDir()
		for _, name := range []string{"d-fourth.theme", "c-third.theme", "b-second.theme", "a-first.theme"} {
			if err := os.WriteFile(filepath.Join(dir, name), sourceMissingTokens(t, "text.primary"), 0o644); err != nil {
				t.Fatalf("seed %s: %v", name, err)
			}
		}

		requireAdvisoryLines(t, unionAdvisoriesFor(t, `{}`, dir),
			"⚠ theme a-first: missing tokens — missing text.primary",
			"⚠ theme b-second: missing tokens — missing text.primary",
			"⚠ theme c-third: missing tokens — missing text.primary",
			"⚠ theme d-fourth: missing tokens — missing text.primary",
		)
	})

	t.Run("the assembly iterates no map", func(t *testing.T) {
		// A two-element map would agree with itself often enough for a repeat-run
		// comparison to pass by luck, so the determinism is made structural: no
		// map type anywhere in the assembly.
		assembly := []string{"collectThemeAdvisories", "themeAdvisoryUnion", "assembleThemeAdvisories", "persistedSlugs"}

		source := parsePackageFilesByName(t)["doctor_theme.go"]
		if source == nil {
			t.Fatal("the cmd package declares no doctor_theme.go — the guard has nothing to scan")
		}

		found := map[string]bool{}
		for _, decl := range source.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || !slices.Contains(assembly, fn.Name.Name) {
				continue
			}
			found[fn.Name.Name] = true
			ast.Inspect(fn, func(n ast.Node) bool {
				if _, ok := n.(*ast.MapType); ok {
					t.Errorf("%s declares a map; the assembly's order must not depend on how one is walked", fn.Name.Name)
				}
				return true
			})
		}
		for _, name := range assembly {
			if !found[name] {
				t.Errorf("doctor_theme.go declares no %s — the guard scanned nothing", name)
			}
		}
	})
}

func TestThemeAdvisoryUnion_CountMatchesRenderedLines(t *testing.T) {
	requireDropInSlug(t, "nord-frost")

	cases := []struct {
		name  string
		prefs string
		files map[string][]byte
		want  []string
	}{
		{
			name:  "nothing to report",
			prefs: `{}`,
			files: map[string][]byte{"nord-frost.theme": validThemeSource(t)},
		},
		{
			name:  "a persisted line covering a file line",
			prefs: `{"theme_dark":"nord-frost"}`,
			files: map[string][]byte{"nord-frost.theme": sourceBadColours(t, themeOverride{"canvas", "blue"})},
			want:  []string{"⚠ theme nord-frost (dark) does not resolve: bad colour"},
		},
		{
			name:  "two lines sharing the empty slug",
			prefs: `{}`,
			files: map[string][]byte{"A-Nord.theme": validThemeSource(t), "B_Nord.theme": validThemeSource(t)},
			want: []string{
				"⚠ theme file A-Nord.theme: slug must be lowercase letters, digits and hyphens",
				"⚠ theme file B_Nord.theme: slug must be lowercase letters, digits and hyphens",
			},
		},
		{
			name:  "a file line and a persisted line for different slugs",
			prefs: `{"theme":"gone"}`,
			files: map[string][]byte{"nord-frost.theme": sourceBadColours(t, themeOverride{"canvas", "blue"})},
			want: []string{
				"⚠ theme nord-frost: bad colour — canvas = blue",
				"⚠ theme gone does not resolve: not found",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := persistedThemeDeps(t, tc.prefs, themesDirWith(t, tc.files))
			advisories := themeAdvisoryUnion(deps)
			requireAdvisoryLines(t, advisories, tc.want...)

			lines := renderedLines(t, mixedCatalog(), collectThemeAdvisories(deps))

			var rendered []string
			for _, line := range lines {
				if strings.HasPrefix(line, "  ⚠") {
					rendered = append(rendered, line)
				}
			}
			if len(rendered) != len(advisories) {
				t.Fatalf("report rendered %d advisory lines from %d advisories:\n%s", len(rendered), len(advisories), strings.Join(lines, "\n"))
			}

			want := "  1 of 2 checks passed"
			switch len(rendered) {
			case 0:
				// Deliberately empty: no suffix at all.
			case 1:
				want += " · 1 advisory"
			default:
				want += fmt.Sprintf(" · %d advisories", len(rendered))
			}
			if got := lines[len(lines)-1]; got != want {
				t.Errorf("rendered summary = %q; want %q", got, want)
			}
		})
	}

	t.Run("two lines sharing a slug are counted twice", func(t *testing.T) {
		// A same-slug pair: counted as distinct slugs rather than lines, this
		// reads 1 while printing two.
		deps := persistedThemeDeps(t, `{}`, themesDirWith(t, map[string][]byte{
			"A-Nord.theme": validThemeSource(t),
			"B_Nord.theme": validThemeSource(t),
		}))
		advisories := themeAdvisoryUnion(deps)
		if len(advisories) != 2 {
			t.Fatalf("union produced %d advisories, want 2:\n  %s", len(advisories), strings.Join(advisoryLines(advisories), "\n  "))
		}
		for i, a := range advisories {
			if a.slug != "" {
				t.Fatalf("advisory[%d].slug = %q; both lines must carry the same (empty) slug for this to be the collision case", i, a.slug)
			}
		}

		if got, want := doctorSummaryLine(mixedCatalog(), collectThemeAdvisories(deps)), "1 of 2 checks passed · 2 advisories"; got != want {
			t.Errorf("doctorSummaryLine = %q; want %q — <M> counts lines, not distinct slugs", got, want)
		}
	})
}
