package cmd

import (
	"fmt"
	"go/ast"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/themetest"
)

// Names are filenames rather than slugs, so a fixture can carry a non-`.theme`
// file the enumeration must ignore entirely.
func themesDirWith(t *testing.T, files map[string][]byte) string {
	t.Helper()

	return themesDirIn(t, t.TempDir(), files)
}

func themesDirIn(t *testing.T, parent string, files map[string][]byte) string {
	t.Helper()

	dir := filepath.Join(parent, "themes")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	return dir
}

// The assembled form is the last point the union's identity fields are
// observable.
func themeAdvisoriesFor(t *testing.T, dir string) []themeAdvisory {
	t.Helper()

	return themeAdvisoryUnion(&DoctorDeps{ThemesDir: dir})
}

func advisoryLines(advisories []themeAdvisory) []string {
	lines := make([]string, 0, len(advisories))
	for _, a := range advisories {
		lines = append(lines, a.line)
	}
	return lines
}

func requireOneAdvisory(t *testing.T, advisories []themeAdvisory) themeAdvisory {
	t.Helper()

	if len(advisories) != 1 {
		t.Fatalf("scan produced %d advisories, want exactly 1:\n  %s", len(advisories), strings.Join(advisoryLines(advisories), "\n  "))
	}
	return advisories[0]
}

func TestThemeAdvisories_InvalidFileFrame(t *testing.T) {
	cases := []struct {
		name   string
		slug   string
		source func(*testing.T) []byte
		want   string
	}{
		{
			name:   "missing tokens names every absent token in canonical order",
			slug:   "mine",
			source: func(t *testing.T) []byte { return sourceMissingTokens(t, "text.primary", "bg.subtle") },
			want:   "⚠ theme mine: missing tokens — missing text.primary, bg.subtle",
		},
		{
			name: "bad colour names every offending pair in file order",
			slug: "nord-lee",
			source: func(t *testing.T) []byte {
				return sourceBadColours(t, themeOverride{"text.primary", "#GGGGGG"}, themeOverride{"canvas", "blue"})
			},
			want: "⚠ theme nord-lee: bad colour — text.primary = #GGGGGG, canvas = blue",
		},
		{
			name:   "bad syntax names the second occurrence's line",
			slug:   "dupes",
			source: func(t *testing.T) []byte { return sourceDuplicateKeyAt(t, 12, "text.primary") },
			want:   "⚠ theme dupes: bad syntax — line 12: duplicate key text.primary",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := themesDirWith(t, map[string][]byte{tc.slug + ".theme": tc.source(t)})

			got := requireOneAdvisory(t, themeAdvisoriesFor(t, dir))
			if got.line != tc.want {
				t.Errorf("advisory line = %q; want %q", got.line, tc.want)
			}
		})
	}
}

func TestThemeAdvisories_UnreadableFileKeepsOSError(t *testing.T) {
	t.Run("a mode-0000 file", func(t *testing.T) {
		dir := themesDirWith(t, map[string][]byte{"mine.theme": validThemeSource(t)})
		path := filepath.Join(dir, "mine.theme")

		assertUnreadableAdvisory(t, dir, "mine", themetest.DenyRead(t, path))
	})

	t.Run("a dangling symlink", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "mine.theme")
		if err := os.Symlink(filepath.Join(dir, "no-such-target"), path); err != nil {
			t.Fatalf("symlink %s: %v", path, err)
		}

		assertUnreadableAdvisory(t, dir, "mine", osReadError(t, path))
	})
}

func assertUnreadableAdvisory(t *testing.T, dir, slug string, osErr error) {
	t.Helper()

	got := requireOneAdvisory(t, themeAdvisoriesFor(t, dir))
	want := fmt.Sprintf("⚠ theme %s: unreadable — %s", slug, osErr)
	if got.line != want {
		t.Errorf("advisory line = %q; want %q", got.line, want)
	}
	if n := strings.Count(got.line, osErr.Error()); n != 1 {
		t.Errorf("advisory line = %q carries the OS error %d times, want exactly 1", got.line, n)
	}
}

