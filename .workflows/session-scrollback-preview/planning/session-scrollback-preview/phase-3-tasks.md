---
phase: 3
phase_name: Multi-pane cycling, chrome, and focus-change reads
total: 7
---

## session-scrollback-preview-3-1 | approved

### Task 3-1: Focus state and pane-key resolution helpers

**Problem**: Phase 2 left `previewModel.windowIdx` / `paneIdx` as 0-based ordinal indices into the cached `groups []tmux.WindowGroup`, but Phase 3 introduces three focus-mutating keys (`]`, `[`, `Tab`) plus chrome rendering. Without small, well-named helpers for "current window group", "current raw pane index", "current pane key" and "is single-pane single-window", the cycling and chrome code that follows in tasks 3-2 through 3-7 will repeat arithmetic and conflate ordinals with raw tmux indices — exactly the bug the spec calls out under chrome counter semantics and pane-key derivation.

**Solution**: Add small, pure helpers on `previewModel` (or as private package functions taking the model by value) that produce: (a) the current `tmux.WindowGroup`, (b) the current raw `(window_index, pane_index)` pair, (c) the current `paneKey` via `state.SanitizePaneKey`, (d) a `degenerate()` predicate that returns true when the cached `groups` describe a single-window single-pane session. These helpers carry no I/O — they are read-only views over `groups` plus `windowIdx`/`paneIdx`. They become the single source of truth used by the 3-2 / 3-3 cycle handlers, the 3-1 read-trigger glue, and the 3-5 chrome renderer.

**Outcome**: A focused pane's tmux raw indices, paneKey, and group structure are accessible via named methods that are unit-tested directly. Subsequent tasks invoke these helpers rather than indexing `groups[m.windowIdx].PaneIndices[m.paneIdx]` inline. The degenerate-session check that 3-2 and 3-3 rely on for silent no-ops becomes a single boolean call.

**Do**:
- In `internal/tui` (alongside the Phase 2 `previewModel`), add methods on `*previewModel`:
  - `currentGroup() tmux.WindowGroup` — returns `m.groups[m.windowIdx]`.
  - `currentRawIndices() (windowIndex, paneIndex int)` — returns `m.groups[m.windowIdx].WindowIndex` and `m.groups[m.windowIdx].PaneIndices[m.paneIdx]`.
  - `currentPaneKey() string` — calls `state.SanitizePaneKey(m.session, windowIndex, paneIndex)` using the raw tmux indices, not the ordinal positions.
  - `degenerate() bool` — true iff `len(m.groups) == 1 && len(m.groups[0].PaneIndices) == 1`.
- These helpers must use the raw `WindowIndex` / `PaneIndices[]` values from the cached `tmux.WindowGroup`, never `m.windowIdx` / `m.paneIdx` directly, so non-contiguous tmux indices and base-index-1 sessions resolve to the same pane key the daemon used when writing the `.bin`.
- Do not change the existing `previewModel` struct shape from Phase 2 (no new fields). These are pure derived views.
- Add a small unit test file (or extend the existing `pagepreview_test.go`) constructing a `previewModel` with synthetic `groups` and asserting each helper's output for: (i) standard 0-indexed, (ii) base-index 1 with non-contiguous gaps `0, 2, 5`, (iii) single-window single-pane.

**Acceptance Criteria**:
- [ ] `currentGroup()` returns `m.groups[m.windowIdx]` and is the only call site that reads `m.groups[m.windowIdx]` from cycle/chrome code in subsequent tasks.
- [ ] `currentRawIndices()` returns the values stored in `tmux.WindowGroup.WindowIndex` and `tmux.WindowGroup.PaneIndices[m.paneIdx]` — not `m.windowIdx` / `m.paneIdx`.
- [ ] `currentPaneKey()` produces a key byte-identical to the one the daemon writes for that pane (`state.SanitizePaneKey(session, rawWindowIndex, rawPaneIndex)`).
- [ ] `degenerate()` returns true only when `len(groups) == 1 && len(groups[0].PaneIndices) == 1`; false in every other shape including single-window two-pane and two-window single-pane each.
- [ ] All four helpers are pure (no I/O, no mutation, no goroutines) and safe to call from `Update` without producing tea.Cmds.

**Tests**:
- `"currentGroup returns the cached WindowGroup at windowIdx"`
- `"currentRawIndices returns raw tmux WindowIndex and PaneIndices[paneIdx], not ordinals"`
- `"currentRawIndices handles non-contiguous window_index (0,2,5) and base-index-1 panes"`
- `"currentPaneKey matches state.SanitizePaneKey on raw indices for the same session"`
- `"degenerate returns true for single-window single-pane and false for 1x2, 2x1, and 2x2 shapes"`

**Edge Cases**:
- Non-contiguous tmux `window_index` values like `0, 2, 5` must produce the correct pane key (using raw `5`, not ordinal `2`).
- `pane-base-index 1` sessions must produce the correct pane key (using raw `1`, not ordinal `0`).
- Pane-key derivation uses raw tmux indices; chrome (in task 3-5) will use ordinals — these helpers expose only the raw side, the ordinal side stays inline at the chrome call site.

