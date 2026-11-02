package sessionindex

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/leeovery/portal/internal/commitlock"
	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/log"
)

// The op verb is both the slog message and a required "op" attr, so the
// `sessions: <verb>` grep idiom and `grep op=set-label` filtering both work.
var logger = log.For("sessions")

var now = time.Now

const fileName = "sessions.json"

// Path is sessions.json in the state directory dir.
func Path(dir string) string { return filepath.Join(dir, fileName) }

// Read returns the index in dir, an empty one when sessions.json is absent.
// It takes no lock: a write replaces the file whole, so a read sees either
// side of it.
func Read(dir string) (Index, error) {
	data, err := os.ReadFile(Path(dir))
	if errors.Is(err, os.ErrNotExist) {
		return emptyIndex(), nil
	}
	if err != nil {
		return Index{}, fmt.Errorf("read sessions.json: %w", err)
	}
	return Decode(data)
}

// Update reads the index in dir under the exclusive commit lock, the one the
// state saver's commit cycle holds, and runs fn on it. When fn returns nil
// having changed something through Index's methods, it stamps saved_at and
// replaces sessions.json atomically; when fn changes nothing or errors,
// nothing is written, and fn's error is returned as fn gave it. A lock not
// granted within the bound returns an error wrapping commitlock.ErrHeld with
// nothing read or written, and an index that cannot be read or decoded is
// returned as an error with fn never run. via records the calling surface:
// cli, tui or internal.
func Update(dir, via string, fn func(*Index) error) error {
	lock, err := commitlock.Acquire(dir)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close() }()

	idx, err := Read(dir)
	if err != nil {
		return err
	}
	if err := fn(&idx); err != nil {
		return err
	}
	if len(idx.changes) == 0 {
		logger.Debug("update-noop", "op", "update-noop", "via", via)
		return nil
	}
	idx.SavedAt = now().UTC()
	data, err := Encode(idx)
	if err != nil {
		return fmt.Errorf("encode sessions.json: %w", err)
	}
	if err := fileutil.AtomicWrite0600(Path(dir), data); err != nil {
		for _, c := range idx.changes {
			logger.Warn(c.op, append(c.attrs(via), "error", err, "error_class", fileutil.ClassifyWriteError(err))...)
		}
		return fmt.Errorf("write sessions.json: %w", err)
	}
	for _, c := range idx.changes {
		logger.Info(c.op, c.attrs(via)...)
	}
	return nil
}

func (c change) attrs(via string) []any {
	attrs := []any{"op", c.op, "session", c.session}
	if c.value != "" {
		attrs = append(attrs, "value", c.value)
	}
	return append(attrs, "via", via)
}
