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

// untaggedHeading is the dimmed label for the pinned catch-all bucket in By Tag
// mode — the group collecting sessions whose directory has no usable tags. See
// spec § Empty States → Untagged bucket.
const untaggedHeading = "Untagged"

// buildByProject assembles the live sessions into By-Project grouped order: a
// pre-sorted []list.Item of SessionItems where every session appears exactly
// once (Pattern A) under its project name heading, ready for the delegate to
// inject a heading at each GroupKey boundary.
//
// Each session resolves to a canonical directory key from Session.Dir (already
// resolved by the render-layer resolution pass in rebuildSessionList — the lazy
// stamp-on-render fallback). idx.Match (the pre-canonicalised project.Index,
// built once per project-load) canonicalises Session.Dir once and returns that
// canonical key alongside the match result; a hit yields a known-project item
// keyed on that returned canonical path (reused as GroupKey — no second
// CanonicalDirKey/EvalSymlinks call) with the matched project name as heading. A
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
func buildByProject(sessions []tmux.Session, idx project.Index) []list.Item {
	var known []SessionItem
	var unknown []list.Item

	for _, s := range sessions {
		if s.Dir == "" {
			unknown = append(unknown, unknownItem(s))
			continue
		}

		matched, key, ok := idx.Match(s.Dir)
		if !ok {
			unknown = append(unknown, unknownItem(s))
			continue
		}

		known = append(known, SessionItem{
			Session:      s,
			GroupKey:     key,
			GroupHeading: matched.Name,
		})
	}

	return assembleGroups(known, unknown, unknownHeading)
}

// buildByTag assembles the live sessions into By-Tag grouped order: a pre-sorted
// []list.Item of SessionItems materialising the multi-membership Pattern B — one
// item per (session, tag) pair — ready for the delegate to inject a heading at
// each GroupKey boundary.
//
// For each session, its directory resolves to a project via idx.Match (the
// pre-canonicalised project.Index; Session.Dir re-run through the same canonical
// keying as buildByProject). The project's Tags — stored canonical (lower-cased)
// by Phase 1 — are each re-normalised through project.NormaliseTag defensively,
// so a stray non-canonical stored value (e.g. "Work") cannot split a heading and
// junk values (empty/whitespace) are skipped entirely. Every usable tag emits a
// SessionItem with GroupKey = GroupHeading = the canonical tag.
//
// A session whose project has no usable tags — empty Tags, all-junk Tags, a
// project miss, or an empty Dir — emits exactly one item flagged for the pinned
// Untagged bucket (never zero items, so no session is dropped). Because a
// multi-tag session contributes N items, the total can exceed the live session
// count (the header-count rule for Pattern B).
//
// Tagged items are sorted by (GroupKey, Session.Name) — the canonical tag, then
// the session name. Untagged items are appended after the sorted tagged items.
//
// Every emitted instance of a session shares the same underlying tmux.Session,
// so selecting any instance attaches the same target (task 2-6).
//
// Pure function — no tmux call, no I/O. Zero live sessions yields an empty slice.
func buildByTag(sessions []tmux.Session, idx project.Index) []list.Item {
	var tagged []SessionItem
	var untagged []list.Item

	for _, s := range sessions {
		tags := resolveSessionTags(s, idx)
		if len(tags) == 0 {
			untagged = append(untagged, untaggedItem(s))
			continue
		}
		for _, tag := range tags {
			tagged = append(tagged, SessionItem{
				Session:      s,
				GroupKey:     tag,
				GroupHeading: tag,
			})
		}
	}

	return assembleGroups(tagged, untagged, untaggedHeading)
}

// resolveSessionTags returns the canonical, usable tags for a session's
// directory. It resolves the session's project (project miss or empty Dir yields
// no tags) and defensively re-normalises each stored tag through
// project.NormaliseTag, dropping any that fail (empty/whitespace junk). The
// result is the set of tags under which the session should appear; an empty
// result routes the session to the Untagged catch-all.
func resolveSessionTags(s tmux.Session, idx project.Index) []string {
	if s.Dir == "" {
		return nil
	}

	matched, _, ok := idx.Match(s.Dir)
	if !ok {
		return nil
	}

	var tags []string
	for _, raw := range matched.Tags {
		if tag, ok := project.NormaliseTag(raw); ok {
			tags = append(tags, tag)
		}
	}
	return tags
}

