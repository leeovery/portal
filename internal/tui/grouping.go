package tui

import (
	"cmp"
	"slices"

	"github.com/charmbracelet/bubbles/list"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/tmux"
)

// unknownHeading is the dimmed label for the pinned catch-all bucket in By
// Project mode — the group collecting sessions whose directory cannot be
// resolved to a known project. See spec § Empty States → Unknown bucket.
const unknownHeading = "Unknown"

// buildByProject assembles the live sessions into By-Project grouped order: a
// pre-sorted []list.Item of SessionItems where every session appears exactly
// once (Pattern A) under its project name heading, ready for the delegate to
// inject a heading at each GroupKey boundary.
//
// Each session resolves to a canonical directory key from Session.Dir (already
// canonicalised upstream by Phase 1's lazy fallback/restamp, but re-run through
// project.CanonicalDirKey here so the lookup key matches the stored
// Project.Path form). A hit on project.MatchProjectByDir yields a known-project
// item keyed on the canonical path with the matched project name as heading; a
// miss — empty Dir, or a stamped path with no matching Project record (e.g. a
// deleted project) — routes the session to the pinned Unknown bucket, which is
// always rendered last. No session is ever dropped.
//
// Known-project items are sorted by (GroupKey, Session.Name): the key is the
// canonical path, not the heading name, so two distinct directories that share
// a project name form two distinct groups. Unknown items are appended after the
// sorted known-project items.
//
// Pure function — no tmux call, no I/O. Zero live sessions yields an empty
// slice.
func buildByProject(sessions []tmux.Session, projects []project.Project) []list.Item {
	var known []SessionItem
	var unknown []list.Item

	for _, s := range sessions {
		if s.Dir == "" {
			unknown = append(unknown, unknownItem(s))
			continue
		}

		matched, ok := project.MatchProjectByDir(projects, s.Dir)
		if !ok {
			unknown = append(unknown, unknownItem(s))
			continue
		}

		known = append(known, SessionItem{
			Session:      s,
			GroupKey:     project.CanonicalDirKey(s.Dir),
			GroupHeading: matched.Name,
		})
	}

	slices.SortFunc(known, func(a, b SessionItem) int {
		if c := cmp.Compare(a.GroupKey, b.GroupKey); c != 0 {
			return c
		}
		return cmp.Compare(a.Session.Name, b.Session.Name)
	})

	items := make([]list.Item, 0, len(known)+len(unknown))
	for _, ki := range known {
		items = append(items, ki)
	}
	items = append(items, unknown...)
	return items
}

// unknownItem builds the catch-all SessionItem for a session that cannot be
// resolved to a known project. It is kept cohesive (CatchAll flag + Unknown
// heading set in one place) so it can later be extracted/extended into the
// shared pinned + empty-suppression catch-all helper.
func unknownItem(s tmux.Session) SessionItem {
	return SessionItem{
		Session:      s,
		GroupHeading: unknownHeading,
		CatchAll:     true,
	}
}
