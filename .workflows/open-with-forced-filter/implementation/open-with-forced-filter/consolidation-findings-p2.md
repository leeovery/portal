# Consolidation Findings: open-with-forced-filter (Phase 2)

## Findings

None. The phase's four tasks assemble cleanly:

- `resolver.MatchesSearchTerm` is the single containment predicate, consumed by both deciders (`cmd/open_search.go:112`, `internal/tui/search_filter.go:46`) — the count and the narrowed list cannot disagree by construction.
- `ListSessions` / `ListSessionsProbe` share `listSessionsArgs` and `parseSessionList` (`internal/tmux/tmux.go:125`, `:129`, `:145`, `:150`), and the Probe naming and doc phrasing match the pre-existing `HasSessionProbe` (`internal/tmux/tmux.go:92`) — no near-miss, no drift.
- The one rule authored twice — "the enumeration less the session the caller is already in" (`cmd/open_search.go:98-105` against `internal/tui/model.go:1108-1118`) — is a single-line exclusion that agrees with its counterpart today, both reached through the same `tmux.InsideTmux()` + tolerant `CurrentSessionName` gate (`cmd/open.go:682-689`), and a divergence would show as a visible row set rather than silently. It does not clear the duplication bar.
- `searchItems` staleness: the re-point precedes `SetItems` on every rebuild path (`internal/tui/model.go:1223-1228`), and each of the four orderings that reach `applySearchLanding` — sessions-first, projects-first, grouped-mode projects rebuild (`:1637`), and the loading-page transition (`:1473`) — leaves the source holding the slice the filter pass is handed. No dead code, no supersession, no accretion.

## Comment Corrections

- `internal/tui/session_item.go:81` — `FilterValue` is no longer what every filter entry point narrows on. Task 2-3 installed a containment `list.FilterFunc` that ignores the `targets` built from this method and reads `Session.Name` / `Session.Dir` off the item instead (`internal/tui/search_filter.go:35-51`), precisely because the single-space join cannot be split back into the two fields. A contributor who narrows or widens the match domain by editing this method would leave the search form matching the old set, silently diverging from `-f` — the divergence the specification's shared-fields rule forbids. The rest of the comment (why the directory is home-abbreviated, why the grouping-derived directory is excluded) carries what the code cannot and is kept.
  OLD:
  ```
  // FilterValue is the text every filter entry point narrows on: the session name
  // joined to its recorded directory, home-abbreviated so a term hitting the home
  // prefix cannot match a session on characters no row shows. The grouping-derived
  // directory is no part of it — a regroup must never make a session findable by a
  // path it was not findable by a moment earlier.
  ```
  NEW:
  ```
  // FilterValue is the text the picker's own fuzzy rule narrows on: the session
  // name joined to its recorded directory, home-abbreviated so a term hitting the
  // home prefix cannot match a session on characters no row shows. The
  // grouping-derived directory is no part of it — a regroup must never make a
  // session findable by a path it was not findable by a moment earlier. A filter
  // rule reading those two fields off the item rather than this string matches the
  // same values by another route, so widening or narrowing what a filter matches
  // means changing both.
  ```
