package cmd

import (
	"encoding/json"
	"go/ast"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/sourceguardtest"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/themetest"
)

func prefsJSONWith(t *testing.T, keys map[string]string) string {
	t.Helper()

	data, err := json.Marshal(keys)
	if err != nil {
		t.Fatalf("marshal prefs fixture %v: %v", keys, err)
	}
	return string(data)
}

// An empty content leaves the prefs file absent.
func persistedThemeDeps(t *testing.T, content, themesDir string) *DoctorDeps {
	t.Helper()

	setPrefsFile(t, content)
	store, err := loadPrefsStoreNoMigrate()
	if err != nil {
		t.Fatalf("loadPrefsStoreNoMigrate: %v", err)
	}
	return &DoctorDeps{PrefsStore: store, ThemesDir: themesDir}
}

func persistedAdvisoriesFor(t *testing.T, content, themesDir string) []themeAdvisory {
	t.Helper()

	return persistedAdvisoriesUnder(t, persistedThemeDeps(t, content, themesDir), theme.NewSilentLoader())
}

func persistedAdvisoriesUnder(t *testing.T, deps *DoctorDeps, loader theme.Loader) []themeAdvisory {
	t.Helper()

	return persistedThemeAdvisories(deps, loader, loader.OpenEnumeration(deps.ThemesDir))
}

func requireNoAdvisories(t *testing.T, advisories []themeAdvisory) {
	t.Helper()

	if len(advisories) != 0 {
		t.Fatalf("producer emitted %d advisories, want none:\n  %s", len(advisories), strings.Join(advisoryLines(advisories), "\n  "))
	}
}

// A built-in resolves from the embedded set with no directory involved.
func requireDropInSlug(t *testing.T, slug string) {
	t.Helper()

	for _, builtin := range theme.BuiltinSlugs() {
		if builtin == slug {
			t.Fatalf("%q is a built-in slug (the embedded set is %v) — it resolves from the embedded set, so an unresolvable-slug assertion over it would prove nothing", slug, theme.BuiltinSlugs())
		}
	}
}

func TestPersistedThemeAdvisory_ConstantOmitsSlot(t *testing.T) {
	requireDropInSlug(t, "nord-lee")

	got := requireOneAdvisory(t, persistedAdvisoriesFor(t, `{"theme":"nord-lee"}`, t.TempDir()))

	const want = "⚠ theme nord-lee does not resolve: not found"
	if got.line != want {
		t.Errorf("advisory line = %q; want %q", got.line, want)
	}
	if strings.Contains(got.line, "(") {
		t.Errorf("advisory line = %q carries a slot parenthetical; a constant setting has no slots", got.line)
	}
	if got.slug != "nord-lee" {
		t.Errorf("advisory slug = %q; want %q — the advisory union dedups on it", got.slug, "nord-lee")
	}
	if !got.fromPrefs {
		t.Error("advisory fromPrefs = false; this line comes from prefs.json and outranks a file line for the same slug")
	}
}

func TestPersistedThemeAdvisory_SlotRendersLightOrDark(t *testing.T) {
	requireDropInSlug(t, "solar")

	cases := []struct{ name, content, want string }{
		{
			name:    "the light slot",
			content: `{"theme_light":"solar"}`,
			want:    "⚠ theme solar (light) does not resolve: not found",
		},
		{
			name:    "the dark slot",
			content: `{"theme_dark":"solar"}`,
			want:    "⚠ theme solar (dark) does not resolve: not found",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := requireOneAdvisory(t, persistedAdvisoriesFor(t, tc.content, t.TempDir()))
			if got.line != tc.want {
				t.Errorf("advisory line = %q; want %q", got.line, tc.want)
			}
			if !got.fromPrefs {
				t.Error("advisory fromPrefs = false; this line comes from prefs.json")
			}
		})
	}
}

