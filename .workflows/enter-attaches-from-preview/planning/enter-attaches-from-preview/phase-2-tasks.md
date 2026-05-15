---
phase: 2
phase_name: Session-killed-externally bail path with inline flash
total: 6
---

## enter-attaches-from-preview-2-1 | approved

### Task 2-1: Add flash state fields and setFlash/clearFlash helpers to Sessions page model

**Problem**: Phase 1's bail handler (task 1-7) flips back to the Sessions page and refreshes the list, but the Sessions page model has nowhere to store the user-facing flash text the spec mandates (`session "<name>" no longer exists`). Without dedicated state on the model — text plus a generation counter for the rapid-bail replacement contract (spec § Replacement on rapid successive bails) — there is no foundation for the render row (task 2-2), the tick auto-clear (task 2-3), the keystroke clear (task 2-4), the bail dispatch (task 2-5), or the rapid-bail replacement semantics (task 2-6) to build on. A small, focused state-and-helpers task lets the rest of Phase 2 move in clean, isolated TDD cycles.

**Solution**: Add three fields to the Sessions page model — `flashText string`, `flashGen uint64` (monotonic generation counter), and `flashActive bool` (or omit and use `flashText != ""` as the active signal — pick one canonical signal at implementation time and document it in the godoc). Add two helper methods on the model: `setFlash(text string)` increments the generation, sets the text, marks active; `clearFlash()` zeros text and active-flag and leaves generation as-is (so any in-flight tick from the cleared flash compares unequal against future generations). The generation counter is the spec's "build-phase shape (e.g. monotonic flash ID, single-shot tick handle, generation counter)" choice — pinned here so subsequent tasks consume a stable contract.

**Outcome**: The Sessions page model carries flash state with a documented contract: `setFlash` always supersedes any prior flash (text + generation bump); `clearFlash` is idempotent; the generation never decreases; a stale-generation tick (task 2-3) cannot mutate state set by a newer `setFlash` call. Zero-value model has no flash. No render or tick scheduling happens in this task — those are tasks 2-2 and 2-3.

**Do**:
- Locate the Sessions page model. The TUI's Sessions page state is currently held on `tui.Model` itself (the page-state machine in `internal/tui/model.go`), not in a separate sub-model file. Add the new fields to `tui.Model` next to existing Sessions-page-scoped fields (e.g. near the filter input / list state — search for existing `sessions`-prefixed fields in `internal/tui/model.go` and group them accordingly). If there is a clearly-scoped struct (e.g. `sessionsPageModel`) already extracted, prefer that; otherwise add to `tui.Model` directly. Document the placement choice in a one-line godoc.
- Add fields:
  ```go
  // flashText is the active inline flash message rendered between the
  // filter input and the Sessions list. Empty string means no flash.
  flashText string
  // flashGen is a monotonic counter incremented on every setFlash call.
  // It backs the rapid-bail replacement contract: tick handlers compare
  // their captured generation against this value before clearing, so a
  // stale tick from a superseded flash never clears a newer flash early.
  flashGen uint64
  ```
  Decide whether to add a separate `flashActive bool` or rely on `flashText != ""` as the active signal. Recommendation: rely on `flashText != ""` to keep state minimal and avoid two-fields-must-agree drift. Document the decision.
- Add helpers in the same file (unexported):
  ```go
  // setFlash replaces the active flash text and bumps the generation.
  // The bump invalidates any in-flight tick from the prior flash.
  func (m *Model) setFlash(text string) {
      m.flashGen++
      m.flashText = text
  }

  // clearFlash zeros the visible flash but does NOT decrement the
  // generation. An in-flight tick from the cleared flash will compare
  // unequal against any future setFlash call's generation.
  func (m *Model) clearFlash() {
      m.flashText = ""
  }
  ```
- Add a unit test file `internal/tui/sessions_flash_state_test.go` (new) covering:
  - Zero-value model: `flashText == ""`, `flashGen == 0`.
  - `setFlash("hello")`: text set, gen incremented to 1.
  - Successive `setFlash` calls: gen monotonically increments (1 → 2 → 3); text reflects latest.
  - `clearFlash()`: text zeroed; gen unchanged.
  - `clearFlash()` on already-cleared model: idempotent — text stays empty, gen unchanged.
  - `setFlash("a")` then `clearFlash()` then `setFlash("b")`: gen ends at 2; text == "b".

**Acceptance Criteria**:
- [ ] `tui.Model` (or the Sessions sub-model, depending on placement) carries `flashText string` and `flashGen uint64` fields.
- [ ] Zero-value model has `flashText == ""` and `flashGen == 0`.
- [ ] `setFlash(text)` sets `m.flashText = text` and increments `m.flashGen` by exactly 1.
- [ ] `clearFlash()` sets `m.flashText = ""` and does NOT modify `m.flashGen`.
- [ ] Both helpers are unexported (Sessions-page-internal contract; the bail handler in task 2-5 calls them from the same package).
- [ ] No render or scheduling logic introduced — this task is pure state plumbing.

**Tests**:
- `"zero-value Model has no active flash"`
- `"setFlash sets text and increments generation"`
- `"successive setFlash calls increment generation monotonically"`
- `"clearFlash zeros text and leaves generation unchanged"`
- `"clearFlash on already-cleared model is idempotent"`
- `"setFlash then clearFlash then setFlash preserves monotonic generation"`

**Edge Cases**:
- `setFlash("")` with empty text: still bumps generation. The empty string is technically a "no-flash" state, but the helper's contract is "I am a state mutation primitive, the caller decides what counts as a flash". The bail handler in task 2-5 will never pass empty text (the spec wording always interpolates a name). No defensive guard.
- Generation counter overflow at `math.MaxUint64`: not a concern in practice (would require ~10^19 successive bails). uint64 is sized for "effectively infinite" without wraparound bookkeeping.
- Concurrent access: the Sessions page model lives on `tui.Model`, which is mutated only inside `Update`. Bubble Tea serialises Update calls; no mutex needed.

