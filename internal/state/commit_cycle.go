package state

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/leeovery/portal/internal/fileutil"
	"golang.org/x/sys/unix"
)

const commitLockName = "commit.lock"

// ErrCommitLockHeld separates contention on the commit lock from a genuine
// open(2)/flock failure.
var ErrCommitLockHeld = errors.New("commit lock held by another committer")

// commitLockTimeout bounds the acquire. An unbounded acquire would park the
// daemon's tick loop behind a holder that is alive but stuck.
var commitLockTimeout = 5 * time.Second

var commitLockPollInterval = 5 * time.Millisecond

// CommitLock is the sidecar every committing cycle holds exclusively. It is not
// daemon.lock, which the daemon holds for its whole life.
func CommitLock(dir string) string { return filepath.Join(dir, commitLockName) }

// CommitCycle is one committing cycle: capture, re-file, the caller's dump,
// then the commit and its housekeeping pass.
type CommitCycle struct {
	Client CaptureCycleClient
	// OwnServer is the pid of the tmux server the committer belongs to. The
	// cycle commits only on a confirmation that server answered; zero commits
	// nothing, and sends no confirmation.
	OwnServer int
	Dir       string
	// LoadPrev supplies the previous index only when sessions.json, read under
	// the commit lock, is absent or cannot be read or decoded, which the cycle
	// has logged at WARN before calling it. It is called under the lock.
	LoadPrev func() *Index
	HashMap  HashMap
	// Dump writes the caller's scrollback for the capture through the writer
	// it is handed and reports whether any file changed. The writer refuses
	// every pane CaptureCycle.SkipsScrollback answers true for. Nil dumps
	// nothing. An error ends the cycle uncommitted.
	Dump   func(CaptureCycle, ScrollbackWriter) (bool, error)
	Logger *slog.Logger
}

// ScrollbackWriter writes captured scrollback for the commit cycle that handed
// it out, against that cycle's own server, state directory and dedup map. A
// pane's positional name is contested when sessions.json, as the cycle read it
// under the commit lock, names it on a record carrying another pane's token.
// The writer never writes a contested name during the dump: a pane whose token
// the pane-token rule accepts is written under its token-named transcript,
// which its record then names, and any other contested pane's capture is
// deferred until the cycle's sessions.json write has landed.
type ScrollbackWriter struct {
	capture   CaptureCycle
	confirmer AnsweringConfirmer
	ownServer int
	dir       string
	hm        HashMap
	// held maps a pane key to the file keepAnsweredTranscripts held it on —
	// its token-named transcript, or the file its last committed record named —
	// which its record names in this cycle and its empty capture is judged
	// against.
	held map[string]string
	// contested maps the key of every pane whose positional name is contested
	// to that pane's token, empty for none.
	contested map[string]string
	// filed maps every pane key this cycle wrote a capture for, held or
	// contested, to the file its record names once written.
	filed map[string]string
	// deferred maps the key of every contested pane with no usable token to its capture.
	deferred map[string]deferredCapture
}

type deferredCapture struct {
	data []byte
	hash uint64
}

// Write writes data, just captured for paneKey, unless hash matches the
// cycle's dedup entry, reporting whether it wrote. A pane the cycle skips is
// never written: Write returns (false, nil) having sent no confirmation and
// touched no dedup entry or file. An empty capture over a saved transcript the
// cycle's own server does not confirm writes nothing and returns an error
// wrapping ErrUnconfirmedEmptyCapture. A pane's saved transcript is the file
// its record names.
//
// A pane whose positional name is contested is written under its token-named
// transcript, whatever its dedup entry, and its record names that file in this
// cycle's commit. One whose token the pane-token rule refuses, or which has
// none, returns (false, nil) with its dedup entry untouched: its capture is
// written at its positional name only once this cycle's sessions.json write
// lands.
func (w ScrollbackWriter) Write(paneKey string, data []byte, hash uint64) (bool, error) {
	if w.capture.SkipsScrollback(paneKey) {
		return false, nil
	}
	if token, contested := w.contested[paneKey]; contested {
		return w.writeContested(paneKey, token, data, hash)
	}
	_, held := w.held[paneKey]
	if held {
		// The positional file the dedup entry describes may since have been
		// removed by housekeeping, and a dedup hit would then keep the pane
		// off its new capture for good.
		delete(w.hm, paneKey)
	}
	if err := confirmEmptyCapture(w.confirmer, w.ownServer, w.savedTranscript(paneKey), data); err != nil {
		return false, err
	}
	written, err := WriteScrollbackIfChanged(w.dir, paneKey, data, hash, w.hm)
	if written && held {
		w.filed[paneKey] = positionalScrollbackFile(paneKey)
	}
	return written, err
}

