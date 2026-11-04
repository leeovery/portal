package saver

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
)

const (
	// captureEvery is the least time between two asks for one creation's
	// history.
	captureEvery = time.Minute
	// cleanEvery is the least time between two removals of the scrollback
	// files no entry names.
	cleanEvery = time.Minute
	// abandonedAfter is how old a write's temp file in the scrollback
	// directory must be to be one a crash left, not one a write has under
	// way.
	abandonedAfter = time.Hour
)

// captureBudget bounds, together, the History calls one poll makes, so
// sessions that don't answer delay the poll's commit by no more than it.
var captureBudget = 3 * time.Second

var captureLogger = log.For("capture")

// scrollback saves the sessions' scrollback, each creation's to a file of its
// own in the state directory dir.
type scrollback struct {
	backend backend.Backend
	dir     string
	// creations is what the saver last did with each creation's history, by
	// the creation's scrollback file.
	creations map[string]creation
	// cleaned is when the files no entry names were last removed.
	cleaned time.Time
}

type creation struct {
	asked time.Time
	// named and sum are the history the index last named the file with, not
	// the last written: a write the index didn't name is made again.
	named bool
	sum   [sha256.Size]byte
}

func (c creation) due(at time.Time) bool {
	return c.asked.IsZero() || at.Sub(c.asked) >= captureEvery
}

// capture is a session's history, written to its creation's file, waiting for
// the index to name it.
type capture struct {
	session backend.Session
	file    string
	sum     [sha256.Size]byte
}

// capture saves the history of each session the listing shows answering that
// is due a capture, outside the commit lock, as History can take seconds: at
// most once every captureEvery per creation, never while Portal is restoring
// the session, and writing only a history that differs from the one the
// index last named. A session the poll's captureBudget runs out before is not
// asked, so stays due. It returns what it wrote, for the index to name.
func (sb *scrollback) capture(ctx context.Context, l listing) []capture {
	ctx, cancel := context.WithTimeoutCause(ctx, captureBudget, fmt.Errorf("the poll's %s for captures ran out: %w", captureBudget, backend.ErrTimeout))
	defer cancel()
	listed := map[string]creation{}
	var written []capture
	for _, live := range l.answered {
		file, err := state.SessionScrollbackFile(live.Name, live.Created, live.PID)
		if err != nil {
			continue
		}
		memo := sb.creations[file]
		if ctx.Err() == nil && memo.due(l.at) && !state.Flagged(sb.dir, live.Name) {
			memo.asked = l.at
			if c, ok := sb.save(ctx, live, file, memo); ok {
				written = append(written, c)
			}
		}
		listed[file] = memo
	}
	sb.creations = listed
	return written
}

// save asks for the session's history and writes it to file, unless it is
// the history the index last named the file with, reporting what it wrote.
func (sb *scrollback) save(ctx context.Context, live backend.Session, file string, memo creation) (capture, bool) {
	vt, err := sb.backend.History(ctx, live.Name, 0)
	if err != nil {
		skipped(live.Name, err)
		return capture{}, false
	}
	sum := sha256.Sum256(vt)
	if memo.named && memo.sum == sum {
		return capture{}, false
	}
	target := filepath.Join(sb.dir, filepath.FromSlash(file))
	if err := sb.write(target, vt, live.Name); err != nil {
		captureLogger.Warn("write failed", "session", live.Name, "path", target, "error", err, "error_class", fileutil.ClassifyWriteError(err))
		return capture{}, false
	}
	return capture{session: live, file: file, sum: sum}, true
}

// skipped logs a history the session didn't give: at DEBUG when it had no
// screen to give, didn't answer in time or has ended, as the next poll it is
// due asks again, and at WARN for any other failure.
func skipped(session string, err error) {
	for _, skip := range []error{backend.ErrHistoryUnavailable, backend.ErrTimeout, backend.ErrNoSuchSession} {
		if errors.Is(err, skip) {
			captureLogger.Debug("capture skipped", "session", session, "error", err)
			return
		}
	}
	captureLogger.Warn("capture failed", "session", session, "error", err)
}