**Context**:
> Spec § Inline flash — feature-local infrastructure: "State: a small piece of model state on the Sessions page model — at minimum an active flash text string and an associated timestamp or tick handle."
>
> Spec § Replacement on rapid successive bails: "Build-phase shape (e.g. monotonic flash ID, single-shot tick handle, generation counter) is a build decision; the spec-level constraint is 'latest bail wins, prior pending tick must not clear the new flash early'."
>
> The chosen mechanism for this work unit is a monotonic uint64 generation counter on the model. Tick handlers (task 2-3) capture the generation at schedule time and compare on fire; a tick whose captured generation differs from the current `flashGen` is a no-op. This makes "latest bail wins" automatic without explicit cancellation of in-flight ticks.

**Spec Reference**: `.workflows/enter-attaches-from-preview/specification/enter-attaches-from-preview/specification.md` § Inline flash — feature-local infrastructure, § Replacement on rapid successive bails

---

## enter-attaches-from-preview-2-2 | approved

### Task 2-2: Render conditional flash row between filter input and Sessions list

**Problem**: Task 2-1 added flash state but no render. The user cannot see the flash text without a corresponding View-side change. Spec § Inline flash — feature-local infrastructure mandates the flash render as a single chrome line "between the filter input and the Sessions list", with the constraint that **no row is reserved when the flash is inactive** — the list sits directly under the filter input as today, and the first bail visually pushes the list down by one row. This precludes a permanently-reserved row that toggles between "filled" and "blank" — the row must be conditionally absent, not conditionally empty.

**Solution**: Update the Sessions page View rendering (in `internal/tui/model.go` View / pageSessions branch) to insert a conditional flash row between the existing filter input render and the Sessions list render. When `m.flashText == ""`, no row is emitted (no spacer, no styled blank — the list rendering follows the filter input directly). When `m.flashText != ""`, one styled line is emitted carrying the flash text, then the list rendering follows. The list itself is not modified; the flash row is purely additive layout.

**Outcome**: With no flash active, the Sessions page renders byte-identically to today (filter input → list, no visible difference). With a flash active, exactly one row appears between filter input and list carrying the flash text. The list shifts down by exactly one row when a flash appears and shifts back up by one row when it clears. No existing chrome row is replaced or overlaid.

**Do**:
- Locate the Sessions page View branch in `internal/tui/model.go` (search for the `pageSessions` View case or equivalent — the page-state machine dispatches View by `activePage`).
- Identify where the filter input is currently rendered and where the list is currently rendered. Insert the flash row between them.
- Implementation shape:
  ```go
  // existing filter input render
  parts := []string{filterInputView}
  if m.flashText != "" {
      parts = append(parts, m.renderFlashRow())
  }
  parts = append(parts, listView)
  return lipgloss.JoinVertical(lipgloss.Left, parts...)
  ```
  (Adapt to the existing rendering style — if the file already uses `strings.Builder` or direct concatenation, follow that convention. The conditional-append shape is the load-bearing part.)
- Add `(m *Model) renderFlashRow() string` helper that styles the flash text. Style choice is open — recommend a subdued/dimmed style consistent with other Sessions-page chrome (see existing `lipgloss.Style` definitions near the top of `internal/tui/model.go` or in a sibling style file). The text must be the raw `m.flashText` — no truncation, no padding except as needed for visual breathing room.
- Confirm no existing chrome row is removed or overlaid. The filter input keeps its existing position; the list keeps its existing rendering. Only the new conditional row is added.
- Write tests in `internal/tui/sessions_flash_render_test.go` (new):
  - `"View has no flash row when flashText is empty"` — render Sessions page with `flashText = ""`, assert the rendered string contains the filter input followed directly by the list (no intervening flash row). Use a substring distance check: filter-input-marker-line-N, list-first-line-N+1 (or N+2 if a blank-line separator is part of existing chrome — adapt to actual layout).
  - `"View has one flash row between filter input and list when flashText is set"` — render with `flashText = "hello"`, assert the rendered string contains the text "hello" on a line between filter input and list.
  - `"flash row push: list first line moves down by exactly one row when flash activates"` — render twice (with and without flash), compare line indices of the first list row, assert delta is exactly 1.
  - `"flash row pop: list first line moves up by exactly one row when flash clears"` — symmetric.
  - `"flash text is rendered verbatim with no truncation"` — set a long-ish flash like `session "really-long-name" no longer exists`, assert the full text appears in the output.
- Verify other pages' Views (`pageProjects`, `pageFileBrowser`, `pagePreview`) are unaffected — flash is a Sessions-page concept only.

**Acceptance Criteria**:
- [ ] When `m.flashText == ""`, the Sessions page View output contains no flash row — the list's first line follows the filter input's last line at the same vertical distance as before this task.
- [ ] When `m.flashText != ""`, exactly one flash row is rendered between the filter input and the list, carrying the flash text verbatim.
- [ ] Activating a flash shifts the list down by exactly one row; clearing it shifts the list back up by exactly one row. No "permanently reserved blank row" pattern.
- [ ] No existing chrome row (filter input, list header, list rows, help bar) is replaced or overlaid by the flash row.
- [ ] The flash row appears below the filter input and above the list — never above the filter input, never below the list, never interleaved with list rows.
- [ ] Other pages (Projects, FileBrowser, Preview) render byte-identically to before this task.

**Tests**:
- `"Sessions View omits flash row when flashText is empty"`
- `"Sessions View renders flash row between filter input and list when flashText is set"`
- `"flash row activation shifts list down by exactly one row"`
- `"flash row deactivation shifts list up by exactly one row"`
- `"flash text appears verbatim in rendered output"`
- `"non-Sessions pages are unaffected by flash state"` — set flashText, render Projects page, assert flash text does not appear.

