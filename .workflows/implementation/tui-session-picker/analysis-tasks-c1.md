---
topic: tui-session-picker
cycle: 1
total_proposed: 5
---
# Analysis Tasks: tui-session-picker (Cycle 1)

## Task 1: Replace ANSI-unaware placeOverlay with lipgloss.Place
status: pending
severity: high
sources: architecture

**Problem**: `placeOverlay` in `internal/tui/modal.go:37-67` operates on raw runes, treating each rune as one display column. ANSI escape sequences from lipgloss styling (bold session names, colored attached badges, modal borders) are zero-width on screen but occupy runes in the string. This causes the modal overlay to misalign horizontally when composited over styled background content. The specification explicitly states modals should use `lipgloss.Place()` for positioning.

**Solution**: Replace the custom `placeOverlay` function with `lipgloss.Place()` for centering the modal content over the list view. The `renderModal` function should use lipgloss's ANSI-aware layout primitives instead of manual rune-based compositing.

**Outcome**: Modal overlays render correctly centered regardless of ANSI styling in the background content. The custom `placeOverlay` function is removed entirely.

**Do**:
1. In `internal/tui/modal.go`, modify `renderModal` to use `lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, styledModal, lipgloss.WithWhitespaceChars(" "))` or equivalent lipgloss centering to position the modal
2. If lipgloss.Place does not support compositing over existing content, use `lipgloss.PlaceHorizontal` and `lipgloss.PlaceVertical` with the styled modal, then overlay that onto the background using ANSI-aware width measurement (`lipgloss.Width()`) instead of `len([]rune(...))`
3. Delete the `placeOverlay` function
4. Verify existing modal tests still pass
5. Manually verify modals display correctly over styled session and project lists

**Acceptance Criteria**:
- `placeOverlay` function is removed from `internal/tui/modal.go`
- `renderModal` uses lipgloss layout functions for positioning
- All existing modal-related tests pass without modification
- Modal overlays center correctly over styled list content

**Tests**:
- Existing tests in model_test.go covering kill confirmation, rename, delete, and edit modals continue to pass
- Add a test that renders a modal over content containing ANSI escape sequences and verifies the modal is horizontally centered (using lipgloss.Width for measurement)

## Task 2: Unify duplicated modal dispatch into single method
status: pending
severity: medium
sources: duplication, architecture

**Problem**: `updateModal` (line 980) and `updateProjectModal` (line 704) in `internal/tui/model.go` are structural clones -- both check for Ctrl+C quit, then switch on `m.modal` to dispatch to specific modal handlers. The only difference is which modal constants they handle. Since `modalState` is a single shared enum, a new modal type requires edits in two places.

**Solution**: Merge into a single `updateModal` method that handles all modal states (kill, rename, deleteProject, editProject). Both `updateSessionList` and `updateProjectsPage` call the same unified dispatcher.

**Outcome**: One modal dispatch point. Adding a new modal type requires a single switch case addition.

**Do**:
1. In `internal/tui/model.go`, merge the body of `updateProjectModal` into `updateModal` by adding cases for `modalDeleteProject` and `modalEditProject` to the existing switch
2. Delete `updateProjectModal`
3. Update `updateProjectsPage` (the caller of `updateProjectModal`) to call `updateModal` instead
4. Verify the Ctrl+C guard appears exactly once in the unified method
5. Run all tests

**Acceptance Criteria**:
- Only one modal dispatch method exists (`updateModal`)
- `updateProjectModal` is deleted
- The unified method handles all four modal states: `modalKillConfirm`, `modalRename`, `modalDeleteProject`, `modalEditProject`
- All existing tests pass

**Tests**:
- All existing modal tests (kill, rename, delete project, edit project) pass unchanged
- Ctrl+C during any modal state still triggers quit

## Task 3: Extract shared view-list-with-modal rendering helper
status: pending
severity: medium
sources: duplication

**Problem**: `viewProjectList` (lines 1123-1143) and `viewSessionList` (lines 1193-1214) in `internal/tui/model.go` share identical structure: get list view string, extract width/height with fallback to 80x24 when zero, then switch on modal state to overlay content. The dimension-fallback block is duplicated verbatim.

