---
phase: 2
phase_name: Preview page entry, dismiss, and single-pane content rendering
total: 7
---

## session-scrollback-preview-2-1 | approved

### Task 2-1: Define TmuxEnumerator and ScrollbackReader seam interfaces in internal/tui

**Problem**: Preview's logic in `internal/tui` must depend on small interfaces (not concrete `*tmux.Client` or filesystem helpers) so unit tests can run hermetically without a real tmux server, without `tmuxtest`, and with `t.Parallel()` safe. The spec mandates two seams (`TmuxEnumerator`, `ScrollbackReader`) with a precise three-shape return contract on the reader.

**Solution**: Add two new dependency interfaces in the `internal/tui` package — one for window-grouped pane enumeration, one for tail-N scrollback reads keyed by `paneKey` — together with a record-type alias for the enumeration return shape. The interfaces are pure declarations in this task; production adapters and consumers land in later tasks.

**Outcome**: `internal/tui` exposes `TmuxEnumerator` and `ScrollbackReader` interface types whose method shapes match the spec exactly, with `stateDir` intentionally absent from `ScrollbackReader.Tail` (closed over at construction time). Tests can compile against the interface types using minimal mock structs.

**Do**:
- In `internal/tui` (new file, e.g. `internal/tui/preview_seams.go`), declare:
  - `type TmuxEnumerator interface { ListWindowsAndPanesInSession(session string) ([]tmux.WindowGroup, error) }` — return type reuses the `tmux.WindowGroup` shape landed in Phase 1 (`{ WindowIndex int; WindowName string; PaneIndices []int }`).
  - `type ScrollbackReader interface { Tail(paneKey string) ([]byte, error) }`.
- Add a doc comment on `ScrollbackReader.Tail` documenting the three observable shapes (verbatim per spec):
  - `(bytes != nil, nil)` — normal content; caller renders bytes verbatim.
  - `(nil, nil)` — "no content available" (collapses ENOENT, zero-byte file, and zero-line file with only an unterminated partial). Caller renders the placeholder `(no saved content)`.
  - `(nil, err != nil)` — OS-level read failure (EACCES, EIO, etc.). Caller renders the error string.
- Document on the interface that `stateDir` is intentionally hidden (closed over at construction) so tests can mock by `paneKey` alone.
- Do NOT add adapter wiring in this task (lands in 2-7). Do NOT add a `previewModel` (lands in 2-2).
- Do NOT introduce a package-level `previewDeps` variable; the spec mandates constructor injection.

**Acceptance Criteria**:
- [ ] `TmuxEnumerator` and `ScrollbackReader` are exported types in `internal/tui`.
- [ ] `TmuxEnumerator.ListWindowsAndPanesInSession(session string) ([]tmux.WindowGroup, error)` matches the Phase 1 client method signature exactly.
- [ ] `ScrollbackReader.Tail(paneKey string) ([]byte, error)` has no `stateDir` parameter.
- [ ] The doc comment on `Tail` enumerates the three return shapes verbatim.
- [ ] Package compiles (`go build ./internal/tui/...`).
- [ ] No package-level mutable seam variable exists for preview.

**Tests**:
- `"it declares TmuxEnumerator with the Phase 1 client signature"` — compile-time assertion via a `var _ TmuxEnumerator = (*tmux.Client)(nil)` check (or equivalent) in a test file.
- `"it declares ScrollbackReader hiding stateDir behind the interface"` — compile-time assertion: a tiny mock implementing only `Tail(paneKey string) ([]byte, error)` satisfies the interface.
- `"it allows mocks to express the three Tail return shapes"` — instantiate three mocks returning `(bytes, nil)`, `(nil, nil)`, and `(nil, err)` respectively and confirm each compiles against the interface.

**Edge Cases**:
- The `stateDir` hiding is the load-bearing test seam: any future change that adds a `stateDir` parameter to `Tail` breaks the construction-time-closure invariant and the mock-by-paneKey test pattern.
- The `tmux.WindowGroup` type comes from Phase 1; do not redefine it locally.

**Context**:
> Spec § *Architecture Summary > Test seams* defines both interfaces and the three-shape contract verbatim. Spec § *Cross-cutting Seams > State Package API Reuse > stateDir resolution* mandates `stateDir` is captured once at TUI startup and stable for process lifetime. Spec § *Architecture Summary > Wiring shape* mandates constructor injection over a package-level mutable `previewDeps`.

**Spec Reference**: `.workflows/session-scrollback-preview/specification/session-scrollback-preview/specification.md` § *Architecture Summary > Test seams* and § *Cross-cutting Seams > State Package API Reuse*

---

## session-scrollback-preview-2-2 | approved

### Task 2-2: previewModel constructor with injected seams and initial-open flow

**Problem**: A preview model is needed that owns the embedded `bubbles/viewport`, current focus indices, captured structural enumeration, and minimal keymap — constructed fresh on every `Space` press with no caching. The initial-open flow has spec-mandated ordering: enumeration first, fail/empty → silent no-open, success → set focus to window 0 / pane 0 and synchronously read tail-N bytes for that pane, then render chrome and viewport atomically in the first frame.

**Solution**: Add a `previewModel` struct in a new file (e.g. `internal/tui/pagepreview.go`) plus an exported constructor `NewPreviewModel(session string, enumerator TmuxEnumerator, reader ScrollbackReader, width, height int) (previewModel, bool)` returning `(model, ok)` where `ok=false` signals "do not transition to pagePreview". The constructor performs steps 1–4 of the spec's initial-open ordering inline; the caller (Sessions page) checks `ok` to decide whether to switch pages.