**Edge Cases**:
- Very narrow terminal: the flash text may visually wrap at the terminal edge. Acceptable — the spec does not mandate single-line truncation. The chrome row remains "logically one row" in the layout; visual wrap is a terminal concern.
- Flash text containing a literal newline (would not happen via `setFlash` from the bail handler, but defensive): treat as the same chrome row. If a newline appears in `flashText`, the renderer joins it as-is — acceptable since the bail dispatch (task 2-5) controls the wording.
- Flash style and Sessions-page filter input style coexistence: both render in the same view; ensure no style bleed (e.g. background-color leak). Use a fresh `lipgloss.NewStyle()` for the flash row, not a shared style.
- WindowSizeMsg layout: when the terminal resizes, the View re-renders fresh — the flash row count is recomputed from `flashText != ""` each render. No stale layout state.

**Context**:
> Spec § Inline flash — feature-local infrastructure > Render: "a single chrome line rendered between the filter input and the Sessions list. The flash sits adjacent to the list it is describing — the filter input remains in its existing position above the flash, and no existing chrome row is replaced or overlaid. The flash row is rendered **only when a flash is active**; when no flash is active, no row is reserved (the list sits directly under the filter input as today). First bail visually pushes the list down by one row; tick expiry or the next clearing keystroke pops it back up."

**Spec Reference**: `.workflows/enter-attaches-from-preview/specification/enter-attaches-from-preview/specification.md` § Inline flash — feature-local infrastructure > Render

---

## enter-attaches-from-preview-2-3 | approved

### Task 2-3: Add flashTickMsg with generation guard for tick-based auto-clear

**Problem**: Spec § Inline flash > Clear conditions mandates the flash auto-clear "via a tick `tea.Cmd` after a short duration" with the principle "long enough to read, short enough not to linger". Without an auto-clear, the flash would persist indefinitely until the user pressed a clearing key (task 2-4) — a stale flash from minutes ago would still be on screen. Additionally, spec § Replacement on rapid successive bails mandates that "any pending tick from the prior flash is cancelled or otherwise prevented from firing against the new flash". The implementation must honour both: the tick fires, but only clears the flash it was scheduled for — never a newer flash.

**Solution**: Define a new internal message `flashTickMsg{ Gen uint64 }` and a helper `flashTickCmd(gen uint64, d time.Duration) tea.Cmd` that schedules `flashTickMsg{Gen: gen}` after `d` via `tea.Tick`. Add a top-level case `flashTickMsg` in `Model.Update` that compares `msg.Gen` against `m.flashGen`: if equal, `clearFlash()`; if not equal, no-op (the flash this tick was scheduled for has already been superseded by a newer `setFlash`). The generation guard is the "prior pending tick must not clear the new flash early" mechanism — ticks are not cancelled, they self-discriminate.

This task lands the tick infrastructure but does NOT yet schedule a tick from any caller. Task 2-5 (bail dispatch) and task 2-6 (rapid-bail replacement) wire the cmd from `setFlash` callers.

**Outcome**: A scheduled tick fires after the build-chosen duration; if the flash it was scheduled for is still active (generation matches), it clears; if a newer flash has superseded it (generation mismatch), it is silently dropped. The build-chosen tick duration honours the spec principle (~3s recommended).

**Do**:
- In `internal/tui/model.go` (or a new sibling file `internal/tui/sessions_flash_tick.go` if grouping by feature is preferred — recommend grouping with the flash state from task 2-1 in a sibling file `internal/tui/sessions_flash.go` to keep the feature-local infrastructure cohesive):
  - Define the message type:
    ```go
    // flashTickMsg fires after a flash's auto-clear duration. The Gen
    // field is captured at schedule time; if it does not match the
    // current m.flashGen on dispatch, the tick is a stale handle for
    // a superseded flash and is silently discarded.
    type flashTickMsg struct {
        Gen uint64
    }
    ```
  - Define the duration constant. Recommended value: `flashAutoClearDuration = 3 * time.Second`. Document the spec rationale: "long enough to read the message, short enough not to linger after the user has moved on". If a different value is chosen, justify in a code comment.
    ```go
    const flashAutoClearDuration = 3 * time.Second
    ```
  - Define the cmd factory:
    ```go
    func flashTickCmd(gen uint64) tea.Cmd {
        return tea.Tick(flashAutoClearDuration, func(time.Time) tea.Msg {
            return flashTickMsg{Gen: gen}
        })
    }
    ```
- Add the handler case in `Model.Update`'s message switch (in `internal/tui/model.go`):
  ```go
  case flashTickMsg:
      if msg.Gen == m.flashGen {
          m.clearFlash()
      }
      return m, nil
  ```
  Place it near other internal-message cases. Document the generation-guard rationale inline.
- Add tests in `internal/tui/sessions_flash_tick_test.go` (new):
  - `"flashTickMsg with current generation clears the flash"` — set flash via `setFlash` (gen becomes 1), capture gen, dispatch `flashTickMsg{Gen: 1}` through `Update`, assert `m.flashText == ""`.
  - `"flashTickMsg with stale generation does not clear the flash"` — set flash (gen=1), then setFlash again (gen=2), dispatch `flashTickMsg{Gen: 1}` (the stale tick), assert `m.flashText` is still set to the second flash's text.
  - `"flashTickMsg after manual clear is a no-op"` — setFlash (gen=1), clearFlash (text empty, gen still 1), dispatch `flashTickMsg{Gen: 1}` — `clearFlash` is called again on already-empty state, idempotent. Assert no panic, text still empty.
  - `"flashTickCmd schedules at flashAutoClearDuration"` — exercise the cmd via a fake `tea.Tick` (or directly construct and inspect the tea.Cmd shape). If `tea.Tick` is hard to mock, assert duration via the constant value rather than dynamic interception.
  - `"flashAutoClearDuration is a reasonable default"` — assert the constant is in the range [1s, 10s] as a sanity check pinning the spec principle.