**Context**:
> Per spec: "the resolution chain ... `paneKey = state.SanitizePaneKey(session, window_index, pane_index)`" uses raw runtime tmux fields, while chrome counters are 1-based ordinal positions in enumeration order. Tasks 3-2/3-3/3-5/3-7 all need both views; collapsing them into one indexing scheme would break either chrome (showing `Window 5 of 3`) or the read (writing to a non-existent `.bin`). These helpers separate the two unambiguously.

**Spec Reference**: `.workflows/session-scrollback-preview/specification/session-scrollback-preview/specification.md` — § *Multi-pane Rendering Shape > Chrome Floor (Counter semantics)* and § *Cross-cutting Seams > State Package API Reuse > Resolution chain*.

## session-scrollback-preview-3-2 | approved

### Task 3-2: Tab cycle: next pane within current window with wrap and re-read

**Problem**: Multi-pane windows currently render only their first pane — Phase 2 wired open-time read but never advances `paneIdx`. The spec mandates `Tab` advance forward through panes within the focused window, wrap from last to first, trigger a fresh `ScrollbackReader.Tail` read of the newly-focused pane, and reset scroll position to tail. In single-pane windows it must silently no-op.

**Solution**: Extend `previewModel.Update` (in the existing Phase 2 `pagePreview` arm) to handle `tea.KeyTab`. The handler advances `m.paneIdx` modulo `len(currentGroup().PaneIndices)`, then performs a synchronous `ScrollbackReader.Tail(currentPaneKey())` call, applies the three-shape result to the viewport (bytes / placeholder / error string — tasks 3-7 / Phase 4 finalise placeholder strings; for now reuse the same dispatch Phase 2 used at open), and calls `viewport.GotoBottom()` to reset scroll to tail. When `degenerate()` (single-window single-pane) or when the current window has only one pane, `Tab` is consumed silently — no model mutation, no read, no Cmd.

**Outcome**: With a multi-pane window cached, pressing `Tab` updates the focus, re-reads the pane's `.bin` synchronously, and scrolls the viewport to the tail of the newly-loaded buffer. In single-pane contexts `Tab` is invisible. Test harness assertions on the mock `ScrollbackReader` show exactly one `Tail` call per `Tab` press — never zero, never two.

**Do**:
- In `internal/tui/model.go` (or the preview-page sub-file), inside the `pagePreview` Update arm, add a `tea.KeyMsg` branch matching `tea.KeyTab`.
- Compute `paneCount := len(m.currentGroup().PaneIndices)`. If `paneCount <= 1`, return `m, nil` without touching state — silent no-op.
- Otherwise: `m.paneIdx = (m.paneIdx + 1) % paneCount`.
- Call `m.reader.Tail(m.currentPaneKey())` synchronously; pass `(bytes, err)` through the same dispatcher Phase 2 uses to set `viewport.SetContent` (raw bytes verbatim on success; placeholder/error rendering may be stubbed to the same Phase 2 path until Phase 4 finalises wording).
- Call `m.viewport.GotoBottom()` to reset scroll position to tail.
- Return `m, nil` (no `tea.Cmd` — read is synchronous in `Update` per spec).
- Crucially: the `Tab` branch must be intercepted **before** the catch-all that forwards keys to `m.viewport.Update` so that `bubbles/viewport` never sees the Tab byte. This wins the keymap-precedence requirement; full coverage of precedence lives in task 3-4.
- Add tests in `pagepreview_test.go` driving `Tab` against synthetic groups via `Update` and a recording `ScrollbackReader` mock.

**Acceptance Criteria**:
- [ ] `Tab` on a window with N≥2 panes advances `paneIdx` by 1 mod N and triggers exactly one `ScrollbackReader.Tail` call with the paneKey for the newly-focused pane.
- [ ] `Tab` from the last pane in a window wraps to `paneIdx = 0` of the same window (windowIdx unchanged).
- [ ] `Tab` in a single-pane window or single-pane single-window session is a silent no-op: no state mutation, zero `Tail` calls, no Cmd produced.
- [ ] After a `Tab` that triggers a read, the viewport reports `AtBottom()` true (scroll reset to tail) before the next `Update` returns.
- [ ] `windowIdx` is never modified by a `Tab` press (only `paneIdx` moves).
- [ ] Reading is synchronous within the Update call; no `tea.Cmd` is returned for the read I/O.

**Tests**:
- `"Tab advances paneIdx by 1 within a multi-pane window"`
- `"Tab wraps from last pane back to pane 0 within the same window"`
- `"Tab in a single-pane window is a silent no-op (zero Tail calls)"`
- `"Tab in a single-window single-pane session is a silent no-op"`
- `"Tab triggers exactly one Tail call with the paneKey of the newly-focused pane"`
- `"Tab resets viewport scroll position to tail (GotoBottom called)"`
- `"Tab does not modify windowIdx"`

