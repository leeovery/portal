package state

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/leeovery/portal/internal/fileutil"
)

// Commit atomically persists idx to sessions.json and runs orphan-scrollback GC
// when anything has changed.
//
// The change-detection rule is:
//   - The structural delta is "idx (with SavedAt zeroed) != prior on-disk idx
//     (with SavedAt zeroed)". Absent or undecodable prior file is treated as
//     changed.
//   - The scrollback delta is the caller-supplied anyScrollbackChanged flag.
//
// When neither has changed, both the JSON write and GC are skipped — a fully
// no-op cycle produces zero disk activity (per the spec's content-hash dedup
// goal).
//
// On a real change, sessions.json is written via fileutil.AtomicWrite0600
// (atomic write + post-rename chmod 0600 against a permissive umask). After a
// successful write, gcOrphanScrollback removes any .bin files no longer
// referenced by idx. GC failure is logged but never fails the commit —
// sessions.json is the source of truth.
func Commit(dir string, idx Index, anyScrollbackChanged bool, logger *slog.Logger) error {
	logger = loggerOrDiscard(logger)
	idx.Canonicalize()

	data, err := EncodeIndex(idx)
	if err != nil {
		return fmt.Errorf("encode sessions.json: %w", err)
	}

	if !structuralChange(dir, idx) && !anyScrollbackChanged {
		return nil
	}

	if err := fileutil.AtomicWrite0600(SessionsJSON(dir), data); err != nil {
		return fmt.Errorf("write sessions.json: %w", err)
	}

	if err := gcOrphanScrollback(dir, idx, logger); err != nil {
		logger.Warn("gc orphan scrollback failed", "error", err)
		// GC failure is non-fatal — sessions.json was committed successfully.
	}

	return nil
}

// structuralChange reports whether idx differs structurally from the prior
// sessions.json on disk. SavedAt is zeroed on both sides so timestamp churn
// alone never counts as a change. A missing or undecodable prior file is
// treated as changed.
func structuralChange(dir string, idx Index) bool {
	priorBytes, err := os.ReadFile(SessionsJSON(dir))
	if err != nil {
		return true
	}
	prior, err := DecodeIndex(priorBytes)
	if err != nil {
		return true
	}
	prior.Canonicalize()

	a := idx
	a.SavedAt = time.Time{}
	b := prior
	b.SavedAt = time.Time{}
	return !reflect.DeepEqual(a, b)
}

// ComputeReferencedSet collects every pane's ScrollbackFile path into a set.
// The values are exactly the strings stored in idx — typically forward-slash
// "scrollback/<paneKey>.bin" relative paths produced by CaptureStructure.
func ComputeReferencedSet(idx Index) map[string]struct{} {
	set := make(map[string]struct{})
	for _, s := range idx.Sessions {
		for _, w := range s.Windows {
			for _, p := range w.Panes {
				set[p.ScrollbackFile] = struct{}{}
			}
		}
	}
	return set
}

// gcOrphanScrollback removes any .bin file under dir/scrollback/ that is not
// referenced by idx. A missing scrollback directory is a no-op. Per-file
// remove failures are logged at WARN and do not abort the sweep — the next
// successful commit will retry. ENOENT during remove (e.g. concurrent
// cleanup) is treated as success.
func gcOrphanScrollback(dir string, idx Index, logger *slog.Logger) error {
	sbDir := ScrollbackDir(dir)
	entries, err := os.ReadDir(sbDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}

	refSet := ComputeReferencedSet(idx)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".bin") {
			continue
		}
		// Match the on-disk schema: forward-slash relative path.
		relPath := filepath.ToSlash(filepath.Join("scrollback", name))
		if _, found := refSet[relPath]; found {
			continue
		}

		fullPath := filepath.Join(sbDir, name)
		if err := os.Remove(fullPath); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			paneKey := strings.TrimSuffix(name, ".bin")
			logger.Warn("gc remove scrollback failed", "pane_key", paneKey, "error", err)
			// Continue: subsequent files may still be removable.
		}
	}
	return nil
}
