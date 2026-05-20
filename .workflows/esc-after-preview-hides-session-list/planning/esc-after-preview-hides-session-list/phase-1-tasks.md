---
phase: 1
phase_name: Forward SetItems cmd through applySessions and sweep sibling discard sites
total: 6
---

## esc-after-preview-hides-session-list-1-1 | approved

### Task esc-after-preview-hides-session-list-1-1: Extend test harness to drain propagated refilter cmd

**Problem**: The fix mechanism is asynchronous — `SetItems` against a `FilterApplied` list synchronously nils `filteredItems` and returns a `filterItems` `tea.Cmd` that emits `FilterMatchesMsg`; only after that message is round-tripped through `Update` does `VisibleItems()` return the refiltered slice. The existing helper `pressSpaceThenEscWithRefresh` (`internal/tui/pagepreview_refetch_test.go:76-112`) discards the cmd returned by the refresh-message `Update` call (`updated4, _ := got3.Update(refreshMsg)` at line 106). Without harness extension, the prescribed `visibleSessionNames` assertion in task 1-2 will fail against a correctly-fixed implementation.

**Solution**: Extend `pressSpaceThenEscWithRefresh` (and add an equivalent drain pattern reusable by the new kill-refresh test in task 1-4) to capture the cmd returned by the `Update` call that processes `previewSessionsRefreshedMsg` / `SessionsMsg`, invoke it to obtain the `FilterMatchesMsg`, and feed that message back through `Update` so `filteredItems` is repopulated before assertions run.

**Outcome**: A test-package-level helper exists that performs the full refilter round-trip; after the helper returns, calling `VisibleItems()` on the returned model yields the refiltered slice rather than nil. The helper is a no-op when the returned cmd is `nil` (unfiltered list) so it does not perturb boot-path tests.

**Do**:
- Open `internal/tui/pagepreview_refetch_test.go` and locate `pressSpaceThenEscWithRefresh` at lines 76-112.
- At line 106 where `updated4, _ := got3.Update(refreshMsg)` discards the cmd, capture the cmd instead (e.g. `updated4, refilterCmd := got3.Update(refreshMsg)`).
- If `refilterCmd != nil`, invoke it (`msg := refilterCmd()`) and feed the resulting message back through `Update` on `updated4` to produce the final model; return that model.
- If `refilterCmd == nil`, return `updated4` unchanged — preserves boot-path / unfiltered behaviour.
- Factor the drain step into a small package-private helper (e.g. `drainRefilterCmd(m tea.Model, cmd tea.Cmd) tea.Model`) so the new kill-refresh test in task 1-4 can reuse it without duplicating the drain logic. Place this helper next to the existing test helpers in the `internal/tui` test package.
- Do not change call signatures that other tests depend on unless required; if a signature change is required, update all in-package callers in the same edit.

