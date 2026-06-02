package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/xdg"
)

// configFileComponents is the closed filename -> owning-component mapping for
// the in-scope user-config files (the state-mutation audit-trail set). It is
// the single source of truth that lets configFilePath thread the correct
// owning component into migrateConfigFile's breadcrumb. A filename absent from
// this map (none today, but defensive) resolves to "" and suppresses the
// migrate emission entirely — see migrateConfigFile's empty-component guard.
var configFileComponents = map[string]string{
	"hooks.json":    "hooks",
	"aliases":       "aliases",
	"projects.json": "projects",
}

// migrateConfigFile moves a config file from oldPath to newPath if oldPath
// exists and newPath does not. This handles one-shot migration from the old
// macOS config location (~/Library/Application Support/portal/) to the
// XDG-compliant path. Migration is best-effort.
//
// component is the owning component of the migrated file (one of the closed
// "hooks"/"aliases"/"projects" values from configFileComponents). On a
// successful move it emits one INFO breadcrumb (op rendered as the slog message
// verb "migrate", per the established state-mutation pattern; via="migrate";
// path=newPath) and on a MkdirAll/Rename failure one WARN — both under the
// owning component's logger, bound dynamically here because migrateConfigFile
// is generic across the three config files. There is deliberately NO per-entry
// key attr (hook_key/alias/project): a whole-file move has no single entry key,
// and the path attr plus the component already identify the file.
//
// An EMPTY component (an unmapped filename) suppresses every emission — we must
// never log under an empty/invalid component — but the move itself still runs
// best-effort.
//
// PR-timing caveat: migrateConfigFile lands with the state-mutation work; a
// migration firing in an earlier window goes unlogged — accepted (rare
// idempotent one-shot most users already ran).
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
			log.For(component).Warn("migrate", "via", "migrate", "path", filepath.Dir(newPath), "error", err, "error_class", "write-failed-temp-create")
		}
		return
	}

	if err := os.Rename(oldPath, newPath); err != nil {
		if component != "" {
			log.For(component).Warn("migrate", "via", "migrate", "path", newPath, "error", err, "error_class", "write-failed-rename")
		}
		return
	}

	if component != "" {
		log.For(component).Info("migrate", "via", "migrate", "path", newPath)
	}

	// Clean up old directory if empty.
	_ = os.Remove(filepath.Dir(oldPath))
}

// configFilePath returns a config file path by checking the given environment
// variable first. If the env var is set and non-empty, its value is returned.
// Otherwise, it uses XDG-compliant resolution via xdg.ConfigBase, then appends
// portal/<filename>.
// Before returning, it attempts a one-shot migration from the old macOS
// config path (~/Library/Application Support/portal/) if the file exists there.
func configFilePath(envVar, filename string) (string, error) {
	if envPath := os.Getenv(envVar); envPath != "" {
		return envPath, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to determine home directory: %w", err)
	}

	configDir, err := xdg.ConfigBase()
	if err != nil {
		return "", err
	}
	newPath := filepath.Join(configDir, "portal", filename)

	oldPath := filepath.Join(homeDir, "Library", "Application Support", "portal", filename)
	// configFileComponents is the closed filename->component mapping; an
	// unmapped filename yields "" and migrateConfigFile suppresses emission.
	migrateConfigFile(oldPath, newPath, configFileComponents[filename])

	return newPath, nil
}