**Edge Cases**:
- Single-pane window where `len(PaneIndices) == 1`: silent no-op — verify mock reader recorded zero calls.
- Last pane in window wraps back to pane 0 (not advancing windowIdx — that is `]` territory).
- Scroll position reset: even if user had scrolled up 50 lines in pane A, after `Tab` to pane B and `Tab` back to pane A, viewport is at tail (a fresh re-read is performed each focus change so there is no "stale buffer at offset 50" path).
- The paneKey passed to `Tail` must use the raw tmux pane index for the new pane (via `currentPaneKey()` from task 3-1) — not the ordinal `paneIdx`.

**Context**:
> Per spec § *Within-preview Key Bindings*: "`Tab` cycles forward from pane 0 ... forward-only `Tab` with wraparound is sufficient". Per § *Refresh Semantics > Read Trigger Events*: "`Tab` — re-reads the newly-focused pane after a within-window pane cycle." Per § *Refresh Semantics > Scroll Position Resets on Focus Change*: scroll always returns to tail on focus change. Per § *Read Pipeline > Read is synchronous*: "the read happens inline in `Update` for the focus-changing event. No `tea.Cmd` deferral".

**Spec Reference**: `.workflows/session-scrollback-preview/specification/session-scrollback-preview/specification.md` — § *Multi-pane Rendering Shape > Within-preview Key Bindings*, § *Refresh Semantics > Read Trigger Events*, § *Refresh Semantics > Scroll Position Resets on Focus Change*.

## session-scrollback-preview-3-3 | approved

### Task 3-3: Bracket cycles: next/previous window with pane-0 reset and re-read

**Problem**: `]` and `[` move focus across windows in the cached enumeration. The spec mandates wraparound in both directions, **resetting `paneIdx` to 0** on every window cycle (per-window pane focus is intentionally not preserved), a fresh `ScrollbackReader.Tail` read of the new pane, scroll-to-tail, and silent no-op in single-window sessions.

**Solution**: Add two more `tea.KeyMsg` branches to the `pagePreview` Update arm matching `]` and `[`. Each branch: checks `len(m.groups) > 1`; advances or rewinds `m.windowIdx` modulo `len(m.groups)`; **always** sets `m.paneIdx = 0`; performs the synchronous `Tail` read; resets viewport to tail. Single-window cases (regardless of pane count within that window) silently no-op for both keys.

**Outcome**: Pressing `]` walks forward through windows wrapping last→first; `[` walks backward wrapping first→last. Both land on pane 0 of the new window, render that pane's tail-N bytes, and scroll to tail. Per-window scroll/focus state is **not** retained — landing on window W twice in succession (e.g. `]` then `[`) shows window W's pane 0 fresh both times.

