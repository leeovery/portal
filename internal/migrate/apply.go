package migrate

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
)

// via is the surface migrate's writes come from: the portal migrate command.
const via = "cli"

// Apply writes plan into dir, the zmx version's state dir, made 0700 if
// absent, and rewrites hooks.json through store, each file replaced
// atomically:
//   - each session's entry in sessions.json, owed a restore and recording no
//     creation, and its scrollback copied to the file the entry names, all in
//     one hold of the commit lock, so no saver's clean takes a copy before an
//     entry names it;
//   - then hooks.json's keys moved to the new names and the rest removed, in
//     one write.
//
// Applying a plan again changes nothing: its scrollback is copied again as it
// was, its entries are found made and its keys found moved or removed. An
// entry under one of its names that isn't the one it makes refuses it with
// sessionindex.ErrNameHeld, writing nothing.
func Apply(plan Plan, dir string, store *hooks.Store) error {
	if err := os.MkdirAll(state.ScrollbackDir(dir), 0o700); err != nil {
		return fmt.Errorf("make the state dir: %w", err)
	}
	if _, err := sessionindex.Update(dir, via, func(idx *sessionindex.Index) error {
		for _, s := range plan.Sessions {
			if err := idx.Migrated(s.entry()); err != nil {
				return err
			}
		}
		return copyTranscripts(dir, plan.Sessions)
	}); err != nil {
		return fmt.Errorf("write the migrated sessions: %w", err)
	}
	if err := store.Rekey(plan.moves(), plan.removedKeys(), hooks.ViaCLI); err != nil {
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
func copyTranscripts(dir string, sessions []Session) error {
	for _, s := range sessions {
		if s.Transcript.File == "" {
			continue
		}
		data, err := os.ReadFile(s.Transcript.source)
		if err != nil {
			return fmt.Errorf("read the scrollback of %s: %w", s.From, err)
		}
		if err := fileutil.AtomicWrite0600(filepath.Join(dir, filepath.FromSlash(s.Transcript.File)), data); err != nil {
			return fmt.Errorf("copy the scrollback of %s: %w", s.From, err)
		}
	}
	return nil
}
