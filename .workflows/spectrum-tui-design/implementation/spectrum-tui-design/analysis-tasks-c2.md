---
topic: spectrum-tui-design
cycle: 2
total_proposed: 4
---
# Analysis Tasks: Spectrum TUI Design (Cycle 2)

## Task 1: Projects list must drop vim/uppercase/page-jump keys (§12.2 arrow-only nav)
status: pending
severity: high
sources: standards

**Problem**: §12.2 mandates dropping all vim aliases (`h/j/k/l`, `g/G`), `PgUp/PgDn/Home/End`, and "no uppercase bindings anywhere" on BOTH list pages (§12.1 lists the same nav revision for Sessions AND Projects). The Sessions list enforces this: `newSessionList` calls `pinArrowOnlyNav`, which rebinds `CursorUp/Down` to `up/down`, `PrevPage/NextPage` to `ctrl+up/ctrl+down`, and empties `GoToStart/GoToEnd`. `newProjectList` (internal/tui/model.go:1080-1093) does NOT call `pinArrowOnlyNav` — the only difference between the two constructors — so the Projects list retains the `bubbles/list` v2 `DefaultKeyMap` (`CursorUp=[up,k]`, `CursorDown=[down,j]`, `PrevPage=[left,h,pgup,b,u]`, `NextPage=[right,l,pgdown,f,d]`, `GoToStart=[home,g]`, `GoToEnd=[end,G]`). `updateProjectsPage` (model.go:2363-2364) intercepts `?`, `esc`, `q`, `x`, `n`, `d`, `e`, `enter` before delegating to `m.projectList.Update`, but every un-handled key — `j`, `k`, `h`, `l`, `g`, `G`, `pgup`, `pgdown`, `home`, `end`, `b`, `u`, `f` — falls through and moves the cursor/page. So a user pressing these on Projects gets live navigation §12.2 forbids; the uppercase `G` additionally violates "no uppercase bindings anywhere." This is genuine behaviour drift, not cosmetic.
**Solution**: Call `pinArrowOnlyNav(&l.KeyMap)` in `newProjectList`, mirroring `newSessionList`, so the banned keys never reach the Projects list's own `Update`.
**Outcome**: On the Projects page, `j/k/h/l/g/G/pgup/pgdown/home/end/b/u/f` are inert for cursor/page movement; move is `↑/↓` and page is `Ctrl+↑/↓`, identical to the Sessions list. The §12.2 nav revision holds on both list pages.
**Do**:
1. In `internal/tui/model.go` `newProjectList` (around line 1080-1093), add a `pinArrowOnlyNav(&l.KeyMap)` call, matching the placement/usage in `newSessionList` (model.go:1056-1063 is the helper; confirm the exact call shape used by the Sessions constructor).
2. Verify `pinArrowOnlyNav` mutates the `KeyMap` in place exactly as the Sessions sibling expects (rebinds `CursorUp/Down` → `up/down`, `PrevPage/NextPage` → `ctrl+up/ctrl+down`, empties `GoToStart/GoToEnd`).
3. Do not alter the `updateProjectsPage` interception list — the fix is purely the omitted `pinArrowOnlyNav` call.
**Acceptance Criteria**:
- `newProjectList` calls `pinArrowOnlyNav` on its list `KeyMap`, identically to `newSessionList`.
- On the Projects page, `j`, `k`, `h`, `l`, `g`, `G`, `pgup`, `pgdown`, `home`, `end`, `b`, `u`, `f` do not move the cursor or change the page.
- `↑/↓` move the cursor and `Ctrl+↑/↓` page on the Projects page.
- No uppercase binding (`G`) navigates on the Projects page.
**Tests**:
- Add a dispatch-layer (not descriptor-layer) test asserting that `j/k/h/l/g/G/pgup/pgdown/home/end` are no-ops on the Projects page cursor — the same coverage the Sessions list has. The existing `projects_keymap_test.go:133` test ("it has no uppercase or vim-alias key in the descriptor") tests the descriptor DISPLAY layer only and gives false assurance; the new test must drive the live `bubbles/list` dispatch (send the key through `updateProjectsPage` / the model `Update` and assert cursor index is unchanged).
- Add an arrow-key positive test confirming `↑/↓` still move and `Ctrl+↑/↓` still page on Projects.

## Task 2: Remove dead per-modal footer/key-hint wrappers and consolidate the accent.blue key-hint path
status: pending
severity: medium
sources: duplication, architecture