**Do**:
- In `internal/tui` `pagePreview` Update arm, add `tea.KeyMsg` branches matching `]` (KeyType `tea.KeyRunes` with rune `']'`) and `[` (rune `'['`).
- Both branches: if `len(m.groups) <= 1`, return `m, nil` silently — no read, no mutation, no Cmd.
- For `]`: `m.windowIdx = (m.windowIdx + 1) % len(m.groups)`.
- For `[`: `m.windowIdx = (m.windowIdx - 1 + len(m.groups)) % len(m.groups)` (the `+ len(m.groups)` makes Go's `%` yield non-negative results when wrapping from 0 backwards).
- Both branches: set `m.paneIdx = 0` unconditionally — per-window pane focus is not preserved.
- Both branches: `m.reader.Tail(m.currentPaneKey())` synchronously, dispatch result to viewport via the same path as Phase 2 / task 3-2, then `m.viewport.GotoBottom()`.
- Both branches must be intercepted before the viewport-passthrough catch-all (full precedence test in task 3-4).
- Tests in `pagepreview_test.go` cover wrap directions, pane-0 reset, single-window no-op, and scroll-tail reset.

**Acceptance Criteria**:
- [ ] `]` advances `windowIdx` by 1 mod `len(groups)` and resets `paneIdx` to 0; one `Tail` call is issued.
- [ ] `[` rewinds `windowIdx` by 1 mod `len(groups)` (wrapping correctly for `windowIdx == 0` → last) and resets `paneIdx` to 0; one `Tail` call is issued.
- [ ] After a window cycle, `paneIdx` is 0 even if it was non-zero before (per-window pane focus not preserved).
- [ ] In a single-window session both `]` and `[` are silent no-ops: zero state mutation, zero `Tail` calls, no Cmd produced — regardless of how many panes the single window contains.
- [ ] After cycling, viewport is at tail (`AtBottom()` true).
- [ ] Reading is synchronous in `Update`; no `tea.Cmd` returned for the read I/O.

**Tests**:
- `"]'  advances windowIdx by 1 and resets paneIdx to 0"`
- `"]' wraps from last window back to window 0"`
- `"['  rewinds windowIdx by 1 and resets paneIdx to 0"`
- `"['  from window 0 wraps to last window"`
- `"]'  in a single-window session is a silent no-op even if window has multiple panes"`
- `"['  in a single-window session is a silent no-op even if window has multiple panes"`
- `"window cycle resets paneIdx to 0 even when paneIdx was non-zero before"`
- `"window cycle triggers exactly one Tail call with the paneKey of pane 0 of the new window"`
- `"window cycle resets viewport scroll position to tail"`

**Edge Cases**:
- `[` from `windowIdx == 0` wraps to `len(groups) - 1`; the `+ len(groups)` modulo guard prevents Go's `%` returning -1.
- Single-window two-pane session: both `]` and `[` are no-ops (the spec keeps cycle keys "purposeful" — windows iterate windows, panes iterate panes; a one-window session has nothing to iterate).
- Re-cycling back to a previously-visited window does not restore prior `paneIdx` — pane 0 always.
- The new pane's paneKey uses the raw tmux pane index from `groups[newWindowIdx].PaneIndices[0]` via `currentPaneKey()` (task 3-1), not the literal `0`.

**Context**:
> Per spec § *Multi-pane Rendering Shape > Pane focus on window cycle*: "After `]` or `[`, the focused pane within the new window resets to **pane 0** ... Per-window pane focus is not preserved across window cycles. `Tab` then cycles forward from pane 0." Per § *Within-preview Key Bindings*: "`]` Next window (wraps from last → first); `[` Previous window (wraps from first → last)". Per § *Multi-pane Rendering Shape > Degenerate cases*: "In single-window single-pane sessions ... all three cycle keys silently no-op." Single-window multi-pane is not explicitly enumerated as degenerate, but `]` / `[` having no windows to advance to falls under the same intent — windows are the iteration unit.

**Spec Reference**: `.workflows/session-scrollback-preview/specification/session-scrollback-preview/specification.md` — § *Multi-pane Rendering Shape > Within-preview Key Bindings*, § *Multi-pane Rendering Shape > Pane focus on window cycle*, § *Refresh Semantics > Read Trigger Events*, § *Refresh Semantics > Scroll Position Resets on Focus Change*.

## session-scrollback-preview-3-4 | approved

### Task 3-4: Keymap precedence over embedded viewport

**Problem**: Phase 2 forwards arbitrary keys to `m.viewport.Update` so that `bubbles/viewport` defaults (`Up`, `Down`, `PgUp`, `PgDn`, `j`, `k`, etc.) work. Tasks 3-2 and 3-3 wire `Tab`, `]`, `[`. If those keys fall through to the viewport — even after preview has already handled them — the viewport could double-handle the input (e.g. consume `Tab` as a focus key in a future `bubbles/viewport` revision, or scroll incorrectly). The spec explicitly carves out preview-owned keys with a precedence rule: "preview's binding wins inside the preview page".

**Solution**: Audit and codify the dispatch order in the `pagePreview` Update arm: preview-owned keys (`]`, `[`, `Tab`, `Esc`) are matched first and short-circuit the function with their own return; only the unmatched-default branch forwards to `m.viewport.Update`. Add a focused test that captures every keypress reaching the viewport via a recording viewport wrapper (or by asserting viewport scroll offset / content state remains untouched after `]` `[` `Tab`).

**Outcome**: After a `Tab`, `]`, or `[` press, the embedded viewport never sees the key — it does not move scroll offset, does not change content, and does not emit any side effect. Conversely, `Up` / `Down` / `j` / `k` / `PgUp` / `PgDn` / `Home` / `End` and other viewport defaults still pass through and scroll the loaded buffer as in Phase 2.

**Do**:
- In `internal/tui` `pagePreview` Update arm, ensure the switch on `tea.KeyMsg` matches preview-owned keys explicitly and `return m, nil` (or whatever Cmd those branches need) before any `m.viewport, cmd = m.viewport.Update(msg)` passthrough call.
- Concretely the structure is:
  ```
  switch msg := msg.(type) {
  case tea.KeyMsg:
      switch {
      case key matches Esc: ... return m, dismissCmd
      case key matches ']': ... return m, nil  // task 3-3
      case key matches '[': ... return m, nil  // task 3-3
      case msg.Type == tea.KeyTab: ... return m, nil  // task 3-2
      }
      // fallthrough to viewport
  }
  var cmd tea.Cmd
  m.viewport, cmd = m.viewport.Update(msg)
  return m, cmd
  ```
- Verify that `tea.WindowSizeMsg` still reaches the viewport (the early-returns are gated on `tea.KeyMsg`, not all messages).
- Add a test using a viewport wrapper or scroll-offset snapshot: drive the preview with `Tab`, `]`, `[`; assert the viewport's `YOffset` (or equivalent) is unchanged from immediately after the read, and assert no double-handling occurs.
- Add a test that drives `Up`, `Down`, `j`, `k`, `PgUp`, `PgDn` and asserts the viewport's scroll offset moves accordingly (regression guard for the inverse — preview must NOT swallow scroll keys).

**Acceptance Criteria**:
- [ ] Preview-owned keys (`]`, `[`, `Tab`, `Esc`) never reach `m.viewport.Update` — verifiable by snapshotting the viewport's scroll offset across the keypress and asserting it changes only via the post-read `GotoBottom`, not via a viewport scroll handler.
- [ ] Viewport default scroll keys (`Up`, `Down`, `j`, `k`, `PgUp`, `PgDn`, `Home`, `End`, `ctrl-u`, `ctrl-d`) continue to pass through and produce viewport scroll offset changes.
- [ ] `tea.WindowSizeMsg` still reaches the viewport (resize handling from Phase 2 is not regressed).
- [ ] No double-handling: a single `Tab` keypress produces exactly one `Tail` call (not two — which would happen if both preview and viewport handlers ran).

**Tests**:
- `"Tab does not advance viewport scroll offset (does not reach viewport handler)"`
- `"]' does not advance viewport scroll offset"`
- `"['  does not advance viewport scroll offset"`
- `"Up advances viewport scroll offset upward (passthrough preserved)"`
- `"PgDn advances viewport scroll offset downward (passthrough preserved)"`
- `"j and k still scroll the viewport (vim-style passthrough preserved)"`
- `"WindowSizeMsg still reaches viewport for re-flow during preview"`
- `"a single Tab produces exactly one Tail call (no double-handling)"`

**Edge Cases**:
- A future `bubbles/viewport` version could bind `Tab` for focus traversal; precedence ordering protects preview against that without coupling to upstream changes.
- `Esc` is owned by preview (dismiss), already handled in Phase 2 — confirm that branch still short-circuits before viewport passthrough.
- Non-key messages (e.g. `tea.WindowSizeMsg`, custom `tea.Msg`) must continue to flow to the viewport — early-returns are scoped to `tea.KeyMsg` only.

**Context**:
> Per spec § *Within-preview Key Bindings > Keymap policy*: "Preview owns `]`, `[`, `Tab`, `Esc`. Everything else either passes through to the embedded `bubbles/viewport` (scroll keys above) or is unbound/no-op." Per § *Open Items Handed to the Build Phase*: "Within-preview keymap collisions — confirm `]`, `[`, `Tab` do not collide with any inherited `bubbles/viewport` or page-level bindings; if they do, preview's binding wins inside the preview page." This task is the operationalisation of "preview's binding wins".

**Spec Reference**: `.workflows/session-scrollback-preview/specification/session-scrollback-preview/specification.md` — § *Multi-pane Rendering Shape > Within-preview Key Bindings > Keymap policy*, § *Open Items Handed to the Build Phase (keymap collisions)*.

## session-scrollback-preview-3-5 | approved

### Task 3-5: Chrome rendering: counters, window name, and keystroke hints

**Problem**: Without chrome, multi-pane / multi-window sessions are structurally invisible — the user cannot tell whether they are looking at "window 1 of 1" or "window 2 of 4", whether the pane on screen is the only one or one of three siblings, or which keys cycle. The spec pins the v1 chrome floor (Window M of N · Pane X of Y · `#W` window name · keystroke hints) and the counter semantics (1-based ordinals in enumeration order — never raw tmux indices).

**Solution**: Add a `renderChrome() string` method (or pure function `chromeLine(m previewModel) string`) on `previewModel` that renders a single chrome line from the cached `groups`, current ordinal indices (`m.windowIdx + 1`, `m.paneIdx + 1`), the cached window name (`m.groups[m.windowIdx].WindowName`), and a static hint string (`] [ Tab Esc`). Wire it into the View output in task 3-6; this task focuses on producing the correct string.

**Outcome**: A pure `chromeLine(...)` function returns a deterministic single-line string for any given `(groups, windowIdx, paneIdx)` combination, with counters in 1-based ordinal form, the window name verbatim, and the static hint suffix. Non-contiguous window indices (`0, 2, 5`) produce `Window 1 of 3`, `Window 2 of 3`, `Window 3 of 3` as the user cycles — never `Window 5 of 3`. Window names with spaces or special characters render verbatim.

**Do**:
- In `internal/tui` (or alongside `previewModel`), add a chrome renderer:
  ```
  func (m *previewModel) chromeLine() string
  ```
  that returns a single-line string of the form (working layout — exact wording is a build-phase decision per *Open Items*; pick a clean rendering and pin it in tests):
  ```
  Window {wOrdinal} of {wTotal} · Pane {pOrdinal} of {pTotal} · #W: {windowName}    ] [ Tab  Esc
  ```
  - `wOrdinal = m.windowIdx + 1`, `wTotal = len(m.groups)`.
  - `pOrdinal = m.paneIdx + 1`, `pTotal = len(m.currentGroup().PaneIndices)`.
  - `windowName = m.currentGroup().WindowName` — verbatim, no escaping or sanitisation.
- Use `lipgloss` (already a project dependency) for any styling/coloration, but the test assertions key off plain content not styled bytes — render a plain-text variant or strip ANSI in the test, so coloration changes don't break tests.
- The function must be **pure** — no I/O, no `Tail` call, no tmux call, just a string format from in-model state.
- The function does not consult raw `WindowIndex` or `PaneIndices[i]` for the counter — only `len(...)` and the ordinal positions.
- Add tests asserting:
  - Standard 0-indexed: `Window 1 of 2 · Pane 1 of 3` etc.
  - Non-contiguous `WindowIndex` values `[0, 2, 5]`: chrome shows `Window 1 of 3`, `Window 2 of 3`, `Window 3 of 3` as `windowIdx` advances 0→1→2 — never the raw 0/2/5.
  - `pane-base-index 1` with `PaneIndices = [1, 2]`: chrome shows `Pane 1 of 2` then `Pane 2 of 2` — never the raw 1/2.
  - Window name containing spaces (`"editor window"`) renders verbatim.
  - Hint string includes `]`, `[`, `Tab`, `Esc` in some visible form.

**Acceptance Criteria**:
- [ ] `chromeLine()` returns a string containing `Window {wOrdinal} of {wTotal}` where `wOrdinal = windowIdx + 1` and `wTotal = len(groups)`.
- [ ] The string contains `Pane {pOrdinal} of {pTotal}` where `pOrdinal = paneIdx + 1` and `pTotal = len(currentGroup().PaneIndices)`.
- [ ] The string contains the window name (`currentGroup().WindowName`) verbatim, including any spaces or special characters.
- [ ] The string contains visible cycle-key hints: at minimum `]`, `[`, `Tab`, `Esc` each appear textually.
- [ ] Counters never expose raw tmux `WindowIndex` or `PaneIndices[i]` values — confirmed by a test case with non-contiguous indices.
- [ ] `chromeLine()` is pure: no I/O, no calls to `m.reader` or `m.enumerator`, no goroutines.

**Tests**:
- `"chromeLine renders 1-based ordinals for 0-indexed groups"`
- `"chromeLine renders 1..N counters when WindowIndex values are non-contiguous (0,2,5)"`
- `"chromeLine renders 1..N counters when PaneIndices start at 1 (pane-base-index 1)"`
- `"chromeLine includes the window name verbatim including spaces"`
- `"chromeLine includes ] [ Tab Esc as visible hints"`
- `"chromeLine produces no I/O when invoked (does not call reader or enumerator)"`

**Edge Cases**:
- Window name with whitespace: render verbatim (no trimming, no quoting).
- Window name containing the pipe character `|`: the enumeration parser in Phase 1 handles this at parse time; chrome simply renders `WindowName` verbatim — no further parsing.
- Single-window single-pane: still renders `Window 1 of 1 · Pane 1 of 1 · #W: …` — the chrome floor applies regardless of session shape.
- Build-phase wording is not user-facing copy-pinned; tests should assert on structural content (substrings, ordinals) rather than exact whitespace, so the wording can be tuned without rewriting tests.

**Context**:
> Per spec § *Multi-pane Rendering Shape > Chrome Floor*: must show "Window M of N", "Pane X of Y", window name (`#W`), and keystroke hints (`] [ Tab Esc`). Per *Counter semantics*: "M and X ... are 1-based ordinal positions in enumeration order, not the tmux `window_index` / `pane_index` values" — under base-index drift or window-kill gaps, the counter must still show `1..N`. Per *Open Items*: exact wording, header vs footer, single-line vs two-line is a build-phase decision; tests should assert structural content not exact wording.

**Spec Reference**: `.workflows/session-scrollback-preview/specification/session-scrollback-preview/specification.md` — § *Multi-pane Rendering Shape > Chrome Floor (v1 must-show)* (Counter semantics, chrome data source).

## session-scrollback-preview-3-6 | approved

### Task 3-6: Chrome layout integration with viewport sizing

**Problem**: Phase 2 sized the embedded viewport at `viewport.Width = m.width` and `viewport.Height = m.height`, assuming no chrome. Adding a chrome line on top (or below) the viewport without correctly subtracting its height from the viewport's height will cause the chrome and viewport content to overlap, the viewport to spill off-screen, or the bottom row of content to be hidden under chrome. The spec also requires `tea.WindowSizeMsg` to update both surfaces atomically and to NOT trigger any disk re-read.

**Solution**: Update `previewModel.View()` to compose `chromeLine()` (from task 3-5) plus `viewport.View()` into a full-screen frame, and update the resize handler in `Update` to subtract the chrome height (1 line in v1) from the viewport's height. The disk-read decoupling already established in Phase 2 (resize does not call `Tail`) is preserved.

**Outcome**: At any terminal size, the chrome line occupies one row and the viewport fills the remainder. Resizing the terminal (drag-resize → many `tea.WindowSizeMsg` fires) re-flows both surfaces atomically without any `ScrollbackReader.Tail` calls. Cycling between panes / windows produces no visible chrome layout shift — chrome row count is constant.

**Do**:
- In `internal/tui` `previewModel.View()`:
  - Render `chromeLine()` first (or last, depending on header-vs-footer choice — the spec defers to build phase; pick one and pin in test).
  - Render `m.viewport.View()` as the body.
  - Compose with `lipgloss.JoinVertical` (or a simple `\n` join) so the chrome is on its own line above (or below) the viewport.
- In `previewModel.Update()` `tea.WindowSizeMsg` branch (originally added in Phase 2):
  - Update `m.width = msg.Width` and `m.height = msg.Height`.
  - Define `chromeHeight := 1` (v1 chrome is single-line per spec floor).
  - Set `m.viewport.Width = m.width` and `m.viewport.Height = m.height - chromeHeight`.
  - Do **not** call `m.reader.Tail(...)` — resize never triggers a fresh read.
- Pin the chrome height as a named constant (e.g. `previewChromeHeight = 1`) to make it discoverable when wording revisions land.
- Add tests:
  - Drive a `tea.WindowSizeMsg` and assert `m.viewport.Height == msg.Height - 1` and `m.viewport.Width == msg.Width`.
  - Drive multiple sequential `tea.WindowSizeMsg` (simulating drag-resize) and assert the recording `ScrollbackReader` mock has zero `Tail` calls across all of them.
  - Drive a `tea.WindowSizeMsg` followed by a chrome cycle (`Tab` or `]`) and assert chrome row height is still 1 (constant).
  - On a small terminal (e.g. height = 5), assert viewport height is `5 - 1 = 4` and no panic / negative dimension occurs.

**Acceptance Criteria**:
- [ ] `previewModel.View()` returns a string containing both the chrome line and the viewport content composed vertically.
- [ ] After `tea.WindowSizeMsg{Width: W, Height: H}`, `m.viewport.Width == W` and `m.viewport.Height == H - 1`.
- [ ] A `tea.WindowSizeMsg` triggers zero `ScrollbackReader.Tail` calls — verified by mock.
- [ ] Cycling (`Tab`, `]`, `[`) does not change chrome row height.
- [ ] On a small terminal (height ≤ 2) the viewport height is computed defensively (never negative; the viewport renders empty or near-empty rather than panicking).
- [ ] The chrome height is a named constant (`previewChromeHeight = 1`) — discoverable for future wording changes.

**Tests**:
- `"View renders chrome line above (or below) viewport content"`
- `"WindowSizeMsg sets viewport.Width to msg.Width and viewport.Height to msg.Height - chromeHeight"`
- `"Multiple WindowSizeMsg events trigger zero Tail calls (drag-resize incurs no reads)"`
- `"chrome row height is constant across Tab and ] [ cycles"`
- `"WindowSizeMsg with small height does not produce a negative viewport.Height"`

**Edge Cases**:
- Drag-resize (many `WindowSizeMsg` events in rapid succession): zero reads. The N-line buffer is decoupled from viewport dimensions per spec; only re-flow runs.
- Very small terminal (height 1 or 2): use `max(0, height - chromeHeight)` to keep viewport.Height non-negative.
- Chrome wording change in a future revision could push to two lines — pinning `previewChromeHeight` as a constant means the resize math updates in one place.
- Cycling does not change chrome line count — even if pane / window names change widely in length, chrome remains a single line (truncate / overflow handled by the renderer in task 3-5; layout math here assumes 1 line).

**Context**:
> Per spec § *Interaction Shape > Layout*: "viewport width = terminal width; viewport height = terminal height minus chrome lines. `tea.WindowSizeMsg` is forwarded to the embedded viewport so the slice re-flows on resize." Per § *Refresh Semantics > Read Trigger Events*: "Resize is not a read trigger." Per § *Refresh Semantics*: "The loaded N-line buffer is decoupled from viewport dimensions, so the viewport re-renders the existing buffer at the new size without re-reading from disk. This matters for the performance budget."

**Spec Reference**: `.workflows/session-scrollback-preview/specification/session-scrollback-preview/specification.md` — § *Interaction Shape > Layout*, § *Refresh Semantics > Read Trigger Events*, § *Refresh Semantics > Viewport-internal Scroll Does Not Re-read*.

## session-scrollback-preview-3-7 | approved

### Task 3-7: Chrome stability under focus changes (no mid-preview re-enumeration)

**Problem**: The spec is explicit that structural enumeration runs **once at preview-open** and is cached for the entire preview lifecycle — chrome counts and window names never re-fetch. If a careless implementation called `m.enumerator.ListWindowsAndPanesInSession` from the `Tab` / `]` / `[` handlers (e.g. to "refresh" the structural view), chrome could shift mid-preview, which is both spec-violating and breaks the externally-killed-session invariant where chrome stays stable as `.bin` files are cleaned. This task locks the invariant via a hermetic test rather than as a code change.

**Solution**: Add an explicit hermetic test that exercises a full cycle of `]`, `[`, `Tab` keypresses (covering window cycling, pane cycling, and wraparound) over a multi-window multi-pane synthetic session, and asserts the recording `TmuxEnumerator` mock saw exactly **one** `ListWindowsAndPanesInSession` call (the open-time enumeration), and the chrome line continues to render correctly from the cached `groups` after each cycle.

**Outcome**: A regression-guarding test pins the "no live re-enumeration mid-preview" invariant. The chrome rendered after every cycle uses cached `groups` data — even if the enumerator mock is wired to return a different shape on a hypothetical second call, chrome would still reflect the open-time cache. The test is hermetic (no real tmux) and runs without `tmuxtest`.

**Do**:
- In `pagepreview_test.go` (or a peer file inside `internal/tui`), add a test:
  - Construct a recording `TmuxEnumerator` mock that records every `ListWindowsAndPanesInSession` call and returns a synthetic 2-window 3-pane shape on the first call. Have it return a *different* shape (or an error) on every subsequent call to make it observable if the model ever calls back.
  - Construct a recording `ScrollbackReader` mock that returns benign bytes for any paneKey.
  - Call `NewPreviewModel(...)` to drive the open-time enumeration.
  - Drive a sequence of keypresses through `Update`: `]`, `]`, `[`, `[`, `Tab`, `Tab`, `Tab` — covering window forward (with wrap), window backward (with wrap), and pane cycling within a window.
  - Assert `enumerator.callCount == 1`.
  - Assert `chromeLine()` after each keypress reflects the cached open-time groups (e.g. `len(groups)` and `WindowName` values match the original mock return, not any later return).
  - Assert that across the entire cycle, the side-effect set is bounded: `Tail` calls equal "1 open + 7 cycles = 8" (each cycle key is a focus change including the no-op-resistant ones; for this test ensure groups are large enough that none of the cycles is a no-op).
- This is a test-only task; it does not change production code. If the test fails, the production fix is to remove any rogue `enumerator.ListWindowsAndPanesInSession` call from cycle handlers.
- Document the invariant inline in the test file as a comment: "Chrome data is captured once at preview-open; cycle handlers must not re-enumerate."

**Acceptance Criteria**:
- [ ] A test covering `]` `]` `[` `[` `Tab` `Tab` `Tab` against a recording `TmuxEnumerator` mock asserts exactly one `ListWindowsAndPanesInSession` call across the full sequence.
- [ ] After each keypress in the sequence, `chromeLine()` continues to reflect the open-time cached `groups` (window count, pane counts, window names match the original mock return).
- [ ] The test is hermetic — does not import `tmuxtest`, does not require a real tmux server, does not touch `t.Cleanup()` for package-level deps (constructor injection).
- [ ] If a future change accidentally adds `m.enumerator.ListWindowsAndPanesInSession(...)` to any cycle handler, this test fails with a clear "called N times, expected 1" assertion.
- [ ] Tail call count across the sequence equals 1 (open) + the number of focus-changing cycles in the sequence — i.e. cycle handlers always read once per focus change, never zero, never two.

**Tests**:
- `"full ] [ Tab cycle sequence produces exactly one Enumerate call"`
- `"chromeLine after each cycle reflects open-time cached groups"`
- `"chromeLine never reflects post-open enumerator state changes"`
- `"Tail calls per cycle = 1 (no double-read, no skipped read)"`

**Edge Cases**:
- Externally-killed session: even if the underlying tmux session disappears mid-preview, chrome stays stable because no live re-enumeration runs (this is the operational consequence of the invariant — Phase 4 task adds the placeholder rendering for missing `.bin`, but chrome integrity is locked here).
- A future "refresh" feature would have to explicitly opt-in by calling enumerator from a new handler; the current invariant is "no implicit re-enumeration".
- The test must drive enough keypresses to traverse all three cycle types — `Tab` forward (within window), `]` and `[` (across windows including wraparound) — to be confident no individual handler re-enumerates.

**Context**:
> Per spec § *Multi-pane Rendering Shape > Chrome Floor (Chrome data source)*: "Window/pane counts and names come from tmux structural enumeration ... Chrome is computed once at preview-open and cycled in place; no live re-enumeration mid-preview." Per § *Cross-cutting Seams > Externally-Killed Session During Preview > Chrome*: "structural counts and names were captured at preview-open. Cycle keys cycle the captured shape; no live re-enumeration is performed mid-preview, so chrome stays stable." Per § *Acceptance Criteria > Side-effect-free contract*: "exactly one `TmuxEnumerator` call (the structural enumeration at open) and zero further tmux invocations."

**Spec Reference**: `.workflows/session-scrollback-preview/specification/session-scrollback-preview/specification.md` — § *Multi-pane Rendering Shape > Chrome Floor (Chrome data source)*, § *Cross-cutting Seams > Externally-Killed Session During Preview*, § *Acceptance Criteria > Side-effect-free contract*.
