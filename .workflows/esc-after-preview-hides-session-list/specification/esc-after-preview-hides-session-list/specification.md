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

### Root Cause

`applySessions` (`internal/tui/model.go:660-668`) calls `m.sessionList.SetItems(ToListItems(filtered))` and discards the `tea.Cmd` that `bubbles/list` returns.

`bubbles/list.SetItems` (`bubbles@v1.0.0/list.go:385-397`) has a two-phase contract when the list's `filterState != Unfiltered`:

1. **Synchronously** nils `m.filteredItems` — the next render shows zero visible items.
2. **Returns a `filterItems` cmd** that asynchronously runs the filter against the new items and emits `FilterMatchesMsg` (`bubbles@v1.0.0/list.go:1260-1284`). The list's own `Update` consumes this message (`bubbles@v1.0.0/list.go:833-835`) to repopulate `filteredItems`.

When `applySessions` drops the returned cmd, the `FilterMatchesMsg` never fires; `filteredItems` stays nil; the list renders empty while `filterState` is still `FilterApplied`. A second `Esc` is then routed by the list's `handleBrowsing` path to `KeyMap.ClearFilter` (`bubbles@v1.0.0/list.go:864-867`), which calls `resetFiltering()` — clearing the committed filter text and flipping back to `Unfiltered`. The list re-renders with all items; the committed filter is permanently lost.

**Execution path on the buggy Esc:**

1. `internal/tui/pagepreview.go:467-468` — `tea.KeyEsc` in `previewModel.Update` returns `previewDismissedMsg{}`.
2. `internal/tui/model.go:954-974` — top-level Update receives `previewDismissedMsg`, captures `m.preview.session`, calls `m.exitPreviewToSessions(captured)`.
3. `internal/tui/model.go:743-747` — `exitPreviewToSessions` sets `m.activePage = PageSessions`, zeros `m.preview`, returns the `refreshSessionsAfterPreviewCmd` `tea.Cmd`.
4. Refresh cmd resolves → emits `previewSessionsRefreshedMsg`.
5. `internal/tui/model.go:1011-1023` — handler calls `m.applySessions(msg.Sessions)`, then `m.reanchorSessionCursor(msg.PreserveName)`, returns `(m, nil)`.
6. `internal/tui/model.go:660-668` — `applySessions` calls `m.sessionList.SetItems(...)` and **discards the returned cmd**.

The preview-dismiss path is the most prominently affected because `previewSessionsRefreshedMsg` always fires after a `Space` keystroke on the Sessions page, where a filter may be applied. The same `applySessions` call site is reached from `killAndRefresh` (`model.go:1517-1525`), `renameAndRefresh` (`model.go:1571-1579`), and the `previewAttachBailMsg` handler (`model.go:975-993`) — all of which can run while a filter is applied. Those paths share the same blank-list / lost-filter outcome.

`Model.WithInsideTmux` (`model.go:403-411`) and the `ProjectsLoadedMsg` handler (`model.go:936-947`) also call `SetItems` and discard the cmd. They are currently safe because they run before any filter is applied, but the lossy plumbing shape is identical.

### Why It Wasn't Caught

- `TestPreviewEscFilterStatePreservedAcrossDismissWithRefresh` (`internal/tui/pagepreview_refetch_test.go:270-301`) exercises the exact buggy sequence (filter + Space + Esc with a wired `SessionLister` driving the refresh) but only asserts `FilterState`, `FilterValue`, and `IsFiltered`. None of those probe `filteredItems`. A single `VisibleItems()` assertion would have caught the bug — wrong axis was checked, not a missing test.
- `TestPreviewEscPreservesCommittedFilter` (`internal/tui/pagepreview_dismiss_test.go:121-151`) uses `pressSpaceThenEsc` which discards the refresh cmd (`updated3, _ := got2.Update(msg)` at line 41), and `modelWithSeams` does not wire a `SessionLister` — so the test never reaches `previewSessionsRefreshedMsg` / `applySessions`. False sense of coverage.
- Bubble Tea's "returns a cmd you must forward" pattern is easy to miss for void-returning helpers — no compile-time signal.

### Fix Approach

Forward the `tea.Cmd` returned by `m.sessionList.SetItems(...)` out of every call site in `internal/tui/model.go` that currently discards it.

**Primary change — `applySessions`:**

- Change the signature from `func (m *Model) applySessions(sessions []tmux.Session)` to `func (m *Model) applySessions(sessions []tmux.Session) tea.Cmd`.
- Return whatever `m.sessionList.SetItems(...)` returns.
- Update both call sites to propagate the cmd:
  - **`SessionsMsg` handler** (`internal/tui/model.go:893-918`) — batch the returned cmd into whatever the handler already returns. The cmd is `nil` at boot time, so the boot path is functionally unchanged. On `killAndRefresh` / `renameAndRefresh` round-trips, the cmd carries the deferred refilter.
  - **`previewSessionsRefreshedMsg` handler** (`internal/tui/model.go:1011-1023`) — return the cmd directly (handler currently returns `nil`).

**Secondary sweep — other `SetItems` discard sites:**

