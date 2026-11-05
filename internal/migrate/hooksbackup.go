package migrate

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/hooks"
)

// hooksBackupName is hooks.json's backup, kept in the tmux version's state
// dir, so it moves to state-tmux/ with that state.
const hooksBackupName = "hooks.json.tmux"

// HooksBackup is where migrate keeps its copy of hooks.json in the state dir
// dir.
func HooksBackup(dir string) string { return filepath.Join(dir, hooksBackupName) }

// backUpHooks copies hooks.json, at live, to backup, unless backup exists:
// a backup is never overwritten, since a run cut short may have rewritten
// hooks.json since. It decodes hooks.json through the hooks codec first, as
// strictly as the tmux version read it, so a file that doesn't parse is
// refused, matching ErrMalformed, before anything is written, and it copies
// the bytes it decoded. It reports whether it wrote a backup: no hooks.json
// writes none.
func backUpHooks(live, backup string) (bool, error) {
	switch _, err := os.Lstat(backup); {
	case err == nil:
		return false, nil
	case !errors.Is(err, fs.ErrNotExist):
		return false, fmt.Errorf("look for hooks.json's backup: %w", err)
	}
	data, err := os.ReadFile(live)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read hooks.json: %w", err)
	}
	if _, err := decodeHooks(data, live); err != nil {
		return false, err
	}
	if err := fileutil.AtomicWrite0600(backup, data); err != nil {
		return false, fmt.Errorf("back up hooks.json: %w", err)
	}
	return true, nil
}

// hooksSource is the file a run plans its hooks from: the backup a run cut
// short kept, which backUpHooks never overwrites, else hooks.json itself.
func hooksSource(live, backup string) (string, error) {
	switch present, err := exists(backup); {
	case err != nil:
		return "", err
	case present:
		return backup, nil
	}
	return live, nil
}

// backupKeys is every key hooks.json's backup holds, none when there is no
// backup.
func backupKeys(backup string) ([]string, error) {
	held, err := readHooks(backup)
	if err != nil {
		return nil, err
	}
	return slices.Sorted(maps.Keys(held)), nil
}

// readHooks decodes the hooks file at path through the hooks codec, nothing
// when there is no file.
func readHooks(path string) (hooks.Snapshot, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return decodeHooks(data, path)
}

// decodeHooks decodes data, the hooks file at path, through the hooks codec.
// A file that doesn't parse is refused, matching ErrMalformed.
func decodeHooks(data []byte, path string) (hooks.Snapshot, error) {
	var held hooks.Snapshot
	if err := json.Unmarshal(data, &held); err != nil {
		return nil, refused(hooks.ErrMalformed, hooksMalformedCopy, path, err)
	}
	return held, nil
}
