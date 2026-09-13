package tui

import (
	"charm.land/bubbles/v2/list"
	"github.com/leeovery/portal/internal/resolver"
)

// searchItemSource holds the item slice a filter pass resolves its ranks
// against. It is held by pointer because Model is a value Bubble Tea copies on
// every Update: a slice field would go stale in whichever copy the filter
// closure was built against, while a pointer is shared by every copy.
type searchItemSource struct {
	items []list.Item
}

func (s *searchItemSource) set(items []list.Item) {
	s.items = items
}

func (s *searchItemSource) current() []list.Item {
	return s.items
}

// containmentFilter narrows to the sessions whose own name or recorded
// directory contains term, in the order the list already holds them, for as
// long as the committed filter text is still term; any other value falls
// through to the picker's own fuzzy rule. A rank indexes the unfiltered item
// slice, headers included, so the source must hold exactly the slice the
// targets were built from — a length mismatch means it is out of step, and the
// picker's rule stands rather than a lookup against the wrong rows.
//
// MatchedIndexes is left nil, so a delegate that highlights matched runes would
// get none from this rule.
func containmentFilter(term string, src *searchItemSource) list.FilterFunc {
	return func(query string, targets []string) []list.Rank {
		items := src.current()
		if query != term || len(items) != len(targets) {
			return list.DefaultFilter(query, targets)
		}
		var ranks []list.Rank
		for i, item := range items {
			si, ok := item.(SessionItem)
			if !ok {
				continue
			}
			if resolver.MatchesSearchTerm(query, si.Session.Name, si.Session.Dir) {
				ranks = append(ranks, list.Rank{Index: i})
			}
		}
		return ranks
	}
}

// installSearchFilter is a no-op for every picker but one a search form opened
// with a term, so every other list keeps list.DefaultFilter.
func (m *Model) installSearchFilter() {
	if !m.searchForm || m.searchTerm == "" {
		return
	}
	m.searchItems = &searchItemSource{}
	m.sessionList.Filter = containmentFilter(m.searchTerm, m.searchItems)
}