**Acceptance Criteria**:
- [ ] `flashTickMsg` struct exists with a `Gen uint64` field.
- [ ] `flashTickCmd(gen)` returns a non-nil `tea.Cmd` that, when run, eventually emits `flashTickMsg{Gen: gen}`.
- [ ] `flashAutoClearDuration` constant exists and is documented; default value honours the spec's "long enough to read, short enough not to linger" principle (recommended `3 * time.Second`).
- [ ] `Model.Update` handles `flashTickMsg`: clears flash iff `msg.Gen == m.flashGen`, otherwise silent no-op.
- [ ] No caller schedules a tick yet — that wiring lives in tasks 2-5 and 2-6. This task is plumbing only.

**Tests**:
- `"flashTickMsg clears flash when generation matches"`
- `"flashTickMsg is a no-op when generation is stale"`
- `"flashTickMsg is a no-op after manual clearFlash"`
- `"flashTickCmd produces a tea.Cmd that emits flashTickMsg with the captured generation"`
- `"flashAutoClearDuration is in the spec-principle range"`

**Edge Cases**:
- Generation rollover: see task 2-1 — `uint64` is effectively infinite; not a concern.
- Tick fires after the model has been replaced (e.g. TUI lifecycle reset between sessions): Bubble Tea's lifecycle ensures the message routes to the current Model — if the Model has been re-created, its `flashGen` starts at 0, the tick's captured gen will not match, and the tick is silently dropped. Safe by construction.
- Multiple ticks in flight (rapid bails before any auto-clear): each tick has its own captured generation; only the most recent will match `m.flashGen` (since each setFlash bumps gen). All older ticks are discarded on dispatch. Verified by task 2-6's replacement test.
- Zero-duration tick: not used. The constant is a non-zero positive duration. If someone passes `flashTickCmd(0)` to schedule with no delay, `tea.Tick(0, ...)` still works (fires near-immediately) but is not a use case this task introduces.

**Context**:
> Spec § Inline flash — feature-local infrastructure > Clear conditions: "A tick `tea.Cmd` after a short duration. Default principle: 'long enough to read, short enough not to linger'. Exact tick duration is a build-phase decision; the discussion noted `~3s` as a reasonable default."
>
> Spec § Replacement on rapid successive bails: "Any pending tick from the prior flash is cancelled or otherwise prevented from firing against the new flash. Build-phase shape (e.g. monotonic flash ID, single-shot tick handle, generation counter) is a build decision; the spec-level constraint is 'latest bail wins, prior pending tick must not clear the new flash early'."
>
> Mechanism chosen: generation-guarded ticks. Ticks are not cancelled (Bubble Tea has no first-class cancellation primitive for `tea.Tick`); instead they self-discriminate on dispatch. This is the simplest correct implementation of the spec contract.

**Spec Reference**: `.workflows/enter-attaches-from-preview/specification/enter-attaches-from-preview/specification.md` § Inline flash — feature-local infrastructure > Clear conditions, § Replacement on rapid successive bails

---

## enter-attaches-from-preview-2-4 | approved

### Task 2-4: Clear flash on actionable KeyMsg without swallowing keystroke

**Problem**: Spec § Inline flash > Clear conditions mandates the flash clears on "the next `tea.KeyMsg` (actionable keystroke)". Spec § Flash interaction with filter input refines this: "The first keystroke post-bail clears the flash AND applies to the filter input as normal — **one key, one intent**. The flash does not swallow the keystroke on the user's behalf." Additionally, modifier-only events, resize events, and focus events do NOT count as clearing events. Without this handler, the flash would only clear via the tick (task 2-3) or the next bail — not on the very natural "user starts typing" signal.

**Solution**: In the Sessions-page `KeyMsg` handling path, add a check at the top: if `m.flashText != ""` and the incoming key is an actionable keystroke (any key with a non-empty `tea.KeyMsg.String()` representation OR any key with `Type != tea.KeyMsg{}.Type` zero-value — pick the discriminator that correctly excludes modifier-only events; see Do section), call `m.clearFlash()`. **Then continue processing the key normally** — the same key still flows into the filter input handler / list keybindings as it would have without the flash. The flash clear is a side effect, not a consumption.

`WindowSizeMsg` and focus messages (`tea.FocusMsg`, `tea.BlurMsg`) are non-`KeyMsg` types and naturally do not enter the `KeyMsg` branch — they require no special handling, but the test suite must verify they do not clear the flash.

**Outcome**: After a bail, the user types `f` to filter — the flash clears AND the `f` lands in the filter input as the first character of their query. Resizing the terminal does not clear the flash. Holding shift alone does not clear the flash. The user's first intentional keystroke is the clearing signal.

**Do**:
- Locate the Sessions-page `KeyMsg` handler in `internal/tui/model.go` (the `case tea.KeyMsg` branch within the `pageSessions` activePage switch, or wherever Sessions-page key handling currently lives).
- At the top of the Sessions-page KeyMsg handling — BEFORE the existing filter-input dispatch and list-keybinding dispatch — add:
  ```go
  if m.flashText != "" && isActionableKey(msg) {
      m.clearFlash()
      // Fall through — the keystroke itself continues to its normal
      // handler (filter input, list binding, etc.). Flash clear is a
      // side effect, not a consumption.
  }
  ```
- Define `isActionableKey(msg tea.KeyMsg) bool` as a small helper. The discriminator question: what counts as "modifier-only"? Bubble Tea's `tea.KeyMsg` does not deliver modifier-only events (e.g. shift held alone) as a `KeyMsg` — the runtime only emits `KeyMsg` when a key with a meaningful resolution is pressed. In practice, any `tea.KeyMsg` reaching the handler is actionable. Defensive shape:
  ```go
  func isActionableKey(msg tea.KeyMsg) bool {
      // Bubble Tea does not emit KeyMsg for modifier-only presses
      // (shift-alone, ctrl-alone). Any KeyMsg reaching here is the
      // result of a meaningful keypress and counts as actionable.
      // The defensive check below excludes the zero-value KeyMsg
      // (which should never arrive in practice but guards against
      // future runtime changes).
      return msg.Type != 0 || len(msg.Runes) > 0
  }
  ```
  If the project has an existing convention for "actionable key" detection, prefer it over reinventing. Document the chosen shape inline.
