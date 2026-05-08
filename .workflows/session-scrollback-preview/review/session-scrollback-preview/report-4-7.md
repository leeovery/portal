TASK: session-scrollback-preview-4-7 — Confirm _portal-saver exclusion at Sessions-list source

ACCEPTANCE CRITERIA:
- Audit recorded.
- If a leak was found, fix is in list-population code path, NOT in preview layer.
- Regression-pin test asserts _portal-saver is absent from the rendered Sessions list when upstream contains it.
- No string reference to _portal-saver exists in internal/tui/pagepreview.go.
- Phase 1 ListWindowsAndPanesInSession call is unchanged.

STATUS: Complete

SPEC CONTEXT:
Spec § Cross-cutting Seams > _portal-saver Self-Reference mandates exclusion at list-population, NOT preview. Spec § Out of Scope (v1) reinforces "Preview-layer _portal-saver suppression (excluded at list-population layer instead)."

IMPLEMENTATION:
- Status: Implemented (audit-first; pre-existing exclusion confirmed sound).
- Location:
  - internal/tmux/tmux.go:149-163 — Client.ListSessions applies strings.HasPrefix(s.Name, "_") as a final post-processing filter. Comment block anchors the Portal-wide invariant.
- Notes:
  - Filter is by _ prefix (broader than exact-match) — also covers _portal-bootstrap.
  - ListSessionNames delegates to ListSessions, inheriting filter.
  - All TUI Sessions-list callsites funnel through m.sessionLister.ListSessions().
  - Full read of internal/tui/pagepreview.go (323 lines) confirms zero _portal-saver references — preview-layer blacklist correctly absent.

TESTS:
- Status: Adequate.
- Coverage:
  - internal/tmux/tmux_test.go:139 TestListSessionsFiltersUnderscorePrefixed — mixed output, all-underscore yields non-nil empty slice, mid-name underscore not filtered.
  - internal/tmux/tmux_test.go:205 TestListSessions_PortalSaverExcludedAtSource — regression pin: raw output includes _portal-saver between dev and work.
  - internal/tmux/tmux_test.go:242 TestListSessions_PortalSaverExclusionRefactorPin — uses similar prefixes (pigeon, pigeon-saver, _foo, _portal-saver, _portal-bootstrap) to pin exact prefix-match semantics.

CODE QUALITY:
- Project conventions: Followed.
- SOLID: Good — single responsibility; filter is a final post-processing step every caller inherits.
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Good — inline comment names the invariant.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Exclusion is by underscore prefix rather than exact-match on _portal-saver. This is intentional (covers _portal-bootstrap) and pinned by TestListSessions_PortalSaverExclusionRefactorPin.