// writeContested writes without consulting the dedup map: the token-named
// file may since have been re-linked to other bytes.
func (w ScrollbackWriter) writeContested(paneKey, token string, data []byte, hash uint64) (bool, error) {
	if err := confirmEmptyCapture(w.confirmer, w.ownServer, w.savedTranscript(paneKey), data); err != nil {
		return false, err
	}
	path, ok := PendingScrollbackPath(w.dir, token)
	if !ok {
		w.deferred[paneKey] = deferredCapture{data: bytes.Clone(data), hash: hash}
		return false, nil
	}
	if err := fileutil.AtomicWrite0600(path, data); err != nil {
		return false, fmt.Errorf("write scrollback %s: %w", paneKey, err)
	}
	w.filed[paneKey] = PendingScrollbackFile(token)
	return true, nil
}

func (w ScrollbackWriter) savedTranscript(paneKey string) string {
	if stored, held := w.held[paneKey]; held {
		return joinStored(w.dir, stored)
	}
	return ScrollbackFile(w.dir, paneKey)
}

// RunCommitCycle runs cycle under the exclusive commit lock, held from the
// skeleton-marker read to the end of the housekeeping pass, so no two
// committers' captures and collections interleave. A lock not granted within
// the bound returns an error wrapping ErrCommitLockHeld with nothing read or
// written. A failed capture returns before the dump, and a failed dump before
// the commit, its error returned as the dump gave it.
//
// The cycle's previous index is sessions.json as read under the lock, which a
// caller's own index can lag behind once another committer has committed;
// LoadPrev supplies it only when sessions.json cannot be read. The skeleton
// merge, the waiting-pane merge, the carry and the hold all take their records
// from that one index.
//
// A tokened pane whose last committed record names a file other than its live
// positional file keeps those bytes on its record, under its token-named
// transcript, until a dump writes its new capture — unless that file cannot be
// linked under its token, or another such pane's last record names it too.
//
// A capture the writer deferred is written at its positional name, under the
// lock, only once sessions.json has been written, and only when the index just
// committed names that file on the deferred pane's record alone. A failed
// write is logged at WARN and does not fail the cycle, which has committed.
func RunCommitCycle(cycle CommitCycle) (CaptureCycle, error) {
	lock, err := acquireCommitLock(cycle.Dir)
	if err != nil {
		return CaptureCycle{}, err
	}
	defer func() { _ = lock.Close() }()

	committed, absent, err := readPriorIndex(cycle.Dir)
	prev := committed
	if prev == nil {
		logUnreadIndex(cycle.Logger, absent, err)
		prev = cycle.LoadPrev()
	}
	capture, err := captureAndRefile(cycle.Client, cycle.OwnServer, cycle.Dir, prev, cycle.HashMap, cycle.Logger)
	if err != nil {
		return capture, fmt.Errorf("capture: %w", err)
	}
	if prev != nil {
		keepAnsweredTranscripts(&capture, *prev, cycle.Dir, cycle.Logger)
	}
	changed := false
	deferred := map[string]deferredCapture{}
	if cycle.Dump != nil {
		writer := ScrollbackWriter{
			capture:   capture,
			confirmer: cycle.Client,
			ownServer: cycle.OwnServer,
			dir:       cycle.Dir,
			hm:        cycle.HashMap,
			held:      heldTranscripts(capture),
			contested: contestedPanes(capture.Index, committed),
			filed:     map[string]string{},
			deferred:  deferred,
		}
		if changed, err = cycle.Dump(capture, writer); err != nil {
			return capture, err
		}
		fileWritten(&capture.Index, writer.filed)
	}
	wrote, err := commitOver(cycle.Dir, capture.Index, committed, changed, cycle.Logger)
	if err != nil {
		return capture, fmt.Errorf("commit: %w", err)
	}
	if wrote {
		writeDeferredCaptures(cycle.Dir, capture.Index, deferred, cycle.HashMap, cycle.Logger)
	}
	return capture, nil
}

