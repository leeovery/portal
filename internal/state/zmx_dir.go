package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/leeovery/portal/internal/fileutil"
)

const zmxDirName = "zmx-dir"

// zmxDirRecord is what the zmx-dir file holds: the zmx directory, canonical,
// whose sessions the state directory's index, flags and scrollback describe.
type zmxDirRecord struct {
	ZmxDir string `json:"zmx_dir"`
}

// ZmxDirRecordPath is the record, in the state directory dir, of the zmx
// directory dir belongs to.
func ZmxDirRecordPath(dir string) string { return filepath.Join(dir, zmxDirName) }

// ReadZmxDir is the zmx directory dir's record names, empty when dir holds no
// record. A record that can't be read, doesn't decode or names no absolute
// directory is an error: it proves nothing about whose state dir is. The
// error is the cause alone, naming no file: its caller names
// ZmxDirRecordPath, as the user is told to delete it.
func ReadZmxDir(dir string) (string, error) {
	data, err := os.ReadFile(ZmxDirRecordPath(dir))
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if pathErr, ok := errors.AsType[*fs.PathError](err); ok {
		return "", pathErr.Err
	}
	if err != nil {
		return "", err
	}
	var r zmxDirRecord
	if err := json.Unmarshal(data, &r); err != nil {
		return "", fmt.Errorf("it doesn't decode: %w", err)
	}
	if !filepath.IsAbs(r.ZmxDir) {
		return "", fmt.Errorf("%q is not an absolute directory", r.ZmxDir)
	}
	return r.ZmxDir, nil
}

// RecordZmxDir records zmxDir, absolute, as the zmx directory dir belongs to,
// replacing any record whole. Its error is the cause alone, as ReadZmxDir's
// is.
func RecordZmxDir(dir, zmxDir string) error {
	if !filepath.IsAbs(zmxDir) {
		return fmt.Errorf("%q is not an absolute directory", zmxDir)
	}
	data, err := json.Marshal(zmxDirRecord{ZmxDir: zmxDir})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := fileutil.AtomicWrite0600(ZmxDirRecordPath(dir), data); err != nil {
		return err
	}
	bootstrapLogger.Info("zmx dir recorded", "path", zmxDir)
	return nil
}
