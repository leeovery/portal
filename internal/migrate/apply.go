package migrate

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
)

// via is the surface migrate's writes come from: the portal migrate command.
const via = "cli"

// Build writes plan's sessions into dir, the fresh state dir migrate builds
// beside the tmux version's, made 0700 with its scrollback dir if absent:
// first each scrollback file, copied from the tmux version's state dir through
// a root on it, so no read leaves it; then sessions.json's entries, owed a
// restore and recording no creation. A copy that fails writes no entry, so no
// entry names a file not copied. Each file is written atomically.
func Build(plan Plan, dir string) error {
	if err := os.MkdirAll(state.ScrollbackDir(dir), 0o700); err != nil {
		return fmt.Errorf("make the state dir: %w", err)
	}
	if err := copyTranscripts(plan, dir); err != nil {
		return err
	}
	if _, err := sessionindex.Update(dir, via, func(idx *sessionindex.Index) error {
		for _, s := range plan.Sessions {
			if err := idx.Migrated(s.entry()); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("write the migrated sessions: %w", err)
	}
	return nil
}

// RewriteHooks writes hooks.json through store from backup, the copy of it
// migrate keeps: each key the plan moves under its session's name, each it
// removes gone, every value as backup holds it, in one write. It never reads
// hooks.json, so every run writes the same file. No backup writes nothing.
func RewriteHooks(plan Plan, backup string, store *hooks.Store) error {
	if err := store.RekeyFrom(backup, plan.moves(), plan.removedKeys(), hooks.ViaCLI); err != nil {
		return fmt.Errorf("rewrite hooks.json: %w", err)
	}
	return nil
}

// entry is the index entry Migrated makes of s.
func (s Session) entry() sessionindex.Entry {
	return sessionindex.Entry{Name: s.Name, Label: s.Label, Dir: s.Dir, Cwd: s.Cwd, ScrollbackFile: s.Transcript.File}
}

// copyTranscripts copies each session's scrollback file into dir under the
// name its entry gives it.
func copyTranscripts(plan Plan, dir string) error {
	copied := slices.DeleteFunc(slices.Clone(plan.Sessions), func(s Session) bool { return s.Transcript.File == "" })
	if len(copied) == 0 {
		return nil
	}
	root, err := os.OpenRoot(plan.legacyDir)
	if err != nil {
		return fmt.Errorf("open the tmux version's state dir: %w", err)
	}
	defer func() { _ = root.Close() }()
	for _, s := range copied {
		data, err := root.ReadFile(s.Transcript.source)
		if err != nil {
			return fmt.Errorf("read the scrollback of %s: %w", s.From, err)
		}
		if err := fileutil.AtomicWrite0600(filepath.Join(dir, filepath.FromSlash(s.Transcript.File)), data); err != nil {
			return fmt.Errorf("copy the scrollback of %s: %w", s.From, err)
		}
	}
	return nil
}
