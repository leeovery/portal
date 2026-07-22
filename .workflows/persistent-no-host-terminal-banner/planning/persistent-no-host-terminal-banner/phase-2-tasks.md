---
phase: 2
phase_name: Proactive Multi-Select Entry Block on Unsupported Terminals
total: 3
---

## persistent-no-host-terminal-banner-2-1 | approved

### Task 2.1: Rework the reactive-backstop no-op tests onto the in-flight entry path

**Problem**: `TestBurstUnsupported_NonNullAtomicNoOp` and `TestBurstUnsupported_NullFlash` (`internal/tui/burst_unsupported_noop_test.go`) enter multi-select mode *after* `resolveDetection` resolves an unsupported terminal — i.e. they press `m` while `DetectUnsupported()` is already true. Task 2.2 adds a proactive entry block that makes exactly that post-resolve `m` a no-op, so their `markTwo` precondition (which calls `enterMultiSelectEmpty` → presses `m` through `handleMultiSelectToggle`) will fail once 2.2 lands. They must be reworked **first**, as a standalone test-only commit, so the suite stays green per-commit and the two tests are sharpened onto the async-race path the retained `decideBurst` backstop actually guards.

**Solution**: In both tests, move the `markTwo(t, m)` call to **before** `resolveDetection(...)` so multi-select is entered and the two rows are marked while detection is still in flight (`detectResolved == false` → entry block inert → the mode opens). Then resolve detection to the unsupported identity, then press Enter — which routes through `handleMultiSelectEnter → beginBurst → decideBurst` (detection now resolved) and lands the same atomic no-op + re-asserted flash. No asserted copy string, no `assertAtomicNoOp` invariant, and no other test in the file changes.

**Outcome**: The two reworked tests pass under the **current** production code (no entry block yet) *and* continue to pass unchanged after Task 2.2 lands; they now document the A1 "entered-before-resolve → Enter" backstop path rather than the (soon-blocked) post-resolve entry.

**Do**:
- In `TestBurstUnsupported_NonNullAtomicNoOp` (`internal/tui/burst_unsupported_noop_test.go` ~L119): reorder so the sequence is `m := NewModelWithSessions(sessions)` → `wireUnsupportedBurstSeams(&m, adapter, ack)` → `m = markTwo(t, m)` (enters mode + marks `alpha`/`bravo` while `detectResolved == false`) → `m = resolveDetection(t, m, appleTerminalIdentity())` → `if !m.DetectUnsupported() { t.Fatal(...) }` → `m, cmd := pressEnter(t, m)` → `assertAtomicNoOp(t, m, adapter)` + the existing `isQuitCmd` guard + the named-flash assertion. The `const want = "unsupported terminal — Apple Terminal · com.apple.Terminal — nothing opened"` and the `m.flashText` check stay **byte-identical**.
- In `TestBurstUnsupported_NullFlash` (~L149): apply the identical reordering with `spawn.Identity{}` (NULL) and the unchanged `const want = "no host-local terminal — nothing opened"`. Keep the existing comment noting `spawn.Identity{}` pins both the remote/mosh and the transient-detection-error case.
- Update each reworked test's doc comment to state that it now enters multi-select **during the in-flight window** (A1 — before detection resolves), so the Enter drives `decideBurst`'s retained reactive unsupported arm.
- Leave `TestBurstUnsupported_DeferredThenUnsupported` (~L183) **unchanged** — it already enters via `markTwo` while `detectDispatched = true && !detectResolved`, then defers the Enter, then resolves through the `terminalDetectedMsg` arm.
- Leave `TestBurstUnsupported_SupportedStillDispatches` (~L229) **unchanged** — a supported (ghostty→native) resolution is never blocked at entry, so its `resolveDetection`-then-`markTwo` order stays valid.
- Leave `TestUnsupportedFlashText` (~L87), `wireUnsupportedBurstSeams`, `markTwo`, `assertAtomicNoOp`, and every asserted string **unchanged**.
- Do **not** touch any production code in this task, and do **not** touch `internal/spawn/message.go` or the `unsupportedFlashText` copy (Phase 3 owns that rewrite).

