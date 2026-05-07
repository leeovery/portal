---
status: in-progress
created: 2026-05-06
cycle: 5
phase: Plan Integrity Review
topic: session-scrollback-preview
---

# Review Tracking: session-scrollback-preview - Integrity

Re-read planning.md plus all four phase task detail files end-to-end after cycle 4's handler-name fix (`updateSessionsPage` → `updateSessionList`). Verified cycle 4 fix landed cleanly: zero `updateSessionsPage` references remain in current task files (only historical references in earlier tracking files — unchanged by design). Cross-checked the actual `internal/tui/model.go` to confirm:

- `updateSessionList` is the live Sessions-page key handler (line 796 routing, line 1107 selected-item access).
- `PageLoading`, `PageSessions`, `PageProjects` are exported page constants; `pageFileBrowser` is unexported (lines 22–31).
- `sessionList list.Model` (line 132) and `activePage page` (line 145) are the live Model field names.
- `state.SanitizePaneKey(session, window, pane int) string` (panekey.go:27) and `state.ScrollbackFile(dir, paneKey string) string` (paths.go:92) match plan signatures.
- `*tmux.Client.ListPanesInSession` (tmux.go:385) is the existing pattern referenced in Phase 1-5.
- `bubbles/viewport.Model` exposes `SetContent`, `GotoBottom`, `AtBottom`, direct `Width`/`Height`/`YOffset` fields — all referenced correctly in plan.
- `bubbles/list.Model` exposes `SettingFilter`, `IsFiltered`, `FilterValue`, `Index`, `Items`, `SelectedItem` — all referenced correctly in plan.

Cycles 1–4's prior fixes (method rename, filename, key constant, planning.md table-row, model field names, handler name) all remain applied with no regression.

**Overall assessment**: The plan is implementation-ready on every architectural dimension. Phase ordering, vertical slicing, dependency edges, AC quality, scope/granularity, and self-containment are sound. Cycle 5 (the safety-cap cycle) re-scans for any remaining symbol-name drifts of the same shape cycles 1–4 fixed. One Minor consistency drift surfaces: lowercase `pageSessions` is used as a code-bearing identifier in a Phase 4 test description and in two related narrative sites where an implementer would read it as the actual page constant (which is `PageSessions`, exported). This is the same pattern as cycle 4's `updateSessionsPage` finding — a name-translation drift inherited from spec-level narrative shorthand. The narrative-arrow form (`pagePreview → pageSessions` describing a transition) is defensible convention, but a backticked test-assertion identifier should match the live symbol.

No other findings. Phases 1, 2, 3 are clean.

## Findings

### 1. Lowercase `pageSessions` in code-bearing test description does not match exported `PageSessions` constant

**Severity**: Minor
**Plan Reference**: Phase 4 Task 4-6 (Tests entry — last bullet)
**Category**: Task Self-Containment
**Change Type**: update-task

**Details**:
Phase 4 Task 4-6's final Tests entry asserts `transition to pageSessions succeeds`. The actual page constant in `internal/tui/model.go` is `PageSessions` (exported, line 26). An implementer writing the test would either grep for `pageSessions` and find nothing in the new preview surface (only narrative usage in plan + spec), then translate to `PageSessions` after a moment of confusion — same name-translation friction cycles 1–4 fixed.

The other lowercase usages in the plan are narrative transition arrows describing the page transition itself (`"pagePreview → pageSessions transition"` — a convention inherited verbatim from the spec at line 313). These narrative-arrow forms in `planning.md` lines 118 and 134, in `phase-4-tasks.md` lines 207, 208, 209, 218, 220, 230, 249, 257, are defensible as convention — they describe the abstract transition, not a code symbol, and they pair naturally with `pagePreview` (a new lowercase constant peer of `pageFileBrowser`). Modifying those would over-correct against a long-established narrative shorthand.

The single drift worth fixing is the test-assertion at Task 4-6 (line 297), where `pageSessions` is in backticks alongside other code-bearing identifiers (`Esc`, `(nil, nil)`) and the test description reads as a runnable assertion. This is the only line where the name-translation drift bites the implementer.

Phase 4 Task 4-5 contains a structurally similar test phrasing at multiple Tests entries (lines 240–245), but those use different formulations ("after `Esc`, list reflects second call", "rendered Sessions list", "cursor is on `A` post-dismiss") — none of which conflict with the live constant. The single concrete drift is at 4-6's last Tests bullet.

**Current** (Phase 4 Task 4-6, Tests — last bullet, line 297):
```markdown
- `"Esc dismisses cleanly from a fully-degraded preview"` — after all panes return `(nil, nil)`, send `Esc`; assert transition to `pageSessions` succeeds.
```

**Proposed** (Phase 4 Task 4-6, Tests — last bullet):
```markdown
- `"Esc dismisses cleanly from a fully-degraded preview"` — after all panes return `(nil, nil)`, send `Esc`; assert transition to `PageSessions` succeeds.
```

**Resolution**: Pending
**Notes**: Mechanical alignment with the actual `internal/tui/model.go` page-constant casing. No spec content changes; no acceptance criterion changes meaning; no architectural impact. Same category and severity as cycle 4 finding. The narrative transition-arrow form (`pagePreview → pageSessions`) appearing elsewhere in the plan is a defensible spec-inherited convention and is intentionally left unchanged — it describes the abstract transition, not the live constant. Only the backticked code-identifier in Task 4-6's Tests bullet drifts in a way that misleads an implementer.
