package state

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/cespare/xxhash/v2"
	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/nanoid"
)

// HashMap holds the xxhash of the bytes most recently committed for each
// paneKey. A missing entry means nothing was persisted for that pane, which a
// zero hash does not — empty bytes hash non-zero.
type HashMap map[string]uint64

// PaneCapturer is declared here so internal/state need not import internal/tmux
// — which it must not, since that package imports this one. The target type is a
// parameter rather than a plain string for the same reason: the tmux client
// takes its own named target type, and hardcoding string here would put this
// seam out of its reach.
type PaneCapturer[T ~string] interface {
	CapturePane(target T) (string, error)
}

// SeedHashMap rebuilds the dedup map from the on-disk scrollback files, so the
// first cycle after a daemon restart does not rewrite every pane. It always
// returns a usable map: a missing directory is silent first-run state, and an
// unreadable directory or file is logged and skipped.
func SeedHashMap(dir string, logger *slog.Logger) HashMap {
	logger = loggerOrDiscard(logger)
	hm := HashMap{}
	sbDir := ScrollbackDir(dir)
	entries, err := os.ReadDir(sbDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return hm
		}
		logger.Warn("seed read scrollback dir failed", "path", sbDir, "error", err)
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
			logger.Warn("seed read scrollback file failed", "path", path, "error", err)
			continue
		}
		hm[paneKey] = xxhash.Sum64(data)
	}
	return hm
}

func CaptureAndHashPane[T ~string](c PaneCapturer[T], target T) ([]byte, uint64, error) {
	out, err := c.CapturePane(target)
	if err != nil {
		return nil, 0, err
	}
	return []byte(out), xxhash.Sum64String(out), nil
}

// WriteScrollbackIfChanged writes only when newHash differs from hm's entry,
// updating hm on a write. A dedup hit touches no disk and returns (false, nil).
func WriteScrollbackIfChanged(dir, paneKey string, data []byte, newHash uint64, hm HashMap) (bool, error) {
	if existing, ok := hm[paneKey]; ok && existing == newHash {
		return false, nil
	}
	path := ScrollbackFile(dir, paneKey)
	if err := fileutil.AtomicWrite0600(path, data); err != nil {
		return false, fmt.Errorf("write scrollback %s: %w", paneKey, err)
	}
	hm[paneKey] = newHash
	return true, nil
}

// RefilePendingScrollback moves each waiting pane's scrollback out of the
// positional namespace and onto its durable token, rewriting the record in idx
// and dropping the dedup entry for the name the bytes left — which is what lets
// the next pane to occupy that address write its own file there. Panes whose
// key is absent from pending, whose token the pane-token mint could not have
// produced, or which are already filed under their token are left untouched. A
// rename that fails for any reason other than a missing source leaves that
// pane's record and dedup entry alone and emits one WARN; the caller commits
// regardless and the next call retries.
func RefilePendingScrollback(dir string, idx *Index, pending map[string]struct{}, hm HashMap, logger *slog.Logger) {
	if idx == nil || len(pending) == 0 {
		return
	}
	logger = loggerOrDiscard(logger)
	for si := range idx.Sessions {
		s := &idx.Sessions[si]
		for wi := range s.Windows {
			w := &s.Windows[wi]
			for pi := range w.Panes {
				p := &w.Panes[pi]
				key := SanitizePaneKey(s.Name, w.Index, p.Index)
				if _, waiting := pending[key]; !waiting {
					continue
				}
				refilePendingPane(dir, key, p, hm, logger)
			}
		}
	}
}

func refilePendingPane(dir, paneKey string, p *Pane, hm HashMap, logger *slog.Logger) {
	if !nanoid.IsTokenShaped(p.PortalPaneID) {
		return
	}
	tokenPath := PendingScrollbackFile(p.PortalPaneID)
	stored := p.ScrollbackFile
	if stored == tokenPath {
		return
	}
	if err := renameStoredScrollback(dir, stored, tokenPath); err != nil {
		logger.Warn("refile pending scrollback failed", "pane_key", paneKey, "path", stored, "error", err)
		return
	}
	p.ScrollbackFile = tokenPath
	delete(hm, dedupKeyOf(stored))
}

// A record naming no file is treated as a missing source: the bytes are already
// wherever they are, and joining an empty path onto dir would name the state
// directory itself.
func renameStoredScrollback(dir, stored, tokenPath string) error {
	if stored == "" {
		return nil
	}
	err := os.Rename(joinStored(dir, stored), joinStored(dir, tokenPath))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func joinStored(dir, stored string) string {
	return filepath.Join(dir, filepath.FromSlash(stored))
}

// dedupKeyOf derives a stored path's HashMap key the way SeedHashMap derives
// one from a file name.
func dedupKeyOf(stored string) string {
	return strings.TrimSuffix(filepath.Base(filepath.FromSlash(stored)), ".bin")
}
