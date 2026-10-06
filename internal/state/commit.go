package state

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/portal/internal/fileutil"
)

// Commit atomically persists idx and garbage-collects unreferenced scrollback
// files, but only when something changed: the caller's anyScrollbackChanged
// flag, or a structural difference from the prior on-disk index with SavedAt
// ignored on both sides, so timestamp churn alone never triggers a write. A GC
// failure is logged, not returned — sessions.json is the source of truth.
//
// A written commit logs, at INFO, each session the prior on-disk index held
// that idx does not hold under its name or, renamed, under another. The
// on-disk index is the measure, not any caller's in-memory one, so a session
// removed by one committer is never reported again by another.
func Commit(dir string, idx Index, anyScrollbackChanged bool, logger *slog.Logger) error {
	return commitOver(dir, idx, readPriorIndex(dir), anyScrollbackChanged, logger)
}

// commitOver is Commit measured against prior, the on-disk index its caller
// read; nil when sessions.json could not be read.
func commitOver(dir string, idx Index, prior *Index, anyScrollbackChanged bool, logger *slog.Logger) error {
	logger = loggerOrDiscard(logger)
	idx.Canonicalize()

	data, err := EncodeIndex(idx)
	if err != nil {
		return fmt.Errorf("encode sessions.json: %w", err)
	}

	if prior != nil && !structuralChange(*prior, idx) && !anyScrollbackChanged {
		return nil
	}

	if err := fileutil.AtomicWrite0600(SessionsJSON(dir), data); err != nil {
		return fmt.Errorf("write sessions.json: %w", err)
	}

	if prior != nil {
		logDroppedSessions(*prior, idx, logger)
	}

	if err := gcOrphanScrollback(dir, idx, logger); err != nil {
		logger.Warn("gc orphan scrollback failed", "error", err)
	}

	return nil
}

// readPriorIndex returns nil when sessions.json cannot be read or decoded.
func readPriorIndex(dir string) *Index {
	priorBytes, err := os.ReadFile(SessionsJSON(dir))
	if err != nil {
		return nil
	}
	prior, err := DecodeIndex(priorBytes)
	if err != nil {
		return nil
	}
	prior.Canonicalize()
	return &prior
}

func structuralChange(prior, idx Index) bool {
	a := idx
	a.SavedAt = time.Time{}
	b := prior
	b.SavedAt = time.Time{}
	return !reflect.DeepEqual(a, b)
}

func logDroppedSessions(prior, idx Index, logger *slog.Logger) {
	priorNames := make(map[string]struct{}, len(prior.Sessions))
	for _, s := range prior.Sessions {
		priorNames[s.Name] = struct{}{}
	}
	keptNames := make(map[string]struct{}, len(idx.Sessions))
	var appeared []Session
	for _, s := range idx.Sessions {
		keptNames[s.Name] = struct{}{}
		if _, ok := priorNames[s.Name]; !ok {
			appeared = append(appeared, s)
		}
	}
	for _, s := range prior.Sessions {
		if _, ok := keptNames[s.Name]; ok {
			continue
		}
		if i := slices.IndexFunc(appeared, func(a Session) bool { return sameSessionUnderAnyName(s, a) }); i >= 0 {
			appeared = slices.Delete(appeared, i, i+1)
			continue
		}
		logger.Info("session dropped", "session", s.Name)
	}
}

// sameSessionUnderAnyName reports whether two records describe one session by
// their window layouts alone, which a rename leaves as they were. A window
// layout carries tmux's server-unique pane ids, so two live sessions compare
// equal only when they share their windows.
func sameSessionUnderAnyName(a, b Session) bool {
	return slices.EqualFunc(a.Windows, b.Windows, func(x, y Window) bool { return x.Layout == y.Layout })
}

// ComputeReferencedSet collects ScrollbackFile paths verbatim, as stored in idx.
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
		// The on-disk schema stores forward slashes on every platform.
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
		}
	}
	return nil
}