// write replaces the file target with vt, making the scrollback directory
// 0700 first. A directory sync that fails once the file has landed is a
// warning, the file written all the same.
func (sb *scrollback) write(target string, vt []byte, session string) error {
	if err := os.MkdirAll(state.ScrollbackDir(sb.dir), 0o700); err != nil {
		return fmt.Errorf("%w: make the scrollback directory: %w", fileutil.ErrWriteTempCreate, err)
	}
	return fileutil.AtomicWrite0600(target, vt, fileutil.ReportUnsynced(func(err error) {
		captureLogger.Warn("unsynced", "session", session, "path", target, "error", err, "error_class", fileutil.ClassifyWriteError(err))
	}))
}

// name names each capture's file in its session's entry of idx, the index as
// read under the commit lock, while the entry still records the creation
// captured and Portal is still not restoring the session, and returns those
// it named. Any other is left unnamed, for clean to remove.
func (sb *scrollback) name(idx *sessionindex.Index, captures []capture) []capture {
	var named []capture
	for _, c := range captures {
		e, ok := idx.Lookup(c.session.Name)
		if !ok || !e.SameCreation(c.session.Created, c.session.PID) || state.Flagged(sb.dir, c.session.Name) {
			continue
		}
		idx.SetScrollback(c.session.Name, c.file)
		named = append(named, c)
	}
	return named
}

// saved remembers the history the index now names each named capture's file
// with, so the same history is not written again.
func (sb *scrollback) saved(named []capture) {
	for _, c := range named {
		memo := sb.creations[c.file]
		memo.named, memo.sum = true, c.sum
		sb.creations[c.file] = memo
	}
}

// clean removes, at most once every cleanEvery, each file in the scrollback
// directory that Portal names for a creation of a session and no entry of idx,
// the index as written, names: a capture the index didn't name, or a file an
// entry left for a later creation's. It removes too each temp file a write
// left there abandonedAfter ago or more. Anything else there, a legacy
// <name>.bin among it, is not the saver's to remove.
func (sb *scrollback) clean(idx *sessionindex.Index, at time.Time) {
	if !sb.cleaned.IsZero() && at.Sub(sb.cleaned) < cleanEvery {
		return
	}
	sb.cleaned = at
	dir := state.ScrollbackDir(sb.dir)
	files, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	if err != nil {
		captureLogger.Warn("clean failed", "path", dir, "error", err)
		return
	}
	named := namedFiles(idx)
	reaped := 0
	for _, f := range files {
		if reapable(f, named, at) && removed(filepath.Join(dir, f.Name())) {
			reaped++
		}
	}
	if reaped > 0 {
		captureLogger.Info("cleaned", "reaped", reaped)
	}
}

func reapable(f fs.DirEntry, named map[string]bool, at time.Time) bool {
	switch {
	case !f.Type().IsRegular():
		return false
	case fileutil.IsAtomicTemp(f.Name()):
		info, err := f.Info()
		return err == nil && at.Sub(info.ModTime()) >= abandonedAfter
	}
	return !named[f.Name()] && state.IsSessionScrollbackFile(f.Name())
}

func removed(target string) bool {
	err := os.Remove(target)
	switch {
	case err == nil:
		captureLogger.Debug("removed", "path", target)
		return true
	case !errors.Is(err, fs.ErrNotExist):
		captureLogger.Warn("remove failed", "path", target, "error", err)
	}
	return false
}

func namedFiles(idx *sessionindex.Index) map[string]bool {
	named := map[string]bool{}
	for _, e := range idx.Entries() {
		if e.ScrollbackFile != "" {
			named[path.Base(e.ScrollbackFile)] = true
		}
	}
	return named
}
