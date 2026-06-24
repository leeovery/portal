TASK: spectrum-tui-design-2-9 — Filtering no-matches state: centred glyph + `No sessions match "<query>"` + widen-search hint (§7.3)

ACCEPTANCE CRITERIA (from plan):
- Active non-empty query matching zero sessions renders centred dim glyph (text.faint), `No sessions match "<query>"` (text.primary) with query interpolated, and `⌫ to widen the search · esc to clear the filter` hint (text.detail).
- Footer stays input-active form (not list-active).
- Renders ONLY when query matches zero — not when results exist, distinct from empty-sessions state (§11.1, Phase 4, no active query).
- Query interpolated with byte-exact literal quotes (verbatim, like formatSessionGoneFlash), not %q.
- All colours via tokens; rendered on the owned canvas.
- VISUAL VERIFICATION: vhs capture matches `Filtering — no matches (MV)`.
- Behaviour parity: display-only empty state; filter engine / commit-clear transitions / ⌫·Esc unchanged.

STATUS: Complete

SPEC CONTEXT:
§7.3 (over-filtered no matches): "a centred empty state — a dim `⌀` glyph (text.faint),
`No sessions match "<query>"` (text.primary), hint `⌫ to widen the search · esc to clear
the filter` (text.detail). Footer stays in input-active form." §7.1 pins the input-active
footer form. §11.1 (Phase 4) is the DISTINCT empty-sessions state (no sessions exist, no
active query). §2.9 supplies text.faint/text.primary/text.detail tokens. Note 8-3 later
restructured the no-matches footer to DROP the `browse results` entry (verified against
current code below).

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/filtering_no_matches.go — the whole surface (glyph/hint consts,
    formatNoMatchesMessage, sessionListNoMatches predicate, renderNoMatchesBody,
    noMatchesFooterEntries, renderNoMatchesFooter).
  - internal/tui/model.go:4055-4057 (body swap via replaceListBodyWithNoMatches),
    :4122-4124 (footer routing in renderSessionsFooterForFilterState),
    :4238-4250 (replaceListBodyWithNoMatches — height-neutral body replace).
  - Shared centring helper internal/tui/empty_states.go:56 (renderEmptyStateBody) reused
    by both §7.3 and §11.1, so the two surfaces cannot drift in layout while staying
    distinct in content.
  - Footer machinery reused from internal/tui/filter_footer.go (filteringFooterEntries +
    dropBrowseResults, structural BrowseResults flag).
- Notes:
  - Predicate sessionListNoMatches (filtering_no_matches.go:48) is exactly right: requires
    FilterState in {Filtering, FilterApplied}, non-empty FilterValue, and zero VisibleItems.
    The non-empty-query requirement is what structurally keeps it distinct from §11.1.
  - Distinctness is doubly guarded: the §7.3 predicate needs an active query; the §11.1
    predicate (sessionListEmpty) needs Unfiltered — mutually exclusive, and ordered in
    model.go so they cannot both fire.
  - formatNoMatchesMessage uses fmt.Sprintf(`No sessions match "%s"`, query) — literal
    straight-quote bytes, byte-exact, faithfully mirroring formatSessionGoneFlash. NOT %q.
  - Footer correctly drops the browse-results entry STRUCTURALLY (BrowseResults flag, not
    label match), so the rendered footer reads `type to filter · esc clear`. This matches
    the 8-3 restructure and §7.3.
  - Height-neutral body replacement (Height()-1 rows) preserves the one-row-per-delegate
    pagination invariant — body is empty in this state anyway.
  - GLYPH DEVIATION (non-blocking): the spec §7.3, the plan task (title/problem/solution/
    outcome/Do/AC/test-name), and the task gist ALL byte-specify `⌀` (U+2300 DIAMETER SIGN).
    The implementation ships `∅` (U+2205 EMPTY SET) — noMatchesGlyph = "∅". The code comment
    asserts "the §7.3 reference shows ∅", but the reference PNG's circle-with-stroke is
    visually ambiguous between the two and the spec/plan TEXT says `⌀`. The glyphs render
    near-identically at terminal sizes (confirmed: shipped capture matches the reference
    frame), and `∅` is arguably more semantically apt for "no matches", so this is a
    copy/intent confirmation, not a functional defect. Flagged for author/reviewer decision.