**Solution**: Extract a helper function that takes a `list.Model` and a function that returns optional modal content, handles the dimension fallback, and applies the modal overlay if present.

**Outcome**: Dimension-fallback logic exists in one place. Adding a new modal type on either page only requires updating the modal content provider.

**Do**:
1. Create a helper method on Model (or a standalone function) like `renderListWithModal(l list.Model, modalContent string, hasModal bool) string` that: gets `l.View()`, extracts width/height with 80/24 fallback, and calls `renderModal` if `hasModal` is true
2. Refactor `viewSessionList` to compute its modal content and call the helper
3. Refactor `viewProjectList` to compute its modal content and call the helper
4. Run all tests

**Acceptance Criteria**:
- Dimension-fallback logic (w==0 -> 80, h==0 -> 24) appears exactly once
- `viewSessionList` and `viewProjectList` delegate to the shared helper
- All existing tests pass

**Tests**:
- All existing view rendering tests pass unchanged
- Modal overlay tests for both pages continue to produce correct output

## Task 4: Eliminate duplicated window-label pluralization in SessionDelegate.Render
status: pending
severity: medium
sources: duplication

**Problem**: Window count pluralization logic is implemented identically in both `SessionItem.Description()` (lines 39-42) and `SessionDelegate.Render()` (lines 80-83) in `internal/tui/session_item.go`. The `Description()` method already computes the correct label, but `Render()` re-derives it independently.

**Solution**: Have `SessionDelegate.Render()` use the already-computed description logic rather than re-implementing it. Either call a shared helper or restructure so the pluralization exists once.

**Outcome**: Window-label pluralization logic exists in exactly one place. Changes to the label format only require one edit.

**Do**:
1. In `internal/tui/session_item.go`, extract a `windowLabel(count int) string` function that returns the pluralized string
2. Update `SessionItem.Description()` to call `windowLabel(i.Session.Windows)`
3. Update `SessionDelegate.Render()` to call `windowLabel(si.Session.Windows)` and apply `detailStyle.Render()` to the result
4. Run all tests

**Acceptance Criteria**:
- Pluralization logic (`"%d windows"` with special case for 1) appears exactly once
- `Description()` and `Render()` both use the shared helper
- All existing session item and delegate tests pass

**Tests**:
- Existing tests for SessionItem.Description and SessionDelegate.Render pass unchanged
- Verify that `windowLabel(1)` returns "1 window" and `windowLabel(3)` returns "3 windows"

## Task 5: Fix command-pending status line position to below title
status: pending
severity: low
sources: standards

**Problem**: In `internal/tui/model.go:1106-1113`, the command-pending status line ("Select project to run: {command}") is prepended before the project list view, causing it to render above the "Projects" title. The spec states: "Title stays 'Projects' for consistency" and "A status line below the title indicates the pending command".

**Solution**: Move the status line so it appears below the list title rather than above it. This can be done by setting the status line as the list's subtitle or by injecting it after the title row in the rendered output.

**Outcome**: When in command-pending mode, the rendering order is: "Projects" title, then status line, then list items -- matching the specification.

**Do**:
1. In `internal/tui/model.go`, in the `View()` method's `commandPending` branch (lines 1106-1113), instead of prepending the status line before `viewProjectList()`, set the status message on the project list model before rendering (e.g., use `m.projectList.NewStatusMessage()` or set a subtitle-style field)
2. If bubbles/list does not support subtitle injection, split the list View output string at the first newline (title line) and insert the status line after it
3. Remove the current `strings.Builder` prepend approach
4. Run all tests

**Acceptance Criteria**:
- In command-pending mode, the "Projects" title renders first, followed by the status line, followed by list items
- The status line text remains "Select project to run: {command}"
- All existing command-pending tests pass

**Tests**:
- Existing command-pending view tests pass (may need assertion updates if they checked the old ordering)
- Add or update a test that verifies the status line appears after the title line in the rendered output
