---
topic: session-scrollback-preview
cycle: 1
total_proposed: 5
---
# Analysis Tasks: session-scrollback-preview (Cycle 1)

## Task 1: Extract applySessions helper to deduplicate session-list refresh
status: approved
severity: medium
sources: duplication

**Problem**: The `SessionsMsg` handler (`internal/tui/model.go:802-809`) and the new `previewSessionsRefreshedMsg` handler (`internal/tui/model.go:896-901`) both run the same four-step sequence — assign sessions, compute `filteredSessions()`, call `sessionList.SetItems(ToListItems(...))`, and conditionally re-apply terminal size. The post-preview-refresh arm is a near-verbatim copy.

**Solution**: Extract `(*Model).applySessions(sessions []tmux.Session)` colocated with `filteredSessions` that performs assign → filter → SetItems → conditional SetSize. Call it from both handler arms. Keep the inside-tmux title rewrite at the `SessionsMsg` call site.

**Outcome**: One canonical place for "ingest a fresh session slice into the list model"; both handlers reduced to a single call plus their handler-specific tail.

**Do**:
1. Add a method `(m *Model) applySessions(sessions []tmux.Session)` near `filteredSessions` in `internal/tui/model.go`.
2. Body: assign `m.sessions = sessions`, compute `filtered := m.filteredSessions()`, call `m.sessionList.SetItems(ToListItems(filtered))`, and re-apply `SetSize` when `m.width != 0 && m.height != 0`.
3. Replace the duplicated block at `internal/tui/model.go:802-809` with `m.applySessions(msg.Sessions)`. Preserve the inside-tmux title rewrite verbatim.
4. Replace the duplicated block at `internal/tui/model.go:896-901` with `m.applySessions(msg.Sessions)`.

**Acceptance Criteria**:
- Both handlers compile and produce identical observable behaviour.
- The four-step sequence appears only once in the file.
- The inside-tmux title rewrite remains at the `SessionsMsg` call site.

**Tests**:
- Existing TUI tests covering `SessionsMsg` and `previewSessionsRefreshedMsg` continue to pass without modification.

---

## Task 2: Unify previewModel receiver discipline
status: approved
severity: medium
sources: architecture

**Problem**: `previewModel` mixes value and pointer receivers around viewport mutation (`internal/tui/pagepreview.go:193, 241, 69`). Most lifecycle methods are value receivers while `readFocusedPaneIntoViewport` is the lone pointer receiver. Works today because `viewport.Model`'s content survives a value copy, but a future field that does not survive value-copy (mutex, channel, atomic) would silently break the helper's mutations.

**Solution**: Pick one receiver discipline uniformly. Smallest change: make `readFocusedPaneIntoViewport` return the updated `viewport.Model` so the caller assigns it onto `m.viewport`, keeping the surrounding value-receiver style consistent.

**Outcome**: Every `previewModel` method shares the same receiver style; no latent bug class around future non-copyable fields.

**Do**:
1. In `internal/tui/pagepreview.go`, change `readFocusedPaneIntoViewport` from a pointer-receiver mutating method into a value-receiver method (or a free function) that returns the updated `viewport.Model`.
2. Update the call site in `NewPreviewModel` (around line 193) to assign the returned value: `m.viewport = m.readFocusedPaneIntoViewport()`.
3. Update the call sites inside `Update`'s Tab / `]` / `[` branches (around lines 261, 280, 288) to assign the returned value before `return m, nil`.
4. Verify no remaining method on `previewModel` uses a pointer receiver.

**Acceptance Criteria**:
- All `previewModel` methods use value receivers.
- All cycle branches still re-read the focused pane after mutation; viewport content updates are observable to `View()`.
- `NewPreviewModel` returns a fully-initialised value with viewport already populated.

**Tests**:
- Existing preview tests covering pane cycling and initial render continue to pass.

---

## Task 3: Drop `#W:` prefix from preview chrome
status: approved
severity: low
sources: standards