**Outcome**: A constructor-injected `previewModel` exists that, on construction, runs structural enumeration, returns `ok=false` on enumeration error or empty result (zero windows, or first window having zero panes), and on success sets focus to (0, 0) and reads the focused pane's tail-N bytes synchronously, populating the embedded `viewport.Model` via `SetContent`. A tail-N read failure does NOT block opening — placeholder/error rendering is the v1 default; in this task, the simple form is "pass whatever bytes (possibly nil) into the viewport". The model exposes an `Update`, `View`, and a way to read its current `windowIdx`/`paneIdx` for tests.

**Do**:
- In `internal/tui/pagepreview.go` declare:
  - `type previewModel struct { session string; enumerator TmuxEnumerator; reader ScrollbackReader; groups []tmux.WindowGroup; windowIdx, paneIdx int; viewport viewport.Model; width, height int }` (visibility is package-private; only the constructor and any helpers exported as needed for tests).
  - `func NewPreviewModel(session string, enumerator TmuxEnumerator, reader ScrollbackReader, width, height int) (previewModel, bool)`.
- Inside the constructor, in order:
  1. Call `enumerator.ListWindowsAndPanesInSession(session)`. On error, return `(previewModel{}, false)`.
  2. If result is empty (`len(groups) == 0`) or `len(groups[0].PaneIndices) == 0`, return `(previewModel{}, false)`.
  3. Set `windowIdx = 0`, `paneIdx = 0`. Construct `viewport.New(width, height)` (full terminal for v1; chrome layout is Phase 3).
  4. Compute `paneKey := state.SanitizePaneKey(session, groups[0].WindowIndex, groups[0].PaneIndices[0])`.
  5. Call `reader.Tail(paneKey)`. Pass the resulting bytes (which may be `nil`) verbatim to `viewport.SetContent(string(bytes))`. Do NOT translate `(nil, nil)` into placeholder text yet — Phase 4 owns placeholder/error wording. For now, `nil` bytes become an empty viewport; this is a permitted "preview still opens" outcome per spec acceptance.
  6. Call `m.viewport.GotoBottom()` so the first frame renders at scroll-tail. `bubbles@v1.0.0/viewport.SetContent` only auto-jumps to bottom when `YOffset > len(lines)-1`; a fresh viewport has `YOffset == 0`, so without an explicit `GotoBottom()` the user sees the OLDEST lines, contradicting spec § *History Depth > Scroll within bounds* ("the viewport renders the tail by default") and the Phase 4 task 4-3 test that asserts `AtBottom()` immediately post-construction. Tasks 3-2 / 3-3 already call `GotoBottom()` after focus-change reads — this preserves that invariant for the initial-open read.
  7. Return `(model, true)`.
- Implement `Update(msg tea.Msg) (previewModel, tea.Cmd)` that delegates messages to the embedded viewport. Cycle-key handling (`]`, `[`, `Tab`) is Phase 3 — leave a placeholder no-op or omit until Phase 3 lands. Esc handling is task 2-4. WindowSizeMsg handling is task 2-6.
- Implement `View() string` that returns `m.viewport.View()`. Chrome rendering is Phase 3 — for Phase 2 the view is just the viewport.
- Critical: bytes from `reader.Tail` must reach `viewport.SetContent` verbatim — no preprocessing, sanitisation, re-wrap, or escape stripping (spec § *Cross-cutting Seams > ANSI Passthrough vs Viewport Width*).
- Use `state.SanitizePaneKey` directly — its arguments must match the daemon's writer call site verbatim per spec § *Cross-cutting Seams > State Package API Reuse*.

**Acceptance Criteria**:
- [ ] `NewPreviewModel` is constructor-injected — no package-level seam variable consulted; both interfaces arrive as parameters.
- [ ] On enumeration error, `NewPreviewModel` returns `ok=false` and performs no further calls (no `Tail`, no `SetContent`).
- [ ] On empty enumeration (zero groups), `ok=false`.
- [ ] On a group with zero panes (`len(groups[0].PaneIndices) == 0`), `ok=false`.
- [ ] On enumeration success, `windowIdx == 0` and `paneIdx == 0` post-construction.
- [ ] `reader.Tail` is called exactly once during construction with the paneKey produced by `state.SanitizePaneKey(session, groups[0].WindowIndex, groups[0].PaneIndices[0])`.
- [ ] If `reader.Tail` returns `(nil, err)` or `(nil, nil)`, `ok=true` is still returned (preview opens).
- [ ] If `reader.Tail` returns `(bytes, nil)`, those exact bytes are the argument to `viewport.SetContent` (no transformation).
- [ ] After `SetContent`, the viewport is at scroll-tail (`m.viewport.AtBottom()` returns true) — `GotoBottom()` is called explicitly because `bubbles@v1.0.0/viewport.SetContent` does not jump to bottom when `YOffset == 0`.
- [ ] Re-invoking `NewPreviewModel` for the same session triggers a fresh enumeration AND a fresh `Tail` call (no caching).

