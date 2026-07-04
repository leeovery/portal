package state

import "testing"

// TestFindOrAppendSessionCopiesPortalID pins the append branch of
// findOrAppendSession — the branch that reconstructs a Session by hand when a
// prev session is (re)introduced into the fresh index. That literal must copy
// the whole struct the doc-comment promises ("a shallow copy of ps"), including
// the PortalID field: losing the id here would erase it from the snapshot and
// re-orphan the resume hook on the next reboot (spec § Cross-Reboot Persistence
// → Capture; Acceptance Criteria 4).
//
// This is a white-box test because the append branch is unreachable via the
// public CaptureStructure merge path today (mergeSkippedPanes gates every prev
// pane on the session being present in the freshly-captured live structure, so
// findOrAppendSession's "found" loop returns the existing index before the
// append ever runs). Calling findOrAppendSession directly exercises the append
// literal without relaxing the sessionLive gate or otherwise changing
// production reachability.
func TestFindOrAppendSessionCopiesPortalID(t *testing.T) {
	fresh := &Index{Sessions: []Session{}}
	ps := Session{
		Name:        "portal-aB3xY9kZ",
		PortalID:    "aB3xY9kZ",
		Environment: map[string]string{"FOO": "bar"},
		Windows: []Window{
			{Index: 0, Panes: []Pane{{Index: 0}}},
		},
	}

	si := findOrAppendSession(fresh, ps)

	if si != 0 {
		t.Fatalf("findOrAppendSession index = %d; want 0 (appended into empty index)", si)
	}
	got := fresh.Sessions[si]
	if got.PortalID != ps.PortalID {
		t.Errorf("appended Session.PortalID = %q; want %q", got.PortalID, ps.PortalID)
	}
	if got.Name != ps.Name {
		t.Errorf("appended Session.Name = %q; want %q", got.Name, ps.Name)
	}
	if got.Environment["FOO"] != "bar" {
		t.Errorf("appended Session.Environment = %v; want FOO=bar", got.Environment)
	}
	// Windows must stay an empty (non-nil) slice — the caller populates windows
	// via subsequent findOrAppendWindow/mergePane calls, so ps.Windows is
	// intentionally NOT copied here.
	if len(got.Windows) != 0 {
		t.Errorf("appended Session.Windows = %v; want empty (windows populated by caller)", got.Windows)
	}
	if got.Windows == nil {
		t.Errorf("appended Session.Windows is nil; want empty non-nil []Window{}")
	}
}
