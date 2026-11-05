package hooks

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"time"

	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/log"
)

// ErrKeyHeld refuses a rekey moving a key onto one the backup holds and the
// rewrite keeps: the move would lose that key's hooks.
var ErrKeyHeld = errors.New("the backup holds that key, and the rewrite keeps it")

// errNoBackup is a rekey with no backup to write hooks.json from.
var errNoBackup = errors.New("no backup of hooks.json")

// RekeyFrom writes hooks.json as backup, the copy of it portal migrate keeps,
// rewritten by migrate's plan: every hook of each key of moves under the key
// it maps to, each event's value carried as backup holds it, byte for byte,
// and each key of remove gone. A key both removed and moved onto holds the
// moved hooks. It never reads hooks.json, so the same backup and plan always
// write the same file, in one write under the mutation hold. No backup, as
// when there was never a hooks.json, writes nothing. A key backup doesn't hold
// is passed over. Two keys moved onto one, a key both moved and removed, a
// move onto a key backup holds that the rewrite keeps (ErrKeyHeld), a backup
// that can't be read (ErrStoreRead) or doesn't parse (ErrMalformed), and a
// lock not granted each refuse it, writing nothing. It logs each key it moved
// or removed at DEBUG and one INFO summary, or one WARN saying why it wrote
// nothing.
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

	lock, err := s.acquireMutationLock()
	if err != nil {
		return 0, err
	}
	defer func() { _ = lock.Close() }()

	r, err := rekey(held, moves, from, remove)
	if err != nil {
		return 0, err
	}
	if err := s.save(r.file); err != nil {
		return 0, err
	}
	for _, key := range r.moved {
		logger.Debug("rekey-move", "op", "rekey-move", "hook_key", key, "value", moves[key], "via", via.String())
	}
	// The removed commands are the value, so they can be copied back out of
	// the log.
	for _, key := range r.removed {
		logger.Debug("rekey-rm", "op", "rekey-rm", "hook_key", key, "value", removedValue(held[key]), "via", via.String())
	}
	return len(r.moved) + len(r.removed), nil
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
	if held == nil {
		held = Snapshot{}
	}
	return held, nil
}

// rekeyed is a rekey's file, and the keys it moved and removed.
type rekeyed struct {
	file           Snapshot
	moved, removed []string
}

// rekey is held with each key of from that it holds moved to the key moves
// maps it to, and each key of remove it holds gone.
func rekey(held Snapshot, moves map[string]string, from, remove []string) (rekeyed, error) {
	absent := func(key string) bool {
		_, ok := held[key]
		return !ok
	}
	r := rekeyed{
		file:    maps.Clone(held),
		moved:   slices.DeleteFunc(from, func(key string) bool { return absent(key) || moves[key] == key }),
		removed: slices.DeleteFunc(remove, absent),
	}
	for _, key := range slices.Concat(r.moved, r.removed) {
		delete(r.file, key)
	}
	for _, key := range r.moved {
		if _, kept := r.file[moves[key]]; kept {
			return rekeyed{}, fmt.Errorf("move %q to %q: %w", key, moves[key], ErrKeyHeld)
		}
	}
	for _, key := range r.moved {
		r.file[moves[key]] = held[key]
	}
	return r, nil
}