**Tests**:
- `"it returns ok=false when enumeration errors"` — pass a `TmuxEnumerator` mock returning `nil, errors.New("boom")`; assert `ok==false` and `Tail` was never called.
- `"it returns ok=false on empty enumeration result"` — mock returns `[]tmux.WindowGroup{}`; assert `ok==false`.
- `"it returns ok=false when first window has zero panes"` — mock returns one group with `PaneIndices: []int{}`; assert `ok==false`.
- `"it sets focus to (0,0) on successful enumeration"` — mock returns two groups, several panes; assert `m.windowIdx==0`, `m.paneIdx==0`.
- `"it reads tail-N for the (0,0) pane synchronously during construction"` — mock records the paneKey argument; assert it equals `state.SanitizePaneKey(session, groups[0].WindowIndex, groups[0].PaneIndices[0])`.
- `"it passes raw ANSI bytes verbatim to viewport.SetContent"` — reader returns bytes containing ANSI escape sequences (e.g. `\x1b[31mred\x1b[0m`); assert viewport's content equals the input string byte-for-byte.
- `"it positions the viewport at scroll-tail on initial open"` — fixture: bytes containing more lines than the viewport height; assert `m.viewport.AtBottom() == true` immediately after construction; regression-pin against any future change that drops the explicit `GotoBottom()`.
- `"it returns ok=true when Tail returns (nil, nil)"` — reader returns `nil, nil`; assert `ok==true`, viewport content is empty.
- `"it returns ok=true when Tail returns (nil, err)"` — reader returns `nil, errors.New("eio")`; assert `ok==true` (placeholder/error wording is Phase 4).
- `"it constructs a fresh model per call with no carried state"` — call `NewPreviewModel` twice with the same session; assert `Tail` was called twice (no caching).

**Edge Cases**:
- Enumeration returns one group whose `PaneIndices` is empty (defensive: tmux normally never produces this, but the spec treats it as enumeration-empty for no-open).
- `reader.Tail` returns `(nil, err)` — preview must still open (Phase 4 will render the error string; Phase 2 just lets viewport content be empty).
- The bytes returned by `Tail` may be very large (tail-N at 1000 lines × wide content); `SetContent` accepts the full slice without preprocessing.

**Context**:
> Spec § *Refresh Semantics > Initial-open ordering* lists the five-step open sequence verbatim. Spec § *Multi-pane Rendering Shape > Model lifecycle* mandates "fresh `previewModel` constructed each time `Space` is pressed" with no singleton/caching. Spec § *Cross-cutting Seams > State Package API Reuse* requires `state.SanitizePaneKey` arguments to match the daemon's writer call site verbatim. Spec § *Read-Failure Handling* and § *Architecture Summary > Test seams* note that placeholder/error wording lives at the call site; Phase 2 lets bytes flow through and Phase 4 layers the placeholder/error text.

**Spec Reference**: `.workflows/session-scrollback-preview/specification/session-scrollback-preview/specification.md` § *Refresh Semantics > Initial-open ordering*, § *Multi-pane Rendering Shape > Model lifecycle*, § *Architecture Summary > Wiring shape*

---

## session-scrollback-preview-2-3 | approved

### Task 2-3: Add pagePreview arm to page state machine and bind Space on Sessions page

**Problem**: The TUI's page state machine has no preview arm and no Space binding. The spec mandates a new `pagePreview` arm peer to `pageFileBrowser`, with `Space` opening preview only from the Sessions page (Loading, Projects, FileBrowser pages must not bind it).

**Solution**: Add `pagePreview` to the `page` constant block in `internal/tui/model.go`, hold a `*previewModel` (or value-typed `previewModel` with a sentinel) on the root `Model`, route the page in the top-level `Update` switch, and bind `Space` in `updateSessionList` to construct a `previewModel` and transition to `pagePreview` when the constructor returns `ok=true`. When `ok=false`, the user remains on the Sessions page silently.

**Outcome**: Pressing `Space` on a highlighted session in the Sessions page transitions the TUI to the preview page with the tail bytes already loaded. Pressing `Space` on an empty Sessions list, with no highlighted item, while the list filter is in `SettingFilter()` mode (handled by passthrough — see task 2-5), or when enumeration fails, keeps the user on the Sessions page silently. Loading, Projects, and FileBrowser pages do not bind `Space` to preview.