**Acceptance Criteria**:
- [ ] `pressSpaceThenEscWithRefresh` captures and invokes the cmd returned by the refresh `Update` call, then feeds the `FilterMatchesMsg` back through `Update` before returning.
- [ ] A reusable `drainRefilterCmd` (or equivalent) helper exists at the test-package level for reuse by the kill-refresh test.
- [ ] When the captured cmd is `nil`, the helper returns the model unchanged (boot/unfiltered path).
- [ ] `go test ./internal/tui/...` still passes (red on the augmented test in task 1-2 is expected and lands in that task, not this one — existing assertions in `TestPreviewEscFilterStatePreservedAcrossDismissWithRefresh` continue to pass because they don't yet probe `filteredItems`).

**Tests**:
- `"drainRefilterCmd returns model unchanged when cmd is nil"` — exercises the boot/unfiltered guard.
- `"pressSpaceThenEscWithRefresh round-trips FilterMatchesMsg through Update when filter is applied"` — verified indirectly by the existing assertions in `TestPreviewEscFilterStatePreservedAcrossDismissWithRefresh` continuing to pass after the helper change.

**Edge Cases**:
- Helper must return unchanged when no `SessionLister` is wired (the refresh cmd path doesn't emit a `previewSessionsRefreshedMsg` and the captured cmd is `nil`).
- Filter cmd is `nil` when the list is `Unfiltered` — `SetItems` returns `nil` in that case per `bubbles@v1.0.0/list.go:385-397`. The helper must handle `nil` without panicking.

**Context**:
> From spec "Test harness must drain the propagated refilter cmd" section: "Extend the helper (and any analogous helper used by the new kill-refresh test) to: (1) Capture the cmd returned by the Update call that processes previewSessionsRefreshedMsg / SessionsMsg. (2) Invoke the cmd to obtain its tea.Msg (the FilterMatchesMsg emitted by filterItems). (3) Feed that message back through the model's Update. After this second Update call, VisibleItems() returns the refiltered slice."
>
> "Update the helper at the test-package level — do not duplicate the drain logic per-test."

**Spec Reference**: `.workflows/esc-after-preview-hides-session-list/specification/esc-after-preview-hides-session-list/specification.md` (Test Coverage → "Test harness must drain the propagated refilter cmd")

---

## esc-after-preview-hides-session-list-1-2 | approved

### Task esc-after-preview-hides-session-list-1-2: Add VisibleItems and cursor-index assertions to existing preview-Esc filter test

**Problem**: `TestPreviewEscFilterStatePreservedAcrossDismissWithRefresh` (`internal/tui/pagepreview_refetch_test.go:270-301`) exercises the exact buggy filter + `Space` + `Esc` sequence with a wired `SessionLister` but only asserts `FilterState`, `FilterValue`, and `IsFiltered` — wrong-axis assertions that never probe `filteredItems`. A single `VisibleItems()` assertion would have caught the original bug. Without adding the missing axis, the same wrong-axis miss could recur on future regressions.

**Solution**: Add two assertions to the test: (a) a `visibleSessionNames(got)` slice-equality check against the expected filtered slice, and (b) a cursor-index assertion verifying `got.sessionList.Index()` still points at the previously-highlighted row. The augmented test runs red until task 1-3 lands the fix, then runs green — locking in the regression coverage.

**Outcome**: The test asserts on `filteredItems` (via `visibleSessionNames`) and on the bubbles list selected index. The test is red against the current code (pre-fix) and green after task 1-3 lands.

**Do**:
- Open `internal/tui/pagepreview_refetch_test.go` and locate `TestPreviewEscFilterStatePreservedAcrossDismissWithRefresh` at lines 270-301.
- After the existing `FilterState` / `FilterValue` / `IsFiltered` assertions, add a `visibleSessionNames(got)` (or equivalent helper already present in the test package) slice-equality assertion against the expected filtered session name slice. Use `reflect.DeepEqual` / explicit slice compare — not length-only.
- Add a cursor-index assertion: capture the bubbles list `Index()` before `Space` is pressed (or compute the expected index from the filtered slice), then assert `got.sessionList.Index()` equals that value after dismiss + refresh + refilter round-trip. If the test fixture does not currently capture the pre-Space cursor position, add the capture at the appropriate point in the existing flow.
- Confirm the test now relies on the extended `pressSpaceThenEscWithRefresh` from task 1-1 — the helper's drain step is what makes `VisibleItems()` populated at assertion time.
- Run the test — it must be red against the unfixed code (`VisibleItems()` returns nil; assertion fails on empty vs expected filtered slice). Confirm red before moving on.

**Acceptance Criteria**:
- [ ] Test gains a `visibleSessionNames(got)` slice-equality assertion against the expected filtered slice (not length-only).
- [ ] Test gains a cursor-index assertion against `got.sessionList.Index()` pointing at the previously-highlighted row.
- [ ] Test is red against pre-fix code (run before task 1-3) — failure mode is empty `VisibleItems()` slice vs expected non-empty filtered slice.
- [ ] Existing `FilterState` / `FilterValue` / `IsFiltered` assertions remain in place unchanged.

**Tests**:
- `"TestPreviewEscFilterStatePreservedAcrossDismissWithRefresh asserts VisibleItems equals filtered slice"` — the augmented assertion itself.
- `"TestPreviewEscFilterStatePreservedAcrossDismissWithRefresh asserts cursor index preserved across dismiss"` — the augmented cursor assertion.

**Edge Cases**:
- Cursor preservation across asynchronous refilter round-trip — the assertion must run after the drain step in `pressSpaceThenEscWithRefresh`, otherwise `reanchorSessionCursor` may have early-returned on empty `VisibleItems()` and the cursor may sit at a different index than expected.
- Slice-equality (order-sensitive) is mandatory — length-only would let row-substitution regressions pass silently.

**Context**:
> From spec Test Coverage → "Lock in the fix at the wrong-axis miss site": "Add two assertions: A `VisibleItems()` assertion — use `visibleSessionNames(got)` (or equivalent helper already in the test package) and assert equality with the expected filtered slice. This is the single assertion that would have caught the original bug and is what prevents the same wrong-axis miss recurring. A cursor-index assertion — assert the bubbles list's selected index (`got.sessionList.Index()` or via an existing helper) points at the previously-highlighted row. This locks in AC #1's cursor-preservation clause; without it, a future handler reordering or library behaviour shift could regress cursor preservation silently."

**Spec Reference**: `.workflows/esc-after-preview-hides-session-list/specification/esc-after-preview-hides-session-list/specification.md` (Test Coverage → "Lock in the fix at the wrong-axis miss site"; Acceptance Criteria #1, #6)

---

## esc-after-preview-hides-session-list-1-3 | approved

### Task esc-after-preview-hides-session-list-1-3: Change applySessions signature to return tea.Cmd and propagate at both call sites

**Problem**: `applySessions` (`internal/tui/model.go:660-668`) calls `m.sessionList.SetItems(ToListItems(filtered))` and discards the `tea.Cmd` returned by `bubbles/list`. When the list is `FilterApplied`, `SetItems` synchronously nils `filteredItems` and returns a `filterItems` cmd that asynchronously emits `FilterMatchesMsg`; dropping that cmd leaves `filteredItems` nil indefinitely, the list renders empty, and a second `Esc` is routed to `KeyMap.ClearFilter` (`bubbles@v1.0.0/list.go:864-867`) which silently discards the committed filter. This is the root cause of the preview-dismiss blank-list symptom and the latent kill-refresh / rename-refresh / preview-attach-bail variants.

**Solution**: Change `applySessions` from `func (m *Model) applySessions(sessions []tmux.Session)` to `func (m *Model) applySessions(sessions []tmux.Session) tea.Cmd`. Return the cmd from `SetItems`. Update both call sites — the `SessionsMsg` handler (`internal/tui/model.go:893-918`) and the `previewSessionsRefreshedMsg` handler (`internal/tui/model.go:1011-1023`) — to propagate the cmd. The `previewAttachBailMsg` path is covered transitively because it reaches `applySessions` via `exitPreviewToSessions` → `refreshSessionsAfterPreviewCmd` → `previewSessionsRefreshedMsg`.

**Outcome**: After Esc dismisses the preview against a filtered list, the propagated `filterItems` cmd runs, the bubbles list consumes its `FilterMatchesMsg` and repopulates `filteredItems`, and the Sessions page re-renders the filtered list with the previously-highlighted row still selected. The same fix applies to `killAndRefresh`, `renameAndRefresh`, and the `previewAttachBailMsg` bail path. The augmented test from task 1-2 turns green.

**Do**:
- Open `internal/tui/model.go`. At lines 660-668, change the signature of `applySessions` to return `tea.Cmd`. Capture the cmd from `m.sessionList.SetItems(ToListItems(filtered))` and return it as the function's final return value. Preserve all other side effects (cursor handling, etc.) unchanged.
- At the `SessionsMsg` handler (lines 893-918), update the two branches that currently call `applySessions` and return `nil` to instead capture the cmd and return it directly. Per spec implementation notes, no `tea.Batch` is needed — neither branch combines the returned cmd with another at the return point today.
- At the `previewSessionsRefreshedMsg` handler (lines 1011-1023), capture the cmd from `m.applySessions(msg.Sessions)`, leave the `m.reanchorSessionCursor(msg.PreserveName)` call in place after the `applySessions` call, and return `(m, cmd)` instead of `(m, nil)`.
- Do not touch the `previewAttachBailMsg` handler (`internal/tui/model.go:975-993`) — it reaches `applySessions` transitively and is covered by the `previewSessionsRefreshedMsg` site fix.
- Run `go build ./...` to verify the signature change compiles. Run the augmented test from task 1-2 — it must now be green.

**Acceptance Criteria**:
- [ ] `applySessions` signature is `func (m *Model) applySessions(sessions []tmux.Session) tea.Cmd`.
- [ ] `applySessions` returns the `tea.Cmd` from `m.sessionList.SetItems(...)`.
- [ ] `SessionsMsg` handler branches that previously returned `nil` after `applySessions` now propagate the captured cmd.
- [ ] `previewSessionsRefreshedMsg` handler returns the captured cmd instead of `nil`.
- [ ] `previewAttachBailMsg` handler is left unchanged (covered transitively).
- [ ] Augmented test from task 1-2 (`TestPreviewEscFilterStatePreservedAcrossDismissWithRefresh` with `VisibleItems` + cursor-index assertions) passes green.
- [ ] `go test ./internal/tui/...` passes; no regressions.

**Tests**:
- `"TestPreviewEscFilterStatePreservedAcrossDismissWithRefresh"` (augmented in task 1-2) turns green.
- `"existing TUI tests still pass"` — full `./internal/tui/...` suite green.

**Edge Cases**:
- `SetItems` returns `nil` when the list is `Unfiltered` — boot path (first `SessionsMsg` after `tea.NewProgram(m).Run()`) propagates a `nil` cmd, identical to current behaviour. AC #3 (boot path unchanged).
- `previewAttachBailMsg` reaches `applySessions` transitively via `exitPreviewToSessions` → `refreshSessionsAfterPreviewCmd` → `previewSessionsRefreshedMsg`; covered by the `previewSessionsRefreshedMsg` site fix without a separate code change.
- No `tea.Batch` is required at either call site — both currently return a single cmd (or `nil`) at the return point.

**Context**:
> From spec Fix Approach → "Primary change — applySessions": "Change the signature from `func (m *Model) applySessions(sessions []tmux.Session)` to `func (m *Model) applySessions(sessions []tmux.Session) tea.Cmd`. Return whatever `m.sessionList.SetItems(...)` returns. Update both call sites to propagate the cmd: SessionsMsg handler (internal/tui/model.go:893-918) — batch the returned cmd into whatever the handler already returns. The cmd is nil at boot time, so the boot path is functionally unchanged. On killAndRefresh / renameAndRefresh round-trips, the cmd carries the deferred refilter. previewSessionsRefreshedMsg handler (internal/tui/model.go:1011-1023) — return the cmd directly (handler currently returns nil)."
>
> From Implementation Notes: "The current SessionsMsg handler (internal/tui/model.go:893-915) returns nil on both branches it can reach after applySessions. After the change, return the propagated cmd directly on the post-applySessions branches — no tea.Batch is needed at this site."
>
> "The previewAttachBailMsg handler (internal/tui/model.go:975-993) reaches applySessions transitively via exitPreviewToSessions → refreshSessionsAfterPreviewCmd → previewSessionsRefreshedMsg. The bail path is therefore covered by the previewSessionsRefreshedMsg call-site fix — no separate change is required at the bail handler itself."

**Spec Reference**: `.workflows/esc-after-preview-hides-session-list/specification/esc-after-preview-hides-session-list/specification.md` (Root Cause; Fix Approach → "Primary change"; Acceptance Criteria #1, #2, #3, #4)

---

## esc-after-preview-hides-session-list-1-4 | approved

### Task esc-after-preview-hides-session-list-1-4: Add kill-refresh-under-filter regression test

**Problem**: The latent variant in which `killAndRefresh` (`internal/tui/model.go:1517-1525`) reaches `applySessions` via the production `SessionsMsg` path has no test coverage. After task 1-3 lands the fix, a regression that re-introduces the discard at that site (or in any future refactor that bypasses the `applySessions` propagation) would not be caught without a dedicated test. The spec scopes test work to a single representative latent-variant test rather than one-per-variant; the kill-refresh path is the canonical case.

**Solution**: Add a test that applies a committed filter on the Sessions page, drives the `x` kill-confirm modal via real keystrokes (no hand-crafted `SessionsMsg`), wires `SessionKiller` and `SessionLister` seams following existing kill-test patterns in the package, and asserts `visibleSessionNames(got)` slice-equality against the expected post-kill filtered slice.

**Outcome**: A new test in the kill-refresh flow exercises the filter-applied → `x` → confirm sequence end-to-end and asserts the filtered list is rendered intact after the refresh (with the killed row absent). The test depends on the `drainRefilterCmd` helper from task 1-1 to round-trip the propagated refilter cmd before assertion.

**Do**:
- Locate the existing kill-test pattern in `internal/tui/` — identify the test file housing kill-confirm modal tests and the seam-wiring helpers (`SessionKiller`, `SessionLister`) used there. Follow the same `modelWithSeams`-style construction.
- Add a new test (e.g. `TestKillRefreshUnderFilterPreservesFilteredList`) that:
  1. Constructs a model with a `SessionLister` that returns an initial multi-session slice and a `SessionKiller` that succeeds.
  2. Applies a committed filter using the same filter-commit drive used by `TestPreviewEscFilterStatePreservedAcrossDismissWithRefresh` (mirror that fixture).
  3. Drives `x` to open the kill-confirm modal, then the confirm key (look up the exact confirm keystroke used in the existing kill tests in the package — do not invent one).
  4. Rewires the `SessionLister` to return the post-kill slice (initial minus the killed session) before the refresh fires, mirroring existing kill-test patterns.
  5. Uses `drainRefilterCmd` (from task 1-1) to round-trip the propagated `filterItems` cmd after the `SessionsMsg` is processed.
  6. Asserts `visibleSessionNames(got)` slice-equality against the expected filtered slice (initial filter applied to the post-kill universe). Length-only assertion is insufficient — use slice equality.
- Do not hand-craft a `SessionsMsg` — the point is to exercise the production `killAndRefresh` → `SessionsMsg` → `applySessions` path through real keystrokes.
- Do not add separate tests for `renameAndRefresh`, `previewAttachBailMsg`, or `ProjectsLoadedMsg` — per the spec, this kill-refresh test is the single representative latent-variant test; reviewers verify the rest by reading the diff.

**Acceptance Criteria**:
- [ ] New test exists in the kill-refresh flow exercising filter-commit → `x` → confirm via real keystrokes.
- [ ] Test wires `SessionKiller` (success) and `SessionLister` (post-kill slice) seams using existing kill-test patterns.
- [ ] Test uses `drainRefilterCmd` (from task 1-1) to round-trip the propagated cmd before assertions.
- [ ] Test asserts `visibleSessionNames(got)` slice-equality against expected filtered post-kill slice (not length-only).
- [ ] Test passes green against the fix from task 1-3; would fail red if the `SessionsMsg` handler's cmd propagation were reverted.
- [ ] No additional tests added for `renameAndRefresh` / `previewAttachBailMsg` / `ProjectsLoadedMsg` variants.

**Tests**:
- `"TestKillRefreshUnderFilterPreservesFilteredList"` — primary test added by this task.
- `"the killed row is absent from the post-refresh visible slice"` — verified by the slice-equality assertion.
- `"the committed filter survives killAndRefresh"` — verified by the same assertion (filter still applied means non-matching unkilled rows still excluded).

**Edge Cases**:
- Real-keystroke path is mandatory — hand-crafting a `SessionsMsg` would short-circuit the production `killAndRefresh` plumbing and fail to exercise the fix at the right call site.
- Killed row must be absent from the post-kill slice returned by the rewired `SessionLister` — otherwise the assertion conflates "kill succeeded" with "filter retained".
- Filter retained through `killAndRefresh` — the test's expected slice is the filter applied to the post-kill universe, not the raw post-kill universe.

**Context**:
> From spec Test Coverage → "Cover the latent variant": "Add a test in the kill-refresh flow that: (1) Applies a committed filter to the Sessions page (mirror the filter-commit drive used by TestPreviewEscFilterStatePreservedAcrossDismissWithRefresh). (2) Drives the full x kill-confirm modal flow via real keystrokes (x to open the confirm modal, then the confirm key as used elsewhere in the package's kill tests) — do not shortcut by hand-crafting a SessionsMsg. The point is to exercise the production message path through killAndRefresh → SessionsMsg → applySessions. (3) Wire a SessionKiller seam that succeeds and a SessionLister seam that returns the post-kill session slice (sans the killed row), following the same mock/seam wiring pattern used by the existing kill tests in the package. (4) Asserts post-refresh state with visibleSessionNames(got) against the expected filtered slice (the same helper used by the augmented preview test). Slice-equality is the assertion; length-only is insufficient."
>
> "Do not add separate tests for each latent variant — the spec deliberately scopes test work to the single representative case; reviewers verify the rest by reading the diff."

**Spec Reference**: `.workflows/esc-after-preview-hides-session-list/specification/esc-after-preview-hides-session-list/specification.md` (Test Coverage → "Cover the latent variant"; "Test scope — one representative latent-variant test is sufficient"; Acceptance Criteria #2, #6)

---

## esc-after-preview-hides-session-list-1-5 | approved

### Task esc-after-preview-hides-session-list-1-5: Sweep WithInsideTmux and ProjectsLoadedMsg SetItems discard sites

**Problem**: `Model.WithInsideTmux` (`internal/tui/model.go:403-411`) and the `ProjectsLoadedMsg` handler (`internal/tui/model.go:936-947`) both call `SetItems` and discard the returned cmd. These sites are currently safe — `WithInsideTmux` runs before any filter can be applied (called before `tea.NewProgram(m).Run()` at `cmd/open.go:360`), and `ProjectsLoadedMsg` fires before the page transitions to `pageProjects` and therefore before any projects-list filter can be committed — but the lossy plumbing shape is identical to the bug fixed in task 1-3. Without sweeping these sites, a future code move (e.g. running `WithInsideTmux` after filter state can exist) would silently re-introduce the same symptom on the projects path.

**Solution**: Apply propagation/lock-in changes at both sites. For `WithInsideTmux`, since it runs before `tea.NewProgram` and has no cmd dispatcher available, lock in the always-nil invariant explicitly: capture the returned cmd and either panic on non-nil (preferred — surfaces breakage immediately if the call site ever moves) or capture-with-comment. Preserve the chained `*Model` return shape. For the `ProjectsLoadedMsg` handler, capture the cmd from the `SetItems` call and return it directly from the handler (currently returns `m, nil`); no `tea.Batch` is needed.

**Outcome**: Both call sites no longer silently discard the `SetItems` cmd. `WithInsideTmux` documents and enforces its safe invariant (panic-on-non-nil or commented capture). The `ProjectsLoadedMsg` handler propagates the cmd through `Update`, shape-consistent with `applySessions`.

**Do**:
- Open `internal/tui/model.go` at lines 403-411 (`Model.WithInsideTmux`). Capture the cmd from the `SetItems` call into a local variable. Per spec, prefer the panic guard: `if cmd := m.sessionList.SetItems(...); cmd != nil { panic("unreachable: WithInsideTmux runs before any filter can be applied") }`. If the panic shape conflicts with surrounding code style, use the commented-discard variant — but include an explicit code comment explaining the always-nil invariant. Do not change the function's return signature; keep the chained `*Model` return shape.
- Open `internal/tui/model.go` at lines 936-947 (`ProjectsLoadedMsg` handler). The handler currently calls `m.projectList.SetItems(...)` and returns `m, nil`. Capture the cmd from the `SetItems` call and return it directly as the handler's second return value. No `tea.Batch` is needed — the handler does not currently combine multiple cmds at the return point.
- Do not add a test for the `ProjectsLoadedMsg` propagation — per the spec, "no production-reachable failure exists to test against, and no test is added for this site — a contrived test would have to construct a state that cannot occur in production."
- Do not refactor either site beyond the cmd propagation / lock-in changes.

**Acceptance Criteria**:
- [ ] `WithInsideTmux` captures the cmd returned by `SetItems` and explicitly locks in the always-nil invariant (panic-on-non-nil or commented capture with explanatory comment).
- [ ] `WithInsideTmux` keeps its existing return signature (chained `*Model`) — no constructor signature change.
- [ ] `ProjectsLoadedMsg` handler captures the cmd from the `SetItems` call and returns it as the handler's second return value instead of `nil`.
- [ ] No tea.Batch is used at either site.
- [ ] No test added for the `ProjectsLoadedMsg` propagation (spec explicitly scopes test work to the kill-refresh case).
- [ ] `go test ./internal/tui/...` passes; no regressions.

**Tests**:
- No new tests for this task (per spec). Coverage relies on diff review and existing `./internal/tui/...` suite continuing to pass.

**Edge Cases**:
- `WithInsideTmux` runs before `tea.NewProgram(m).Run()` (called from `cmd/open.go:360`) — no cmd dispatcher exists at this point. The panic guard exists to surface a regression immediately if the call site is ever moved to a context where a filter can be applied.
- `ProjectsLoadedMsg` fires before the page transitions to `pageProjects`; the projects-list filter is never applied at this point in production. Propagation is shape-consistency only.

**Context**:
> From spec Fix Approach → "Secondary sweep — other SetItems discard sites": "Model.WithInsideTmux (internal/tui/model.go:403-411) — WithInsideTmux is called before tea.NewProgram(m).Run() at TUI construction time (cmd/open.go:360), so there is no tea.Cmd dispatcher available to batch into. At this point the session list is empty and no filter can be applied, so SetItems returns nil unconditionally. Keep the chained *Model return shape, but capture the returned cmd locally and assert/comment that it is always nil at this site (e.g. `if cmd := m.sessionList.SetItems(...); cmd != nil { panic("unreachable: WithInsideTmux runs before any filter can be applied") }`, or a quieter discard-with-comment variant). The intent is to lock in the safe invariant without rewiring the constructor signature; if the call site ever moves to a point where a filter can be applied, the panic surfaces the breakage immediately."
>
> "ProjectsLoadedMsg handler (internal/tui/model.go:936-947) — call site updates the projects list, not the sessions list. Apply the same propagation: capture the cmd from the SetItems call and batch/return it from the handler. Currently safe (handler runs before any project filter can be committed), but treated the same way."
>
> From Implementation Notes: "The ProjectsLoadedMsg handler propagation is shape-consistency only: it is not reachable today with a committed projects filter ... No production-reachable failure exists to test against, and no test is added for this site."
>
> "The current ProjectsLoadedMsg handler (internal/tui/model.go:936-947) returns m, nil. After the change, capture the cmd from the SetItems call and return it directly — no tea.Batch is needed at this site."

**Spec Reference**: `.workflows/esc-after-preview-hides-session-list/specification/esc-after-preview-hides-session-list/specification.md` (Fix Approach → "Secondary sweep"; Implementation Notes; Acceptance Criteria #5)

---

## esc-after-preview-hides-session-list-1-6 | approved

### Task esc-after-preview-hides-session-list-1-6: Audit sibling bubbles/list mutator call sites (SetItem/InsertItem/RemoveItem)

**Problem**: `bubbles/list` exposes sibling mutator APIs (`SetItem`, `InsertItem`, `RemoveItem`) that share the same "returns a cmd you must propagate" contract as `SetItems`. Any call site that discards the cmd against a filtered list would blank-render the same way. The fix from tasks 1-3 and 1-5 addresses every known `SetItems` discard site, but without an audit of the sibling APIs against `m.sessionList` and `m.projectList`, latent variants on those APIs could remain. The spec mandates auditing these sites and recording the outcome in the PR description even when empty.

**Solution**: Grep / audit the `internal/tui/` codebase for calls to `m.sessionList.SetItem(...)`, `m.sessionList.InsertItem(...)`, `m.sessionList.RemoveItem(...)`, and the same three methods on `m.projectList`. Propagate the returned cmd at any sites found following the same pattern as task 1-3 (for handlers with a cmd dispatcher) or task 1-5 (for sites without one). If no sites are found, record the audit outcome (sites checked + result) in the PR description.

**Outcome**: Either (a) any sibling mutator call sites that discard their cmd are fixed by propagation, or (b) the audit confirms no such call sites exist. Either outcome is recorded in the PR description (sites checked + result) per the spec's acceptance criterion.

**Do**:
- Run `Grep` for `sessionList.SetItem\b`, `sessionList.InsertItem\b`, `sessionList.RemoveItem\b`, `projectList.SetItem\b`, `projectList.InsertItem\b`, `projectList.RemoveItem\b` across `internal/tui/`. Use word-boundary regex to avoid false matches against `SetItems` (plural).
- For each match found:
  1. Determine whether the call site discards the returned cmd.
  2. If discarded, fix using the same pattern as task 1-3 (propagate the cmd out of the handler / call site) or, if the site has no cmd dispatcher available, the lock-in pattern from task 1-5.
  3. If a code change is required, run `go test ./internal/tui/...` to verify no regression.
- Compile the audit outcome — list of methods searched, files searched, and either (a) the list of sites fixed or (b) "no sites found, no code change required". This outcome must go into the PR description per AC #5 in the spec.
- If no sites are found and no code change is needed, this task produces only the audit record — that is the deliverable.

**Acceptance Criteria**:
- [ ] `Grep` audit performed for `SetItem`, `InsertItem`, `RemoveItem` against `m.sessionList` and `m.projectList` across `internal/tui/`.
- [ ] Any discard sites found are fixed via cmd propagation (or lock-in if no dispatcher is available).
- [ ] Audit outcome (sites checked + result, or sites fixed) is recorded for inclusion in the PR description.
- [ ] If code changes are made, `go test ./internal/tui/...` passes.
- [ ] If no sites are found, the audit record explicitly states "no sites found".

**Tests**:
- No new tests required by this task per the spec (the kill-refresh test from task 1-4 is the single representative latent-variant test). If a real discard site is discovered and fixed, follow the project's existing test patterns for that surface — but the spec does not pre-mandate coverage here.

**Edge Cases**:
- Audit outcome must be captured even when empty — AC #5 in the spec phase mandates "audit outcome (sites checked + result) is recorded in the PR description" regardless of whether any sites were found.
- Word-boundary regex matters — `SetItems` (plural, already covered by task 1-3 / 1-5) must not be conflated with `SetItem` (singular) when searching.
- Sibling APIs on lists outside `m.sessionList` / `m.projectList` are out of audit scope — the spec scopes the audit to those two lists.

**Context**:
> From spec Scope: "Sweep of the remaining SetItems discard sites in internal/tui/model.go (Model.WithInsideTmux, ProjectsLoadedMsg handler), plus an audit of sibling bubbles/list mutator APIs (SetItem, InsertItem, RemoveItem) against m.sessionList and m.projectList. The sibling APIs share the same 'returns a cmd you must propagate' contract — any call site that discards the cmd against a filtered list would blank-render the same way. Propagate the cmd at any sites found; if none exist, record the audit outcome (sites checked + result) in the PR description."
>
> From Acceptance Criteria #5: "Sibling mutators (SetItem, InsertItem, RemoveItem) on m.sessionList/m.projectList are audited; any discard sites found are fixed the same way; the audit outcome is recorded."

**Spec Reference**: `.workflows/esc-after-preview-hides-session-list/specification/esc-after-preview-hides-session-list/specification.md` (Scope; Acceptance Criteria #5)
