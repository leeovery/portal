TASK: Session List Item and Custom ItemDelegate (tick-5d021f)

ACCEPTANCE CRITERIA:
- SessionItem implements list.Item interface (compiler check)
- FilterValue() returns the session name
- SessionDelegate implements list.ItemDelegate interface (compiler check)
- Render output contains session name, window count, and attached badge when attached
- Window count uses "1 window" (singular) and "N windows" (plural) correctly
- Long session names render without truncation
- Non-attached sessions do not show the attached badge
- toListItems correctly converts []tmux.Session to []list.Item

STATUS: Complete

SPEC CONTEXT: The spec says "Sessions show the session name, window count, and attached badge" via a custom ItemDelegate. The tmux.Session struct has fields Name string, Windows int, Attached bool. The bubbles/list integration requires implementing list.Item and list.ItemDelegate interfaces.

IMPLEMENTATION:
- Status: Implemented
- Location: /Users/leeovery/Code/portal/internal/tui/session_item.go:1-102
- Notes:
  - SessionItem wraps tmux.Session and implements list.Item via FilterValue() (line 35-37)
  - Title() returns session name (line 40-42)
  - Description() returns window count + optional attached badge (line 46-54)
  - windowLabel() helper handles singular/plural correctly (line 21-26)
  - SessionDelegate implements list.ItemDelegate with Height()=1, Spacing()=0, Update()=nil (lines 60-66)
  - Render() formats cursor indicator, bold name, dimmed window count, green attached badge (lines 70-93)
  - ToListItems() converts []tmux.Session to []list.Item (lines 96-102)
  - Lipgloss styles defined as package-level vars matching spec (cursorStyle, nameStyle, detailStyle, attachedStyle) (lines 13-18)
  - All acceptance criteria met. Plan says lowercase `toListItems` but implementation exports as `ToListItems` -- correct since it is used from model.go

TESTS:
- Status: Adequate
- Coverage:
  - FilterValue returns session name -- covered (line 14-22)
  - Compiler check: list.Item interface -- covered (line 24-26)
  - Title returns session name -- covered (line 28-36)
  - Description with plural/singular/attached/detached -- covered via table-driven test (line 38-77)
  - Compiler check: list.ItemDelegate interface -- covered (line 81-83)
  - Height returns 1 -- covered (line 85-89)
  - Spacing returns 0 -- covered (line 93-98)
  - Update returns nil -- covered (line 101-109)
  - Render with name and window count -- covered (line 111-128)
  - Render singular window -- covered (line 130-147)
  - Render plural windows -- covered (line 149-163)
  - Render attached badge -- covered (line 165-179)
  - No attached badge for detached -- covered (line 181-195)
  - Highlights selected item (cursor ">") -- covered (line 197-222)
  - Long session name without truncation -- covered (line 224-239)
  - ToListItems conversion -- covered (line 242-271)
  - Empty/nil sessions -- covered (line 273-287)
- Notes:
  - Tests use idiomatic Go table-driven style for Description()
  - Render tests use strings.Contains which is appropriate for styled output (ANSI codes present)
  - Tests are external package (tui_test) which validates the public API
  - All 9 planned tests are present plus a few additional useful ones (empty/nil slices, Title, Height, Spacing, Update)
  - No over-testing -- additional tests cover distinct behaviors not already tested

CODE QUALITY:
- Project conventions: Followed -- table-driven tests, explicit error handling, exported functions documented
- SOLID principles: Good -- SessionItem has single responsibility (wrapping session as list item), SessionDelegate has single responsibility (rendering), windowLabel extracted as reusable helper
- Complexity: Low -- all functions are short and linear, no branching beyond simple if/else
- Modern idioms: Yes -- proper use of lipgloss for styling, io.Writer for render output, type assertion with ok check
- Readability: Good -- clear naming, doc comments on all exported types/functions, logical code organization
- Issues: None

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- The `windowLabel` function is unexported, which is fine for its current usage within the package. It was later extracted to be reused (Phase 4, tick-dad932), and tests for it exist in a separate file (window_label_test.go).
- The `_ = fmt.Fprint(w, line)` pattern (line 92) discards the write error. This is acceptable for TUI rendering where write failures to a buffer are non-actionable.