**Do**:
- In `internal/tui/model.go`, add `pagePreview` to the `page` const block as a peer of `pageFileBrowser`.
- Add a `preview previewModel` field (and optionally a `hasPreview bool` sentinel, since Go zero values for the embedded viewport are not meaningful) to the root `Model`.
- In the top-level `Update` switch, add a case for `pagePreview` that delegates to `m.preview.Update(msg)` and returns the updated model. Cycle keys, Esc, and resize are handled in tasks 2-3..2-6 (this task lands the routing skeleton; per-key behaviour comes from those tasks or 2-2's `Update` delegation).
- In `updateSessionList` (Sessions page handler), add a `tea.KeyMsg` branch matching `Space` (key string `" "` or use `bubbles/key.NewBinding(key.WithKeys(" "))`):
  1. If `m.sessionList.SettingFilter()` is true → fall through to `bubbles/list`'s default handler (literal-space passthrough is task 2-5; this branch must NOT fire `NewPreviewModel`).
  2. If the list is empty (`len(m.sessionList.Items()) == 0`) or `m.sessionList.SelectedItem() == nil` → return without transition (no-op).
  3. Resolve the highlighted session name from the selected item.
  4. Construct `previewModel := NewPreviewModel(sessionName, m.enumerator, m.reader, m.termWidth, m.termHeight)`. The seams `m.enumerator` and `m.reader` arrive from task 2-7's TUI construction; in this task the fields are added with placeholder zero values acceptable for compilation. The root `Model` already caches terminal dimensions in `termWidth` / `termHeight` — see `internal/tui/model.go` line 174 and the `tea.WindowSizeMsg` branch at line 700.
  5. If `ok==false` → return without transition (no preview page shown).
  6. If `ok==true` → store `m.preview = pmodel`, set `m.activePage = pagePreview`, return.
- Confirm Loading, Projects, and FileBrowser page handlers do NOT bind `Space` to preview. (Verify by code inspection; no edit required if the existing handlers do not match `Space`.)

**Acceptance Criteria**:
- [ ] `pagePreview` is declared in the `page` const block in `internal/tui/model.go`.
- [ ] The top-level `Update` routes `pagePreview` to `m.preview.Update`.
- [ ] `Space` on the Sessions page constructs `previewModel` and transitions to `pagePreview` when `ok==true`.
- [ ] `Space` is a no-op when the Sessions list is empty.
- [ ] `Space` is a no-op when no item is highlighted (`SelectedItem() == nil`).
- [ ] `Space` is a no-op when `NewPreviewModel` returns `ok==false` (enumeration error or empty).
- [ ] The Loading, Projects, and FileBrowser page handlers do not invoke `NewPreviewModel`.
- [ ] When `Space` is pressed during `SettingFilter()`, this branch does not call `NewPreviewModel` (passthrough integration is finalised in task 2-5).

**Tests**:
- `"it transitions to pagePreview on Space when a session is highlighted"` — synthesise a `tea.KeyMsg{Type: tea.KeySpace}` (matches the runtime shape bubbletea produces for a standalone space keypress; see `internal/ui/browser_test.go` for the existing in-tree pattern), drive `Update`, assert `m.activePage == pagePreview`.
- `"it remains on Sessions page when Space is pressed on an empty list"` — empty list, Space; assert page unchanged, `NewPreviewModel` not called (mock counter zero).
- `"it remains on Sessions page when enumeration fails"` — `TmuxEnumerator` mock returns error; Space; assert page unchanged.
- `"it remains on Sessions page when enumeration returns empty"` — mock returns empty groups; Space; assert page unchanged.
- `"it does not bind Space on the Loading page"` — drive `Update` with `m.activePage == PageLoading` and a Space KeyMsg; assert no preview construction.
- `"it does not bind Space on the Projects page"` — same but `m.activePage == PageProjects`.
- `"it does not bind Space on the FileBrowser page"` — same but `m.activePage == pageFileBrowser`.

**Edge Cases**:
- Enumeration failure must be treated identically to empty enumeration (silent no-open).
- `SelectedItem() == nil` happens when a committed filter has narrowed the list to zero matches.
- The seam fields on `Model` may be nil during early construction; the no-op branches must come before any seam invocation.

**Context**:
> Spec § *Trigger and Entry Point* anchors `Space` to the Sessions page only, with empty-list and no-highlighted-item being no-ops. Spec § *Refresh Semantics > Initial-open ordering* mandates that enumeration failure or empty result results in silent no-open (no preview page shown). Spec § *Architecture Summary > Page state machine* declares `pagePreview` as a peer of `pageFileBrowser`. Spec § *Filter Behaviour with Preview* mandates `Space` does not intercept while filtering — task 2-5 finalises this; this task only defers to that branch.

**Spec Reference**: `.workflows/session-scrollback-preview/specification/session-scrollback-preview/specification.md` § *Trigger and Entry Point*, § *Refresh Semantics > Initial-open ordering*, § *Architecture Summary > Page state machine*

---

## session-scrollback-preview-2-4 | approved

### Task 2-4: Esc dismiss returns to Sessions list preserving cursor and filter state

**Problem**: Within preview, `Esc` must return the user to the Sessions list with the cursor at the same position it was on when `Space` was pressed, and with filter state preserved per the Esc level tree (committed filter remains committed; no filter remains no filter; mid-typing-filter is a sessions-page concern handled by `bubbles/list`'s default Esc behaviour, not preview's). The preview model owns one Esc level: dismiss preview.

**Solution**: In `previewModel.Update`, intercept `Esc` and return a marker (either via a returned `tea.Cmd` carrying a `previewDismissedMsg`, or by setting a flag the root model consults). The root model reacts by setting `m.activePage = PageSessions`. The `bubbles/list` model is left untouched across the open/dismiss round-trip — its cursor and filter state survive automatically because preview never mutates it.

**Outcome**: `Esc` while on `pagePreview` returns the user to the Sessions list. The list cursor is byte-identical to where it was when `Space` was pressed (verified by reading `m.sessionList.Index()` before and after). Committed filter state remains committed; no-filter remains no-filter. A second `Esc` (now on the Sessions page with a committed filter) clears the filter via `bubbles/list`'s default behaviour — preview doesn't need to do anything to make this work.

**Do**:
- In `internal/tui/pagepreview.go` (or wherever `previewModel` lives), in `previewModel.Update`:
  - Match `tea.KeyMsg` with `Type == tea.KeyEsc` (or `String() == "esc"`).
  - Return a sentinel `tea.Cmd` that emits `previewDismissedMsg{}` (declare the type in the same file).
- In `internal/tui/model.go` top-level `Update`, handle `previewDismissedMsg`:
  - Set `m.activePage = PageSessions`.
  - Do NOT mutate `m.sessionList` (cursor and filter state must survive untouched).
  - Optional: zero out `m.preview` to release viewport memory; the next `Space` constructs fresh.
- Confirm that `bubbles/list` cursor (`m.sessionList.Index()`) and filter state (`m.sessionList.IsFiltered()`, `m.sessionList.FilterValue()`) are not consulted/mutated during preview lifetime.
- Note: re-fetch on dismiss (for externally-killed sessions) is Phase 4 scope; this task only preserves existing cursor/filter, not session-list refresh.

**Acceptance Criteria**:
- [ ] `Esc` while on `pagePreview` transitions back to `PageSessions`.
- [ ] After dismiss, `m.sessionList.Index()` equals the value captured before `Space`.
- [ ] After dismiss, `m.sessionList.FilterValue()` equals the value captured before `Space` (no filter case).
- [ ] After dismiss when a filter was committed before `Space`, the filter remains committed (`m.sessionList.IsFiltered() == true` and same `FilterValue()`).
- [ ] A subsequent `Esc` on the Sessions page falls through to `bubbles/list`'s default Esc handling (committed filter clears, then unfiltered list), confirmed by integration test driving two consecutive Escs.
- [ ] Re-opening preview after dismiss constructs a fresh `previewModel` (no carried-over state).

**Tests**:
- `"it dismisses preview on Esc and returns to PageSessions"` — open preview, drive Esc; assert page transitions.
- `"it preserves the list cursor across open/dismiss"` — set list cursor to index 3, Space, Esc; assert `m.sessionList.Index() == 3`.
- `"it preserves the no-filter state across open/dismiss"` — list with no filter, Space, Esc; assert `m.sessionList.FilterValue() == ""`.
- `"it preserves committed filter across open/dismiss"` — commit a filter (`pigeon`), Space, Esc; assert filter still committed with same value.
- `"a second Esc clears a committed filter via list default behaviour"` — commit filter, Space, Esc (back to list with filter), Esc; assert filter cleared.
- `"it constructs a fresh previewModel on re-open after dismiss"` — open, dismiss, re-open; assert `Tail` was called twice total (once per open).

**Edge Cases**:
- `bubbles/list` distinguishes "filter input mode" (`SettingFilter()`) from "filter committed" — preview never opens during filter input mode (task 2-5), so dismiss never lands back into mid-typing-filter.
- The cursor preservation invariant relies on preview NEVER calling `m.sessionList.Select` or any list mutator. Confirm by code review.

**Context**:
> Spec § *Esc Level Tree* defines the four-level Esc semantics. Preview owns level 1 (in-preview → return to list). Levels 2–4 are existing `bubbles/list` and Portal behaviours preview must not break. Spec § *Trigger and Entry Point* mandates "cursor stays on the previewed session" on Esc. Spec § *No In-preview Between-Session Stepping* notes "Preview cannot move the underlying list cursor, so on `Esc` the cursor is exactly where it was when `Space` was pressed."

**Spec Reference**: `.workflows/session-scrollback-preview/specification/session-scrollback-preview/specification.md` § *Esc Level Tree*, § *Trigger and Entry Point*, § *No In-preview Between-Session Stepping*

---

## session-scrollback-preview-2-5 | approved

### Task 2-5: Filter-mode Space passthrough integration

**Problem**: `bubbles/list`'s filter input mode (`SettingFilter()` true) treats every keypress as text input — `Space` must insert a literal space into the filter, not open preview. The spec mandates "default `bubbles/list` semantics" — no magic-Space, no second binding for "open preview while filtering". The user must commit the filter (`Enter`) before `Space` opens preview.

**Solution**: In `updateSessionList`, the `Space` branch added in task 2-3 must explicitly check `m.sessionList.SettingFilter()` and fall through to the `bubbles/list` default handler (passing the message via `m.sessionList.Update(msg)`) when true. Only after the filter is committed (`SettingFilter()` returns false) does `Space` invoke `NewPreviewModel`.

**Outcome**: Typing `pigeon ` (with a space) into the filter input adds a literal space character — no preview is opened. After pressing `Enter` to commit the filter, the highlighted match (e.g. `pigeon-AbCdEf`) opens preview when `Space` is pressed. The filter input field accepts spaces transparently as part of text entry.

**Do**:
- In `updateSessionList` in `internal/tui/model.go`, ensure the `Space` branch from task 2-3 begins with:
  ```go
  if m.sessionList.SettingFilter() {
      // Filter input mode: Space is text input — pass through to bubbles/list.
      var cmd tea.Cmd
      m.sessionList, cmd = m.sessionList.Update(msg)
      return m, cmd
  }
  ```
- Confirm there is exactly one `Space` keybinding in `updateSessionList` — no second binding for "open preview while filtering".
- Confirm that after `Enter` commits the filter, `m.sessionList.SettingFilter()` returns false on subsequent `Space` events, so preview opens normally on the highlighted match.

**Acceptance Criteria**:
- [ ] When `m.sessionList.SettingFilter()` is true, `Space` passes through to `m.sessionList.Update(msg)` and is consumed by `bubbles/list` as text input.
- [ ] When `m.sessionList.SettingFilter()` is true, `NewPreviewModel` is NOT called (verified via mock counter).
- [ ] After `Enter` commits the filter, `Space` opens preview on the highlighted match.
- [ ] No second key binding exists for "open preview while filtering" (verified by code review — exactly one `Space` branch in `updateSessionList`).
- [ ] A literal space character is observably present in `m.sessionList.FilterValue()` after typing `Space` during `SettingFilter()`.

**Tests**:
- `"it inserts a literal space into the filter while SettingFilter"` — start filter input mode (drive a `/` key or whatever bubble/list uses to enter filter mode), type `pigeon`, then Space; assert `m.sessionList.FilterValue()` contains `"pigeon "` and `NewPreviewModel` was not called.
- `"it does not open preview while SettingFilter"` — same setup, drive Space; assert `m.activePage != pagePreview`.
- `"Space after Enter-commit opens preview on the highlighted match"` — drive filter input, type `pigeon`, Enter, then Space; assert `m.activePage == pagePreview` and the previewed session matches the highlighted item.
- `"it does not register a second open-preview binding for filter mode"` — code-level test: assert no key in the keymap has `Space` while `SettingFilter` is true that fires preview. (Can be enforced via the test that the Space-during-SettingFilter path consumes the message via `m.sessionList.Update` only.)

**Edge Cases**:
- A space character at the start of the filter (typed before any other character) — passthrough must still work; `bubbles/list` accepts leading spaces in filter text.
- A space typed when the filter is empty — same passthrough, no preview.
- `Enter` on an empty filter — `bubbles/list`'s standard behaviour governs whether the filter "commits" to empty or cancels; whatever `bubbles/list` does, preview's branch must not interfere.

**Context**:
> Spec § *Filter Behaviour with Preview* documents the two-phase filter interaction explicitly: "Filtering" (text input) vs "Filter committed" (cursor mode). Spec mandates "Preview does **not** intercept `Space` while filtering. There is no 'magic Space' that commits and previews in one step. There is no second binding for 'open preview' while filtering." Spec § *Filter Behaviour with Preview > Why this rather than magic Space* notes that allowing literal-space typing in the filter is a hard requirement.

**Spec Reference**: `.workflows/session-scrollback-preview/specification/session-scrollback-preview/specification.md` § *Filter Behaviour with Preview*

---

## session-scrollback-preview-2-6 | approved

### Task 2-6: Viewport scroll keys and resize handling within preview

**Problem**: Inside preview, scroll keys must scroll within the focused pane's loaded N-line slice, with scroll-up at the top silently no-opping (the tail-N slice is a hard top edge — no deeper history extend in v1). `bubbles/viewport`'s `DefaultKeyMap` covers `Up`, `Down`, `PgUp`, `PgDn`, `ctrl-u`, `ctrl-d`, `j`, `k` natively (verifiable in `bubbles@v1.0.0/viewport/keymap.go`); `Home` and `End` are NOT in the default keymap and need explicit handling in preview's Update — calling `m.viewport.GotoTop()` / `m.viewport.GotoBottom()`. Window resize during preview must re-flow the viewport without triggering a fresh disk read (the loaded buffer is decoupled from viewport dimensions). Drag-resize fires many events; none must incur tail-N read cost.

**Solution**: In `previewModel.Update`, delegate `tea.KeyMsg` events for scroll keys directly to the embedded `viewport.Model.Update` (passing the message through). Intercept `tea.WindowSizeMsg` to update `m.width`/`m.height` and resize the embedded viewport, but do NOT call `m.reader.Tail`. `bubbles/viewport` handles scroll boundaries natively (no-op at top and bottom).

**Outcome**: With preview open on a session whose tail-N read returned non-trivial bytes, pressing `Down` scrolls the viewport content down by one line; pressing `End` jumps to the bottom; pressing `Home` jumps to the top; pressing `Up` at the top is a silent no-op (no error, no flicker). Pressing `tea.WindowSizeMsg{Width: 120, Height: 40}` reflows the viewport at the new dimensions; scroll offset is preserved by `bubbles/viewport`'s native resize handling. Across many resize messages in succession (simulating drag-resize), `m.reader.Tail` is called exactly zero additional times beyond the initial-open call.

**Do**:
- In `previewModel.Update`, after the Esc branch (task 2-4) and before any cycle-key handlers (Phase 3), intercept `tea.KeyHome` and `tea.KeyEnd` explicitly — `bubbles@v1.0.0/viewport/keymap.go::DefaultKeyMap` does NOT bind these. Concretely:
  ```go
  case msg.Type == tea.KeyHome:
      m.viewport.GotoTop()
      return m, nil
  case msg.Type == tea.KeyEnd:
      m.viewport.GotoBottom()
      return m, nil
  ```
- For the remaining scroll keys (`Up`, `Down`, `PgUp`, `PgDn`, `ctrl-u`, `ctrl-d`, `j`, `k`), default-delegate `tea.KeyMsg` events to `m.viewport, cmd = m.viewport.Update(msg)` — the viewport's default keymap covers exactly these eight via `PageDown`, `PageUp`, `HalfPageUp`, `HalfPageDown`, `Down`, `Up`, `Left`, `Right` bindings (note: `Left`/`Right` are also passed through as harmless no-ops since the loaded N-line slice has no horizontal scroll dimension).
- Add a `tea.WindowSizeMsg` branch:
  - Update `m.width = msg.Width`, `m.height = msg.Height`.
  - Set `m.viewport.Width = msg.Width` and `m.viewport.Height = msg.Height` directly — `bubbles@v1.0.0/viewport.Model` exposes `Width` and `Height` as exported fields; there is no `SetSize` method in this version.
  - Do NOT call `m.reader.Tail`.
  - Return updated model with no command (or any non-`Tail` command needed by viewport).
- Confirm by code review that `m.reader.Tail` is invoked only from `NewPreviewModel` (task 2-2). Phase 3 will add focus-change reads; this phase has only the initial-open read.
- Note: chrome takes vertical space (Phase 3); for Phase 2 the viewport occupies the full terminal. Resize uses the full terminal dimensions.

**Acceptance Criteria**:
- [ ] `Down` scrolls the viewport down by one line; observable via `m.viewport.YOffset` increasing.
- [ ] `Up` scrolls the viewport up by one line.
- [ ] `Up` at the top of the loaded slice (`YOffset == 0`) is a silent no-op (no error, `YOffset` stays at 0).
- [ ] `Down` at the bottom of the loaded slice is a silent no-op (`YOffset` stays at max).
- [ ] `Home` jumps to `YOffset == 0` via preview-owned `m.viewport.GotoTop()` (the viewport library does not bind `Home` natively).
- [ ] `End` jumps to the bottom of the loaded slice via preview-owned `m.viewport.GotoBottom()` (the viewport library does not bind `End` natively).
- [ ] `PgUp`, `PgDn`, `ctrl-u`, `ctrl-d`, `j`, `k` all behave per `bubbles/viewport` defaults (no preview-specific overrides).
- [ ] `tea.WindowSizeMsg` resizes the viewport without calling `m.reader.Tail`.
- [ ] 100 successive `tea.WindowSizeMsg` events trigger zero additional `Tail` calls beyond the initial-open call (drag-resize simulation).
- [ ] Scroll offset is preserved across a `tea.WindowSizeMsg` (verified by setting `YOffset = 5`, then resizing, then asserting `YOffset == 5`).

**Tests**:
- `"it scrolls down on Down key"` — open preview with multi-line content, drive Down; assert `YOffset` increased.
- `"it silently no-ops scroll-up at the top"` — open preview, drive Up while `YOffset == 0`; assert `YOffset == 0` and no error.
- `"it silently no-ops scroll-down at the bottom"` — synthesise `tea.KeyMsg{Type: tea.KeyEnd}` to jump to bottom, then drive Down; assert `YOffset` unchanged.
- `"it jumps to top on Home via preview-owned binding"` — scroll down with PgDn, then synthesise `tea.KeyMsg{Type: tea.KeyHome}`; assert `YOffset == 0`. Recipe note: the test exercises preview's own Home interception (`m.viewport.GotoTop()`), not viewport's default keymap, which has no Home binding in `bubbles@v1.0.0`.
- `"it jumps to bottom on End via preview-owned binding"` — synthesise `tea.KeyMsg{Type: tea.KeyEnd}`; assert `YOffset` equals viewport's max offset for the loaded slice. Recipe note: same as Home — preview owns this via `m.viewport.GotoBottom()`.
- `"it does NOT call Tail on WindowSizeMsg"` — open preview (records 1 Tail call), send `tea.WindowSizeMsg{Width: 120, Height: 40}`; assert `Tail` call count remains 1.
- `"it does NOT call Tail across 100 successive WindowSizeMsg events"` — open preview, send 100 resize messages with varying dimensions; assert `Tail` count remains 1.
- `"it preserves scroll offset across resize"` — scroll to a known offset, resize, assert offset preserved.

**Edge Cases**:
- Empty viewport content (e.g. `(nil, nil)` from `Tail`) — scroll keys must not error; `YOffset` stays at 0.
- Single-line content — scroll-down must be a no-op.
- Resize to smaller dimensions where current `YOffset` exceeds new max — `bubbles/viewport`'s native resize handling clamps offset; preview must not interfere.

**Context**:
> Spec § *Within-preview Key Bindings > Keymap policy* mandates "Up/Down are explicitly **not** between-session navigation — they scroll the focused viewport." Spec § *History Depth > Scroll within bounds* declares "The top boundary of the slice is a hard edge — pressing scroll-up at the top silently no-ops. Deeper history (beyond N) is not reachable in v1." Spec § *Refresh Semantics > Read Trigger Events* states "Resize is not a read trigger" and emphasises this matters for the performance budget on drag-resize. Spec § *Interaction Shape > Layout* mandates that `tea.WindowSizeMsg` is forwarded to the embedded viewport so the slice re-flows on resize, with scroll offset preserved by `bubbles/viewport`'s native resize handling.

**Spec Reference**: `.workflows/session-scrollback-preview/specification/session-scrollback-preview/specification.md` § *Within-preview Key Bindings*, § *History Depth*, § *Refresh Semantics > Read Trigger Events*, § *Interaction Shape > Layout*

---

## session-scrollback-preview-2-7 | approved

### Task 2-7: Production adapters wired at TUI construction

**Problem**: The seam interfaces declared in task 2-1 need concrete production adapters wiring `*tmux.Client` (for enumeration) and the Phase-1 tail-N helper (for scrollback reads) to satisfy them. `stateDir` must be resolved once at TUI startup via the existing `internal/state` paths helper and closed over inside the `ScrollbackReader` adapter so preview never has its own state-path resolution policy. Pane key derivation must use `state.SanitizePaneKey(session, window_index, pane_index)` matching the daemon's writer call site verbatim. Tests in `pagepreview_test.go` must NOT import `tmuxtest`.

**Solution**: Add a small adapter type implementing `ScrollbackReader.Tail(paneKey)` that closes over `stateDir` (resolved at construction) and calls `state.TailScrollback(state.ScrollbackFile(stateDir, paneKey), 1000)`. Confirm `*tmux.Client` already satisfies `TmuxEnumerator` (the Phase 1 method `ListWindowsAndPanesInSession(session)` matches the interface signature). Modify the TUI construction site to resolve `stateDir`, build the adapter, and pass both seams into the root `Model`.

**Outcome**: When the TUI is started in production (via the `cmd` layer), the root `Model` carries a real `*tmux.Client` as the `TmuxEnumerator` and a `scrollbackReaderAdapter{stateDir, n: 1000}` as the `ScrollbackReader`. Preview reads from real `.bin` files on disk; structural enumeration runs against the real tmux server. Tests live in `internal/tui/pagepreview_test.go`, build with mocks (no real adapters), and do not import `tmuxtest`. The `stateDir` is captured exactly once during TUI startup and is stable for the Portal process lifetime.

**Do**:
- In `internal/tui` (e.g. `internal/tui/preview_adapter.go`), add:
  ```go
  type scrollbackReaderAdapter struct {
      stateDir string
      n        int // 1000 per spec
  }

  func (a scrollbackReaderAdapter) Tail(paneKey string) ([]byte, error) {
      path := state.ScrollbackFile(a.stateDir, paneKey)
      return state.TailScrollback(path, a.n)
  }
  ```
  - Confirm `state.TailScrollback` is the Phase 1 helper signature; if the actual exported name differs, use whatever Phase 1 landed.
- Confirm `*tmux.Client` already satisfies `TmuxEnumerator` via the Phase 1 `ListWindowsAndPanesInSession` method. Add a compile-time assertion in a non-test file: `var _ TmuxEnumerator = (*tmux.Client)(nil)`.
- Define a constant `const previewTailLines = 1000` (or wherever the build phase prefers, per spec § *Open Items*).
- At the TUI construction site (the function that builds the root `Model` — typically in `internal/tui/model.go` or wherever the constructor lives, or at the cmd-layer call site that builds the model):
  - Resolve `stateDir` via the existing `internal/state` paths helper (the same source the daemon and bootstrap orchestrator already use — review `internal/state/paths.go`).
  - Construct `reader := scrollbackReaderAdapter{stateDir: stateDir, n: previewTailLines}`.
  - Pass `tmuxClient` (already available as `*tmux.Client`) and `reader` into the root `Model` (assigned to the `enumerator` and `reader` fields added in task 2-3).
  - `stateDir` resolution must happen exactly once per Portal process — confirm by code review that the construction is at the top-level startup path, not inside a per-frame or per-open call.
- Create `internal/tui/pagepreview_test.go` if not already present from earlier tasks. Confirm the file does NOT import `github.com/leeovery/portal/internal/tmuxtest` or any tmux-real-server fixture.
- Confirm `state.SanitizePaneKey(session, window_index, pane_index)` is the exact call used in `previewModel` (task 2-2) and that it matches the daemon's writer call site — open `internal/state/scrollback.go` (or wherever the daemon writes `.bin` files) and verify the same helper signature is used at both sites.

**Acceptance Criteria**:
- [ ] `scrollbackReaderAdapter` exists in `internal/tui` and satisfies `ScrollbackReader`.
- [ ] `scrollbackReaderAdapter.Tail` resolves the path via `state.ScrollbackFile(stateDir, paneKey)` and reads via `state.TailScrollback(path, 1000)`.
- [ ] `*tmux.Client` satisfies `TmuxEnumerator` (compile-time assertion present).
- [ ] `stateDir` is captured exactly once at TUI construction; no per-open re-resolution.
- [ ] `previewTailLines == 1000` per spec.
- [ ] Pane key derivation in `previewModel` uses `state.SanitizePaneKey(session, window_index, pane_index)` with the same argument shape as the daemon's writer.
- [ ] `internal/tui/pagepreview_test.go` does not import `tmuxtest` (grep-verifiable).
- [ ] `pagepreview_test.go` uses `t.Parallel()` safely (no package-level mutable seam state to clean up).
- [ ] The production wiring is exercised by an end-to-end manual smoke test: build portal, attach to a real session, press `Space` on a session in the Sessions list, observe the preview page rendering content from disk.

**Tests**:
- `"scrollbackReaderAdapter satisfies the ScrollbackReader interface"` — compile-time `var _ ScrollbackReader = scrollbackReaderAdapter{}`.
- `"*tmux.Client satisfies the TmuxEnumerator interface"` — compile-time assertion.
- `"scrollbackReaderAdapter.Tail returns bytes for a valid pane key"` — integration test with a temp `stateDir` containing a known `.bin` file; assert returned bytes equal the expected tail.
- `"scrollbackReaderAdapter.Tail returns (nil, nil) for missing .bin"` — temp `stateDir` with no file for the paneKey; assert `(nil, nil)`.
- `"scrollbackReaderAdapter.Tail returns (nil, err) for permission denied"` — temp `stateDir` with a `.bin` file the test process cannot read (chmod 000); assert error and bytes nil.
- `"pagepreview_test.go does not import tmuxtest"` — `grep -r "tmuxtest" internal/tui/pagepreview_test.go` returns no matches; can be enforced as a static-analysis test.
- `"stateDir is captured once at TUI construction"` — call site review: `state` paths helper is called once during `NewModel` (or equivalent), not per `Space` press.
- `"pane key matches daemon writer"` — read both call sites and assert identical argument shape (the test can be a doc/code-review item if a programmatic assertion is awkward).

**Edge Cases**:
- The `stateDir` resolution helper consults `$PORTAL_STATE_DIR` first, then falls back through `$XDG_CONFIG_HOME/portal/state` to `$HOME/.config/portal/state` (per `internal/state/paths.go::Dir`) — preview must use this exact helper so both preview and the daemon resolve to the same directory; otherwise preview reads from an empty dir.
- If `state.TailScrollback` is the Phase 1 export but Phase 1 used a different name (e.g. `state.Tail` or `state.ReadTail`), use whatever name Phase 1 landed.
- The compile-time assertion `var _ TmuxEnumerator = (*tmux.Client)(nil)` may live in any non-test file in `internal/tui` — but it must be in a place that prevents accidental signature drift.

**Context**:
> Spec § *Cross-cutting Seams > State Package API Reuse > stateDir resolution* mandates "stateDir is captured once and stable for the Portal process lifetime; it is not re-resolved per preview-open." Spec § *Architecture Summary > Test seams* mandates "production-code adapters wire `*tmux.Client` and the tail-N helper to the seams at TUI construction." Spec § *Architecture Summary > Wiring shape* mandates "no `tmuxtest` import" in `pagepreview_test.go`. Spec § *Cross-cutting Seams > State Package API Reuse* requires `state.SanitizePaneKey` arguments matching the daemon's writer call site verbatim — "the goal is byte-identical pane keys."

**Spec Reference**: `.workflows/session-scrollback-preview/specification/session-scrollback-preview/specification.md` § *Cross-cutting Seams > State Package API Reuse*, § *Architecture Summary > Test seams*, § *Architecture Summary > Wiring shape*
