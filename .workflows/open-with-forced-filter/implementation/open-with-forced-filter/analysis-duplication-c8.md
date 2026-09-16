AGENT: duplication
FINDINGS: none
COMMENT_CORRECTIONS:
- internal/tui/search_filter.go:39-40 — attributes the exclusion of header rows to their empty filter value, but the loop excludes them by the `item.(SessionItem)` type assertion, which never reads a filter value; the surviving half restates the dedup the `slices.Contains` line already shows in the unit the clause names.
  OLD: // Rows sharing a field pair collapse onto one entry, and a header's empty filter
  // value contributes none.
  NEW: // Rows sharing a field pair collapse onto one entry.
SUMMARY: A full fresh pass over the whole change set found no duplication clearing the floor — every rule the feature restates across files (sigil recognition and term extraction, the containment predicate, the searchable field set, home abbreviation, the searched-set exclusion, the `list-sessions` argv and its parse, the `portal open` expansion the shell shims ask for) resolves to one named home that the other sites call, and the two places that state a rule twice (`runSearchForm`'s emit-on-no-picker branches, the shim function names in `cmd/init.go`) each have a test that fails loudly on divergence. One comment in new code misattributes a mechanism and is corrected above.
