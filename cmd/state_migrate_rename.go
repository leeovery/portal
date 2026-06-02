package cmd

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/spf13/cobra"
)

// stateMigrateRenameCmd migrates hooks.json keys after a tmux session rename.
// Invoked from a session-renamed hook with the old and new session names.
// Hidden from --help.
var stateMigrateRenameCmd = &cobra.Command{
	Use:    "migrate-rename <old-name> <new-name>",
	Short:  "Migrate hook keys across a session rename (internal)",
	Args:   cobra.ExactArgs(2),
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := loadHookStore()
		if err != nil {
			return fmt.Errorf("load hook store: %w", err)
		}

		// Migration diagnostics (load failures, key collisions, save
		// failures) land in the central log under the hooks component via the
		// handler configured once by main -> log.Init. Rotation is
		// handler-owned (Phase 2), so this command no longer opens or closes a
		// per-process logger.
		return runMigrateRename(store, args[0], args[1], hooksLogger)
	},
}

// runMigrateRename rewrites every hooks.json key matching "<oldName>:*" to
// "<newName>:*". The trailing colon disambiguates similarly-prefixed session
// names ("work" must not match "work-2:0.0"). Empty newName is rejected.
//
// Behaviour:
//   - Missing or malformed hooks.json is treated as empty (no write).
//   - Zero matches: no write at all (mtime preserved).
//   - Collision on a destination key: WARN to portal.log under the hooks
//     component and overwrite.
//   - Save failure: WARN to portal.log under the hooks component and
//     propagate the error.
//
// logger must be a real *slog.Logger (production passes log.For("hooks")).
//
// Map iteration in Go is randomised and mutating during iteration is
// unspecified. Matching keys are collected first, then rewritten.
func runMigrateRename(store *hooks.Store, oldName, newName string, logger *slog.Logger) error {
	if newName == "" {
		return fmt.Errorf("new name must be non-empty")
	}

	h, err := store.Load()
	if err != nil {
		// Store.Load swallows missing-file and malformed-JSON; a non-nil error
		// here is a genuine I/O failure.
		logger.Warn("load hooks failed", "error", err)
		return err
	}

	prefix := oldName + ":"
	var toMigrate []string
	for key := range h {
		if strings.HasPrefix(key, prefix) {
			toMigrate = append(toMigrate, key)
		}
	}

	if len(toMigrate) == 0 {
		return nil
	}

	for _, key := range toMigrate {
		events := h[key]
		newKey := newName + ":" + strings.TrimPrefix(key, prefix)
		if _, collision := h[newKey]; collision {
			logger.Warn("hook key collision; overwriting", "hook_key", newKey)
		}
		h[newKey] = events
		delete(h, key)
	}

	// Persist through the store's audited seam so the rewrite leaves a
	// breadcrumb in portal.log under the hooks component. The bulk rewrite has
	// no single per-file key, so it is recorded as a batch op=modify with
	// entries=N where N is the number of rewritten keys (len(toMigrate)) — the
	// meaningful count of entries actually changed by this migration. The
	// audited method owns both the success INFO and the save-failure WARN, so
	// the previous hand-rolled save-fail WARN here is removed (no double WARN).
	return store.SaveAudited(h, "modify", len(toMigrate), "internal")
}

func init() {
	stateCmd.AddCommand(stateMigrateRenameCmd)
}
