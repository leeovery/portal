package cmd

import (
	"fmt"
	"io"
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
		return runMigrateRename(store, args[0], args[1], cmd.ErrOrStderr())
	},
}

// runMigrateRename rewrites every hooks.json key matching "<oldName>:*" to
// "<newName>:*". The trailing colon disambiguates similarly-prefixed session
// names ("work" must not match "work-2:0.0"). Empty newName is rejected.
//
// Behaviour:
//   - Missing or malformed hooks.json is treated as empty (no write).
//   - Zero matches: no write at all (mtime preserved).
//   - Collision on a destination key: warn on stderr and overwrite.
//   - Save failure: warn on stderr and propagate the error.
//
// Map iteration in Go is randomised and mutating during iteration is
// unspecified. Matching keys are collected first, then rewritten.
func runMigrateRename(store *hooks.Store, oldName, newName string, stderr io.Writer) error {
	if newName == "" {
		return fmt.Errorf("new name must be non-empty")
	}

	h, err := store.Load()
	if err != nil {
		// Store.Load swallows missing-file and malformed-JSON; a non-nil error
		// here is a genuine I/O failure.
		_, _ = fmt.Fprintf(stderr, "portal state migrate-rename: %v\n", err)
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
			_, _ = fmt.Fprintf(stderr, "portal state migrate-rename: collision on %s; overwriting\n", newKey)
		}
		h[newKey] = events
		delete(h, key)
	}

	if err := store.Save(h); err != nil {
		_, _ = fmt.Fprintf(stderr, "portal state migrate-rename: save failed: %v\n", err)
		return err
	}
	return nil
}

func init() {
	stateCmd.AddCommand(stateMigrateRenameCmd)
}