func TestThemeAdvisories_ValidFileIsSilent(t *testing.T) {
	dir := themesDirWith(t, map[string][]byte{
		"mine.theme":     validThemeSource(t),
		"nord-lee.theme": validThemeSource(t),
		"notes.txt":      []byte("text.primary = not a theme file\n"),
		"README.md":      []byte("# themes\n"),
	})

	if got := themeAdvisoriesFor(t, dir); len(got) != 0 {
		t.Errorf("scan produced %d advisories over a valid directory, want none:\n  %s", len(got), strings.Join(advisoryLines(got), "\n  "))
	}
}

func TestThemeAdvisories_AbsentDirectoryIsSilent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "themes")

	sink := logtest.Install(t)

	if got := themeAdvisoriesFor(t, dir); len(got) != 0 {
		t.Errorf("scan produced %d advisories over an absent directory, want none:\n  %s", len(got), strings.Join(advisoryLines(got), "\n  "))
	}
	if records := sink.Records(); len(records) != 0 {
		t.Errorf("scan emitted %d log records over an absent directory, want none: %+v", len(records), records)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("os.Stat(%s) = %v; the scan must never create the themes directory", dir, err)
	}
}

func TestThemeAdvisories_UnusableDirectoryLine(t *testing.T) {
	cases := []struct {
		name string
		make func(*testing.T) string
	}{
		{
			name: "a mode-0000 directory",
			make: func(t *testing.T) string {
				dir := themesDirWith(t, map[string][]byte{"mine.theme": sourceMissingTokens(t, "canvas")})
				_ = themetest.DenyDir(t, dir)
				return dir
			},
		},
		{
			name: "a regular file where the directory belongs",
			make: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "themes")
				if err := os.WriteFile(path, sourceMissingTokens(t, "canvas"), 0o644); err != nil {
					t.Fatalf("seed %s: %v", path, err)
				}
				return path
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := tc.make(t)

			got := requireOneAdvisory(t, themeAdvisoriesFor(t, dir))
			if want := "⚠ themes directory unreadable: " + dir; got.line != want {
				t.Errorf("advisory line = %q; want %q", got.line, want)
			}
			if got.slug != "" {
				t.Errorf("directory advisory slug = %q; the line is about a path, not a theme", got.slug)
			}
			if got.fromPrefs {
				t.Error("directory advisory fromPrefs = true; the line comes from the directory, not from prefs.json")
			}
		})
	}
}

func TestThemeAdvisories_UnresolvedDirDegrades(t *testing.T) {
	t.Run("an empty themes directory scans nothing", func(t *testing.T) {
		if got := themeAdvisoriesFor(t, ""); len(got) != 0 {
			t.Errorf("scan produced %d advisories over an unresolved path, want none:\n  %s", len(got), strings.Join(advisoryLines(got), "\n  "))
		}
	})

	t.Run("the diagnosis still renders in full", func(t *testing.T) {
		// resolveDoctorDeps' own themesDirPath() call is under test, so the
		// deps carry no ThemesDir override.
		deps := healthyDoctorDeps(t)
		unresolvableThemesDir(t)

		outBuf, _, err := runDoctorWith(t, deps)
		if err != nil {
			t.Fatalf("Execute err = %v; an unresolvable themes directory must never abort the diagnosis", err)
		}

		out := outBuf.String()
		if strings.Contains(out, "⚠") {
			t.Errorf("report carries an advisory line with no themes directory to scan:\n%s", out)
		}
		if !strings.HasSuffix(out, "\n  7 checks passed\n") {
			t.Errorf("report does not close with the full all-passed summary:\n%s", out)
		}
	})
}

func TestThemeAdvisories_DetailIsVerbatim(t *testing.T) {
	sources := map[string][]byte{
		"missing.theme": sourceMissingTokens(t, "text.primary", "bg.subtle"),
		"colour.theme":  sourceBadColours(t, themeOverride{"canvas", "blue"}),
		"syntax.theme":  sourceDuplicateKeyAt(t, 12, "text.primary"),
		"denied.theme":  validThemeSource(t),
	}
	dir := themesDirWith(t, sources)
	_ = themetest.DenyRead(t, filepath.Join(dir, "denied.theme"))

	loader := theme.NewSilentLoader()
	byLine := map[string]string{}
	for _, a := range themeAdvisoriesFor(t, dir) {
		byLine[a.slug] = a.line
	}

	for _, name := range slices.Sorted(maps.Keys(sources)) {
		slug := strings.TrimSuffix(name, ".theme")
		t.Run(slug, func(t *testing.T) {
			_, rejection := loader.LoadFile(filepath.Join(dir, name))
			if rejection == nil {
				t.Fatalf("%s loaded cleanly — the verbatim assertion over it would be vacuous", name)
			}
			if rejection.Detail == "" {
				t.Fatalf("%s rejected with an empty detail — the assertion would be vacuous", name)
			}

			want := fmt.Sprintf("⚠ theme %s: %s — %s", slug, rejection.Reason, rejection.Detail)
			if got := byLine[slug]; got != want {
				t.Errorf("advisory line = %q; want %q — the loader's own reason and detail, carried through", got, want)
			}
		})
	}
}

