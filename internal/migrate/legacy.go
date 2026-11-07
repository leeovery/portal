// Package migrate converts the tmux version's saved state into the zmx
// version's, for portal migrate. ReadLegacy reads the tmux version's
// sessions.json, version 1, and looks for the scrollback files it names;
// NewPlan works out, writing nothing, the zmx session each tmux pane becomes
// and what becomes of each hooks.json key; Build writes the plan's sessions
// into a fresh state dir, and RewriteHooks writes hooks.json from its backup
// through the plan. It is the one reader of version 1.
package migrate

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// legacyVersion is the only sessions.json version the tmux version wrote.
const legacyVersion = 1

const legacyIndexFile = "sessions.json"

var (
	// ErrOutsideState is why a scrollback file version 1 names outside its
	// state dir, which only a hand edit makes, isn't copied: migrate reads
	// nothing there.
	ErrOutsideState = errors.New("names a file outside the tmux version's state dir")
	// ErrNotRegular is why a scrollback file that isn't a regular file isn't
	// copied.
	ErrNotRegular = errors.New("is not a regular file")
	// ErrNoWindows and ErrNoPanes are why a tmux session version 1 saved
	// becomes no zmx session.
	ErrNoWindows = errors.New("version 1 saved no windows for it")
	ErrNoPanes   = errors.New("version 1 saved no panes for it")
)

// legacyIndex is as much of a version 1 sessions.json as migrate reads: each
// tmux session's windows and their panes. Members it doesn't model are
// ignored.
type legacyIndex struct {
	Version  int             `json:"version"`
	SavedAt  time.Time       `json:"saved_at"`
	Sessions []legacySession `json:"sessions"`
}

type legacySession struct {
	Name    string         `json:"name"`
	Windows []legacyWindow `json:"windows"`
}

type legacyWindow struct {
	Index int          `json:"index"`
	Panes []legacyPane `json:"panes"`
}

// legacyPane's ScrollbackFile is relative to the tmux version's state dir,
// forward-slashed. PortalPaneID is the pane's token, which keyed its hook in
// hooks.json; it is empty for a pane never stamped, the ordinary case.
type legacyPane struct {
	Index          int    `json:"index"`
	CWD            string `json:"cwd"`
	ScrollbackFile string `json:"scrollback_file"`
	PortalPaneID   string `json:"portal_pane_id"`
}

// decodeLegacyIndex refuses malformed JSON, a missing version, a version other
// than 1, and a missing saved_at, which names the creation of every
// scrollback file migrate copies.
func decodeLegacyIndex(data []byte) (legacyIndex, error) {
	var idx legacyIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return legacyIndex{}, fmt.Errorf("decode the tmux version's sessions.json: %w", err)
	}
	switch {
	case idx.Version == 0:
		return legacyIndex{}, errors.New("the tmux version's sessions.json has no version")
	case idx.Version != legacyVersion:
		return legacyIndex{}, fmt.Errorf("the tmux version's sessions.json is version %d, not %d", idx.Version, legacyVersion)
	case idx.SavedAt.IsZero():
		return legacyIndex{}, errors.New("the tmux version's sessions.json has no saved_at")
	}
	return idx, nil
}

// savedPane is a pane version 1 saved, at its address.
type savedPane struct {
	at   TmuxPane
	pane legacyPane
}

// panes are the session's saved panes in the order migrate names them: its
// windows by index, then each window's panes by index.
func (s legacySession) panes() []savedPane {
	windows := slices.SortedStableFunc(slices.Values(s.Windows), func(a, b legacyWindow) int { return cmp.Compare(a.Index, b.Index) })
	var saved []savedPane
	for _, w := range windows {
		for _, p := range slices.SortedStableFunc(slices.Values(w.Panes), func(a, b legacyPane) int { return cmp.Compare(a.Index, b.Index) }) {
			saved = append(saved, savedPane{at: TmuxPane{Session: s.Name, Window: w.Index, Pane: p.Index}, pane: p})
		}
	}
	return saved
}

// empty says why a session with no saved panes has none.
func (s legacySession) empty() error {
	if len(s.Windows) == 0 {
		return ErrNoWindows
	}
	return ErrNoPanes
}

// Legacy is the tmux version's saved state, as ReadLegacy found it.
type Legacy struct {
	dir   string
	index legacyIndex
	// transcripts is what ReadLegacy found of each scrollback file a pane
	// names, by the name the pane gives it.
	transcripts map[string]Transcript
}

// ReadLegacy reads the version 1 sessions.json in dir, the tmux version's
// state dir, and looks there for each scrollback file it names, every read
// through a root on dir, so none leaves it, whatever links lie below. A dir or
// sessions.json that is absent (an error matching fs.ErrNotExist), unreadable,
// corrupt or of another version is an error. A scrollback file that can't be
// found, or is outside dir or no regular file, is not: its pane is migrated
// with no scrollback, and the plan says why.
func ReadLegacy(dir string) (Legacy, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return Legacy{}, fmt.Errorf("open the tmux version's state dir: %w", err)
	}
	defer func() { _ = root.Close() }()
	data, err := root.ReadFile(legacyIndexFile)
	if err != nil {
		return Legacy{}, fmt.Errorf("read the tmux version's sessions.json: %w", err)
	}
	idx, err := decodeLegacyIndex(data)
	if err != nil {
		return Legacy{}, err
	}
	transcripts := map[string]Transcript{}
	for _, s := range idx.Sessions {
		for _, p := range s.panes() {
			if named := p.pane.ScrollbackFile; named != "" {
				transcripts[named] = lookFor(root, named)
			}
		}
	}
	return Legacy{dir: dir, index: idx, transcripts: transcripts}, nil
}

// lookFor finds the scrollback file named, relative to root, saying why it
// can't be copied when it can't.
func lookFor(root *os.Root, named string) Transcript {
	found := Transcript{Named: named}
	rel := filepath.FromSlash(named)
	if !filepath.IsLocal(rel) {
		found.Missing = fmt.Errorf("scrollback file %q %w", named, ErrOutsideState)
		return found
	}
	info, err := root.Lstat(rel)
	switch {
	case err != nil:
		found.Missing = fmt.Errorf("look for scrollback file %q: %w", named, err)
	case !info.Mode().IsRegular():
		found.Missing = fmt.Errorf("scrollback file %q %w", named, ErrNotRegular)
	default:
		found.source = rel
	}
	return found
}
