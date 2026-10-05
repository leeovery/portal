package state

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/cespare/xxhash/v2"
	"github.com/leeovery/portal/internal/fileutil"
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

// refilePendingScrollback moves each waiting pane's scrollback out of the
// positional namespace and onto its durable token, rewriting the record in idx
// and dropping the dedup entry for the name the bytes left — which is what lets
// the next pane to occupy that address write its own file there. Panes whose
// key is absent from pending, whose token the pane-token mint could not have
// produced, or which are already filed under their token are left untouched. A
// pane whose token-named file already exists adopts it, leaving the positional
// file where it is. A rename that fails for any reason other than a missing
// source or an existing token-named file leaves that pane's record and dedup
// entry alone and emits one WARN; the caller commits regardless and the next
// call retries.
func refilePendingScrollback(dir string, idx *Index, pending map[string]struct{}, hm HashMap, logger *slog.Logger) {
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
	if _, ok := PendingScrollbackPath(dir, p.PortalPaneID); !ok {
		return
	}
	tokenPath := PendingScrollbackFile(p.PortalPaneID)
	stored := p.ScrollbackFile
	if stored == tokenPath {
		return
	}
	if err := placeStoredScrollback(dir, stored, tokenPath, moveNoClobber); err != nil {
		logger.Warn("refile pending scrollback failed", "pane_key", paneKey, "path", stored, "error", err)
		return
	}
	p.ScrollbackFile = tokenPath
	delete(hm, dedupKeyOf(stored))
}

// A record naming no file is treated as a missing source: the bytes are already
// wherever they are, and joining an empty path onto dir would name the state
// directory itself. An existing token-named file is adopted rather than
// replaced: the positional file may by now hold another pane's capture, and
// place must refuse an existing name atomically so a concurrent re-file cannot
// slip between check and placement.
func placeStoredScrollback(dir, stored, tokenPath string, place func(src, dst string) error) error {
	if stored == "" {
		return nil
	}
	err := place(joinStored(dir, stored), joinStored(dir, tokenPath))
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrExist) {
		return nil
	}
	return err
}

// errNoReplaceUnsupported is what a platform's no-replace rename reports when
// the filesystem rejects the flag itself.
var errNoReplaceUnsupported = errors.New("no-replace rename unsupported by filesystem")

var renameNoReplace = platformRenameNoReplace

// Where the filesystem rejects the no-replace flag, link(2) stands in: it too
// refuses an existing name atomically. Once linked the bytes are safe under
// dst, so a failed remove of src is ignored.
func moveNoClobber(src, dst string) error {
	err := renameNoReplace(src, dst)
	if !errors.Is(err, errNoReplaceUnsupported) {
		return err
	}
	if err := os.Link(src, dst); err != nil {
		return err
	}
	_ = os.Remove(src)
	return nil
}

func joinStored(dir, stored string) string {
	return filepath.Join(dir, filepath.FromSlash(stored))
}

// dedupKeyOf derives a stored path's HashMap key the way SeedHashMap derives
// one from a file name.
func dedupKeyOf(stored string) string {
	return strings.TrimSuffix(filepath.Base(filepath.FromSlash(stored)), ".bin")
}

// linkMovedSkeletonScrollback gives each mid-restore pane whose token the
// pane-token rule accepts, and whose record names another address's positional
// file that another record in idx also names, a second name under that token,
// and points the record there, so the two records no longer share a file. A
// record no other record shares is left as it is: moving it off its name would
// let the housekeeping pass delete that file before the hydrate helper has read
// it. It links rather than moves for the same reason: the helper may not yet
// have opened the positional path it was launched with, and the other record's
// writer replaces that name atomically, leaving the linked inode intact. A link
// that fails for any reason other than a missing source or an existing
// token-named file leaves that pane's record alone and emits one WARN.
func linkMovedSkeletonScrollback(dir string, idx *Index, skeleton map[string]struct{}, logger *slog.Logger) {
	if len(skeleton) == 0 {
		return
	}
	logger = loggerOrDiscard(logger)
	naming := recordsPerScrollbackFile(*idx)
	for si := range idx.Sessions {
		s := &idx.Sessions[si]
		for wi := range s.Windows {
			w := &s.Windows[wi]
			for pi := range w.Panes {
				p := &w.Panes[pi]
				key := SanitizePaneKey(s.Name, w.Index, p.Index)
				if _, armed := skeleton[key]; !armed {
					continue
				}
				if naming[p.ScrollbackFile] < 2 {
					continue
				}
				linkMovedPane(dir, key, p, logger)
			}
		}
	}
}

func recordsPerScrollbackFile(idx Index) map[string]int {
	counts := map[string]int{}
	for _, s := range idx.Sessions {
		for _, w := range s.Windows {
			for _, p := range w.Panes {
				counts[p.ScrollbackFile]++
			}
		}
	}
	return counts
}