func TestPersistedThemeAdvisory_BothSlots(t *testing.T) {
	t.Run("one slug in both slots collapses to a single line", func(t *testing.T) {
		requireDropInSlug(t, "solar")

		got := requireOneAdvisory(t, persistedAdvisoriesFor(t, `{"theme_light":"solar","theme_dark":"solar"}`, t.TempDir()))

		const want = "⚠ theme solar (both) does not resolve: not found"
		if got.line != want {
			t.Errorf("advisory line = %q; want %q", got.line, want)
		}
	})

	t.Run("two slugs stay two lines in slot order", func(t *testing.T) {
		requireDropInSlug(t, "solar")
		requireDropInSlug(t, "gruv")

		got := advisoryLines(persistedAdvisoriesFor(t, `{"theme_light":"solar","theme_dark":"gruv"}`, t.TempDir()))
		want := []string{
			"⚠ theme solar (light) does not resolve: not found",
			"⚠ theme gruv (dark) does not resolve: not found",
		}
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Errorf("advisory lines =\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
		}
	})
}

func TestPersistedThemeAdvisory_ConstantWinsOverSlots(t *testing.T) {
	requireBuiltinSlug(t, "nord")
	requireDropInSlug(t, "broken")

	t.Run("a resolvable constant silences a broken slot", func(t *testing.T) {
		requireNoAdvisories(t, persistedAdvisoriesFor(t, `{"theme":"nord","theme_dark":"broken"}`, t.TempDir()))
	})

	t.Run("an unresolvable constant is still the only line", func(t *testing.T) {
		requireDropInSlug(t, "nord-lee")

		got := requireOneAdvisory(t, persistedAdvisoriesFor(t, `{"theme":"nord-lee","theme_dark":"broken"}`, t.TempDir()))

		const want = "⚠ theme nord-lee does not resolve: not found"
		if got.line != want {
			t.Errorf("advisory line = %q; want %q", got.line, want)
		}
	})
}

func TestPersistedThemeAdvisory_VirginInstallIsSilent(t *testing.T) {
	cases := []struct{ name, content string }{
		{name: "an absent prefs.json", content: ""},
		{name: "an empty object", content: `{}`},
		{name: "no theme keys at all", content: `{"session_list_mode":"by-tag"}`},
		{name: "a retained appearance and nothing else", content: `{"appearance":"dark"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requireNoAdvisories(t, persistedAdvisoriesFor(t, tc.content, t.TempDir()))
		})
	}

	t.Run("only the set half of a partial pair is reported", func(t *testing.T) {
		requireDropInSlug(t, "solar")

		got := requireOneAdvisory(t, persistedAdvisoriesFor(t, `{"theme_dark":"solar"}`, t.TempDir()))
		if want := "⚠ theme solar (dark) does not resolve: not found"; got.line != want {
			t.Errorf("advisory line = %q; want %q — the unset light slot holds the shipped default and resolves", got.line, want)
		}
	})

	t.Run("an unset slot is not checked even when no built-in resolves", func(t *testing.T) {
		requireDropInSlug(t, "solar")

		// Checking the Setting's slots rather than the raw keys would print a
		// line naming a shipped default the user never chose — only observable
		// on the broken-binary loader staged here.
		dir := t.TempDir()
		loader := theme.NewSilentLoader()
		loader.BuiltinSource = func(string) ([]byte, bool) { return nil, false }

		if _, rejection := loader.ResolveByName(theme.DefaultLightSlug, dir); rejection == nil {
			t.Fatalf("the staged loader still resolves the shipped light default %q — the assertion below would be vacuous", theme.DefaultLightSlug)
		}

		deps := persistedThemeDeps(t, `{"theme_dark":"solar"}`, dir)
		got := advisoryLines(persistedAdvisoriesUnder(t, deps, loader))
		want := []string{"⚠ theme solar (dark) does not resolve: not found"}
		if !slices.Equal(got, want) {
			t.Errorf("advisory lines =\n  %s\nwant\n  %s — only a PERSISTED key produces a line", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
		}
	})
}

func TestPersistedThemeAdvisory_ValidSlugIsSilent(t *testing.T) {
	t.Run("a built-in", func(t *testing.T) {
		requireBuiltinSlug(t, "nord")

		requireNoAdvisories(t, persistedAdvisoriesFor(t, `{"theme":"nord"}`, t.TempDir()))
	})

	t.Run("a valid drop-in", func(t *testing.T) {
		requireDropInSlug(t, "nord-lee")
		dir := themesDirWith(t, map[string][]byte{"nord-lee.theme": validThemeSource(t)})

		requireNoAdvisories(t, persistedAdvisoriesFor(t, `{"theme":"nord-lee"}`, dir))
	})

	t.Run("a valid drop-in in each slot", func(t *testing.T) {
		requireDropInSlug(t, "nord-lee")
		requireDropInSlug(t, "solar")
		dir := themesDirWith(t, map[string][]byte{
			"nord-lee.theme": validThemeSource(t),
			"solar.theme":    validThemeSource(t),
		})

		requireNoAdvisories(t, persistedAdvisoriesFor(t, `{"theme_light":"solar","theme_dark":"nord-lee"}`, dir))
	})
}

func TestPersistedThemeAdvisory_CharsetFailureIsBadName(t *testing.T) {
	cases := []struct{ name, value string }{
		{name: "a path traversal", value: "../evil"},
		{name: "an uppercase letter", value: "Nord"},
		{name: "an underscore", value: "nord_lee"},
		{name: "a space", value: "nord lee"},
		{name: "a leading hyphen", value: "-nord"},
		{name: "an absolute path", value: "/etc/passwd"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			content := prefsJSONWith(t, map[string]string{"theme": tc.value})

			got := requireOneAdvisory(t, persistedAdvisoriesFor(t, content, t.TempDir()))
			want := "⚠ theme " + tc.value + " does not resolve: bad name"
			if got.line != want {
				t.Errorf("advisory line = %q; want %q", got.line, want)
			}
			if strings.Contains(got.line, string(theme.ReasonNotFound)) {
				t.Errorf("advisory line = %q reports %q; an illegal name sends the user to re-type it, not to look for a file", got.line, theme.ReasonNotFound)
			}
		})
	}

	t.Run("a traversal composes no path", func(t *testing.T) {
		root := t.TempDir()
		dir := themesDirIn(t, root, nil)
		// Exactly where filepath.Join(dir, "../evil.theme") lands.
		planted := filepath.Join(root, "evil.theme")
		if err := os.WriteFile(planted, validThemeSource(t), 0o644); err != nil {
			t.Fatalf("plant %s: %v", planted, err)
		}

		requireNoAdvisories(t, persistedAdvisoriesFor(t, `{"theme":"evil"}`, root))

		got := requireOneAdvisory(t, persistedAdvisoriesFor(t, `{"theme":"../evil"}`, dir))
		if want := "⚠ theme ../evil does not resolve: bad name"; got.line != want {
			t.Errorf("advisory line = %q; want %q — the planted file must never be reached", got.line, want)
		}
	})
}

func TestPersistedThemeAdvisory_NotFoundVersusUnreadable(t *testing.T) {
	requireDropInSlug(t, "nord-lee")

	cases := []struct {
		name string
		make func(*testing.T) string
		want string
	}{
		{
			name: "an absent directory is not found",
			make: func(t *testing.T) string { return filepath.Join(t.TempDir(), "themes") },
			want: "⚠ theme nord-lee does not resolve: not found",
		},
		{
			name: "an empty directory is not found",
			make: func(t *testing.T) string { return t.TempDir() },
			want: "⚠ theme nord-lee does not resolve: not found",
		},
		{
			name: "a mode-0000 directory is unreadable",
			make: func(t *testing.T) string {
				dir := themesDirWith(t, map[string][]byte{"nord-lee.theme": validThemeSource(t)})
				_ = themetest.DenyDir(t, dir)
				return dir
			},
			want: "⚠ theme nord-lee does not resolve: unreadable",
		},
		{
			name: "a regular file where the directory belongs is unreadable",
			make: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "themes")
				if err := os.WriteFile(path, []byte("not a directory\n"), 0o644); err != nil {
					t.Fatalf("seed %s: %v", path, err)
				}
				return path
			},
			want: "⚠ theme nord-lee does not resolve: unreadable",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := requireOneAdvisory(t, persistedAdvisoriesFor(t, `{"theme":"nord-lee"}`, tc.make(t)))
			if got.line != tc.want {
				t.Errorf("advisory line = %q; want %q", got.line, tc.want)
			}
		})
	}
}

func TestPersistedThemeAdvisory_UnresolvedThemesDirStillReports(t *testing.T) {
	requireDropInSlug(t, "nord-lee")

	deps := persistedThemeDeps(t, `{"theme":"nord-lee"}`, "")
	loader := theme.NewSilentLoader()

	got := requireOneAdvisory(t, persistedAdvisoriesUnder(t, deps, loader))
	if want := "⚠ theme nord-lee does not resolve: not found"; got.line != want {
		t.Errorf("advisory line = %q; want %q", got.line, want)
	}

	requireNoAdvisories(t, scanThemesDirectory(loader.OpenEnumeration(deps.ThemesDir)))

	t.Run("a built-in still resolves with no directory", func(t *testing.T) {
		requireBuiltinSlug(t, "nord")

		requireNoAdvisories(t, persistedAdvisoriesUnder(t, persistedThemeDeps(t, `{"theme":"nord"}`, ""), loader))
	})
}

func TestPersistedThemeAdvisory_ControlStrippedUntruncated(t *testing.T) {
	// Long enough that no terminal width could carry it. Length is otherwise
	// unconstrained here: the enumeration path composes no path, so no per-name
	// filesystem limit applies.
	stripped := strings.Repeat("nord-lee-", 12)
	requireDropInSlug(t, stripped)
	raw := "\x1b[31m" + stripped + "\n\t"

	content := prefsJSONWith(t, map[string]string{"theme": raw})

	got := requireOneAdvisory(t, persistedAdvisoriesFor(t, content, t.TempDir()))
	want := "⚠ theme " + stripped + " does not resolve: not found"
	if got.line != want {
		t.Errorf("advisory line = %q; want %q", got.line, want)
	}
	if strings.ContainsAny(got.line, "\x1b\n\t") {
		t.Errorf("advisory line = %q still carries a control character or escape", got.line)
	}
	if strings.Contains(got.line, "[31m") {
		t.Errorf("advisory line = %q carries the escape's printable tail; the strip is a terminal-grammar parse, not a byte filter", got.line)
	}
	if got.slug != stripped {
		t.Errorf("advisory slug = %q; want the stripped value %q — the union dedups on it", got.slug, stripped)
	}
}

func TestPersistedThemeAdvisory_ControlOnlyValueIsUnset(t *testing.T) {
	const controlOnly = "\x1b[0m\n\t"

	cases := []struct {
		name string
		keys map[string]string
	}{
		{name: "a control-only constant", keys: map[string]string{"theme": controlOnly}},
		{name: "a control-only light slot", keys: map[string]string{"theme_light": controlOnly}},
		{name: "a control-only dark slot", keys: map[string]string{"theme_dark": controlOnly}},
		{name: "control-only everywhere", keys: map[string]string{"theme": controlOnly, "theme_light": controlOnly, "theme_dark": controlOnly}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requireNoAdvisories(t, persistedAdvisoriesFor(t, prefsJSONWith(t, tc.keys), t.TempDir()))
		})
	}

	t.Run("a control-only constant leaves the slots in force", func(t *testing.T) {
		requireDropInSlug(t, "solar")
		content := prefsJSONWith(t, map[string]string{"theme": controlOnly, "theme_dark": "solar"})

		got := requireOneAdvisory(t, persistedAdvisoriesFor(t, content, t.TempDir()))
		if want := "⚠ theme solar (dark) does not resolve: not found"; got.line != want {
			t.Errorf("advisory line = %q; want %q — a constant that strips to empty never wins the tiebreak", got.line, want)
		}
	})
}

func TestPersistedThemeSlotLabel_ReadsTheSlotsOwnName(t *testing.T) {
	for _, slot := range []theme.Slot{theme.SlotConstant, theme.SlotLight, theme.SlotDark} {
		want, _ := slot.AttrName()
		if got := persistedThemeSlotLabel(theme.InForceKey{Slot: slot}); got != want {
			t.Errorf("persistedThemeSlotLabel(slot %v) = %q; want theme.Slot's own name %q", slot, got, want)
		}
	}

	if got := persistedThemeSlotLabel(theme.InForceKey{Slot: theme.SlotLight, Both: true}); got != themeSlotBoth {
		t.Errorf("persistedThemeSlotLabel(both) = %q; want %q", got, themeSlotBoth)
	}

	t.Run("the words are declared nowhere in doctor", func(t *testing.T) {
		light, _ := theme.SlotLight.AttrName()
		dark, _ := theme.SlotDark.AttrName()
		for _, word := range []string{light, dark} {
			if n := cmdLiteralSites(t, word)["doctor_theme.go"]; n != 0 {
				t.Errorf("doctor_theme.go declares the literal %q %d times; the light/dark words live in internal/theme alone", word, n)
			}
		}

		var arms []string
		ast.Inspect(parsePackageFilesByName(t)["doctor_theme.go"], func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "theme" && strings.HasPrefix(sel.Sel.Name, "Slot") {
				arms = append(arms, sel.Sel.Name)
			}
			return true
		})
		if len(arms) != 0 {
			t.Errorf("doctor_theme.go names %v; a slot added to the vocabulary would compile here and render no label at all", arms)
		}
	})
}

func TestPersistedThemeAdvisory_TolerantOnDegeneratePrefs(t *testing.T) {
	cases := []struct{ name, content string }{
		{name: "an absent file", content: ""},
		{name: "an empty file", content: " "},
		{name: "a stray comma", content: `{"theme":"nord-lee",}`},
		{name: "a truncated document", content: `{"theme":`},
		{name: "a top-level array", content: `["nord-lee"]`},
		{name: "a top-level string", content: `"nord-lee"`},
		{name: "a wrong-typed theme key", content: `{"theme":42}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requireNoAdvisories(t, persistedAdvisoriesFor(t, tc.content, t.TempDir()))
		})
	}

	t.Run("an unreadable file", func(t *testing.T) {
		deps := persistedThemeDeps(t, `{"theme":"nord-lee"}`, t.TempDir())
		path, err := prefsFilePath()
		if err != nil {
			t.Fatalf("prefsFilePath: %v", err)
		}
		_ = themetest.DenyRead(t, path)

		requireNoAdvisories(t, persistedAdvisoriesUnder(t, deps, theme.NewSilentLoader()))
	})

	t.Run("a nil store", func(t *testing.T) {
		requireNoAdvisories(t, persistedAdvisoriesUnder(t, &DoctorDeps{ThemesDir: t.TempDir()}, theme.NewSilentLoader()))
	})

	t.Run("a corrupt file never aborts the diagnosis", func(t *testing.T) {
		setPrefsFile(t, `{"theme":`)

		outBuf, _, err := runDoctorWith(t, healthyDoctorDeps(t))
		if err != nil {
			t.Fatalf("Execute err = %v; a corrupt prefs.json must never abort the diagnosis", err)
		}
		if out := outBuf.String(); !strings.HasSuffix(out, "\n  7 checks passed\n") {
			t.Errorf("report does not close with the full all-passed summary:\n%s", out)
		}
	})
}

func TestPersistedThemeAdvisory_UsesNonMigratingRead(t *testing.T) {
	t.Run("a pending appearance translation is left pending", func(t *testing.T) {
		const seeded = `{"appearance":"dark"}`
		path := setPrefsFile(t, seeded)

		load, err := loadPrefsStore()
		if err != nil {
			t.Fatalf("loadPrefsStore: %v", err)
		}
		if !load.TranslationPending || load.TranslatedSlug == "" {
			t.Fatalf("the migrating read computes pending=%t slug=%q over this fixture; it would translate nothing, so the assertions below would be vacuous", load.TranslationPending, load.TranslatedSlug)
		}

		if _, _, err := runDoctorWith(t, healthyDoctorDeps(t)); err != nil {
			t.Fatalf("Execute err = %v; want nil over a healthy catalog", err)
		}

		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read back prefs.json: %v", err)
		}
		if string(after) != seeded {
			t.Errorf("prefs.json = %s; want it byte-identical: %s", after, seeded)
		}
		if strings.Contains(string(after), "theme_migrated") {
			t.Errorf("prefs.json = %s; the migration marker must still be absent after a diagnosis", after)
		}
	})

	t.Run("the persisted theme is still reported", func(t *testing.T) {
		requireDropInSlug(t, "nord-lee")
		const seeded = `{"appearance":"dark","theme":"nord-lee"}`
		path := setPrefsFile(t, seeded)

		outBuf, _, err := runDoctorWith(t, healthyDoctorDeps(t))
		if err != nil {
			t.Fatalf("Execute err = %v; an unresolvable persisted theme must never drive the exit code", err)
		}

		out := outBuf.String()
		want := "" +
			"  ⚠ theme nord-lee does not resolve: not found\n" +
			"  7 checks passed · 1 advisory\n"
		if !strings.HasSuffix(out, want) {
			t.Errorf("report does not close with the persisted-theme advisory and its summary:\n%s", out)
		}

		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read back prefs.json: %v", err)
		}
		if string(after) != seeded {
			t.Errorf("prefs.json = %s; want it byte-identical: %s", after, seeded)
		}
	})

	t.Run("doctor's sources call only the non-migrating read", func(t *testing.T) {
		var migrating, nonMigrating []string
		for name, file := range parsePackageFilesByName(t) {
			if !strings.HasPrefix(name, "doctor") {
				continue
			}
			sourceguardtest.ForEachFuncCall(file, func(funcName string, call *ast.CallExpr) bool {
				ident, ok := call.Fun.(*ast.Ident)
				if !ok {
					return true
				}
				switch ident.Name {
				case "loadPrefsStore":
					migrating = append(migrating, name+":"+funcName)
				case "loadPrefsStoreNoMigrate":
					nonMigrating = append(nonMigrating, name+":"+funcName)
				}
				return true
			})
		}

		if len(migrating) != 0 {
			t.Errorf("doctor calls the MIGRATING loadPrefsStore from %v; a diagnosis must never trigger the one-shot appearance translation", migrating)
		}
		if want := []string{"doctor.go:resolveDoctorDeps"}; !slices.Equal(nonMigrating, want) {
			t.Errorf("loadPrefsStoreNoMigrate is called from %v; want %v", nonMigrating, want)
		}
	})
}

func TestPersistedThemeAdvisory_NoFallbackAndNoFatal(t *testing.T) {
	t.Run("it names the persisted slug, never the fallback", func(t *testing.T) {
		requireDropInSlug(t, "solar")
		requireDropInSlug(t, "gruv")

		got := advisoryLines(persistedAdvisoriesFor(t, `{"theme_light":"solar","theme_dark":"gruv"}`, t.TempDir()))
		want := []string{
			"⚠ theme solar (light) does not resolve: not found",
			"⚠ theme gruv (dark) does not resolve: not found",
		}
		if !slices.Equal(got, want) {
			t.Fatalf("advisory lines =\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
		}
		for _, line := range got {
			for _, fallback := range []string{theme.DefaultLightSlug, theme.DefaultDarkSlug} {
				if strings.Contains(line, fallback) {
					t.Errorf("advisory line = %q names the fallback %q; the line must report the slug the user chose", line, fallback)
				}
			}
		}
	})

	t.Run("a binary whose embedded set cannot supply a fallback still reports", func(t *testing.T) {
		requireDropInSlug(t, "solar")
		requireDropInSlug(t, "gruv")

		dir := t.TempDir()
		loader := theme.NewSilentLoader()
		// No built-in resolves at all, so every fallback fails.
		loader.BuiltinSource = func(string) ([]byte, bool) { return nil, false }

		setting, _ := theme.ResolveSetting(theme.RawKeys{Light: "solar", Dark: "gruv"})
		if _, err := loader.ResolveNomination(setting, dir); err == nil {
			t.Fatal("the staged loader raises no fatal through ResolveNomination — the assertion below would be vacuous")
		}

		deps := persistedThemeDeps(t, `{"theme_light":"solar","theme_dark":"gruv"}`, dir)
		got := advisoryLines(persistedAdvisoriesUnder(t, deps, loader))
		want := []string{
			"⚠ theme solar (light) does not resolve: not found",
			"⚠ theme gruv (dark) does not resolve: not found",
		}
		if !slices.Equal(got, want) {
			t.Errorf("advisory lines =\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
		}
	})

	t.Run("doctor's theme source never calls ResolveNomination", func(t *testing.T) {
		calls := doctorThemeCallCounts(t)

		if calls["ResolveNomination"] != 0 {
			t.Errorf("doctor_theme.go calls ResolveNomination %d times; it resolves fallbacks and can raise the fatal, and neither belongs in a diagnosis", calls["ResolveNomination"])
		}
		if calls["ResolveByNameFrom"] == 0 {
			t.Error("doctor_theme.go calls no by-name resolver at all; the guard above would pass over a producer that resolves nothing")
		}
	})
}

func doctorThemeCallCounts(t *testing.T) map[string]int {
	t.Helper()

	source := parsePackageFilesByName(t)["doctor_theme.go"]
	if source == nil {
		t.Fatal("the cmd package declares no doctor_theme.go — the guard has nothing to scan")
	}

	calls := map[string]int{}
	ast.Inspect(source, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			calls[sel.Sel.Name]++
		}
		return true
	})
	return calls
}

func TestThemeAdvisories_DirectoryIsReadOnce(t *testing.T) {
	calls := doctorThemeCallCounts(t)

	if calls["OpenEnumeration"] != 1 {
		t.Errorf("doctor_theme.go calls OpenEnumeration %d times; the directory is read once per diagnosis and the retained enumeration drives both producers", calls["OpenEnumeration"])
	}
	for _, reader := range []string{"Enumerate", "ResolveByName", "LoadFile", "ReadDir", "ReadFile", "Open"} {
		if calls[reader] != 0 {
			t.Errorf("doctor_theme.go calls %s %d times; nothing may open a theme file after the enumeration", reader, calls[reader])
		}
	}
}

func TestPersistedThemeAdvisory_EmitsNoThemeRecords(t *testing.T) {
	requireDropInSlug(t, "nord-lee")

	unusableDir := func(t *testing.T) string {
		t.Helper()
		dir := themesDirWith(t, map[string][]byte{"nord-lee.theme": validThemeSource(t)})
		_ = themetest.DenyDir(t, dir)
		return dir
	}

	t.Run("the condition emits through a real component logger", func(t *testing.T) {
		sink := logtest.Install(t)

		loud := theme.NewLoader(theme.NewEventLogger(log.For("theme")))
		if _, rejection := loud.ResolveByName("nord-lee", unusableDir(t)); rejection == nil {
			t.Fatal("the fixture resolved cleanly — the zero-record assertion below would be about the wrong run")
		}

		if events := themeEvents(t, sink); len(events) == 0 {
			t.Fatal("the capture harness recorded no theme events over an unusable directory — the assertion below would be vacuous")
		}
	})

	t.Run("a full doctor run writes nothing", func(t *testing.T) {
		dir := unusableDir(t)
		t.Setenv("PORTAL_THEMES_DIR", dir)
		setPrefsFile(t, `{"theme":"nord-lee"}`)

		sink := logtest.Install(t)

		outBuf, _, err := runDoctorWith(t, healthyDoctorDeps(t))
		if err != nil {
			t.Fatalf("Execute err = %v; theme advisories never drive the exit code", err)
		}

		// Both lines must be present, each reaching the emitting condition by
		// its own route; their relative order is deliberately not asserted.
		out := outBuf.String()
		for _, want := range []string{
			"  ⚠ themes directory unreadable: " + dir + "\n",
			"  ⚠ theme nord-lee does not resolve: unreadable\n",
		} {
			if !strings.Contains(out, want) {
				t.Fatalf("report is missing %q — the zero-record assertion needs both emitting conditions to have been reached:\n%s", want, out)
			}
		}
		if !strings.HasSuffix(out, "\n  7 checks passed · 2 advisories\n") {
			t.Fatalf("report does not close with two advisories counted:\n%s", out)
		}
		if events := themeEvents(t, sink); len(events) != 0 {
			t.Errorf("the run emitted %d theme records, want none:\n  %s", len(events), strings.Join(events, "\n  "))
		}
	})
}

func TestPersistedThemeAdvisory_FrameIsSingleSourced(t *testing.T) {
	sites := cmdLiteralSites(t, "does not resolve")

	if want := map[string]int{"doctor_theme.go": 1}; !maps.Equal(sites, want) {
		t.Errorf("the literal %q is declared at %v; want %v — one const in the file that owns doctor's theme copy", "does not resolve", sites, want)
	}
}

func TestPersistedThemeAdvisories_NothingToReportIsEmptyNotNil(t *testing.T) {
	advisories := persistedAdvisoriesFor(t, `{"theme":"nord"}`, t.TempDir())

	if advisories == nil {
		t.Errorf("the persisted producer returned nil over a resolvable key, want an empty slice")
	}
	requireNoAdvisories(t, advisories)
}

func TestScanThemesDirectory_NothingToReportIsEmptyNotNil(t *testing.T) {
	advisories := scanThemesDirectory(theme.Enumeration{})

	if advisories == nil {
		t.Errorf("the directory scan returned nil over an enumeration with no findings, want an empty slice")
	}
	requireNoAdvisories(t, advisories)
}
