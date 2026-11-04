package saver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
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
	// captureBudget is how long into a poll's captures it may start another
	// History. One under way is never cut short, keeping the backend's own
	// ceiling, so a session whose history takes longer than the budget is
	// still saved, and a poll's captures take at most the budget and one
	// call.
	captureBudget = 3 * time.Second
)

var captureLogger = log.For("capture")

// scrollback saves the sessions' scrollback, each creation's to a file of its
// own in the state directory dir.
type scrollback struct {
	backend backend.Backend
	dir     string
	now     func() time.Time
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

// capture saves, for a poll, the history of each session the listing shows
// answering that is due a capture: at most once every captureEvery per
// creation. A session the poll's captureBudget runs out before is not asked,
// so stays due.
func (sb *scrollback) capture(ctx context.Context, l listing) []capture {
	until := sb.now().Add(captureBudget)
	return sb.each(ctx, l, func(memo creation) bool {
		return sb.now().Before(until) && memo.due(l.at)
	})
}

// flush saves, for a flush, the history of every session the listing shows
// answering, however recently it was asked.
func (sb *scrollback) flush(ctx context.Context, l listing) []capture {
	return sb.each(ctx, l, func(creation) bool { return true })
}

// each saves the history of each session the listing shows answering whose
// creation ask takes, by what the saver last did with it, outside the commit
// lock, as History can take seconds: never once ctx ends, never while Portal
// is restoring the session, and writing only a history that differs from the
// one the index last named. It returns what it wrote, for the index to name.
func (sb *scrollback) each(ctx context.Context, l listing, ask func(creation) bool) []capture {
	listed := map[string]creation{}
	var written []capture
	for _, live := range l.answered {
		file, err := state.SessionScrollbackFile(live.Name, live.Created, live.PID)
		if err != nil {
			continue
		}
		memo := sb.creations[file]
		if ctx.Err() == nil && ask(memo) && !state.Flagged(sb.dir, live.Name) {
			if c, ok := sb.save(ctx, live, file, &memo, l.at); ok {
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
// The ask marks the creation asked at at, unless the session is on the
// alternate screen: that history holds none of the main screen and its
// scrollback, so it never replaces a saved transcript, and the creation
// stays due, saved once the session is back on its main screen.
func (sb *scrollback) save(ctx context.Context, live backend.Session, file string, memo *creation, at time.Time) (capture, bool) {
	vt, err := sb.backend.History(ctx, live.Name, 0)
	if err == nil && alternateScreen(vt) {
		skipped(live.Name, errAlternateScreen)
		return capture{}, false
	}
	memo.asked = at
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

// errAlternateScreen skips the history of a session on the alternate screen.
var errAlternateScreen = errors.New("the session is on the alternate screen, whose history holds none of its main screen")

// skipped logs a history the saver didn't save: at DEBUG when the session had
// no screen to give, didn't answer in time, has ended or is on the alternate
// screen, as the next poll it is due asks again, and at WARN for any other
// failure.
func skipped(session string, err error) {
	for _, skip := range []error{backend.ErrHistoryUnavailable, backend.ErrTimeout, backend.ErrNoSuchSession, errAlternateScreen} {
		if errors.Is(err, skip) {
			captureLogger.Debug("capture skipped", "session", session, "error", err)
			return
		}
	}
	captureLogger.Warn("capture failed", "session", session, "error", err)
}

// alternateScreenModes are the DEC private modes that put a terminal on its
// alternate screen.
var alternateScreenModes = []string{"47", "1047", "1049"}

// alternateScreen reports whether vt, a session's history, is that of a
// session on the alternate screen. zmx leads a history with the modes that
// differ from a terminal's defaults, each a DECSET or DECRST of its own
// (ESC [ ? n h, or l), ahead of the screen's first cell; on the alternate
// screen 47, 1047 or 1049 is set among them, and the history holds that
// screen alone. Only the lead is read, so a mode sequence later in a
// history is never taken for one.
func alternateScreen(vt []byte) bool {
	for {
		params, set, rest, ok := leadingMode(vt)
		if !ok {
			return false
		}
		if set && slices.ContainsFunc(strings.Split(params, ";"), func(mode string) bool {
			return slices.Contains(alternateScreenModes, mode)
		}) {
			return true
		}
		vt = rest
	}
}

// leadingMode is the DEC private mode sequence vt starts with: its
// parameters, whether it sets them (h) or resets them (l), and what follows.
func leadingMode(vt []byte) (params string, set bool, rest []byte, ok bool) {
	body, ok := bytes.CutPrefix(vt, []byte("\x1b[?"))
	if !ok {
		return "", false, nil, false
	}
	end := bytes.IndexFunc(body, func(r rune) bool { return r != ';' && (r < '0' || r > '9') })
	if end < 0 || body[end] != 'h' && body[end] != 'l' {
		return "", false, nil, false
	}
	return string(body[:end]), body[end] == 'h', body[end+1:], true
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
// <name>.bin among it, is not the saver's to remove. It returns how many
// files it removed.
func (sb *scrollback) clean(idx *sessionindex.Index, at time.Time) int {
	if !sb.cleaned.IsZero() && at.Sub(sb.cleaned) < cleanEvery {
		return 0
	}
	sb.cleaned = at
	dir := state.ScrollbackDir(sb.dir)
	files, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return 0
	}
	if err != nil {
		captureLogger.Warn("clean failed", "path", dir, "error", err)
		return 0
	}
	named := namedFiles(idx)
	reaped := 0
	for _, f := range files {
		if reapable(f, named, at) && removed(filepath.Join(dir, f.Name())) {
			reaped++
		}
	}
	if reaped > 0 {
		captureLogger.Debug("cleaned", "reaped", reaped)
	}
	return reaped
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
