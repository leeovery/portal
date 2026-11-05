package hooks

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/log"
)

// ErrUnaccountedKeys refuses a rekey that doesn't account for every key: one
// the backup holds that it neither moves nor removes, which would be written
// back unchanged, or one it names that the backup doesn't hold.
var ErrUnaccountedKeys = errors.New("the rekey doesn't account for every key")

// errNoBackup is a rekey with no backup to write hooks.json from.
var errNoBackup = errors.New("no backup of hooks.json")

// RekeyFrom writes hooks.json from backup, the copy of it portal migrate
// keeps, by migrate's plan: every hook of each key of moves under the key it
// maps to, each event's value carried as backup holds it, byte for byte, and
// each key of remove gone. Every key backup holds must be moved or removed,
// and every key moved or removed held, so nothing is carried that the plan
// didn't name. A key removed and moved onto holds the moved hooks. It never
// reads hooks.json, so the same backup and plan always write the same file, in
// one write under the mutation hold. No backup, as when there was never a
// hooks.json, writes nothing. A rekey not accounting for every key
// (ErrUnaccountedKeys), two keys moved onto one, a key both moved and removed,
// a backup that can't be read (ErrStoreRead) or doesn't parse (ErrMalformed),
// and a lock not granted each refuse it, writing nothing. It logs each key at
// DEBUG and one INFO summary, or one WARN saying why it wrote nothing.
func (s *Store) RekeyFrom(backup string, moves map[string]string, remove []string, via Via) error {
	start := time.Now()
	entries, err := s.rekeyFrom(backup, moves, remove, via)
	switch {
	case errors.Is(err, errNoBackup):
		logger.Debug("rekey-noop", "op", "rekey-noop", "via", via.String(), "detail", err.Error())
		return nil
	case err != nil:
		attrs := []any{"op", "rekey", "via", via.String(), "error", err}
		if fileutil.IsWriteFailure(err) {
			attrs = append(attrs, "error_class", fileutil.ClassifyWriteError(err))
		}
		logger.Warn("rekey", append(attrs, log.Took(start))...)
		return err
	}
	logger.Info("rekey", "op", "rekey", "entries", entries, "via", via.String(), log.Took(start))
	return nil
}

// rekeyFrom is RekeyFrom's work, returning how many keys it moved or removed.
func (s *Store) rekeyFrom(backup string, moves map[string]string, remove []string, via Via) (int, error) {
	from := slices.Sorted(maps.Keys(moves))
	remove = slices.Compact(slices.Sorted(slices.Values(remove)))
	if err := oneKeyEach(moves, from, remove); err != nil {
		return 0, err
	}
	held, err := readBackup(backup)
	if err != nil {
		return 0, err
	}
	if err := accountedFor(held, slices.Concat(from, remove)); err != nil {
		return 0, err
	}

	lock, err := s.acquireMutationLock()
	if err != nil {
		return 0, err
	}
	defer func() { _ = lock.Close() }()

	file := make(Snapshot, len(from))
	for _, key := range from {
		file[moves[key]] = held[key]
	}
	if err := s.save(file); err != nil {
		return 0, err
	}
	for _, key := range from {
		logger.Debug("rekey-move", "op", "rekey-move", "hook_key", key, "value", moves[key], "via", via.String())
	}
	// The removed commands are the value, so they can be copied back out of
	// the log.
	for _, key := range remove {
		logger.Debug("rekey-rm", "op", "rekey-rm", "hook_key", key, "value", removedValue(held[key]), "via", via.String())
	}
	return len(from) + len(remove), nil
}

// oneKeyEach refuses a rekey that would lose a key's hooks: two keys of moves,
// in order in from, moved onto one, or a key both moved and removed.
func oneKeyEach(moves map[string]string, from, remove []string) error {
	onto := make(map[string]string, len(from))
	for _, key := range from {
		if earlier, taken := onto[moves[key]]; taken {
			return fmt.Errorf("rekey moves both %q and %q to %q", earlier, key, moves[key])
		}
		onto[moves[key]] = key
	}
	for _, key := range remove {
		if to, moving := moves[key]; moving {
			return fmt.Errorf("rekey both moves %q to %q and removes it", key, to)
		}
	}
	return nil
}

// accountedFor refuses named, the keys a rekey moves or removes, unless they
// are the keys held.
func accountedFor(held Snapshot, named []string) error {
	unnamed := slices.DeleteFunc(slices.Sorted(maps.Keys(held)), func(key string) bool { return slices.Contains(named, key) })
	unheld := slices.DeleteFunc(slices.Sorted(slices.Values(named)), func(key string) bool {
		_, ok := held[key]
		return ok
	})
	var why []string
	if len(unnamed) > 0 {
		why = append(why, fmt.Sprintf("the backup holds %q, which it neither moves nor removes", unnamed))
	}
	if len(unheld) > 0 {
		why = append(why, fmt.Sprintf("it names %q, which the backup doesn't hold", unheld))
	}
	if why == nil {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrUnaccountedKeys, strings.Join(why, "; "))
}

// readBackup decodes the backup at path through the codec, so each value keeps
// its bytes.
func readBackup(path string) (Snapshot, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w at %s", errNoBackup, path)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: the backup at %s: %w", ErrStoreRead, path, err)
	}
	var held Snapshot
	if err := json.Unmarshal(data, &held); err != nil {
		return nil, fmt.Errorf("%w: the backup at %s: %w", ErrMalformed, path, err)
	}
	return held, nil
}