// writeDeferredCaptures ignores a deferred pane's dedup entry: that entry may
// describe the file before another pane's bytes were filed under its name.
func writeDeferredCaptures(dir string, committed Index, deferred map[string]deferredCapture, hm HashMap, logger *slog.Logger) {
	if len(deferred) == 0 {
		return
	}
	logger = loggerOrDiscard(logger)
	naming := recordKeysPerScrollbackFile(committed)
	for paneKey, kept := range deferred {
		if !slices.Equal(naming[positionalScrollbackFile(paneKey)], []string{paneKey}) {
			continue
		}
		delete(hm, paneKey)
		if _, err := WriteScrollbackIfChanged(dir, paneKey, kept.data, kept.hash, hm); err != nil {
			logger.Warn("write scrollback failed", "pane_key", paneKey, "error", err)
		}
	}
}

func recordKeysPerScrollbackFile(idx Index) map[string][]string {
	naming := map[string][]string{}
	for _, s := range idx.Sessions {
		for _, w := range s.Windows {
			for _, p := range w.Panes {
				naming[p.ScrollbackFile] = append(naming[p.ScrollbackFile], SanitizePaneKey(s.Name, w.Index, p.Index))
			}
		}
	}
	return naming
}

func logUnreadIndex(logger *slog.Logger, absent bool, cause error) {
	logger = loggerOrDiscard(logger)
	if absent {
		logger.Warn("sessions.json absent; committing without the saved index")
		return
	}
	logger.Warn("read sessions.json failed; committing without the saved index", "error", cause)
}

// keepAnsweredTranscripts points each tokened pane the dump may write back at
// the file its record in from, matched on its token, names when that file is
// not the pane's live positional file: that file holds the pane's bytes, and
// housekeeping would delete it once no record named it. A pane's own
// token-named transcript is held whatever else names it, so any other such
// file is first given a second name under the pane's token: a later write to
// the original name replaces that name and leaves the pane's bytes under the
// token. A pane whose file cannot be linked, or which another such pane claims,
// is held only on a file no other record in the capture names and no other
// such pane claims, so no commit names one file on two records.
func keepAnsweredTranscripts(capture *CaptureCycle, from Index, dir string, logger *slog.Logger) {
	byToken, _ := indexPrevPanes(from, nil)
	named := recordsPerScrollbackFile(capture.Index)
	holds := map[*Pane]heldFile{}
	claims := map[string]int{}
	for si := range capture.Index.Sessions {
		s := &capture.Index.Sessions[si]
		for wi := range s.Windows {
			w := &s.Windows[wi]
			for pi := range w.Panes {
				p := &w.Panes[pi]
				key := SanitizePaneKey(s.Name, w.Index, p.Index)
				if p.PortalPaneID == "" || capture.SkipsScrollback(key) {
					continue
				}
				record, found := byToken[p.PortalPaneID]
				if !found || record.ScrollbackFile == "" || record.ScrollbackFile == p.ScrollbackFile {
					continue
				}
				holds[p] = heldFile{paneKey: key, stored: record.ScrollbackFile}
				claims[record.ScrollbackFile]++
			}
		}
	}
	logger = loggerOrDiscard(logger)
	for p, held := range holds {
		file := held.stored
		if claims[file] == 1 {
			file = linkHeldTranscript(dir, held, p.PortalPaneID, logger)
		}
		if file == PendingScrollbackFile(p.PortalPaneID) || (named[file] == 0 && claims[file] == 1) {
			p.ScrollbackFile = file
		}
	}
}

