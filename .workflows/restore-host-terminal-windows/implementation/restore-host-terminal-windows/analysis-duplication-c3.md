AGENT: duplication
FINDINGS:
- FINDING: "Replace the first line of listView with a rendered header" snippet copy-pasted across every applySectionHeader branch
  SEVERITY: medium
  FILES: internal/tui/model.go:4707-4711 (Opening band), internal/tui/model.go:4732-4736 (abort banner), internal/tui/model.go:4750-4754 (multi-select banner), internal/tui/model.go:4771-4775 (unsupported banner), internal/tui/model.go:4784-4788 (filter query), internal/tui/model.go:4799-4803 (standard header), internal/tui/model.go:4436-4440 + 4448-4452 (applyProjectsSectionHeader)
  DESCRIPTION: The identical four-line idiom that swaps a freshly-rendered header line in for the
    first line of the bubbles/list view —
      idx := strings.IndexByte(listView, '\n')
      if idx < 0 { return header }
      return header + listView[idx:]
    — is repeated verbatim EIGHT times across the two section-header appliers (a ninth
    strings.IndexByte at model.go:4845 in replaceListBodyWithNoMatches is a DIFFERENT operation —
    it keeps the first line and replaces the body — so it is not part of this cluster). This
    work unit authored FOUR of the eight copies as it added new section-header claimants across
    separate tasks: the Opening band (task 6-5), the pre-flight abort banner (task 6-7), the
    multi-select `N selected` banner (task 5-3), and the proactive unsupported banner (task 6-2).
    Each new branch independently re-copied the snippet rather than reusing the two pre-existing
    copies (filter-query / standard header from spectrum-tui-design), so the count crossed the
    Rule-of-Three three times over without extraction. This is textbook copy-paste drift across
    task boundaries: every branch now hand-maintains the "no-newline degenerate returns header
    bare, otherwise splice header onto listView from the first newline" contract, and a future
    tweak (e.g. handling a "\r\n" listView, or a different degenerate fallback) has to be applied
    in eight places or it silently diverges between the burst banners and the standard header.
  RECOMMENDATION: Extract one small helper — e.g. `func replaceHeaderLine(listView, header string)
    string` (or a Model method) holding the IndexByte + degenerate-guard + splice — and have all
    eight branches return `replaceHeaderLine(listView, renderXxx(...))`. Each claimant branch then
    collapses to its render call plus the shared splice, so the section-header row's "swap, don't
    insert" invariant (the one-row-per-delegate pagination contract the branches all depend on)
    lives in exactly one place.

- FINDING: Left-bar single-glyph column renderers duplicated per selection state (marked / gone)
  SEVERITY: low
  FILES: internal/tui/session_item.go:387-390 (renderMarkedLeftBarColumn), internal/tui/session_item.go:402-405 (renderGoneLeftBarColumn), internal/tui/session_item.go:370-376 (renderLeftBarColumn, selected branch)
  DESCRIPTION: renderMarkedLeftBarColumn and renderGoneLeftBarColumn are byte-identical except for
    the glyph constant they render:
      markerStyle.Render(<glyph>) + bg.Render(padTo("", leftBarColumnWidth-lipgloss.Width(<glyph>)))
    (multiSelectMarker `●` vs flashWarningGlyph `⚠`). The selected branch of the pre-existing
    renderLeftBarColumn renders the SAME shape for the `▌` selectorBar. This work unit added the
    two new copies across separate tasks — the marked-row `●` column (task 5-2) and the gone-row
    `⚠` column (task 6-7) — each hand-rolling the "render one glyph in the fixed 2-cell left-bar
    column and pad the remainder" logic that already existed for the selector bar, taking the
    pattern to three near-identical instances. The shared invariant is the leftBarColumnWidth
    (2-cell) geometry that keeps the name's left edge fixed regardless of which glyph occupies col
    0; three independent copies mean a change to that column width or the pad rule must be mirrored
    three ways or the marked/gone/selected rows drift out of column alignment.
  RECOMMENDATION: Extract a single glyph-column helper — e.g. `func renderLeftBarGlyphColumn(glyph
    string, glyphStyle, bg lipgloss.Style) string` returning
    `glyphStyle.Render(glyph) + bg.Render(padTo("", leftBarColumnWidth-lipgloss.Width(glyph)))` —
    and have renderMarkedLeftBarColumn (`●`), renderGoneLeftBarColumn (`⚠`), and
    renderLeftBarColumn's selected branch (`▌`) all delegate to it. The precedence switch in
    renderSessionRow (gone → marked → selector) is unchanged; only the identical column-geometry
    plumbing is shared, so the 2-cell left-bar contract lives in one place.