**Problem**: A prior consolidation round moved the modal footer shape into shared `renderConfirmCancelFooter` / `renderKeyHint` (modal_footer.go) and the destructive-confirm grammar into `renderDestructiveConfirm` (destructive_confirm.go), but the superseded per-modal wrappers were never deleted. `killModalFooterRow` (kill_modal.go:67) and `deleteModalFooterRow` (delete_modal.go:70) now have ZERO production callers — the kill/delete modals render footers through `destructiveFooterRow` — surviving only because their golden tests still call them (modal_footer_test.go:256, 274). The three `*ModalKeyHint` wrappers (`killModalKeyHint` kill_modal.go:73, `deleteModalKeyHint` delete_modal.go:76, `renameModalKeyHint` rename_modal.go:177) are likewise production-dead byte-identical one-liners returning `renderKeyHint(key, label, theme.MV.AccentBlue, mode, colourless)`, referenced only by their own tests (modal_footer_test.go:152/169/187). Separately, two LIVE wrappers — `editFooterGroup` (edit_modal.go:554) and `previewFooterHint` (pagepreview.go:311) — have the identical byte-for-byte body, wrapping nothing more than the accent.blue default `renderKeyHint` already supports. So the same trivial "key hint in accent.blue" wrapper has been independently re-authored five times (a Rule-of-Three violation), with three dead copies plus two live ones. (Both the duplication and architecture agents independently flagged the three dead `*ModalKeyHint` wrappers; modal_footer.go's own header comment names these functions as the ones it absorbed.)
**Solution**: Delete the two dead footer-row functions (`killModalFooterRow`, `deleteModalFooterRow`) and all three dead `*ModalKeyHint` wrappers along with their dedicated sub-tests, then collapse the two live accent.blue wrappers (`editFooterGroup`, `previewFooterHint`) to a single canonical path — either by routing both through one shared `renderBlueKeyHint(key, label, mode, colourless)` helper in modal_footer.go that pins `AccentBlue`, or by calling `renderKeyHint(..., theme.MV.AccentBlue, ...)` directly at the two live call sites and deleting the wrappers. The production paths through `destructiveFooterRow` / `renderKeyHint` are already covered by modal_footer_test.go's `renderConfirmCancelFooter` and `renderKeyHint` golden cases.
**Outcome**: There is exactly one canonical blue-key-hint path. No dead footer-row or key-hint wrapper remains in kill_modal.go / delete_modal.go / rename_modal.go. The live edit and preview footers render byte-identically to today through the shared path, and a future edit cannot silently leave a per-modal clone inconsistent with the shared path. `go build` and `go test ./...` pass.
**Do**:
1. Delete `killModalFooterRow` (kill_modal.go:67-75) and `deleteModalFooterRow` (delete_modal.go:70-78) — confirm via grep they have no non-test callers before deleting.
2. Delete `killModalKeyHint` (kill_modal.go:73), `deleteModalKeyHint` (delete_modal.go:76), and `renameModalKeyHint` (rename_modal.go:177) — confirm via grep they have no non-test callers.
3. Delete the now-orphaned sub-tests in modal_footer_test.go that exercised those five deleted functions (the footer-row clones at ~lines 256/274 and the three key-hint wrappers at ~lines 152/169/187). Do NOT delete the `renderConfirmCancelFooter` / `renderKeyHint` golden cases — those cover the surviving production path.
4. Collapse the two live accent.blue wrappers: pick ONE approach — (a) add a shared `renderBlueKeyHint(key, label, mode, colourless)` in modal_footer.go that pins `theme.MV.AccentBlue` and route both `editFooterGroup` and `previewFooterHint` call sites through it (then delete the two wrappers), OR (b) inline `renderKeyHint(..., theme.MV.AccentBlue, ...)` at the two live call sites and delete the wrappers. Prefer (a) for a named canonical seam.
5. Note `renameModalFooterRow` STAYS — it has a live caller (rename_modal.go:71). The duplication agent observed it is a thin wrapper over `renderConfirmCancelFooter` that could be inlined at its single call site; inlining it is optional polish, not required by this task.
6. Run `go build -o portal .` and `go test ./...`.
**Acceptance Criteria**:
- `killModalFooterRow`, `deleteModalFooterRow`, `killModalKeyHint`, `deleteModalKeyHint`, `renameModalKeyHint` no longer exist anywhere in the codebase (grep returns no matches).
- Exactly one shared accent.blue key-hint path exists; `editFooterGroup` and `previewFooterHint` either both route through it or are deleted in favour of direct `renderKeyHint` calls.
- The edit modal footer and the preview footer render byte-identically to before this change.
- `go build` succeeds and `go test ./...` passes.
**Tests**:
- Confirm the surviving `renderConfirmCancelFooter` and `renderKeyHint` golden tests in modal_footer_test.go still pass and cover the live kill/delete/rename footer paths through `destructiveFooterRow` / `renderConfirmCancelFooter`.
- If a shared `renderBlueKeyHint` is introduced, add/retain one golden test asserting it pins `theme.MV.AccentBlue` (so the canonical blue path stays covered after the five wrappers are removed).
- Verify (golden snapshot or string assertion) that the edit modal footer and preview footer outputs are unchanged.

## Task 3: Scope the keymap descriptor "single source of truth" framing to display and guard descriptor↔dispatch drift
status: pending
severity: medium
sources: architecture

**Problem**: The per-page `keymapEntry` descriptors (`sessionsKeymap` / `projectsKeymap` / `previewKeymap`, keymap.go:14-49) are documented across many comments as the single source of truth for each page's keybindings "so a binding change updates the footer and the help together." That guarantee holds only for the two DISPLAY surfaces — the condensed footer (footer.go:52-66) and the `?` help modal (help_modal.go:10-15). The actual key dispatch in `Update` (model.go:3056-3083, 2322-2358) is a separate hand-coded switch over literal strings (`isRuneKey(msg, "k")`, `case "s"`, `keyIsCode(msg, tea.KeyEnter)`) with no structural link to the descriptor. So a keybinding is defined in two unconnected places and the compiler cannot catch divergence: rebind kill from `k`, or add a new bound key, and the footer/help can silently advertise a key dispatch no longer honours (or omit one it does) with nothing failing to compile or test. The dispatch half is the half a user actually feels. (The Cycle-2 high-severity Projects-nav bug, handled in Task 1, is a concrete instance of exactly this descriptor/dispatch gap — the descriptor-level `projects_keymap_test.go` passed while live dispatch leaked the banned keys.)
**Solution**: Keep the descriptor and dispatch separate (the dispatch switch pre-existed the reskin and must stay behaviourally identical), but close the drift gap two ways: (1) narrow the "single source of truth" doc comments to say "single source of truth for the footer + help *display*," removing the implication that dispatch is covered; and (2) add a guard test that asserts every non-help descriptor `Key` has a corresponding dispatch arm and vice-versa, so the two cannot silently diverge.
**Outcome**: The descriptor's documented guarantee matches what it actually delivers (display only). A guard test fails if a descriptor advertises a key dispatch does not honour, or if a dispatched bound key is missing from the descriptor — making the previously-uncatchable divergence a test failure.
**Do**:
1. Locate the "single source of truth" doc comments on/around the `keymapEntry` descriptors (keymap.go:14-49) and the help/footer consumers, and rescope the wording to "single source of truth for the footer + help *display*" (do not claim dispatch coverage). Keep the existing comment that already concedes "Key ... is a display token, not a tea key code — the live dispatch ... owns the actual key matching."
2. Add a guard test (e.g. `keymap_dispatch_guard_test.go`) that, for each page's descriptor (`sessionsKeymap`, `projectsKeymap`, `previewKeymap`), asserts every non-help descriptor `Key` maps to a dispatch arm honoured by that page's `Update`, and that every bound dispatch key on that page appears in the descriptor. Use whatever enumeration of the dispatch arms is reachable from the model layer; if the dispatch switch is not introspectable, drive each descriptor `Key` through the page's `Update` and assert it produces the bound effect (or a documented allow-list excludes intentionally display-only/help entries).
3. Do NOT change live dispatch behaviour — this task is documentation + a guard test only. (Any behavioural fix surfaced by the new guard is out of scope here except where Task 1 already addresses it.)
**Acceptance Criteria**:
- The "single source of truth" comments no longer imply dispatch coverage; they explicitly scope the guarantee to footer + help display.
- A guard test exists asserting descriptor↔dispatch correspondence per page (every non-help descriptor `Key` honoured by dispatch, every bound dispatch key present in the descriptor).
- The guard test passes on the current (post-Task-1) tree.
- No live dispatch behaviour changes as part of this task.
**Tests**:
- The new descriptor↔dispatch guard test itself (described above) is the deliverable test.
- Confirm existing footer/help display tests still pass after the comment rescoping.

## Task 4: Close residual enforcement and DRY gaps in the reskin (colour guard, command-pending footer, separator constants)
status: pending
severity: low
sources: standards, architecture, duplication

**Problem**: Three small, code-certain enforcement/DRY gaps remain after the reskin. (1) The closed-vocabulary "no literal hex at call sites" guard `TestNoRawColourLiteralAtCentralisedSites` (colour_literal_guard_test.go:21-33) enumerates only 11 files in `centralisedColourSites` and was never grown for Phases 3-5; later render files (`destructive_confirm.go`, `kill_modal.go`, `delete_modal.go`, `pagepreview.go`, `loading_view.go`, `notice_band.go`, `sessions_flash.go`, `empty_states.go`, and others) reference theme tokens but are NOT guarded — a future raw-hex regression in e.g. `pagepreview.go` would not be caught (no live violation exists today; the gap is purely lost enforcement, and the file's own comment says coverage should grow with the migration). (2) The command-pending footer path is the one footer source that bypasses the descriptor/entry vocabulary entirely: `commandPendingHelpKeys()` returns a raw `[]key.Binding` that footer.go re-maps in `commandPendingFooterEntries` with bespoke `if glyph == "enter" { glyph = "⏎" }` string-rewriting that the descriptor path already encodes declaratively via `HelpKey`. (3) The `" · "` dot separator is declared twice as same-role constants — `editFooterSep` (edit_modal.go:65) and `footerEntrySeparator` (footer.go:45) — both joining footer groups to the §3.4 condensed-footer dot rhythm, so they can silently drift.
**Solution**: (1) Extend `centralisedColourSites` to enumerate every production render file in internal/tui (and internal/capture, as applicable) that references theme tokens — or switch the guard to glob all non-test `.go` files in internal/tui except `theme/` — so the closed-vocabulary rule stays enforced across the whole reskin. (2) Fold `commandPendingHelpKeys()`'s raw `[]key.Binding` into a `keymapEntry` descriptor (or at minimum a `filterFooterEntry` slice authored directly), eliminating the one-off `enter→⏎` rewrite and the only footer source outside the descriptor/entry vocabulary. (3) Promote the single shared `footerEntrySeparator` (footer.go) and have the edit modal reference it instead of its own `editFooterSep`. Leave the `helpColumnGap` / `modalFooterGap` `"   "` pair separate (arguably distinct roles — body-column gap vs footer-group gap) and leave the `filterFooterEntry` / `footerHintGroup` parallel models as-is (each earns its shape).
**Outcome**: The colour-literal guard protects the full set of token-referencing render files (no Phase 3-5 blind spot). The command-pending footer is sourced from the same descriptor/entry vocabulary as every other footer, with no bespoke `enter→⏎` rewrite. There is one `" · "` footer separator constant shared by the edit modal and the main footer. `go build` and `go test ./...` pass and the footers render byte-identically.
**Do**:
1. Colour guard: extend `centralisedColourSites` in colour_literal_guard_test.go to include every production render file in internal/tui that references theme tokens (enumerate them, or convert the guard to glob non-test `.go` files in internal/tui excluding `theme/`). Run the guard to confirm it passes (the agent grep found no live raw-hex outside theme/theme.go).
2. Command-pending footer: refactor `commandPendingHelpKeys()` / `commandPendingFooterEntries` so the command-pending footer is authored as a `keymapEntry` slice (preferred) or a directly-authored `filterFooterEntry` slice, using the declarative `HelpKey` glyph encoding instead of the inline `if glyph == "enter" { glyph = "⏎" }` rewrite. Preserve the exact rendered output.
3. Separator constant: delete `editFooterSep` (edit_modal.go:65) and reference the shared `footerEntrySeparator` (footer.go:45) at its use site. Do NOT consolidate the `helpColumnGap` / `modalFooterGap` `"   "` gap pair (distinct roles).
4. Run `go build -o portal .` and `go test ./...`; verify footer outputs are unchanged.
**Acceptance Criteria**:
- `centralisedColourSites` (or the guard's glob) covers every production token-referencing render file in internal/tui; the guard passes.
- The command-pending footer is sourced from a `keymapEntry`/`filterFooterEntry` slice with no inline `enter→⏎` string rewrite; its rendered output is unchanged.
- `editFooterSep` is gone; the edit modal and main footer share the single `footerEntrySeparator` constant.
- The `helpColumnGap` / `modalFooterGap` pair is left as-is.
- `go build` succeeds and `go test ./...` passes.
**Tests**:
- The extended colour-literal guard test must pass on the current tree (proving no live raw-hex regression in the newly-covered files).
- Add or retain a golden/string assertion that the command-pending footer renders identically after sourcing it from the descriptor/entry vocabulary.
- Verify (golden snapshot or string assertion) the edit modal footer renders identically after switching to the shared `footerEntrySeparator`.