func TestThemeAdvisories_OneReasonPerFile(t *testing.T) {
	t.Run("a doubly-broken file reports the ladder's first reason only", func(t *testing.T) {
		lines := missingTokenLines(t, badColourLines(t, themeKeyLines(t), themeOverride{"canvas", "blue"}), "bg.subtle")
		dir := themesDirWith(t, map[string][]byte{"mine.theme": themetest.Render(lines)})

		got := requireOneAdvisory(t, themeAdvisoriesFor(t, dir))
		if want := "⚠ theme mine: bad colour — canvas = blue"; got.line != want {
			t.Errorf("advisory line = %q; want %q", got.line, want)
		}
		if strings.Contains(got.line, "missing") {
			t.Errorf("advisory line = %q; a `bad colour` line must say nothing about presence", got.line)
		}
	})

	t.Run("a full reject set reports one line per file", func(t *testing.T) {
		dir := themesDirWith(t, map[string][]byte{
			"a-missing.theme": sourceMissingTokens(t, "text.primary"),
			"b-colour.theme":  sourceBadColours(t, themeOverride{"canvas", "blue"}),
			"c-syntax.theme":  sourceDuplicateKeyAt(t, 12, "text.primary"),
			"d-valid.theme":   validThemeSource(t),
		})

		got := themeAdvisoriesFor(t, dir)
		want := []string{
			"⚠ theme a-missing: missing tokens — missing text.primary",
			"⚠ theme b-colour: bad colour — canvas = blue",
			"⚠ theme c-syntax: bad syntax — line 12: duplicate key text.primary",
		}
		if !slices.Equal(advisoryLines(got), want) {
			t.Errorf("advisory lines =\n  %s\nwant\n  %s", strings.Join(advisoryLines(got), "\n  "), strings.Join(want, "\n  "))
		}
	})
}

func TestThemeAdvisories_FileLinesCarryTheirSlug(t *testing.T) {
	dir := themesDirWith(t, map[string][]byte{
		"a-missing.theme": sourceMissingTokens(t, "text.primary"),
		"b-colour.theme":  sourceBadColours(t, themeOverride{"canvas", "blue"}),
		"c-syntax.theme":  sourceDuplicateKeyAt(t, 12, "text.primary"),
	})

	got := themeAdvisoriesFor(t, dir)
	if len(got) != 3 {
		t.Fatalf("scan produced %d advisories, want 3:\n  %s", len(got), strings.Join(advisoryLines(got), "\n  "))
	}

	wantSlugs := []string{"a-missing", "b-colour", "c-syntax"}
	for i, a := range got {
		if a.slug != wantSlugs[i] {
			t.Errorf("advisory[%d].slug = %q; want %q", i, a.slug, wantSlugs[i])
		}
		if a.fromPrefs {
			t.Errorf("advisory[%d].fromPrefs = true; a file line never comes from prefs.json", i)
		}
		if !strings.Contains(a.line, wantSlugs[i]) {
			t.Errorf("advisory[%d].line = %q; want it to name the slug %q", i, a.line, wantSlugs[i])
		}
	}
}

