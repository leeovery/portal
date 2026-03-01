TASK: Extract shared view-list-with-modal rendering helper (tick-f56bab)

ACCEPTANCE CRITERIA:
- Dimension-fallback logic (w==0 -> 80, h==0 -> 24) appears exactly once
- viewSessionList and viewProjectList delegate to the shared helper
- All existing tests pass

STATUS: Complete

SPEC CONTEXT: The spec describes a modal overlay system where lipgloss.Place() (later replaced by a custom overlay) positions styled content over the list output in View(). Both the Sessions and Projects pages use this same pattern for their respective modals (kill confirm, rename, delete confirm, edit project). The dimension fallback is needed because list dimensions may be zero before a WindowSizeMsg arrives.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `/Users/leeovery/Code/portal/internal/tui/modal.go:27-46` - `renderListWithModal` helper function
  - `/Users/leeovery/Code/portal/internal/tui/model.go:1122-1131` - `viewProjectList` delegates to helper (line 1130)
  - `/Users/leeovery/Code/portal/internal/tui/model.go:1181-1190` - `viewSessionList` delegates to helper (line 1189)
- Notes:
  - The helper `renderListWithModal(l list.Model, modalContent string) string` takes a list model and optional modal content string. When content is empty, returns plain list view. When non-empty, applies dimension fallback (w==0->80, h==0->24) and calls `renderModal`.
  - Dimension-fallback logic appears exactly once at modal.go:38-43. Confirmed via grep that no other file contains `w == 0` or `h == 0` fallback patterns.
  - Both `viewProjectList` and `viewSessionList` follow the same clean pattern: compute modal content based on current modal state, then call `renderListWithModal`. No duplication of fallback or overlay logic.
  - The helper is a standalone function (not a method on Model), which is appropriate since it only needs the list model and content string.

TESTS:
- Status: Adequate
- Coverage:
  - `renderModal` is directly tested in `/Users/leeovery/Code/portal/internal/tui/modal_test.go` with 5 subtests covering: content visibility, non-empty output, differs from plain view, border styling, and ANSI-aware centering.
  - `renderListWithModal` is exercised indirectly through numerous integration tests in `model_test.go` that call `model.View()` and verify modal content appears with border styling. These include kill confirmation tests (lines ~1370, 1653, 1675, 1718, 2973, 3090), rename modal tests (line ~1812), delete project modal tests (lines ~3899, 4085, 4244), and edit project modal tests (line ~4829).
  - No dedicated unit test for `renderListWithModal` itself, but the function is a thin wrapper (6 lines of logic) that composes two well-tested pieces. The integration tests adequately verify end-to-end behavior.
  - The task's micro-acceptance criteria state "All existing view rendering tests pass unchanged" and "Modal overlay tests for both pages continue to produce correct output" -- the existing tests cover both pages' modal overlays.
- Notes: Test coverage is appropriate. A dedicated unit test for `renderListWithModal` would add minimal value given the function's simplicity and extensive integration coverage.

CODE QUALITY:
- Project conventions: Followed. Clean Go idioms, proper documentation comments on exported/package-level items.
- SOLID principles: Good. Single responsibility -- `renderListWithModal` handles only the dimension-fallback + delegation pattern. `renderModal` handles the actual overlay compositing. Clean separation.
- Complexity: Low. The helper is 15 lines including the doc comment. Two simple conditionals for fallback, one delegation call.
- Modern idioms: Yes. Uses `max()` builtin (Go 1.21+), proper string handling, ANSI-aware width measurement.
- Readability: Good. Function name clearly describes intent. Doc comment explains the fallback behavior. The callers (`viewProjectList`, `viewSessionList`) are now clean and focused on computing modal content.
- Issues: None.

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- None
