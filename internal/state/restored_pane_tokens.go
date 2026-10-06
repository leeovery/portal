package state

import (
	"fmt"
	"os"

	"github.com/leeovery/portal/internal/fileutil"
)

// RestoredPaneToken ties the token restore stamped on a pane it recreated to
// the saved record that pane was built from, named by its saved address.
type RestoredPaneToken struct {
	Session string
	Window  int
	Pane    int
	Token   string
}

// RecordRestoredPaneTokens writes each token onto the sessions.json record at
// its saved address, so the first commit after restore finds a restored pane's
// record by the token its live pane carries rather than by an address restore
// may have placed it away from. A record already carrying a token keeps it.
//
// It holds the commit lock, returning an error wrapping ErrCommitLockHeld when
// the lock is not granted within the bound, and an error when sessions.json
// cannot be read or decoded. It rewrites sessions.json only when a record takes
// a token, and runs no housekeeping pass.
func RecordRestoredPaneTokens(dir string, ties []RestoredPaneToken) error {
	if len(ties) == 0 {
		return nil
	}
	lock, err := acquireCommitLock(dir)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close() }()

	data, err := os.ReadFile(SessionsJSON(dir))
	if err != nil {
		return fmt.Errorf("read sessions.json: %w", err)
	}
	idx, err := DecodeIndex(data)
	if err != nil {
		return fmt.Errorf("parse sessions.json: %w", err)
	}
	if !applyRestoredPaneTokens(&idx, ties) {
		return nil
	}
	encoded, err := EncodeIndex(idx)
	if err != nil {
		return fmt.Errorf("encode sessions.json: %w", err)
	}
	if err := fileutil.AtomicWrite0600(SessionsJSON(dir), encoded); err != nil {
		return fmt.Errorf("write sessions.json: %w", err)
	}
	return nil
}

func applyRestoredPaneTokens(idx *Index, ties []RestoredPaneToken) bool {
	changed := false
	for _, tie := range ties {
		p := paneAtAddress(idx, tie.Session, tie.Window, tie.Pane)
		if p == nil || p.PortalPaneID != "" {
			continue
		}
		p.PortalPaneID = tie.Token
		changed = true
	}
	return changed
}

func paneAtAddress(idx *Index, session string, window, pane int) *Pane {
	for si := range idx.Sessions {
		s := &idx.Sessions[si]
		if s.Name != session {
			continue
		}
		for wi := range s.Windows {
			w := &s.Windows[wi]
			if w.Index != window {
				continue
			}
			for pi := range w.Panes {
				if w.Panes[pi].Index == pane {
					return &w.Panes[pi]
				}
			}
		}
	}
	return nil
}
