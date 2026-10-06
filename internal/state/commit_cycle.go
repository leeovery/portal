package state

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

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
	// cycle commits only on a confirmation that server answered; zero stands
	// every cycle down.
	OwnServer int
	Dir       string
	// LoadPrev supplies the previous index, and is called only once the commit
	// lock is held.
	LoadPrev func() *Index
	HashMap  HashMap
	// Dump writes the caller's scrollback for the capture through the writer
	// it is handed and reports whether any file changed, skipping every pane
	// CaptureCycle.SkipsScrollback answers true for. Nil dumps nothing. An
	// error ends the cycle uncommitted.
	Dump   func(CaptureCycle, ScrollbackWriter) (bool, error)
	Logger *slog.Logger
}

// ScrollbackWriter writes captured scrollback for the commit cycle that handed
// it out, against that cycle's own server, state directory and dedup map.
type ScrollbackWriter struct {
	confirmer AnsweringConfirmer
	ownServer int
	dir       string
	hm        HashMap
	// held maps a pane key to the token-named transcript its record still
	// names.
	held map[string]string
	// captured holds every held pane key this cycle wrote a capture for.
	captured map[string]struct{}
}

// Write writes data, just captured for paneKey, unless hash matches the
// cycle's dedup entry, reporting whether it wrote. An empty capture over a
// saved transcript the cycle's own server does not confirm writes nothing and
// returns an error wrapping ErrUnconfirmedEmptyCapture. A pane's saved
// transcript is the file its record names.
func (w ScrollbackWriter) Write(paneKey string, data []byte, hash uint64) (bool, error) {
	saved := ScrollbackFile(w.dir, paneKey)
	stored, held := w.held[paneKey]
	if held {
		saved = joinStored(w.dir, stored)
		// The positional file the dedup entry describes may since have been
		// removed by housekeeping, and a dedup hit would then keep the pane
		// off its new capture for good.
		delete(w.hm, paneKey)
	}
	if err := confirmEmptyCapture(w.confirmer, w.ownServer, saved, data); err != nil {
		return false, err
	}
	written, err := WriteScrollbackIfChanged(w.dir, paneKey, data, hash, w.hm)
	if written && held {
		w.captured[paneKey] = struct{}{}
	}
	return written, err
}

// RunCommitCycle runs cycle under the exclusive commit lock, held from the
// skeleton-marker read to the end of the housekeeping pass, so no two
// committers' captures and collections interleave. A lock not granted within
// the bound returns an error wrapping ErrCommitLockHeld with nothing read or
// written. A failed capture returns before the dump, and a failed dump before
// the commit, its error returned as the dump gave it.
//
// A pane answered since it was filed under its token keeps that token-named
// transcript on its record until a dump writes its new capture, so no commit
// leaves the pane's record off the file holding its bytes.
func RunCommitCycle(cycle CommitCycle) (CaptureCycle, error) {
	lock, err := acquireCommitLock(cycle.Dir)
	if err != nil {
		return CaptureCycle{}, err
	}
	defer func() { _ = lock.Close() }()

	capture, err := captureAndRefile(cycle.Client, cycle.OwnServer, cycle.Dir, cycle.LoadPrev(), cycle.HashMap, cycle.Logger)
	if err != nil {
		return capture, fmt.Errorf("capture: %w", err)
	}
	changed := false
	if cycle.Dump != nil {
		writer := ScrollbackWriter{
			confirmer: cycle.Client,
			ownServer: cycle.OwnServer,
			dir:       cycle.Dir,
			hm:        cycle.HashMap,
			held:      heldTranscripts(capture),
			captured:  map[string]struct{}{},
		}
		if changed, err = cycle.Dump(capture, writer); err != nil {
			return capture, err
		}
		fileAtPositional(&capture.Index, writer.captured)
	}
	if err := Commit(cycle.Dir, capture.Index, changed, cycle.Logger); err != nil {
		return capture, fmt.Errorf("commit: %w", err)
	}
	return capture, nil
}

// heldTranscripts maps the key of every pane the dump may write whose record
// names its token-named transcript to that stored path.
func heldTranscripts(capture CaptureCycle) map[string]string {
	held := map[string]string{}
	for _, s := range capture.Index.Sessions {
		for _, w := range s.Windows {
			for _, p := range w.Panes {
				key := SanitizePaneKey(s.Name, w.Index, p.Index)
				if p.PortalPaneID == "" || p.ScrollbackFile != PendingScrollbackFile(p.PortalPaneID) || capture.SkipsScrollback(key) {
					continue
				}
				held[key] = p.ScrollbackFile
			}
		}
	}
	return held
}

func fileAtPositional(idx *Index, keys map[string]struct{}) {
	if len(keys) == 0 {
		return
	}
	for si := range idx.Sessions {
		s := &idx.Sessions[si]
		for wi := range s.Windows {
			w := &s.Windows[wi]
			for pi := range w.Panes {
				key := SanitizePaneKey(s.Name, w.Index, w.Panes[pi].Index)
				if _, ok := keys[key]; ok {
					w.Panes[pi].ScrollbackFile = positionalScrollbackFile(key)
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