**Acceptance Criteria**:
- [ ] Both `TestBurstUnsupported_NonNullAtomicNoOp` and `TestBurstUnsupported_NullFlash` call `markTwo(t, m)` **before** `resolveDetection(...)`.
- [ ] The two asserted flash strings and every `assertAtomicNoOp` invariant are byte-identical to the pre-rework versions.
- [ ] `go test ./internal/tui -run TestBurstUnsupported` passes against the current code (before Task 2.2 is applied) — unit lane, no tmux daemon.
- [ ] `TestBurstUnsupported_DeferredThenUnsupported` and `TestBurstUnsupported_SupportedStillDispatches` are unchanged and still pass.
- [ ] No production file is modified by this task (test-only commit).

**Tests**:
- `"it is an atomic no-op + named flash when multi-select was entered in-flight then resolves unsupported"` (reworked `TestBurstUnsupported_NonNullAtomicNoOp`).
- `"it is an atomic no-op + honest NULL flash when multi-select was entered in-flight then resolves NULL"` (reworked `TestBurstUnsupported_NullFlash`).
- `"it defers an in-flight Enter and lands the no-op when detection resolves unsupported"` (existing `TestBurstUnsupported_DeferredThenUnsupported`, unchanged — retained backstop coverage).
- `"it still dispatches the burst on a supported terminal"` (existing `TestBurstUnsupported_SupportedStillDispatches`, unchanged).