TESTS:
- Status: Adequate
- Coverage (internal/tui/filtering_no_matches_test.go):
  - RendersGlyphMessageHint — glyph + message + hint present, both modes; asserts the ⌫
    glyph not the literal word "backspace".
  - InterpolatesQueryVerbatimWithLiteralQuotes — space/dash query + a discriminating
    EMBEDDED-QUOTE case that actually separates the literal-quote pattern from %q (asserts
    verbatim `"say "hi""` present and `\"` absent). This is the right discriminator.
  - FooterStaysInputActiveForm — `type to filter`/`esc clear` present; `browse results`,
    list-active labels (`navigate`/`clear filter`), and standard `switch view` all absent.
  - DoesNotRenderWhenResultsExist — matching query shows rows, not the empty state.
  - NotRenderedWithoutActiveQuery — §11.1 condition (zero sessions, Unfiltered) does NOT
    trigger no-matches (distinctness pinned).
  - OnlyRendersWithActiveNonEmptyQueryAndZeroItems — predicate truth table (3 cases).
  - FooterEntries_ExcludesBrowseResultsStructurally + DecoupledFromBrowseResultsCopy —
    pin the structural (flag-based, not label-based) membership model; the second mutates a
    reworded label to prove a label filter would regress and the flag filter does not. Strong.
  - ColourRoles — exact mode-resolved SGR for text.faint/text.primary/text.detail, both
    modes — catches a token swap, not just glyph presence.
  - QueryWhittledToEmptyExitsState — backspace-to-empty exits the state (parity edge case).
- Notes:
  - Coverage maps 1:1 onto the plan's seven required tests plus two extra footer-structure
    tests and the whittle-to-empty parity test. Not over-tested — each test pins a distinct
    contract; the SGR/structural tests guard against silent token/label regressions rather
    than re-asserting presence.
  - Tests pin `∅` via the noMatchesGlyph const symbol (not a literal), so they would NOT
    catch the spec-vs-impl glyph deviation — by design they assert whatever the const holds.
    Acceptable, but it means the glyph choice is locked only by the const, not by a
    spec-anchored literal. (See non-blocking note.)
  - No t.Parallel() — correct per the package's shared-mock convention.

CODE QUALITY:
- Project conventions: Followed. Small focused helpers, interface-free (pure render), token
  layer respected (no literal hex — all colours via theme.MV.* through headerStyle). No
  *slog.Logger constructed. No bare os.Exit. Idiomatic Go.
- SOLID principles: Good. Single-responsibility helpers; the no-matches surface composes the
  shared renderEmptyStateBody and the shared filter-footer machinery rather than duplicating
  centring/footer logic (DRY without premature abstraction).
- Complexity: Low. sessionListNoMatches is a flat 3-guard predicate; render paths are linear.
- Modern idioms: Yes — fmt.Sprintf literal-quote pattern, lipgloss compositional render.
- Readability: Strong. Comments are unusually thorough and accurately document the §-anchored
  intent, the distinctness invariant, and the structural-drop rationale. (One comment is
  factually contestable — see glyph note.)
- Issues: none functional.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] internal/tui/filtering_no_matches.go:26 — noMatchesGlyph ships `∅` (U+2205 EMPTY
  SET) but the spec §7.3, the plan task (every section incl. the test-name string), and the
  task gist all byte-specify `⌀` (U+2300 DIAMETER SIGN). Decide whether to switch the const
  to `⌀` to match the spec/plan letter, or to amend the spec/plan to ratify `∅` as the
  intended glyph (and fix the line-25 comment which asserts the reference shows `∅`).
  Requires a decision on which is authoritative — hence idea, not quickfix.