// untaggedItem builds the catch-all SessionItem for a session that has no usable
// tags. It is kept cohesive (CatchAll flag + Untagged heading set in one place)
// so it can later be extracted/unified with unknownItem into the shared pinned +
// empty-suppression catch-all helper (task 2-4).
func untaggedItem(s tmux.Session) SessionItem {
	return SessionItem{
		Session:      s,
		GroupHeading: untaggedHeading,
		CatchAll:     true,
	}
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

// appendCatchAll pins the catch-all bucket (Unknown / Untagged) last and applies
// the empty-suppression rule. resolved holds the already-alphabetically-ordered
// resolvable groups (keyed on their real GroupKey — canonical path or tag);
// catchAll holds the flagged catch-all SessionItems (CatchAll = true,
// GroupHeading = heading) in arbitrary input order.
//
// The catch-all items are sorted by Session.Name and each is stamped with
// GroupKey = heading so that 2-5's boundary logic treats them as one contiguous
// group under a single heading. They are appended after every resolvable group,
// pinning them last regardless of where the heading would fall alphabetically.
//
// Empty-suppression: when there are no catch-all items, resolved is returned
// unchanged — no heading materialises, because 2-5 only emits a header where
// items exist. The catch-all heading's count is derived by 2-5 from the rows
// beneath it, like any other heading.
//
// Pure function — no I/O.
func appendCatchAll(resolved []list.Item, catchAll []list.Item, heading string) []list.Item {
	if len(catchAll) == 0 {
		return resolved
	}

	stamped := make([]SessionItem, 0, len(catchAll))
	for _, it := range catchAll {
		si, ok := it.(SessionItem)
		if !ok {
			// Defensive: only SessionItems are ever routed to a catch-all bucket.
			continue
		}
		si.GroupKey = heading
		stamped = append(stamped, si)
	}
	slices.SortFunc(stamped, func(a, b SessionItem) int {
		return cmp.Compare(a.Session.Name, b.Session.Name)
	})

	items := make([]list.Item, 0, len(resolved)+len(stamped))
	items = append(items, resolved...)
	items = append(items, sessionItemsToList(stamped)...)
	return items
}

// sessionItemsToList boxes a typed []SessionItem into a []list.Item, preserving
// order and element identity. It is the single source of truth for the
// SessionItem → list.Item conversion shared by assembleGroups and appendCatchAll.
// A nil or empty input yields a non-nil, len-0 slice (no panic).
func sessionItemsToList(items []SessionItem) []list.Item {
	out := make([]list.Item, 0, len(items))
	for _, si := range items {
		out = append(out, si)
	}
	return out
}

// assembleGroups is the shared grouping-assembly tail for buildByProject and
// buildByTag. It sorts the resolvable groups by (GroupKey, Session.Name) — the
// single definition of grouped-list ordering — boxes them into []list.Item via
// sessionItemsToList, and hands off to appendCatchAll, which pins the catch-all
// bucket last (under heading) and applies empty-suppression.
//
// resolved holds the typed resolvable-group items (real GroupKey: canonical path
// or tag); catchAll holds the flagged catch-all SessionItems in arbitrary order;
// heading is the catch-all bucket label (Unknown / Untagged). Empty resolved +
// empty catch-all yields an empty slice.
//
// Pure function — no I/O.
func assembleGroups(resolved []SessionItem, catchAll []list.Item, heading string) []list.Item {
	slices.SortFunc(resolved, func(a, b SessionItem) int {
		if c := cmp.Compare(a.GroupKey, b.GroupKey); c != 0 {
			return c
		}
		return cmp.Compare(a.Session.Name, b.Session.Name)
	})

	return appendCatchAll(sessionItemsToList(resolved), catchAll, heading)
}
