package sessionindex

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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

// Outcome is what a written Update dropped, each entry as Update read it: those
// fn removed, and those it replaced with an entry made afresh, in the order
// they were read. What Portal keeps beside an entry goes with it, so a caller
// forgets these entries in UpdateThen's landed, under the same lock.
type Outcome struct {
	Removed  []Entry
	Replaced []Entry
}

// Dropped is every entry the outcome lists.
func (o Outcome) Dropped() []Entry {
	return slices.Concat(o.Removed, o.Replaced)
}

// outcomeOf compares the entries read with the index fn left. It judges by the
// entries, not the change log: an fn that removes a name and claims it again
// leaves an entry under the name that is not the one read.
func outcomeOf(read []Entry, idx Index) Outcome {
	var o Outcome
	for _, was := range read {
		switch {
		case idx.indexOf(was.Name) < 0:
			o.Removed = append(o.Removed, was)
		case idx.fresh[was.Name]:
			o.Replaced = append(o.Replaced, was)
		}
	}
	return o
}

// Update reads the index in dir under the exclusive commit lock, the one the
// state saver's commit cycle holds, and runs fn on it. When fn returns nil
// having changed something through Index's methods, it stamps saved_at and
// replaces sessions.json atomically, returning what that write dropped; when
// fn changes nothing or errors, nothing is written, and fn's error is
// returned as fn gave it. A lock not granted within the bound returns an error
// wrapping commitlock.ErrHeld with nothing read or written, and an index that
// cannot be read or decoded is returned as an error with fn never run. The
// outcome is empty whenever nothing was written. via records the calling
// surface: cli, tui or internal.
func Update(dir, via string, fn func(*Index) error) (Outcome, error) {
	return UpdateThen(dir, via, fn, nil)
}

// UpdateThen is Update, running landed, when given, before the commit lock is
// released: once fn's changes have landed, with what the write dropped, or
// once fn changed nothing, with an empty outcome. It is where a caller forgets
// what went with a dropped entry, and writes what goes beside an entry, so no
// claim, kill or hook set slips in between. landed runs for no Update that
// failed. An error landed returns is UpdateThen's, the outcome's write having
// landed all the same.
func UpdateThen(dir, via string, fn func(*Index) error, landed func(Outcome) error) (Outcome, error) {
	lock, err := commitlock.Acquire(dir)
	if err != nil {
		return Outcome{}, err
	}
	defer func() { _ = lock.Close() }()

	outcome, err := update(dir, via, fn)
	if err == nil && landed != nil {
		err = landed(outcome)
	}
	return outcome, err
}

// update is Update's work under the commit lock.
func update(dir, via string, fn func(*Index) error) (Outcome, error) {
	idx, err := Read(dir)
	if err != nil {
		return Outcome{}, err
	}
	read := slices.Clone(idx.sessions)
	if err := fn(&idx); err != nil {
		return Outcome{}, err
	}
	if len(idx.changes) == 0 {
		logger.Debug("update-noop", "op", "update-noop", "via", via)
		return Outcome{}, nil
	}
	idx.SavedAt = now().UTC()
	data, err := Encode(idx)
	if err != nil {
		return Outcome{}, fmt.Errorf("encode sessions.json: %w", err)
	}
	// Once the rename lands the changes have: a sync after it that fails is
	// a warning, not a write to undo.
	unsynced := fileutil.ReportUnsynced(func(err error) {
		logger.Warn("unsynced", "op", "unsynced", "error", err, "error_class", fileutil.ClassifyWriteError(err), "via", via)
	})
	if err := fileutil.AtomicWrite0600(Path(dir), data, unsynced); err != nil {
		for _, c := range idx.changes {
			logger.Warn(c.op, append(c.attrs(via), "error", err, "error_class", fileutil.ClassifyWriteError(err))...)
		}
		return Outcome{}, fmt.Errorf("write sessions.json: %w", err)
	}
	for _, c := range idx.changes {
		logger.Log(context.Background(), c.level, c.op, c.attrs(via)...)
	}
	return outcomeOf(read, idx), nil
}

func (c change) attrs(via string) []any {
	attrs := []any{"op", c.op}
	if c.session != "" {
		attrs = append(attrs, "session", c.session)
	}
	if c.value != "" {
		attrs = append(attrs, "value", c.value)
	}
	if c.detail != "" {
		attrs = append(attrs, "detail", c.detail)
	}
	return append(attrs, "via", via)
}
