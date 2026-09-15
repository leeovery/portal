package tui

import (
	"slices"
	"sync"

	"charm.land/bubbles/v2/list"
	"github.com/leeovery/portal/internal/resolver"
)

// searchEntry holds the session fields a filter value was built from, so a pass
// can rank a target it was handed without holding the item it came from.
type searchEntry struct {
	name string
	dir  string
}

// searchItemSource maps each session row's filter value to every distinct field
// pair that produced it, so a filter pass can answer for targets from any
// generation the source has held. A filter value is not injective — a session
// named for the text another session's directory abbreviates to yields the same
// one — so a key holds a set rather than a single pair. It is held by pointer
// because Model is a value Bubble Tea copies on every Update: a map field would
// go stale in whichever copy the filter closure was built against, while a
// pointer is shared by every copy. The mutex is load-bearing: the write runs on
// the Update goroutine and the read on the goroutine Bubble Tea gives the filter
// command, and the two are free to overlap.
type searchItemSource struct {
	mu      sync.RWMutex
	entries map[string][]searchEntry
}

// set records the session rows among items. Rows sharing a session collapse onto
// one entry, and a header's empty filter value contributes none.
func (s *searchItemSource) set(items []list.Item) {
	entries := make(map[string][]searchEntry, len(items))
	for _, item := range items {
		si, ok := item.(SessionItem)
		if !ok {
			continue
		}
		entry := searchEntry{name: si.Session.Name, dir: si.Session.Dir}
		value := si.FilterValue()
		if !slices.Contains(entries[value], entry) {
			entries[value] = append(entries[value], entry)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = entries
}

// current answers with the whole map under one hold. set replaces it rather than
// mutating it, so the caller may read the returned map with no hold.
func (s *searchItemSource) current() map[string][]searchEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.entries
}

// containmentFilter narrows to the sessions whose own name or recorded directory
// contains term, in the order the list already holds them, for as long as the
// committed filter text is still term; any other value falls through to the
// picker's own fuzzy rule. Ranks index the targets the pass was handed, so they
// resolve against the item generation that pass belongs to however many rebuilds
// have landed since. A target ranks only when the source holds it and every field
// pair under it matches: a target the source does not hold, and one whose pairs
// disagree about the term, both rank nothing — so a pass running behind, or one
// answering for a target two distinct sessions produced, can only omit a row,
// never surface one the term does not match.
//
// MatchedIndexes is left nil, so a delegate that highlights matched runes would
// get none from this rule.
func containmentFilter(term string, src *searchItemSource) list.FilterFunc {
	return func(query string, targets []string) []list.Rank {
		if query != term {
			return list.DefaultFilter(query, targets)
		}
		entries := src.current()
		var ranks []list.Rank
		for i, target := range targets {
			if held := entries[target]; len(held) > 0 && everyEntryMatches(held, query) {
				ranks = append(ranks, list.Rank{Index: i})
			}
		}
		return ranks
	}
}

// everyEntryMatches reports whether term is contained by every field pair one
// filter value was built from.
func everyEntryMatches(entries []searchEntry, term string) bool {
	for _, entry := range entries {
		if !resolver.MatchesSearchTerm(term, entry.name, entry.dir) {
			return false
		}
	}
	return true
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