func linkMovedPane(dir, paneKey string, p *Pane, logger *slog.Logger) {
	if _, ok := PendingScrollbackPath(dir, p.PortalPaneID); !ok {
		return
	}
	tokenPath := PendingScrollbackFile(p.PortalPaneID)
	stored := p.ScrollbackFile
	if stored == tokenPath || stored == positionalScrollbackFile(paneKey) {
		return
	}
	if err := placeStoredScrollback(dir, stored, tokenPath, os.Link); err != nil {
		logger.Warn("link moved skeleton scrollback failed", "pane_key", paneKey, "path", stored, "error", err)
		return
	}
	p.ScrollbackFile = tokenPath
}

// CaptureCycleClient is what one capture cycle reads through: the skeleton
// markers as well as the structure, and the confirmation sent after them.
type CaptureCycleClient interface {
	CaptureClient
	ServerOptionLister
	AnsweringConfirmer
}

// AnsweringConfirmer confirms tmux is still answering. ConfirmAnswering must
// return a nil error only for a read tmux answered with exit status 0, with the
// pid of the server that answered it, or 0 for an answer naming none: an exiting
// tmux can answer a read already in flight with exit status 0 and no output, and
// only a refused read sent after it tells that answer apart from an empty
// server.
type AnsweringConfirmer interface {
	ConfirmAnswering() (int, error)
}

// ErrNotOwnServer is a confirmation that does not prove the committer's own
// tmux server answered it: one answered by another server or naming none. A
// committer that does not know its own server sends none and is refused with it.
var ErrNotOwnServer = errors.New("confirmation not answered by the committer's own tmux server")

// confirmOwnServer refuses the cycle unless ownServer answers the
// confirmation. A tmux server that has begun exiting refuses every new
// connection and never stops exiting, and only a server that has exited lets
// another start on its socket, so an answer from ownServer proves every read
// before it reached ownServer too.
func confirmOwnServer(c AnsweringConfirmer, ownServer int) error {
	if ownServer <= 0 {
		return fmt.Errorf("%w: own server unknown", ErrNotOwnServer)
	}
	answered, err := c.ConfirmAnswering()
	if err != nil {
		return err
	}
	if answered != ownServer {
		return fmt.Errorf("%w: answered by server pid %d, own server pid %d", ErrNotOwnServer, answered, ownServer)
	}
	return nil
}

// CaptureCycle is what one capture cycle hands its caller: the index to commit
// and the sets of pane keys the caller's own scrollback dump must skip.
type CaptureCycle struct {
	Index Index
	// Pending holds every live pane carrying the resume pending marker in a
	// session the capture reached.
	Pending map[string]struct{}
	// Skeleton holds every pane key a skeleton marker named when the capture was
	// taken.
	Skeleton map[string]struct{}
	// Carried holds every pane key of a session the index carries forward from
	// the previous index rather than from the capture: none of its panes was
	// captured, so none may be dumped.
	Carried map[string]struct{}
}

// SkipsScrollback reports whether a scrollback dump over this capture must skip
// paneKey: a key in Skeleton, Pending or Carried. Capturing a pane held behind a
// waiting resume panel writes back its transcript minus the screenful that
// panel covers, and no copy of those lines survives anywhere else. A carried
// session's address may now answer to another pane, or to that waiting pane.
func (c CaptureCycle) SkipsScrollback(paneKey string) bool {
	for _, set := range []map[string]struct{}{c.Skeleton, c.Pending, c.Carried} {
		if _, skipped := set[paneKey]; skipped {
			return true
		}
	}
	return false
}

// captureAndRefile reads the skeleton markers, takes a capture merged against
// them, and re-files every frozen pane's scrollback in one step, so no caller
// can commit an index that omits a mid-restore pane's record, has two records
// naming one scrollback file, or still names a waiting pane's vacated
// positional path, bar a pane whose token the pane-token rule refuses or whose
// link or re-file failed. A failed marker read returns its wrapped error before
// any capture is taken. A failed capture returns before anything is re-filed,
// with the empty index, the empty pending set and the error the capture gave. A
// refused confirmation, sent after the last capture read, returns its error and
// an empty cycle before anything is linked or re-filed; so does one not
// answered by ownServer, the pid of the committer's own tmux server.
func captureAndRefile(c CaptureCycleClient, ownServer int, dir string, prev *Index, hm HashMap, logger *slog.Logger) (CaptureCycle, error) {
	skeleton, err := ListSkeletonMarkers(c)
	if err != nil {
		return CaptureCycle{}, fmt.Errorf("list skeleton markers: %w", err)
	}
	captured, err := captureStructure(c, skeleton, prev, logger)
	capture := CaptureCycle{Index: captured.index, Pending: captured.pending, Skeleton: skeleton, Carried: captured.carried}
	if err != nil {
		return capture, err
	}
	if err := confirmOwnServer(c, ownServer); err != nil {
		return CaptureCycle{}, err
	}
	linkMovedSkeletonScrollback(dir, &capture.Index, skeleton, logger)
	waiting := make(map[string]struct{}, len(capture.Pending)+len(captured.carriedWaiting))
	maps.Copy(waiting, capture.Pending)
	maps.Copy(waiting, captured.carriedWaiting)
	refilePendingScrollback(dir, &capture.Index, waiting, hm, logger)
	return capture, nil
}