- Confirm `WindowSizeMsg`, `tea.FocusMsg`, `tea.BlurMsg`, and `MouseMsg` are NOT routed through the `KeyMsg` branch — they are distinct message types with their own (or absent) handlers. No code change needed for them; the test suite verifies they don't clear the flash.
- Verify the existing Esc / Enter / `r` / `k` / arrow-key handlers still work correctly — the flash clear runs before them and does not consume the message, so their behaviour is unchanged. Specifically, Esc on Sessions page (which currently exits the TUI or returns to a parent page) must still work after a bail-induced flash.
- Add tests in `internal/tui/sessions_flash_clear_test.go` (new):
  - `"first keystroke after flash clears the flash"` — `setFlash("hello")`, dispatch `tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}`, assert `m.flashText == ""`.
  - `"first keystroke after flash also lands in filter input"` — wire a fake or real filter input model, dispatch `'a'` as above, assert filter input now contains `"a"`. This is the load-bearing "one key, one intent" assertion.
  - `"WindowSizeMsg does not clear flash"` — setFlash, dispatch `tea.WindowSizeMsg{Width: 100, Height: 50}`, assert flash still present.
  - `"FocusMsg does not clear flash"` — setFlash, dispatch `tea.FocusMsg{}` (or whatever the runtime uses), assert flash still present.
  - `"BlurMsg does not clear flash"` — symmetric.
  - `"MouseMsg does not clear flash"` — setFlash, dispatch a `tea.MouseMsg`, assert flash still present.
  - `"second keystroke after flash is normal — flash already gone"` — setFlash, dispatch `'a'` (clears flash), dispatch `'b'`, assert filter input contains `"ab"` and flash still empty.
  - `"keystroke when no flash is active is a normal keystroke"` — no flash, dispatch `'a'`, assert filter input contains `"a"`. Regression that the helper does not introduce overhead/side-effects when no flash is active.

**Acceptance Criteria**:
- [ ] When `m.flashText != ""` and an actionable `tea.KeyMsg` arrives on the Sessions page, `m.clearFlash()` runs once.
- [ ] After the flash-clear, the same `KeyMsg` continues to its normal handler (filter input, list binding, etc.). The keystroke is NOT consumed by the flash-clear path.
- [ ] `WindowSizeMsg`, `tea.FocusMsg`, `tea.BlurMsg`, and `MouseMsg` do NOT clear the flash (verified by test).
- [ ] When no flash is active, the new code path is a near-zero-cost no-op (single bool check) and does not alter normal key handling.
- [ ] `isActionableKey` (or equivalent inline check) is documented and small (one screen of code).

**Tests**:
- `"first keystroke clears flash AND lands in filter input"`
- `"WindowSizeMsg does not clear flash"`
- `"FocusMsg / BlurMsg do not clear flash"`
- `"MouseMsg does not clear flash"`
- `"keystroke when no flash is active is a normal keystroke"`
- `"successive keystrokes after flash all land normally"`
- `"flash-clearing keystroke also reaches list bindings (e.g. arrow)"` — setFlash, dispatch `tea.KeyMsg{Type: tea.KeyDown}`, assert flash cleared AND list cursor moved (or whatever the existing arrow-key handler does).

**Edge Cases**:
- Esc on Sessions page (which today exits the TUI or returns to parent) with an active flash: the flash clears AND Esc still does its normal action. The user does not have to press Esc twice. Verified by adding an Esc test if helpful.
- Enter on Sessions page (attaches to the highlighted session) with an active flash: the flash clears AND the attach proceeds normally. Verified by an Enter test if helpful.
- Rapid bursts (user types 5 characters in 50ms): the first character clears the flash; characters 2-5 land normally (no flash to clear). No re-entrancy concerns — `clearFlash` is idempotent and Bubble Tea serialises `Update`.
- Bail arrives concurrently with a keystroke: Bubble Tea serialises message processing. If the keystroke is processed first, the (still-empty) flash is not cleared and the bail dispatches normally. If the bail is processed first, the next keystroke clears the bail's flash. Either order is correct; no race.

**Context**:
> Spec § Inline flash — feature-local infrastructure > Clear conditions:
> - "The next `tea.KeyMsg` (actionable keystroke) — see *Flash interaction with filter input* below."
> - "Modifier-only events (e.g. holding shift alone), resize events, and focus events do NOT count as clearing events."
>
> Spec § Flash interaction with filter input: "The first keystroke post-bail clears the flash AND applies to the filter input as normal — **one key, one intent**. The flash does not swallow the keystroke on the user's behalf. If the user starts typing into the filter immediately after the bail, those characters land in the filter input as they would on any other Sessions-page render; the flash simply clears."

**Spec Reference**: `.workflows/enter-attaches-from-preview/specification/enter-attaches-from-preview/specification.md` § Inline flash — feature-local infrastructure > Clear conditions, § Flash interaction with filter input

---

## enter-attaches-from-preview-2-5 | approved

### Task 2-5: Replace placeholder previewAttachBailMsg handler with refresh + exact-text flash dispatch

**Problem**: Phase 1 task 1-7 landed the placeholder `previewAttachBailMsg` handler that flips to PageSessions and dispatches the refresh — but emits no flash. The user lands on a refreshed list with no explanation of why their Enter "didn't work". Spec § Session-killed-externally bail path > Behaviour mandates the bail emit an inline flash with the exact text `session "<name>" no longer exists` (double quotes around the name, no trailing punctuation, no paraphrase). The wording is fixed by the spec — build phase must not paraphrase.

**Solution**: Replace the body of the existing `case previewAttachBailMsg` in `Model.Update` (added in task 1-7) with the full bail behaviour: compose the exact spec text via a small helper, call `m.setFlash(...)` (from task 2-1), schedule the auto-clear tick `flashTickCmd(m.flashGen)` (from task 2-3), and batch it with the existing `refreshSessionsAfterPreviewCmd` so both run from the same Update return. The transition + zero-out + refresh shape from task 1-7 is preserved; the flash and tick are added.

