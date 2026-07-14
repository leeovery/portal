TASK: 5.6 — Sticky selection across filter/paging/regroup + Space-preview prune (restore-host-terminal-windows-5-6 / tick-c7c428)

ACCEPTANCE CRITERIA:
- Marks survive an s-regroup: same sessions stay selected, banner count unchanged.
- Marks survive paging: navigating across pages does not clear the set.
- A row filtered out by an active query stays selected and its ● reappears when the filter clears.
- The Space-preview round-trip returns to Sessions in multi-select mode with the selection intact and re-rendered.
- A session externally killed during the preview is pruned on the post-dismiss refresh; every survivor stays selected.
- A marked session that moves buckets on an s-regroup (By-Project → By-Tag) stays marked.

STATUS: Complete

SPEC CONTEXT:
Spec §Multi-Select Mode → Sticky selection: selection is sticky across filtering, paging, regrouping, and the Space-preview round-trip; on return, rebuildSessionList re-renders in-mode with the selection intact, pruning ONLY a selection whose session was externally killed during the preview (consistent with the pre-flight "gone session can't be opened" rule). §Granularity: the selection model is a set of session identities (Session.Name), so a mark survives regroup/filter/paging even though a By-Tag session spans multiple list rows. This task establishes stickiness end-to-end and adds the single active mutation — the live-session prune on the sessions-refresh chokepoint — reusable by 5.7 / Phase 6.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/model.go:1613-1626 — pruneSelectionToLiveSessions(): builds a live-name set from m.sessions and deletes any marked name not present. Short-circuits on empty set (nil-tolerant), so non-multi-select refresh paths pay nothing. Keyed on Session.Name (pure set-difference), reusable method per plan.
  - internal/tui/model.go:1592-1599 — applySessions(): prunes BEFORE rebuildSessionList so the pruned set feeds the ● re-render. This is the single refresh chokepoint shared by SessionsMsg and previewSessionsRefreshedMsg.
  - internal/tui/model.go:2431-2443 — previewSessionsRefreshedMsg arm routes the post-dismiss refresh through applySessions (→ prune), then reanchors the cursor.
  - internal/tui/model.go:3357-3375 — Space handler sets m.preview + activePage=pagePreview only; leaves multiSelectMode/selectedSessions untouched (not gated on !multiSelectMode, so Space stays live in mode per 5.5).
  - internal/tui/model.go:1919-1923 — exitPreviewToSessions() (dismiss transition) flips activePage=PageSessions and zeroes m.preview only; does not reset mode or set.
  - internal/tui/model.go:3477-3494 — handleSwitchViewKey() (s-regroup) recomputes items/title via rebuildSessionList and resets the paginator; never touches multiSelectMode/selectedSessions.
  - internal/tui/model.go:1743-1790 — rebuildSessionList() mutates sessionList items only; never resets the set.
  - internal/tui/model.go:1281-1289 — sessionDelegate() propagates MultiSelect + Selected (aliased map); the ● tracks the in-place-mutated set.
- Notes: The delegate references the SAME selectedSessions map, so the prune's in-place delete re-renders the ● off the pruned rows without an explicit SetDelegate — the plan's "re-apply the delegate after the prune (already hit by rebuildSessionList via applyCanvasMode)" is achieved via map aliasing rather than a literal applyCanvasMode call (rebuildSessionList does not call applyCanvasMode). Behaviourally correct and documented in the prune comment; not a defect. No drift from the plan: the reusable helper + one-line applySessions call match exactly. No scope creep.

TESTS:
- Status: Adequate
- Location: internal/tui/multi_select_sticky_test.go (6 tests, one per acceptance criterion / plan test name).
- Coverage:
  - TestMultiSelectMarksSurviveRegroup — s-regroup keeps alpha+charlie marked, count unchanged, mode intact.
  - TestMultiSelectMarksSurvivePaging — 60 sessions (>1 page), Ctrl+↓/Ctrl+↑ preserve the set + mode.
  - TestMultiSelectFilteredOutSessionStaysMarked — full Build() model + live filter drain; bravo stays in set while filtered out (no ● rendered), ● reappears on Esc-clear; asserts both set membership AND rendered marker.
  - TestMultiSelectPreviewRoundTripKeepsSelection — Space→Esc→refresh (no kill): returns PageSessions, in-mode, both marks intact, ● re-rendered.
  - TestMultiSelectPrunesExternallyKilledSession — alpha killed during preview (stepListerStub post-kill list): pruned, charlie survives, count 1, survivor's ● still renders.
  - TestMultiSelectMarkedSessionSurvivesBucketMove — By-Project→By-Tag move ("Portal"→"work" bucket) keeps portal-abc marked; ● renders in the new bucket.
  - The prune path is exercised through the real applySessions chokepoint (pressSpaceThenEscWithRefresh drives Space→Esc→dismiss→refresh→refilter through Update), i.e. behaviour not implementation detail.
- Notes: Assertions combine set state (IsSessionSelected/SelectedSessionCount/MultiSelectActive) with the rendered ● (ansi.Strip + multiSelectMarker), so a test would fail if either the model state or the render broke. Preconditions guard test setup (page-count > 1, filter narrows, bucket heading moves). No over-testing: each test targets a distinct AC/edge with no redundant variations; the markTwoFlatSessions helper removes duplication; no unnecessary mocking. Not under-tested: all 6 ACs and all 5 named edge cases covered.

CODE QUALITY:
- Project conventions: Followed — value/pointer-receiver split matches the model.go convention; no t.Parallel(); test seams reuse existing stubs (stepListerStub, modelWithSeams, enterMultiSelect); reusable helper method per the plan's 5.7/Phase-6 reuse mandate.
- SOLID principles: Good — pruneSelectionToLiveSessions is single-responsibility; applySessions stays the one refresh chokepoint.
- Complexity: Low — the prune is an O(sessions + selected) set-difference with an early return.
- Modern idioms: Yes — `for i := range m.sessions` index loop (avoids struct copy), map-set membership.
- Readability: Good — self-documenting; comments correctly explain the map-aliasing re-render and the shared prune rule.
- Security/Performance: No concerns — prune only allocates when the set is non-empty (multi-select active); no N+1, no per-render store reads.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
