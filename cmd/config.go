package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/xdg"
)

func migrateConfigFile(oldPath, newPath, component string) {
	if _, err := os.Stat(oldPath); err != nil {
		return
	}

	if _, err := os.Stat(newPath); err == nil {
		return
	} else if !os.IsNotExist(err) {
		return
	}

	if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
		if component != "" {
			wrapped := fmt.Errorf("%w: failed to create directory: %w", fileutil.ErrWriteTempCreate, err)
			log.For(component).Warn("migrate", "op", "migrate", "via", "migrate", "path", filepath.Dir(newPath), "error", wrapped, "error_class", "write-failed-temp-create")
		}
		return
	}

	if err := os.Rename(oldPath, newPath); err != nil {
		if component != "" {
			wrapped := fmt.Errorf("%w: failed to rename: %w", fileutil.ErrWriteRename, err)
			log.For(component).Warn("migrate", "op", "migrate", "via", "migrate", "path", newPath, "error", wrapped, "error_class", "write-failed-rename")
		}
		return
	}

	if component != "" {
		log.For(component).Info("migrate", "op", "migrate", "via", "migrate", "path", newPath)
	}

	_ = os.Remove(filepath.Dir(oldPath))
}

// configFilePath is the production route into the shared precedence declared by
// xdg.ConfigFilePath, read against the process environment. The one-shot
// Application Support migration lives here and only here: it is a side effect of
// production reading its default location, never part of resolving a path, so a
// caller resolving the same rule against some other environment — a test seeder
// asking where the binary under test will look — can never move a real file.
func configFilePath(id xdg.ConfigFileID) (string, error) {
	resolved, err := xdg.ConfigFilePath(xdg.OSEnv, id)
	if err != nil {
		return "", err
	}
	if resolved.Overridden {
		return resolved.Path, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to determine home directory: %w", err)
	}

	oldPath := filepath.Join(homeDir, "Library", "Application Support", "portal", id.Filename)
	migrateConfigFile(oldPath, resolved.Path, id.LogComponent)

	return resolved.Path, nil
}

func loadProjectStore() (*project.Store, error) {
	path, err := projectsFilePath()
	if err != nil {
		return nil, err
	}
	return project.NewStore(path), nil
}

func projectsFilePath() (string, error) {
	return configFilePath(xdg.ProjectsFile)
}

// Must stay inert: this is the read `portal doctor` uses, so anything added
// here lands on doctor's read-only diagnosis path — a read whose error is
// discarded as surely as a write.
func loadPrefsStoreNoMigrate() (*prefs.Store, error) {
	path, err := prefsFilePath()
	if err != nil {
		return nil, err
	}
	return prefs.NewStore(path), nil
}

// prefsLoad is what the migrating load produces: the bound store alongside the
// state the one-shot appearance translation resolved for this launch.
type prefsLoad struct {
	Store *prefs.Store
	// Keys carries the theme setting as this launch renders it — the
	// post-translation in-memory value, not the on-disk bytes.
	Keys prefs.ThemeKeys
	// True even when nothing translated: recording the marker is what stops the
	// condition being re-evaluated on every future launch.
	TranslationPending bool
	TranslatedSlug     string
}

// The translation is applied in memory before it is persisted, so this launch
// renders the correct theme even if the write fails — which leaves the marker
// unset for the next launch to retry. Every degenerate read is tolerated; only
// path resolution can fail the load. It lives here rather than in prefs, a leaf
// that must not import internal/log.
func loadPrefsStore() (prefsLoad, error) {
	store, err := loadPrefsStoreNoMigrate()
	if err != nil {
		return prefsLoad{}, err
	}

	keys, migration, _ := store.LoadThemeState()

	load := prefsLoad{Store: store, Keys: keys}

	// The trigger is the marker, never the absence of theme keys: absence-gating
	// is re-armable, so a user hand-editing their keys away to return to the
	// shipped pair would be silently re-pinned on the next launch.
	if migration.Migrated {
		return load, nil
	}

	load.TranslationPending = true
	load.TranslatedSlug = translateAppearance(migration.Appearance)

	// Applied against the load-time snapshot: the only moment early enough to
	// affect what is painted. Scoping this to the write alone would flip a user
	// who already set a theme key onto the translated theme for one launch.
	if load.TranslatedSlug != "" && keys == (prefs.ThemeKeys{}) {
		// A pinned appearance becomes a pinned constant, so detection stays off.
		load.Keys = prefs.ThemeKeys{Theme: load.TranslatedSlug}
	}

	// TranslatedSlug is deliberately not zeroed above: the write re-checks the
	// same condition against its own re-read, which is what absorbs a theme
	// another instance committed in between.
	persistTranslation(store, load.TranslatedSlug)

	return load, nil
}

// A var so a synchronous implementation can be substituted; the body lives in
// runTranslationPersist so a substitute still runs the real save-and-emit path.
var persistTranslation = func(store *prefs.Store, slug string) {
	go runTranslationPersist(store, slug)
}

// Emits only when a theme key was actually persisted — a marker-only write would
// otherwise announce a migration that translated nothing. Failure is silent: the
// absence of the event is the signal, and commit-failed belongs to the panel's
// theme persister.
func runTranslationPersist(store *prefs.Store, slug string) {
	persisted, err := store.SaveTranslation(slug)
	if err != nil || !persisted {
		return
	}

	themeLogger.Info("appearance migrated", "slug", slug)
}

// The match is exact: anything the old appearance decode treated as `auto`
// (`Dark`, `" dark"`, a trailing newline) must translate to nothing, so do not
// trim or lowercase here.
func translateAppearance(raw string) string {
	switch raw {
	case "dark":
		return theme.DefaultDarkSlug
	case "light":
		return theme.DefaultLightSlug
	default:
		return ""
	}
}

func prefsFilePath() (string, error) {
	return configFilePath(xdg.PrefsFile)
}

// themeRawKeys is the one place prefs' theme keys map onto the theme package's.
// Spelled at a call site the triple is positional, and a one-word slip there
// would invert every user's light and dark palettes.
func themeRawKeys(k prefs.ThemeKeys) theme.RawKeys {
	return theme.NewRawKeys(k.Theme, k.Light, k.Dark)
}

// Not a configFilePath member: it resolves a directory, and there is no old
// macOS path to migrate from. It returns a path and nothing more — never
// creating, seeding or stat-ing the directory, whose absence is the common case.
func themesDirPath() (string, error) {
	return xdg.ConfigDirPath(xdg.OSEnv, xdg.ThemesDir)
}