**Outcome**: When `has-session` reports the previewed session is gone, the TUI flips to PageSessions, refreshes the list (killed session removed), and renders the flash `session "<name>" no longer exists` between the filter input and the list. The flash clears after ~3s or on the user's next keystroke, whichever comes first.

**Do**:
- Edit `internal/tui/model.go` `Model.Update` — locate the `case previewAttachBailMsg` added in task 1-7 (around the same area as `previewDismissedMsg`).
- Replace the placeholder body. New shape:
  ```go
  case previewAttachBailMsg:
      preserveName := msg.Session
      m.activePage = PageSessions
      m.preview = previewModel{}
      m.setFlash(formatSessionGoneFlash(preserveName))
      tickCmd := flashTickCmd(m.flashGen)
      refreshCmd := m.refreshSessionsAfterPreviewCmd(preserveName)
      return m, tea.Batch(refreshCmd, tickCmd)
  ```
  Note: `setFlash` is called BEFORE `flashTickCmd` is composed, so the captured `m.flashGen` matches the just-bumped generation. Order matters.
- Add a small helper to compose the flash text — fixed by spec, must not paraphrase:
  ```go
  // formatSessionGoneFlash returns the exact spec-pinned wording for
  // the session-killed-externally bail flash. Wording is fixed:
  // double quotes around the name, no trailing punctuation. Build
  // phase must not paraphrase per spec § Session-killed-externally
  // bail path > Behaviour.
  func formatSessionGoneFlash(name string) string {
      return fmt.Sprintf(`session "%s" no longer exists`, name)
  }
  ```
  Place adjacent to the flash helpers from task 2-1.
- Verify `tea.Batch` is the right composer. The two cmds (refresh and tick) are independent and should both run; `tea.Batch` runs them concurrently. Do NOT use `tea.Sequence` — sequencing would delay the tick until the refresh resolves, blowing the spec's "render not gated on refresh completion" guarantee.
- Verify the render-frame ordering contract (spec § Render-frame ordering): the transition, refresh dispatch, and flash emission share a single Bubble Tea cycle on the dispatch side (all happen inside one `Update` return). The refresh is async — a brief render frame may show the freshly-transitioned Sessions page with the prior list state plus the flash, followed by a second render once the refresh resolves. **The build phase MUST NOT gate the flash render on refresh completion.** The implementation above honours this — `setFlash` is a synchronous mutation; the next render after the Update return shows the flash regardless of whether `previewSessionsRefreshedMsg` has dispatched yet.
- If `refreshSessionsAfterPreviewCmd` can return nil (e.g. when the session lister is not wired in tests), ensure `tea.Batch(nil, tickCmd)` is safe. Bubble Tea's `tea.Batch` filters nils — verify against the current bubbletea version. If not, conditionally compose:
  ```go
  cmds := []tea.Cmd{tickCmd}
  if refreshCmd != nil { cmds = append(cmds, refreshCmd) }
  return m, tea.Batch(cmds...)
  ```
- Update tests from task 1-7 — the placeholder tests asserted no flash. Now those assertions must flip:
  - `"previewAttachBailMsg sets flash with exact spec wording"` — dispatch bail with session "foo", assert `m.flashText == "session \"foo\" no longer exists"` after Update.
  - `"previewAttachBailMsg flash uses double quotes around the session name"` — assert the rendered text contains `"foo"` with literal double-quote characters.
  - `"previewAttachBailMsg flash has no trailing punctuation"` — assert the text does NOT end in `.`, `!`, or `?`.
  - `"previewAttachBailMsg dispatches the auto-clear tick"` — assert the returned cmd, when run, eventually produces a `flashTickMsg` with the bail's generation.
  - `"previewAttachBailMsg dispatches refresh and tick from the same Update return"` — assert returned cmd is non-nil and inspecting its result (or its composition) shows both cmds present.
  - `"previewAttachBailMsg flash render is not gated on refresh completion"` — dispatch bail, do NOT dispatch the refresh-result message, render View, assert flash text is visible. (Proves the flash appears before refresh resolves.)
  - `"previewAttachBailMsg flash text preserves session name verbatim including special chars"` — bail with session name `"my-session_v2"`, assert flash contains exactly `session "my-session_v2" no longer exists`.
  - `"previewAttachBailMsg with empty session name still composes a flash"` — defensive — composes `session "" no longer exists`. (Should not happen in practice since the pipeline only dispatches with a real captured name, but the helper must not panic.)
- Preserve all existing task 1-7 assertions where they are still valid:
  - PageSessions transition still happens.
  - `m.preview` is still zeroed.
  - Refresh cmd is still dispatched.
  - Esc-dismiss path is unaffected.

**Acceptance Criteria**:
- [ ] `case previewAttachBailMsg` calls `m.setFlash(formatSessionGoneFlash(msg.Session))`.
- [ ] The flash text is exactly `session "<name>" no longer exists` — double quotes around the name, no trailing punctuation, no paraphrase. Verified by string-equality test, not substring.
- [ ] The handler returns `tea.Batch(refreshCmd, tickCmd)` (or the nil-safe equivalent) — both cmds are dispatched from the same Update return.
- [ ] `flashTickCmd` is composed with the post-`setFlash` value of `m.flashGen` so the tick matches its own flash generation.
- [ ] Flash render is observable on the next View call WITHOUT the refresh-result message having been processed (proven by test).
- [ ] All task 1-7 assertions that remain valid (transition, preview zero, refresh dispatch, Esc unaffected) still pass.
- [ ] `tea.Sequence` is NOT used — refresh and tick must not be ordered.

