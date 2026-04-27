package state

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/cespare/xxhash/v2"
	"github.com/leeovery/portal/internal/fileutil"
)

// HashMap holds the daemon's content-hash dedup state keyed by canonical
// paneKey. Each value is the xxhash of the bytes most-recently committed to
// disk for that pane. A missing entry means "no scrollback persisted for this
// pane yet" — distinct from a zero hash (the hash of empty bytes is non-zero).
type HashMap map[string]uint64

// PaneCapturer is the narrow seam CaptureAndHashPane needs. Defining a
// scrollback-local interface keeps state's tests free of *tmux.Client
// construction and avoids importing internal/tmux. *tmux.Client satisfies it
// implicitly via its CapturePane method.
//
// A different name from internal/state.CaptureClient (used by CaptureStructure)
// is deliberate: that interface composes three structural methods, while this
// one is single-method and shouldn't pretend to be a subset.
type PaneCapturer interface {
	CapturePane(target string) (string, error)
}

// SeedHashMap rebuilds the dedup map from the on-disk scrollback directory at
// daemon startup. Reading and hashing every existing `.bin` file means the
// first capture cycle after a daemon restart skips every pane whose live
// scrollback still matches what is on disk — avoiding a full rewrite of every
// scrollback file each time the daemon restarts (which happens on every
// `portal open` for `dev`/empty-version builds).
//
// Resilient by design: a missing scrollback directory yields an empty map
// with no warning (legitimate first-run state); an unreadable directory or
// individual file is logged at WARN and skipped so seeding always returns a
// usable map.
func SeedHashMap(dir string, logger *Logger) HashMap {
	hm := HashMap{}
	sbDir := ScrollbackDir(dir)
	entries, err := os.ReadDir(sbDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return hm
		}
		logger.Warn("daemon", "seed: read scrollback dir %s: %v", sbDir, err)
		return hm
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".bin") {
			continue
		}
		paneKey := strings.TrimSuffix(name, ".bin")
		path := filepath.Join(sbDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			logger.Warn("daemon", "seed: read %s: %v", name, err)
			continue
		}
		hm[paneKey] = xxhash.Sum64(data)
	}
	return hm
}

// CaptureAndHashPane is a small composition over the supplied PaneCapturer
// that returns both the captured bytes and their xxhash. Callers feed the
// returned (bytes, hash) into WriteScrollbackIfChanged to commit only when
// content actually changed.
func CaptureAndHashPane(c PaneCapturer, target string) ([]byte, uint64, error) {
	out, err := c.CapturePane(target)
	if err != nil {
		return nil, 0, err
	}
	bytes := []byte(out)
	return bytes, xxhash.Sum64(bytes), nil
}

// WriteScrollbackIfChanged is the dedup-aware writer for per-pane scrollback.
// It commits data to `scrollback/<paneKey>.bin` via fileutil.AtomicWrite only
// when the supplied newHash differs from the entry already stored in hm; on
// hit (identical hash, paneKey present) it returns (false, nil) without
// touching disk.
//
// The returned bool is "did we write?" — letting callers track whether the
// surrounding save cycle has anything to commit at the index level. On a
// successful write, hm[paneKey] is updated to newHash so subsequent calls in
// the same cycle (and across ticks) keep the dedup map honest.
//
// The scrollback file is chmodded to 0o600 after the rename so its mode does
// not depend on the user's umask. AtomicWrite errors are wrapped with the
// paneKey for traceable failure logs.
func WriteScrollbackIfChanged(dir, paneKey string, data []byte, newHash uint64, hm HashMap) (bool, error) {
	if existing, ok := hm[paneKey]; ok && existing == newHash {
		return false, nil
	}
	path := ScrollbackFile(dir, paneKey)
	if err := fileutil.AtomicWrite(path, data); err != nil {
		return false, fmt.Errorf("write scrollback %s: %w", paneKey, err)
	}
	_ = os.Chmod(path, 0o600)
	hm[paneKey] = newHash
	return true, nil
}
