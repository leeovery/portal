package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/leeovery/portal/internal/devversion"
	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/log"
)

// The latch is the bootstrap's record of itself, so it logs under the
// bootstrap's component.
var bootstrapLogger = log.For("bootstrap")

const bootstrappedName = "bootstrapped"

// ErrLatchIncomplete refuses a latch missing its boot id or version, which no
// running Portal could ever match.
var ErrLatchIncomplete = errors.New("a bootstrapped latch needs a boot id and a version")

// Latch is what the bootstrapped file records: the boot, and the Portal
// version, of the last bootstrap that finished. A reboot or another version
// owes a new bootstrap.
type Latch struct {
	BootID  string `json:"boot_id"`
	Version string `json:"version"`
}

func (l Latch) complete() bool { return l.BootID != "" && l.Version != "" }

func Bootstrapped(dir string) string { return filepath.Join(dir, bootstrappedName) }

// WriteLatch records latch in dir, replacing any earlier one whole.
func WriteLatch(dir string, latch Latch) error {
	if !latch.complete() {
		return fmt.Errorf("write bootstrapped latch %+v: %w", latch, ErrLatchIncomplete)
	}
	data, err := json.Marshal(latch)
	if err != nil {
		return fmt.Errorf("write bootstrapped latch: %w", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("write bootstrapped latch: %w", err)
	}
	if err := fileutil.AtomicWrite0600(Bootstrapped(dir), data, fileutil.ReportUnsynced(warnLatchUnsynced)); err != nil {
		return fmt.Errorf("write bootstrapped latch: %w", err)
	}
	return nil
}

// warnLatchUnsynced logs a latch that landed though the directory sync after
// its rename failed: the latch stands, and the bootstrap is recorded.
func warnLatchUnsynced(err error) {
	bootstrapLogger.Warn("latch unsynced", "error", err, "error_class", fileutil.ClassifyWriteError(err))
}

// ReadLatch returns the latch recorded in dir. A missing file is an error
// matching fs.ErrNotExist; one that does not decode is an error too.
func ReadLatch(dir string) (Latch, error) {
	data, err := os.ReadFile(Bootstrapped(dir))
	if err != nil {
		return Latch{}, fmt.Errorf("read bootstrapped latch: %w", err)
	}
	var latch Latch
	if err := json.Unmarshal(data, &latch); err != nil {
		return Latch{}, fmt.Errorf("read bootstrapped latch: %w", err)
	}
	return latch, nil
}

// LatchCurrent reports whether the latch in dir was written by a bootstrap
// of this boot and this version, as running records them. A latch that is
// missing, unreadable or corrupt is not current. Nor is a match on a latch
// lacking either half, or on devversion.Unidentified, a version any build
// that can't say what it is reports: neither says which bootstrap wrote it.
func LatchCurrent(dir string, running Latch) bool {
	latch, err := ReadLatch(dir)
	return err == nil && running.complete() && running.Version != devversion.Unidentified && latch == running
}
