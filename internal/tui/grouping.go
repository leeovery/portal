package tui

import (
	"cmp"
	"slices"

	"charm.land/bubbles/v2/list"
	"github.com/leeovery/portal/internal/project"
)

// effectiveDir is the directory grouping matches on: the index's recorded dir
// when present, else the grouping-only derived guess. The item a builder emits
// keeps the recorded value untouched — the two are not interchangeable (the
// derived one is canonicalised, the recorded one is raw).
func effectiveDir(s Session, derived map[string]string) string {
	if s.Dir != "" {
		return s.Dir
	}
	return derived[s.Name]
}

const unknownHeading = "Unknown"

const untaggedHeading = "Untagged"

// buildFlat lists the sessions ungrouped, in the order their rows read.
func buildFlat(sessions []Session) []list.Item {
	return ToListItems(slices.SortedFunc(slices.Values(sessions), compareShown))
}

// The group key is the canonical path, not the heading name, so two distinct
// directories sharing a project name form two distinct groups.
func buildByProject(sessions []Session, idx project.Index, derived map[string]string) []list.Item {
	var known []SessionItem
	var unknown []SessionItem

	for _, s := range sessions {
		dir := effectiveDir(s, derived)
		if dir == "" {
			unknown = append(unknown, unknownItem(s))
			continue
		}

		matched, key, ok := idx.Match(dir)
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

func buildByTag(sessions []Session, idx project.Index, derived map[string]string) []list.Item {
	var tagged []SessionItem
	var untagged []SessionItem

	for _, s := range sessions {
		tags := resolveSessionTags(s, idx, derived)
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

func resolveSessionTags(s Session, idx project.Index, derived map[string]string) []string {
	dir := effectiveDir(s, derived)
	if dir == "" {
		return nil
	}

	matched, _, ok := idx.Match(dir)
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

func untaggedItem(s Session) SessionItem {
	return SessionItem{
		Session:      s,
		GroupHeading: untaggedHeading,
		CatchAll:     true,
	}
}

func unknownItem(s Session) SessionItem {
	return SessionItem{
		Session:      s,
		GroupHeading: unknownHeading,
		CatchAll:     true,
	}
}

// The catch-all is pinned last regardless of where its heading would sort;
// stamping GroupKey = heading makes injectGroupHeaders treat it as one group.
func orderedSessionItems(resolved, catchAll []SessionItem, heading string) []SessionItem {
	slices.SortFunc(resolved, func(a, b SessionItem) int {
		return cmp.Or(cmp.Compare(a.GroupKey, b.GroupKey), compareShown(a.Session, b.Session))
	})

	out := make([]SessionItem, 0, len(resolved)+len(catchAll))
	out = append(out, resolved...)

	if len(catchAll) > 0 {
		stamped := make([]SessionItem, 0, len(catchAll))
		for _, si := range catchAll {
			si.GroupKey = heading
			stamped = append(stamped, si)
		}
		slices.SortFunc(stamped, func(a, b SessionItem) int {
			return compareShown(a.Session, b.Session)
		})
		out = append(out, stamped...)
	}

	return out
}

// Headers must stay real height-1 list items so pagination counts them —
// uncounted heading lines overflow the viewport. Cursor logic relies on: row
// 0 is always a header, and no two headers are adjacent.
func injectGroupHeaders(items []SessionItem) []list.Item {
	out := make([]list.Item, 0, len(items)+8)
	for i := 0; i < len(items); {
		key := items[i].GroupKey
		j := i
		for j < len(items) && items[j].GroupKey == key {
			j++
		}
		out = append(out, HeaderItem{Heading: items[i].GroupHeading, Count: j - i, Key: key})
		out = append(out, sessionItemsToList(items[i:j])...)
		i = j
	}
	return out
}

func sessionItemsToList(items []SessionItem) []list.Item {
	out := make([]list.Item, 0, len(items))
	for _, si := range items {
		out = append(out, si)
	}
	return out
}

func assembleGroups(resolved []SessionItem, catchAll []SessionItem, heading string) []list.Item {
	return injectGroupHeaders(orderedSessionItems(resolved, catchAll, heading))
}
