TASK: spectrum-tui-design-8-3 — Model the no-matches footer membership structurally instead of by magic label string

ACCEPTANCE CRITERIA:
- `noMatchesFooterEntries` no longer matches on the `"browse results"` label literal (no cross-file display-string coupling remains).
- The §7.3 no-matches footer renders exactly as before (browse-results hint absent).
- Rewording the input-active footer's "browse results" copy does not change which entries the no-matches footer contains.
- Unit test asserts the no-matches footer excludes the browse-results entry, written so it would still pass if the browse-results display copy changed (does not depend on the literal).
- Existing filtering-footer render tests continue to pass with byte-identical output.

STATUS: Complete

SPEC CONTEXT:
§7.1 (specification.md:285-289) defines the input-active footer as `type to filter · ↵/↓ browse results · esc clear`. §7.3 (specification.md:295-296) keeps the footer in input-active form for the over-filtered no-matches state but reduced — there are no results to browse. The task is a pure structural-model refactor: derive the reduced §7.3 set without coupling to the display text of the browse-results entry.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/filter_footer.go:50-54 — `filterFooterEntry` gains a `BrowseResults bool` flag (documented as mirroring the sessionsKeymap Core-flag membership model).
  - internal/tui/filter_footer.go:61-71 — `filteringFooterEntries` tags the browse-results entry with `BrowseResults: true` at the single source of truth.
  - internal/tui/filter_footer.go:79-88 — new `dropBrowseResults` helper filters on the flag (`if e.BrowseResults { continue }`), order-preserving, capacity-hinted.
  - internal/tui/filtering_no_matches.go:77-79 — `noMatchesFooterEntries` now returns `dropBrowseResults(filteringFooterEntries())`. No `Label == "browse results"` literal anywhere.
- Notes:
  - Confirmed via grep: the only remaining `"browse results"` string occurrences in non-test code are (a) the entry's own `Label` value (filter_footer.go:68) and (b) doc comments (filter_footer.go:12,32,116; filtering_no_matches.go:15,69-70; model.go:4112,4119). NONE is a membership predicate. The cross-file display-string coupling the task targeted (the former `Label == "browse results"` drop) is fully removed.
  - The §7.3 render path (model.go:4117-4124 → renderNoMatchesFooter → renderFilterFooter) is unchanged in structure; it now sources the reduced entry slice through the flag-based drop. Output is byte-identical: the same two entries (`type to filter`, `esc clear`) in the same order with the same per-glyph tokens survive, because dropBrowseResults removes exactly the same single entry the old label match did.

TESTS:
- Status: Adequate
- Coverage:
  - internal/tui/filtering_no_matches_test.go:198-227 (TestNoMatchesFooterEntries_ExcludesBrowseResultsStructurally) — asserts no surviving entry has `BrowseResults == true` and that the result equals the shared input-active set minus the tagged entry. The test identifies the entry by the flag, never by label text; it never references the `"browse results"` literal.
  - internal/tui/filtering_no_matches_test.go:235-269 (TestNoMatchesFooterEntries_DecoupledFromBrowseResultsCopy) — the decisive regression guard: it reswords the tagged entry's label to `"view the matches"` on a copy and proves `dropBrowseResults` still removes it. A label-string filter would have silently regained the entry here; the flag filter still drops it. This directly encodes the "rewording cannot break the filter" acceptance criterion.
  - internal/tui/filtering_no_matches_test.go:106-126 (TestNoMatches_FooterStaysInputActiveForm) — end-to-end render assertion: `type to filter` + `esc clear` present, `browse results` absent, list-active entries absent, standard footer absent. Confirms the §7.3 footer renders byte-correct (browse-results hint absent).
  - internal/tui/filtering_reskin_test.go:175-207 (TestFiltering_InputActiveFooter / ...Colours) — the existing input-active render tests still assert `browse results` is present in the FULL input-active footer and pin its accent.blue/orange/detail SGR. These are unaffected by the refactor (the entry and its Label are unchanged) — byte-identical output preserved.
- Notes:
  - Both new tests are correctly written to be label-agnostic; the reword-on-a-copy test is the strongest possible form of the "test passes even if copy reworded" requirement.
  - The decoupled test also pins "exactly one browse-results-tagged entry" (a precondition guard), which catches accidental double-tagging or untagging at the source.
  - Not over-tested: the two new tests are focused and non-redundant (one pins exclusion + composition-from-shared-set, the other pins copy-decoupling). No bloat.
  - Not under-tested: the render-level assertion (TestNoMatches_FooterStaysInputActiveForm) plus the entry-level assertions cover both the data model and the rendered surface.

CODE QUALITY:
- Project conventions: Followed. Flag-based membership mirrors the codebase's stated keymap Core-flag pattern (called out explicitly in the doc comment at filter_footer.go:45-49). No t.Parallel() in the test file (correct for this package). No new slog logger constructed. Idiomatic Go slice filtering.
- SOLID principles: Good. Single source of truth for the entry set (filteringFooterEntries); the reduced set is now derived, not hand-maintained — removes the prior duplication/divergence risk.
- Complexity: Low. dropBrowseResults is a trivial order-preserving filter loop.
- Modern idioms: Yes — capacity-hinted `make([]filterFooterEntry, 0, len(src))`, range-with-continue filter.
- Readability: Good. Intent is self-documenting and the doc comments explicitly explain WHY the flag exists (the fragility being eliminated).
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