func TestThemeAdvisories_EmitsNoThemeRecords(t *testing.T) {
	t.Run("a full reject set writes nothing", func(t *testing.T) {
		dir := themesDirWith(t, map[string][]byte{
			"a-missing.theme": sourceMissingTokens(t, "text.primary"),
			"b-colour.theme":  sourceBadColours(t, themeOverride{"canvas", "blue"}),
			"c-syntax.theme":  sourceDuplicateKeyAt(t, 12, "text.primary"),
			"d-denied.theme":  validThemeSource(t),
			"e-valid.theme":   validThemeSource(t),
		})
		_ = themetest.DenyRead(t, filepath.Join(dir, "d-denied.theme"))

		records := assertNoThemeRecords(t, func() {
			if got := themeAdvisoriesFor(t, dir); len(got) != 4 {
				t.Fatalf("scan produced %d advisories, want 4 — the zero-record assertion needs a full reject set:\n  %s", len(got), strings.Join(advisoryLines(got), "\n  "))
			}
		})

		if len(records) != 0 {
			t.Errorf("scan emitted %d log records, want none: %+v", len(records), records)
		}
	})

	t.Run("an unusable directory writes nothing", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "themes")
		if err := os.WriteFile(path, []byte("not a directory\n"), 0o644); err != nil {
			t.Fatalf("seed %s: %v", path, err)
		}

		records := assertNoThemeRecords(t, func() {
			requireOneAdvisory(t, themeAdvisoriesFor(t, path))
		})

		if len(records) != 0 {
			t.Errorf("scan emitted %d log records over an unusable directory, want none: %+v", len(records), records)
		}
	})
}

func TestThemeAdvisories_ScanIsReadOnly(t *testing.T) {
	root := t.TempDir()
	files := map[string][]byte{
		"a-missing.theme": sourceMissingTokens(t, "text.primary"),
		"b-colour.theme":  sourceBadColours(t, themeOverride{"canvas", "blue"}),
		"c-syntax.theme":  sourceDuplicateKeyAt(t, 12, "text.primary"),
		"d-valid.theme":   validThemeSource(t),
		"notes.txt":       []byte("not a theme file\n"),
	}
	dir := themesDirIn(t, root, files)
	prefsPath := filepath.Join(root, "prefs.json")
	if err := os.WriteFile(prefsPath, []byte(`{"session_list_mode":"by-tag","appearance":"light"}`), 0o600); err != nil {
		t.Fatalf("seed prefs.json: %v", err)
	}
	t.Setenv("PORTAL_PREFS_FILE", prefsPath)

	before := treeFingerprint(t, root)
	if len(before) < len(files) {
		t.Fatalf("snapshot holds %d entries over %d seeded files — the comparison would be vacuous", len(before), len(files))
	}

	if got := themeAdvisoriesFor(t, dir); len(got) != 3 {
		t.Fatalf("scan produced %d advisories, want 3:\n  %s", len(got), strings.Join(advisoryLines(got), "\n  "))
	}

	assertTreeUnchanged(t, root, before, "the config tree changed")

	// Through a real store, never themeAdvisoriesFor's nil one: a nil PrefsStore
	// returns before any prefs code runs, so the assertion would hold however the
	// read behaved.
	t.Run("it creates no prefs.json when there is none", func(t *testing.T) {
		absent := filepath.Join(t.TempDir(), "prefs.json")
		t.Setenv("PORTAL_PREFS_FILE", absent)

		store, err := loadPrefsStoreNoMigrate()
		if err != nil {
			t.Fatalf("loadPrefsStoreNoMigrate: %v", err)
		}
		themeAdvisoryUnion(&DoctorDeps{ThemesDir: dir, PrefsStore: store})

		if _, err := os.Stat(absent); !os.IsNotExist(err) {
			t.Errorf("os.Stat(%s) = %v; the scan must never create prefs.json", absent, err)
		}
	})
}

