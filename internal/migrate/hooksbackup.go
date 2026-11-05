package migrate

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/hooks"
)

// hooksBackupName is hooks.json's backup, kept in the tmux version's state
// dir, so it moves to state-tmux/ with that state.
const hooksBackupName = "hooks.json.tmux"

// HooksBackup is where migrate keeps its copy of hooks.json in the state dir
// dir.
func HooksBackup(dir string) string { return filepath.Join(dir, hooksBackupName) }

// hooksRewritesName records, beside the backup, the digest of each hooks.json
// a run writes from it, one per line, each before its write lands.
const hooksRewritesName = "hooks.json.tmux.rewrites"

func hooksRewrites(backup string) string {
	return filepath.Join(filepath.Dir(backup), hooksRewritesName)
}

// hooksUnchanged refuses a run that finds a backup while hooks.json, at live,
// is neither byte for byte the backup nor a file an earlier run wrote from it:
// the rewrite would replace it from the backup, losing every hook registered
// since, as after a run cut short and a rollback to the tmux version.
func hooksUnchanged(live, backup string) error {
	left, err := hooksAsLeft(live, backup)
	if err != nil {
		return err
	}
	if !left {
		return refused(ErrHooksChanged, hooksChangedCopy, live, backup)
	}
	return nil
}

// hooksAsLeft reports whether hooks.json, at live, is as migrate left it:
// there is no backup, or it is the backup or a rewrite of it. A hooks.json
// gone since the backup is not.
func hooksAsLeft(live, backup string) (bool, error) {
	backed, err := os.ReadFile(backup)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return true, nil
	case err != nil:
		return false, fmt.Errorf("read hooks.json's backup: %w", err)
	}
	current, err := os.ReadFile(live)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("read hooks.json: %w", err)
	}
	if bytes.Equal(current, backed) {
		return true, nil
	}
	written, err := rewriteDigests(backup)
	return slices.Contains(written, digest(current)), err
}

// recordRewrite adds the digest of file, the hooks.json about to be written
// from backup, to the digests beside it.
func recordRewrite(backup string, file []byte) error {
	written, err := rewriteDigests(backup)
	if err != nil {
		return err
	}
	written = append(written, digest(file))
	if err := fileutil.AtomicWrite0600(hooksRewrites(backup), []byte(strings.Join(written, "\n")+"\n")); err != nil {
		return fmt.Errorf("record hooks.json's rewrite: %w", err)
	}
	return nil
}

// rewriteDigests is the digest of every hooks.json a run wrote from backup,
// none before the first.
func rewriteDigests(backup string) ([]string, error) {
	data, err := os.ReadFile(hooksRewrites(backup))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read the record of hooks.json's rewrites: %w", err)
	}
	return strings.Fields(string(data)), nil
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// backUpHooks copies hooks.json, at live, to backup, unless backup exists:
// a backup is never overwritten, since a run cut short may have rewritten
// hooks.json since. It decodes hooks.json through the hooks codec first, as
// strictly as the tmux version read it, so a file that doesn't parse is
// malformedHooks, matching ErrMalformed, before anything is written, and it copies
// the bytes it decoded. A fresh backup starts a fresh rewrite record: any
// record already there belongs to a backup moved aside, and its digests must
// not vouch for hooks.json against this one. The record goes first, so a run
// that dies between the two leaves no backup beside a stale record. It
// reports whether it wrote a backup: no hooks.json writes none.
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
	if err := os.Remove(hooksRewrites(backup)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, fmt.Errorf("remove the rewrite record of an earlier backup: %w", err)
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
// A file that doesn't parse is malformedHooks, matching ErrMalformed.
func decodeHooks(data []byte, path string) (hooks.Snapshot, error) {
	var held hooks.Snapshot
	if err := json.Unmarshal(data, &held); err != nil {
		return nil, malformedHooks{path: path, err: err}
	}
	return held, nil
}