**Problem**: The chrome format string at `internal/tui/pagepreview.go:165-168` embeds the literal text `#W:` as a user-facing label. In the spec, `#W` is a tmux format-code reference identifying which value to display — not user-facing text. Users unfamiliar with tmux will read `#W:` as an opaque label.

**Solution**: Drop the `#W:` prefix; the preceding `Window %d of %d` counter contextualises the trailing window name without need for an explicit label.

**Outcome**: Chrome reads as natural prose to users who have never seen a tmux format code.

**Do**:
1. Edit the format string at `internal/tui/pagepreview.go:165-168` from `"Window %d of %d · Pane %d of %d · #W: %s    ] [ Tab Esc"` to `"Window %d of %d · Pane %d of %d · %s    ] [ Tab Esc"`.
2. Update any chrome-line snapshot/golden assertions in tests to drop `#W: `.

**Acceptance Criteria**:
- Rendered chrome no longer contains the literal substring `#W:`.
- Window name still appears, separated from the pane counter by ` · `.
- All preview tests pass after assertion updates.

**Tests**:
- Update existing chrome-line tests in `internal/tui/pagepreview_chrome_test.go` (and any other chrome-aware tests) to match the new wording.

---

## Task 4: Correct View() doc-comment about chrome placement
status: approved
severity: low
sources: standards

**Problem**: The doc-comment on `View()` at `internal/tui/pagepreview.go:300-306` claims header-on-top is "fixed in v1 per § Interaction Shape > Layout". The spec actually defers final placement to a build-phase decision per § Open Items > Chrome Floor.

**Solution**: Tighten the doc-comment to attribute the orientation choice correctly.

**Outcome**: Doc-comment matches the spec's actual wording; future readers know which constraints are spec-fixed and which are build-phase choices.

**Do**:
1. In `internal/tui/pagepreview.go` near `View()`, replace the existing rationale with wording such as: "Header-on-top is the build-phase choice (spec § Open Items > Chrome Floor defers placement); only `previewChromeHeight` and this orientation change if footer is later preferred."
2. If the type-level comment carries the same misattribution, update it to match.

**Acceptance Criteria**:
- No doc-comment in `pagepreview.go` claims spec § Interaction Shape > Layout fixes the header-on-top orientation.
- The build-phase nature of the choice is explicit.

**Tests**:
- No test changes; comment-only edit.

---

## Task 5: Add invariant comments around preview lifecycle fragilities
status: approved
severity: low
sources: standards, architecture

**Problem**: Three small lifecycle fragilities in the preview code have no inline guard:
1. The Home/End interception at `internal/tui/pagepreview.go:253-258` is necessary because `bubbles/viewport@v1.0.0`'s `DefaultKeyMap` does not bind Home/End — but the code does not record that rationale.
2. The dismiss handler at `internal/tui/model.go:882` reads `m.preview.session` then zeroes `m.preview` then dispatches `refreshSessionsAfterPreviewCmd(preserveName)`. A refactor that flips read-then-zero would silently send empty `PreserveName`.
3. `previewModel.session` (`internal/tui/pagepreview.go:46`) is identity-bearing but the type carries no comment forbidding method calls on the zero value.

**Solution**: Add three short inline comments locking each invariant. No behaviour change.

**Outcome**: Each fragility is self-documenting.

**Do**:
1. Above the Home/End case, add: `// viewport.DefaultKeyMap (bubbles@v1.0.0) does not bind Home/End; preview must own them to satisfy the acceptance criterion.`
2. At the preserveName capture site, add: `// Capture preserveName before zeroing m.preview below — flipping the order silently sends an empty value.`
3. On the `previewModel` type or its `session` field, add: `// Zero value reserved for "between opens"; methods must not be called on a zero previewModel.`

**Acceptance Criteria**:
- All three comments present at the indicated locations.
- No code-path behaviour change; tests pass without modification.

**Tests**:
- No test changes; comment-only edit.