- **`Model.WithInsideTmux`** (`internal/tui/model.go:403-411`) — propagate the returned cmd out of the constructor-style helper. Since `WithInsideTmux` is currently a chained builder returning `*Model`, this change requires returning `(*Model, tea.Cmd)` (or analogous) and updating the call site to batch the cmd. Currently safe (construction-time, no filter possible), but the lossy shape is fixed for consistency and future-safety.
- **`ProjectsLoadedMsg` handler** (`internal/tui/model.go:936-947`) — call site updates the *projects* list, not the sessions list. Apply the same propagation: capture the cmd from the `SetItems` call and batch/return it from the handler. Currently safe (handler runs before any project filter can be committed), but treated the same way.

**Mechanism:** When `SetItems` is called against a `FilterApplied` list, the propagated `filterItems` cmd actually fires; the list's `FilterMatchesMsg` consumer repopulates `filteredItems`; the Sessions (or Projects) page renders the filtered list intact with the previously-highlighted row still visible. When the list is `Unfiltered`, `SetItems` returns `nil` and behaviour is unchanged.

Result: the preview-dismiss path, the kill-refresh path, the rename-refresh path, and the externally-killed-during-preview bail path all preserve their committed filter across the round-trip via a single point change in `applySessions`. The secondary sweep eliminates the same lossy shape from the remaining call sites.

### Alternatives Considered (Rejected)

1. **Clear the committed filter on preview-dismiss.** Defeats the existing documented intent (`TestPreviewEscPreservesCommittedFilter` and the `previewDismissedMsg` handler explicitly preserve filter state byte-identically). Users committed for a reason.
2. **Intercept `FilterMatchesMsg` / re-route the filter pipeline.** Overbuilt. The library already does the right thing if its cmd is propagated.

### Risk

- **Fix complexity:** Low — signature change in one helper, plus mechanical cmd propagation at the call sites.
- **Regression risk:** Low — `SetItems` returns `nil` when filter state is `Unfiltered`, so all currently-unfiltered call paths are functionally unchanged. The change is strictly more correct, never less.
- **Release:** Regular release, single PR. No feature flag, no hotfix urgency (UX friction only).

### Test Coverage

**Lock in the fix at the wrong-axis miss site:**

- `TestPreviewEscFilterStatePreservedAcrossDismissWithRefresh` (`internal/tui/pagepreview_refetch_test.go:270-301`) already exercises the exact buggy sequence (filter applied + `Space` + `Esc` with a wired `SessionLister` driving the refresh) but only asserts `FilterState`, `FilterValue`, and `IsFiltered`. **Add a `VisibleItems()` assertion** — use `visibleSessionNames(got)` (or equivalent helper already in the test package) and assert equality with the expected filtered slice. This is the single assertion that would have caught the bug and is what prevents the same wrong-axis miss recurring.

**Cover the latent variant:**

- Add a test in the kill-refresh flow that:
  1. Applies a committed filter to the Sessions page.
  2. Triggers the `x` kill-confirm modal flow against a filtered row.
  3. Asserts the Sessions page still renders filtered items (via `VisibleItems()` / `visibleSessionNames`) after the resulting `SessionsMsg` round-trip.

  Codifies the latent-variant coverage; ensures `killAndRefresh` going through `applySessions` retains the filter.

**Existing test left unchanged:**

- `TestPreviewEscPreservesCommittedFilter` (`internal/tui/pagepreview_dismiss_test.go:121-151`) correctly asserts filter retention; it just never reached `applySessions` (no wired `SessionLister`, and `pressSpaceThenEsc` discards the refresh cmd). Its assertions are still correct — no change needed. The new `VisibleItems()` assertion in `TestPreviewEscFilterStatePreservedAcrossDismissWithRefresh` is the one that exercises the fix end-to-end.

### Acceptance Criteria

1. **Primary symptom resolved:** On the documented reproduction path (filter → commit → Space → Esc), the Sessions page renders the filtered list intact after the single `Esc` keystroke. The committed filter text is preserved; matching rows remain visible; the previously-highlighted row remains the cursor.
2. **Latent variants resolved:** Killing a session via `x` while a filter is applied leaves the filtered list rendered after the refresh. Renaming a session via `r` while a filter is applied leaves the filtered list rendered after the refresh. Externally-killed-during-preview bail (`previewAttachBailMsg`) leaves the filtered list rendered after the refresh.
3. **Boot path unchanged:** Initial Sessions/Projects load (no filter applied) renders identically to before — `SetItems` returns `nil` in the unfiltered case, so the propagated cmd is a no-op.
4. **`applySessions` returns the `SetItems` cmd:** Signature is `func (m *Model) applySessions(sessions []tmux.Session) tea.Cmd`; both call sites batch/return the returned cmd.
5. **Secondary sweep applied:** `Model.WithInsideTmux` and the `ProjectsLoadedMsg` handler no longer discard the cmd returned by `SetItems`. Their lossy shape is removed.
6. **Test additions:** `TestPreviewEscFilterStatePreservedAcrossDismissWithRefresh` includes a `VisibleItems()` assertion. A new test covers the kill-refresh-under-filter scenario.
7. **No regressions in existing TUI tests:** `go test ./internal/tui/...` passes.

---

## Working Notes
