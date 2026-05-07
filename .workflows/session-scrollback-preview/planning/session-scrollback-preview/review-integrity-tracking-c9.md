---
status: in-progress
created: 2026-05-06
cycle: 9
phase: Plan Integrity Review
topic: session-scrollback-preview
---

# Review Tracking: Session Scrollback Preview - Integrity

Two findings — one critical library-API drift sibling to cycle 8's Home/End fix, and
one minor field-name drift between plan content and the actual `internal/tui` `Model`
struct. Eight prior cycles have substantially hardened the plan; the residual issues
here are the kind that would surface on first compile or first failing test.

## Findings

### 1. Initial-open viewport never lands at scroll-tail

**Severity**: Critical
**Plan Reference**: Phase 2 task 2-2 (`previewModel constructor with injected seams and initial-open flow`)
**Category**: Task Template Compliance / Acceptance Criteria Quality (load-bearing library-API assumption)
**Change Type**: update-task

**Details**:
Task 2-2 step 5 sets the viewport content via `viewport.SetContent(string(bytes))` and returns. It never calls `m.viewport.GotoBottom()`. But the spec mandates "the viewport (`bubbles/viewport`) renders the tail by default" (§ *History Depth > Scroll within bounds*) and "Preview simply renders whatever lines are present, with the viewport at scroll-tail" (§ *Read-Failure Handling > Placeholder > Non-triggering condition*), and Phase 4 task 4-3 explicitly tests `AtBottom()` immediately after construction.

Verifying against the vendored `bubbles@v1.0.0/viewport/viewport.go` source: `SetContent` only auto-jumps to bottom when `m.YOffset > len(m.lines)-1`. On a freshly-constructed `viewport.Model`, `YOffset == 0`, so this branch is never taken on initial open — the viewport stays at scroll-top showing the **oldest** lines, not the tail. To satisfy the spec the constructor must call `m.viewport.GotoBottom()` after `SetContent`.

This is the same class of issue cycle 8 caught with `DefaultKeyMap` not binding Home/End — a load-bearing library-API assumption that doesn't match the actual library behaviour. Tasks 3-2 and 3-3 already correctly call `GotoBottom()` after focus-change reads; the inconsistency is that initial open does not.

Without this fix, the implementer follows the plan literally, the initial-open viewport renders at scroll-top, and task 4-3's `AtBottom() == true` assertion fails. The Phase 2 acceptance criterion ("the first frame renders viewport content atomically with the page transition") would technically pass while shipping the wrong default scroll position.

**Current**:
````
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
  6. Return `(model, true)`.
````

**Proposed**:
````
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
````

Also update Acceptance Criteria to add a scroll-tail invariant, and add a corresponding test:

**Current** (acceptance criteria block, last two bullets onwards):
````
- [ ] If `reader.Tail` returns `(bytes, nil)`, those exact bytes are the argument to `viewport.SetContent` (no transformation).
- [ ] Re-invoking `NewPreviewModel` for the same session triggers a fresh enumeration AND a fresh `Tail` call (no caching).
````

**Proposed**:
````
- [ ] If `reader.Tail` returns `(bytes, nil)`, those exact bytes are the argument to `viewport.SetContent` (no transformation).
- [ ] After `SetContent`, the viewport is at scroll-tail (`m.viewport.AtBottom()` returns true) — `GotoBottom()` is called explicitly because `bubbles@v1.0.0/viewport.SetContent` does not jump to bottom when `YOffset == 0`.
- [ ] Re-invoking `NewPreviewModel` for the same session triggers a fresh enumeration AND a fresh `Tail` call (no caching).
````

**Current** (tests block, after the verbatim-ANSI test):
````
- `"it passes raw ANSI bytes verbatim to viewport.SetContent"` — reader returns bytes containing ANSI escape sequences (e.g. `\x1b[31mred\x1b[0m`); assert viewport's content equals the input string byte-for-byte.
- `"it returns ok=true when Tail returns (nil, nil)"` — reader returns `nil, nil`; assert `ok==true`, viewport content is empty.
````

**Proposed**:
````
- `"it passes raw ANSI bytes verbatim to viewport.SetContent"` — reader returns bytes containing ANSI escape sequences (e.g. `\x1b[31mred\x1b[0m`); assert viewport's content equals the input string byte-for-byte.
- `"it positions the viewport at scroll-tail on initial open"` — fixture: bytes containing more lines than the viewport height; assert `m.viewport.AtBottom() == true` immediately after construction; regression-pin against any future change that drops the explicit `GotoBottom()`.
- `"it returns ok=true when Tail returns (nil, nil)"` — reader returns `nil, nil`; assert `ok==true`, viewport content is empty.
````

**Resolution**: Pending
**Notes**:

---

### 2. Plan references `m.width`/`m.height` on root `Model`; actual fields are `termWidth`/`termHeight`

**Severity**: Minor
**Plan Reference**: Phase 2 task 2-3 (`Add pagePreview arm to page state machine and bind Space on Sessions page`), step 4 of the Do section
**Category**: Task Self-Containment (codebase identifier accuracy)
**Change Type**: update-task

**Details**:
Task 2-3 step 4 says:

> Construct `previewModel := NewPreviewModel(sessionName, m.enumerator, m.reader, m.width, m.height)`.

But `internal/tui/model.go` line 174 declares the actual fields as:

```go
// Terminal dimensions (cached for re-applying after data loads)
termWidth, termHeight int
```

There is no `m.width` or `m.height` on the root `Model`. The fields are `m.termWidth` and `m.termHeight`, populated by the `tea.WindowSizeMsg` branch at line 700–705 of `model.go`. An implementer following the plan literally would write `m.width, m.height`, hit a compile error on the first build, and have to deduce the actual field names by reading `model.go`. Trivial to fix in flight, but the plan should be byte-accurate about codebase identifiers — every other instance in tasks 2-2..2-7 is precise (`m.sessionList`, `m.activePage`, `m.preview`, etc.), so this drift stands out.

Note: the `m.width`/`m.height` fields on the **`previewModel`** struct (declared in 2-2 and used in 2-6) are correct — those are new fields on the new struct. The drift is only on the root `Model` reference at the call site.

**Current**:
````
  4. Construct `previewModel := NewPreviewModel(sessionName, m.enumerator, m.reader, m.width, m.height)`. The seams `m.enumerator` and `m.reader` arrive from task 2-7's TUI construction; in this task the fields are added with placeholder zero values acceptable for compilation.
````

**Proposed**:
````
  4. Construct `previewModel := NewPreviewModel(sessionName, m.enumerator, m.reader, m.termWidth, m.termHeight)`. The seams `m.enumerator` and `m.reader` arrive from task 2-7's TUI construction; in this task the fields are added with placeholder zero values acceptable for compilation. The root `Model` already caches terminal dimensions in `termWidth` / `termHeight` — see `internal/tui/model.go` line 174 and the `tea.WindowSizeMsg` branch at line 700.
````

**Resolution**: Pending
**Notes**:
