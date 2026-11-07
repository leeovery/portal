package cmd

import (
	"bytes"
	"go/ast"
	"os"
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/sourceguardtest"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/themetest"
)

func migratingLoadForTest(t *testing.T) prefsLoad {
	t.Helper()

	load, err := loadPrefsStore()
	if err != nil {
		t.Fatalf("load prefs store: %v", err)
	}
	if load.Store == nil {
		t.Fatal("loadPrefsStore returned no store; the initial mode read and the theme persister both need one")
	}
	return load
}

func assertLoad(t *testing.T, got, want prefsLoad) {
	t.Helper()

	if got.Keys != want.Keys {
		t.Errorf("Keys = %+v, want %+v", got.Keys, want.Keys)
	}
	if got.TranslationPending != want.TranslationPending {
		t.Errorf("TranslationPending = %v, want %v", got.TranslationPending, want.TranslationPending)
	}
	if got.TranslatedSlug != want.TranslatedSlug {
		t.Errorf("TranslatedSlug = %q, want %q", got.TranslatedSlug, want.TranslatedSlug)
	}
}

func TestLoadPrefsStore_TranslatesDark(t *testing.T) {
	setPrefsFile(t, `{"appearance":"dark"}`)

	load := migratingLoadForTest(t)

	assertLoad(t, load, prefsLoad{Keys: prefs.ThemeKeys{Theme: theme.DefaultDarkSlug}, TranslationPending: true, TranslatedSlug: theme.DefaultDarkSlug})
}

func TestLoadPrefsStore_TranslatesLight(t *testing.T) {
	setPrefsFile(t, `{"appearance":"light"}`)

	load := migratingLoadForTest(t)

	assertLoad(t, load, prefsLoad{Keys: prefs.ThemeKeys{Theme: theme.DefaultLightSlug}, TranslationPending: true, TranslatedSlug: theme.DefaultLightSlug})
}

// The keys must reach the nomination as the post-translation in-memory value:
// a second read of the store would answer with zero keys, which is the shipped
// adaptive pair rather than the translated pin.
func TestLoadPrefsStore_TranslatedPinRendersAsAConstantThisLaunch(t *testing.T) {
	for _, tc := range []struct{ appearance, want string }{
		{"dark", theme.DefaultDarkSlug},
		{"light", theme.DefaultLightSlug},
	} {
		t.Run("appearance "+tc.appearance, func(t *testing.T) {
			setPrefsFile(t, `{"appearance":"`+tc.appearance+`"}`)

			assertConstant(t, themeNominationForTest(t), themetest.Builtin(t, tc.want))
		})
	}
}

// The marker is still owed in every row: "nothing translated" refers to the
// theme keys, and leaving it unrecorded re-evaluates the condition forever.
func TestLoadPrefsStore_NoTranslationCases(t *testing.T) {
	for _, tc := range []struct{ name, content string }{
		{"auto", `{"appearance":"auto"}`},
		{"absent", `{"session_list_mode":"by-tag"}`},
		{"empty", `{"appearance":""}`},
		{"unrecognised", `{"appearance":"sepia"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setPrefsFile(t, tc.content)

			load := migratingLoadForTest(t)

			assertLoad(t, load, prefsLoad{TranslationPending: true})
		})
	}
}

// The second case is the one absence-gating gets wrong: a user who hand-edited
// their theme keys away to return to the shipped pair must not be re-pinned.
func TestLoadPrefsStore_MarkerGatesTheTranslation(t *testing.T) {
	t.Run("a migrated file with a key set is returned exactly as read", func(t *testing.T) {
		setPrefsFile(t, `{"appearance":"dark","theme":"`+nordSlug+`","theme_migrated":true}`)

		load := migratingLoadForTest(t)

		assertLoad(t, load, prefsLoad{Keys: prefs.ThemeKeys{Theme: nordSlug}})
	})

	t.Run("a migrated file whose keys were deleted stays on the shipped pair", func(t *testing.T) {
		setPrefsFile(t, `{"appearance":"dark","theme_migrated":true}`)

		load := migratingLoadForTest(t)

		assertLoad(t, load, prefsLoad{})
	})
}

// TranslatedSlug survives the no-op check unzeroed on purpose: the write half
// re-checks against its own read, which is what absorbs a concurrent commit.
func TestLoadPrefsStore_ExistingKeySuppressesTheInMemoryValue(t *testing.T) {
	for _, tc := range []struct {
		key  string
		want prefs.ThemeKeys
	}{
		{"theme", prefs.ThemeKeys{Theme: nordSlug}},
		{"theme_light", prefs.ThemeKeys{Light: nordSlug}},
		{"theme_dark", prefs.ThemeKeys{Dark: nordSlug}},
	} {
		t.Run(tc.key+" already set", func(t *testing.T) {
			setPrefsFile(t, `{"appearance":"dark","`+tc.key+`":"`+nordSlug+`"}`)

			load := migratingLoadForTest(t)

			assertLoad(t, load, prefsLoad{Keys: tc.want, TranslationPending: true, TranslatedSlug: theme.DefaultDarkSlug})
		})
	}
}

// The hand-edit must paint on the translating launch itself, not one launch of
// the translated default followed by it.
func TestLoadPrefsStore_HandEditedSlotWinsOnTheTranslatingLaunch(t *testing.T) {
	setPrefsFile(t, `{"appearance":"dark","theme_dark":"`+nordSlug+`"}`)

	nomination := themeNominationForTest(t)

	assertPair(t, nomination, themetest.Builtin(t, theme.DefaultLightSlug), themetest.Builtin(t, nordSlug))
}

// The absent-file row carries a second rule: the migration never creates
// prefs.json, so a fresh install gains no file just to record a marker.
func TestLoadPrefsStore_ComputesWithoutWriting(t *testing.T) {
	t.Run("a translating launch leaves the file byte-identical", func(t *testing.T) {
		before := []byte(`{"session_list_mode":"by-tag","appearance":"dark"}`)
		path := setPrefsFile(t, string(before))

		load := migratingLoadForTest(t)
		assertLoad(t, load, prefsLoad{Keys: prefs.ThemeKeys{Theme: theme.DefaultDarkSlug}, TranslationPending: true, TranslatedSlug: theme.DefaultDarkSlug})

		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read back prefs.json: %v", err)
		}
		if !bytes.Equal(after, before) {
			t.Errorf("prefs.json = %s, want it untouched: %s", after, before)
		}
	})

	t.Run("an absent prefs.json stays absent", func(t *testing.T) {
		path := setPrefsFile(t, "")

		load := migratingLoadForTest(t)
		assertLoad(t, load, prefsLoad{TranslationPending: true})

		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("prefs.json exists at %s (stat err %v); the migration must create nothing", path, err)
		}
	})
}

func TestLoadPrefsStore_TolerantOnDegenerateFiles(t *testing.T) {
	for _, tc := range []struct{ name, content string }{
		{"absent", ""},
		{"corrupt", "{ this is not json"},
		{"empty", " "},
		{"a top-level array", `[1,2]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setPrefsFile(t, tc.content)

			load := migratingLoadForTest(t)
			assertLoad(t, load, prefsLoad{TranslationPending: true})

			assertPair(t, themeNominationForTest(t),
				themetest.Builtin(t, theme.DefaultLightSlug),
				themetest.Builtin(t, theme.DefaultDarkSlug))
		})
	}
}

func TestLoadPrefsStoreNoMigrate_ComputesAndWritesNothing(t *testing.T) {
	t.Run("it returns a bound store against an unreadable file", func(t *testing.T) {
		// The theme key is seeded so the read below has something to return that
		// a zero value would not.
		before := []byte(`{"appearance":"dark","theme_dark":"` + nordSlug + `"}`)
		path := setPrefsFile(t, string(before))
		_ = themetest.DenyRead(t, path)

		store, err := loadPrefsStoreNoMigrate()
		if err != nil {
			t.Fatalf("loadPrefsStoreNoMigrate: %v", err)
		}
		if store == nil {
			t.Fatal("loadPrefsStoreNoMigrate returned no store")
		}

		if err := os.Chmod(path, 0o644); err != nil {
			t.Fatalf("chmod 0644 prefs.json: %v", err)
		}
		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read back prefs.json: %v", err)
		}
		if !bytes.Equal(after, before) {
			t.Errorf("prefs.json = %s, want it untouched: %s", after, before)
		}

		keys, err := store.LoadThemeKeys()
		if err != nil {
			t.Fatalf("LoadThemeKeys: %v", err)
		}
		// The file's own key, not a zero value: a store bound to nothing, or one
		// that read nothing at all, satisfies a zero expectation and the
		// assertion would distinguish neither from a real read.
		if want := (prefs.ThemeKeys{Dark: nordSlug}); keys != want {
			t.Errorf("keys = %+v, want %+v — the returned store reads the file's own keys", keys, want)
		}
	})

	t.Run("its body resolves the path and constructs the store, and nothing else", func(t *testing.T) {
		// A read whose error is discarded is invisible to the runtime half, and
		// is exactly the shape behaviour creeps in as. The closed call set is
		// the "must never gain behaviour" contract.
		want := map[string]bool{"prefsFilePath": true, "prefs.NewStore": true}

		var calls []string
		ast.Inspect(funcDeclForTest(t, "config.go", "loadPrefsStoreNoMigrate"), func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch target := call.Fun.(type) {
			case *ast.Ident:
				calls = append(calls, target.Name)
			case *ast.SelectorExpr:
				if pkg, ok := target.X.(*ast.Ident); ok {
					calls = append(calls, pkg.Name+"."+target.Sel.Name)
				}
			}
			return true
		})

		for _, call := range calls {
			if !want[call] {
				t.Errorf("loadPrefsStoreNoMigrate calls %s — it must never gain behaviour; doctor's read-only path is what a read or a mutation here would break", call)
			}
		}
		if len(calls) != len(want) {
			t.Errorf("loadPrefsStoreNoMigrate makes %d calls (%v), want the %d that resolve the path and construct the store", len(calls), calls, len(want))
		}
	})
}

func funcDeclForTest(t *testing.T, file, name string) *ast.FuncDecl {
	t.Helper()

	parsed, ok := parsePackageFilesByName(t)[file]
	if !ok {
		t.Fatalf("cmd/%s is not a production source of the package", file)
	}
	for _, decl := range parsed.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Name.Name == name {
			return fn
		}
	}
	t.Fatalf("cmd/%s declares no func %s", file, name)
	return nil
}

// A second caller — a doctor check, a bootstrap step, a shared pre-run — would
// translate on a path that paints nothing and mutate the user's config from a
// verb with no business doing so. The non-migrating variant exists for those.
func TestLoadPrefsStore_SingleProductionCaller(t *testing.T) {
	var callers []string

	for name, file := range parsePackageFilesByName(t) {
		sourceguardtest.ForEachFuncCall(file, func(funcName string, call *ast.CallExpr) bool {
			if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == "loadPrefsStore" {
				callers = append(callers, name+":"+funcName)
			}
			return true
		})
	}

	// Map-ordered, so sort for a stable failure message.
	slices.Sort(callers)

	if len(callers) != 1 || callers[0] != "open.go:runPicker" {
		t.Errorf("loadPrefsStore is called from %v, want exactly [open.go:runPicker] — the translation runs only where a TUI is constructed; use loadPrefsStoreNoMigrate for a read-only one", callers)
	}
}

// The old appearance decode matched its tokens exactly, so every value it read
// as `auto` (`Dark`, ` dark`, a trailing newline) must translate to nothing:
// trimming or lowercasing would change a value's meaning.
func TestTranslateAppearance_ExactMatchOnly(t *testing.T) {
	for _, raw := range []string{"Dark", " dark", "DARK", "dark\n", "Light", "light ", "LIGHT", "auto", "sepia", ""} {
		t.Run("translates "+raw+" to nothing", func(t *testing.T) {
			if got := translateAppearance(raw); got != "" {
				t.Errorf("translateAppearance(%q) = %q, want %q — only the exact tokens translate", raw, got, "")
			}
		})
	}
}

// Named by the shared constants, never by a literal: a hardcoded slug here
// would go on passing after the shipped default moved.
func TestTranslateAppearance_UsesSharedConstants(t *testing.T) {
	if theme.DefaultDarkSlug == theme.DefaultLightSlug {
		t.Fatal("the two shipped default slugs are equal; the assertions below could not tell the two mappings apart")
	}

	for _, tc := range []struct{ raw, want string }{
		{"dark", theme.DefaultDarkSlug},
		{"light", theme.DefaultLightSlug},
	} {
		t.Run("appearance "+tc.raw, func(t *testing.T) {
			if got := translateAppearance(tc.raw); got != tc.want {
				t.Errorf("translateAppearance(%q) = %q, want %q", tc.raw, got, tc.want)
			}
			// A slug naming no built-in would pin the user to the fallback
			// rather than to the theme their pin meant.
			themetest.Builtin(t, tc.want)
		})
	}
}