type heldFile struct {
	paneKey string
	stored  string
}

// linkHeldTranscript returns the pane's token-named transcript once held.stored
// is linked to it, and held.stored itself when the token is one the pane-token
// rule refuses or the link fails.
func linkHeldTranscript(dir string, held heldFile, token string, logger *slog.Logger) string {
	if _, ok := PendingScrollbackPath(dir, token); !ok {
		return held.stored
	}
	tokenPath := PendingScrollbackFile(token)
	if held.stored == tokenPath {
		return tokenPath
	}
	if err := linkStoredScrollback(dir, held.stored, tokenPath); err != nil {
		logger.Warn("link held scrollback failed", "pane_key", held.paneKey, "path", held.stored, "error", err)
		return held.stored
	}
	return tokenPath
}

// heldTranscripts maps the key of every tokened pane the dump may write whose
// record names a file other than its positional file to that stored path: the
// file keepAnsweredTranscripts held it on.
func heldTranscripts(capture CaptureCycle) map[string]string {
	held := map[string]string{}
	for _, s := range capture.Index.Sessions {
		for _, w := range s.Windows {
			for _, p := range w.Panes {
				key := SanitizePaneKey(s.Name, w.Index, p.Index)
				if p.PortalPaneID == "" || p.ScrollbackFile == positionalScrollbackFile(key) || capture.SkipsScrollback(key) {
					continue
				}
				held[key] = p.ScrollbackFile
			}
		}
	}
	return held
}

// contestedPanes ignores a record carrying no token: it may be this same pane,
// stamped since it was saved.
func contestedPanes(idx Index, committed *Index) map[string]string {
	contested := map[string]string{}
	if committed == nil {
		return contested
	}
	tokensNaming := map[string][]string{}
	for _, s := range committed.Sessions {
		for _, w := range s.Windows {
			for _, p := range w.Panes {
				if p.PortalPaneID != "" {
					tokensNaming[p.ScrollbackFile] = append(tokensNaming[p.ScrollbackFile], p.PortalPaneID)
				}
			}
		}
	}
	for _, s := range idx.Sessions {
		for _, w := range s.Windows {
			for _, p := range w.Panes {
				key := SanitizePaneKey(s.Name, w.Index, p.Index)
				for _, token := range tokensNaming[positionalScrollbackFile(key)] {
					if token != p.PortalPaneID {
						contested[key] = p.PortalPaneID
						break
					}
				}
			}
		}
	}
	return contested
}

func fileWritten(idx *Index, filed map[string]string) {
	if len(filed) == 0 {
		return
	}
	for si := range idx.Sessions {
		s := &idx.Sessions[si]
		for wi := range s.Windows {
			w := &s.Windows[wi]
			for pi := range w.Panes {
				key := SanitizePaneKey(s.Name, w.Index, w.Panes[pi].Index)
				if file, ok := filed[key]; ok {
					w.Panes[pi].ScrollbackFile = file
				}
			}
		}
	}
}

// The lock is released by closing the returned file, and by the kernel when
// the holding process exits, so a crashed holder never wedges the next one.
func acquireCommitLock(dir string) (*os.File, error) {
	path := CommitLock(dir)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open commit lock: %w", err)
	}
	deadline := time.Now().Add(commitLockTimeout)
	for {
		err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return f, nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) {
			_ = f.Close()
			return nil, fmt.Errorf("flock commit lock %s: %w", path, err)
		}
		if !time.Now().Before(deadline) {
			_ = f.Close()
			return nil, fmt.Errorf("%w: %s", ErrCommitLockHeld, path)
		}
		time.Sleep(commitLockPollInterval)
	}
}