func TestThemeAdvisories_ReachTheDoctorReport(t *testing.T) {
	deps := healthyDoctorDeps(t)
	deps.ThemesDir = themesDirWith(t, map[string][]byte{
		"a-missing.theme": sourceMissingTokens(t, "text.primary"),
		"b-colour.theme":  sourceBadColours(t, themeOverride{"canvas", "blue"}),
		"c-valid.theme":   validThemeSource(t),
	})

	outBuf, _, err := runDoctorWith(t, deps)
	if err != nil {
		t.Fatalf("Execute err = %v; a broken drop-in must never drive the exit code", err)
	}

	out := outBuf.String()
	want := "" +
		"  ⚠ theme a-missing: missing tokens — missing text.primary\n" +
		"  ⚠ theme b-colour: bad colour — canvas = blue\n" +
		"  7 checks passed · 2 advisories\n"
	if !strings.HasSuffix(out, want) {
		t.Errorf("report does not close with the advisory block and its summary:\n%s", out)
	}

	t.Run("the resolved themes directory is scanned with no override", func(t *testing.T) {
		t.Setenv("PORTAL_THEMES_DIR", themesDirWith(t, map[string][]byte{
			"a-missing.theme": sourceMissingTokens(t, "text.primary"),
		}))

		// No ThemesDir on the deps: the field can only arrive through the
		// production resolution.
		outBuf, _, err := runDoctorWith(t, healthyDoctorDeps(t))
		if err != nil {
			t.Fatalf("Execute err = %v; a broken drop-in must never drive the exit code", err)
		}

		out := outBuf.String()
		want := "" +
			"  ⚠ theme a-missing: missing tokens — missing text.primary\n" +
			"  7 checks passed · 1 advisory\n"
		if !strings.HasSuffix(out, want) {
			t.Errorf("report does not close with the resolved directory's advisory:\n%s", out)
		}
	})
}

// A `nord.theme` beside no built-in `nord` is an ordinary valid drop-in
// producing no line at all.
func requireBuiltinSlug(t *testing.T, slug string) {
	t.Helper()

	if !slices.Contains(theme.BuiltinSlugs(), slug) {
		t.Fatalf("%q is not a built-in slug (the embedded set is %v) — a fixture colliding with it would prove nothing", slug, theme.BuiltinSlugs())
	}
}

func reservedNameLine(filename, slug string) string {
	return "⚠ theme file " + filename + ": " + slug + " is a built-in — rename it (e.g. " + slug + "-mine.theme)"
}

func TestThemeAdvisories_BadNameSlugFrame(t *testing.T) {
	cases := []struct{ name, filename string }{
		{name: "an uppercase stem", filename: "Nord.theme"},
		{name: "an underscore", filename: "nord_lee.theme"},
		{name: "a space", filename: "nord lee.theme"},
		{name: "a leading hyphen", filename: "-nord.theme"},
		{name: "an empty stem", filename: ".theme"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := themesDirWith(t, map[string][]byte{tc.filename: validThemeSource(t)})

			got := requireOneAdvisory(t, themeAdvisoriesFor(t, dir))
			want := "⚠ theme file " + tc.filename + ": slug must be lowercase letters, digits and hyphens"
			if got.line != want {
				t.Errorf("advisory line = %q; want %q", got.line, want)
			}
		})
	}
}

// Each case gets its own directory: the two names fold together on a
// case-insensitive filesystem.
func TestThemeAdvisories_BadNameExtensionFrame(t *testing.T) {
	cases := []struct{ name, filename string }{
		{name: "a shouted extension", filename: "nord.THEME"},
		{name: "a title-cased extension", filename: "nord.Theme"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := themesDirWith(t, map[string][]byte{tc.filename: validThemeSource(t)})

			got := requireOneAdvisory(t, themeAdvisoriesFor(t, dir))
			want := "⚠ theme file " + tc.filename + ": extension must be lowercase .theme"
			if got.line != want {
				t.Errorf("advisory line = %q; want %q", got.line, want)
			}
		})
	}
}

func TestThemeAdvisories_DoublyIllegalNameRendersTheSlugLine(t *testing.T) {
	cases := []struct{ name, filename string }{
		{name: "an uppercase stem", filename: "Nord.THEME"},
		{name: "a space in the stem", filename: "My Theme.THEME"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := themesDirWith(t, map[string][]byte{tc.filename: validThemeSource(t)})

			got := requireOneAdvisory(t, themeAdvisoriesFor(t, dir))
			want := "⚠ theme file " + tc.filename + ": slug must be lowercase letters, digits and hyphens"
			if got.line != want {
				t.Errorf("advisory line = %q; want %q", got.line, want)
			}
		})
	}
}