**Tests**:
- `"bail handler sets flash to exact spec wording"`
- `"bail handler flash text uses literal double quotes around session name"`
- `"bail handler flash text has no trailing punctuation"`
- `"bail handler dispatches refresh AND tick in single Update return"`
- `"bail handler flash visible before refresh resolves"`
- `"bail handler preserves session name verbatim with special characters"`
- `"bail handler with empty session name does not panic"` — defensive.
- `"bail handler still transitions to PageSessions"` — regression.
- `"bail handler still zeros m.preview"` — regression.

**Edge Cases**:
- Session name containing double quotes (e.g. `my"weird"name`): the format string `session "%s" no longer exists` would produce `session "my"weird"name" no longer exists` — visually parseable but quote-broken. Portal's session-name generator (`{project}-{nanoid}`) does not produce quotes, so this is a defensive concern only. No special escaping required — the spec wording is fixed and does not call for escaping. Document the limitation in a code comment if it surfaces.
- Session name containing a newline (impossible from portal's generator): would render across two flash rows. Since portal-generated names don't contain newlines, no defensive handling.
- Refresh cmd is nil (test harness without lister): `tea.Batch` handles nils gracefully in current bubbletea versions; if not, the conditional compose pattern in Do covers it. The tick still fires, the flash still renders.
- Bail arrives while a prior flash is still visible: covered by task 2-6. This task's handler simply calls `setFlash`, which monotonically bumps the generation — task 2-6 verifies the end-to-end replacement contract.
- TOCTOU between has-session and connector (spec § Accepted residual): outside this task's scope. The pipeline's has-session check has already returned absent by the time this handler runs; the residual is about the time between has-session-returns-zero and connector-fires, which is a Phase 1 path, not the bail path.

**Context**:
> Spec § Session-killed-externally bail path > Behaviour: "Emits an inline flash message — one ephemeral line pinned above the Sessions list. Exact wording is fixed by this spec:
>
> ```
> session "<name>" no longer exists
> ```
>
> The `<name>` placeholder is the captured session name. Double quotes surround the name. No trailing punctuation. Build phase must not paraphrase the message."
>
> Spec § Render-frame ordering: "The transition, refresh dispatch, and flash emission are issued from the same `Update` return — they share a single Bubble Tea cycle on the dispatch side. The refresh itself may be asynchronous (returning a later `sessionsLoadedMsg`); in that case a brief render frame may show the freshly-transitioned Sessions page with the **prior** list state plus the flash, followed by a second render once the refresh resolves. This transient stale-row frame is **acceptable** — the flash text always reflects the bail, and the killed-session row is removed by the next render. The build phase MUST NOT gate the flash render on refresh completion (which would delay the visible response to Enter)."

**Spec Reference**: `.workflows/enter-attaches-from-preview/specification/enter-attaches-from-preview/specification.md` § Session-killed-externally bail path > Behaviour, § Render-frame ordering

---

## enter-attaches-from-preview-2-6 | approved

### Task 2-6: Rapid-bail replacement resets text and supersedes prior tick via generation bump

**Problem**: Spec § Replacement on rapid successive bails mandates that "a new bail while a prior flash is still visible **replaces** the prior flash's text and **resets** the tick — the visible message always reflects the most recent bail. Any pending tick from the prior flash is cancelled or otherwise prevented from firing against the new flash." The infrastructure is already in place: task 2-1 made `setFlash` monotonically bump generation; task 2-3 made `flashTickMsg` self-discriminate on generation; task 2-5 schedules a fresh tick on every bail. This task is the **end-to-end verification** that the contract holds across multi-bail sequences and that no prior in-flight tick clears a newer flash early.

The risk this task addresses: even though each individual piece is correct in isolation, integration bugs can lurk (off-by-one in generation capture, ordering bug in setFlash-then-flashTickCmd, batched tick capturing pre-setFlash gen). A dedicated end-to-end test surface is the contract gate.

**Solution**: This task is primarily **test-driven verification** of the rapid-bail replacement contract. No new production code is expected if tasks 2-1 / 2-3 / 2-5 implemented their parts correctly — but the tests will surface any integration bug. If a bug is found, fix it in this task (typically a one-line ordering or generation-capture correction in task 2-5's handler, or an additional invariant in `setFlash`/`flashTickCmd`).

**Outcome**: A documented, tested invariant — "latest bail wins, prior pending tick is harmless" — backed by integration tests that drive multi-bail sequences and assert the visible text and tick semantics are correct. Future regressions would surface as test failures.

**Do**:
- Add an integration test file `internal/tui/sessions_flash_replacement_test.go` (new) covering the multi-bail scenarios end-to-end. Each test drives `Update` with `previewAttachBailMsg`s and `flashTickMsg`s in sequence, asserting model state at each step.
- Test scenarios:
  1. **Two bails in quick succession, second's text wins**:
     - Dispatch `previewAttachBailMsg{Session: "foo"}` → assert `m.flashText == \`session "foo" no longer exists\``, capture `gen1 := m.flashGen` (should be 1 from a fresh model).
     - Dispatch `previewAttachBailMsg{Session: "bar"}` → assert `m.flashText == \`session "bar" no longer exists\``, capture `gen2 := m.flashGen` (should be 2).
     - Assert `gen2 > gen1`.
  2. **Prior tick does not clear newer flash**:
     - Dispatch bail "foo" (gen=1). Capture the tick cmd from the returned `tea.Cmd` (or simulate by directly constructing `flashTickMsg{Gen: 1}`).
     - Dispatch bail "bar" (gen=2). Flash text now `bar`'s.
     - Dispatch the captured `flashTickMsg{Gen: 1}` (the prior, now-stale tick fires).
     - Assert `m.flashText` is STILL `bar`'s text — the stale tick was discarded.
     - Assert `m.flashGen == 2` — generation unchanged by the stale tick.
  3. **Newer tick still clears its own flash**:
     - Continuing from scenario 2: dispatch `flashTickMsg{Gen: 2}` — the live tick for the current flash.
     - Assert `m.flashText == ""` — flash cleared.
     - Assert `m.flashGen == 2` — generation NOT decremented (clearFlash leaves gen alone, per task 2-1 contract).
  4. **N successive bails preserve only the latest**:
     - Dispatch 5 bails with sessions `s1, s2, s3, s4, s5` in sequence.
     - Assert `m.flashText == \`session "s5" no longer exists\`` after the last.
     - Assert `m.flashGen == 5`.
     - Now dispatch all of `flashTickMsg{Gen: 1}`, `{Gen: 2}`, `{Gen: 3}`, `{Gen: 4}` — all stale.
     - Assert `m.flashText` STILL == s5's text after each.
     - Finally dispatch `flashTickMsg{Gen: 5}` — the live one.
     - Assert `m.flashText == ""`.
  5. **Manual clear (keystroke) followed by bail does not get confused by old ticks**:
     - Dispatch bail "foo" (gen=1). Capture tick.
     - Dispatch a clearing keystroke (from task 2-4) — flash now empty, gen still 1.
     - Dispatch bail "bar" (gen=2). Flash text `bar`'s.
     - Dispatch captured `flashTickMsg{Gen: 1}` — stale, no-op.
     - Assert `m.flashText == \`session "bar" no longer exists\``.
- If any test fails, the bug is in tasks 2-1 / 2-3 / 2-5. Common failure modes and fixes:
  - **Off-by-one in generation capture in task 2-5**: tick captures `m.flashGen` BEFORE `setFlash` increments it. Fix: ensure `setFlash` is called BEFORE `flashTickCmd(m.flashGen)` — already specified in task 2-5's Do section, but if a test fails, audit the ordering.
  - **`flashTickMsg` handler missing the generation guard**: task 2-3 defined this, but if the handler accidentally clears unconditionally, scenario 2 fails. Fix: re-add the `if msg.Gen == m.flashGen` guard.
  - **`setFlash` resetting generation instead of bumping**: task 2-1 specified bump-only. If a test shows generations not increasing, audit the helper.
- Document the verified invariant in a top-of-file comment in the test file:
  ```go
  // Tests in this file verify the spec § Replacement on rapid successive
  // bails contract end-to-end:
  //
  //   1. The visible flash always reflects the most recent bail.
  //   2. A pending tick from a superseded flash never clears a newer flash.
  //   3. The newest flash's own tick still clears it at the deadline.
  //
  // The mechanism is monotonic generation counters on the model
  // (setFlash bumps; tick handler compares; clearFlash leaves gen alone).
  // This file is the integration-level gate on that contract — unit
  // tests in sessions_flash_state_test.go and sessions_flash_tick_test.go
  // cover the individual pieces.
  ```
- If any production code changes are required during this task to make a test pass, document the change clearly with a code comment referencing the failing test scenario.

**Acceptance Criteria**:
- [ ] Two successive bails: second bail's text replaces first; generation increments.
- [ ] Prior in-flight tick (from a superseded flash) does NOT clear the newer flash when fired.
- [ ] Newer flash's own tick still clears it at its deadline.
- [ ] N successive bails (N ≥ 5) preserve only the latest flash; all stale ticks are silently discarded; the latest tick clears at its deadline.
- [ ] Manual keystroke clear interleaved with bails does not desynchronise generation.
- [ ] No new production code added IF tasks 2-1 / 2-3 / 2-5 are correct; if changes are needed, they are scoped corrections to those tasks' deliverables and documented with comments.

**Tests**:
- `"second bail replaces first bail's text"`
- `"prior in-flight tick does not clear newer flash"`
- `"newer flash's own tick clears it at deadline"`
- `"N successive bails preserve only the latest"`
- `"all stale ticks from superseded flashes are no-ops"`
- `"manual keystroke clear followed by new bail and stale tick: stale tick is no-op"`
- `"generation counter increments exactly once per setFlash call"`

**Edge Cases**:
- Concurrent bail and tick: Bubble Tea serialises message processing — both messages enter `Update` in some order. If the tick is processed before the new bail, it clears the prior flash (gen matches), then the new bail re-sets the flash with gen+1. If the new bail is processed first, the tick is now stale and discarded. Both orderings yield correct end state. No race.
- Bail arrives at the exact moment the tick fires (tea.Tick fires asynchronously): same answer — Bubble Tea queues both messages, processes serially, end state correct in both orderings.
- Generation overflow: covered in task 2-1 — uint64, not a practical concern.
- A bail with the same session name twice in a row: still bumps generation (the helper does not deduplicate on text); the second bail's flash is identical text but gets a fresh tick. Visually indistinguishable from no replacement, but the generation bump still invalidates the prior tick. Acceptable.
- Tick captured with `gen := m.flashGen` BEFORE `setFlash` (subtle bug): the tick's captured gen would be N-1, would never match (since setFlash bumps to N), and would never fire. The flash would NEVER auto-clear — only the next keystroke would clear it. Test scenario 3 (the live tick clears) catches this — if the captured-before-setFlash bug exists, scenario 3 fails because the tick is stale on arrival.

**Context**:
> Spec § Replacement on rapid successive bails: "A new bail while a prior flash is still visible **replaces** the prior flash's text and **resets** the tick — the visible message always reflects the most recent bail. Any pending tick from the prior flash is cancelled or otherwise prevented from firing against the new flash. Build-phase shape (e.g. monotonic flash ID, single-shot tick handle, generation counter) is a build decision; the spec-level constraint is 'latest bail wins, prior pending tick must not clear the new flash early'."
>
> Mechanism summary across Phase 2:
> - Task 2-1: `setFlash` monotonically bumps `flashGen`. `clearFlash` does not change gen.
> - Task 2-3: `flashTickMsg{Gen}` self-discriminates — clears only when `msg.Gen == m.flashGen`.
> - Task 2-5: bail handler calls `setFlash` first, then captures the post-bump gen for the tick. Both run from the same Update return via `tea.Batch`.
> - Task 2-6 (this task): integration tests prove the three pieces compose correctly.

**Spec Reference**: `.workflows/enter-attaches-from-preview/specification/enter-attaches-from-preview/specification.md` § Replacement on rapid successive bails
