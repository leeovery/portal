# Specification: Esc After Preview Hides Session List

## Specification

### Problem Statement

When a user filters the Sessions page, commits the filter, opens the scrollback preview, and dismisses it with `Esc`, the session list visibly disappears and the committed filter text is silently discarded. A second `Esc` is required to restore the list, which then appears unfiltered.

**Expected behaviour:** After `Esc` dismisses the preview, the Sessions page renders the previously filtered list intact — committed filter still applied, matching rows visible, highlighted row preserved.

**Reproduction:**

1. Launch the TUI (`portal open` / `x`) — Sessions page visible.
2. Press `/` to enter filter mode; type until the list narrows.
3. Press `Enter` to commit the filter.
4. Press `Space` to open the scrollback preview for the highlighted session.
5. Press `Esc` — **bug**: list appears empty, filter text gone.
6. Press `Esc` again — list reappears, unfiltered.

**Severity:** Low — UX friction only. No tmux state affected; no data destroyed. User must re-type the filter to recover.

### Scope

**In scope:**

- The blank-list / lost-filter symptom on the preview-dismiss path.
- The same latent symptom on every other code path that routes through `applySessions` — kill-refresh, rename-refresh, externally-killed-during-preview bail.
- Sweep of the remaining `SetItems` discard sites in `internal/tui/model.go` (`Model.WithInsideTmux`, `ProjectsLoadedMsg` handler). These are currently safe because they run before any filter is applied, but the lossy plumbing shape is identical and would break if a filter could be applied at those points in the future. Fixing them in the same pass closes the class of bug.

**Out of scope:**

- Cursor reanchoring under an applied filter on the externally-killed-during-preview branch. `reanchorSessionCursor` runs synchronously in the `previewSessionsRefreshedMsg` handler before the bubbles list's deferred `FilterMatchesMsg` repopulates `filteredItems`, so `VisibleItems()` is empty at reanchor time and the call silently no-ops. After this bugfix, the refilter still completes asynchronously — making reanchor land on a filtered row needs a different mechanism (e.g. stash the target name and reanchor on the refilter-completion tick). Filed as a separate follow-up; narrow surface (only the kill-during-preview-while-filtered path).

---

## Working Notes