**Edge Cases**:
- **In-flight entry before resolve (NonNull + NULL)**: the mode is entered while `detectResolved == false`, so the entry block (Task 2.2) is inert; the reactive no-op then fires on the post-resolve Enter through `decideBurst`. This is the exact race the backstop exists to catch (spec §3 "Async in-flight window").
- **Deferred-Enter no-op retained**: `TestBurstUnsupported_DeferredThenUnsupported` covers the sibling path where the Enter itself is pressed in-flight and resolves via the `terminalDetectedMsg` `pendingBurstEnter` branch — kept as-is.
- **Supported-still-dispatches unchanged**: a supported resolution never trips the entry block, so its post-resolve `markTwo` is still valid; guards that the rework does not regress the supported path.
- **Backstop copy unchanged (Phase 3)**: the `— nothing opened` reactive strings are NOT touched here — this task restructures entry timing only; Phase 3 owns the `spawn.UnsupportedNoopMessage` / `unsupportedFlashText` rewrite.
- **Ordered before the entry block**: this rework must be committed **before** Task 2.2 so the two tests never transiently fail (their old post-resolve `markTwo` would fail the moment 2.2's block lands).

**Context**:
> Spec §7 (Testing Requirements — Rework): "`TestBurstUnsupported_NonNullAtomicNoOp` and `TestBurstUnsupported_NullFlash` enter multi-select *after* `resolveDetection`; the post-resolve `m` is now blocked, so their `markTwo` precondition fails. Rework both to enter multi-select **before** resolving detection (the in-flight path). `TestBurstUnsupported_DeferredThenUnsupported` (already in-flight) and `TestBurstUnsupported_SupportedStillDispatches` (supported) stay valid. Keep the deferred-Enter → reactive no-op coverage (the retained backstop)."
> Spec §3 (Retain the reactive backstop — Fork A → A1): "`decideBurst`'s reactive unsupported no-op ... is **retained**. It is not redundant: detection is asynchronous, so the entry block cannot fully replace it." Until detection resolves, `DetectUnsupported() == false`, so a user *can* enter multi-select during the in-flight window; the reactive `decideBurst` no-op is the sole backstop for that path.
> Mechanic: `markTwo` (`internal/tui/burst_unsupported_noop_test.go`) → `enterMultiSelectEmpty` → `pressSession(t, m, pressM)` drives `handleMultiSelectToggle`. Task 2.2 gates that entry on `DetectUnsupported()`; pressing `m` while `detectResolved == false` keeps the gate inert. The retained reactive arm is `decideBurst` (`internal/tui/burst_progress.go` ~L425, `if m.DetectUnsupported()`), which sets `m.setFlash(unsupportedFlashText(...))` and returns `m, nil`.

**Spec Reference**: `/Users/leeovery/Code/portal/.workflows/persistent-no-host-terminal-banner/specification/persistent-no-host-terminal-banner/specification.md` §3 (Sub-fix 2 — Retain the reactive backstop / Async in-flight window), §7 (Testing Requirements — Rework).

## persistent-no-host-terminal-banner-2-2 | approved

### Task 2.2: Proactive multi-select entry block + TUI-local blocked-entry flash helper

**Problem**: On **any** resolved-unsupported terminal (NULL/remote or named-undriven), pressing `m` enters multi-select mode and lets the user mark sessions, only to dead-end at the N≥2 Enter with the reactive no-op flash. The entry branch of `handleMultiSelectToggle` reads no detection state — the only unsupported gate is downstream at `decideBurst` — so the mode is offered for a burst that can never fire. The affordance is a walkable dead-end.

**Solution**: Gate the entry branch of `handleMultiSelectToggle` (`internal/tui/model.go`) on `DetectUnsupported()`. When detection has resolved unsupported, pressing `m` does **not** open the mode — it sets a transient blocked-entry flash (a visible honest signal, not a silent swallow) and returns. Add a TUI-local `multiSelectBlockedFlashText(id)` helper that selects the flash shape via `id.IsNull()` (mirroring `unsupportedFlashText`'s branch), returning two plain intent-only strings that carry **no** `⚠` glyph (the notice band prepends it). Retain `decideBurst`'s reactive backstop for the async in-flight window (untouched).

**Outcome**: Once detection has resolved unsupported, `m` fails immediately with an honest transient flash and the mode never opens (nothing marked); the flash self-clears on the next actionable key; on a named terminal the block flash co-renders two-row with the persistent banner (both rows carry `⚠`); before detection resolves the mode is still enterable (backstop catches the burst at Enter); and a supported terminal is entirely unaffected.

**Do**:
- Add the TUI-local copy helper in `internal/tui/model.go` (place it near `handleMultiSelectToggle` ~L3497, mirroring `unsupportedFlashText`'s free-function shape). Prefer two package-level string constants for testability:
  ```go
  const (
      multiSelectBlockedRemoteFlash = "multi-select isn't available over a remote connection"
      multiSelectBlockedNamedFlash  = "multi-select isn't available on this terminal"
  )

  func multiSelectBlockedFlashText(id spawn.Identity) string {
      if id.IsNull() {
          return multiSelectBlockedRemoteFlash
      }
      return multiSelectBlockedNamedFlash
  }
  ```
  It carries no `⚠` and no `— nothing opened` suffix (a pre-emptive block attempts nothing); it is NOT shared with the CLI and needs no `cli-verb-surface-redesign` coordination.
- In `handleMultiSelectToggle` (`internal/tui/model.go` ~L3508), at the **top** of the `if !m.multiSelectMode {` entry branch — before `m.multiSelectMode = true` — insert the proactive block:
  ```go
  if !m.multiSelectMode {
      if m.DetectUnsupported() {
          (&m).setFlash(multiSelectBlockedFlashText(m.detectIdentity))
          return m, flashTickCmd(m.flashGen)
      }
      m.multiSelectMode = true
      ...
  }
  ```
  `setFlash` is a pointer method, so call it via `(&m)`; return `flashTickCmd(m.flashGen)` (the post-bump gen) so the block flash inherits the standard auto-clear timer exactly like the session-gone bail (`internal/tui/model.go` ~L2413-2414). The authoritative clear is still the next-actionable-key path at the top of `updateSessionList` (`internal/tui/model.go` ~L3328).
- Add an inline source note directly above the new gate recording the latent guard coupling (spec §4 / §8): `keymap_dispatch_guard_test`'s `m` probe drives `sessionsGuardModel` (`NewModelWithSessions`) with detection **unwired**, so `DetectUnsupported()` is false here and this block is inert (the probe still enters the mode); a future change that wires detection into `NewModelWithSessions` (or defaults `DetectUnsupported()` true) would make the `m` probe hit this block and fail — keep the guard seed detection-unwired.
- Do **not** gate `WithInitialMultiSelect` (`internal/tui/model.go` ~L1006) — it is a construction-time capture-harness setter (`m.multiSelectMode = true` directly), not a keypress, and must stay ungated so existing multi-select capture fixtures render regardless of detection state.
- Do **not** touch `decideBurst`, `unsupportedFlashText`, `emitUnsupportedNoop`, or `internal/spawn/message.go` — the reactive backstop and its (Phase-3-owned) copy are out of scope here. The entry block emits no observability line (unlike the reactive no-op) — it only sets the flash and returns.
- Add a new white-box test file `internal/tui/multi_select_entry_block_test.go` (`package tui`, no `t.Parallel()`), driving the states through `unsupportedResolvedModel(t, <identity>)` (sized, resolved, on `PageSessions`) and `pressSession(t, m, pressM)`.

**Acceptance Criteria**:
- [ ] On a resolved-unsupported **named** terminal (`appleTerminalIdentity()`), pressing `m` leaves `m.MultiSelectActive() == false`, `m.SelectedSessionCount() == 0`, and sets `m.flashText == "multi-select isn't available on this terminal"`.
- [ ] On a resolved **NULL/remote** terminal (`spawn.Identity{}`), pressing `m` leaves the mode closed and sets `m.flashText == "multi-select isn't available over a remote connection"`.
- [ ] The blocked flash clears when the next actionable key is pressed (`m.flashText == ""` after a subsequent `pressSession` with e.g. `tea.KeyPressMsg{Code: tea.KeyDown}`).
- [ ] On a **named** unsupported terminal, after a blocked `m` the two-row co-render holds: `m.unsupportedBannerActive() == true`; `bannerFirstLine(m)` carries `flashWarningGlyph` + `unsupported terminal`; `m.renderActiveNoticeBand()` carries `flashWarningGlyph` + the block-flash string; and the notice band does **not** repeat `unsupported terminal`, the identity string, or `see docs` (intent-only copy).
- [ ] Pressing `m` twice on an unsupported terminal keeps the mode closed and re-sets the block flash (clear-then-reflash): `m.MultiSelectActive() == false` and `m.flashText` equals the block string after the second press.
- [ ] While detection is **in flight** (`detectDispatched == true && !detectResolved`), pressing `m` still enters the mode (`m.MultiSelectActive() == true`) — the A1 window is not blocked.
- [ ] `WithInitialMultiSelect([...])` combined with resolved-unsupported detection still yields `m.multiSelectMode == true` (construction seam not gated).
- [ ] On a **supported** terminal (`ghosttyIdentity()`), pressing `m` enters the mode and sets no flash (`m.flashText == ""`).
- [ ] `multiSelectBlockedFlashText` returns the two plain per-shape strings and neither contains `flashWarningGlyph`.
- [ ] `go test ./internal/tui/...` passes (unit lane — no tmux daemon).

**Tests**:
- `"it blocks m and flashes the named intent line on a resolved-unsupported named terminal"`
- `"it blocks m and flashes the remote intent line on a resolved NULL/remote terminal"`
- `"the blocked flash self-clears on the next actionable key"`
- `"a blocked m on a named unsupported terminal co-renders banner + block flash, both carrying the warning glyph"`
- `"repeated m while blocked stays out of the mode and re-flashes"`
- `"m still enters multi-select while detection is in flight"`
- `"WithInitialMultiSelect is not gated under resolved-unsupported detection"`
- `"a supported terminal still enters multi-select with no flash"`
- `"multiSelectBlockedFlashText returns the plain per-shape strings and embeds no warning glyph"`

**Edge Cases**:
- **NULL vs named flash copy via `IsNull`**: `multiSelectBlockedFlashText` branches on `id.IsNull()` (`BundleID == ""`) exactly as `unsupportedFlashText` branches on the identity shape — remote copy for NULL, named copy for a recognised-but-undriven bundle id.
- **Intent-only copy (no bundle id / no `see docs` / no `— nothing opened`)**: the named block flash is the bare `multi-select isn't available on this terminal` — no identity string, no docs pointer, no attempt-suffix. A pre-emptive block attempts nothing (distinguishing it from the reactive no-op's `— nothing opened`), and the co-rendered banner already supplies the identity + remedy (spec §5 "Named non-repetition constraint").
- **Named two-row co-render, both rows carry `⚠`**: the persistent banner (row 1, `renderUnsupportedHeader`) and the transient block flash (row 2, `renderActiveNoticeBand`'s unconditional warning glyph) each carry `⚠` — accepted by design (a consistent warning *marker*, not duplicated *information*; the flash is transient). The block flash is **not** special-cased to drop its glyph.
- **Self-clears on next actionable key**: reuses the §11 flash slot and its `setFlash`/`isActionableKey` lifecycle; the top-of-`updateSessionList` clear (~L3328) fires on the next actionable key. The scheduled `flashTickCmd` auto-clear timer is an accepted (and expected) secondary path, not the assertion target.
- **Repeated `m` re-blocks + re-flashes (intentional)**: a second `m` while the flash shows first clears the flash (top-of-`updateSessionList`) then re-enters `handleMultiSelectToggle`, re-blocks, and re-flashes — net effect the flash stays shown and the mode stays closed.
- **In-flight window still enters mode**: `DetectUnsupported()` is false until `detectResolved`, so the block is inert during the async window (A1) — the reactive backstop (`decideBurst`) remains the only guard there.
- **`WithInitialMultiSelect` not gated**: it sets `m.multiSelectMode = true` directly at construction, bypassing `handleMultiSelectToggle`, so the capture harness is unaffected.
- **Supported terminal unaffected**: `DetectUnsupported()` false → the gate is skipped, `m` enters and dispatches as today.
- **Inline guard-coupling source note**: the gate carries a comment recording that `keymap_dispatch_guard_test`'s `m` probe depends on `NewModelWithSessions` keeping detection unwired (spec §4 latent note, carried into implementation).

**Context**:
> Spec §3 (Sub-fix 2 — Proactive Multi-Select Entry Block): "Gate the entry branch of `handleMultiSelectToggle` on `DetectUnsupported()`. Today the entry branch ... has **no** detection read ... The fix adds a proactive check: when `DetectUnsupported()` is true, pressing `m` does **not** open the mode — it sets a transient blocked-entry flash instead ... and returns. Applies to **both** unsupported shapes (NULL and named) ... the entry block is deliberately identity-blind (only the *flash copy* differs by shape)."
> Spec §3 (Only the keypress entry is gated): "`WithInitialMultiSelect` is a construction-time option used only by the capture harness (not a keypress) ... deliberately **not** gated ... Do **not** gate `WithInitialMultiSelect`."
> Spec §3 (Visible flash, not a silent no-op — Fork B → B1): "The block produces a **visible** transient flash rather than silently swallowing the keypress. ... a silent `m` reads as broken ... it must still fire so the keypress is not silently swallowed. This is the sole reason the pre-emptive entry block surfaces any feedback at all — do **not** 'simplify' it to a silent no-op."
> Spec §5 (Blocked-entry flash renderer — TUI-local): "The two blocked-entry strings live in `internal/tui` (not `internal/spawn`), rendered by a new TUI-local helper (e.g. `multiSelectBlockedFlashText(id)`) that selects the shape via `m.detectIdentity.IsNull()` — mirroring `unsupportedFlashText`'s shape branch. Unlike `UnsupportedNoopMessage`, this copy is **not** shared with the CLI and needs no `cli-verb-surface-redesign` coordination." Copy set: NULL/remote = `multi-select isn't available over a remote connection`; named = `multi-select isn't available on this terminal`.
> Spec §5 (Named non-repetition constraint): "In the named two-row state, the block flash carries **only** the 'multi-select isn't available' intent and must **not** repeat the co-rendered banner's `unsupported terminal` text, identity string, or `see docs` ... Keeping the flash intent-only is what keeps the two co-rendered rows non-redundant."
> Spec §5 (Shared `⚠` glyph — accepted): "The named two-row state therefore momentarily shows **two** `⚠` (banner + flash) until the flash self-clears on the next key. This is accepted ... The block flash is **not** special-cased to drop its glyph."
> Spec §4 / §8 (Latent guard-coupling note): "Sub-fix 2's entry block makes `keymap_dispatch_guard_test` newly sensitive to that seed state: a future change that wires detection into `NewModelWithSessions` (or defaults `DetectUnsupported()` true) would make the `m` probe hit the block and fail. This coupling is **not introduced** by this fix, but an inline source note near the entry-block gate / the guard probe should record it."
> Mechanics: `setFlash` (`internal/tui/model.go` ~L1969) is a pointer method that bumps `flashGen` and re-syncs layout; `flashTickCmd(gen)` (`internal/tui/sessions_flash.go` ~L67) schedules the auto-clear tick; the top-of-`updateSessionList` clear (`internal/tui/model.go` ~L3328, `if m.flashText != "" && isActionableKey(msg) { m.clearFlash() }`) is the next-key clear. `bannerFirstLine(m)` (`internal/tui/multi_select_banner_test.go` ~L189) returns `applySectionHeader(sessionList.View())`'s first line; `m.renderActiveNoticeBand()` renders the arbitrated §11 band with the warning glyph prepended for `bandWarning` (`internal/tui/notice_band.go` `statusGlyph`). `unsupportedResolvedModel(t, id)` (`internal/tui/unsupported_banner_test.go` ~L33) yields a sized (`80×24`), resolved, `PageSessions` model via the production `nativeResolve()`; `appleTerminalIdentity()` resolves named-unsupported, `spawn.Identity{}` resolves NULL-unsupported, `ghosttyIdentity()` resolves native.

**Spec Reference**: `/Users/leeovery/Code/portal/.workflows/persistent-no-host-terminal-banner/specification/persistent-no-host-terminal-banner/specification.md` §3 (Sub-fix 2 — Proactive Multi-Select Entry Block), §5 (Unsupported-Terminal Copy — blocked-entry flash renderer, non-repetition, shared glyph), §4 / §8 (latent guard-coupling note), §7 (Testing Requirements — `m`-entry block new coverage).

## persistent-no-host-terminal-banner-2-3 | approved

### Task 2.3: Help-modal m-suppression at the Sessions call site

**Problem**: While `m` is unavailable (an unsupported terminal, not already in multi-select), the `?` help body still lists the `m` (multi-select) row — advertising a key that does nothing. The help must hide `m` exactly when it would be blocked, without hiding it in the states where it still works (supported, or already in the mode via the A1 in-flight path).

**Solution**: Filter the `m` entry out of the descriptor slice passed to the Sessions help modal **at the call site** (`renderHelpModalOnClearedCanvas(sessionsKeymap(), …)` in `internal/tui/model.go` ~L4547) when `DetectUnsupported() && !m.multiSelectMode`. `sessionsKeymap()` itself stays a pure static constant — the filter is applied only to the copy fed to the modal, via a small `m.sessionsHelpKeymap()` helper. The Projects help call site is untouched.

**Outcome**: The `?` help body omits the `m` row **iff** `DetectUnsupported() && !m.multiSelectMode`; it lists `m` on a supported terminal and in the A1 unsupported-but-in-multi-select state; the footer is unchanged under every resolution (`m` is non-`Core`); `keymap_dispatch_guard_test` stays green (it runs with detection unwired, so the filter is inert); and the Projects help is byte-unchanged.

**Do**:
- Add a filter helper in `internal/tui/model.go` (near the Sessions `modalHelp` branch, or in `internal/tui/keymap.go` alongside `sessionsKeymap`):
  ```go
  func (m Model) sessionsHelpKeymap() []keymapEntry {
      entries := sessionsKeymap()
      if !(m.DetectUnsupported() && !m.multiSelectMode) {
          return entries
      }
      filtered := make([]keymapEntry, 0, len(entries))
      for _, e := range entries {
          if e.Key == "m" {
              continue
          }
          filtered = append(filtered, e)
      }
      return filtered
  }
  ```
  The predicate is `DetectUnsupported() && !m.multiSelectMode` — exactly "`m` appears in `?` help iff `m` is functional."
- Change the Sessions `modalHelp` case (`internal/tui/model.go` ~L4547) from `renderHelpModalOnClearedCanvas(sessionsKeymap(), …)` to `renderHelpModalOnClearedCanvas(m.sessionsHelpKeymap(), …)`. Leave the rest of the call (`m.contentWidth(), m.contentHeight(), m.canvasMode, m.colourless`) unchanged.
- Leave the Projects `modalHelp` case (`internal/tui/model.go` ~L4365, `renderHelpModalOnClearedCanvas(projectsKeymap(), …)`) **untouched**.
- Do **not** parameterise `sessionsKeymap()` (dropping `m` inside the descriptor function) — that is explicitly rejected because it would break `keymap_dispatch_guard_test` (which probes the *static* descriptor with detection unwired). The filter must live at the call site.
- Add a new white-box test file `internal/tui/help_modal_m_suppression_test.go` (`package tui`, no `t.Parallel()`), building states via `unsupportedResolvedModel(t, <identity>)` and asserting on `m.sessionsHelpKeymap()` (has/lacks the `Key == "m"` entry) plus a render-level check on `renderHelpModalContent(m.sessionsHelpKeymap(), m.canvasMode, m.colourless)` (or `helpModalBody(...)`) for the `m` row's HelpAction `"Multi-select mode"`.

**Acceptance Criteria**:
- [ ] On a resolved-unsupported terminal (both `appleTerminalIdentity()` and `spawn.Identity{}`), **not** in multi-select, `m.sessionsHelpKeymap()` contains **no** entry with `Key == "m"`, and the rendered help body omits the `"Multi-select mode"` label.
- [ ] On a **supported** terminal (`ghosttyIdentity()`), `m.sessionsHelpKeymap()` contains the `Key == "m"` entry, and the rendered help body lists `"Multi-select mode"`.
- [ ] On a resolved-unsupported terminal **while in multi-select mode** (A1 in-flight-entered — set `m.multiSelectMode = true`), `m.sessionsHelpKeymap()` **lists** `m` (the working row-toggle is never hidden).
- [ ] The condensed Sessions footer never lists `m` under any resolution (`m` is non-`Core`) — assert the rendered `renderSessionsFooter(...)` output contains no `multi-select` label with detection supported and unsupported.
- [ ] `sessionsKeymap()` remains a pure static constant (unchanged) — only the call-site copy is filtered.
- [ ] `TestSessionsDescriptorDispatchParity` stays green (`go test ./internal/tui -run TestSessionsDescriptorDispatchParity`) — the guard runs with detection unwired, so the filter is inert and the static descriptor still advertises `m`.
- [ ] The Projects help call site is unchanged; `projectsKeymap()` is unaffected.
- [ ] `go test ./internal/tui/...` passes (unit lane — no tmux daemon).

**Tests**:
- `"it omits the m row from Sessions help on an unsupported terminal not in multi-select (named)"`
- `"it omits the m row from Sessions help on a NULL/remote terminal not in multi-select"`
- `"it lists the m row in Sessions help on a supported terminal"`
- `"it lists the m row in Sessions help when unsupported but already in multi-select (A1)"`
- `"the Sessions footer omits m under both supported and unsupported resolutions"`
- `"the descriptor↔dispatch guard stays green with detection unwired"` (existing `TestSessionsDescriptorDispatchParity` as a regression guard).

**Edge Cases**:
- **Unsupported + not in multi-select → `m` omitted**: the only state where `m` is actually blocked (Task 2.2), so help hides it. Covers both NULL and named identity shapes (the predicate is `DetectUnsupported()`, identity-blind).
- **Supported → `m` listed**: `DetectUnsupported()` false → filter inert → help lists `m` as today.
- **Unsupported + in multi-select (A1 in-flight-entered) → `m` listed**: `!m.multiSelectMode` is false → filter inert → `m` stays listed because it is a live row-toggle in the mode (spec §4 "Consistency with A1"). Keeps the rule "help never hides a working key."
- **Footer unchanged (`m` non-`Core`)**: `renderCondensedFooter` renders only `Core` entries; `m` is non-`Core`, so the footer never listed it and needs no change under any resolution.
- **`sessionsKeymap()` stays static (call-site filter only)**: parameterising the descriptor is rejected — it would break the guard, which relies on the static descriptor advertising `m` against an unwired-detection probe.
- **`keymap_dispatch_guard_test` stays green (detection unwired)**: `sessionsGuardModel` (`NewModelWithSessions`) leaves detection unwired, so `DetectUnsupported()` is false in the probe, the filter is inert, and the `m` dispatch probe still enters the mode.
- **Projects help call site untouched**: only the Sessions `modalHelp` branch changes; the Projects branch keeps `projectsKeymap()` verbatim (Projects has no `m` entry regardless).

**Context**:
> Spec §4 (Sub-fix 3 — Help-Modal `m`-Suppression): "When `DetectUnsupported() && !m.multiSelectMode` is true, filter the `m` (multi-select) entry out of the keymap descriptor slice passed to the help modal **at the call site** (`renderHelpModalOnClearedCanvas`, `internal/tui/model.go`). `sessionsKeymap()` itself stays a pure static constant — the filter is applied to the copy fed to the modal, not baked into the descriptor function."
> Spec §4 (Consistency with A1): "The filter is gated on `!m.multiSelectMode` as well as `DetectUnsupported()` so the rule is exactly '`m` appears in `?` help iff `m` is functional.' A1 (§3) permits a state where detection resolves unsupported *while* multi-select is already open ... in that state `m` is a live row-toggle, so it stays listed in help. ... (The extra `&& !m.multiSelectMode` is guard-safe — `keymap_dispatch_guard_test` probes with detection unwired, so `DetectUnsupported()` is false and the filter is inert regardless.)"
> Spec §4 (Why call-site filter, not a parameterised keymap): "A parameterised `sessionsKeymap()` ... is **rejected**. `keymap_dispatch_guard_test.go` probes the *static* descriptor against an unwired-detection model ... A call-site filter leaves that static descriptor — and therefore the guard — green; parameterising the descriptor would break it."
> Spec §4 (Footer unchanged): "`m` is a non-`Core` descriptor entry, so `renderCondensedFooter` never lists it — the footer needs no change under any resolution."
> Mechanics: the Sessions help call site is `internal/tui/model.go` ~L4547 (`case modalHelp:` → `renderHelpModalOnClearedCanvas(sessionsKeymap(), …)`); the Projects sibling is ~L4365. `sessionsKeymap()` (`internal/tui/keymap.go` ~L89) lists the `m` entry `{Key: "m", Action: "multi-select", HelpAction: "Multi-select mode"}` (non-`Core`, non-`RightAligned`). `helpModalBodyRows` (`internal/tui/help_modal.go`) renders every non-`RightAligned` entry's `HelpAction`; the `?` self-entry is skipped there. The guard `TestSessionsDescriptorDispatchParity` (`internal/tui/keymap_dispatch_guard_test.go`) drives `sessionsGuardModel(t)` (`NewModelWithSessions`, detection unwired) and asserts the `m` probe enters the mode.

**Spec Reference**: `/Users/leeovery/Code/portal/.workflows/persistent-no-host-terminal-banner/specification/persistent-no-host-terminal-banner/specification.md` §4 (Sub-fix 3 — Help-Modal `m`-Suppression), §7 (Testing Requirements — Help suppression, cover cases a/b/c + guard stays green).