// The reserved row is labelled by filename despite having a valid slug: that
// slug is identical to the built-in's, so labelling by slug would print the
// same name twice with no way to tell which row is the user's file.
func TestThemeAdvisories_FilenameReasonsLabelledByFilename(t *testing.T) {
	requireBuiltinSlug(t, "nord")
	dir := themesDirWith(t, map[string][]byte{
		"a-colour.theme":  sourceBadColours(t, themeOverride{"canvas", "blue"}),
		"b_bad.theme":     validThemeSource(t),
		"c-missing.theme": sourceMissingTokens(t, "text.primary"),
		"d-ext.THEME":     validThemeSource(t),
		"nord.theme":      validThemeSource(t),
	})

	got := advisoryLines(themeAdvisoriesFor(t, dir))
	want := []string{
		"⚠ theme a-colour: bad colour — canvas = blue",
		"⚠ theme file b_bad.theme: slug must be lowercase letters, digits and hyphens",
		"⚠ theme c-missing: missing tokens — missing text.primary",
		"⚠ theme file d-ext.THEME: extension must be lowercase .theme",
		"⚠ theme file nord.theme: nord is a built-in — rename it (e.g. nord-mine.theme)",
	}
	if !slices.Equal(got, want) {
		t.Errorf("advisory lines =\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

func TestThemeAdvisories_ReservedNameFrame(t *testing.T) {
	requireBuiltinSlug(t, "nord")
	dir := themesDirWith(t, map[string][]byte{"nord.theme": sourceMissingTokens(t, "text.primary")})

	got := requireOneAdvisory(t, themeAdvisoriesFor(t, dir))
	want := "⚠ theme file nord.theme: nord is a built-in — rename it (e.g. nord-mine.theme)"
	if got.line != want {
		t.Errorf("advisory line = %q; want %q", got.line, want)
	}
	if strings.Contains(got.line, string(theme.ReasonReservedName)) {
		t.Errorf("advisory line = %q carries the terse reason label; the reserved-name line names the conflict and the fix INSTEAD of following the generic `<reason> — <detail>` frame", got.line)
	}
	if got.slug != "nord" {
		t.Errorf("advisory slug = %q; want %q — a reserved-name entry has a valid slug, and it is what collided", got.slug, "nord")
	}
	if got.fromPrefs {
		t.Error("advisory fromPrefs = true; a file line never comes from prefs.json")
	}
}

func TestThemeAdvisories_ReservedSetIsTheEmbeddedSet(t *testing.T) {
	slugs := theme.BuiltinSlugs()
	if len(slugs) == 0 {
		t.Fatal("the embedded set is empty — every assertion below would be vacuous")
	}

	files := map[string][]byte{}
	for _, slug := range slugs {
		files[slug+".theme"] = validThemeSource(t)
	}
	dir := themesDirWith(t, files)

	got := themeAdvisoriesFor(t, dir)
	if len(got) != len(slugs) {
		t.Fatalf("scan produced %d advisories over %d built-in collisions, want one each:\n  %s", len(got), len(slugs), strings.Join(advisoryLines(got), "\n  "))
	}

	lines := advisoryLines(got)
	for _, slug := range slugs {
		if want := reservedNameLine(slug+".theme", slug); !slices.Contains(lines, want) {
			t.Errorf("scan produced no reserved-name line for the built-in %q:\n  %s\nwant it to carry\n  %s", slug, strings.Join(lines, "\n  "), want)
		}
	}
}

func TestThemeAdvisories_BadNameNeverReportsContent(t *testing.T) {
	lines := duplicateKeyLines(t, badColourLines(t, themeKeyLines(t), themeOverride{"canvas", "blue"}), "text.primary", len(themeKeyLines(t))+1)

	dir := themesDirWith(t, map[string][]byte{"Bad_Name.theme": themetest.Render(lines)})
	_ = themetest.DenyRead(t, filepath.Join(dir, "Bad_Name.theme"))

	got := requireOneAdvisory(t, themeAdvisoriesFor(t, dir))
	want := "⚠ theme file Bad_Name.theme: slug must be lowercase letters, digits and hyphens"
	if got.line != want {
		t.Errorf("advisory line = %q; want %q", got.line, want)
	}
	for _, reason := range []theme.Reason{theme.ReasonUnreadable, theme.ReasonBadSyntax, theme.ReasonBadColour, theme.ReasonMissingTokens} {
		if strings.Contains(got.line, string(reason)) {
			t.Errorf("advisory line = %q reports %q; the filename is decided before the file is opened, so a `bad name` file can never report on its contents", got.line, reason)
		}
	}
}

func TestThemeAdvisories_BadNameCarriesNoSlug(t *testing.T) {
	cases := []struct{ name, filename string }{
		{name: "the slug cause", filename: "Nord.theme"},
		{name: "the extension cause", filename: "nord.THEME"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := themesDirWith(t, map[string][]byte{tc.filename: validThemeSource(t)})

			got := requireOneAdvisory(t, themeAdvisoriesFor(t, dir))
			if got.slug != "" {
				t.Errorf("advisory slug = %q; a `bad name` file yields no slug at all, and the union must never dedup a persisted slug against one", got.slug)
			}
			if got.fromPrefs {
				t.Error("advisory fromPrefs = true; a file line never comes from prefs.json")
			}
		})
	}
}

// Only a hand-built entry separates stated from copied: every enumerated
// `bad name` entry already carries an empty Slug.
func TestThemeAdvisories_BadNameSlugIsStatedNotCopied(t *testing.T) {
	entry := theme.Entry{
		Filename:  "Bad_Name.theme",
		Slug:      "bad-name",
		Rejection: &theme.Rejection{Reason: theme.ReasonBadName, BadNameCause: theme.BadNameSlug},
	}

	got, reported := themeFileAdvisory(entry)
	if !reported {
		t.Fatal("themeFileAdvisory reported nothing for a `bad name` entry")
	}
	if got.slug != "" {
		t.Errorf("advisory slug = %q; want %q — the emptiness is a consequence of the reason, not a copy of the entry's field", got.slug, "")
	}
	if want := "⚠ theme file Bad_Name.theme: slug must be lowercase letters, digits and hyphens"; got.line != want {
		t.Errorf("advisory line = %q; want %q", got.line, want)
	}
}

func TestThemeAdvisories_NotFoundIsNeverAFileLine(t *testing.T) {
	entry := theme.Entry{
		Filename:  "gone.theme",
		Slug:      "gone",
		Rejection: &theme.Rejection{Reason: theme.ReasonNotFound},
	}

	if got, reported := themeFileAdvisory(entry); reported {
		t.Errorf("themeFileAdvisory reported %q for a `not found` entry; the persisted producer owns that reason", got.line)
	}
}

func TestThemeAdvisories_ReservedNameDecidedBeforeRead(t *testing.T) {
	requireBuiltinSlug(t, "nord")
	want := reservedNameLine("nord.theme", "nord")

	t.Run("contents that are perfectly valid", func(t *testing.T) {
		dir := themesDirWith(t, map[string][]byte{"nord.theme": validThemeSource(t)})

		got := requireOneAdvisory(t, themeAdvisoriesFor(t, dir))
		if got.line != want {
			t.Errorf("advisory line = %q; want %q", got.line, want)
		}
	})

	t.Run("contents that cannot be read at all", func(t *testing.T) {
		dir := themesDirWith(t, map[string][]byte{"nord.theme": validThemeSource(t)})
		_ = themetest.DenyRead(t, filepath.Join(dir, "nord.theme"))

		got := requireOneAdvisory(t, themeAdvisoriesFor(t, dir))
		if got.line != want {
			t.Errorf("advisory line = %q; want %q", got.line, want)
		}
	})
}

func TestThemeAdvisories_FilenameFramesAreSingleSourced(t *testing.T) {
	fragments := []string{
		"slug must be lowercase letters, digits and hyphens",
		"extension must be lowercase .theme",
		"is a built-in — rename it",
	}

	for _, fragment := range fragments {
		t.Run(fragment, func(t *testing.T) {
			sites := cmdLiteralSites(t, fragment)

			if want := map[string]int{"doctor_theme.go": 1}; !maps.Equal(sites, want) {
				t.Errorf("the literal %q is declared at %v; want %v — one const in the file that owns doctor's theme copy", fragment, sites, want)
			}
		})
	}
}

func cmdLiteralSites(t *testing.T, fragment string) map[string]int {
	t.Helper()

	sites := map[string]int{}
	for name, file := range parsePackageFilesByName(t) {
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			value, err := strconv.Unquote(lit.Value)
			if err == nil && strings.Contains(value, fragment) {
				sites[name]++
			}
			return true
		})
	}
	return sites
}