- FINDING: fitFilterCluster re-implements the fitLeftCluster narrow-degrade ellipsis-fitting loop
  SEVERITY: low
  FILES: internal/tui/footer.go:186-222 (fitFilterCluster), internal/tui/footer.go:322-369 (fitLeftCluster)
  DESCRIPTION: fitFilterCluster (added by this work unit, task 5-4, for the multi-select mode
    footer) is a near-line-for-line copy of the pre-existing fitLeftCluster narrow-degrade
    algorithm: try the full cluster first and return it if it fits; otherwise greedily grow a
    leading prefix in a `for n := 1; n <= len(entries); n++` loop, appending a
    `<cluster> · …` separator+ellipsis, breaking when the candidate width exceeds the budget;
    then fall back to the bare ellipsis if it fits, else an empty cluster. The two differ only in
    (a) the entry type they range over (filterFooterEntry vs keymapEntry), (b) the cluster
    renderer they call (renderFilterCluster vs renderFooterCluster), and (c) whether a right-anchor
    budget is reserved (fitFilterCluster uses the full width; fitLeftCluster subtracts the right
    anchor + one spacer). fitFilterCluster's own doc-comment flags the relationship ("It mirrors
    fitLeftCluster … for the per-glyph filterFooterEntry cluster path"), i.e. the parallel is
    acknowledged and hand-kept. This is only two instances (borderline against the Rule of Three),
    but each is a ~25-line block and the §2.7 narrow-degrade behaviour (how footers truncate on
    one line without wrapping) is exactly the kind of layout invariant that should not be able to
    drift between the standard/Projects footers and the multi-select footer.
  RECOMMENDATION: Parameterise the shared narrow-degrade loop over the cluster renderer and the
    budget — e.g. a package-private helper taking a `renderCluster func(n int) (string, int)`
    (or a `func([]E) string` plus the separator/ellipsis widths) and a budget int, returning the
    fitted cluster + width — and have both fitLeftCluster and fitFilterCluster call it with their
    own renderer and budget. The per-type cluster renderers (renderFilterCluster /
    renderFooterCluster) stay separate; only the try-full-then-greedy-prefix-with-ellipsis
    algorithm is unified.
SUMMARY: The Phase-7/8 consolidations landed well — the spawn log-emission (logemit.go),
  message renderers (message.go), count-semantics (classify.go), exec-boundary (exec_boundary.go),
  and nanoid alphabet are all now single-sourced, so the cycle-1/2 high-severity duplication is
  resolved. The residual duplication is in the TUI presentation layer, all introduced as the
  multi-select/burst UI was built out task-by-task: four fresh copies of the "replace the first
  header line" splice across applySectionHeader's new banner branches (medium), two new near-
  identical left-bar single-glyph column renderers alongside the pre-existing selector one (low),
  and a mirrored footer narrow-degrade fitter (low). The parallel CLI-runSpawn / TUI-decideBurst
  orchestration and the burst-progress-pipe / bootstrap-progress-pipe structural mirror were
  reviewed and left as-is — their extractable decisions already route through the shared spawn.*
  helpers, and the remaining parallelism is genuine sync-vs-async control-flow, not copy-pasted
  logic.
