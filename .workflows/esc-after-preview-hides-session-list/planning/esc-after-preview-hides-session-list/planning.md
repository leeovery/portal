# Plan: Esc After Preview Hides Session List

## Phases

### Phase 1: Forward SetItems cmd through applySessions and sweep sibling discard sites
status: draft

**Goal**: Eliminate the blank-list / lost-filter symptom across every code path that mutates the sessions or projects `bubbles/list` while a committed filter may be applied, by propagating the `tea.Cmd` returned by `SetItems` out of all current discard sites, and lock the fix in with targeted test additions.

**Why this order**: Single root cause with a contained fix surface; no precursor refactor or staged rollout is warranted. Per the bugfix phase-design guidance, reproduce-then-fix-then-regression-test belongs in one phase when the cause is singular and the change is surgical. There is no intermediate state that has independent value, and splitting the secondary sweep or the test additions into a separate phase would create a trivial phase with no meaningful checkpoint.

**Acceptance**:
- [ ] On the documented reproduction path (filter → commit → `Space` → `Esc`), the Sessions page renders the filtered list intact after a single `Esc` keystroke: committed filter text preserved, matching rows visible, previously-highlighted row remains the cursor.
- [ ] Killing a session via `x` while a filter is applied leaves the filtered list rendered after the refresh; renaming via `r` while filtered leaves the filtered list rendered; the externally-killed-during-preview bail (`previewAttachBailMsg`) leaves the filtered list rendered.
- [ ] `applySessions` signature is `func (m *Model) applySessions(sessions []tmux.Session) tea.Cmd`; both call sites (`SessionsMsg` handler at `internal/tui/model.go:893-918`, `previewSessionsRefreshedMsg` handler at `internal/tui/model.go:1011-1023`) propagate the returned cmd.
- [ ] `Model.WithInsideTmux` (`internal/tui/model.go:403-411`) and the `ProjectsLoadedMsg` handler (`internal/tui/model.go:936-947`) no longer silently discard the `SetItems` cmd; `WithInsideTmux` locks in its always-nil invariant explicitly (panic-on-non-nil or commented capture).
- [ ] Sibling `bubbles/list` mutators (`SetItem`, `InsertItem`, `RemoveItem`) on `m.sessionList` and `m.projectList` are audited; any discard sites found are fixed the same way; audit outcome (sites checked + result) is recorded in the PR description.
- [ ] `TestPreviewEscFilterStatePreservedAcrossDismissWithRefresh` (`internal/tui/pagepreview_refetch_test.go:270-301`) gains a `VisibleItems()` / `visibleSessionNames` slice-equality assertion and a cursor-index assertion; the `pressSpaceThenEscWithRefresh` helper is extended to drain the propagated `filterItems` cmd and feed the resulting `FilterMatchesMsg` back through `Update` before assertions run.
- [ ] A new test in the kill-refresh flow exercises the filter-applied → `x` → confirm sequence via real keystrokes with wired `SessionKiller` and `SessionLister` seams, asserting `visibleSessionNames` slice equality against the expected filtered post-kill slice.
- [ ] Boot path unchanged — initial unfiltered Sessions/Projects load renders identically to before (`SetItems` returns `nil` when filter state is `Unfiltered`; propagated cmd is a no-op).
- [ ] `go test ./internal/tui/...` passes; no regressions in the wider suite.
